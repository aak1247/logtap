# Alert Signal Detector Plugin (Iteration 2) - Execution Plan

## 1. Plan Header

- Feature name: Alert Signal Detector Plugin (Iteration 2)
- Linked design doc: `logtap/docs/specs/alert-signal-detector-v2-design.md`
- Linked review report: `logtap/docs/specs/alert-signal-detector-v2-review.md`
- Plan owner: Codex
- Contributors: TBD
- Last updated: 2026-02-17
- Plan status: `DONE`
- Review gate status: `PASS`

## 2. Scope Lock

- Allowed change range (systems/modules/files):
  - `logtap/internal/detector/*`（服务封装、测试）
  - `logtap/internal/query/monitors.go`
  - `logtap/internal/query/detectors.go`（新建）
  - `logtap/internal/httpserver/httpserver.go`
  - `logtap/cmd/gateway/main.go`
  - 对应 `*_test.go`
- Explicitly excluded change range:
  - `logtap/internal/model/*`（无 schema 变更）
  - `logtap/internal/monitor/worker.go`（调度逻辑不改）
  - `logtap/internal/alert/worker.go`（通知渠道不改）
  - `logtap/web/*`, `logtap-cloud/*`
- Change control rule (when to reopen design):
  - 若出现 DB schema 变更、通知链路改造、或新增跨服务依赖，必须回到设计评审。
- Review-gated activation rule: keep plan in `DRAFT` until review gate is `PASS`

## 3. Task Breakdown

### Task `T-201` - Detector 服务层封装与契约稳定

- Status: `DONE`
- Owner: Codex
- Priority: `P0`
- Linked requirements: `FR-201`, `FR-202`, `FR-203`, `FR-204`
- Objective: 提供统一 detector catalog/schema/validate/test 服务。
- Task details:
  - 抽象 registry 访问服务，屏蔽 handler 对 registry 细节依赖。
  - 输出结构包含 descriptor + schema + validate 接口。
- Implementation requirements:
  - 空 registry 或 type 缺失要返回明确错误。
  - schema 返回必须可 JSON 序列化。
- Allowed modification range:
  - `logtap/internal/detector/*`
  - `logtap/internal/detector/*_test.go`
- Dependencies: None
- Risks/notes: 避免与 monitor worker 引入循环依赖。
- Evidence links (PR, test report, dashboard): `go test ./internal/detector/...`
- Completion conditions:
  - [x] detector service 单测通过
  - [x] catalog/schema/validate 核心路径覆盖

### Task `T-202` - 新增 detector catalog/schema API

- Status: `DONE`
- Owner: Codex
- Priority: `P0`
- Linked requirements: `FR-201`, `FR-202`, `NFR-203`
- Objective: 提供前端可用的 detector 元数据与 schema 接口。
- Task details:
  - 新增 `query/detectors.go`。
  - 接入路由：
    - `GET /api/plugins/detectors`
    - `GET /api/plugins/detectors/:type/schema`
- Implementation requirements:
  - 遵循现有响应 envelope（`code/data`）。
  - 鉴权策略与项目管理接口一致。
- Allowed modification range:
  - `logtap/internal/query/detectors.go`
  - `logtap/internal/httpserver/httpserver.go`
  - `logtap/internal/query/*_test.go`
- Dependencies: `T-201`
- Risks/notes: 类型不存在时返回 404 而非 500。
- Evidence links (PR, test report, dashboard): `go test ./internal/httpserver -run "TestDetectorCatalogAndSchemaAPI"`
- Completion conditions:
  - [x] 2 个新 API 可用
  - [x] 类型不存在返回可预期错误

### Task `T-203` - Monitor create/update 前置 detector 校验

- Status: `DONE`
- Owner: Codex
- Priority: `P0`
- Linked requirements: `FR-203`, `FR-204`, `NFR-202`
- Objective: 在 monitor 管理 API 层提前发现无效 detector 配置。
- Task details:
  - 修改 `CreateMonitorHandler` / `UpdateMonitorHandler`。
  - 在写库前调用 detector service validate。
- Implementation requirements:
  - 非法 type/config 返回 400。
  - 合法请求行为与当前保持兼容。
