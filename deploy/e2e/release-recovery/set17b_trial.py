#!/usr/bin/env python3
"""set17b: the update page through a LONG restart outage (at least 3 minutes), as on the owner's real server, on
disposable QEMU guests (lab.py), with a real headless Chrome on the host (web/tools/browser-inspect/cold-load-set17b.mjs).

Set17 (2026-10-15) measured outages of 26-35 s, below the licence access validity, so its D1 (the licence gate's own
dialog over the update's restart) appeared once by chance and the session gate's PANEL_STARTING path never. Set17b
repeats set17's cell 4b with the alpha.86 candidate rebuilt from a later source commit (``SOURCE_COMMIT`` below, the
D1 and session-gate changes) and lengthens the outage on the lab guest only:

``set17b-upd86-debian13-good`` (twice, runs a and b): the candidate installed fresh by the real installer (set up
``web_mail``, one static site seeded), then the owner's update in the browser to a fixture commit labelled
v0.1.0-alpha.87 (release policy only), artifacts document from ``build-set17-artifacts.sh --second``.
``set17b-upd1-debian13-good-control``: the alpha.85 tag rebuilt as a test-licence build, installed fresh, then the
owner's update in the browser to the candidate (artifacts document from ``build-set17-artifacts.sh --baseline-ref
v0.1.0-alpha.85``): alpha.85's own page under the same outage.

LAB PREPARATION (not a product change): right after the owner's start, ``guest_set17b_restart_delay.py`` is started as
a transient unit on the guest for the page's own request id. It holds the update's own worker (cgroup freeze, no
signal) for ``DELAY_SECONDS`` once its release transaction turns ``completion.pending`` (the candidate installed, the
Panel stopped, before the controlled starts), then thaws it; see that file for why no drop-in is used (the updater refuses any drop-in besides its
own guard). Its event file is read back after the terminal state (step ``set17b-restart-delay``), with the transient
unit's final state and the Panel unit's drop-in paths.

Every other step is set17's (``set17_trial.Set17UpdateTrial``: the owner's press in the browser, the page's own start
held until the observer is armed, set16's and set6's update steps), except that set17's post-update browser window
("ui", the cell-5 checks) is not opened: only the update page is observed. Every result carries
``native_evidence: false``. Nothing here judges a P0 row.

  set17b_trial.py plan --cell CELL --artifacts A.json --work-root /var/tmp/cp-release-drill-X [--dry-run]
  set17b_trial.py run  --cell CELL --artifacts A.json --work-root /var/tmp/cp-release-drill-X --execute
"""
from __future__ import annotations

import argparse
import json
from pathlib import Path
import shlex
import sys
from typing import Any

HERE = Path(__file__).resolve().parent
if str(HERE) not in sys.path:
    sys.path.insert(0, str(HERE))
import set17_trial as t17  # noqa: E402  (set16/set15/set14 chain and set17_fixture)

fx17, s16, base, out = t17.fx17, t17.s16, t17.base, t17.out
SOURCE_COMMIT = "ecd2a5f088d676f7cc8a8e571ed3c903ea951f3e"   # set17b (2026-10-15): HEAD of fix/alpha86-update-restart-notice
fx17.SOURCE_COMMIT = SOURCE_COMMIT   # set17_fixture's provenance texts name the source; no other use
HELPER = "guest_set17b_restart_delay.py"
DELAY_SECONDS = 150
HELPER_LIMIT_SECONDS = 900
CELLS = {
    "set17b-upd86-debian13-good": ("upd1-debian13-good", fx17.SECOND_REF),
    "set17b-upd1-debian13-good-control": ("upd1-debian13-good", fx17.ALPHA85_REF),
}
for _name, (_cell, _ref) in CELLS.items():
    t17.UPDATE_CELLS[_name] = (_cell, _ref, True)
    t17.BROWSER[_name] = {"lang": "en", "viewport": "desktop"}


