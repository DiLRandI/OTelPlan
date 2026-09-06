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
  version: "PINNED_VERSION"
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
- `root`: explicitly permit a root span;
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

Allowed value types should initially be conservative:

- bool;
- integer;
- float;
- string;
- stringer only with explicit opt-in.

No automatic JSON encoding.

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
