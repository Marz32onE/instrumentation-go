# otel-mongo (otelmongo)

[繁體中文 (Traditional Chinese)](README.zh-TW.md)

---

OpenTelemetry wrapper around the [MongoDB Go Driver v2](https://www.mongodb.com/docs/drivers/go/current/). Injects **W3C Trace Context** into documents on write (`_oteltrace` field) and restores it on read so the same trace can be followed across services. Per [OTel Go Contrib](https://github.com/open-telemetry/opentelemetry-go-contrib/tree/main/instrumentation): the package accepts **TracerProvider** and **Propagators** via options; it does **not** provide InitTracer. Set the global provider and propagator at process startup (see **example/**).

---

## Layout

```
otel-mongo/
└── otelmongo/
    ├── version.go      # ScopeName, Version()
    ├── client.go       # Client, Connect, ConnectWithOptions, NewClient, WithTracerProvider, WithPropagators
    ├── database.go     # Database, Collection
    ├── collection.go   # InsertOne, InsertMany, Find, FindOne, UpdateOne, UpdateMany, ReplaceOne, DeleteOne, DeleteMany
    ├── cursor.go       # Cursor (DecodeWithContext), SingleResult
    ├── tracing.go      # _oteltrace inject/extract, ContextFromDocument
    ├── semconv.go      # OTel DB/MongoDB semantics
    ├── example/        # How to create TracerProvider + set global + use otelmongo
    └── *_test.go
```

- **Trace storage:** Written/updated documents get a reserved **`_oteltrace`** field (W3C `traceparent` and optional `tracestate`). Use **ContextFromDocument(ctx, raw)** for raw BSON (e.g. change streams).
- **Two layers:** (1) **Driver:** Client uses contrib `otelmongo.NewMonitor` for connection/command spans. (2) **Document:** Collection CRUD injects `_oteltrace` on write and supports span links / propagation on read.

---

## Usage

### 1. Initialize provider and propagator (application responsibility)

See **example/main.go**. In short: create TracerProvider (e.g. OTLP), set `otel.SetTracerProvider(tp)` and `otel.SetTextMapPropagator(prop)`, defer shutdown.

### 2. Connect and use

```go
import (
    "github.com/Marz32onE/instrumentation-go/otel-mongo/otelmongo"
    "go.mongodb.org/mongo-driver/v2/mongo/options"
)

client, err := otelmongo.Connect(options.Client().ApplyURI(uri))
if err != nil { log.Fatal(err) }
defer client.Disconnect(ctx)

db := client.Database("mydb")
coll := db.Collection("mycoll")
// InsertOne, Find, UpdateOne, etc. handle _oteltrace automatically
```

Optional: **ConnectWithOptions(traceOpts, mongoOpts)** with **WithTracerProvider(tp)** or **WithPropagators(p)**.

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

- `go.mongodb.org/mongo-driver/v2`
- `go.opentelemetry.io/contrib/instrumentation/.../mongo/otelmongo` (driver-level spans, imported as contribmongo)
- `go.opentelemetry.io/otel` and SDK
- Go 1.25+
