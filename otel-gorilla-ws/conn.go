// Package otelgorillaws wraps github.com/gorilla/websocket and adds
// OpenTelemetry distributed-tracing support by propagating the W3C Trace
// Context inside the WebSocket message body via otel-ws subprotocol negotiation.
//
// Tracing is enabled only when both sides agree on the otel-ws subprotocol:
//   - Client: Dial injects "otel-ws" at the front of the proposed subprotocol list.
//     Tracing is enabled if the server responds with an "otel-ws+" prefixed protocol.
//   - Server: Upgrader.Upgrade detects "otel-ws" in the client's list and responds
//     with "otel-ws+<negotiated>". Tracing is enabled on acceptance.
//
// Connections without otel-ws negotiation operate in passthrough mode (no span
// creation, no envelope wrapping). NewConn keeps tracing always on for callers
// that manage the WebSocket handshake themselves (backwards compatibility).
//
// Tracer initialization: Set the global TracerProvider and TextMapPropagator at
// process startup (see example/) or pass WithTracerProvider/WithPropagators when
// creating a Conn. Defaults to otel.GetTracerProvider() and otel.GetTextMapPropagator().
package otelgorillaws

import (
	"context"
	"net/http"
	"strings"

	"github.com/gorilla/websocket"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// ScopeName is the instrumentation scope name for Tracer creation (OTel contrib guideline).
const ScopeName = "github.com/Marz32onE/instrumentation-go/otel-gorilla-ws"

// otelWSProtocol is the subprotocol token injected during the WebSocket handshake
// to negotiate otel-ws trace propagation support.
const otelWSProtocol = "otel-ws"

// Conn is a WebSocket connection with built-in OpenTelemetry trace-context
// propagation.  It embeds *websocket.Conn so that callers can still use all
// other gorilla/websocket methods directly.
type Conn struct {
	*websocket.Conn

	propagator     propagation.TextMapPropagator
	tracer         trace.Tracer
	tracingEnabled bool // true only after successful otel-ws subprotocol negotiation
}

// NewConn wraps an existing gorilla *websocket.Conn with tracing always enabled.
// This preserves backwards-compatible behaviour for callers that manage the
// WebSocket handshake themselves. For spec-compliant subprotocol negotiation,
// use Dial (client) or Upgrader.Upgrade (server).
func NewConn(conn *websocket.Conn, opts ...Option) *Conn {
	return newConn(conn, true, opts...)
}

// newConn is the internal constructor used by Dial and Upgrader.Upgrade.
func newConn(conn *websocket.Conn, tracingEnabled bool, opts ...Option) *Conn {
	c := &Conn{Conn: conn, tracingEnabled: tracingEnabled}
	applyOptions(c, opts)
	return c
}

// WriteMessage sends a message over the WebSocket connection.
// When tracing is enabled (otel-ws negotiated), it injects traceparent/tracestate
// into a wire envelope and creates a "websocket.send" producer span.
// When tracing is disabled, the message is sent as-is (passthrough).
func (c *Conn) WriteMessage(ctx context.Context, messageType int, data []byte) error {
	if !c.tracingEnabled {
		return c.Conn.WriteMessage(messageType, data)
	}

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

// ReadMessage reads the next message from the WebSocket connection.
// When tracing is enabled (otel-ws negotiated), it extracts traceparent/tracestate
// from the wire envelope and creates a "websocket.receive" consumer span linked
// to the sender span.
// When tracing is disabled, the raw message is returned as-is (passthrough).
func (c *Conn) ReadMessage(ctx context.Context) (context.Context, int, []byte, error) {
	msgType, raw, err := c.Conn.ReadMessage()
	if err != nil {
		return ctx, msgType, raw, err
	}

	if !c.tracingEnabled {
		return ctx, msgType, raw, nil
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

// Dial connects to the WebSocket server and returns a *Conn with trace
// propagation enabled only when the server supports otel-ws.
//
// If subprotocols is non-empty, "otel-ws" is injected at the front of the
// list during the WebSocket handshake. Tracing is enabled only if the server
// confirms otel-ws by returning a protocol with the "otel-ws+" prefix
// (Scenario G). If the server returns a non-otel protocol or no protocol at
// all, the connection operates in passthrough mode (Scenarios C and D).
//
// If subprotocols is nil or empty, no otel-ws injection is performed and the
// returned Conn operates in passthrough mode (Scenario E).
func Dial(ctx context.Context, urlStr string, requestHeader http.Header, subprotocols []string, opts ...Option) (*Conn, *http.Response, error) {
	var otelInjected bool
	dialProtos := subprotocols
	if len(subprotocols) > 0 {
		dialProtos = make([]string, 0, len(subprotocols)+1)
		dialProtos = append(dialProtos, otelWSProtocol)
		dialProtos = append(dialProtos, subprotocols...)
		otelInjected = true
	}

	dialer := websocket.Dialer{
		Subprotocols:     dialProtos,
		HandshakeTimeout: websocket.DefaultDialer.HandshakeTimeout,
		TLSClientConfig:  websocket.DefaultDialer.TLSClientConfig,
		Proxy:            websocket.DefaultDialer.Proxy,
	}

	raw, resp, err := dialer.DialContext(ctx, urlStr, requestHeader)
	if err != nil {
		return nil, resp, err
	}

	var tracingEnabled bool
	if otelInjected {
		negotiated := raw.Subprotocol()
		// Scenario C: server returned a non-otel app protocol → passthrough.
		// Scenario D: server returned no protocol → passthrough (connection kept alive).
		// Scenario G: server returned "otel-ws+<proto>" → tracing enabled.
		tracingEnabled = strings.HasPrefix(negotiated, otelWSProtocol+"+")
	}
	// Scenario E: otelInjected=false → tracingEnabled=false (passthrough).

	return newConn(raw, tracingEnabled, opts...), resp, nil
}
