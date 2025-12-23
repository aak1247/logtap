# logtap Node Demo

```powershell
$env:LOGTAP_BASE_URL="http://localhost:8080"
$env:LOGTAP_PROJECT_ID="1"
$env:LOGTAP_PROJECT_KEY="pk_xxx"
node demo/node-app/index.mjs
```

会每秒上报一条 info 日志，每 5 秒上报一次 event（track）。

说明：

- `LOGTAP_PROJECT_KEY` 仅当网关启用 `AUTH_SECRET` 时必填。
- `LOGTAP_GZIP` 默认 `true`，设为 `false` 可禁用压缩。
- 可选：设置 `DEMO_DURATION_MS` 自动退出（便于脚本化演示/CI）。
- Demo 使用本地 SDK 路径；实际项目请安装 `logtap-sdk` 并改为包导入。
