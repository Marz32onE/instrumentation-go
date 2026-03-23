package otelnats

import (
	"context"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	nats "github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.27.0"
	"go.opentelemetry.io/otel/trace"
)

const (
	// ScopeName is the instrumentation scope name for Tracer creation (OTel contrib guideline).
	ScopeName              = "github.com/Marz32onE/instrumentation-go/otel-nats/otelnats"
	instrumentationVersion = "0.1.3"
	messagingSystem        = "nats"
)

// MsgWithContext carries a message and the context with extracted trace (Subscribe/QueueSubscribe).
// Use m.Msg for the message and m.Context() for the trace context. Naming aligns with oteljetstream.MsgWithContext.
type MsgWithContext struct {
	Msg *nats.Msg
	Ctx context.Context
}

// Context returns the context with extracted trace.
func (m MsgWithContext) Context() context.Context { return m.Ctx }

// MsgHandler is the callback for subscriptions. Same as nats.MsgHandler but receives MsgWithContext
// (trace in m.Context(), message in m.Msg). Type name matches nats.MsgHandler.
type MsgHandler func(m MsgWithContext)

// Conn is a tracing-aware wrapper around *nats.Conn. API mirrors nats.Conn; the only
// difference is Publish/PublishMsg take context.Context and handlers receive MsgWithContext.
type Conn struct {
	nc            *nats.Conn
	tracer        trace.Tracer
	propagator    propagation.TextMapPropagator
	serverAttrs   []attribute.KeyValue
	deliverTracer trace.Tracer             // NATS deliver span tracer (nil when disabled)
	natsTP        *sdktrace.TracerProvider // independent TracerProvider for NATS service (nil when disabled)
}

// Option configures Conn. Per OTel contrib: accept TracerProvider and Propagators, not Tracer.
type Option interface {
	apply(*connConfig)
}

type optionFunc func(*connConfig)

func (f optionFunc) apply(c *connConfig) { f(c) }

type connConfig struct {
	TracerProvider trace.TracerProvider
	Propagators    propagation.TextMapPropagator
}

func newConnConfig(opts ...Option) *connConfig {
	c := &connConfig{}
	for _, o := range opts {
		o.apply(c)
	}
	return c
}

// WithTracerProvider sets the TracerProvider for this Conn. Defaults to otel.GetTracerProvider().
func WithTracerProvider(tp trace.TracerProvider) Option {
	return optionFunc(func(c *connConfig) {
		if tp != nil {
			c.TracerProvider = tp
		}
	})
}

// WithPropagators sets the TextMapPropagator for inject/extract. Defaults to otel.GetTextMapPropagator().
func WithPropagators(p propagation.TextMapPropagator) Option {
	return optionFunc(func(c *connConfig) {
		if p != nil {
			c.Propagators = p
		}
	})
}

// Version returns the instrumentation module version for tracer creation (OTel contrib guideline).
func Version() string {
	return instrumentationVersion
}

func newConn(nc *nats.Conn, opts ...Option) *Conn {
	cfg := newConnConfig(opts...)
	if cfg.TracerProvider == nil {
		cfg.TracerProvider = otel.GetTracerProvider()
	}
	if cfg.Propagators == nil {
		cfg.Propagators = otel.GetTextMapPropagator()
	}
	tracer := cfg.TracerProvider.Tracer(ScopeName, trace.WithInstrumentationVersion(Version()), trace.WithSchemaURL(semconv.SchemaURL))
	serverAttrs := serverAttrsFromConn(nc)
	c := &Conn{
		nc:          nc,
		tracer:      tracer,
		propagator:  cfg.Propagators,
		serverAttrs: serverAttrs,
	}
	natsTP, deliverTracer := initNATSProvider(nc.ConnectedAddr())
	c.natsTP = natsTP
	c.deliverTracer = deliverTracer
	return c
}

// serverAttrsFromConn parses the connected NATS server address into server.address / server.port attributes.
// The default port 4222 is omitted (consistent with otel-mongo omitting 27017).
func serverAttrsFromConn(nc *nats.Conn) []attribute.KeyValue {
	addr := nc.ConnectedAddr()
	if addr == "" {
		return nil
	}
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return []attribute.KeyValue{attribute.String("server.address", addr)}
	}
	attrs := []attribute.KeyValue{attribute.String("server.address", host)}
	if port, err := strconv.Atoi(portStr); err == nil && port > 0 && port != 4222 {
		attrs = append(attrs, attribute.Int("server.port", port))
	}
	return attrs
}

