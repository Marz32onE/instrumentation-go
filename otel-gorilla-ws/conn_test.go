package otelgorillaws_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"

	otelgorillaws "github.com/Marz32onE/instrumentation-go/otel-gorilla-ws"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func TestRoundTrip(t *testing.T) {
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
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	if err := conn.WriteMessage(context.Background(), websocket.TextMessage, []byte(`{"x":1}`)); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, _, _, err = conn.ReadMessage(context.Background())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
}
