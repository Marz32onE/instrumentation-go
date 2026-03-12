# otelwebsocket

`otelwebsocket` wraps [gorilla/websocket](https://github.com/gorilla/websocket)
and adds [OpenTelemetry](https://opentelemetry.io/) distributed-tracing support
by propagating the **W3C Trace Context** inside the WebSocket message body.

## How it works

| Side | What happens |
|------|--------------|
| **Sender** (`WriteMessage`) | The current span's trace-context headers (e.g. `traceparent`, `tracestate`) are injected into a lightweight JSON envelope that wraps the original payload. The envelope is sent as the WebSocket message body. |
| **Receiver** (`ReadMessage`) | The JSON envelope is unwrapped, trace-context headers are extracted and used to reconstruct the remote span context, and a new Go `context.Context` that carries the propagated span is returned to the caller. |

```
┌─────────────────────────────────────┐
│  WebSocket message body (JSON)      │
│  {                                  │
│    "headers": {                     │
│      "traceparent": "00-abc…-01"    │
│    },                               │
│    "payload": <original bytes>      │
│  }                                  │
└─────────────────────────────────────┘
```

## Installation

```bash
go get github.com/Marz32onE/otelwebsocket
```

## Quick start

```go
// ── Client (sender) ──────────────────────────────────────────────────────────
raw, _, err := websocket.DefaultDialer.DialContext(ctx, serverURL, nil)
if err != nil { /* handle */ }

conn := otelwebsocket.NewConn(raw)

ctx, span := tracer.Start(ctx, "send-message")
defer span.End()

// Trace context is automatically injected into the message body.
err = conn.WriteMessage(ctx, websocket.TextMessage, []byte("hello"))

// ── Server (receiver) ────────────────────────────────────────────────────────
raw, err := upgrader.Upgrade(w, r, nil)
if err != nil { /* handle */ }

conn := otelwebsocket.NewConn(raw)

// recvCtx carries the propagated span from the client.
recvCtx, msgType, data, err := conn.ReadMessage(context.Background())

// Create a child span that is linked to the client's trace.
_, childSpan := tracer.Start(recvCtx, "handle-message")
defer childSpan.End()
```

## Configuration

```go
conn := otelwebsocket.NewConn(raw,
    // Use a custom propagator (default: otel.GetTextMapPropagator()).
    otelwebsocket.WithPropagator(myPropagator),
    // Use a custom TracerProvider (default: otel.GetTracerProvider()).
    otelwebsocket.WithTracerProvider(myTracerProvider),
)
```

## Convenience dial helper

```go
conn, resp, err := otelwebsocket.Dial(ctx, serverURL, nil)
```

## Backward compatibility

If a message was **not** produced by this library (i.e. it is not a valid
JSON envelope), `ReadMessage` returns the raw bytes unchanged and no span
context is injected into the returned context.  This makes it safe to
introduce `otelwebsocket` incrementally alongside plain WebSocket messages.