// initNATSProvider creates an independent TracerProvider with service.name = "nats://{addr}"
// for synthetic deliver spans. Only enabled when OTEL_EXPORTER_OTLP_ENDPOINT is set.
func initNATSProvider(connectedAddr string) (*sdktrace.TracerProvider, trace.Tracer) {
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
		return nil, nil
	}

	serviceName := "nats://" + connectedAddr
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
	tracer := tp.Tracer(ScopeName, trace.WithInstrumentationVersion(Version()), trace.WithSchemaURL(semconv.SchemaURL))
	return tp, tracer
}

// useHTTPEndpoint detects whether endpoint is HTTP (explicit http(s) scheme, port 4318,
// or host-only with no port — defaults to HTTP OTLP).
func useHTTPEndpoint(endpoint string) bool {
	s := strings.TrimSpace(endpoint)
	if s == "" {
		return false
	}
	if u, err := url.Parse(s); err == nil && (u.Scheme == "http" || u.Scheme == "https") {
		return true
	}
	if u, err := url.Parse("//" + s); err == nil {
		portStr := u.Port()
		if portStr == "" {
			return true
		}
		if p, _ := strconv.Atoi(portStr); p == 4318 {
			return true
		}
	}
	return false
}

// StartDeliverSpan creates a synthetic messaging span for NATS broker delivery using
// SpanKindConsumer (not INTERNAL). The returned context contains the deliver span's trace context,
// suitable for injecting into message headers so consumers link to the deliver span.
// If deliver spans are disabled (no OTEL_EXPORTER_OTLP_ENDPOINT), returns ctx unchanged.
func (c *Conn) StartDeliverSpan(ctx context.Context, subject string) context.Context {
	if c.deliverTracer == nil {
		return ctx
	}
	attrs := make([]attribute.KeyValue, 0, 2+len(c.serverAttrs))
	attrs = append(attrs,
		semconv.MessagingSystemKey.String(messagingSystem),
		semconv.MessagingDestinationNameKey.String(subject),
	)
	attrs = append(attrs, c.serverAttrs...)
	deliverCtx, span := c.deliverTracer.Start(ctx, subject+" deliver",
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(attrs...),
	)
	span.End()
	return deliverCtx
}

// DeliverSpanEnabled reports whether the NATS deliver span feature is active.
func (c *Conn) DeliverSpanEnabled() bool { return c.deliverTracer != nil }

// ServerAttrs returns the pre-built server.address / server.port attributes for this connection.
func (c *Conn) ServerAttrs() []attribute.KeyValue { return c.serverAttrs }

// TraceContext returns the tracer and propagator used by this Conn. Used by oteljetstream.
func (c *Conn) TraceContext() (trace.Tracer, propagation.TextMapPropagator) {
	return c.tracer, c.propagator
}

// NatsConn returns the underlying *nats.Conn (same as nats package).
func (c *Conn) NatsConn() *nats.Conn {
	return c.nc
}

// Close closes the connection (same as nats.Conn.Close).
func (c *Conn) Close() {
	c.nc.Close()
	if c.natsTP != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = c.natsTP.Shutdown(ctx)
	}
}

// Drain flushes and closes (same as nats.Conn.Drain).
func (c *Conn) Drain() error {
	err := c.nc.Drain()
	if c.natsTP != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = c.natsTP.Shutdown(ctx)
	}
	return err
}

// Publish publishes data to subject. Same as nats.Conn.Publish but accepts context for trace.
func (c *Conn) Publish(ctx context.Context, subject string, data []byte) error {
	msg := &nats.Msg{
		Subject: subject,
		Data:    data,
		Header:  make(nats.Header),
	}
	return c.PublishMsg(ctx, msg)
}

