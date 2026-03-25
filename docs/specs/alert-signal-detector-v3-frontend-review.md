# Alert Signal Detector Plugin (Iteration 3 Frontend) - Review Report

## 1. Review Verdict

- Verdict: `PASS`
- Confidence: `HIGH`
- Summary: 方案聚焦前端接入与交互，不扩展通知通道和 route 模型，范围可在单迭代收敛。

## 2. Scope and Feasibility Check

- One-iteration feasible: `YES`
- Why:
  - 后端 API 已在 v2 就绪，本迭代只做前端消费。
  - 变更集中在 `web/src/lib/api.ts` 与 `web/src/ui/pages/*`，不涉及 DB 与后端核心链路。

## 3. Key Findings

### Finding `RV-301`

- Severity: `MINOR`
- Problem: detector catalog 返回字段可能存在大小写差异（`Type` vs `type`）。
- Required action: API 客户端增加归一化逻辑。

### Finding `RV-302`

- Severity: `MINOR`
- Problem: `run` 与 `test` 在操作入口容易混淆。
- Required action: 在按钮和结果区明确写出“run=异步调度，test=同步预检（不发通知）”。

## 4. Required Changes Before Implementation

- [x] `A-301` 在 API 客户端实现 detector descriptor 字段兼容解析。
- [x] `A-302` 在监控页显式区分 run/test 语义。
- [x] `A-303` 控制范围，不新增通知通道扩展功能。
