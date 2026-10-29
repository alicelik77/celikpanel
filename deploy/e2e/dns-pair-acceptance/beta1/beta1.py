#!/usr/bin/env python3
"""Beta-1 VM acceptance driver (test harness only, not a product change).

Drives fresh CelikPanel installs on the beta1 lab guests (lab.py) the way the
owner does: the customer installer from the dist archive, the first
administrator, the test-only acceptance licence, then the setup wizard v2
through the panel's own API exactly as ``web/src`` calls it. Every exchange is
redacted before it is written (``redaction.Redactor`` plus the join code,
link credentials and enrollment codes registered as they appear).

Phases are separate commands so each can be bounded and inspected:
  provision CELL NODE OS IP HOSTNAME [MEM]   fresh guest + lab trust (pebble root, certbot cli.ini)
  install CELL NODE                          stage dist, credentials, install.sh, postinstall checks
  login CELL NODE                            login + acceptance-fixture licence
  api CELL NODE METHOD PATH [BODY|@FILE] [--label L]
  poll CELL NODE PATH SECONDS EVERY [--until KEY=VALUE ...] [--label L]   read-only; never a mutation
  sh CELL NODE CMD                           root command on the guest, output recorded
  dbcounts CELL NODE [--label L]             read-only row counts of every panel table + audit actions
  leaf CELL NODE                             TLS leaf served on IP:2083 (read on the guest)
"""
from __future__ import annotations

import argparse
import datetime as dt
import hashlib
import json
import os
import re
import secrets
import subprocess
import sys
import time
from pathlib import Path

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE.parent))
sys.path.insert(0, str(HERE))
import install_steps  # noqa: E402
from panel_api import PanelClient, PinnedHTTPSTransport, Response, UnknownOutcome, PanelError  # noqa: E402
from redaction import Redactor  # noqa: E402
import lab  # noqa: E402

BASE = Path("/var/tmp/cp-beta1")
STATE = BASE / "state"
EVID = BASE / "evidence"
DIST = Path(os.environ.get("BETA1_DIST", "/var/tmp/cp-pair-accept/dist/68a9915a0e2387f85a900fb60dafe5a32fc94d81-acceptance-license/dist.json"))
PEBBLE_ROOT = BASE / "lab" / "pebble-root.pem"
ACCEPTANCE_FIXTURE_KEY = "CPK-acce57f1c7" + "0" * 54
JOIN_CODE_RE = re.compile(r"\b[A-Z2-7]{4}(?:-[A-Z2-7]{4}){5}\b|\b[A-Z2-7]{24}\b")
EXTRA_SECRET_KEYS = ("credential", "proof", "enrollment", "bearer")
PATH_ENV = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"


def now() -> str:
    return dt.datetime.now(dt.timezone.utc).strftime("%Y-%m-%dT%H:%M:%S.%fZ")[:-4] + "Z"


class LabRedactor(Redactor):
    """The harness redactor plus the setup link's secret-bearing keys."""

    def value(self, value):  # type: ignore[override]
        if isinstance(value, dict):
            out = {}
            for key, item in value.items():
                k = str(key).lower()
                if any(f in k for f in EXTRA_SECRET_KEYS) and not isinstance(item, (dict, list, bool)) and item not in (None, ""):
                    out[str(key)] = "[REDACTED]"
                else:
                    out[str(key)] = super().value({key: item})[str(key)]
            return out
        return super().value(value)

    def text(self, value: str) -> str:  # type: ignore[override]
        for code in JOIN_CODE_RE.findall(value):
            self.register(code)
            self.register(code.replace("-", ""))
        return super().text(value)


def state_path(cell: str, node: str) -> Path:
    return STATE / cell / f"{node}.json"


def load_state(cell: str, node: str) -> dict:
    p = state_path(cell, node)
    return json.loads(p.read_text()) if p.exists() else {}


def save_state(cell: str, node: str, st: dict) -> None:
    p = state_path(cell, node)
    p.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
    tmp = p.with_suffix(".tmp")
    tmp.write_text(json.dumps(st, indent=2))
    os.chmod(tmp, 0o600)
    os.replace(tmp, p)


def redactor_for(st: dict) -> LabRedactor:
    r = LabRedactor()
    for key in ("password", "cookie"):
        if st.get(key):
            r.register(st[key])
    for code in st.get("join_codes", []):
        r.register(code)
        r.register(code.replace("-", ""))
    return r


