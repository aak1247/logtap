# Alert Signal Detector Plugin (Iteration 3 Frontend) - Design Document

## 1. Document Control

- Feature name: Alert Signal Detector Plugin (Iteration 3 Frontend)
- Version: v3.0
- Author: Codex
- Last updated: 2026-02-17
- Status: `APPROVED`
- Review gate status: `PASS`
- Linked review report: `logtap/docs/specs/alert-signal-detector-v3-frontend-review.md`

## 2. Context and Problem

- Iteration 2 已完成 detector catalog/schema API、monitor create/update 校验、monitor 同步 test API。
- 当前前端缺口：
  - 无 detector 列表与 schema 展示能力，无法基于后端真实插件能力配置监控。
  - 无 monitor CRUD / run / test / runs 查询入口，运维只能靠 API 手工调用。
  - `run`（异步调度）与 `test`（同步预检）的语义在 UI 无清晰区分。

## 3. Goals and Non-Goals

- Goals:
  - 在现有 `AlertsPage` 内新增“监控插件”工作区。
  - 前端接入 detector catalog/schema API 并展示 schema。
  - 支持 monitor 的创建、编辑、删除、立即调度、同步试运行、运行历史查看。
  - 明确 `run` 与 `test` 的交互语义和结果展示。
- Non-goals:
  - 不改造通知渠道、联系人组、route 数据模型。
  - 不新增通知通道扩展能力。
  - 不实现 Cloud 端跨项目配置下发。

## 4. Scope

- In scope:
  - `web/src/lib/api.ts` 增加 detector/monitor 类型与 API 封装。
  - 新增前端组件承载监控插件管理 UI，并嵌入 `AlertsPage`。
  - 构建时类型校验与 UI 可用性验证。
- Out of scope:
  - 后端 API 协议修改。
  - 后端规则/路由引擎变更。

## 5. Requirements

- `FR-301`: 前端必须可查询 detector 列表并展示 `type/mode/path`。
- `FR-302`: 前端必须可按 detectorType 拉取 schema 并展示原始 JSON 结构。
- `FR-303`: 前端必须支持 monitor create/update/delete 并提交 `detectorType/config/intervalSec/timeoutMs/enabled`。
- `FR-304`: 前端必须支持 `POST /monitors/:id/run` 与 `POST /monitors/:id/test`，并在 UI 上区分“异步调度”与“同步预检”。
- `FR-305`: 前端必须支持查询 monitor runs 并显示状态、耗时、信号数、错误信息。
- `NFR-301`: 关键交互错误信息可读，直接透传后端错误文案。
- `NFR-302`: 所有新增 API 调用走现有鉴权与 envelope 处理，不绕过 `fetchJSON`。

## 6. Proposed Solution

- 在 `AlertsPage` 增加 `monitors` tab，减少导航结构改动，保持与现有告警配置并列。
- 新增独立组件 `MonitorTab` 管理 detector/monitor 状态，避免继续膨胀 `AlertsPage` 主文件。
- 配置编辑策略：
  - 采用“JSON 配置编辑器 + schema 只读预览”。
  - 若 schema 为空，仍允许 JSON 自由编辑（与后端兜底一致）。
- 执行策略：
  - `Run` 按钮仅提示“已入队调度”。
  - `Test` 按钮展示 `signalCount/elapsedMs/samples`，不展示通知投递结果。

## 7. Risks and Mitigation

- Risk: detector 描述字段大小写差异导致前端解析失败。
  - Mitigation: 在 API 层做大小写兼容归一化。
- Risk: JSON 配置输入门槛高。
  - Mitigation: 展示 schema 与示例占位，后续迭代再做 schema 驱动表单。

## 8. Acceptance Criteria

- `AC-301`: 前端可展示 detector 列表及 schema。
- `AC-302`: monitor create/update 在非法配置下显示后端 4xx 错误。
- `AC-303`: monitor `run` 与 `test` 均可触发，且 UI 文案明确两者语义差异。
- `AC-304`: 可查看指定 monitor 最近运行记录。
