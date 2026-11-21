# Policy and safety

File name: `otelplan.yaml`

Supported identity: `otelplan.io/v1alpha1`. CLI releases and format versions are
separate; see [compatibility](COMPATIBILITY.md).

## Design principles

- readable;
- backend-independent;
- explicit;
- composable;
- safe by default;
- deterministic.

## Top level

```yaml
apiVersion: otelplan.io/v1alpha1
kind: InstrumentationPlan

project: {}
backend: {}
defaults: {}
denyPatterns: []
rules: []
exclusions: []
```

## Project

```yaml
project:
  packages:
    - "./..."
  includeTests: false
  includeDependencies: false
  buildTags: []
```

## Backend

```yaml
backend:
  name: otelc
  version: "v1.1.0"
```

`version` must be exact for lock/build workflows.

## Defaults

```yaml
defaults:
  context:
    mode: require
  errors:
    record: true
  attributes:
    arguments: false
    results: false
```

When `spanName` is omitted, functions use `{{package}}.{{function}}` and methods
use `{{package}}.{{receiver}}.{{method}}`. An explicit template replaces that default.

Context modes:

- `require`: skip/error if no context is available;
- `root`: permit a root fallback only when the declaration has no context argument;
Multiple context arguments are rejected; there is no ambient continuation mode.

## Rules

Each rule has a stable ID.

```yaml
rules:
  - id: checkout
    description: Trace checkout business operations
    match:
      packages:
        - "github.com/acme/shop/internal/checkout"
      exported: true
      hasContext: true
    span:
      name: "{{package}}.{{receiver}}.{{method}}"
```

### Match fields

All specified fields in a single `match` block are ANDed. Lists inside a field
are ORed. An empty match is invalid.

| Field | Value |
| --- | --- |
| `packages`, `files`, `functions`, `receivers`, `methods` | Lists of glob patterns. |
| `symbols` | List of exact canonical symbol IDs. |
| `implements` | List of interface identities. |
| `exported`, `hasContext`, `returnsError` | Boolean `true` or `false`; omission leaves the constraint unset. |
| `ownership` | `application`, `dependency`, or `any`. |

Example:

```yaml
match:
  packages:
    - "github.com/acme/shop/internal/payment"
    - "github.com/acme/shop/internal/checkout"
  methods:
    - "Create"
    - "Authorize"
```

means:

```text
(package payment OR checkout) AND (method Create OR Authorize)
```

## Exact symbols

Exact symbols are the strongest and safest selector.

```yaml
match:
  symbols:
    - "github.com/acme/shop/internal/payment.(*Processor).Authorize"
```

`otelplan inspect` can emit canonical symbols for copy/paste.

## Interfaces

```yaml
match:
  implements:
    - "github.com/acme/shop/internal/ports.PaymentGateway"
```

This matches concrete methods that satisfy the selected interface.

Optional method narrowing:

```yaml
match:
  implements:
    - "github.com/acme/shop/internal/ports.PaymentGateway"
  methods:
    - "Authorize"
```

## Exclusions

Exclusions are globally applied after inclusion.

```yaml
exclusions:
  - id: health
    match:
      methods:
        - "Health"
        - "Ready"

  - id: generated
    match:
      files:
        - "**/*.gen.go"
        - "**/mock_*.go"
```

## Span configuration

```yaml
span:
  name: "{{package}}.{{receiver}}.{{method}}"
  kind: internal
```

Only `internal` is supported for business spans. Protocol span kinds are not inferred.

## Error configuration

```yaml
errors:
  record: true
```

The initial release does not allow arbitrary error-message attributes by default.

## Attributes

Safe explicit mapping:

```yaml
attributes:
  - key: "order.type"
    from:
      argument: "order.Type"

  - key: "payment.method"
    from:
      argument: "payment.Method"
```

Supported capture value types:

- bool;
- integer;
- float;
- string;
- named primitive aliases whose underlying type is supported.

Object serialization is not automatic. Attribute
sources support `argument`, `result`, `constant`, `symbol`, and `errorType`, with
one source per mapping. Capture cannot be enabled wholesale through defaults.

No automatic JSON encoding.

### Explicit Stringer conversion

Argument/result attributes can opt in to a type-checked `String() string` method:

```yaml
attributes:
  - key: order.category
    from: {argument: order.Category}
    stringer: true
```

The declared method set must implement `String() string`. Pointer/value receiver,
interface, alias, and promoted-method rules follow Go's type system; the tool
does not take an implicit address to enable a pointer-only method. Conversion
cannot be applied to constants, symbol metadata, or error-type classification.
It produces a string attribute and remains explicit in inspection and the lock.
Without this flag, named primitives keep their underlying scalar representation,
and object/interface capture stays prohibited.

Calls occur only for recording spans. Nil statically nilable values and nil field
paths are omitted. An interface holding a typed nil follows its method's normal
behavior; a formatting panic omits the attribute. Recovery surrounds only the
reader call in generated runtime code, preserving application panic propagation.
The panic value is not recorded or printed. There is no reflection or automatic
JSON marshaling for conversion. Existing secret/PII and cardinality checks apply
to the explicit key and source.

