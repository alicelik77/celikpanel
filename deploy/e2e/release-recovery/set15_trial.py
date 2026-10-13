#!/usr/bin/env python3
"""set15: the pre-publication measurement of v0.1.0-alpha.84 (commit 1636b73f0; product code last changed in
68b79b372), on disposable QEMU guests (lab.py), with a real headless Chrome on the host
(web/tools/browser-inspect/cold-load-set15.mjs, set12's browser window "ui").

``set15-<debian13|ubuntu|arch>`` (``Set15Trial`` over ``set14_trial.Set14Trial``): the source commit itself installed
FRESH (artifacts document from ``build-set15-artifacts.sh --fresh``; set6's steps: pinning, preflight, origin,
install, owner, licence, setup with purpose ``web``), then one reading step through the Panel's API (panel version,
licence access, the stored scan, the Domains list, setup) with the installed bootstrap pins and the schema ledger read
over SSH, then the browser window: the Domains list and the sidebar, one Dashboard load with a stale scan, the
Components page, three direct reads (``/api/v1/panel/version``, ``/api/v1/license/access``,
``/api/v1/managed-services``).

``upd1-debian13-good`` | ``upd1-debian13-defective`` (``Set15UpdateTrial`` over ``set6_trial.Set6UpdateTrial``), with
an artifacts document from ``build-set15-artifacts.sh --baseline-ref v0.1.0-alpha.83``: set6's owner update from the
alpha.83 tag rebuilt as a test-licence build to the source commit (good) or to a candidate that fails at start
(defective: set5's migrate-only defect, automatic return), without set4/set5's added PHP-site and mail sections. Before
``collect``: the same API readings and ledger, then the same browser window (the Dashboard after the update or after
the return, the sidebar's version line).

Preparations the browser may ask for: ``probe`` (guest_set14_native.py read-probe14: the stored scan's age, the units)
and ``journal``. The driver calls only the Panel's HTTP API; every native fact is read over SSH; owner actions on the
guest are recorded as such. Every result carries ``native_evidence: false``. Nothing here judges a P0 row.

  set15_trial.py plan --cell CELL --artifacts A.json --work-root /var/tmp/cp-release-drill-X [--dry-run]
  set15_trial.py run  --cell CELL --artifacts A.json --work-root /var/tmp/cp-release-drill-X --execute
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
from typing import Any

HERE = Path(__file__).resolve().parent
if str(HERE) not in sys.path:
    sys.path.insert(0, str(HERE))
import set14_trial as t14  # noqa: E402
import set15_fixture as fx15  # noqa: E402  (after set12_fixture, which set14's chain imports)

s12, s10, s6, sw, base = t14.s12, t14.s10, t14.s6, t14.sw, t14.base
CELL_KIND = "set15-fresh-install"
UPDATE_KIND = "set15-update"
CELLS = {
    "set15-debian13": sw.SettingsCell("set15-debian13", "debian13", "web", sw.WEB_PRESET, False),
    "set15-ubuntu": sw.SettingsCell("set15-ubuntu", "ubuntu", "web", sw.WEB_PRESET, False),
    "set15-arch": sw.SettingsCell("set15-arch", "arch", "web", sw.WEB_PRESET, False),
}
UPDATE_CELLS = ("upd1-debian13-good", "upd1-debian13-defective")
BROWSER = {"set15-debian13": {"lang": "en", "viewport": "desktop"}, "set15-ubuntu": {"lang": "tr", "viewport": "desktop"},
           "set15-arch": {"lang": "en", "viewport": "desktop"},
           "upd1-debian13-good": {"lang": "en", "viewport": "desktop"},
           "upd1-debian13-defective": {"lang": "en", "viewport": "desktop"}}
PREPS = ("probe", "journal")
WINDOW_SECONDS = 3600
READS = (("panel version", "/api/v1/panel/version"), ("licence access", "/api/v1/license/access"),
         ("the stored scan", "/api/v1/managed-services"), ("the Domains list", "/api/v1/domains"),
         ("setup", "/api/v1/setup"))
PINS_SCRIPT = r"""
f=/usr/libexec/celikpanel/get.sh
echo "== $f"; ls -l "$f" 2>&1; sha256sum "$f" 2>&1; grep -n '^bootstrap_release_' "$f" 2>&1
for p in $(timeout 60 find /opt /usr/libexec /usr/local /etc /var/lib -xdev -maxdepth 5 -name release-sequence-policy -type f 2>/dev/null | sort); do
  echo "== $p"; cat "$p"
