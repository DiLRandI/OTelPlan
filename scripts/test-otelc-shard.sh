#!/bin/sh
set -eu

if [ "$#" -ne 1 ]; then
  echo 'usage: test-otelc-shard.sh a-m|remaining|architecture|cli-build' >&2
  exit 2
fi
architecture='^TestArchitectureTracesWithPinnedBackend$'
cli_build='^TestBuildCLI(WithPinnedBackend|LockedWithPinnedBackend|LibraryWithoutOutput|FromWorkspaceRoot)$'
case "$1" in
  a-m) set -- -run '^Test[A-M]' -skip "$architecture|$cli_build" ./... ;;
  remaining) set -- -skip '^Test[A-M]' ./... ;;
  architecture) set -- -run "$architecture" ./internal/cli ;;
  cli-build) set -- -run "$cli_build" ./internal/cli ;;
  *) echo 'unknown OTelC test shard' >&2; exit 2 ;;
esac
: "${OTELPLAN_OTELC:?OTELPLAN_OTELC must name the pinned backend}"

go test -v -count=1 -timeout=20m "$@"
go test -race -v -count=1 -timeout=20m "$@"
