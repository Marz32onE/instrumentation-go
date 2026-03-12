# otel-mongo（otelmongo）

**[English](README.md)**

---

以 [MongoDB Go Driver v2](https://www.mongodb.com/docs/drivers/go/current/) 為基礎的 OpenTelemetry 包裝。寫入時將 **W3C Trace Context** 注入文件的 **`_oteltrace`** 欄位，讀取時還原，使同一條 trace 可跨服務延續。依 [OTel Go Contrib](https://github.com/open-telemetry/opentelemetry-go-contrib/tree/main/instrumentation) 規範：套件僅透過 option 接受 **TracerProvider** 與 **Propagators**，不提供 InitTracer；由應用程式在啟動時設定 global provider 與 propagator（見 **example/**）。

---

## 目錄結構

```
otel-mongo/
└── otelmongo/
    ├── version.go, client.go, database.go, collection.go, cursor.go, tracing.go, semconv.go
    ├── example/   # 如何建立 TracerProvider、設定 global、使用 otelmongo
    └── *_test.go
```

- **Trace 儲存：** 寫入/更新的文件會有保留欄位 **`_oteltrace`**。對 raw BSON（例如 change stream）可使用 **ContextFromDocument(ctx, raw)** 還原 context。
- **兩層：** (1) **Driver** 使用 contrib otelmongo Monitor 產生連線/指令 span。(2) **Document** 層在 CRUD 寫入時注入 `_oteltrace`，讀取時支援 span link 與傳播。

---

## 使用方式

### 1. 初始化 Provider 與 Propagator（應用程式負責）

見 **example/main.go**：建立 TracerProvider（如 OTLP）、設定 `otel.SetTracerProvider(tp)` 與 `otel.SetTextMapPropagator(prop)`、defer shutdown。

### 2. Connect 與 CRUD

```go
client, err := otelmongo.Connect(options.Client().ApplyURI(uri))
defer client.Disconnect(ctx)

db := client.Database("mydb")
coll := db.Collection("mycoll")
// InsertOne、Find、UpdateOne 等會自動處理 _oteltrace
```

可選：**ConnectWithOptions(traceOpts, mongoOpts)** 傳入 **WithTracerProvider(tp)** 或 **WithPropagators(p)**。

### 3. 從文件還原 trace（例如 change stream）

```go
outCtx := otelmongo.ContextFromDocument(ctx, fullDoc)
```

### 4. 測試

```go
otel.SetTracerProvider(trace.NewTracerProvider())
client, err := otelmongo.Connect(opts)
```

---

## API 摘要

| 項目 | 說明 |
|------|------|
| **Connect / ConnectWithOptions** | 未傳入 option 時使用 `otel.GetTracerProvider()`。 |
| **NewClient** | 可選 **WithTracerProvider**、**WithPropagators**。 |
| **ContextFromDocument** | 從文件的 `_oteltrace` 還原 trace context。 |
| **ScopeName / Version()** | 建立 Tracer 時使用（OTel contrib 規範）。 |
