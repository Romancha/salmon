.PHONY: build build-xcall build-app test test-coverage test-race test-xcall test-app lint fmt tidy clean generate tools swagger help all install-bridge uninstall-bridge verify-bridge

BINARY_HUB=bear-sync-hub
BINARY_BRIDGE=bear-bridge

# Version for ldflags injection (default: dev, override with make build VERSION=v1.0.0)
VERSION ?= dev

# Code signing identity for bear-xcall.app (use "Developer ID Application: ..." for distribution)
CODESIGN_IDENTITY ?= -

# Requirement for verifying Developer ID Application signatures (release archives).
# Checks Apple root CA → Developer ID CA intermediate → Developer ID Application leaf.
DEVID_REQ = anchor apple generic and certificate 1[field.1.2.840.113635.100.6.2.6] \
  and certificate leaf[field.1.2.840.113635.100.6.1.13]

# Bridge install paths
PLIST_LABEL=com.romancha.bear-bridge
PLIST_DST=$(HOME)/Library/LaunchAgents/$(PLIST_LABEL).plist
BRIDGE_BIN_DIR=$(HOME)/bin
BRIDGE_LOG_DIR=$(HOME)/Library/Logs/bear-bridge
BRIDGE_CONFIG_DIR=$(HOME)/.config/bear-bridge

# Detect release archive vs repository context.
# In a release archive there is no go.mod — binaries and configs are at the root.
ifneq ($(wildcard go.mod),)
# Repository context — build from source
INSTALL_BRIDGE_DEPS = build
BRIDGE_SRC_BIN = bin/$(BINARY_BRIDGE)
XCALL_SRC_APP = bin/bear-xcall.app
PLIST_SRC = deploy/$(PLIST_LABEL).plist
WRAPPER_SRC = deploy/bear-bridge-wrapper.sh
ENV_EXAMPLE_SRC = deploy/.env.bridge.example
ENTITLEMENTS_SRC = tools/bear-xcall/entitlements.plist
IS_RELEASE_ARCHIVE = 0
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
else
# Release archive context — pre-built signed binaries
INSTALL_BRIDGE_DEPS =
BRIDGE_SRC_BIN = $(BINARY_BRIDGE)
XCALL_SRC_APP = bear-xcall.app
PLIST_SRC = $(PLIST_LABEL).plist
WRAPPER_SRC = bear-bridge-wrapper.sh
ENV_EXAMPLE_SRC = .env.bridge.example
ENTITLEMENTS_SRC = entitlements.plist
IS_RELEASE_ARCHIVE = 1
endif

# Default target
ifneq ($(wildcard go.mod),)
all: test build
else
all: install-bridge
endif

# Help
help:
	@echo "Usage:"
	@echo ""
	@echo "  Build:"
	@echo "    make build          - Build all binaries to bin/ (includes bear-xcall on macOS)"
	@echo "    make build-xcall    - Build bear-xcall Swift CLI .app bundle (macOS only)"
	@echo "    make build-app      - Build BearBridge menu bar .app bundle (macOS only)"
	@echo "    make install-bridge - Install bridge + launchd agent to ~/bin/ (macOS only)"
	@echo "    make uninstall-bridge - Uninstall bridge + launchd agent (macOS only)"
	@echo "    make verify-bridge  - Verify installed bridge code signatures (macOS only)"
	@echo ""
	@echo "  Test:"
	@echo "    make test           - Run all Go tests"
	@echo "    make test-coverage  - Run tests with coverage report"
	@echo "    make test-race      - Run tests with race detector"
	@echo "    make test-xcall     - Run bear-xcall manual tests (macOS + Bear)"
	@echo "    make test-app       - Run BearBridge Swift tests (macOS only)"
	@echo ""
	@echo "  Tools:"
	@echo "    make lint           - Run golangci-lint"
	@echo "    make fmt            - Format code (gofumpt + goimports)"
	@echo "    make tidy           - go mod tidy"
	@echo "    make clean          - Clean build artifacts"
	@echo "    make generate       - Run go generate (moq)"
	@echo "    make swagger        - Generate Swagger docs (swag init)"
	@echo "    make tools          - Install dev tools"

build: build-xcall
	@echo "Building $(BINARY_HUB) and $(BINARY_BRIDGE)..."
	go build -o bin/$(BINARY_HUB) ./cmd/hub
	go build -ldflags "-X main.version=$(VERSION)" -o bin/$(BINARY_BRIDGE) ./cmd/bridge

build-xcall:
ifeq ($(shell uname),Darwin)
	@echo "Building bear-xcall .app bundle..."
	@mkdir -p bin/bear-xcall.app/Contents/MacOS
	swiftc -o bin/bear-xcall.app/Contents/MacOS/bear-xcall tools/bear-xcall/main.swift
	cp tools/bear-xcall/Info.plist bin/bear-xcall.app/Contents/
else
	@echo "Skipping bear-xcall build (macOS only)"
endif

build-app:
ifeq ($(shell uname),Darwin)
	@echo "Building BearBridge.app..."
	cd tools/bear-bridge-app && swift build -c release
	@mkdir -p bin/BearBridge.app/Contents/MacOS
	cp tools/bear-bridge-app/.build/release/BearBridge bin/BearBridge.app/Contents/MacOS/
	cp tools/bear-bridge-app/Info.plist bin/BearBridge.app/Contents/