class Recorder:
    def __init__(self, cell: str, redactor: Redactor):
        self.dir = EVID / cell
        self.dir.mkdir(parents=True, exist_ok=True, mode=0o700)
        self.redactor = redactor

    def seq(self) -> int:
        p = self.dir / ".seq"
        n = int(p.read_text()) + 1 if p.exists() else 1
        p.write_text(str(n))
        return n

    def write(self, name: str, value) -> Path:
        n = self.seq()
        safe = re.sub(r"[^a-z0-9._-]+", "-", name.lower()).strip("-")[:90]
        path = self.dir / f"{n:04d}-{safe}.json"
        text = json.dumps(self.redactor.value(value), indent=2, sort_keys=True, ensure_ascii=False)
        text = self.redactor.text(text)
        if self.redactor.contains_secret(text):
            raise SystemExit("a registered secret survived redaction; nothing written")
        path.write_text(text + "\n")
        return path


def guest(cell: str, node: str, command: str, stdin: bytes | None = None, timeout: int = 600) -> subprocess.CompletedProcess:
    return subprocess.run(lab.ssh_argv(cell, node, command), input=stdin, capture_output=True, timeout=timeout,
                          env={**os.environ, "PATH": PATH_ENV})


def leaf(cell: str, node: str) -> str:
    ip = lab.address_of(cell, node)
    cmd = (f"sudo -n sh -c 'echo | openssl s_client -connect {ip}:2083 -noservername 2>/dev/null "
           "| openssl x509 -outform DER | sha256sum'")
    out = guest(cell, node, cmd, timeout=60).stdout.decode().split()
    if not out or len(out[0]) != 64:
        raise SystemExit("could not read the TLS leaf on the guest")
    return out[0]


def client_for(cell: str, node: str, rec: Recorder, st: dict) -> PanelClient:
    ip = lab.address_of(cell, node)
    pin = st.get("pin") or leaf(cell, node)
    st["pin"] = pin
    exchanges = []

    def record(exchange):
        method = exchange["request"]["method"]
        path = exchange["request"]["path"].split("?")[0]
        rec.write(f"{node}-{method}-{path.replace('/api/v1/', '')}", {**exchange, "at": now(), "label": os.environ.get("BETA1_LABEL", "")})
        exchanges.append(exchange)

    transport = PinnedHTTPSTransport(ip, 2083, pin)

    def watching(method, path, headers, body, timeout):
        resp = transport(method, path, headers, body, timeout)
        try:
            doc = json.loads(resp.body or b"null")
        except ValueError:
            doc = None
        if isinstance(doc, dict) and isinstance(doc.get("code"), str) and JOIN_CODE_RE.fullmatch(doc["code"]):
            rec.redactor.register(doc["code"])
            rec.redactor.register(doc["code"].replace("-", ""))
            st.setdefault("join_codes", []).append(doc["code"])
            save_state(cell, node, st)
            (STATE / cell / f"{node}.last-join-code").write_text(doc["code"])
            os.chmod(STATE / cell / f"{node}.last-join-code", 0o600)
        return resp

    c = PanelClient(node, f"https://{ip}:2083", watching, rec.redactor, record)
    if st.get("cookie"):
        c.cookie = st["cookie"]
    return c


def cmd_provision(a) -> dict:
    st = load_state(a.cell, a.node)
    if st:
        raise SystemExit("state exists; this node was provisioned already")
    t0 = time.monotonic()
    info = lab.start(a.cell, a.node, a.os, a.ip, a.hostname, memory=a.mem)
    ready = lab.wait_ssh(a.cell, a.node, 600)
    root = PEBBLE_ROOT.read_bytes()
    # Arch keeps local anchors in /etc/ca-certificates/trust-source/anchors (p11-kit's update-ca-trust).
    trust = ("cat > /etc/ca-certificates/trust-source/anchors/beta1-lab-pebble-root.crt; update-ca-trust; "
             if a.os == "arch" else
             "cat > /usr/local/share/ca-certificates/beta1-lab-pebble-root.crt; update-ca-certificates >/dev/null; ")
    prep = ("sudo -n sh -c 'set -e; " + trust + "install -d -m 0700 /root/.config/letsencrypt; "
            "printf \"%s\\n\" \"# beta1 lab only: certbot uses the lab ACME server (pebble)\" "
            "\"server = https://192.0.2.60:14000/dir\" \"no-verify-ssl = true\" > /root/.config/letsencrypt/cli.ini; "
            "cat /root/.config/letsencrypt/cli.ini; ip -4 route get 1.1.1.1; ip -4 route get 192.0.2.1; cat /etc/resolv.conf | grep -v ^#; "
            "cat /etc/os-release | grep -E \"^(PRETTY_NAME|VERSION_ID)=\"; uname -r; hostname'")
    r = guest(a.cell, a.node, prep, stdin=root, timeout=120)
    pre = guest(a.cell, a.node, install_steps.preinstall_check(), timeout=60)
    save_state(a.cell, a.node, {"provisioned_at": now(), "os": a.os, "ip": a.ip})
    rec = Recorder(a.cell, LabRedactor())
    out = {"node": info, "ssh": ready, "lab_trust_and_certbot": r.stdout.decode()[-3000:], "rc": r.returncode,
           "preinstall": pre.stdout.decode()[-500:], "preinstall_rc": pre.returncode, "seconds": round(time.monotonic() - t0, 1)}
    rec.write(f"{a.node}-provision", out)
    return out


