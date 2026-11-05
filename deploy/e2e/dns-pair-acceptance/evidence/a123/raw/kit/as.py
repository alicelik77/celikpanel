#!/usr/bin/env python3
"""as.py L CELL NODE CREDFILE METHOD PATH [BODY|@file]: one panel API call as another lab account
(creds JSON {"username","password"} in a 0600 file; session cookie cached next to it). Lab only."""
import json, os, sys
from pathlib import Path
L = sys.argv[1]; sys.path.insert(0, L); sys.path.insert(0, str(Path(L).parent))
import beta1
cell, node, credf, method, path = sys.argv[2:7]
body = sys.argv[7] if len(sys.argv) > 7 else None
cr = json.load(open(credf))
st = dict(beta1.load_state(cell, node)); st.pop("cookie", None)
rec = beta1.Recorder(cell, beta1.redactor_for(st)); rec.redactor.register(cr["password"])
c = beta1.client_for(cell, node, rec, st)
ck = credf + ".cookie"
if os.path.exists(ck):
    c.cookie = open(ck).read().strip()
def go():
    return c.request(method, path, beta1.body_arg(body), timeout=float(os.environ.get("AS_TIMEOUT","120")), purpose="as " + cr["username"])
r = go()
if r.status == 401:
    c.cookie = None
    lr = c.post("/api/v1/auth/login", {"username": cr["username"], "password": cr["password"]}, purpose="login as " + cr["username"])
    if lr.status != 200 or not c.cookie:
        print(json.dumps({"status": lr.status, "body": "login failed: " + (lr.text or "")[:300]})); sys.exit(0)
    fd = os.open(ck, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600); os.write(fd, c.cookie.encode()); os.close(fd)
    r = go()
print(json.dumps(beta1.short(r, 200000, rec.redactor)))
