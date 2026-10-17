# 多阶段构建 Dockerfile for CodeI18n

# 阶段 1: 构建
FROM golang:1.27.1-alpine AS builder

# 设置工作目录
WORKDIR /build

# 安装构建依赖
RUN apk add --no-cache git make

# 复制 go mod 文件
COPY go.mod go.sum ./

# 下载依赖
RUN go mod download

# 复制源代码
COPY . .

# 构建可执行文件
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags="-w -s" -o codei18n ./cmd/codei18n

# 阶段 2: 运行时
FROM alpine:latest

# 安装运行时依赖
RUN apk --no-cache add ca-certificates

# 设置工作目录
WORKDIR /app

# 从构建阶段复制可执行文件
COPY --from=builder /build/codei18n .

# 创建配置目录
RUN mkdir -p /.codei18n

# 设置环境变量
ENV PATH="/app:${PATH}"

# 暴露配置目录作为 volume
VOLUME ["/.codei18n"]

# 设置入口点
ENTRYPOINT ["/app/codei18n"]

# 默认命令
CMD ["--help"]
