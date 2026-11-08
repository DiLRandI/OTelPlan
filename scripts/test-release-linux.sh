#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
builder=$script_dir/release-linux.sh
test_root=$(mktemp -d)
trap 'rm -rf "$test_root"' EXIT HUP INT TERM

fail() {
	printf '%s\n' "release self-test: $*" >&2
	exit 1
}

require_file() {
	test -f "$1" || fail "missing $1"
}

expect_early_rejection() {
	expected_exit=$1
	shift
	rm -f "$test_root/rejected-go.log"
	if RELEASE_TEST_GO_LOG=$test_root/rejected-go.log RELEASE_TEST_VERSION=$version \
		PATH=$fakebin:/usr/bin:/bin TMPDIR=$repo/tmp \
		"$repo/scripts/release-linux.sh" "$@" > "$test_root/rejection.log" 2>&1; then
		fail 'accepted invalid input'
	else
		actual_exit=$?
	fi
	test "$actual_exit" -eq "$expected_exit" || fail 'input failed at the wrong stage'
	test ! -e "$test_root/rejected-go.log" || fail 'invalid input invoked Go'
}

test -x "$builder" || fail "missing executable $builder"

repo=$test_root/repo
mkdir -p "$repo/scripts" "$repo/tmp"
cp "$builder" "$repo/scripts/release-linux.sh"
chmod +x "$repo/scripts/release-linux.sh"
printf 'license\n' > "$repo/LICENSE"
printf '# test\n' > "$repo/README.md"
printf 'committed\n' > "$repo/tracked.marker"
mkdir -p "$repo/third_party/go"
printf 'fake go license\n' > "$repo/third_party/go/LICENSE"
printf 'fake go patents\n' > "$repo/third_party/go/PATENTS"
printf 'third-party notice\n' > "$repo/third_party/NOTICE"
printf '%s\n' 'example.invalid/dep v1.2.3' > "$repo/third_party/modules.txt"
(cd "$repo/third_party" && sha256sum go/LICENSE go/PATENTS NOTICE modules.txt > SHA256SUMS)
git -C "$repo" init -q
git -C "$repo" config user.name release-test
git -C "$repo" config user.email release-test@example.invalid
git -C "$repo" add LICENSE README.md tracked.marker scripts/release-linux.sh third_party
GIT_AUTHOR_DATE='2024-01-02T03:04:05Z' GIT_COMMITTER_DATE='2024-01-02T03:04:05Z' \
	git -C "$repo" -c commit.gpgsign=false commit -q -m 'release test fixture'
revision=$(git -C "$repo" rev-parse HEAD)
epoch=$(git -C "$repo" show -s --format=%ct HEAD)
printf 'dirty\n' > "$repo/tracked.marker"
printf 'untracked\n' > "$repo/untracked.marker"

fakebin=$test_root/bin
mkdir -p "$fakebin"
fakegoroot=$test_root/goroot
mkdir -p "$fakegoroot"
printf 'fake go license\n' > "$fakegoroot/LICENSE"
printf 'fake go patents\n' > "$fakegoroot/PATENTS"
cat > "$fakebin/go" <<'GO'
#!/bin/sh
set -eu

log=${RELEASE_TEST_GO_LOG:?}
case "${1-}" in
  env)
    case "${2-}" in
      GOVERSION) printf '%s\n' "go1.27.3" ;;
      GOROOT) printf '%s\n' "${RELEASE_TEST_GOROOT:?}" ;;
      *) exit 41 ;;
    esac
    ;;
  build)
    test "${RELEASE_TEST_FAIL_BUILD-}" != 1 || exit 42
    test "${GOENV-}" = off
    test "${GOWORK-}" = off
    test "${GOFLAGS-}" = ''
    test "${GOTOOLCHAIN-}" = local
    test "${CGO_ENABLED-}" = 0
    test "${GOOS-}" = linux
    test "${GOARCH-}" = amd64
    test "${GOAMD64-}" = v1
    test "${GOEXPERIMENT-}" = ''
    test "${GOFIPS140-}" = off
    test "${GOPROXY-}" = off
    test "${GONOPROXY-}" = none
    test "${GOSUMDB-}" = off
    ldflags=''
    previous=''
    for arg do
      if test "$previous" = -ldflags; then ldflags=$arg; fi
      previous=$arg
    done
    test "$ldflags" = "-s -w -X github.com/DiLRandI/OTelPlan/internal/cli.Version=${RELEASE_TEST_VERSION:?}"
    case "$*" in
      *' -mod=readonly '*'-trimpath '*'-buildvcs=false '*'-ldflags '*'-o '*) ;;
      *) exit 43 ;;
    esac
    test -f tracked.marker
    test "$(cat tracked.marker)" = committed
    test ! -e untracked.marker
    test ! -e dirty.marker
    output=''
    previous=''
    for arg do
      if test "$previous" = -o; then output=$arg; fi
      previous=$arg
    done
    test -n "$output"
    cat > "$output" <<'STUB'
