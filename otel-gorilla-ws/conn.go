// Package otelgorillaws wraps github.com/gorilla/websocket and adds
// OpenTelemetry distributed-tracing support by propagating the W3C Trace
// Context inside the WebSocket message body.
//
// Tracer initialization: Set the global TracerProvider and TextMapPropagator at
// process startup (see example/) or pass WithTracerProvider/WithPropagators when
// creating a Conn. Defaults to otel.GetTracerProvider() and otel.GetTextMapPropagator().
//
// # How it works
//
// On the sender side, WriteMessage serialises the application payload as
// header-style envelope ({ "headers", "payload" }). On the receiver side,
// ReadMessage accepts both header-style envelope and the legacy embedded form
// ({ "traceparent", "tracestate", "data" }), extracts trace context, and
// returns a context carrying the propagated span.
package otelgorillaws

import (
	"context"
	"net/http"

	"github.com/gorilla/websocket"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// ScopeName is the instrumentation scope name for Tracer creation (OTel contrib guideline).
const ScopeName = "github.com/Marz32onE/instrumentation-go/otel-gorilla-ws"

// Conn is a WebSocket connection with built-in OpenTelemetry trace-context
// propagation.  It embeds *websocket.Conn so that callers can still use all
// other gorilla/websocket methods directly.
type Conn struct {
	*websocket.Conn

	propagator propagation.TextMapPropagator
	tracer     trace.Tracer
}

// NewConn wraps an existing gorilla *websocket.Conn.  Any number of Option
// values may be provided to customise the propagator or tracer provider.
func NewConn(conn *websocket.Conn, opts ...Option) *Conn {
	c := &Conn{Conn: conn}
	applyOptions(c, opts)
	return c
}

// WriteMessage encodes data together with trace-context headers extracted from
// ctx and sends a header-style JSON envelope over the WebSocket connection.
// Creates a "websocket.send" producer span so the send is visible in traces.
func (c *Conn) WriteMessage(ctx context.Context, messageType int, data []byte) error {
	ctx, span := c.tracer.Start(ctx, "websocket.send",
		trace.WithSpanKind(trace.SpanKindProducer),
		trace.WithAttributes(
			attribute.Int("websocket.message.type", messageType),
			attribute.Int("messaging.message.body.size", len(data)),
		),
	)
	defer span.End()

	carrier := make(propagation.MapCarrier)
	c.propagator.Inject(ctx, carrier)

	encoded, err := marshalWire(carrier, data)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	if err := c.Conn.WriteMessage(messageType, encoded); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

// ReadMessage reads the next instrumented message (embedded or header-style
// envelope), extracts trace context, and returns a new context that carries
// the remote span. Creates a "websocket.receive" consumer span linked to the sender span.
func (c *Conn) ReadMessage(ctx context.Context) (context.Context, int, []byte, error) {
	msgType, raw, err := c.Conn.ReadMessage()
	if err != nil {
		return ctx, msgType, raw, err
	}

	payload, hdrs, ok := tryUnmarshalWire(raw)
	if !ok {
		return ctx, msgType, raw, nil
	}

	carrier := propagation.MapCarrier(hdrs)
	senderCtx := c.propagator.Extract(ctx, carrier)

	startOpts := []trace.SpanStartOption{
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(
			attribute.Int("websocket.message.type", msgType),
			attribute.Int("messaging.message.body.size", len(payload)),
		),
	}
	if sc := trace.SpanContextFromContext(senderCtx); sc.IsValid() {
		startOpts = append(startOpts, trace.WithLinks(trace.Link{SpanContext: sc}))
	}
	outCtx, span := c.tracer.Start(senderCtx, "websocket.receive", startOpts...)
	span.End()

	return outCtx, msgType, payload, nil
}

// Dial connects to the WebSocket server and returns a trace-enabled *Conn.
func Dial(ctx context.Context, urlStr string, requestHeader http.Header, opts ...Option) (*Conn, *http.Response, error) {
	raw, resp, err := websocket.DefaultDialer.DialContext(ctx, urlStr, requestHeader)
	if err != nil {
		return nil, resp, err
	}
	return NewConn(raw, opts...), resp, nil
}
