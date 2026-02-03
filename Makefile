# localsend-go Makefile

BINARY_NAME := localsend-go
DIST_DIR := dist
WEB_DIR := web
WEB_OUT := $(WEB_DIR)/out

.PHONY: all build build-go build-web run clean lint test web-dev install-web-deps help


all: build-go build-web


build: build-go

build-go:
	@mkdir -p $(DIST_DIR)
	go build -ldflags "-s -w -X main.buildTime=$$(date -u +%Y-%m-%dT%H:%M:%SZ)" -o $(DIST_DIR)/$(BINARY_NAME) .


build-web: install-web-deps
	cd $(WEB_DIR) && pnpm build


install-web-deps:
	cd $(WEB_DIR) && pnpm install --frozen-lockfile


run:
	go run .


web-dev: install-web-deps
	cd $(WEB_DIR) && pnpm dev


clean:
	rm -rf $(DIST_DIR) $(WEB_OUT)


lint:
	golangci-lint run
