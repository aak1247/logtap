# HTTP Uptime 监控插件 (http_check)

## 概述

`http_check` 是 logtap 内置的 detector 插件，用于定期检查 HTTP/HTTPS 端点的可用性，支持状态码、响应体关键字、TLS 证书到期等多种检测条件。

检查结果会生成 `detector.Signal`，通过现有的 Monitor → Alert pipeline 自动触发告警。

## 快速开始

### 1. 创建 HTTP 监控

通过 API 创建一个 HTTP 监控项：

```bash
curl -X POST http://localhost:8011/api/v1/monitors \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <your-token>" \
  -d '{
    "name": "官网健康检查",
    "project_id": 1,
    "detector_type": "http_check",
    "interval_seconds": 60,
    "config": {
      "url": "https://example.com/healthz",
      "method": "GET",
      "timeoutMs": 5000,
      "expectStatus": [200, 204]
    }
  }'
```

### 2. 配置告警规则

创建一条 AlertRule，当 HTTP 检查失败时触发通知：

```bash
curl -X POST http://localhost:8011/api/v1/alert-rules \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <your-token>" \
  -d '{
    "name": "站点不可用告警",
    "project_id": 1,
    "conditions": {
      "source_type": "http_check",
      "severity": "error"
    },
    "channels": ["webhook"],
    "repeat_interval_minutes": 30
  }'
```

### 3. 查看结果

在 Alerts → Monitor 页面中查看运行历史，每次运行记录包含：
- **状态**：`success` 或 `failed`
- **响应时间**：`elapsed_ms`
- **状态码**：`status_code`
- **错误信息**：失败时显示具体原因

## 配置参数

| 参数 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| `url` | string | ✅ | - | 目标 URL，必须为 http/https |
| `method` | string | ❌ | `GET` | HTTP 方法 |
| `headers` | object | ❌ | - | 自定义请求头，`{"key": "value"}` |
| `body` | string | ❌ | - | 请求体 |
| `expectStatus` | int[] | ❌ | 2xx | 期望的 HTTP 状态码列表 |
| `expectBodySubstring` | string | ❌ | - | 响应体中必须包含的子串 |
| `timeoutMs` | int | ❌ | 5000 | 超时时间（毫秒），范围 100-60000 |
| `minTlsValidDays` | int | ❌ | 0 | TLS 证书至少剩余有效天数，0 表示不检查 |

## 配置示例

### 基本健康检查

```json
{
  "url": "https://myapp.com/api/health",
  "timeoutMs": 3000,
  "expectStatus": [200]
}
```

### API 响应内容检查

```json
{
  "url": "https://myapp.com/api/status",
  "method": "POST",
  "headers": {"Authorization": "Bearer token123"},
  "body": "{\"check\": true}",
  "expectStatus": [200],
  "expectBodySubstring": "\"status\":\"ok\""
}
```

### TLS 证书到期监控

```json
{
  "url": "https://myapp.com",
  "minTlsValidDays": 30
}
```

当证书有效期不足 30 天时，会产生 `severity=error` 的告警信号。

## Signal 字段说明

每次执行产出的 Signal 包含以下 Fields：

| 字段 | 说明 |
|------|------|
| `source_type` | 固定为 `"http_check"` |
| `url` | 检查的 URL |
| `method` | HTTP 方法 |
| `elapsed_ms` | 响应耗时（毫秒） |
| `status_code` | HTTP 状态码（请求成功时） |
| `error` | 错误信息（请求失败时） |
| `body_snippet` | 响应体片段（配置了 expectBodySubstring 时） |
| `cert_not_after` | TLS 证书过期时间（HTTPS 时） |
| `cert_days_left` | TLS 证书剩余天数（HTTPS 时） |
| `cert_expiring_soon` | 证书即将过期标志 |

Signal 的 Severity 和 Status：
- **成功**：`severity=info`, `status=resolved`
- **失败**：`severity=error`, `status=firing`

## 注意事项

- **误报抑制**：建议在 AlertRule 中设置合理的窗口和阈值，避免偶发超时导致频繁告警。
- **超时控制**：`timeoutMs` 建议设为 3000-10000ms，确保不会因单个慢站点阻塞 worker。
- **响应体**：插件最多读取 64KB 响应体用于子串匹配，不会存储完整内容。
- **安全性**：Result 中不记录完整响应体，仅保留状态码、耗时和简短错误信息。
