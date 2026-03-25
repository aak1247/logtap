# Alert Rule Test Delivery (v1) - Design Document

## 1. Document Control

- Feature name: Alert Rule Test Delivery (v1)
- Version: v1.0
- Author: Codex
- Reviewers: TODO
- Last updated: 2026-03-11
- Status: `DRAFT`
- Review gate status: `NOT_REVIEWED`
- Linked review report: -

## 2. Context and Problem

- Business background:
  - 当前已有告警规则系统和通知投递链路：日志/监控信号 -> AlertRule -> AlertDelivery -> AlertWorker -> 邮件/短信/企微/Webhook。
  - 控制台支持“规则测试”（`POST /alerts/rules/test`），可以输入一条虚拟事件查看哪些规则匹配、是否 "will enqueue"、以及预览通知内容。
- Current pain points:
  - 规则测试目前是**纯预览**：不会写入 `alert_deliveries`，也不会触发 AlertWorker 发送通知。
  - 用户无法验证完整链路是否通畅（如 SMTP 配置是否生效、联系人/联系组是否正确），只能等真实告警触发，调试体验差。
- Why now:
  - 监控插件和 HTTP/TCP 等检测能力已经补齐，告警链路可用性变得更重要；需要一个安全可控的“手动触发一次真实投递”的入口来验证通知配置。
- Related links:
  - `logtap/docs/monitoring-plugin-design.md`
  - `logtap/docs/specs/alert-signal-detector-v2-design.md`

## 3. Goals and Non-Goals

- Goals:
  - 在现有“规则测试”结果基础上，提供一个**显式的“执行测试投递”操作**，可以基于当前测试输入创建真实的 `alert_deliveries` 记录。
  - 保持默认规则测试仍是“无副作用预览”，只有用户进一步点击“执行投递”才会写库/触发通知。
  - 不污染告警去重/退避状态（不更新 `alert_states`）。
- Non-goals:
  - 不重构 Alert 引擎的主路径（`Evaluate`/`EvaluateSignal`）。
  - 不在本迭代引入新的通知渠道类型或变更 `AlertDelivery` schema（例如增加 `is_test` 字段）。
  - 不从“执行测试投递”入口同步直接发送 HTTP/SMTP，只负责创建 `alert_deliveries`；发送仍由现有 AlertWorker 负责。

## 4. Scope

- In scope:
  - 新增一个规则测试投递接口：`POST /api/:projectId/alerts/rules/test-deliveries`。
  - 后端利用现有 `EvaluatePreview` 结果，基于 `RulePreview` + `DeliveryPreview` 创建 `alert_deliveries` 记录。
  - 对投递记录做明显的“测试”标记（例如标题前增加 `[TEST]` 前缀），便于在 UI 中辨识。
- Out of scope:
  - 不修改规则存储结构、匹配语义或去重算法。
  - 不修改 AlertWorker 行为（重试、退避、最大并发等保持不变）。
  - 不新增前端页面结构，只在现有 “Rules Test” 面板上追加按钮/操作（前端变更另行设计）。
- Constraints:
  - 调用新接口时，假定用户已经理解将会在当前 project 下创建真实 `alert_deliveries` 记录。
  - API 设计保持向后兼容：未调用新接口时，系统行为与当前一致。

## 5. Users and Use Cases

- Target users:
  - 平台维护者 / SRE：验证邮件/短信/企微/Webhook 配置是否正确；
  - 应用开发：给自己或小组发一封“测试告警”，检查模板和内容是否符合预期。
- Key use cases:
  - UC-AR-T1：在规则测试中输入一条模拟事件（如 http_check 失败），看到某条规则 `willEnqueue=true`，点击“执行测试投递”，随后在“投递记录”列表中看到一条 `[TEST]` 投递，并收到对应邮件。
  - UC-AR-T2：对某规则测试发现 `willEnqueue=false`（由于 threshold/backoff 影响），点击“执行测试投递”返回 `created=0`，明确告知“当前输入不会触发任何投递”。
- Edge cases:
  - 多条规则匹配同一测试输入时，需要清晰地创建多条投递记录（每条 Rule 对应自己的 Channel/Target）。
  - 某些渠道目标无效（例如 Webhook URL 已删除），应沿用现有 Worker 的错误处理，不需要在 test-deliveries 接口提前阻断。

