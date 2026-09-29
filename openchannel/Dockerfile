# ---- 构建阶段 ----
FROM golang:1.22 AS build

WORKDIR /src

# 先拷贝依赖清单，尽量利用构建缓存（本项目仅依赖标准库，无外部 require）
COPY go.mod ./
RUN go mod download

# 拷贝源码并静态编译（CGO_ENABLED=0，产出可放进精简基础镜像的静态二进制）
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
        -trimpath \
        -ldflags="-s -w" \
        -o /out/openchannel \
        ./cmd/openchannel

# ---- 运行阶段 ----
# 与构建同属 golang:1.22 系列，单容器启动即可对外应答。
FROM golang:1.22

# 非 root 用户运行
RUN useradd --create-home --shell /usr/sbin/nologin appuser
WORKDIR /app
COPY --from=build /out/openchannel /app/openchannel
USER appuser

EXPOSE 8080

# 容器级健康检查复用服务的 /healthz。
# golang 基础镜像不带 curl，用 wget 的 --spider 只探活不取正文。
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -q --spider http://127.0.0.1:8080/healthz || exit 1

ENTRYPOINT ["/app/openchannel"]