Choose pure, bounded String methods. Opt-in invokes application code: panic
recovery cannot undo side effects or stop a blocking method.

### Symbol metadata attributes

Static declaration facts require explicit keys and source fields:

```yaml
attributes:
  - key: code.operation
    from: {symbol: id}
  - key: code.package
    from: {symbol: packageName}
```

| Field | String value |
| --- | --- |
| `id` | Full canonical symbol ID. |
| `package` | Go import path. |
| `packageName` | Declared Go package name. |
| `name` | Function or method name. |
| `receiver` | Receiver type, with `*` for pointer receivers; empty for functions. |
| `signature` | Analyzed Go signature. |
| `kind` | `function` or `method`. |

These sources use no application arguments, results, object serialization, or
method calls. Source positions are not supported metadata fields. Inspection
retains `from.symbol`; the lock additionally freezes its resolved `symbolValue`.
Changing a captured fact creates attribute drift, including a package-name
change when the span name is fixed. Ordinary source line movement does not.
Existing key, classification, acknowledgment, and reserved-namespace checks apply.

### Error-type classification attributes

Select one complete returned `error` by name or zero-based index:

```yaml
errors: {record: false}
attributes:
  - key: failure.class
    from: {errorType: err}
```

The selector uses the same declared `error`/alias results recognized for error
recording. It cannot select fields, arguments, or concrete result declarations
that the analyzer does not classify as error results. The output is the concrete
error type's full Go import path and declared name, for example
`errors.errorString` or `example.com/app.Failure`. Pointer shape and generic type
arguments are removed. An unnamed concrete error type uses the fixed class
`unnamed`; its fields and type tags are never formatted.

A nil error interface omits the attribute. A typed nil held inside an error
interface is classified, following Go's non-nil-interface semantics. Wrappers
keep their own class; classification does not unwrap the error. It reads only
runtime type identity and never calls `Error`, `String`, or `Unwrap`.

Classification is independent of error-event recording. Use the rule's
`errors: {record: false}` when only the explicit class attribute is wanted.
Existing attribute-key, sensitive-name, classification, and acknowledgment
checks still apply. The lock stores the selector and string kind; runtime error
values and messages never become lock data.

## Sensitive attribute override

A capture may require explicit acknowledgment. See [sensitive-name validation](#sensitive-name-validation) for secret handling. This example acknowledges a PII field.

```yaml
attributes:
  - key: "customer.email"
    from:
      argument: "customer.Email"
    safety:
      classification: pii
      allow: true
```

This makes risky capture visible in code review.

## Rule-local exclusions

Supported:

```yaml
rules:
  - id: domain
    match:
      packages:
        - "github.com/acme/shop/internal/domain/**"
    exclude:
      methods:
        - "String"
        - "Validate"
```

## Templates

Initial stable variables:

- `{{symbol}}`
- `{{package}}`
- `{{import_path}}`
- `{{function}}`
- `{{receiver}}`
- `{{method}}`

Unknown variables are errors.

## Ordering and precedence

1. parse/schema validation;
2. resolve inclusion rules;
3. apply rule-local exclusions;
4. union selected targets;
5. apply global exclusions;
6. detect duplicate/conflicting target configuration;
7. produce exact `ResolvedPlan`.

A target matched by multiple rules with conflicting behavior is an error unless the behaviors are structurally identical.

No "last rule wins" behavior in v1.

A rule with include matches that are all removed by local/global exclusions emits
warning `OTP1006`. This makes ineffective rules visible without changing selection
or exclusion precedence. `--strict` rejects that warning. Partially excluded rules
and active identical overlaps remain valid; an include selector matching no symbols
still produces `OTP1002`.

## Environment interpolation

Not supported in v1.

Policy checked into source control must resolve identically in CI. Runtime OTEL exporter configuration belongs in normal OTEL environment configuration, not in OTelPlan policy.

## Sensitive-name validation

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
dot, underscore, and hyphen separators. Validation checks the attribute key,
selector spelling, and resolved Go declaration and field path. Entries must contain a non-separator character and no whitespace. An omitted
or empty list leaves every built-in check enabled. Sensitive captures still
require `safety: {classification: secret, allow: true}` on that attribute; public
classification cannot bypass the check. Captured constants and deny-pattern
contents are not echoed in these diagnostics.

Order and duplicate entries do not affect the policy digest. A changed set of
configured fragments changes policy identity and can make a lock stale. Analysis
cache reuse does not bypass current policy safety validation.

## PII warnings

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

## Cardinality warnings

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

## Target-count limits

Warn or fail when a policy resolves to an unusually broad set.

Default selected-target thresholds:

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

## Recursion

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

## Manual-span overlap

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
[error semantics](OTELC_BACKEND.md#returned-errors-and-formatting-panics). The guard does not redact
normally returned error messages.

Errors stop the operation. Warnings remain visible and fail `--strict`; policy
commands validate before lock/artifact writes or backend execution. Diagnostic
codes and process exits are documented in [CLI](CLI.md#json-and-diagnostics).
