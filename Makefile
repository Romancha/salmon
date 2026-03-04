.PHONY: build test test-coverage test-race test-xcall lint fmt tidy clean generate tools help all

BINARY_HUB=bear-sync-hub
BINARY_BRIDGE=bear-bridge

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

# Default target
all: test build

# Help
help:
	@echo "Usage:"
	@echo "  make build          - Build both binaries to bin/"
	@echo "  make test           - Run all tests"
	@echo "  make test-coverage  - Run tests with coverage report"
	@echo "  make test-race      - Run tests with race detector"
	@echo "  make lint           - Run golangci-lint"
	@echo "  make fmt            - Format code (gofumpt + goimports)"
	@echo "  make tidy           - go mod tidy"
	@echo "  make clean          - Clean build artifacts"
	@echo "  make generate       - Run go generate (moq)"
	@echo "  make test-xcall     - Run bear-xcall manual tests (macOS + Bear)"
	@echo "  make tools          - Install dev tools"

build:
	@echo "Building $(BINARY_HUB) and $(BINARY_BRIDGE)..."
	go build -o bin/$(BINARY_HUB) ./cmd/hub
	go build -o bin/$(BINARY_BRIDGE) ./cmd/bridge

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

test-xcall:
ifeq ($(shell uname),Darwin)
	@echo "Building bear-xcall tests..."
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

generate:
	@echo "Generating mocks..."
	go generate ./...

# Install dev tools
tools:
	@echo "Installing dev tools..."
	go install github.com/matryer/moq@v0.6.0
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.7.2
	go install mvdan.cc/gofumpt@latest
	go install golang.org/x/tools/cmd/goimports@latest
