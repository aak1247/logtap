# Analytics v2 – 基础分析 / 事件分析 / 属性分析

## 1. Document Control

- Feature name: Analytics v2 – 基础分析 / 事件分析 / 属性分析
- Version: v1.0
- Author: Codex
- Reviewers: TBD
- Last updated: 2026-03-10
- Status: `DRAFT`
- Review gate status: `PASS`
- Linked review report: (TBD)

## 2. Context and Problem

- Business background:
  - logtap 已提供完整的数据采集与分析基础设施：
    - 上报接口：`/api/:projectId/logs/`（日志）与 `/api/:projectId/track/`（事件）。
    - 存储：Postgres/TimescaleDB 中的 `logs`、`track_events`、`track_event_daily`。
    - Redis 指标：`RedisRecorder` 维护 DAU/MAU、留存、设备/地域分布等。
  - Analytics 页面目前提供固定报表：用户增长、DAU/MAU、OS/国家/ASN 分布、留存、Top 事件与简单漏斗。

- Current pain points:
  - 事件缺乏“官方定义”：event name（如 `signup`、`project_created`）被当作字符串使用，产品/研发难以对齐语义和推荐用法。
  - 属性缺乏类型与定义：`fields`/`properties` 中的 key 没有统一 schema，不区分字符串 / 枚举 / 数字，难以在 UI 中安全地做过滤与分组。
  - 分析方式固定：
    - 用户无法像 We 分析那样按事件 / 属性自由配置分析视图（如按 `plan` 分组看事件趋势）。
    - 无法保存分析配置为“报表”，每次需要手动重复配置。

- Why now:
  - 底层数据链路已经比较完备；
  - 在一轮可控迭代内，可以在现有 Analytics 上方增加“事件/属性管理 + 自定义分析”的产品层能力，大幅提升日常分析效率。

- Related links (issues, PRD, incidents, metrics):
  - `logtap/docs/OVERVIEW.md`
  - `logtap/docs/INGEST.md`
  - `logtap/internal/query/analytics.go`
  - `logtap/internal/query/event_analysis.go`
  - `logtap/web/src/ui/pages/AnalyticsPage.tsx`

## 3. Goals and Non-Goals

- Goals:
  - G1: 将当前 `/analytics` 页面拆分为三个子菜单：基础分析 / 事件分析 / 属性分析，保持基础分析功能不退化。
  - G2: 引入“事件定义”和“属性定义”模型与 API，属性支持类型：字符串 / 枚举 / 数字。
  - G3: 在 Analytics 页内实现自定义分析配置器：
    - 支持事件分析（按事件维度，趋势、多事件对比、按属性拆分）。
    - 支持属性分析（按属性值分布、按时间 + 属性堆积趋势）。
    - 支持图表类型：折线图、柱状图、堆积图、饼图（根据分析配置自动选择/限制）。
  - G4: 支持将分析配置保存为“报表视图”（analysis view），可在列表中查看并一键恢复执行。

- Non-goals:
  - NG1: 不实现完整“用户分群生命周期管理”（仅做基于事件/属性的统计分析）。
  - NG2: 不实现 AB 实验平台（不包含 AB 分流、显著性检测等）。
  - NG3: 不实现复杂看板/拖拽布局，多图拼装留给后续迭代。
  - NG4: 不改动现有 ingest pipeline 与 Redis 指标逻辑。

## 4. Scope

- In scope:
  - `/analytics` 页面内部结构重构：左侧三个子菜单，右侧对应面板。
  - 事件定义（EventDefinition）与属性定义（PropertyDefinition）数据模型及 CRUD API。
  - 自定义分析 API `POST /api/:projectId/analytics/custom`，支持事件分析与属性分析。
  - 事件分析与属性分析前端配置器与图表渲染。
  - 报表视图（AnalysisView）模型、API 与简单的“保存/打开报表”前端功能。

- Out of scope:
  - 多租户复杂权限模型（本迭代不区分谁可以编辑事件/属性定义）。
  - 复杂数值属性分析（高维指标、分位数、多区间桶等），先只支持 count/简单数值聚合。

- Constraints (time, compliance, platform, compatibility):
  - 需在一个普通迭代周期内完成（1–2 周开发 + 内部试用）。
  - 后端需兼容 Postgres + SQLite（仍使用 GORM + AutoMigrate）。
  - 保持现有 Analytics API 与 URL 向后兼容。

## 5. Users and Use Cases

- Target users/actors:
  - logtap/logtap-cloud 自身的开发、产品、运营。
  - 未来使用 logtap 的高级用户（需要接近 We 分析的自助能力）。

