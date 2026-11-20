# Linux candidate archives

OTelPlan is pre-stable. This procedure prepares local candidate artifacts from an
exact committed source revision. Public releases still require the remaining
[release acceptance](ROADMAP.md#phase-6--stable-v1) work and maintainer review.
Version strings below are examples, not a new release announcement. Check the
existing tags and agree an unused candidate version before preparing a release.

## Prerequisites

Use Linux/amd64, Go 1.27.4 or a later 1.27.x patch, Git, GNU tar/mv, gzip, sha256sum, and Python 3.11 or newer. Select the tested
Go toolchain before packaging. Prepare the committed module's dependencies with
`go mod download`; the packaging build disables dependency and toolchain downloads.

Run the explicit [Release validation workflow](DEVELOPMENT.md#authoritative-quality-gates),
including real pinned backend tests and vulnerability checks, for the exact
candidate source. Ordinary PR Check success is insufficient for release acceptance.
Review the [compatibility contracts](COMPATIBILITY.md) and backend support matrix.

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
otelplan_v0.1.0-rc.1_linux_amd64.spdx.json
SHA256SUMS
```

The archive contains the executable, project license/README, CLI version JSON,
Go build information, and `release.json` with source revision, toolchain, target,
and commit epoch. `third_party/` contains the linked modules' original license,
patent, and notice texts, plus the Go license/patent texts. Tar ordering, modes,
ownership, timestamps, and gzip headers are normalized. Packaged README
documentation links point to that exact source revision. Compare the archive, SPDX SBOM, and checksum list across two builds with the
same revision, version, Go toolchain, and dependencies.

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

The local SPDX file covers the archive, CLI, Go standard library, and linked Go
modules. Authenticate the release assets using the provenance verification
procedure before trusting their checksums. Tagging and publication require
maintainer approval.

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

### Exercise the packaged CLI locally

After verifying the candidate checksums, extract the archive into a disposable
directory. Run the packaged CLI exercise with the extracted binary and the
exact pinned backend.

```sh
python3 -I scripts/test-packaged-linux.py \
  /absolute/path/to/extracted/otelplan /absolute/path/to/pinned/otelc v1.0.0-rc.1
```

Each layout runs scan, inspect, explain, lock, strict validation, compile,
locked build, lock check, and diff check. It compares repeated lock and compile
outputs. The exercise verifies exact targets, preserved
application returns, span parentage and trace continuity, exception events,
and privacy. It also compares project file membership, hashes, modes, inodes,
and modification times. The offline run first checks that a cold lookup fails,
then prepares dependencies in a disposable project and runs with Go lookup
disabled and a rejecting HTTP(S) proxy.

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
changes and update `third_party/spdx-licenses.txt` alongside its corresponding
notice files and module versions.

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

## SPDX SBOM verification

The archive build uses the generator from the committed source revision.
It records SHA-256 checksums for the archive and executable, the Go standard
library version, and exactly the modules in the executable's build information.
The generator requires those modules to agree with `third_party/modules.txt`
and their SPDX license entries to match the linked module set. The OTelC
executable and other build tools are installed separately.

Validate the JSON with the [official SPDX 2.3 schema](https://github.com/spdx/spdx-spec/blob/6f2cb47d13db19b12c23885573b22ef32ec1ada5/schemas/spdx-schema.json).
From the output directory with Python's `jsonschema` installed:

```sh
curl -fsSL https://raw.githubusercontent.com/spdx/spdx-spec/6f2cb47d13db19b12c23885573b22ef32ec1ada5/schemas/spdx-schema.json -o spdx-schema.json
printf '%s  %s\n' 4126dc29f15e92feec92e9622f4939131ad58b125d6f578aa93e980a6e6f3212 spdx-schema.json | sha256sum -c -
python3 -m jsonschema -i otelplan_v0.1.0-rc.1_linux_amd64.spdx.json spdx-schema.json
```

Compare the SPDX hashes to the archive and extracted executable. Compare the
module and standard library versions with `go version -m` on that executable,
and check the license expressions against the archived notice inventory.

## Stage a candidate with provenance

Run Release validation on `main` with an unused candidate version in
`prepare_version`. Leave the input empty for a validation-only run.

```sh
gh workflow run release-validation.yml --ref main -f prepare_version=v1.0.0-rc.1
```

The candidate job depends on the full release gate. It packages the exact
tested commit twice, compares the outputs, and validates the SBOM against a
checksum-pinned copy of the official schema. The job runs the packaged architecture exercise
with the extracted CLI and pinned OTelC, covering independent online and
prepared-offline runs. It then signs the unchanged archive,
SBOM, and checksum list with GitHub artifact attestations. The workflow verifies
the Sigstore bundle and checks that verification rejects a modified archive
before retaining the assets as a workflow artifact for review.

Use a current GitHub CLI with `gh attestation verify`. Download the artifact from
the specific successful run and attempt. Obtain the expected full source SHA
from the reviewed candidate record. The verifier checks the checksum list, all
three signatures, the source SHA and ref, the signer workflow, and the hosted
runner requirement.

```sh
sh scripts/verify-release-provenance.sh /absolute/path/to/candidate \
  v1.0.0-rc.1 "$REVIEWED_SOURCE_SHA" /absolute/path/to/candidate/provenance.jsonl
```

The signed payload uses the [SLSA v1 provenance format](https://slsa.dev/spec/v1.1/provenance).
The workflow artifact is for candidate review. Use the separate publishing workflow after candidate review and maintainer approval.

### Linux release build type

The workflow dispatch accepts `prepare_version` and runs on the selected
`refs/heads/main` commit. These inputs and the repository and workflow path are
recorded in `externalParameters`. The archive recipe is
`scripts/release-linux.sh VERSION OUTPUT_DIRECTORY` at that commit.
`internalParameters` records the entry point, target and selected Go version.
`resolvedDependencies` records the Git commit, Go toolchain version, and linked
modules with Go's normalized `h1:` checksums. The archive ships `release.json`
and `build-info.txt` alongside the binary for independent comparison. The
attestation authenticates the workflow and subjects with GitHub OIDC and
Sigstore. The run ID and attempt identify the build invocation.

## Publish a reviewed Linux release

Before enabling publication, protect `main` and create the GitHub Actions
environment `release`. Allow deployments from the `main` branch only. Set at
least one required reviewer, prevent self-review, and disable administrator
bypass. Enable immutable releases in repository settings to protect published
tags and assets. This workflow uses the built-in job token for publication.
See [GitHub environment protection](https://docs.github.com/en/actions/how-tos/deploy/configure-and-manage-deployments/manage-environments).

First run **Publish Linux release** on `main` with the candidate run ID and
version. Leave `publish` false to exercise download and cryptographic
verification with read permissions. The run must be a successful manual
**Release validation** run at the exact commit and include the signed candidate
job. Fast PR checks alone cannot satisfy this gate.

After candidate acceptance and release approval, the maintainer creates the
reviewed tag at that source commit. Use an annotated tag with the reviewed
release notes. Dispatch **Publish Linux release** with `publish=true` and
have the independent reviewer approve the `release` environment. The workflow
checks branch and environment protection, tag identity, and the candidate run
before approval. The publishing job repeats the checks and verifies every
attested subject. It uploads the archive, SPDX SBOM, checksums, and Sigstore
bundle to a draft, then compares every uploaded SHA-256 with the staged file
before publishing. Release candidate versions are GitHub prereleases.

A failed upload or digest comparison can leave a draft for inspection.
Resolve the draft explicitly before retrying. A publication attempt never
reuses an existing draft or creates a missing tag.
