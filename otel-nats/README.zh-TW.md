# natstrace

**[English](README.md)**

---

OpenTelemetry 分散式追蹤包裝 [NATS](https://nats.io/) 與 [NATS JetStream](https://docs.nats.io/nats-concepts/jetstream)，對齊官方 `nats.go` / `nats.go/jetstream` API，並在訊息 header 中傳播 W3C Trace Context。

---

## 架構概覽

```
pkg/natstrace/
├── natstrace/           # Core NATS 連線與 Pub/Sub
│   ├── otel.go          # InitTracer, ShutdownTracer, WithTracerProvider
│   ├── connect.go       # Connect, ConnectTLS, ConnectWithCredentials
│   ├── conn.go          # Conn: Publish, PublishMsg, Subscribe, QueueSubscribe, Request
│   ├── propagation.go   # HeaderCarrier (nats.Header ↔ TextMapCarrier)
│   └── doc.go
├── jetstreamtrace/      # JetStream 串流與消費者
│   ├── jetstream.go     # New, JetStream, Publish, CreateOrUpdateStream
│   ├── stream.go        # Stream, Consumer, CreateOrUpdateConsumer
│   ├── consumer.go      # Consume, Messages, Fetch, FetchBytes, FetchNoWait, MessageBatch
│   └── doc.go
├── go.mod
└── README.md
```

- **Tracer 與 Propagator**：由 **global** 提供。必須先呼叫 **`InitTracer(endpoint, attrs...)`**，之後 `Connect` / `ConnectTLS` / `ConnectWithCredentials` 與 `jetstreamtrace.New(conn)` 才會使用同一個 TracerProvider 與 TextMapPropagator（TraceContext + Baggage）。
- **連線**：`natstrace.Connect(url, natsOpts)` 回傳 **`*natstrace.Conn`**，可當作 `*nats.Conn` 使用；Publish/Request 多一個 `context.Context`，Subscribe 的 handler 收到 **`MsgWithContext`**（m.Msg、m.Context()）；型別 **MsgHandler** 與 nats.MsgHandler 命名一致。
- **JetStream**：`jetstreamtrace.New(conn)` 需要 **`*natstrace.Conn`**。Publish 接受 `context.Context`；Consume / Messages / Fetch 皆使用 **MsgWithContext**；handler 型別 **MsgHandler** 與 natstrace、nats.go 一致。

---

## 使用方式

### 1. 初始化（必須在 Connect 之前）

```go
import (
    "go.opentelemetry.io/otel/attribute"
    "github.com/Marz32onE/natstrace/natstrace"
)

func main() {
    if err := natstrace.InitTracer("", attribute.String("service.name", "my-service")); err != nil {
        log.Fatal(err)
    }
    defer natstrace.ShutdownTracer()

    conn, err := natstrace.Connect(natsURL, nil)
    if err != nil {
        log.Fatal(err) // 若未呼叫 InitTracer 會得到 ErrInitTracerRequired
    }
    defer conn.Close()
    // ...
}
```

- 空字串 `endpoint` 會使用環境變數 `OTEL_EXPORTER_OTLP_ENDPOINT` 或 `localhost:4317`。
- HTTP 端點（例如 `http://...` 或 port 4318）會用 OTLP/HTTP，其餘用 OTLP/gRPC。

### 2. Core NATS：Publish / Subscribe

```go
// Publish：傳入 context 以注入 trace
conn.Publish(ctx, "subject", []byte("data"))
conn.PublishMsg(ctx, msg)

// Subscribe：handler 收到 MsgWithContext（m.Msg、m.Context()）
conn.Subscribe("subject", func(m natstrace.MsgWithContext) {
    // m.Context() 帶有從 header 解出的 trace
})
conn.QueueSubscribe("subject", "queue", handler)

// Request
reply, err := conn.Request(ctx, "subject", []byte("ping"), 2*time.Second)
```

### 3. JetStream

```go
js, err := jetstreamtrace.New(conn)
// 建立 stream / consumer 後：
cons.Consume(func(m jetstreamtrace.MsgWithContext) {
    // m 實作 Msg（m.Data()、m.Ack()）；m.Context() 帶有從訊息 header 解出的 trace
})
// 或
iter, _ := cons.Messages()
ctx, msg, err := iter.Next()
// 或
batch, _ := cons.Fetch(5, jetstream.FetchMaxWait(time.Second))
for m := range batch.MessagesWithContext() {
    // m.Data()、m.Ack()、m.Context() — MsgWithContext 對齊 jetstream.Msg
}
```

### 4. 底層 *nats.Conn

`conn.NatsConn()` 回傳 `*nats.Conn`，供需要原生 API 的程式使用。`conn.TraceContext()` 回傳目前使用的 Tracer 與 TextMapPropagator（jetstreamtrace 內部使用）。

---

## API 約定

| 項目 | 說明 |
|------|------|
| **InitTracer** | 必呼叫一次；設定 global TracerProvider 與 TextMapPropagator。 |
| **Connect 簽名** | `Connect(url string, natsOpts []nats.Option)`，不再接受 WithTracerProvider / WithPropagator。 |
| **錯誤** | 未先 InitTracer 就 `Connect` 會回傳 **`ErrInitTracerRequired`**。 |
| **測試** | 測試中可呼叫 `natstrace.InitTracer("", natstrace.WithTracerProviderInit(tp))`（若有自訂 propagator 可先 `otel.SetTextMapPropagator(prop)`），再 `Connect(url, nil)`。 |

---

## 依賴

- `github.com/nats-io/nats.go`（含 JetStream）
- `go.opentelemetry.io/otel` 與 SDK（trace、propagation、OTLP exporter）
- Go 1.25+

測試依賴 `github.com/stretchr/testify` 與 `nats-server/v2`（整合測試）。
