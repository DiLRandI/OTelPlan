# Architecture

## 1. Components

```text
                         +------------------+
                         |   Go codebase    |
                         +--------+---------+
                                  |
                                  v
+-------------+          +--------+---------+
| otelplan.yml|--------->| Project Analyzer |
+------+------+          +--------+---------+
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

Suggested implementation packages:

```text
cmd/otelplan
internal/project
internal/discovery
internal/policy
internal/resolve
internal/validate
internal/inspect
internal/lockfile
internal/compiler
internal/backend
internal/backend/otelc
internal/diagnostic
internal/config
internal/fs
pkg/model
```

Keep the public surface small. Most implementation packages should remain `internal`.

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

### Backend

```go
type Backend interface {
    Name() string
    Version(ctx context.Context) (string, error)
    Capabilities(ctx context.Context) (BackendCapabilities, error)
    Validate(ctx context.Context, plan ResolvedPlan) []Diagnostic
    Compile(ctx context.Context, plan ResolvedPlan, outDir string) (Artifacts, error)
    Build(ctx context.Context, req BuildRequest) error
}
```

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
validate -> lock check -> compile -> backend build -> binary
```

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
