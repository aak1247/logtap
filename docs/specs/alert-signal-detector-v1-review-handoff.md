# Review Handoff - Alert Signal Detector Plugin (Iteration 1)

## 1. Feature Context

- Feature name: Alert Signal Detector Plugin (Iteration 1)
- Business goal: 建立可扩展的“告警信号生产”能力，通知侧保持现状。
- Current iteration target: 一次迭代内落地 detector 插件底座 + 告警引擎信号入口。
- In scope:
  - Detector 插件接口/注册/动态加载
  - `AlertSignal` 统一模型
  - `Engine.EvaluateSignal` 与兼容桥接
- Out of scope:
  - 监控调度/API/UI
  - 通知渠道扩展与 worker 机制改造
  - 数据库 schema 变更

## 2. Attached Artifacts

- Design document link: `logtap/docs/specs/alert-signal-detector-v1-design.md`
- Execution plan link: `logtap/docs/specs/alert-signal-detector-v1-plan.md`
- Related references: `logtap/docs/monitoring-plugin-design.md`

## 3. Requested Review Focus

- Overdesign concerns to verify:
  - 在无 API/UI 的情况下，是否过早引入过多扩展抽象。
- One-iteration feasibility concerns:
  - `T-001`~`T-004` 是否能在一次迭代内完整交付。
- Missing design considerations to verify:
  - Go `plugin` 失败处理与回滚路径是否足够明确。

## 4. Known Risks and Constraints

- Technical constraints:
  - Go `plugin` 与构建环境强绑定。
  - 不能破坏现有日志/事件告警行为。
- Team/time constraints:
  - 只做一次迭代最小可交付。
- External dependencies:
  - 无外部服务依赖。
- Release constraints:
  - 必须可回退到纯静态 detector 路径。

## 5. Open Questions for Reviewer

- `Q-001`: Iteration 1 是否应保留 `source` 现状，仅靠字段匹配区分 detector？
- `Q-002`: 动态插件加载失败是否只做日志记录即可，还是需要暴露调试接口？

## 6. Handoff Acceptance

Handoff is complete only when all are true:

- Design doc and plan links are present
- Scope and iteration target are explicit
- At least one concrete concern or question is provided
