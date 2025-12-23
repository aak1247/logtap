# logtap Gin Demo

```bash
set LOGTAP_BASE_URL=http://localhost:8080
set LOGTAP_PROJECT_ID=1
set LOGTAP_PROJECT_KEY=pk_xxx
go run ./demo/gin-app
```

访问：

- `GET http://localhost:8090/`
- `GET http://localhost:8090/track?name=signup`
- `GET http://localhost:8090/panic`

说明：

- `LOGTAP_PROJECT_KEY` 仅当网关启用 `AUTH_SECRET` 时必填。
- `LOGTAP_GZIP` 默认 `true`，设为 `false` 可禁用压缩。
- `HTTP_ADDR` 可修改服务监听地址（默认 `:8090`）。
