Build with `go build -o bin/otelplan ./cmd/otelplan`. Requires Go 1.27.

Implemented commands:

```sh
otelplan scan --root /path/to/project ./... --format=json
otelplan scan --root /path/to/project --calls ./...
otelplan inspect --root /path/to/project
otelplan validate --root /path/to/project --strict --offline
otelplan lock --root /path/to/project
otelplan lock --root /path/to/project --check
otelplan diff --root /path/to/project --check
otelplan compile --root /path/to/project --output .otelplan/build
otelplan build --root /path/to/project -- -trimpath -o bin/api ./cmd/api
otelplan explain --root /path/to/project 'example.com/app.(*Worker).Run'
```

`scan --calls` includes advisory calls and analysis limits in text or JSON output. Static edges identify a known callee; conservative edges are possible calls, not proof of runtime execution. This opt-in analysis does not select instrumentation targets.

`inspect` and `explain` read `otelplan.yaml` relative to `--root`; override it with `--config`. JSON responses use `otelplan.io/cli/v1alpha1`. Exit codes distinguish usage (2), policy (3), project loading (4), and resolution (5) failures.

`validate` checks policy, static safety, pinned backend capabilities, and an existing lockfile. It does not require the backend binary; compile/build verify that executable separately. Pin `backend.version` to `v1.1.0`.

`lock --dry-run` previews without writing. `lock --check` and `diff --check` return 6 for drift. Plain `diff` reports drift with exit 0. Backend incompatibility returns 7. `--offline` disables Go dependency resolution and toolchain downloads. Constant values are redacted from inspection output; the lockfile preserves explicit policy constants.

`compile` requires the pinned `otelc` executable on `PATH`. It resolves and validates the policy, verifies backend identity, compiles the generated Go package in temporary module state, and publishes rules, hooks, and a file-hash manifest. Output defaults to `.otelplan/build` relative to `--root`. Identical output is reused. Use `--clean` to replace changed output; edited generated files or unrelated files prevent replacement. Compilation failure returns 8; output failures return 1.

`build` executes the pinned backend in copied module state and verifies the output digest before publication. Pass Go arguments after `--`. A single command without `-o` uses Go's default executable name; library and multi-package builds without `-o` do not publish a binary. Output paths are relative to `--root`. An existing directory or an output ending in a slash or backslash receives each resulting executable, for example `otelplan build -- -o bin/ ./cmd/...`. Directory output JSON lists each path and digest under `data.files`; single-file output retains `data.path` and `data.digest`. Files are verified and published individually, so an I/O failure during publication can leave earlier files published.

Supported build flags include `-o`, `-p`, `-tags`, `-mod`, `-modfile`, `-race`, `-msan`, `-asan`, `-trimpath`, `-buildvcs`, `-a`, `-v`, and `-x`. Explicit source-selection flags override analysis defaults. Vendor-mode builds and arbitrary compiler overrides remain unsupported. Build from an application module or pass explicit package targets from a workspace root, for example `otelplan build -- ./app`. Workspace-root builds require a package target. Backend subprocess logs are suppressed. Build commands do not refresh the resolution lock.

`lock`, `lock --check`, `diff`, and `validate` operate on resolution state. They do not verify a backend executable or generated artifact files. Executable digests and artifact hashes belong to the build phase; the full lockfile comparison API still compares them.

A lock refresh preserves existing build identity when resolution is unchanged. If resolution changes while build identity is present, refresh fails without writing: a build must regenerate the associated identity. Check/diff commands still report the resolution changes. Coordinate-only source movement does not invalidate that identity. This contract prevents resolution-only commands from erasing or incorrectly reusing a build's metadata. Compilation publishes its own manifest and does not update the resolution lock.

With `--format=json`, usage errors also return a JSON envelope on stdout with `ok=false`, diagnostics, and exit code 2. Invalid flag values are not echoed. Output failures return 1. Text usage errors remain on stderr.

`lock --check --dry-run` is rejected as contradictory. `--dependencies` and `--interfaces` apply to `scan`; `--config`, `--strict`, and `--allow-large-plan` apply to policy commands (`inspect`, `explain`, `validate`, `lock`, `diff`, `compile`, `build`). `--verbose` is rejected until verbose output is implemented. A stale `diff --check` returns 6 with both the diff and a diagnostic explaining the failure.
