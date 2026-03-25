# Alert Signal Detector Plugin (Iteration 2) - Design Document

## 1. Document Control

- Feature name: Alert Signal Detector Plugin (Iteration 2)
- Version: v2.0
- Author: Codex
- Reviewers: TBD
- Last updated: 2026-02-17
- Status: `APPROVED`
- Review gate status: `PASS`
- Linked review report: `logtap/docs/specs/alert-signal-detector-v2-review.md`

## 2. Context and Problem

- Business background: Iteration 1 已完成 detector 基础能力、monitor 定义与 worker 执行链路。
- Current pain points:
  - monitor 创建/更新未基于运行中 detector 注册表做强校验，配置错误会延迟到 worker 执行时暴露。
  - 缺少 detector catalog/schema API，前端无法基于后端真实插件能力渲染配置表单。
  - 监控“试运行”仅支持异步入队（`POST /monitors/:id/run`），缺少同步预检结果接口。
- Why now: 该缺口会直接影响配置体验与可运维性，且属于单迭代可收敛范围。
- Related links (issues, PRD, incidents, metrics):
  - `logtap/docs/specs/alert-signal-detector-v1-design.md`
  - `logtap/docs/specs/alert-signal-detector-v1-plan.md`

## 3. Goals and Non-Goals

- Goals:
  - 提供 detector catalog 与 schema 查询 API。
  - monitor 创建/更新时做 detector 存在性与配置合法性校验。
  - 提供 monitor 同步试运行 API，返回 detector 执行结果与信号摘要。
  - 保持现有通知发送链路不变。
- Non-goals:
  - 不新增通知渠道、通知插件或 route 模型重构。
  - 不引入新的 monitor 调度存储模型。
  - 不做 Cloud 端统一配置下发。

## 4. Scope

- In scope:
  - 新增 detector 服务层，统一注册表读取、配置校验与 schema 输出。
  - 新增 API：
    - `GET /api/plugins/detectors`
    - `GET /api/plugins/detectors/:type/schema`
    - `POST /api/:projectId/monitors/:monitorId/test`
  - monitor 创建/更新接口接入 detector 校验。
  - 补充单元测试与关键 handler 测试。
- Out of scope:
  - UI 页面开发（仅提供后端契约）。
  - 复杂权限模型变更（沿用当前鉴权中间件）。
  - worker 执行策略大改（claim/lease 逻辑保持）。
- Constraints (time, compliance, platform, compatibility):
  - 一次迭代内完成。
  - API 保持向后兼容；仅新增接口与更严格参数校验。
  - 动态 plugin 仍为可选，不依赖外部插件也可运行。

## 5. Users and Use Cases

- Target users/actors:
  - 前端控制台（动态表单）
  - 平台运维（配置与试运行）
- Key use cases:
  - 获取所有 detector 元信息和 schema，用于渲染配置表单。
  - 创建 monitor 时即时发现 `detectorType/config` 错误。
  - 在保存前对某 monitor 做同步试运行并查看返回信号。
- Edge cases:
  - detector schema 返回空时前端使用兜底 JSON 编辑器。
  - test 执行超时返回可读错误，不写入告警通知链路。

## 6. Requirements

### 6.1 Functional Requirements

- `FR-201`: 系统必须提供 detector catalog 查询接口，包含 `type/mode/path`。
- `FR-202`: 系统必须提供 detector schema 查询接口，返回插件 `ConfigSchema()`。
- `FR-203`: monitor 创建时必须校验 `detectorType` 存在且 `config` 可通过 `ValidateConfig`。
- `FR-204`: monitor 更新时，当 `detectorType/config` 变更必须执行同等校验。
- `FR-205`: 系统必须提供 monitor 同步试运行接口并返回执行结果（信号列表/错误/耗时）。
- `FR-206`: 试运行接口不得触发通知投递，不写 `alert_deliveries`。

### 6.2 Non-Functional Requirements

- `NFR-201` Performance: 试运行接口默认超时 <= monitor `timeout_ms`，最大不超过 120s。
- `NFR-202` Reliability: detector 缺失或配置错误必须在 API 层返回 4xx，不进入 worker 重试。
- `NFR-203` Security/Privacy: catalog 与 schema 接口沿用现有鉴权；不返回敏感配置值。
- `NFR-204` Observability: 试运行结果需记录结构化日志（成功/失败/耗时）。

## 7. Proposed Solution

- High-level approach:
  - 新增 `internal/detector/service`（或等价文件）封装：
    - `ListDescriptors()`
    - `GetSchema(type)`
    - `Validate(type, cfg)`
    - `TestExecute(type, req)`
  - `query/monitors.go` 创建/更新路径改为调用该服务校验。
  - 新增 `query/detectors.go` 暴露 catalog/schema API。
  - 新增 monitor test handler：读取 monitor 定义、同步执行 detector、返回标准结果，不调用 `alert.Engine.EvaluateSignal`。
- Architecture impact:
  - 在 monitor API 层新增“配置前置校验与试运行”，调度/告警路径不变。
