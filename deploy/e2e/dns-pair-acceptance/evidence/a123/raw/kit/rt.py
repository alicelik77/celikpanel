#!/usr/bin/env python3
"""rt.py L CELL NODE PATH BODYJSON: one admin POST that carries X-CelikPanel-Request-Id equal to the body's
request_id, as the fixed Components page sends it (894fd63c9's page omitted it). Lab only."""
import json, sys
from pathlib import Path
L = sys.argv[1]; sys.path.insert(0, L); sys.path.insert(0, str(Path(L).parent))
import beta1, panel_api
cell, node, path, body = sys.argv[2:6]
body = json.loads(body)
st = beta1.load_state(cell, node)
rec = beta1.Recorder(cell, beta1.redactor_for(st))
c = beta1.client_for(cell, node, rec, st)
orig = c._headers
c._headers = lambda m, p: {**orig(m, p), panel_api.REQUEST_ID_HEADER: body["request_id"]}
r = c.request("POST", path, body, timeout=300, purpose="remove-together with header")
print(json.dumps(beta1.short(r, 200000, rec.redactor)))
