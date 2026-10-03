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

The internal validator accepts supplemental deny patterns. The current policy
schema and CLI do not expose a configurable denylist. Built-in checks cannot be
disabled wholesale; sensitive captures require explicit per-attribute safety
acknowledgment.

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

The internal validator accepts threshold overrides. The CLI exposes
`--allow-large-plan` for the 2,000-target limit, with warnings retained. Arbitrary
threshold configuration is not currently a policy/CLI feature.

## 9. Recursion

Instrumenting recursive functions is allowed but can create deep traces.
Dedicated recursion warnings remain planned. Advisory call edges can help a
human review recursion but are not an automatic safety diagnostic.

## 10. Double instrumentation

Dedicated manual-span/double-instrumentation detection remains planned.
The current validator does not claim to detect existing `tracer.Start` calls or
automatically suppress them. Review overlap with existing application tracing.

## Returned-error messages

When `errors.record` is enabled, generated hooks call OpenTelemetry `RecordError`
with the application error. Its exception message can include the application's
error text even though the status description is fixed to `operation failed`.
This is distinct from CLI diagnostics and explicit argument/result attributes.
Use `errors.record: false` for targets whose error messages may contain private
data, or sanitize errors in the application. There is no runtime error-message
redaction policy in the current schema.
