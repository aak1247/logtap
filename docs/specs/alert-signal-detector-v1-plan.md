# Alert Signal Detector Plugin (Iteration 1) - Execution Plan

## 1. Plan Header

- Feature name: Alert Signal Detector Plugin (Iteration 1)
- Linked design doc: `logtap/docs/specs/alert-signal-detector-v1-design.md`
- Linked review report: `logtap/docs/specs/alert-signal-detector-v1-review.md`
- Plan owner: Codex
- Contributors: TBD
- Last updated: 2026-02-15
- Plan status: `DONE`
- Review gate status: `PASS`

## 2. Scope Lock

- Allowed change range (systems/modules/files):
  - `logtap/internal/detector/*`（新建）
  - `logtap/internal/alert/*`（仅信号入口与兼容桥接）
  - `logtap/internal/config/config.go`（仅 detector 配置项）
  - `logtap/cmd/gateway/main.go`（仅 detector 初始化）
  - 对应 `*_test.go`
- Explicitly excluded change range:
  - `logtap/internal/httpserver/*`
  - `logtap/web/*`, `logtap-cloud/*`
  - 告警通知发送实现（`alert/worker.go` 渠道逻辑）
  - 数据库 schema（`model`/`migrate`）
- Change control rule (when to reopen design):
  - 如需新增 DB 表、公开 API、或修改通知通道模型，必须回到设计评审。
- Review-gated activation rule: keep plan in `DRAFT` until review gate is `PASS`

## 3. Task Breakdown

### Task `T-001` - 建立 Detector 基础模型与注册中心

- Status: `DONE`
- Owner: Codex
- Priority: `P0`
- Linked requirements: `FR-001`, `FR-002`, `FR-003`, `NFR-002`
- Objective: 提供信号模型、插件接口与静态注册能力。
- Task details:
  - 新增 `AlertSignal`、`ExecuteRequest`、`DetectorPlugin`。
  - 新增 `Registry`，支持 register/get/list 与重复检测。
- Implementation requirements:
  - 注册冲突返回明确错误。
  - 对调用方提供只读列表，避免外部修改内部状态。
- Allowed modification range:
  - `logtap/internal/detector/*.go`
  - `logtap/internal/detector/*_test.go`
- Dependencies: None
- Risks/notes: 包命名需避免与 `alert` 循环依赖。
- Evidence links (PR, test report, dashboard): `go test ./internal/detector/...`
- Completion conditions:
  - [x] `internal/detector` 编译通过
  - [x] 注册中心单测通过（含重复注册用例）

### Task `T-002` - 增加 Go plugin 动态加载能力

- Status: `DONE`
- Owner: Codex
- Priority: `P1`
- Linked requirements: `FR-004`, `NFR-002`, `NFR-003`, `NFR-004`
- Objective: 支持从本地 `.so` 加载 detector 并注册。
- Task details:
  - 增加 `LoadPluginFile(path)` 与 `LoadPluginDir(dir)`。
  - 约定导出符号 `Plugin`，类型为 `DetectorPlugin`。
- Implementation requirements:
  - 加载失败时返回错误，不 panic。
  - 错误信息包含路径与失败原因。
- Allowed modification range:
  - `logtap/internal/detector/*.go`
  - `logtap/internal/detector/*_test.go`
- Dependencies: `T-001`
- Risks/notes: CI 环境对动态 plugin 测试有限，优先覆盖失败路径测试。
- Evidence links (PR, test report, dashboard): `go test ./internal/detector/...`
- Completion conditions:
  - [x] 动态加载代码编译通过
  - [x] 至少覆盖 2 个失败路径测试

### Task `T-003` - 告警引擎接入 `EvaluateSignal`

- Status: `DONE`
- Owner: Codex
- Priority: `P0`
- Linked requirements: `FR-005`, `FR-006`, `NFR-001`
- Objective: 支持直接用信号驱动规则评估，并保持旧入口兼容。
- Task details:
  - 新增 `Engine.EvaluateSignal(ctx, signal)`。
  - 新增从 `AlertSignal` 到内部 `Input` 的转换桥接。
  - 保持 `Evaluate(Input)` 结果一致。
- Implementation requirements:
  - 不修改现有通知发送逻辑。
  - 不引入数据库 schema 改动。
- Allowed modification range:
  - `logtap/internal/alert/*.go`
  - `logtap/internal/alert/*_test.go`
- Dependencies: `T-001`
- Risks/notes: 兼容路径需防止字段丢失导致规则行为变化。
- Evidence links (PR, test report, dashboard): `go test ./internal/alert`
- Completion conditions:
  - [x] 引擎测试新增 `EvaluateSignal` 用例
  - [x] 现有 `internal/alert` 测试全部通过

### Task `T-004` - 网关初始化与文档同步

- Status: `DONE`
- Owner: Codex
- Priority: `P1`
- Linked requirements: `FR-004`, `NFR-004`
- Objective: 网关启动时可初始化 detector（静态+可选动态），并记录加载结果。
- Task details:
  - 增加配置项 `DETECTOR_PLUGIN_DIRS`。
  - 在 `cmd/gateway` 初始化注册中心并尝试加载目录。
  - 输出加载统计日志。
- Implementation requirements:
  - 目录为空时不影响启动。
  - 失败按 warn 记录并继续。
- Allowed modification range:
  - `logtap/internal/config/config.go`
  - `logtap/cmd/gateway/main.go`
  - `logtap/docs/specs/alert-signal-detector-v1-*.md`
- Dependencies: `T-001`, `T-002`
- Risks/notes: 启动阶段错误处理必须保持可观测性。
- Evidence links (PR, test report, dashboard): `go test ./internal/config ./cmd/gateway`
- Completion conditions:
  - [x] 至少 1 个自动化验证（配置解析或初始化行为）
  - [x] 初始化日志路径已实现（`detector registry initialized ...`）
  - [x] 网关代码编译通过

## 4. TODO Board

### TODO

- [ ] `T-xxx` None

### IN_PROGRESS

- [ ] `T-xxx` None

### BLOCKED

- [ ] `T-xxx` None

### DONE

- [x] `T-000` 设计文档与计划初稿 - `logtap/docs/specs/alert-signal-detector-v1-design.md`
- [x] `T-001` 建立 Detector 基础模型与注册中心 - `go test ./internal/detector/...`
- [x] `T-002` 增加 Go plugin 动态加载能力 - `go test ./internal/detector/...`
- [x] `T-003` 告警引擎接入 `EvaluateSignal` - `go test ./internal/alert`
- [x] `T-004` 网关初始化与文档同步 - `go test ./internal/config ./cmd/gateway`

## 5. Milestones

- Milestone name: M1 - Detector Foundation + Engine Signal Entry
- Planned date: 2026-02-15
- Required tasks: `T-001`, `T-002`, `T-003`, `T-004`
- Exit criteria:
  - 所有 P0/P1 任务完成
  - 相关测试通过
  - Review gate 保持 `PASS`

## 6. Feature-Level Completion Conditions

Mark plan/feature as complete only when all conditions pass:

- All mandatory tasks are `DONE`
- Every done task has evidence and satisfied task completion conditions
- No unresolved `P0`/`P1` blockers remain
- Scope-lock is respected or formally re-approved
- Release/rollout checks are passed
- Latest review gate verdict remains `PASS` for delivered scope

## 7. Plan Update Rules

- Update status immediately when state changes
- Record why any task is added, split, or removed
- If a task exceeds allowed modification range, pause and revise design+plan
- Keep requirement links accurate after every update
