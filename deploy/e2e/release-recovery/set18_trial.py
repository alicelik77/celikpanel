#!/usr/bin/env python3
"""set18: the twenty-first native record. The alpha.87 candidate (HEAD 5d54aba1e of feat/alpha87-second-server plus a
policy-only fixture commit labelled v0.1.0-alpha.87 / 87) installed FRESH on a disposable Debian 13 guest whose lab
has its own DNS and its own ACME server (guest_set18_lab_dns.py, a lab preparation, not a product change), so that the
owner's guided setup can reach "ready" and an extension of it can add a mail profile, in a real headless Chrome
(web/tools/browser-inspect/cold-load-set18.mjs, set12's browser window protocol).

``set18-debian13`` (``Set18Trial`` over ``set17_trial.Set17Trial``): pinning, preflight, origin, the lab network
preparation (``set18-lab-network``: DNS responder, DNAT of the public resolvers, outbound 443 block with the package
hosts allowed, pebble and its root in the system trust, certbot's user configuration), install, owner, licence, then
the browser window "setup" (the owner runs the wizard with purpose web to ready; the Components mail route and
"Stop extending"; the extension from the ready screen adding Core mail), then readings, pinning at the end, collect.

Preparations the browser may ask for: ``probe``, ``journal``, ``lab-state`` (read-only), ``lab-mail-a`` (the owner adds
the mail host's A record at the DNS provider: the lab record table), ``lab-ptr-mail`` (the owner sets the reverse DNS
at the server provider: the lab record table), ``refresh-pin`` (the driver re-reads the Panel's TLS leaf after the
certificate handover), ``setup-read`` (GET /api/v1/setup and the latest operation). The driver calls only the Panel's
HTTP API; every native fact is read over SSH. Every result carries ``native_evidence: false``. Nothing here judges a
P0 row.

  set18_trial.py plan --cell set18-debian13 --artifacts A.json --work-root /var/tmp/cp-release-drill-X [--dry-run]
  set18_trial.py run  --cell set18-debian13 --artifacts A.json --work-root /var/tmp/cp-release-drill-X --execute
"""
from __future__ import annotations

import argparse
import dataclasses
import json
import os
from pathlib import Path
import shlex
import sys
import time

HERE = Path(__file__).resolve().parent
if str(HERE) not in sys.path:
    sys.path.insert(0, str(HERE))
import set17_trial as t17  # noqa: E402  (set16/set15/set14 chain, set17_fixture)
import set18_fixture as fx18  # noqa: E402  (after set17_fixture: the alpha.87 profile and the 88 origin copy)

s16, s6, sw, base, t14 = t17.s16, t17.s6, t17.sw, t17.base, t17.t14
out = fx18.out
HELPER = "guest_set18_lab_dns.py"
SERVER_IP = "192.0.2.10"
PANEL_DOMAIN = "panel.set18.test"
MAIL_HOSTNAME = "mail.set18.test"
CELL_KIND = "set18-fresh-setup-to-ready"
CELLS = {"set18-debian13": sw.SettingsCell("set18-debian13", "debian13", "web", sw.WEB_PRESET, False)}
BROWSER = {"set18-debian13": {"lang": "en", "viewport": "1280"}}
PREPS = ("probe", "journal", "lab-state", "lab-mail-a", "lab-ptr-mail", "refresh-pin", "setup-read")
WINDOW_SECONDS = 4 * 3600
SETUP_READS = (("setup", "/api/v1/setup"), ("operation", "/api/v1/setup/operation"),
               ("version", "/api/v1/panel/version"), ("managed services", "/api/v1/managed-services"))
NATIVE_READING = r"""set +e
echo "== hostname"; hostnamectl --static 2>&1; hostname -f 2>&1
echo "== panel TLS (127.0.0.1:2083, SNI panel.set18.test)"; echo | timeout 10 openssl s_client -connect 127.0.0.1:2083 -servername panel.set18.test 2>/dev/null | openssl x509 -noout -subject -issuer -dates -ext subjectAltName 2>&1
echo "== submission TLS (127.0.0.1:587)"; echo | timeout 10 openssl s_client -starttls smtp -connect 127.0.0.1:587 -servername mail.set18.test 2>/dev/null | openssl x509 -noout -subject -issuer -dates 2>&1
echo "== imaps TLS (127.0.0.1:993)"; echo | timeout 10 openssl s_client -connect 127.0.0.1:993 -servername mail.set18.test 2>/dev/null | openssl x509 -noout -subject -issuer -dates 2>&1
echo "== certbot lineages"; ls -la /etc/letsencrypt/live 2>&1; for f in /etc/letsencrypt/renewal/*.conf; do echo "-- $f"; grep -E '^(server|no_verify_ssl|authenticator|webroot_path|account) ' "$f"; done 2>&1
echo "== certbot timer"; systemctl is-enabled certbot.timer 2>&1; systemctl is-active certbot.timer 2>&1
echo "== postfix"; postconf -h myhostname smtpd_tls_security_level smtpd_tls_cert_file 2>&1
echo "== units"; systemctl is-active nginx postfix dovecot mariadb celikpanel-panel celikpanel-agent 2>&1 | paste -sd' '
echo "== listeners"; ss -ltnp 2>/dev/null | awk 'NR>1{print $4, $6}' | sort | head -n 60
echo "== celikpanel firewall table"; nft list tables 2>&1
"""


