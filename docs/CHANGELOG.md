# Changelog

## Unreleased

OTelPlan is pre-stable. This entry records the current capability baseline; it
does not reconstruct historical release entries. See the [GitHub releases](https://github.com/DiLRandI/OTelPlan/releases)
and [tags](https://github.com/DiLRandI/OTelPlan/tags) for existing records.

### Toolchain security baseline

Builds now require Go 1.27.4 or a later 1.27.x patch. Go 1.27.3 fails the current
vulnerability gate for `GO-2026-6859`. Rebuild the CLI and separately installed
backend with the patched toolchain, and review toolchain-related lock drift using
the [upgrade procedure](COMPATIBILITY.md#reviewing-an-upgrade).

### Fixes

- Recheck earlier generated output before removing it during `compile --clean`.
  If the file inventory changed, preserve the earlier output and report the failure.

- Apply capture-name safety checks to the resolved Go declaration and field
  path. Review existing captures that now require explicit acknowledgment.

- Protect selected policy, lock, alternate manifest, and workspace paths from
  build-output replacement through directory aliases or metadata symlink targets.
  Ordinary output through symlinked directories remains supported.

- Reject nonregular lockfile inputs before reading or refreshing them. FIFO
  entries return the existing stale-lock diagnostic without blocking or being
  replaced; valid symlinked locks remain readable.

- Preserve external compiler and CGO path identities when their names share a
  checkout prefix. Fingerprints retain opaque literal values and respect quoted
  path arguments. Affected locks need an explicit reviewed refresh; see
  [CGO fingerprint correction](COMPATIBILITY.md#cgo-fingerprint-correction).

- Reject nonregular objects at an analysis-cache entry without blocking on a
  FIFO or overwriting it. Read-only and writable lookups retain the cache
  storage error category; corrupt regular JSON still triggers fresh analysis.

- Preserve nil context arguments, nil-sensitive returns, and application panics.
  Argument-context hooks now skip those invocations instead of creating an
  implicit root span and replacing the argument. Existing generated bundles and
  binaries need explicit regeneration to receive this fix; resolution locks keep
  their existing meaning. See [runtime context semantics](OTELC_BACKEND.md#exact-rules-and-runtime-hooks).

### Current baseline

- `scan` and `init` discover Go declarations and produce policy starting points.
- `inspect` and `explain` show exact selected symbols and policy provenance.
- `validate --strict` checks context, error, capture, privacy, cardinality, and
  backend capability constraints.
- `lock`, `lock --check`, and `diff` provide deterministic resolution review.
- `compile` renders and verifies a pinned OTelC backend bundle.
- `build` compiles instrumented output without editing application Go source.
- Policies remain independent of OTelC rule syntax and directory naming.

### Reproducibility and safety

- `build --locked` requires a current, valid resolution lock before compilation.
- Lock and build inputs include the selected Go/toolchain, backend identity,
  module and workspace state, vendor inputs, and relevant compiler settings.
- Compile publishes a verified, owned generated bundle. Build uses disposable
  module/workspace copies, generated runtime modules, and isolated vendor state.
- Source files and project manifests remain unchanged. Build does not refresh the
  resolution lock.
- Argument and result capture is opt-in. Context and unsupported target failures
  are reported rather than silently creating disconnected spans.
- See [lockfile reproducibility](COMPATIBILITY.md), the
  [CLI contracts](CLI.md), and [validation and safety](POLICY.md).

### Supported release target

- The supported release and CI target is Linux/amd64 with Go 1.27.x.
- The compiler integration targets OTelC `v1.1.0` at its pinned source commit.
- Compile/build verify the selected OTelC executable. It is installed separately
  and is not bundled in OTelPlan candidate archives.
- Isolated module and combined-workspace vendor builds are supported when their
  dependency caches are prepared. Offline mode does not download missing modules.
- See the [compatibility contracts](COMPATIBILITY.md) and the
  [OTelC support matrix](OTELC_BACKEND.md#support-matrix).

### Known backend limits

- Generic context replacement and unbound generic capture types are rejected.
- Some generic receiver methods fail under the pinned OTelC version.
- Application-defined variadic element types are rejected.
- `package main` instrumentation targets are rejected because the pinned backend
  cannot establish safe command-specific scope.
- Panic values are not exposed to generated hooks. A package-level function named
  `recover` can trigger the documented pinned-trampoline limitation.
- These are limits of the current backend integration, not promises about other
  OTelC versions. See [backend limitations and hooks](OTELC_BACKEND.md).

### Verification and candidate archives

- Local development uses the [test strategy](DEVELOPMENT.md), including
  focused tests, race tests, vet, lint, and real-backend integration when OTelC
  is available.
- Candidate Linux archives are built from an exact committed `HEAD` with
  normalized tar and gzip inputs, SHA-256 output checks, and release metadata.
- Archives carry the executable, README and license, version/build metadata,
  and the committed third-party notice inventory. Packaging verifies notice
  checksums, Go license and patent text, and linked module versions.
- Packaging emits an SPDX 2.3 SBOM alongside the archive and checksum list.
  The hosted candidate workflow can attest these three files after full
  validation on `main`. The release procedure covers consumer verification
  and protected publication.
- Follow the [candidate archive procedure](RELEASING.md) and review the
  [release-blocking acceptance criteria](14_ACCEPTANCE_CRITERIA.md) before any
  publication or tag.

### Pending before a stable release

- The [Linux preflight](release-evidence/v1.0.0-rc.1-preflight.md) records
  performance, security, and acceptance evidence for the engineering revision.
- Validate and attest the reviewed `main` revision, configure publishing
  protection, and complete the release-candidate feedback cycle before
  requesting stable release approval. See
  [Phase 6, stable v1](ROADMAP.md#phase-6--stable-v1).