class Set17bUpdateTrial(t17.Set17UpdateTrial):
    def __init__(self, cell: Any, artifacts: dict, work_root: str, local_port: int, *, name: str) -> None:
        super().__init__(cell, artifacts, work_root, local_port, name=name, page_window=True)
        self.s17b: dict = {}

    def upload_helpers(self) -> dict:
        helpers = super().upload_helpers()
        helpers[f"{HELPER} (set17b lab preparation)"] = self.lab.put_file(
            self.root, self.record, self.plan, self.node_name, HERE / HELPER, HELPER)[1]
        return helpers

    def arm(self, checks: dict) -> str:
        verdict = super().arm(checks)
        checks["set17b_restart_delay_start"] = self.start_delay()
        return verdict

    def start_delay(self) -> dict:
        """Lab preparation: the transient unit that holds this update's worker at completion-pending for DELAY_SECONDS."""
        rid = self.state.get("request_id")
        if not rid or "start_answer" not in self.s17:
            record = {"started": False, "why": "the owner's start in the browser was not adopted; no delay is armed"}
            self.s17b["start"] = record
            return record
        unit = f"celikpanel-lab-set17b-restart-delay-{rid}.service"
        worker = f"celikpanel-self-update-{rid}.service"
        argv = ["systemd-run", f"--unit={unit}", "--no-block", f"--property=RuntimeMaxSec={HELPER_LIMIT_SECONDS + 300}",
                "--property=TimeoutStopSec=30", "--property=UMask=0077",
                f"--property=ExecStopPost=-/usr/bin/systemctl thaw {worker}",
                "python3", "-I", f"{out.PRIVATE}/{HELPER}", "--operation-id", rid,
                "--delay-seconds", str(DELAY_SECONDS), "--limit-seconds", str(HELPER_LIMIT_SECONDS)]
        try:
            result = self.guest(shlex.join(argv), timeout=30)
            record = {"started": result.returncode == 0, "at": base.utc_now(), "unit": unit, "worker": worker,
                      "argv": argv, "returncode": result.returncode, "stderr": (result.stderr or "")[-400:],
                      "what": "lab preparation (not a product change): hold the update's own worker once its transaction is "
                              f"completion-pending, for {DELAY_SECONDS} s while the Panel is stopped"}
        except Exception as exc:  # noqa: BLE001 - recorded; the update goes on unchanged
            record = {"started": False, "at": base.utc_now(), "unit": unit, "why": self.error_text(exc)}
        self.s17b["start"] = record
        return record

    def delay_record(self, checks: dict) -> str:
        """Read-only: the helper's events, its unit's end state, the worker's freezer state, the Panel's drop-ins.
        (set17b run a: the reading ran under the guard's ``set -eu`` and ended at the first absent drop-in directory;
        it now starts with ``set +e``.)"""
        rid = self.state.get("request_id")
        start = self.s17b.get("start") or {}
        checks["start"] = start
        if not rid or not start.get("started"):
            raise base.StepInconclusive(f"no restart delay was armed: {start}")
        unit, worker = start["unit"], start["worker"]
        raw = self.lab_event_bytes(f"set17b-restart-delay-{rid}.jsonl")
        self.ev.write_text(f"{self.step_dir}/set17b-restart-delay-events.jsonl", raw.decode("utf-8", "replace"))
        events = [json.loads(line) for line in raw.splitlines() if line.strip()]
        reading = ("set +e; for u in " + shlex.quote(unit) + " " + shlex.quote(worker) + "; do echo \"== $u\"; "
                   "systemctl show \"$u\" --no-pager -p LoadState -p ActiveState -p SubState -p Result "
                   "-p FreezerState -p ExecMainStatus 2>&1; done; "
                   "echo '== celikpanel-panel.service'; systemctl show celikpanel-panel.service --no-pager "
                   "-p ActiveState -p DropInPaths -p TimeoutStartUSec 2>&1; "
                   "echo '== drop-in directories of the Panel unit'; "
                   "ls -la /etc/systemd/system/celikpanel-panel.service.d /run/systemd/system/celikpanel-panel.service.d "
                   "/run/systemd/system.control/celikpanel-panel.service.d 2>&1; "
                   "echo '== transient lab units'; systemctl list-units --all --no-pager --plain 'celikpanel-lab-set17b-*' 2>&1; true")
        before = self.guest(reading, timeout=60)
        self.ev.write_text(f"{self.step_dir}/units-at-collection.txt", (before.stdout or "") + (before.stderr or ""))
        kinds = [e.get("event") for e in events]
        if "released" not in kinds and "ActiveState=active" in (before.stdout or "").split("== " + worker)[0]:
            stopped = self.guest(f"systemctl stop {shlex.quote(unit)}; systemctl show {shlex.quote(worker)} --no-pager "
                                 "-p ActiveState -p FreezerState", timeout=60)
            checks["helper_stopped_by_the_driver"] = {"returncode": stopped.returncode, "stdout": stopped.stdout[-400:]}
        pick = lambda name: next((e for e in events if e.get("event") == name), None)  # noqa: E731
        seen, frozen, thawed = pick("completion_pending_seen"), pick("frozen"), pick("thawed")
        held = [e for e in events if e.get("event") == "held"]
        after = [e for e in events if e.get("event") == "after_thaw"]
        panel_up = next((e for e in after if e.get("panel") == "active"), None)
        checks.update(
            events=len(events), kinds=kinds,
            completion_pending_seen=seen and {k: seen.get(k) for k in ("at", "epoch", "panel", "agent", "worker", "markers")},
            frozen=frozen and {k: frozen.get(k) for k in ("at", "epoch", "after_marker_ms", "panel", "agent", "worker_freezer", "markers")},
            thawed=thawed and {k: thawed.get(k) for k in ("at", "epoch", "reason", "returncode", "panel", "agent", "worker_freezer")},
            held_seconds=round(thawed["epoch"] - frozen["epoch"], 1) if frozen and thawed else None,
            panel_states_while_held=sorted({e.get("panel") for e in held}),
            agent_states_while_held=sorted({e.get("agent") for e in held}),
            panel_active_after_thaw=panel_up and {k: panel_up.get(k) for k in ("at", "epoch", "after_thaw_s")},
            released=pick("released"))
        self.record_json("set17b-restart-delay.json", checks)
        if not (frozen and thawed):
            raise base.StepInconclusive(f"the restart delay did not hold the worker: {kinds}")
        return "observed"

    def step(self, name: str, function, *, needs: tuple = ()) -> str:
        if name == "collect":
            label = "after-the-update" if self.cell.variant == "good" else "after-the-return"
            base.Trial.step(self, f"set17-readings-{label}", lambda checks: self.readings17(checks, label),
                            needs=("terminal",))
            base.Trial.step(self, "set17b-restart-delay", self.delay_record, needs=("arm",))
        # set17's post-update browser window ("ui") is not opened in set17b.
        return super(s16.Set16UpdateTrial, self).step(name, function, needs=needs)


