# logtap 云端：多租户 + 可替换存储（设计稿 v1）

目标读者：准备把 `logtap` 用作云端 data-plane（高写入/高查询）并由 `logtap-cloud` 作为 cloud-plane/edge 的维护者。

本设计稿的核心：让 **开源版与云端版尽量共用同一套业务语义与 API**，同时云端可以在不改 handler 业务逻辑的前提下：
- 实现软隔离多租户（tenant-scoped routing / query enforcement）
- 替换为更高性能的数据存储（如 ClickHouse 等）
- 支持后续硬隔离（按租户独立库/集群）而不推倒重来

---

## 1. 背景与现状

当前仓库结构：
- `logtap/`：data-plane（Go + Gin + NSQ + Postgres/Timescale + 可选 Redis）
- `logtap-cloud/`：cloud-plane + edge（项目/Key/登录/套餐/限流/配额）并代理到 `LOGTAP_UPSTREAM`

写入链路（现状）：
1) 客户端上报 → `logtap-cloud edge`（公网）
2) edge 校验 project key / cloud JWT，并反代到 `logtap`（私网）
3) `logtap` 接收 → 发布 NSQ → consumer 批量写入 Postgres/Timescale

现状没有「tenant」的显式概念；数据隔离主要依赖 `project_id`。

云端诉求：
- 多租户软隔离：同一套 data-plane 支撑多个租户（组织/账户）
- 存储后端可替换：高写入与查询需要比 Postgres 更强的后端（常见选择 ClickHouse）
- 开源版保持简单：不引入云端租户解析/鉴权逻辑，默认单租户即可

---

## 2. 目标与非目标

### 2.1 目标（Goals）

1) **Tenant identity**：data-plane 在 ingest/query 的所有路径上都能得到一个可信的 `tenant_id`。
2) **强制租户作用域**：任何落库/查询都必须 tenant-scoped，避免跨租户访问（软隔离）。
3) **异步链路透传**：tenant 信息必须从 HTTP → NSQ → consumer 全链路保真。
4) **可替换存储**：抽象出 data-plane 的存储接口（写入/查询/元数据）与后端实现解耦。
5) **开源版保持单租户**：默认固定 `tenant_id=1`，无 header 解析/无多租户鉴权逻辑。

### 2.2 容量与套餐（Sizing / Plans，先用约束反推）

当业务数据未知时，不建议先“拍一个 EPS”；更稳妥的方式是用套餐约束来反推容量与压测基线：
- 每租户：`月上报条数/字节数`、`峰值限流（eps）`、`保留天数`
- 云端运营：配额与限流在 edge 落地（拒绝越早越省成本）
- data-plane：按这些约束做压测基线与容量预估

建议起步档位（可按真实客户再调整）：
- Free：`1,000,000 events/月`、`7 天保留`、`50 eps/tenant`
- Team：`10,000,000/月`、`30 天保留`、`300 eps/tenant`
- Business：`100,000,000/月`、`90 天保留`、`2,000 eps/tenant`

粗略存储估算（用于第一版容量规划，后续用真实样本校准）：
- 单条 event/log 压缩后常见 `0.5–2KB`
- `10,000,000/月` ≈ `~3.9 eps` 平均；按 `10x` 峰值系数 ≈ `~40 eps`
- 数据量（不含索引/副本）：`5–20GB/月/tenant`

### 2.3 非目标（Non-goals）

- 本阶段不做“每租户强隔离（单独 VPC/独立集群）”的完整自动化运维（但接口设计需支持演进）。
- 不强制立即迁移现有 Postgres schema（允许先以“按租户路由到不同 DB/schema”的方式实现隔离）。
- 不在 data-plane 内实现 cloud-plane 级别的用户/组织权限体系（由 `logtap-cloud` 负责）。

---

## 3. 术语与身份边界

### 3.1 术语

