#!/usr/bin/env python3
"""Linux local-executor acceptance. Creates an isolated data/store directory; no Kubernetes."""
import argparse
import copy
import json
import pathlib
import socket
import subprocess
import tempfile
import time
import urllib.error
import urllib.request

parser = argparse.ArgumentParser()
parser.add_argument("--malta", required=True)
parser.add_argument("--fio", required=True)
parser.add_argument("--root", required=True, help="Parent directory for isolated acceptance data")
args = parser.parse_args()
repo = pathlib.Path(__file__).resolve().parents[1]
root = pathlib.Path(tempfile.mkdtemp(prefix="cases-", dir=args.root))
target = root / "data"
target.mkdir()
with socket.socket() as sock:
    sock.bind(("127.0.0.1", 0))
    port = sock.getsockname()[1]
base = f"http://127.0.0.1:{port}"
process = None
log = (root / "server.log").open("w")

def cli(*parts):
    return json.loads(subprocess.check_output([args.malta, *parts], cwd=repo, text=True))["data"]

def request(path, body=None, status=200):
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(base + "/api/v1" + path, data=data, headers={"Content-Type": "application/json"})
    try:
        with urllib.request.urlopen(req, timeout=15) as response:
            code, result = response.status, json.load(response)
    except urllib.error.HTTPError as exc:
        code, result = exc.code, json.load(exc)
    assert code == status, (path, code, result)
    return result.get("data")

def start():
    global process
    process = subprocess.Popen([args.malta, "serve", "--listen", f"127.0.0.1:{port}",
        "--data-dir", str(root / "store"), "--allow-target", str(target), "--fio", args.fio], stdout=log, stderr=log)
    for _ in range(100):
        if process.poll() is not None:
            raise RuntimeError("server exited; inspect " + str(root / "server.log"))
        try:
            request("/health")
            return
        except (urllib.error.URLError, ConnectionError):
            time.sleep(.1)
    raise RuntimeError("server startup timeout")

def stop():
    if process and process.poll() is None:
        process.terminate()
        process.wait(timeout=15)

def plan(kind):
    out = root / (kind + ".json")
    flag, source = ("--spec", "benchmarks/data/smoke-read.yaml") if kind == "fio" else ("--scenario", "benchmarks/scenarios/weighted-metadata.yaml")
    cli("plan", flag, source, "--environment", "environments/local.yaml", "--version", "environments/version-example.yaml", "--target", str(target), "--resource-group", "case-smoke", "--out", str(out))
    p = json.loads(out.read_text())
    if kind == "fio":
        p["spec"]["repeats"] = 1
        p["spec"]["matrix"] = {"numjobs": [1], "iodepth": [1]}
        p["spec"]["workload"].update(runtime_seconds=1, warmup_seconds=0, file_size_bytes=1048576, direct=False, ioengine="psync")
    else:
        p["spec"]["scenario"]["repeats"] = 1
        for stage in p["spec"]["scenario"]["stages"]:
            stage.update(runtime_seconds=1, warmup_seconds=0)
    return p

summary = {"executor": "local", "checks": [], "cases": [], "runs": []}
try:
    start()
    for kind in ("fio", "scenario"):
        q = {"name": kind + " case smoke", "description": "Isolated development acceptance", "change_note": "initial", "base_revision": 0, "plan": plan(kind)}
        first = request("/cases", q, 201)
        cid = first["case_id"]
        q["base_revision"] = 1
        q["description"] = "new revision"
        if kind == "fio":
            q["plan"]["spec"]["workload"]["runtime_seconds"] = 2
        else:
            q["plan"]["spec"]["scenario"]["stages"][0]["runtime_seconds"] = 2
        second = request(f"/cases/{cid}/versions", q, 201)
        assert first["plan"]["hash"] != second["plan"]["hash"]
        assert request(f"/cases/{cid}/versions/1") == first
        request(f"/cases/{cid}/versions", q, 409)
        invalid = copy.deepcopy(q)
        invalid.update(base_revision=2, name="")
        request(f"/cases/{cid}/versions", invalid, 400)
        assert len(request(f"/cases/{cid}/versions")) == 2
        request(f"/cases/{cid}/versions/999", status=404)
        assert cli("--server", base, "case", "get", cid, "1") == first
        request_id = kind + "-case-run"
        run = cli("--server", base, "run", "start", "--case", cid, "--case-revision", "1", "--request-id", request_id, "--source", "ci")
        assert run["case_ref"] == {"id": cid, "revision": 1}
        assert run["plan"]["hash"] == first["plan"]["hash"]
        retried = cli("--server", base, "run", "start", "--case", cid, "--case-revision", "1", "--request-id", request_id, "--source", "ci")
        assert retried["id"] == run["id"]
        request("/runs", {"case_ref": {"id": cid, "revision": 2}, "request_id": request_id, "source": "ci", "executor": "local"}, 409)
        for _ in range(200):
            run = request("/runs/" + run["id"])
            if run["status"] in ("succeeded", "failed", "cancelled", "interrupted") and run["attempt"]["result_state"] != "collecting":
                break
            time.sleep(.2)
        assert run["status"] == "succeeded", (run["status"], run.get("error"))
        assert run["attempt"]["result_state"] == "complete"
        bundle = request("/runs/" + run["id"] + "/bundle")
        assert bundle["run"]["case_ref"] == run["case_ref"]
        summary["cases"].append({"id": cid, "versions": 2})
        summary["runs"].append({"id": run["id"], "kind": kind, "status": run["status"], "tool": run["tool_version"]})
    stop()
    start()
    for item in summary["cases"]:
        assert len(request(f"/cases/{item['id']}/versions")) == 2
    for item in summary["runs"]:
        assert request("/runs/" + item["id"])["case_ref"]["revision"] == 1
    summary["checks"] = ["create", "edit-rebuild-hash", "immutable-history", "stale-write-409", "invalid-write-400", "missing-version-404", "cli-read", "cli-pinned-execution", "request-id-idempotency", "version-conflict", "fio-real-io", "native-real-io", "bundle-provenance", "restart-persistence"]
    (root / "summary.json").write_text(json.dumps(summary, indent=2) + "\n")
    print(json.dumps({"result": "passed", "directory": str(root), **summary}, indent=2))
finally:
    stop()
    log.close()
