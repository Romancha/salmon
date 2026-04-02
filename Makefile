.PHONY: build build-mcp build-xcall build-app test test-coverage test-race test-xcall test-app lint fmt tidy clean generate tools swagger help all dmg

BINARY_HUB=salmon-hub
BINARY_BRIDGE=salmon-run
BINARY_MCP=salmon-mcp

# Version for ldflags injection (default: dev, override with make build VERSION=v1.0.0)
VERSION ?= dev

# Code signing identity for bear-xcall.app (use "Developer ID Application: ..." for distribution)
CODESIGN_IDENTITY ?= -

ENTITLEMENTS_SRC = tools/bear-xcall/entitlements.plist

# Go tools path
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif
# Tool binaries
GOLANGCI_LINT=$(GOBIN)/golangci-lint
GOFUMPT=$(GOBIN)/gofumpt
GOIMPORTS=$(GOBIN)/goimports
MOQ=$(GOBIN)/moq
SWAG=$(GOBIN)/swag

# Default target
all: test build

# Help
help:
	@echo "Usage:"
	@echo ""
	@echo "  Build:"
	@echo "    make build          - Build all binaries to bin/ (includes bear-xcall on macOS)"
	@echo "    make build-mcp      - Build salmon-mcp MCP server binary"
	@echo "    make build-xcall    - Build bear-xcall Swift CLI .app bundle (macOS only)"
	@echo "    make build-app      - Build SalmonRun menu bar .app bundle (macOS only)"
	@echo "    make dmg            - Create SalmonRun .dmg disk image (macOS only)"
	@echo ""
	@echo "  Test:"
	@echo "    make test           - Run all Go tests"
	@echo "    make test-coverage  - Run tests with coverage report"
	@echo "    make test-race      - Run tests with race detector"
	@echo "    make test-xcall     - Run bear-xcall manual tests (macOS + Bear)"
	@echo "    make test-app       - Run SalmonRun Swift tests (macOS only)"
	@echo ""
	@echo "  Tools:"
	@echo "    make lint           - Run golangci-lint"
	@echo "    make fmt            - Format code (gofumpt + goimports)"
	@echo "    make tidy           - go mod tidy"
	@echo "    make clean          - Clean build artifacts"
	@echo "    make generate       - Run go generate (moq)"
	@echo "    make swagger        - Generate Swagger docs (swag init)"
	@echo "    make tools          - Install dev tools"

build: build-xcall build-mcp
	@echo "Building $(BINARY_HUB), $(BINARY_BRIDGE), and $(BINARY_MCP)..."
	go build -o bin/$(BINARY_HUB) ./cmd/hub
	go build -ldflags "-X main.version=$(VERSION)" -o bin/$(BINARY_BRIDGE) ./cmd/bridge

build-mcp:
	@echo "Building $(BINARY_MCP)..."
	go build -o bin/$(BINARY_MCP) ./cmd/mcp

build-xcall:
ifeq ($(shell uname),Darwin)
	@echo "Building bear-xcall .app bundle..."
	@mkdir -p bin/bear-xcall.app/Contents/MacOS
	swiftc -o bin/bear-xcall.app/Contents/MacOS/bear-xcall tools/bear-xcall/main.swift
	cp tools/bear-xcall/Info.plist bin/bear-xcall.app/Contents/
else
	@echo "Skipping bear-xcall build (macOS only)"
endif

build-app: build build-xcall
ifeq ($(shell uname),Darwin)
	@echo "Building SalmonRun.app..."
	xcodebuild -project tools/salmon-run-app/SalmonRun.xcodeproj \
		-scheme SalmonRun -configuration Release \
		CONFIGURATION_BUILD_DIR=$(CURDIR)/bin/xcodebuild-out \
		CODE_SIGN_IDENTITY="$(CODESIGN_IDENTITY)" \
		MACOSX_DEPLOYMENT_TARGET=14.0 \
		build
	@mkdir -p bin/SalmonRun.app/Contents/MacOS
	cp -R bin/xcodebuild-out/SalmonRun.app/ bin/SalmonRun.app/
	rm -rf bin/xcodebuild-out
	cp bin/$(BINARY_BRIDGE) bin/SalmonRun.app/Contents/MacOS/$(BINARY_BRIDGE)
	cp -R bin/bear-xcall.app bin/SalmonRun.app/Contents/MacOS/
	codesign --force --sign "$(CODESIGN_IDENTITY)" --entitlements $(ENTITLEMENTS_SRC) --options runtime bin/SalmonRun.app/Contents/MacOS/bear-xcall.app
	codesign --force --sign "$(CODESIGN_IDENTITY)" --options runtime bin/SalmonRun.app/Contents/MacOS/$(BINARY_BRIDGE)
	codesign --force --sign "$(CODESIGN_IDENTITY)" --options runtime bin/SalmonRun.app
else
	@echo "Skipping SalmonRun.app build (macOS only)"
endif

dmg: build-app
ifeq ($(shell uname),Darwin)
	@echo "Creating SalmonRun .dmg..."
	./tools/create-dmg.sh bin/SalmonRun.app bin/SalmonRun.dmg
else
	@echo "Skipping .dmg creation (macOS only)"
endif

test-app:
ifeq ($(shell uname),Darwin)
	@echo "Running SalmonRun Swift tests..."
	xcodebuild -project tools/salmon-run-app/SalmonRun.xcodeproj \
		-scheme SalmonRun \
		MACOSX_DEPLOYMENT_TARGET=14.0 \
		test
else
	@echo "Skipping SalmonRun tests (macOS only)"
endif

test:
	@echo "Running tests..."
	go test ./...

test-coverage:
	@echo "Running tests with coverage..."
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

test-race:
	@echo "Running tests with race detection..."
	go test -race -timeout=60s -count 1 ./...

test-xcall: build-xcall
ifeq ($(shell uname),Darwin)
	@echo "Building bear-xcall tests..."
	@mkdir -p bin
	swiftc -o bin/bear-xcall-tests tools/bear-xcall/BearXcallTests.swift
	@echo "Running bear-xcall tests..."
	bin/bear-xcall-tests
else
	@echo "Skipping bear-xcall tests (macOS only)"
endif

lint:
	@echo "Running linter..."
	$(GOLANGCI_LINT) run ./...

fmt:
	@echo "Formatting code..."
	$(GOFUMPT) -l -w .
	$(GOIMPORTS) -l -w .

tidy:
	@echo "Tidying modules..."
	go mod tidy

clean:
	@echo "Cleaning..."
	rm -rf bin/ coverage.out coverage.html
	go clean
	@echo "Cleaned bin/, coverage files"

generate:
	@echo "Generating mocks..."
	go generate ./...

swagger:
	@echo "Generating Swagger docs..."
	$(SWAG) init -g cmd/hub/main.go --output internal/api/docs --parseInternal

# Install dev tools
tools:
	@echo "Installing dev tools..."
	go install github.com/matryer/moq@v0.6.0
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.7.2
	go install mvdan.cc/gofumpt@latest
	go install golang.org/x/tools/cmd/goimports@latest
	go install github.com/swaggo/swag/cmd/swag@v1.16.6