- **Tenant**：云端租户（组织/账户）。设计上用 `tenant_id` 表示。
- **Project**：logtap 项目（已有 `project_id`）。
- **Cloud-plane**：`logtap-cloud`，负责账号/套餐/计费/权限。
- **Data-plane**：`logtap`，负责 ingest + query + 存储。

### 3.2 关键决策（v1）

以下为 v1 的明确决策（用于消除歧义、指导实现与测试）：
- `tenant_id`：UUID（建议存储为字符串，规范为小写、无花括号）。
- 外部 `projectId`（公网路径参数 `/api/:projectId/...`）：推荐 **全局唯一**（如 `p_...`），可显著降低 edge 查询与排错复杂度；若选择“仅在 tenant 内唯一”，必须在 edge 做 tenant-scope 查找与注入（见 §4.1.1），并确保所有计费/限流/指标 Key 都包含 tenant 维度以避免冲突。
- tenant 缺失时：默认 `tenant_id = "00000000-0000-0000-0000-000000000001"`（简称 “tenant=1” 的 UUID 常量；实现中可用常量名 `DefaultTenantID`）。
- 多租户隔离优先级：先保证“不会串租户”（安全/隔离）与“核心查询可用”（控制台常用路径），再做性能特化与语义完全对齐（详见 §6.3）。

### 3.3 信任边界（非常重要）

tenant header **只允许由 edge 注入**，客户端不可直连伪造。

已存在的安全机制：
- `logtap` 有 `LOGTAP_PROXY_SECRET`（通过 `X-Logtap-Proxy-Secret` 验证）用于区分“可信代理请求”。

本设计要求：
- 只有当 `logtap` 判定该请求 **proxy_ok**（`X-Logtap-Proxy-Secret` 校验通过）时，才会读取并信任 `X-LOGTAP-TENANTID`（或兼容 header）。
- 若不是 proxy_ok：开源版固定 `tenant_id=1`；云端可选择直接拒绝（401）或降级为固定值（不推荐）。

---

## 4. Tenant 注入与传播（HTTP → NSQ → Consumer）

### 4.1 HTTP 注入（Gin Middleware）

原则：**tenant 注入应写入标准 `context.Context`**，避免仅依赖 Gin 的 `c.Set`（更利于跨层调用与测试）。

推荐：在 `logtap` 增加 tenant helpers（公共分支允许存在，但默认固定 tenant）。

关键类型（建议）：

```go
// package tenant
type ID string

func With(ctx context.Context, id ID) context.Context
func From(ctx context.Context) (ID, bool)
func MustFrom(ctx context.Context) ID // 无则 panic（仅用于强约束点）
```

HTTP 中间件（开源版）：
- `FixedTenant(tenantID=DefaultTenantID)`：无条件把 tenant 写入 request context

HTTP 中间件（私有/云端分支）：
- `TenantFromProxyHeader()`：
  - 前置依赖：proxy secret middleware 已设置 `proxy_ok=true`
  - 若 proxy_ok：
    - 从 `X-LOGTAP-TENANTID`（或 `X-Logtap-Tenant-Id`）读取值并做格式校验
    - 将 tenant 写入 request context
  - 若非 proxy_ok：直接 401（推荐），避免被直连伪造

注意：该中间件应放在 ingest/query 的 group 上，且应在任何 handler 之前执行。

### 4.1.1 Edge 如何确定 tenant（projectId 仅 tenant 内唯一）

由于外部 `projectId` 仅在 tenant 内唯一，`logtap-cloud` edge 在“查 project / 校验权限 / 注入 tenant header”时必须避免仅凭 `projectId` 做全局查找。

推荐策略：
- Ingest 路径（`POST /api/:projectId/{logs,track,envelope,store}`）：
  1) 从请求中提取 project key（`X-Project-Key` / `Authorization: Bearer pk_...` / `X-Sentry-Auth`）。
  2) 用 key 反查 project（以及其所属 tenant），并验证路径 `:projectId` 与该 project 的 `public_project_id` 一致（不一致则 404/401）。
  3) 注入 `X-LOGTAP-TENANTID=<tenant_uuid>`，并反代到 upstream。
