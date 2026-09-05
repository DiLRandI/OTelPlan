# Acceptance Criteria

## Release-blocking criteria

### A. Architecture independence

Given all required architecture fixtures, users can select meaningful operations without relying on `Service`/`Repository` names.

### B. Exactness

`inspect` shows the exact canonical symbols that will be instrumented.

No backend build may select additional application symbols not present in the resolved plan.

### C. Source immutability

Running:

```text
scan
validate
lock
compile
build
```

does not modify application `.go` files.

Any `go.mod`/`go.sum` mutation must either be isolated or explicitly requested.

### D. Context correctness

For methods with `context.Context`, nested business spans join the expected trace and propagate the new child context to downstream instrumented operations where backend capability permits.

For methods without usable context, default behavior is a clear diagnostic rather than silently disconnected spans.

### E. Error correctness

Non-nil returned errors are recorded according to the plan without changing return behavior.

### F. Privacy

No argument or result is captured without an explicit attribute rule.

Known secret patterns fail validation unless explicitly acknowledged.

### G. Determinism

Same source + same policy + same Go version + same backend version yields byte-equivalent lockfile and semantically equivalent generated backend plan.

### H. Reviewability

Every selected target can answer:

```text
Why was I selected?
Which policy rule selected me?
What span name will I have?
Which context is used?
Which attributes are captured?
Which error is recorded?
```

### I. CI

A repository can enforce:

```bash
otelplan validate --strict
otelplan lock --check
```

without interactive prompts.

### J. Trace E2E

At least three fixture architectures produce verified parent/child OTEL spans through the real backend.

### K. Backend isolation

No backend-specific rule syntax appears in the public policy schema.

### L. Quality

- race detector clean;
- vet clean;
- linter clean;
- no known critical/high security findings in dependencies;
- tests cover matching conflict/precedence and safety behavior.