#!/bin/sh
set -eu
if test "${1-}" = version && test "${2-}" = --format=json; then
  printf '{"version":"%s"}\n' "${RELEASE_TEST_VERSION:?}"
  exit 0
fi
exit 44
STUB
    chmod +x "$output"
    printf 'build env normalized\n' >> "$log"
    ;;
  version)
    test "${2-}" = -m || exit 45
    test "${GOENV-}" = off
    test "${GOWORK-}" = off
    test "${GOFLAGS-}" = ''
    test "${GOTOOLCHAIN-}" = local
    test "${GOPROXY-}" = off
    test "${GONOPROXY-}" = none
    test "${GOSUMDB-}" = off
    printf 'otelplan: go1.27.3\n'
    printf 'build info deterministic\n'
    printf '\tdep example.invalid/dep v1.2.3 h1:fake\n'
    if test "${RELEASE_TEST_REPLACEMENT-}" = 1; then
      printf '\t=> example.invalid/replacement v1.2.3 h1:fake\n'
    fi
    ;;
  *) exit 46 ;;
esac
printf '%s\n' "$*" >> "$log"
GO
chmod +x "$fakebin/go"
cat > "$fakebin/mv" <<'MV'
#!/bin/sh
set -eu
case " $* " in
  *' --version '*) exec /usr/bin/mv "$@" ;;
esac
if test "${RELEASE_TEST_RACE-}" = 1; then
  has_t=0
  last=''
  for arg do
    test "$arg" = -T && has_t=1
    last=$arg
  done
  if test "$has_t" = 1; then
    mkdir "$last"
    printf sentinel > "$last/sentinel"
  fi
fi
exec /usr/bin/mv "$@"
MV
chmod +x "$fakebin/mv"
export RELEASE_TEST_GOROOT=$fakegoroot

commit_third_party_change() {
	git -C "$repo" add third_party
	GIT_AUTHOR_DATE='2024-01-02T03:04:05Z' GIT_COMMITTER_DATE='2024-01-02T03:04:05Z' \
		git -C "$repo" -c commit.gpgsign=false commit -q -m 'release test third-party fixture'
}

expect_release_failure() {
	expected=$1
	failure_output=$2
	replacement=0
	test "$expected" = 'release dependencies must not use replacements' && replacement=1
	rm -f "$test_root/release-failure.log"
	if RELEASE_TEST_GO_LOG=$test_root/failure-go.log RELEASE_TEST_VERSION=$version RELEASE_TEST_REPLACEMENT=$replacement \
		PATH=$fakebin:/usr/bin:/bin TMPDIR=$repo/tmp \
		"$repo/scripts/release-linux.sh" "$version" "$failure_output" > "$test_root/release-failure.log" 2>&1; then
		fail "accepted invalid third-party fixture: $expected"
	fi
	grep -F "$expected" "$test_root/release-failure.log" >/dev/null ||
		fail "missing meaningful third-party error: $expected"
	test ! -e "$failure_output" || fail 'published output after third-party rejection'
	test "$(find "$test_root" -type d -name '.otelplan-release.*' | wc -l)" -eq 0 ||
		fail 'staging leaked after third-party rejection'
}

version=v1.2.3-rc.4
output1=$test_root/output-one
output2=$test_root/output-two
RELEASE_TEST_GO_LOG=$test_root/go.log RELEASE_TEST_VERSION=$version \
	PATH=$fakebin:/usr/bin:/bin TMPDIR=$repo/tmp \
	"$repo/scripts/release-linux.sh" "$version" "$output1"

