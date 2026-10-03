# Lockfile and Reproducibility

File: `otelplan.lock`

Format: machine-generated canonical JSON.

Users should not hand-edit it.

## Purpose

A broad source policy such as:

```yaml
match:
  packages:
    - "github.com/acme/shop/internal/payment"
  exported: true
```

can select new methods after a code change.

The lockfile makes this visible.

## Required contents

```json
{
  "apiVersion": "otelplan.io/lock/v1alpha1",
  "policyDigest": "sha256:...",
  "goVersion": "go1.27.1",
  "moduleGraphDigest": "sha256:...",
  "backend": {
    "name": "otelc",
    "version": "...",
    "digest": "sha256:..."
  },
  "targets": []
}
```

Each target:

```json
{
  "symbol": "github.com/acme/shop/internal/payment.(*Processor).Authorize",
  "signature": "func(context.Context, Payment) error",
  "signatureDigest": "sha256:...",
  "sourceRule": "payment",
  "spanName": "payment.Processor.Authorize",
  "context": {
    "strategy": "argument",
    "index": 0
  },
  "errors": {
    "indexes": [0]
  },
  "attributes": []
}
```

## Canonicalization

Before hashing:

- sort maps by key;
- sort targets by canonical symbol;
- canonicalize order-independent policy rules, exclusions, selector sets, build tags, and attribute keys without mutating caller policy;
- normalize path separators;
- do not include absolute local checkout paths;
- do not include timestamps in digest material.

## `lock --check`

CI command:

```bash
otelplan lock --check
```

Fails when meaningful resolution differs. Source file moves produce `SOURCE`
drift; line/column changes are retained as diagnostic metadata and ignored for
drift. Go toolchain, target platform, tags, effective module mode, alternate
manifests, workspace/vendor inputs, and relevant cgo/compiler configuration are
fingerprinted. Absolute checkout locations are normalized.

The lock records selection and signatures, not arbitrary source-body content. It
is not a byte-for-byte application source snapshot.

## Diff classifications

```text
ADD
REMOVE
SIGNATURE
POLICY
SPAN_NAME
CONTEXT
ERROR_STRATEGY
ATTRIBUTE
BACKEND
SOURCE
BUILD
ARTIFACT
```

## Build policy

Recommended production behavior:

- require an up-to-date lockfile;
- exact backend version;
- no network dependency resolution unless explicitly permitted by CI;
- fail when generated backend artifacts differ from manifest expectations.

## Resolution and build ownership

The CLI lock, check, diff, and validate commands construct resolution state using
the configured backend version/capabilities. They do not verify its executable
or generated artifacts. `Backend.Digest` and `Artifacts` are independently-owned
build identity when populated by an API caller. The full diff API compares them;
resolution-only CLI comparison excludes them.

Refresh preserves build identity if resolution is unchanged. If resolution
changes while build identity is present, refresh refuses to write rather than
erasing or incorrectly retaining it. The owning build phase must regenerate that
identity. Compile currently publishes a separate verified artifact manifest and
does not populate the resolution lock. Build likewise does not read or update it.
Enforce `lock --check` explicitly in production build scripts.

Malformed locks fail check/diff/validate safely. A normal `lock` command can
regenerate a malformed resolution file. Check and diff never write project files.
