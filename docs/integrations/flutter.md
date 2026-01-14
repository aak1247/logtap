# Flutter 集成

推荐使用官方 Flutter SDK（同时支持 Flutter Mobile 与 Flutter Web；Web 端 gzip 会自动降级）。

相关文档：

- 上报接口与数据模型：`../INGEST.md`
- SDK 快速开始：`../SDKs.md`

## 使用官方 Flutter SDK

目录：`sdks/flutter/logtap_flutter/`（包名：`logtap_flutter`）

```dart
final client = await LogtapClient.create(
  const LogtapClientOptions(
    baseUrl: "http://localhost:8080",
    projectId: 1,
    projectKey: "pk_xxx", // 开启 AUTH_SECRET 时必填
    gzip: true, // Web 自动降级为非 gzip
  ),
);

client.captureFlutterErrors(); // 可选：捕获未处理异常

client.info("hello", {"k": "v"});
client.track("signup", {"plan": "pro"});

await client.close();
```