- Query 路径：
  1) 先校验 cloud JWT，得到用户所属 tenant（或组织）。
  2) 在 tenant scope 内查 project：`WHERE tenant_id=? AND public_project_id=?`。
  3) 注入 `X-LOGTAP-TENANTID=<tenant_uuid>`，并反代到 upstream。

这样可以确保：
- 不需要 `public_project_id` 全局唯一；
- 也不会因为 projectId 冲突造成“串租户”。

补充：如果外部 `projectId` 选择“全局唯一”（推荐），edge 可以继续用 `public_project_id` 直接查 project，再校验 key/JWT；tenant 注入仍然有价值（用于后端路由、清理与未来硬隔离），但不再依赖“tenant-scope 查找”来避免冲突。

### 4.2 NSQ 消息透传（必须做）

现状：`logtap/internal/ingest.NSQMessage` 不包含 tenant 信息，consumer 无法推导。

要求：所有发布到 NSQ 的消息都必须携带 `tenant_id` 字段（由 HTTP 中间件注入后读取）。

建议的消息 envelope：

```go
type NSQMessage struct {
  Type      string          `json:"type"`
  TenantID  string          `json:"tenant_id"`
  ProjectID string          `json:"project_id"`
  Received  time.Time       `json:"received"`
  Payload   json.RawMessage `json:"payload"`
  Meta      *MessageMeta    `json:"meta,omitempty"`
}
```

兼容性策略：
- consumer 解析 `tenant_id` 缺失时：
  - 默认：填充为 `DefaultTenantID`（兼容开源旧消息/灰度期）

可选方案（用于“公共分支尽量少改”）：
- **方案 1（推荐）**：公共分支把 `tenant_id` 作为 `omitempty` 字段加入 `NSQMessage`，但不做任何 header 解析，写入永远是 `DefaultTenantID`。好处：fork 差异最小，私有分支只需打开“读取 header + 写 tenant_id”即可。
- **方案 2（完全分叉）**：公共分支 NSQMessage 不含 `tenant_id`；私有分支自定义消息 envelope（或不同 topic）。好处：公共分支更“干净”；代价：后续合并冲突与工具链复用成本更高。

### 4.3 Consumer 侧上下文恢复

consumer 在处理消息时创建 `ctx := tenant.With(context.Background(), msg.TenantID)`（若缺失则用 `DefaultTenantID`），再把该 ctx 传入 store。

原则：**store 的租户路由/隔离只依赖 ctx，不依赖 handler**，避免遗漏。

---

## 5. 存储抽象（可替换后端）

### 5.1 分层建议

把存储按职责拆为三类接口，降低云端替换的耦合：

1) `MetadataStore`（低 QPS，强一致）
   - 用户/项目/Key/清理策略/告警配置等
   - 推荐继续用 Postgres（或 cloud-plane 自己的 DB）

2) `IngestStore`（高写入吞吐，append-heavy）
   - logs/events/track_events 写入
   - 云端可切到更强后端（如 ClickHouse、时序库）

3) `QueryStore`（读放大 + 聚合/搜索）
   - 搜索、聚合、漏斗、TopN 等
   - 云端后端往往与 ingest 后端同源（ClickHouse 优势明显）

### 5.2 Tenant 路由与隔离应在 store 实现层完成

对上层（handler/service/consumer）建议的约束：
- **只传 `context.Context`**，不在参数中显式传 `tenant_id`（减少上层多租户逻辑）
- store 实现从 ctx 读取 tenant，并强制加租户 scope（过滤条件或路由选择）

接口签名示例（建议）：

