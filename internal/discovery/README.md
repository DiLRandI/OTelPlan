Go package and type discovery uses the effective toolchain, target platform, build tags, module mode, workspace, and cgo configuration. The lock graph fingerprints these inputs and normalized module/workspace contents without recording checkout paths.

Explicit `GOFLAGS=-mod=mod`, `-mod=readonly`, and `-mod=vendor` take precedence over vendor detection. Otherwise discovery defaults to readonly mode, or vendor mode when vendor metadata exists. Explicit policy build tags override ambient tags; tag order and duplicates are normalized.

Module and workspace manifests are isolated during analysis so Go can resolve dependencies without rewriting the project's manifests or checksum files. The Go module/build caches may still be populated; offline mode disables dependency network resolution.

Discovery supports `-mod`, `-modfile`, `-tags`, `-race`, `-msan`, `-asan`, `-trimpath`, and `-buildvcs` in GOFLAGS. Output/cache flags are normalized away. Other flags, including overlays and custom tool executors, are rejected rather than omitted from the fingerprint. Custom package drivers are unsupported; automatic external driver discovery is disabled.
