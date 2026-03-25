# Logtap E2E 用例清单与描述（v1）

## 1. 目标与范围

- 目标：为“已上线功能”建立稳定的端到端（E2E）回归用例，覆盖 UI 交互与后端关键链路。
- In Scope：
  - 认证与初始化（bootstrap/login）
  - 项目管理（create/switch）
  - 日志/事件核心链路
  - 告警（rule test + delivery）
  - 监控插件（detector/monitor）
- Out of Scope：
  - 新功能设计与实现
  - 性能压测（k6）与大规模并发场景
  - 第三方真实 SMS/Email 平台联调

## 2. 用例清单（Checklist）

| ID | 用例名称 | 层级 | 优先级 | 状态 |
|---|---|---|---|---|
| `E2E-AUTH-001` | 首次初始化 + 登录闭环 | UI + API | P0 | DONE |
| `E2E-PROJ-001` | 项目创建 + 切换 + 进入系统 | UI + API | P0 | DONE |
| `E2E-LOG-001` | 日志写入（ingest）到查询（search）闭环 | API | P0 | DONE |
| `E2E-EVENT-001` | 事件写入到事件列表/详情闭环 | API + UI | P1 | DONE |
| `E2E-ALERT-001` | 规则测试（dry-run）仅预览不入投递队列 | API | P0 | DONE |
| `E2E-ALERT-002` | Webhook 告警投递完整链路（pending→sent） | API + Worker | P0 | DONE |
| `E2E-ALERT-003` | 报警页联系人/Webhook/规则创建闭环（含 API 清理） | UI + API | P1 | DONE |
| `E2E-ANA-001` | 分析页 Top Events 展示 track 上报事件 | UI + API | P1 | DONE |
| `E2E-ANA-002` | 分析页漏斗计算展示步骤与转化结果 | UI + API | P1 | DONE |
| `E2E-MON-001` | 监控插件页：detector 列表与 schema 展示 | UI + API | P1 | DONE |
| `E2E-MON-002` | 监控项创建 + Test（同步预检） | UI + API | P0 | DONE |
| `E2E-MON-003` | 监控项 Run（异步调度）+ Runs 历史可见 | UI + API + Worker | P0 | DONE |
| `E2E-MON-ALERT-001` | Monitor Signal 触发 Alert Delivery 投递 | API + Worker | P0 | DONE |
| `E2E-SET-001` | 项目 Key 创建/吊销与上报鉴权生效 | UI + API | P1 | DONE |
| `E2E-NEG-001` | 未登录访问受保护页面重定向登录 | UI | P1 | DONE |

## 3. 用例描述（Case Description）

### `E2E-AUTH-001` 首次初始化 + 登录闭环

- 目标：验证未初始化系统可以 bootstrap，随后登录成功进入项目页。
- 前置条件：空数据库；浏览器本地 `localStorage` 清空。
- 步骤：
  1. 打开 `/bootstrap`，输入 email/password/projectName 并提交。
  2. 跳转 `/login` 后输入账号密码登录。
  3. 跳转 `/projects`。
- 断言：
  - bootstrap 返回成功（UI 无错误）。
  - 登录成功，页面 URL 为 `/projects`。
  - 页面出现项目列表卡片。

### `E2E-PROJ-001` 项目创建 + 切换 + 进入系统

- 目标：验证多项目管理的核心入口行为。
- 前置条件：用户已登录。
- 步骤：
  1. 在项目页创建新项目。
  2. 点击“进入”。
  3. 打开“设置”确认当前项目 ID 已变更。
- 断言：
  - 项目列表出现新项目。
  - 进入后跳转 `/`（Dashboard）。
  - 顶部显示正确项目标识。

### `E2E-LOG-001` 日志写入（ingest）到查询（search）闭环

- 目标：验证日志从上报到查询可见。
- 前置条件：已存在项目与可用 project key。
- 步骤：
  1. 通过 `/api/:projectId/logs/` 上报唯一 message 日志。
  2. 调用 `/api/:projectId/logs/search` 按 message 检索。
- 断言：
  - ingest 返回 `202`。
  - search 返回包含该 message 的记录。

### `E2E-EVENT-001` 事件写入到事件列表/详情闭环

- 目标：验证 event 数据链路。
- 前置条件：项目可用，持有 project key，已登录控制台。
- 步骤：
  1. 调用 `/api/:projectId/track/` 上报唯一事件名。
  2. 进入 UI 事件页查看列表。
  3. 打开事件详情页。
- 断言：
  - 事件列表出现该事件。
  - 事件详情页字段完整可读。

### `E2E-ALERT-001` 规则测试（dry-run）仅预览不入投递队列

- 目标：验证 `rules/test` 行为与真实投递链路隔离。
- 前置条件：存在 webhook endpoint 与 alert rule。
- 步骤：
  1. 调用 `/alerts/rules/test`，输入可匹配 payload。
  2. 查询 `alert_deliveries` 记录数。
- 断言：
  - 响应中 `matched=true` 且 `willEnqueue=true`。
  - 实际投递表无新增记录。

### `E2E-ALERT-002` Webhook 告警投递完整链路（pending→sent）

- 目标：验证规则匹配后可被 worker 消费并成功投递 webhook。
- 前置条件：已创建 webhook endpoint + alert rule。
- 步骤：
  1. 写入一条匹配规则的日志。
  2. 确认产生 `pending` delivery。
  3. 执行 alert worker。
  4. 监听测试 webhook 收包。
