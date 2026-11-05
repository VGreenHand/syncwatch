# syntax=docker/dockerfile:1
#
# 基础镜像可通过构建参数替换（国内服务器无法直连 Docker Hub 时用镜像源）：
#   docker build --build-arg GO_IMAGE=docker.m.daocloud.io/library/golang:1.22-alpine \
#                --build-arg RUNTIME_IMAGE=docker.m.daocloud.io/library/alpine:3.20 .
ARG GO_IMAGE=golang:1.22-alpine
ARG RUNTIME_IMAGE=alpine:3.20

# ---------- 构建阶段 ----------
FROM ${GO_IMAGE} AS builder
ARG GOPROXY=https://proxy.golang.org,direct
WORKDIR /src
ENV CGO_ENABLED=0 GOOS=linux GOPROXY=${GOPROXY}
COPY go.mod go.sum ./
RUN go mod download
COPY main.go ./
RUN go build -trimpath -ldflags="-s -w" -o /syncwatch .

# ---------- 运行阶段 ----------
FROM ${RUNTIME_IMAGE}
RUN apk add --no-cache ca-certificates tzdata \
 && adduser -D -u 10001 syncwatch
COPY --from=builder /syncwatch /syncwatch
USER syncwatch
EXPOSE 8995
ENTRYPOINT ["/syncwatch"]
