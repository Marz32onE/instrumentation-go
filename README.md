# instrumentation-go

OpenTelemetry instrumentation packages for NATS, MongoDB, and WebSocket, aligned with [OTel Go Contrib instrumentation guidelines](https://github.com/open-telemetry/opentelemetry-go-contrib/tree/main/instrumentation). This repo contains **four Go modules**, each versioned independently (per-module tags). All modules require **Go 1.25**; CI runs build, test, and golangci-lint per module (see [.github/workflows/ci.yml](.github/workflows/ci.yml)). Each package accepts **TracerProvider** and **Propagators** via options and uses the global provider/propagator when not supplied; **applications** are responsible for creating and setting the TracerProvider at startup (see each package’s **example/**).

## Packages

| Package | Import path | Version | Description |
|---------|-------------|---------|-------------|
| **otel-mongo** (v1) | `github.com/Marz32onE/instrumentation-go/otel-mongo/otelmongo` | 0.2.0 | MongoDB driver v1 wrapper; `_oteltrace` in documents, ContextFromDocument, BulkWrite; deliver spans for service graph. |
| **otel-mongo/v2** | `github.com/Marz32onE/instrumentation-go/otel-mongo/v2` | 0.2.0 | MongoDB driver v2 wrapper; same API as v1; deliver spans for service graph. |
| **otel-nats** | `github.com/Marz32onE/instrumentation-go/otel-nats/otelnats` | 0.1.1 | Core NATS connection, Publish, Subscribe (W3C trace in headers); deliver spans for service graph. |
| **otel-nats** | `github.com/Marz32onE/instrumentation-go/otel-nats/oteljetstream` | 0.1.1 | JetStream streams, consumers, Publish, Consume, Messages, Fetch; deliver spans for service graph. |
| **otel-websocket** | `github.com/Marz32onE/instrumentation-go/otel-websocket` | 0.1.1 | WebSocket trace-context propagation (JSON envelope in message body). |

## Install

Each module is tagged separately. Use the matching tag with `go get`:

```bash
go get github.com/Marz32onE/instrumentation-go/otel-mongo@otel-mongo/v0.2.0
go get github.com/Marz32onE/instrumentation-go/otel-mongo/v2@otel-mongo/v2/v0.2.0
go get github.com/Marz32onE/instrumentation-go/otel-nats@otel-nats/v0.1.1
go get github.com/Marz32onE/instrumentation-go/otel-websocket@otel-websocket/v0.1.1
```

Then import the subpackages in your code (e.g. `.../otel-mongo/otelmongo`, `.../otel-nats/otelnats`).

## Layout

```
instrumentation-go/
├── otel-mongo/
│   ├── otelmongo/      # v1 driver wrapper (root module)
│   ├── v2/             # v2 driver wrapper (separate module, own go.mod)
│   ├── example/
│   └── README.md
├── otel-nats/
│   ├── otelnats/       # Connect, Conn, Publish, Subscribe, HeaderCarrier
│   ├── oteljetstream/  # New, JetStream, Stream, Consumer, Consume, Messages, Fetch
│   ├── example/
│   ├── go.mod
│   └── README.md
├── otel-websocket/
│   ├── *.go            # Conn, NewConn, WriteMessage, ReadMessage, WithTracerProvider, WithPropagators
│   ├── example/
│   ├── go.mod
│   └── README.md
└── README.md           # This file
```

## Usage pattern

1. **Application** creates a TracerProvider (e.g. OTLP exporter), sets `otel.SetTracerProvider(tp)` and `otel.SetTextMapPropagator(prop)`, and defers shutdown.
2. **Application** uses the instrumentation: `otelnats.Connect(url, nil)`, `otelmongo.Connect(opts)`, `otelwebsocket.NewConn(raw)`, etc. Options like `WithTracerProvider(tp)` override the global when needed.

See **otel-nats/example**, **otel-mongo/example**, and **otel-websocket/example** for runnable examples.
