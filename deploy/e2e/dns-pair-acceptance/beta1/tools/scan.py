# Secret scan of the evidence tree: every secret the run held (state files) must be absent.
import json, re, sys, pathlib
root = pathlib.Path(sys.argv[1])
secrets = set()
for st in pathlib.Path("/var/tmp/cp-beta1/state").rglob("*.json"):
    try: d = json.loads(st.read_text())
    except Exception: continue
    for k in ("password", "cookie"):
        if d.get(k): secrets.add(d[k])
    for c in d.get("join_codes", []): secrets |= {c, c.replace("-", "")}
for f in pathlib.Path("/var/tmp/cp-beta1/state").rglob("*last-join-code"):
    c = f.read_text().strip(); secrets |= {c, c.replace("-", "")}
for f in pathlib.Path("/var/tmp/cp-beta1/state").rglob("join-*.json"):
    c = json.loads(f.read_text())["code"]; secrets |= {c, c.replace("-", "")}
pem = re.compile(r"BEGIN [A-Z ]*PRIVATE KEY")
code = re.compile(r"\b[A-Z2-7]{4}(?:-[A-Z2-7]{4}){5}\b")
cred = re.compile(r'"(link_credential|enrollment_code|reverse_credential|peer_credential|proof)"\s*:\s*"(?!\[REDACTED\])[^"]+')
bad = []
n = 0
for p in root.rglob("*"):
    if not p.is_file(): continue
    n += 1
    t = p.read_text(errors="replace")
    for s in secrets:
        if s and s in t: bad.append((str(p), "registered secret"))
    if pem.search(t): bad.append((str(p), "pem private key"))
    if code.search(t): bad.append((str(p), "join-code shape"))
    if cred.search(t): bad.append((str(p), "credential value"))
print(json.dumps({"files": n, "secrets_checked": len(secrets), "findings": bad[:20], "clean": not bad}))