```go
// package storage
type IngestStore interface {
  InsertEvents(ctx context.Context, rows []model.Event) error
  InsertLogsAndTrackEvents(ctx context.Context, rows []model.Log) error
}

type QueryStore interface {
  SearchLogs(ctx context.Context, projectID int, q string, opts SearchOpts) (SearchResult, error)
  RecentEvents(ctx context.Context, projectID int, opts RecentOpts) ([]model.Event, error)
  Funnel(ctx context.Context, projectID int, params FunnelParams) (FunnelResult, error)
}
```

其中 `projectID` 仍然在方法参数中（它是业务维度），`tenantID` 只从 ctx 取（隔离维度）。

### 5.3 StorageRouter（多后端/分片）

云端需要按 tenant 路由到不同后端/集群（软隔离最实用的落地方式之一）。

建议引入：

```go
type Router interface {
  Ingest(ctx context.Context) (IngestStore, error)
  Query(ctx context.Context) (QueryStore, error)
  Metadata(ctx context.Context) (MetadataStore, error)
}
```

实现方式（云端常用）：
- `tenant_id -> backend shard`：一致性哈希/配置映射
- 每个 shard 内部维护连接池（ClickHouse/PG）
- 对 ingest 与 query 可以路由到不同集群（写入集群/查询集群），但需考虑数据延迟与一致性

### 5.4 接口面（v1 必需覆盖的 API 子集）

为了让“替换后端”真正可落地，建议先明确 data-plane 必须覆盖的最小查询/写入面（对应控制台与常用 API）。

#### 5.4.1 IngestStore（必须）

用于 consumer 批量落库（高吞吐、append-heavy）：

```go
type IngestStore interface {
  InsertEvents(ctx context.Context, rows []model.Event) error
  InsertLogsAndTrackEvents(ctx context.Context, rows []model.Log) error
}
```

约束：
- 幂等：相同 `(tenant_id, project_id, ingest_id)` 不得重复写入（至少“最终不重复”）。
- 时间戳：以 payload timestamp 为主（缺失则用 received/now），避免后端写入时钟漂移导致查询错位。

#### 5.4.2 QueryStore（P1 覆盖）

控制台核心能力优先（详见 §6.3 的优先级）：

```go
type QueryStore interface {
  RecentEvents(ctx context.Context, projectID int, opts RecentEventsOpts) ([]model.Event, error)
  GetEvent(ctx context.Context, projectID int, eventID string) (*model.Event, error)

  SearchLogs(ctx context.Context, projectID int, params SearchLogsParams) (SearchLogsResult, error)

  TopEvents(ctx context.Context, projectID int, params TopEventsParams) (TopEventsResult, error)
  Funnel(ctx context.Context, projectID int, params FunnelParams) (FunnelResult, error)
}
```

说明：
- `SearchLogsParams` 建议包含：`start/end`、`q`、`limit`、`cursor/offset`、`level`、`device_id/distinct_id` 等常用维度。
- 对 ClickHouse：cursor 分页通常比 offset 更稳（避免深分页抖动）；contract tests 要覆盖“重复翻页不丢不重”的语义。

#### 5.4.3 Cleanup/Retention（建议拆为可选能力）

清理/保留对不同后端实现差异很大（PG delete vs ClickHouse TTL）。建议通过 capability 明确：

```go
type RetentionManager interface {
  ApplyRetentionPolicy(ctx context.Context, projectID int, p RetentionPolicy) error
  RunCleanup(ctx context.Context, projectID int, now time.Time) (CleanupResult, error)
}
```

云端推荐：**保留策略由 cloud-plane 管理并下发**，后端优先用内建 TTL（更便宜、更稳定）。

#### 5.4.4 Capabilities（可选，但强烈建议）

用于让 handler 在少量场景下做“稳定降级”（例如 FTS 不支持时 fallback like，或明确返回 501）：

```go
type Capabilities struct {
  SupportsFTS bool
  SupportsJSONFieldQuery bool
  SupportsFunnel bool
}
```

---

## 6. 数据隔离方案（软隔离优先）

这里给三种可选方案，按“改造成本”排序；本设计默认优先推荐 B（低侵入）。

