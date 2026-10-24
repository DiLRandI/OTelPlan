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

Generic IDs identify declarations without instantiated type arguments. Signatures
retain declaration type parameters and constraints.

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

Suggestions use `main.main`, statically visible `net/http` registrations, and
typed gRPC service registrations as
technical roots. Discovery records the registration API identity in each exact
handler declaration's optional `entrypoints` metadata. It recognizes package
`HandleFunc`/`Handle` calls and the equivalent `ServeMux` methods using Go types,
including import aliases, bound methods, and concrete `Handler` implementations.
Standard `http.HandlerFunc` adapters follow the underlying declaration; custom
function types follow their own `ServeHTTP` implementation. Registration values
such as route patterns are not retained.

For gRPC, discovery reads actual `Server.RegisterService` and
`ServiceRegistrar.RegisterService` calls with a statically known concrete service.
It follows known `ServiceDesc` initializers and the unary/stream handlers' typed
calls to the registered implementation. Wire method names and matching method
signatures alone do not identify application methods. One-level generated-style
registration wrappers are supported when they forward the service parameter
unchanged. Descriptor and handler source can live in an imported dependency;
including dependency symbols in the inventory is not required. Tests exercise
the generated health service from gRPC v1.81.1. Service names, wire names, and
descriptor metadata values are not retained.

Initialization uses the nearest root through statically resolved application
calls, with deterministic provenance when paths tie. HTTP handlers lacking an
explicit context parameter remain outside the default starter targets; eligible
downstream operations receive the additional evidence. Registered gRPC methods
with one explicit context parameter can themselves receive entrypoint evidence.
Streaming methods receiving context only through a stream remain outside default
starter targets. Policy remains authoritative.
The metadata does not prove that registration executes or that requests reach a
handler. Conservative call candidates do not become proven static paths.

Mutable function variables, interface-valued callbacks, anonymous handlers, and
indirect HTTP registration wrappers remain unresolved by this reader. gRPC
interface-valued implementations, dynamically constructed descriptors, reassigned
service parameters, and wrappers beyond the direct forwarding form remain
unresolved. These facts do not prove registration executes; arbitrary descriptor
mutation through aliases or external calls is not modeled. Message integration
metadata recognition remains separate discovery work. Recognizers are
private typed-analysis functions; adding integrations does not change policy syntax
or load executable plugins.

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

## Current implementation details

Go package and type discovery uses the effective toolchain, target platform, build tags, module mode, workspace, and cgo configuration. The lock graph fingerprints these inputs and normalized module/workspace contents without recording checkout paths.

Explicit `GOFLAGS=-mod=mod`, `-mod=readonly`, and `-mod=vendor` take precedence over vendor detection. Otherwise discovery defaults to readonly mode, or vendor mode when vendor metadata exists. Explicit policy build tags override ambient tags; tag order and duplicates are normalized.

Module and workspace manifests are isolated during analysis so Go can resolve dependencies without rewriting the project's manifests or checksum files. The Go module/build caches may still be populated; offline mode disables dependency network resolution.

Discovery supports `-mod`, `-modfile`, `-tags`, `-race`, `-msan`, `-asan`, `-trimpath`, and `-buildvcs` in GOFLAGS. Output/cache flags are normalized away. Other flags, including overlays and custom tool executors, are rejected rather than omitted from the fingerprint. Custom package drivers are unsupported; automatic external driver discovery is disabled.

`Options.CallGraph` adds advisory SSA/CHA call edges from analyzed package bodies. Edges distinguish static callees from conservative candidates; closures are attributed to their enclosing declaration. External callees are retained without traversing external bodies unless dependencies are included in analysis. Reflection and some generic dispatch can be missing, and conservative candidates need not be reachable at runtime. Graph metadata reports these limits. The graph is sorted and deduplicated and does not change policy matching or lock fingerprints.

Basic discovery also records `recursive=true` on symbols belonging to known
static call cycles. This uses existing typed AST data independently of SSA and
does not populate optional call edges. Direct and mutual cycles preserve exact
function/method identity. See [recursion safety and analysis limits](08_VALIDATION_SAFETY.md#9-recursion).

Symbols with recognizable calls to the OpenTelemetry `trace.Tracer.Start` API
also record `manualSpan=true`. Receiver types must implement the actual imported
Tracer interface; matching the method name alone is insufficient. This metadata
uses typed AST calls independently of SSA and contains no span-name or captured
values. See [overlap warnings and limitations](08_VALIDATION_SAFETY.md#10-double-instrumentation).

`Options.CacheDir` opts into persistent normalized CodeModel caching. Leave it empty to avoid cache storage. Use a private caller-owned directory, never an untrusted shared cache. Newly created directories and files use permissions 0700 and 0600. Entries contain symbol/type/source metadata, but omit raw effective build environment values and restore those from the current request.

Each lookup reruns Go package selection and hashes selected source, embedded files, dependency sources, manifests, workspace/vendor metadata, analysis options, the effective build environment, and the analyzer executable. Cache identity includes absolute input paths; moving a checkout causes a cache miss without changing the relocatable lock fingerprint. Compiler-generated temporary paths can also prevent hits. A hit avoids rebuilding syntax/type/SSA metadata. Invalid, damaged, obsolete, or oversized entries trigger fresh analysis; errors accessing explicitly requested cache storage are reported. Atomic publication permits concurrent callers. Changed inputs during analysis are not cached. Cache entries are disposable and may be removed by the caller; successful writable publication bounds recognized entries to 64 files and 256 MiB by removing the oldest published entries, with filename ordering for equal timestamps. The current publication is protected. Ownership checks require a regular SHA-keyed file with the expected version, matching key/payload digest, and valid model identity. Unrecognized or damaged files are preserved and excluded from these managed-entry budgets. Retention observes caller cancellation; concurrent removal is harmless because metadata is disposable.

`Options.CacheReadOnly` reuses valid existing entries without creating or refreshing cache files. A missing cache directory, absent entry, changed input, or corrupt entry produces fresh semantic analysis while leaving cache storage unchanged. This mode covers the semantic metadata cache, not Go module/build caches or disposable analysis files. Actual storage failures remain errors and preserve their filesystem causes; `errors.Is(err, discovery.ErrCacheStorage)` distinguishes them from project-analysis failures.

The CLI's `--cache-dir` flag opts into this engine for scan, init, and policy
analysis. Check, diff, and dry-run operations select read-only cache access.
Caching remains disabled by default. Relative CLI cache paths resolve against
the project root. Retention never runs in read-only mode. See [CLI contracts](CLI.md).
