# 部署指引

本文覆盖两种部署方式：

1) 本地/测试：Docker Compose 一键启动（推荐先用它验证）
2) 服务器/生产：无 Docker（systemd）或自定义容器化部署

## 0. 依赖组件

- PostgreSQL / TimescaleDB（用于事件与日志存储）
- NSQ（用于异步写库与削峰）
- （可选）Redis（用于部分指标/缓存能力，按项目配置而定）

## 1) Docker Compose（本地/测试）

目录：`deploy/`

```bash
cd deploy
docker compose up --build
```

可选：启用 GeoIP（国家/城市/运营商分布）

- 需要 MaxMind GeoLite2 下载密钥：设置环境变量 `MAXMIND_LICENSE_KEY`（见 `.env.example`）
- Compose 会把 mmdb 下载/缓存到 volume：`/data/geoip/`
- 如果 mmdb 存在但你未显式配置 `GEOIP_CITY_MMDB/GEOIP_ASN_MMDB`，容器启动脚本会自动使用：
  - `GEOIP_CITY_MMDB=/data/geoip/GeoLite2-City.mmdb`
  - `GEOIP_ASN_MMDB=/data/geoip/GeoLite2-ASN.mmdb`

启动后：

- 网关：`http://localhost:8080`
- NSQ Admin：`http://localhost:4171`
- Postgres：`localhost:5432`（默认库：`logtap`）

`docker-compose.yml` 已包含：TimescaleDB、Redis、NSQ（lookupd/nsqd/admin）与 logtap 网关。

说明：控制台构建会在打包时读取仓库根目录下的 `docs/` Markdown（用于内置文档页），所以 Docker 构建阶段会一并拷贝 `docs/`。

## 2) 无 Docker（服务器/生产）

### 2.1 构建网关

需要 Go 1.22+：

```bash
go build -o gateway ./cmd/gateway
```

把 `gateway` 上传到服务器（例如 `/opt/logtap/gateway`）。

### 2.2 最小环境变量

环境变量参考 `.env.example`，常用项：

- `HTTP_ADDR`：HTTP 监听地址，例如 `:8080`
- `NSQD_ADDRESS`：NSQ 地址，例如 `127.0.0.1:4150`
- `RUN_CONSUMERS`：是否启动消费者写库（生产通常为 `true`）
- `POSTGRES_URL`：当 `RUN_CONSUMERS=true` 时必填，例如：
  - `postgres://logtap:logtap@127.0.0.1:5432/logtap?sslmode=disable`
- `AUTH_SECRET`：开启登录与上报鉴权（推荐）

网关启动时会使用 GORM `AutoMigrate` 自动建表/建索引。

### 2.3 systemd（可选模板）

模板在 `deploy/systemd/`：

- `deploy/systemd/logtap-gateway.service`
- `deploy/systemd/nsqd.service`

建议做法：

- 创建用户/目录：`/opt/logtap/`
- 把二进制与 `.env` 放到固定目录
- `systemctl enable --now logtap-gateway`

## 3) 控制台（Web）

目录：`web/`（Vite + React + Tailwind）

开发：

```bash
cd web
bun install
bun run dev
```

如果 Bun 在你的环境（尤其是老 CPU / 某些 CI）会出现 `SIGILL / Illegal instruction`（缺少 AVX），可改用 Node/NPM：

```bash
cd web
npm install
npm run dev
```

生产构建（静态资源）：

```bash
cd web
bun install
bun run build
```

或：

```bash
cd web
npm install
npm run build
```

构建产物在 `web/dist/`，可用 Nginx/静态托管服务部署。

## 4) 生产建议

- 反向代理：用 Nginx/Traefik 提供 HTTPS，并把 `/api/` 反代到网关
- 数据持久化：Postgres 数据盘、NSQ（可按需做高可用）与日志保留策略
- 资源隔离：消费者写库与网关可拆开部署（不同进程/不同实例）
