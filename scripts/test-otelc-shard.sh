#!/bin/sh
set -eu

if [ "$#" -ne 1 ]; then
  echo 'usage: test-otelc-shard.sh a-m|remaining' >&2
  exit 2
fi
case "$1" in
  a-m) selection=-run ;;
  remaining) selection=-skip ;;
  *) echo 'unknown OTelC test shard' >&2; exit 2 ;;
esac
: "${OTELPLAN_OTELC:?OTELPLAN_OTELC must name the pinned backend}"

go test -timeout=20m "$selection" '^Test[A-M]' ./...
go test -race -timeout=20m "$selection" '^Test[A-M]' ./...
