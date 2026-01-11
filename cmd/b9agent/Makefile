# b9agent Makefile
# Build and install the Beta9 external worker agent

VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "0.2.0")
BUILD_TIME := $(shell date -u '+%Y-%m-%d_%H:%M:%S')
LDFLAGS := -ldflags "-X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME)"

# Binary name
BINARY := b9agent

# Installation directory
PREFIX ?= /usr/local
BINDIR := $(PREFIX)/bin

# Go parameters
GOCMD := go
GOBUILD := $(GOCMD) build
GOTEST := $(GOCMD) test
GOCLEAN := $(GOCMD) clean
GOMOD := $(GOCMD) mod

# Source directory (relative to beta9 root)
BETA9_ROOT := ../..
PKG_DIR := $(BETA9_ROOT)/pkg/agent
CMD_DIR := .

# Build targets
.PHONY: all build build-linux build-darwin build-darwin-arm64 clean install uninstall test help

all: build

## build: Build for current platform
build:
	@echo "Building $(BINARY) $(VERSION)..."
	cd $(BETA9_ROOT) && $(GOBUILD) $(LDFLAGS) -o $(CURDIR)/$(BINARY) ./cmd/b9agent/
	@echo "Built: $(BINARY)"

## build-linux: Build for Linux amd64
build-linux:
	@echo "Building $(BINARY) for Linux amd64..."
	cd $(BETA9_ROOT) && GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(CURDIR)/$(BINARY)-linux-amd64 ./cmd/b9agent/
	@echo "Built: $(BINARY)-linux-amd64"

## build-darwin: Build for macOS amd64
build-darwin:
	@echo "Building $(BINARY) for macOS amd64..."
	cd $(BETA9_ROOT) && GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(CURDIR)/$(BINARY)-darwin-amd64 ./cmd/b9agent/
	@echo "Built: $(BINARY)-darwin-amd64"

## build-darwin-arm64: Build for macOS arm64 (Apple Silicon)
build-darwin-arm64:
	@echo "Building $(BINARY) for macOS arm64..."
	cd $(BETA9_ROOT) && GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(CURDIR)/$(BINARY)-darwin-arm64 ./cmd/b9agent/
	@echo "Built: $(BINARY)-darwin-arm64"

## build-all: Build for all platforms
build-all: build-linux build-darwin build-darwin-arm64

## install: Install to /usr/local/bin (requires sudo on most systems)
install: build
	@echo "Installing $(BINARY) to $(BINDIR)..."
	@mkdir -p $(BINDIR)
	@install -m 755 $(BINARY) $(BINDIR)/$(BINARY)
	@echo "Installed: $(BINDIR)/$(BINARY)"
	@echo ""
	@echo "Run 'b9agent init' to configure the agent"

## uninstall: Remove from /usr/local/bin
uninstall:
	@echo "Removing $(BINARY) from $(BINDIR)..."
	@rm -f $(BINDIR)/$(BINARY)
	@echo "Uninstalled"

## test: Run tests
test:
	@echo "Running tests..."
	cd $(BETA9_ROOT) && $(GOTEST) -v ./pkg/agent/...

## test-coverage: Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	cd $(BETA9_ROOT) && $(GOTEST) -v -coverprofile=coverage.out ./pkg/agent/...
	cd $(BETA9_ROOT) && $(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: $(BETA9_ROOT)/coverage.html"

## clean: Remove build artifacts
clean:
	@echo "Cleaning..."
	@rm -f $(BINARY) $(BINARY)-*
	@echo "Clean"

## version: Show version
version:
	@echo $(VERSION)

## help: Show this help
help:
	@echo "b9agent - Beta9 External Worker Agent"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## /  /'
	@echo ""
	@echo "Examples:"
	@echo "  make build          # Build for current platform"
	@echo "  make install        # Build and install to /usr/local/bin"
	@echo "  make test           # Run tests"
	@echo "  make build-all      # Build for all platforms"
