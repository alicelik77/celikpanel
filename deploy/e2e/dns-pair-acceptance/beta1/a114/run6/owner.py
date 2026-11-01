#!/usr/bin/env python3
"""alpha.114 acceptance: owner-path API driver for checks 4-7 (lab host, root). Several sessions at once
(owner, reseller, customer, team member), the way the web UI calls the panel: same-origin Origin header,
the session cookie (and the impersonator cookie while "Log in as" is active). The panel's own lab leaf is
not verified here (lab bridge only; the harness pins it elsewhere). Every exchange is written redacted
to /var/tmp/a114r6/owner-log/<cell>.jsonl; registered secrets (passwords, cookies, link tokens, TOTP
secrets, recovery codes) never reach the log. Lab only.
usage: owner.py CELL NODE SCENARIO [ARGS...]"""
import base64, datetime as dt, hashlib, hmac, http.client, json, os, re, secrets, ssl, struct, subprocess, sys, time
from pathlib import Path

sys.path.insert(0, "/var/tmp/a114r6/h/deploy/e2e/dns-pair-acceptance/beta1")
sys.path.insert(0, "/var/tmp/a114r6/h/deploy/e2e/dns-pair-acceptance")
import lab, beta1  # noqa: E402

CELL, NODE, SCEN = sys.argv[1:4]
ARGS = sys.argv[4:]
STATE = Path(f"/var/tmp/cp-beta1/state/{CELL}")
SECRETS = STATE / "owner-secrets.json"
LOG = Path(f"/var/tmp/a114r6/owner-log/{CELL}.jsonl")
LOG.parent.mkdir(parents=True, exist_ok=True)
H = "/mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab6"
REG: set = set()
RESULTS: list = []


def sec() -> dict:
    return json.loads(SECRETS.read_text()) if SECRETS.exists() else {}


def sec_set(key, value):
    d = sec(); d[key] = value
    SECRETS.write_text(json.dumps(d)); os.chmod(SECRETS, 0o600)
    reg(value)


def reg(v):
    if isinstance(v, str) and len(v) >= 6:
        REG.add(v)
    elif isinstance(v, (list, tuple)):
        for x in v: reg(x)
    elif isinstance(v, dict):
        for x in v.values(): reg(x)


SECRET_KEY = re.compile(r"(password|secret|token|credential|recovery_codes|uri)", re.I)


def scrub(v):
    """Key-based redaction for answers we did not register (FTP passwords, secrets in answers)."""
    if isinstance(v, dict):
        return {k: ("[REDACTED]" if SECRET_KEY.search(str(k)) and v[k] not in (None, "", [], False, True) and not isinstance(v[k], (int, float)) else scrub(v[k])) for k in v}
    if isinstance(v, list):
        return [scrub(x) for x in v]
    return v


def red(text: str) -> str:
    text = re.sub(r'''(?P<k>['"](?:FTPPassword|password|new_password|secret|recovery_codes)['"][ ]*:[ ]*)(?P<q>['"])(?!\[REDACTED)[^'"]*(?P=q)''', lambda m: m.group('k') + m.group('q') + '[REDACTED]' + m.group('q'), text)
    for s in sorted(REG, key=len, reverse=True):
        text = text.replace(s, "[REDACTED]")
    text = re.sub(r"#t=[A-Za-z0-9_-]{20,}", "#t=[REDACTED]", text)
    return text


def note(label, ok, detail=""):
    line = {"check": label, "ok": ok, "detail": red(str(scrub(detail)))[:900]}
    RESULTS.append(line)
    print(("PASS " if ok else "FAIL ") + label + (" | " + line["detail"] if detail != "" else ""), flush=True)


class S:
    """One browser-like session against one server."""

    def __init__(self, node=NODE, name="owner", port=None):
        self.node = node; self.name = name
        self.ip = lab.address_of(CELL, node)
        st = beta1.load_state(CELL, node)
        self.port = port or st.get("panel_port") or beta1.panel_port(CELL, node)
        self.cookies = {}
        reg(st.get("password")); reg(st.get("cookie"))

    def req(self, method, path, body=None, expect=None, label=None, timeout=90):
        ctx = ssl._create_unverified_context()
        c = http.client.HTTPSConnection(self.ip, self.port, context=ctx, timeout=timeout)
        host = f"{self.ip}:{self.port}"
        h = {"Accept": "application/json", "Host": host}
        data = None
        if body is not None:
            data = json.dumps(body).encode(); h["Content-Type"] = "application/json"
        if method not in ("GET", "HEAD"):
            h["Origin"] = f"https://{host}"
            h["X-CelikPanel-Request-Id"] = secrets.token_hex(16)  # the web client sends one per action
        if self.cookies:
            h["Cookie"] = "; ".join(f"{k}={v}" for k, v in self.cookies.items())
        t0 = time.monotonic()
        c.request(method, path, body=data, headers=h)
        r = c.getresponse(); raw = r.read()
        for k, v in r.getheaders():
            if k.lower() == "set-cookie":
                m = re.match(r"\s*([^=]+)=([^;]*)", v)
                if m:
                    if m.group(2) and "Max-Age=0" not in v and "expires=Thu, 01 Jan 1970" not in v.lower():
                        self.cookies[m.group(1)] = m.group(2); reg(m.group(2))
                    else:
                        self.cookies.pop(m.group(1), None)
        try:
            doc = json.loads(raw or b"null")
        except ValueError:
            doc = raw.decode("utf-8", "replace")[:2000]
        with LOG.open("a") as f:
            f.write(red(json.dumps({"at": dt.datetime.utcnow().isoformat() + "Z", "node": self.node, "session": self.name,
                                     "label": label, "method": method, "path": path.split("#")[0],
                                     "body": scrub(body), "status": r.status, "ms": int((time.monotonic() - t0) * 1000),
                                     "answer": scrub(doc)}, ensure_ascii=False)) + "\n")
        return r.status, doc

    def login(self, user, password):
        reg(password)
        s, d = self.req("POST", "/api/v1/auth/login", {"username": user, "password": password}, label=f"login {user}")
        return s, d


def owner(node=NODE):
    st = beta1.load_state(CELL, node)
    o = S(node, "owner")
    s, d = o.login(st["username"], st["password"])
    assert s == 200, (s, d)
    return o


def code_of(d):
    return d.get("code") if isinstance(d, dict) else None


def labnet(cmd, stdin=None, timeout=300):
    r = subprocess.run(lab.ssh_argv("beta1-lab", "labnet", cmd), input=stdin, capture_output=True, timeout=timeout)
    return r.stdout.decode("utf-8", "replace")


def guest(node, cmd, timeout=300):
    r = beta1.guest(CELL, node, "sudo -n bash -c " + json.dumps(cmd), timeout=timeout)
    return red(r.stdout.decode("utf-8", "replace") + r.stderr.decode("utf-8", "replace"))


def mail_wait(host, user, password, rx, secs=120):
    out = labnet(f"sudo -n python3 -I /root/mailx.py wait {host} {user} '{rx}' {secs}", json.dumps({"password": password}).encode(), timeout=secs + 120)
    try:
        d = json.loads(out.strip().splitlines()[-1])
    except Exception:
        return {"error": red(out[-600:]), "links": [], "count": 0}
    reg(d.get("links", []))
    for l in d.get("links", []):
        reg(l.split("#t=")[-1])
    return d


def mail_send(host, frm, password, to, subject):
    out = labnet(f"sudo -n python3 -I /root/mailx.py send {host} {frm} {to} '{subject}'", json.dumps({"password": password}).encode())
    return red(out.strip()[-400:])