- Key use cases:
  - U1: 产品想分析“用户注册 → 创建项目 → 配置告警”路径中，各事件的用户数趋势，并按 `plan` 拆分比较。
  - U2: 运营想查看最近 30 天不同 `plan` 用户的事件占比（饼图）以及随时间变化的堆积趋势。
  - U3: 开发定义了新事件 `cloud_ingest_started`，希望在“事件管理”中登记语义，并在事件/属性分析中可见。
  - U4: 将一套常用分析配置（例如“按 plan 分组的注册趋势”）保存为报表，之后无需重复配置即可查看。

- Edge cases:
  - 事件/属性尚未在定义中登记，但已存在真实数据，需要在 UI 中 graceful fallback。
  - 属性定义为 enum，但真实数据中存在未列出的值（需要在图表中展示“其他/未知”）。
  - 数据量很少或为空时，图表需要显示“暂无数据”而不是报错。

## 6. Requirements

### 6.1 Functional Requirements

- `FR-001`: `/analytics` 页左侧提供“基础分析 / 事件分析 / 属性分析”三个子菜单，切换不改变 URL。
- `FR-002`: 基础分析子页展示当前 Analytics 页全部现有内容，不退化任何已有指标与图表。
- `FR-003`: 提供事件定义 API，以 project 为作用域：
  - `GET /api/:projectId/events/schema`
  - `POST /api/:projectId/events/schema`
  - `PUT /api/:projectId/events/schema/:name`
- `FR-004`: 提供属性定义 API，以 project 为作用域，支持类型：string/enum/number：
  - `GET /api/:projectId/properties/schema`
  - `POST /api/:projectId/properties/schema`
  - `PUT /api/:projectId/properties/schema/:key`
- `FR-005`: 在 Settings 区域提供“事件与属性管理”界面，至少支持：
  - 事件列表：name、display_name、category、status、最近使用时间（可后置）。
  - 事件详情编辑：display_name、category、description、status。
- `FR-006`: 在 Settings 中提供属性管理 UI，支持：
  - 属性列表：key、display_name、type、status。
  - 属性详情编辑：type、enum_values（枚举值）、description。
- `FR-007`: 实现 `POST /api/:projectId/analytics/custom` 自定义分析 API，支持：
  - `analysis_type` = `event | property`；
  - metric: `count_events`, `count_users`；
  - group_by: 至多两个维度（time + event / time + property / property）。
- `FR-008`: 事件分析子页支持：
  - 选择时间范围与粒度；
  - 选择 1–N 个事件；
  - 选择一个指标；
  - 选择 0–1 个属性维度与过滤条件；
  - 展示折线图/柱状图/堆积图与数据表格。
- `FR-009`: 属性分析子页支持：
  - 选择单个属性；
  - 可选“按时间展示”开关；
  - 支持属性值分布（饼图/柱状图）和按时间堆积趋势（堆积柱/多折线）。
- `FR-010`: 支持将分析配置保存为报表 view，提供：
  - `GET/POST/GET/:id/DELETE /api/:projectId/analytics/views`；
  - 前端在事件/属性分析页提供“保存报表”“打开报表”入口。

### 6.2 Non-Functional Requirements

- `NFR-001` Performance:
  - 自定义分析 API 在正常数据量下应在 2 秒内返回；
  - 对复杂查询设置合理超时（例如 5 秒），超时返回可读错误。
- `NFR-002` Reliability:
  - 自定义分析 API 出错不得影响基础 Analytics API；
  - 所有新表通过 AutoMigrate 自动初始化，避免手工 SQL 差异。
- `NFR-003` Security/Privacy:
  - 通过现有 auth 中间件限制分析与定义 API 仅在当前 project 内访问；
  - 不额外开放跨 project 查询能力。
- `NFR-004` Observability:
  - 自定义分析请求记录结构化日志（project_id、analysis_type、耗时、是否成功）；
  - 若后续需要，可在 metrics 中增加基本指标（本迭代可选）。

## 7. Proposed Solution

- High-level approach:
  - 在后端新增三类模型：事件定义、属性定义、分析视图。
  - 在 `internal/query` 中新增对应 CRUD handler 与 `analytics_custom` handler。
  - 在前端重构 Analytics 页为 3 个子面板，并新增事件/属性分析 UI。
  - 在 Settings 添加“事件与属性管理”入口，管理事件与属性定义。

- Architecture impact:
  - 数据模型：在 `internal/model/models.go` 中新增 3 个 struct，并接入 AutoMigrate。
  - Query 层：新增 3–4 个 handler 文件；其余现有 handler 基本不变。
  - 前端：Analytics 页从单一组件拆分为多个 Panel，增加分析配置器和与新 API 的交互。

