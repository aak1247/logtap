# logtap ClickHouse 存储设计（云端高吞吐写入与查询）

- 状态：终稿（用户决策已定稿，进入实施阶段）
- 日期：2026-10
- 前置文档：`docs/CLOUD_MULTI_TENANCY_STORAGE_DESIGN.md`（tenant 模型、StorageRouter、契约测试与 Phase 划分，本文是其 Phase 2/3 的落地细化）
- 范围：`logtap` 数据面（ingest / 查询 / 日志与事件存储）。企业版云端服务（cloud-plane/edge，以下简称"云端 edge"）只做鉴权、限流、配额与反向代理，不存日志数据，本文不涉及其内部实现。
- 开源安全：本文不包含任何内部域名、IP、密钥或镜像仓库地址；涉及云端专属能力一律以"企业版/云端分支"泛称。

---

## 1. 背景与目标

### 1.1 现状瓶颈（量化）

写入链路现状（代码事实）：

```
HTTP ingest (/logs/ /track/ /store/ /envelope/)
  → AsyncBufferedPublisher（队列 10000、16 worker、入队超时 1s 后丢弃，internal/queue/async_publisher.go:29-33,52-73）
  → NSQ（topics logs/events，单节点无副本）
  → consumer（logs 并发默认 200、MaxInFlight 默认 2000、MsgTimeout 30s，internal/consumer/consumer.go:80-96，config/config.go:143-145）
  → Batcher[model.Log]（默认 500 条/20ms flush，Add 阻塞等本批 flush 完成，internal/consumer/batcher.go:32-86）
  → 单事务写 Postgres/TimescaleDB：
     pg_advisory_xact_lock 串行化 + 按 (project_id, ingest_id) 预查询去重
     + ON CONFLICT DO NOTHING 写 logs
     + 同事务副作用 upsert（user_first_seen / project_counters / log_daily_stats / track_event_daily）
     （internal/store/track_events.go:68-110, 207-282；internal/store/batch.go:41-）
```

已实测/已核实的瓶颈：

| 瓶颈 | 证据 | 影响 |
|---|---|---|
| PG 行存写入拐点约 7.5k EPS，200 并发后 P95 冲到 1.9s+ | `docs/perf/README.md` 阶梯压测表（160 VU 拐点 7,463 EPS；200 VU P95 1,988.9ms）。外部基准：PG/Timescale 日志写入上限约 10–20k rows/s（来源：外部调研） | 云端单租户 Business 档峰值 2,000 eps（`CLOUD_MULTI_TENANCY_STORAGE_DESIGN.md` §2.2）尚可，但多租户叠加后很快触顶 |
| `fields JSONB` + GIN + FTS 表达式索引 + pg_trgm 三重索引写放大 | `internal/migrate/migrate.go:78-104` | 每行写入维护 3 个 GIN 索引，是写入拐点早到的主因之一 |
| 副作用表热点行锁：`track_event_daily`/`project_counters`/`log_daily_stats` 按 (project_id, day, ...) 聚合 upsert | `internal/store/track_events.go:207-282`、`internal/store/metrics_rollup.go:60-91` | 大租户并发 flush 时在热点行上排队 |
| 去重成本：每批先 `pg_advisory_xact_lock` 再按 ingest_id 预查询 | `internal/store/track_events.go:121-196` | 每批至少 2 次额外 RTT + 锁 |
| 聚合查询极慢：`/logs/trend` P50 8.4s、稳定 QPS 2.5；`/analytics/retention` QPS 5.0 | `docs/perf/README.md` 查询基准表 | 控制台仪表盘体验差，查询一多就拖垮 DB |
| `/metrics/total` 回退路径全表 `COUNT(*)` + 跨表 `COUNT(DISTINCT distinct_id)` | `internal/store/metrics_rollup.go:265-340`（`GetDBMetricsTotalRaw` / `countDistinctUsersAcrossSources`） | PG 上最贵的查询，rollup 缺失时直接扫全表 |
| DB 连接池默认 25，生产 k8s 配 10 | `internal/config/config.go:146`、`deploy/k8s/prod.yaml`（`DB_MAX_OPEN_CONNS: "10"`） | 查询与写入共享小池，互相挤压 |
| 全单实例部署（gateway×1、nsqd×1） | `deploy/k8s/prod.yaml`（`replicas: 1`）、`deploy/docker-compose.yml` | 无横向扩展能力，NSQ 无副本/无重放是已知短板 |

### 1.2 目标容量

以 `CLOUD_MULTI_TENANCY_STORAGE_DESIGN.md` §2.2 的套餐档位反推（Business：1 亿条/月 ≈ 平均 38.6 eps、峰值按 10x ≈ 386 eps/租户）：

| 指标 | 现状（实测/配置） | 目标（设计值） | 依据 |
|---|---|---|---|
| 写入吞吐 | 拐点 ~7.5k EPS（实测） | 单 CH 节点 100k EPS，预留到 200k+ | 外部调研：16–32C NVMe 单节点 100k–500k+ EPS（来源数据）；目标值留 5x 头部空间 |
| 日志查询 P95 | trend P50 8.4s（实测） | 常用时间窗（24h）查询 P95 < 1s；30 天聚合 P95 < 3s | 列存 + 排序键裁剪，估算 |
| 查询并发 | trend QPS 2.5（实测） | 单读节点 50–100 QPS 短查询，生产起步部署 1 写 1 读双副本（读写分离）+ 网关 3~5s 短 TTL 缓存，支撑云端 Enterprise 档 500 RPS 限流（`limits.go:44`） | 外部调研：CH 单节点高并发短查询约 10–100 QPS，扩并发靠只读副本；叠加缓存吸收重复聚合（来源数据与代码实测） |
| 存储成本 | 行存 + 3 GIN 索引，无列存压缩 | 同等数据量磁盘占用降至 1/5–1/10 | 外部调研：日志场景约 10x 压缩（来源数据）；按 5–10x 计 |
| 保留策略 | 按项目 `CleanupPolicy` 逐行 DELETE | TTL 整分区删除（`ttl_only_drop_parts=1`），零删除放大 | CH 官方推荐做法（来源数据） |

非目标：不替换元数据存储（用户/项目/Key/告警/监控定义继续用 PG，见 §7.1）；不改 HTTP API 契约；不在本期引入 Kafka（NSQ 复用，见 §5.1）。

---

## 2. 选型结论与淘汰理由

### 2.1 结论：**ClickHouse**

理由（按决策权重的排序）：

1. **查询模型匹配度最高**。logtap 的查询负载是"日志检索 + 分析聚合"双负载：`/logs/search`（关键词/字段过滤）、`/logs/trend`、`/analytics/custom`（JSON 属性 group by + `COUNT(DISTINCT distinct_id)`，`internal/query/analytics_custom.go:361-363`）、`/analytics/funnel`。CH 是 SQL，团队现有 GORM/SQL 经验直接复用；`uniqExact`/`uniqHLL12`、`windowFunnel`、`groupArray` 覆盖全部现有分析语义。`internal/search/types.go:66-67` 的 `SearchAdapter` 注释已点名 ClickHouse 作为预期实现。
2. **写入链路契合**。CH 官方不推荐 Kafka 表引擎、推荐外部 consumer 攒批 push（来源：外部调研）。现有链路"NSQ → Go consumer → Batcher 攒批"恰好就是推荐形态，只换 sink（clickhouse-go v2 `PrepareBatch` 原生协议批量写），不动队列与网关。
3. **多租户有成熟范式**。共享大表 + tenant 进排序键（PostHog `(team_id, toDate(ts), ...)`、Sentry Snuba `(project_id, toStartOfHour(ts), ...)`，来源：外部调研），与既定 tenant 设计稿（`CLOUD_MULTI_TENANCY_STORAGE_DESIGN.md` §3.2：tenant_id UUID）直接对齐。
4. **生态与运维可控**。Altinity clickhouse-operator 是 k8s 事实标准（v0.27.x），clickhouse-backup 支持 S3 增量备份；单节点起步即可承载目标容量，小团队运维负担可接受。
5. **成本**。日志压缩约 10x（来源数据），对照 ES 存储倍率 1.5–3x、总成本 8–15 倍（来源数据），存储成本下降一个数量级。

### 2.2 淘汰候选与理由

| 候选 | 淘汰理由 |
|---|---|
| **继续增强 PG/TimescaleDB** | 瓶颈是结构性的：行存 + JSONB GIN 写放大 + B-tree 索引维护决定写入上限约 10–20k rows/s（来源数据）；Timescale 列存压缩只改善存储与部分聚合，不解决 GIN 写放大、`COUNT(DISTINCT)` 全表扫、vacuum 膨胀。实测拐点 7.5k EPS、trend 查询 P50 8.4s（`docs/perf/README.md`）已说明调参空间耗尽。PG 保留为元数据与过渡期内的事务型写入库。 |
| **VictoriaLogs** | 单二进制、原生多租户（AccountID:ProjectID）、资源开销低，运维上确实最省。但：(a) LogsQL 非 SQL、无 join，团队 SQL/GORM 背景与现有查询生成代码（`internal/query/analytics_custom.go` 等即时 SQL）无法复用；(b) 分析能力弱——漏斗、自定义属性 group by、精确去重计数等对标 Sentry 的能力要么不支持要么需要绕路；(c) 接入意味着查询层整体重写而非实现一个 adapter。日志检索单一场景它是好选择，但 logtap 是"日志+事件+分析"三合一。 |
| **Apache Doris** | 倒排索引强、SQL 兼容好，但整体更重（FE/BE 多进程、元数据管理），运维人力少的团队持有成本高；日志场景压缩与写入性价比不比 CH 优。 |
| **Elasticsearch / OpenSearch** | 存储倍率 1.5–3x、总成本 8–15 倍（来源数据），与"降低存储成本"目标直接冲突；JVM 堆管理与分片运维负担重。 |
| **Grafana Loki** | AGPLv3 许可证对 SaaS 商业化不友好；grep 式查询并发差，结构化聚合能力弱（来源：外部调研）。 |
| **ClickStack/HyperDX 整套引入** | 其 otel collector + CH 宽表设计（PARTITION BY toDate、5 分钟粗时间桶进 ORDER BY、热字段物化列、`DateTime64 CODEC(Delta,ZSTD)`）被本方案吸收进 §4，但整套引入会替换现有 ingest API（Sentry 兼容端点），超出本期范围。 |

---

## 3. 总体架构

### 3.1 写入链路（目标态）

