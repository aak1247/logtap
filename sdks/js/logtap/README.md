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
  // 本地持久化队列（可选）：应用重启后继续发送未发送成功的日志/事件
  // Browser: localStorage；Node: 文件
  // persistQueue: true,
  // queueStorageKey: "logtap_queue:prod", // Browser 可选：自定义 localStorage key
  // queueFilePath: "/tmp/logtap-queue.json", // Node 可选：自定义文件位置
  // 批处理：尽量攒够再发，避免请求过多
  flushIntervalMs: 5000, // 最大延迟（到时间也会发）
  minBatchSize: 20,      // 达到条数立即发
  // 关键事件：绕过批处理立即上报（只影响 track）
  immediateEvents: ["purchase", "payment_succeeded"],
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
client.track("purchase", { amount: 1 }, { immediate: true }); // 单次强制立即上报
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

SDK 会把日志/事件先放在本地队列里，满足「条数阈值」或「时间阈值」才上报；也可手动调用：

```js
await client.flush();
await client.close();
```
