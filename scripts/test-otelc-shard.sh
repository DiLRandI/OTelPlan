#!/bin/sh
set -eu

if [ "$#" -ne 1 ]; then
  echo 'usage: test-otelc-shard.sh a-m|remaining|architecture|cli-build' >&2
  exit 2
fi
architecture='^TestArchitectureTracesWithPinnedBackend$'
cli_build='^TestBuildCLI(WithPinnedBackend|LockedWithPinnedBackend|LibraryWithoutOutput|FromWorkspaceRoot)$'
lock_fifo='^TestLockCommandsRejectFIFOWithoutMutation$'
: "${OTELPLAN_OTELC:?OTELPLAN_OTELC must name the pinned backend}"

run_tests() {
  go test -v -count=1 -timeout=20m "$@"
  go test -race -v -count=1 -timeout=20m "$@"
}

case "$1" in
  a-m)
    # Keep the FIFO deadline independent of concurrent Go package loading.
    run_tests -parallel=1 -run "$lock_fifo" ./internal/cli
    set -- -run '^Test[A-M]' -skip "$architecture|$cli_build|$lock_fifo" ./...
    ;;
  remaining) set -- -skip '^Test[A-M]' ./... ;;
  architecture) set -- -run "$architecture" ./internal/cli ;;
  cli-build) set -- -run "$cli_build" ./internal/cli ;;
  *) echo 'unknown OTelC test shard' >&2; exit 2 ;;
esac

run_tests "$@"
