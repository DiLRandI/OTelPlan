# Discovery Engine Specification

## Principle

Discovery answers **what exists** and **what may be interesting**.

It does not decide what is instrumented.

## 1. Inputs

- module root;
- package patterns, default `./...`;
- Go build tags;
- GOOS / GOARCH;
- test inclusion flag;
- dependency inclusion flag.

## 2. Loading

Prefer `golang.org/x/tools/go/packages` with enough load mode to obtain:

- syntax;
- types;
- types info;
- imports;
- compiled files;
- module metadata.

Use `go/types` for semantic relationships.

Use AST for source-level metadata.

SSA/call graph is optional for basic matching but required for advanced suggestion ranking.

## 3. Canonical symbol IDs

Functions:

```text
<import-path>.<function>
```

Methods:

```text
<import-path>.(<receiver>).<method>
<import-path>.(*<receiver>).<method>
```

Canonicalization must distinguish pointer/value receiver where semantically necessary.

For generic declarations, IDs do not include instantiated type arguments; signature fingerprints do.

## 4. Indexed metadata

For every symbol:

```text
identity
package
file
line/column
visibility
kind: function|method
receiver
receiver pointer/value
parameters
results
has context.Context
context argument indexes
returns error
error result indexes
generic declaration metadata
generated-file flag
test-file flag
module ownership
interface implementation relations
```

## 5. Interface implementation index

Support queries such as:

```yaml
implements:
  - "github.com/acme/shop/internal/ports.PaymentGateway"
```

Use `types.Implements` against method sets.

Handle both `T` and `*T`.

Do not require compile-time explicit declarations because Go interfaces are structural.

## 6. Suggested-policy heuristics

Suggestions are optional and must be deterministic.

Candidate scoring inputs may include:

- application-owned symbol;
- exported;
- receives `context.Context`;
- returns `error`;
- reachable from entrypoint;
- calls an already instrumentable infrastructure boundary;
- interface implementation;
- fan-in/fan-out;
- file/package grouping.

Never score based solely on words such as Service/Repository.

Name patterns may appear as weak evidence only and must be disclosed.

## 7. Entrypoints

Recognize technical entrypoints for discovery assistance:

- `main.main`;
- HTTP handlers when statically identifiable;
- gRPC methods;
- message handlers through known integration metadata.

Entrypoint recognition should be pluggable.

## 8. Call graph

Advanced mode:

- build SSA;
- construct a conservative call graph;
- retain confidence/precision metadata.

The call graph is used to help humans understand trace boundaries, not as the sole instrumentation selector.

Dynamic dispatch and reflection mean a complete precise call graph is not guaranteed. Diagnostics/UI must not present it as certain when it is conservative.

## 9. Output

Human format example:

```text
PACKAGE internal/payment

  (*Processor).Authorize(ctx, Payment) error
    context: arg[0]
    error: result[0]
    implements:
      internal/ports.PaymentGateway.Authorize
    called by:
      internal/checkout.(*Checkout).Submit

  (*Processor).Refund(ctx, Refund) error
    context: arg[0]
    error: result[0]
```

JSON output must use a versioned schema.

## 10. Performance

Cache analysis keyed by:

- Go version;
- module graph digest;
- build flags;
- file content digests.

A second scan without source/module changes should avoid full rebuild of semantic state where practical.
