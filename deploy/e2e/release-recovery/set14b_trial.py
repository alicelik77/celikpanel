#!/usr/bin/env python3
"""set14b: a short follow-up of the fourteenth native record, on commit 1a74eeda5 (after e6de64a5b): the managed-services
answers carry ``scan_age_seconds`` and the pages date a scan from it (D1 of set14: "Not checked in the last 5 minutes"
right after a check when the server's clock is ahead), and an unreachable Agent answers 503 ``AGENT_UNREACHABLE``
(O1 of set14: 500 "internal error").

Everything of set14_trial.py is reused (fresh install, setup, the browser window, its preparations). Added
preparations the browser (web/tools/browser-inspect/cold-load-set14b.mjs) asks for, each recorded:

  probe              read-probe14b: read-probe14 plus the guest's NTP settings and nginx's systemd state
  clockshift_<p|m>N  lab preparation: the guest clock moved by N ms forward (p) or back (m) relative to its own
                     reading (``timedatectl set-ntp false``; ``date -u -s @...``); the total is tracked and undone at
                     the end of the window if the browser did not undo it
  ntp-on             lab preparation: ``timedatectl set-ntp true``
  scan-now           the owner's unconditional scan through the Panel's API (``POST /api/v1/managed-services/scan``,
                     what the "Scan services" button sends), its answer's ``scanned_at`` and ``scan_age_seconds``
  admission          the owner restarts nginx from the Panel (``POST /api/v1/service/action`` ``restart``) while the
                     Agent is stopped; nginx's MainPID and start time read before and after

  set14b_trial.py plan --cell set14b-debian13 --artifacts A.json --work-root /var/tmp/cp-release-drill-X [--dry-run]
  set14b_trial.py run  --cell set14b-debian13 --artifacts A.json --work-root /var/tmp/cp-release-drill-X --execute
"""
from __future__ import annotations

import argparse
import base64
import dataclasses
import json
import os
from pathlib import Path
import re
import sys
from typing import Any

HERE = Path(__file__).resolve().parent
if str(HERE) not in sys.path:
    sys.path.insert(0, str(HERE))
import set14_trial as s14  # noqa: E402

s13, s12, s10, s6, sw, base, fx = s14.s13, s14.s12, s14.s10, s14.s6, s14.sw, s14.base, s14.fx
HELPER14B = "guest_set14b_native.py"
CELL_KIND = "set14b-fresh-install"
CELLS = {"set14b-debian13": sw.SettingsCell("set14b-debian13", "debian13", "web", sw.WEB_PRESET, False)}
# The browser switches the language per page itself (English first, Turkish for the Agent-unreachable card).
BROWSER = {"set14b-debian13": {"lang": "en", "viewport": "desktop", "switch_lang": "tr"}}
PREPS = ("probe", "clockshift_m420000", "clockshift_p420000", "ntp-on", "scan-now", "admission", "agent-stop",
         "agent-start", "journal")
SHIFT = re.compile(r"clockshift_([pm])(\d{1,6})")
SHIFT_LIMIT_MS = 15 * 60 * 1000
ADMISSION_BODY = {"name": "nginx", "action": "restart"}
prep_name = s14.prep_name


def shift_of(name: str) -> int | None:
    """``clockshift_m420000`` -> -420000; anything else -> None."""
    match = SHIFT.fullmatch(name)
    if not match:
        return None
    value = int(match.group(2)) * (1 if match.group(1) == "p" else -1)
    return value if 0 < abs(value) <= SHIFT_LIMIT_MS else None


def compact_probe(value: dict) -> dict:
    nginx = value.get("nginx") or {}
    return s14.compact_probe(value) | {
        "clock": {k: (value.get("clock") or {}).get(k) for k in ("NTP", "NTPSynchronized", "CanNTP", "returncode")},
        "nginx": {k: nginx.get(k) for k in ("ActiveState", "MainPID", "NRestarts", "ActiveEnterTimestampMonotonic")}}


def scan_fields(body: Any) -> dict:
    """What item 1 records of a managed-services answer: the two fields, their JSON types, and the code if refused."""
    if not isinstance(body, dict):
        return {"json_object": False}
    age = body.get("scan_age_seconds", "<absent>")
    return {"scanned_at": body.get("scanned_at"), "scan_age_seconds": age,
            "scan_age_seconds_type": "absent" if age == "<absent>" else type(age).__name__,
            "services": len(body["services"]) if isinstance(body.get("services"), list) else None,
            "code": body.get("code"), "error": body.get("error"), "message": body.get("message")}