- Allowed modification range:
  - `logtap/internal/query/monitors.go`
  - `logtap/internal/query/*_test.go`
- Dependencies: `T-201`
- Risks/notes: 需要兼顾部分更新场景（仅改 name）。
- Evidence links (PR, test report, dashboard): `go test ./internal/httpserver -run "TestMonitorCreateUpdateAndTestValidation"`
- Completion conditions:
  - [x] 新增 create/update 校验失败测试
  - [x] 现有 monitor API 行为回归通过

### Task `T-204` - Monitor 同步试运行 API

- Status: `DONE`
- Owner: Codex
- Priority: `P1`
- Linked requirements: `FR-205`, `FR-206`, `NFR-201`, `NFR-204`
- Objective: 新增同步 test 接口并返回执行结果，不触发通知链路。
- Task details:
  - 新增（或替换）`POST /api/:projectId/monitors/:monitorId/test`。
  - 读取 monitor 配置并同步执行 detector。
  - 返回信号摘要、耗时、错误。
- Implementation requirements:
  - 遵循 timeout 限制。
  - 禁止调用 `alert.Engine.EvaluateSignal`。
- Allowed modification range:
  - `logtap/internal/query/monitors.go`
  - `logtap/internal/httpserver/httpserver.go`
  - `logtap/internal/query/*_test.go`
- Dependencies: `T-201`
- Risks/notes: 需避免与异步 `run` 语义混淆。
- Evidence links (PR, test report, dashboard): `go test ./internal/httpserver -run "TestMonitorCreateUpdateAndTestValidation"`
- Completion conditions:
  - [x] test API 返回成功与失败路径结果
  - [x] 验证 test 不写入 `alert_deliveries`

### Task `T-205` - 网关依赖注入与回归验证

- Status: `DONE`
- Owner: Codex
- Priority: `P1`
- Linked requirements: `NFR-202`, `NFR-204`
- Objective: 将 registry 服务注入 query 层并完成回归验证。
- Task details:
  - 调整 gateway/httpserver 初始化参数。
  - 确保 monitor worker 与 query 层共享同一 registry 实例。
- Implementation requirements:
  - registry 初始化失败不致命（保持服务启动可用）。
  - 日志记录应可定位问题。
- Allowed modification range:
  - `logtap/cmd/gateway/main.go`
  - `logtap/internal/httpserver/httpserver.go`
  - 相关测试文件
- Dependencies: `T-201`, `T-202`, `T-203`, `T-204`
- Risks/notes: 函数签名变更可能影响现有测试初始化。
- Evidence links (PR, test report, dashboard): `go test ./internal/query ./internal/integration -run TestDoesNotExist ./cmd/gateway`
- Completion conditions:
  - [x] 目标模块测试通过
  - [x] 关键启动路径编译通过

## 4. TODO Board

### TODO

- [ ] `T-xxx` None

### IN_PROGRESS

- [ ] `T-xxx` None

### BLOCKED

- [ ] `T-xxx` None

### DONE

- [x] `T-200` 设计与评审完成 - `logtap/docs/specs/alert-signal-detector-v2-design.md`
- [x] `T-201` Detector 服务层封装与契约稳定 - `go test ./internal/detector/...`
- [x] `T-202` 新增 detector catalog/schema API - `go test ./internal/httpserver -run "TestDetectorCatalogAndSchemaAPI"`
- [x] `T-203` Monitor create/update 前置 detector 校验 - `go test ./internal/httpserver -run "TestMonitorCreateUpdateAndTestValidation"`
- [x] `T-204` Monitor 同步试运行 API - `go test ./internal/httpserver -run "TestMonitorCreateUpdateAndTestValidation"`
- [x] `T-205` 网关依赖注入与回归验证 - `go test ./internal/query ./internal/integration -run TestDoesNotExist ./cmd/gateway`

## 5. Milestones

- Milestone name: M2 - Detector Config UX Backend
- Planned date: 2026-02-20
- Required tasks: `T-201`, `T-202`, `T-203`, `T-204`, `T-205`
- Exit criteria:
  - 所有 P0/P1 任务 `DONE`
  - API 与测试验收通过
  - Review gate 维持 `PASS`

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