```
SDK/客户端
  │  HTTP /api/:projectId/{logs,track,store,envelope}
  ▼
[云端 edge（企业版）] 鉴权/限流/配额 → 注入 X-Logtap-Proxy-Secret（+ 企业版: X-LOGTAP-TENANTID）→ 反代
  │                                    （edge 现状：logtap-cloud internal/edge/reverse_proxy.go:27）
  ▼
[logtap gateway] ingest handler → NSQMessage{type, project_id, payload, meta, (tenant_id)}
  │                                （internal/ingest/handlers.go:18-29；tenant_id 字段按设计稿 §4.2 方案 1 以 omitempty 加入）
  ▼
NSQ topic logs / events
  │  channel: log-consumer（PG，过渡期保留）   ← 既有消费者，不动
  │  channel: ch-log-consumer（CH）           ← 新增，同 topic 独立 channel，天然双写；MaxInFlight=50000
  ▼
[CHAsyncBatcher（新组件，同进程）]
  ├── 1. NSQ Handler: 显式调用 msg.DisableAutoResponse() 阻止自动 ACK；反序列化后塞入一级管道 incoming 即刻释放
  ├── 2. 二级聚合缓冲：带锁有序时间戳切片 activeBatch []timedBatchItem
  ├── 3. 保活循环：每 5s 获取锁遍历切片，对驻留 >15s 的消息显式调用 msg.Touch() 续租，防止 MsgTimeout 误重投递
  └── 4. Flusher Workers（起步 1~2 分片 × 5,000 行 / 500ms~1s）
        → clickhouse-go v2 PrepareBatch 批量写入 ClickHouse 写入主节点 (Pod-0)
        → 写入成功：
            ├── 批量并发调用 msg.Finish() 进行真实 ACK 确认
            ├── 异步 Pipeline 写入 Redis 去重记录：SETEX dedup:{tenant_id}:{project_id}:{ingest_id}
            ├── 旁路异步派发：Redis recorder（活跃用户/地理分布打点）
            └── 旁路异步派发：alert evaluator（实时日志告警规则评估）
        → 写入失败：统一调用 msg.Requeue(backoff) 退避重试并释放占位
        → 优雅停机：捕获 SIGTERM 触发 20s 强制排空落盘，超时未落盘消息批量 Requeue(0) 释放回 NSQ
```

要点：

- **双写不靠改 producer**：NSQ 同一 topic 增加一个独立 channel 即获得完整消息副本（NSQ 语义），PG 消费者与 CH 消费者互不影响、独立 ack/重试。这是灰度期最安全的设计。
- **解耦同步等待，消灭微批写入**：废除 PG consumer 中 `Batcher.Add`（`internal/consumer/batcher.go:67-86`）同步阻塞等待单个批次落盘的强耦合模型。采用专用 `CHAsyncBatcher` 异步内存两级缓冲池 + 批量 ACK，配合 `NSQMaxInFlight` 调大至 50,000，彻底打通大吞吐消息灌入管道，杜绝微批小文件引发的 Too Many Parts 崩溃（详见 §5.2）。
- **显式 DisableAutoResponse 杜绝静默丢数**：NSQ Handler 入队即关闭 AutoResponse，落盘成功显式 Finish，优雅停机 20s 兜底 Requeue(0)，生命周期全受控。
- CH 消费者与 PG 消费者跑在同一 gateway 进程内（`cmd/gateway/main.go:189-203` 现有 consumer 装配处并排新增），由配置开关控制；需要独立扩缩容时可拆为单独 Deployment（同二进制、不同 env）。
- 双写期间告警评估与 Redis 指标打点由 PG 链路主导；Phase 3 停写 PG 前由 CHAsyncBatcher 旁路无缝接管（§9.3）。

### 3.2 查询链路（目标态）

```
控制台/API → [云端 edge（企业版）]
                  │ limits.go:44 Enterprise QueryRate: 500 RPS / Burst: 1000 RPS
                  ▼
         [3~5s 短 TTL 聚合缓存]（本地内存/Redis，Key: cache:query:{tenant}:{project}:{hash(q)}）
                  │ 命中则直接返回（吸收 80%+ 仪表盘轮询与重复请求）
                  ▼ （未命中）
         [logtap gateway] query handler（internal/query/*）
                  │ StorageRouter / 后端开关
                  ├── PG（元数据永远走 PG + 过渡期的数据读）
                  └── ClickHouse 集群（读写分离：1 写主 + 1 只读副本 + 3 Keeper）
                        ├── 写入流量（CHAsyncBatcher）──► ClickHouse 写入主节点 (Pod-0)
                        │                                    │ 数据实时复制 (ReplicatedMergeTree)
                        │                                    ▼
                        └── 查询流量（SearchAdapter）───► ClickHouse 只读副本 (Pod-1)
                              ├── /logs/search、/search        → logs 表（文本索引 + cursor 稳定分页）
                              ├── /logs/trend、/analytics/*    → logs/track_events + rollup 表
                              └── /metrics/today、/metrics/total → rollup 表（AggregatingMergeTree 毫秒级汇总）
```

- **只读副本前置为生产基线**：由于单节点 CH 短查询并发上限仅 10~100 QPS，面对企业版 Enterprise 档位 500 RPS 限流（`limits.go:44`）与多租户并发极易打崩，**云端生产起步即部署 1 写 1 读双副本**，物理隔离读写流量，并预留无状态只读副本横向扩容通道（详见 §6.4 与 §8.1）。
- **短 TTL 缓存层防击穿**：高频仪表盘（Trend/Today/Total）在网关/Edge 层增加 3~5s 短 TTL 缓存，将突发前端轮询拦截在存储引擎之外。
- 元数据（用户/项目/Key/告警规则/监控定义/分析视图/清理策略）**永远走 PG**，即 `CLOUD_MULTI_TENANCY_STORAGE_DESIGN.md` §5.1 的 `MetadataStore`。
- 数据读按端点 + 按项目灰度切换到 CH（§9）。

### 3.3 与云端 edge 的关系

- edge 的职责不变：project key / cloud JWT 校验、按字节计量的限流与配额（Redis Lua）、反代并注入 `X-Logtap-Proxy-Secret`（现状代码：`internal/edge/reverse_proxy.go:27`、`internal/edge/dataplane_proxy.go:297`）。
- 本方案只新增一个要求（企业版分支）：edge 额外注入 `X-LOGTAP-TENANTID`，与既定设计稿 §4.1 一致。开源版不解析该头，固定 `tenant_id = 00000000-0000-0000-0000-000000000001`（设计稿 §3.2 的 `DefaultTenantID`）。
- **限流/配额与存储解耦**：edge 按字节计量计费保持不变；CH 的存储成本下降不改变计费口径，只改善毛利。

---

## 4. 数据模型（ClickHouse）

目标版本：ClickHouse **26.3 LTS**（或更新的 26.8 LTS；只升 LTS、禁跨大版本）。以下 DDL 均幂等（`IF NOT EXISTS`），放在 `logtap` 新包 `internal/store/clickhouse/migrations/` 下按序执行。

### 4.1 logs 表

```sql
CREATE TABLE IF NOT EXISTS logtap.logs
(
    tenant_id    UUID                    DEFAULT '00000000-0000-0000-0000-000000000001',
    project_id   UInt32,
    timestamp    DateTime64(3, 'UTC')    CODEC(Delta(8), ZSTD(1)),
    ingest_id    UUID,                   -- NSQ message id，at-least-once 去重语义载体
    level        LowCardinality(String)  DEFAULT '',
    message      String                  CODEC(ZSTD(1)),
    fields       Map(LowCardinality(String), String) CODEC(ZSTD(1)),
    trace_id     String                  DEFAULT '',
    span_id      String                  DEFAULT '',
    distinct_id  String                  DEFAULT '',
    device_id    String                  DEFAULT '',

    -- 文本索引：jieba 分词（中文+英文 token），26.2 GA
    INDEX idx_message_text message TYPE text(tokenizer = 'jieba') GRANULARITY 1,

    -- minmax 跳过索引辅助 trace 点查之外的稀疏过滤
    INDEX idx_trace trace_id TYPE bloom_filter(0.01) GRANULARITY 4
)
ENGINE = MergeTree
PARTITION BY toDate(timestamp)
ORDER BY (tenant_id, project_id, toStartOfFiveMinutes(timestamp), timestamp)
SETTINGS ttl_only_drop_parts = 1,
         index_granularity = 8192;
```

设计论证：

- **PARTITION BY toDate(timestamp)**：按天分区。分区是 TTL 整分区删除与备份的基本单元；按天在 Business 90 天保留下约 90+ 个活跃分区/表，远低于分区过多警戒线。分区键不含 tenant/project（避免高基数分区爆炸），租户裁剪靠排序键。
- **ORDER BY (tenant_id, project_id, toStartOfFiveMinutes(timestamp), timestamp)**：
  - `tenant_id` 第一列：开源版全表常量（LowCardinality 等效压缩为常量），成本为零；企业版多租户时是最高效的租户物理隔离（同一 tenant 的数据在 part 内连续）。对标 PostHog/Snuba 范式（来源数据）。
  - `project_id` 第二列：所有现有查询的第一过滤条件（`internal/query/handlers.go:141`、`internal/search/adapters/postgres/postgres.go:55`）。
  - `toStartOfFiveMinutes(timestamp)` 粗时间桶第三列：ClickStack otel_logs 同款技巧（来源数据），同桶内 `message`/`fields` 聚簇提升压缩率；时间范围过滤仍可由第四列精确 `timestamp` 的 granule minmax 裁剪。
  - 不将 `level` 放入排序键：level 基数低（<10），LowCardinality + 列存过滤已足够，放入反而降低时间局部性。
- **fields 用 `Map(LowCardinality(String), String)` 而非新 JSON 类型**：新 JSON 类型 25.3+ GA，但官方对日志场景仍推荐 Map(LowCardinality(String), String) + ZSTD(1)（来源数据）。现有查询只用到 `fields->>'key'` 等值/IN（`internal/query/analytics_custom.go:286-296`、`internal/search/adapters/postgres/postgres.go:204-220`），Map 查找 `fields['key']` 完全覆盖；嵌套 JSON 查询不是现有 API 能力，不为它付 JSON 类型的动态列治理成本。值为非字符串时在写入侧序列化为 JSON 字符串。
- **timestamp 精度**：PG 侧是 `timestamptz`（`internal/model/models.go:62`），`DateTime64(3)` 毫秒精度既兼容又比 DateTime 更稳（NSQ 消息本身是毫秒级 id 时间戳）。
- **`ttl_only_drop_parts = 1`**：TTL 到期整分区删除，避免行级 mutation（来源数据）。推论：**不支持按项目差异化保留期**（见 §4.6 与 §11）。

### 4.2 events 表（Sentry 兼容事件/错误）

```sql
CREATE TABLE IF NOT EXISTS logtap.events
(
    tenant_id    UUID                   DEFAULT '00000000-0000-0000-0000-000000000001',
    project_id   UInt32,
    timestamp    DateTime64(3, 'UTC')   CODEC(Delta(8), ZSTD(1)),
    event_id     UUID,                  -- 原 events.id（model.Event.ID，internal/model/models.go:42）
    level        LowCardinality(String) DEFAULT '',
    title        String                 DEFAULT '' CODEC(ZSTD(1)),
    distinct_id  String                 DEFAULT '',
    device_id    String                 DEFAULT '',
    os           LowCardinality(String) DEFAULT '',
    platform     LowCardinality(String) DEFAULT '',
    release_tag  LowCardinality(String) DEFAULT '',
    environment  LowCardinality(String) DEFAULT '',
    user_id      String                 DEFAULT '',
    data         String                 CODEC(ZSTD(3)),  -- 原始 Sentry payload JSON
    INDEX idx_event_id event_id TYPE bloom_filter(0.005) GRANULARITY 4
)
ENGINE = MergeTree
PARTITION BY toDate(timestamp)
ORDER BY (tenant_id, project_id, toStartOfFiveMinutes(timestamp), timestamp)
SETTINGS ttl_only_drop_parts = 1;
```

- `data` 保留原始 JSON 字符串：`GetEventHandler` 只需要原样返回 `data`（`internal/query/handlers.go:41-53`），`RecentEventsHandler` 只读 `id/timestamp/level/title`（`internal/query/handlers.go:86-91`）——热字段已全部物化为列。ZSTD(3) 因为该列是大头且很少被扫。
- `event_id` 是随机 UUID，点查（`/events/:eventId`）无时间窗时靠 bloom_filter 跳索引 + project 裁剪；若后续延迟不达标，再在 API 层要求带时间 hint（开放问题 §11）。

