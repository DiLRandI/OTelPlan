# CLI Specification

Binary: `otelplan`

## Global flags

```text
--config <path>       default: ./otelplan.yaml
--root <path>         project root
--format text|json    default: text
--quiet
--verbose
--no-color
```

## Commands

### `otelplan init [packages...]`

Analyze project and generate a conservative starter policy.

Rules:

- never overwrite an existing policy without `--force`;
- starter policy contains explanations/comments;
- generated selections are suggestions, not hidden automatic behavior.

Options:

```text
--output
--force
--interactive
--non-interactive
```

### `otelplan scan [packages...]`

Produce code inventory.

Options:

```text
--calls
--interfaces
--dependencies
```

### `otelplan inspect`

Resolve current policy and show exact behavior.

Example:

```text
SELECTED  internal/payment.(*Processor).Authorize
  span     payment.Processor.Authorize
  context  argument[0]
  error    result[0]
  rule     payment-operations

SKIPPED   internal/payment.(*Processor).health
  reason   not exported
```

### `otelplan explain <canonical-symbol>`

Show all matching decisions.

### `otelplan validate`

Validate policy, code, safety, lockfile, and backend.

Options:

```text
--strict
--offline
```

### `otelplan lock`

Write/update `otelplan.lock`.

Options:

```text
--check       fail if lock would change
```

### `otelplan diff`

Compare current resolved state with lockfile.

Exit non-zero only with `--check`.

### `otelplan compile`

Generate backend artifacts.

Options:

```text
--output .otelplan/build
--clean
```

### `otelplan build [go build arguments...]`

Compile and execute backend build.

Everything after `--` is passed to the Go build command.

Example:

```bash
otelplan build -- -o bin/api ./cmd/api
```

### `otelplan version`

Show:

- OTelPlan version;
- Go version;
- configured backend;
- backend version if available.

## Exit codes

```text
0  success
1  general failure
2  invalid CLI usage
3  invalid policy
4  project analysis failure
5  validation failure
6  stale lockfile
7  backend incompatibility
8  build failure
```

Do not overload dozens of shell exit codes; detailed diagnostics live in output.

## JSON contract

JSON output is versioned:

```json
{
  "apiVersion": "otelplan.io/cli/v1alpha1",
  "command": "validate",
  "ok": false,
  "diagnostics": []
}
```

## CI examples

```bash
otelplan validate --format=json
otelplan lock --check
otelplan build -- -trimpath -o app ./cmd/app
```
