# logtap 概览

logtap 是一个轻量化的 **Sentry 兼容上报网关** + **自定义结构化日志/埋点网关**（Go + Gin + NSQ + TimescaleDB/Postgres），并提供一个 React 控制台用于查询与分析。

## 你能用它做什么

- 兼容 Sentry 的 `store/envelope` 上报路径，方便复用现有 Sentry SDK 或接入链路
- 用统一的 HTTP 接口上报：
  - **结构化日志**：`POST /api/:projectId/logs/`
  - **埋点/事件**：`POST /api/:projectId/track/`
- 控制台提供：概览指标、DAU/MAU、事件列表/详情、日志搜索、事件分析/漏斗等

## 快速开始

- 部署：见 `DEPLOYMENT.md`
- 上报模型与接口：见 `INGEST.md`
- SDK（JS/Go/Flutter）：见 `SDKs.md` 与 `SDK_SPEC.md`
- 语言集成教程：见 `integrations/javascript.md`（也可从文档页左侧导航进入）

## 鉴权模式（推荐）

当设置了 `AUTH_SECRET`（base64，>= 32 bytes）后：

- 控制台需要登录
- 上报必须携带项目 Key（`X-Project-Key: pk_...`，或 Sentry 的 `sentry_key`）

生成 `AUTH_SECRET`：

- Taskfile：`task auth:secret`
- PowerShell：`powershell -ExecutionPolicy Bypass -File scripts/gen-auth-secret.ps1`