// PublishMsg publishes the message. Same as nats.Conn.PublishMsg but accepts context for trace.
// Per OTel messaging semconv: "Send" span with creation context injected into message; consumer
// spans link to this context. Span name is "{operation.name} {destination}".
func (c *Conn) PublishMsg(ctx context.Context, msg *nats.Msg) error {
	if msg.Header == nil {
		msg.Header = make(nats.Header)
	}
	spanName := "send " + msg.Subject
	ctx, span := c.tracer.Start(ctx, spanName,
		trace.WithSpanKind(trace.SpanKindProducer),
		trace.WithAttributes(publishAttrs(msg, c.serverAttrs)...),
	)
	defer span.End()
	injectCtx := ctx
	if c.deliverTracer != nil {
		injectCtx = c.StartDeliverSpan(ctx, msg.Subject)
	}
	c.propagator.Inject(injectCtx, &HeaderCarrier{H: msg.Header})
	if err := c.nc.PublishMsg(msg); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

// Request sends a request and waits for reply. Same as nats.Conn.Request but accepts context.
// The timeout parameter is applied to the request; the call returns when the reply is received or timeout is reached.
func (c *Conn) Request(ctx context.Context, subject string, data []byte, timeout time.Duration) (*nats.Msg, error) {
	msg := &nats.Msg{
		Subject: subject,
		Data:    data,
		Header:  make(nats.Header),
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	spanName := "send " + subject
	reqCtx, span := c.tracer.Start(reqCtx, spanName,
		trace.WithSpanKind(trace.SpanKindProducer),
		trace.WithAttributes(publishAttrs(msg, c.serverAttrs)...),
	)
	defer span.End()
	injectCtx := reqCtx
	if c.deliverTracer != nil {
		injectCtx = c.StartDeliverSpan(reqCtx, msg.Subject)
	}
	c.propagator.Inject(injectCtx, &HeaderCarrier{H: msg.Header})
	reply, err := c.nc.RequestMsgWithContext(reqCtx, msg)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	span.SetAttributes(attribute.Int(string(semconv.MessagingMessageBodySizeKey), len(reply.Data)))
	return reply, nil
}

// Subscribe subscribes to subject. Handler receives MsgWithContext (m.Msg, m.Context()).
func (c *Conn) Subscribe(subject string, handler MsgHandler) (*nats.Subscription, error) {
	return c.nc.Subscribe(subject, c.wrapHandler(subject, "", handler))
}

// QueueSubscribe is the queue-group variant. Handler receives MsgWithContext.
func (c *Conn) QueueSubscribe(subject, queue string, handler MsgHandler) (*nats.Subscription, error) {
	return c.nc.QueueSubscribe(subject, queue, c.wrapHandler(subject, queue, handler))
}

func (c *Conn) wrapHandler(subject, queue string, handler MsgHandler) nats.MsgHandler {
	return func(msg *nats.Msg) {
		msgCtx := c.propagator.Extract(context.Background(), &HeaderCarrier{H: msg.Header})
		// Per OTel messaging semconv: correlate producer and consumer only via span link (no parent-child).
		spanName := "process " + subject
		opts := []trace.SpanStartOption{
			trace.WithSpanKind(trace.SpanKindConsumer),
			trace.WithAttributes(receiveAttrs(msg, queue, "process", c.serverAttrs)...),
		}
		if sc := trace.SpanContextFromContext(msgCtx); sc.IsValid() {
			opts = append(opts, trace.WithLinks(trace.LinkFromContext(msgCtx)))
		}
		ctx, span := c.tracer.Start(context.Background(), spanName, opts...)
		defer span.End()
		handler(MsgWithContext{Msg: msg, Ctx: ctx})
	}
}

func publishAttrs(msg *nats.Msg, serverAttrs []attribute.KeyValue) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		semconv.MessagingSystemKey.String(messagingSystem),
		semconv.MessagingDestinationNameKey.String(msg.Subject),
		attribute.String(string(semconv.MessagingOperationTypeKey), "send"),
		semconv.MessagingOperationNameKey.String("publish"),
	}
	if len(msg.Data) > 0 {
		attrs = append(attrs, semconv.MessagingMessageBodySize(len(msg.Data)))
	}
	if msg.Reply != "" {
		attrs = append(attrs, semconv.MessagingMessageConversationID(msg.Reply))
	}
	attrs = append(attrs, serverAttrs...)
	return attrs
}

// receiveAttrs builds consumer span attributes. opType is "process" (push) or "receive" (pull).
// Note: oteljetstream/consumer.go has a parallel receiveAttrs for jetstream.Msg — keep both in sync.
func receiveAttrs(msg *nats.Msg, queue string, opType string, serverAttrs []attribute.KeyValue) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		semconv.MessagingSystemKey.String(messagingSystem),
		semconv.MessagingDestinationNameKey.String(msg.Subject),
		attribute.String(string(semconv.MessagingOperationTypeKey), opType),
		semconv.MessagingOperationNameKey.String(opType),
	}
	if len(msg.Data) > 0 {
		attrs = append(attrs, semconv.MessagingMessageBodySize(len(msg.Data)))
	}
	if queue != "" {
		attrs = append(attrs, semconv.MessagingConsumerGroupName(queue))
	}
	attrs = append(attrs, serverAttrs...)
	return attrs
}
