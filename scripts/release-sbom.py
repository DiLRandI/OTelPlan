"""Describe the archive, executable, and linked Go components in SPDX 2.3."""

import argparse
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
from urllib.parse import quote


DOCUMENT = "SPDXRef-DOCUMENT"
ARCHIVE = "SPDXRef-Archive"
BINARY = "SPDXRef-Executable"
STDLIB = "SPDXRef-GoStdlib"


def sha256(path):
    with path.open("rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


def linked_modules(payload, go_version):
    lines = (payload / "build-info.txt").read_text(encoding="utf-8").splitlines()
    if not lines or lines[0].rpartition(": ")[2] != go_version:
        raise ValueError("binary build information disagrees with the release toolchain")
    modules = {}
    for line in lines[1:]:
        fields = line.split()
        if fields and fields[0] == "=>":
            raise ValueError("linked replacements are unsupported")
        if fields and fields[0] == "dep":
            if len(fields) != 4 or fields[1] in modules:
                raise ValueError("invalid linked module build information")
            modules[fields[1]] = fields[2]
    expected = [f"{name} {version}" for name, version in sorted(modules.items())]
    notices = payload / "third_party"
    actual = (notices / "modules.txt").read_text(encoding="utf-8").splitlines()
    if expected != actual:
        raise ValueError("linked modules disagree with the redistribution inventory")
    return modules


def declared_licenses(payload, modules):
    notices = payload / "third_party"
    entries = {}
    for line in (notices / "spdx-licenses.txt").read_text(encoding="utf-8").splitlines():
        name, _, expression = line.partition(" ")
        if not expression.strip() or name in entries:
            raise ValueError("invalid SPDX license inventory")
        entries[name] = expression
    if entries.keys() != set(modules) | {"go"}:
        raise ValueError("SPDX licenses must cover Go and exactly the linked modules")
    for name in entries:
        if not (notices / name / "LICENSE").is_file():
            raise ValueError(f"missing license notice for {name}")
    return entries


def component(identifier, name, version, license_expression, purpose):
    return {
        "SPDXID": identifier,
        "name": name,
        "versionInfo": version,
        "downloadLocation": "NOASSERTION",
        "filesAnalyzed": False,
        "licenseConcluded": "NOASSERTION",
        "licenseDeclared": license_expression,
        "copyrightText": "NOASSERTION",
        "primaryPackagePurpose": purpose,
    }


def relationship(source, kind, target):
    return {
        "spdxElementId": source,
        "relationshipType": kind,
        "relatedSpdxElement": target,
    }


def describe(payload, archive):
    release = json.loads((payload / "release.json").read_text(encoding="utf-8"))
    version = release["version"]
    go_version = release["goVersion"]
    revision = release["sourceRevision"]
    modules = linked_modules(payload, go_version)
    licenses = declared_licenses(payload, modules)
    archive_hash = sha256(archive)

    distribution = component(ARCHIVE, "OTelPlan Linux amd64 archive", version, "NOASSERTION", "APPLICATION")
    distribution["packageFileName"] = archive.name
    distribution["checksums"] = [{"algorithm": "SHA256", "checksumValue": archive_hash}]

    binary = component(BINARY, "otelplan", version, "Apache-2.0", "APPLICATION")
    binary["packageFileName"] = "otelplan"
    binary["checksums"] = [{"algorithm": "SHA256", "checksumValue": sha256(payload / "otelplan")}]
    binary["sourceInfo"] = f"Built from OTelPlan commit {revision} with {go_version}."

    stdlib = component(STDLIB, "Go standard library", go_version, licenses["go"], "LIBRARY")
    packages = [distribution, binary, stdlib]
    relationships = [
        relationship(DOCUMENT, "DESCRIBES", ARCHIVE),
        relationship(ARCHIVE, "CONTAINS", BINARY),
        relationship(BINARY, "STATIC_LINK", STDLIB),
    ]
    for index, (name, module_version) in enumerate(sorted(modules.items()), start=1):
        identifier = f"SPDXRef-GoModule-{index}"
        dependency = component(identifier, name, module_version, licenses[name], "LIBRARY")
        dependency["externalRefs"] = [{
            "referenceCategory": "PACKAGE-MANAGER",
            "referenceType": "purl",
            "referenceLocator": f"pkg:golang/{quote(name, safe='/')}@{quote(module_version, safe='')}",
        }]
        packages.append(dependency)
        relationships.append(relationship(BINARY, "STATIC_LINK", identifier))

    timestamp = datetime.fromtimestamp(release["sourceDateEpoch"], tz=timezone.utc)
    return {
        "spdxVersion": "SPDX-2.3",
        "dataLicense": "CC0-1.0",
        "SPDXID": DOCUMENT,
        "name": archive.name,
        "documentNamespace": f"https://github.com/DiLRandI/OTelPlan/spdx/{revision}/{archive_hash}",
        "creationInfo": {
            "created": timestamp.strftime("%Y-%m-%dT%H:%M:%SZ"),
            "creators": [f"Tool: OTelPlan-release-sbom-{revision}"],
        },
        "packages": packages,
        "relationships": relationships,
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("payload", type=Path)
    parser.add_argument("archive", type=Path)
    args = parser.parse_args()
    try:
        document = describe(args.payload, args.archive)
    except (KeyError, OSError, ValueError) as error:
        parser.exit(1, f"release SBOM: {error}\n")
    print(json.dumps(document, indent=2, sort_keys=True))


if __name__ == "__main__":
    main()
