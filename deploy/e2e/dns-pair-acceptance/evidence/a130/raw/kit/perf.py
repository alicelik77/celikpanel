#!/usr/bin/env python3
"""perf.py L CELL NODE LABEL OUTDIR: case 3 timings + answers of the Components scan, /managed-services,
/hosting/capabilities and /setup/components (5 reads each, panel API as the page calls it), and the
controls-during-scan check (a scan and a component reload pressed together, 6 times). Lab only."""
import json, sys, os, time, threading, hashlib
from pathlib import Path
L = sys.argv[1]; sys.path.insert(0, L); sys.path.insert(0, str(Path(L).parent))
import beta1
cell, node, label, out = sys.argv[2:6]
st = beta1.load_state(cell, node)
rec = beta1.Recorder(cell, beta1.redactor_for(st))
def client():
    c = beta1.client_for(cell, node, rec, dict(st)); c.recorder = lambda e: None; return c
c = client()
VOLATILE = {"scanned_at", "scan_age_seconds", "checked_at", "generated_at", "now", "uptime", "uptime_seconds", "memory", "load",
            "active_since", "since", "pid", "main_pid", "memory_bytes", "cpu", "started_at", "read_at", "as_of", "age_seconds"}
def norm(x):
    if isinstance(x, dict): return {k: norm(v) for k, v in sorted(x.items()) if k not in VOLATILE}
    if isinstance(x, list): return [norm(v) for v in x]
    return x
res = {"label": label, "reads": {}}
os.makedirs(out, exist_ok=True)
for name, method, path in [("scan", "POST", "/api/v1/managed-services/scan"), ("managed-services", "GET", "/api/v1/managed-services"),
                           ("capabilities", "GET", "/api/v1/hosting/capabilities"), ("setup-components", "GET", "/api/v1/setup/components")]:
    times = []; digests = set(); last = None
    for i in range(5):
        r = c.request(method, path, {} if method == "POST" else None, timeout=180, purpose="perf")
        times.append(round(r.elapsed, 3))
        try: doc = norm(r.json())
        except Exception: doc = r.text[:200]
        s = json.dumps(doc, sort_keys=True); digests.add(hashlib.sha256(s.encode()).hexdigest()[:12]); last = doc
        if r.status != 200: times[-1] = f"{r.status}:{times[-1]}"
        time.sleep(0.5)
    Path(out, f"{label}-{name}.json").write_text(json.dumps(last, indent=1, sort_keys=True))
    res["reads"][name] = {"elapsed_s": times, "distinct_answers": sorted(digests)}
    print(f"{label} {name:17s} elapsed {times} answers {sorted(digests)}", flush=True)
# controls during a scan: a component reload pressed while a scan reads
ctl = []
for i in range(6):
    box = {}
    def scan():
        cc = client(); t = time.monotonic(); r = cc.request("POST", "/api/v1/managed-services/scan", {}, timeout=180, purpose="perf-scan")
        box["scan"] = (r.status, round(time.monotonic() - t, 3))
    th = threading.Thread(target=scan); th.start(); time.sleep(0.05 + 0.03 * i)
    cc = client(); t = time.monotonic()
    r = cc.request("POST", "/api/v1/service/action", {"service_name": "nginx", "action": "reload"}, timeout=180, purpose="perf-action")
    code = ""
    try: code = (r.json() or {}).get("code") or (r.json() or {}).get("reason") or ""
    except Exception: code = r.text[:120]
    a = (r.status, round(time.monotonic() - t, 3), code)
    th.join(); ctl.append({"scan": box.get("scan"), "reload": a})
    print(f"{label} controls-during-scan #{i+1}: scan {box.get('scan')} reload nginx {a}", flush=True)
    time.sleep(1.5)
res["controls_during_scan"] = ctl
Path(out, f"{label}-summary.json").write_text(json.dumps(res, indent=1))
