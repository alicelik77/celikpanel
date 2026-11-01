import json, pathlib, re, sys
root = pathlib.Path(sys.argv[1]); secrets = set()
for st in pathlib.Path("/var/tmp/cp-beta1/state").rglob("*.json"):
    try: d = json.loads(st.read_text())
    except Exception: continue
    def walk(x):
        if isinstance(x, str) and len(x) >= 8: secrets.add(x)
        elif isinstance(x, dict): [walk(v) for k, v in x.items() if k not in ("os", "ip", "provisioned_at", "username", "pin", "panel_port", "plan_id", "package_updates_digest", "start_request_id", "ids")]
        elif isinstance(x, list): [walk(v) for v in x]
    if st.name == "owner-secrets.json" or st.name.startswith("mbx-"):
        walk(d)
    else:
        for k in ("password", "cookie"):
            if d.get(k): secrets.add(d[k])
        for c in d.get("join_codes", []): secrets |= {c, c.replace("-", "")}
for f in pathlib.Path("/var/tmp/cp-beta1/state").rglob("*last-join-code"):
    c = f.read_text().strip(); secrets |= {c, c.replace("-", "")}
for f in pathlib.Path("/var/tmp/cp-beta1/state").rglob("invite-link-path"):
    secrets.add(f.read_text().strip().split("#t=")[-1])
for k in ("/var/tmp/a114r4/portal-key/private.pem", "/var/tmp/a114r4/portalnet/ca-key.pem", "/var/tmp/a114r4/portalnet/tls-key.pem"):
    p = pathlib.Path(k)
    if p.exists(): secrets.add(p.read_text().strip().splitlines()[1])
pem = re.compile(r"BEGIN [A-Z ]*PRIVATE KEY"); code = re.compile(r"\b[A-Z2-7]{4}(?:-[A-Z2-7]{4}){5}\b")
tok = re.compile(r"#t=(?!\[REDACTED\])[A-Za-z0-9_-]{20,}")
cred = re.compile(r'"(link_credential|enrollment_code|reverse_credential|peer_credential|proof|secret|recovery_codes|pending_token)"\s*:\s*"(?!\[REDACTED\])[^"]+')
pwv = re.compile(r'''['"](FTPPassword|password|new_password)['"][ ]*:[ ]*['"](?!\[REDACTED)[^'"]+''')
bad, n = [], 0
for p in root.rglob("*"):
    if not p.is_file() or p.suffix == ".png": continue
    n += 1; t = p.read_text(errors="replace")
    for s in secrets:
        if s and s in t: bad.append((str(p), "registered secret")); break
    for name, rx in (("pem private key", pem), ("join-code shape", code), ("link token", tok), ("credential value", cred), ("password value", pwv)):
        if rx.search(t): bad.append((str(p), name))
print(json.dumps({"files": n, "secrets_checked": len(secrets), "findings": bad[:30], "clean": not bad}))
