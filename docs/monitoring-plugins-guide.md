# 监控插件使用指南（Detector Plugins）

本指南基于现有监控插件架构设计文档：

- 架构：`docs/monitoring-plugin-design.md`
- 插件/Monitor/Detector API 设计：`docs/specs/alert-signal-detector-v1-design.md`、`v2-design.md`

只聚焦于**内置监控插件**的实际使用，帮助你在 Alerts → Monitor 页面快速配置监控项并联动告警。

## 1. 基本概念回顾

- **Detector 插件（DetectorPlugin）**
  - 以 Go 插件形式实现，负责执行一次检测并产出一批统一结构的 `Signal`。
  - 插件类型由 `Type() string` 决定，例如：`log_basic`、`http_check`、`tcp_check`、`metric_threshold`。
- **Monitor（监控项）**
  - 数据表：`monitor_definitions`。
  - 包含：`name / detectorType / config / intervalSec / timeoutMs / enabled / nextRunAt` 等。
  - Monitor Worker 周期性扫描 due 的监控项，通过 `detectorType` 找到对应插件执行一次检测，并记录到 `monitor_runs`。
- **AlertRule（告警规则）**
  - 从 `Signal` 转成告警事件并下发通知。
  - 对监控插件来说，最常用的匹配条件是：
    - `fields.source_type == "<plugin_type>"`；
    - `level` / `severity` 包含 `error`。

API 关键路径：

- 查询插件列表：`GET /api/plugins/detectors`
- 查询插件配置 Schema：`GET /api/plugins/detectors/:type/schema`
- 管理监控项：`POST/GET/PUT/DELETE /api/:projectId/monitors`
- 手动触发一次：`POST /api/:projectId/monitors/:monitorId/run`
- 同步试运行：`POST /api/:projectId/monitors/:monitorId/test`

## 2. 内置插件一览

目前 gateway 启动时静态注册的插件：

| 插件类型             | 说明                         | 典型场景                         |
|----------------------|------------------------------|----------------------------------|
| `log_basic`          | 日志/事件桥接插件            | 现有日志告警、心跳类 Monitor     |
| `http_check`         | HTTP/HTTPS 可用性 + 内容检查 | 网站/健康检查接口 Uptime 监控   |
| `tcp_check`          | TCP 端口连通性检查           | DB/Redis/自建服务端口监控       |
| `metric_threshold`   | 简单数值阈值判断             | 自定义数值指标的阈值告警（高级）|

下面分别介绍三个新的监控插件。

---

## 3. HTTP Uptime：`http_check`

### 3.1 能力与语义

- 通过 HTTP/HTTPS 请求检查目标 URL 是否可用：
  - 状态码是否在期望列表中；
  - 是否返回 2xx；
  - 可选：响应体是否包含某个关键字。
- 每次执行产生 1 条 `Signal`：
  - 成功：`severity=info`、`status=resolved`，消息 `"http_check ok"`；
  - 失败/异常：`severity=error`、`status=firing`，消息中包含失败原因（网络错误/状态码不符合/内容不匹配）。
- `SourceType` 固定为 `"http_check"`，便于告警规则匹配。

### 3.2 配置字段

`ConfigSchema()` 的字段（简化描述）：

- `url`（string，必填）
  - 必须是 `http` 或 `https` 的合法 URL。
- `method`（string，可选）
  - HTTP 方法，默认 `GET`。
- `headers`（object，可选）
  - 额外请求头：`{"Header-Name": "value"}`。
- `body`（string，可选）
  - 请求体内容，常用于 `POST` 检查。
- `expectStatus`（int[]，可选）
  - 允许的状态码列表，例如 `[200,201]`；如果不配置，则“2xx 为成功，非 2xx 视为失败”。
- `expectBodySubstring`（string，可选）
  - 要求响应体包含的关键字，例如 `"OK"`。
- `timeoutMs`（int，可选）
  - 单次请求的超时时间，单位毫秒，默认 5000ms，上下限在 schema 中约束（100–60000）。
- `minTlsValidDays`（int，可选）
  - 仅对 HTTPS 有效。配置为大于 0 的整数时，如果证书剩余有效天数小于该值，会把本次检查视为失败（`severity=error`，`status=firing`），并在 `fields` 中写入 `cert_not_after`、`cert_days_left`、`cert_expiring_soon=true`。

### 3.3 示例：创建 HTTP 监控项

```jsonc
POST /api/123/monitors
Authorization: Bearer <token>

{
  "name": "homepage-uptime",
  "detectorType": "http_check",
  "config": {
    "url": "https://example.com/healthz",
    "method": "GET",
    "expectStatus": [200],
    "expectBodySubstring": "OK",
    "timeoutMs": 3000
  },
  "intervalSec": 30,
  "timeoutMs": 5000,
  "enabled": true
}
```

### 3.4 示例：告警规则（http_check 失败通知）

以下是通过 HTTP API 创建规则的大致结构（字段以实际实现为准）：

```jsonc
POST /api/123/alerts/rules
Authorization: Bearer <token>

{
  "name": "http-check-failed",
  "enabled": true,
  "source": "logs",         // 沿用现有告警引擎的 Source 枚举
  "match": {
    "levels": ["error"],
    "fieldsAll": [
      { "path": "source_type", "op": "eq", "value": "http_check" }
    ]
  },
  "repeat": {
    "windowSec": 60,
    "threshold": 1,
    "baseBackoffSec": 60,
    "maxBackoffSec": 3600
  },
  "targets": {
    "webhookEndpointIds": [1]
  }
}
```

逻辑含义：

