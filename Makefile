.PHONY: build test test-race vet fmt check lint test-gates clean

BIN_DIR := bin
GOLANGCI_LINT ?= golangci-lint
GOLANGCI_LINT_VERSION := 2.13.2

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
	@command -v "$(GOLANGCI_LINT)" >/dev/null 2>&1 || { echo "golangci-lint $(GOLANGCI_LINT_VERSION) is required" >&2; exit 1; }
	@test "$$("$(GOLANGCI_LINT)" version --short)" = "$(GOLANGCI_LINT_VERSION)" || { echo "expected golangci-lint $(GOLANGCI_LINT_VERSION)" >&2; exit 1; }
	"$(GOLANGCI_LINT)" run ./...

test-gates:
	@sh scripts/test-quality-gates.sh "$(GOLANGCI_LINT_VERSION)"

check: test test-race vet lint test-gates

clean:
	rm -rf $(BIN_DIR) .otelplan
