Go package and type discovery uses the effective toolchain, target platform, build tags, module mode, workspace, and cgo configuration. The lock graph fingerprints these inputs and normalized module/workspace contents without recording checkout paths.

Explicit `GOFLAGS=-mod=mod`, `-mod=readonly`, and `-mod=vendor` take precedence over vendor detection. Otherwise discovery defaults to readonly mode, or vendor mode when vendor metadata exists. Explicit policy build tags override ambient tags; tag order and duplicates are normalized.

Module and workspace manifests are isolated during analysis so Go can resolve dependencies without rewriting the project's manifests or checksum files. The Go module/build caches may still be populated; offline mode disables dependency network resolution.

Discovery supports `-mod`, `-modfile`, `-tags`, `-race`, `-msan`, `-asan`, `-trimpath`, and `-buildvcs` in GOFLAGS. Output/cache flags are normalized away. Other flags, including overlays and custom tool executors, are rejected rather than omitted from the fingerprint. Custom package drivers are unsupported; automatic external driver discovery is disabled.

`Options.CallGraph` adds advisory SSA/CHA call edges from analyzed package bodies. Edges distinguish static callees from conservative candidates; closures are attributed to their enclosing declaration. External callees are retained without traversing external bodies unless dependencies are included in analysis. Reflection and some generic dispatch can be missing, and conservative candidates need not be reachable at runtime. Graph metadata reports these limits. The graph is sorted and deduplicated and does not change policy matching or lock fingerprints.