archive=$output1/otelplan_${version}_linux_amd64.tar.gz
checksums=$output1/SHA256SUMS
require_file "$archive"
require_file "$checksums"
(cd "$output1" && sha256sum -c SHA256SUMS)
test "$(find "$output1" -mindepth 1 -maxdepth 1 -type f | wc -l)" -eq 2
test "$(tar -tzf "$archive" | sed '/\/$/d' | wc -l)" -eq 11
top=otelplan_${version}_linux_amd64
tar -tzf "$archive" | grep -Fx "$top/otelplan" >/dev/null
tar -tzf "$archive" | grep -Fx "$top/LICENSE" >/dev/null
tar -tzf "$archive" | grep -Fx "$top/README.md" >/dev/null
tar -tzf "$archive" | grep -Fx "$top/build-info.txt" >/dev/null
tar -tzf "$archive" | grep -Fx "$top/version.json" >/dev/null
tar -tzf "$archive" | grep -Fx "$top/release.json" >/dev/null
tar -tzf "$archive" | grep -Fx "$top/third_party/go/LICENSE" >/dev/null
tar -tzf "$archive" | grep -Fx "$top/third_party/go/PATENTS" >/dev/null
tar -tzf "$archive" | grep -Fx "$top/third_party/NOTICE" >/dev/null
tar -tzf "$archive" | grep -Fx "$top/third_party/modules.txt" >/dev/null
tar -tzf "$archive" | grep -Fx "$top/third_party/SHA256SUMS" >/dev/null
mkdir "$test_root/extracted"
tar -xzf "$archive" -C "$test_root/extracted"
grep -F 'go1.27.3' "$test_root/extracted/$top/build-info.txt" >/dev/null
cmp "$repo/third_party/go/LICENSE" "$test_root/extracted/$top/third_party/go/LICENSE"
cmp "$repo/third_party/go/PATENTS" "$test_root/extracted/$top/third_party/go/PATENTS"
cmp "$repo/third_party/NOTICE" "$test_root/extracted/$top/third_party/NOTICE"
cmp "$repo/third_party/modules.txt" "$test_root/extracted/$top/third_party/modules.txt"
cmp "$repo/third_party/SHA256SUMS" "$test_root/extracted/$top/third_party/SHA256SUMS"
(cd "$test_root/extracted/$top/third_party" && sha256sum --strict -c SHA256SUMS)
grep -F '"version":"'$version'"' "$test_root/extracted/$top/version.json" >/dev/null
tr -d '[:space:]' < "$test_root/extracted/$top/release.json" > "$test_root/release.normalized.json"
grep -F '"version":"'$version'"' "$test_root/release.normalized.json" >/dev/null
grep -F '"sourceRevision":"'$revision'"' "$test_root/release.normalized.json" >/dev/null
grep -F '"goVersion":"go1.27.3"' "$test_root/release.normalized.json" >/dev/null
grep -F '"target":"linux/amd64"' "$test_root/release.normalized.json" >/dev/null
grep -F '"sourceDateEpoch":'$epoch "$test_root/release.normalized.json" >/dev/null
test "$(cat "$repo/tracked.marker")" = dirty
test -f "$repo/untracked.marker"

RELEASE_TEST_GO_LOG=$test_root/go2.log RELEASE_TEST_VERSION=$version \
	PATH=$fakebin:/usr/bin:/bin TMPDIR=$repo/tmp \
	"$repo/scripts/release-linux.sh" "$version" "$output2"
cmp "$archive" "$output2/otelplan_${version}_linux_amd64.tar.gz"
cmp "$checksums" "$output2/SHA256SUMS"

output3=$test_root/output-hostile-ambient
TAR_OPTIONS='--exclude=tracked.marker' GIT_DIR=/definitely/missing/git \
  RELEASE_TEST_GO_LOG=$test_root/go3.log RELEASE_TEST_VERSION=$version \
  PATH=$fakebin:/usr/bin:/bin TMPDIR=$repo/tmp \
  "$repo/scripts/release-linux.sh" "$version" "$output3"
require_file "$output3/otelplan_${version}_linux_amd64.tar.gz"
(cd "$output3" && sha256sum -c SHA256SUMS)

printf '%s\n' 'example.invalid/dep v1.2.3 => example.invalid/replacement v1.2.3' > "$repo/third_party/modules.txt"
(cd "$repo/third_party" && sha256sum go/LICENSE go/PATENTS NOTICE modules.txt > SHA256SUMS)
commit_third_party_change
expect_release_failure 'release dependencies must not use replacements' "$test_root/replacement-output"

