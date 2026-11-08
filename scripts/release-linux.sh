#!/bin/sh
set -eu

LC_ALL=C
export LC_ALL
unset TAR_OPTIONS GZIP GIT_DIR GIT_WORK_TREE GIT_COMMON_DIR GIT_INDEX_FILE
unset GIT_OBJECT_DIRECTORY GIT_ALTERNATE_OBJECT_DIRECTORIES

fail() {
  printf 'release: %s\n' "$*" >&2
  exit 1
}

if [ "$#" -ne 2 ]; then
  printf 'usage: release-linux.sh VERSION NEW_OUTPUT_DIRECTORY\n' >&2
  exit 2
fi
release_version=$1
output_request=$2
case "$release_version" in
  ''|*[!0-9vrc.-]*) printf 'release: invalid release version\n' >&2; exit 2 ;;
esac
if [ "${#release_version}" -gt 64 ] || ! printf '%s\n' "$release_version" |
  LC_ALL=C grep -Eq '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-rc\.(0|[1-9][0-9]*))?$'; then
  printf 'release: expected vMAJOR.MINOR.PATCH or vMAJOR.MINOR.PATCH-rc.N\n' >&2
  exit 2
fi

release_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)
release_root=$(git -C "$release_root" rev-parse --show-toplevel)
output_parent=$(CDPATH= cd -- "$(dirname -- "$output_request")" && pwd -P) || fail 'output parent must exist'
output_name=$(basename -- "$output_request")
case "$output_name" in ''|.|..) fail 'output must name a new directory' ;; esac
output_directory="$output_parent/$output_name"
if [ -e "$output_directory" ] || [ -L "$output_directory" ]; then
  fail 'output already exists'
fi

[ "$(uname -s)" = Linux ] && [ "$(uname -m)" = x86_64 ] || fail 'release packaging requires Linux/amd64'
tar --version | grep -q 'GNU tar' || fail 'GNU tar is required'
mv --version | grep -q 'GNU coreutils' || fail 'GNU mv is required'
go_version=$(GOENV=off GOWORK=off GOFLAGS= GOTOOLCHAIN=local go env GOVERSION)
printf '%s\n' "$go_version" | grep -Eq '^go1\.27\.[0-9]+$' || fail 'Go 1.27.x is required'
source_revision=$(git -C "$release_root" rev-parse --verify 'HEAD^{commit}')
source_epoch=$(git -C "$release_root" show -s --format=%ct "$source_revision")
case "$source_epoch" in ''|*[!0-9]*) fail 'commit timestamp must be a nonnegative epoch' ;; esac

staging=$(mktemp -d "$output_parent/.otelplan-release.XXXXXX")
trap 'rm -rf -- "$staging"' 0
trap 'exit 1' HUP INT TERM
mkdir "$staging/source" "$staging/payload" "$staging/publish"
git -C "$release_root" archive --format=tar "$source_revision" > "$staging/source.tar"
tar -xf "$staging/source.tar" -C "$staging/source"

stem="otelplan_${release_version}_linux_amd64"
payload="$staging/payload/$stem"
mkdir "$payload"
chmod 0755 "$payload"
for document in LICENSE README.md; do
  [ -f "$staging/source/$document" ] && [ ! -L "$staging/source/$document" ] || fail 'release documents must be regular files'
  cp "$staging/source/$document" "$payload/$document"
  chmod 0644 "$payload/$document"
done
sed -E "s|\]\((docs/[^)]*)\)|](https://github.com/DiLRandI/OTelPlan/blob/$source_revision/\1)|g" \
  "$staging/source/README.md" > "$payload/README.md"

(
  cd "$staging/source"
  GOENV=off GOWORK=off GOFLAGS= GOTOOLCHAIN=local CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GOAMD64=v1 \
    GOEXPERIMENT= GOFIPS140=off GOPROXY=off GONOPROXY=none GOSUMDB=off \
    go build -mod=readonly -trimpath -buildvcs=false \
    -ldflags "-s -w -X github.com/DiLRandI/OTelPlan/internal/cli.Version=$release_version" \
    -o "$payload/otelplan" ./cmd/otelplan
)
chmod 0755 "$payload/otelplan"
(
  cd "$payload"
  GOENV=off GOWORK=off GOFLAGS= GOTOOLCHAIN=local GOPROXY=off GONOPROXY=none GOSUMDB=off \
    go version -m ./otelplan > build-info.txt
  PATH= ./otelplan version --format=json > version.json
)
cat > "$payload/release.json" <<MANIFEST
{
  "version": "$release_version",
  "sourceRevision": "$source_revision",
  "goVersion": "$go_version",
  "target": "linux/amd64",
  "sourceDateEpoch": $source_epoch
}
MANIFEST
chmod 0644 "$payload/build-info.txt" "$payload/version.json" "$payload/release.json"
tar --sort=name --format=ustar --mtime="@$source_epoch" --owner=0 --group=0 --numeric-owner \
  -C "$staging/payload" -cf "$staging/package.tar" "$stem"
gzip -n -9 -c "$staging/package.tar" > "$staging/publish/$stem.tar.gz"
(
  cd "$staging/publish"
  sha256sum "$stem.tar.gz" > SHA256SUMS
)
chmod 0644 "$staging/publish/$stem.tar.gz" "$staging/publish/SHA256SUMS"
mv -T --no-clobber "$staging/publish" "$output_directory"
[ ! -d "$staging/publish" ] || fail 'output appeared during publication'
printf 'prepared %s\n' "$output_directory/$stem.tar.gz"
