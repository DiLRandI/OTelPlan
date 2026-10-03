# CLI reference

Build `otelplan` with `go build -o bin/otelplan ./cmd/otelplan` using Go 1.27.x.
This document describes the implemented CLI. All commands support text and JSON
output. Flags may appear before or after the command, except arguments following
`--`, which are positional and are used for Go build arguments.

## Commands

| Command | Behavior |
| --- | --- |
| `init [packages...]` | Analyze application declarations and write a conservative starter policy with exact symbols and suggestion evidence. |
| `scan [packages...]` | Report the semantic code inventory without choosing instrumentation. Package patterns default to `./...`. |
| `inspect` | Resolve the policy and show exact target behavior, provenance, and diagnostics. |
| `explain <canonical-symbol>` | Show matching decisions for one declaration. |
| `validate` | Validate policy, resolution, static safety, pinned backend capabilities, and an existing lock. A missing lock is permitted. |
| `lock` | Generate or refresh `otelplan.lock` relative to the project root. |
| `diff` | Compare current resolution with the lock. Drift alone returns success unless `--check` is set. |
| `compile` | Verify the backend executable, generate and compile its runtime artifacts, then publish a verified bundle. |
| `build` | Generate artifacts and execute the pinned backend in isolated module/workspace copies, then verify and publish binaries. |
| `version` | Report the OTelPlan version and the Go version used to build it. It does not query a backend executable. |
| `help` | Print a short command overview. `--help` and `-h` select this operation. |

Inspect, validate, lock, diff, and compile take no positional arguments.
Explain requires exactly one canonical symbol. Build arguments should follow `--`.

## Flags

| Flag | Applicability and meaning |
| --- | --- |
| `--root PATH` | Project root, default `.`. Relative policy/output paths are resolved against it. |
| `--format text\|json` | Output format, default `text`. |
| `--quiet` | Suppress informational text while retaining diagnostics for failures. JSON envelopes remain available. |
| `--verbose` | Add safe cause categories, operation stage, subprocess exit status, and relevant analyzed build target details. |
| `--no-color` | Accepted for monochrome output. Current output contains no ANSI color. |
| `--offline` | Disable Go module/checksum lookups and automatic toolchain downloads for analysis, compile, and build. |
| `--config PATH` | Policy commands only: inspect, explain, validate, lock, diff, compile, build. Default `otelplan.yaml`. |
| `--strict` | Policy commands only. Treat warnings as failures. |
| `--allow-large-plan` | Policy commands only. Acknowledge the target-count limit; warnings still apply. |
| `--dependencies` | Scan only. Include dependency declarations in analysis output. |
| `--interfaces` | Scan only. Show interface method bindings in text output; JSON contains model metadata. |
| `--calls` | Scan only. Include advisory SSA/CHA call relationships and precision limits. |
| `--check` | Lock/diff only. Fail for missing or stale resolution without writing files. |
| `--dry-run` | Lock only. Preview refresh without writing. |
| `--output PATH` | Init/compile only. Init defaults to `otelplan.yaml`; compile defaults to `.otelplan/build`. |
| `--force` | Init only. Permit replacement of an existing regular starter-policy file. |
| `--interactive` | Init only, text output. Review suggestions individually before writing. |
| `--non-interactive` | Init only. Explicitly request the default mode without prompts. |
| `--clean` | Compile only. Replace changed output after verifying ownership and existing manifest hashes. |

`--check --dry-run` and `--interactive --non-interactive` are contradictory and
return usage exit code 2. Interactive JSON initialization is rejected.
There is no CLI analysis-cache flag yet; caching is an opt-in discovery engine API.

## Main workflow

```sh
otelplan init --root /path/to/project --non-interactive
otelplan scan --root /path/to/project --format=json ./...
otelplan inspect --root /path/to/project
otelplan explain --root /path/to/project 'example.com/app.(*Worker).Run'
otelplan validate --root /path/to/project --strict
otelplan lock --root /path/to/project
otelplan lock --root /path/to/project --check
otelplan diff --root /path/to/project --check
otelplan compile --root /path/to/project --output .otelplan/build
otelplan build --root /path/to/project -- -trimpath -o bin/api ./cmd/api
```

Init suggestions are deterministic and advisory. Review the generated policy
before committing it. Candidates currently require one context argument and
exclude generated/test/main declarations, generics, and variadics. Some of those
shapes are supported by explicit policies even though init does not suggest them.
Capture remains disabled by default. Use `--output
starter.yml` to write another policy path. Interactive initialization writes only
after accepting suggestions; declining all or losing input/cancellation leaves
no new policy.

`scan --calls` distinguishes known static callees from conservative candidates.
A candidate is not proof of runtime execution. Call graphs do not select targets
or alter resolution locks. See [discovery](03_DISCOVERY_ENGINE.md).

## Compile and build

Both commands require the pinned `otelc v1.1.0` executable on `PATH`.
See [backend installation and compatibility](OTELC_BACKEND.md).

Compile verifies the executable version and digest, compiles generated Go source
in temporary module state, and publishes rules, hooks, and `manifest.json` with
file hashes. Identical output is reused. `--clean` replaces changed generated
output only after validating the previous manifest; edited generated files and
unrelated files prevent replacement.