class Set14bTrial(s14.Set14Trial):
    def upload_helpers(self) -> dict:
        helpers = super().upload_helpers()
        helpers[HELPER14B] = self.lab.put_file(self.root, self.record, self.plan, self.node_name, HERE / HELPER14B, HELPER14B)[1]
        return helpers

    @property
    def s14b(self) -> dict:
        return self.state.setdefault("set14b", {})

    def snap14b(self, label: str, mode: str, **arguments: Any) -> dict:
        payload = base64.b64encode(json.dumps(arguments).encode()).decode()
        value = self.helper(HELPER14B, mode, "--args-b64", payload, timeout=420)
        self.native_sequence += 1
        name = f"native/{self.native_sequence:03d}-{s10.safe_name(label)}.json"
        self.record_json(name, value)
        self.current.setdefault("natives", []).append({"label": label, "mode": mode, "file": name, "owner_action": False})
        value["_file"] = f"{self.step_dir}/{name}"
        return value

    def prep(self, name: str) -> dict:
        n = self.s14.get("prep_count", 0) + 1
        if name == "probe":
            self.s14["prep_count"] = n
            return compact_probe(self.snap14b(f"probe-{n}", "read-probe14b"))
        shift = shift_of(name)
        if shift is not None:
            self.s14["prep_count"] = n
            return self.clock(f"lab-clock-shift-{n}", shift_ms=shift)
        if name == "ntp-on":
            self.s14["prep_count"] = n
            return self.clock(f"lab-ntp-on-{n}", ntp=True)
        if name == "scan-now":
            self.s14["prep_count"] = n
            entry = self.call("ui: the owner's unconditional scan (POST /api/v1/managed-services/scan)", "POST",
                              "/api/v1/managed-services/scan", None, timeout=300)
            return {"status": entry["status"], "at": entry["at"], "answer": scan_fields(entry.get("_parsed"))}
        if name == "admission":
            self.s14["prep_count"] = n
            return self.admission(n)
        return super().prep(name)

    def clock(self, label: str, **arguments: Any) -> dict:
        value = self.snap14b(label, "lab-clock14b", **arguments)
        if "shift_ms" in arguments and value.get("returncode") == 0:
            self.s14b["shift_total_ms"] = self.s14b.get("shift_total_ms", 0) + arguments["shift_ms"]
        if arguments.get("ntp") is True and value.get("returncode") == 0:
            self.s14b["ntp_restored"] = True
        elif "shift_ms" in arguments:
            self.s14b["ntp_restored"] = False
        self.current.setdefault("lab_preparation", []).append({"label": label, "file": value["_file"]})
        return {k: value.get(k) for k in ("command", "shift_ms", "returncode", "stderr", "ntp_off_returncode",
                                          "before_epoch", "after_epoch", "before_clock", "after_clock", "at", "epoch")} | {
            "shift_total_ms": self.s14b.get("shift_total_ms", 0)}

    def admission(self, n: int) -> dict:
        before = self.snap14b(f"admission-before-{n}", "read-probe14b")
        entry = self.call("ui: the owner restarts nginx from the Panel while the Agent is stopped (POST /api/v1/service/action)",
                          "POST", "/api/v1/service/action", dict(ADMISSION_BODY), timeout=120)
        after = self.snap14b(f"admission-after-{n}", "read-probe14b")
        body = entry.get("_parsed")
        pick = lambda v: {k: (v.get("nginx") or {}).get(k) for k in ("ActiveState", "MainPID", "NRestarts",
                                                                     "ActiveEnterTimestampMonotonic", "ExecMainStartTimestamp")}
        return {"request": {"method": "POST", "path": "/api/v1/service/action", "body": dict(ADMISSION_BODY)},
                "status": entry["status"], "body": body if isinstance(body, dict) else None, "at": entry["at"],
                "nginx_before": pick(before), "nginx_after": pick(after),
                "agent_before": ((before.get("units") or {}).get("agent") or {}).get("ActiveState"),
                "agent_after": ((after.get("units") or {}).get("agent") or {}).get("ActiveState"),
                "files": [before["_file"], after["_file"]]}

    def window14(self, checks: dict) -> str:
        # set14's window reads its module's BROWSER and PREPS when it runs; set14b's cell and preparations are lent to
        # them for this window only (an import of this module changes nothing in set14_trial).
        saved = (dict(s14.BROWSER), s14.PREPS)
        s14.BROWSER.update(BROWSER)
        s14.PREPS = PREPS
        try:
            return super().window14(checks)
        finally:
            s14.BROWSER.clear()
            s14.BROWSER.update(saved[0])
            s14.PREPS = saved[1]
            # A clock this harness moved is put back even when the browser stopped half-way.
            restore: dict = {"shift_total_ms_before": self.s14b.get("shift_total_ms", 0)}
            try:
                total = self.s14b.get("shift_total_ms", 0)
                if total:
                    restore["undo"] = self.clock("lab-clock-undo-at-the-end", shift_ms=-total)
                if self.s14b.get("ntp_restored") is False:
                    restore["ntp_on"] = self.clock("lab-ntp-on-at-the-end", ntp=True)
                restore["probe"] = compact_probe(self.snap14b("probe-after-the-clock-check", "read-probe14b"))
            except Exception as exc:  # noqa: BLE001
                restore["error"] = self.error_text(exc)
            checks["clock_at_the_end"] = restore
            self.record_json("clock-at-the-end.json", restore)

    def execute(self) -> dict:
        self.step("preflight", self.preflight)
        self.step("origin", self.origin, needs=("preflight",))
        self.step("baseline-install", self.baseline_install, needs=("origin",))
        self.step("owner-login", self.owner_login, needs=("baseline-install",))
        self.step("license", self.license, needs=("owner-login",))
        self.step("setup", self.setup, needs=("license",))
        self.step("set14-browser-window", self.window14, needs=("setup",))
        self.step(s6.PIN_END_STEP, self.pin_at_the_end6)
        self.step("collect", self.collect)
        self.tunnel.close()
        verdicts = [s["verdict"] for s in self.steps]
        result = {"schema": base.RESULT_SCHEMA, "cell_kind": CELL_KIND, "native_evidence": False,
                  "cell": dataclasses.asdict(self.settings),
                  "identity": {k: self.identity[k] for k in ("cell_id", "node", "vm_uuid")},
                  "request_id": None, "provenance": {"what": "set14b: 1a74eeda5 installed fresh; no update"},
                  "artifacts": {"baseline": {k: self.artifacts["baseline"][k] for k in ("version", "commit", "sha256")}},
                  "outcome": {"classification": "set14b-measured", "final_status": None},
                  "setup": {"purpose": self.settings.purpose, "components": sorted(self.settings.components),
                            "waiting": self.state.get("setup_waiting")},
                  "browser": BROWSER[self.settings.name], "findings": self.state["findings"],
                  "steps": [{k: s.get(k) for k in ("name", "verdict", "reason", "started_at", "finished_at")} for s in self.steps],
                  "overall": base.overall(verdicts),
                  "note": "set14b: a measurement on the fresh install; no update is started and no P0 row is judged. "
                          "The browser's own records (requests, card texts, pictures) are written on the host."}
        self.step_dir = "result"
        return self.ev.finalize_upd1(result)


