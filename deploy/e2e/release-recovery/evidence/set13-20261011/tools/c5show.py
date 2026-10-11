"""set13: C5's raw facts of one run (the unit's lines and state per reading, the shim log per take, the take answers).
usage: c5show.py RUN_DIR"""
import glob
import json
import os
import sys

run = sys.argv[1]
step = sorted(glob.glob(os.path.join(run, "steps", "*c5-app-unit")))[-1]
for f in sorted(glob.glob(os.path.join(step, "native", "*.json"))):
    d = json.load(open(f))
    name = os.path.basename(f)
    if d.get("mode") == "read-unit13":
        st = d.get("state") or {}
        v = d.get("verify_by_harness") or {}
        print(name, {k: st.get(k) for k in ("ActiveState", "MainPID", "NRestarts", "ExecMainStatus")}, "sha", str((d.get("unit") or {}).get("sha256"))[:12],
              "http", d.get("http"), "verify_rc", v.get("returncode"), str(v.get("stderr"))[-160:].replace("\n", " | "))
        if "c5-0-unit" in name:
            print("   lines", json.dumps(d.get("lines")))
            print("   journal", [l[-120:] for l in (d.get("journal") or [])][-6:])
    elif d.get("mode") == "read-lab13":
        print(name, "agent_pid", d.get("agent_pid"), "path", d.get("agent_path"))
        for l in d.get("lines") or []:
            print("    ", l[:200])
    elif d.get("mode") in ("owner-unit13", "lab-shims13"):
        print(name, d.get("what"), {k: (d.get(k) or {}).get("returncode") for k in ("daemon_reload", "verify_by_harness", "mount", "umount") if d.get(k)},
              str((d.get("verify_by_harness") or {}).get("stderr"))[-200:].replace("\n", " | "))
sec = json.load(open(os.path.join(step, "section.json")))
for c in sec.get("calls") or []:
    j = c.get("json") if isinstance(c.get("json"), dict) else {}
    if "take" in str(c.get("label")) or "hosting" in str(c.get("request", {}).get("path")):
        print("CALL", c.get("at"), c.get("status"), c.get("label")[:70])
        print("     ", json.dumps({k: j.get(k) for k in ("code", "reason", "detail", "restore", "service", "service_reason", "service_active", "vars", "details", "error", "outcome")}, ensure_ascii=False)[:900])
