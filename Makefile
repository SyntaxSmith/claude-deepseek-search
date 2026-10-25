GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)
VERSION ?= 0.2.1
BUILD_DIR := $(CURDIR)/build
DIST_DIR := $(CURDIR)/dist
PLUGIN_EXT := $(if $(filter darwin,$(GOOS)),dylib,$(if $(filter windows,$(GOOS)),dll,so))
PLUGIN := $(BUILD_DIR)/claude-deepseek-search.$(PLUGIN_EXT)
PACKAGE_NAME := claude-deepseek-search_$(VERSION)_$(GOOS)_$(GOARCH)
PACKAGE_DIR := $(DIST_DIR)/$(PACKAGE_NAME)

.PHONY: build package test fmt clean

build:
	mkdir -p "$(BUILD_DIR)"
	cd go && CGO_ENABLED=1 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -trimpath -ldflags "-X main.pluginVersion=$(VERSION)" -buildmode=c-shared -o "$(PLUGIN)" .
	rm -f "$(BUILD_DIR)/claude-deepseek-search.h"

package: build
	mkdir -p "$(PACKAGE_DIR)/$(GOOS)/$(GOARCH)"
	cp "$(PLUGIN)" "$(PACKAGE_DIR)/$(GOOS)/$(GOARCH)/"
	cp README.md RELEASE_NOTES.md LICENSE NOTICE.md config.example.yaml "$(PACKAGE_DIR)/"
	tar -C "$(DIST_DIR)" -czf "$(DIST_DIR)/$(PACKAGE_NAME).tar.gz" "$(PACKAGE_NAME)"

test:
	cd go && go test ./...

fmt:
	gofmt -w go/*.go

clean:
	rm -rf "$(BUILD_DIR)" "$(DIST_DIR)"
