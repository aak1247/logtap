# logtap Flutter SDK

用于向 logtap 网关上报「结构化日志」与「埋点事件」。

## 使用

```dart
import "package:logtap_flutter/logtap_flutter.dart";

final client = await LogtapClient.create(
  const LogtapClientOptions(
    baseUrl: "http://localhost:8080",
    projectId: 1,
    projectKey: "pk_xxx", // 启用 AUTH_SECRET 时必填
    gzip: true,           // Web 会自动降级为非 gzip
  ),
);

client.captureFlutterErrors(); // 可选：捕获未处理异常

client.info("hello", {"k": "v"});
client.identify("u1", {"plan": "pro"});
client.track("signup", {"from": "landing"});

await client.close();
```

## Device ID 持久化

`persistDeviceId=true` 时默认：

- Flutter Web：使用 `localStorage`
- 非 Web：使用用户目录下的文件（尽力而为）

如需更精确的持久化位置（例如移动端），可通过 `LogtapClientOptions.deviceIdStore` 自行实现存储。
