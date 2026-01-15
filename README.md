# logtap

轻量化的 Sentry 兼容上报 + 自定义结构化日志网关（Go + Gin + NSQ + TimescaleDB/Postgres）。

## 用户/项目/鉴权（推荐）

当你设置了 `AUTH_SECRET`（base64，>= 32 bytes）后：

- 控制台必须先登录
- 上报必须携带项目 Key（`X-Project-Key: pk_...`，或 Sentry 的 `sentry_key`）
- 查询接口必须携带登录 Token（`Authorization: Bearer ...`）

生成 `AUTH_SECRET`：

- Taskfile：`task auth:secret`
- PowerShell：`powershell -ExecutionPolicy Bypass -File scripts/gen-auth-secret.ps1`
  - 然后把输出填到环境变量 `AUTH_SECRET`

首次初始化（系统无用户时）：

- 打开控制台 `/login`，使用「初始化管理员 + 默认项目」
- 或接口：`POST http://localhost:8080/api/auth/bootstrap`

常用接口（登录模式）：

- `POST /api/auth/login` → 返回 `token`
- `GET /api/me`（Header: `Authorization: Bearer <token>`）
- `GET /api/projects` / `POST /api/projects`
- `GET /api/projects/:projectId/keys` / `POST /api/projects/:projectId/keys` / `POST /api/projects/:projectId/keys/:keyId/revoke`

## 运行（本地 Docker）

```bash
cd deploy
docker compose up --build
```

- 网关：`http://localhost:8080`
- NSQ Admin：`http://localhost:4171`

## SDK（Browser/Node、Go、Flutter）

- SDK 快速开始：`docs/SDKs.md`
- 上报接口与数据模型：`docs/INGEST.md`
- SDK 统一能力与接口约定：`docs/SDK_SPEC.md`
- SDK 源码目录：`sdks/`
- 集成演示（demo）：`demo/README.md`

## 上报示例

### 1) Sentry store（兼容 `/api/:projectId/store/`）

```bash
curl -sS -X POST "http://localhost:8080/api/1/store/" \
  -H "Content-Type: application/json" \
  -d '{"event_id":"11111111-1111-1111-1111-111111111111","level":"error","message":"boom","timestamp":"2025-01-01T00:00:00Z"}'
```

如果启用了鉴权（设置了 `AUTH_SECRET`），需要额外带上项目 Key：

```bash
curl -sS -X POST "http://localhost:8080/api/1/store/" \
  -H "Content-Type: application/json" \
  -H "X-Project-Key: pk_xxx" \
  -d '{"event_id":"11111111-1111-1111-1111-111111111111","level":"error","message":"boom","timestamp":"2025-01-01T00:00:00Z"}'
```

### 1.1) Sentry SDK DSN 示例

SDK 通常会把 DSN 映射到 `/api/:projectId/envelope/`：

- DSN：`http://anykey@localhost:8080/1`
- 实际上报：`POST http://localhost:8080/api/1/envelope/`

## 无 Docker 部署（服务器）

前提：服务器已有 `PostgreSQL`（示例：`5432 / logtap / logtap / password`），并准备好 NSQ（至少 `nsqd`）。网关启动时会用 GORM `AutoMigrate` 自动建表/建索引。

1) 构建并上传网关二进制（需要 Go 1.22+）

`go build -o gateway ./cmd/gateway`

2) 配置并启动（systemd 可选）

- 环境变量参考 `.env.example`，最少需要：`NSQD_ADDRESS`、`HTTP_ADDR`，以及 `RUN_CONSUMERS=true` 时的 `POSTGRES_URL`
- systemd 模板：`deploy/systemd/logtap-gateway.service`、`deploy/systemd/nsqd.service`

## Windows 启动（无 Docker）

前提：本机有 Go 1.22+，并能访问本机 Postgres + 远端 NSQ。

启动网关（默认：本机 PG、远端 NSQ `172.168.1.226:4150`，启动时自动建表/索引）：

- PowerShell：`powershell -ExecutionPolicy Bypass -File scripts/run-gateway.ps1`
- Taskfile：`task run`

可覆盖参数（示例）：

`powershell -ExecutionPolicy Bypass -File scripts/run-gateway.ps1 -NSQDAddress "172.168.1.226:4150" -HTTPAddr ":8080"`

查看今日指标（需要本机 Redis `127.0.0.1:6379`）：

`GET http://localhost:8080/api/1/metrics/today`

查看活跃用户（DAU/MAU，去重口径：优先 `user.id`，否则 `device_id`）：

- `GET http://localhost:8080/api/1/analytics/active?bucket=day`
- `GET http://localhost:8080/api/1/analytics/active?bucket=month`

