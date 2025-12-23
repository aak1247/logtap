# Demo（SDK 集成演示）

本目录提供 4 个可运行的集成演示：

- Browser（前端页面）：`demo/js-browser/`
- Node（脚本）：`demo/node-app/`
- Go + Gin（服务端）：`demo/gin-app/`
- Flutter App：`demo/flutter-app/`

## 0) 先启动 logtap 网关

任选一种方式：

### Docker（推荐）

```bash
cd deploy
docker compose up --build
```

网关默认：`http://localhost:8080`

### Windows（无 Docker）

```powershell
powershell -ExecutionPolicy Bypass -File scripts/run-gateway.ps1
```

## 1) 准备项目配置

演示默认使用：

- `LOGTAP_BASE_URL`：`http://localhost:8080`
- `LOGTAP_PROJECT_ID`：`1`
- `LOGTAP_PROJECT_KEY`：`pk_xxx`（仅当网关启用 `AUTH_SECRET` 时需要）

> 网关未启用鉴权时可以不填 `LOGTAP_PROJECT_KEY`。

## 2) 运行各演示

### A) Browser Demo

```bash
node demo/js-browser/server.mjs
```

打开浏览器：`http://localhost:5174/demo/js-browser/`

### B) Node Demo

```bash
node demo/node-app/index.mjs
```

环境变量（PowerShell 示例）：

```powershell
$env:LOGTAP_BASE_URL="http://localhost:8080"
$env:LOGTAP_PROJECT_ID="1"
$env:LOGTAP_PROJECT_KEY="pk_xxx"
```

可选：`$env:DEMO_DURATION_MS="10000"` 自动运行 10 秒后退出。

### C) Gin Demo

```bash
go run ./demo/gin-app
```

环境变量（PowerShell 示例）：

```powershell
$env:LOGTAP_BASE_URL="http://localhost:8080"
$env:LOGTAP_PROJECT_ID="1"
$env:LOGTAP_PROJECT_KEY="pk_xxx"
```

访问：

- `GET http://localhost:8090/`（写一条 info 日志）
- `GET http://localhost:8090/track?name=signup`（写一条 event）
- `GET http://localhost:8090/panic`（触发 panic 并上报 fatal）

### D) Flutter Demo

```bash
cd demo/flutter-app
flutter pub get --offline
flutter run -d chrome
```

> Android Emulator 访问宿主机网关：把 baseUrl 改为 `http://10.0.2.2:8080`。

## 3) 如何确认已上报

- 控制台：`web/`（概览页可看到最新日志/事件）
- 或直接看网关返回：上报接口成功返回 `202 Accepted`
