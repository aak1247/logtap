# .NET 集成

.NET 端目前没有内置 SDK，推荐直接用 `HttpClient` 按 `INGEST.md` 上报。

相关文档：

- 上报接口与数据模型：`../INGEST.md`

## 1) HttpClient 示例（C#）

### 1.1 结构化日志

```csharp
using System.Text;
using System.Text.Json;

var http = new HttpClient();

var payload = new
{
  level = "info",
  message = "hello",
  user = new { id = "u1" },
  fields = new { k = "v" },
};

var req = new HttpRequestMessage(HttpMethod.Post, "http://localhost:8080/api/1/logs/");
req.Headers.Add("X-Project-Key", "pk_xxx");
req.Content = new StringContent(JsonSerializer.Serialize(payload), Encoding.UTF8, "application/json");

var resp = await http.SendAsync(req);
```

### 1.2 埋点/事件

```csharp
var payload = new
{
  name = "signup",
  user = new { id = "u1" },
  properties = new { plan = "pro" },
};

var req = new HttpRequestMessage(HttpMethod.Post, "http://localhost:8080/api/1/track/");
req.Headers.Add("X-Project-Key", "pk_xxx");
req.Content = new StringContent(JsonSerializer.Serialize(payload), Encoding.UTF8, "application/json");

var resp = await http.SendAsync(req);
```
