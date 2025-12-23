# 上报接口与数据模型（SDK 统一规范）

logtap 的客户端 SDK（Browser/Node、Go、Flutter）统一使用如下 HTTP 接口与 JSON 数据模型进行「日志」与「埋点/事件」上报。

## 基本约定

- Base URL：例如 `http://localhost:8080`
- Project ID：项目 ID（控制台项目页可见），作为路径参数：`/api/:projectId/...`
- 鉴权（推荐开启 `AUTH_SECRET`）：
  - 上报必须携带项目 Key：`X-Project-Key: pk_...`
  - 也兼容 `Authorization: Bearer pk_...` 与 Sentry SDK 的 `X-Sentry-Auth`（`sentry_key=...`）
- 批量上报：同一路径支持「单条对象」或「JSON 数组」
- 压缩：支持 `Content-Encoding: gzip`（浏览器使用 gzip 时需要预检允许 `Content-Encoding`）

## 1) 自定义日志上报

- Endpoint：`POST /api/:projectId/logs/`
- 成功：`202 Accepted`

### Payload：`CustomLogPayload`

```json
{
  "level": "info",
  "message": "hello",
  "timestamp": "2025-01-01T00:00:00Z",
  "device_id": "d1",
  "trace_id": "t1",
  "span_id": "s1",
  "fields": { "k": "v" },
  "tags": { "env": "prod" },
  "user": { "id": "u1" },
  "contexts": { "app": { "version": "1.2.3" } }
}
```

字段说明：

- `message`：必填
- `level`：建议值 `debug/info/warn/error/fatal`（为空时服务端会默认 `info`）
- `timestamp`：RFC3339；不传则服务端补当前时间（UTC）
- `user.id`：用于用户去重口径（DAU/MAU、漏斗等）；不传时会回退到 `device_id`
- `fields`：自定义结构化字段（建议把业务维度放这里）

## 2) 埋点/事件上报

- Endpoint：`POST /api/:projectId/track/`
- 成功：`202 Accepted`

### Payload：`TrackEventPayload`

```json
{
  "name": "signup",
  "timestamp": "2025-01-01T00:00:00Z",
  "user": { "id": "u1" },
  "device_id": "d1",
  "properties": { "plan": "pro", "ab": "b" }
}
```

服务端落库规则：

- 事件会写入 `logs` 表（用于事件分析/漏斗）
- `logs.level = "event"`
- `logs.message = name`
- `logs.fields = properties`

因此：

- 「日志」与「埋点事件」建议分开上报：日志走 `/logs/`，埋点走 `/track/`
- 事件分析只统计 `logs.level="event"`，不会被普通日志污染

