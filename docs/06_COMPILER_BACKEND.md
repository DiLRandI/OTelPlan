# Compiler and Backend Specification

## 1. Principle

OTelPlan compiles a backend-neutral `ResolvedPlan` into backend artifacts.

Implemented backend: OpenTelemetry Go compile-time instrumentation (`otelc v1.1.0`).
See the authoritative [OTelC reference](OTELC_BACKEND.md) for capabilities, upstream
limitations, installation, and compile/build integration.

## 2. Why not emit raw backend rules directly from user policy

Backend syntax and capabilities can change independently.

The pipeline must be:

```text
otelplan.yaml
   -> Policy
   -> ResolvedPlan
   -> backend capability validation
   -> backend-specific model
   -> generated artifacts
```

Never:

```text
otelplan.yaml -> string templates -> otelc YAML
```

## 3. Backend version

A build-grade policy must pin an exact backend version.

The lockfile records:

- backend name;
- exact configured version;
- binary digest when available;
- supported capability snapshot.

## 4. OTelC integration assumptions

The current `otelc` project provides compile-time instrumentation via Go `-toolexec`, uses instrumentation rules to select functions/calls, and can apply before/after advice hooks.

OTelPlan must verify actual support against the pinned version rather than assume every documented/development feature exists.

## 5. Generated artifacts

Default published artifact directory:

```text
.otelplan/
  build/
    manifest.json
    rules/
    hooks/
    accessors/
    go.mod
    go.sum
```

This directory should be gitignored except when debugging.

The manifest records every file and digest.

## 6. Runtime hook package

OTelPlan generates a standalone hook runtime that:

- obtains tracer;
- starts span;
- safely stores per-call state;
- replaces context argument where supported;
- records configured attributes;
- records errors;
- ends span.

Generated rules reference these hook functions.

The runtime uses the OpenTelemetry API and stays vendor neutral. The application
configures its SDK/exporter; hooks do not install one.

## 7. Hook state

Each invocation needs isolated state.

State includes:

- span;
- derived context;
- target metadata;
- possibly attribute extraction metadata.

Never use a global mutable map keyed only by goroutine identity.

Use backend-provided hook state/lifecycle facilities.

## 8. Context argument replacement

For a function:

```go
func Authorize(ctx context.Context, req Request) error
```

desired semantics:

```text
before:
  parent <- ctx
  childCtx, span <- Start(parent)
  replace function argument ctx with childCtx

original function executes

after:
  inspect error
  End(span)
```

If the backend cannot replace the context argument for a target, OTelPlan must not claim nested child operations will automatically inherit the new span.

This becomes a validation error in `context.mode=require`.

## 9. Backend capability probing

At compile time:

1. obtain backend version;
2. map version to known capabilities or query a machine-readable capability interface if available;
3. reject unsupported plan features;
4. include capability snapshot in build manifest.

## 10. Generated rule quality

Generated rules must:

- select exact symbols whenever possible;
- avoid broad patterns in backend output;
- prevent self-instrumentation of OTelPlan hook packages;
- include stable generated IDs;
- include comments or metadata tying each rule to source policy rule ID.

## 11. Build isolation

Current model:

- copy application modules, workspace state, and local replacements to a disposable workspace;
- add generated runtime with pinned dependencies;
- verify application and runtime dependency selection before backend execution;
- preserve module/vendor mode and compare regenerated application vendor inputs;
- verify generated artifacts and backend executable identity;
- verify and publish built binaries without dirtying the source checkout.

Compile publishes a standalone artifact bundle and manifest. Build does not
update or enforce the resolution lock. Use `lock --check` explicitly when CI must
freeze resolution. Resolution commands cannot erase independently-owned build
identity. See [reproducibility](09_LOCKFILE_REPRODUCIBILITY.md).

## 12. Failure behavior

Compilation fails if:

- target is unresolved;
- backend cannot represent an exact target;
- required hook behavior is unsupported;
- generated backend config fails backend validation;
- generated source fails `gofmt` or compile checks.

No silent fallback to uninstrumented code.