- Components/modules affected:
  - Backend:
    - `logtap/internal/model/models.go`
    - `logtap/internal/migrate/migrate.go`
    - `logtap/internal/query/event_schema.go`
    - `logtap/internal/query/property_schema.go`
    - `logtap/internal/query/analytics_custom.go`
    - `logtap/internal/query/analysis_views.go`
    - `logtap/internal/httpserver` 路由绑定文件
  - Frontend:
    - `logtap/web/src/ui/pages/AnalyticsPage.tsx`
    - `logtap/web/src/ui/pages/analytics/EventAnalyticsPanel.tsx`
    - `logtap/web/src/ui/pages/analytics/PropertyAnalyticsPanel.tsx`
    - `logtap/web/src/lib/api.ts`
    - Settings 下新的事件/属性管理页面（如 `SettingsEventsPage` 等）。

- Data model changes:
  - `EventDefinition`：project 级事件定义表。
  - `PropertyDefinition`：project 级属性定义表，包含类型与枚举值。
  - `AnalysisView`：报表视图表，存储 query JSON 与元信息。

- API/contract changes:
  - 新增事件定义、属性定义、自定义分析、报表视图相关 API。
  - 所有新 API 使用统一的 JSON envelope 与错误风格，保持与现有 query 接口一致。

- UI/UX changes:
  - `/analytics` 中新增左侧导航，用户可在“基础/事件/属性”之间切换。
  - 事件/属性分析页面提供图形化配置器和结果图表。
  - Settings 中新增“事件与属性管理”入口，供高级用户维护定义。

- Backward compatibility:
  - 原有 Analytics 功能与 URL 均保持不变，默认进入“基础分析”标签。
  - 未定义的事件/属性仍可被分析，UI 在找不到 display_name 时回退使用原始 key/name。

## 8. Alternatives and Tradeoffs

- Option A: 新增独立路由 `/analytics/custom`，将自定义分析与基础 Analytics 完全分离。
  - Pros: 路由清晰、责任边界明确。
  - Cons: 用户体验割裂，入口增多；需要在多个页面之间跳转。

- Option B（选定）: 复用 `/analytics`，内部通过左侧子菜单划分“基础/事件/属性分析”。
  - Pros: 单一入口、符合“分析页分 3 个子菜单”的需求；便于复用现有数据加载逻辑。
  - Cons: 单页面内组件较多，需要良好组件组织避免过于庞大。

- Selected option: Option B。

- Tradeoff summary:
  - 选择在同一页面中统一 Analytics 能力，使用户以“分析视图”而非“页面”思考问题，代价是 `AnalyticsPage` 需进行一定程度的重构与拆分。

## 9. Risk Assessment

- Risk: 自定义分析 SQL 实现复杂，易出现性能问题或维护困难。
  - Impact: 某些分析请求缓慢或超时，影响体验。
  - Mitigation: 本迭代限制功能范围：仅支持 count_events/count_users、最多两个 group_by 维度，使用明确的 query builder；对超时做限制。
  - Fallback/rollback: 遇到严重性能问题时，可在路由层暂时关闭自定义分析 API 或隐藏事件/属性分析 tab。

- Risk: 属性数据不规范（例如 number 类型字段混入非数字字符串）。
  - Impact: 分析查询报错或统计结果不准确。
  - Mitigation: 解析字段时做容错，对无法转换的值忽略并计入日志；类型主要用于 UI 提示与简单校验。
  - Fallback/rollback: 可以将属性类型暂时回退为 string，减少强绑定假设。

- Risk: 一次迭代内范围过大，导致交付延期。
  - Impact: 特性无法在预期版本中使用；增加维护负担。
  - Mitigation: 将任务拆分为 P0（后端模型+API+3 tab + 事件分析）与 P1（属性分析+报表视图），保证 P0 场景可独立交付。
  - Fallback/rollback: 若时间不足，可只启用事件分析，属性分析与报表视图放到后续迭代。

## 10. Validation Strategy

- Unit testing strategy:
  - 对新模型相关 CRUD 逻辑进行基础单元测试（尤其是查询/唯一性约束）。
  - 对自定义分析 query builder 进行单元测试，覆盖 event/property 两种 analysis_type 及典型 group_by/filter 组合。

- Integration/E2E strategy:
  - 在本地或 CI 环境，通过 API 测试：定义事件/属性 → 上报 track/logs → 调用自定义分析 API → 对比结果与手写 SQL。
  - 使用前端 E2E（如 Playwright）进行简单回归：进入 `/analytics`，在 3 个 tab 中执行基本操作。

- Manual verification:
  - 用 demo project 配置 3–4 套常见分析场景，人工确认图表与预期一致。
  - 验证基础 Analytics tab 行为与旧版本一致。

- Metrics and alerts:
  - 本迭代可先仅通过日志观测；如有需要，可在后续迭代中加入 Prometheus 指标。

