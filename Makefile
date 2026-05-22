WAILS ?= wails

.PHONY: launch dev build appimage test test-go test-frontend generate

launch:
	./start-power-mine.sh

dev:
	$(WAILS) dev

build:
	$(WAILS) build

appimage:
	WAILS="$(WAILS)" ./scripts/build-appimage.sh

test: test-go test-frontend

test-go:
	go test ./...

test-frontend:
	npm --prefix frontend run build

generate:
	$(WAILS) generate module
