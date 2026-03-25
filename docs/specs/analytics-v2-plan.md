# Analytics v2 - Execution Plan

## 1. Plan Header

- Feature name: Analytics v2 – 基础分析 / 事件分析 / 属性分析
- Linked design doc: `logtap/docs/specs/analytics-v2-design.md`
- Linked review report: (TBD)
- Plan owner: Codex
- Contributors: TBD
- Last updated: 2026-03-10
- Plan status: `DRAFT`
- Review gate status: `PASS`

## 2. Scope Lock

- Allowed change range (systems/modules/files):
  - Backend: `logtap/internal/model`, `logtap/internal/migrate`, `logtap/internal/query`, `logtap/internal/httpserver`
  - Frontend: `logtap/web/src/ui/pages/AnalyticsPage.tsx`, `logtap/web/src/ui/pages/analytics/*`, `logtap/web/src/lib/api.ts`, Settings 相关页面
- Explicitly excluded change range:
  - ingest pipeline、Redis metrics、alerts、projects（除必要导航入口调整）
- Change control rule (when to reopen design):
  - 若需支持多属性 group_by、更复杂 metrics 或多图看板，需更新设计文档并重新评审。

## 3. Task Breakdown

### Task `T-001` - 数据模型与迁移

- Status: `DONE`
- Owner: Codex
- Priority: `P0`
- Linked requirements: `FR-003`, `FR-004`, `FR-010`
- Objective:
  - 在后端新增事件定义、属性定义与分析视图模型，并接入 AutoMigrate。
- Task details:
  - 在 `internal/model/models.go` 中新增 `EventDefinition`, `PropertyDefinition`, `AnalysisView`。
  - 在 `internal/migrate/migrate.go` 的 AutoMigrate 中加入新模型。
  - 在 Postgres init SQL 中补充参考表定义（保持仅供参考）。
- Allowed modification range:
  - `logtap/internal/model/models.go`
  - `logtap/internal/migrate/migrate.go`
  - `logtap/deploy/postgres/init.sql`
- Dependencies:
  - 无（首个任务）。
- Risks/notes:
  - 需确保新表不会与现有表名/索引冲突。
- Completion conditions:
  - [ ] `cd logtap && go test ./internal/...` 至少编译通过。
  - [ ] 本地启动服务后，数据库中可见新表结构。

### Task `T-002` - 事件定义 API（后端）

- Status: `DONE`
- Owner: Codex
- Priority: `P0`
- Linked requirements: `FR-003`, `FR-005`
- Objective:
  - 为每个 project 提供事件定义 CRUD 接口。
- Task details:
  - 新增 `internal/query/event_schema.go`，实现：
    - `GET /api/:projectId/events/schema`
    - `POST /api/:projectId/events/schema`
    - `PUT /api/:projectId/events/schema/:name`
  - 在 httpserver 路由中注册上述 handler。
- Allowed modification range:
  - `logtap/internal/query/*.go`
  - `logtap/internal/httpserver/*.go`
- Dependencies:
  - `T-001` 完成模型与迁移。
- Risks/notes:
  - 需要与现有 auth 方案正确集成，以 projectId 为作用域。
- Completion conditions:
  - [ ] 本地通过 curl 验证列表/创建/更新事件定义。
  - [ ] 非法输入（缺 name、重复 name）返回清晰 4xx 错误。

### Task `T-003` - 属性定义 API（后端）

- Status: `DONE`
- Owner: Codex
- Priority: `P0`
- Linked requirements: `FR-004`, `FR-006`
- Objective:
  - 为每个 project 提供属性定义 CRUD 接口，支持类型与枚举。
- Task details:
  - 新增 `internal/query/property_schema.go`，实现：
    - `GET /api/:projectId/properties/schema`
    - `POST /api/:projectId/properties/schema`
    - `PUT /api/:projectId/properties/schema/:key`
  - 校验 `type`：
    - 若 `type=enum`，`enum_values` 必须为非空数组。
- Allowed modification range:
  - 同 `T-002`。
- Dependencies:
  - `T-001` 完成模型与迁移。
- Risks/notes:
  - 注意避免与 `logs.fields` 中真实字段命名冲突（本质是逻辑冲突，不影响存储）。
- Completion conditions:
  - [ ] 本地通过 curl 验证列表/创建/更新属性定义。
  - [ ] 非法 `type` 或枚举配置返回 4xx。

### Task `T-004` - 自定义分析 API MVP（后端）

- Status: `DONE`
- Owner: Codex
- Priority: `P0`
- Linked requirements: `FR-007`, `FR-008`, `FR-009`, `NFR-001`
- Objective:
  - 实现统一的 `POST /api/:projectId/analytics/custom` 分析接口。
