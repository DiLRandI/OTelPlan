"""Exercise a packaged OTelPlan executable with the real pinned backend."""

import argparse
from contextlib import contextmanager
import hashlib
from http.server import BaseHTTPRequestHandler, HTTPServer
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import threading


FIXTURES = Path(__file__).resolve().parents[1] / "internal/cli/testdata/architectures"
ARCHITECTURES = {
    "package": ("internal/service.(*Order).Submit", "internal/service.(User).Authorize"),
    "feature": ("internal/order.Submit", "internal/payment.Authorize"),
    "hexagonal": ("internal/application.(Checkout).Submit", "internal/adapters.(*Gateway).Authorize"),
}
ZERO_SPAN = "0" * 16


def require(condition, description):
    if not condition:
        raise ValueError(description)


def execute(args, env, cwd, expect=0):
    completed = subprocess.run(args, env=env, cwd=cwd, capture_output=True, text=True, timeout=240)
    require(completed.returncode == expect,
            f"{Path(args[0]).name} {args[1:]} exited {completed.returncode}, expected {expect}:\n"
            f"{completed.stdout}{completed.stderr}")
    return completed.stdout


def snapshot(root):
    result = {}
    for path in sorted(root.rglob("*")):
        if path.is_file():
            stat = path.stat()
            result[str(path.relative_to(root))] = (
                hashlib.sha256(path.read_bytes()).hexdigest(), stat.st_mode, stat.st_ino, stat.st_mtime_ns,
            )
    return result


def fixture(work, architecture):
    root = work / architecture
    shutil.copytree(FIXTURES / "common", root)
    shutil.copytree(FIXTURES / architecture, root, dirs_exist_ok=True)
    return root


class RejectingProxy(BaseHTTPRequestHandler):
    def reject(self):
        self.server.requests += 1
        self.send_response(503)
        self.end_headers()

    do_GET = do_POST = do_CONNECT = reject

    def log_message(self, format, *args):
        pass


@contextmanager
def offline_proxy(env):
    with HTTPServer(("127.0.0.1", 0), RejectingProxy) as server:
        server.requests = 0
        thread = threading.Thread(target=server.serve_forever)
        thread.start()
        try:
            proxy = f"http://127.0.0.1:{server.server_port}"
            settings = {key: proxy for key in ("HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY")}
            settings["NO_PROXY"] = ""
            yield {**env, **settings, **{key.lower(): value for key, value in settings.items()},
                   "GOPROXY": "off", "GOSUMDB": "off"}
            require(server.requests == 0, f"offline commands used a proxy {server.requests} time(s)")
        finally:
            server.shutdown()
            thread.join()


def command(executable, env, root, cache, offline, *args, expect=0):
    flags = [str(executable), "--root", str(root), "--format=json", "--cache-dir", str(cache)]
    if offline:
        flags.append("--offline")
    reply = json.loads(execute([*flags, *args], env, root, expect))
    require(reply["apiVersion"] == "otelplan.io/cli/v1alpha1" and reply["ok"] == (expect == 0),
            f"unexpected JSON envelope for {args}")
    return reply


def check_trace(result):
    require(result["ReturnedError"] == "declined", "instrumentation changed the application return")
    spans = result["Spans"]
    by_name = {span["Name"]: span for span in spans}
    require(len(spans) == 4 and by_name.keys() == {"root", "checkout", "payment", "downstream"},
            "unexpected span inventory")
    span_ids = {span["ID"] for span in spans}
    require(len(span_ids) == 4 and ZERO_SPAN not in span_ids, "invalid or repeated span identities")
    root = by_name["root"]
    require(root["Parent"] == ZERO_SPAN and root["Trace"] != "0" * 32, "invalid root span")
    parent = root
    for name in ("checkout", "payment", "downstream"):
        span = by_name[name]
        require(span["Parent"] == parent["ID"] and span["Trace"] == root["Trace"],
                f"broken OpenTelemetry parentage for {name}")
        if name != "downstream":
            require(span["Scope"] == "otelplan.io/business" and span["Kind"] == "internal"
                    and span["Error"] and span["Events"] == 1, f"incorrect error recording for {name}")
        parent = span
    require(all(not span["Attributes"] for span in spans), "unexpected capture in no-capture fixture")
    require(all(not by_name[name]["Events"] and not by_name[name]["Error"] for name in ("root", "downstream")),
            "instrumentation changed the manual spans")