printf '%s\n' 'example.invalid/dep v1.2.4' > "$repo/third_party/modules.txt"
(cd "$repo/third_party" && sha256sum go/LICENSE go/PATENTS NOTICE modules.txt > SHA256SUMS)
commit_third_party_change
expect_release_failure 'linked dependencies differ; review third-party notices' "$test_root/dependency-mismatch-output"

printf '%s\n' 'example.invalid/dep v1.2.3' > "$repo/third_party/modules.txt"
printf 'tampered notice\n' > "$repo/third_party/NOTICE"
commit_third_party_change
expect_release_failure 'third-party notice checksum verification failed' "$test_root/tampered-notice-output"

rm "$repo/third_party/NOTICE"
commit_third_party_change
expect_release_failure 'third-party notice checksum verification failed' "$test_root/missing-notice-output"

ln -s go/LICENSE "$repo/third_party/NOTICE"
commit_third_party_change
expect_release_failure 'third-party notices must be regular files' "$test_root/symlink-notice-output"

rm "$repo/third_party/NOTICE"
printf 'third-party notice\n' > "$repo/third_party/NOTICE"
(cd "$repo/third_party" && sha256sum go/LICENSE go/PATENTS NOTICE modules.txt > SHA256SUMS)
commit_third_party_change

printf 'mismatched go license\n' > "$fakegoroot/LICENSE"
expect_release_failure 'Go license texts differ; review third-party notices' "$test_root/go-license-mismatch-output"
printf 'fake go license\n' > "$fakegoroot/LICENSE"

for invalid in v1.2 v1.2.03 v01.2.3 v1.2.3-rc.01 v1.2.3-rc v1.2.3+meta; do
	expect_early_rejection 2 "$invalid" "$test_root/invalid-output"
	test ! -e "$test_root/invalid-output"
done
expect_early_rejection 2
expect_early_rejection 2 "$version"
test ! -e "$test_root/missing-output"
expect_early_rejection 2 "$version" "$test_root/extra-output" extra
test ! -e "$test_root/extra-output"
metachar="v1.2.3;touch $test_root/version-injected"
expect_early_rejection 2 "$metachar" "$test_root/metachar-output"
test ! -e "$test_root/metachar-output"
test ! -e "$test_root/version-injected"
newline_version=$(printf 'v1.2.3\nx')
expect_early_rejection 2 "$newline_version" "$test_root/newline-output"
test ! -e "$test_root/newline-output"
long=v1.2.3-rc.
i=0
while test "$i" -lt 60; do long=${long}1; i=$((i + 1)); done
expect_early_rejection 2 "$long" "$test_root/long-output"
test ! -e "$test_root/long-output"

existing=$test_root/existing
mkdir "$existing"
printf sentinel > "$existing/sentinel"
expect_early_rejection 1 "$version" "$existing"
test "$(cat "$existing/sentinel")" = sentinel
link=$test_root/dangling-output
ln -s nowhere "$link"
expect_early_rejection 1 "$version" "$link"
test -L "$link"

failed=$test_root/failed-output
if RELEASE_TEST_FAIL_BUILD=1 RELEASE_TEST_GO_LOG=$test_root/fail.log RELEASE_TEST_VERSION=$version \
	PATH=$fakebin:/usr/bin:/bin TMPDIR=$repo/tmp \
	"$repo/scripts/release-linux.sh" "$version" "$failed" >/dev/null 2>&1; then
	fail 'swallowed fake build failure'
fi
test ! -e "$failed"
test "$(find "$test_root" -type d -name '.otelplan-release.*' | wc -l)" -eq 0

race=$test_root/race-output
if RELEASE_TEST_GO_LOG=$test_root/race.log RELEASE_TEST_VERSION=$version RELEASE_TEST_RACE=1 \
	PATH=$fakebin:/usr/bin:/bin TMPDIR=$repo/tmp \
	"$repo/scripts/release-linux.sh" "$version" "$race" >/dev/null 2>&1; then
	fail 'accepted output appearing before publication'
fi
test -f "$race/sentinel"
test "$(cat "$race/sentinel")" = sentinel
test "$(find "$test_root" -type d -name '.otelplan-release.*' | wc -l)" -eq 0

printf '%s\n' 'release-linux self-test: PASS'