### 4.3 track_events 表（行为事件，源自 level='event' 的 logs）

现状：consumer 从 logs 派生（`internal/store/track_events.go:16-43` `TrackEventRowsFromLogs`）。CH 侧保持同一派生逻辑，在写入侧一次拆两路写两张表（不用 MV 级联，原因见 §4.7）：

```sql
CREATE TABLE IF NOT EXISTS logtap.track_events
(
    tenant_id    UUID                   DEFAULT '00000000-0000-0000-0000-000000000001',
    project_id   UInt32,
    timestamp    DateTime64(3, 'UTC')   CODEC(Delta(8), ZSTD(1)),
    ingest_id    UUID,
    name         String                 CODEC(ZSTD(1)),
    distinct_id  String                 DEFAULT '',
    device_id    String                 DEFAULT '',
    INDEX idx_name name TYPE bloom_filter(0.01) GRANULARITY 4
)
ENGINE = MergeTree
PARTITION BY toDate(timestamp)
ORDER BY (tenant_id, project_id, name, timestamp)   -- name 进排序键：top/funnel 按 name 过滤
SETTINGS ttl_only_drop_parts = 1;
```

### 4.4 Rollup：AggregatingMergeTree + 物化视图（多物理分表扇入架构）

替代现有 PG 副作用表（`log_daily_stats` / `project_counters` / `track_event_daily` / `user_first_seen`，`internal/model/models.go:92-135`）：

```sql
-- 日粒度日志统计：/metrics/today、/logs/trend、/analytics/users 的读取源
CREATE TABLE IF NOT EXISTS logtap.log_daily_stats
(
    tenant_id   UUID,
    project_id  UInt32,
    day         Date,
    level       LowCardinality(String),
    cnt         AggregateFunction(count, UInt64),
    uniq_users  AggregateFunction(uniqExact, String)   -- distinct_id 精确去重
)
ENGINE = AggregatingMergeTree
PARTITION BY toYYYYMM(day)
ORDER BY (tenant_id, project_id, day, level);

-- 【关键设计：物化视图必须与底层写入物理表 1:1 绑定并扇入同一个聚合表】
-- 1. 开源版单表模式物化视图：
CREATE MATERIALIZED VIEW IF NOT EXISTS logtap.logs_daily_mv
TO logtap.log_daily_stats
AS SELECT
    tenant_id, project_id,
    toDate(timestamp) AS day,
    level,
    countState() AS cnt,
    uniqExactState(distinct_id) AS uniq_users
FROM logtap.logs
GROUP BY tenant_id, project_id, day, level;

-- 2. 企业版套餐分档物理表（logs_7d / logs_30d / logs_90d）专用物化视图：
-- ClickHouse 物化视图是物理表的写入触发器，写入分档表必须分别触发专用 MV 扇入目标表
CREATE MATERIALIZED VIEW IF NOT EXISTS logtap.logs_7d_daily_mv
TO logtap.log_daily_stats
AS SELECT
    tenant_id, project_id,
    toDate(timestamp) AS day,
    level,
    countState() AS cnt,
    uniqExactState(distinct_id) AS uniq_users
FROM logtap.logs_7d
GROUP BY tenant_id, project_id, day, level;

CREATE MATERIALIZED VIEW IF NOT EXISTS logtap.logs_30d_daily_mv
TO logtap.log_daily_stats
AS SELECT
    tenant_id, project_id,
    toDate(timestamp) AS day,
    level,
    countState() AS cnt,
    uniqExactState(distinct_id) AS uniq_users
FROM logtap.logs_30d
GROUP BY tenant_id, project_id, day, level;

CREATE MATERIALIZED VIEW IF NOT EXISTS logtap.logs_90d_daily_mv
TO logtap.log_daily_stats
AS SELECT
    tenant_id, project_id,
    toDate(timestamp) AS day,
    level,
    countState() AS cnt,
    uniqExactState(distinct_id) AS uniq_users
FROM logtap.logs_90d
GROUP BY tenant_id, project_id, day, level;

-- 行为事件日粒度：/analytics/events/top、漏斗加速
CREATE TABLE IF NOT EXISTS logtap.track_event_daily
(
    tenant_id   UUID,
    project_id  UInt32,
    day         Date,
    name        String,
    cnt         AggregateFunction(count, UInt64),
    uniq_users  AggregateFunction(uniqExact, String)
)
ENGINE = AggregatingMergeTree
PARTITION BY toYYYYMM(day)
ORDER BY (tenant_id, project_id, day, name);

CREATE MATERIALIZED VIEW IF NOT EXISTS logtap.track_events_daily_mv
TO logtap.track_event_daily
AS SELECT
    tenant_id, project_id,
    toDate(timestamp) AS day,
    name,
    countState() AS cnt,
    uniqExactState(distinct_id) AS uniq_users
FROM logtap.track_events
GROUP BY tenant_id, project_id, day, name;
```

读取侧用 `countMerge(cnt)` / `uniqExactMerge(uniq_users)`。

- **MV 无回填**（来源数据）：MV 只对建成后的新写入生效。双写期（§9 Phase 1）CH 表从零开始累积，天然规避回填；历史数据如需迁移，用手动 `INSERT INTO ... SELECT`（§9.4）。
- **MV 失败语义**：MV 是同步插入的一部分，rollup 表不可用会拖慢主表写入；因此 rollup 表保持极简（两列聚合键），并监控 `system.errors`。若成为问题，降级方案：去掉 MV，改为每 10 分钟的 `INSERT INTO ... SELECT ... WHERE timestamp >= watermark` 增量任务（可回填、可控速，运维上更笨但更稳）。该降级方案作为 §11 开放问题跟踪。
- **Rollup 表误差与精确度声明（废除 exact=1 承诺）**：
  - MV 随原始日志写入流式触发。若发生网络抖动、worker 崩溃导致 NSQ 消息重投递，重复投递的消息落库时 MV 会再次触发累加，其聚合状态（`countState` / `uniqExactState`）物理上无法回溯剔除历史已累加的重试数据；
  - **明确废除在 Rollup 表上支持 `exact=1` 的设计**：Rollup 表专为秒级宏观概览与仪表盘设计，在网络重试窗口内允许存在细微统计误差（统计误差率 ≈ NSQ 重投递率，正常工况下通常 < 0.05%）；
  - `/metrics/total` 与 `/metrics/today` 常规查询恒读 rollup 表，毫秒级响应，不再需要 PG 那个跨表 `COUNT(DISTINCT)` 全表回退（`internal/store/metrics_rollup.go:265-340`）。需要绝对精确对账的场景统一走 raw 明细表短窗口对账（见 §4.7）。

### 4.5 文本索引与中文分词策略

现状 PG 有两条路径（`internal/query/handlers.go:154-164`）：
- `mode=fts`（默认）：`to_tsvector('simple') @@ plainto_tsquery`（表达式 GIN 索引，`internal/migrate/migrate.go:86-91`）；
- `mode=contains`：`message ILIKE '%kw%'`（pg_trgm GIN，`internal/migrate/migrate.go:97-104`）。

CH 映射：

| 现有模式 | CH 实现 | 说明 |
|---|---|---|
| `mode=fts`（默认） | `hasToken(message, ?)` / `hasAllTokens(message, [...])`，走 `text(tokenizer='jieba')` 索引（26.2 GA，来源数据） | 中文按词匹配，英文按 token；语义从 PG 的 'simple' 分词变为 jieba 分词，**召回会更好但排序无 BM25**（来源数据）——现有 API 本来就不返回相关性排序（按 timestamp DESC），无语义回归 |
| `mode=contains` | `positionCaseInsensitive(message, ?)`，无索引时列内扫描（列存下远快于 PG 行存 seq scan）；可选加 `ngrambf_v1(3, 65536, 4, 0)` 跳索引 | v1 默认不开 ngram 索引（message 是大列，ngram 索引体积可观），以开放问题跟踪（§11） |

关键词高亮继续在应用层做（现有实现就是应用层子串截取，`internal/search/adapters/postgres/postgres.go:222-247`），不依赖后端。

### 4.6 TTL 与分档保留策略（用户已拍板决策定稿）

#### 4.6.1 决策落地：按 SaaS 套餐档位（7/30/90 天）固定分表统一保留

根据用户明确决策、`CLOUD_MULTI_TENANCY_STORAGE_DESIGN.md` §2.2 套餐定义与外部调研事实：
ClickHouse 官方强烈建议日志场景采用 `ttl_only_drop_parts = 1` 执行整分区物理 Drop，以达成零写放大与零磁盘 Compaction 开销（来源数据）。
若在单表中采用行级表达式 TTL（如 `TTL timestamp + INTERVAL project_retention_days DAY`），会彻底破坏整分区删除机制，导致 ClickHouse 持续产生沉重的后台逐行 Mutation，严重侵蚀 NVMe I/O 并压低写入吞吐。

**终稿边界规则与实施要求**：
1. **废弃 CH 模式下每项目任意自定义保留天数**：云端统一按套餐档位划分为三档固定保留分表，物理 DDL 如下：

```sql
-- 基础物理分表定义（结构完全继承模版，仅 TTL 差异）
-- 1. Free 档（7 天保留）：
CREATE TABLE IF NOT EXISTS logtap.logs_7d AS logtap.logs
ENGINE = ReplicatedMergeTree
PARTITION BY toDate(timestamp)
ORDER BY (tenant_id, project_id, toStartOfFiveMinutes(timestamp), timestamp)
TTL timestamp + INTERVAL 7 DAY
SETTINGS ttl_only_drop_parts = 1, index_granularity = 8192;

-- 2. Team 档（30 天保留）：
CREATE TABLE IF NOT EXISTS logtap.logs_30d AS logtap.logs
ENGINE = ReplicatedMergeTree
PARTITION BY toDate(timestamp)
ORDER BY (tenant_id, project_id, toStartOfFiveMinutes(timestamp), timestamp)
TTL timestamp + INTERVAL 30 DAY
SETTINGS ttl_only_drop_parts = 1, index_granularity = 8192;

-- 3. Business 档（90 天保留）：
CREATE TABLE IF NOT EXISTS logtap.logs_90d AS logtap.logs
ENGINE = ReplicatedMergeTree
PARTITION BY toDate(timestamp)
ORDER BY (tenant_id, project_id, toStartOfFiveMinutes(timestamp), timestamp)
TTL timestamp + INTERVAL 90 DAY
SETTINGS ttl_only_drop_parts = 1, index_granularity = 8192;

-- 统一查询视图：屏蔽物理分表差异，查询层无感
CREATE VIEW IF NOT EXISTS logtap.logs_all AS
SELECT * FROM logtap.logs_7d
UNION ALL
SELECT * FROM logtap.logs_30d
UNION ALL
SELECT * FROM logtap.logs_90d;
```

2. **写入路由与生命周期管理（套餐升降级规则）**：
   - 写入层由 StorageRouter 根据项目所属租户的 Plan 动态路由至对应物理分表（Free → `logs_7d`，Team → `logs_30d`，Business → `logs_90d`）；
   - **租户升级（如 Free 7d → Team 30d）**：零搬迁成本！升级时刻起，新日志直接写入 `logs_30d`；残留在 `logs_7d` 中的历史旧日志无需搬迁，7 天到期由分区 Drop 自然淘汰。查询层通过 `logs_all` 视图进行 `UNION ALL` 检索，用户在过渡期内体验完全平滑连续；
   - **租户降级（如 Business 90d → Free 7d）**：降级时刻起新日志写入 `logs_7d`；已在 `logs_90d` 中的历史数据按原 90 天到期平滑自然清理（或合规要求下执行分区级 DROP），避免激进数据截断引发纠纷。
