# DeepAICode2API 常用命令。
#
# Windows 上 `go test -race` 需要 PATH 里有 `ld.exe`：
# scoop 的 mingw 只带了 ld.bfd.exe，可临时做一个 shim：
#   mkdir -p /tmp/ldshim
#   cp "$(dirname "$(which gcc)")/ld.bfd.exe" /tmp/ldshim/ld.exe
#   PATH="/tmp/ldshim:$PATH" make test-race

BIN     := bin
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

GOFILES := find . -name '*.go' -not -path './.git/*'

.PHONY: all build register probe test test-race vet fmt lint cover docker clean

all: build register

## build: 构建网关
build:
	@mkdir -p $(BIN)
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN)/deepaicode2api .

## register: 构建批量注册 CLI
register:
	@mkdir -p $(BIN)
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN)/register ./cmd/register

## probe: 构建上游探测 CLI
probe:
	@mkdir -p $(BIN)
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN)/probe ./cmd/probe

## test: 跑测试
test:
	go test -count=1 ./...

## test-race: 跑竞态检测（需要可用的 C 工具链）
test-race:
	go test -race -count=1 ./...

## vet: 静态检查
vet:
	go vet ./...

## fmt: 格式化
fmt:
	$(GOFILES) -print0 | xargs -0 gofmt -w

## lint: 与 CI 一致（格式 + vet + 测试）
lint:
	@sh scripts/lint.sh

## cover: 覆盖率
cover:
	go test -cover ./internal/...

## docker: 构建镜像
docker:
	docker build -t deepaicode2api:$(VERSION) .

## clean: 清理构建产物
clean:
	rm -rf $(BIN)