查看分布（默认近 7 天 Top 10）：

- `GET http://localhost:8080/api/1/analytics/dist?dim=os`
- `GET http://localhost:8080/api/1/analytics/dist?dim=browser`
- `GET http://localhost:8080/api/1/analytics/dist?dim=country`（需要 GeoIP City mmdb）
- `GET http://localhost:8080/api/1/analytics/dist?dim=asn_org`（需要 GeoIP ASN mmdb）

留存（基于“活跃用户”，近 14 天 cohort，默认 D1/D7/D30）：

- `GET http://localhost:8080/api/1/analytics/retention`
- `GET http://localhost:8080/api/1/analytics/retention?days=1,3,7`

事件分析（基于埋点事件：`logs.level=event`，`logs.message` 作为事件名）：

- `GET http://localhost:8080/api/1/analytics/events/top`
- `GET http://localhost:8080/api/1/analytics/funnel?steps=signup,checkout,paid&within=24h`

### GeoIP（可选：国家/运营商）

1) 去 MaxMind 注册并生成 License Key（GeoLite2 免费版即可）

2) 下载 mmdb（默认保存到 `data/geoip/`，并已加入 `.gitignore`）：

- Taskfile：`task geoip:download`
- PowerShell：`$env:MAXMIND_LICENSE_KEY="xxx"; powershell -ExecutionPolicy Bypass -File scripts/download-geoip.ps1`

3) 启动网关：`scripts/run-gateway.ps1` 会自动探测 `data/geoip/GeoLite2-City.mmdb` / `GeoLite2-ASN.mmdb` 并设置 `GEOIP_CITY_MMDB` / `GEOIP_ASN_MMDB`

## 控制台（React + Bun + Tailwind）

目录：`web/`

启动（开发模式）：

```bash
cd web
bun install
bun run dev
```

如果你的环境（尤其是老 CPU / 某些 CI）运行 Bun 会遇到 `SIGILL / Illegal instruction`（缺少 AVX），可改用 Node/NPM：

```bash
cd web
npm install
npm run dev
```

浏览器打开：`http://localhost:5173`

- 内置文档页：`/docs`（部署/集成/SDK 说明）
- 右上角「设置」里填 `API Base`（例如 `http://localhost:8080`）
- 登录模式下：先去 `/login` 登录，再到「项目」页选择项目并查看项目 Key（用于上报）
- 未启用登录（未设置 `AUTH_SECRET`）时：可在 `/login` 选择「无需登录（开发模式）」或在「设置」里填 `Project ID`
- 页面：概览（今日指标+最新事件）、分析（DAU/MAU）、事件列表/详情、日志搜索
  - 分析页额外展示：OS/国家/运营商 Top 分布（国家/运营商需 GeoIP）

### 2) 自定义日志（`/api/:projectId/logs/`）

```bash
curl -sS -X POST "http://localhost:8080/api/1/logs/" \
  -H "Content-Type: application/json" \
  -d '{"level":"info","message":"hello","trace_id":"t1","fields":{"k":"v"}}' -i
```

支持批量（JSON 数组）与 `Content-Encoding: gzip`（更适合浏览器/移动端 SDK 上报），例如：

```bash
curl -sS -X POST "http://localhost:8080/api/1/logs/" \
  -H "Content-Type: application/json" \
  -d '[{"level":"info","message":"hello-1"},{"level":"info","message":"hello-2"}]' -i
```

如果启用了鉴权（设置了 `AUTH_SECRET`），需要额外带上项目 Key：

```bash
curl -sS -X POST "http://localhost:8080/api/1/logs/" \
  -H "Content-Type: application/json" \
  -H "X-Project-Key: pk_xxx" \
  -d '{"level":"info","message":"hello","trace_id":"t1","fields":{"k":"v"}}' -i
```

### 3) 埋点/事件上报（`/api/:projectId/track/`）

事件会以 `logs.level=event` 写入（`logs.message` 作为事件名），用于事件分析/漏斗。

```bash
curl -sS -X POST "http://localhost:8080/api/1/track/" \
  -H "Content-Type: application/json" \
  -d '{"name":"signup","user":{"id":"u1"},"properties":{"plan":"pro"}}' -i
```

## 当前实现范围（MVP）

- 接收端点：`/api/:projectId/store/`、`/api/:projectId/envelope/`、`/api/:projectId/logs/`、`/api/:projectId/track/`
- 查询端点：`/api/:projectId/events/recent`、`/api/:projectId/events/:eventId`、`/api/:projectId/logs/search`
- NSQ Topics：`events`、`logs`
- 消费写库：`events` → `events` 表，`logs` → `logs` 表（TimescaleDB hypertable）