- 断言：
  - delivery 状态从 `pending` 变为 `sent`。
  - webhook payload 含 `projectId/ruleId/deliveryId`。

### `E2E-ALERT-003` 报警页联系人/Webhook/规则创建闭环（含 API 清理）

- 目标：验证报警管理页面主配置流程在 UI 可用。
- 前置条件：登录控制台并进入项目。
- 步骤：
  1. 在“联系人”创建 email 联系人。
  2. 在“通知渠道”创建 webhook endpoint。
  3. 在“规则”创建匹配规则并绑定 endpoint。
  4. 通过 API 删除新建规则（避免测试数据污染）。
- 断言：
  - 联系人创建成功并出现在列表。
  - endpoint 创建成功并出现在列表。
  - 规则创建成功并出现在规则列表。
  - 规则最终可被删除（API 返回成功，列表不再包含）。

### `E2E-ANA-001` 分析页 Top Events 展示 track 上报事件

- 目标：验证 track 上报后，Top Events 数据可在分析页显示。
- 前置条件：登录控制台，项目可创建 key。
- 步骤：
  1. 创建新 project key。
  2. 通过 `/api/:projectId/track/` 上报唯一事件名。
  3. 轮询 `/api/:projectId/analytics/events/top` 直到命中该事件。
  4. 打开 `/analytics` 页面验证 Top Events 表格渲染。
- 断言：
  - top events 接口返回包含该事件名。
  - 分析页“事件分析”区域展示该事件行。

### `E2E-ANA-002` 分析页漏斗计算展示步骤与转化结果

- 目标：验证漏斗接口与页面“计算”按钮闭环可用。
- 前置条件：登录控制台，项目可创建 key。
- 步骤：
  1. 创建新 project key。
  2. 通过 `/api/:projectId/track/` 上报 `signup -> checkout -> paid` 的事件序列（含多用户）。
  3. 轮询 `/api/:projectId/analytics/funnel`，直到返回预期计数。
  4. 打开 `/analytics`，点击“计算”触发漏斗计算渲染。
- 断言：
  - funnel 接口返回步骤计数（示例：signup=2, checkout=2, paid=1）。
  - 页面漏斗表格出现 `signup/checkout/paid` 三个步骤行。

### `E2E-MON-001` 监控插件页：detector 列表与 schema 展示

- 目标：验证前端可读取 detector catalog/schema。
- 前置条件：至少注册 `log_basic` detector。
- 步骤：
  1. 登录后进入 `/alerts`。
  2. 切换到“监控插件”tab。
  3. 观察 detector 列表与 schema 展示区。
- 断言：
  - 列表包含 `log_basic`。
  - schema 区域渲染 JSON 内容。

### `E2E-MON-002` 监控项创建 + Test（同步预检）

- 目标：验证 monitor 创建与 test API 在 UI 上可用。
- 前置条件：登录并进入监控插件页。
- 步骤：
  1. 创建 monitor（detector=`log_basic`）。
  2. 在列表中点击 `Test`。
- 断言：
  - 页面提示“监控已创建”。
  - 试运行结果区域出现 `signalCount` 与 sample。
  - 不产生 alert delivery。

### `E2E-MON-003` 监控项 Run（异步调度）+ Runs 历史可见

- 目标：验证 run 与 runs 查询闭环。
- 前置条件：monitor worker 可运行。
- 步骤：
  1. 创建 monitor。
  2. 点击 `Run`。
  3. 触发 monitor worker 执行。
  4. 点击 `Runs`。
- 断言：
  - 列表出现最新 run 记录。
  - run 状态为 `success`，`signal_count >= 1`。

### `E2E-MON-ALERT-001` Monitor Signal 触发 Alert Delivery 投递

- 目标：验证 monitor 与 alert 两个 worker 的串联。
- 前置条件：存在匹配 monitor 输出的 alert rule（如 message keyword 命中）。
- 步骤：
  1. 创建 monitor 并触发 `Run`。
  2. 执行 monitor worker，确认生成 alert delivery（pending）。
  3. 执行 alert worker。
- 断言：
  - delivery 最终为 `sent`。
  - webhook 收到与 monitor run 对应的告警消息。

### `E2E-SET-001` 项目 Key 创建/吊销与上报鉴权生效

- 目标：验证 key 生命周期对 ingest 生效。
- 前置条件：已登录并存在项目。
- 步骤：
  1. 在设置页创建新 key。
  2. 使用新 key 上报日志，验证成功。
  3. 吊销该 key。
  4. 再次使用该 key 上报。
- 断言：
  - 吊销前上报成功。
  - 吊销后上报失败（401/403）。

### `E2E-NEG-001` 未登录访问受保护页面重定向登录

- 目标：验证路由鉴权守卫。
- 前置条件：清空 token。
- 步骤：
  1. 直接访问 `/alerts` 或 `/settings`。
- 断言：
  - 自动重定向至 `/login`。
  - 页面不展示受保护内容。

## 4. 建议分批执行

- 第一批（Smoke，P0）：`E2E-AUTH-001`、`E2E-PROJ-001`、`E2E-LOG-001`、`E2E-ALERT-001`、`E2E-ALERT-002`、`E2E-MON-002`、`E2E-MON-003`、`E2E-MON-ALERT-001`
- 第二批（Regression，P1）：`E2E-EVENT-001`、`E2E-ALERT-003`、`E2E-ANA-001`、`E2E-ANA-002`、`E2E-MON-001`、`E2E-SET-001`、`E2E-NEG-001`
