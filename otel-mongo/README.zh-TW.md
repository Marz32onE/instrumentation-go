# mongodbtrace (mongotrace)

[English](README.md)

---

以 [MongoDB Go Driver v2](https://www.mongodb.com/docs/drivers/go/current/) 為基礎的 OpenTelemetry 包裝。在寫入時將 **W3C Trace Context** 注入文件，讀取時還原，讓同一條 trace 可跨服務延續。

---

## 架構

```
pkg/mongodbtrace/
└── mongotrace/
    ├── otel.go          # InitTracer, ShutdownTracer, WithTracerProvider
    ├── client.go        # Client, NewClient, Database, WithOtelMongoOptions, ErrInitTracerRequired
    ├── database.go      # Database, Collection
    ├── collection.go    # Collection: InsertOne, InsertMany, Find, FindOne, UpdateOne, UpdateMany,
    │                    #            ReplaceOne, DeleteOne, DeleteMany（皆使用 _oteltrace）
    ├── cursor.go        # Cursor (DecodeWithContext), SingleResult (Decode → span link + trace)
    ├── tracing.go       # _oteltrace 注入/擷取: injectTraceIntoDocument, extractMetadataFromRaw,
    │                    # ContextFromDocument, injectTraceIntoUpdate, upsertSetField
    ├── semconv.go       # OTel DB/MongoDB 語意: dbSpanName, dbAttributes, recordSpanError
    ├── *_test.go        # 單元與整合測試 (testify)
    └── ...
├── go.mod
└── README.md
```

- **Trace 儲存：** 每個寫入/更新的文件都會有保留欄位 **`_oteltrace`**（W3C `traceparent` 與選用 `tracestate`）。讀取時可從中還原 context，或對 raw BSON 使用 **`ContextFromDocument(ctx, raw)`**。
- **Tracer：** 使用全域 TracerProvider。可先呼叫 **`InitTracer(endpoint, attrs...)`** 再 **`NewClient(uri)`** 以設定 service 名稱/版本與 endpoint；若不呼叫，**`NewClient(uri)`** 會用預設 endpoint、自動產生的 `service.name`（UUID）與 `service.version`（`0.0.0`）初始化。**建議顯式呼叫 InitTracer** 以設定正確的服務名稱與版本。
- **兩層：** (1) **Driver：** `NewClient` 使用 `otelmongo.NewMonitor` 產生連線/指令 span。(2) **Document：** Collection CRUD 在寫入時注入 `_oteltrace`，讀取時透過 Cursor.DecodeWithContext / SingleResult 的 span link 延續「誰寫入這份文件」的 trace。

---

## 使用方式

### 1. 初始化（建議）或直接建立 client

可不先呼叫 **InitTracer** 就呼叫 **`NewClient(uri)`**；套件會用預設 endpoint、自動產生的 `service.name`（UUID v4）與 `service.version`（`0.0.0`）初始化。**仍建議顯式呼叫 InitTracer** 以設定服務名稱與版本。

**建議（顯式 InitTracer）：**

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
    defer mongotrace.ShutdownTracer() // 可選；套件也會註冊 runtime.AddCleanup 於 process 結束時執行

    client, err := mongotrace.NewClient("mongodb://localhost:27017")
    if err != nil {
        log.Fatal(err)
    }
    defer client.Disconnect(context.Background())

    db := client.Database("mydb")
    coll := db.Collection("mycoll")
    // InsertOne / Find / UpdateOne 等會自動處理 _oteltrace
}
```

**最精簡（不呼叫 InitTracer）：** `mongotrace.NewClient(uri)` 會自動以預設值初始化。

- 空 `endpoint` 會使用 `OTEL_EXPORTER_OTLP_ENDPOINT` 或 `localhost:4317`；HTTP（例如 port 4318）使用 OTLP/HTTP，其餘為 gRPC。
- 若在 InitTracer 的參數中未提供 `service.name` / `service.version`，套件會將 `service.name` 設為 UUID、`service.version` 設為 `0.0.0`。

### 2. Collection CRUD 與 trace 保證

所有對外的 CRUD 方法皆已包裝，確保 trace context 正確寫入與傳遞：

| 操作 | 保證 |
|------|------|
| **Insert**（InsertOne, InsertMany, ReplaceOne） | 目前請求的 trace context 會寫入文件的 **`_oteltrace`** 中繼欄位。 |
| **Update**（UpdateOne, UpdateMany） | 會修改 update 內容，使文件的 **`_oteltrace`** 被**替換**為目前 trace context（例如 operator update 使用 `$set._oteltrace`）；單次 round-trip，不需先讀再寫。 |
| **Read**（Find, FindOne） | 會為讀取建立新 span；當文件含有 `_oteltrace` 時，**SingleResult** / **Cursor** 會記錄指向該來源 trace 的 **span link**。可使用 **DecodeWithContext** 或 **SingleResult.TraceContext()** 將文件的 trace context 傳給下游。 |
| **Delete**（DeleteOne, DeleteMany） | 文件（及其 **`_oteltrace`** 中繼）在一次 round-trip 中一併刪除；無需額外的 metadata 刪除步驟。 |

所有 Collection 方法皆遵循 OTel DB/MongoDB 語意（db.operation.name, db.collection.name, server.address 等）。

### 3. 從文件還原 trace（例如 change stream）

當你有 `bson.Raw`（例如來自 change stream）時，使用 **`ContextFromDocument`** 還原寫入者的 trace context：

```go
fullDoc := changeStreamEvent.FullDocument
outCtx := mongotrace.ContextFromDocument(ctx, fullDoc)
// 用 outCtx 做下游 span 或轉發（例如到 NATS）
```

### 4. Cursor 與 SingleResult

- **Cursor.DecodeWithContext(ctx, val)：** 將目前文件解碼進 `val`，並回傳帶有該文件 `_oteltrace` 的 context，供下游傳遞。
- **SingleResult：** `Decode` 會結束 findOne 的 span，並在存在 `_oteltrace` 時加入 span link。`TraceContext()` 回傳該結果的 tracer 與 propagator。

### 5. 選項

```go
client, err := mongotrace.NewClient(uri, mongotrace.WithOtelMongoOptions(/* otelmongo.Option */))
```

僅支援 **`WithOtelMongoOptions`**（例如自訂 span 名稱）。Endpoint 與 resource 由 **InitTracer** 設定。

---

## API 與錯誤

| 項目 | 說明 |
|------|------|
| **InitTracer** | 可選但**建議使用**。設定全域 TracerProvider 與 TextMapPropagator；若不呼叫，第一次 `NewClient` 會以預設 endpoint 與自動 service.name/version 初始化。 |
| **ShutdownTracer** | 可選；套件會註冊 `runtime.AddCleanup`（Go 1.24+）於 process 結束時執行。若要確保結束前 flush，請呼叫 `defer ShutdownTracer()`。 |
| **NewClient** | 若 tracer 尚未初始化，會先呼叫 `InitTracer("", nil)`。 |
| **TraceMetadataKey** | 保留的文件欄位名稱 **`_oteltrace`**（匯出常數）。 |
| **TraceMetadata** | 包含 `Traceparent`、`Tracestate`（W3C 格式）。 |
| **Tests** | 在 `NewClient(uri)` 前使用 `mongotrace.InitTracer("", mongotrace.WithTracerProvider(tp))`；整合測試需設定 `MONGO_URI`。 |

---

## 依賴

- `go.mongodb.org/mongo-driver/v2`
- `go.opentelemetry.io/contrib/instrumentation/.../mongo/otelmongo`（driver 層 span）
- `go.opentelemetry.io/otel` 與 SDK
- Go 1.25+

測試使用 `github.com/stretchr/testify`。
