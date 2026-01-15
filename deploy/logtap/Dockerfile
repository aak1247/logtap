# Build stage for Go backend
FROM golang:1.22 AS go-build
WORKDIR /src
# Configure Go proxy for China
ENV GOPROXY=https://goproxy.cn,https://goproxy.io,direct \
    GO111MODULE=on
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/gateway ./cmd/gateway

# Build stage for frontend
FROM oven/bun:1 AS web-build
WORKDIR /web
# Configure npm registry for China
ENV npm_config_registry=https://registry.npmmirror.com
COPY web/package.json web/bun.lock ./
RUN bun install
COPY web/ ./
RUN bun run build

# Final stage with nginx + supervisord
FROM alpine:3.19
RUN apk add --no-cache \
    ca-certificates \
    nginx \
    supervisor \
    && rm -rf /var/cache/apk/*

# Copy nginx config
COPY deploy/logtap/nginx.conf /etc/nginx/nginx.conf

# Copy supervisord config
COPY deploy/logtap/supervisord.conf /etc/supervisor.d/supervisord.conf

# Copy Go binary
COPY --from=go-build /out/gateway /gateway

# Copy frontend build
COPY --from=web-build /web/dist /usr/share/nginx/html

# Create directory for nginx temp files
RUN mkdir -p /tmp/nginx && \
    mkdir -p /var/log/supervisor /var/run && \
    chown -R nginx:nginx /usr/share/nginx/html && \
    chown -R nginx:nginx /tmp/nginx && \
    chmod +x /gateway

EXPOSE 8080
ENTRYPOINT ["/usr/bin/supervisord", "-c", "/etc/supervisor.d/supervisord.conf"]
