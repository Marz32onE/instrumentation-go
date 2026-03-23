package otelmongo

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.27.0"
	"go.opentelemetry.io/otel/trace"
)

// Client wraps *mongo.Client with OpenTelemetry instrumentation.
// Tracer and propagator are read from otel globals (set via WithTracerProvider/WithPropagators at Connect).
type Client struct {
	*mongo.Client
	serverAddr    string
	serverPort    int
	deliverTracer trace.Tracer             // MongoDB deliver span tracer (nil when disabled)
	mongoTP       *sdktrace.TracerProvider // independent TracerProvider (nil when disabled)
}

// ClientOption configures Connect/NewClient. Per OTel contrib: accept TracerProvider and Propagators.
type ClientOption interface {
	apply(*clientConfig)
}

type clientOptionFunc func(*clientConfig)

func (f clientOptionFunc) apply(c *clientConfig) { f(c) }

type clientConfig struct {
	TracerProvider trace.TracerProvider
	Propagators    propagation.TextMapPropagator
}

// WithTracerProvider sets the TracerProvider for the client. Defaults to otel.GetTracerProvider().
func WithTracerProvider(tp trace.TracerProvider) ClientOption {
	return clientOptionFunc(func(c *clientConfig) {
		if tp != nil {
			c.TracerProvider = tp
		}
	})
}

// WithPropagators sets the TextMapPropagator. Defaults to otel.GetTextMapPropagator().
func WithPropagators(p propagation.TextMapPropagator) ClientOption {
	return clientOptionFunc(func(c *clientConfig) {
		if p != nil {
			c.Propagators = p
		}
	})
}

func newClientConfig(opts []ClientOption) *clientConfig {
	cfg := &clientConfig{}
	for _, o := range opts {
		o.apply(cfg)
	}
	return cfg
}

// Connect creates a new Client with the given configuration options, with OpenTelemetry instrumentation.
// Signature aligns with mongo.Connect(opts ...*options.ClientOptions). TracerProvider and Propagators default to global.
// Set them at process startup (see example/) or pass WithTracerProvider/WithPropagators via ConnectWithOptions.
func Connect(opts ...*options.ClientOptions) (*Client, error) {
	return ConnectWithOptions(nil, opts...)
}

// ConnectWithOptions creates a Client. Passed-in TracerProvider/Propagators are set to otel globals; tracer/propagator are then read from globals.
func ConnectWithOptions(traceOpts []ClientOption, opts ...*options.ClientOptions) (*Client, error) {
	cfg := newClientConfig(traceOpts)
	if cfg.TracerProvider != nil {
		otel.SetTracerProvider(cfg.TracerProvider)
	}
	if cfg.Propagators != nil {
		otel.SetTextMapPropagator(cfg.Propagators)
	}
	merged := options.MergeClientOptions(opts...)
	mc, err := mongo.Connect(merged)
	if err != nil {
		return nil, err
	}
	addr, port := parseServerFromURI(merged.GetURI())
	mongoTP, deliverTracer := initMongoProvider(addr, port)
	return &Client{Client: mc, serverAddr: addr, serverPort: port, mongoTP: mongoTP, deliverTracer: deliverTracer}, nil
}

// NewClient connects to MongoDB using uri and returns an instrumented Client.
// For custom TracerProvider/Propagators pass ClientOptions.
func NewClient(uri string, traceOpts ...ClientOption) (*Client, error) {
	return ConnectWithOptions(traceOpts, options.Client().ApplyURI(uri))
}

// Disconnect disconnects the MongoDB client and shuts down the deliver TracerProvider if active.
func (c *Client) Disconnect(ctx context.Context) error {
	err := c.Client.Disconnect(ctx)
	if c.mongoTP != nil {
		shutCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = c.mongoTP.Shutdown(shutCtx) // best-effort; deliver spans may be lost on failure
	}
	return err
}

// Ping runs a ping command against the server. Use readpref.Primary() or nil for default.
func (c *Client) Ping(ctx context.Context, rp *readpref.ReadPref) error {
	return c.Client.Ping(ctx, rp)
}

// StartSession starts a new session. Operations executed with the session
// should use this client's Database/Collection so document-level tracing applies.
func (c *Client) StartSession(opts ...options.Lister[options.SessionOptions]) (*mongo.Session, error) {
	return c.Client.StartSession(opts...)
}

// parseServerFromURI extracts server address and port from a MongoDB URI for semconv server.* attributes.
// Uses the first host when the URI has multiple hosts (e.g. replica set). Handles IPv6 (e.g. [::1]).
// Returns ("", 0) on parse failure or when the URI has no host (e.g. some SRV forms).
func parseServerFromURI(uri string) (addr string, port int) {
	u, err := url.Parse(uri)
	if err != nil || u.Host == "" {
		return "", 0
	}
	firstHost := u.Host
	if i := strings.Index(u.Host, ","); i >= 0 {
		firstHost = strings.TrimSpace(u.Host[:i])
	}
	u2, err := url.Parse("//" + firstHost)
	if err != nil {
		return "", 0
	}
	addr = u2.Hostname()
	if addr == "" {
		return "", 0
	}
	portStr := u2.Port()
	if portStr == "" {
		port = 27017
		return addr, port
	}
	p, _ := strconv.Atoi(portStr)
	if p <= 0 {
		p = 27017
	}
	return addr, p
}

// initMongoProvider creates an independent TracerProvider with service.name = "mongodb://{addr}"
// for synthetic deliver spans. Only enabled when OTEL_EXPORTER_OTLP_ENDPOINT is set.
// The endpoint must be a full URL (e.g. "http://otel-collector:4318") for HTTP,
// or a host:port (e.g. "otel-collector:4317") for gRPC.
func initMongoProvider(addr string, port int) (*sdktrace.TracerProvider, trace.Tracer) {
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		return nil, nil
	}
	ctx := context.Background()

	var exp sdktrace.SpanExporter
	var err error
	if strings.HasPrefix(endpoint, "http://") || strings.HasPrefix(endpoint, "https://") {
		exp, err = otlptracehttp.New(ctx,
			otlptracehttp.WithEndpointURL(endpoint),
			otlptracehttp.WithInsecure(),
		)
	} else {
		exp, err = otlptracegrpc.New(ctx,
			otlptracegrpc.WithEndpoint(endpoint),
			otlptracegrpc.WithInsecure(),
		)
	}
	if err != nil {
		return nil, nil
	}

	serviceName := mongoServiceName(addr, port)
	res, err := resource.New(ctx, resource.WithAttributes(
		semconv.ServiceName(serviceName),
	))
	if err != nil {
		_ = exp.Shutdown(ctx) // avoid leaking the exporter connection
		return nil, nil
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
	)
	tracer := tp.Tracer(ScopeName, trace.WithInstrumentationVersion(Version()))
	return tp, tracer
}

// mongoServiceName returns the service.name for the MongoDB deliver span TracerProvider.
func mongoServiceName(addr string, port int) string {
	if addr == "" {
		return "mongodb"
	}
	if port > 0 && port != 27017 {
		return fmt.Sprintf("mongodb://%s:%d", addr, port)
	}
	return "mongodb://" + addr
}

// Database returns a wrapped Database for document-level tracing (uses otel globals).
func (c *Client) Database(name string, opts ...options.Lister[options.DatabaseOptions]) *Database {
	return &Database{
		Database:      c.Client.Database(name, opts...),
		serverAddr:    c.serverAddr,
		serverPort:    c.serverPort,
		deliverTracer: c.deliverTracer,
	}
}
