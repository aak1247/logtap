# PHP 集成

PHP 端目前没有内置 SDK，推荐用 cURL 或你熟悉的 HTTP 客户端按 `INGEST.md` 上报。

相关文档：

- 上报接口与数据模型：`../INGEST.md`

## 1) cURL 示例

### 1.1 结构化日志

```php
<?php

$baseUrl = "http://localhost:8080";
$projectId = 1;
$projectKey = "pk_xxx"; // 开启 AUTH_SECRET 时必填

$payload = json_encode([
  "level" => "info",
  "message" => "hello",
  "user" => ["id" => "u1"],
  "fields" => ["k" => "v"],
]);

$ch = curl_init("$baseUrl/api/$projectId/logs/");
curl_setopt($ch, CURLOPT_RETURNTRANSFER, true);
curl_setopt($ch, CURLOPT_POST, true);
curl_setopt($ch, CURLOPT_HTTPHEADER, [
  "Content-Type: application/json",
  "X-Project-Key: $projectKey",
]);
curl_setopt($ch, CURLOPT_POSTFIELDS, $payload);

$res = curl_exec($ch);
curl_close($ch);
```

同一路径也支持批量：把 `$payload` 改成数组（`[ [...], [...] ]`）后再 `json_encode` 即可。

### 1.2 埋点/事件

```php
<?php

$baseUrl = "http://localhost:8080";
$projectId = 1;
$projectKey = "pk_xxx";

$payload = json_encode([
  "name" => "signup",
  "user" => ["id" => "u1"],
  "properties" => ["plan" => "pro"],
]);

$ch = curl_init("$baseUrl/api/$projectId/track/");
curl_setopt($ch, CURLOPT_RETURNTRANSFER, true);
curl_setopt($ch, CURLOPT_POST, true);
curl_setopt($ch, CURLOPT_HTTPHEADER, [
  "Content-Type: application/json",
  "X-Project-Key: $projectKey",
]);
curl_setopt($ch, CURLOPT_POSTFIELDS, $payload);

$res = curl_exec($ch);
curl_close($ch);
```

同一路径也支持批量：把 `$payload` 改成数组（`[ [...], [...] ]`）后再 `json_encode` 即可。
