# syntax=docker/dockerfile:1
FROM golang:1.26-alpine AS build

WORKDIR /src
COPY go.mod ./
COPY . .

# VERSION 必须能在构建时覆盖：控制台的「检查更新」要拿它跟 GitHub release 比，
# 而 "docker" 解析不出 semver（parseSemver 失败 → 永远不提示新版本）。
#   docker compose build --build-arg VERSION=$(git describe --tags)
# 不传时保持 "docker"：明确表示「这个二进制不是按 tag 构建的」。
ARG VERSION=docker
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X main.version=${VERSION}" \
    -o /out/deepaicode2api .

FROM alpine:3.20

# hadolint ignore=DL3018
RUN apk add --no-cache ca-certificates tzdata \
    && adduser -D -u 10001 app \
    && mkdir -p /data \
    && chown 10001:10001 /data

COPY --from=build /out/deepaicode2api /usr/local/bin/deepaicode2api

WORKDIR /app
USER 10001:10001

ENV DEEPAI2API_HOST=0.0.0.0 \
    DEEPAI2API_PORT=10087 \
    DEEPAI2API_DATA_PATH=/data/deepaicode2api-data.json

VOLUME ["/data"]
EXPOSE 10087

ENTRYPOINT ["deepaicode2api"]
