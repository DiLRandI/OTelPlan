#!/bin/sh
set -eu

if [ "$#" -ne 1 ]; then
  echo 'usage: check-pinned-release-dependencies.sh PINNED_OTELC_SOURCE' >&2
  exit 2
fi

backend=$(CDPATH= cd -- "$1" && pwd -P)
project=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)
scanner=${GOVULNCHECK:-govulncheck}
pinned_revision=449ee08a682586adb177e4402845ed404565879f
if [ "$(git -C "$backend" rev-parse HEAD)" != "$pinned_revision" ] ||
   [ -n "$(git -C "$backend" status --porcelain)" ]; then
  echo 'expected a clean checkout of the pinned OTelC source' >&2
  exit 1
fi

scratch=$(mktemp -d)
trap 'rm -rf -- "$scratch"' EXIT
trap 'exit 1' HUP INT TERM

export GOENV=off GOWORK=off GOFLAGS= GOTOOLCHAIN=local
"$scanner" -C "$backend/tool/cmd/otelc" -scan=module

mkdir "$scratch/bin" "$scratch/application"
(
  cd "$backend"
  go build -mod=readonly -trimpath -ldflags '-X go.opentelemetry.io/otelc/tool/util.Version=v1.1.0' \
    -o "$scratch/bin/otelc" ./tool/cmd/otelc
)
(
  cd "$project"
  go build -mod=readonly -o "$scratch/bin/otelplan" ./cmd/otelplan
)

fixture="$project/internal/cli/testdata/architectures"
cp -R "$fixture/common/." "$scratch/application"
cp -R "$fixture/package/." "$scratch/application"
PATH="$scratch/bin:$PATH" "$scratch/bin/otelplan" compile --root "$scratch/application" --output "$scratch/runtime"
"$scanner" -C "$scratch/runtime/hooks" -scan=module
