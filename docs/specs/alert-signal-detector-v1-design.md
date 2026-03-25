# Alert Signal Detector Plugin (Iteration 1) - Design Document

## 1. Document Control

- Feature name: Alert Signal Detector Plugin (Iteration 1)
- Version: v1.0
- Author: Codex
- Reviewers: TBD
- Last updated: 2026-02-15
- Status: `APPROVED`
- Review gate status: `PASS`
- Linked review report: `logtap/docs/specs/alert-signal-detector-v1-review.md`

## 2. Context and Problem

- Business background: 现有告警能力已覆盖规则匹配与通知投递，但“产生告警信号”的来源仍主要绑定日志/事件链路。
- Current pain points: 缺少统一可扩展入口来接入 HTTP/进程/自定义检测信号，且无法同时支持“静态内置”与 Go `plugin` 动态扩展。
- Why now: 本次迭代目标是建立插件化信号入口，同时保持通知侧稳定，避免改动过大。
- Related links (issues, PRD, incidents, metrics): `logtap/docs/monitoring-plugin-design.md`

## 3. Goals and Non-Goals

- Goals:
  - 提供统一 `AlertSignal` 模型，作为告警评估输入。
  - 提供 Detector 插件接口与注册中心，支持 `static` 与 Go `plugin` 双模式。
  - 在不改通知发送机制的前提下，让告警引擎支持信号入口。
  - 保持现有日志/事件告警行为兼容。
- Non-goals:
  - 不引入通知插件机制。
  - 不做监控项调度器、数据库新表、监控 UI 表单。
  - 不扩展新的通知渠道类型。

## 4. Scope

- In scope:
  - 新增 Detector 插件基础接口、注册中心、动态加载器。
  - 新增 `AlertSignal` 数据结构与 `Engine.EvaluateSignal` 入口。
  - 新增内置 `log_basic` detector（作为桥接插件），用于兼容现有日志/事件输入。
  - 新增必要单元测试。
- Out of scope:
  - `/api/:projectId/monitors` 等监控配置 API。
  - 规则路由模型重构（保留现有 `alert_rules.targets`）。
  - Cloud 侧管理面改造。
- Constraints (time, compliance, platform, compatibility):
  - 一次迭代内完成。
  - 兼容现有 `alert_rules` / `alert_deliveries` 数据模型与 worker。
  - Go `plugin` 仅作为可选能力，不影响主流程。

## 5. Users and Use Cases

- Target users/actors:
  - 后端开发者（新增 detector）
  - 运维/平台工程师（部署 detector）
- Key use cases:
  - 使用静态 detector 直接注册并产生 `AlertSignal`。
  - 从 `.so` 加载 detector 并注册。
  - 将 `AlertSignal` 交由现有引擎评估并走现有通知链路。
- Edge cases:
  - 动态插件加载失败时不影响服务启动（记录错误并跳过）。
  - 插件重复注册时拒绝覆盖。

## 6. Requirements

### 6.1 Functional Requirements

- `FR-001`: 系统必须定义统一 `AlertSignal` 模型，能表达 `project/source/severity/status/message/labels/fields/time`。
- `FR-002`: 系统必须提供 Detector 插件接口，支持配置校验与执行产信号。
- `FR-003`: 系统必须支持静态注册 detector。
- `FR-004`: 系统必须支持从 Go `plugin` 动态加载 detector（可选启用）。
- `FR-005`: 告警引擎必须新增 `EvaluateSignal` 入口，并复用现有通知投递链路。
- `FR-006`: 现有日志/事件告警入口行为必须保持兼容。

### 6.2 Non-Functional Requirements

- `NFR-001` Performance: `EvaluateSignal` 不得引入比现有 `Evaluate` 更高数量级开销（同类输入近似等价）。
- `NFR-002` Reliability: 插件加载失败不应导致网关不可用。
- `NFR-003` Security/Privacy: 动态加载目录需显式配置；不执行远程下载插件。
- `NFR-004` Observability: 插件加载成功/失败及注册情况需可记录。

## 7. Proposed Solution

