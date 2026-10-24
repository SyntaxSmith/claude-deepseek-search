GOOS ?= darwin
GOARCH ?= arm64
BUILD_DIR := $(CURDIR)/build
PLUGIN := $(BUILD_DIR)/claude-deepseek-search.dylib

.PHONY: build test clean fmt

build:
	mkdir -p $(BUILD_DIR)
	cd go && GOOS=$(GOOS) GOARCH=$(GOARCH) go build -buildmode=c-shared -o $(PLUGIN) .
	rm -f $(BUILD_DIR)/claude-deepseek-search.h

test:
	cd go && go test ./...

fmt:
	gofmt -w go/*.go

clean:
	rm -rf $(BUILD_DIR)
