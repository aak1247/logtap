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

