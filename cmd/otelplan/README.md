Build with `go build -o bin/otelplan ./cmd/otelplan`. Requires Go 1.27.

Implemented commands:

```sh
otelplan scan --root /path/to/project ./... --format=json
otelplan inspect --root /path/to/project
otelplan validate --root /path/to/project --strict --offline
otelplan lock --root /path/to/project
otelplan lock --root /path/to/project --check
otelplan diff --root /path/to/project --check
otelplan explain --root /path/to/project 'example.com/app.(*Worker).Run'
```

`inspect` and `explain` read `otelplan.yaml` relative to `--root`; override it with `--config`. JSON responses use `otelplan.io/cli/v1alpha1`. Exit codes distinguish usage (2), policy (3), project loading (4), and resolution (5) failures.

`validate` checks policy, static safety, pinned backend capabilities, and an existing lockfile. It does not require the backend binary; compile/build verify that executable separately. Pin `backend.version` to `v1.1.0`.

`lock --dry-run` previews without writing. `lock --check` and `diff --check` return 6 for drift. Plain `diff` reports drift with exit 0. Backend incompatibility returns 7. `--offline` disables Go dependency resolution and toolchain downloads. Constant values are redacted from inspection output; the lockfile preserves explicit policy constants.

Compilation and instrumented builds are not implemented in this slice.
