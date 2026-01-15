# Java 集成

Java 端目前没有内置 SDK，推荐用你熟悉的 HTTP 客户端按 `INGEST.md` 上报。

相关文档：

- 上报接口与数据模型：`../INGEST.md`

## 1) OkHttp 示例

Maven（参考版本）：

```xml
<dependency>
  <groupId>com.squareup.okhttp3</groupId>
  <artifactId>okhttp</artifactId>
  <version>4.12.0</version>
</dependency>
```

结构化日志：

```java
OkHttpClient client = new OkHttpClient();
String json = "{\"level\":\"info\",\"message\":\"hello\",\"user\":{\"id\":\"u1\"},\"fields\":{\"k\":\"v\"}}";

Request req = new Request.Builder()
  .url("http://localhost:8080/api/1/logs/")
  .post(RequestBody.create(json, MediaType.parse("application/json")))
  .addHeader("Content-Type", "application/json")
  .addHeader("X-Project-Key", "pk_xxx")
  .build();

try (Response resp = client.newCall(req).execute()) {
  // 202 Accepted 表示已接收
}
```

同一路径也支持批量：请求体改为 JSON 数组（`[{},{}]`）即可。

埋点/事件：

```java
OkHttpClient client = new OkHttpClient();
String json = "{\"name\":\"signup\",\"user\":{\"id\":\"u1\"},\"properties\":{\"plan\":\"pro\"}}";

Request req = new Request.Builder()
  .url("http://localhost:8080/api/1/track/")
  .post(RequestBody.create(json, MediaType.parse("application/json")))
  .addHeader("Content-Type", "application/json")
  .addHeader("X-Project-Key", "pk_xxx")
  .build();

try (Response resp = client.newCall(req).execute()) {
}
```

同一路径也支持批量：请求体改为 JSON 数组（`[{},{}]`）即可。
