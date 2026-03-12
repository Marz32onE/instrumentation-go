// Package otelwebsocket wraps github.com/gorilla/websocket and adds
// OpenTelemetry distributed-tracing support by propagating the W3C Trace
// Context inside the WebSocket message body.
//
// Tracer initialization: Set the global TracerProvider and TextMapPropagator at
// process startup (see example/) or pass WithTracerProvider/WithPropagators when
// creating a Conn. Defaults to otel.GetTracerProvider() and otel.GetTextMapPropagator().
//
// # How it works
//
// On the sender side, WriteMessage serialises the application payload into a
// small JSON envelope that also contains the current span's trace-context
// headers (e.g. "traceparent" and "tracestate").  On the receiver side,
// ReadMessage deserialises the envelope, re-creates the remote span context
// from those headers, and returns a new Go context that carries the
// propagated span so that the handler can create child spans that are
// correctly linked to the originating trace.
//
// # Usage
//
//	// Dialling side
//	raw, _, err := websocket.DefaultDialer.Dial(url, nil)
//	conn := otelwebsocket.NewConn(raw)
//	err = conn.WriteMessage(ctx, websocket.TextMessage, []byte("hello"))
//
//	// Upgrading side
//	raw, err := upgrader.Upgrade(w, r, nil)
//	conn := otelwebsocket.NewConn(raw)
//	ctx, msgType, data, err := conn.ReadMessage(context.Background())
package otelwebsocket

import (
	"context"
	"net/http"

	"github.com/gorilla/websocket"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// ScopeName is the instrumentation scope name for Tracer creation (OTel contrib guideline).
const ScopeName = "github.com/Marz32onE/otelwebsocket"

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

// WriteMessage encodes data together with the trace-context headers extracted
// from ctx and sends the resulting JSON envelope over the WebSocket
// connection.
//
// The messageType must be websocket.TextMessage or websocket.BinaryMessage;
// the encoded payload is always JSON text regardless of the original type,
// but the WebSocket frame type is preserved so that receivers can use the
// same type-switch logic they would use without this library.
func (c *Conn) WriteMessage(ctx context.Context, messageType int, data []byte) error {
	carrier := make(propagation.MapCarrier)
	c.propagator.Inject(ctx, carrier)

	encoded, err := marshalEnvelope(carrier, data)
	if err != nil {
		return err
	}
	return c.Conn.WriteMessage(messageType, encoded)
}

// ReadMessage reads the next envelope from the connection, extracts the
// trace-context headers embedded in it, and returns a new context that
// carries the remote span.  The returned context is derived from the
// provided parent ctx.
//
// The returned messageType, data, and error values have the same semantics
// as those of the underlying gorilla *websocket.Conn.ReadMessage.
func (c *Conn) ReadMessage(ctx context.Context) (context.Context, int, []byte, error) {
	msgType, raw, err := c.Conn.ReadMessage()
	if err != nil {
		return ctx, msgType, raw, err
	}

	env, err := unmarshalEnvelope(raw)
	if err != nil {
		// The message was not produced by this library; return it as-is so
		// that the application can still handle plain WebSocket messages.
		return ctx, msgType, raw, nil
	}

	carrier := propagation.MapCarrier(env.Headers)
	outCtx := c.propagator.Extract(ctx, carrier)

	return outCtx, msgType, env.Payload, nil
}

// Dial connects to the WebSocket server at the given URL and returns a
// *Conn with trace-context propagation enabled.  It is a thin wrapper
// around websocket.DefaultDialer.DialContext.
func Dial(ctx context.Context, urlStr string, requestHeader http.Header, opts ...Option) (*Conn, *http.Response, error) {
	raw, resp, err := websocket.DefaultDialer.DialContext(ctx, urlStr, requestHeader)
	if err != nil {
		return nil, resp, err
	}
	return NewConn(raw, opts...), resp, nil
}
