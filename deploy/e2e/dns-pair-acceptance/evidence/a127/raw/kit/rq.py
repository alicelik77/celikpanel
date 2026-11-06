#!/usr/bin/env python3
"""rq.py L CELL NODE METHOD PATH BODY RID N GAP: N admin requests with the SAME X-CelikPanel-Request-Id (RID),
GAP seconds apart (the second starts while the first runs), as a repeated press or a re-asked lost answer. Lab only."""
import json, sys, threading, time
from pathlib import Path
L = sys.argv[1]; sys.path.insert(0, L); sys.path.insert(0, str(Path(L).parent))
import beta1, panel_api
cell, node, method, path, body, rid, n, gap = sys.argv[2:10]
panel_api.secrets.token_hex = lambda k=16: rid
st = beta1.load_state(cell, node)
rec = beta1.Recorder(cell, beta1.redactor_for(st))
out = [None] * int(n)
def go(i):
    c = beta1.client_for(cell, node, rec, dict(st)); t = time.monotonic()
    try:
        r = c.request(method, path, beta1.body_arg(body), timeout=600, purpose=f"repeat {i}")
        out[i] = {"i": i, "status": r.status, "s": round(time.monotonic() - t, 2), "body": rec.redactor.text(r.text or "")[:500],
                  "step": (dict(r.headers) if isinstance(r.headers, list) else (r.headers or {})).get("X-CelikPanel-Operation-Step")}
    except Exception as e:
        out[i] = {"i": i, "error": str(e)[:300]}
ths = []
for i in range(int(n)):
    th = threading.Thread(target=go, args=(i,)); th.start(); ths.append(th); time.sleep(float(gap))
for th in ths: th.join()
for o in out: print(json.dumps(o, ensure_ascii=False))
