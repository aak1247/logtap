# 现有功能 E2E 回归（v1）执行计划

## 1. Plan Header

- Feature name: Existing Features E2E Regression (v1)
- Linked design doc: `logtap/docs/specs/e2e-test-cases-v1.md`
- Last updated: 2026-03-09
- Plan status: `ACTIVE`
- Review gate status: `PASS`（范围已锁定为“仅补测试，不加新功能”）

## 2. Scope Lock

- Allowed:
  - `logtap/web/e2e/*`
  - `logtap/web/playwright.config.ts`
  - `logtap/web/package.json`
  - `logtap/cmd/e2e-server/*`
  - `logtap/internal/integration/*_test.go`
  - `logtap/internal/httpserver/*_test.go`
  - `logtap/docs/specs/e2e-test-*.md`
- Excluded:
  - `logtap/internal/*` 业务实现逻辑（非测试辅助代码）
  - `logtap/web/src/*` 产品功能代码
  - `logtap-cloud/*`

## 3. 执行策略

- 策略 A（API/Worker 链路）：优先用 `internal/integration` 覆盖高价值稳定链路。
- 策略 B（UI 真端到端）：用 Playwright 驱动真实浏览器，覆盖用户主流程。
- 策略 C（分层稳定）：P0 先落地 smoke；P1 再补扩展回归与负向场景。

## 4. Task Breakdown

### Task `T-E2E-001` - E2E 运行基座稳定化

- Status: `DONE`
- Priority: `P0`
- Requirements: `E2E-AUTH-001`, `E2E-MON-002`
- Change range:
  - `logtap/cmd/e2e-server/*`
  - `logtap/web/playwright.config.ts`
  - `logtap/web/package.json`
- Deliverables:
  - 前后端自动拉起（Playwright `webServer`）
  - SQLite 轻量后端测试服务
  - 本地/CI 浏览器安装说明与脚本
- Evidence:
  - `go test ./cmd/e2e-server -count=1`
  - `cd logtap/web && npm run e2e -- --list`
  - `cd logtap/web && npm run e2e -- --reporter=list`（11 passed）

### Task `T-E2E-002` - UI 主链路 Smoke（初始化/登录/项目）

- Status: `DONE`
- Priority: `P0`
- Requirements: `E2E-AUTH-001`, `E2E-PROJ-001`, `E2E-NEG-001`
- Change range:
  - `logtap/web/e2e/*.spec.ts`
- Deliverables:
  - 独立的 bootstrap/login/project 相关 UI 用例
  - 受保护路由重定向负向用例
- Evidence:
  - `cd logtap/web && npm run e2e -- --reporter=list`（11 passed）

### Task `T-E2E-003` - 监控插件 UI 用例分层

- Status: `DONE`
- Priority: `P0`
- Requirements: `E2E-MON-001`, `E2E-MON-002`, `E2E-MON-003`
- Change range:
  - `logtap/web/e2e/*.spec.ts`
  - `logtap/cmd/e2e-server/*`（如需启用 worker 模式）
- Deliverables:
  - detector/schema 展示用例
  - monitor create/test 用例
  - run/runs 历史验证用例
- Evidence:
  - `cd logtap/web && npm run e2e -- --reporter=list`（11 passed）

### Task `T-E2E-004` - API/Worker 集成回归补齐

- Status: `DONE`
- Priority: `P0`
- Requirements: `E2E-LOG-001`, `E2E-ALERT-001`, `E2E-ALERT-002`, `E2E-MON-ALERT-001`
- Change range:
  - `logtap/internal/integration/*_test.go`
- Deliverables:
  - monitor -> alert -> webhook 端到端集成用例
  - run/runs API 集成用例
  - existing alerting regression 用例完善
- Evidence:
  - `go test ./internal/integration -count=1`

### Task `T-E2E-005` - Settings/Key 生命周期回归

- Status: `DONE`
- Priority: `P1`
- Requirements: `E2E-SET-001`
- Change range:
  - `logtap/web/e2e/*.spec.ts`
  - `logtap/internal/integration/*_test.go`（必要时）
- Deliverables:
  - key 创建/吊销后 ingest 鉴权效果验证（包含吊销后鉴权最终生效轮询）
