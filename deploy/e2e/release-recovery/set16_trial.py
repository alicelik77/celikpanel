#!/usr/bin/env python3
"""set16: the pre-publication measurement of the v0.1.0-alpha.85 candidate (commit 89cf88397, first planned on 0bea00fa0; branch
fix/alpha85-setup-finish-wait), on disposable QEMU guests (lab.py), with a real headless Chrome on the host
(web/tools/browser-inspect/cold-load-set16.mjs, set12's browser window "ui").

``set16-<debian13|ubuntu>`` (``Set16Trial`` over ``set14c_trial.Set14cTrial``): the candidate labelled
v0.1.0-alpha.85 (a fixture commit over the source that changes only the release policy; the update cells' good
archive) installed FRESH (artifacts document from ``build-set16-artifacts.sh --fresh``; pinning, preflight, origin,
install, owner, licence, setup with purpose ``web``), then one reading step through the Panel's API (panel version,
licence access, the stored scan, the Domains list, setup) with the installed bootstrap pins and the schema ledger
read over SSH, then the browser window: the Domains list and the sidebar, the sidebar build id at 1280x800 and
390x844, one Dashboard load with a stale scan (System services and Mail stacks cards), the Components page fresh and
stale, the licence gate with the guest clock moved +90 s and -90 s, the Agent stopped (Components and Dashboard in
Turkish, Dashboard in English, then a Rescan after the Agent is back), three direct reads.

``upd1-<debian13|ubuntu>-<good|defective>`` (``Set16UpdateTrial`` over ``set6_trial.Set6UpdateTrial``), with an
artifacts document from ``build-set16-artifacts.sh --baseline-ref v0.1.0-alpha.84``: set6's owner update from the
alpha.84 tag rebuilt as a test-licence build to the candidate (good) or to a candidate that fails at start
(defective: set5's migrate-only defect, automatic return), without set4/set5's PHP-site and mail sections. The
terminal step's database comparison now carries the ``managed_site_files`` rows and both digests (a87ce0b97). After
``terminal``: the same API readings and ledger, then the browser window (sidebar, sidebar widths, the Dashboard with
its Mail stacks card, Components, reads).

Preparations the browser may ask for (fresh): set14b/c's ``probe`` (read-probe14b), ``clockshift_<p|m>N`` and
``ntp-on`` (lab preparation, recorded as such, undone at the end of the window), ``agent-stop``/``agent-start``
(owner actions), ``agent-reads``, ``scan-now``, ``journal``; (update) ``probe`` and ``journal``. The driver calls only
the Panel's HTTP API; every native fact is read over SSH. Every result carries ``native_evidence: false``. Nothing
here judges a P0 row.

  set16_trial.py plan --cell CELL --artifacts A.json --work-root /var/tmp/cp-release-drill-X [--dry-run]
  set16_trial.py run  --cell CELL --artifacts A.json --work-root /var/tmp/cp-release-drill-X --execute
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
import set15_trial as t15  # noqa: E402  (set14 chain, set15_fixture)
import set14c_trial as s14c  # noqa: E402
import set16_fixture as fx16  # noqa: E402  (after set15_fixture: replaces the alpha.84 profile and the origin copy)

t14, s14b = t15.t14, s14c.s14b
s12, s10, s6, sw, base = t15.s12, t15.s10, t15.s6, t15.sw, t15.base
CELL_KIND = "set16-fresh-install"
UPDATE_KIND = "set16-update"
CELLS = {
    "set16-debian13": sw.SettingsCell("set16-debian13", "debian13", "web", sw.WEB_PRESET, False),
    "set16-ubuntu": sw.SettingsCell("set16-ubuntu", "ubuntu", "web", sw.WEB_PRESET, False),
}
UPDATE_CELLS = ("upd1-debian13-good", "upd1-debian13-defective", "upd1-ubuntu-good", "upd1-ubuntu-defective")
BROWSER = {name: {"lang": "en", "viewport": "desktop"} for name in (*CELLS, *UPDATE_CELLS)}
FRESH_PREPS = ("probe", "clockshift_p90000", "clockshift_m180000", "ntp-on", "agent-stop", "agent-start",
               "agent-reads", "scan-now", "journal")
UPDATE_PREPS = ("probe", "journal")
WINDOW_SECONDS = 3 * 3600
LEDGER_VERSION = 44


class Readings16:
    """The set16 reading step and its browser window, shared by the fresh and the update trials."""

    def readings16(self, checks: dict, label: str) -> str:
        self.current = {"section": f"set16-readings-{label}", "node": self.node_name, "native_evidence": False,
                        "started_at": base.utc_now(), "calls": [], "checks": [], "natives": [], "notes": []}
        try:
            out = {}
            for what, path in t15.READS:
                entry = self.call(f"set16: {what} ({path}) {label}", "GET", path, None, timeout=120)
                out[path] = {"status": entry["status"], "at": entry["at"], "body": t15.brief_body(path, entry.get("_parsed"))}
            checks["api"] = out
            access = (out.get("/api/v1/license/access") or {}).get("body") or {}
            scan = (out.get("/api/v1/managed-services") or {}).get("body") or {}
            setup = (out.get("/api/v1/setup") or {}).get("body") or {}
            checks["fields"] = {
                "panel_version": (out.get("/api/v1/panel/version") or {}).get("body"),
                "access_has_valid_for_seconds": isinstance(access, dict) and "valid_for_seconds" in access,
                "valid_for_seconds": access.get("valid_for_seconds") if isinstance(access, dict) else None,
                "managed_services_has_scan_age_seconds": isinstance(scan, dict) and "scan_age_seconds" in scan,
                "scan_age_seconds": scan.get("scan_age_seconds") if isinstance(scan, dict) else None,
                "domains_rows": ((out.get("/api/v1/domains") or {}).get("body") or {}).get("rows"),
                "setup_status": setup.get("status") if isinstance(setup, dict) else None}
            pins = self.guest(t15.PINS_SCRIPT, timeout=120)
            pins_text = (pins.stdout or "") + ("\n# stderr:\n" + pins.stderr if pins.stderr else "")
            self.ev.write_text(f"{self.step_dir}/installed-pins-{label}.txt", pins_text)
            checks["installed_pins"] = {"returncode": pins.returncode,
                                        "lines": [line for line in pins_text.splitlines()
                                                  if line.startswith(("==", "version=", "current=", "previous"))
                                                  or "bootstrap_release_" in line]}
            try:
                checks["ledger"] = self.schema_ledger(f"set16-{label}", LEDGER_VERSION)
            except Exception as exc:  # noqa: BLE001 - recorded; the cell goes on
                checks["ledger"] = {"verdict": "inconclusive", "error": f"{type(exc).__name__}: {str(exc)[:300]}"}
            if label != "fresh":
                checks["site_file_ledger_digests"] = self.state.get("site_file_ledger_digests")
        finally:
            for entry in self.current.get("calls", []):
                entry.pop("_parsed", None)
            self.current["finished_at"] = base.utc_now()
            self.record_json("readings.json", self.current)
        return "observed"

    def window16(self, checks: dict, cell_name: str, mode: str, preps: tuple) -> str:
        self.current = {"section": "set16-browser-window", "node": self.node_name, "native_evidence": False,
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
                                                                 "mode": mode, "preps": list(preps)})
            checks.update({k: v for k, v in record.items() if k != "requests"})
            checks["prep_count"] = len(record.get("requests") or [])
            self.record_json("browser-window.json", record)
        finally:
            try:
                self.after_the_window(checks)
            except Exception as exc:  # noqa: BLE001
                checks["end_readings_error"] = self.error_text(exc)
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

    def after_the_window(self, checks: dict) -> None:
        checks["probe_after_the_window"] = t14.compact_probe(self.snap14("probe-after-the-window", "read-probe14"))


class Set16Trial(Readings16, s14c.Set14cTrial):
    def after_the_window(self, checks: dict) -> None:
        # A clock this harness moved is put back and the Agent started even when the browser stopped half-way
        # (set14b's and set14's ends of the window).
        restore: dict = {"shift_total_ms_before": self.s14b.get("shift_total_ms", 0)}
        try:
            total = self.s14b.get("shift_total_ms", 0)
            if total:
                restore["undo"] = self.clock("lab-clock-undo-at-the-end", shift_ms=-total)
            if self.s14b.get("ntp_restored") is False:
                restore["ntp_on"] = self.clock("lab-ntp-on-at-the-end", ntp=True)
            restore["probe"] = s14b.compact_probe(self.snap14b("probe-after-the-clock-check", "read-probe14b"))
        except Exception as exc:  # noqa: BLE001
            restore["error"] = self.error_text(exc)
        checks["clock_at_the_end"] = restore
        self.record_json("clock-at-the-end.json", restore)
        self.snap14("agent-start-at-the-end", "owner-agent14", action="start")
        super().after_the_window(checks)

    def execute(self) -> dict:
        self.step("preflight", self.preflight)
        self.step("origin", self.origin, needs=("preflight",))
        self.step("baseline-install", self.baseline_install, needs=("origin",))
        self.step("owner-login", self.owner_login, needs=("baseline-install",))
        self.step("license", self.license, needs=("owner-login",))
        self.step("setup", self.setup, needs=("license",))
        self.step("set16-readings", lambda checks: self.readings16(checks, "fresh"), needs=("setup",))
        self.step("set16-browser-window", lambda checks: self.window16(checks, self.settings.name, "fresh", FRESH_PREPS),
                  needs=("setup",))
        self.step(s6.PIN_END_STEP, self.pin_at_the_end6)
        self.step("collect", self.collect)
        self.tunnel.close()
        verdicts = [s["verdict"] for s in self.steps]
        result = {"schema": base.RESULT_SCHEMA, "cell_kind": CELL_KIND, "native_evidence": False,
                  "cell": dataclasses.asdict(self.settings),
                  "identity": {k: self.identity[k] for k in ("cell_id", "node", "vm_uuid")},
                  "request_id": None,
                  "provenance": {"what": f"set16: the alpha.85-labelled fixture over {fx16.SOURCE_COMMIT[:9]} installed fresh; "
                                         "no update"},
                  "artifacts": {"baseline": {k: self.artifacts["baseline"][k] for k in ("version", "commit", "sha256")}},
                  "outcome": {"classification": "set16-measured", "final_status": None},
                  "setup": {"purpose": self.settings.purpose, "components": sorted(self.settings.components),
                            "waiting": self.state.get("setup_waiting")},
                  "browser": BROWSER[self.settings.name], "findings": self.state["findings"],
                  "steps": [{k: s.get(k) for k in ("name", "verdict", "reason", "started_at", "finished_at")} for s in self.steps],
                  "overall": base.overall(verdicts),
                  "note": "set16: a measurement on the fresh install; no update is started and no P0 row is judged. "
                          "The browser's own records (requests, card texts, pictures) are written on the host."}
        self.step_dir = "result"
        return self.ev.finalize_upd1(result)


class Set16UpdateTrial(Readings16, s6.Set6UpdateTrial):
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
        for name in ("guest_request_identity_native.py", "guest_set4_native.py", "guest_set8_native.py",
                     "guest_set10_native.py", "guest_set11_native.py", "guest_set12_native.py", "guest_set13_native.py",
                     t14.HELPER14):
            helpers[name] = self.lab.put_file(self.root, self.record, self.plan, self.node_name, HERE / name, name)[1]
        # the owner-update observer's closed set of labels gains (alpha.84, alpha.85): set16_fixture's copy
        helpers["guest_bound_worker.py (set16 generated copy)"] = self.lab.put_file(
            self.root, self.record, self.plan, self.node_name, fx16.bound_worker_copy(), "guest_bound_worker.py")[1]
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
            base.Trial.step(self, f"set16-readings-{label}", lambda checks: self.readings16(checks, label),
                            needs=("terminal",))
            base.Trial.step(self, "set16-browser-window",
                            lambda checks: self.window16(checks, self.cell.name, label, UPDATE_PREPS),
                            needs=("terminal",))
        return super().step(name, function, needs=needs)


def ledger_summary(database_check: dict | None) -> dict:
    """What the record says of the terminal step's database comparison (owner_update_trial.compare_databases):
    whether managed_site_files came back, by the full and by the stable digest, and which columns changed."""
    check = database_check or {}
    ledger = check.get("site_file_ledger")
    if not isinstance(ledger, dict):
        return {"site_file_ledger": "not-recorded", "verdict": check.get("verdict"), "unexpected": check.get("unexpected")}
    columns = sorted({c for cols in (ledger.get("changed_columns_by_rowid") or {}).values() for c in cols})
    return {"verdict": check.get("verdict"), "unexpected": check.get("unexpected"),
            "volatile_columns_only": check.get("volatile_columns_only"),
            "full_equal": ledger.get("full_equal"), "stable_equal": ledger.get("stable_equal"),
            "changed_columns": columns, "only_observed_at_changed": columns == ["observed_at"],
            "rows_before": ledger.get("rows_before"), "rows_after": ledger.get("rows_after"),
            "rows_added": ledger.get("rows_added"), "rows_removed": ledger.get("rows_removed")}


def build_plan(name: str, artifacts: dict, work_root: str, local_port: int) -> dict:
    plan = {"schema": "celikpanel/set16-plan/v1", "native_evidence": False, "cell": name, "work_root": work_root,
            "local_port": local_port, "pinned_names": list(s6.PINNED_NAMES), "browser": BROWSER[name],
            "rule": "the driver calls only the Panel's HTTP API as the logged-in owner; every native fact is an SSH reading; "
                    "owner actions and lab preparation (the guest clock) on the guest are recorded as such; no "
                    "certificate authority and no licence service is contacted by the driver"}
    if name in CELLS:
        settings = CELLS[name]
        plan.update(cell_kind=CELL_KIND, candidate={k: artifacts["baseline"][k] for k in ("version", "commit", "sha256")},
                    setup={"purpose": settings.purpose, "components": sorted(settings.components), "dns_mode": "external"},
                    preparations=list(FRESH_PREPS),
                    steps=[s6.PIN_STEP, "preflight", "origin", "baseline-install", "owner-login", "license", "setup",
                           "set16-readings", "set16-browser-window", s6.PIN_END_STEP, "collect"])
    else:
        plan.update(cell_kind=UPDATE_KIND, baseline_ref=(artifacts.get("baseline_ref") or {}).get("ref"),
                    preparations=list(UPDATE_PREPS), update=s6.build_plan(name, artifacts, work_root, local_port),
                    added_steps=["set16-readings-after-the-update|after-the-return", "set16-browser-window"],
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
    labels = fx16.configure(document)
    wanted = fx16.FRESH_REF if args.cell in CELLS else fx16.ALPHA84_REF
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
        result = Set16Trial(CELLS[args.cell], document, args.work_root, args.local_port).execute()
    else:
        result = Set16UpdateTrial(cell, document, args.work_root, args.local_port).execute()
    print(json.dumps({"overall": result["overall"], "outcome": (result.get("outcome") or {}).get("classification"),
                      "steps": {s["name"]: s["verdict"] for s in result["steps"]}}, sort_keys=True))
    return 0 if result["overall"] != "failed" else 1


if __name__ == "__main__":
    raise SystemExit(main())
