# 监控插件架构设计（v2）

## 1. 目标与约束

本版按以下约束设计：
- 插件扩展优先使用 Go `plugin`（`.so` 动态加载）。
- 同时支持“直接调用”模式（静态编译内置插件，不走 `plugin.Open`）。
- 不采用“先转日志再告警”的兼容路径，统一为“规则 -> 告警信号”。
- 通知发送不做插件化，继续复用现有告警通知实现。
- 支持图形化配置插件（尤其是 HTTP 监控）。
- 支持按告警来源/标签路由到不同联系人组与通知渠道。

非目标（本次迭代）：
- 不扩展新的通知渠道类型。
- 不改造 `alert-worker` 发送机制，仅在上游新增信号来源与路由能力。

## 2. 核心抽象

### 2.1 告警信号（统一中间模型）

```go
type AlertSignal struct {
    ProjectID   int
    SourceType  string            // http_check, process_check, log_basic...
    SourceID    string            // monitor_id 或 detector_id
    Severity    string            // info|warn|crit
    Status      string            // firing|resolved
    Title       string
    Message     string
    Labels      map[string]string // service/env/team/endpoint...
    Fields      map[string]any
    OccurredAt  time.Time
}
```

### 2.2 插件类型

仅一种插件类型：
1. **Detector 插件**：负责产出 `AlertSignal`（监测类插件）。

> 现有“日志告警”能力抽象为一个基础 Detector：`log_basic_detector`。

## 3. 插件加载与执行模型

### 3.1 双模式（同一接口）

- **Static 模式（直接调用）**：插件以 Go 包形式内置，通过注册表直接调用。
- **Dynamic 模式（Go plugin）**：从 `.so` 加载插件符号并注册到同一注册表。

统一接口：

```go
type DetectorPlugin interface {
    Type() string
    ConfigSchema() json.RawMessage
    ValidateConfig(cfg json.RawMessage) error
    Execute(ctx context.Context, req ExecuteRequest) ([]AlertSignal, error)
}
```

动态插件约定导出符号：

```go
var Plugin DetectorPlugin
```

### 3.2 兼容性要求（Go plugin）

- `go` 版本、编译参数、依赖版本必须与宿主一致。
- 建议通过同仓构建流程产出 `.so`，避免 ABI 漂移。

## 4. 执行链路（替代旧 Phase A）

`scheduler -> detector runner -> signal store -> policy engine -> route engine -> alert_deliveries(outbox) -> alert-worker`

说明：
- detector 直接产出 `AlertSignal`，不经日志中转。
- policy 只做“信号匹配与去重/抑制/恢复判定”。
- route 决定“通知谁、使用现有哪类渠道（webhook/企微/短信/邮件）”。
- outbox 与 worker 复用现有可靠投递模型。

## 5. 规则与路由解耦

### 5.1 Policy（规则）

输入：`AlertSignal`  
输出：`AlertEvent`（是否触发、是否恢复、聚合键、抑制结果）

规则示例：
- `source_type == "http_check" && severity == "crit"`
- `source_type == "log_basic_detector" && labels.service == "payment"`

### 5.2 Route（通知路由）

输入：`AlertEvent`  
输出：接收者组 + 通知通道目标（现有联系人组/渠道配置）

路由示例：
- `source_type=http_check` 且 `labels.env=prod` -> `SRE-OnCall` + `wecom`
- `source_type=log_basic_detector` 且 `labels.team=payments` -> `Payments-Dev` + `webhook`

这样即可实现“不同来源触发不同联系人组”。

## 6. 图形化配置设计

### 6.1 Schema 驱动表单

每个 Detector 插件提供 `ConfigSchema()`（JSON Schema），前端按 schema 渲染表单。

HTTP 插件 schema 示例字段：
- `name`（监控名）
- `endpoint.url`
- `endpoint.method`
- `endpoint.headers`（支持 secret 引用）
- `expect.status_codes`
- `expect.body_regex`
- `timeout_ms`
- `interval_sec`
- `labels`（service/env/team）

### 6.2 API 草案

- `GET /api/plugins/detectors`：插件列表（type/version/mode）
- `GET /api/plugins/detectors/:type/schema`：配置 schema
- `POST /api/:projectId/monitors`：创建监控项（type + config）
- `PUT /api/:projectId/monitors/:id`
- `POST /api/:projectId/monitors/:id/test`
- `GET /api/:projectId/monitors/:id/runs`
- `GET/POST /api/:projectId/alert-policies`
- `GET/POST /api/:projectId/alert-routes`

## 7. 数据模型建议

- `detector_plugins`：插件元数据（`type, mode(static|plugin), so_path, version, enabled`）
- `monitor_definitions`：监控定义（插件类型、配置、调度、标签）
- `monitor_runs`：执行历史
- `alert_signals`：标准化信号流水
- `alert_policies`：信号匹配规则与抑制策略
- `alert_routes`：来源/标签到接收组的路由
- `alert_deliveries`：沿用现有 outbox（可补充 `route_id/source_type`）

## 8. 与现有能力联动

- 通知侧：不引入通知插件，直接复用现有联系人组、webhook/企微/短信/邮件通道与 worker。
- 规则侧：将现有日志告警规则迁移为 `log_basic_detector + alert_policy`。
- 引擎侧：保留当前去重、退避、重试思想，迁移到 `policy + route + outbox` 分层。

## 9. 迭代路线

1. M1：引入 `AlertSignal`、`policy/route`，完成 `http_check`（static 模式）。
2. M2：接入 Go plugin 动态加载，增加 `process_check`。
3. M3：将现有日志告警迁移为 `log_basic_detector`，完成 UI 路由配置。
4. M4：插件签名校验、版本回滚、灰度启用。

## 10. 风险与决策点

- Go plugin 运维成本高（版本强绑定），需明确构建与发布规范。
- 多副本调度需严格 claim/lease，避免重复执行。
- route 规则复杂度上升，UI 需要提供模板与冲突检测。