- High-level approach:
  - 新增 `internal/detector` 包，提供 `AlertSignal`、`DetectorPlugin`、`Registry`、`LoadPluginFile`。
  - `internal/alert` 新增信号转换与 `EvaluateSignal`，内部复用现有规则匹配与投递逻辑。
  - 现有 `Evaluate(Input)` 保持入口不变，通过桥接转换路径保持兼容。
- Architecture impact:
  - 告警链路前置一个可扩展“信号输入层”，通知链路不变。
- Components/modules affected:
  - `logtap/internal/detector/*`（新增）
  - `logtap/internal/alert/*`（扩展）
  - `logtap/internal/config/config.go`（可选新增插件目录配置）
  - `logtap/cmd/gateway/main.go`（可选初始化注册与加载）
- Data model changes:
  - 无数据库 schema 变更（Iteration 1）。
- API/contract changes:
  - Go 内部 contract 新增：`DetectorPlugin`、`Engine.EvaluateSignal`。
- UI/UX changes:
  - 无。
- Backward compatibility:
  - 对现有 API 和通知行为保持兼容。

## 8. Alternatives and Tradeoffs

- Option A: 先做监控调度/API/UI，再补插件。
- Option B: 先做信号插件底座 + 引擎入口（本方案）。
- Selected option: Option B。
- Tradeoff summary:
  - 优点：迭代小、风险低、可验证性高。
  - 代价：本迭代无法提供完整监控配置 UI。

## 9. Risk Assessment

- Risk: Go `plugin` 版本耦合导致运行时加载失败。
- Impact: 动态插件不可用。
- Mitigation: 静态注册作为主路径；动态加载失败仅告警日志。
- Fallback/rollback: 关闭动态加载配置，保留静态 detector。

- Risk: 新信号入口引入兼容回归。
- Impact: 现有日志/事件告警误触发或漏触发。
- Mitigation: 增加兼容性回归测试（旧入口与新入口等价案例）。
- Fallback/rollback: 保留旧 `Evaluate(Input)` 主路径，必要时绕过 `EvaluateSignal`。

## 10. Validation Strategy

- Unit testing strategy:
  - `Registry` 注册/重复注册/查询测试。
  - `LoadPluginFile` 失败路径测试（非法路径/符号缺失）。
  - `EvaluateSignal` 与 `Evaluate(Input)` 结果一致性测试。
- Integration/E2E strategy:
  - 暂不新增跨进程 E2E；保持现有 alert integration tests 通过。
- Manual verification:
  - 启动网关，确认日志中 detector 注册与加载信息。
  - 通过测试输入触发一次告警并进入 `alert_deliveries`。
- Metrics and alerts:
  - 先采用日志可观测；指标增强留到后续迭代。
- Acceptance criteria:
  - `AC-001`: 新增 detector 包测试通过。
  - `AC-002`: 现有 `internal/alert` 单测全部通过。
  - `AC-003`: 旧入口行为不变（关键用例回归通过）。

## 11. Rollout Plan

- Release phases:
  1. 合入信号插件底座与兼容桥接。
  2. 默认启用静态 detector；动态加载按配置开启。
- Feature flags:
  - `DETECTOR_PLUGIN_DIRS`（为空则关闭动态加载）。
- Migration/backfill:
  - 无数据迁移。
- Rollback triggers:
  - 告警触发行为异常、加载错误频发。
- Communication plan:
  - 在开发文档中补充 detector 开发规范与加载约束。

## 12. Open Questions

- `Q-001`: 是否在 Iteration 2 引入 `source=synthetic` 规则枚举（当前决策：Iteration 1 不引入，保持兼容）？
- `Q-002`: 动态插件加载失败是否需要暴露到 `/debug/vars`（当前决策：Iteration 1 仅日志，后续迭代再评估指标化）？

## 13. Review Feedback Tracking

- Required changes from latest review:
  - `A-001`: 计划中的验证口径收敛为自动化优先。
  - `A-002`: 开放问题补充本迭代默认决策。
- Applied changes:
  - 已在执行计划 `T-004` 中明确“自动化验证 + 日志补充证据”。
  - 已在 `12. Open Questions` 写入当前决策与后续方向。
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
