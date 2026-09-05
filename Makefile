.PHONY: build test test-race vet fmt check lint clean

BIN_DIR := bin

build:
	go build -o $(BIN_DIR)/otelplan ./cmd/otelplan

test:
	go test ./...

test-race:
	go test -race ./...

vet:
	go vet ./...

fmt:
	gofmt -w $$(find . -name '*.go' -type f)

lint:
	golangci-lint run ./... || echo "golangci-lint not installed; skipping"

check: test test-race vet

clean:
	rm -rf $(BIN_DIR) .otelplan