3. **开源兼容与策略对齐（Snap-to-Tier）**：
   - 开源单租户 PG 模式：现有每项目 `model.CleanupPolicy`（`internal/model/models.go:137-144`）的自定义 `logs_retention_days` 逻辑保持 100% 不变；
   - 云端 ClickHouse 模式：项目自定义策略自动向所属套餐档位对齐（Snap）。例如用户设置 15 天，在 Team 套餐下统一执行 30 天保留，该行为明确标注为云端版已知限制。
4. **不支持反向覆盖与大客户硬隔离**：
   - v1 不支持租户在所在套餐档位下设置短于档期的自定义覆盖；
   - Enterprise 大客户若提出 >90 天或专属合规保留期需求，走独立 ClickHouse 集群硬隔离物理部署，不与共享集群分表混部。
5. **GDPR/合规单条删除逃生通道**：
   - 现有 `DELETE /logs/cleanup`、`/events/cleanup` 定时清理 API（`internal/httpserver/httpserver.go:162-163`）在 CH 模式下直接返回成功并注明"由 TTL 托管"；
   - 针对个人数据保护法规（GDPR/CCPA）的“被遗忘权”单用户擦除需求，保留轻量删除（Lightweight Delete，`ALTER TABLE ... DELETE WHERE project_id=? AND distinct_id=?`）作为低频合规逃生通道；网关层严格限制该操作频率（单租户每日上限 5 次），**严禁将其作为常规过期清理手段**。

### 4.7 去重与幂等语义（用户已拍板决策定稿）

现状（PG）：`(project_id, ingest_id)` 精确去重，advisory lock + 预查询 + 唯一索引（`internal/store/track_events.go:121-196`、`internal/migrate/migrate.go:127`），`CLOUD_MULTI_TENANCY_STORAGE_DESIGN.md` §5.4.1 要求"相同 (tenant_id, project_id, ingest_id) 不得重复写入（至少最终不重复）"。

CH 上各选项的评估（依据：外部调研与用户评审）：

| 方案 | 结论 | 理由 |
|---|---|---|
| ReplacingMergeTree + FINAL | ❌ | 查询加 FINAL 性能劣化 2–10x 且极易引发内存 OOM；不加则无法保证读时去重 |
| 块级 insert dedup（`insert_deduplication_token`） | ❌（单独用） | 去重以块为单位且依赖批次边界完全一致；NSQ 重投递会与新消息重新组批改变批次边界，块 token 彻底失效（来源数据） |
| 追加写 + 查询层容错 | ✅ **终稿采用** | 业界 AP 可观测性标准（Sentry Snuba / PostHog 同款）；仅在进程崩溃或网络闪断窗口内发生微量重复（<0.05%） |
| 消费者侧 Redis 去重闸门 | ✅ **防御纵深** | 复用现有 Redis：`SETNX dedup:{project_id}:{ingest_id} EX 172800`（48h），拦截绝大部分网络故障重投递 |

#### 4.7.1 决策落地与实施要求

用户已正式批准去重语义调整方案：**CH 常规聚合容忍 NSQ 重投递微量重复（<0.05%），exact=1 仅支持 ≤24h 短窗口并下推 raw 表精确对账**。具体落地约束如下：

1. **底层物理模型**：
   - ClickHouse 明细表统一使用 `MergeTree` 纯追加写，保留 `ingest_id UUID` 字段作为幂等追踪载体；
   - 保留 ClickHouse 块级去重窗口 `non_replicated_deduplication_window = 1000`，作为批次边界未变时的第一道硬件级过滤。
2. **多层防御纵深与 Redis 去重时机（彻底防止重试误杀）**：
   - **包含租户的隔离命名空间**：去重 Key 统一调整为多租户安全格式：`dedup:{tenant_id}:{project_id}:{ingest_id}`（对齐 `CLOUD_MULTI_TENANCY_STORAGE_DESIGN.md` §3.2 与全局唯一约束），避免跨租户 project_id 重叠发生冲突；
   - **严禁提交前误杀占位**：若在提交 CH 之前通过 SETNX 占位，当 CH 写入偶发失败触发 `Requeue` 时，重投递的消息会被误杀丢弃！
   - **正确时机与容灾回滚规程**：
     - 消费者采用**落盘成功后异步记录**模式：在 Flusher 成功执行 `PrepareBatch.Send()` 落盘之后，通过后台 pipeline 向 Redis 异步批量写入 `SETEX dedup:{tenant_id}:{project_id}:{ingest_id} 172800 1`（48h TTL）；
     - 若采用提交前只读探测（`EXISTS`），未落盘前绝对不写入占位 Key；
     - 若开启入队预占位模式，当 PrepareBatch 发生错误触发 Requeue 时，必须在错误处理分支中通过 pipeline 显式批量执行 `DEL dedup:{tenant_id}:{project_id}:{ingest_id}` 释放已占位 Key，确保退避重试的消息在重新入队时能被正确接纳。
3. **误差可观测性与生产告警**：
   - 在 `CHAsyncBatcher` 导出监控指标：`logtap_consumer_nsq_requeue_total`（重投递计数）与 `logtap_consumer_dedup_dropped_total`（Redis 拦截重复数）；
   - 生产告警规则：当重投递率（requeue / total）**持续 5 分钟超过 0.1%** 时触发 P2 告警，督促排查网络与 NSQ 节点健康度。
4. **精确对账模式（exact=1）的严格管控**：
   - **废除 Rollup 表支持 exact=1**：Rollup 表专为秒级宏观概览设计，MV 流式累加状态不可撤回，不支持后验精确去重；
   - **限定短时间窗与下推 Raw 表**：当财务审计、合规对账确需绝对精确数字时，客户端调用携带 `exact=1`；API **强制校验 `start` 与 `end` 参数且时间跨度必须 ≤ 24 小时**；
   - 查询直接绕过 Rollup，下推至 raw 明细表执行：
     `SELECT count(DISTINCT ingest_id) AS exact_logs, uniqExact(distinct_id) AS exact_users FROM logtap.logs_all WHERE tenant_id=? AND project_id=? AND timestamp BETWEEN ? AND ?`；
   - 控制台与 API 文档明确标识 `exact=1` 为“高延迟精确审计模式”；网关对此类请求实施单项目并发限制 1，超时 15s。
5. **计费计量完全脱耦（资金安全）**：
   - 云端商业化计费、月度配额与超额截断严格在 **云端 edge 层**（`logtap-cloud/internal/edge/quota.go:11-30` 与 `limits.go`）基于 Redis 原子计数实时完成，按请求字节数计量，**完全不依赖存储层 ClickHouse 的去重状态与查询结果**，去重语义变更对计费准确性零影响。
6. **双写灰度期权威数据源**：
   - 在 Phase 1 双写与 Phase 2 灰度切读期间，所有业务指标与报表以 **PostgreSQL 为权威基准**，对账差异率稳定 <0.05% 且通过 14 天稳定性观测后，方可启动切读。

---

## 5. 写入链路改造

### 5.1 队列：复用 NSQ，不引入 Kafka

- 官方不推荐 CH Kafka 表引擎（级联 MV 非原子、librdkafka 线程坑，来源数据）；外部 consumer push 是推荐形态，现有 NSQ consumer 正是该形态。
- NSQ 单节点无副本/无重放是已知短板（`deploy/docker-compose.yml:25-32` 单 nsqd）。CH 重启窗口内 NSQ 落盘队列削峰；NSQ 本身宕机则 ingest 端点 503（现状 `internal/ingest/handlers.go:90-93` 行为保留）。Kafka/Redpanda 替换列入 §11 开放问题，本期不做。

### 5.2 新增 CH 消费者与异步攒批写入器（CHAsyncBatcher）

新代码位置（均开源安全）：

```
logtap/internal/store/clickhouse/
    client.go          -- clickhouse-go v2 连接封装（原生协议，PrepareBatch）
    migrations/        -- §4 的 DDL，幂等，启动时执行
    async_batcher.go   -- 专为 CH 打造的异步内存缓冲与批量 ACK 攒批器（CHAsyncBatcher）
    writer.go          -- 多分片/单分片写入调度器
    ingest_store.go    -- IngestStore 接口的 CH 实现
    query_store.go     -- QueryStore / SearchAdapter 的 CH 实现
logtap/internal/consumer/ch_consumer.go    -- NSQ channel "ch-log-consumer" / "ch-event-consumer"
```

装配点：`cmd/gateway/main.go:189-203`（现有 `NewNSQLogConsumer`/`NewNSQEventConsumer` 旁边，按 `STORAGE_BACKEND` 开关挂载）。

#### 5.2.1 架构缺陷剖析：为何不可复用 PG 消费者的 Batcher.Add？

现有 PG Consumer（`internal/consumer/consumer.go:263-354`）采用 `Batcher[model.Log].Add(row)` 模型（`internal/consumer/batcher.go:67-86`）：
1. **同步阻塞等待**：`Batcher.Add` 的语义是同步的（`req := batchReq[T]{item: item, done: done}; b.in <- req; <-done`），每个 NSQ handler goroutine 在将数据交给 batcher 后，必须阻塞等待当前批次真正落库返回后才能退出。
2. **并发数即并发积攒上限**：在任意瞬时，能够并发调用 `Add` 的最大数量完全受限于 NSQ handler 的并发数（`NSQLogConcurrency`，代码中默认 200，`config/config.go:145`；原 v1 误设为 50）。
3. **致命死锁与微批退化**：
   - 若设置 `CH_LOG_BATCH_SIZE = 20,000`，但并发 handler 仅有 50~200 个，内存中同时在等 flush 的元素上限就是 50~200 个，**批次永远不可能凑满 20,000 条**！
   - 系统只能不断退化为等待 `flushInterval`（1s）定时器超时强制 flush。结果是：**每秒只能 flush 一次，且每批仅有 50~200 条数据**！
   - 吞吐被硬生生卡死在 50~200 EPS；若外部涌入上万 EPS，因 handler 全被阻塞，默认 `NSQMaxInFlight = 2000`（`config/config.go:143`）会瞬间被占满，NSQ 停止推送，网关出现严重堵塞；
   - 更严重的是：ClickHouse 每秒接收多次几十条的极小批次，会产生海量微小 data parts，导致后台合并无法跟上，数分钟内就会爆发 `Too many parts in all data parts in table`（HTTP 500 / code 252）异常，彻底拒绝写入并拖垮集群。

#### 5.2.2 CHAsyncBatcher 核心设计：异步两级缓冲池 + 滑动窗口批量 ACK + 优雅排空

为彻底解决上述矛盾，针对 ClickHouse 大批次写入与长连接特征，重构设计专用组件 `CHAsyncBatcher`：

