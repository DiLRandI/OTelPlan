OTelPlan binary redistribution notices

This directory accompanies the Linux candidate executable. LICENSE, PATENTS,
and NOTICE files are unmodified copies from the source versions listed below.
The project Apache-2.0 license is at the archive root. These third-party works
retain their own licenses. The OTelC executable is not bundled.

Go toolchain and standard library: Go 1.27.3
https://go.googlesource.com/go/+/refs/tags/go1.27.3/
The packaging script compares these Go license/patent texts with the selected
Go 1.27.x installation. The actual toolchain version is in release.json.

Go modules linked into the executable:
  golang.org/x/mod v0.41.0
  https://go.googlesource.com/mod/+/refs/tags/v0.41.0/
  golang.org/x/sync v0.23.0
  https://go.googlesource.com/sync/+/refs/tags/v0.23.0/
  golang.org/x/tools v0.50.0
  https://go.googlesource.com/tools/+/refs/tags/v0.50.0/
  gopkg.in/yaml.v3 v3.0.1
  https://github.com/go-yaml/yaml/tree/v3.0.1

modules.txt records the sorted module/version pairs verified against the built
executable. SHA256SUMS protects this directory against incomplete notice edits.
Dependency updates require reviewing the linked source's licenses/notices,
updating these files and modules.txt, then regenerating SHA256SUMS. Packaging
fails for changed module identities, replacements, missing/tampered notice files,
or different Go license/patent texts. Test-only dependencies are not bundled.

This inventory is not a standardized SBOM or a signed provenance statement.
