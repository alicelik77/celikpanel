#!/usr/bin/env python3
"""set14: the fourteenth native record, on commit a5830dd3e (the Dashboard's "System services" card checks the
components by itself: ``POST /api/v1/managed-services/scan?max_age_seconds=300`` after a read whose stored scan is
missing or older than 300 s, only while the tab is visible, single-flight, at most once per window; a 409
``server_setup_busy``/``service_operation_busy`` is "a change is in progress" with one follow-up after the readiness
poll sees busy then ready; a failure is "Could not check: <reason>" with the "Scan services" button).

On disposable QEMU guests (lab.py): the code installed FRESH (set6's steps: pinning, preflight, origin, install, owner,
licence, setup), then one browser window (set12's handover, SET12_HAND) in which a real headless Chrome on the host
(web/tools/browser-inspect/cold-load-set14.mjs) loads the Panel's Dashboard and Components page and asks the driver
for these preparations, each recorded:

  probe          native reading (guest_set14_native.py read-probe14): the stored scan's ``scanned_at`` and age (the
                 Panel database opened read-only), the Agent's and the Panel's systemd state and CPU counters (cgroup
                 ``cpu.stat``, ``CPUUsageNSec``, ``/proc/<pid>/stat``), ``/proc/loadavg``
  agent-stop     owner action: ``systemctl stop celikpanel-agent.service``
  agent-start    owner action: ``systemctl start celikpanel-agent.service``
  slow-on        lab preparation: nginx's reload made to take 90 s (a drop-in ``ExecReload=/bin/sleep 90``)
  action-start   the owner reloads nginx from the Panel (``POST /api/v1/service/action``, the Services page's own call),
                 sent in a thread: it holds the Panel's change lease while the reload runs
  action-wait    the answer of that call (bounded wait)
  slow-off       lab preparation removed
  journal        the Panel's and the Agent's journal since the window opened

The driver calls only the Panel's HTTP API; every native fact is read over SSH; owner actions and lab preparation on
the guest are recorded as such. Every result carries ``native_evidence: false``. Nothing here judges a P0 row.

  set14_trial.py plan --cell set14-debian13 --artifacts A.json --work-root /var/tmp/cp-release-drill-X [--dry-run]
  set14_trial.py run  --cell set14-debian13 --artifacts A.json --work-root /var/tmp/cp-release-drill-X --execute
"""
from __future__ import annotations

import argparse
import base64
import dataclasses
import json
import os
from pathlib import Path
import sys
import threading
import time
from typing import Any

HERE = Path(__file__).resolve().parent
if str(HERE) not in sys.path:
    sys.path.insert(0, str(HERE))
import set13_trial as s13  # noqa: E402

s12, s10, s6, sw, base, fx = s13.s12, s13.s10, s13.s6, s13.sw, s13.base, s13.fx
HELPER14 = "guest_set14_native.py"
CELL_KIND = "set14-fresh-install"
# purpose "web": the Dashboard card needs installed components, not mail; the setup is shorter.
CELLS = {
    "set14-debian13": sw.SettingsCell("set14-debian13", "debian13", "web", sw.WEB_PRESET, False),
    "set14-ubuntu": sw.SettingsCell("set14-ubuntu", "ubuntu", "web", sw.WEB_PRESET, False),
    "set14-arch": sw.SettingsCell("set14-arch", "arch", "web", sw.WEB_PRESET, False),
}
# The browser's language and viewport per platform (handover "browser"): each card state in English on Debian and in
# Turkish on Ubuntu.
BROWSER = {
    "set14-debian13": {"lang": "en", "viewport": "desktop"},
    "set14-ubuntu": {"lang": "tr", "viewport": "phone"},
    "set14-arch": {"lang": "en", "viewport": "phone"},
}
SLOW_RELOAD_SECONDS = 90
ACTION_SERVICE = "nginx"
WINDOW_SECONDS = 4 * 3600
PREPS = ("probe", "agent-stop", "agent-start", "slow-on", "slow-off", "action-start", "action-wait", "journal")


