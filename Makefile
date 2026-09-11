WAILS ?= wails

.PHONY: launch dev build appimage deb rpm windows agent test test-go test-frontend generate

launch:
	./start-power-mine.sh

dev:
	$(WAILS) dev

build:
	$(WAILS) build

appimage:
	WAILS="$(WAILS)" ./scripts/build-appimage.sh

deb:
	WAILS="$(WAILS)" ./scripts/build-deb.sh

rpm:
	WAILS="$(WAILS)" ./scripts/build-rpm.sh

windows:
	pwsh -NoProfile -ExecutionPolicy Bypass -File ./scripts/build-windows.ps1

agent:
	./scripts/build-agent.sh

test: test-go test-frontend

test-go:
	go test ./...

test-frontend:
	npm --prefix frontend run build

generate:
	$(WAILS) generate module
