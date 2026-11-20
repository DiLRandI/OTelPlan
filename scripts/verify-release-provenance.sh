#!/bin/sh
set -eu

if [ "$#" -ne 4 ]; then
  echo 'usage: verify-release-provenance.sh ASSET_DIRECTORY VERSION SOURCE_SHA BUNDLE' >&2
  exit 2
fi

directory=$1
version=$2
source_sha=$3
bundle=$4
stem="otelplan_${version}_linux_amd64"

(
  cd "$directory"
  sha256sum --strict -c SHA256SUMS
)
for filename in "$stem.tar.gz" "$stem.spdx.json" SHA256SUMS; do
  gh attestation verify "$directory/$filename" \
    --bundle "$bundle" \
    --repo DiLRandI/OTelPlan \
    --signer-workflow DiLRandI/OTelPlan/.github/workflows/release-validation.yml \
    --source-ref refs/heads/main \
    --source-digest "$source_sha" \
    --deny-self-hosted-runners \
    --predicate-type https://slsa.dev/provenance/v1
done