else
	@echo "Skipping BearBridge.app build (macOS only)"
endif

test-app:
ifeq ($(shell uname),Darwin)
	@echo "Running BearBridge Swift tests..."
	cd tools/bear-bridge-app && swift test
else
	@echo "Skipping BearBridge tests (macOS only)"
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

install-bridge: $(INSTALL_BRIDGE_DEPS)
ifeq ($(shell uname),Darwin)
	@echo "Installing bear-bridge to $(BRIDGE_BIN_DIR)..."
	@launchctl bootout gui/$$(id -u)/$(PLIST_LABEL) 2>/dev/null || true
	@mkdir -p $(BRIDGE_BIN_DIR)
	@mkdir -p $(BRIDGE_LOG_DIR)
	@mkdir -p $(BRIDGE_CONFIG_DIR)
ifeq ($(IS_RELEASE_ARCHIVE),1)
	@if codesign --verify --deep --strict -R '$(DEVID_REQ)' $(BRIDGE_SRC_BIN) 2>/dev/null && \
	    codesign --verify --deep --strict -R '$(DEVID_REQ)' $(XCALL_SRC_APP) 2>/dev/null; then \
		echo "Code signatures valid (Developer ID)"; \
	else \
		echo "ERROR: Code signature verification failed."; \
		echo "Binaries must be signed with a Developer ID Application certificate."; \
		echo "The release archive may be corrupted or tampered with."; \
		echo "Please re-download from GitHub Releases."; \
		exit 1; \
	fi
endif
	cp $(BRIDGE_SRC_BIN) $(BRIDGE_BIN_DIR)/
	cp -R $(XCALL_SRC_APP) $(BRIDGE_BIN_DIR)/
ifeq ($(IS_RELEASE_ARCHIVE),0)
	codesign --force --deep --sign "$(CODESIGN_IDENTITY)" --entitlements $(ENTITLEMENTS_SRC) --options runtime $(BRIDGE_BIN_DIR)/bear-xcall.app
endif
	cp $(WRAPPER_SRC) $(BRIDGE_BIN_DIR)/bear-bridge-wrapper.sh
	chmod +x $(BRIDGE_BIN_DIR)/bear-bridge-wrapper.sh
	@if [ ! -f $(BRIDGE_CONFIG_DIR)/.env.bridge ]; then \
		cp $(ENV_EXAMPLE_SRC) $(BRIDGE_CONFIG_DIR)/.env.bridge; \
		echo "Created $(BRIDGE_CONFIG_DIR)/.env.bridge from template"; \
	else \
		echo "$(BRIDGE_CONFIG_DIR)/.env.bridge already exists, skipping"; \
	fi
	@echo "Installing launchd plist to $(PLIST_DST)..."
	sed 's|__HOME__|$(HOME)|g' $(PLIST_SRC) > $(PLIST_DST)
	@echo "Loading launchd agent..."
	launchctl bootstrap gui/$$(id -u) $(PLIST_DST)
	@echo ""
	@echo "Installed. Edit your config:"
	@echo "  nano $(BRIDGE_CONFIG_DIR)/.env.bridge"
	@echo ""
	@echo "Then reload the agent:"
	@echo "  launchctl bootout gui/$$(id -u)/$(PLIST_LABEL)"
	@echo "  launchctl bootstrap gui/$$(id -u) $(PLIST_DST)"
else
	@echo "install-bridge is macOS only"
	@exit 1
endif

uninstall-bridge:
ifeq ($(shell uname),Darwin)
	@echo "Unloading launchd agent..."
	@launchctl bootout gui/$$(id -u)/$(PLIST_LABEL) 2>/dev/null || true
	@echo "Removing plist..."
	rm -f $(PLIST_DST)
	@echo "Removing binaries from $(BRIDGE_BIN_DIR)..."
	rm -f $(BRIDGE_BIN_DIR)/$(BINARY_BRIDGE)
	rm -rf $(BRIDGE_BIN_DIR)/bear-xcall.app
	rm -f $(BRIDGE_BIN_DIR)/bear-bridge-wrapper.sh
	@echo ""
	@echo "Uninstalled. The following were NOT removed (manual cleanup if desired):"
	@echo "  $(BRIDGE_CONFIG_DIR)/.env.bridge"
	@echo "  $(HOME)/.bear-bridge-state.json"
	@echo "  $(BRIDGE_LOG_DIR)/"
else
	@echo "uninstall-bridge is macOS only"
	@exit 1
endif

verify-bridge:
ifeq ($(shell uname),Darwin)
	@echo "Verifying bear-bridge signature..."
	codesign --verify --deep --strict --verbose=2 $(BRIDGE_BIN_DIR)/$(BINARY_BRIDGE)
	@echo ""
	@echo "Verifying bear-xcall.app signature..."
	codesign --verify --deep --strict --verbose=2 $(BRIDGE_BIN_DIR)/bear-xcall.app
	@echo ""
	@echo "All signatures valid."
else
	@echo "verify-bridge is macOS only"
	@exit 1
endif
