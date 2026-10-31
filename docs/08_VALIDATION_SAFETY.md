# Validation, Privacy, and Safety

## Principle

A tracing tool can accidentally create a data leak or telemetry bill. Safety is a core feature.

## 1. Validation levels

### Error

Build/lock cannot continue.

Examples:

- invalid policy;
- unknown template variable;
- unresolved exact symbol;
- conflicting rules;
- backend cannot satisfy required context propagation;
- explicit capture of a secret without unsafe acknowledgment.

### Warning

Operation may continue unless `--strict`.

Examples:

- root span requested inside a likely traced call path;
- high-cardinality attribute source;
- broad selector matching hundreds of methods;
- no returned `error` but `errors.record=true`;
- source selector includes generated code.

### Info

Explanatory diagnostics.

## 2. Secret detection

Case-insensitive suspicious names include:

```text
password
passwd
pwd
secret
token
access_token
refresh_token
authorization
cookie
api_key
apikey
private_key
credential
session
```

Search both:

- telemetry key;
- source field/path.

Policies can add sensitive-name fragments:

```yaml
denyPatterns:
  - bank_reference
  - account_number
```

These literal fragments supplement built-in patterns. Matching ignores case and
dot, underscore, and hyphen separators, using the attribute key and argument/result
path. Entries must contain a non-separator character and no whitespace. An omitted
or empty list leaves every built-in check enabled. Sensitive captures still
require `safety: {classification: secret, allow: true}` on that attribute; public
classification cannot bypass the check. Captured constants and deny-pattern
contents are not echoed in these diagnostics.

Order and duplicate entries do not affect the policy digest. A changed set of
configured fragments changes policy identity and can make a lock stale. Analysis
cache reuse does not bypass current policy safety validation.

## 3. PII

Potential PII patterns should warn:

```text
email
phone
address
name
user_id
customer_id
ip
device_id
```

Not all identifiers are prohibited, but users must make deliberate decisions.

## 4. Cardinality

Warn for likely unbounded values:

- raw URL;
- query string;
- request/response bodies;
- UUID-like IDs;
- order IDs;
- user IDs;
- timestamps;
- arbitrary error messages.

Offer bounded alternatives in diagnostics where possible.

## 5. No automatic argument/result capture

This is mandatory.

The policy:

```yaml
attributes:
  arguments: false
  results: false
```

must be the default and cannot be changed globally to "capture everything" without an explicit unsafe feature gate.

## 6. Local-only operation

OTelPlan code analysis is local.

The CLI must not upload source, symbol names, code model, or policy to an external service.

If AI-assisted discovery is ever added, it must be a separate opt-in subsystem with an explicit data-handling contract.

## 7. Generated artifact safety

Work directories:

- permissions should follow platform-safe defaults;
- must not intentionally contain captured runtime data;
- should be safe to delete;
- must not contain source copies unless technically required.

## 8. Denial of service / telemetry explosion

Warn or fail when a policy resolves to an unusually broad set.

Suggested defaults:

```text
> 100 targets: warning
> 500 targets: strong warning
> 2000 targets: require explicit --allow-large-plan
```

Policy commands accept `--warn-targets`, `--strong-warn-targets`, and
`--max-targets`. Values must be positive integers; the first warning threshold
cannot exceed the strong-warning threshold. A maximum may be lower than either
warning threshold when a stricter cap is desired. A boundary is exceeded only
when the selected count is greater than its configured value.

Without an explicit maximum, `--allow-large-plan` retains its existing behavior:
it acknowledges exceeding 2,000 targets while keeping warnings. Raising an
explicit maximum above 2,000 also requires that acknowledgment. An explicit
maximum remains binding even with `--allow-large-plan`; exceeding it fails
validation rather than silently bypassing the configured cap.

These flags control validation, not selection or lock identity. They cannot
bypass context, secret/PII, attribute, or backend checks. Init and scan do not
accept these policy controls; their existing conservative behavior is unchanged.

## 9. Recursion

Instrumenting recursive functions is allowed but can create deep traces.
Discovery marks declarations that belong to statically resolved call cycles,
including mutual recursion through other analyzed declarations. Selecting one
produces warning `OTP4006` with its policy rule and exact symbol; `--strict`
treats the warning as a validation failure. A caller that only reaches a cycle
is not itself marked recursive. No target is automatically excluded.

This check uses typed calls in analyzed bodies without requiring the optional
SSA call graph. Immediately invoked closures are included; stored or returned
closure bodies are not attributed to the enclosing declaration. Arbitrary
function-value calls, interface dispatch, reflection, and cycles through bodies
outside the analyzed package set are not resolved. A static cycle does not
prove that the recursive path executes for a particular input. Absence of a
warning is not proof that recursion is impossible.

## 10. Double instrumentation

Selecting a declaration with a recognizable OpenTelemetry span-start call
produces warning `OTP4007`. The check uses Go types: a `Start` method must have
a receiver implementing `go.opentelemetry.io/otel/trace.Tracer`. It recognizes
interface calls, aliases, concrete wrappers, promoted methods, and method
expressions. Unrelated methods named `Start` do not match by name alone.

Immediately invoked closure bodies are included. Stored/returned closures,
indirect function-value calls, helper-function bodies, and reflection are not
attributed to the enclosing declaration. The warning does not prove that a
particular path executes or that spans are duplicates. Absence of a warning is
not proof that manual instrumentation is absent.

The diagnostic retains the selected symbol and rule without exposing span names
or attribute values. No target is automatically excluded and no application span
is suppressed. Review the overlap and use ordinary policy exclusions when
appropriate. Existing `--strict` semantics treat this warning as a validation
failure.

## Returned-error messages

When `errors.record` is enabled, generated hooks call OpenTelemetry `RecordError`
with the application error. Its exception message can include the application's
error text even though the status description is fixed to `operation failed`.
This is distinct from CLI diagnostics and explicit argument/result attributes.
Use `errors.record: false` for targets whose error messages may contain private
data, or sanitize errors in the application. There is no runtime error-message
redaction policy in the current schema.

Formatting panics produce the bounded fallback described in
[error semantics](07_RUNTIME_HOOKS.md#7-error-semantics). The guard does not redact
normally returned error messages.
