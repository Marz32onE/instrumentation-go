# instrumentation-go

OpenTelemetry instrumentation packages for NATS, MongoDB, and WebSocket, aligned with [OTel Go Contrib instrumentation guidelines](https://github.com/open-telemetry/opentelemetry-go-contrib/tree/main/instrumentation). This repo contains **four Go modules**, each versioned independently (per-module tags). All modules require **Go 1.24**; CI runs build, test, and golangci-lint per module (see [.github/workflows/ci.yml](.github/workflows/ci.yml)). Each package accepts **TracerProvider** and **Propagators** via options and uses the global provider/propagator when not supplied; **applications** are responsible for creating and setting the TracerProvider at startup (see each package’s **example/**).

## Packages

| Package | Import path | Version | Description |
|---------|-------------|---------|-------------|
| **otel-mongo** (v1) | `github.com/Marz32onE/instrumentation-go/otel-mongo/otelmongo` | 0.2.3 | MongoDB driver v1 wrapper; `_oteltrace` in documents, ContextFromDocument, BulkWrite; deliver spans for service graph. |
| **otel-mongo/v2** | `github.com/Marz32onE/instrumentation-go/otel-mongo/v2` | 0.2.2 | MongoDB driver v2 wrapper; same API as v1; deliver spans for service graph. |
| **otel-nats** | `github.com/Marz32onE/instrumentation-go/otel-nats/otelnats` | 0.1.5 | Core NATS connection, Publish, Subscribe (W3C trace in headers); deliver spans for service graph. |
| **otel-nats** | `github.com/Marz32onE/instrumentation-go/otel-nats/oteljetstream` | 0.1.5 | JetStream streams, consumers, Publish, Consume, Messages, Fetch; deliver spans for service graph. |
| **otel-gorilla-ws** | `github.com/Marz32onE/instrumentation-go/otel-gorilla-ws` | 0.1.1 | gorilla/websocket trace-context propagation (JSON envelope in message body). |

## Install

Each module is tagged separately. Use the matching tag with `go get`:

```bash
go get github.com/Marz32onE/instrumentation-go/otel-mongo@otel-mongo/v0.2.3
go get github.com/Marz32onE/instrumentation-go/otel-mongo/v2@otel-mongo/v2/v0.2.2
go get github.com/Marz32onE/instrumentation-go/otel-nats@otel-nats/v0.1.5
go get github.com/Marz32onE/instrumentation-go/otel-gorilla-ws@otel-gorilla-ws/v0.1.1
```

Then import the subpackages in your code (e.g. `.../otel-mongo/otelmongo`, `.../otel-nats/otelnats`).

## Tracing feature flags

All wrappers support one global switch plus module-level switches.

| Env var | Scope | Default | Effect |
|---------|-------|---------|--------|
| `OTEL_INSTRUMENTATION_GO_TRACING_ENABLED` | All modules | enabled | Global master switch. `false/0/no/off` disables tracing and propagation in all wrappers. |
| `OTEL_MONGO_TRACING_ENABLED` | `otel-mongo` + `otel-mongo/v2` | enabled | Module switch for Mongo wrappers. |
| `OTEL_NATS_TRACING_ENABLED` | `otelnats` + `oteljetstream` | enabled | Module switch for NATS wrappers. |
| `OTEL_GORILLA_WS_TRACING_ENABLED` | `otel-gorilla-ws` | enabled | Module switch for WebSocket wrapper. |

Priority: global switch first, then module switch. If global is off, module switches are ignored.

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
├── otel-gorilla-ws/
│   ├── *.go            # Conn, NewConn, WriteMessage, ReadMessage, WithTracerProvider, WithPropagators
│   ├── example/
│   ├── go.mod
│   └── README.md
└── README.md           # This file
```

## Usage pattern

1. **Application** creates a TracerProvider (e.g. OTLP exporter), sets `otel.SetTracerProvider(tp)` and `otel.SetTextMapPropagator(prop)`, and defers shutdown.
2. **Application** uses the instrumentation: `otelnats.Connect(url, nil)`, `otelmongo.Connect(opts)`, `otelgorillaws.NewConn(raw)`, etc. Options like `WithTracerProvider(tp)` override the global when needed.

See **otel-nats/example**, **otel-mongo/example**, and **otel-gorilla-ws/example** for runnable examples.

## Diagnostic logging

All packages use [`log/slog`](https://pkg.go.dev/log/slog) for structured diagnostic output — no output by default (slog respects the default handler level).

| Package | Level | Events logged |
|---------|-------|---------------|
| `otel-nats` | `DEBUG` | Server address parse failure, deliver tracer init success |
| `otel-nats` | `WARN` | Deliver tracer init failure (OTLP endpoint missing or unreachable) |
| `otel-mongo` | `DEBUG` | Deliver tracer init success |
| `otel-mongo` | `WARN` | OTLP exporter creation failure, resource creation failure |

To enable verbose output during development, configure the default slog handler:

```go
slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
    Level: slog.LevelDebug,
})))
```

Log entries use a package prefix (`otelnats:`, `otelmongo:`) and key-value pairs (`reason`, `error`, `service`, `endpoint`) for easy filtering.

---

## `OTEL_EXPORTER_OTLP_ENDPOINT` format

The deliver span feature (otel-mongo, otel-nats) reads `OTEL_EXPORTER_OTLP_ENDPOINT` to create an independent TracerProvider for synthetic broker spans. The endpoint value must be explicit:

| Protocol | Format | Example |
|----------|--------|---------|
| OTLP/HTTP | Full URL with scheme | `http://otel-collector:4318` |
| OTLP/gRPC | `host:port` (no scheme) | `otel-collector:4317` |

Bare hostnames without scheme or port (e.g. `otel-collector`) are **not** supported — always include the scheme for HTTP or the port for gRPC.
