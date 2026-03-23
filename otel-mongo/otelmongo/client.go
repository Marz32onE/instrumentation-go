package otelmongo

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
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
// Signature aligns with mongo.Connect(ctx, opts ...*options.ClientOptions). TracerProvider and Propagators default to global.
func Connect(ctx context.Context, opts ...*options.ClientOptions) (*Client, error) {
	return ConnectWithOptions(ctx, nil, opts...)
}

// ConnectWithOptions creates a Client. Passed-in TracerProvider/Propagators are set to otel globals;
// tracer/propagator are then read from globals. Call otel.SetTracerProvider and otel.SetTextMapPropagator
// at process startup as an alternative to passing options here.
func ConnectWithOptions(ctx context.Context, traceOpts []ClientOption, opts ...*options.ClientOptions) (*Client, error) {
	cfg := newClientConfig(traceOpts)
	if cfg.TracerProvider != nil {
		otel.SetTracerProvider(cfg.TracerProvider)
	}
	if cfg.Propagators != nil {
		otel.SetTextMapPropagator(cfg.Propagators)
	}
	merged := options.MergeClientOptions(opts...)
	mc, err := mongo.Connect(ctx, merged)
	if err != nil {
		return nil, err
	}
	addr, port := parseServerFromClientOptions(merged)
	mongoTP, deliverTracer := initMongoProvider(addr, port)
	return &Client{
		Client:        mc,
		serverAddr:    addr,
		serverPort:    port,
		mongoTP:       mongoTP,
		deliverTracer: deliverTracer,
	}, nil
}

func parseServerFromClientOptions(opts *options.ClientOptions) (addr string, port int) {
	if opts == nil {
		return "", 0
	}
	return parseServerFromURI(opts.GetURI())
}

// NewClient connects to MongoDB using uri and returns an instrumented Client.
// For custom TracerProvider/Propagators pass traceOpts.
func NewClient(ctx context.Context, uri string, traceOpts ...ClientOption) (*Client, error) {
	return ConnectWithOptions(ctx, traceOpts, options.Client().ApplyURI(uri))
}

// Disconnect disconnects the MongoDB client and shuts down the deliver TracerProvider if active.
func (c *Client) Disconnect(ctx context.Context) error {
	err := c.Client.Disconnect(ctx)
	if c.mongoTP != nil {
		shutCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = c.mongoTP.Shutdown(shutCtx)
	}
	return err
}

// Ping runs a ping command against the server. Use readpref.Primary() or nil for default.
func (c *Client) Ping(ctx context.Context, rp *readpref.ReadPref) error {
	return c.Client.Ping(ctx, rp)
}

// StartSession starts a new session. Operations executed with the session
// should use this client's Database/Collection so document-level tracing applies.
func (c *Client) StartSession(opts ...*options.SessionOptions) (mongo.Session, error) {
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
func initMongoProvider(addr string, port int) (*sdktrace.TracerProvider, trace.Tracer) {
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		return nil, nil
	}
	ctx := context.Background()
	useHTTP := useHTTPEndpoint(endpoint)

	var exp sdktrace.SpanExporter
	var err error
	if useHTTP {
		exp, err = otlptracehttp.New(ctx,
			otlptracehttp.WithEndpointURL(otlpHTTPExporterURL(endpoint)),
			otlptracehttp.WithInsecure(),
		)
	} else {
		exp, err = otlptracegrpc.New(ctx,
			otlptracegrpc.WithEndpoint(otlpGRPCExporterEndpoint(endpoint)),
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

// useHTTPEndpoint chooses OTLP/HTTP vs gRPC. Env without scheme and without port defaults to HTTP.
func useHTTPEndpoint(endpoint string) bool {
	s := strings.TrimSpace(endpoint)
	if s == "" {
		return false
	}
	if u, err := url.Parse(s); err == nil && (u.Scheme == "http" || u.Scheme == "https") {
		return true
	}
	if u, err := url.Parse("//" + s); err == nil {
		if u.Port() == "" {
			return true
		}
		if p, _ := strconv.Atoi(u.Port()); p == 4318 {
			return true
		}
	}
	return false
}

func otlpHTTPExporterURL(endpoint string) string {
	s := strings.TrimSpace(endpoint)
	if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") {
		return s
	}
	u, err := url.Parse("//" + s)
	if err != nil || u.Hostname() == "" {
		return "http://" + s
	}
	if p := u.Port(); p != "" {
		return "http://" + net.JoinHostPort(u.Hostname(), p)
	}
	return "http://" + net.JoinHostPort(u.Hostname(), "4318")
}

func otlpGRPCExporterEndpoint(endpoint string) string {
	s := strings.TrimSpace(endpoint)
	u, err := url.Parse("//" + s)
	if err != nil || u.Hostname() == "" {
		return s
	}
	if p := u.Port(); p != "" {
		return net.JoinHostPort(u.Hostname(), p)
	}
	return net.JoinHostPort(u.Hostname(), "4317")
}

// Database returns a wrapped Database for document-level tracing (uses otel globals).
func (c *Client) Database(name string, opts ...*options.DatabaseOptions) *Database {
	return &Database{
		Database:      c.Client.Database(name, opts...),
		serverAddr:    c.serverAddr,
		serverPort:    c.serverPort,
		deliverTracer: c.deliverTracer,
	}
}