### A) 共享库 + tenant_id 列 +（可选）RLS

- 做法：所有表增加 `tenant_id` 列，所有查询强制 `WHERE tenant_id=?`；可叠加 Postgres RLS。
- 优点：单库管理简单、跨租户聚合（如果需要）更容易。
- 缺点：改造面大（所有表/索引/迁移/查询都要动），高写入下膨胀与索引维护成本更高。

### B) 按 tenant 路由到不同 DB/schema（推荐起步）

- 做法：schema 不改（或少改），store 根据 tenant 选择对应的 DB/schema 连接。
- 优点：最小侵入、隔离强、迁移成本低；开源版天然就是“单 tenant 的一个 DB”。
- 缺点：租户数非常大时连接与运维成本上升；需要管理 per-tenant 迁移（可批处理）。

注意点（非常容易踩坑）：
- **同库多 schema + 连接池**：如果用连接池复用 session，`search_path`/临时设置很容易串；建议优先用“独立 DB/独立用户”或在连接创建时固定 schema（并确保每次取连接都满足预期）。
- **共享 Redis/缓存**：若 data-plane 仍启用 Redis 指标/缓存且 Key 只包含 `project_id`，当不同 tenant 的 `project_id` 可能重叠时会发生混写；要么把 Redis 按 shard 隔离，要么把 key 变为 `tenant_id + project_id`（或引入全局唯一的 internalProjectId）。

### C) 共享 ClickHouse 表 + tenant 分区键

- 做法：ClickHouse 表把 `tenant_id` 作为主分区键/排序键的一部分；查询强制 tenant 过滤。
- 优点：高写入/聚合性能好，适合“对标 Sentry”的数据面。
- 缺点：需要较多查询改写与数据建模（物化视图/rollup/ttl 等）。

建议演进路线：
- Phase 1：先做 B（按 tenant DB/schema 路由），快速实现隔离与云端可运营
- Phase 2：引入 ClickHouse（C），并保持接口不变

### 6.3 新后端的“语义对齐”优先级（按资深工程经验）

如果目标是“对标 Sentry”的云端可用性，建议按如下顺序推进（先保证隔离与核心体验，再优化性能/覆盖边角）：

P0（必须，阻断上线的项）
- **隔离正确性**：任何读写都不可跨 tenant；tenant 缺失默认值仅允许在 proxy_ok 外的开源/兼容路径出现。
- **写入可靠性**：consumer 至少一次投递下的幂等（基于 ingest_id）保持不回归。
- **时间范围与排序**：所有查询必须在时间窗内正确过滤；排序稳定、分页可重复（避免翻页丢/重）。

P1（控制台核心功能）
- 事件列表/详情：`/events/recent`、`/events/:eventId`
- 日志搜索：`/logs/search`（先做 time range + keyword + limit；FTS 与 like 的差异可先用“能用”）
- Top Events / 基础聚合：`/analytics/events/top`（可先基于 rollup/materialized view）

P2（分析能力与性能一致性）
- 漏斗：`/analytics/funnel`（先保证不 OOM、结果可解释；ClickHouse 版可与 PG 版在边界上存在小差异，但要文档化）
- 分布/留存等：如果这些依赖 Redis 指标，优先保持边界一致即可（云端计费与配额不应依赖 data-plane 的 Redis 指标口径）

P3（语义精细对齐与高级搜索）
- FTS 中文/分词策略、字段全文检索一致性、复杂 filter（JSON path）、跨字段 relevance 等
- 告警/审计等高级功能（若云端需要）

---

## 7. 主要逻辑（端到端）

### 7.1 Ingest（日志/事件/埋点）

1) `logtap-cloud edge`：
   - 校验 project key / quota / rate limit
   - 注入：
     - `X-Logtap-Proxy-Secret: <secret>`
     - `X-LOGTAP-TENANTID: <tenant>`
   - 反代到 `logtap` upstream

