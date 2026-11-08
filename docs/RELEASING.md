# Linux candidate archives

OTelPlan is pre-stable. This procedure prepares local candidate artifacts from an
exact committed source revision. Public releases still require the remaining
[release acceptance](13_ROADMAP.md#phase-6--stable-v1) work and maintainer review.

## Prerequisites

Use Linux/amd64, Go 1.27.x, Git, GNU tar/mv, gzip, and sha256sum. Select the tested
Go toolchain before packaging. Prepare the committed module's dependencies with
`go mod download`; the packaging build disables dependency and toolchain downloads.

Run the [authoritative quality gates](10_TEST_STRATEGY.md), including real pinned
backend tests and vulnerability checks, for the candidate source. Review the
[compatibility contracts](COMPATIBILITY.md) and backend support matrix.

## Prepare artifacts

From the intended source checkout:

```sh
sh scripts/release-linux.sh v0.1.0-rc.1 /absolute/path/to/new-output
```

The version must be `vMAJOR.MINOR.PATCH` or `vMAJOR.MINOR.PATCH-rc.N`, without
leading zeros in numeric components. The output directory must be absent and its
parent must exist. Existing directories, files, and dangling links are rejected.

The script archives `HEAD` into private disposable state. Tracked edits and
untracked checkout files are excluded. It builds with readonly module selection,
trimmed source paths, an embedded version, CGO disabled, and normalized Go/Git/
archiver settings. Source files and manifests in the checkout remain untouched.
Preparing new dependencies remains an explicit online step.

The output contains:

```text
otelplan_v0.1.0-rc.1_linux_amd64.tar.gz
SHA256SUMS
```

The archive contains the executable, project license/README, CLI version JSON,
Go build information, and `release.json` with source revision, toolchain, target,
and commit epoch. `third_party/` contains the linked modules' original license,
patent, and notice texts, plus the Go license/patent texts. Tar ordering, modes,
ownership, timestamps, and gzip headers are normalized. Packaged README documentation links point to that exact source
revision. Compare archives built from the same revision, version, toolchain,
and dependency inputs when checking reproducibility.

## Verify and review

```sh
cd /absolute/path/to/new-output
sha256sum -c SHA256SUMS
tar -tzf otelplan_v0.1.0-rc.1_linux_amd64.tar.gz
```

Extract into disposable state and inspect the manifest/build information and
`otelplan version --format=json`. Confirm the source revision and requested
version, then run the candidate workflow/trace checks before publication.
Checksums detect changed bytes; publisher authenticity requires the release's
separate provenance and verification procedure.

Build information records embedded Go dependency versions. Standardized
SBOM/provenance publication, changelog entries, and an actual RC cycle
remain release-preparation work. The utility creates local files; tagging and
publishing require a reviewed maintainer action.

## Maintain redistribution notices

The committed [notice inventory](../third_party/README.txt) records the exact
linked module versions and upstream sources. License files are copied unchanged.
Packaging verifies their checksums, compares the selected Go installation's
license/patent texts, and compares embedded binary dependencies with
`third_party/modules.txt`. It rejects module replacements. A dependency change
requires reviewing and updating notices, even if the build otherwise succeeds.
The inventory covers the CLI executable; OTelC is installed separately and is not
bundled. No online license lookup happens during packaging.

After reviewing updated texts and the linked dependency set, regenerate the
inventory from the checkout root:

```sh
(cd third_party && find . -type f ! -name SHA256SUMS -printf '%P\n' |
  LC_ALL=C sort | while IFS= read -r file; do sha256sum "$file"; done) > third_party/SHA256SUMS
```

The selected Go 1.27.x patch must have matching license/patent texts. Review any
changes rather than bypassing the check. This notice inventory does not replace
the pending standardized SBOM or provenance work.

`make test-gates` runs the fast packaging self-test with isolated Git/fake-tool
fixtures. It checks committed-source isolation, normalized inputs, checksums,
reproducibility, output collision protection, and failure cleanup. Real toolchain
archive builds supplement that test before accepting changes to packaging.
