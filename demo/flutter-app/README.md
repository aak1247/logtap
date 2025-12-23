# logtap Flutter Demo（Web）

```bash
cd demo/flutter-app
flutter pub get --offline
flutter run -d chrome
```

说明：

- Android Emulator 访问宿主机网关：Base URL 用 `http://10.0.2.2:8080`
- 启用 `AUTH_SECRET` 时需要填 `Project Key`
- `gzip` 在 Web 会自动降级（不支持时使用非 gzip）