2) `logtap`（HTTP）：
   - `accept/requireProxySecretMiddleware` 校验 proxy secret，标记 `proxy_ok`
   - （云端分支）tenant middleware 从 header 读取 tenant，写入 request ctx
   - handler 读取 tenant，从而构造 NSQMessage（含 tenant_id）并 publish

3) `logtap` consumer：
   - 从 NSQMessage 反序列化得到 tenant_id
   - 构造 ctx 写入 tenant
   - 通过 Router 选择 IngestStore
   - 批量写入后端

### 7.2 Query（搜索/分析/告警等）

1) `logtap-cloud edge`：
   - cloud JWT 校验
   - 注入 proxy secret + tenant header

2) `logtap`（HTTP）：
   - tenant middleware 注入 ctx
   - query handler 调用 Router.Query(ctx) 获取 QueryStore
   - QueryStore 强制 tenant scope（路由/过滤）

---

## 8. 配置与默认行为

### 8.1 开源版（默认）

- tenant 固定为 `DefaultTenantID`
- 不解析 `X-LOGTAP-TENANTID`
- store/router 只有一个后端（当前 Postgres/Timescale）

### 8.2 云端私有分支（建议）

新增配置（示例）：
- `TENANT_HEADER_NAME`（默认 `X-LOGTAP-TENANTID`）
- `TENANT_REQUIRE_PROXY_OK=true`（默认 true）
- `TENANT_STRICT=false`（默认 false；当 `true` 时，在 `proxy_ok` 场景若 tenant 缺失/非法则直接 401，避免“全部写入默认 tenant”这类事故）
- `STORAGE_ROUTER_MODE=single|sharded`
- `STORAGE_SHARDS_CONFIG=/path/to/shards.json`（tenant->shard 映射，或 hash ring 配置）

---

## 9. 安全与滥用防护

1) tenant header 必须在 proxy_ok 时才信任（否则可伪造越权）。
2) 若允许直连（不推荐），则 tenant 必须来自 `AUTH` token / JWT claim，而不是 header。
3) 指标维度不要轻易加 tenant（高基数风险）；可只在 edge 侧统计计费维度，data-plane 侧仅保留必要 debug 抽样。

生产建议（云端）：
- `TENANT_STRICT=true` 并且 data-plane 的 ingest/query 路径一律 **require proxy secret**（避免“可直连 + tenant fallback”导致的串租户事故）。
- 若担心静态 `LOGTAP_PROXY_SECRET` 泄露，可升级为 edge 签发短期 token（JWT/EdDSA 签名），包含 `tenant_id`、`project_id`、`exp`，data-plane 验签后再信任（header 仅承载 token 而不是明文 tenant）。

---

## 10. 可观测性与运维

建议新增（云端）：
- storage router 维度：每次路由选择记录 shard、错误率、p95/p99
- consumer lag：按 tenant 聚合会爆基数；建议按 shard 聚合
- ingest 拒绝：由 edge 做（配额/限流）并打点（tenant+plan 维度）

---

## 11. 测试策略（契约测试优先）

关键点：**同一套 contract tests 跑不同 store 实现**，保证“语义一致”。

建议：
- 为 `IngestStore`/`QueryStore` 建立最小契约：
  - 幂等：相同 ingest_id 不重复写（已存在设计）
  - 时间范围过滤、排序稳定性、分页一致性
  - FTS/like 的语义差异需明确（例如中文分词策略）
- 增加 **租户隔离** 契约用例：两租户各自写入同 `projectId`/同 `event_id`/相同查询条件时，查询结果与聚合不得互相可见。
- CI（开源）：只跑 Postgres/SQLite（现有）
- 私有 CI：额外跑 ClickHouse 实现（若有）

---

## 12. 迁移与发布（推荐分阶段）

### Phase 0：设计落地但不改变行为
- 引入 tenant context helper（固定为 1）
- NSQMessage 先兼容 tenant_id 字段（可选填）