def cmd_install(a) -> dict:
    st = load_state(a.cell, a.node)
    dist = json.loads(DIST.read_text())
    password = secrets.token_urlsafe(24)
    st["password"], st["username"] = password, "owner"
    save_state(a.cell, a.node, st)
    rec = Recorder(a.cell, redactor_for(st))
    t0 = time.monotonic()
    out: dict = {"dist": {k: dist[k] for k in ("sha256", "commit", "tree", "version", "license_mode")}}
    archive = Path(dist["archive"]).read_bytes()
    s = guest(a.cell, a.node, install_steps.stage_archive(dist["root"], dist["sha256"], dist["commit"], dist["tree"]),
              stdin=archive, timeout=600)
    out["stage"] = {"rc": s.returncode, "out": s.stdout.decode()[-800:], "err": s.stderr.decode()[-800:]}
    if s.returncode:
        rec.write(f"{a.node}-install", out)
        raise SystemExit(json.dumps(out, indent=1))
    c = guest(a.cell, a.node, install_steps.upload_credentials(),
              stdin=install_steps.credentials_document("owner", "owner@beta1-lab.test", password), timeout=60)
    out["credentials"] = {"rc": c.returncode, "out": c.stdout.decode()[-400:]}
    t1 = time.monotonic()
    i = guest(a.cell, a.node, install_steps.run_installer(dist["root"]), timeout=3600)
    text = rec.redactor.text(install_steps.ANSI_RE.sub("", i.stdout.decode("utf-8", "replace") + i.stderr.decode("utf-8", "replace")))
    (EVID / a.cell).mkdir(parents=True, exist_ok=True)
    (EVID / a.cell / f"install-sh-{a.node}.txt").write_text(text)
    out["installer"] = {"rc": i.returncode, "seconds": round(time.monotonic() - t1, 1), "tail": text[-2500:],
                        "restart_required": install_steps.RESTART_REQUIRED_BANNER in text,
                        "restart_recommended": install_steps.RESTART_RECOMMENDED_BANNER in text}
    p = guest(a.cell, a.node, install_steps.postinstall_check(), timeout=60)
    out["postinstall"] = {"rc": p.returncode, "out": p.stdout.decode()[-600:]}
    if out["installer"]["restart_required"]:
        out["owner_restart"] = lab.reboot(a.cell, a.node, 900)
        time.sleep(10)
        p2 = guest(a.cell, a.node, "systemctl is-active celikpanel-agent.service celikpanel-panel.service", timeout=60)
        out["after_restart_units"] = p2.stdout.decode()
    out["seconds"] = round(time.monotonic() - t0, 1)
    rec.write(f"{a.node}-install", out)
    return out


def cmd_login(a) -> dict:
    st = load_state(a.cell, a.node)
    st.pop("pin", None)
    rec = Recorder(a.cell, redactor_for(st))
    c = client_for(a.cell, a.node, rec, st)
    identity = c.login(st["username"], st["password"])
    st["cookie"] = c.cookie
    save_state(a.cell, a.node, st)
    out = {"identity_role": identity.get("role"), "pin": st["pin"]}
    lic = c.get("/api/v1/panel/license").json()
    out["license_before"] = {k: lic.get(k) for k in ("state", "license_kind", "acceptance_cell", "acceptance_node")} if isinstance(lic, dict) else lic
    if not (isinstance(lic, dict) and lic.get("state") == "active"):
        r = c.post("/api/v1/panel/license", {"action": "activate", "key": ACCEPTANCE_FIXTURE_KEY}, purpose="License screen activate (fixture)")
        out["activate_status"] = r.status
        lic = c.get("/api/v1/panel/license").json()
    out["license_after"] = {k: lic.get(k) for k in ("state", "license_kind", "license_label", "license_service", "acceptance_cell", "acceptance_node")} if isinstance(lic, dict) else lic
    out["access"] = c.get("/api/v1/license/access").json()
    return out


