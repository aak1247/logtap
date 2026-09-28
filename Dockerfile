# logtap 数据面网关镜像（无 web，控制台由 logtap-cloud 提供）
# 注意：go.mod 有 replace ./sdks/go/logtap 指向仓库内模块，
# 因此必须先 COPY 全量源码再 go mod download。
FROM golang:1.26-alpine AS build
ENV GOPROXY=https://goproxy.cn,direct
WORKDIR /src
COPY . .
RUN go mod download
RUN CGO_ENABLED=0 go build -trimpath -o /out/gateway ./cmd/gateway

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata && adduser -D -u 10001 logtap
WORKDIR /app
COPY --from=build /out/gateway ./gateway
USER logtap
EXPOSE 8080
ENTRYPOINT ["./gateway"]
