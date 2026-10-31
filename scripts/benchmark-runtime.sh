#!/bin/sh
set -eu

if [ "$#" -gt 1 ]; then
    echo 'usage: benchmark-runtime.sh [positive sample count]' >&2
    exit 2
fi

samples=${1:-3}
case "$samples" in
    ''|*[!0-9]*|0*)
        echo 'sample count must be a positive integer' >&2
        exit 2
        ;;
esac

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repository=$(CDPATH= cd -- "$script_dir/.." && pwd)
fixture=$repository/internal/cli/testdata/runtime-benchmarks
backend=${OTELPLAN_OTELC:-}
if [ -z "$backend" ]; then
    backend=$(command -v otelc) || {
        echo 'pinned otelc v1.1.0 is required; set OTELPLAN_OTELC or PATH' >&2
        exit 1
    }
fi

case "$backend" in
    /*) ;;
    *) backend=$PWD/$backend ;;
esac
if [ ! -f "$backend" ] || [ ! -x "$backend" ]; then
    echo 'configured backend must be an executable file' >&2
    exit 1
fi

temporary=$(mktemp -d "${TMPDIR:-/tmp}/otelplan-runtime-bench.XXXXXXXX")
trap 'rm -rf "$temporary"' 0
trap 'exit 130' INT
trap 'exit 143' TERM

mkdir "$temporary/bin" "$temporary/app"
ln -s "$backend" "$temporary/bin/otelc"
cp -R "$fixture/." "$temporary/app/"
export PATH="$temporary/bin:$PATH" GOWORK=off GOFLAGS=

cd "$repository"
revision=$(git rev-parse HEAD)
tree=clean
if [ -n "$(git status --porcelain --untracked-files=normal)" ]; then
    tree=dirty
fi

printf '# revision: %s (%s worktree)\n' "$revision" "$tree"
go version
printf '# platform: %s\n' "$(uname -sm)"
case "$(uname -s)" in
    Linux) awk -F ': ' '/model name/ {print "# CPU: " $2; exit}' /proc/cpuinfo ;;
esac
printf '# samples per case: %s\n' "$samples"
printf '# backend version: %s\n' "$("$backend" version)"
if command -v sha256sum >/dev/null 2>&1; then
    checksum=$(sha256sum "$backend")
elif command -v shasum >/dev/null 2>&1; then
    checksum=$(shasum -a 256 "$backend")
else
    echo 'sha256sum or shasum is required to identify the backend' >&2
    exit 1
fi
printf '# backend SHA256: %s\n' "${checksum%% *}"

go build -mod=readonly -trimpath -ldflags "-X github.com/DiLRandI/OTelPlan/internal/cli.Version=$revision" \
    -o "$temporary/otelplan" ./cmd/otelplan

cd "$temporary/app"
find . -type f -exec cksum {} \; | LC_ALL=C sort > "$temporary/before"
go mod download
go test -race ./...
go build -mod=readonly -trimpath -o "$temporary/baseline" ./cmd/bench
"$temporary/otelplan" validate --root "$temporary/app" --strict >&2
"$temporary/otelplan" compile --root "$temporary/app" --output "$temporary/bundle" >&2
"$temporary/otelplan" build --root "$temporary/app" -- -trimpath -o "$temporary/instrumented" ./cmd/bench >&2
find . -type f -exec cksum {} \; | LC_ALL=C sort > "$temporary/after"
if ! cmp -s "$temporary/before" "$temporary/after"; then
    echo 'runtime benchmark modified fixture inputs' >&2
    exit 1
fi

"$temporary/baseline" --verify-only
"$temporary/instrumented" --instrumented --verify-only
"$temporary/baseline" --samples "$samples"
"$temporary/instrumented" --instrumented --samples "$samples"