Build copies application modules, local replacements, and workspace state into
a disposable workspace. It verifies that adding the generated runtime preserves
the analyzed module selection and that generated artifacts and backend identity
remain valid. It publishes binaries only after successful build and hashing.
It does not read or refresh the resolution lock. Run `lock --check` explicitly
before build when enforcing frozen resolution in CI.

Supported Go flags after `--` are `-o`, `-p`, `-tags`, `-mod`, `-modfile`, `-race`,
`-msan`, `-asan`, `-trimpath`, `-buildvcs`, `-a`, `-v`, and `-x`. These flags precede
package targets. Source-selection flags override analysis defaults. Arbitrary
compiler overrides and unsupported flags are rejected. `-modfile` requires a
`.mod` extension. Vendor mode is preserved through isolated vendoring; see the
[backend constraints](OTELC_BACKEND.md#vendor-builds).

A single command without `-o` uses Go's default executable name. Library and
multi-package builds without `-o` do not publish a binary. Output paths are
relative to `--root`. An existing directory or output ending in `/` or `\`
receives each executable, for example:

```sh
otelplan build -- -o bin/ ./cmd/...
```

Directory-output JSON reports `data.files` with paths and digests. Single-file
output uses `data.path` and `data.digest`. Publication occurs individually; an
I/O failure can leave earlier files published. Output cannot replace source,
project metadata, directories, or symlinks.

Build from an application module or pass explicit targets from a workspace root,
for example `otelplan build -- ./app`. Workspace-root builds require a package
target. Backend subprocess logs are suppressed.

## Offline execution and cancellation

Offline mode requires the application, generated runtime, and backend dependency
modules to be available in the selected Go module cache. A checked-in vendor tree
alone is insufficient for regenerating the combined isolated vendor workspace.
Prepare dependencies online in disposable module state, then use `--offline`.
Offline is a dependency-resolution guarantee, not an operating-system network
sandbox for arbitrary application code. The CLI installs no exporter or SDK.

SIGINT/SIGTERM cancels discovery, backend verification, generated-source
compilation, and isolated builds. The Go embedding entry points accept a caller
context; cancellation does not use a separate shell exit-code contract.

## Lock ownership

Lock, check, diff, and validate compare resolution state. They do not verify a
backend executable or generated artifact files. Executable digests and artifact
hashes belong to the build phase; the full lockfile comparison API compares them.

Refresh preserves independently-owned build identity when resolution is unchanged.
A resolution change with existing build identity fails refresh without writing;
that identity must be regenerated by its owner. Check/diff still report resolution
changes. Source line/column movement does not count as drift; a file move does.
Compilation publishes its own artifact manifest and does not update the lock.
See [reproducibility](09_LOCKFILE_REPRODUCIBILITY.md).

## JSON and diagnostics

JSON responses use `otelplan.io/cli/v1alpha1`:

```json
{
  "apiVersion": "otelplan.io/cli/v1alpha1",
  "command": "lock",
  "ok": false,
  "diagnostics": [{"severity": "error", "code": "OTP6001", "message": "lockfile is missing or stale"}],
  "data": {"entries": []}
}
```

`data` is command-specific. `--verbose` adds an optional `details` object.
Machine-readable usage errors also produce an envelope on stdout with `ok=false`;
invalid flag values are not echoed. Text usage errors go to stderr. Failures to
write output return 1 and may prevent emitting an envelope. A stale `diff --check`
includes both diff data and an explanatory diagnostic.

Verbose details withhold raw underlying error text, subprocess logs, error paths,
free-form environment values, and captured attribute values. Unknown causes are
reported as redacted. Inspect redacts constant values. Scan JSON replaces nonempty
CGO compiler flags and CC/CXX commands with `[redacted]`. Policy, locks, and generated
artifacts may contain explicitly configured constants; protect those files.

| Exit | Meaning |
| --- | --- |
| 0 | Success. Plain diff may report drift. |
| 1 | General filesystem/publication/output failure. |
| 2 | Invalid CLI usage or unsupported/conflicting flags. |
| 3 | Unreadable, malformed, or invalid policy. |
| 4 | Go project analysis/fingerprinting failure. |
| 5 | Resolution/static validation failure, or warnings under strict mode. |
| 6 | Missing/stale/invalid resolution lock or refresh ownership conflict. |
| 7 | Backend incompatibility, unsupported target, or missing/mismatched executable. |
| 8 | Generated-source compilation or isolated backend build failure. |

| Diagnostic codes | Category |
| --- | --- |
| OTP1001 through OTP1005 | Invalid selector, unresolved symbol, conflicting rules, unknown template variable, invalid policy. |
| OTP3001 through OTP3002 | Missing or multiple usable contexts. |
| OTP4001 through OTP4005 | Secret, PII, cardinality, broad plan, unsupported capture. |
| OTP5001 through OTP5004 | Backend unsupported/version mismatch, compilation, artifact output. |
| OTP6001 | Resolution lock failure. |

Codes identify diagnostic categories; they are not a one-to-one mapping to exits.
Consumers should inspect both the exit code and the envelope.
