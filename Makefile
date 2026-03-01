.PHONY: build build-local test test-coverage test-race lint fmt tidy clean generate

BINARY_HUB=bear-sync-hub
BINARY_BRIDGE=bear-bridge

build:
	go build -o bin/$(BINARY_HUB) ./cmd/hub
	go build -o bin/$(BINARY_BRIDGE) ./cmd/bridge

build-local: build

test:
	go test ./...

test-coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

test-race:
	go test -race ./...

lint:
	golangci-lint run ./...

fmt:
	gofumpt -l -w .
	goimports -l -w .

tidy:
	go mod tidy

clean:
	rm -rf bin/ coverage.out coverage.html

generate:
	go generate ./...
