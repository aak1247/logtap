# SDK 统一能力与接口（Browser/Node、Go、Flutter）

本文件描述 logtap 各端 SDK 的统一能力清单与对外接口（方法/配置项）的约定；上报 HTTP 接口与数据模型见 `./INGEST.md`。

## 统一能力（各端尽量对齐）

- 支持两类上报：
  - **结构化日志**：`POST /api/:projectId/logs/`
  - **埋点/事件**：`POST /api/:projectId/track/`
- 自动补齐基础字段：
  - `timestamp`：客户端本地时间（RFC3339/ISO8601）
  - `device_id`：默认生成；支持用户自定义与（部分端）持久化
  - `sdk`：`{name, version, runtime}`
- 批量与异步：
  - SDK 内部维护队列；按 `flushInterval` 定时 flush
  - 单次最多发送 `maxBatchSize` 条；队列上限 `maxQueueSize`（超出丢弃最旧）
- 可靠性：
  - 发送失败自动指数退避（上限 30s）
  - `flush()`/`close()`：显式触发发送；`close()` 会停止定时器并尽力发送剩余数据
- 压缩：
  - 可选 `gzip`（不支持的平台自动降级为非 gzip）
- 上下文合并：
  - `globalTags/globalFields/globalProperties/globalContexts` 会合并进每条 payload
  - 单次调用传入的 tags/fields/properties/contexts 覆盖同名键
- 用户标识：
  - `identify(userId, traits)` 或 `setUser(user)` 设置 `user` 字段
  - `clearUser()` 清理用户上下文
- 发送前钩子：
  - `beforeSend(payload)` 可用于脱敏/丢弃（返回 `null`/`nil` 表示丢弃）

## 建议对外 API（跨语言同名/同语义）

配置项（示例字段名以 JS 风格展示，各语言按习惯映射）：

- `baseUrl`（必填）
- `projectId`（必填）
- `projectKey`（可选；启用 `AUTH_SECRET` 时必填）
- `flushIntervalMs` / `flushInterval`
- `maxBatchSize`
- `maxQueueSize`
- `timeoutMs` / `timeout`
- `gzip`
- `deviceId` / `persistDeviceId`
- `user`
- `globalTags`
- `globalFields`（日志字段）
- `globalProperties`（事件属性）
- `globalContexts`
- `beforeSend`

方法：

- `log(level, message, fields?, options?)`
- `debug/info/warn/error/fatal(message, fields?, options?)`
- `track(name, properties?, options?)`
- `setUser(user?)` / `clearUser()`
- `identify(userId, traits?)`
- `setDeviceId(deviceId, options?)`
- `flush()`
- `close()`

各端可选增强：

- Browser：`captureBrowserErrors()`
- Node：`captureNodeErrors()`
- Go：panic 捕获 helper（可选）
- Flutter：`captureFlutterErrors()`（可选）
