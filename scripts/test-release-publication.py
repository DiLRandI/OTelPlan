"""Exercise publication gates without remote writes or production credentials."""

import hashlib
import json
import os
from pathlib import Path
import subprocess
import tempfile

SOURCE = "1" * 40
REPOSITORY = "DiLRandI/OTelPlan"
VERSION = "v1.0.0-rc.1"
SCRIPT = Path(__file__).with_name("release-publication.sh")

GH = """#!/usr/bin/env python3
import json, os, pathlib, shutil, sys
fixtures = pathlib.Path(os.environ['RELEASE_TEST_FIXTURES'])
log = fixtures / 'calls'
args = sys.argv[1:]
with log.open('a') as stream:
    print(json.dumps(args), file=stream)
if args[0] == 'api':
    print((fixtures / args[1].split('/')[-1].split('?')[0]).read_text())
elif args[:2] == ['run', 'download']:
    shutil.copytree(fixtures / 'assets', args[args.index('--dir') + 1], dirs_exist_ok=True)
elif args[:2] == ['attestation', 'verify']:
    sys.exit(int(os.environ.get('RELEASE_TEST_ATTESTATION_EXIT', '0')))
elif args[:2] == ['release', 'view']:
    print('21')
elif args[:2] not in (['release', 'create'], ['release', 'edit']):
    sys.exit(90)
"""


def prepare(root):
    tools = root / "bin"
    tools.mkdir()
    gh = tools / "gh"
    gh.write_text(GH)
    gh.chmod(0o700)
    git = tools / "git"
    git.write_text(f"#!/bin/sh\nprintf '%s\\t%s\\n' '{SOURCE}' 'refs/tags/{VERSION}'\n")
    git.chmod(0o700)
    assets = root / "assets"
    assets.mkdir()
    stem = f"otelplan_{VERSION}_linux_amd64"
    filenames = (f"{stem}.tar.gz", f"{stem}.spdx.json")
    for name in (*filenames, "provenance.jsonl"):
        (assets / name).write_text(name)
    (assets / "SHA256SUMS").write_text("".join(
        f"{hashlib.sha256((assets / name).read_bytes()).hexdigest()}  {name}\n" for name in filenames
    ))
    responses = {
        "42": {
            "conclusion": "success", "status": "completed", "event": "workflow_dispatch",
            "path": ".github/workflows/release-validation.yml", "head_branch": "main",
            "head_sha": SOURCE, "repository": {"full_name": REPOSITORY},
            "head_repository": {"full_name": REPOSITORY}, "run_attempt": 1,
        },
        "jobs": {"jobs": [
            {"name": name, "conclusion": "success"}
            for name in ("release-validation", "prepare-linux-candidate")
        ]},
        "main": {"protected": True, "commit": {"sha": SOURCE}},
        "release": {
            "deployment_branch_policy": {"custom_branch_policies": True},
            "protection_rules": [{
                "type": "required_reviewers", "prevent_self_review": True,
                "reviewers": [{"type": "User"}],
            }],
        },
        "deployment-branch-policies": {
            "total_count": 1, "branch_policies": [{"name": "main", "type": "branch"}],
        },
        "21": {
            "draft": True, "tag_name": VERSION,
            "assets": [
                {"name": path.name, "digest": "sha256:" + hashlib.sha256(path.read_bytes()).hexdigest()}
                for path in assets.iterdir()
            ],
        },
    }
    for name, data in responses.items():
        (root / name).write_text(json.dumps(data))
    return {**os.environ, "PATH": f"{tools}{os.pathsep}{os.environ['PATH']}", "RELEASE_TEST_FIXTURES": str(root)}


def exercise(mode, failure=None):
    with tempfile.TemporaryDirectory(prefix="release-publication-test-") as tmp:
        root = Path(tmp)
        env = prepare(root)
        if failure == "wrong source":
            data = json.loads((root / "42").read_text())
            data["head_sha"] = "2" * 40
            (root / "42").write_text(json.dumps(data))
        elif failure == "unprotected environment":
            (root / "release").write_text("{}")
        elif failure == "bad attestation":
            env["RELEASE_TEST_ATTESTATION_EXIT"] = "1"
        elif failure == "changed draft asset":
            data = json.loads((root / "21").read_text())
            data["assets"][0]["digest"] = "sha256:" + "0" * 64
            (root / "21").write_text(json.dumps(data))

        result = subprocess.run(
            ["sh", str(SCRIPT), mode, "42", VERSION, SOURCE, str(root / "received")],
            env=env, capture_output=True, text=True,
        )
        assert (result.returncode == 0) == (failure is None), (mode, failure, result)
        log = [json.loads(line) for line in (root / "calls").read_text().splitlines()]
        writes = [args[1] for args in log if args[0] == "release" and args[1] in ("create", "edit")]
        if mode == "verify" or failure not in (None, "changed draft asset"):
            assert writes == [], (mode, failure, writes)
        elif failure is None:
            assert writes == ["create", "edit"], writes
        else:
            assert writes == ["create"], writes


exercise("verify")
exercise("verify", "wrong source")
exercise("publish", "unprotected environment")
exercise("publish", "bad attestation")
exercise("publish", "changed draft asset")
exercise("publish")
print("release publication self-test: PASS")
