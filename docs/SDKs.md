# 客户端 SDK（Browser/Node、Go、Flutter）

logtap 提供三端上报 SDK，用于向网关上报「结构化日志」与「埋点/事件」。

- 上报接口与数据模型：`./INGEST.md`
- SDK 能力/接口统一约定：`./SDK_SPEC.md`

## 共同配置

- `baseUrl`：例如 `http://localhost:8080`
- `projectId`：项目 ID
- `projectKey`：启用 `AUTH_SECRET` 时必填（`X-Project-Key: pk_...`）
- `gzip`：可选（SDK 会在不支持的平台自动降级）

## Browser / Node（JS SDK）

目录：`sdks/js/logtap/`（包名：`logtap-sdk`）

```js
import { LogtapClient } from "logtap-sdk";

const client = new LogtapClient({
  baseUrl: "http://localhost:8080",
  projectId: 1,
  projectKey: "pk_xxx",
  gzip: true,
  globalTags: { env: "prod" },
  globalContexts: { app: { version: "1.2.3" } },
});

client.captureBrowserErrors(); // browser 可选
client.captureNodeErrors();    // node 可选

client.identify("u1", { plan: "pro" });
client.info("hello", { k: "v" }, { tags: { req: "1" } });
client.track("signup", { from: "landing" });

await client.close();
```

## Go SDK

目录：`sdks/go/logtap/`（包：`github.com/aak1247/logtap/sdks/go/logtap`）

```go
client, err := logtap.NewClient(logtap.ClientOptions{
  BaseURL:    "http://localhost:8080",
  ProjectID:  1,
  ProjectKey: "pk_xxx",
  Gzip:       true,
  GlobalTags: map[string]string{"env": "prod"},
  GlobalContexts: map[string]any{
    "app": map[string]any{"version": "1.2.3"},
  },
})
if err != nil {
  panic(err)
}
defer client.Close(context.Background())
defer client.Recover(true) // 可选：捕获 panic 上报

client.Identify("u1", map[string]any{"plan": "pro"})
client.Info("hello", map[string]any{"k": "v"}, nil)
client.Track("signup", map[string]any{"from": "landing"}, nil)
```

## Flutter SDK

目录：`sdks/flutter/logtap_flutter/`（包名：`logtap_flutter`）

```dart
final client = await LogtapClient.create(
  const LogtapClientOptions(
    baseUrl: "http://localhost:8080",
    projectId: 1,
    projectKey: "pk_xxx",
    gzip: true, // Web 自动降级为非 gzip
    globalTags: {"env": "prod"},
    globalContexts: {"app": {"version": "1.2.3"}},
  ),
);

client.captureFlutterErrors(); // 可选：捕获未处理异常

client.identify("u1", {"plan": "pro"});
client.info("hello", {"k": "v"});
client.track("signup", {"from": "landing"});

await client.close();
```
