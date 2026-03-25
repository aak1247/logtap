# E2E CI 落地清单（v1）

## 1. 目标

- 将现有回归分层接入 CI：
  - API/Worker 集成：`go test ./internal/integration -count=1`
  - UI 真端到端：`cd web && npm run e2e -- --reporter=list`
- 形成可重复的首轮验收与后续稳定性排障流程。

## 2. 前置条件

- 代码已包含工作流：`.github/workflows/e2e-regression.yml`
- 分支已推送到远端（GitHub 仓库）。
- 仓库 Secrets（可选，网络受限时建议配置）：
  - `HTTP_PROXY`
  - `HTTPS_PROXY`
  - `ALL_PROXY`

## 3. 触发方式

1. 通过 PR 触发（推荐）：
   - 提交包含 E2E 相关改动的 PR。
   - 变更命中工作流 `paths` 规则后自动触发。
2. 手动触发（`workflow_dispatch`）：
   - GitHub Actions 页面选择 `e2e-regression`。
   - 点击 `Run workflow`。

## 4. 首轮验收标准

1. `integration-api-worker` 通过。
2. `ui-e2e-playwright` 通过。
3. 两个 job 连续成功至少 2 次（可通过 re-run 验证稳定性）。
4. Playwright 报告产物可下载（失败时也应存在）。

## 5. Flaky 与失败排障

1. 浏览器安装失败/超时：
   - 检查代理 secrets 是否生效。
   - 观察 `Install Playwright Chromium` 步骤日志。
2. UI 用例偶发失败：
   - 下载 `playwright-artifacts-*` 查看截图、video、trace。
   - 对单个失败用例本地复现：`cd web && npm run e2e -- e2e/<case>.spec.ts --reporter=list`
3. 后端集成失败：
   - 本地先复现：`go test ./internal/integration -count=1`
   - 对应修复后再次触发 workflow。

## 6. 关闭条件（T-E2E-006）

- 已有可追溯 CI job records（至少 2 次连续通过）。
- 计划文档 `T-E2E-006` 状态更新为 `DONE`。
