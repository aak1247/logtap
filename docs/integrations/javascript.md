# JavaScript / TypeScript 集成

推荐使用官方 JS SDK（Browser/Node）。如果你不想引入 SDK，也可以直接用 `fetch` 调用上报 HTTP 接口。

相关文档：

- 上报接口与数据模型：`../INGEST.md`
- SDK 快速开始：`../SDKs.md`

## 1) 使用官方 JS SDK（Browser/Node）

目录：`sdks/js/logtap/`（包名：`logtap-sdk`）

安装（任选其一）：

```bash
npm i logtap-sdk
# 或 bun add logtap-sdk
# 或 pnpm add logtap-sdk
```

初始化与上报：

```ts
import { LogtapClient } from "logtap-sdk";

const client = new LogtapClient({
  baseUrl: "http://localhost:8080",
  projectId: 1,
  projectKey: "pk_xxx", // 开启 AUTH_SECRET 时必填
  gzip: true,
  flushIntervalMs: 5000, // 最大延迟（到时间也会发）
  minBatchSize: 20,      // 达到条数立即发
  immediateEvents: ["purchase", "payment_succeeded"],
  globalTags: { env: "prod" },
});

client.captureBrowserErrors(); // 浏览器可选
client.captureNodeErrors();    // Node 可选

client.info("hello", { k: "v" });
client.track("signup", { plan: "pro" });
```

## 2) 不使用 SDK：直接用 fetch

### 2.1 结构化日志

```ts
await fetch("http://localhost:8080/api/1/logs/", {
  method: "POST",
  headers: {
    "Content-Type": "application/json",
    "X-Project-Key": "pk_xxx",
  },
  body: JSON.stringify({
    level: "info",
    message: "hello",
    user: { id: "u1" },
    fields: { k: "v" },
    tags: { env: "prod" },
  }),
});
```

同一路径也支持批量（JSON 数组），返回同样是 `202 Accepted`：

```ts
await fetch("http://localhost:8080/api/1/logs/", {
  method: "POST",
  headers: {
    "Content-Type": "application/json",
    "X-Project-Key": "pk_xxx",
  },
  body: JSON.stringify([
    { level: "info", message: "hello-1", user: { id: "u1" }, fields: { k: "v1" } },
    { level: "info", message: "hello-2", user: { id: "u1" }, fields: { k: "v2" } },
  ]),
});
```

### 2.2 埋点/事件

```ts
await fetch("http://localhost:8080/api/1/track/", {
  method: "POST",
  headers: {
    "Content-Type": "application/json",
    "X-Project-Key": "pk_xxx",
  },
  body: JSON.stringify({
    name: "signup",
    user: { id: "u1" },
    properties: { plan: "pro" },
  }),
});
```

同一路径也支持批量（JSON 数组）：

```ts
await fetch("http://localhost:8080/api/1/track/", {
  method: "POST",
  headers: {
    "Content-Type": "application/json",
    "X-Project-Key": "pk_xxx",
  },
  body: JSON.stringify([
    { name: "signup", user: { id: "u1" }, properties: { plan: "pro" } },
    { name: "purchase", user: { id: "u1" }, properties: { sku: "s1" } },
  ]),
});
```