```
NSQ Daemon (nsqd)
  │  推送在途未确认消息（NSQ_MAX_IN_FLIGHT_CH = 50,000）
  ▼
NSQ Handlers（并发 50~100）
  │  1. 显式调用 msg.DisableAutoResponse() 阻止 go-nsq 自动 ACK
  │  2. 反序列化与清洗，封装为 chBatchItem{msg: *nsq.Message, row: CHLogRow, enqueuedAt: time.Now()}
  │  3. 推入第一级无锁入队通道 incoming chan *chBatchItem（容量 100,000）即刻返回
  ▼
[CHAsyncBatcher 内部聚合引擎]
  ├── 【第二级时间戳切片缓冲】Flusher 聚合器持有互斥锁，维护当前批次切片 activeBatch []timedBatchItem
  ├── 【Touch() 保活巡检协程】每 5s 获取锁遍历 activeBatch，对排队 >15s 消息显式调用 msg.Touch() 续租
  └── 【独立 Flusher Workers】（起步 1~2 分片）
        │  当 len(activeBatch) >= 5,000 或等待达到 500ms~1s：
        │  原子切出批次，交付 clickhouse-go v2 PrepareBatch 批量刷盘
        ▼
        落盘成功：
        ├── 遍历批次所有消息，批量并发执行 msg.Finish() 完成真实 ACK
        ├── 异步 Pipeline 写入 Redis 去重记录：SETEX dedup:{tenant_id}:{project_id}:{ingest_id}
        ├── 旁路异步派发：调用 Redis recorder.ObserveLog/ObserveEventDist（活跃/分布打点）
        └── 旁路异步派发：调用 evaluator.Submit(alert.InputFromLog(r))（实时告警评估）
        落盘失败：
        └── 遍历批次所有消息，执行 msg.Requeue(backoff) 指数退避，并释放占位
```

关键机制落地：
1. **显式关闭 AutoResponse（防静默丢数）**：
   - 依据 `logtap/internal/consumer/consumer.go:277-347` 与 `go-nsq` 原理，`nsq.HandlerFunc` 默认在函数返回 `nil` 时自动向 nsqd 发送 `FIN` 确认。
   - 在 `CHAsyncBatcher` 的 Handler 逻辑**最第一行必须显式调用 `m.DisableAutoResponse()`**！将该消息的确认权完全移交给后台写入器，只有当 ClickHouse 物理落盘成功后才由 Flusher 显式调用 `msg.Finish()`。若 Pod 异常崩溃，未落盘的消息因未收到 FIN，将在 MsgTimeout 到期后由 nsqd 自动安全重投递，杜绝内存数据静默丢失。
2. **两级缓冲池解决 Channel 无法窥探（Peek）的物理限制**：
   - Go 语言纯 `chan` 无法在不读取出队的情况下查看内部元素。为此采用**两级流水线结构**：
     - **一级管道**：`incoming chan *chBatchItem`（容量 100,000），由 NSQ handlers 高并发无锁灌入；
     - **二级切片**：Flusher 核心协程单向从 `incoming` 读出消息，聚拢存入带 `sync.Mutex` 保护的结构体 `activeBatch struct { sync.Mutex; items []timedBatchItem }`，每个条目带有 `enqueuedAt time.Time` 精确时间戳；
   - **精确 Touch() 续租保活**：独立保活协程每 5 秒获取互斥锁扫描 `items` 切片，对于停留超过 15 秒的消息直接调用 `item.msg.Touch()` 延长租期，彻底解决流量低谷期凑批超过 30 秒触发 NSQ 超时误重投递的问题。
3. **起步批大小保守化与单/双分片运行**：
   - 起步阶段将 `CH_LOG_BATCH_SIZE` 设为 **5,000 行**（约 5MB，处于 CH 极佳写入性能区间），吞吐升高后可按需放大至 10,000~20,000 行；
   - 避免分片过多导致批次碎片化：起步阶段 `CH_WRITE_SHARDS` 限制为 **1~2**（单节点 1~2 条写入流已足够喂饱 NVMe IO，每批数据高度集中），集群化后才扩展至 4。
4. **旁路接管 Redis Recorder 与日志告警评估（防止切流断流）**：
   - 依据 `internal/consumer/consumer.go:269-273, 328-343`，原有 PG consumer 同步驱动 Redis recorder 与 alert evaluator；
   - `CHAsyncBatcher` 在批次落盘成功后，通过独立的非阻塞 Worker 线程池无缝接管：
     - 调用 `recorder.ObserveLog` 与 `ObserveEventDist`，保障 `/analytics/active`、`/analytics/dist*` 接口持续输出指标；
     - 提交有效行至 `evaluator.Submit(alert.InputFromLog(r))`，保障实时告警规则引擎无缝运行。
5. **优雅关机规程（Drain Protocol）与安全排空时限**：
   - 进程收到 `SIGTERM` 信号时的退出顺序：
     1. 第一步：调用 `consumer.Stop()`，立即向 nsqd 声明停止拉取新消息；
     2. 第二步：向 Flusher 触发强制刷盘信号（Force Flush），设置最长排空超时为 **20 秒**；
     3. 第三步：若在 20 秒内落盘成功，对批内全部消息调用 `msg.Finish()`，完成安全退出；
     4. 第四步（超时兜底）：若因 CH 宕机在 20 秒内无法完成刷盘，立即对二级切片与一级管道中所有未完成的消息显式批量调用 `msg.Requeue(0)`，立即释放回 NSQ 重新入队，绝不遗留悬挂消息；
   - **K8s 部署规范**：在 `deploy/k8s/prod.yaml` 中将 Deployment 的 `terminationGracePeriodSeconds` 明确配置为 **60s**，为 20s 强制排空与服务注销预留充裕时间。

### 5.3 攒批参数与背压

| 参数（新增 env） | 默认值 | 依据与优化说明 |
|---|---|---|
| `STORAGE_BACKEND` | `postgres`（可选 `postgres`/`dual`/`clickhouse`） | 迁移总开关，§9 |
| `CLICKHOUSE_DSN` | 空（空则 CH 链路不启用） | `clickhouse://user:pass@host:9000/logtap`（原生协议直连） |
| `NSQ_MAX_IN_FLIGHT_CH` | `50000` | **重构关键项**：从 PG 默认的 2000（`config.go:143`）调大至 50,000，匹配 5,000~20,000 行大批次所需在途量 |
| `CH_WRITE_SHARDS` | `2`（起步建议 1~2） | **重构关键项**：从原 4 分片收敛为 1~2 分片，避免多分片稀释批次导致小微批 |
| `CH_LOG_BATCH_SIZE` | `5000` | **重构关键项**：起步取稳妥 5,000 行（约 5MB），彻底消除微批；后续压测稳定后可上调至 10,000~20,000 |
| `CH_LOG_FLUSH_INTERVAL` | `500ms` | 批次未满时 500ms 强制落盘，兼顾延迟与批次大小 |
| `CH_EVENT_BATCH_SIZE` / `CH_EVENT_FLUSH_INTERVAL` | `2000` / `500ms` | events 流量小于 logs，取 2,000 行攒批 |
| `CH_FLUSH_TIMEOUT` | `10s` | 必须 < NSQ MsgTimeout 30s（`consumer.go:87`），留出重试与 Touch 余量 |
| `CH_DRAIN_TIMEOUT` | `20s` | 进程优雅退出（SIGTERM）时强制排空的最长等待时限，超时则批量 Requeue(0) |
| `CH_QUEUE_BUFFER_SIZE` | `100000` | CHAsyncBatcher 一级入队管道容量 |
| `CH_DEDUP_MODE` | `redis`（无 Redis 自动降级 `none`） | 消费者侧落盘后异步 Redis 闸门（§4.7） |
| `NSQ_LOG_CHANNEL_CH` / `NSQ_EVENT_CHANNEL_CH` | `ch-log-consumer` / `ch-event-consumer` | 双写独立 channel 名 |
| `CH_LOG_CONCURRENCY` | `50` | 异步投递模式下，50 个 handler 已能打出 100k+ EPS 内存入队速度 |

理论写入容量估算（**估算**，需压测校准）：2 分片 × 5,000 行/批 ÷ max(500ms 刷新间隔, CH 写入耗时约 80ms) ≈ 20,000 ~ 50,000 EPS；当调至 4 分片 × 10,000 行时，轻松覆盖 100k~200k EPS 目标容量。

背压链路（端到端）：

```
CH 宕机/极慢 → PrepareBatch 耗时暴增/失败 → Flusher 阻塞 → CHAsyncBatcher 内存缓冲区（100k）填满
  → NSQ Handlers 向 channel 投递受阻 → NSQ Client 停止接收数据（MaxInFlight 50k 耗尽）
  → NSQ Daemon 队列堆积（进入磁盘持久化削峰）
  → （若堆积达到阈值 NSQ_DEPTH_ALERT_THRESHOLD 100k）
  → 网关主动对 Ingest 端点返回 503 + Retry-After，保护整个系统不被打崩
```

改进点：现有 AsyncBufferedPublisher 队列满即丢消息返回错误，in ingest handler 映射为 503（客户端可重试）。本期增加两个保护：
- `/debug/metrics`（已存在，`internal/httpserver/httpserver.go:45`）增加 CH 每分片 flush 延迟/失败率/批次大小直方图；
- NSQ depth 超阈值（默认 100k，env `NSQ_DEPTH_ALERT_THRESHOLD`）时网关主动对 ingest 返回 503 + `Retry-After`，把积压挡在内存外（NSQ depth 已有 5s 轮询，`cmd/gateway/main.go:64-66`）。

### 5.4 不再需要的写入侧开销

CH 链路移除以下 PG 特有成本（代码位置供删除/旁路时参考）：
- advisory lock + ingest_id 预查询去重（`internal/store/track_events.go:121-196`）→ 替换为 §4.7 Redis 闸门；
- 副作用表 upsert（`internal/store/metrics_rollup.go:60-91`、`internal/store/track_events.go:207-282`）→ 由 §4.4 MV 替代；
- track_events 拆分逻辑 `TrackEventRowsFromLogs`（`internal/store/track_events.go:16-43`）→ **保留复用**（纯函数，CH writer 同样调用它拆两路）。

---

## 6. 查询链路改造

### 6.1 SearchAdapter 实现

新增 `logtap/internal/search/adapters/clickhouse/clickhouse.go`，实现 `search.SearchAdapter`（`internal/search/types.go:68-82`）：

- `Type()` → `"clickhouse"`；
- `Search()`：`SearchQuery` → CH SQL（映射表见 §6.2）；
- `Index()`：no-op（数据由写入链路落 CH，与 PG 版语义一致，`internal/search/adapters/postgres/postgres.go:40-42`）；
- `DeleteBefore()`：CH 模式下返回"由 TTL 托管"的说明性错误（调用方 cleanup worker 需按后端类型跳过）；
- `Ping()`：`SELECT 1`。

`Filter` 白名单映射（对照 `internal/search/adapters/postgres/postgres.go:204-220` `mapColumn`）：

| DSL 字段 | PG 表达式 | CH 表达式 |
|---|---|---|
| `level` | `level` | `level` |
| `trace_id`/`span_id` | 同名列 | 同名列 |
| `message` | `message` | `message`（contains → `positionCaseInsensitive`） |
| `environment` | `fields->>'environment'` | `fields['environment']` |
| `service` / `tag` | 同上 | 同上（Map 查找） |

装配点：`internal/httpserver/httpserver.go:158-161`（现 `search.NewEngine(searchpostgres.NewAdapter(db))`）改为按 `STORAGE_BACKEND` 选择 adapter；`Pagination` 增加可选 `Cursor` 字段（向后兼容，见 §6.3）。

### 6.2 端点 → SQL 映射

