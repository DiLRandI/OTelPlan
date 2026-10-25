.PHONY: build test test-race vet fmt check lint vuln test-gates clean

BIN_DIR := bin
GOLANGCI_LINT ?= ./bin/golangci-lint
GOLANGCI_LINT_VERSION := 2.14.0
GOVULNCHECK ?= ./bin/govulncheck
GOVULNCHECK_VERSION := v1.9.0

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

init-lint:
	curl -sSfL https://golangci-lint.run/install.sh | sh -s v2.14.0

lint:
	@command -v "$(GOLANGCI_LINT)" >/dev/null 2>&1 || { echo "golangci-lint $(GOLANGCI_LINT_VERSION) is required" >&2; exit 1; }
	@test "$$("$(GOLANGCI_LINT)" version --short)" = "$(GOLANGCI_LINT_VERSION)" || { echo "expected golangci-lint $(GOLANGCI_LINT_VERSION)" >&2; exit 1; }
	"$(GOLANGCI_LINT)" run ./...

vuln:
	@command -v "$(GOVULNCHECK)" >/dev/null 2>&1 || { echo "govulncheck $(GOVULNCHECK_VERSION) is required" >&2; exit 1; }
	@output="$$("$(GOVULNCHECK)" -version 2>&1)" || { echo "cannot verify govulncheck version" >&2; exit 1; }; \
	version="$$(printf '%s\n' "$$output" | sed -n 's/^Scanner: //p')"; \
	test "$$version" = "govulncheck@$(GOVULNCHECK_VERSION)" || { echo "expected govulncheck $(GOVULNCHECK_VERSION)" >&2; exit 1; }
	"$(GOVULNCHECK)" -test ./...
	"$(GOVULNCHECK)" -C cmd/otelplan -scan=module

test-gates:
	@sh scripts/test-quality-gates.sh "$(GOLANGCI_LINT_VERSION)"

check: test test-race vet lint test-gates

clean:
	rm -rf $(BIN_DIR) .otelplan
