#!/usr/bin/env python3
"""perf7.py L CELL NODE DOMAIN_ID LABEL: case 4 timings of the domain page reads (as the page calls them):
2 warm-up reads, then 5 reads each; then once more after 35 s (past the 30 s check cache). Lab only."""
import json, sys, time
from pathlib import Path
L = sys.argv[1]; sys.path.insert(0, L); sys.path.insert(0, str(Path(L).parent))
import beta1
cell, node, did, label = sys.argv[2:6]
st = beta1.load_state(cell, node)
rec = beta1.Recorder(cell, beta1.redactor_for(st))
c = beta1.client_for(cell, node, rec, dict(st)); c.recorder = lambda e: None
paths = [("capabilities (Databases tab)", "/api/v1/hosting/capabilities"), ("connection (Overview)", f"/api/v1/domains/{did}/connection"),
         ("mail/health", f"/api/v1/domains/{did}/mail/health"), ("apps", f"/api/v1/domains/{did}/apps"), ("usage", f"/api/v1/domains/{did}/usage")]
def one(p):
    t = time.monotonic(); r = c.request("GET", p, None, timeout=120, purpose="perf"); e = round(time.monotonic() - t, 3)
    keys = ""
    try:
        j = r.json()
        if isinstance(j, dict): keys = ",".join(k for k in j.keys() if k in ("state", "checked_at", "stale", "checking", "measured_at", "age_seconds", "status", "dns_server_state", "as_of"))
    except Exception: pass
    return e if r.status == 200 else f"{r.status}:{e}", keys
res = {}
for name, p in paths:
    for _ in range(2): one(p)
    v = [one(p) for _ in range(5)]
    times = [x[0] for x in v]; res[name] = times
    print(f"{label} {name:30s} warm x5 s: {times}  keys:{v[-1][1]}", flush=True)
time.sleep(35)
for name, p in paths:
    v = [one(p) for _ in range(3)]
    print(f"{label} {name:30s} after 35 s idle x3 s: {[x[0] for x in v]}", flush=True)
