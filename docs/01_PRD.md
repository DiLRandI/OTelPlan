# Product Requirements Document

## 1. Product

Name: **OTelPlan**

Category: Go observability developer tooling.

Positioning:

> Policy-driven, zero-source-change business tracing for Go.

## 2. Goals

### G1 — Architecture-agnostic selection

Support projects organized by package, file, concrete type, interface, implementation relationship, function, method, exact symbol, or combinations of these.

No required naming convention.

### G2 — Source-code independence

The normal instrumentation workflow must not edit application source files.

Generated build artifacts must live under an OTelPlan-controlled work directory or explicit output directory.

### G3 — Human-owned policy

Discovery generates suggestions only. `otelplan.yaml` is the authoritative intent.

### G4 — Meaningful business spans

Create spans around selected application functions/methods while preserving parent-child context where technically possible.

### G5 — Safe defaults

Argument/result capture is disabled by default. Sensitive fields require explicit opt-in. High-cardinality attributes must be detectable or warnable.

### G6 — Reproducible instrumentation

A lockfile records every resolved target, signature fingerprint, policy rule, backend version, and generated backend artifact digest.

### G7 — Explainability

For every selected or rejected symbol, the tool must be able to explain which rule caused the result.

### G8 — CI suitability

All non-interactive operations must support deterministic output, machine-readable JSON, stable exit codes, and no prompts.

## 3. Non-goals

The first stable release will not:

- infer business meaning solely through an LLM;
- modify user source code;
- replace `otelc`;
- replace the OpenTelemetry SDK or Collector;
- capture arbitrary local code blocks inside a function;
- promise correct context propagation for functions that have no usable context path;
- implement vendor-specific New Relic transaction APIs;
- instrument unsupported compiler constructs by patching the Go compiler.

## 4. Functional requirements

### FR-001 Project loading

Load one or more Go modules using the standard Go build context.

Inputs:

- module root;
- package patterns;
- build tags;
- GOOS / GOARCH;
- environment overrides.

Must respect:

- `go.work`;
- `replace` directives;
- vendor mode;
- build constraints;
- generated files.

### FR-002 Symbol model

Build a normalized model for:

- packages;
- source files;
- functions;
- methods;
- receiver types;
- interfaces;
- concrete types;
- method sets;
- parameters;
- results;
- `context.Context` parameters;
- `error` results;
- source positions;
- export visibility;
- generic type/function/method information supported by the active Go version.

Each symbol gets a stable canonical ID.

Example:

```text
github.com/acme/shop/internal/payment.(*Processor).Authorize
```

### FR-003 Discovery

`otelplan scan` must produce a code inventory without selecting instrumentation.

It may also produce **suggestions** using deterministic heuristics such as:

- exported functions;
- methods reachable from known entrypoints;
- methods implementing explicitly selected interfaces;
- application-owned packages;
- call graph centrality;
- functions containing infrastructure calls;
- functions returning errors;
- functions accepting `context.Context`.

Suggestions must include confidence and evidence. Suggestions are never automatically committed to the final policy without an explicit user action.

### FR-004 Policy matching

Rules must support:

- package glob;
- file glob;
- exact symbol;
- function name glob;
- receiver type glob;
- interface implementation;
- method name glob;
- exported/unexported;
- has context;
- returns error;
- source ownership: application/dependency;
- include and exclude combinators.

All final matches resolve to exact canonical symbols.

### FR-005 Exclusions

Exclusion rules have higher precedence than inclusion rules unless an explicit rule-level override is supported in a future schema version.

Default recommended exclusions:

- generated code;
- `_test.go`;
- health/readiness endpoints;
- `String`, `Marshal*`, `Unmarshal*` style utility methods when selected only by broad rules;
- dependency code unless explicitly requested.

### FR-006 Span naming

Support templates using stable low-cardinality metadata:

