# Architecture trace fixtures

These fixtures exercise the same policy intent across package-oriented, feature-oriented, and hexagonal layouts. The pinned OTelC test checks exact targets, trace parentage, error recording, explicit capture defaults, lock stability, and unchanged application files.

`TestArchitectureTracesWithPinnedBackend` has two independent scenarios:

- `cold-cache-online` starts with an empty, test-owned Go module cache and permits downloads from the public Go module proxy.
- `prepared-cache-offline` starts with a different empty cache and first verifies that offline discovery fails because dependencies are unavailable. It then runs `go mod download all` against a disposable fixture copy. The actual CLI workflow retains `--offline`, disables module and checksum lookups, disables direct VCS downloads, and rejects/counts HTTP(S) proxy requests. No request may reach that proxy.

The offline scenario can run independently; it does not depend on the online scenario or the developer's module cache. Preparation requires network access. The subsequent offline workflow does not. Module caches and retained backend work directories belong to the test and are cleaned up.

Build the pinned backend as described in `.github/workflows/check.yml`, then run either scenario:

```sh
OTELPLAN_OTELC=/absolute/path/to/otelc go test ./internal/cli \
  -run '^TestArchitectureTracesWithPinnedBackend/cold-cache-online$' -count=1
OTELPLAN_OTELC=/absolute/path/to/otelc go test ./internal/cli \
  -run '^TestArchitectureTracesWithPinnedBackend/prepared-cache-offline$' -count=1
```

Both scenarios run when the parent test is selected. They retain the fixture's pinned module versions and checksum verification during online preparation. The network controls cover dependency downloads and proxy-aware HTTP clients; they are not an operating-system network sandbox.
