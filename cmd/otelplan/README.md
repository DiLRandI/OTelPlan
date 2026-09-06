Build with `go build -o bin/otelplan ./cmd/otelplan`. Requires Go 1.27.

Implemented commands:

```sh
otelplan scan --root /path/to/project ./... --format=json
otelplan inspect --root /path/to/project
otelplan explain --root /path/to/project 'example.com/app.(*Worker).Run'
```

`inspect` and `explain` read `otelplan.yaml` relative to `--root`; override it with `--config`. JSON responses use `otelplan.io/cli/v1alpha1`. Exit codes distinguish usage (2), policy (3), project loading (4), and resolution (5) failures.

These commands inspect source locally. Compilation and instrumented builds are not implemented in this slice.
