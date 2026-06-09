# 性能压测（k6）

目标：提供一套可重复的写入压测脚本，用于比较优化前/后的吞吐与延迟。

## 依赖

- 安装 `k6`：https://k6.io/docs/get-started/installation/

## 变量

脚本使用环境变量：

- `BASE_URL`：服务地址（例如 `http://127.0.0.1:8080`）
- `PROJECT_ID`：项目 ID（例如 `1`）
- `PROJECT_KEY`：项目 key（用于 `X-Project-Key`）
- `BATCH_SIZE`：每个请求包含的条数（默认 `50`）
- `SLEEP_MS`：每次请求后的 sleep（默认 `0`）

k6 通用参数（示例）：
- `--vus 50 --duration 2m`

## 写入压测：logs

```
BASE_URL=http://127.0.0.1:8080 \
PROJECT_ID=1 \
PROJECT_KEY=pk_xxx \
BATCH_SIZE=50 \
k6 run --vus 50 --duration 2m docs/perf/k6_ingest_logs.js
```

## 写入压测：track

```
BASE_URL=http://127.0.0.1:8080 \
PROJECT_ID=1 \
PROJECT_KEY=pk_xxx \
BATCH_SIZE=50 \
k6 run --vus 50 --duration 2m docs/perf/k6_ingest_track.js
```

## 建议记录

- 网关：`p95/p99`、错误率（202/5xx）、CPU/内存
- NSQ：topic depth、重试率（超时/重投递）
- DB：insert 延迟、WAL、锁等待、CPU/IO
- Redis（如启用）：命令耗时与内存曲线


## 100 万日志 + 常用查询混合压测

脚本：`docs/perf/k6_ingest_and_query_1m.js`

覆盖两类负载：

- 写入：默认向 `/api/:projectId/logs/` 灌入 `1,000,000` 条日志，`INCLUDE_TRACK=true` 时额外按 10% 比例写 `/track/`。
- 查询：并发轮询常用查询接口，包括 `metrics/today`、`metrics/total`、`logs/search`、事件 top、用户增长、漏斗、自定义分析、存储估算、schema、cleanup、analysis views、alerts、monitors。

常用变量：

- `BASE_URL`：服务地址，例如 `http://<247-server>:8080`
- `PROJECT_ID`：项目 ID
- `PROJECT_KEY`：写入用项目 key；如果通过内网代理密钥访问可不填
- `AUTH_TOKEN`：查询用用户 JWT；如果用 `PROXY_SECRET` 可不填
- `PROXY_SECRET`：对应服务端 `LOGTAP_PROXY_SECRET`，同时可绕过写入 key 和查询用户 token
- `TARGET_LOGS`：写入日志总数，默认 `1000000`
- `BATCH_SIZE`：每个写入请求的日志条数，默认 `100`
- `INGEST_VUS`：写入并发，默认 `50`
- `QUERY_VUS`：查询并发，默认 `20`
- `QUERY_DURATION`：查询持续时间，默认 `5m`
- `RUN_ID`：压测批次标记，默认当前时间戳
- `INCLUDE_TRACK`：是否额外压测 `/track/`，默认 `false`
- `INCLUDE_REDIS_ANALYTICS`：是否额外压测依赖 Redis recorder 的 `active/dist/retention`，默认 `false`

示例：

```
BASE_URL=http://<247-server>:8080 \
PROJECT_ID=1 \
PROJECT_KEY=pk_xxx \
AUTH_TOKEN=eyJxxx \
TARGET_LOGS=1000000 \
BATCH_SIZE=100 \
INGEST_VUS=50 \
QUERY_VUS=20 \
QUERY_DURATION=10m \
k6 run docs/perf/k6_ingest_and_query_1m.js
```

如果 247 上的数据面启用了 `LOGTAP_PROXY_SECRET`，推荐直接用代理密钥跑，避免额外准备用户 token：

```
BASE_URL=http://<247-server>:8080 \
PROJECT_ID=1 \
PROXY_SECRET=proxy_secret_xxx \
TARGET_LOGS=1000000 \
k6 run docs/perf/k6_ingest_and_query_1m.js
```

建议记录：

- k6：`http_req_duration p95/p99`、`http_req_failed`、按 `endpoint` tag 分组的慢接口。
- 服务端：CPU、内存、goroutine、`/debug/metrics`、错误日志。
- DB：CPU/IO、慢 SQL、锁等待、索引命中、表/索引膨胀。
- 队列/消费者：NSQ topic depth、requeue、consumer 写入延迟；确认 100 万条最终入库后再看 rollup 统计。