def build_plan(name: str, artifacts: dict, work_root: str, local_port: int) -> dict:
    plan = t17.build_plan(name, artifacts, work_root, local_port)
    plan.update(schema="celikpanel/set17b-plan/v1", source_commit=SOURCE_COMMIT,
                added_steps=["set17-readings-after-the-update", "set17b-restart-delay"],
                not_run=list(plan.get("not_run") or []) + ["set17's post-update browser window (ui)"],
                lab_preparation={"helper": HELPER, "delay_seconds": DELAY_SECONDS, "limit_seconds": HELPER_LIMIT_SECONDS,
                                 "what": "hold the update's own worker (cgroup freeze, no signal) once its release transaction is "
                                         "completion-pending, while the Panel is stopped, then thaw it"})
    return plan


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
    labels = fx17.configure(document)
    base_cell, wanted = CELLS[args.cell]
    if labels.get("ref") != wanted:
        parser.error(f"cell {args.cell} needs an artifacts document with baseline_ref {wanted}, not {labels.get('ref')}")
    if document.get("source_head") != SOURCE_COMMIT and (document.get("baseline_ref") or {}).get("tag_commit") != SOURCE_COMMIT:
        parser.error(f"the artifacts document was not built from the set17b source {SOURCE_COMMIT}")
    cell = base.validate_cell(base_cell)
    if args.command == "plan":
        base.validate_cell_artifacts(document, cell, check_files=not args.dry_run)
        print(json.dumps(build_plan(args.cell, document, args.work_root, args.local_port), indent=2, sort_keys=True))
        return 0
    if not args.execute:
        parser.error("run mutates one registered disposable guest and requires --execute")
    base.validate_cell_artifacts(document, cell)
    result = Set17bUpdateTrial(cell, document, args.work_root, args.local_port, name=args.cell).execute()
    print(json.dumps({"overall": result["overall"], "outcome": (result.get("outcome") or {}).get("classification"),
                      "steps": {s["name"]: s["verdict"] for s in result["steps"]}}, sort_keys=True))
    return 0 if result["overall"] != "failed" else 1


if __name__ == "__main__":
    raise SystemExit(main())
