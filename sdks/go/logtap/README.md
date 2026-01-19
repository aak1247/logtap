# logtap Go SDK

用于向 logtap 网关上报「结构化日志」与「埋点事件」。

## 安装/导入

本仓库内置 Go SDK 包：

```go
import "github.com/aak1247/logtap/sdks/go/logtap"
```

## 使用

```go
client, err := logtap.NewClient(logtap.ClientOptions{
  BaseURL:    "http://localhost:8080",
  ProjectID:  1,
  ProjectKey: "pk_xxx", // 启用 AUTH_SECRET 时必填
  Gzip:       true,
  // 批处理：尽量攒够再发，避免请求过多
  FlushInterval: 5 * time.Second, // 最大延迟（到时间也会发）
  MinBatchSize:  20,              // 达到条数立即发
  // 关键事件：绕过批处理立即上报（只影响 Track）
  ImmediateEvents: []string{"purchase", "payment_succeeded"},
})
if err != nil {
  panic(err)
}
defer client.Close(context.Background())

client.Info("hello", map[string]any{"k": "v"}, nil)
client.Identify("u1", map[string]any{"plan": "pro"})
client.Track("signup", map[string]any{"from": "landing"}, nil)
client.Track("purchase", map[string]any{"amount": 1}, &logtap.TrackOptions{Immediate: true})

_ = client.Flush(context.Background())
```

## Panic 捕获（可选）

```go
defer client.Recover(true) // 上报 panic，然后重新 panic
```
