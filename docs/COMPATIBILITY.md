# Compatibility and upgrades

OTelPlan is pre-stable. The formats below are development contracts, not a stable
v1 compatibility guarantee. Pin the OTelPlan revision or release as well as the
Go toolchain and backend used by a production build. Tool versions and schema
versions are separate identities.

## Current contracts

| Contract | Current identity and behavior |
| --- | --- |
| Policy | `apiVersion: otelplan.io/v1alpha1`, `kind: InstrumentationPlan`. Known YAML fields, one document, and semantic validation are required. |
| Resolution lock | `apiVersion: otelplan.io/lock/v1alpha1`. Strict JSON parsing and identity validation precede check/diff. |
| CLI JSON | `apiVersion: otelplan.io/cli/v1alpha1`. Common envelope with command-specific data; see [CLI contracts](CLI.md#json-and-diagnostics). |
| Diagnostics | Stable `OTP` category codes and process exits. Message wording is for humans; codes and exits have different purposes. |
| Backend | Exact OTelC v1.1.0 and its documented capability limits. Compile/build verify the selected executable. |
| Go/platform | Go 1.27.x on Linux. See the [tested support matrix](OTELC_BACKEND.md#support-matrix). |

Unknown policy fields and unsupported policy/lock versions fail validation rather
than being silently interpreted. Changing only an `apiVersion` string does not
migrate the contents. There is no automatic `migrate` command in the current CLI.
Preserve the reviewed policy when upgrading; initialization generates suggestions
rather than migrating existing intent.

The primary integration contracts are policy YAML, locks, CLI JSON, and diagnostic
codes. `pkg/model` preserves existing Go compatibility names, but is not a stable
public SDK commitment. Domain definitions belong to `internal/model`; see
[package ownership and reflection behavior](02_ARCHITECTURE.md#2-package-boundaries).

## Automation

Check the process exit, envelope `ok`, and supported `apiVersion`. Interpret
diagnostic codes instead of matching message prose. Handle absent optional `data`
or `details` and ignore JSON fields your consumer does not use. Output-write
failures may prevent an envelope from being emitted.

Plain `diff` can report changes while exiting 0. Use `diff --check` or
`lock --check` when CI must fail for drift. `--strict` also fails for validation
warnings. Read the [exit-code reference](CLI.md#json-and-diagnostics) instead of
assuming every failure means an invalid policy.

## Reviewing an upgrade

Keep the previous tool, policy, lock, and generated artifacts available for
comparison or rollback. Review release notes and the pinned backend's support
matrix before changing versions. A backend upgrade requires a verified adapter
update; changing the policy pin alone cannot enable an unsupported backend.

From the project root with its reviewed `otelplan.yaml`, use the new executable
to inspect the resolution without refreshing the lock:

```sh
otelplan version --format=json
otelplan inspect --format=json
otelplan diff --format=json
otelplan lock --check --format=json
```

Review selected and removed symbols, signatures, context/error strategies,
attributes, policy provenance, backend identity, and build-environment changes.
`version` reports available backend details but does not verify a build identity.
Check/diff do not write project files; analysis may populate Go caches. Prepare
dependencies first if you need `--offline`.

A meaningful resolution change or missing/invalid lock causes `lock --check` to
return 6. `validate --strict` can also return 6 for an existing stale lock after
policy and safety checks pass. Do not discard policy, analysis, safety, or backend
errors to force an upgrade through. An unparseable old lock cannot produce a
meaningful diff; retain it for inspection with the previous tool before replacing
it.

## Regenerating reviewed state

For a resolution-only lock, preview and explicitly refresh after accepting the
changes:

```sh
otelplan lock --dry-run --format=json
otelplan lock
otelplan validate --strict
otelplan lock --check
```

Refresh preserves independently owned backend digests/artifact identity only while
resolution is unchanged. If refresh reports an ownership conflict, the phase that
wrote that metadata must regenerate it. Do not erase those sections to bypass the
conflict. Current CLI compile/build publish separate artifacts and do not populate
or automatically refresh that lock metadata.

Regenerate backend output and build only after strict validation and lock checking
succeed. For an existing, verified owned compile directory, `compile --clean`
permits replacement; unrelated or modified output still fails ownership checks:

```sh
otelplan compile --clean --output .otelplan/build
otelplan build --locked -- -o bin/api ./cmd/api
```

Adapt the application target and destination to your project. Review the resulting
artifact manifest and policy/lock changes. Locked builds enforce resolution
freshness; default builds and compile do not. Generate the lock with matching
effective Go flags if adding options such as `-trimpath`; see [build inputs](CLI.md).
See [resolution/build ownership](09_LOCKFILE_REPRODUCIBILITY.md#resolution-and-build-ownership).

## Stable-line commitments

The following policy takes effect with the first stable CLI release and applies
through its `1.x` line. It does not retroactively make pre-stable releases stable
or announce that release acceptance is complete.

The first stable CLI retains the policy, lock, and CLI JSON identifiers in the
current-contract table. A tool release does not rename those formats. Existing
files do not need an `apiVersion` edit solely because the CLI reaches `1.0`.
Introducing another format identifier requires its implemented reader/writer and
documented migration path; changing a label alone is never a migration.

### Policy

Newer stable tools continue to accept valid policies using supported fields and
retain their selector, exclusion, template, context, error, and capture meanings.
Do not remove fields, change their types, reinterpret defaults, or change rule
precedence under an existing format identity. Capture remains explicit, policy
stays backend-independent, and unsupported targets fail validation.

Optional fields and capabilities may be added with documented minimum tool and
backend versions. Older tools are not promised to accept those additions; their
strict unknown-field/version rejection remains intentional. A policy using only
older supported features does not have to opt into a new feature to keep working.

### Locks

Stable tools retain the meaning and types of supported lock fields, exact symbol
identity, and the distinction between resolution and independently owned build
metadata. Refresh must not erase another phase's identity. Canonical encoding
remains deterministic for identical normalized inputs; line/column movement
remains diagnostic metadata, while source-file movement remains drift.

Read compatibility does not imply freshness after source, policy, Go environment,
backend, or artifact changes. A necessary digest/resolution correctness fix must
document expected drift and explicit review/refresh steps. Check/diff stay
read-only, malformed locks fail safely, and checks do not silently migrate files.
Unknown fields or versions are rejected rather than guessed at.

### CLI JSON and diagnostics

Keep existing command/flag meanings, process-exit categories, JSON envelope keys
and types, and supported command-data fields compatible. Additive commands,
optional flags, and optional JSON fields may be introduced. Consumers must ignore
unknown response fields and object-key order; optional `data`/`details` remain
optional. Text formatting and diagnostic prose are not machine contracts.

Existing diagnostic codes retain their category and meaning and are not reused
for unrelated failures. New codes and warnings may be added. Release notes must
identify new validation that can change an exit, including warnings that fail
`--strict`, with actionable remediation. Security/safety fixes may reject inputs
previously accepted unsafely; explain the affected behavior rather than silently
weakening checks to preserve it.

### Support boundaries and migrations

Format compatibility does not extend the tested Go/platform/backend matrix or
promise a public Go SDK. Backend versions remain exact pins; no silent upgrade,
module-mode fallback, or capability substitution is allowed. Document support
changes and verify them independently of format compatibility.

Incompatible changes to existing command/flag meanings or exit categories require
a new major CLI line. An incompatible policy, lock, or JSON format requires a new
`apiVersion`. Each change needs release notes and a migration guide naming
source/destination versions, supported
commands or exact edits, expected drift/exits, backup/rollback, and before/after
regression coverage. Preserve the old input for review; never rewrite it as a
side effect of inspect, validate, check, diff, or locked build. The existing
review/refresh procedure above is the current upgrade workflow, not an automatic
format converter.

The [release candidate record](RELEASING.md#candidate-acceptance-record) must verify
these commitments against independent policy/lock/CLI contracts and real backend
behavior before stable publication. The [remaining roadmap](13_ROADMAP.md#phase-6--stable-v1)
still requires an actual RC feedback and acceptance cycle.
