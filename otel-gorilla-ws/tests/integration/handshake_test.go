package integration_test

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

	otelgorillaws "github.com/Marz32onE/instrumentation-go/otel-gorilla-ws"
)

func newIntegrationTP(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	t.Cleanup(func() {
		_ = tp.Shutdown(context.Background())
	})
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}),
	)
	return recorder
}

func wsURL(srv *httptest.Server) string {
	return "ws" + strings.TrimPrefix(srv.URL, "http")
}

func TestIntegration_Handshake_SubprotocolNegotiation(t *testing.T) {
	recorder := newIntegrationTP(t)

	upgrader := &otelgorillaws.Upgrader{
		CheckOrigin:  func(r *http.Request) bool { return true },
		Subprotocols: []string{"json"},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		require.NoError(t, err)
		defer conn.Close()

		assert.Equal(t, "json", conn.Subprotocol())

		ctx, typ, payload, err := conn.ReadMessage(context.Background())
		require.NoError(t, err)
		require.NoError(t, conn.WriteMessage(ctx, typ, payload))
	}))
	defer srv.Close()

	conn, _, err := otelgorillaws.Dial(context.Background(), wsURL(srv), nil, []string{"json"})
	require.NoError(t, err)
	defer conn.Close()

	assert.Equal(t, "json", conn.Subprotocol())

	input := []byte(`{"kind":"handshake"}`)
	require.NoError(t, conn.WriteMessage(context.Background(), websocket.TextMessage, input))
	_, _, got, err := conn.ReadMessage(context.Background())
	require.NoError(t, err)
	assert.Equal(t, input, got)

	assert.GreaterOrEqual(t, len(recorder.Ended()), 2)
}
