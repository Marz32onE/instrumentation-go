package natstrace

import (
	"context"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

const (
	serviceNameKey    = attribute.Key("service.name")
	serviceVersionKey = attribute.Key("service.version")
	defaultVersion    = "0.0.0"
)

var (
	tracerInitialized bool
	globalShutdown    func()
	// cleanupOwner is used with runtime.AddCleanup so ShutdownTracer runs when the process tears down (best-effort; call defer ShutdownTracer() for guaranteed flush).
	cleanupOwner *struct{}
)

// defaultPropagator is the process-wide propagator (TraceContext + Baggage). Used by InitTracer.
var defaultPropagator = propagation.NewCompositeTextMapPropagator(
	propagation.TraceContext{},
	propagation.Baggage{},
)

// WithTracerProviderInit returns an InitTracer argument that uses the given TracerProvider
// instead of creating one from the endpoint. Intended for tests (e.g. with a SpanRecorder).
// For per-connection TracerProvider, use natstrace.ConnectWithOptions(..., natstrace.WithTracerProvider(tp)).
func WithTracerProviderInit(tp trace.TracerProvider) TracerProviderOption {
	return TracerProviderOption{TracerProvider: tp}
}

// TracerProviderOption holds a TracerProvider to use when passed to InitTracer.
type TracerProviderOption struct {
	TracerProvider trace.TracerProvider
}

// InitTracer sets the global TracerProvider and TextMapPropagator for the process.
// Call once at startup (e.g. in main) with OTLP endpoint and resource attributes.
// Propagator is fixed to TraceContext + Baggage. If the global provider is already
// an SDK TracerProvider (e.g. set by another package), InitTracer is a no-op.
// Empty endpoint uses OTEL_EXPORTER_OTLP_ENDPOINT env or "localhost:4317".
//
// For tests, pass WithTracerProviderInit(tp) as an extra argument to use a custom provider
// (e.g. one with tracetest.SpanRecorder) instead of creating an OTLP exporter.
func InitTracer(endpoint string, args ...interface{}) error {
	var attrs []attribute.KeyValue
	for _, a := range args {
		if opt, ok := a.(TracerProviderOption); ok {
			otel.SetTracerProvider(opt.TracerProvider)
			otel.SetTextMapPropagator(defaultPropagator)
			if sdkTP, ok := opt.TracerProvider.(*sdktrace.TracerProvider); ok {
				globalShutdown = func() {
					shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					_ = sdkTP.Shutdown(shutdownCtx)
				}
				cleanupOwner = &struct{}{}
				runtime.AddCleanup(cleanupOwner, func(struct{}) { ShutdownTracer() }, struct{}{})
			} else {
				globalShutdown = nil
			}
			tracerInitialized = true
			return nil
		}
		if kv, ok := a.(attribute.KeyValue); ok {
			attrs = append(attrs, kv)
			continue
		}
		if slice, ok := a.([]attribute.KeyValue); ok {
			attrs = append(attrs, slice...)
		}
	}

	if g := otel.GetTracerProvider(); g != nil {
		if _, ok := g.(*sdktrace.TracerProvider); ok {
			tracerInitialized = true
			return nil
		}
	}
	if endpoint == "" {
		endpoint = os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	}
	if endpoint == "" {
		endpoint = "localhost:4317"
	}
	useHTTP := useHTTPEndpoint(endpoint)
	ctx := context.Background()

	var exp sdktrace.SpanExporter
	var err error
	if useHTTP {
		exp, err = otlptracehttp.New(ctx,
			otlptracehttp.WithEndpoint(endpoint),
			otlptracehttp.WithInsecure(),
		)
	} else {
		exp, err = otlptracegrpc.New(ctx,
			otlptracegrpc.WithEndpoint(endpoint),
			otlptracegrpc.WithInsecure(),
		)
	}
	if err != nil {
		return err
	}

	attrs = ensureServiceNameAndVersion(attrs)
	res, err := resource.New(ctx, resource.WithAttributes(attrs...))
	if err != nil {
		return err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(defaultPropagator)

	globalShutdown = func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tp.Shutdown(shutdownCtx)
	}
	cleanupOwner = &struct{}{}
	runtime.AddCleanup(cleanupOwner, func(struct{}) { ShutdownTracer() }, struct{}{})
	tracerInitialized = true
	return nil
}

// ShutdownTracer shuts down the TracerProvider set by InitTracer and resets the global state.
// After ShutdownTracer, Connect must not be used until InitTracer is called again.
// The package also registers runtime.AddCleanup (Go 1.24+) so ShutdownTracer runs when the process tears down (best-effort).
// For guaranteed flush before exit, call "defer natstrace.ShutdownTracer()" in main.
func ShutdownTracer() {
	if globalShutdown != nil {
		globalShutdown()
		globalShutdown = nil
	}
	tracerInitialized = false
	otel.SetTracerProvider(noop.NewTracerProvider())
}

func useHTTPEndpoint(endpoint string) bool {
	s := strings.TrimSpace(endpoint)
	if s == "" {
		return false
	}
	if u, err := url.Parse(s); err == nil && (u.Scheme == "http" || u.Scheme == "https") {
		return true
	}
	_, port, err := splitHostPort(s)
	if err != nil {
		return false
	}
	p, _ := strconv.Atoi(port)
	return p == 4318
}

func splitHostPort(hostport string) (host, port string, err error) {
	u, err := url.Parse("//" + hostport)
	if err != nil {
		return "", "", err
	}
	return u.Hostname(), u.Port(), nil
}

func isTracerInitialized() bool {
	return tracerInitialized
}

// ensureServiceNameAndVersion adds service.name (UUID v4) and service.version ("0.0.0") if missing.
func ensureServiceNameAndVersion(attrs []attribute.KeyValue) []attribute.KeyValue {
	hasName, hasVersion := false, false
	for _, kv := range attrs {
		if kv.Key == serviceNameKey {
			hasName = true
		}
		if kv.Key == serviceVersionKey {
			hasVersion = true
		}
	}
	if !hasName {
		attrs = append(attrs, serviceNameKey.String(uuid.New().String()))
	}
	if !hasVersion {
		attrs = append(attrs, serviceVersionKey.String(defaultVersion))
	}
	return attrs
}
