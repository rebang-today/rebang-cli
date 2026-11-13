# rebang CLI 构建脚本
VERSION ?= $(or $(shell git describe --tags --match 'v*' --dirty 2>/dev/null | sed 's/^v//'),0.0.0-dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
PKG     := github.com/rebang-today/rebang-cli/internal/buildinfo
LDFLAGS := -s -w -X $(PKG).Version=$(VERSION) -X $(PKG).Commit=$(COMMIT) -X $(PKG).Date=$(DATE)

.PHONY: build test vet fmt lint check install snapshot clean

build: ## 构建到 bin/rebang
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/rebang ./cmd/rebang

test: ## 运行全部测试
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w cmd internal tools

lint: vet
	@test -z "$$(gofmt -l cmd internal tools)" || (echo "需要 gofmt:"; gofmt -l cmd internal tools; exit 1)

check: lint test ## 提交前检查

install: ## 安装到 GOPATH/bin
	CGO_ENABLED=0 go install -trimpath -ldflags "$(LDFLAGS)" ./cmd/rebang

snapshot: ## 本地试跑发布流程（不发布、不签名）
	goreleaser release --snapshot --clean --skip=publish,sign

clean:
	go clean
	rm -rf ./bin ./dist
