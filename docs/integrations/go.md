# Go 集成

推荐使用官方 Go SDK；也可以直接用 `net/http` 调用上报接口。

相关文档：

- 上报接口与数据模型：`../INGEST.md`
- SDK 快速开始：`../SDKs.md`

## 1) 使用官方 Go SDK

目录：`sdks/go/logtap/`（模块：`github.com/aak1247/logtap/sdks/go/logtap`）

```go
client, err := logtap.NewClient(logtap.ClientOptions{
  BaseURL:    "http://localhost:8080",
  ProjectID:  1,
  ProjectKey: "pk_xxx", // 开启 AUTH_SECRET 时必填
  Gzip:       true,
})
if err != nil {
  panic(err)
}
defer client.Close(context.Background())
defer client.Recover(true) // 可选：捕获 panic 上报

client.Info("hello", map[string]any{"k": "v"}, nil)
client.Track("signup", map[string]any{"plan": "pro"}, nil)
```

## 2) 不使用 SDK：直接 HTTP 上报

### 2.1 结构化日志

```go
payload := map[string]any{
  "level": "info",
  "message": "hello",
  "user": map[string]any{"id": "u1"},
  "fields": map[string]any{"k": "v"},
}

b, _ := json.Marshal(payload)
req, _ := http.NewRequest("POST", "http://localhost:8080/api/1/logs/", bytes.NewReader(b))
req.Header.Set("Content-Type", "application/json")
req.Header.Set("X-Project-Key", "pk_xxx")

resp, err := http.DefaultClient.Do(req)
if err != nil {
  panic(err)
}
defer resp.Body.Close()
```

### 2.2 埋点/事件

```go
payload := map[string]any{
  "name": "signup",
  "user": map[string]any{"id": "u1"},
  "properties": map[string]any{"plan": "pro"},
}

b, _ := json.Marshal(payload)
req, _ := http.NewRequest("POST", "http://localhost:8080/api/1/track/", bytes.NewReader(b))
req.Header.Set("Content-Type", "application/json")
req.Header.Set("X-Project-Key", "pk_xxx")

resp, err := http.DefaultClient.Do(req)
if err != nil {
  panic(err)
}
defer resp.Body.Close()
```