```text
{{package}}.{{receiver}}.{{method}}
{{import_path}}.{{function}}
{{symbol}}
```

Default:

- methods: `<package>.<receiver>.<method>`
- functions: `<package>.<function>`

### FR-007 Context strategy

For each target, determine one of:

- `argument`: use a `context.Context` argument;
- `ambient`: backend-supported continuation only if explicitly supported and safe;
- `root`: start a root span;
- `skip`: do not instrument.

Default:

- use `argument` if available;
- otherwise `skip` with a diagnostic.

Never silently invent propagation.

### FR-008 Error handling

If a selected function/method returns an `error`, generated hooks must:

- record non-nil error;
- set span status according to OpenTelemetry conventions;
- always end the span.

Panic behavior:

- span must end when backend hooks support guaranteed after execution;
- optionally record panic if backend exposes it;
- never recover/swallow a panic.

### FR-009 Attributes

Default capture:

```yaml
arguments: false
results: false
```

Allowed attribute sources:

- constants;
- selected primitive/string-like parameter fields;
- selected result fields;
- symbol metadata;
- error classification.

Every attribute rule requires an explicit key.

Never serialize whole objects by default.

### FR-010 Attribute safety

Validation must reject or warn on likely secrets:

- password;
- passwd;
- secret;
- token;
- authorization;
- cookie;
- API keys;
- private keys;
- credentials.

The denylist is configurable but secure defaults cannot be disabled without an explicit unsafe flag.

### FR-011 Inspect

`otelplan inspect` must show:

- selected targets;
- excluded targets;
- span names;
- context strategy;
- error strategy;
- attributes;
- source rule;
- warnings;
- unsupported targets.

### FR-012 Explain

`otelplan explain <symbol>` explains matching step-by-step.

### FR-013 Validate

Validation categories:

- syntax/schema;
- project load;
- unresolved selectors;
- ambiguous selectors;
- context propagation;
- attribute safety;
- cardinality;
- backend capabilities;
- backend version compatibility;
- stale lockfile;
- rule shadowing;
- duplicate span selection.

### FR-014 Lock

Resolve policy to exact targets and write `otelplan.lock`.

Locking must be deterministic.

### FR-015 Diff

Compare policy/lock state against current code.

Report:

- added target;
- removed target;
- signature changed;
- source moved but canonical symbol stable;
- rule changed;
- backend changed;
- safety classification changed.

### FR-016 Compile

Compile exact targets into backend artifacts.

Initial backend: `otelc`.

OTelPlan's public policy schema must not expose raw `otelc` pointcut/advice syntax.

### FR-017 Build

`otelplan build` orchestrates:

1. validate;
2. ensure lock state according to mode;
3. compile;
4. invoke pinned backend;
5. invoke Go build;
6. clean temporary state according to configuration.

### FR-018 Dry-run

All mutating/generating commands support `--dry-run` where meaningful.

### FR-019 JSON output

Commands must support `--format=json`.

### FR-020 Diagnostic stability

Diagnostics have stable codes, e.g.:

```text
OTP1001 invalid-policy
OTP2001 unresolved-symbol
OTP3001 missing-context
OTP4001 sensitive-attribute
OTP5001 backend-unsupported
OTP6001 stale-lock
```

## 5. Quality requirements

- No panic on malformed project input.
- Clear errors with source locations.
- Deterministic generated files.
- Unit tests for every matcher.
- Golden tests for policy -> resolved plan.
- Integration tests that build and run an instrumented fixture.
- Race-detector clean.
- `go vet` clean.
- Strong linting.
- No telemetry sent by the OTelPlan CLI itself unless an explicit future opt-in feature is added.

## 6. Compatibility

Initial supported Go line: Go 1.27.x.

The implementation should avoid needlessly depending on 1.27-only syntax so future support for older supported Go lines remains possible.

Backend compatibility is capability-negotiated and pinned; see `06_COMPILER_BACKEND.md`.
