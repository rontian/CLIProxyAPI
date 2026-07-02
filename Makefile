.PHONY: help dev dev-preflight build plugins build-copilot-plugin test test-auto fmt sync-config sync-config-dry tools build-sync-config

GO_PROXY ?= https://goproxy.cn,direct
DEV_PORT ?= 8317
GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)
PLUGIN_EXT := $(shell if [ "$(GOOS)" = "darwin" ]; then echo dylib; elif [ "$(GOOS)" = "windows" ]; then echo dll; else echo so; fi)
PLUGIN_OUTPUT_DIR ?= plugins/$(GOOS)/$(GOARCH)
COPILOT_PLUGIN_OUTPUT ?= $(PLUGIN_OUTPUT_DIR)/github-copilot.$(PLUGIN_EXT)

help:
	@echo "CLIProxyAPI development commands"
	@echo "  make dev                  Build local plugins and run the local server"
	@echo "  make dev-preflight        Stop stale local CLIProxyAPI listeners on DEV_PORT"
	@echo "  make build                Build cmd/server"
	@echo "  make plugins              Build maintained local plugins"
	@echo "  make build-copilot-plugin Build GitHub Copilot provider plugin"
	@echo "  make test                 Run all Go tests"
	@echo "  make test-auto            Run Auto Router focused tests"
	@echo "  make fmt                  Format Go files"
	@echo "  make sync-config          Add missing keys to config.yaml and .env"
	@echo "  make sync-config-dry      Preview config sync without writing"
	@echo "  make tools                Build production helper binaries"
	@echo "  make build-sync-config    Build only sync-config helper binaries"

dev: dev-preflight plugins
	./scripts/dev-run.sh

dev-preflight:
	./scripts/dev-port-preflight.sh $(DEV_PORT)

build:
	env GOPROXY=$(GO_PROXY) go build -o test-output ./cmd/server
	rm -f test-output

plugins: build-copilot-plugin

build-copilot-plugin:
	mkdir -p $(PLUGIN_OUTPUT_DIR)
	env GOPROXY=$(GO_PROXY) go build -buildmode=c-shared -o $(COPILOT_PLUGIN_OUTPUT) ./plugins-src/github-copilot/go

test:
	env GOPROXY=$(GO_PROXY) go test ./...

test-auto:
	env GOPROXY=$(GO_PROXY) go test ./internal/api/handlers/management ./internal/api ./internal/autorouter ./internal/config ./sdk/api/handlers ./sdk/api/handlers/openai ./sdk/config

fmt:
	gofmt -w .

sync-config:
	cd tools/sync-config && go run . --config ../../config.yaml --config-example ../../config.example.yaml --env ../../.env --env-example ../../.env.example

sync-config-dry:
	cd tools/sync-config && go run . --config ../../config.yaml --config-example ../../config.example.yaml --env ../../.env --env-example ../../.env.example --dry-run

tools: build-sync-config

build-sync-config:
	cd tools/sync-config && \
		CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o ../sync-config-linux-amd64 . && \
		CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags='-s -w' -o ../sync-config-linux-arm64 . && \
		CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o ../sync-config-darwin-amd64 . && \
		CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags='-s -w' -o ../sync-config-darwin-arm64 . && \
		CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o ../sync-config-windows-amd64.exe . && \
		CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build -trimpath -ldflags='-s -w' -o ../sync-config-windows-arm64.exe .