- Task details:
  - 新增 `internal/query/analytics_custom.go`：
    - 解析请求体（analysis_type, time_range, target, metric, group_by, filter）。
    - 限制：group_by 最多两个维度，metric 仅支持 count_events/count_users。
  - 生成 SQL：基于 `track_events`/`logs` 做聚合，并返回 `series` 结构。
  - 为复杂查询设置超时，避免占用过多资源。
- Allowed modification range:
  - `logtap/internal/query/*.go`
- Dependencies:
  - 依赖现有 events/logs/track_events 表结构。
- Risks/notes:
  - 需注意 Postgres 与 SQLite 的 SQL 差异。
- Completion conditions:
  - [ ] 单元或集成测试覆盖至少 3 个典型场景（单事件趋势、多事件对比、属性分布）。
  - [ ] 正常数据量下单次请求耗时 < 2s。

### Task `T-005` - AnalyticsPage 3 子菜单拆分（前端）

- Status: `DONE`
- Owner: Codex
- Priority: `P0`
- Linked requirements: `FR-001`, `FR-002`
- Objective:
  - 将现有 `/analytics` 页拆分为基础分析/事件分析/属性分析三个子菜单。
- Task details:
  - 在 `AnalyticsPage` 中添加 tab state 与左侧菜单 UI。
  - 抽取现有 JSX 到 `BasicAnalyticsPanel`，保持功能逻辑不变。
- Allowed modification range:
  - `logtap/web/src/ui/pages/AnalyticsPage.tsx`
- Dependencies:
  - 无（只依赖现有 API）。
- Risks/notes:
  - 需谨慎处理现有 useEffect / state，以免引入回归。
- Completion conditions:
  - [ ] 3 个 tab 切换正常，基础分析渲染与当前版本对齐。

### Task `T-006` - 事件分析 Panel（前端）

- Status: `DONE`
- Owner: Codex
- Priority: `P0`
- Linked requirements: `FR-008`
- Objective:
  - 在 Analytics 页内实现“事件分析”子面板。
- Task details:
  - 新增 `EventAnalyticsPanel`，包含：
    - 配置区：时间/粒度、事件多选、指标、属性维度、过滤器。
    - 结果区：折线/柱状/堆积图 + 数据表格。
  - 在 `lib/api.ts` 中新增 `postCustomAnalytics` 封装调用。
- Allowed modification range:
  - `logtap/web/src/ui/pages/analytics/*`
  - `logtap/web/src/lib/api.ts`
- Dependencies:
  - `T-004` 自定义分析 API 可用。
- Risks/notes:
  - 初期可使用简单图表组件，后续再优化样式与交互。
- Completion conditions:
  - [ ] 可在 UI 中完成单事件趋势、多事件对比、按属性拆分的分析。

### Task `T-007` - 属性分析 Panel（前端）

- Status: `DONE`
- Owner: Codex
- Priority: `P1`
- Linked requirements: `FR-009`
- Objective:
  - 在 Analytics 页内实现“属性分析”子面板。
- Task details:
  - 新增 `PropertyAnalyticsPanel`，包含：
    - 配置区：属性选择、时间开关、指标、事件过滤、属性过滤。
    - 结果区：饼图/柱状/堆积图 + 数据表格。
- Allowed modification range:
  - 同 `T-006`。
- Dependencies:
  - `T-004` 完成；`T-003` 提供属性定义列表用于 UI。
- Risks/notes:
  - 数值属性可先只支持简单平均值/总和趋势，复杂直方图后续迭代。
- Completion conditions:
  - [ ] 可按属性值查看整体分布与按时间堆积趋势。

### Task `T-008` - 报表视图保存与列表（前后端）

- Status: `DONE`
- Owner: Codex
- Priority: `P1`
- Linked requirements: `FR-010`
- Objective:
  - 支持将分析配置保存为报表，并从列表中打开。
- Task details:
  - 后端：实现 `GET/POST/GET/:id/DELETE /api/:projectId/analytics/views`。
  - 前端：在事件/属性分析 Panel 中加入“保存报表”“打开报表”入口与弹窗列表。
- Allowed modification range:
  - `logtap/internal/model/models.go`
  - `logtap/internal/query/analysis_views.go`
  - `logtap/web/src/ui/pages/analytics/*`
- Dependencies:
  - `T-001`、`T-004` 完成。
- Risks/notes:
  - 本迭代不做复杂权限控制，所有 user 共享视图列表。
- Completion conditions:
  - [ ] 可在 UI 中保存当前配置为报表，并从列表打开恢复配置并执行分析。

## 4. TODO Board

### TODO

- [ ] （空）

### IN_PROGRESS

- [ ] （空）

### BLOCKED

- [ ] （空）

### DONE

