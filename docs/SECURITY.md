# Security

## Reporting a vulnerability

Use GitHub's [private vulnerability reporting](https://github.com/DiLRandI/OTelPlan/security/advisories/new)
for suspected security issues. Private reporting is enabled for this repository.
Please keep exploit details and sensitive reproductions out of public issues and
pull requests while maintainers investigate.

Include a minimal, sanitized reproduction with:

- the OTelPlan commit or version, Go version, Linux environment, and backend version;
- the command and relevant policy, using dummy values instead of credentials;
- the expected security property, observed result, and practical impact;
- affected files or diagnostic codes, if known.

Do not attach production source, captured application data, credentials, full
environment dumps, or unsanitized telemetry. Maintainers can request additional
details through the private report. No response-time or remediation-time guarantee
is currently offered.

## Support status

OTelPlan is pre-stable. Security fixes currently target the latest `main` branch;
there is no supported stable release or older release maintenance window yet.
Linux is the supported platform. Use the toolchain and pinned backend documented
in the [support matrix](OTELC_BACKEND.md#support-matrix).

## Toolchain security baseline

Build and validate with Go 1.27.4 or a later patched 1.27.x toolchain. The module
requires at least 1.27.4 so an older local toolchain cannot silently build the CLI.
The [Go vulnerability database advisory](https://pkg.go.dev/vuln/GO-2026-6859)
identifies a `net/http` HTTP/1 server race fixed in 1.27.4. The full test scan on
1.27.3 reported it; both source/test and CLI module scans passed on 1.27.4 with
scanner v1.9.0 in a 2026-11-14 scan. This is scanner evidence, not a claim
that the CLI exposes the advisory's HTTP server attack path.

Rebuild existing executables and the separately installed OTelC backend with the
patched toolchain. A toolchain patch can make resolution locks stale because Go
version participates in build identity. Use the [reviewed upgrade procedure](COMPATIBILITY.md#reviewing-an-upgrade)
rather than suppressing drift or disabling vulnerability checks. Historical
performance and release records retain their original toolchain identity and do
not establish acceptance of a newly built candidate.

## Using OTelPlan safely

OTelPlan analyzes local Go projects and builds instrumentation in disposable
workspace copies. It does not upload source, policies, or code inventories to an
external service, and generated hooks install no SDK or exporter. The application
owns telemetry destinations and transport configuration.

The following boundaries matter when operating the tool:

- Run with permission to read the selected project, policy, workspace members,
  and local replacement modules. These inputs may extend outside `--root`.
  Disposable build copies preserve source immutability; they are not an operating
  system sandbox for untrusted projects or build tools.
- Trust the Go toolchain and the OTelC executable selected through `PATH`, and
  protect their files and parent directories. Version and digest checks detect
  identity changes; they do not establish publisher authenticity or protect a
  compromised host.
- Online Go operations may download dependencies or toolchains. Prepare required
  dependencies before using `--offline`. That flag controls Go dependency lookup,
  not network access by arbitrary application or build-tool code.
- Use a private, caller-owned `--cache-dir`. Newly created cache directories and
  files use restrictive permissions; existing shared directories are not repaired.
- Review every explicit attribute mapping and keep secrets out of policy constants.
  Sensitive-name checks are heuristics, not runtime content redaction. Constants
  can remain in policies, locks, and generated artifacts. An opted-in Stringer
  invokes application code and may return sensitive data, block, or have side
  effects despite its formatting-panic guard.
- Configured error recording includes normal application error text in exception
  events. Disable `errors.record` for targets whose errors may contain private
  data, or sanitize those errors in the application. A fixed span status description
  does not redact the exception message.
- Use `otelplan build --locked` when resolution must be frozen; keep
  `otelplan lock --check` as a separate gate where useful. Default builds and
  compile do not enforce resolution-lock freshness. Artifact hashes establish
  consistency with the supplied manifest, not its origin.

See [capture and privacy](POLICY.md),
[CLI behavior](CLI.md), and [reproducibility](COMPATIBILITY.md)
for the detailed controls and limitations. Please report a suspected bypass even
when its particular scenario is not described here.
