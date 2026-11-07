#!/usr/bin/env python3
"""raw.py L CELL NODE METHOD PATH [options]: one raw panel request (pinned TLS) for the cases the JSON helpers cannot make:
raw upload bodies, binary downloads, Bearer tokens, cookie+token mixes, foreign Authorization headers. Lab only.
Options: --as admin|none|CREDFILE  (session cookie: the lab admin's, none, or a lab account's cached cookie file)
         --bearer-file F (Authorization: Bearer <file content>)  --authz VALUE (raw Authorization header)
         --body-file F (raw bytes)  --json STR  --out F (save the body)  --hdr K:V (repeatable)  --show N
Secrets: token values are never printed (cpk_... redacted)."""
import json, os, re, sys
from pathlib import Path
L = sys.argv[1]; sys.path.insert(0, L); sys.path.insert(0, str(Path(L).parent))
import beta1, lab
from panel_api import PinnedHTTPSTransport
cell, node, method, path = sys.argv[2:6]
opts = sys.argv[6:]
def opt(name, default=None, many=False):
    vals = [opts[i + 1] for i, o in enumerate(opts) if o == name and i + 1 < len(opts)]
    return vals if many else (vals[-1] if vals else default)
st = beta1.load_state(cell, node)
ip = lab.address_of(cell, node); port = st.get("panel_port") or beta1.panel_port(cell, node); pin = st.get("pin") or beta1.leaf(cell, node)
t = PinnedHTTPSTransport(ip, port, pin)
h = {"Accept": "application/json", "Origin": f"https://{ip}:{port}"}
who = opt("--as", "admin")
if who == "admin" and st.get("cookie"):
    h["Cookie"] = "celikpanel_session=" + st["cookie"]
elif who not in ("admin", "none"):
    ck = who + ".cookie"
    if os.path.exists(ck): h["Cookie"] = "celikpanel_session=" + open(ck).read().strip()
secrets = []
if opt("--bearer-file"):
    tok = open(opt("--bearer-file")).read().strip(); secrets.append(tok); h["Authorization"] = "Bearer " + tok
if opt("--authz"): h["Authorization"] = opt("--authz")
for kv in opt("--hdr", many=True):
    k, v = kv.split(":", 1); h[k] = v
body = None
if opt("--body-file"): body = open(opt("--body-file"), "rb").read(); h["Content-Type"] = "application/octet-stream"
if opt("--json-file"): body = open(opt("--json-file"), "rb").read(); h["Content-Type"] = "application/json"
if opt("--json") is not None: body = opt("--json").encode(); h["Content-Type"] = "application/json"
r = t(method, path, h, body, float(os.environ.get("RAW_TIMEOUT", "300")))
if opt("--out"):
    open(opt("--out"), "wb").write(r.body); print(r.status, "saved", len(r.body), "bytes", dict((k.lower(), v) for k, v in r.headers if k.lower() in ("content-type", "content-disposition")))
else:
    txt = r.body.decode("utf-8", "replace")
    for s in secrets: txt = txt.replace(s, "[TOKEN]")
    txt = re.sub(r"cpk_[A-Za-z0-9]{12}_[A-Za-z0-9_-]{20,}", "cpk_[REDACTED]", txt)
    txt = re.sub(r'("(password|token)"\s*:\s*")[^"]+', r'\1[REDACTED]', txt)
    hs = {k.lower(): v for k, v in r.headers if k.lower() in ("retry-after", "www-authenticate", "x-celikpanel-request-replayed")}
    print(r.status, (json.dumps(hs) + " ") if hs else "", txt[: int(opt("--show", "900"))].replace("\n", " "))