- [x] `T-001` 数据模型与迁移
- [x] `T-002` 事件定义 API（后端）
- [x] `T-003` 属性定义 API（后端）
- [x] `T-004` 自定义分析 API MVP（后端）
- [x] `T-005` AnalyticsPage 3 子菜单拆分（前端）
- [x] `T-006` 事件分析 Panel（前端）
- [x] `T-007` 属性分析 Panel（前端）
- [x] `T-008` 报表视图保存与列表（前后端）
- [x] `T-009` Chart Type 模型与报表持久化（前端）
- [x] `T-010` 事件分析图表类型切换（line/column）
- [x] `T-011` 属性分析图表类型切换（bar/pie）

## 5. Milestones

- Milestone M1: Backend Ready
  - 包含: T-001 ~ T-004
  - Exit criteria:
    - 新模型与迁移可用；
    - 自定义分析 API 可在本地被前端或 curl 调用并返回正确 JSON。

- Milestone M2: Analytics UI Ready
  - 包含: T-005 ~ T-007
  - Exit criteria:
    - `/analytics` 中三个子菜单可用；
    - 事件分析与属性分析基础场景可完成。

- Milestone M3: Reports Ready
  - 包含: T-008
  - Exit criteria:
    - 报表视图保存/打开流程可用于 1–2 个日常场景。

## 6. Feature-Level Completion Conditions

- 所有 P0 任务标记为 `DONE`，并满足对应 Completion conditions。
- 基础 Analytics tab 功能不退化。
- 关键事件分析/属性分析场景在新 UI 中可用。
- 报表视图（若本迭代完成）在真实使用场景中可稳定使用。

## 7. Plan Update Rules

- 状态变化时及时更新 TODO Board 中对应任务状态。
- 新增/拆分任务需更新本计划，并在说明中标记原因与关联需求。
- 若实现范围显著超出设计（例如支持多属性 group_by、多图看板），需先更新设计文档并重新评审后再执行。


### Task `T-009` - Chart Type 模型与报表持久化（前端）

- Status: `DONE`
- Owner: Codex
- Priority: `P1`
- Linked requirements: `FR-008`, `FR-009`
- Objective:
  - 在前端增加图表类型（chart_type）配置，并在报表视图中持久化保存/恢复该配置。
- Task details:
  - 设计并实现前端 chart_type 模型：`presentation.chart_type`（line/column/bar/pie）；
  - 在事件/属性分析 Panel 保存报表时，将当前 chart_type 写入 `AnalysisView.query.presentation`；
  - 在打开报表时，若存在该字段则恢复到 UI state，未配置则使用默认类型。
- Allowed modification range:
  - `logtap/web/src/ui/pages/analytics/*`
- Dependencies:
  - `T-006`、`T-007` 完成基础事件/属性分析 UI。
- Completion conditions:
  - [ ] 保存报表后重新打开，可正确恢复图表类型选择，并按该类型渲染结果。

### Task `T-010` - 事件分析图表类型切换（line/column）

- Status: `DONE`
- Owner: Codex
- Priority: `P1`
- Linked requirements: `FR-008`
- Objective:
  - 在事件分析面板中支持按时间的折线/柱状图切换展示。
- Task details:
  - 为事件分析增加 chart_type state（line/column），并提供 UI 切换控件；
  - 扩展现有 Sparkline 组件或新增变体，支持使用柱状图样式渲染时间序列；
  - 在保存/打开报表时通过 `T-009` 的模型持久化 chart_type。
- Allowed modification range:
  - `logtap/web/src/ui/components/Sparkline.tsx`
  - `logtap/web/src/ui/pages/analytics/EventAnalyticsPanel.tsx`
- Dependencies:
  - `T-004`、`T-006` 完成。
- Completion conditions:
  - [ ] 在 UI 中可以切换折线/柱状图，渲染数据一致；
  - [ ] 报表保存后再次打开可恢复图表类型。

### Task `T-011` - 属性分析图表类型切换（bar/pie）

- Status: `DONE`
- Owner: Codex
- Priority: `P1`
- Linked requirements: `FR-009`
- Objective:
  - 为「属性值分布」场景增加条形图/饼图两种可切换展示方式。
- Task details:
  - 在 PropertyAnalyticsResult（无 time 场景）中增加 chart_type state（bar/pie）和 UI 切换控件；
  - 实现一个轻量 SVG 饼图组件或内联实现；
  - 通过 `T-009` 的模型在报表保存/打开时持久化 chart_type。
- Allowed modification range:
  - `logtap/web/src/ui/pages/analytics/PropertyAnalyticsPanel.tsx`
- Dependencies:
  - `T-007` 完成。
- Completion conditions:
  - [ ] 属性值分布可在条形图/饼图之间切换，统计结果一致；
  - [ ] 报表保存后再次打开可恢复图表类型。

