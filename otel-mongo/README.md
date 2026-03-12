# mongodbtrace (mongotrace)

[繁體中文 (Traditional Chinese)](README.zh-TW.md)

---

OpenTelemetry wrapper around the [MongoDB Go Driver v2](https://www.mongodb.com/docs/drivers/go/current/). Injects **W3C Trace Context** into documents on write and restores it on read so the same trace can be followed across services.

---

## Architecture

```
pkg/mongodbtrace/
└── mongotrace/
    ├── otel.go          # InitTracer, ShutdownTracer, WithTracerProvider
    ├── client.go        # Client, NewClient, Database, WithOtelMongoOptions, ErrInitTracerRequired
    ├── database.go      # Database, Collection
    ├── collection.go    # Collection: InsertOne, InsertMany, Find, FindOne, UpdateOne, UpdateMany,
    │                    #            ReplaceOne, DeleteOne, DeleteMany (all use _oteltrace)
    ├── cursor.go        # Cursor (DecodeWithContext), SingleResult (Decode → span link + trace)
    ├── tracing.go       # _oteltrace inject/extract: injectTraceIntoDocument, extractMetadataFromRaw,
    │                    # ContextFromDocument, injectTraceIntoUpdate, upsertSetField
    ├── semconv.go       # OTel DB/MongoDB semantics: dbSpanName, dbAttributes, recordSpanError
    ├── *_test.go        # Unit and integration tests (testify)
    └── ...
├── go.mod
└── README.md
```

- **Trace storage:** Each written/updated document gets a reserved **`_oteltrace`** field (W3C `traceparent` and optional `tracestate`). Reads restore context from it, or use **`ContextFromDocument(ctx, raw)`** for raw BSON.
- **Tracer:** Global TracerProvider. You **may** call **`InitTracer(endpoint, attrs...)`** before **`NewClient(uri)`** to set service name/version and endpoint; if you don’t, **`NewClient(uri)`** will initialize the tracer with default endpoint, auto-generated `service.name` (UUID), and `service.version` (`0.0.0`). **Explicit InitTracer is recommended** so you can set a proper service name and version.
- **Two layers:** (1) **Driver:** `NewClient` uses `otelmongo.NewMonitor` for connection/command spans. (2) **Document:** Collection CRUD injects `_oteltrace` on write and uses `Cursor.DecodeWithContext` / SingleResult span links on read so “who wrote this doc” continues downstream.

---

## Usage

### 1. Initialize (recommended) or create client directly

You can call **`NewClient(uri)`** without calling **InitTracer** first; the package will initialize the tracer with default endpoint, auto-generated `service.name` (UUID v4), and `service.version` (`0.0.0`). **Calling InitTracer explicitly is recommended** so you can set a proper service name and version.

**Recommended (explicit InitTracer):**

```go
import (
    "context"
    "log"

    "go.opentelemetry.io/otel/attribute"
    "github.com/Marz32onE/mongodbtrace/mongotrace"
)

func main() {
    if err := mongotrace.InitTracer("", attribute.String("service.name", "my-service"), attribute.String("service.version", "1.0.0")); err != nil {
        log.Fatal(err)
    }
    defer mongotrace.ShutdownTracer() // optional; package also registers runtime.AddCleanup for process teardown

    client, err := mongotrace.NewClient("mongodb://localhost:27017")
    if err != nil {
        log.Fatal(err)
    }
    defer client.Disconnect(context.Background())

    db := client.Database("mydb")
    coll := db.Collection("mycoll")
    // InsertOne / Find / UpdateOne etc. handle _oteltrace automatically
}
```

**Minimal (no InitTracer):** `mongotrace.NewClient(uri)` will initialize the tracer automatically with defaults.

- Empty `endpoint` uses `OTEL_EXPORTER_OTLP_ENDPOINT` or `localhost:4317`; HTTP (e.g. port 4318) uses OTLP/HTTP, else gRPC.
- If you omit `service.name` / `service.version` in InitTracer args, the package sets `service.name` to a UUID and `service.version` to `0.0.0`.

### 2. Collection CRUD and trace guarantees

All user-facing CRUD methods are wrapped so that trace context is correctly stored and propagated:

| Operation | Guarantee |
|-----------|-----------|
| **Insert** (InsertOne, InsertMany, ReplaceOne) | Current request trace context is written into the document’s **`_oteltrace`** metadata field. |
| **Update** (UpdateOne, UpdateMany) | The update payload is augmented so the document’s **`_oteltrace`** is **replaced** with the current trace context (e.g. `$set._oteltrace` for operator updates); single round-trip, no extra read. |
| **Read** (Find, FindOne) | A new span is created for the read; when the document has `_oteltrace`, the **SingleResult** / **Cursor** records a **span link** to that origin trace. Use **DecodeWithContext** or **SingleResult.TraceContext()** to propagate the document’s trace context downstream. |
| **Delete** (DeleteOne, DeleteMany) | The document (and thus its **`_oteltrace`** metadata) is removed in one round-trip; no separate metadata delete step. |

All Collection methods use OTel DB/MongoDB semantics (db.operation.name, db.collection.name, server.address, etc.).

### 3. Restore trace from document (e.g. change streams)

When you have `bson.Raw` (e.g. from a change stream), use **`ContextFromDocument`** to restore the writer’s trace context:

```go
fullDoc := changeStreamEvent.FullDocument
outCtx := mongotrace.ContextFromDocument(ctx, fullDoc)
// Use outCtx for downstream spans or forwarding (e.g. to NATS)
```

### 4. Cursor and SingleResult

- **Cursor.DecodeWithContext(ctx, val):** Decodes the current document into `val` and returns a context with that document’s `_oteltrace` for downstream propagation.
- **SingleResult:** `Decode` ends the findOne span and adds a span link when `_oteltrace` is present. `TraceContext()` returns the result’s tracer and propagator.

### 5. Options

```go
client, err := mongotrace.NewClient(uri, mongotrace.WithOtelMongoOptions(/* otelmongo.Option */))
```

Only **`WithOtelMongoOptions`** is supported (e.g. custom span names). Endpoint and resource are set by **InitTracer**.

---

## API and errors

| Item | Description |
|------|-------------|
| **InitTracer** | Optional but **recommended**. Sets global TracerProvider and TextMapPropagator; if not called, the first `NewClient` initializes with default endpoint and auto service.name/version. |
| **ShutdownTracer** | Optional; the package registers `runtime.AddCleanup` (Go 1.24+) so shutdown runs at process teardown. Call `defer ShutdownTracer()` for guaranteed flush before exit. |
| **NewClient** | If tracer not initialized, calls `InitTracer("", nil)` first. |
| **TraceMetadataKey** | Reserved document field name **`_oteltrace`** (exported constant). |
| **TraceMetadata** | Holds `Traceparent`, `Tracestate` (W3C format). |
| **Tests** | Use `mongotrace.InitTracer("", mongotrace.WithTracerProvider(tp))` before `NewClient(uri)`; integration tests need `MONGO_URI`. |

---

## Dependencies

- `go.mongodb.org/mongo-driver/v2`
- `go.opentelemetry.io/contrib/instrumentation/.../mongo/otelmongo` (driver-level spans)
- `go.opentelemetry.io/otel` and SDK
- Go 1.25+

Tests use `github.com/stretchr/testify`.
