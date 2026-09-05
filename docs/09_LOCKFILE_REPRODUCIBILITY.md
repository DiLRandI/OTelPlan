# Lockfile and Reproducibility

File: `otelplan.lock`

Format: machine-generated YAML or JSON. JSON is recommended for canonical serialization.

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
- sort target list by canonical symbol;
- normalize path separators;
- do not include absolute local checkout paths;
- do not include timestamps in digest material.

## `lock --check`

CI command:

```bash
otelplan lock --check
```

Fails when generated lock differs.

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
```

## Build policy

Recommended production behavior:

- require an up-to-date lockfile;
- exact backend version;
- no network dependency resolution unless explicitly permitted by CI;
- fail when generated backend artifacts differ from manifest expectations.