| 端点（handler 位置） | 现状（PG） | CH 实现 | 超时（现状→CH） |
|---|---|---|---|
| `GET /logs/search`（`internal/query/handlers.go:109-207`） | tsquery/ILIKE + `ORDER BY timestamp DESC LIMIT n`，5s | `WHERE tenant_id=? AND project_id=? AND timestamp BETWEEN ? AND ? [AND level=?] [AND trace_id=?] [AND hasToken(message,?) / positionCaseInsensitive(...)] ORDER BY timestamp DESC LIMIT n` | 5s → 5s（CH 侧 `max_execution_time=4`） |
| `GET /search`（`internal/search/handler.go:15-`，engine 装配 `httpserver.go:159-160`） | COUNT + 分页 + level facet 三条 SQL | 同结构：`count()` 估算列存下便宜；facet `GROUP BY level`；翻页用 cursor（§6.3） | 30s（`handler.go:52`）→ 10s |
| `GET /logs/trend`（`handlers.go:213-322`，30 天上限） | `date_trunc` GROUP BY + Top messages，10s | `toStartOfHour(timestamp)` GROUP BY（或读 `log_daily_stats` 按天桶）；Top messages 走 `topK` 或裸 `GROUP BY substring(message,1,80)` | 10s → 10s（实测 P50 8.4s → 预期 <1s，**估算**） |
| `POST /analytics/custom`（`analytics_custom.go:82-167`，180 天上限） | JSON 属性 group by + `COUNT(DISTINCT distinct_id)` | `fields['key']` group by + `uniqExact(distinct_id)`；超过 30 天的按天粒度可走 rollup 表 | 10s → 15s（上限可放宽到 365 天，列存扫描量可控，**估算**） |
| `GET /analytics/events/top` | track_event_daily rollup 或裸表 | 读 `track_event_daily`（`countMerge`/`uniqExactMerge`），缺失窗口回退裸表 `GROUP BY name` | 不变 |
| `GET /analytics/funnel`（`internal/query/event_analysis.go`） | SQL 自连接 | `windowFunnel(window_sec)(timestamp, name='step1', name='step2', ...)` 单遍扫描 | 10s → 15s |
| `GET /metrics/today`、`/metrics/total`（`internal/query/metrics.go:15-82`） | 优先 PG rollup 表，缺失回退全表 COUNT + 跨表 COUNT(DISTINCT)（`metrics_rollup.go:265-340`） | **常规模式**：恒读 `log_daily_stats` rollup：`sum(countMerge)` / `uniqExactMerge`，毫秒级响应，彻底废除全表回退。<br>**精确对账模式 (`exact=1`)**：废除 rollup 对账承诺；强制限定时间跨度 ≤ 24 小时，下推 raw 表 `count(DISTINCT ingest_id)` 与 `uniqExact(distinct_id)` | 2s/10s → 常规 2s / 对账 15s（预期常规毫秒级，**估算**） |
| `GET /events/recent`、`/events/:eventId`（`handlers.go:20-107`） | events 表索引扫 | §4.2 events 表；点查靠 `idx_event_id` bloom 跳索引 | 5s → 5s |
| `GET /storage/estimate` | PG 表大小统计 | `system.tables`/`system.parts` 的 `data_compressed_bytes` | 不变 |
| Redis 系（`/analytics/active`、`/analytics/dist*`、`/analytics/retention`） | Redis recorder | **不变**，继续 Redis（独立子系统，本期不动） | 不变 |

### 6.3 分页

- 现状：`/logs/search` 只有 limit（≤500，`handlers.go:136`），无深分页；`/search` 是 page/pageSize offset 分页（`internal/search/handler.go:37-46`）。
- CH 上 offset 深分页=扫描并丢弃前 N 行，成本随页码线性增长。设计：**保留 offset 分页兼容**（第 1–10 页体验与现状一致），并在 `Pagination` 增加 `cursor`（base64 编码的 `(timestamp, ingest_id)`）：`WHERE (timestamp, ingest_id) < (?, ?) ORDER BY timestamp DESC, ingest_id DESC LIMIT n`，翻页成本恒定。前端后续版本切换。契约测试覆盖"重复翻页不丢不重"（设计稿 §11 已要求）。

### 6.4 超时与并发保护

#### 6.4.1 并发容量挑战：Enterprise 500 RPS vs 单节点 10~100 QPS

根据云端 edge 代码事实（`logtap-cloud/internal/edge/limits.go:44`）：
Enterprise 档位配额为 **QueryRate: 500 RPS、QueryBurst: 1000 RPS**；即使 Business 档位也有 QueryRate: 200 RPS。
然而外部调研事实表明：ClickHouse 单节点的高并发短查询上限仅约 **10~100 QPS**（每个查询内部默认多线程并发吃满 CPU 核数，来源数据）。
若云端生产环境采用单节点承载查询，面对 Enterprise 租户的并发流量或多租户仪表盘集中刷新，CH 会在数秒内耗尽 CPU 线程池并发生排队雪崩；同时网关层若仅设 32 全局信号量，将导致前端出现大面积 HTTP 429 报错，严重影响控制台体验。

为此，必须在架构上构建"短缓存拦截 + 物理读写分离 + 算力限制限流"三道防线：

#### 6.4.2 防线一：网关/Edge 3~5s 短 TTL 聚合缓存（吸收 80%+ 重复请求）

- **痛点**：控制台仪表盘页面打开时，通常前端会有 5~10 个图表组件并发请求同一项目的趋势与概览（`/logs/trend`、`/metrics/today`、`/metrics/total` 等），且多用户可能同时查看同一项目，前端还常伴有 5s 定时刷新。
- **缓存策略**：在网关层（或云端 edge 反代层）对上述高频聚合接口引入基于本地内存（如 go-cache / ristretto）或 Redis 的 **3~5 秒短 TTL 缓存**。
- **Cache Key**：`cache:query:{tenant_id}:{project_id}:{hash(endpoint+query_params)}`。
- **效果**：短 TTL 缓存完全对齐用户对"秒级近实时"的预期，同时将绝大多数并发重复请求与高频刷新就地拦截，直接削减 80%~90% 的下推查询量，使进入 CH 的实际查询 QPS 稳定在几十 QPS 的舒适区间。

#### 6.4.3 防线二：物理读写分离与只读副本横向扩展（只读副本前置为生产基线）

- **云端生产起步即部署 1 写 1 读双副本**（见 §8.1 阶段 B 前置）：
  - 写入流量专走写入主节点 (Pod-0)，查询流量完全由只读副本 (Pod-1) 承载；
  - 消除写入 LSM Compaction 与高并发查询争抢 CPU 和磁盘 IO 的隐患；
- **无状态水平扩容**：当云端 Enterprise 租户增多、聚合缓存穿透后的真实 QPS 突破 100 QPS 时，无需分片扩容，只需向 k8s 集群增加只读副本 Pod（如增加 Pod-2、Pod-3），通过 Service 内建负载均衡将短查询 QPS 近线性推升至 300~500+ QPS。

#### 6.4.4 防线三：单查询吃核限制与分级信号量下推

1. **限制单查询 CPU 占用（提升并发槽位数）**：
   - clickhouse-go 连接参数强制设置 `max_threads = 2`（对交互式搜索）或 `max_threads = 4`（对范围聚合），而非默认占用宿主机全部核数；
   - 牺牲极小单查询耗时（如 3ms 变 6ms），换取单机并发 query slot 上限提升 4~8 倍。
2. **网关应用层分级信号量**：
   - 交互式短查询信号量（`/logs/search`、点查）：默认并发 **64**（env `CH_SHORT_QUERY_MAX_CONCURRENT`），超出排队 1.5s 后返回 429；
   - 重型分析与精确对账信号量（`/analytics/funnel`、`exact=1` 对账）：默认并发 **8**（env `CH_HEAVY_QUERY_MAX_CONCURRENT`），超出排队 3s 后返回 429；
   - 每查询下推 `max_execution_time`（比网关 ctx 超时小 1s）、`max_memory_usage`（默认 8GB）。
3. **CH 服务端工作负载隔离（Workload Scheduling，26.x GA）**：
   - 专用只读用户 `logtap_read`（`SETTINGS readonly = 1`），`max_concurrent_queries_for_user = 128`；
   - 建立两个资源工作负载：
     - `console_workload`（权重 3）：服务交互式检索与仪表盘；
     - `analytics_workload`（权重 1）：服务多维漏斗与长周期自定义分析；
   - 彻底防止后台重型分析打爆前台实时交互查询。

---

## 7. 多租户与隔离

### 7.1 tenant 贯穿（与既定设计稿对齐）

按 `CLOUD_MULTI_TENANCY_STORAGE_DESIGN.md` §3–4 执行，本文不重复设计，只列 CH 相关落点：

- `tenant_id UUID` 全表首列（§4 DDL），开源版固定 `DefaultTenantID`；
- NSQMessage 增加 `tenant_id`（omitempty，设计稿 §4.2 方案 1），CH writer 按消息写入；
- CH adapter 的**每一条 SQL 无条件拼 `tenant_id = ?`**（从 ctx 取，`tenant.MustFrom`）；契约测试增加"两 tenant 同 project_id 同查询互不可见"用例（设计稿 §11 已列）；
- 信任边界不变：只有 `proxy_ok`（`X-Logtap-Proxy-Secret` 校验通过）时才信任 tenant 头（设计稿 §3.3；现有校验代码 `internal/httpserver/httpserver.go:118-147`）。

### 7.2 CH 层配额与 workload（企业版）

- `CREATE QUOTA` 按租户限制单位时间查询量/扫描行数（来源数据）；租户→CH 用户/profile 的映射由企业版分支的 StorageRouter（设计稿 §5.3）下发；
- workload 权重按套餐档位区分（Business 权重 > Free）；
- 数据隔离仍是"共享大表 + 排序键首列"软隔离；硬隔离（独立集群）由 StorageRouter 按 tenant 路由到不同 CH 集群实现（设计稿 §5.3 已预留），本期不做。

### 7.3 开源版边界

开源版：单 tenant（固定 ID）、单 CH 集群、无 quota 下发、无分档保留表。所有 CH 能力（adapter、writer、DDL、双写消费者）本身开源可用；企业版独有的是 tenant 头解析/路由/配额下发。

---

## 8. 部署与运维

### 8.1 k8s 拓扑与生产准入基线

#### 8.1.1 阶段 A（单节点：仅限本地开发、测试与开源自建试用）

- **拓扑**：StatefulSet 1 副本 + 单块 NVMe PVC，规格 **16C / 64GB / 1TB**；容器镜像 `clickhouse/clickhouse-server:26.3`（LTS）。
- **灾难恢复客观指标（灾难风险量化）**：
  - **RPO = 最大 24 小时**：单节点无任何实时数据副本。若底层物理宿主机故障或 NVMe 介质损坏，数据只能回退至最近一次由 `clickhouse-backup` 备份到 S3 的增量快照，意味着**面临最长达 24 小时的数据丢失窗口**；
  - **RTO = 小时级（1~4 小时）**：单节点彻底损毁后，需要重新调度创建 Pod、重新挂载存储并从远程对象存储拉取数百 GB 备份数据解包校验并启动实例，无法提供分钟级业务自愈。
- **核心生产准入禁令**：**承载商业化核心 SLA 的云端多租户生产环境严禁停留在阶段 A！** 阶段 A 仅作为开源社区单机尝鲜、本地联调或允许数据丢失的非关键日志归档场景。

#### 8.1.2 阶段 B（云端生产基线：1 分片 2 副本 + 3 Keeper，Day 1 生产硬性准入）

为解决 Enterprise 500 RPS 查询冲击并确保云端多租户数据资产安全，**云端生产上线即以阶段 B 作为最低准入基线**（彻底从原"写入>100k 才上"的迟滞规划中剥离）：

