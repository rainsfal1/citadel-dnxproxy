BIN_DIR := bin
APP := dnsproxy

.PHONY: all build test clean validate-config

all: build

build:
	go build -o $(BIN_DIR)/$(APP) ./cmd/dnsproxy

test:
	GOCACHE=$(PWD)/.cache/gobuild go test ./...

validate-config:
	@CONFIGS=$${CONFIG:-$(shell ls configs/*.json | grep -v schema.json)}; \
	go run ./cmd/validateconfig -schema configs/schema.json $$CONFIGS

clean:
	rm -rf $(BIN_DIR) .cache/gobuild
