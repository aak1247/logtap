# Python 集成

Python 端目前没有内置 SDK，推荐直接按 `INGEST.md` 的接口进行 HTTP 上报。

相关文档：

- 上报接口与数据模型：`../INGEST.md`

## 1) requests 示例

安装：

```bash
pip install requests
```

结构化日志：

```py
import requests

base_url = "http://localhost:8080"
project_id = 1
project_key = "pk_xxx"  # 开启 AUTH_SECRET 时必填

requests.post(
    f"{base_url}/api/{project_id}/logs/",
    headers={
        "Content-Type": "application/json",
        "X-Project-Key": project_key,
    },
    json={
        "level": "info",
        "message": "hello",
        "user": {"id": "u1"},
        "fields": {"k": "v"},
    },
    timeout=5,
)
```

同一路径也支持批量：把 `json={...}` 改成 `json=[{...}, {...}]` 即可。

埋点/事件：

```py
requests.post(
    f"{base_url}/api/{project_id}/track/",
    headers={
        "Content-Type": "application/json",
        "X-Project-Key": project_key,
    },
    json={
        "name": "signup",
        "user": {"id": "u1"},
        "properties": {"plan": "pro"},
    },
    timeout=5,
)
```

同一路径也支持批量：把 `json={...}` 改成 `json=[{...}, {...}]` 即可。
