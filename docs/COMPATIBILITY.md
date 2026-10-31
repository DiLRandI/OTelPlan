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
otelplan build -- -trimpath -o bin/api ./cmd/api
```

Adapt the application target and destination to your project. Review the resulting
artifact manifest and policy/lock changes. Compile/build do not enforce lock
freshness themselves. See [resolution/build ownership](09_LOCKFILE_REPRODUCIBILITY.md#resolution-and-build-ownership).

## Requirements before stable v1

A stable release must name its supported policy, lock, CLI JSON, Go, and backend
versions. Within a stable schema, preserve accepted field meanings and diagnostic
code categories; document additive changes and any new validation requirements.
An incompatible format needs an explicit version decision and migration steps,
not silent reinterpretation. Older tools are not promised to accept newer fields.
Security fixes may reject previously accepted unsafe inputs and must explain the
impact in release notes.

Future format migrations must document the source and destination versions,
available commands or explicit edits, expected drift and exit codes, backup and
rollback procedure, and before/after regression coverage. These commitments and
an actual release-candidate cycle remain part of [stable-v1 acceptance](13_ROADMAP.md#phase-6--stable-v1).
