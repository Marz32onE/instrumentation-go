package otelmongo

import (
	"context"
	"net/url"
	"strconv"
	"strings"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	contribmongo "go.opentelemetry.io/contrib/instrumentation/go.mongodb.org/mongo-driver/mongo/otelmongo"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// Client wraps *mongo.Client with OpenTelemetry instrumentation.
// Tracer and propagator are read from otel globals (set via WithTracerProvider/WithPropagators at Connect).
type Client struct {
	*mongo.Client
	serverAddr string
	serverPort int
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

// ConnectWithOptions creates a Client. Passed-in TracerProvider/Propagators are set to otel globals; tracer/propagator are then read from globals.
// v1 driver requires context for Connect.
func ConnectWithOptions(ctx context.Context, traceOpts []ClientOption, opts ...*options.ClientOptions) (*Client, error) {
	cfg := newClientConfig(traceOpts)
	if cfg.TracerProvider != nil {
		otel.SetTracerProvider(cfg.TracerProvider)
	}
	if cfg.Propagators != nil {
		otel.SetTextMapPropagator(cfg.Propagators)
	}
	tp := otel.GetTracerProvider()
	monitor := contribmongo.NewMonitor(contribmongo.WithTracerProvider(tp))
	base := options.Client().SetMonitor(monitor)
	merged := options.MergeClientOptions(append(opts, base)...)
	mc, err := mongo.Connect(ctx, merged)
	if err != nil {
		return nil, err
	}
	// v1 ClientOptions does not expose GetURI(); server attributes are left empty when using Connect.
	addr, port := "", 0
	return &Client{Client: mc, serverAddr: addr, serverPort: port}, nil
}

// NewClient connects to MongoDB using uri and returns an instrumented Client.
// For custom TracerProvider/Propagators pass ClientOptions.
func NewClient(ctx context.Context, uri string, traceOpts ...ClientOption) (*Client, error) {
	client, err := ConnectWithOptions(ctx, traceOpts, options.Client().ApplyURI(uri))
	if err != nil {
		return nil, err
	}
	addr, port := parseServerFromURI(uri)
	client.serverAddr = addr
	client.serverPort = port
	return client, nil
}

// Disconnect disconnects the MongoDB client.
func (c *Client) Disconnect(ctx context.Context) error {
	return c.Client.Disconnect(ctx)
}

// Ping runs a ping command against the server. Driver-level contribmongo monitor
// instruments the command. Use readpref.Primary() or nil for default.
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

// Database returns a wrapped Database for document-level tracing (uses otel globals).
func (c *Client) Database(name string, opts ...*options.DatabaseOptions) *Database {
	return &Database{
		Database:   c.Client.Database(name, opts...),
		serverAddr: c.serverAddr,
		serverPort: c.serverPort,
	}
}
