# natstrace

[繁體中文 (Traditional Chinese)](README.zh-TW.md)

---

OpenTelemetry tracing wrapper for [NATS](https://nats.io/) and [NATS JetStream](https://docs.nats.io/nats-concepts/jetstream), aligned with the official `nats.go` / `nats.go/jetstream` APIs. Propagates W3C Trace Context in message headers.

---

## Architecture

```
pkg/natstrace/
├── natstrace/           # Core NATS connection and Pub/Sub
│   ├── otel.go          # InitTracer, ShutdownTracer, WithTracerProvider
│   ├── connect.go       # Connect, ConnectTLS, ConnectWithCredentials
│   ├── conn.go          # Conn: Publish, PublishMsg, Subscribe, QueueSubscribe, Request
│   ├── propagation.go   # HeaderCarrier (nats.Header ↔ TextMapCarrier)
│   └── doc.go
├── jetstreamtrace/      # JetStream streams and consumers
│   ├── jetstream.go     # New, JetStream, Publish, CreateOrUpdateStream
│   ├── stream.go        # Stream, Consumer, CreateOrUpdateConsumer
│   ├── consumer.go      # Consume, Messages, Fetch, FetchBytes, FetchNoWait, MessageBatch
│   └── doc.go
├── go.mod
└── README.md
```

- **Tracer and propagator:** Provided by the **global** default. You **may** call **`InitTracer(endpoint, attrs...)`** first to set service name/version and endpoint; if you don’t, the first **`Connect(url, ...)`** will initialize the tracer with default endpoint (`OTEL_EXPORTER_OTLP_ENDPOINT` or `localhost:4317`), auto-generated `service.name` (UUID), and `service.version` (`0.0.0`). **Explicit InitTracer is recommended** so you can set a proper service name and version.
- **Connection:** `natstrace.Connect(url, natsOpts)` returns **`*natstrace.Conn`**, used like `*nats.Conn`; Publish/Request take an extra `context.Context`, and Subscribe handlers receive **`MsgWithContext`** (m.Msg, m.Context()); type **MsgHandler** matches nats.MsgHandler naming.
- **JetStream:** `jetstreamtrace.New(conn)` requires **`*natstrace.Conn`**. Publish accepts `context.Context`; Consume / Messages / Fetch use **MsgWithContext**; handler type **MsgHandler** aligns with natstrace and nats.go.

---

## Usage

### 1. Initialize (recommended) or connect directly

You can call **`Connect`** without calling **InitTracer** first; the package will initialize the tracer with default endpoint, auto-generated `service.name` (UUID v4), and `service.version` (`0.0.0`). **Calling InitTracer explicitly is recommended** so you can set a proper service name and version.

**Recommended (explicit InitTracer):**

```go
import (
    "go.opentelemetry.io/otel/attribute"
    "github.com/Marz32onE/natstrace/natstrace"
)

func main() {
    if err := natstrace.InitTracer("", attribute.String("service.name", "my-service"), attribute.String("service.version", "1.0.0")); err != nil {
        log.Fatal(err)
    }
    defer natstrace.ShutdownTracer() // optional; package also registers runtime.AddCleanup for process teardown

    conn, err := natstrace.Connect(natsURL, nil)
    if err != nil {
        log.Fatal(err)
    }
    defer conn.Close()
    // ...
}
```

**Minimal (no InitTracer):** `natstrace.Connect(natsURL, nil)` will initialize the tracer automatically with defaults.

- Empty `endpoint` uses `OTEL_EXPORTER_OTLP_ENDPOINT` or `localhost:4317`.
- HTTP endpoints (e.g. `http://...` or port 4318) use OTLP/HTTP; otherwise OTLP/gRPC.
- If you omit `service.name` / `service.version` in InitTracer args, the package sets `service.name` to a UUID and `service.version` to `0.0.0`.

### 2. Core NATS: Publish / Subscribe

```go
conn.Publish(ctx, "subject", []byte("data"))
conn.PublishMsg(ctx, msg)

conn.Subscribe("subject", func(m natstrace.MsgWithContext) {
    // m.Msg, m.Context() — trace from headers in m.Context()
})
conn.QueueSubscribe("subject", "queue", handler)

reply, err := conn.Request(ctx, "subject", []byte("ping"), 2*time.Second)
```

### 3. JetStream

```go
js, err := jetstreamtrace.New(conn)
// After creating stream/consumer:
cons.Consume(func(m jetstreamtrace.MsgWithContext) {
    // m implements Msg (m.Data(), m.Ack()); m.Context() has trace from message headers
})
// Or
iter, _ := cons.Messages()
ctx, msg, err := iter.Next()
// Or
batch, _ := cons.Fetch(5, jetstream.FetchMaxWait(time.Second))
for m := range batch.MessagesWithContext() {
    // m.Data(), m.Ack(), m.Context() — MsgWithContext aligns with jetstream.Msg
}
```

### 4. Underlying *nats.Conn

`conn.NatsConn()` returns `*nats.Conn`. `conn.TraceContext()` returns the Tracer and TextMapPropagator (used internally by jetstreamtrace).

---

## API

| Item | Description |
|------|-------------|
| **InitTracer** | Optional but **recommended**. Sets global TracerProvider and TextMapPropagator; if not called, the first `Connect` initializes with default endpoint and auto service.name/version. |
| **ShutdownTracer** | Optional; the package registers `runtime.AddCleanup` (Go 1.24+) so shutdown runs at process teardown. Call `defer ShutdownTracer()` for guaranteed flush before exit. |
| **Connect** | `Connect(url string, natsOpts []nats.Option)`. If tracer not initialized, calls `InitTracer("", nil)` first. |
| **ConnectTLS / ConnectWithCredentials** | Require InitTracer to have been called first (return **`ErrInitTracerRequired`** otherwise). |
| **Tests** | Use `natstrace.InitTracer("", natstrace.WithTracerProviderInit(tp))` (and optionally `otel.SetTextMapPropagator(prop)`) before `Connect(url, nil)`. |

---

## Dependencies

- `github.com/nats-io/nats.go` (includes JetStream)
- `go.opentelemetry.io/otel` and SDK (trace, propagation, OTLP exporter)
- Go 1.25+

Tests use `github.com/stretchr/testify` and `nats-server/v2` for integration tests.
