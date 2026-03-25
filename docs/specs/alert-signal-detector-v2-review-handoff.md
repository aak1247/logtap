# Review Handoff - Alert Signal Detector Plugin (Iteration 2)

## 1. Feature Context

- Feature name: Alert Signal Detector Plugin (Iteration 2)
- Business goal: 提升 monitor 配置可用性（catalog/schema/前置校验/同步试运行），降低无效调度与迟后报错。
- Current iteration target: 在不改通知通道与数据库模型的前提下，完善 detector 配置面后端能力。
- In scope:
  - detector catalog/schema API
  - monitor create/update 前置 detector 校验
  - monitor 同步 test API
  - registry 统一注入
- Out of scope:
  - 通知链路与 route 重构
  - monitor worker 调度算法重构
  - UI 实现

## 2. Attached Artifacts

- Design document link: `logtap/docs/specs/alert-signal-detector-v2-design.md`
- Execution plan link: `logtap/docs/specs/alert-signal-detector-v2-plan.md`
- Related references:
  - `logtap/docs/specs/alert-signal-detector-v1-design.md`
  - `logtap/docs/specs/alert-signal-detector-v1-plan.md`

## 3. Requested Review Focus

- Overdesign concerns to verify:
  - 是否在单迭代内引入了不必要的新抽象层或过多 API。
- One-iteration feasibility concerns:
  - `T-201`~`T-205` 是否可在单迭代闭环完成并验证。
- Missing design considerations to verify:
  - test API 与现有 async run 语义边界是否清晰，是否会误用。

## 4. Known Risks and Constraints

- Technical constraints:
  - query 层需要共享 gateway 初始化的 registry 实例。
  - 动态 plugin 可选；不可依赖外部插件存在。
- Team/time constraints:
  - 仅做一个迭代可交付能力，不扩 scope。
- External dependencies:
  - 无外部系统依赖。
- Release constraints:
  - 必须保持已有 monitor/worker/alert 路径可用。

## 5. Open Questions for Reviewer

- `Q-201`: 是否需要本迭代就给 schema 引入版本字段？
- `Q-202`: monitor test 是否需要写入 run 历史，还是保持纯即时结果？

## 6. Handoff Acceptance

Handoff is complete only when all are true:

- Design doc and plan links are present
- Scope and iteration target are explicit
- At least one concrete concern or question is provided