## 6. Requirements

### 6.1 Functional Requirements

- `FR-TD-001`: 规则测试投递接口
  - 系统必须提供 `POST /api/:projectId/alerts/rules/test-deliveries` 接口，接收与现有 `/rules/test` 相同的请求体结构（`source/level/message/fields`）。

- `FR-TD-002`: 复用规则测试语义
  - `test-deliveries` 必须使用与 `/rules/test` 相同的匹配逻辑，包括：
    - 源类型（logs/events/both）；
    - RuleMatch 匹配；
    - 窗口、阈值和 backoff 逻辑；
  - 接口应基于 `EvaluatePreview` 结果决定哪些规则“应当 enqueue”。

- `FR-TD-003`: 创建真实 AlertDelivery
  - 对于 `EvaluatePreview` 返回的每条 `RulePreview`，当 `WillEnqueue == true` 时，必须为其 `Deliveries` 创建对应的 `alert_deliveries` 记录：
    - `project_id` = 请求中的 `projectId`；
    - `rule_id` = 对应 `RulePreview.RuleID`；
    - `channel_type` / `target` / `title` / `content` 来自 `DeliveryPreview`；
    - `status` = `pending`，`attempts` = 0，`next_attempt_at` = 当前时间；
    - `title` 需增加明显的 `[TEST]` 前缀。

- `FR-TD-004`: 不更新 AlertState
  - `test-deliveries` 不得更新或插入 `alert_states` 记录，避免影响真实告警的去重和退避行为。

- `FR-TD-005`: 可观测的返回结果
  - 接口返回体必须包含以下字段：
    - `created`（int）：本次创建的 `alert_deliveries` 条数；
    - `items`（数组）：每条测试投递的概要信息（`id/ruleId/channelType/target/title`）。

### 6.2 Non-Functional Requirements

- `NFR-TD-001` Safety:
  - 默认规则测试仍然是只读操作；只有明确调用 `test-deliveries` 时才会产生真实投递记录。
  - 建议在文档和 UI 上明确标注“这是一个会发送真实通知的操作”。

- `NFR-TD-002` Performance:
  - 单次调用的规则数量通常较少（数十条级别），在一个事务中创建有限数量的 `alert_deliveries` 不应对数据库造成显著压力。

- `NFR-TD-003` Observability:
  - 通过现有投递记录列表（含 `[TEST]` 标记）即可观察测试投递；如有需要，后续可增加按标题前缀过滤的能力（本迭代不强制）。

## 7. Proposed Solution

### 7.1 新接口与路由

- 在 `internal/httpserver/httpserver.go` 中，为 alerts 路由组增加：

  ```go
  alerts.POST("/rules/test-deliveries", query.TestAlertRulesDeliveriesHandler(db))
  ```

- 请求体沿用 `TestAlertRulesHandler` 中的匿名结构：

  ```json
  {
    "source": "logs | events | both",
    "level": "info | error | ...",
    "message": "...",
    "fields": {"k": "v", ...}
  }
  ```

### 7.2 Handler 实现（query 层）

新增 `TestAlertRulesDeliveriesHandler` 于 `internal/query/alerts.go`，与 `TestAlertRulesHandler` 靠近：

- 步骤：
  1. 校验 DB、解析 `projectId`；
  2. 解析请求体 `source/level/message/fields`；
  3. 构造 `alert.Input`（与 `TestAlertRulesHandler` 相同）；
  4. 使用 `alert.NewEngine(db).EvaluatePreview(ctx, in)` 获取 `[]RulePreview`；
  5. 遍历 `RulePreview`：
     - 忽略 `Matched=false` 或 `WillEnqueue=false` 的规则；
     - 对于每个 `DeliveryPreview`，构造一条待插入的 `model.AlertDelivery`：
       - `Title` = `"[TEST] " + preview.Title`；
       - 其他字段按 `FR-TD-003` 填充；
  6. 若待插入列表为空，直接返回 `200 OK`：`{"created":0,"items":[]}`；
  7. 否则在限制时间的 context 内执行 `db.Create(&deliveries)`；
  8. 返回 `201` 或 `200`，body 为：

     ```json
     {
       "created": 3,
       "items": [
         {
           "id": 101,
           "ruleId": 5,
           "channelType": "email",
           "target": "dev@example.com",
           "title": "[TEST] [logtap] Rule http-check-failed triggered"
         },
         ...
       ]
     }
     ```