def build_plan(name: str, artifacts: dict, work_root: str, local_port: int) -> dict:
    settings = CELLS[name]
    return {"schema": "celikpanel/set14b-plan/v1", "native_evidence": False, "cell": name, "work_root": work_root,
            "local_port": local_port, "pinned_names": list(s6.PINNED_NAMES), "cell_kind": CELL_KIND,
            "candidate": {k: artifacts["baseline"][k] for k in ("version", "commit", "sha256")},
            "setup": {"purpose": settings.purpose, "components": sorted(settings.components), "dns_mode": "external"},
            "browser": BROWSER[name], "preparations": list(PREPS),
            "steps": [s6.PIN_STEP, "preflight", "origin", "baseline-install", "owner-login", "license", "setup",
                      "set14-browser-window", s6.PIN_END_STEP, "collect"],
            "rule": "the driver calls only the Panel's HTTP API as the logged-in owner; every native fact is an SSH reading; "
                    "owner actions and lab preparation (the guest clock) on the guest are recorded as such; no "
                    "certificate authority and no licence service is contacted by the driver"}


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
    fx.configure(document)
    cell = CELLS[args.cell].cell
    if args.command == "plan":
        base.validate_cell_artifacts(document, cell, check_files=not args.dry_run)
        print(json.dumps(build_plan(args.cell, document, args.work_root, args.local_port), indent=2, sort_keys=True))
        return 0
    if not args.execute:
        parser.error("run mutates one registered disposable guest and requires --execute")
    base.validate_cell_artifacts(document, cell)
    result = Set14bTrial(CELLS[args.cell], document, args.work_root, args.local_port).execute()
    print(json.dumps({"overall": result["overall"], "cell_kind": CELL_KIND,
                      "steps": {s["name"]: s["verdict"] for s in result["steps"]}}, sort_keys=True))
    return 0 if result["overall"] != "failed" else 1


if __name__ == "__main__":
    raise SystemExit(main())
