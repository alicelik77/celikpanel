#!/usr/bin/env python3
"""set14c: the native record after set14b, on commit 68b79b372 (7024534bc: the licence gate follows the server's own
time left, ``valid_for_seconds``; an unreachable Agent is named on every read; a failed read no longer claims an
action), measured on one disposable Debian 13 guest installed fresh, with a real headless Chrome on the host
(web/tools/browser-inspect/cold-load-set14c.mjs).

Everything of set14b_trial.py is reused unchanged (fresh install, setup, the browser window, its preparations: probe,
clockshift_<p|m>N, ntp-on, scan-now, admission, agent-stop, agent-start, journal). Added preparation:

  agent-reads   the owner's two reads that need the Agent, through the Panel's API (``GET /api/v1/firewall``,
                ``GET /api/v1/system/check``): status, the whole body and whether it names the Agent's socket

  set14c_trial.py plan --cell set14c-debian13 --artifacts A.json --work-root /var/tmp/cp-release-drill-X [--dry-run]
  set14c_trial.py run  --cell set14c-debian13 --artifacts A.json --work-root /var/tmp/cp-release-drill-X --execute
"""
from __future__ import annotations

import argparse
import dataclasses
import json
from pathlib import Path
import re
import sys
from typing import Any

HERE = Path(__file__).resolve().parent
if str(HERE) not in sys.path:
    sys.path.insert(0, str(HERE))
import set14b_trial as s14b  # noqa: E402

s14, s6, sw, base, fx = s14b.s14, s14b.s6, s14b.sw, s14b.base, s14b.fx
CELL_KIND = "set14c-fresh-install"
CELLS = {"set14c-debian13": sw.SettingsCell("set14c-debian13", "debian13", "web", sw.WEB_PRESET, False)}
# English desktop first; the browser opens the Turkish phone page itself (item 4).
BROWSER = {"set14c-debian13": {"lang": "en", "viewport": "desktop", "switch_lang": "tr", "switch_viewport": "phone"}}
PREPS = s14b.PREPS + ("clockshift_p840000", "agent-reads")
AGENT_READS = ("/api/v1/firewall", "/api/v1/system/check")
SOCKET = re.compile(r"agent\.sock|/run/celikpanel|dial unix|connect:", re.I)
prep_name = s14b.prep_name
shift_of = s14b.shift_of


def read_fields(status: int, parsed: Any, text: str | None) -> dict:
    """What item 3 records of a read with the Agent stopped: status, code, the sentence, the socket check."""
    shown = json.dumps(parsed, sort_keys=True) if parsed is not None else (text or "")
    body = parsed if isinstance(parsed, dict) else {}
    return {"status": status, "code": body.get("code"), "error": body.get("error"), "message": body.get("message"),
            "keys": sorted(body) if isinstance(parsed, dict) else None, "body": shown[:3000],
            "body_mentions_socket": bool(SOCKET.search(shown))}


class Set14cTrial(s14b.Set14bTrial):
    def prep(self, name: str) -> dict:
        if name == "agent-reads":
            self.s14["prep_count"] = self.s14.get("prep_count", 0) + 1
            out = {}
            for path in AGENT_READS:
                entry = self.call(f"ui: the owner reads {path} while the Agent is stopped", "GET", path, None, timeout=120)
                out[path] = read_fields(entry["status"], entry.get("_parsed"), entry.get("text")) | {"at": entry["at"]}
            return out
        return super().prep(name)

    def window14(self, checks: dict) -> str:
        # set14b's window lends its module's BROWSER and PREPS to set14's; set14c's are lent to set14b's for this
        # window only (an import of this module changes nothing in set14b_trial or set14_trial).
        saved = (s14b.BROWSER, s14b.PREPS)
        s14b.BROWSER = dict(saved[0]) | BROWSER
        s14b.PREPS = PREPS
        try:
            return super().window14(checks)
        finally:
            s14b.BROWSER, s14b.PREPS = saved

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
                  "request_id": None, "provenance": {"what": "set14c: 68b79b372 installed fresh; no update"},
                  "artifacts": {"baseline": {k: self.artifacts["baseline"][k] for k in ("version", "commit", "sha256")}},
                  "outcome": {"classification": "set14c-measured", "final_status": None},
                  "setup": {"purpose": self.settings.purpose, "components": sorted(self.settings.components),
                            "waiting": self.state.get("setup_waiting")},
                  "browser": BROWSER[self.settings.name], "findings": self.state["findings"],
                  "steps": [{k: s.get(k) for k in ("name", "verdict", "reason", "started_at", "finished_at")} for s in self.steps],
                  "overall": base.overall(verdicts),
                  "note": "set14c: a measurement on the fresh install; no update is started and no P0 row is judged. "
                          "The browser's own records (requests, card texts, pictures) are written on the host."}
        self.step_dir = "result"
        return self.ev.finalize_upd1(result)


def build_plan(name: str, artifacts: dict, work_root: str, local_port: int) -> dict:
    settings = CELLS[name]
    return {"schema": "celikpanel/set14c-plan/v1", "native_evidence": False, "cell": name, "work_root": work_root,
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
    result = Set14cTrial(CELLS[args.cell], document, args.work_root, args.local_port).execute()
    print(json.dumps({"overall": result["overall"], "cell_kind": CELL_KIND,
                      "steps": {s["name"]: s["verdict"] for s in result["steps"]}}, sort_keys=True))
    return 0 if result["overall"] != "failed" else 1


if __name__ == "__main__":
    raise SystemExit(main())