### Phase 1：全链路 tenant 透传
- ingest handler 写 tenant_id 到 NSQMessage
- consumer 恢复 ctx 并调用 store（从 ctx 取 tenant）

### Phase 2：存储抽象与 router
- 引入 Router + store 接口
- Postgres 实现先落地为默认

### Phase 3：云端后端接入
- 私有分支实现 sharded router + 新后端（ClickHouse/其他）
- 灰度 tenant：按 tenant 映射部分流量到新后端（双写/读切换）

---

## 13. 开放问题（需要你确认的点）

已确认：
1) `tenant_id`：UUID
2) 外部 `projectId`：string public id（如 `p_...`，OSS 兼容 `"1"` 这类数字字符串），由 edge 映射到 data-plane 的 internal project id
3) tenant 缺失：开源版可 fallback 到 `DefaultTenantID`；云端建议 `TENANT_STRICT=true`（tenant 缺失/非法直接 401）

仍需确认（建议尽快定下来，便于写 contract tests）：
4) 新后端（如 ClickHouse）与现有 Postgres 版的“结果一致性”要求：
   - 必须完全一致的：时间窗、排序、分页、权限/隔离、幂等等
   - 允许存在差异但要文档化的：FTS relevance、中文分词、funnel 边界（比如去重口径/会话定义）
5) 外部 `projectId` 的唯一性策略：全局唯一（推荐）还是仅 tenant 内唯一；如果选后者，哪些系统（限流/配额/指标/缓存）必须引入 tenant 维度防冲突？
6) Router 的粒度：是“tenant -> shard（每 shard 多 tenant）”还是“tenant -> 独立 DB/集群”；二者对迁移、连接数、隔离等级的要求不同，需要明确上线时的默认落地形态。

---

## 14. 配套内容：tenant 的创建/管理（cloud-plane 负责）

本设计刻意让 data-plane 不关心“tenant 如何产生/管理”，只要求能拿到一个可信的 `tenant_id` 来做隔离与路由。

建议 `logtap-cloud` 负责以下能力（最小可运营闭环）：

### 14.1 数据模型（建议）

- `tenants`：
  - `id (uuid)`：tenant_id
  - `name`、`plan`、`status`
  - `created_at`
- `tenant_members`：
  - `tenant_id`、`user_id`、`role`（owner/admin/member）
- `projects`（cloud-plane 的项目表）：
  - `tenant_id`
  - `public_project_id`（tenant 内唯一）
  - `upstream_project_id`（data-plane 内部 numeric id，用于反代路径替换）
  - `tier/plan overrides`（若需要）
- `project_keys`：
  - `project_id`、`key`（建议全局唯一，便于 ingest 反查）

> 注：你现有的 `projects` 里有 `OwnerUserID` 与 `public_project_id`；引入 tenant 后建议逐步迁移为 `tenant_id + membership` 的模型。

### 14.2 创建流程（建议）

1) 用户注册/登录后创建 tenant（或首个 tenant）：
   - `POST /api/tenants`（或注册时自动创建）
2) 在 tenant 下创建 project：
   - `POST /api/tenants/:tenantId/projects`（生成 `public_project_id`，tenant 内唯一）
3) 为 project 生成 key：
   - `POST /api/tenants/:tenantId/projects/:projectId/keys`
4) 懒创建/显式创建 upstream project：
   - edge 调用 data-plane 的 internal API（受 `LOGTAP_PROXY_SECRET` 保护）创建项目并缓存 `upstream_project_id`
   - 或者创建 project 时同步创建 upstream（更直观，但更耦合）

### 14.3 管理与运营（建议）

- 成员/角色管理：谁能管理 project、查看数据、创建 key
- 套餐与配额：强制在 edge 执行（rate limit、monthly quota、retention policy）
- 审计：key 创建/撤销、成员变更、计划变更等
- 数据保留：优先在存储后端用 TTL（ClickHouse/Timescale），边界规则由 cloud-plane 配置下发