done
echo "== end"
"""


def brief_body(path: str, parsed: Any) -> Any:
    """What the record keeps of an answer: whole for small answers; the stored scan without its service list."""
    if path == "/api/v1/managed-services" and isinstance(parsed, dict):
        services = parsed.get("services")
        return {k: v for k, v in parsed.items() if k != "services"} | {
            "services_count": len(services) if isinstance(services, list) else None,
            "keys": sorted(parsed)}
    if path == "/api/v1/domains":
        rows = parsed if isinstance(parsed, list) else (parsed or {}).get("domains") if isinstance(parsed, dict) else None
        return {"type": type(parsed).__name__, "rows": len(rows) if isinstance(rows, list) else None,
                "body": json.dumps(parsed, sort_keys=True)[:2000]}
    return parsed


class Readings15:
    """The set15 reading step and its window, shared by the fresh and the update trials."""

    def readings15(self, checks: dict, label: str, ledger_version: int) -> str:
        self.current = {"section": f"set15-readings-{label}", "node": self.node_name, "native_evidence": False,
                        "started_at": base.utc_now(), "calls": [], "checks": [], "natives": [], "notes": []}
        try:
            out = {}
            for what, path in READS:
                entry = self.call(f"set15: {what} ({path}) {label}", "GET", path, None, timeout=120)
                out[path] = {"status": entry["status"], "at": entry["at"], "body": brief_body(path, entry.get("_parsed"))}
            checks["api"] = out
            access = (out.get("/api/v1/license/access") or {}).get("body") or {}
            scan = (out.get("/api/v1/managed-services") or {}).get("body") or {}
            checks["fields"] = {
                "panel_version": (out.get("/api/v1/panel/version") or {}).get("body"),
                "access_has_valid_for_seconds": isinstance(access, dict) and "valid_for_seconds" in access,
                "valid_for_seconds": access.get("valid_for_seconds") if isinstance(access, dict) else None,
                "managed_services_has_scan_age_seconds": isinstance(scan, dict) and "scan_age_seconds" in scan,
                "scan_age_seconds": scan.get("scan_age_seconds") if isinstance(scan, dict) else None,
                "domains_rows": ((out.get("/api/v1/domains") or {}).get("body") or {}).get("rows")}
            pins = self.guest(PINS_SCRIPT, timeout=120)
            pins_text = (pins.stdout or "") + ("\n# stderr:\n" + pins.stderr if pins.stderr else "")
            self.ev.write_text(f"{self.step_dir}/installed-pins-{label}.txt", pins_text)
            checks["installed_pins"] = {"returncode": pins.returncode,
                                        "lines": [line for line in pins_text.splitlines()
                                                  if line.startswith(("==", "version=", "current=", "previous"))
                                                  or "bootstrap_release_" in line]}
            try:
                checks["ledger"] = self.schema_ledger(f"set15-{label}", ledger_version)
            except Exception as exc:  # noqa: BLE001 - recorded; the cell goes on
                checks["ledger"] = {"verdict": "inconclusive", "error": f"{type(exc).__name__}: {str(exc)[:300]}"}
        finally:
            for entry in self.current.get("calls", []):
                entry.pop("_parsed", None)
            self.current["finished_at"] = base.utc_now()
            self.record_json("readings.json", self.current)
        return "observed"

    def window15(self, checks: dict, cell_name: str, mode: str) -> str:
        self.current = {"section": "set15-browser-window", "node": self.node_name, "native_evidence": False,
                        "started_at": base.utc_now(), "calls": [], "checks": [], "natives": [], "notes": [],
                        "owner_actions": [], "lab_preparation": []}
        if not os.environ.get("SET12_HAND"):
            checks["reason"] = "no browser in this lab (SET12_HAND is not set)"
            return "skipped"
        record: dict = {}
        try:
            checks["leave_setup"] = self.leave_setup()
            first = self.snap14("probe-before-the-window", "read-probe14")
            self.s14["window_since"] = float(first.get("epoch") or time.time())
            checks["probe_before_the_window"] = t14.compact_probe(first)
            record = self.browser_window("ui", WINDOW_SECONDS, {"browser": BROWSER[cell_name], "cell": cell_name,
                                                                 "mode": mode, "preps": list(PREPS)})
            checks.update({k: v for k, v in record.items() if k != "requests"})
            checks["prep_count"] = len(record.get("requests") or [])
            self.record_json("browser-window.json", record)
            try:
                checks["probe_after_the_window"] = t14.compact_probe(self.snap14("probe-after-the-window", "read-probe14"))
            except Exception as exc:  # noqa: BLE001
                checks["end_readings_error"] = self.error_text(exc)
        finally:
            for entry in self.current.get("calls", []):
                entry.pop("_parsed", None)
            self.current["finished_at"] = base.utc_now()
            self.record_json("window.json", self.current)
        if record.get("skipped"):
            return "skipped"
        if record.get("not_finished"):
            raise base.StepInconclusive(record["not_finished"])
        self.ensure_session()
        return "observed"


class Set15Trial(Readings15, t14.Set14Trial):
    def execute(self) -> dict:
        self.step("preflight", self.preflight)
        self.step("origin", self.origin, needs=("preflight",))
        self.step("baseline-install", self.baseline_install, needs=("origin",))
        self.step("owner-login", self.owner_login, needs=("baseline-install",))
        self.step("license", self.license, needs=("owner-login",))
        self.step("setup", self.setup, needs=("license",))
        self.step("set15-readings", lambda checks: self.readings15(checks, "fresh", 44), needs=("setup",))
        self.step("set15-browser-window", lambda checks: self.window15(checks, self.settings.name, "fresh"), needs=("setup",))
        self.step(s6.PIN_END_STEP, self.pin_at_the_end6)
        self.step("collect", self.collect)
        self.tunnel.close()
        verdicts = [s["verdict"] for s in self.steps]
        result = {"schema": base.RESULT_SCHEMA, "cell_kind": CELL_KIND, "native_evidence": False,
                  "cell": dataclasses.asdict(self.settings),
                  "identity": {k: self.identity[k] for k in ("cell_id", "node", "vm_uuid")},
                  "request_id": None, "provenance": {"what": "set15: 1636b73f0 installed fresh; no update"},
                  "artifacts": {"baseline": {k: self.artifacts["baseline"][k] for k in ("version", "commit", "sha256")}},
                  "outcome": {"classification": "set15-measured", "final_status": None},
                  "setup": {"purpose": self.settings.purpose, "components": sorted(self.settings.components),
                            "waiting": self.state.get("setup_waiting")},
                  "browser": BROWSER[self.settings.name], "findings": self.state["findings"],
                  "steps": [{k: s.get(k) for k in ("name", "verdict", "reason", "started_at", "finished_at")} for s in self.steps],
                  "overall": base.overall(verdicts),
                  "note": "set15: a measurement on the fresh install; no update is started and no P0 row is judged. "
                          "The browser's own records (requests, card texts, pictures) are written on the host."}
        self.step_dir = "result"
        return self.ev.finalize_upd1(result)


class Set15UpdateTrial(Readings15, s6.Set6UpdateTrial):
    # borrowed as plain functions: set14's probe, set12's window and setup exit, set10's session check
    native14 = t14.Set14Trial.native14
    snap14 = t14.Set14Trial.snap14
    s14 = t14.Set14Trial.s14
    browser_window = s12.Set12Trial.browser_window
    leave_setup = s12.Set12Trial.leave_setup
    error_text = s12.Set12Mixin.error_text
    ensure_session = s10.D031.ensure_session

    def __init__(self, cell: Any, artifacts: dict, work_root: str, local_port: int) -> None:
        super().__init__(cell, artifacts, work_root, local_port)
        # set4/set5's PHP site (item 9) and mail (M10) sections are not part of this record (measured in set4/set5)
        self.additions = {"item9": False, "m10": False}

    def upload_helpers(self) -> dict:
        helpers = super().upload_helpers()
        # guest_set14_native.py loads its chain on the guest: set13 -> set12 -> set11 -> set10 -> set8 -> set4 ->
        # request_identity (set5 uploads the last two only with its item 9 or M10, which this record does not run)
        for name in ("guest_request_identity_native.py", "guest_set4_native.py", "guest_set8_native.py",
                     "guest_set10_native.py", "guest_set11_native.py", "guest_set12_native.py", "guest_set13_native.py",
                     t14.HELPER14):
            helpers[name] = self.lab.put_file(self.root, self.record, self.plan, self.node_name, HERE / name, name)[1]
        # the owner-update observer's closed set of labels gains (alpha.83, alpha.84): set15_fixture's copy
        helpers["guest_bound_worker.py (set15 generated copy)"] = self.lab.put_file(
            self.root, self.record, self.plan, self.node_name, fx15.bound_worker_copy(), "guest_bound_worker.py")[1]
        return helpers

    def prep(self, name: str) -> dict:
        n = self.s14["prep_count"] = self.s14.get("prep_count", 0) + 1
        if name == "probe":
            return t14.compact_probe(self.snap14(f"probe-{n}", "read-probe14"))
        if name == "journal":
            since = int(float(self.s14.get("window_since") or time.time() - 3600)) - 1
            result = self.guest("journalctl --no-pager -o short-iso-precise --since " + shlex.quote(f"@{since}") +
                                " -u celikpanel-panel.service -u celikpanel-agent.service | tail -n 600", timeout=120)
            name_ = f"{self.step_dir}/journal-window-{n}.txt"
            self.ev.write_text(name_, result.stdout or "")
            return {"file": name_, "lines": len((result.stdout or "").splitlines()), "returncode": result.returncode}
        return {"unknown": name}

    def step(self, name: str, function, *, needs: tuple = ()) -> str:
        if name == "collect":
            label = "after-the-update" if self.cell.variant == "good" else "after-the-return"
            base.Trial.step(self, f"set15-readings-{label}", lambda checks: self.readings15(checks, label, 44),
                            needs=("terminal",))
            base.Trial.step(self, "set15-browser-window", lambda checks: self.window15(checks, self.cell.name, label),
                            needs=("terminal",))
        return super().step(name, function, needs=needs)


def build_plan(name: str, artifacts: dict, work_root: str, local_port: int) -> dict:
    plan = {"schema": "celikpanel/set15-plan/v1", "native_evidence": False, "cell": name, "work_root": work_root,
            "local_port": local_port, "pinned_names": list(s6.PINNED_NAMES), "browser": BROWSER[name],
            "preparations": list(PREPS),
            "rule": "the driver calls only the Panel's HTTP API as the logged-in owner; every native fact is an SSH reading; "
                    "owner actions on the guest are recorded as such; no certificate authority and no licence service is "
                    "contacted by the driver"}
    if name in CELLS:
        settings = CELLS[name]
        plan.update(cell_kind=CELL_KIND, candidate={k: artifacts["baseline"][k] for k in ("version", "commit", "sha256")},
                    setup={"purpose": settings.purpose, "components": sorted(settings.components), "dns_mode": "external"},
                    steps=[s6.PIN_STEP, "preflight", "origin", "baseline-install", "owner-login", "license", "setup",
                           "set15-readings", "set15-browser-window", s6.PIN_END_STEP, "collect"])
    else:
        plan.update(cell_kind=UPDATE_KIND, baseline_ref=(artifacts.get("baseline_ref") or {}).get("ref"),
                    update=s6.build_plan(name, artifacts, work_root, local_port),
                    added_steps=["set15-readings-after-the-update|after-the-return", "set15-browser-window"],
                    not_run=["set4-php-site-before/after-the-update (item 9)", "M10 (mail)"])
    return plan


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    sub = parser.add_subparsers(dest="command", required=True)
    for name in ("plan", "run"):
        cmd = sub.add_parser(name)
        cmd.add_argument("--cell", required=True, choices=sorted(CELLS) + sorted(UPDATE_CELLS))
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
    labels = fx15.configure(document)
    wanted = fx15.FRESH_REF if args.cell in CELLS else fx15.ALPHA83_REF
    if labels.get("ref") != wanted:
        parser.error(f"cell {args.cell} needs an artifacts document with baseline_ref {wanted}, not {labels.get('ref')}")
    cell = CELLS[args.cell].cell if args.cell in CELLS else base.validate_cell(args.cell)
    if args.command == "plan":
        base.validate_cell_artifacts(document, cell, check_files=not args.dry_run)
        print(json.dumps(build_plan(args.cell, document, args.work_root, args.local_port), indent=2, sort_keys=True))
        return 0
    if not args.execute:
        parser.error("run mutates one registered disposable guest and requires --execute")
    base.validate_cell_artifacts(document, cell)
    if args.cell in CELLS:
        result = Set15Trial(CELLS[args.cell], document, args.work_root, args.local_port).execute()
    else:
        result = Set15UpdateTrial(cell, document, args.work_root, args.local_port).execute()
    print(json.dumps({"overall": result["overall"], "outcome": (result.get("outcome") or {}).get("classification"),
                      "steps": {s["name"]: s["verdict"] for s in result["steps"]}}, sort_keys=True))
    return 0 if result["overall"] != "failed" else 1


if __name__ == "__main__":
    raise SystemExit(main())