def exercise(executable, env, work, offline, architecture):
    project = fixture(work, architecture)
    cache = work / f"analysis-{architecture}"
    before = snapshot(project)

    def invoke(*args):
        return command(executable, env, project, cache, offline, *args)

    invoke("scan")
    targets = invoke("inspect")["data"]["targets"]
    expected = {name: f"example.com/architecture/{symbol}"
                for name, symbol in zip(("checkout", "payment"), ARCHITECTURES[architecture], strict=True)}
    require(len(targets) == 2 and {item["sourceRule"]: item["symbol"] for item in targets} == expected,
            f"incorrect resolved targets for {architecture}")

    for symbol in expected.values():
        explanation = invoke("explain", symbol)["data"]
        require(explanation["symbol"] == symbol and explanation["selected"]
                and len(explanation["decisions"]) > 0, f"missing selection explanation for {symbol}")

    invoke("lock")
    locked = snapshot(project)
    lock_info = locked.pop("otelplan.lock")
    require(locked == before, "scan, inspect or lock changed application files")
    invoke("lock")
    reviewed = snapshot(project)
    repeated_lock = reviewed.pop("otelplan.lock")
    require(repeated_lock[0] == lock_info[0] and reviewed == before,
            "repeated lock generation changed lock bytes or application files")
    reviewed["otelplan.lock"] = repeated_lock
    invoke("validate", "--strict")
    bundles = [work / f"bundle-{architecture}-{attempt}" for attempt in ("first", "repeat")]
    for bundle in bundles:
        invoke("compile", "--output", str(bundle))
    first, repeat = ({name: stamp[0] for name, stamp in snapshot(bundle).items()} for bundle in bundles)
    require(first == repeat, "repeated compile changed generated bundle bytes")
    binary = work / f"app-{architecture}"
    invoke("build", "--locked", "--", "-o", str(binary), ".")
    trace_output = execute([str(binary)], env, project)
    require("private-token-never-capture" not in trace_output, "fixture argument appeared in telemetry")
    check_trace(json.loads(trace_output))
    invoke("lock", "--check")
    invoke("diff", "--check")
    require(snapshot(project) == reviewed, "application source, module, policy, or lock changed")
    mode = "prepared-cache-offline" if offline else "cold-cache-online"
    print(f"PASS {mode}/{architecture}", flush=True)


def exercise_modes(executable, work, env):
    for mode in ("online", "offline"):
        isolated = work / mode
        isolated.mkdir()
        current = {**env, "GOMODCACHE": str(isolated / "modules")}
        try:
            if mode == "online":
                for architecture in ARCHITECTURES:
                    exercise(executable, current, isolated, False, architecture)
                continue
            seed = fixture(isolated, "package")
            cold = command(executable, current, seed, isolated / "cold-cache", True, "inspect", expect=4)
            require(any("module lookup disabled by GOPROXY=off" in item["message"]
                        for item in cold["diagnostics"]), "cold offline discovery failed for the wrong reason")
            execute(["go", "mod", "download", "all"], current, seed)
            shutil.rmtree(seed)
            with offline_proxy(current) as offline:
                for architecture in ARCHITECTURES:
                    exercise(executable, offline, isolated, True, architecture)
        finally:
            execute(["go", "clean", "-modcache"], current, isolated)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("executable", type=Path)
    parser.add_argument("backend", type=Path)
    parser.add_argument("version")
    args = parser.parse_args()
    try:
        executable, backend = (path.resolve(strict=True) for path in (args.executable, args.backend))
        with tempfile.TemporaryDirectory(prefix="otelplan-packaged-") as temporary:
            work = Path(temporary)
            tools = work / "bin"
            tools.mkdir()
            (tools / "otelc").symlink_to(backend)
            env = {**os.environ, "PATH": f"{tools}:{os.environ['PATH']}",
                   "GOENV": "off", "GOWORK": "off", "GOFLAGS": "-race -buildvcs=false",
                   "GOTOOLCHAIN": "local", "GOPACKAGESDRIVER": "off", "GOPROXY": "https://proxy.golang.org",
                   "GOSUMDB": "sum.golang.org", "GOPRIVATE": "", "GONOPROXY": "none",
                   "GONOSUMDB": "none", "GOVCS": "*:off"}
            version = json.loads(execute([str(executable), "version", "--format=json"], env, work))
            require(version["data"]["otelplan"] == args.version, "packaged CLI version disagrees with candidate")
            require(version["data"]["backendAvailableVersion"] == "v1.1.0", "expected pinned OTelC v1.1.0")
            exercise_modes(executable, work, env)
    except (KeyError, OSError, ValueError, subprocess.TimeoutExpired) as error:
        parser.exit(1, f"packaged CLI exercise: {error}\n")


if __name__ == "__main__":
    main()
