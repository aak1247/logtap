# logtap Browser Demo

启动：

```bash
node demo/js-browser/server.mjs
```

打开：

- `http://localhost:5174/demo/js-browser/`

在页面里点击「初始化」，再发送日志/事件。

说明：

- `Project Key` 仅当网关启用 `AUTH_SECRET` 时必填。
- 勾选 `gzip` 需要浏览器支持 `CompressionStream`（不支持会自动降级）。
- Demo 使用本地 SDK 路径；实际项目请安装 `logtap-sdk` 并改为包导入。