- **拓扑**：引入 Altinity clickhouse-operator（v0.27.x，来源数据），部署 **1 分片 × 2 副本（跨可用区 Multi-AZ）+ 3 节点 ClickHouse Keeper**；
- **表引擎**：全面采用 `ReplicatedMergeTree` 系列引擎（`ReplicatedMergeTree`、`ReplicatedAggregatingMergeTree`），DDL 统一下发到集群；
- **物理读写分离拓扑**：
  - `clickhouse-write` Service：指向主写入副本（Pod-0），仅供数据面 `CHAsyncBatcher` 写入；
  - `clickhouse-read` Service：指向只读副本（Pod-1），专供网关 `SearchAdapter` 执行查询，配合网关 3~5s 聚合缓存与单查询限核（§6.4）；
- **生产级高可用指标**：
  - **RPO = 0**：Keeper 强一致事务保证，副本间通过内部二进制协议近实时同步数据块，单机物理损毁零数据丢失；
  - **RTO < 30 秒**：任一节点宕机时，K8s Service 自动切断流量，Keeper 秒级重新选主，读写流量在 30 秒内恢复；
- **双重备份与安全停机**：
  - 在双副本高可用之上，继续保留每日 `clickhouse-backup` 增量归档至 S3（保留 14 天），用于防御人为误操作与勒索灾难；
  - **优雅停机参数配置**：网关 Deployment（`deploy/k8s/prod.yaml`）必须显式配置 `terminationGracePeriodSeconds: 60`（对比默认 30s），确保 Pod 驱逐或滚动更新时，`CHAsyncBatcher` 拥有足够时限完成 20s 强制排空（§5.2.2 Drain Protocol）并完成流量反注册。

#### 8.1.3 阶段 C（规模扩展：只读副本横向弹性 + S3 冷热分层）

- **只读并发线性扩展**：当企业租户与控制台并发查询量持续上涨突破 200 QPS 时，无需拆分数据分片，直接水平扩展只读副本 Pod（增加 Pod-2、Pod-3），无缝将集群查询能力推升至 500~1,000 QPS；
- **S3 冷热分层存储（保留期 >90 天或数据量 >5TB 时）**：
  - 本地 NVMe 磁盘作为热层（保留最近 14~30 天日志并充当读缓存）；
  - 配置 ClickHouse Storage Policy 挂载 S3/对象存储卷，设置 `perform_ttl_move_on_insert = false`（来源数据）；
  - 超过热期的数据由 MergeTree 后台自动平滑 move 至对象存储，存储每 GB 成本再降 70%~80%。

### 8.2 升级与变更纪律

- 只升 LTS 版本、禁跨大版本升级（来源数据）；先升只读副本（阶段 B 后）再升写主；
- DDL 变更全部走 `internal/store/clickhouse/migrations/` 顺序迁移文件，启动时幂等执行（对齐现有 `internal/migrate/migrate.go` 的 AutoMigrate 习惯）；
- MV 变更需先 `DETACH` 再重建，禁止直接改 SELECT（DDL 刚性，来源数据）。

### 8.3 监控

| 指标 | 来源 | 告警阈值（初始值，可调） |
|---|---|---|
| NSQ topic depth（logs/events，按 channel 拆分） | 现有 5s depth 轮询（`cmd/gateway/main.go:64-66`） | ch channel depth > 100k 持续 5min |
| CH flush p95/失败率/批大小（每分片） | `/debug/metrics` 新增直方图 | p95 > 5s 或失败率 > 1% |
| CH 查询延迟/并发/内存 | `system.query_log` 定时汇总 + CH Prometheus endpoint | p95 超 §1.2 目标 |
| part 数量/合并压力 | `system.parts`、`system.merges` | parts per partition > 300（攒批失效信号） |
| 磁盘水位 | node exporter | > 75%（TTL 之外需扩容/降保留） |
| 主从延迟（阶段 B） | `system.replicas` | `absolute_delay` > 60s |

### 8.4 容量与成本模型（估算）

以 `CLOUD_MULTI_TENANCY_STORAGE_DESIGN.md` §2.2 档位为输入：

- 规模假设（**估算**）：100 个 Business 租户 × 1 亿条/月 = 100 亿条/月 ≈ 平均 3.9k EPS、峰值 39k EPS → 阶段 A 单节点足够（目标 100k EPS）。
- 存储：单条压缩后 0.5–2KB（设计稿 §2.2 假设）取 1KB，CH 压缩比按保守 5x（来源区间 10x，取保守）→ 100 亿条/月 ≈ 10TB 原始 ≈ 2TB/月压缩后；30 天保留 ≈ 2TB + rollup/索引开销（+20%，估算）≈ 2.4TB → 阶段 A 需把 1TB 盘升到 4TB 或开 S3 分层（阶段 C）。90 天保留场景必须 S3 分层。
- 成本对照（**估算**，同规格云主机+云盘）：PG 方案存 10TB 原始数据需行存+索引约 15–20TB 高性能盘；CH 方案 2.4TB NVMe +（可选）S3。存储成本下降约 **6–8 倍**，与 §1.2 目标一致。
- 计算成本：CH 单节点 16C/64G 对 PG 16C/64G 同规格打平；查询并发扩展（阶段 B 只读副本）按 QPS 增长线性加副本。

---

## 9. 分阶段迁移

### Phase 0：抽象与开关（行为不变）

- 引入 `STORAGE_BACKEND=postgres|dual|clickhouse` 及 §5.3 配置项（`internal/config/config.go` 扩展）；
- `internal/httpserver/httpserver.go:158-161` 的 search engine 装配改为工厂按后端选择；
- 落地 `IngestStore`/`QueryStore` 接口（设计稿 §5.4 签名），PG 实现先包一层现状代码（纯重构，无行为变化）；
- NSQMessage 增加 `tenant_id` omitempty 字段（设计稿 Phase 0/1）。

出口标准：`go test ./...` 全绿 + 契约测试对 PG 后端通过。

### Phase 1：CH 就绪 + 双写（读全在 PG）

- 部署生产基线阶段 B 的 CH（1 分片 2 副本 + 3 Keeper）；启动时跑 §4 DDL 迁移；
- 挂载 `ch-log-consumer` / `ch-event-consumer` channel（§5.2），`STORAGE_BACKEND=dual`；
- 观察：NSQ depth、CH flush 指标、part 合并压力，跑 ≥7 天覆盖完整业务周期；
- **历史回填安全规范（严禁击穿生产 PG 连接池）**：
  - **背景风险**：生产 k8s 配置中 `deploy/k8s/prod.yaml:45` 的 `DB_MAX_OPEN_CONNS: "10"`，生产连接池极小。若直接使用 ClickHouse `postgresql()` 表函数对主库执行全量拉取，CH 默认的高并发连接会瞬间吃满这仅有的 10 个连接，造成线上业务连接耗尽（`too many clients`）并报 503 崩溃。
  - **回填准则与防击穿红线**：
    1. **通道隔离**：严禁直接连接业务 PG 主库拉取！优先通过 **PG 独立只读备库 (Read Replica)** 进行回填；
    2. **受控的管道脚本拉取（若无备库）**：采用专用离线迁移脚本（Go / Python）单向管道同步，强制执行：
       - **连接数锁定**：全局仅允许占用 **1 个 PG 连接 (`pool_size=1`)**；
       - **细粒度切片**：按 1 小时或更小时间切片拉取，单批限制 2,000 ~ 5,000 行；
       - **强制节流**：每批写入 CH 成功后强制 `time.Sleep(300ms ~ 500ms)`，平滑 CPU 与 IO 开销；
       - **低峰窗口限制**：仅限在业务低谷时段（凌晨 02:00 ~ 05:00 UTC）执行；
       - **实时探针与动态熔断**：脚本每批执行前查询 PG `pg_stat_activity`，若发现当前生产活跃连接数 > 5（占满连接池 50%）或锁等待增多，立即主动挂起 60 秒冷却，绝不与生产业务争夺连接池。

出口标准：CH 写入 p95 < 500ms、双写两侧行数差 < 0.1%（对账时限定 ≤24h 短时间窗口，直接下推 raw 表 `count(DISTINCT ingest_id)` 验证，§4.7）。

### Phase 2：灰度切读

- 按项目灰度：`QUERY_BACKEND_PROJECTS`（项目 ID 列表）或 `QUERY_BACKEND_PERCENT`（按 project_id 哈希百分比）；
- 顺序：`/metrics/*` → `/logs/trend` → `/analytics/*` → `/logs/search`、`/search`（检索语义差异最大，放最后）；
- 每步对比两侧结果（计数类要求一致；检索类允许分词差异，需产品确认召回可接受）；
- PG 写入**继续**（回滚保障）。

### Phase 3：PG 停写数据表 + 最终切流通行（用户决策定稿）

#### 9.3.1 准入门槛（必须全部满足）
1. **灰度周期达标**：Phase 2 灰度切读在生产环境持续平稳运行 **≥ 2~4 周**，覆盖月底账单与突发流量周期；
2. **对账结果通过**：基于 raw 表短时间窗（`exact=1`）抽样对账，PG 与 CH 连续 14 天数据差异率稳定在 `< 0.05%` 阈值内；
3. **副作用链路无缝接管验证（防断流硬性红线）**：
   - 确认 `CHAsyncBatcher` 旁路调用的 `recorder.ObserveLog` / `ObserveEventDist`（驱动 `/analytics/active` 等 Redis 指标）已在双写期间完成联调，Redis 指标口径与 PG consumer 生产输出严格一致；
   - 确认 `evaluator.Submit(alert.InputFromLog(r))` 已挂载至 CH 批次落盘成功回调，并在测试环境中完成告警触发与抑制演练，杜绝停写 PG 后实时日志告警出现静默漏报；
4. **演练与审批**：完成一次离线回滚流程演练，运维与产品负责人双签字批准。

#### 9.3.2 停写与切换执行
- `STORAGE_BACKEND=clickhouse`：PG 数据面消费者下线（`log-consumer` 关闭；`events` 消费者关闭，告警评估与指标打点完全由 `CHAsyncBatcher` 独立承接）；
- PG 中历史明细数据（`logs`/`events`/`track_events`）不再执行写入，按原有 `CleanupPolicy` 自然老化过期删除释放物理磁盘；PG 永久保留核心元数据表（用户/项目/Key/告警/监控定义）；
- 企业版套餐分档分表路由（`logs_7d`/`logs_30d`/`logs_90d`，§4.6）全面正式接管写入与 TTL 生命周期。

### 回滚预案与空洞管理（用户决策选项 A 定稿）

用户已正式拍板批准**选项 A：接受切流窗口期 PG 的检索空洞，不投入研发 CH→PG 反向补水工具**。

#### 决策论证与背景
- **PG 写入能力物理受限**：生产实测 PG 写入拐点约 7,463 EPS（`docs/perf/README.md`），且生产连接池仅配 10（`deploy/k8s/prod.yaml:45`）。若在回滚时尝试将 CH 沉淀的海量日志反向灌回 PG，高并发反灌会瞬间冲垮 PG，引发生产级级联雪崩（连接耗尽、503 宕机）；
- **性价比最优**：Phase 1 双写与 Phase 2 灰度提供了极长的安全性验证缓冲，Phase 3 之后发生回滚的概率极低；接受空洞并保留 CH 只读视图是业界成熟的风险对冲策略。

