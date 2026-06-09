WAILS ?= wails

.PHONY: launch dev build appimage agent agent-forge agent-all test test-go test-frontend generate

launch:
	./start-power-mine.sh

dev:
	$(WAILS) dev

build:
	$(WAILS) build

appimage:
	WAILS="$(WAILS)" ./scripts/build-appimage.sh

agent:
	./scripts/build-agent.sh fabric-1.20.1

agent-forge:
	./scripts/build-agent.sh forge-1.7.10

agent-all:
	./scripts/build-agent.sh all

test: test-go test-frontend

test-go:
	go test ./...

test-frontend:
	npm --prefix frontend run build

generate:
	$(WAILS) generate module