- 匹配所有 `source_type = http_check` 且 `level = error` 的信号；
- 在 60 秒窗口内至少 1 次违反时触发一次通知；
- 通知发往 ID 为 1 的 Webhook endpoint 或其他渠道。

---

## 4. TCP 端口检查：`tcp_check`

### 4.1 能力与语义

- 使用 TCP 建连检查某个 `host:port` 是否可连通：
  - 成功：端口可连接，视为健康；
  - 失败：连接超时、拒绝连接、DNS 失败等视为异常。
- 每次执行产生 1 条 `Signal`：
  - 成功：`severity=info`、`status=resolved`、`message="tcp_check ok"`；
  - 失败：`severity=error`、`status=firing`、`message="tcp_check failed: ..."`。
- `SourceType` 固定为 `"tcp_check"`。

### 4.2 配置字段

- `host`（string，必填）
  - 目标主机，例如 `"127.0.0.1"` 或域名。
- `port`（int，必填）
  - 1–65535，例如 `5432`、`6379`。
- `timeoutMs`（int，可选）
  - 建连超时，默认 5000ms。

### 4.3 示例：创建 TCP 监控项

```jsonc
POST /api/123/monitors
Authorization: Bearer <token>

{
  "name": "postgres-tcp",
  "detectorType": "tcp_check",
  "config": {
    "host": "10.0.0.5",
    "port": 5432,
    "timeoutMs": 2000
  },
  "intervalSec": 30,
  "timeoutMs": 5000,
  "enabled": true
}
```

### 4.4 示例：告警规则（tcp_check 失败通知）

```jsonc
POST /api/123/alerts/rules

{
  "name": "tcp-check-failed",
  "enabled": true,
  "source": "logs",
  "match": {
    "levels": ["error"],
    "fieldsAll": [
      { "path": "source_type", "op": "eq", "value": "tcp_check" }
    ]
  },
  "repeat": {
    "windowSec": 60,
    "threshold": 1,
    "baseBackoffSec": 60,
    "maxBackoffSec": 3600
  },
  "targets": {
    "webhookEndpointIds": [1]
  }
}
```

---

## 5. 数值阈值：`metric_threshold`

> 说明：`metric_threshold` 相比前两种更偏“高级/内部”用法，目前 Monitor Worker 在执行监控时默认只给插件传入 `monitor_id` / `monitor_name` 这两个字段作为 `Payload`，因此如果直接用 Monitor 跑 `metric_threshold`，会因为找不到数值字段而返回配置/执行错误。
>
> 更典型的用途是：
> - 在日志/事件处理链路中直接调用 Detector Service；或
> - 在未来的 metrics 管道中将最新数值打包进 `Payload`，再交给插件做判断。

### 5.1 能力与语义

- 从 `ExecuteRequest.Payload` 中读取某个数值字段，根据配置的阈值条件判断是否“违反”：
  - 常见比较符：`> < >= <=`；
  - 区间判断：`between`（不在区间内视为违反）。
- 始终返回 1 条 `Signal`：
  - 未违反：`severity=info`、`status=resolved`；
  - 违反：`severity=severityOnViolation`（默认 `error`）、`status=firing`。
- `SourceType` 固定为 `"metric_threshold"`。

### 5.2 配置字段

- `field`（string，必填）
  - 在 `Payload` 中查找的字段路径，如：`"value"`、`"metrics.latency_ms"`。
- `op`（string，必填）
  - `">", "<", ">=", "<=", "between"` 之一。
- `value`（number，可选）
  - 用于 `> < >= <=` 四种操作的比较值。
- `min` / `max`（number，可选）
  - `between` 时使用的下界/上界，要求 `min <= max`。
- `severityOnViolation`（string，可选）
  - 违反阈值时使用的严重级别，默认 `"error"`，也可以设置为 `"warn"` 等。

### 5.3 示例：直接调用 Detector Service

在 Go 代码中手动调用（比如在某个 metrics 处理流程中）：

```go
svc := detector.NewService(registry) // registry 中已注册 metric_threshold

cfg := json.RawMessage(`{
  "field": "value",
  "op": ">",
  "value": 3,
  "severityOnViolation": "warn"
}`)

signals, elapsed, err := svc.TestExecute(
    ctx,
    "metric_threshold",
    detector.ExecuteRequest{
        ProjectID: 1,
        Config:    cfg,
        Payload: map[string]any{
            "value": 5,
        },
        Now: time.Now().UTC(),
    },
)
// signals[0].Severity == "warn", Status == "firing" (因为 5 > 3)
_ = elapsed
_ = err
```

如果未来要把 `metric_threshold` 作为 Monitor 的常用插件，需要扩展：

- 要么在 Monitor Worker 中为该类型注入所需的数值 payload；
- 要么在配置层面为它提供“从最近日志/事件中抽取字段”的桥接逻辑。

当前版本**没有**做这些额外桥接，所以建议将它视为“面向内部/高级用例”的插件，先服务于你自己的自定义处理流程。

---

## 6. 总结与后续建议

- 目前监控相关的开箱即用能力已经包括：
  - `http_check`：网站/API Uptime；
  - `tcp_check`：端口连通性；
  - `metric_threshold`：高级数值阈值判断（需要自定义 payload 注入）。
- 整体链路仍然是统一的：
  - Monitor 定义 -> Detector 插件执行 -> Signal -> AlertRule -> AlertDelivery。
- 下一步可以考虑：
  - 在文档中补充更多真实案例（例如监控某个 SaaS API、数据库端口、队列长度等）；
  - 如果你计划引入 metrics 管道，可以优先考虑把 `metric_threshold` 更好地嫁接到 Monitor/Alert 流程中。

