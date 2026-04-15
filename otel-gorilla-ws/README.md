# otel-gorilla-ws

`otel-gorilla-ws` wraps [gorilla/websocket](https://github.com/gorilla/websocket) and adds OpenTelemetry distributed tracing with W3C Trace Context propagation inside WebSocket message bodies.

Outgoing messages use the shared envelope format (compatible with `otel-ws` and `otel-rxjs-ws` JS packages):

```json
{
  "header": { "traceparent": "...", "tracestate": "..." },
  "data": <original-payload>
}
```

`data` is the original payload as-is if it is valid JSON, or a JSON-encoded string for non-JSON bytes.

Incoming messages support two formats:
1. **Envelope format** (above) — used by new Go and JS clients.
2. **Legacy flat format** — backward compatible with old Go-only deployments: `{ "traceparent": "...", "tracestate": "...", ...fields }`.

## Installation

```bash
go get github.com/Marz32onE/instrumentation-go/otel-gorilla-ws
```

## Usage

### Tracing feature flags

`otel-gorilla-ws` supports:

- `OTEL_INSTRUMENTATION_GO_TRACING_ENABLED` (global master switch)
- `OTEL_GORILLA_WS_TRACING_ENABLED` (ws module switch)

Defaults: enabled when unset. Values `false/0/no/off` disable.

Priority:
1. Global off disables ws tracing regardless of module flag.
2. Otherwise module flag controls ws tracing.

When disabled, both send/receive spans and trace-context propagation are turned off.

```go
raw, _, _ := websocket.DefaultDialer.DialContext(ctx, serverURL, nil)
conn := otelgorillaws.NewConn(raw)

_ = conn.WriteMessage(ctx, websocket.TextMessage, []byte("hello"))
recvCtx, msgType, data, _ := conn.ReadMessage(context.Background())
_, _ = recvCtx, msgType
_ = data
```
