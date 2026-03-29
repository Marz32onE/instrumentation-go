# otel-gorilla-ws

`otel-gorilla-ws` wraps [gorilla/websocket](https://github.com/gorilla/websocket) and adds OpenTelemetry distributed tracing with W3C Trace Context propagation inside WebSocket message bodies.

By default, outgoing messages use a header-style envelope:

```json
{
  "headers": { "traceparent": "...", "tracestate": "..." },
  "payload": "base64-encoded-bytes"
}
```

Incoming messages are backward compatible with both header-style envelope and legacy embedded form (`{ traceparent, tracestate, data }`).

## Installation

```bash
go get github.com/Marz32onE/instrumentation-go/otel-gorilla-ws
```

## Usage

```go
raw, _, _ := websocket.DefaultDialer.DialContext(ctx, serverURL, nil)
conn := otelgorillaws.NewConn(raw)

_ = conn.WriteMessage(ctx, websocket.TextMessage, []byte("hello"))
recvCtx, msgType, data, _ := conn.ReadMessage(context.Background())
_, _ = recvCtx, msgType
_ = data
```
