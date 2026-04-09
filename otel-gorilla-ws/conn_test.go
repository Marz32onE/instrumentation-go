package otelgorillaws_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	oteltrace "go.opentelemetry.io/otel/trace"

	otelgorillaws "github.com/Marz32onE/instrumentation-go/otel-gorilla-ws"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// newTestTP creates a TracerProvider backed by an in-memory SpanRecorder,
// sets it as the global provider, and registers t.Cleanup to shut it down.
// NewConn/Dial read from otel.GetTracerProvider(), so the provider must be
// set before the connection is created.
func newTestTP(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}),
	)
	return sr
}

func TestRoundTrip(t *testing.T) {
	sr := newTestTP(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		conn := otelgorillaws.NewConn(raw)
		defer conn.Close()

		ctx, typ, data, err := conn.ReadMessage(context.Background())
		if err != nil {
			t.Errorf("read: %v", err)
			return
		}
		_ = conn.WriteMessage(ctx, typ, data)
	}))
	defer srv.Close()

	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, _, err := otelgorillaws.Dial(context.Background(), url, nil)
	require.NoError(t, err, "dial")
	defer conn.Close()

	require.NoError(t, conn.WriteMessage(context.Background(), websocket.TextMessage, []byte(`{"x":1}`)))

	_, _, _, err = conn.ReadMessage(context.Background())
	require.NoError(t, err)

	assert.NotEmpty(t, sr.Ended(), "expected spans to be recorded")
}

func TestSpanAttributes(t *testing.T) {
	sr := newTestTP(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		conn := otelgorillaws.NewConn(raw)
		defer conn.Close()
		ctx, typ, data, err := conn.ReadMessage(context.Background())
		if err != nil {
			return
		}
		_ = conn.WriteMessage(ctx, typ, data)
	}))
	defer srv.Close()

	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, _, err := otelgorillaws.Dial(context.Background(), url, nil)
	require.NoError(t, err)
	defer conn.Close()

	payload := []byte(`{"msg":"hello"}`)
	require.NoError(t, conn.WriteMessage(context.Background(), websocket.TextMessage, payload))

	_, _, got, err := conn.ReadMessage(context.Background())
	require.NoError(t, err)
	assert.Equal(t, payload, got)

	var sendSpans, recvSpans []sdktrace.ReadOnlySpan
	for _, s := range sr.Ended() {
		switch s.Name() {
		case "websocket.send":
			sendSpans = append(sendSpans, s)
		case "websocket.receive":
			recvSpans = append(recvSpans, s)
		}
	}

	// Client WriteMessage + server echo WriteMessage = 2 send spans
	require.NotEmpty(t, sendSpans, "expected websocket.send spans")
	assert.Equal(t, oteltrace.SpanKindProducer, sendSpans[0].SpanKind())
	sendAttrs := attrsToMap(sendSpans[0])
	assert.Equal(t, int64(websocket.TextMessage), sendAttrs["websocket.message.type"])
	assert.Equal(t, int64(len(payload)), sendAttrs["messaging.message.body.size"])

	// Server ReadMessage + client ReadMessage = 2 receive spans
	require.NotEmpty(t, recvSpans, "expected websocket.receive spans")
	assert.Equal(t, oteltrace.SpanKindConsumer, recvSpans[0].SpanKind())

	// Client's receive span (last one) must link back to sender span context
	lastRecv := recvSpans[len(recvSpans)-1]
	require.NotEmpty(t, lastRecv.Links(), "receive span must link to sender span context")
}

// attrsToMap converts span attributes to map[string]any for easy lookup.
func attrsToMap(s sdktrace.ReadOnlySpan) map[string]any {
	m := make(map[string]any, len(s.Attributes()))
	for _, a := range s.Attributes() {
		m[string(a.Key)] = a.Value.AsInterface()
	}
	return m
}
