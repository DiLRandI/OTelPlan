# OTelPlan Policy Specification

File name: `otelplan.yaml`

Schema version: `v1alpha1` during development.

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
  spanName: "{{package}}.{{receiver}}.{{method}}"
  context:
    mode: require
  errors:
    record: true
  attributes:
    arguments: false
    results: false
```

Context modes:

- `require`: skip/error if no context is available;
- `root`: permit a root fallback only when the declaration has no context argument;
- future modes may be added only with clear backend semantics.

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

All fields in a single `match` block are ANDed unless specified otherwise.

```yaml
match:
  packages: []
  files: []
  symbols: []
  functions: []
  receivers: []
  methods: []
  implements: []
  exported: true|false
  hasContext: true|false
  returnsError: true|false
  ownership: application|dependency|any
```

Lists inside one field are ORed.

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

Initial release only permits `internal` for business spans unless a rule explicitly represents a protocol semantic convention supported by a future extension.

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

## Additional sensitive-name patterns

The optional root `denyPatterns` list adds literal name fragments to the built-in
secret checks. It cannot remove built-in protections:

```yaml
denyPatterns: [bank_reference, account_number]
```

Matching ignores case and `.`, `_`, and `-` separators. Entries cannot be empty,
contain whitespace, or consist only of separators. Matching applies to telemetry
keys and selected argument/result paths. Order and duplicates are irrelevant to
lock identity; changing the configured set is a policy change. See
[safety validation](08_VALIDATION_SAFETY.md#2-secret-detection).

## Sensitive attribute override

A suspicious key/path should fail validation unless explicitly acknowledged:

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

## Environment interpolation

Not supported in v1.

Policy checked into source control must resolve identically in CI. Runtime OTEL exporter configuration belongs in normal OTEL environment configuration, not in OTelPlan policy.
