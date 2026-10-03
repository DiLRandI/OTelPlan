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
pkg/model
```

Presentation and diagnostics are handled by the CLI and domain model rather than
separate placeholder packages. The public Go model package currently supports
internal cross-package contracts; it is not a stable SDK commitment. Public Go
types require a demonstrated consumer need. Primary compatibility contracts are
policy YAML, lockfiles, CLI JSON, and diagnostic codes.

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
validate -> generate -> isolated module/backend verification -> backend build -> verify/publish binary
```

Build does not check or refresh `otelplan.lock`. Enforce `lock --check` explicitly
in CI before building. Compile publishes an independently verified artifact
manifest. See [lock ownership](09_LOCKFILE_REPRODUCIBILITY.md).

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
