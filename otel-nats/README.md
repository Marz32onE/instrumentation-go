# otel-nats (otelnats + oteljetstream)

[繁體中文 (Traditional Chinese)](README.zh-TW.md)

---

OpenTelemetry tracing for [NATS](https://nats.io/) and [NATS JetStream](https://docs.nats.io/nats-concepts/jetstream), aligned with the official `nats.go` / `nats.go/jetstream` APIs. Propagates W3C Trace Context in message headers. Per [OTel Go Contrib](https://github.com/open-telemetry/opentelemetry-go-contrib/tree/main/instrumentation): packages accept **TracerProvider** and **Propagators** via options; they do **not** provide InitTracer. Set the global provider and propagator at process startup (see **example/**).

---

## Layout

```
otel-nats/
├── otelnats/           # Core NATS: Connect, Conn, Publish, Subscribe, HeaderCarrier
│   ├── connect.go      # Connect, ConnectWithOptions, ConnectTLS, ConnectWithCredentials
│   ├── conn.go         # Conn, Publish, PublishMsg, Subscribe, QueueSubscribe, WithTracerProvider, WithPropagators
│   ├── propagation.go  # HeaderCarrier (nats.Header ↔ TextMapCarrier)
│   └── doc.go
├── oteljetstream/      # JetStream: New, JetStream, Stream, Consumer, PushConsumer, Consume, Messages, Fetch
│   ├── jetstream.go    # New(conn), Publish, CreateOrUpdateStream
│   ├── stream.go       # Stream, Consumer/PushConsumer, CreateOrUpdateConsumer/CreateOrUpdatePushConsumer
│   ├── consumer.go     # Consume, Messages, Fetch, MessageBatch (MessagesWithContext), MsgWithContext
│   └── doc.go
├── example/            # How to create TracerProvider + set global + use otelnats/oteljetstream
├── go.mod
└── README.md
```

---

## Usage

### 1. Initialize provider and propagator (application responsibility)

Create a TracerProvider (e.g. OTLP) and set the global provider and propagator once at startup. See **example/main.go** for a full runnable.

```go
import (
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/attribute"
    "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
    "go.opentelemetry.io/otel/propagation"
    "go.opentelemetry.io/otel/sdk/resource"
    sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// In main:
tp, err := newTracerProvider() // create with OTLP exporter + resource
if err != nil { log.Fatal(err) }
defer func() { _ = tp.Shutdown(ctx) }()

otel.SetTracerProvider(tp)
otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
    propagation.TraceContext{},
    propagation.Baggage{},
))
```

### 2. Core NATS: Connect, Publish, Subscribe

```go
import (
    "github.com/Marz32onE/instrumentation-go/otel-nats/otelnats"
)

conn, err := otelnats.Connect(natsURL, nil)
if err != nil { log.Fatal(err) }
defer conn.Close()

conn.Publish(ctx, "subject", []byte("data"))
conn.Subscribe("subject", func(m otelnats.MsgWithContext) {
    // m.Msg, m.Context() — trace from headers in m.Context()
})
conn.QueueSubscribe("subject", "queue", handler)
```

Optional: pass **WithTracerProvider(tp)** or **WithPropagators(p)** to **ConnectWithOptions** for per-connection overrides.

### 3. JetStream

```go
import (
    "github.com/Marz32onE/instrumentation-go/otel-nats/oteljetstream"
    "github.com/Marz32onE/instrumentation-go/otel-nats/otelnats"
)

conn, _ := otelnats.Connect(natsURL, nil)
defer conn.Close()

js, err := oteljetstream.New(conn)
// After creating stream/consumer:
cons.Consume(func(m oteljetstream.MsgWithContext) {
    // m.Data(), m.Ack(), m.Context() — trace from message headers
})

// Push consumer is also wrapped:
pushCons, _ := js.CreateOrUpdatePushConsumer(ctx, "MYSTREAM", oteljetstream.ConsumerConfig{
    Durable:        "push-consumer",
    DeliverSubject: "push.deliver",
    FilterSubject:  "events.push",
})
pushCons.Consume(func(m oteljetstream.MsgWithContext) {
    // same trace extraction behavior
    _ = m.Ack()
})
```

### 4. Tests

Set the global provider (and optionally propagator) before Connect; no InitTracer.

```go
otel.SetTracerProvider(tp)
otel.SetTextMapPropagator(prop) // if testing propagation
conn, err := otelnats.Connect(url, nil)
```

---

## API summary

| Item | Description |
|------|-------------|
| **Connect** | `Connect(url string, natsOpts ...nats.Option)`. Uses `otel.GetTracerProvider()` and `otel.GetTextMapPropagator()` unless overridden via ConnectWithOptions. |
| **ConnectWithOptions** | Same with optional **WithTracerProvider(tp)** and **WithPropagators(p)**. |
| **ConnectTLS** | `ConnectTLS(url, certFile, keyFile, caFile string, natsOpts ...nats.Option)`. Connects with mutual TLS. |
| **ConnectWithCredentials** | `ConnectWithCredentials(url, credFile string, natsOpts ...nats.Option)`. Connects with JWT/NKey credentials. |
| **ScopeName / Version()** | Used when creating Tracer (OTel contrib guideline). |
| **Tests** | Use `otel.SetTracerProvider(tp)` (and `otel.SetTextMapPropagator(prop)` if needed) before Connect. |

---

## Deliver Spans (Service Graph)

When `OTEL_EXPORTER_OTLP_ENDPOINT` is set, otelnats/oteljetstream creates synthetic "deliver" spans so NATS appears as a broker in Grafana service graph.

### Span hierarchy

```
send subject (PRODUCER, api)
  └── subject deliver (CONSUMER, nats://addr)  ← injected into headers

process subject (CONSUMER, worker)  ← links to deliver span
```

### Resulting service graph

```
api ──► nats ──► worker
```

Deliver spans use an independent TracerProvider with `service.name = "nats://{connected_addr}"`. This is initialised automatically during `Connect`; no extra configuration is needed beyond setting the OTLP endpoint.

The endpoint must be a **full URL** for HTTP (e.g. `http://otel-collector:4318`) or **host:port** for gRPC (e.g. `otel-collector:4317`). Bare hostnames without scheme or port are not supported.

---

## MessageBatch (`Fetch` / `FetchBytes` / `FetchNoWait`)

Iterate `MessagesWithContext()` to receive each message with its extracted trace context. Drain the channel completely for each batch before the next `Fetch`.

```go
batch, err := consumer.Fetch(10)
if err != nil { ... }
for mwc := range batch.MessagesWithContext() {
    _ = mwc.Context()
    _ = mwc.Ack()
}
if err := batch.Error(); err != nil { ... }
```

---

## Dependencies

- `github.com/nats-io/nats.go` (includes JetStream)
- `go.opentelemetry.io/otel` and SDK (trace, propagation)
- Go 1.25+

Tests use `github.com/stretchr/testify` and `nats-server/v2` for integration tests.