def compact_probe(value: dict) -> dict:
    """What the browser needs of a probe (the full reading stays in the driver's record)."""
    units = value.get("units") or {}
    return {"at": value.get("at"), "epoch": value.get("epoch"), "scan_cache": value.get("scan_cache"),
            "units": {k: {f: (units.get(k) or {}).get(f) for f in ("ActiveState", "MainPID", "NRestarts", "CPUUsageNSec")}
                      for k in ("agent", "panel")},
            "cgroup_cpu": value.get("cgroup_cpu"), "proc_cpu": value.get("proc_cpu"), "loadavg": value.get("loadavg"),
            "nginx_dropin": (value.get("nginx_dropin") or {}).get("exists"), "file": value.get("_file")}


def prep_name(key: str) -> str:
    """The preparation a request names: ``<name>-<milliseconds>`` (set12's handover)."""
    return key.rsplit("-", 1)[0]


class Set14Trial(s13.Set13Trial):
    def upload_helpers(self) -> dict:
        helpers = super().upload_helpers()
        helpers[HELPER14] = self.lab.put_file(self.root, self.record, self.plan, self.node_name, HERE / HELPER14, HELPER14)[1]
        return helpers

    @property
    def s14(self) -> dict:
        return self.state.setdefault("set14", {})

    def native14(self, mode: str, **arguments: Any) -> dict:
        payload = base64.b64encode(json.dumps(arguments).encode()).decode()
        return self.helper(HELPER14, mode, "--args-b64", payload, timeout=420)

    def snap14(self, label: str, mode: str, **arguments: Any) -> dict:
        value = self.native14(mode, **arguments)
        self.native_sequence += 1
        name = f"native/{self.native_sequence:03d}-{s10.safe_name(label)}.json"
        self.record_json(name, value)
        self.current.setdefault("natives", []).append({"label": label, "mode": mode, "file": name,
                                                       "owner_action": mode.startswith("owner-")})
        value["_file"] = f"{self.step_dir}/{name}"
        return value

    # -- the preparations the browser asks for -------------------------------------------------------------------

    def prep(self, name: str) -> dict:
        n = self.s14["prep_count"] = self.s14.get("prep_count", 0) + 1
        if name == "probe":
            return compact_probe(self.snap14(f"probe-{n}", "read-probe14"))
        if name in ("agent-stop", "agent-start"):
            action = name.split("-", 1)[1]
            value = self.snap14(f"owner-agent-{action}-{n}", "owner-agent14", action=action)
            self.current.setdefault("owner_actions", []).append({"label": name, "at": value.get("at"), "file": value["_file"]})
            return {k: value.get(k) for k in ("command", "returncode", "waited_seconds", "at", "epoch")} | {
                "before": (value.get("before") or {}).get("ActiveState"), "after": (value.get("after") or {}).get("ActiveState"),
                "main_pid_after": (value.get("after") or {}).get("MainPID")}
        if name in ("slow-on", "slow-off"):
            value = self.snap14(f"lab-{name}-{n}", "lab-slow-reload14",
                                **({"remove": True} if name == "slow-off" else {"seconds": SLOW_RELOAD_SECONDS}))
            self.current.setdefault("lab_preparation", []).append({"label": name, "file": value["_file"]})
            return {k: value.get(k) for k in ("daemon_reload_returncode", "nginx", "removed", "at", "epoch")}
        if name == "action-start":
            return self.action_start()
        if name == "action-wait":
            return self.action_wait()
        if name == "journal":
            since = float(self.s14.get("window_since") or time.time() - 3600)
            window = self.window12(f"ui-journal-{n}", since - 1)
            return {"file": window.get("_file"), "units": sorted((window.get("by_unit") or {}).keys())}
        return {"unknown": name}

    def action_start(self) -> dict:
        if self.s14.get("action_thread"):
            return {"refused": "an action was already started in this window"}
        holder: dict = {}
        self.s14["action_holder"] = holder

        def reload_nginx() -> None:
            started = time.time()
            holder["started_epoch"] = started
            try:
                entry = self.call("ui: the owner reloads nginx from the Panel (POST /api/v1/service/action)", "POST",
                                  "/api/v1/service/action", {"name": ACTION_SERVICE, "action": "reload"}, timeout=400)
                holder["status"] = entry["status"]
                holder["answer"] = entry.get("answer") or (entry["_parsed"] if isinstance(entry["_parsed"], dict) else None)
            except Exception as exc:  # noqa: BLE001 - recorded and handed to the browser
                holder["error"] = self.error_text(exc)
            holder["seconds"] = round(time.time() - started, 2)
            holder["ended_epoch"] = time.time()

        thread = threading.Thread(target=reload_nginx, daemon=True)
        self.s14["action_thread"] = thread
        thread.start()
        time.sleep(3)
        return {"started_epoch": holder.get("started_epoch"), "service": ACTION_SERVICE, "action": "reload",
                "still_running_after_3s": thread.is_alive()}

    def action_wait(self) -> dict:
        thread = self.s14.get("action_thread")
        if not thread:
            return {"refused": "no action was started"}
        thread.join(timeout=420)
        holder = self.s14.get("action_holder") or {}
        return {k: holder.get(k) for k in ("status", "answer", "error", "seconds", "started_epoch", "ended_epoch")} | {
            "finished": not thread.is_alive()}

    # -- the browser window --------------------------------------------------------------------------------------

    def window14(self, checks: dict) -> str:
        self.current = {"section": "set14-browser-window", "node": self.node_name, "native_evidence": False,
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
            checks["probe_before_the_window"] = compact_probe(first)
            record = self.browser_window("ui", WINDOW_SECONDS, {"browser": BROWSER[self.settings.name],
                                                                 "cell": self.settings.name, "preps": list(PREPS)})
            checks.update({k: v for k, v in record.items() if k != "requests"})
            checks["prep_count"] = len(record.get("requests") or [])
            self.record_json("browser-window.json", record)
            try:
                checks["probe_after_the_window"] = compact_probe(self.snap14("probe-after-the-window", "read-probe14"))
                for label, mode, arguments in (("slow-off-at-the-end", "lab-slow-reload14", {"remove": True}),
                                               ("agent-start-at-the-end", "owner-agent14", {"action": "start"})):
                    self.snap14(label, mode, **arguments)
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

    # -- the cell ---------------------------------------------------------------------------------------------------

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
                  "request_id": None, "provenance": {"what": "set14: a5830dd3e installed fresh; no update"},
                  "artifacts": {"baseline": {k: self.artifacts["baseline"][k] for k in ("version", "commit", "sha256")}},
                  "outcome": {"classification": "set14-measured", "final_status": None},
                  "setup": {"purpose": self.settings.purpose, "components": sorted(self.settings.components),
                            "waiting": self.state.get("setup_waiting")},
                  "browser": BROWSER[self.settings.name], "findings": self.state["findings"],
                  "steps": [{k: s.get(k) for k in ("name", "verdict", "reason", "started_at", "finished_at")} for s in self.steps],
                  "overall": base.overall(verdicts),
                  "note": "set14: a measurement on the fresh install; no update is started and no P0 row is judged. "
                          "The browser's own records (requests, card texts, pictures) are written on the host."}
        self.step_dir = "result"
        return self.ev.finalize_upd1(result)