- Components/modules affected:
  - `logtap/internal/detector/*`（服务封装、可测试接口）
  - `logtap/internal/query/monitors.go`（校验接入）
  - `logtap/internal/query/detectors.go`（新增）
  - `logtap/internal/httpserver/httpserver.go`（新路由）
  - `logtap/cmd/gateway/main.go`（把 registry 透传到 query 层）
- Data model changes:
  - 无新增表，无 schema 变更。
- API/contract changes:
  - 新增 3 个 API；monitor create/update 返回更严格参数错误。
  - `run` 与 `test` 语义边界：
    - `POST /monitors/:id/run`：异步入队，走 worker 与告警链路。
    - `POST /monitors/:id/test`：同步预检，不写 `monitor_runs`、不写 `alert_deliveries`。
- UI/UX changes:
  - 前端可通过 schema 渲染配置表单，并支持 test 按钮获取实时反馈。
- Backward compatibility:
  - 已存在 monitor 记录保持可读；非法配置在“更新或试运行”时暴露。

## 8. Alternatives and Tradeoffs

- Option A: 继续依赖 worker 执行时兜底报错。
- Option B: 在 API 层前置校验 + 增加同步 test（本方案）。
- Selected option: Option B。
- Tradeoff summary:
  - 优点：错误前置、配置体验提升、降低无效调度成本。
  - 代价：API 层与 detector 运行时耦合增强，需要显式传递 registry 依赖。

## 9. Risk Assessment

- Risk: registry 生命周期管理不当导致 query 层空指针或状态不一致。
- Impact: catalog/schema/test 接口不可用。
- Mitigation: 在 server 初始化阶段构建单一 registry 实例并注入；空 registry 返回可诊断错误。
- Fallback/rollback: 保留旧 monitor create/update 路径的软校验开关（必要时回退）。

- Risk: 同步 test 接口执行耗时过高占用 HTTP 资源。
- Impact: API 延迟升高。
- Mitigation: 强制超时、并发控制（后续可加限流）；本迭代先基于 timeout + context cancel。
- Fallback/rollback: 关闭 test 接口路由（配置开关或临时回退）。

## 10. Validation Strategy

- Unit testing strategy:
  - detector service：catalog/schema/validate/test 执行路径测试。
  - monitor handler：创建/更新校验成功与失败场景。
- Integration/E2E strategy:
  - query 层 handler 测试覆盖 `GET detectors`、`GET schema`、`POST monitor test`。
- Manual verification:
  - 启动服务后调用 catalog/schema 接口检查返回。
  - 创建错误配置 monitor，确认返回 4xx。
  - 执行 test 接口并确认不产生 `alert_deliveries`。
- Metrics and alerts:
  - 先写结构化日志统计 test 成功/失败；指标化延后。
- Acceptance criteria:
  - `AC-201`: 新增 API 覆盖核心成功/失败路径测试通过。
  - `AC-202`: monitor create/update 在非法 detector/config 下返回 400。
  - `AC-203`: monitor test 接口返回信号摘要（默认不返回全量字段）且不写通知 outbox。

## 11. Rollout Plan

- Release phases:
  1. 合入 detector service + catalog/schema API。
  2. 合入 monitor 校验与 test API。
  3. 文档更新与灰度启用。
- Feature flags:
  - 可选：`ENABLE_MONITOR_TEST_API`（默认 true；仅用于紧急回退）。
- Migration/backfill:
  - 无。
- Rollback triggers:
  - monitor API 4xx 异常激增或 test 接口超时率异常。
- Communication plan:
  - 更新后端 API 文档并通知前端切换 schema 驱动配置。

## 12. Open Questions

- `Q-201`: 是否需要在本迭代返回 schema 版本号（当前决策：先不要求，后续可加）？
- `Q-202`: test 接口是否需要落库 run 记录（当前决策：本迭代不落库，只返回结果）？
- `Q-203`: test 响应是否返回全量信号 fields（当前决策：默认仅摘要与 sample，避免暴露敏感字段）？

## 13. Review Feedback Tracking

- Required changes from latest review:
  - `A-201`: 明确试运行不触发通知链路。
  - `A-202`: 收紧范围，避免把监控 UI 一并纳入本迭代。
  - `A-203`: 明确 run/test 语义边界与 test 返回粒度。
- Applied changes:
  - 已在 `FR-206` 和第 7 节写明 test 不触发 `alert_deliveries`。
  - 已在 Scope/Non-goals 中明确排除 UI 与数据模型扩张。
  - 已补充 run/test 边界定义，并在 `AC-203` 与 `Q-203` 明确 test 返回摘要策略。
- Remaining review actions:
  - 无。

## 14. Definition of Design Complete

Mark design as complete only when all conditions pass:

- All required sections are filled
- Every requirement has an ID
- Latest review gate status is `PASS`
- Main risks and rollback are documented
- Validation strategy and acceptance criteria are measurable
- Open questions are either resolved or explicitly assigned