#### 极端回滚处置规程
1. **阶段 1/2 读回滚（零风险）**：直接调整 `QUERY_BACKEND_*` 切回 PG，PG 一直在双写，数据 100% 完整无损。
2. **阶段 3 极端故障回滚流程**：
   - **秒级止血**：立即修改配置 `STORAGE_BACKEND=dual`，重新拉起 PG 消费者恢复双写，并将所有查询路由切回 PG，确保新产生的业务日志立刻恢复入库；
   - **空洞期界定**：将"PG 停写"至"恢复双写"之间的切流窗口（通常几小时到几天）定义为**历史检索空洞期**；该区间数据明确记录为"仅 ClickHouse 存在，PG 不进行在线反向补水"；
   - **租户通告与免责**：控制台发出维护通告，明确告知租户在空洞期区间的历史日志查询需切换至历史归档视图；同时提示依赖 PG 明细表的告警规则在空洞期区间可能存在漏报风险；
   - **只读归档通道保障**：ClickHouse 集群保持只读挂载状态**至少一个完整生命周期（30~90 天）**，数据面在控制台保留"空洞期历史只读查询"备用接口；
   - **离线应急补水 Runbook（无 SLA 兜底）**：仅提供一份运维离线 Runbook，支持将指定重要项目在空洞期的数据通过 S3 Parquet/CSV 导出，再通过单连接、严格限速管道脚本（<500 EPS，凌晨低峰执行）导入 PG，专供极其特殊的大客户合规或纠纷对账申诉，不作为全量在线恢复承诺。

### 压测验收（每个 Phase 出口必跑）

复用现有 k6 脚本（`docs/perf/k6_ingest_and_query_1m.js`），目标：写入 ≥50k EPS 时 ingest p95 < 50ms、CH flush 无连续失败；查询 20 并发下 `/logs/search` p95 < 1s、`/logs/trend` p95 < 3s（对照 `docs/perf/README.md` 现状基线）。

---

## 10. 开源版与企业版拆分

| 内容 | 归属 | 说明 |
|---|---|---|
| `internal/store/clickhouse/*`（client/writer/migrations/ingest_store/query_store） | 开源 | 通用数据面能力 |
| `internal/search/adapters/clickhouse/*` | 开源 | `SearchAdapter` 第二实现 |
| `internal/consumer/ch_consumer.go`、`STORAGE_BACKEND` 及 CH_* 配置 | 开源 | |
| `deploy/docker-compose.yml` 增加 clickhouse 服务、`deploy/k8s/clickhouse.yaml`（阶段 A StatefulSet） | 开源 | 通用部署件 |
| NSQMessage `tenant_id` 字段（omitempty，恒为 DefaultTenantID） | 开源 | 设计稿 §4.2 方案 1 |
| tenant 头解析中间件、StorageRouter 多集群路由、CH quota/workload 按租户下发、分档保留表路由 | 企业版分支 | 设计稿 §8.2/§9；不进开源主线 |
| 本文档 | 开源 | 不含任何内部信息 |

开源同步纪律：上述开源件进主线（GitHub 镜像 fast-forward only），企业版件留在企业分支/`logtap-cloud`，遵守 AGENTS.md 的双 edition 规则。

---

## 11. 风险与开放问题

### 风险与缓解对策

1. **写入微批与 Too Many Parts 崩溃风险（已通过架构重构彻底化解）**：
   - *风险*：原有 PG 消费者 `Batcher.Add`（`batcher.go:67-86`）要求调用者同步等待 flush 完成，并发数上限由 handler 并发度（50~200）锁定。在期望批大小为 20k 时，批次永远无法凑满，只能由 1s 超时触发，退化为每秒仅写入几十条的极小微批，引发 ClickHouse 产生海量微小 parts 并触发 `Too many parts in all data parts in table` 崩溃拒绝写入。
   - *缓解*：§5.2 全新设计 `CHAsyncBatcher` 异步内存缓冲池（100k 容量），handler 投递后即刻释放；调大 `NSQ_MAX_IN_FLIGHT_CH = 50,000`；起步批大小设为稳妥的 5,000 行并限制单/双分片运行；引入后台 Touch() 机制防超时重投递，从机制上根除微批隐患。
2. **并发容量倒挂与 Enterprise 500 RPS 冲击风险（已通过双副本基线+短缓存化解）**：
   - *风险*：云端 Enterprise 档限流高达 `QueryRate: 500 RPS`、`QueryBurst: 1000 RPS`（`limits.go:44`），而 ClickHouse 单节点短查询上限仅约 10~100 QPS，面对多租户仪表盘集中刷新单机必崩，网关并发信号量 32 也会引发大量 429。
   - *缓解*：将只读副本前置为云端生产起步基线（§8.1 阶段 B，1 写 1 读双副本），物理隔离读写；网关/Edge 层对高频仪表盘接口增加 3~5s 短 TTL 缓存（§6.4.2），吸收 80%+ 的前端重复聚合请求；单查询限制 `max_threads = 2~4` 释放更多并发 slots，保障整体集群稳健支撑 500+ QPS 并发。
3. **Rollup 物化视图重试误差与精确对账冲突（已纠正自相矛盾承诺）**：
   - *风险*：MV（`MATERIALIZED VIEW`）是随 raw 表写入流式同步触发的。当网络闪断引发 NSQ 消息重投递时，MV 会重复累加计数，且由于存储的是聚合物化状态（`countState` / `uniqExactState`），物理上无法后验回溯剔除重试数据。声称 rollup 支持 exact=1 是自相矛盾的。
   - *缓解*：在 §4.4 明确废除在 Rollup 表上支持 `exact=1` 的设计，坦承其存在极低概率的统计误差（误差率 ≈ NSQ 重投递率 < 0.05%）；将 `exact=1` 严格限定为**必须携带 ≤24h 短时间窗口且直接下推 raw 原始明细表**（§4.7），受独立信号量与 15s 超时保护，专供财务与审计对账。
4. **单节点硬件损毁时的灾难恢复风险（已客观量化并下达生产禁令）**：
   - *风险*：阶段 A 单节点架构依赖每日一次 S3 增量快照，若 NVMe 硬件发生物理坏道或宿主机彻底损毁，面临长达 24 小时的数据丢失风险，且恢复耗时达小时级。
   - *缓解*：§8.1 客观量化阶段 A 指标为 **RPO 最大 24h、RTO 小时级（1~4h）**，并设立硬性生产准入红线：**承载商业化核心 SLA 的云端多租户生产环境严禁停留在阶段 A！上线最低基线必须为阶段 B（1 分片 2 副本 + 3 Keeper，Multi-AZ）**，实现 RPO=0、RTO<30s。
5. **历史回填击穿生产 PG 连接池风险（已制定严格安全规范）**：
   - *风险*：生产 PG 连接池默认配置极小（`deploy/k8s/prod.yaml:45` `DB_MAX_OPEN_CONNS: "10"`）。若在数据迁移时使用 ClickHouse `postgresql()` 表函数对主库并发拉取，会瞬间占满仅有的 10 个连接，导致生产应用报 `too many clients` 崩溃（HTTP 503）。
   - *缓解*：§9 Phase 1 增补严格的回填准则：严禁直连线上生产主库，强制优先使用 PG 独立只读备库；无备库时强制使用专用离线管道脚本，单连接锁定（`pool_size=1`）、小时间片分段拉取（2,000~5,000 行）、强制每批 sleep 300ms 节流、仅限凌晨低峰期执行，并具备 `pg_stat_activity` 探针动态熔断保护。
6. **MV 无回填 + DDL 刚性**（§4.4）：rollup 重建需要停 MV → 手动 `INSERT SELECT` 回补。缓解：运维 runbook + 降级为定时增量 INSERT 的备选方案。
7. **检索语义差异**：jieba 分词 vs PG 'simple'/trgm 的召回差异；无 BM25（现有 API 不按相关性排序，影响小）。需在 Phase 2 灰度中用真实查询对比确认。
8. **NSQ 单点**：CH 宕机由 NSQ 磁盘队列缓冲（小时级窗口内安全）；NSQ 自身宕机则 ingest 503。与现状一致，未恶化，但写入目标提高后该单点更值钱——见开放问题 3。
9. **text 索引版本依赖**：`text(tokenizer='jieba')` 需要 26.2+（GA，来源数据）；部署锁死 26.3 LTS 即满足。若需回退到旧版 CH，则降级为 `tokenbf_v1` + `hasToken`（英文 OK、中文退化为按字/bigram），DDL 需变。

### 开放问题（遗留次要验证项）

用户对核心决策（套餐档位固定保留 §4.6、微量容错与短窗口精确对账 §4.7、回滚接受检索空洞与只读通道保障 §9.3）均已拍板定稿。当前仅遗留以下次要工程验证点：

1. **`/events/:eventId` 无时间窗点查的压测验证**（§4.2）：bloom filter 跳索引在数亿级历史数据下的点查延迟是否稳定在 50ms 以内，需在 Phase 1 灰度期用真实数据压测检验；若延迟不达标，控制台/API 需提示前端带上粗略的 `start` 时间范围 hint 以实现精确分区剪枝。
2. **`mode=contains` 是否追加 ngrambf 索引**（§4.5）：v1 默认基于列存高速扫描（相比 PG 行存已提升数十倍）；若灰度期发现包含中英混合子串的 contains 查询占比超过 20% 且 P95 延迟升高，再通过在线 DDL 追加 `ngrambf_v1(3, 65536, 4, 0)` 跳索引。
3. **NSQ → 分布式流（Redpanda/Kafka）的后续演进时点**（§5.1）：本期经由 `CHAsyncBatcher` 异步缓冲与 50,000 MaxInFlight 优化后，单机 NSQ 足以平稳支撑 100k EPS 阶段目标；后续仅在业务出现跨可用区多副本消息重放或异地容灾诉求时，再启动队列层中间件升级。

---

## 附录 A：关键代码位置索引（改造点 → 现状文件）

| 改造点 | 现状文件 |
|---|---|
| SearchAdapter 接口 / 第二实现挂载 | `internal/search/types.go:66-82`、`internal/search/adapters/postgres/postgres.go`、`internal/httpserver/httpserver.go:158-161` |
| 统一搜索 handler（分页/超时） | `internal/search/handler.go:15-` |
| NSQ consumer 装配 | `cmd/gateway/main.go:189-203`、`internal/consumer/consumer.go:48-103` |
| 攒批器（重构为 CHAsyncBatcher） | 原同步实现 `internal/consumer/batcher.go:67-86`（仅保留给 PG）；CH 专属异步实现 `internal/store/clickhouse/async_batcher.go` |
| PG 写入事务与去重（CH 链路移除/旁路） | `internal/store/track_events.go:68-282`、`internal/store/batch.go:15-` |
| 副作用 rollup upsert（由 MV 替代） | `internal/store/metrics_rollup.go:60-91` |
| track_events 派生（CH 复用） | `internal/store/track_events.go:16-43` |
| 数据模型（列映射依据） | `internal/model/models.go:41-151` |
| GIN/FTS/trgm 索引与 hypertable（PG 保留，元数据继续用） | `internal/migrate/migrate.go:78-136` |
| 查询端点与并发限流 | `internal/query/handlers.go`、`internal/query/analytics_custom.go`、`internal/query/metrics.go`、`internal/query/event_analysis.go`、`logtap-cloud/internal/edge/limits.go:44` |
| 配置项扩展 | `internal/config/config.go:136-284`（新增 NSQ_MAX_IN_FLIGHT_CH=50000、CH_*） |
| 部署件与生产基线 | `deploy/docker-compose.yml`、`deploy/k8s/prod.yaml`（新增 `clickhouse.yaml` 1 分片 2 副本 + 3 Keeper） |
| tenant 设计依据 | `docs/CLOUD_MULTI_TENANCY_STORAGE_DESIGN.md` |
| 现状性能基线 | `docs/perf/README.md` |