def body_arg(raw: str | None):
    if raw is None:
        return None
    if raw.startswith("@"):
        return json.loads(Path(raw[1:]).read_text())
    return json.loads(raw)


def short(resp: Response, limit: int, red: Redactor):
    doc = resp.json()
    text = json.dumps(red.value(doc), indent=1, sort_keys=True, ensure_ascii=False) if doc is not None else red.text(resp.text)
    return {"status": resp.status, "elapsed": round(resp.elapsed, 3), "body": text[:limit]}


def cmd_api(a) -> dict:
    st = load_state(a.cell, a.node)
    rec = Recorder(a.cell, redactor_for(st))
    os.environ["BETA1_LABEL"] = a.label or ""
    c = client_for(a.cell, a.node, rec, st)
    try:
        resp = c.request(a.method, a.path, body_arg(a.body), timeout=a.timeout, purpose=a.label)
    except UnknownOutcome as exc:
        return {"outcome": "unknown", "error": str(exc)}
    if resp.status == 401:
        return {"status": 401, "note": "session expired; run login again"}
    return short(resp, a.limit, rec.redactor)


def get_path(doc, dotted: str):
    cur = doc
    for part in dotted.split("."):
        if isinstance(cur, list):
            try:
                cur = cur[int(part)]
            except (ValueError, IndexError):
                return None
        elif isinstance(cur, dict):
            cur = cur.get(part)
        else:
            return None
    return cur


def cmd_poll(a) -> dict:
    st = load_state(a.cell, a.node)
    rec = Recorder(a.cell, redactor_for(st))
    os.environ["BETA1_LABEL"] = a.label or "poll"
    c = client_for(a.cell, a.node, rec, st)
    # Only changes are written: identical reads are counted, not stored.
    quiet_records: list = []
    c.recorder = lambda exchange: quiet_records.append(exchange)
    deadline = time.monotonic() + a.seconds
    reads, last, changes, first = 0, None, [], None
    conds = [x.split("=", 1) for x in (a.until or [])]
    met = None
    with c.polling() as view:
        while time.monotonic() < deadline:
            try:
                resp = view.get(a.path, timeout=40)
                doc = resp.json()
            except PanelError as exc:
                doc = {"read_failed": str(exc)}
                resp = None
            reads += 1
            key = json.dumps(doc, sort_keys=True)
            if key != last:
                changes.append({"at": now(), "read": reads, "status": resp.status if resp else None})
                rec.write(f"{a.node}-poll-{a.path.split('?')[0].replace('/api/v1/', '')}", {"at": now(), "read": reads, "label": a.label,
                                                                                          "status": resp.status if resp else None, "body": doc})
                last = key
            if conds and all(str(get_path(doc, k)) == v for k, v in conds):
                met = {"read": reads, "at": now()}
                break
            time.sleep(a.every)
    out = {"reads": reads, "distinct_states": len(changes), "changes": changes[-12:], "until_met": met,
           "mutations_during_poll": [m for m in c.mutations]}
    try:
        out["last"] = json.loads(last) if last else None
    except ValueError:
        out["last"] = last
    rec.write(f"{a.node}-poll-summary", {k: v for k, v in out.items() if k != "last"} | {"path": a.path, "label": a.label})
    if a.brief and out["last"] is not None:
        out["last"] = json.dumps(rec.redactor.value(out["last"]), sort_keys=True)[: a.brief]
    return out


def cmd_sh(a) -> dict:
    st = load_state(a.cell, a.node)
    rec = Recorder(a.cell, redactor_for(st))
    t0 = time.monotonic()
    r = guest(a.cell, a.node, "sudo -n bash -c " + json.dumps(a.command) if not a.raw else a.command, timeout=a.timeout)
    out = {"command": a.command, "rc": r.returncode, "seconds": round(time.monotonic() - t0, 2),
           "stdout": rec.redactor.text(r.stdout.decode("utf-8", "replace"))[-a.limit:],
           "stderr": rec.redactor.text(r.stderr.decode("utf-8", "replace"))[-1500:], "at": now(), "label": a.label}
    if a.label:
        rec.write(f"{a.node}-sh-{a.label}", out)
    return out


