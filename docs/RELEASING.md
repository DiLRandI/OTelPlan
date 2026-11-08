# Linux candidate archives

OTelPlan is pre-stable. This procedure prepares local candidate artifacts from an
exact committed source revision. Public releases still require the remaining
[release acceptance](13_ROADMAP.md#phase-6--stable-v1) work and maintainer review.
Version strings below are examples, not a new release announcement. Check the
existing tags and agree an unused candidate version before preparing a release.

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
ownership, timestamps, and gzip headers are normalized. Packaged README
documentation links point to that exact source revision. Compare archives built
from the same revision, version, toolchain,
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
SBOM/provenance publication and an actual RC cycle
remain release-preparation work. The utility creates local files; tagging and
publishing require a reviewed maintainer action.

### Recorded packaged-binary exercise

The [Go 1.27.3 Linux/amd64 evidence](release-evidence/packaged-cli-go1.27.3-linux-amd64.json)
records six local workflows using the executable extracted from an archive of
commit `56238b43354c9e65b25b6d9217e01d24cc7c5ebd`, with test version
`v0.0.0-rc.1`. Package, feature, and hexagonal layouts each passed independently
in cold-cache online and dependency-prepared offline modes. This records that
specific input revision, not a public RC or a claim about later changes.

The record includes the archive/backend/application digests, exact selections,
workflow commands, and assertion scope. The CLI came from the CGO-disabled
release archive; its instrumented application outputs used `-race`. Source-suite
race checks remain separate. Both archive builds had identical bytes. The offline
run rejected discovery before preparation, then made zero requests to the
rejecting HTTP(S) proxy during actual workflows. This is not an OS network sandbox.

To repeat the exercise, package that revision with the recorded toolchain and
backend, then copy `common` and each layout from the
[architecture fixtures](../internal/cli/testdata/architectures/README.md) into
disposable projects. Use separate fresh module caches for the two modes. Prepare
offline dependencies with `go mod download all` in a separate seed copy; that
explicit preparation may update its copied `go.sum`. Actual workflow projects
retain their original manifests. Execute the record's workflow with the extracted
CLI, `--format=json`, an external `--cache-dir`, and
`GOFLAGS='-race -buildvcs=false'`. Retain `--offline` after preparation and apply
the fixture's rejecting-proxy controls. Compare project bytes, modes, mtimes,
inodes, and file membership around read-only/build commands. Verify actual trace
output, not just process success.

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

## Candidate acceptance record

Keep a reviewable record for one immutable candidate commit. A successful local
packaging run alone does not satisfy [release acceptance](14_ACCEPTANCE_CRITERIA.md).

- Select a reviewed source revision and unused version. Confirm the intended PRs
  are merged by a maintainer; never publish an unreviewed stack as a release.
- Review [compatibility](COMPATIBILITY.md), [backend support](OTELC_BACKEND.md),
  [security guidance](SECURITY.md), and the [changelog](CHANGELOG.md). Name policy,
  lock, CLI JSON, diagnostic, Go, and backend compatibility for the candidate.
  Pre-stable schema identities do not become a stable guarantee through tagging.
- Record commands, tool versions, backend source/digest, and exit statuses for
  unit, race, vet, lint, quality-gate, vulnerability, and full real pinned-backend
  normal/race suites. Skipped E2E tests are not acceptance evidence. Record any
  external preparation failure separately from an application assertion failure.
- Review the static-analysis and runtime performance reports, their exact input
  revisions, raw measurements, and limits. Resolve any pending lint approval
  before calling a benchmark accepted. Do not derive performance claims from a
  successful build or export-free timing alone.
- Build the same candidate twice and compare archive bytes. Verify both checksum
  inventories, license texts, source revision, embedded version, and normalized
  metadata. Exercise the extracted CLI, not only a checkout-built executable.
- In disposable fixtures, run the documented policy/lock/compile/locked-build
  workflow and inspect actual trace parentage, attributes, and returned errors.
  Include independent cold-cache online and dependency-prepared offline cases.
  Confirm source/manifests stay unchanged and check/diff do not write project
  files. Retain results without application source or sensitive capture values.
- Review the SBOM and verifiable provenance described below. Complete a real RC
  feedback/fix/retest cycle before stable publication. Record unresolved upstream
  limits and maintainer acceptance; no fixed calendar substitutes for these gates.

Local evidence may be collected while hosted Actions are unavailable. It does
not establish that hosted checks passed or that a release was published.

## SBOM and provenance design

This is the intended publication design. No SBOM generator, signed attestation,
or release-publishing workflow is implemented by the current packaging script.

Generate an SPDX JSON SBOM for the exact candidate executable and archive using
a reviewed, pinned generator. Validate its schema and compare module identities
with `build-info.txt` and `third_party/modules.txt`; include the Go runtime and
artifact hashes, preserve applicable license identities, and distinguish build
tools from bundled runtime dependencies. Exclude OTelC from bundled components
because it is installed separately. Fail publication on missing or inconsistent
components. Record the generator identity, then checksum the final SBOM alongside
the archive. A build-info listing or notice inventory is not a standardized SBOM.

Provenance must bind final artifact digests to the source revision, selected Go
toolchain, build entry point/parameters, dependency identities, and authenticated
builder. Follow the [SLSA provenance model](https://slsa.dev/spec/v1.1/provenance)
when defining those fields. Allowlist useful build inputs; do not dump environment
variables, credentials, private paths, or application metadata. The local
`release.json` is unsigned metadata and does not prove publisher identity.

Use a manually initiated trusted workflow for eventual publication. Separate
read-only quality/build jobs from the final protected publishing job, pin actions
and tools to reviewed identities, and grant publishing/attestation permissions
only where required. Attest the final unchanged archive and SBOM bytes using
[GitHub artifact attestations](https://docs.github.com/en/actions/how-tos/secure-your-work/use-artifact-attestations/use-artifact-attestations).
Document consumer verification of digest, repository, workflow identity, and
source revision, including a negative verification test. Attestation generation
does not itself establish a SLSA assurance level.

Implement and test this workflow as a separate reviewed change when hosted
capacity and maintainer publication approval are available. Until then, preserve
local evidence and leave publication pending. Do not upload artifacts, create
tags/releases, or request write-capable credentials from PR quality jobs.
