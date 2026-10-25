#!/bin/sh
set -eu

EXPECTED_LINT_VERSION=$1
export EXPECTED_LINT_VERSION
scratch=$(mktemp -d)
trap 'rm -rf "$scratch"' EXIT HUP INT TERM
cat > "$scratch/golangci-lint" <<'TOOL'
#!/bin/sh
case "$1" in
  version) printf '%s\n' "$EXPECTED_LINT_VERSION" ;;
  run) exit "${FAKE_LINT_EXIT:-0}" ;;
  *) exit 99 ;;
esac
TOOL
chmod +x "$scratch/golangci-lint"
PATH="$scratch:$PATH"
export PATH
make --no-print-directory -s lint GOLANGCI_LINT="$scratch/golangci-lint" > "$scratch/output" 2>&1
if FAKE_LINT_EXIT=42 make --no-print-directory -s lint GOLANGCI_LINT="$scratch/golangci-lint" > "$scratch/output" 2>&1; then
  echo 'lint swallowed a tool failure' >&2
  exit 1
fi
if make --no-print-directory -s lint GOLANGCI_LINT="$scratch/missing" > "$scratch/output" 2>&1; then
  echo 'lint accepted a missing tool' >&2
  exit 1
fi
if EXPECTED_LINT_VERSION=incorrect make --no-print-directory -s lint GOLANGCI_LINT="$scratch/golangci-lint" > "$scratch/output" 2>&1; then
  echo 'lint accepted an unexpected tool version' >&2
  exit 1
fi

cat > "$scratch/go" <<'TOOL'
#!/bin/sh
printf '%s\n' "$*" >> "$FAKE_GO_LOG"
if [ "${FAKE_GO_FAIL:-}" = all ]; then
  exit 42
fi
if [ "${FAKE_GO_FAIL:-}" = race ] && [ "${2:-}" = -race ]; then
  exit 43
fi
TOOL
chmod +x "$scratch/go"
FAKE_GO_LOG="$scratch/go.calls"
export FAKE_GO_LOG
OTELPLAN_OTELC=fake sh scripts/test-otelc-shard.sh a-m
OTELPLAN_OTELC=fake sh scripts/test-otelc-shard.sh remaining
cat > "$scratch/expected.calls" <<'CALLS'
test -timeout=20m -run ^Test[A-M] ./...
test -race -timeout=20m -run ^Test[A-M] ./...
test -timeout=20m -skip ^Test[A-M] ./...
test -race -timeout=20m -skip ^Test[A-M] ./...
CALLS
cmp "$scratch/go.calls" "$scratch/expected.calls"
for shard in a-m remaining; do
  for failure in all race; do
    if OTELPLAN_OTELC=fake FAKE_GO_FAIL="$failure" sh scripts/test-otelc-shard.sh "$shard" > "$scratch/output" 2>&1; then
      echo 'OTelC shard swallowed a test failure' >&2
      exit 1
    fi
  done
done
if OTELPLAN_OTELC= sh scripts/test-otelc-shard.sh a-m > "$scratch/output" 2>&1; then
  echo 'OTelC shard accepted a missing backend' >&2
  exit 1
fi
if OTELPLAN_OTELC=fake sh scripts/test-otelc-shard.sh unknown > "$scratch/output" 2>&1; then
  echo 'OTelC shard accepted an unknown selection' >&2
  exit 1
fi
