# otel-mongo (otelmongo)

[繁體中文 (Traditional Chinese)](README.zh-TW.md)

---

OpenTelemetry wrapper around the [MongoDB Go Driver](https://www.mongodb.com/docs/drivers/go/current/). Injects **W3C Trace Context** into documents on write (`_oteltrace` field) and restores it on read so the same trace can be followed across services. Per [OTel Go Contrib](https://github.com/open-telemetry/opentelemetry-go-contrib/tree/main/instrumentation): the package accepts **TracerProvider** and **Propagators** via options; it does **not** provide InitTracer. Set the global provider and propagator at process startup (see **example/**).

Two driver versions are supported (Go convention: v2 lives under `/v2` for a clear import path):

| Import path | Driver | Use when |
|------------|--------|----------|
| `github.com/Marz32onE/instrumentation-go/otel-mongo/v2` | MongoDB Go Driver **v2** | New projects or v2 driver (recommended) |
| `github.com/Marz32onE/instrumentation-go/otel-mongo/otelmongo` | MongoDB Go Driver **v1** | Existing code using v1 driver |

Both packages expose the same API surface (Client, Collection, Cursor, ContextFromDocument, etc.) and the same `_oteltrace` document-level propagation.

---

## Layout

```
otel-mongo/
├── otelmongo/           # MongoDB driver v1 wrapper (root module)
│   ├── version.go, client.go, database.go, collection.go, cursor.go
│   ├── tracing.go, semconv.go, bulkwrite.go, results.go
│   └── ...
├── v2/                  # MongoDB driver v2 wrapper (separate module, import .../v2)
│   ├── go.mod           # module .../otel-mongo/v2, requires go.mongodb.org/mongo-driver/v2
│   ├── version.go, client.go, database.go, collection.go, cursor.go
│   ├── tracing.go, semconv.go, bulkwrite.go, results.go
│   └── *_test.go
├── example/             # TracerProvider + global + otelmongo (uses v2)
└── README.md
```

- **Trace storage:** Written/updated documents get a reserved **`_oteltrace`** field (W3C `traceparent` and optional `tracestate`). Use **ContextFromDocument(ctx, raw)** for raw BSON (e.g. change streams).
- **Two layers:** (1) **Driver:** Client uses contrib `otelmongo.NewMonitor` for connection/command spans. (2) **Document:** Collection CRUD injects `_oteltrace` on write and supports span links / propagation on read.

---

## Usage

### 1. Initialize provider and propagator (application responsibility)

See **example/main.go**. In short: create TracerProvider (e.g. OTLP), set `otel.SetTracerProvider(tp)` and `otel.SetTextMapPropagator(prop)`, defer shutdown.

### 2. Connect and use

**MongoDB driver v2** (recommended; import path aligns with Go convention):

```go
import (
    "github.com/Marz32onE/instrumentation-go/otel-mongo/v2"
    "go.mongodb.org/mongo-driver/v2/mongo/options"
)

client, err := otelmongo.Connect(options.Client().ApplyURI(uri))
if err != nil { log.Fatal(err) }
defer client.Disconnect(ctx)

db := client.Database("mydb")
coll := db.Collection("mycoll")
// InsertOne, Find, UpdateOne, etc. handle _oteltrace automatically
```

**MongoDB driver v1** (same API, different import and Connect signature):

```go
import (
    "context"
    "github.com/Marz32onE/instrumentation-go/otel-mongo/otelmongo"
    "go.mongodb.org/mongo-driver/mongo/options"
)

client, err := otelmongo.Connect(ctx, options.Client().ApplyURI(uri))
if err != nil { log.Fatal(err) }
defer client.Disconnect(ctx)

db := client.Database("mydb")
coll := db.Collection("mycoll")
// Same CRUD and _oteltrace behaviour as v2 wrapper
```

Optional: **ConnectWithOptions(ctx, traceOpts, mongoOpts)** (v1) or **ConnectWithOptions(traceOpts, mongoOpts)** (v2) with **WithTracerProvider(tp)** or **WithPropagators(p)**.

### 3. Restore trace from document (e.g. change streams)

```go
fullDoc := changeStreamEvent.FullDocument
outCtx := otelmongo.ContextFromDocument(ctx, fullDoc)
// Use outCtx for downstream spans or forwarding (e.g. to NATS)
```

### 4. Tests

```go
otel.SetTracerProvider(trace.NewTracerProvider())
client, err := otelmongo.Connect(opts)
```

---

## API summary

| Item | Description |
|------|-------------|
| **Connect / ConnectWithOptions** | Uses `otel.GetTracerProvider()` unless **WithTracerProvider(tp)** is passed. |
| **NewClient** | Same; accepts optional **WithTracerProvider**, **WithPropagators**. |
| **ContextFromDocument** | Restores trace context from document’s `_oteltrace` (e.g. for change streams). |
| **ScopeName / Version()** | Used when creating Tracer (OTel contrib guideline). |

---

## Dependencies

- **v2** (`.../otel-mongo/v2`): `go.mongodb.org/mongo-driver/v2`, `go.opentelemetry.io/contrib/instrumentation/.../v2/mongo/otelmongo`, `go.opentelemetry.io/otel` and SDK. See `v2/go.mod`.
- **otelmongo** (v1, root): `go.mongodb.org/mongo-driver` v1, `go.opentelemetry.io/contrib/instrumentation/.../mongo/otelmongo`, `go.opentelemetry.io/otel` and SDK. See root `go.mod`.
- Go 1.25+
