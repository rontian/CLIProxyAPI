.PHONY: help dev build test test-auto fmt sync-config sync-config-dry

PYTHON ?= python3
GO_PROXY ?= https://goproxy.cn,direct

help:
	@echo "CLIProxyAPI development commands"
	@echo "  make dev              Run the local server"
	@echo "  make build            Build cmd/server"
	@echo "  make test             Run all Go tests"
	@echo "  make test-auto        Run Auto Router focused tests"
	@echo "  make fmt              Format Go files"
	@echo "  make sync-config      Add missing keys to config.yaml and .env"
	@echo "  make sync-config-dry  Preview config sync without writing"

dev:
	go run ./cmd/server

build:
	env GOPROXY=$(GO_PROXY) go build -o test-output ./cmd/server
	rm -f test-output

test:
	env GOPROXY=$(GO_PROXY) go test ./...

test-auto:
	env GOPROXY=$(GO_PROXY) go test ./internal/api/handlers/management ./internal/api ./internal/autorouter ./internal/config ./sdk/api/handlers ./sdk/api/handlers/openai ./sdk/config

fmt:
	gofmt -w .

sync-config:
	$(PYTHON) ./scripts/sync-config.py

sync-config-dry:
	$(PYTHON) ./scripts/sync-config.py --dry-run
