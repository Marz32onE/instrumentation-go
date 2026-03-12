# instrumentation-go

OpenTelemetry instrumentation packages for NATS, MongoDB, and WebSocket, aligned with [OTel Go Contrib instrumentation guidelines](https://github.com/open-telemetry/opentelemetry-go-contrib/tree/main/instrumentation). Each package accepts **TracerProvider** and **Propagators** via options and uses the global provider/propagator when not supplied; **applications** are responsible for creating and setting the TracerProvider at startup (see each package’s **example/**).

## Packages

| Package | Import path | Description |
|---------|-------------|-------------|
| **otel-nats** | `github.com/Marz32onE/instrumentation-go/otel-nats/otelnats` | Core NATS connection, Publish, Subscribe (W3C trace in headers). |
| **otel-nats** | `github.com/Marz32onE/instrumentation-go/otel-nats/oteljetstream` | JetStream streams, consumers, Publish, Consume, Messages, Fetch. |
| **otel-mongo** | `github.com/Marz32onE/instrumentation-go/otel-mongo/otelmongo` | MongoDB client wrapper; `_oteltrace` in documents, ContextFromDocument for change streams. |
| **otel-websocket** | `github.com/Marz32onE/otelwebsocket` | WebSocket trace-context propagation (JSON envelope in message body). |

## Layout

```
instrumentation-go/
├── otel-nats/
│   ├── otelnats/       # Connect, Conn, Publish, Subscribe, HeaderCarrier
│   ├── oteljetstream/  # New, JetStream, Stream, Consumer, Consume, Messages, Fetch
│   ├── example/        # How to init TracerProvider + use otelnats/oteljetstream
│   └── README.md
├── otel-mongo/
│   ├── otelmongo/      # Client, Connect, NewClient, Database, Collection, _oteltrace
│   ├── example/        # How to init TracerProvider + use otelmongo
│   └── README.md
├── otel-websocket/
│   ├── *.go            # Conn, NewConn, WriteMessage, ReadMessage, WithTracerProvider, WithPropagators
│   ├── example/        # How to init TracerProvider (WebSocket usage in comments)
│   └── README.md
└── README.md           # This file
```

## Usage pattern

1. **Application** creates a TracerProvider (e.g. OTLP exporter), sets `otel.SetTracerProvider(tp)` and `otel.SetTextMapPropagator(prop)`, and defers shutdown.
2. **Application** uses the instrumentation: `otelnats.Connect(url, nil)`, `otelmongo.Connect(opts)`, `otelwebsocket.NewConn(raw)`, etc. Options like `WithTracerProvider(tp)` override the global when needed.

See **otel-nats/example**, **otel-mongo/example**, and **otel-websocket/example** for runnable examples.