def build_plan(name: str, artifacts: dict, work_root: str, local_port: int) -> dict:
    settings = CELLS[name]
    return {"schema": "celikpanel/set14-plan/v1", "native_evidence": False, "cell": name, "work_root": work_root,
            "local_port": local_port, "pinned_names": list(s6.PINNED_NAMES), "cell_kind": CELL_KIND,
            "candidate": {k: artifacts["baseline"][k] for k in ("version", "commit", "sha256")},
            "setup": {"purpose": settings.purpose, "components": sorted(settings.components), "dns_mode": "external"},
            "browser": BROWSER[name], "preparations": list(PREPS),
            "steps": [s6.PIN_STEP, "preflight", "origin", "baseline-install", "owner-login", "license", "setup",
                      "set14-browser-window", s6.PIN_END_STEP, "collect"],
            "rule": "the driver calls only the Panel's HTTP API as the logged-in owner; every native fact is an SSH reading; "
                    "owner actions and lab preparation on the guest are recorded as such; no certificate authority and no "
                    "licence service is contacted by the driver"}


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
    result = Set14Trial(CELLS[args.cell], document, args.work_root, args.local_port).execute()
    print(json.dumps({"overall": result["overall"], "cell_kind": CELL_KIND,
                      "steps": {s["name"]: s["verdict"] for s in result["steps"]}}, sort_keys=True))
    return 0 if result["overall"] != "failed" else 1


if __name__ == "__main__":
    raise SystemExit(main())
