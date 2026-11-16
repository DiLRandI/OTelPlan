# Architecture

## 1. Components

```text
                         +------------------+
                         |   Go codebase    |
                         +--------+---------+
                                  |
                                  v
+--------------+         +--------+---------+
| otelplan.yaml |-------->| Project Analyzer |
+------+-------+          +--------+---------+
       |                          |
       |                  normalized CodeModel
       |                          |
       v                          v
+------+---------------------------+------+
|             Policy Engine               |
| match -> exclude -> resolve -> explain  |
+-------------------+----------------------+
                    |
                    v
              ResolvedPlan
                    |
          +---------+---------+
          |                   |
          v                   v
   Validator             Lockfile
          |
          v
   Backend Compiler
          |
          v
      OTelC backend
          |
     rules + hooks
          |
          v
     instrumented build
```

## 2. Package boundaries

Current implementation packages:

```text
cmd/otelplan
internal/cli
internal/discovery
internal/suggest
internal/policy
internal/resolve
internal/validate
internal/lockfile
internal/compiler
internal/backend/otelc
internal/model
pkg/model                 # compatibility names
```

Presentation and diagnostics are handled by the CLI and domain model rather than
separate placeholder packages. Domain definitions belong to `internal/model`.
`pkg/model` retains existing type aliases, constants, and symbol helpers for Go
source compatibility while internal consumers migrate separately. New internal
types do not expand this compatibility package. Public Go types require a
demonstrated consumer need; the model package is not a stable SDK commitment.
Primary compatibility contracts are policy YAML, lockfiles, CLI JSON, and
diagnostic codes.

Symbol-ID constructors belong to the internal model; public constructor names
forward to them. The canonical parser retains its compatibility implementation
and error identities, with explicit parser-only bridges in the engine. Model
types and their methods are owned internally.

Field tags, serialized formats, helper behavior, and error identities remain the
same. Reflection reports the defining package as `internal/model`, so callers
that inspect `reflect.Type.PkgPath` observe the ownership change. Existing Go
imports, composite literals, and interface implementations remain supported by
the compatibility names.

## 3. Core domain models

### CodeModel

Represents what exists in the Go program.

Contains:

- modules;
- packages;
- symbols;
- type relationships;
- call relationships where available;
- context/error metadata;
- source metadata.

### Policy

Represents user intent from `otelplan.yaml`.

It contains selectors and instrumentation behavior but no backend-specific syntax.

### ResolvedPlan

Exact, backend-independent instrumentation plan.

```go
type ResolvedTarget struct {
    SymbolID        SymbolID
    SpanName        string
    ContextStrategy ContextStrategy
    ErrorStrategy   ErrorStrategy
    Attributes      []AttributePlan
    RuleID          string
}
```

### BackendCapabilities

The backend advertises support instead of the compiler assuming it.

```go
type BackendCapabilities struct {
    BeforeHook              bool
    AfterHook               bool
    ArgumentRead            bool
    ArgumentReplace         bool
    ResultRead              bool
    PanicObservation        bool
    ContextReplacement      bool
    FunctionEntrySelection  bool
    FunctionCallSelection   bool
}
```

### Backend boundary

The current adapter exposes pinned identity/capability validation and deterministic
rule, hook, accessor, and bundle generation through `internal/backend/otelc`.
`internal/compiler` owns artifact staging/publication, disposable module state,
selection verification, and backend execution. It uses caller-owned contexts for
long-running work. The backend boundary currently uses typed adapter functions
rather than a public Go interface. Any future backend must preserve the same
capability-validation and compiler ownership contracts. Policy loading/resolution
does not import OTelC syntax.

## 4. Why backend isolation matters

`otelc` is evolving. OTelPlan must not leak an unstable or backend-specific representation into `otelplan.yaml`.

If `otelc` changes a rule schema, only the adapter should change.

Future backends could include:

- decorator/code generation;
- eBPF configuration where applicable;
- another compile-time mechanism.

The product model remains stable.

## 5. Data flow

### `scan`

```text
go/packages -> types -> AST -> optional SSA -> CodeModel -> report
```

### `init`

```text
CodeModel -> deterministic heuristics -> SuggestedPolicy -> otelplan.yaml
```

### `validate`

```text
Policy + CodeModel -> Resolve -> ResolvedPlan -> Validators -> Diagnostics
```

### `lock`

```text
ResolvedPlan + backend identity -> canonical serialization -> otelplan.lock
```

### `compile`

```text
ResolvedPlan -> backend capability check -> backend compiler -> generated artifacts
```

### `build`

```text
validate -> optional locked-resolution check -> generate -> isolated module/backend verification -> backend build -> verify/publish binary
```

`build --locked` verifies resolution freshness before backend execution. Default
build mode permits an absent lock; neither mode refreshes it. Compile publishes an
independently verified artifact manifest. See [lock ownership](09_LOCKFILE_REPRODUCIBILITY.md).

## 6. No hidden mutation

Commands must not silently edit:

- application `.go` files;
- `go.mod`;
- `go.sum`;
- `go.work`.

If a backend operation needs module changes, OTelPlan must either:

1. operate in an isolated work directory; or
2. require an explicit flag and show the exact changes.

The default should favor an isolated, reproducible build workspace.

## Design invariants and scope

- Policy is authoritative. Discovery and initializer scores provide deterministic,
  explained suggestions; they do not silently choose production instrumentation.
- Selection uses Go symbols, types, method sets, interfaces, source metadata, and
  explicit policy. Service/Repository names and directory layouts are not required.
- Broad selectors resolve to exact declarations before backend generation.
  Inspect, explain, diff, and locks make selection and drift reviewable.
- Application Go files and module/workspace state remain unchanged. Backend
  execution uses disposable copies; explicit policy, lock, and artifact outputs
  retain their documented ownership and replacement checks.
- Argument/result capture requires explicit mappings. Sensitive-name defaults
  cannot be removed by custom patterns, and capture cannot be enabled wholesale.
- Required context propagation fails validation when it cannot be represented.
  Explicit root strategies and nil-context behavior follow the runtime contract;
  there is no inferred ambient continuation.
- The pinned adapter validates capabilities and exact identities before rendering.
  Unsupported targets fail without partial artifacts or silent uninstrumented fallback.
- Generated spans use OpenTelemetry APIs. Applications own their SDK, exporter,
  transport, and Collector configuration. OTelPlan analysis stays local and the
  CLI installs no telemetry exporter or analytics.
- Policy, lock, CLI JSON, diagnostic codes, and exit categories are the primary
  compatibility contracts. Generation, canonicalization, and diagnostics remain
  deterministic for equivalent inputs. Public Go types require a demonstrated
  consumer need; speculative interfaces and extension points are not release work.

The stable scope excludes arbitrary code-block capture, source-patch generation,
a replacement compiler-weaving engine, vendor-specific transaction APIs, GUI/SaaS
or IDE products, LLM-based inference, and additional backends. Verified limitations
of the current pin belong in the [backend reference](OTELC_BACKEND.md), not in
unimplemented interface proposals. Compatibility and migration commitments belong
in [COMPATIBILITY.md](COMPATIBILITY.md). The [acceptance criteria](14_ACCEPTANCE_CRITERIA.md)
remain mandatory; a green development check does not establish release readiness.

Backend updates must be checked against the exact upstream source, rule schema,
hook API, and real trace tests. Upstream main, design proposals, competitor
comparisons, or issue closure do not establish capabilities of the pinned binary.