DBCOUNT = r'''
import json, sqlite3, sys
db = sqlite3.connect("file:/var/lib/celikpanel/celikpanel.db?mode=ro", uri=True, timeout=10)
tables = [r[0] for r in db.execute("select name from sqlite_master where type='table' order by name")]
counts = {t: db.execute(f'select count(*) from "{t}"').fetchone()[0] for t in tables}
audit = {}
try:
    cols = [r[1] for r in db.execute("pragma table_info(audit_logs)")]
    col = "action" if "action" in cols else cols[1]
    audit = dict(db.execute(f"select {col}, count(*) from audit_logs group by {col} order by {col}").fetchall())
except Exception as exc:
    audit = {"error": str(exc)}
ex = []
try:
    ex = [dict(zip(["id", "status"], r)) for r in db.execute("select id, status from server_setup_executions order by rowid")]
except Exception as exc:
    ex = [{"error": str(exc)}]
print(json.dumps({"counts": counts, "audit_actions": audit, "setup_executions": ex}))
'''


def cmd_dbcounts(a) -> dict:
    st = load_state(a.cell, a.node)
    rec = Recorder(a.cell, redactor_for(st))
    r = guest(a.cell, a.node, "sudo -n python3 -I -", stdin=DBCOUNT.encode(), timeout=60)
    try:
        doc = json.loads(r.stdout)
    except ValueError:
        doc = {"error": r.stderr.decode()[-800:]}
    doc["at"] = now()
    doc["label"] = a.label
    rec.write(f"{a.node}-dbcounts-{a.label or ''}", doc)
    return doc


def wizard_client(a):
    st = load_state(a.cell, a.node)
    rec = Recorder(a.cell, redactor_for(st))
    os.environ["BETA1_LABEL"] = getattr(a, "label", None) or ""
    return st, rec, client_for(a.cell, a.node, rec, st)


def cmd_save(a) -> dict:
    """One wizard "Next": the draft as the step leaves it (ServerSetup.tsx saveDraft), PUT with the revision."""
    st, rec, c = wizard_client(a)
    snap = c.get("/api/v1/setup", purpose="setup snapshot (useServerSetup)").json()
    draft = dict(snap["draft"])
    patch = body_arg(a.patch) or {}
    for key, value in patch.items():
        if value is None:
            draft.pop(key, None)
        else:
            draft[key] = value
    if not a.joined:  # setupJoinPendingDraft: the new flow never sends a typed peer
        if draft.get("dns_mode") == "local":
            if draft.get("server_count") == 1:
                draft.update({"dns_role": "primary", "main_dns_server": "this", "peer_ip": "", "peer_ns": ""})
            else:
                draft.update({"peer_ip": "", "peer_ns": draft["ns2"] if draft.get("dns_role") == "primary" else draft["ns1"]})
    resp = c.put("/api/v1/setup", {"revision": snap["revision"], "draft": draft}, purpose=f"wizard next ({a.label})")
    out = short(resp, a.limit, rec.redactor)
    return out


def cmd_records(a) -> dict:
    """The records step: GET /setup/public-records, then one decision per public value (importCandidates in
    ServerSetupWizardSteps.tsx: import by default, a server name of this setup skipped, refused values not sent)."""
    st, rec, c = wizard_client(a)
    snap = c.get("/api/v1/setup").json()
    d = snap["draft"]
    read = c.get("/api/v1/setup/public-records", purpose="records step (SetupRecordsStep)").json()
    own = [x for x in (d.get("panel_domain"), d.get("mail_hostname"), d.get("ns1"), d.get("ns2"),
                       (d.get("infrastructure_dns") or {}).get("peer_panel_domain"), (d.get("infrastructure_dns") or {}).get("peer_mail_hostname")) if x]
    skip = set(a.skip or [])
    out = []
    for item in read.get("records", []):
        if item.get("state") != "found" or item.get("type") not in ("A", "AAAA", "CNAME", "MX", "TXT", "CAA"):
            continue
        for value in item.get("values", []):
            if (item.get("not_importable") or {}).get(value):
                continue
            content, prio = value, None
            if item["type"] == "MX":
                pref, _, host = value.partition(" ")
                prio, content = int(pref or 0), host
            clash = item["name"] in own and item["type"] in ("A", "AAAA", "CNAME")
            entry = {"name": item["name"], "type": item["type"], "content": content, "decision": "skip" if clash or f"{item['name']}/{item['type']}" in skip else "import"}
            if prio is not None:
                entry["priority"] = prio
            out.append(entry)
    resp = c.put("/api/v1/setup", {"revision": snap["revision"], "draft": {**d, "imported_records": out}}, purpose="wizard next (records)")
    return {"public_records": read, "imported_records": out, "save": short(resp, 1500, rec.redactor)}


