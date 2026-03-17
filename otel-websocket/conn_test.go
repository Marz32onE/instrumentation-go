package otelwebsocket_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	otelwebsocket "github.com/Marz32onE/instrumentation-go/otel-websocket"
)

// setupOTel installs an in-memory trace exporter and W3C propagator so that
// tests can inspect exported spans without a real OTLP backend.
func setupOTel(t *testing.T) (*tracetest.SpanRecorder, trace.TracerProvider) {
	t.Helper()
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	t.Cleanup(func() {
		_ = tp.Shutdown(context.Background())
	})
	return rec, tp
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// newTestServer starts an httptest server that echoes every message back
// through an otelwebsocket.Conn so that both sending and receiving paths
// can be tested end-to-end.
func newTestServer(t *testing.T, opts ...otelwebsocket.Option) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("server upgrade: %v", err)
			return
		}
		conn := otelwebsocket.NewConn(raw, opts...)
		defer conn.Close()

		for {
			ctx, msgType, data, err := conn.ReadMessage(context.Background())
			if err != nil {
				// Closed by client – expected.
				return
			}
			// Echo: propagate extracted context back to the client.
			if err := conn.WriteMessage(ctx, msgType, data); err != nil {
				t.Errorf("server WriteMessage: %v", err)
				return
			}
		}
	}))
	return srv
}

// dialTestServer dials the httptest server and returns a wrapped Conn.
func dialTestServer(t *testing.T, srv *httptest.Server, opts ...otelwebsocket.Option) *otelwebsocket.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, _, err := otelwebsocket.Dial(context.Background(), url, nil, opts...)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

// TestTracePropagation verifies the core requirement: a span started on the
// client side is propagated to the server through the message body, and the
// server can use the extracted context to link its own work as a child.
func TestTracePropagation(t *testing.T) {
	_, tp := setupOTel(t)

	srv := newTestServer(t)
	defer srv.Close()

	clientConn := dialTestServer(t, srv)

	// Start a parent span on the client.
	tracer := tp.Tracer("test")
	ctx, span := tracer.Start(context.Background(), "client-send")
	defer span.End()

	const msg = "hello otel"
	if err := clientConn.WriteMessage(ctx, websocket.TextMessage, []byte(msg)); err != nil {
		t.Fatalf("WriteMessage: %v", err)
	}

	// Read the echo back; the context returned should carry the propagated
	// span context (same trace ID as the one we sent).
	recvCtx, msgType, data, err := clientConn.ReadMessage(context.Background())
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	if msgType != websocket.TextMessage {
		t.Errorf("messageType = %d, want %d", msgType, websocket.TextMessage)
	}
	if string(data) != msg {
		t.Errorf("payload = %q, want %q", data, msg)
	}

	// The span extracted from the echo should share the same trace ID.
	remoteSpan := trace.SpanFromContext(recvCtx)
	if !remoteSpan.SpanContext().IsValid() {
		t.Fatal("no valid span context extracted from received message")
	}
	if remoteSpan.SpanContext().TraceID() != span.SpanContext().TraceID() {
		t.Errorf("trace ID mismatch: got %s, want %s",
			remoteSpan.SpanContext().TraceID(),
			span.SpanContext().TraceID(),
		)
	}
}

// TestReadMessagePlain verifies that ReadMessage gracefully handles a raw
// (non-envelope) message that was not written by this library.
func TestReadMessagePlain(t *testing.T) {
	setupOTel(t)

	// Set up a server that sends a plain, non-envelope WebSocket message.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer raw.Close()
		// Send a raw (non-envelope) message.
		_ = raw.WriteMessage(websocket.TextMessage, []byte("plain message"))
		// Wait for client to close.
		for {
			if _, _, err := raw.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	raw, _, err := websocket.DefaultDialer.DialContext(context.Background(), url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer raw.Close()
	conn := otelwebsocket.NewConn(raw)

	ctx, _, data, err := conn.ReadMessage(context.Background())
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	// The raw bytes should be returned unchanged.
	if string(data) != "plain message" {
		t.Errorf("data = %q, want %q", data, "plain message")
	}
	// Context should not carry a valid remote span (no trace headers).
	remoteSpan := trace.SpanFromContext(ctx)
	if remoteSpan.SpanContext().IsValid() {
		t.Error("expected no valid span context for plain message")
	}
}

// TestWriteMessageNoSpan verifies that WriteMessage works when the context
// does not carry an active span – it simply sends an envelope with empty
// headers.
func TestWriteMessageNoSpan(t *testing.T) {
	setupOTel(t)

	srv := newTestServer(t)
	defer srv.Close()

	conn := dialTestServer(t, srv)

	const msg = "no span"
	if err := conn.WriteMessage(context.Background(), websocket.BinaryMessage, []byte(msg)); err != nil {
		t.Fatalf("WriteMessage: %v", err)
	}

	_, msgType, data, err := conn.ReadMessage(context.Background())
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	if msgType != websocket.BinaryMessage {
		t.Errorf("messageType = %d, want %d", msgType, websocket.BinaryMessage)
	}
	if string(data) != msg {
		t.Errorf("data = %q, want %q", data, msg)
	}
}

// TestWithPropagatorsOption verifies that a custom propagator supplied via
// WithPropagators is honoured.
func TestWithPropagatorsOption(t *testing.T) {
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	prop := propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
	)

	opts := []otelwebsocket.Option{
		otelwebsocket.WithPropagators(prop),
		otelwebsocket.WithTracerProvider(tp),
	}

	srv := newTestServer(t, opts...)
	defer srv.Close()

	conn := dialTestServer(t, srv, opts...)

	tracer := tp.Tracer("test-custom-prop")
	ctx, span := tracer.Start(context.Background(), "custom-prop-span")
	defer span.End()

	if err := conn.WriteMessage(ctx, websocket.TextMessage, []byte("with custom prop")); err != nil {
		t.Fatalf("WriteMessage: %v", err)
	}

	recvCtx, _, _, err := conn.ReadMessage(context.Background())
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}

	remoteSpan := trace.SpanFromContext(recvCtx)
	if !remoteSpan.SpanContext().IsValid() {
		t.Fatal("no valid span context extracted with custom propagator")
	}
	if remoteSpan.SpanContext().TraceID() != span.SpanContext().TraceID() {
		t.Errorf("trace ID mismatch with custom propagator: got %s, want %s",
			remoteSpan.SpanContext().TraceID(),
			span.SpanContext().TraceID(),
		)
	}
}
