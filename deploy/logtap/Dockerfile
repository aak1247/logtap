# Build stage for frontend
FROM node:22-alpine AS web-build
WORKDIR /repo
# Configure npm registry for China
ENV npm_config_registry=https://registry.npmmirror.com
COPY web/package.json web/package.json
RUN cd web && npm install --include=dev --no-audit --no-fund
COPY web/ web/
COPY docs/ docs/
RUN cd web && npm run build

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

# Optional build stage: download GeoIP mmdb (MaxMind GeoLite2)
FROM alpine:3.19 AS geoip
RUN apk add --no-cache ca-certificates curl tar gzip && rm -rf /var/cache/apk/*
ARG MAXMIND_LICENSE_KEY=""
ARG MAXMIND_CITY_EDITION_ID="GeoLite2-City"
ARG MAXMIND_ASN_EDITION_ID="GeoLite2-ASN"
WORKDIR /out
RUN set -eux; \
    mkdir -p /out; \
    if [ -n "${MAXMIND_LICENSE_KEY}" ]; then \
      curl -fsSL "https://download.maxmind.com/app/geoip_download?edition_id=${MAXMIND_CITY_EDITION_ID}&license_key=${MAXMIND_LICENSE_KEY}&suffix=tar.gz" -o city.tar.gz; \
      mmdb_path="$(tar -tzf city.tar.gz | awk '/\\.mmdb$/ {print; exit}')"; \
      tar -xzf city.tar.gz -C /out "${mmdb_path}"; \
      rm -f city.tar.gz; \
      mv "/out/${mmdb_path}" /out/GeoLite2-City.mmdb; \
      rm -rf "/out/$(echo "${mmdb_path}" | cut -d/ -f1)" 2>/dev/null || true; \
      curl -fsSL "https://download.maxmind.com/app/geoip_download?edition_id=${MAXMIND_ASN_EDITION_ID}&license_key=${MAXMIND_LICENSE_KEY}&suffix=tar.gz" -o asn.tar.gz; \
      mmdb_path="$(tar -tzf asn.tar.gz | awk '/\\.mmdb$/ {print; exit}')"; \
      tar -xzf asn.tar.gz -C /out "${mmdb_path}"; \
      rm -f asn.tar.gz; \
      mv "/out/${mmdb_path}" /out/GeoLite2-ASN.mmdb; \
      rm -rf "/out/$(echo "${mmdb_path}" | cut -d/ -f1)" 2>/dev/null || true; \
    fi

# Final stage with nginx + supervisord
FROM alpine:3.19
RUN apk add --no-cache \
    ca-certificates \
    curl \
    tar \
    gzip \
    nginx \
    supervisor \
    && rm -rf /var/cache/apk/*

# Copy nginx config
COPY deploy/logtap/nginx.conf /etc/nginx/nginx.conf

# Copy supervisord config
COPY deploy/logtap/supervisord.conf /etc/supervisor.d/supervisord.conf

# Copy Go binary
COPY --from=go-build /out/gateway /gateway

# Optional: GeoIP databases (present only when MAXMIND_LICENSE_KEY was provided at build)
COPY --from=geoip /out/ /data/geoip/

# Startup script: optional runtime GeoIP download + env defaults
COPY deploy/logtap/entrypoint.sh /entrypoint.sh

# Copy frontend build
COPY --from=web-build /repo/web/dist /usr/share/nginx/html

# Create directory for nginx temp files
RUN mkdir -p /tmp/nginx && \
    mkdir -p /var/log/supervisor /var/run /data/geoip && \
    chown -R nginx:nginx /usr/share/nginx/html && \
    chown -R nginx:nginx /tmp/nginx && \
    chmod +x /gateway /entrypoint.sh

EXPOSE 8080
ENTRYPOINT ["/usr/bin/supervisord", "-c", "/etc/supervisor.d/supervisord.conf"]
