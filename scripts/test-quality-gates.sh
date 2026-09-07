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