class Set18Trial(t17.Set17Trial):
    def upload_helpers(self) -> dict:
        helpers = super().upload_helpers()
        helpers[f"{HELPER} (set18 lab preparation)"] = self.lab.put_file(
            self.root, self.record, self.plan, self.node_name, HERE / HELPER, HELPER)[1]
        return helpers

    def lab_helper(self, mode: str, *args: str, timeout: float = 120) -> dict:
        argv = ["python3", "-I", f"{out.PRIVATE}/{HELPER}", mode, *args]
        result = self.guest(shlex.join(argv), timeout=timeout)
        try:
            return json.loads((result.stdout or "").strip().splitlines()[-1])
        except (ValueError, IndexError):
            return {"returncode": result.returncode, "stdout": (result.stdout or "")[-1500:], "stderr": (result.stderr or "")[-1500:]}

    def lab_network(self, checks: dict) -> str:
        """Lab preparation (not a product change): DNS responder, DNAT, outbound rules, pebble, trust, certbot config."""
        record = self.lab_helper("prepare", "--server-ip", SERVER_IP, timeout=1500)
        self.record_json("set18-lab-network.json", record)
        dns = record.get("dns_checks") or []
        answered = [c for c in dns if c.get("name") == PANEL_DOMAIN and c.get("rcode") == 0 and c.get("answers") == 1]
        checks.update(what=record.get("what"), server_ip=SERVER_IP, route_get_1_1_1_1=record.get("route_get_1_1_1_1"),
                      dns_checks=dns, block_check=record.get("block_check_203_0_113_1_443"),
                      pebble_install=record.get("pebble_install"), pebble_version=record.get("pebble_version"),
                      pebble_root=record.get("pebble_root"), directory=(record.get("directory") or "")[:300],
                      certbot_cli_ini=record.get("certbot_cli_ini"), allowed_hosts=(record.get("tables") or {}).get("allowed_hosts"),
                      error=None if "error" not in record else record)
        problems = []
        if len(answered) != 8:
            problems.append(f"the redirected resolvers answered the panel name {len(answered)} of 8 times")
        if f"src {SERVER_IP}" not in str(record.get("route_get_1_1_1_1")):
            problems.append("the route to 1.1.1.1 does not name the server address")
        if "newNonce" not in str(record.get("directory")):
            problems.append("pebble's directory was not read")
        if "rc=0" in str(record.get("block_check_203_0_113_1_443")):
            problems.append("outbound 443 to a non-lab address was not refused")
        if problems:
            raise base.StepFailed("; ".join(problems))
        return "passed"

    def prep(self, name: str) -> dict:
        if name == "lab-state":
            return self.lab_helper("state", timeout=120)
        if name in ("lab-mail-a", "lab-ptr-mail"):
            value = self.lab_helper("set-record", "--what", name[len("lab-"):], timeout=120)
            self.current.setdefault("lab_preparation", []).append({"at": base.utc_now(), "what": name, "result": value})
            return value
        if name == "refresh-pin":
            before = getattr(self.transport, "leaf_sha256", None)
            leaf = self.refresh_pin()
            return {"before": before, "after": leaf, "changed": before != leaf}
        if name == "setup-read":
            self.refresh_pin()
            values = {}
            for what, path in SETUP_READS[:2]:
                entry = self.call(f"set18: {what} ({path})", "GET", path, None, timeout=60)
                values[what] = {"status": entry["status"], "body": entry.get("json")}
            return values
        return super().prep(name)

    def window18(self, checks: dict) -> str:
        self.current = {"section": "set18-browser-window", "node": self.node_name, "native_evidence": False,
                        "started_at": base.utc_now(), "calls": [], "checks": [], "natives": [], "notes": [],
                        "owner_actions": [], "lab_preparation": []}
        if not os.environ.get("SET12_HAND"):
            checks["reason"] = "no browser in this lab (SET12_HAND is not set)"
            return "skipped"
        record: dict = {}
        try:
            first = self.snap14("probe-before-the-window", "read-probe14")
            self.s14["window_since"] = float(first.get("epoch") or time.time())
            checks["probe_before_the_window"] = t14.compact_probe(first)
            record = self.browser_window("setup", WINDOW_SECONDS, {
                "browser": BROWSER[self.settings.name], "cell": self.settings.name, "mode": "set18", "preps": list(PREPS),
                "lab": {"panel_domain": PANEL_DOMAIN, "mail_hostname": MAIL_HOSTNAME, "server_ip": SERVER_IP,
                        "mail_profile": "core-mail"}})
            checks.update({k: v for k, v in record.items() if k != "requests"})
            checks["prep_count"] = len(record.get("requests") or [])
            self.record_json("browser-window.json", record)
        finally:
            try:
                checks["pin_after_the_window"] = self.prep("refresh-pin")
            except Exception as exc:  # noqa: BLE001
                checks["pin_after_the_window_error"] = self.error_text(exc)
            for entry in self.current.get("calls", []):
                entry.pop("_parsed", None)
            self.current["finished_at"] = base.utc_now()
            self.record_json("window.json", self.current)
        if record.get("skipped"):
            return "skipped"
        if record.get("not_finished"):
            raise base.StepInconclusive(record["not_finished"])
        checks["session"] = self.ensure_session()
        return "observed"

    def readings18(self, checks: dict) -> str:
        self.current = {"section": "set18-readings", "node": self.node_name, "native_evidence": False,
                        "started_at": base.utc_now(), "calls": [], "checks": [], "natives": [], "notes": []}
        try:
            self.refresh_pin()
            api = {}
            for what, path in SETUP_READS:
                entry = self.call(f"set18: {what} ({path})", "GET", path, None, timeout=120)
                body = entry.get("json")
                if path == "/api/v1/managed-services" and isinstance(body, dict):
                    body = {k: body.get(k) for k in ("scan_age_seconds", "dns_identity_ready", "mail_dns_guidance", "mail_profiles")}
                api[path] = {"status": entry["status"], "at": entry["at"], "body": body}
            checks["api"] = api
            setup = (api.get("/api/v1/setup") or {}).get("body") or {}
            checks["setup_status"] = setup.get("status") if isinstance(setup, dict) else None
            checks["setup_extension"] = setup.get("extension") if isinstance(setup, dict) else None
            native = self.guest(NATIVE_READING, timeout=180)
            self.ev.write_text(f"{self.step_dir}/native-reading.txt", (native.stdout or "") + (native.stderr or ""))
            lab = self.lab_helper("state", timeout=120)
            self.record_json("set18-lab-state-at-the-end.json", lab)
            checks["lab_units"] = lab.get("units")
            checks["lab_block_log"] = lab.get("block_log")
            checks["lab_renewal"] = lab.get("letsencrypt_renewal")
            checks["lab_queries_total"] = lab.get("queries_total")
            try:
                checks["ledger"] = self.schema_ledger("set18-fresh", t17.LEDGER_VERSION)
            except Exception as exc:  # noqa: BLE001 - recorded; the cell goes on
                checks["ledger"] = {"verdict": "inconclusive", "error": f"{type(exc).__name__}: {str(exc)[:300]}"}
        finally:
            for entry in self.current.get("calls", []):
                entry.pop("_parsed", None)
            self.current["finished_at"] = base.utc_now()
            self.record_json("readings.json", self.current)
        return "observed"

    def execute(self) -> dict:
        self.step("preflight", self.preflight)
        self.step("origin", self.origin, needs=("preflight",))
        self.step("set18-lab-network", self.lab_network, needs=("origin",))
        self.step("baseline-install", self.baseline_install, needs=("set18-lab-network",))
        self.step("owner-login", self.owner_login, needs=("baseline-install",))
        self.step("license", self.license, needs=("owner-login",))
        self.step("set18-browser-window", self.window18, needs=("license",))
        self.step("set18-readings", self.readings18, needs=("baseline-install",))
        self.step(s6.PIN_END_STEP, self.pin_at_the_end6)
        self.step("collect", self.collect)
        self.tunnel.close()
        verdicts = [s["verdict"] for s in self.steps]
        result = {"schema": base.RESULT_SCHEMA, "cell_kind": CELL_KIND, "native_evidence": False,
                  "cell": dataclasses.asdict(self.settings),
                  "identity": {k: self.identity[k] for k in ("cell_id", "node", "vm_uuid")},
                  "request_id": None,
                  "provenance": {"what": f"set18: a policy-only fixture over the source {fx18.SOURCE_COMMIT[:9]} labelled "
                                         "v0.1.0-alpha.87, installed fresh on a lab with its own DNS and ACME; no update"},
                  "artifacts": {"baseline": {k: self.artifacts["baseline"][k] for k in ("version", "commit", "sha256")}},
                  "outcome": {"classification": "set18-measured", "final_status": None},
                  "lab": {"panel_domain": PANEL_DOMAIN, "mail_hostname": MAIL_HOSTNAME, "server_ip": SERVER_IP,
                          "preparation": "guest_set18_lab_dns.py (lab-only DNS, DNAT, outbound 443 block, pebble)"},
                  "browser": BROWSER[self.settings.name], "findings": self.state["findings"],
                  "steps": [{k: s.get(k) for k in ("name", "verdict", "reason", "started_at", "finished_at")} for s in self.steps],
                  "overall": base.overall(verdicts),
                  "note": "set18: a measurement on the fresh install; the owner's setup and its extension run in the "
                          "browser; no update is started and no P0 row is judged."}
        self.step_dir = "result"
        return self.ev.finalize_upd1(result)


