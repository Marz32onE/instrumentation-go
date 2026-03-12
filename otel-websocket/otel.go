// Copyright 2024 The otelwebsocket Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package otelwebsocket

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
	cleanupOwner      *struct{}
)

var defaultPropagator = propagation.NewCompositeTextMapPropagator(
	propagation.TraceContext{},
	propagation.Baggage{},
)

// WithTracerProviderOption returns an InitTracer argument that uses the given TracerProvider
// (for tests). Do not confuse with Conn option WithTracerProvider.
func WithTracerProviderOption(tp trace.TracerProvider) TracerProviderInitOption {
	return TracerProviderInitOption{TracerProvider: tp}
}

// TracerProviderInitOption holds a TracerProvider to use when passed to InitTracer.
type TracerProviderInitOption struct {
	TracerProvider trace.TracerProvider
}

// InitTracer sets the global TracerProvider and TextMapPropagator for the process.
// Call once at startup (e.g. in main) with OTLP endpoint and resource attributes.
// Propagator is fixed to TraceContext + Baggage. Empty endpoint uses
// OTEL_EXPORTER_OTLP_ENDPOINT env or "localhost:4317".
//
// For tests, pass WithTracerProviderOption(tp) to use a custom provider.
func InitTracer(endpoint string, args ...interface{}) error {
	var attrs []attribute.KeyValue
	for _, a := range args {
		if opt, ok := a.(TracerProviderInitOption); ok {
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

// ShutdownTracer shuts down the TracerProvider set by InitTracer and resets global state.
// For guaranteed flush before exit, call "defer otelwebsocket.ShutdownTracer()" in main.
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
	_, port, err := splitHostPortOTLP(s)
	if err != nil {
		return false
	}
	p, _ := strconv.Atoi(port)
	return p == 4318
}

func splitHostPortOTLP(hostport string) (host, port string, err error) {
	u, err := url.Parse("//" + hostport)
	if err != nil {
		return "", "", err
	}
	return u.Hostname(), u.Port(), nil
}

func isTracerInitialized() bool {
	return tracerInitialized
}

// ensureTracer initializes the global tracer with defaults if not yet initialized.
// Called when creating a Conn with default options.
func ensureTracer() {
	if !isTracerInitialized() {
		_ = InitTracer("", nil)
	}
}

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
