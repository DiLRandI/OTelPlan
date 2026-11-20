"""Generate the SLSA build predicate for an OTelPlan Linux candidate."""

import argparse
import json
from pathlib import Path
import re
import tarfile
from urllib.parse import quote


REPOSITORY = "https://github.com/DiLRandI/OTelPlan"
WORKFLOW = ".github/workflows/release-validation.yml"


def read_metadata(archive, name):
    member_name = f"{Path(archive.name).name.removesuffix('.tar.gz')}/{name}"
    members = [member for member in archive.getmembers() if member.name == member_name]
    if len(members) != 1 or not members[0].isfile() or members[0].size > 65536:
        raise ValueError(f"invalid archive member {name}")
    with archive.extractfile(members[0]) as stream:
        return stream.read().decode("utf-8")


def predicate(archive_path, revision, version, invocation):
    if not re.fullmatch("[0-9a-f]{40}", revision):
        raise ValueError("expected a full source commit SHA")
    with tarfile.open(archive_path, "r:gz") as archive:
        metadata = json.loads(read_metadata(archive, "release.json"))
        build_info = read_metadata(archive, "build-info.txt").splitlines()

    go_version = metadata["goVersion"]
    if (metadata["sourceRevision"] != revision or metadata["version"] != version
            or metadata["target"] != "linux/amd64"):
        raise ValueError("archive identity differs from the requested candidate")
    if not re.fullmatch(r"go1\.27\.[0-9]+", go_version):
        raise ValueError("unexpected release Go version")
    if not build_info or build_info[0].rpartition(": ")[2] != go_version:
        raise ValueError("Go build information differs from release metadata")

    dependencies = [
        {"uri": f"git+{REPOSITORY}@refs/heads/main", "digest": {"gitCommit": revision}},
        {"uri": f"https://go.dev/dl/{go_version}.linux-amd64.tar.gz", "name": "Go toolchain"},
    ]
    for line in build_info[1:]:
        fields = line.split()
        if not fields:
            continue
        if fields[0] == "=>":
            raise ValueError("release dependencies must not use replacements")
        if fields[0] == "dep":
            _, name, module_version, checksum = fields
            if not checksum.startswith("h1:"):
                raise ValueError("linked module checksum is missing")
            dependencies.append({
                "uri": f"pkg:golang/{quote(name, safe='/')}@{quote(module_version, safe='')}",
                "digest": {"goSum": checksum},
            })

    return {
        "buildDefinition": {
            "buildType": f"{REPOSITORY}/blob/{revision}/docs/RELEASING.md#linux-release-build-type",
            "externalParameters": {
                "workflow": {"repository": REPOSITORY, "ref": "refs/heads/main", "path": WORKFLOW},
                "prepare_version": version,
            },
            "internalParameters": {
                "entryPoint": "scripts/release-linux.sh",
                "target": "linux/amd64",
                "goVersion": go_version,
            },
            "resolvedDependencies": dependencies,
        },
        "runDetails": {
            "builder": {"id": "https://github.com/actions/runner"},
            "metadata": {"invocationId": invocation},
        },
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("archive", type=Path)
    parser.add_argument("revision")
    parser.add_argument("version")
    parser.add_argument("invocation")
    args = parser.parse_args()
    try:
        result = predicate(args.archive, args.revision, args.version, args.invocation)
    except (KeyError, OSError, ValueError, tarfile.TarError) as error:
        parser.exit(1, f"release provenance: {error}\n")
    print(json.dumps(result, indent=2, sort_keys=True))


if __name__ == "__main__":
    main()