- Evidence:
  - `cd logtap/web && npm run e2e -- --list`
  - `cd logtap/web && npm run e2e -- e2e/settings-keys.spec.ts --reporter=list`（PASS）
  - `cd logtap/web && npm run e2e -- --reporter=list`（11 passed）

### Task `T-E2E-006` - CI 集成与稳定性治理

- Status: `IN_PROGRESS`
- Priority: `P1`
- Requirements: 全部
- Change range:
  - `.github/workflows/*`（或现有 CI 配置）
  - `logtap/docs/specs/e2e-test-*.md`
- Deliverables:
  - CI 中按层运行（API 集成 + UI E2E）
  - flaky 用例隔离策略（重试、trace、截图）
  - 代理环境可配置（`HTTP_PROXY`/`HTTPS_PROXY`/`ALL_PROXY` secrets）
- Evidence:
  - `.github/workflows/e2e-regression.yml`
  - `docs/specs/e2e-ci-rollout-v1.md`
  - `go run github.com/rhysd/actionlint/cmd/actionlint@latest .github/workflows/e2e-regression.yml`
  - `go test ./internal/integration -count=1`（PASS）
  - `cd logtap/web && npm run e2e -- --reporter=list`（11 passed，本地）
  - CI job records（连续通过，待首轮执行）

## 5. TODO Board

### TODO

- 无

### IN_PROGRESS

- [ ] `T-E2E-006` CI 集成与稳定性治理

### BLOCKED

- [ ] `B-002` 当前环境无 `gh` 且改动未推送，无法直接产出远端 CI 首轮 job records

### DONE

- [x] `D-001` E2E 用例设计清单初稿完成（`e2e-test-cases-v1.md`）
- [x] `D-002` 新增 monitor 链路集成测试（`internal/integration/monitor_integration_test.go`）
- [x] `D-003` 拆分 UI smoke 用例并新增 monitor run/runs 场景（`web/e2e/auth-project.spec.ts`, `web/e2e/monitor-runs.spec.ts`）
- [x] `D-004` 新增 Settings/Key 生命周期 UI E2E 用例（`web/e2e/settings-keys.spec.ts`）
- [x] `D-005` Playwright 浏览器安装阻塞解除（配置代理后 `npx playwright install chromium` 成功）
- [x] `D-006` 新增 CI E2E 回归工作流（`.github/workflows/e2e-regression.yml`）
- [x] `D-007` 新增事件列表/详情 UI E2E 用例（`web/e2e/events-detail.spec.ts`）
- [x] `D-008` 新增 CI 落地清单与验收标准（`docs/specs/e2e-ci-rollout-v1.md`）
- [x] `D-009` 新增日志搜索 UI E2E 用例（`web/e2e/logs-search.spec.ts`）
- [x] `D-010` 新增报警规则 dry-run UI E2E 用例（`web/e2e/alerts-rules-test.spec.ts`）
- [x] `D-011` 新增报警配置页联系人/Webhook/规则创建用例（`web/e2e/alerts-crud.spec.ts`）
- [x] `D-012` 新增分析页 Top Events 用例（`web/e2e/analytics-top-events.spec.ts`）
- [x] `D-013` 新增分析页漏斗计算用例（`web/e2e/analytics-funnel.spec.ts`）

## 6. Milestones

1. M1（Smoke Ready）
   - `T-E2E-001` + `T-E2E-002` + `T-E2E-003` P0 子集完成
2. M2（API Regression Ready）
   - `T-E2E-004` 全部完成并稳定
3. M3（CI Ready）
   - `T-E2E-006` 完成，形成可持续回归流水线

## 7. Feature-Level Completion Conditions

- P0 用例全部可重复执行并通过（本地或 CI 至少一种稳定环境）。
- `internal/integration` 相关用例通过：`go test ./internal/integration -count=1`
- UI E2E 用例通过：`cd logtap/web && npm run e2e`
- 用例文档与执行计划状态一致（Checklist 与 TODO Board 同步）。

## 8. Plan Update Rules

- 状态仅允许：`TODO` / `IN_PROGRESS` / `BLOCKED` / `DONE`
- 每次状态变更需更新 evidence（命令与结果）。
- 出现 flaky 时先标 `BLOCKED` 并记录复现条件，不直接降级用例目标。
- 不得将新功能开发混入本计划任务。