- Acceptance criteria:
  - AC-001: `/analytics` 出现“基础/事件/属性分析”三个子菜单，基础分析内容不退化。
  - AC-002: 使用事件/属性定义 API 可为 project 定义 name/key 与类型。
  - AC-003: 在事件分析子页可完成单事件趋势、多事件对比、按属性拆分分析。
  - AC-004: 在属性分析子页可完成属性值分布与按时间堆积趋势分析（若 P1 实现）。
  - AC-005: 至少一种场景的报表视图保存/打开流程可正常工作（若 P1 实现）。

## 11. Rollout Plan

- Release phases:
  1. 合并后端模型与 API（M1: Backend Ready），在内部环境验证。
  2. 合并前端 3 子菜单与事件分析面板（M2: Analytics UI Ready），内部使用并收集反馈。
  3. 合并属性分析与报表视图（M3: Reports Ready，若本迭代完成）。

- Feature flags:
  - 如有需要，可通过简单配置或环境变量临时隐藏事件/属性分析 tab（例如在 Settings 中加开关或前端条件渲染）。

- Migration/backfill:
  - 新表使用 AutoMigrate 创建，不需要额外迁移。
  - 如需对现有事件/属性做初始发现，可在后续迭代中增加“自动发现”逻辑（非本迭代必需）。

- Rollback triggers:
  - 自定义分析请求超时或错误率异常升高；
  - Analytics 页严重 UI 回归影响日常使用。

- Communication plan:
  - 在团队内部同步该特性，并提供简短使用说明（如何在事件/属性分析中构建常用报表）。

## 12. Open Questions

- `Q-001`: 报表视图是否需要共享/权限控制（例如仅 owner 可编辑，其他人只读）？当前决定：本迭代不做权限，所有登录用户均可见/编辑。
- `Q-002`: 是否需要在自定义分析 API 中支持多属性 group_by？当前决定：本迭代仅支持单属性 group_by，避免 SQL 复杂度爆炸。

## 13. Review Feedback Tracking

- Required changes from latest review:
  - 明确属性定义 UI 放在 Settings，而非 Analytics 内。
  - 明确本迭代不做权限控制。
- Applied changes:
  - 在 FR-005/FR-006 中将属性定义 UI 放入 Settings；
  - 在 Non-goals 与 Open Questions 中明确权限控制出于范围外。
- Remaining review actions:
  - 决定报表视图是否在前端暴露为“共享报表”还是“个人视图”（可后续细化）。

## 14. Definition of Design Complete

设计视为完成须满足：

- 所有必需章节已填写，并覆盖本迭代范围；
- 所有功能需求与非功能需求有明确 ID，便于追踪到执行计划；
- Review gate 状态为 `PASS`；
- 主要风险与回退策略已记录；
- 验证策略和接受标准明确可执行；
- Open questions 要么已有临时决策，要么明确留给后续迭代处理。


## 15. 图表类型扩展（Chart Types）

- Scope（本次迭代内）：
  - 事件分析：
    - 支持在按时间维度分析时，在折线图（line）和柱状图（column）之间切换；
    - 不改动后端 API，仅在前端基于相同 `series/points` 数据切换渲染方式。
  - 属性分析：
    - 对「仅按属性值聚合（无时间）」的场景，支持条形图（bar）与饼图（pie）两种可切换展示；
    - 对「按时间 + 属性值」的场景，暂保持当前“多条折线趋势”，堆积面积/堆积柱状图放到后续迭代。
- Chart 类型模型：
  - 在前端报表配置中引入 `presentation.chart_type` 字段（字符串枚举）：
    - `"line"` – 折线图（默认）；
    - `"column"` – 柱状图（仅在 group_by 含 time 时生效）；
    - `"bar"` – 水平条形图（用于属性分布）；
    - `"pie"` – 饼图（用于属性分布）。
  - 后端 `AnalysisView.query` 仍然存储任意 JSON，`presentation` 仅由前端约定使用，不影响自定义分析 API。
- UI 行为：
  - 事件分析：
    - 在结果 Panel 顶部增加「图表类型」切换（line/column）；
    - `group_by` 包含 `time` 时，根据 chart_type 选择折线或柱状渲染；
    - 仅 group_by time 的场景中生效，多维（time+event/property）仍然逐 series 渲染对应线/柱。
  - 属性分析：
    - 在「属性值分布」（无 time）结果 Panel 中增加图表类型切换（bar/pie）；
    - `bar`：使用横向条形图展示各属性值 total；
    - `pie`：使用轻量 SVG 饼图展示占比，同时保留下方表格数据。
- 报表视图：
  - 保存报表时：
    - 将当前图表类型写入 `query.presentation.chart_type`；
  - 打开报表时：
    - 若 `presentation.chart_type` 存在则恢复为当前选中类型，否则使用该场景默认值（事件分析为 line，属性分布为 bar）。