- 注意：
  - 不对 `AlertState` 做任何更新，完全绕过 dedupe/backoff 状态更新。
  - 使用 `time.Now().UTC()` 作为测试投递的 `NextAttemptAt`，让 AlertWorker 尽快处理。

### 7.3 与现有逻辑的关系

- 保持 `/rules/test` 现有行为不变，仅在 query 层新增一个 handler：
  - `/rules/test`：只跑 `EvaluatePreview`，返回 `RulePreview` 列表；
  - `/rules/test-deliveries`：先跑 `EvaluatePreview`，再基于结果落库 `AlertDelivery`。
- 不修改 `alert.Engine` 和 `buildDeliveries`，仅复用其 preview 结果；
- 不新增/修改任何 DB schema（使用现有 `alert_deliveries` 表）。

## 8. Alternatives and Tradeoffs

- Option A: 直接在 `/rules/test` 中添加“写库+发送”逻辑。
  - 优点：无需新增接口；
  - 缺点：破坏现有“测试无副作用”的既有认知，增加误发风险，不利于安全使用。

- Option B（本方案）: 保持 `/rules/test` 纯预览，新增 `/rules/test-deliveries` 作为显式的“发送测试通知”入口。
  - 优点：语义清晰，用户显式选择是否发；
  - 缺点：多一个 API，但前端对接成本很低。

- Option C: 在 Alert 引擎里新增 `EvaluateTestAndEnqueue`，复用主路径更多逻辑。
  - 优点：更贴近真实 Evaluate 行为；
  - 缺点：需要更深改动引擎/状态更新逻辑，本迭代不必要。

## 9. Risk Assessment

- Risk: 用户误用 test-deliveries 在生产环境频繁发测试邮件。
  - Mitigation: 在前端和文档中明显标注“会发送真实通知”；标题统一加 `[TEST]`；必要时可在后续增加开关（如 `ENABLE_ALERT_TEST_DELIVERIES`）。

- Risk: 多规则匹配时创建大量投递记录。
  - Mitigation: 通常规则数量有限；如需限制，可在前端只针对当前正在编辑的规则提供“发送测试通知”。本迭代先不做硬限制。

- Risk: AlertWorker 未跑或 SMTP/SMS 等配置错误时，测试投递记录堆积。
  - Mitigation: 这本身是测试要暴露的问题；可在 UI 中提示用户检查 Worker 状态和渠道配置。

## 10. Validation Strategy

- Unit testing:
  - 为 `TestAlertRulesDeliveriesHandler` 添加 handler 测试，覆盖：
    - 无匹配规则；
    - 匹配但 `WillEnqueue=false`；
    - 匹配且 `WillEnqueue=true`，成功创建若干 `alert_deliveries`。

- Manual verification:
  - 在 dev 环境中：
    - 创建一个简单 Rule（例如 `source_type=http_check && level=error`）；
    - 用 `/rules/test` 验证 `WillEnqueue=true`；
    - 调用 `/rules/test-deliveries`，确认有 `[TEST]` 投递记录写入、AlertWorker 发送到邮箱/Webhook。

- Acceptance criteria:
  - `AC-TD-001`: `/rules/test` 行为保持只读，现有测试用例全部通过；
  - `AC-TD-002`: `/rules/test-deliveries` 在匹配成功时创建正确数量的 `alert_deliveries`，并可触发 AlertWorker 实际发送；
  - `AC-TD-003`: 测试投递能在投递列表中清晰识别（标题带 `[TEST]`）。

## 11. Rollout Plan

- Phase 1: 合入后端实现 + 单元测试；
- Phase 2: 控制台整合按钮（在规则测试结果区域增加“发送测试通知”）；
- Phase 3: 文档更新和对外说明（标明测试通知的行为和使用注意事项）。

## 12. Open Questions

- `Q-TD-001`: 是否需要为测试投递增加 DB 级别标记（例如 `is_test` 列），以便后续在 UI 中单独过滤？当前决策：暂不改 schema，仅通过标题前缀区分。
- `Q-TD-002`: 是否需要对 `/rules/test-deliveries` 增加权限限制（例如仅管理员可用）？当前决策：沿用现有项目鉴权即可，后续按需要细化权限模型。

