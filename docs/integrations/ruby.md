# Ruby 集成

Ruby 端目前没有内置 SDK，推荐按 `INGEST.md` 的接口用 `net/http` / Faraday 等方式上报。

相关文档：

- 上报接口与数据模型：`../INGEST.md`

## 1) net/http 示例

### 1.1 结构化日志

```rb
require "net/http"
require "json"

base_url = "http://localhost:8080"
project_id = 1
project_key = "pk_xxx" # 开启 AUTH_SECRET 时必填

uri = URI("#{base_url}/api/#{project_id}/logs/")
req = Net::HTTP::Post.new(uri)
req["Content-Type"] = "application/json"
req["X-Project-Key"] = project_key
req.body = {
  level: "info",
  message: "hello",
  user: { id: "u1" },
  fields: { k: "v" },
}.to_json

Net::HTTP.start(uri.hostname, uri.port) do |http|
  http.request(req)
end
```

同一路径也支持批量：把 `req.body` 改成数组（`[{...},{...}]`）并 `to_json` 即可。

### 1.2 埋点/事件

```rb
require "net/http"
require "json"

base_url = "http://localhost:8080"
project_id = 1
project_key = "pk_xxx"

uri = URI("#{base_url}/api/#{project_id}/track/")
req = Net::HTTP::Post.new(uri)
req["Content-Type"] = "application/json"
req["X-Project-Key"] = project_key
req.body = {
  name: "signup",
  user: { id: "u1" },
  properties: { plan: "pro" },
}.to_json

Net::HTTP.start(uri.hostname, uri.port) do |http|
  http.request(req)
end
```

同一路径也支持批量：把 `req.body` 改成数组（`[{...},{...}]`）并 `to_json` 即可。