def build_plan(name: str, artifacts: dict, work_root: str, local_port: int) -> dict:
    settings = CELLS[name]
    return {"schema": "celikpanel/set18-plan/v1", "native_evidence": False, "cell": name, "work_root": work_root,
            "local_port": local_port, "pinned_names": list(s6.PINNED_NAMES), "browser": BROWSER[name],
            "cell_kind": CELL_KIND, "candidate": {k: artifacts["baseline"][k] for k in ("version", "commit", "sha256")},
            "setup": {"purpose": settings.purpose, "dns_mode": "external", "panel_domain": PANEL_DOMAIN,
                      "extension": {"mail_profile": "core-mail", "mail_hostname": MAIL_HOSTNAME}},
            "lab": {"helper": HELPER, "server_ip": SERVER_IP},
            "preparations": list(PREPS),
            "steps": [s6.PIN_STEP, "preflight", "origin", "set18-lab-network", "baseline-install", "owner-login",
                      "license", "set18-browser-window", "set18-readings", s6.PIN_END_STEP, "collect"],
            "rule": "the driver calls only the Panel's HTTP API as the logged-in owner; every native fact is an SSH "
                    "reading; owner actions and lab preparation on the guest are recorded as such; no certificate "
                    "authority and no licence service is contacted; outbound 443 from the guest is refused except to "
                    "the package hosts"}


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    sub = parser.add_subparsers(dest="command", required=True)
    for name in ("plan", "run"):
        cmd = sub.add_parser(name)
        cmd.add_argument("--cell", required=True, choices=sorted(CELLS))
        cmd.add_argument("--artifacts", required=True, type=Path)
        cmd.add_argument("--work-root", required=True)
        cmd.add_argument("--local-port", type=int, default=18443)
        if name == "plan":
            cmd.add_argument("--dry-run", action="store_true", help="validate the plan without any guest")
        else:
            cmd.add_argument("--execute", action="store_true")
    args = parser.parse_args(argv)
    base.validate_work_root(args.work_root)
    if not 1024 < args.local_port < 65536:
        parser.error("--local-port must be an unprivileged loopback port")
    document = json.loads(args.artifacts.read_text())
    labels = fx18.configure(document)
    if labels.get("ref") != fx18.THIRD_REF:
        parser.error(f"cell {args.cell} needs an artifacts document with baseline_ref {fx18.THIRD_REF}, not {labels.get('ref')}")
    cell = CELLS[args.cell].cell
    if args.command == "plan":
        base.validate_cell_artifacts(document, cell, check_files=not args.dry_run)
        print(json.dumps(build_plan(args.cell, document, args.work_root, args.local_port), indent=2, sort_keys=True))
        return 0
    if not args.execute:
        parser.error("run mutates one registered disposable guest and requires --execute")
    base.validate_cell_artifacts(document, cell)
    result = Set18Trial(CELLS[args.cell], document, args.work_root, args.local_port).execute()
    print(json.dumps({"overall": result["overall"], "outcome": (result.get("outcome") or {}).get("classification"),
                      "steps": {s["name"]: s["verdict"] for s in result["steps"]}}, sort_keys=True))
    return 0 if result["overall"] != "failed" else 1


if __name__ == "__main__":
    raise SystemExit(main())
