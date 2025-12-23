# logtap JS SDK（Browser / Node）

用于向 logtap 网关上报「结构化日志」与「埋点事件」。

## 配置

```js
import { LogtapClient } from "logtap-sdk";

const client = new LogtapClient({
  baseUrl: "http://localhost:8080",
  projectId: 1,
  projectKey: "pk_xxx", // 启用 AUTH_SECRET 时必填
  gzip: true, // 浏览器需要支持 CompressionStream；否则会自动降级为非 gzip
  globalContexts: { app: { version: "1.2.3" } },
});
```

## 日志

```js
client.info("hello", { k: "v" });
client.error("boom", { err: String(err), stack: err?.stack });
```

## 埋点

```js
client.identify("u1", { plan: "pro" });
client.track("signup", { from: "landing" });
```

也支持在单次调用里带 `contexts/extra/user/deviceId`（覆盖全局默认值）：

```js
client.info("hello", { k: "v" }, { contexts: { page: { path: location.pathname } } });
```

事件上报会调用 `POST /api/:projectId/track/`，服务端会以 `logs.level=event` 落库并用于事件分析/漏斗。

## 自动捕获（可选）

```js
client.captureBrowserErrors(); // 浏览器：window.error / unhandledrejection
client.captureNodeErrors();    // Node：unhandledRejection / uncaughtException
```

## Flush / 退出前发送

SDK 默认每 2 秒自动 flush；也可手动调用：

```js
await client.flush();
await client.close();
```