def cmd_plan(a) -> dict:
    st, rec, c = wizard_client(a)
    snap = c.get("/api/v1/setup").json()
    resp = c.post("/api/v1/setup/plan", {"revision": snap["revision"]}, purpose="review (POST /setup/plan)")
    doc = resp.json()
    if isinstance(doc, dict) and doc.get("id"):
        st["plan_id"] = doc["id"]
        st["package_updates_digest"] = doc.get("package_updates_digest", "")
        save_state(a.cell, a.node, st)
    return short(resp, a.limit, rec.redactor)


def cmd_start(a) -> dict:
    """POST /setup/start once with a fresh request_id (or the saved one with --resume, as resumeUnconfirmed)."""
    st, rec, c = wizard_client(a)
    if a.resume:
        rid = st["start_request_id"]
    else:
        rid = secrets.token_hex(16)
        st["start_request_id"] = rid
        save_state(a.cell, a.node, st)
    try:
        digest = a.digest if a.digest is not None else st.get("package_updates_digest", "")
        resp = c.post("/api/v1/setup/start", {"plan_id": st["plan_id"], "request_id": rid, "confirmed": True, "package_updates_digest": digest or ""},
                      purpose="start (ServerSetup.tsx start)", timeout=90)
    except UnknownOutcome as exc:
        return {"outcome": "unknown", "request_id": rid, "error": str(exc)}
    out = short(resp, a.limit, rec.redactor)
    out["request_id"] = rid
    return out


def main(argv: list[str]) -> int:
    p = argparse.ArgumentParser()
    sub = p.add_subparsers(dest="cmd", required=True)
    x = sub.add_parser("provision"); x.add_argument("cell"); x.add_argument("node"); x.add_argument("os"); x.add_argument("ip"); x.add_argument("hostname"); x.add_argument("mem", nargs="?", type=int, default=2560)
    for name in ("install", "login", "leaf"):
        x = sub.add_parser(name); x.add_argument("cell"); x.add_argument("node")
    x = sub.add_parser("api"); x.add_argument("cell"); x.add_argument("node"); x.add_argument("method"); x.add_argument("path"); x.add_argument("body", nargs="?")
    x.add_argument("--label"); x.add_argument("--limit", type=int, default=6000); x.add_argument("--timeout", type=float, default=60)
    x = sub.add_parser("poll"); x.add_argument("cell"); x.add_argument("node"); x.add_argument("path"); x.add_argument("seconds", type=float); x.add_argument("every", type=float)
    x.add_argument("--until", action="append"); x.add_argument("--label"); x.add_argument("--brief", type=int, default=4000)
    x = sub.add_parser("sh"); x.add_argument("cell"); x.add_argument("node"); x.add_argument("command"); x.add_argument("--label"); x.add_argument("--limit", type=int, default=6000)
    x.add_argument("--timeout", type=int, default=300); x.add_argument("--raw", action="store_true")
    x = sub.add_parser("dbcounts"); x.add_argument("cell"); x.add_argument("node"); x.add_argument("--label")
    x = sub.add_parser("save"); x.add_argument("cell"); x.add_argument("node"); x.add_argument("patch"); x.add_argument("--label", default="")
    x.add_argument("--joined", action="store_true"); x.add_argument("--limit", type=int, default=3000)
    x = sub.add_parser("records"); x.add_argument("cell"); x.add_argument("node"); x.add_argument("--skip", action="append"); x.add_argument("--label", default="records")
    x = sub.add_parser("plan");x.add_argument("cell"); x.add_argument("node"); x.add_argument("--label", default="plan"); x.add_argument("--limit", type=int, default=12000)
    x = sub.add_parser("start"); x.add_argument("cell"); x.add_argument("node"); x.add_argument("--resume", action="store_true")
    x.add_argument("--label", default="start"); x.add_argument("--limit", type=int, default=6000); x.add_argument("--digest")
    a = p.parse_args(argv)
    if a.cmd == "leaf":
        out = {"leaf_sha256": leaf(a.cell, a.node)}
    else:
        out = globals()[f"cmd_{a.cmd}"](a)
    print(json.dumps(out, indent=1, ensure_ascii=False, default=str))
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