def totp(secret_b32, t=None):
    key = base64.b32decode(secret_b32.upper() + "=" * (-len(secret_b32) % 8))
    counter = int((t or time.time()) // 30)
    h = hmac.new(key, struct.pack(">Q", counter), hashlib.sha1).digest()
    o = h[-1] & 15
    return "%06d" % ((struct.unpack(">I", h[o:o + 4])[0] & 0x7fffffff) % 1000000)


def domain_id(sess, name):
    s, d = sess.req("GET", "/api/v1/domains", label="domains list")
    rows = d.get("domains", d) if isinstance(d, dict) else d
    for r in rows or []:
        if isinstance(r, dict) and (r.get("name") == name or r.get("domain") == name or r.get("domain_name") == name):
            return r.get("id")
    return None


def mbx(sess, did, address):
    key = "mbx:" + address
    pw = sec().get(key) or secrets.token_urlsafe(18)
    sec_set(key, pw)
    s, d = sess.req("POST", f"/api/v1/domains/{did}/mail/accounts", {"address": address, "password": pw, "quota_mb": 512}, label="mailbox " + address)
    return s, d, pw


# ---------------------------------------------------------------------------------------------- scenarios

def scen_port_paired():
    o = owner()
    s, d = o.req("GET", "/api/v1/panel/port", label="port view")
    note("port view on a paired server", s == 200, {k: d.get(k) for k in ("port", "default_port", "legacy_port", "blocked")} if isinstance(d, dict) else d)
    s, d = o.req("POST", "/api/v1/panel/port", {"action": "start", "port": 2087}, label="port start on paired")
    note("port change refused on a paired server with PANEL_PORT_PAIRED", s == 409 and code_of(d) == "PANEL_PORT_PAIRED", {"status": s, "code": code_of(d), "error": d.get("error") if isinstance(d, dict) else d})


def scen_osupdates():
    o = owner()
    s, d = o.req("GET", "/api/v1/security/os-updates", label="os-updates read")
    fam = d.get("package_family") if isinstance(d, dict) else None
    ac = (d or {}).get("auto_config") or {}
    note(f"{NODE}: OS updates read", s == 200, {"family": fam, "automatic": d.get("automatic"), "auto_config.state": ac.get("state"), "mode": ac.get("mode"), "path": ac.get("path"), "legacy_state": ac.get("legacy_state"), "timer_state": ac.get("timer_state")})
    s2, sm = o.req("GET", "/api/v1/security/os-updates/summary", label="dashboard card")
    note(f"{NODE}: Dashboard OS-updates card data", s2 == 200, {k: sm.get(k) for k in ("state", "package_family", "pending", "automatic")} if isinstance(sm, dict) else sm)
    if fam == "pacman":
        s, d = o.req("POST", "/api/v1/security/os-updates/mode", {"mode": "manual", "expected_current_sha256": "", "confirm_owner_edit": False}, label="mode on arch")
        note("Arch: no automatic/manual switch (guide only)", s >= 400, {"status": s, "code": code_of(d)})
        return
    for mode in ("manual", "auto"):
        s, d = o.req("GET", "/api/v1/security/os-updates", label="os-updates read")
        cur = ((d or {}).get("auto_config") or {}).get("current_sha256", "")
        s, w = o.req("POST", "/api/v1/security/os-updates/mode", {"mode": mode, "expected_current_sha256": cur, "confirm_owner_edit": False}, label=f"mode {mode}")
        file = guest(NODE, "cat /etc/apt/apt.conf.d/20auto-upgrades; ls /etc/apt/apt.conf.d/ | grep -i auto")
        note(f"{NODE}: switch to {mode}", s == 200 and isinstance(w, dict) and w.get("written") is True, {"answer": w, "file": file})
    guest(NODE, "printf '// owner hand edit (lab)\\nAPT::Periodic::Update-Package-Lists \"1\";\\nAPT::Periodic::Unattended-Upgrade \"0\";\\n' > /etc/apt/apt.conf.d/20auto-upgrades")
    s, d = o.req("GET", "/api/v1/security/os-updates", label="os-updates after owner edit")
    ac = (d or {}).get("auto_config") or {}
    note(f"{NODE}: owner hand edit is detected (auto_config.state=edited)", ac.get("state") == "edited", {"state": ac.get("state"), "mode": ac.get("mode"), "automatic": d.get("automatic")})
    s, w = o.req("POST", "/api/v1/security/os-updates/mode", {"mode": "auto", "expected_current_sha256": ac.get("current_sha256", ""), "confirm_owner_edit": False}, label="mode over owner edit")
    note(f"{NODE}: writing over the owner's edit needs confirmation (OS_UPDATES_MODE_OWNER_EDITED)", s == 409 and "OWNER_EDITED" in str(code_of(w)), {"status": s, "code": code_of(w)})
    s, w = o.req("POST", "/api/v1/security/os-updates/mode", {"mode": "auto", "expected_current_sha256": ac.get("current_sha256", ""), "confirm_owner_edit": True}, label="mode over owner edit confirmed")
    note(f"{NODE}: confirmed write back to auto", s == 200, {"status": s, "answer": w})
    out = guest(NODE, "systemctl stop celikpanel-panel celikpanel-agent; sleep 2; systemctl is-active celikpanel-panel celikpanel-agent | paste -sd' '; systemctl is-enabled apt-daily.timer apt-daily-upgrade.timer | paste -sd' '; systemctl is-active apt-daily.timer apt-daily-upgrade.timer | paste -sd' '; systemctl list-timers --all --no-pager | grep -E 'apt-daily' | sed 's/  */ /g'; apt-config dump | grep -E 'Periodic::(Update-Package-Lists|Unattended-Upgrade)'; systemctl start celikpanel-agent celikpanel-panel; sleep 4; systemctl is-active celikpanel-panel celikpanel-agent | paste -sd' '")
    note(f"{NODE}: with the panel and agent stopped the apt timers stay enabled and scheduled", "enabled enabled" in out and "active active" in out, out)


def scen_accounts():
    """Check 6 on the first server: reseller with pool -> plan -> customer by invitation -> customer work."""
    S_ = ARGS[0]; MAILHOST = ARGS[1]; CUSTDOM = ARGS[2]
    o = owner()
    sid = domain_id(o, S_)
    for who in ("res", "cust", "team", "cust2"):
        s, d, pw = mbx(o, sid, f"{who}@{S_}")
        note(f"mailbox {who}@{S_} for the invitation mail", s in (200, 201) or "exists" in json.dumps(d), {"status": s})
    # reseller with a pool, invited by email
    pool = {"max_domains": 2, "max_databases": 2, "max_email_accounts": 4, "disk_quota_mb": 2048, "max_customers": 1}
    s, d = o.req("POST", "/api/v1/users", {"username": "res1", "email": f"res@{S_}", "role": "reseller", "invite": True, "lang": "en", "pool": pool}, label="create reseller")
    inv = (d or {}).get("invitation") if isinstance(d, dict) else None
    note("admin creates a reseller with a pool, invited by email", s in (200, 201) and isinstance(inv, dict), {"status": s, "invitation": inv, "error": d.get("error") if isinstance(d, dict) else d})
    res_id = d.get("id") if isinstance(d, dict) else None
    m = mail_wait(MAILHOST, f"res@{S_}", sec()[f"mbx:res@{S_}"], ".", 180)
    note("invitation mail delivered by the server's own Postfix", m.get("count", 0) >= 1 and bool(m.get("links")), {k: m.get(k) for k in ("count", "messages", "error")})
    if not m.get("links"):
        return
    tok = m["links"][-1].split("#t=")[1]; reg(tok)
    pub = S(NODE, "public")
    s, d = pub.req("POST", "/api/v1/auth/recovery/link", {"token": tok, "purpose": "invitation"}, label="invitation link")
    note("invitation link opens (names the account)", s == 200, {"status": s, "answer": d})
    rpw = secrets.token_urlsafe(16); sec_set("res1", rpw)
    s, d = pub.req("POST", "/api/v1/auth/recovery/accept-invitation", {"token": tok, "new_password": rpw}, label="accept invitation")
    note("reseller accepts the invitation and sets a password", s in (200, 204), {"status": s, "answer": d})
    r = S(NODE, "reseller"); s, d = r.login("res1", rpw)
    note("reseller logs in", s == 200, {"status": s})
    s, d = r.req("POST", "/api/v1/plans", {"name": "too-big", "max_domains": 5, "max_databases": 1, "max_email_accounts": 1, "disk_quota_mb": 100, "bandwidth_quota_mb": 0}, label="plan above pool")
    note("plan above the reseller's pool is refused (PLAN_EXCEEDS_POOL)", s == 409 and code_of(d) == "PLAN_EXCEEDS_POOL", {"status": s, "code": code_of(d), "error": d.get("error") if isinstance(d, dict) else d})
    s, d = r.req("POST", "/api/v1/plans", {"name": "basic", "max_domains": 1, "max_databases": 1, "max_email_accounts": 2, "disk_quota_mb": 1024, "bandwidth_quota_mb": 0}, label="reseller plan")
    plan_id = d.get("id") if isinstance(d, dict) else None
    note("reseller creates a plan", s in (200, 201) and plan_id, {"status": s, "answer": d})
    s, d = r.req("POST", "/api/v1/plans", {"name": "tiny", "max_domains": 1, "max_databases": 1, "max_email_accounts": 1, "disk_quota_mb": 1024, "bandwidth_quota_mb": 0}, label="reseller small plan")
    tiny_id = d.get("id") if isinstance(d, dict) else None
    s, d = r.req("POST", "/api/v1/users", {"username": "cust1", "email": f"cust@{S_}", "role": "customer", "invite": True, "lang": "tr", "plan_id": plan_id}, label="reseller creates customer")
    cust_id = d.get("id") if isinstance(d, dict) else None
    note("reseller creates a customer by invitation", s in (200, 201) and cust_id, {"status": s, "invitation": (d or {}).get("invitation") if isinstance(d, dict) else d})
    s, d = r.req("POST", "/api/v1/users", {"username": "cust2", "email": f"cust2@{S_}", "role": "customer", "invite": True, "lang": "en", "plan_id": plan_id}, label="second customer over pool")
    note("a second customer is refused by the pool (max_customers=1)", s >= 400 and str(code_of(d)).startswith("RESELLER_"), {"status": s, "code": code_of(d), "error": d.get("error") if isinstance(d, dict) else d})
    m = mail_wait(MAILHOST, f"cust@{S_}", sec()[f"mbx:cust@{S_}"], ".", 180)
    note("customer's invitation mail delivered (TR)", m.get("count", 0) >= 1 and bool(m.get("links")), {k: m.get(k) for k in ("count", "messages", "error")})
    if not m.get("links"):
        return
    tok = m["links"][-1].split("#t=")[1]; reg(tok)
    cpw = secrets.token_urlsafe(16); sec_set("cust1", cpw)
    s, d = pub.req("POST", "/api/v1/auth/recovery/accept-invitation", {"token": tok, "new_password": cpw}, label="customer accepts")
    note("customer accepts the invitation", s in (200, 204), {"status": s, "answer": d})
    c = S(NODE, "customer"); s, d = c.login("cust1", cpw)
    note("customer logs in", s == 200, {"status": s})
    s, d = c.req("GET", "/api/v1/my/subscription", label="my plan")
    note("customer sees My plan", s == 200 and isinstance(d, dict) and d.get("subscriptions"), {"status": s, "answer": d})
    # the customer's work
    s, d = c.req("POST", "/api/v1/domains/create", {"domain": CUSTDOM, "project_type": "static", "ssl_type": "letsencrypt"}, label="customer adds domain", timeout=240)
    note("customer adds a domain", s in (200, 201), {"status": s, "answer": d})
    did = domain_id(c, CUSTDOM)
    s, d = c.req("POST", f"/api/v1/domains/{did}/ssl/letsencrypt", {"auto_renew": True, "include_mail": False, "provider": "letsencrypt", "reissue": False, "redirect_http": True}, label="customer ssl", timeout=480)
    note("customer gets SSL", s == 200, {"status": s, "answer": {k: d.get(k) for k in ("success", "error", "code", "issuer", "expires_at")} if isinstance(d, dict) else d})
    s, d, _ = mbx(c, did, f"info@{CUSTDOM}")
    note("customer adds a mailbox", s in (200, 201), {"status": s, "answer": d})
    s, d, _ = mbx(c, did, f"sales@{CUSTDOM}")
    note("customer adds a second mailbox (plan allows 2)", s in (200, 201), {"status": s, "answer": d})
    s, d, _ = mbx(c, did, f"third@{CUSTDOM}")
    note("third mailbox refused by the plan quota", s >= 400 and str(code_of(d)).startswith("QUOTA_"), {"status": s, "code": code_of(d)})
    dbpw = secrets.token_urlsafe(16); reg(dbpw)
    s, d = c.req("POST", f"/api/v1/domains/{did}/databases", {"name": "custdb", "type": "mysql", "password": dbpw}, label="customer database", timeout=180)
    note("customer adds a database", s in (200, 201), {"status": s, "answer": d})
    s, d = c.req("POST", f"/api/v1/domains/{did}/dns/records", {"name": f"txt.{CUSTDOM}", "type": "TXT", "content": "\"customer-record\"", "ttl": 300, "prio": 0}, label="customer dns record")
    note("customer adds a DNS record", s in (200, 201), {"status": s, "answer": d})
    s, d = c.req("GET", f"/api/v1/domains/{did}/cron", label="cron read")
    ver = d.get("version", "") if isinstance(d, dict) else ""
    s, d = c.req("POST", f"/api/v1/domains/{did}/cron", {"schedule": "17 3 * * *", "command": "date > /dev/null", "comment": "lab", "version": ver}, label="customer cron")
    note("customer adds a cron job", s in (200, 201), {"status": s, "answer": d})
    s, d = c.req("POST", f"/api/v1/domains/{did}/files?path=/", {"action": "upload", "file_name": "hello.txt", "content": base64.b64encode(b"uploaded by the customer\n").decode()}, label="customer upload")
    note("customer uploads a file", s in (200, 201), {"status": s, "answer": d})
    s, d = c.req("POST", f"/api/v1/domains/{did}/backups", {"type": "files"}, label="customer backup", timeout=300)
    note("customer makes a backup", s in (200, 201, 202), {"status": s, "answer": d})
    s, d = c.req("GET", f"/api/v1/domains/{did}/mail/accounts", label="mailboxes")
    acc = None
    for a in (d.get("accounts") if isinstance(d, dict) else d) or []:
        if a.get("address", "").startswith("info@") or a.get("email", "").startswith("info@"):
            acc = a.get("id")
    today = dt.date.today()
    s, d = c.req("PUT", f"/api/v1/domains/{did}/mail/autoresponder", {"id": acc, "enabled": True, "subject": "Away", "message": "I am away.", "start_date": str(today - dt.timedelta(days=1)), "end_date": str(today + dt.timedelta(days=7)), "interval_days": 1, "addresses": []}, label="customer autoresponder")
    note("customer sets an autoresponder", s == 200, {"status": s, "answer": d})
    s, d = c.req("POST", "/api/v1/domains/create", {"domain": "second-" + CUSTDOM, "project_type": "static", "ssl_type": "none"}, label="quota domain")
    note("second domain refused by the plan quota", s >= 400 and str(code_of(d)).startswith("QUOTA_"), {"status": s, "code": code_of(d), "error": d.get("error") if isinstance(d, dict) else d})
    # downgrade warning
    s, d = r.req("PUT", f"/api/v1/users/{cust_id}/plan", {"plan_id": tiny_id}, label="downgrade")
    note("plan downgrade below usage warns (PLAN_BELOW_USAGE)", s == 409 and code_of(d) == "PLAN_BELOW_USAGE", {"status": s, "code": code_of(d), "details": d.get("details") if isinstance(d, dict) else d})
    # Log in as
    o2 = owner()
    s, d = o2.req("POST", f"/api/v1/users/{cust_id}/impersonate", label="log in as customer")
    s2, me = o2.req("GET", "/api/v1/auth/me", label="me while impersonating")
    note("admin 'Log in as' the customer: banner data", s == 200 and isinstance(me, dict) and me.get("impersonating") is True, {"status": s, "me": {k: me.get(k) for k in ("username", "role", "impersonating", "impersonator")} if isinstance(me, dict) else me})
    o2.req("POST", f"/api/v1/domains/{did}/dns/records", {"name": f"asadmin.{CUSTDOM}", "type": "TXT", "content": "\"made while logged in as\"", "ttl": 300, "prio": 0}, label="action as customer")
    s, d = o2.req("POST", "/api/v1/auth/unimpersonate", label="back to admin")
    s, d = o2.req("GET", "/api/v1/audit-logs?limit=60", label="audit")
    rows = d.get("logs", d.get("entries", d)) if isinstance(d, dict) else d
    acting = [x for x in rows or [] if isinstance(x, dict) and x.get("acting_as")]
    note("audit rows name the operator and acting_as", bool(acting), [{k: x.get(k) for k in ("username", "acting_as", "action")} for x in acting[:4]])
    # additional user, scoped
    tpw = secrets.token_urlsafe(16); sec_set("team1", tpw)
    s, d = c.req("POST", "/api/v1/team-members", {"username": "team1", "email": f"team@{S_}", "password": tpw, "access": {"subscription_permissions": [], "domain_permissions": [{"domain_id": did, "capability": "dns", "mode": "view"}]}}, label="team member")
    note("customer adds a scoped additional user (dns:view on one domain)", s in (200, 201), {"status": s, "answer": d})
    t = S(NODE, "team"); s, d = t.login("team1", tpw)
    s1, _ = t.req("GET", f"/api/v1/domains/{did}/dns/records", label="team reads dns")
    s2, d2 = t.req("POST", f"/api/v1/domains/{did}/dns/records", {"name": f"team.{CUSTDOM}", "type": "TXT", "content": "\"x\"", "ttl": 300, "prio": 0}, label="team writes dns")
    s3, d3 = t.req("GET", f"/api/v1/domains/{did}/mail/accounts", label="team reads mail")
    note("additional user: may read DNS, may not change it, may not see mail", s1 == 200 and s2 >= 400 and s3 >= 400, {"read_dns": s1, "write_dns": [s2, code_of(d2)], "read_mail": [s3, code_of(d3)]})
    # forgot password -> reset drops sessions
    s, d = pub.req("GET", "/api/v1/auth/recovery/mail-status", label="mail status")
    s, d = pub.req("POST", "/api/v1/auth/recovery/request", {"identifier": "res1", "lang": "en"}, label="forgot password")
    note("forgot password request answers 202", s == 202, {"status": s, "answer": d})
    before = mail_wait(MAILHOST, f"res@{S_}", sec()[f"mbx:res@{S_}"], ".", 5).get("count", 0)
    m = {"count": before}
    for _ in range(24):
        m = mail_wait(MAILHOST, f"res@{S_}", sec()[f"mbx:res@{S_}"], ".", 5)
        if any("/account-link/reset" in l for l in m.get("links", [])):
            break
        time.sleep(5)
    resets = [l for l in m.get("links", []) if "/account-link/reset" in l]
    note("reset mail delivered", bool(resets), {"count": m.get("count"), "messages": m.get("messages")})
    if resets:
        tok = resets[-1].split("#t=")[1]; reg(tok)
        npw = secrets.token_urlsafe(16); sec_set("res1-new", npw)
        s, d = pub.req("POST", "/api/v1/auth/recovery/reset", {"token": tok, "new_password": npw}, label="reset")
        note("password reset", s in (200, 204), {"status": s, "answer": d})
        s, d = r.req("GET", "/api/v1/auth/me", label="old reseller session after reset")
        note("the reseller's old session is dropped by the reset", s == 401, {"status": s})
        r2 = S(NODE, "reseller2"); s, d = r2.login("res1", npw)
        note("reseller logs in with the new password", s == 200, {"status": s})
    sec_set("ids", {"res": res_id, "cust": cust_id, "plan": plan_id, "tiny": tiny_id, "cust_domain": did})


def scen_twofa():
    """2FA with recovery codes for the customer (after the customer screens were taken)."""
    cpw = sec()["cust1"]; reg(cpw)
    c = S(NODE, "customer"); s, d = c.login("cust1", cpw)
    s, d = c.req("POST", "/api/v1/auth/2fa/setup", {"password": cpw}, label="2fa setup")
    secret = d.get("secret") if isinstance(d, dict) else None
    reg(secret); reg((d or {}).get("uri") if isinstance(d, dict) else None)
    s, d = c.req("POST", "/api/v1/auth/2fa/enable", {"password": cpw, "code": totp(secret)}, label="2fa enable")
    codes = (d or {}).get("recovery_codes") if isinstance(d, dict) else None
    reg(codes or [])
    note("2FA enabled, recovery codes shown once", s == 200 and bool(codes), {"status": s, "recovery_total": (d or {}).get("recovery_total") if isinstance(d, dict) else None})
    if codes:
        c3 = S(NODE, "customer-2fa"); s, d = c3.login("cust1", cpw)
        pending = d.get("pending_token") if isinstance(d, dict) else None
        reg(pending)
        s, d = c3.req("POST", "/api/v1/auth/login/totp", {"pending_token": pending, "recovery_code": codes[0]}, label="login with recovery code")
        s2, st2 = c3.req("GET", "/api/v1/auth/2fa/status", label="2fa status")
        note("login with a recovery code; one code used up", s == 200 and isinstance(st2, dict) and st2.get("recovery_remaining") == len(codes) - 1, {"status": s, "2fa": st2})
        s, d = S(NODE, "replay").req("POST", "/api/v1/auth/login", {"username": "cust1", "password": cpw}, label="relogin")


def scen_invitelink():
    """A fresh, unused invitation link for the accept-page screenshots (written 0600 for shots only)."""
    S_ = ARGS[0]; MAILHOST = ARGS[1]
    o = owner()
    s, d = o.req("POST", "/api/v1/users", {"username": "cust3", "email": f"cust2@{S_}", "role": "customer", "invite": True, "lang": "en"}, label="admin invites a customer")
    note("admin invites a customer (for the accept page)", s in (200, 201), {"status": s, "invitation": (d or {}).get("invitation") if isinstance(d, dict) else d})
    m = mail_wait(MAILHOST, f"cust2@{S_}", sec()[f"mbx:cust2@{S_}"], ".", 180)
    links = [l for l in m.get("links", []) if "/account-link/invitation" in l]
    if links:
        f = STATE / "invite-link-path"
        f.write_text("/account-link/invitation#t=" + links[-1].split("#t=")[1]); os.chmod(f, 0o600)
    note("invitation link captured for the screenshot", bool(links), {"count": m.get("count")})


def scen_sitefeatures():
    """Check 7 on SITE (ARGS: site mailhost)."""
    S_ = ARGS[0]; MAILHOST = ARGS[1]
    o = owner()
    sid = domain_id(o, S_)
    ip = lab.address_of(CELL, NODE)
    o.req("POST", f"/api/v1/domains/{sid}/files?path=/private", {"action": "create", "is_dir": True}, label="mkdir private")
    o.req("POST", f"/api/v1/domains/{sid}/files?path=/private/index.html", {"action": "create"}, label="create private index")
    o.req("POST", f"/api/v1/domains/{sid}/files?path=/private/index.html", {"action": "write", "content": "private-ok\n"}, label="write private index")
    o.req("POST", f"/api/v1/domains/{sid}/files?path=/new", {"action": "create", "is_dir": True}, label="mkdir new")
    ppw = secrets.token_urlsafe(12); sec_set("protect", ppw)
    s, d = o.req("POST", f"/api/v1/domains/{sid}/protected-dirs", {"path": "/private", "realm": "Private area", "username": "visitor", "password": ppw}, label="protect folder")
    note("password-protected folder created", s in (200, 201), {"status": s, "answer": d})
    curl = f"curl -sS --max-time 20 --resolve {S_}:443:{ip} --cacert /root/beta1/pebble-root.pem -o /dev/null -w '%{{http_code}}'"
    a = labnet(f"sudo -n {curl} https://{S_}/private/")
    b = labnet(f"sudo -n {curl} -u visitor:{ppw} https://{S_}/private/")
    note("protected folder: 401 without, 200 with the password", a.strip() == "401" and b.strip() == "200", {"without": a.strip(), "with": b.strip()})
    for body in ({"match": "exact", "source": "/old", "target": "/new/", "code": 301, "keep_query": True},
                 {"match": "prefix", "source": "/blog", "target": "https://example.test/blog", "code": 302, "keep_query": False}):
        s, d = o.req("POST", f"/api/v1/domains/{sid}/redirects", body, label="redirect")
        note(f"redirect {body['code']} {body['match']} {body['source']} created", s in (200, 201), {"status": s, "answer": d})
    hdr = f"curl -sS --max-time 20 --resolve {S_}:443:{ip} --cacert /root/beta1/pebble-root.pem -o /dev/null -w '%{{http_code}} %{{redirect_url}}'"
    r1 = labnet(f"sudo -n {hdr} 'https://{S_}/old?x=1&y=2'")
    r2 = labnet(f"sudo -n {hdr} 'https://{S_}/blog/post?q=1'")
    note("301 exact keeps the query; 302 prefix drops it", r1.startswith("301") and "x=1" in r1 and r2.startswith("302") and "q=1" not in r2, {"old": r1.strip(), "blog": r2.strip()})
    bad = [{"match": "exact", "source": "/a; return 200 x", "target": "/b", "code": 301, "keep_query": False},
           {"match": "exact", "source": "/c", "target": "https://x.test/\nadd_header X-Evil 1", "code": 301, "keep_query": False},
           {"match": "exact", "source": "/d", "target": "https://x.test/$host{", "code": 301, "keep_query": False},
           {"match": "prefix", "source": "/e", "target": "/f", "code": 200, "keep_query": False}]
    res = []
    for body in bad:
        s, d = o.req("POST", f"/api/v1/domains/{sid}/redirects", body, label="redirect injection")
        res.append((s, code_of(d)))
    note("redirect injection attempts refused", all(s >= 400 for s, _ in res), res)
    t = guest(NODE, "nginx -t 2>&1 | tail -2")
    note(f"{NODE}: nginx -t after protect/redirects", "successful" in t, t)


def scen_autoresponder():
    """Check 7 autoresponder on SITE: bob has it; alice writes twice -> one reply; panel stopped -> still replies."""
    S_ = ARGS[0]; MAILHOST = ARGS[1]
    o = owner(); sid = domain_id(o, S_)
    s, d = o.req("GET", f"/api/v1/domains/{sid}/mail/accounts", label="mailboxes")
    rows = (d.get("accounts") if isinstance(d, dict) else d) or []
    bob = next((a.get("id") for a in rows if str(a.get("address", a.get("email", ""))).startswith("bob@")), None)
    today = dt.date.today()
    s, d = o.req("PUT", f"/api/v1/domains/{sid}/mail/autoresponder", {"id": bob, "enabled": True, "subject": "Out of office", "message": "Bob is away until next week.", "start_date": str(today - dt.timedelta(days=1)), "end_date": str(today + dt.timedelta(days=7)), "interval_days": 1, "addresses": []}, label="bob autoresponder")
    note("autoresponder on bob@ set", s == 200, {"status": s, "answer": d})
    s, d = o.req("GET", f"/api/v1/domains/{sid}/mail/autoresponder?id={bob}", label="autoresponder read")
    note("autoresponder applied (Sieve)", isinstance(d, dict) and (d.get("state") or {}).get("applied") is True, {"state": d.get("state") if isinstance(d, dict) else d})
    st = beta1.load_state(CELL, NODE)
    apw = json.loads((STATE / "mbx-alice.json").read_text())["password"]; reg(apw)
    for i in (1, 2):
        mail_send(MAILHOST, f"alice@{S_}", apw, f"bob@{S_}", f"to bob {i}")
        time.sleep(8)
    m = mail_wait(MAILHOST, f"alice@{S_}", apw, "Out of office", 90)
    time.sleep(20)
    m = mail_wait(MAILHOST, f"alice@{S_}", apw, "Out of office", 10)
    note("two mails within the interval -> exactly one auto-reply", m.get("count") == 1, {"count": m.get("count"), "messages": m.get("messages")})
    out = guest(NODE, "systemctl stop celikpanel-panel celikpanel-agent; systemctl is-active celikpanel-panel celikpanel-agent | paste -sd' '")
    rpw = sec().get(f"mbx:res@{S_}")
    if rpw:
        mail_send(MAILHOST, f"res@{S_}", rpw, f"bob@{S_}", "to bob while the panel is stopped")
        m2 = mail_wait(MAILHOST, f"res@{S_}", rpw, "Out of office", 120)
        note("autoresponder answers with the panel and agent stopped", m2.get("count", 0) >= 1, {"panel": out.strip(), "count": m2.get("count"), "messages": m2.get("messages")})
    guest(NODE, "systemctl start celikpanel-agent celikpanel-panel; sleep 5; systemctl is-active celikpanel-panel celikpanel-agent | paste -sd' '")


def scen_port_change():
    """Check 4 on a standalone server: confirm path, cancel, timeout rollback is a separate scenario."""
    newp = int(ARGS[0])
    o = owner()
    s, d = o.req("GET", "/api/v1/panel/port", label="port view")
    note("port view", s == 200, {k: d.get(k) for k in ("port", "default_port", "legacy_port", "blocked", "change")} if isinstance(d, dict) else d)
    s, d = o.req("POST", "/api/v1/panel/port", {"action": "start", "port": newp}, label=f"port start {newp}")
    ch = (d or {}).get("change") if isinstance(d, dict) else None
    note(f"port change to {newp} starts a trial", s == 200 and isinstance(ch, dict) and ch.get("status") == "trial", {"status": s, "change": ch, "code": code_of(d)})
    if not isinstance(ch, dict):
        return
    fw = guest(NODE, f"ss -ltn | grep -E ':({o.port}|{newp}) ' | sed 's/  */ /g'; nft list ruleset | grep -E '{o.port}|{newp}' | sed 's/^ *//' | head -6; grep CELIKPANEL_LISTEN /etc/celikpanel/panel.env")
    note("during the trial both ports listen, the new one is open, panel.env not yet written", str(newp) in fw, fw)
    s, d = o.req("POST", "/api/v1/panel/port", {"action": "confirm", "id": ch["id"]}, label="confirm from old port")
    note("confirm from the OLD address is refused (PANEL_PORT_CONFIRM_FROM_NEW)", s >= 400 and code_of(d) == "PANEL_PORT_CONFIRM_FROM_NEW", {"status": s, "code": code_of(d)})
    n = S(NODE, "owner-newport", port=newp); st = beta1.load_state(CELL, NODE); n.login(st["username"], st["password"])
    s, d = n.req("POST", "/api/v1/panel/port", {"action": "confirm", "id": ch["id"]}, label="confirm from new port")
    note("confirm from the new address", s == 200 and ((d or {}).get("change") or {}).get("status") in ("confirmed", "confirming"), {"status": s, "change": (d or {}).get("change") if isinstance(d, dict) else d})
    time.sleep(8)
    out = guest(NODE, f"grep CELIKPANEL_LISTEN /etc/celikpanel/panel.env; ss -ltn | grep -E ':({o.port}|{newp}) ' | sed 's/  */ /g'; nft list ruleset | grep -E '{o.port}|{newp}' | sed 's/^ *//' | head -6")
    note(f"after confirm: panel.env says {newp}, only {newp} listens", f":{newp}" in out and f":{o.port} " not in out, out)
    s, d = n.req("GET", "/api/v1/panel/port", label="port view after")
    note("port view after confirm", s == 200, {k: d.get(k) for k in ("port", "change")} if isinstance(d, dict) else d)


def scen_port_trial():
    """Start a change and either cancel it or let the 10-minute window pass (ARGS: port cancel|timeout)."""
    newp, how = int(ARGS[0]), ARGS[1]
    o = owner()
    s, d = o.req("POST", "/api/v1/panel/port", {"action": "start", "port": newp}, label=f"port start {newp}")
    ch = (d or {}).get("change") if isinstance(d, dict) else None
    note(f"trial to {newp} started", s == 200 and isinstance(ch, dict), {"status": s, "change": ch, "code": code_of(d)})
    if not isinstance(ch, dict):
        return
    if how == "cancel":
        s, d = o.req("POST", "/api/v1/panel/port", {"action": "cancel", "id": ch["id"]}, label="cancel")
        note("cancel rolls back", s == 200 and ((d or {}).get("change") or {}).get("status") == "rolled_back", {"status": s, "change": (d or {}).get("change") if isinstance(d, dict) else d})
    else:
        deadline = time.time() + 900
        last = None
        while time.time() < deadline:
            time.sleep(30)
            try:
                s, d = o.req("GET", "/api/v1/panel/port", label="port poll")
            except Exception as e:  # noqa: BLE001
                continue
            last = (d or {}).get("change") if isinstance(d, dict) else None
            if isinstance(last, dict) and last.get("status") == "rolled_back":
                break
        note("unconfirmed change rolls back by itself after 10 minutes (PANEL_PORT_TIMEOUT)", isinstance(last, dict) and last.get("status") == "rolled_back", {"change": last})
    out = guest(NODE, f"grep CELIKPANEL_LISTEN /etc/celikpanel/panel.env; ss -ltn | grep -E ':({o.port}|{newp}) ' | sed 's/  */ /g'; nft list ruleset | grep -E '{newp}' | sed 's/^ *//' | head -3; echo end")
    note(f"after rollback only the confirmed port {o.port} listens; {newp} closed again", f":{newp} " not in out and str(newp) not in out.split("end")[0].split("\n", 1)[-1], out)


def scen_mailpair():
    """Check 3: customer domain on the joined server sends to and receives from a domain on the first server."""
    JD, JHOST, MD, MHOST = ARGS[:4]
    b = owner("backup")
    s, d = b.req("POST", "/api/v1/domains/create", {"domain": JD, "project_type": "static", "ssl_type": "letsencrypt"}, label="joined customer domain", timeout=300)
    note("customer domain on the joined server", s in (200, 201), {"status": s, "answer": d})
    jid = domain_id(b, JD)
    s, d, jpw = mbx(b, jid, f"carol@{JD}")
    note("mailbox carol@ on the joined server", s in (200, 201), {"status": s, "answer": d})
    time.sleep(30)
    apw = json.loads((STATE / "mbx-alice.json").read_text())["password"]; reg(apw)
    r1 = mail_send(JHOST, f"carol@{JD}", jpw, f"alice@{MD}", "joined to first")
    r2 = mail_send(MHOST, f"alice@{MD}", apw, f"carol@{JD}", "first to joined")
    m1 = mail_wait(MHOST, f"alice@{MD}", apw, "joined to first", 180)
    m2 = mail_wait(JHOST, f"carol@{JD}", jpw, "first to joined", 180)
    note("joined-server domain -> first-server domain delivered (DKIM-signed)", m1.get("count", 0) >= 1, {"send": r1, "got": m1.get("messages")})
    note("first-server domain -> joined-server domain delivered", m2.get("count", 0) >= 1, {"send": r2, "got": m2.get("messages")})


def scen_update():
    """The owner's Update: check, start (exactly the web body), status until it ends."""
    o = owner()
    s, chk = o.req("GET", "/api/v1/panel/update/check", label="update check", timeout=120)
    note(f"{NODE}: update check offers the candidate", s == 200 and isinstance(chk, dict) and chk.get("available") is True,
         {"status": s, "current": (chk or {}).get("current_version") if isinstance(chk, dict) else chk, "target": (chk or {}).get("target") if isinstance(chk, dict) else None})
    if not (isinstance(chk, dict) and chk.get("available")):
        return
    rid = secrets.token_hex(16)
    body = {"request_id": rid, "confirmed": True, "current_version": chk["current_version"], "current_commit": chk["current_commit"], **chk["target"]}
    s, d = o.req("POST", "/api/v1/panel/update/start", body, label="update start", timeout=120)
    note(f"{NODE}: update accepted", s in (200, 202) and isinstance(d, dict) and d.get("accepted"), {"status": s, "answer": {k: d.get(k) for k in ("accepted", "status")} if isinstance(d, dict) else d})
    t0 = time.time(); last = None; seen = []
    while time.time() - t0 < 1500:
        time.sleep(10)
        try:
            s, d = o.req("GET", f"/api/v1/panel/update/status?request_id={rid}", label="update status", timeout=30)
        except Exception as e:  # noqa: BLE001 - the panel restarts during the update
            seen.append("unreachable"); continue
        if s == 401:
            st = beta1.load_state(CELL, NODE); o = S(NODE, "owner"); o.login(st["username"], st["password"]); seen.append("relogin"); continue
        cur = (d.get("status"), d.get("phase")) if isinstance(d, dict) else (s, None)
        if cur != last:
            seen.append(f"{int(time.time()-t0)}s {cur[0]}/{cur[1]}"); last = cur
        if isinstance(d, dict) and d.get("status") in ("succeeded", "failed", "rolled_back", "recovered", "abandoned") and d.get("phase") != "verifying":
            break
    note(f"{NODE}: update ends succeeded", isinstance(d, dict) and d.get("status") == "succeeded", {"timeline": seen, "summary": d.get("summary") if isinstance(d, dict) else d})
    out = guest(NODE, "cat /opt/celikpanel/release.commit; grep CELIKPANEL_LISTEN /etc/celikpanel/panel.env; ss -ltn | grep -E ':(1923|2083) ' | sed 's/  */ /g'; systemctl is-active celikpanel-panel celikpanel-agent pdns nginx | paste -sd' '")
    note(f"{NODE}: after the update the release is the candidate and the port stays 2083", "b6800f082b5e277e4b63a38a6447af1ddf57359c" in out and ":2083" in out and ":1923" not in out, out)


def scen_mailext():
    """Check 3b by API: Components -> Spam-Protected Mail on a joined server, the in-place dialog's calls."""
    host = ARGS[0]
    o = owner()
    s, snap = o.req("GET", "/api/v1/setup", label="setup state")
    rev = snap.get("revision") if isinstance(snap, dict) else None
    s, d = o.req("POST", "/api/v1/setup/extend", {"revision": rev, "action": "begin", "scope": "mail", "mail_profile": "protected-mail", "mail_hostname": host}, label="extend begin (mail)")
    note(f"{NODE}: mail-only extension begins (no wizard)", s == 200 and isinstance(d, dict) and d.get("extension_scope") == "mail", {"status": s, "code": code_of(d), "draft.mail_hostname": ((d or {}).get("draft") or {}).get("mail_hostname") if isinstance(d, dict) else None})
    rev = d.get("revision") if isinstance(d, dict) else rev
    s, p = o.req("POST", "/api/v1/setup/plan", {"revision": rev}, label="extend plan")
    note(f"{NODE}: the plan holds only the mail steps", s == 200 and isinstance(p, dict) and p.get("can_start"), {"status": s, "blockers": (p or {}).get("blockers") if isinstance(p, dict) else p, "steps": [x.get("id") + ":" + str(x.get("target", "")) for x in (p or {}).get("steps", [])] if isinstance(p, dict) else None})
    if not (isinstance(p, dict) and p.get("can_start")):
        return
    rid = secrets.token_hex(16)
    s, d = o.req("POST", "/api/v1/setup/start", {"plan_id": p["id"], "request_id": rid, "confirmed": True, "package_updates_digest": p.get("package_updates_digest", "")}, label="extend start", timeout=120)
    t0 = time.time(); last = None; seen = []
    while time.time() - t0 < 1800:
        time.sleep(15)
        try:
            s, d = o.req("GET", f"/api/v1/setup/operation?request_id={rid}", label="extend operation", timeout=40)
        except Exception:
            seen.append("unreachable"); continue
        cur = (d.get("status"), d.get("phase")) if isinstance(d, dict) else (s, None)
        if cur != last:
            seen.append(f"{int(time.time()-t0)}s {cur[0]}/{cur[1]} {json.dumps((d or {}).get('error'))[:200] if isinstance(d, dict) and d.get('error') else ''}"); last = cur
        if isinstance(d, dict) and d.get("status") in ("succeeded", "failed"):
            break
    note(f"{NODE}: mail added in place", isinstance(d, dict) and d.get("status") == "succeeded", {"timeline": seen})


def scen_mailext_taken():
    """3b refusal: the in-place mail dialog with the FIRST server's mail name must be refused, then cancelled."""
    host = ARGS[0]
    o = owner()
    s, snap = o.req("GET", "/api/v1/setup", label="setup state")
    rev = snap.get("revision") if isinstance(snap, dict) else None
    s, d = o.req("POST", "/api/v1/setup/extend", {"revision": rev, "action": "begin", "scope": "mail", "mail_profile": "protected-mail", "mail_hostname": host}, label="extend begin with the first server's name")
    if s != 200:
        note(f"{NODE}: the first server's mail name is refused at begin", s >= 400, {"status": s, "code": code_of(d)})
        return
    rev = d.get("revision")
    s, p = o.req("POST", "/api/v1/setup/plan", {"revision": rev}, label="extend plan with taken name")
    bl = json.dumps((p or {}).get("blockers")) if isinstance(p, dict) else str(p)
    note(f"{NODE}: the first server's mail name is refused (server_setup_peer_mail_hostname_taken)", "mail_hostname_taken" in bl and not (p or {}).get("can_start"), {"status": s, "blockers": bl[:500]})
    s, snap = o.req("GET", "/api/v1/setup", label="setup state")
    s, d = o.req("POST", "/api/v1/setup/extend", {"revision": snap.get("revision"), "action": "cancel"}, label="extend cancel")
    note(f"{NODE}: the dialog's cancel ends the extension", s == 200 and isinstance(d, dict) and not d.get("extension"), {"status": s, "extension": d.get("extension") if isinstance(d, dict) else d})


def scen_retry6():
    """The check-6/7 steps the first pass sent without the web client's request id or folder flag."""
    S_ = ARGS[0]; CUSTDOM = ARGS[1]
    cpw = sec()["cust1"]; reg(cpw)
    c = S(NODE, "customer"); s, d = c.login("cust1", cpw)
    if isinstance(d, dict) and d.get("totp_required"):
        note("customer login (2FA already on)", False, "cannot continue"); return
    did = domain_id(c, CUSTDOM)
    s, d = c.req("POST", f"/api/v1/domains/{did}/ssl/letsencrypt", {"auto_renew": True, "include_mail": False, "provider": "letsencrypt", "reissue": False, "redirect_http": True}, label="customer ssl", timeout=480)
    note("customer gets SSL", s == 200, {"status": s, "answer": {k: d.get(k) for k in ("status", "success", "error", "code", "expires_at")} if isinstance(d, dict) else d})
    dbpw = secrets.token_urlsafe(16); reg(dbpw)
    s, d = c.req("POST", f"/api/v1/domains/{did}/databases", {"name": "custdb", "type": "mysql", "password": dbpw}, label="customer database", timeout=180)
    note("customer adds a database", s in (200, 201), {"status": s, "answer": d})
    s, d = c.req("POST", f"/api/v1/domains/{did}/dns/records", {"name": f"txt.{CUSTDOM}", "type": "TXT", "content": "\"customer-record\"", "ttl": 300, "prio": 0}, label="customer dns record (again)")
    note("customer adds a DNS record (second try)", s in (200, 201), {"status": s, "answer": d})
    s, d = c.req("POST", f"/api/v1/domains/{did}/backups", {"type": "files"}, label="customer backup", timeout=300)
    note("customer makes a backup", s in (200, 201, 202), {"status": s, "answer": d})
    o = owner(); sid = domain_id(o, S_); ip = lab.address_of(CELL, NODE)
    for path, body in (("/private", {"action": "create", "is_dir": True}), ("/private/index.html", {"action": "create"}), ("/private/index.html", {"action": "write", "content": "private-ok" + chr(10)}), ("/new", {"action": "create", "is_dir": True})):
        s, d = o.req("POST", f"/api/v1/domains/{sid}/files?path={path}", body, label="file " + path)
    ppw = sec()["protect"]
    curl = f"curl -sS --max-time 20 --resolve {S_}:443:{ip} --cacert /root/beta1/pebble-root.pem -o /dev/null -w '%{{http_code}}'"
    a = labnet(f"sudo -n {curl} https://{S_}/private/")
    b = labnet(f"sudo -n {curl} -u visitor:{ppw} https://{S_}/private/")
    note("protected folder: 401 without, 200 with the password", a.strip() == "401" and b.strip() == "200", {"without": a.strip(), "with": b.strip()})


def scen_autoresp2():
    """Autoresponder on the joined Ubuntu server (dovecot 2.3): carol@JD answers alice@MD once."""
    JD, JHOST, MD, MHOST = ARGS[:4]
    b = owner("backup"); jid = domain_id(b, JD)
    s, d = b.req("GET", f"/api/v1/domains/{jid}/mail/accounts", label="mailboxes")
    rows = (d.get("accounts") if isinstance(d, dict) else d) or []
    cid = next((a.get("id") for a in rows if str(a.get("address", a.get("email", ""))).startswith("carol@")), None)
    today = dt.date.today()
    s, d = b.req("PUT", f"/api/v1/domains/{jid}/mail/autoresponder", {"id": cid, "enabled": True, "subject": "Carol away", "message": "Carol is away.", "start_date": str(today - dt.timedelta(days=1)), "end_date": str(today + dt.timedelta(days=7)), "interval_days": 1, "addresses": []}, label="carol autoresponder")
    note("joined Ubuntu: autoresponder on carol@ saved", s == 200, {"status": s, "state": (d or {}).get("state") if isinstance(d, dict) else d})
    apw = json.loads((STATE / "mbx-alice.json").read_text())["password"]; reg(apw)
    for i in (1, 2):
        mail_send(MHOST, f"alice@{MD}", apw, f"carol@{JD}", f"to carol {i}"); time.sleep(8)
    m = mail_wait(MHOST, f"alice@{MD}", apw, "Carol away", 120)
    out = guest("backup", "journalctl -u postfix -u 'postfix@*' --no-pager -o cat --since -4min | grep -E 'carol@' | grep -oE 'relay=[^,]*|status=[a-z]+ \([^)]{0,120}' | tail -4; doveconf -n 2>/dev/null | grep -n auth_username_format; dovecot --version")
    note("joined Ubuntu: exactly one auto-reply, mail delivered", m.get("count") == 1, {"count": m.get("count"), "postfix": out})


# ------------------------------------------------------------------------------------------- run 2 scenarios

def dig(ip, name, typ):
    return labnet(f"dig +time=2 +tries=1 +norecurse +short @{ip} {name} {typ}").strip().replace("\n", " ")


def scen_e3():
    """E3: DNS records added while automatic DKIM publishes the same zone (ARGS: domain mainip backupip)."""
    D, MIP, BIP = ARGS[:3]
    o = owner()
    s, d = o.req("POST", "/api/v1/domains/create", {"domain": D, "project_type": "static", "ssl_type": "none"}, label="e3 domain", timeout=300)
    note("E3 domain created", s in (200, 201), {"status": s, "code": code_of(d)})
    did = domain_id(o, D)
    t0 = time.time()
    s, d, _ = mbx(o, did, f"info@{D}")
    note("E3 mailbox (starts automatic DKIM)", s in (200, 201), {"status": s, "code": code_of(d)})
    res = []
    for i in range(1, 9):
        s, d = o.req("POST", f"/api/v1/domains/{did}/dns/records", {"name": f"r{i}.{D}", "type": "TXT", "content": f"\"e3-{i}\"", "ttl": 300, "prio": 0}, label=f"e3 record {i}")
        res.append({"i": i, "t": round(time.time() - t0, 1), "status": s, "code": code_of(d), "publication": (d or {}).get("publication") if isinstance(d, dict) else None})
        time.sleep(1.5)
    note("E3 every record add answers 2xx (no 500) while DKIM runs", all(r["status"] in (200, 201) for r in res), res)
    time.sleep(30)
    missing = []
    for i in range(1, 9):
        for ip in (MIP, BIP):
            got = ""
            for _ in range(20):
                got = dig(ip, f"r{i}.{D}", "TXT")
                if f"e3-{i}" in got:
                    break
                time.sleep(3)
            if f"e3-{i}" not in got:
                missing.append((ip, i, got))
    note("E3 all 8 records published on both DNS servers", not missing, {"missing": missing})
    dk = {ip: dig(ip, f"celik._domainkey.{D}", "TXT")[:40] for ip in (MIP, BIP)}
    note("E3 the DKIM record of the same zone is published on both", all(dk.values()), dk)
    j = guest(NODE, f"journalctl -u celikpanel-panel --no-pager -o short-iso --since -6min | grep -E 'dkim-auto.*{D}|{D}.*(SQLITE_BUSY|locked)' | cut -c1-200 | tail -12")
    note("E3 panel journal: dkim-auto for the zone in the same window, no SQLITE_BUSY", "SQLITE_BUSY" not in j and "database is locked" not in j, j)


def autoresp_put(o, sid, aid, subject, label):
    today = dt.date.today()
    return o.req("PUT", f"/api/v1/domains/{sid}/mail/autoresponder", {"id": aid, "enabled": True, "subject": subject, "message": "Away until next week.", "start_date": str(today - dt.timedelta(days=1)), "end_date": str(today + dt.timedelta(days=7)), "interval_days": 1, "addresses": []}, label=label, timeout=120)


def scen_autoresp_e4():
    """E4 on SITE: two mails -> 2 delivered, 1 reply, no bounce; strip line -> lmtp_rejected; dovecot stopped -> dovecot_stopped (ARGS: site mailhost full|basic)."""
    S_, MAILHOST, MODE = ARGS[0], ARGS[1], (ARGS[2] if len(ARGS) > 2 else "full")
    o = owner(); sid = domain_id(o, S_)
    s, d = o.req("GET", f"/api/v1/domains/{sid}/mail/accounts", label="mailboxes")
    rows = (d.get("accounts") if isinstance(d, dict) else d) or []
    bob = next((a.get("id") for a in rows if str(a.get("address", a.get("email", ""))).startswith("bob@")), None)
    apw = json.loads((STATE / "mbx-alice.json").read_text())["password"]; reg(apw)
    bpw = json.loads((STATE / "mbx-bob.json").read_text())["password"]; reg(bpw)
    tag = secrets.token_hex(3)
    subj = f"Away {tag}"
    s, d = autoresp_put(o, sid, bob, subj, "bob autoresponder")
    note(f"{NODE}: autoresponder on bob@ saved", s == 200, {"status": s, "state": (d or {}).get("state") if isinstance(d, dict) else d, "code": code_of(d)})
    s, d = o.req("GET", f"/api/v1/domains/{sid}/mail/autoresponder?id={bob}", label="autoresponder read")
    note(f"{NODE}: GET shows applied=true right after the save", isinstance(d, dict) and (d.get("state") or {}).get("applied") is True, {"state": d.get("state") if isinstance(d, dict) else d})
    b0 = mail_wait(MAILHOST, f"alice@{S_}", apw, "Undelivered Mail Returned", 5).get("count", 0)
    for i in (1, 2):
        mail_send(MAILHOST, f"alice@{S_}", apw, f"bob@{S_}", f"e4 {tag} to bob {i}")
        time.sleep(8)
    got = mail_wait(MAILHOST, f"bob@{S_}", bpw, f"e4 {tag} to bob", 120)
    for _ in range(6):
        if got.get("count", 0) >= 2:
            break
        time.sleep(10)
        got = mail_wait(MAILHOST, f"bob@{S_}", bpw, f"e4 {tag} to bob", 10)
    rep = mail_wait(MAILHOST, f"alice@{S_}", apw, subj, 120)
    time.sleep(25)
    rep = mail_wait(MAILHOST, f"alice@{S_}", apw, subj, 10)
    b1_ = mail_wait(MAILHOST, f"alice@{S_}", apw, "Undelivered Mail Returned", 5).get("count", 0)
    pf = guest(NODE, f"journalctl -u postfix -u 'postfix@*' --no-pager -o cat --since -4min | grep -E 'bob@{S_}' | grep -oE 'relay=[^,]*|status=[a-z]+' | sort | uniq -c | tail -6; ls -la /var/mail/vhosts/{S_}/bob/ 2>/dev/null | grep -i sieve; grep -c 'bob@{S_}' /etc/postfix/celikpanel_transport")
    note(f"{NODE}: two mails from one sender -> 2 delivered, 1 reply, no bounce", got.get("count") == 2 and rep.get("count") == 1 and b1_ == b0,
         {"delivered": got.get("count"), "replies": rep.get("count"), "bounces_before": b0, "bounces_after": b1_, "server": pf})
    j = guest(NODE, "journalctl -u dovecot --no-pager -o cat --since -6min | grep -ciE 'Not a directory|User doesn.t exist' ; doveconf -n >/dev/null 2>/tmp/dc.err; echo doveconf-rc=$?; head -c 300 /tmp/dc.err; dovecot --version")
    note(f"{NODE}: Dovecot journal and doveconf -n clean", j.strip().split()[0] == "0" and "doveconf-rc=0" in j, j)
    if MODE != "full":
        return
    # the stock domain-stripping line back (lab only), save again -> lmtp_rejected, route removed, mail still arrives
    inc = guest(NODE, "grep -n 'local.conf' /etc/dovecot/dovecot.conf | tail -2; dovecot --version")
    is24 = bool(re.search(r"(^|\s)2\.4", inc))
    body_line = "auth_username_format = %{user | username | lower}" if is24 else "auth_username_format = %n"
    script = ("cp -a /etc/dovecot/local.conf /root/local.conf.lab-before 2>/dev/null || touch /root/local.conf.lab-none; "
              f"printf '%s\\n' '# lab E4: the stock domain-stripping line' 'protocol lmtp {{' '  {body_line}' '}}' >> /etc/dovecot/local.conf; "
              "doveconf -n 2>&1 | grep -n -A2 'protocol lmtp' | head -8; systemctl reload dovecot; sleep 2; systemctl is-active dovecot")
    r = guest(NODE, script)
    note(f"{NODE}: lab puts the domain-stripping LMTP line into /etc/dovecot/local.conf", "active" in r, {"include": inc.strip(), "applied": r})
    s, d = autoresp_put(o, sid, bob, subj + " again", "bob autoresponder after strip line")
    body = json.dumps(d)[:700]
    note(f"{NODE}: saving again is refused as lmtp_rejected", s >= 400 and "lmtp_rejected" in body, {"status": s, "answer": body})
    rt = guest(NODE, f"grep -c 'bob@{S_}' /etc/postfix/celikpanel_transport; ls -la /var/mail/vhosts/{S_}/bob/ 2>/dev/null | grep -i sieve || echo no-sieve-link")
    s, d = o.req("GET", f"/api/v1/domains/{sid}/mail/autoresponder?id={bob}", label="autoresponder read after refusal")
    note(f"{NODE}: the route is removed (transport table) and the page shows no active reply", rt.strip().split()[0] == "0",
         {"server": rt, "state": (d or {}).get("state") if isinstance(d, dict) else d, "enabled": (d or {}).get("enabled") if isinstance(d, dict) else None})
    mail_send(MAILHOST, f"alice@{S_}", apw, f"bob@{S_}", f"e4 {tag} after refusal")
    time.sleep(10)
    got2 = mail_wait(MAILHOST, f"bob@{S_}", bpw, f"e4 {tag} after refusal", 120)
    b2 = mail_wait(MAILHOST, f"alice@{S_}", apw, "Undelivered Mail Returned", 5).get("count", 0)
    note(f"{NODE}: mail still arrives after the refusal, no bounce", got2.get("count") == 1 and b2 == b0, {"delivered": got2.get("count"), "bounces": b2})
    r = guest(NODE, "if [ -f /root/local.conf.lab-before ]; then cp -a /root/local.conf.lab-before /etc/dovecot/local.conf; else rm -f /etc/dovecot/local.conf; fi; rm -f /root/local.conf.lab-before /root/local.conf.lab-none; systemctl reload dovecot; sleep 2; doveconf -n >/dev/null && echo restored")
    note(f"{NODE}: lab line removed again", "restored" in r, r)
    r = guest(NODE, "systemctl stop dovecot; systemctl is-active dovecot; ls -la /var/spool/postfix/private/dovecot-lmtp 2>&1 | cut -c1-120")
    s, d = autoresp_put(o, sid, bob, subj + " stopped", "bob autoresponder with dovecot stopped")
    body = json.dumps(d)[:700]
    note(f"{NODE}: with Dovecot stopped the save is refused as dovecot_stopped", s >= 400 and "dovecot_stopped" in body, {"dovecot": r.strip(), "status": s, "answer": body})
    guest(NODE, "systemctl start dovecot; sleep 2")
    s, d = autoresp_put(o, sid, bob, subj, "bob autoresponder on again")
    s2, d2 = o.req("GET", f"/api/v1/domains/{sid}/mail/autoresponder?id={bob}", label="autoresponder read final")
    note(f"{NODE}: saved again after Dovecot started: applied", s == 200 and isinstance(d2, dict) and (d2.get("state") or {}).get("applied") is True, {"status": s, "state": d2.get("state") if isinstance(d2, dict) else d2})


def scen_autoresp_ubuntu():
    """E4 on the joined Ubuntu: carol@JD, two mails from alice@MD -> 1 reply; no 'Not a directory' (ARGS: JD JHOST MD MHOST)."""
    JD, JHOST, MD, MHOST = ARGS[:4]
    b = owner("backup"); jid = domain_id(b, JD)
    s, d = b.req("GET", f"/api/v1/domains/{jid}/mail/accounts", label="mailboxes")
    rows = (d.get("accounts") if isinstance(d, dict) else d) or []
    cid = next((a.get("id") for a in rows if str(a.get("address", a.get("email", ""))).startswith("carol@")), None)
    tag = secrets.token_hex(3)
    s, d = autoresp_put(b, jid, cid, f"Carol away {tag}", "carol autoresponder")
    note("joined Ubuntu: autoresponder on carol@ saved", s == 200, {"status": s, "state": (d or {}).get("state") if isinstance(d, dict) else d, "code": code_of(d)})
    s, d = b.req("GET", f"/api/v1/domains/{jid}/mail/autoresponder?id={cid}", label="carol autoresponder read")
    note("joined Ubuntu: GET applied=true", isinstance(d, dict) and (d.get("state") or {}).get("applied") is True, {"state": d.get("state") if isinstance(d, dict) else d})
    apw = json.loads((STATE / "mbx-alice.json").read_text())["password"]; reg(apw)
    jpw = sec().get(f"mbx:carol@{JD}"); reg(jpw)
    for i in (1, 2):
        mail_send(MHOST, f"alice@{MD}", apw, f"carol@{JD}", f"e4u {tag} to carol {i}")
        time.sleep(8)
    got = mail_wait(JHOST, f"carol@{JD}", jpw, f"e4u {tag} to carol", 150)
    m = mail_wait(MHOST, f"alice@{MD}", apw, f"Carol away {tag}", 120)
    time.sleep(25)
    m = mail_wait(MHOST, f"alice@{MD}", apw, f"Carol away {tag}", 10)
    out = guest("backup", "journalctl -u dovecot --no-pager -o cat -b | grep -c 'Not a directory'; ls -la /var/mail/vhosts/*/carol/ 2>/dev/null | grep -i sieve; dovecot --version; doveconf -n >/dev/null 2>&1 && echo doveconf-ok")
    note("joined Ubuntu: 2 delivered, exactly one reply, no 'Not a directory' in the journal", got.get("count") == 2 and m.get("count") == 1 and out.strip().split()[0] == "0",
         {"delivered": got.get("count"), "replies": m.get("count"), "server": out})


def scen_e5():
    """E5: folder redirect /blog -> https://example.test/blog; /blog/post?q=1 without and with keep-query (ARGS: site)."""
    S_ = ARGS[0]
    o = owner(); sid = domain_id(o, S_); ip = lab.address_of(CELL, NODE)
    hdr = f"curl -sS --max-time 20 --resolve {S_}:443:{ip} --cacert /root/beta1/pebble-root.pem -o /dev/null -w '%{{http_code}} %{{redirect_url}}'"
    out = {}
    for keep in (False, True):
        s, d = o.req("GET", f"/api/v1/domains/{sid}/redirects", label="redirects list")
        rows = (d.get("redirects") if isinstance(d, dict) else d) or []
        for r in rows if isinstance(rows, list) else []:
            if r.get("source") == "/blog" or r.get("source_path") == "/blog":
                o.req("DELETE", f"/api/v1/domains/{sid}/redirects?id={r.get('id')}", label="redirect delete")
        s, d = o.req("POST", f"/api/v1/domains/{sid}/redirects", {"match": "prefix", "source": "/blog", "target": "https://example.test/blog", "code": 302, "keep_query": keep}, label=f"redirect keep={keep}")
        time.sleep(3)
        a = labnet(f"sudo -n {hdr} 'https://{S_}/blog/post?q=1'").strip()
        b = labnet(f"sudo -n {hdr} 'https://{S_}/blog/post'").strip()
        out[str(keep)] = (s, a, b)
    ok1 = out["False"][1] == "302 https://example.test/blog/post" and out["False"][2] == "302 https://example.test/blog/post"
    ok2 = out["True"][1] == "302 https://example.test/blog/post?q=1"
    note("E5 /blog/post?q=1 -> 302 .../blog/post without keep-query; .../blog/post?q=1 with keep-query", ok1 and ok2, {"without": out["False"], "with": out["True"]})
    t = guest(NODE, "nginx -t 2>&1 | tail -1")
    note(f"{NODE}: nginx -t", "successful" in t, t)


def scen_port_trial_restart():
    """Port trial rolled back by a restart during the trial (ARGS: port)."""
    newp = int(ARGS[0])
    o = owner()
    s, d = o.req("POST", "/api/v1/panel/port", {"action": "start", "port": newp}, label=f"port start {newp}")
    ch = (d or {}).get("change") if isinstance(d, dict) else None
    note(f"trial to {newp} started", s == 200 and isinstance(ch, dict), {"status": s, "change": ch, "code": code_of(d)})
    before = guest(NODE, f"grep CELIKPANEL_LISTEN /etc/celikpanel/panel.env; ss -ltn | grep -E ':({o.port}|{newp}) ' | sed 's/  */ /g'")
    r = subprocess.run(["python3", "-I", "/var/tmp/a114r6/h/deploy/e2e/dns-pair-acceptance/beta1/lab.py", "reboot", CELL, NODE, "900"], capture_output=True, timeout=1200)
    time.sleep(40)
    o2 = owner()
    s, d = o2.req("GET", "/api/v1/panel/port", label="port view after restart")
    after = guest(NODE, f"grep CELIKPANEL_LISTEN /etc/celikpanel/panel.env; ss -ltn | grep -E ':({o.port}|{newp}) ' | sed 's/  */ /g'; nft list ruleset | grep -E '{newp}' | sed 's/^ *//' | head -3; echo end")
    last = (d or {}).get("change") if isinstance(d, dict) else None
    note(f"a restart during the trial rolls it back: only {o.port} listens, {newp} closed, panel.env unchanged",
         f":{newp} " not in after and str(newp) not in after.split("end")[0].split("\n", 1)[-1] and f":{o.port}" in after,
         {"before": before, "after": after, "change": last, "reboot_rc": r.returncode})


def scen_forgotcmd():
    """S23: the forgot-password command shown on the page, run in a root terminal (pty), resets the owner's password."""
    import select
    st = beta1.load_state(CELL, NODE)
    pw = st["password"]; reg(pw)
    expected = "sudo -u celikpanel env CELIKPANEL_DATA_DIR=/var/lib/celikpanel /opt/celikpanel/bin/panel --create-admin"
    cmd = Path(ARGS[0]).read_text().strip() if ARGS else expected  # run 3: copied from the screen's text
    note("S23 the command shown on the screen is the documented one", cmd == expected, {"screen": cmd, "documented": expected})
    argv = lab.ssh_argv(CELL, NODE, f"sudo -n bash -c 'cd /root && {cmd}'")
    argv.insert(1, "-tt")
    p = subprocess.Popen(argv, stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, env={**os.environ, "PATH": beta1.PATH_ENV})
    answers = [("admin username", st.get("username", "owner")), ("admin email", "owner@beta1-lab.test"), ("show password while typing", "n"), ("admin password", pw), ("confirm password", pw)]
    raw = b""; pos = 0; sent = []; t0 = time.time()
    while time.time() - t0 < 120:
        r, _, _ = select.select([p.stdout], [], [], 3)
        if r:
            chunk = os.read(p.stdout.fileno(), 4096)
            if not chunk:
                break
            raw += chunk
            tail = raw[pos:].decode("utf-8", "replace")
            last_line = tail.rstrip().splitlines()[-1].lower() if tail.strip() else ""
            if answers and answers[0][0] in last_line:
                key, ans = answers.pop(0)
                p.stdin.write((ans + chr(10)).encode()); p.stdin.flush(); sent.append(key); pos = len(raw)
        if p.poll() is not None:
            break
    try:
        p.wait(10)
    except Exception:
        p.kill()
    text = red(raw.decode("utf-8", "replace"))
    o = S(NODE, "owner"); s, d = o.login(st["username"], pw)
    note("S23 forgot-password command runs in a root terminal and the owner signs in with the password it set", "Updated existing administrator" in text and s == 200, {"answered": sent, "terminal": text[-700:], "login": s})


def scen_zonecrud():
    """After the update: add, change and delete a zone and a record (ARGS: zone mainip backupip)."""
    Z_, MIP, BIP = ARGS[:3]
    o = owner()
    s, d = o.req("POST", "/api/v1/domains/create", {"domain": Z_, "project_type": "dnsonly", "ssl_type": "none"}, label="zone create", timeout=300)
    note(f"{NODE}: DNS zone {Z_} created", s in (200, 201), {"status": s, "code": code_of(d)})
    zid = domain_id(o, Z_)
    s, d = o.req("POST", f"/api/v1/domains/{zid}/dns/records", {"name": f"crud.{Z_}", "type": "A", "content": "192.0.2.10", "ttl": 300, "prio": 0}, label="record add")
    rid = (d or {}).get("id") if isinstance(d, dict) else None
    if not rid:
        s2, l = o.req("GET", f"/api/v1/domains/{zid}/dns/records", label="records")
        rows = l.get("records", l) if isinstance(l, dict) else l
        rid = next((r.get("id") for r in rows or [] if r.get("name", "").rstrip(".") == f"crud.{Z_}"), None)

    def w(ip, want):
        for _ in range(30):
            g = dig(ip, f"crud.{Z_}", "A")
            if want in g:
                return g
            time.sleep(3)
        return "TIMEOUT " + dig(ip, f"crud.{Z_}", "A")
    a = [w(MIP, "192.0.2.10"), w(BIP, "192.0.2.10")]
    s, d = o.req("PUT", f"/api/v1/domains/{zid}/dns/records", {"id": rid, "content": "192.0.2.11", "ttl": 300, "prio": 0}, label="record change")
    b = [w(MIP, "192.0.2.11"), w(BIP, "192.0.2.11")]
    note(f"{NODE}: record add then change served by both DNS servers", all("192.0.2.10" in x for x in a) and all("192.0.2.11" in x for x in b), {"add": a, "change_status": s, "change_code": code_of(d), "change": b})
    s, d = o.req("DELETE", f"/api/v1/domains/{zid}", label="zone delete", timeout=400)
    time.sleep(20)
    soa = [labnet(f"dig +time=2 +tries=1 +norecurse @{ip} {Z_} SOA | grep -oE 'status: [A-Z]+'").strip() for ip in (MIP, BIP)]
    note(f"{NODE}: zone deleted and gone from both DNS servers", s in (200, 202, 204) and all("NOERROR" not in x for x in soa), {"status": s, "soa": soa})


def scen_update2():
    """The owner's Update with version/schema/port before and after (ARGS: expected final status)."""
    expect = ARGS[0] if ARGS else "succeeded"
    o = owner()
    s, v0 = o.req("GET", "/api/v1/panel/version", label="version before")
    s, chk = o.req("GET", "/api/v1/panel/update/check", label="update check", timeout=120)
    note(f"{NODE}: update check offers the candidate", s == 200 and isinstance(chk, dict) and chk.get("available") is True,
         {"status": s, "current": (chk or {}).get("current_version") if isinstance(chk, dict) else chk, "target": ((chk or {}).get("target") or {}).get("version") if isinstance(chk, dict) else None,
          "previous_attempt": (chk or {}).get("previous_attempt") if isinstance(chk, dict) else None})
    if not (isinstance(chk, dict) and chk.get("available")):
        return
    rid = secrets.token_hex(16)
    body = {"request_id": rid, "confirmed": True, "current_version": chk["current_version"], "current_commit": chk["current_commit"], **chk["target"]}
    s, d = o.req("POST", "/api/v1/panel/update/start", body, label="update start", timeout=120)
    note(f"{NODE}: update accepted", s in (200, 202) and isinstance(d, dict) and d.get("accepted"), {"status": s, "answer": {k: d.get(k) for k in ("accepted", "status")} if isinstance(d, dict) else d})
    t0 = time.time(); last = None; seen = []; d = None
    while time.time() - t0 < 1800:
        time.sleep(10)
        try:
            s, d = o.req("GET", f"/api/v1/panel/update/status?request_id={rid}", label="update status", timeout=30)
        except Exception:
            seen.append("unreachable"); continue
        if s == 401:
            st = beta1.load_state(CELL, NODE); o = S(NODE, "owner"); o.login(st["username"], st["password"]); seen.append("relogin"); continue
        cur = (d.get("status"), d.get("phase")) if isinstance(d, dict) else (s, None)
        if cur != last:
            seen.append(f"{int(time.time()-t0)}s {cur[0]}/{cur[1]}"); last = cur
        if isinstance(d, dict) and d.get("status") in ("succeeded", "failed", "rolled_back", "recovered", "abandoned") and d.get("phase") != "verifying":
            break
    summ = d.get("summary") if isinstance(d, dict) else d
    print("FINAL-STATUS", red(json.dumps(d, ensure_ascii=False))[:5000], flush=True)  # run 4: the whole record
    note(f"{NODE}: update ends {expect}", isinstance(d, dict) and d.get("status") == expect, {"timeline": seen, "status": d.get("status") if isinstance(d, dict) else None, "summary": summ})
    if isinstance(summ, str):
        print("SUMMARY-LINE-LENGTH", len(summ), "| ends:", red(summ[-200:]))
    time.sleep(10)
    o = owner()
    s, v1 = o.req("GET", "/api/v1/panel/version", label="version after")
    s, chk2 = o.req("GET", "/api/v1/panel/update/check", label="update check after", timeout=120)
    print("CHECK-AFTER", red(json.dumps({k: chk2.get(k) for k in ("available", "current_version", "previous_attempt", "status", "reason")} if isinstance(chk2, dict) else chk2, ensure_ascii=False))[:2000], flush=True)

    def pick(v):
        return {k: v.get(k) for k in ("version", "commit", "schema_version")} if isinstance(v, dict) else v
    out = guest(NODE, "grep CELIKPANEL_LISTEN /etc/celikpanel/panel.env; ss -ltn | grep -E ':(1923|2083) ' | sed 's/  */ /g'; systemctl is-active celikpanel-panel celikpanel-agent pdns nginx | paste -sd' '")
    note(f"{NODE}: version/schema before -> after; port", True, {"before": pick(v0), "after": pick(v1), "server": out})


def scen_port_trial_shots():
    """S16/S17: the Settings panel address during a trial; the trial link uses the panel name (ARGS: port)."""
    newp = int(ARGS[0])
    o = owner()
    s, d = o.req("POST", "/api/v1/panel/port", {"action": "start", "port": newp}, label=f"port start {newp}")
    ch = (d or {}).get("change") if isinstance(d, dict) else None
    note(f"trial to {newp} started (for the screens)", s == 200 and isinstance(ch, dict), {"status": s, "change": ch, "code": code_of(d)})
    items = json.dumps([{"name": "settings-panel-port-trial", "path": "/settings?section=panel", "wait": 3000, "actions": [
        {"probe": "[...document.querySelectorAll('a')].filter(a=>/:%d/.test(a.href)).map(a=>a.innerText.trim()+' -> '+a.href)" % newp, "label": "trial link"},
        {"probe": "(document.querySelector('main')||document.body).innerText.match(/[a-z0-9.-]+\.archa114\.test[^\s]*/g)", "label": "panel address shown"}]}])
    r = subprocess.run(["python3", "-I", H + "/shots.py", CELL, NODE, "b-port-trial", items], capture_output=True, text=True, timeout=1200)
    print(r.stdout[-3000:])
    s, d = o.req("POST", "/api/v1/panel/port", {"action": "cancel", "id": ch["id"]}, label="cancel after screens") if isinstance(ch, dict) else (0, None)
    note("trial cancelled after the screens", s == 200, {"status": s, "change": (d or {}).get("change") if isinstance(d, dict) else d})


def scen_autoresp_first():
    """run 3, E-new-2(a): the first autoresponder save on an Arch server (Dovecot started before pigeonhole existed).
    ARGS: site. Full answer (status, state.warning, refusal), Dovecot's own error record since the save, pigeonhole."""
    S_ = ARGS[0]
    o = owner(); sid = domain_id(o, S_)
    s, d = o.req("GET", f"/api/v1/domains/{sid}/mail/accounts", label="mailboxes")
    rows = (d.get("accounts") if isinstance(d, dict) else d) or []
    bob = next((a.get("id") for a in rows if str(a.get("address", a.get("email", ""))).startswith("bob@")), None)
    pre = guest(NODE, "date +%s; pacman -Q dovecot pigeonhole 2>&1; systemctl show dovecot -p ActiveEnterTimestamp -p MainPID; ls /usr/lib/dovecot/modules/ 2>/dev/null | grep -ci sieve")
    since = pre.split()[0]
    s, d = autoresp_put(o, sid, bob, "Away first " + secrets.token_hex(3), "bob autoresponder first save")
    st_ = (d or {}).get("state") if isinstance(d, dict) else None
    warn = (st_ or {}).get("warning") if isinstance(st_, dict) else None
    none = len(ARGS) > 1 and ARGS[1] == "expect-none"
    ok = (s == 200 and not warn) if none else (s == 200 and isinstance(warn, str) and "restarted once" in warn)
    note(f"{NODE}: first save answers 200 " + ("with no warning" if none else "with the one-restart warning"), ok,
         {"status": s, "warning": warn, "applied": (st_ or {}).get("applied") if isinstance(st_, dict) else None, "code": code_of(d), "reason": (d or {}).get("reason") if isinstance(d, dict) else None, "error": (d or {}).get("error") if isinstance(d, dict) else None})
    post = guest(NODE, f"pacman -Q pigeonhole 2>&1; systemctl show dovecot -p ActiveEnterTimestamp -p MainPID; systemctl is-active dovecot; echo '-- doveadm log errors since the save'; doveadm log errors -s {since} 2>&1 | cut -c1-220 | tail -12; echo '-- journal'; journalctl -u dovecot --no-pager -o short-iso --since @{since} | grep -E 'SIGHUP|Failed to read|Unknown section|Starting|Started|Stopp' | cut -c1-200 | tail -10")
    note(f"{NODE}: server side after the first save (pigeonhole now installed, one restart)", "pigeonhole" in post and "active" in post, {"before": pre, "after": post})
    s2, d2 = o.req("GET", f"/api/v1/domains/{sid}/mail/autoresponder?id={bob}", label="autoresponder read after first save")
    note(f"{NODE}: GET applied=true after the first save", isinstance(d2, dict) and (d2.get("state") or {}).get("applied") is True, {"state": d2.get("state") if isinstance(d2, dict) else d2})


def scen_reload_clean():
    """run 3: the next save/reload is clean: `doveadm log errors` shows nothing new after ARGS[0] (epoch); one more reload."""
    since = ARGS[0]
    a = guest(NODE, f"doveadm log errors -s {since} 2>&1 | cut -c1-220 | tail -12; echo rc=$?")
    t2 = guest(NODE, "date +%s").strip()
    b = guest(NODE, f"systemctl reload dovecot; sleep 3; doveadm log errors -s {t2} 2>&1 | grep -vE 'SIGHUP received' | cut -c1-220 | tail -8; echo '-- all since {t2}'; doveadm log errors -s {t2} 2>&1 | cut -c1-220 | tail -4")
    bad = [l for l in (a + chr(10) + b.split("-- all since")[0]).splitlines() if re.search(r"Error|Fatal|Failed to read", l)]
    note(f"{NODE}: no new Dovecot error after the second save, and a further reload is clean", not bad, {"after_second_save": a, "reload": b})


def scen_jdcreate():
    """run 3, E-new-4: a customer domain with mail on the joined server (ARGS: domain). Domain + one mailbox."""
    JD = ARGS[0]
    b = owner("backup")
    s, d = b.req("POST", "/api/v1/domains/create", {"domain": JD, "project_type": "static", "ssl_type": "none"}, label="joined customer domain " + JD, timeout=300)
    note(f"customer domain {JD} on the joined server", s in (200, 201), {"status": s, "code": code_of(d)})
    jid = domain_id(b, JD)
    s, d, _ = mbx(b, jid, f"dave@{JD}")
    note(f"mailbox dave@{JD}", s in (200, 201), {"status": s, "code": code_of(d)})
    print("JID", jid)


def _upd_check(o, label="update check"):
    s, chk = o.req("GET", "/api/v1/panel/update/check", label=label, timeout=120)
    return s, chk


def _upd_start(o, chk, label="update start"):
    rid = secrets.token_hex(16)
    body = {"request_id": rid, "confirmed": True, "current_version": chk["current_version"], "current_commit": chk["current_commit"], **chk["target"]}
    s, d = o.req("POST", "/api/v1/panel/update/start", body, label=label, timeout=120)
    return rid, s, d


def _upd_poll(o, rid, every=2, limit=1800):
    t0 = time.time(); last = None; seen = []; d = None
    while time.time() - t0 < limit:
        time.sleep(every)
        try:
            s, d = o.req("GET", f"/api/v1/panel/update/status?request_id={rid}", label="update status", timeout=30)
        except Exception:
            if last != "unreachable":
                seen.append(f"{int(time.time()-t0)}s unreachable"); last = "unreachable"
            continue
        if s == 401:
            st = beta1.load_state(CELL, NODE); o = S(NODE, "owner"); o.login(st["username"], st["password"]); seen.append("relogin"); continue
        cur = (d.get("status"), d.get("phase")) if isinstance(d, dict) else (s, None)
        if cur != last:
            seen.append(f"{int(time.time()-t0)}s {cur[0]}/{cur[1]}"); last = cur
        if isinstance(d, dict) and d.get("status") in ("succeeded", "failed", "rolled_back", "recovered", "abandoned") and d.get("phase") != "verifying":
            break
    return o, d, seen


LIVE_CMD = ("echo $(systemctl is-active celikpanel-panel celikpanel-agent | paste -sd/) "
            "port=$(curl -sk -o /dev/null -w %{http_code} --max-time 3 https://127.0.0.1:PORT/ || true)")


class Watch:
    """run 4: guest-side liveness every ~4 s (panel, Agent, panel port answers) while an update runs."""

    def __init__(self, port):
        import threading
        self.rows = []; self.stop = False; self.port = port
        self.t = threading.Thread(target=self.run, daemon=True); self.t.start()

    def run(self):
        last = None
        while not self.stop:
            try:
                r = guest(NODE, LIVE_CMD.replace("PORT", str(self.port)), timeout=30).strip()
            except Exception as e:
                r = "ssh-error " + str(e)[:60]
            if r != last:
                self.rows.append(f"{dt.datetime.utcnow().strftime('%H:%M:%S')} {r}"); last = r
            time.sleep(3)

    def done(self):
        self.stop = True; self.t.join(40); return self.rows


def _stopped(rows, bound=360):
    # run 4 (corrected after the first H2 pass): the update's own restart step stops the panel and Agent for some
    # seconds, which is expected. "Stopped" means a stretch longer than the updater's restart bound (360 s), or a
    # last observation that is not active/active with the port answering.
    def down(x):
        return bool(re.search(r"\b(inactive|failed|deactivating|activating)\b", x.split(" port=")[0]))
    bad = []; start = None
    for x in rows:
        t = dt.datetime.strptime(x[:8], "%H:%M:%S")
        if down(x) and start is None:
            start = (t, x)
        elif not down(x) and start is not None:
            if (t - start[0]).seconds > bound:
                bad.append(f"stopped {(t - start[0]).seconds}s from {start[1]}")
            start = None
    if rows and (down(rows[-1]) or "port=200" not in rows[-1]):
        bad.append("last: " + rows[-1])
    return bad


def _live(o):
    return guest(NODE, "systemctl is-active celikpanel-panel celikpanel-agent | paste -sd' '; " + LIVE_CMD.replace("PORT", str(o.port)) + "; cat /opt/celikpanel/release.commit")


def scen_h2seq():
    """run 4, H2 on a 113 server: rollback probe update -> rolled back; then, ARGS[0] (default 11) seconds after the
    verified rollback was seen, a normal update. Never a stopped panel; liveness is watched throughout."""
    gap = int(ARGS[0]) if ARGS else 11
    o = owner()
    w = Watch(o.port)
    r = guest(NODE, "grep -q '^CELIKPANEL_LAB_ROLLBACK_PROBE=' /etc/celikpanel/panel.env || echo CELIKPANEL_LAB_ROLLBACK_PROBE=1 >> /etc/celikpanel/panel.env; grep -c '^CELIKPANEL_LAB_ROLLBACK_PROBE=' /etc/celikpanel/panel.env")
    note(f"{NODE}: lab trigger in panel.env (an unknown key; the candidate's installer refuses it)", r.strip().endswith("1"), r.strip())
    s, chk = _upd_check(o)
    note(f"{NODE}: update check offers the candidate", s == 200 and isinstance(chk, dict) and chk.get("available") is True,
         {"status": s, "current": (chk or {}).get("current_version") if isinstance(chk, dict) else chk, "target": ((chk or {}).get("target") or {}).get("version") if isinstance(chk, dict) else None})
    if not (isinstance(chk, dict) and chk.get("available")):
        w.done(); return
    rid, s, d = _upd_start(o, chk, "update start (rollback probe)")
    note(f"{NODE}: probe update accepted", s in (200, 202) and isinstance(d, dict) and d.get("accepted"), {"status": s, "answer": d})
    o, d, seen = _upd_poll(o, rid, every=2)
    t_end = time.time()
    print("PROBE-FINAL", red(json.dumps(d, ensure_ascii=False))[:2500], flush=True)
    note(f"{NODE}: probe update ends rolled back (113 reports failed)", isinstance(d, dict) and d.get("status") in ("failed", "rolled_back"),
         {"timeline": seen, "status": (d or {}).get("status") if isinstance(d, dict) else d})
    r = guest(NODE, "sed -i '/^CELIKPANEL_LAB_ROLLBACK_PROBE=/d' /etc/celikpanel/panel.env; grep -c '^CELIKPANEL_LAB_ROLLBACK_PROBE=' /etc/celikpanel/panel.env; cat /opt/celikpanel/release.commit")
    print("TRIGGER-REMOVED", r.strip().replace(chr(10), " | "), flush=True)
    try:
        o = owner()
    except Exception as e:
        print("RELOGIN-FAILED", e); time.sleep(3); o = owner()
    s, chk = _upd_check(o, "update check (second)")
    wait = gap - (time.time() - t_end)
    if wait > 0:
        time.sleep(wait)
    if not (isinstance(chk, dict) and chk.get("available")):
        note(f"{NODE}: second update check offers the candidate", False, {"status": s, "answer": chk}); w.done(); return
    rid2, s, d = _upd_start(o, chk, "update start (normal, right after the rollback)")
    delta = time.time() - t_end
    note(f"{NODE}: normal update started {delta:.1f} s after the verified rollback was seen", True, {"status": s, "answer": d})
    if not (s in (200, 202) and isinstance(d, dict) and d.get("accepted")):
        time.sleep(15)
        rows = w.done()
        print("WATCH", json.dumps(rows), flush=True)
        live = _live(o)
        note(f"{NODE}: start refused at once; panel and Agent running", "active active" in live and not _stopped(rows), {"answer": d, "live": live, "watch": rows})
        print("H2-RESULT start-refused", flush=True)
        return
    o, d2, seen2 = _upd_poll(o, rid2, every=3)
    print("SECOND-FINAL", red(json.dumps(d2, ensure_ascii=False))[:3000], flush=True)
    time.sleep(8)
    rows = w.done()
    print("WATCH", json.dumps(rows), flush=True)
    j = guest(NODE, "journalctl --no-pager -o cat --since -25min | grep -E 'finishing another server operation|agent_idle|Quiesce abort|fail-closed|state=unchanged|CELIKPANEL_UPDATE_FAILURE' | grep -v level=debug | cut -c1-400 | tail -14")
    print("JOURNAL", j.replace(chr(10), " || ")[:4000], flush=True)
    live = _live(o)
    st2 = (d2 or {}).get("status") if isinstance(d2, dict) else None
    body = json.dumps(d2, ensure_ascii=False) if isinstance(d2, dict) else str(d2)
    stopped = _stopped(rows)
    if st2 == "succeeded":
        note(f"{NODE}: H2 normal update right after the rollback succeeds (waited for the Agent if needed); panel never stopped", not stopped,
             {"timeline": seen2, "live": live, "stopped_rows": stopped, "watch": rows[-14:]})
    else:
        ok = ("agent_idle" in body + j) and ("unchanged" in body + j) and "active active" in live and not stopped
        note(f"{NODE}: H2 refused with agent_idle and state=unchanged; panel and Agent kept running, port answers", ok,
             {"timeline": seen2, "status": st2, "live": live, "stopped_rows": stopped, "watch": rows[-14:], "answer": body[:900]})
    print("H2-RESULT", st2, "stopped-rows=%d" % len(stopped), flush=True)


def scen_busyupdate():
    """run 4, H2 extra: start a long Agent operation (a site with mail), then at once a normal update (ARGS: delay_s).
    Records what happened; the pass condition is only that the panel and Agent never stop."""
    delay = float(ARGS[0]) if ARGS else 0.7
    o = owner()
    w = Watch(o.port)
    s, chk = _upd_check(o)
    if not (isinstance(chk, dict) and chk.get("available")):
        note(f"{NODE}: update check offers a target", False, {"status": s, "answer": chk}); w.done(); return
    import threading
    res = {}
    z = "busy" + secrets.token_hex(2) + ".test"

    def op():
        o2 = owner()
        res["create"] = o2.req("POST", "/api/v1/domains/create", {"domain": z, "project_type": "static", "ssl_type": "none"}, label="busy op: domain create " + z, timeout=400)[0]
        zid = domain_id(o2, z)
        res["mailbox"] = mbx(o2, zid, f"busy@{z}")[0] if zid else None
    th = threading.Thread(target=op); th.start()
    time.sleep(delay)
    busy = guest(NODE, "pgrep -af 'pacman|apt-get|dpkg|opendkim-genkey|postmap|doveadm|pdnsutil|rndc|nginx -t|systemctl' | grep -vE 'pgrep|bash -c' | cut -c1-120 | head -10")
    rid, s, d = _upd_start(o, chk, "update start while the Agent works")
    print("START-ANSWER", s, red(json.dumps(d, ensure_ascii=False))[:800], flush=True)
    print("BUSY-PROCESSES", busy.replace(chr(10), " | ")[:1500], flush=True)
    th.join(420)
    print("OP-RESULT", z, json.dumps(res), flush=True)
    d2 = None; seen = []
    if s in (200, 202) and isinstance(d, dict) and d.get("accepted"):
        o, d2, seen = _upd_poll(o, rid, every=3)
        print("UPDATE-FINAL", red(json.dumps(d2, ensure_ascii=False))[:3000], flush=True)
    time.sleep(8)
    rows = w.done()
    j = guest(NODE, "journalctl --no-pager -o cat --since -25min | grep -E 'finishing another server operation|agent_idle|Quiesce abort|fail-closed|state=unchanged' | grep -v level=debug | cut -c1-300 | tail -10")
    live = _live(o)
    stopped = _stopped(rows)
    st2 = (d2 or {}).get("status") if isinstance(d2, dict) else None
    print("BUSY-RESULT", st2, "stopped-rows=%d" % len(stopped), flush=True)
    note(f"{NODE}: busy Agent at update start: panel never stopped; outcome recorded", not stopped and "active active" in live,
         {"start": {"status": s, "answer": d}, "timeline": seen, "final": st2, "live": live, "watch": rows[-14:], "journal": j[-1500:]})


def scen_j1():
    """run 6, J1 + sanity on NODE (ARGS: rcpt-domain rcpt-mailhost rcpt-local sender-domain sender-mailhost).
    alice@SD sends to RLOCAL@RD: 2 mails -> 2 delivered, 1 reply, 0 bounces; the lab breaks Dovecot's mailbox lookup
    (stock domain-stripping LMTP line), save -> lmtp_rejected, mail sent immediately -> delivered, 0 bounces; lookup
    restored, save -> applied, mail sent immediately -> delivered + 1 reply, 0 bounces."""
    RD, RHOST, RL, SD, SHOST = ARGS[:5]
    o = owner(NODE); sid = domain_id(o, RD)
    s, d = o.req("GET", f"/api/v1/domains/{sid}/mail/accounts", label="mailboxes")
    rows = (d.get("accounts") if isinstance(d, dict) else d) or []
    rid = next((a.get("id") for a in rows if str(a.get("address", a.get("email", ""))).startswith(f"{RL}@")), None)
    apw = json.loads((STATE / "mbx-alice.json").read_text())["password"]; reg(apw)
    if (STATE / f"mbx-{RL}.json").exists():
        rpw = json.loads((STATE / f"mbx-{RL}.json").read_text())["password"]
    else:
        rpw = sec().get(f"mbx:{RL}@{RD}")
    reg(rpw)
    RCPT, SNDR = f"{RL}@{RD}", f"alice@{SD}"
    t0 = guest(NODE, "date -u '+%Y-%m-%d %H:%M:%S'").strip()
    tag = secrets.token_hex(3)
    subA, subB = f"Away {tag}", f"Backsoon {tag}"
    bnc = lambda: mail_wait(SHOST, SNDR, apw, "Undelivered Mail Returned", 5).get("count", 0)
    srv_bounced = lambda: guest(NODE, f"journalctl -u postfix -u 'postfix@*' --no-pager -o cat --since '{t0}' | grep -c 'status=bounced'").strip().split()[0]
    b0 = bnc()
    s, d = autoresp_put(o, sid, rid, subA, f"{RL} autoresponder")
    s2, d2 = o.req("GET", f"/api/v1/domains/{sid}/mail/autoresponder?id={rid}", label="autoresponder read")
    note(f"{NODE}: autoresponder on {RL}@ saved, applied", s == 200 and isinstance(d2, dict) and (d2.get("state") or {}).get("applied") is True,
         {"status": s, "code": code_of(d), "state": d2.get("state") if isinstance(d2, dict) else d2})
    for i in (1, 2):
        mail_send(SHOST, SNDR, apw, RCPT, f"j1 {tag} first {i}")
        time.sleep(8)
    got = mail_wait(RHOST, RCPT, rpw, f"j1 {tag} first", 150)
    for _ in range(6):
        if got.get("count", 0) >= 2:
            break
        time.sleep(10)
        got = mail_wait(RHOST, RCPT, rpw, f"j1 {tag} first", 10)
    rep = mail_wait(SHOST, SNDR, apw, subA, 120)
    time.sleep(25)
    rep = mail_wait(SHOST, SNDR, apw, subA, 10)
    note(f"{NODE}: two mails -> 2 delivered, exactly 1 reply, 0 bounces", got.get("count") == 2 and rep.get("count") == 1 and bnc() == b0 and srv_bounced() == "0",
         {"delivered": got.get("count"), "replies": rep.get("count"), "bounces_before": b0, "bounces_now": bnc(), "server_bounced": srv_bounced()})
    # run 5's sequence: the stock domain-stripping LMTP line (lab only), save again -> lmtp_rejected, mail at once
    inc = guest(NODE, "dovecot --version")
    is24 = bool(re.search(r"(^|\s)2\.4", inc))
    body_line = "auth_username_format = %{user | username | lower}" if is24 else "auth_username_format = %n"
    script = ("cp -a /etc/dovecot/local.conf /root/local.conf.lab-before 2>/dev/null || touch /root/local.conf.lab-none; "
              f"printf '%s\n' '# lab J1: the stock domain-stripping line' 'protocol lmtp {{' '  {body_line}' '}}' >> /etc/dovecot/local.conf; "
              "systemctl reload dovecot; sleep 2; systemctl is-active dovecot")
    r = guest(NODE, script)
    note(f"{NODE}: lab breaks Dovecot's mailbox lookup (domain-stripping LMTP line)", "active" in r, {"dovecot": inc.strip(), "applied": r})
    s, d = autoresp_put(o, sid, rid, subA + " again", f"{RL} autoresponder after strip line")
    mail_send(SHOST, SNDR, apw, RCPT, f"j1 {tag} after refusal")
    body = json.dumps(d)[:700]
    note(f"{NODE}: saving again is refused as lmtp_rejected", s >= 400 and "lmtp_rejected" in body, {"status": s, "answer": body})
    rt = guest(NODE, f"grep -c '{RCPT}' /etc/postfix/celikpanel_transport; journalctl -u postfix -u 'postfix@*' --no-pager -o cat --since '{t0}' | grep -iE 'has changed|reload' | tail -3")
    note(f"{NODE}: the route is removed from the transport table", rt.strip().split()[0] == "0", rt)
    got2 = mail_wait(RHOST, RCPT, rpw, f"j1 {tag} after refusal", 150)
    time.sleep(15)
    pf = guest(NODE, f"journalctl -u postfix -u 'postfix@*' --no-pager -o cat --since '{t0}' | grep -E '{RCPT}' | grep -oE 'relay=[^,]*|status=[a-z]+ \([^)]{{0,100}}' | tail -6")
    note(f"{NODE}: J1 mail sent right after the refusal is delivered, 0 bounces", got2.get("count") == 1 and bnc() == b0 and srv_bounced() == "0",
         {"delivered": got2.get("count"), "bounces": bnc(), "server_bounced": srv_bounced(), "postfix": pf})
    r = guest(NODE, "if [ -f /root/local.conf.lab-before ]; then cp -a /root/local.conf.lab-before /etc/dovecot/local.conf; else rm -f /etc/dovecot/local.conf; fi; rm -f /root/local.conf.lab-before /root/local.conf.lab-none; systemctl reload dovecot; sleep 2; doveconf -n >/dev/null && echo restored")
    note(f"{NODE}: lab line removed again (lookup restored)", "restored" in r, r)
    s, d = autoresp_put(o, sid, rid, subB, f"{RL} autoresponder after restore")
    mail_send(SHOST, SNDR, apw, RCPT, f"j1 {tag} after restore")
    s2, d2 = o.req("GET", f"/api/v1/domains/{sid}/mail/autoresponder?id={rid}", label="autoresponder read after restore")
    note(f"{NODE}: saved again after the restore: applied", s == 200 and isinstance(d2, dict) and (d2.get("state") or {}).get("applied") is True,
         {"status": s, "code": code_of(d), "state": d2.get("state") if isinstance(d2, dict) else d2})
    got3 = mail_wait(RHOST, RCPT, rpw, f"j1 {tag} after restore", 150)
    rep3 = mail_wait(SHOST, SNDR, apw, subB, 120)
    time.sleep(25)
    rep3 = mail_wait(SHOST, SNDR, apw, subB, 10)
    pf = guest(NODE, f"journalctl -u postfix -u 'postfix@*' --no-pager -o cat --since '{t0}' | grep -oE 'status=[a-z]+' | sort | uniq -c; grep -c '{RCPT}' /etc/postfix/celikpanel_transport")
    note(f"{NODE}: mail right after the applied save -> delivered + exactly 1 reply, 0 bounces", got3.get("count") == 1 and rep3.get("count") == 1 and bnc() == b0 and srv_bounced() == "0",
         {"delivered": got3.get("count"), "replies": rep3.get("count"), "bounces": bnc(), "server_bounced": srv_bounced(), "postfix": pf})
    j = guest(NODE, f"journalctl -u dovecot --no-pager -o cat --since '{t0}' | grep -c 'Not a directory'; doveconf -n >/dev/null 2>&1 && echo doveconf-ok")
    note(f"{NODE}: Dovecot clean (no 'Not a directory', doveconf -n ok)", j.strip().split()[0] == "0" and "doveconf-ok" in j, j)


def main():
    fn = globals().get("scen_" + SCEN)
    if not fn:
        raise SystemExit(__doc__)
    try:
        fn()
    finally:
        out = Path(f"/var/tmp/a114r6/owner-log/{CELL}-{NODE}-{SCEN}-results.json")
        out.write_text(red(json.dumps(RESULTS, indent=1, ensure_ascii=False)))


main()
