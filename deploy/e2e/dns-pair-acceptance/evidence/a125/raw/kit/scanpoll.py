#!/usr/bin/env python3
"""scanpoll.py L CELL NODE COMPONENT_ID T0_EPOCH: the Components page's Rescan / page read every 3 s after an
apt install outside the panel finished at T0; prints when the component shows as installed. Lab only."""
import json, sys, time
from pathlib import Path
L = sys.argv[1]; sys.path.insert(0, L); sys.path.insert(0, str(Path(L).parent))
import beta1
cell, node, cid, t0 = sys.argv[2], sys.argv[3], sys.argv[4], float(sys.argv[5])
st = beta1.load_state(cell, node); rec = beta1.Recorder(cell, beta1.redactor_for(st))
c = beta1.client_for(cell, node, rec, dict(st)); c.recorder = lambda e: None
def find(doc):
    L = doc if isinstance(doc, list) else (doc.get("services") or doc.get("items") or [])
    for s in L:
        if isinstance(s, dict) and s.get("id") == cid: return s
for mode in ("GET", "POST"):
    pass
end = time.time() + 120; first = {}
while time.time() < end:
    for mode, path in (("GET", "/api/v1/managed-services"), ("POST", "/api/v1/managed-services/scan")):
        if mode in first: continue
        r = c.request(mode, path, {} if mode == "POST" else None, timeout=120, purpose="scanpoll")
        s = find(r.json() or {}) or {}
        if s.get("is_installed"):
            first[mode] = round(time.time() - t0, 1)
            print(f"  {mode} {path}: {cid} installed after {first[mode]} s (status {s.get('status')}, versions {json.dumps(s.get('versions'))[:120]})", flush=True)
    if len(first) == 2: break
    time.sleep(3)
for mode in ("GET", "POST"):
    if mode not in first: print(f"  {mode}: {cid} NOT shown as installed within 120 s", flush=True)
