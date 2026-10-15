#!/usr/bin/env python3
"""set17: the pre-publication measurement of the v0.1.0-alpha.86 candidate (commit 313f3eea2, branch
fix/alpha86-update-restart-notice), on disposable QEMU guests (lab.py), with a real headless Chrome on the host
(web/tools/browser-inspect/cold-load-set17.mjs, set12's browser window protocol).

``set17-debian13`` (``Set17Trial`` over ``set14c_trial.Set14cTrial``): the candidate (the source commit itself, its
own policy v0.1.0-alpha.86 / 86) installed FRESH (artifacts document from ``build-set17-artifacts.sh --second``;
pinning, preflight, origin, install, owner, licence, setup with purpose ``web``), one reading step through the
Panel's API with the installed bootstrap pins and the schema ledger read over SSH, then the browser window "ui":
the sidebar build id, Components / Dashboard / Store in English and Turkish, a component's own page on an
unverifiable stored record (lab preparation ``unverifiable-record``), three direct reads.

``upd1-<debian13|ubuntu>-<good|defective>`` (``Set17UpdateTrial`` over ``set16_trial.Set16UpdateTrial``), with an
artifacts document from ``build-set17-artifacts.sh --baseline-ref v0.1.0-alpha.85``: set6's owner update from the
alpha.85 tag rebuilt as a test-licence build to the candidate (good) or to a candidate that fails at start
(defective: set5's migrate-only defect, automatic return). ``upd86-debian13-good``: the same owner update with the
``--second`` document: the candidate installed as the baseline and updated to a fixture commit labelled
v0.1.0-alpha.87 (policy only), so that the candidate's OWN update page is seen during a real restart.

In the good cells the update is started by the owner in the browser (window "page"): the page's own POST is held
by the browser until the driver has armed its read-only observer for the page's own request id (``arm`` is split:
``page-ready`` does the read-only checks and the update check before the press, ``arm_<id>`` arms the observer for
that id), then released unchanged; ``owner-start`` adopts the page's start answer instead of sending its own. Every
other step is set16's / set6's. After ``terminal``: the readings and the browser window "ui".

Preparations the browser may ask for: ``probe``, ``journal``, (fresh) ``unverifiable-record`` (lab preparation:
the stored component record's data is replaced by bytes that do not decode, so the Panel answers
``SERVICE_STATE_UNVERIFIED``; recorded as such); (page) ``page-ready``, ``arm_<32 hex>``. The driver calls only the
Panel's HTTP API; every native fact is read over SSH. Every result carries ``native_evidence: false``. Nothing here
judges a P0 row.

  set17_trial.py plan --cell CELL --artifacts A.json --work-root /var/tmp/cp-release-drill-X [--dry-run]
  set17_trial.py run  --cell CELL --artifacts A.json --work-root /var/tmp/cp-release-drill-X --execute
"""
from __future__ import annotations

import argparse
import dataclasses
import json
import os
from pathlib import Path
import re
import shlex
import sys
import time
from typing import Any

HERE = Path(__file__).resolve().parent
if str(HERE) not in sys.path:
    sys.path.insert(0, str(HERE))
import set16_trial as s16  # noqa: E402  (set15/set14 chain, set16_fixture)
import set17_fixture as fx17  # noqa: E402  (after set16_fixture: replaces the alpha.85 profile and the origin copy)
import set11_trial as s11  # noqa: E402  (loaded by the chain already: the handover root)

t15, s14c, s14b, t14 = s16.t15, s16.s14c, s16.s14b, s16.t14
s12, s10, s6, sw, base = s16.s12, s16.s10, s16.s6, s16.sw, s16.base
out = fx17.out
CELL_KIND = "set17-fresh-install"
UPDATE_KIND = "set17-update"
CELLS = {"set17-debian13": sw.SettingsCell("set17-debian13", "debian13", "web", sw.WEB_PRESET, False)}
# cell name -> (owner_update_trial cell, artifacts document kind, update page window)
UPDATE_CELLS = {
    "upd1-debian13-good": ("upd1-debian13-good", fx17.ALPHA85_REF, True),
    "upd1-debian13-defective": ("upd1-debian13-defective", fx17.ALPHA85_REF, False),
    "upd1-ubuntu-good": ("upd1-ubuntu-good", fx17.ALPHA85_REF, True),
    "upd1-ubuntu-defective": ("upd1-ubuntu-defective", fx17.ALPHA85_REF, False),
    "upd86-debian13-good": ("upd1-debian13-good", fx17.SECOND_REF, True),
}
BROWSER = {name: {"lang": "en", "viewport": "desktop"} for name in (*CELLS, *UPDATE_CELLS)}
FRESH_PREPS = ("probe", "journal", "unverifiable-record")
UPDATE_PREPS = ("probe", "journal")
PAGE_PREPS = ("page-ready", "arm_<request id>")
WINDOW_SECONDS = 3 * 3600
PAGE_WINDOW_SECONDS = 3600
ARM_WAIT_SECONDS = 9
LEDGER_VERSION = 44
RID = re.compile(r"[0-9a-f]{32}")
UNVERIFIABLE_SCRIPT = r"""python3 -I - <<'CP_SET17_UNVERIFIABLE'
import json, sqlite3, time
db = sqlite3.connect('file:/var/lib/celikpanel/celikpanel.db', uri=True, timeout=30)
db.execute('PRAGMA busy_timeout=30000')
before = db.execute('SELECT length(data), scanned_at, substr(data, 1, 16) FROM service_scan_cache WHERE id = 1').fetchone()
changed = 0
if before is not None:
    with db:
        changed = db.execute("UPDATE service_scan_cache SET data = 'set17-lab-preparation: bytes that do not decode' WHERE id = 1").rowcount
after = db.execute('SELECT length(data), scanned_at FROM service_scan_cache WHERE id = 1').fetchone()
print(json.dumps({'epoch': time.time(), 'row_before': None if before is None else {'data_length': before[0], 'scanned_at': before[1], 'data_head_is_json': (before[2] or '').lstrip().startswith(('{', '['))},
                  'rows_changed': changed, 'row_after': None if after is None else {'data_length': after[0], 'scanned_at': after[1]},
                  'what': 'lab preparation: the stored component record (service_scan_cache id 1, data column) replaced by bytes that do not decode; scanned_at unchanged'}))
CP_SET17_UNVERIFIABLE
"""


class Readings17(s16.Readings16):
    """set16's reading step and browser window, with set17's names, browser and preparations."""

    def readings17(self, checks: dict, label: str) -> str:
        self.current = {"section": f"set17-readings-{label}", "node": self.node_name, "native_evidence": False,
                        "started_at": base.utc_now(), "calls": [], "checks": [], "natives": [], "notes": []}
        try:
            api = {}
            for what, path in t15.READS:
                entry = self.call(f"set17: {what} ({path}) {label}", "GET", path, None, timeout=120)
                api[path] = {"status": entry["status"], "at": entry["at"], "body": t15.brief_body(path, entry.get("_parsed"))}
            checks["api"] = api
            access = (api.get("/api/v1/license/access") or {}).get("body") or {}
            scan = (api.get("/api/v1/managed-services") or {}).get("body") or {}
            setup = (api.get("/api/v1/setup") or {}).get("body") or {}
            checks["fields"] = {
                "panel_version": (api.get("/api/v1/panel/version") or {}).get("body"),
                "access_has_valid_for_seconds": isinstance(access, dict) and "valid_for_seconds" in access,
                "managed_services_has_scan_age_seconds": isinstance(scan, dict) and "scan_age_seconds" in scan,
                "domains_rows": ((api.get("/api/v1/domains") or {}).get("body") or {}).get("rows"),
                "setup_status": setup.get("status") if isinstance(setup, dict) else None}
            pins = self.guest(t15.PINS_SCRIPT, timeout=120)
            pins_text = (pins.stdout or "") + ("\n# stderr:\n" + pins.stderr if pins.stderr else "")
            self.ev.write_text(f"{self.step_dir}/installed-pins-{label}.txt", pins_text)
            checks["installed_pins"] = {"returncode": pins.returncode,
                                        "lines": [line for line in pins_text.splitlines()
                                                  if line.startswith(("==", "version=", "current=", "previous"))
                                                  or "bootstrap_release_" in line]}
            try:
                checks["ledger"] = self.schema_ledger(f"set17-{label}", LEDGER_VERSION)
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

    def window17(self, checks: dict, cell_name: str, mode: str, preps: tuple) -> str:
        self.current = {"section": "set17-browser-window", "node": self.node_name, "native_evidence": False,
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

    def unverifiable_record(self) -> dict:
        """Lab preparation (fresh cell): the stored component record's data replaced by bytes that do not decode."""
        result = self.guest(UNVERIFIABLE_SCRIPT, timeout=120)
        try:
            value = json.loads((result.stdout or "").strip().splitlines()[-1])
        except (ValueError, IndexError):
            value = {"stdout": (result.stdout or "")[-600:], "stderr": (result.stderr or "")[-600:]}
        value["returncode"] = result.returncode
        self.current.setdefault("lab_preparation", []).append({"at": base.utc_now(), "what": "unverifiable-record",
                                                                "result": value})
        return value


class Set17Trial(Readings17, s14c.Set14cTrial):
    def prep(self, name: str) -> dict:
        if name == "unverifiable-record":
            self.s14["prep_count"] = self.s14.get("prep_count", 0) + 1
            return self.unverifiable_record()
        return super().prep(name)

    def execute(self) -> dict:
        self.step("preflight", self.preflight)
        self.step("origin", self.origin, needs=("preflight",))
        self.step("baseline-install", self.baseline_install, needs=("origin",))
        self.step("owner-login", self.owner_login, needs=("baseline-install",))
        self.step("license", self.license, needs=("owner-login",))
        self.step("setup", self.setup, needs=("license",))
        self.step("set17-readings", lambda checks: self.readings17(checks, "fresh"), needs=("setup",))
        self.step("set17-browser-window", lambda checks: self.window17(checks, self.settings.name, "fresh", FRESH_PREPS),
                  needs=("setup",))
        self.step(s6.PIN_END_STEP, self.pin_at_the_end6)
        self.step("collect", self.collect)
        self.tunnel.close()
        verdicts = [s["verdict"] for s in self.steps]
        result = {"schema": base.RESULT_SCHEMA, "cell_kind": CELL_KIND, "native_evidence": False,
                  "cell": dataclasses.asdict(self.settings),
                  "identity": {k: self.identity[k] for k in ("cell_id", "node", "vm_uuid")},
                  "request_id": None,
                  "provenance": {"what": f"set17: the source commit {fx17.SOURCE_COMMIT[:9]} (its own policy v0.1.0-alpha.86) "
                                         "installed fresh; no update"},
                  "artifacts": {"baseline": {k: self.artifacts["baseline"][k] for k in ("version", "commit", "sha256")}},
                  "outcome": {"classification": "set17-measured", "final_status": None},
                  "setup": {"purpose": self.settings.purpose, "components": sorted(self.settings.components),
                            "waiting": self.state.get("setup_waiting")},
                  "browser": BROWSER[self.settings.name], "findings": self.state["findings"],
                  "steps": [{k: s.get(k) for k in ("name", "verdict", "reason", "started_at", "finished_at")} for s in self.steps],
                  "overall": base.overall(verdicts),
                  "note": "set17: a measurement on the fresh install; no update is started and no P0 row is judged. "
                          "The browser's own records (requests, page texts, pictures) are written on the host."}
        self.step_dir = "result"
        return self.ev.finalize_upd1(result)


class Set17UpdateTrial(Readings17, s16.Set16UpdateTrial):
    def __init__(self, cell: Any, artifacts: dict, work_root: str, local_port: int, *, name: str, page_window: bool) -> None:
        super().__init__(cell, artifacts, work_root, local_port)
        self.set17_name = name
        self.page_window = page_window
        self.s17: dict = {}

    def upload_helpers(self) -> dict:
        helpers = super().upload_helpers()
        # the owner-update observer's closed set of labels gains (alpha.85, alpha.86) and (alpha.86, alpha.87)
        helpers.pop("guest_bound_worker.py (set16 generated copy)", None)
        helpers["guest_bound_worker.py (set17 generated copy)"] = self.lab.put_file(
            self.root, self.record, self.plan, self.node_name, fx17.bound_worker_copy(), "guest_bound_worker.py")[1]
        return helpers

    # -- the update page window: the owner starts the update in the browser -------------------------------------

    def prep(self, name: str) -> dict:
        if name == "page-ready":
            return self.page_ready()
        if name.startswith("arm_"):
            return self.page_arm(name[len("arm_"):])
        return super().prep(name)

    def page_ready(self) -> dict:
        """arm()'s first half (owner_update_trial), before the owner presses Check: the read-only inspection, the
        origin proved again, GET update/check validated against the sealed candidate. No observer, no start."""
        checks = self.s17.setdefault("arm_checks", {})
        try:
            if self.owner_waits() and self.scenario() != "busy-start":
                checks["owner_waited_for_idle"] = self.wait_host_package_manager_idle()
            if self.pk_on():
                checks["packagekit_at_arm"] = self.pk_observation("arm-before-check")
            self.inspect("before-check")
            checks["origin_check"] = self.origin_check("before-arm")
            if not checks["origin_check"]["ok"]:
                return {"ok": False, "why": f"the fixture origin is not serving before arm: {checks['origin_check']}"}
            self.state["check_at"] = time.time()
            self.state["awaiting_terminal"] = True
            check = self.api("GET", "/api/v1/panel/update/check", purpose="PanelUpdateCard check (driver, before the "
                             "owner's own check in the browser)", timeout=60)
            body = check.json() or {}
            target = body.get("target") or {}
            checks["check"] = {"http": check.status, "body": body}
            expected = self.state["origin_target"]
            if (check.status != 200 or body.get("available") is not True or target.get("version") != out.CANDIDATE_VERSION
                    or target.get("commit") != self.candidate["commit"] or target.get("sequence") != str(out.CANDIDATE_SEQUENCE)
                    or target.get("archive_sha256") != expected["archive_sha256"]
                    or body.get("current_version") != out.BASELINE_VERSION):
                return {"ok": False, "why": f"update check does not offer the sealed fixture candidate: {body}"}
            self.state["check"] = body
            self.s17["page_ready_at"] = base.utc_now()
            return {"ok": True, "target": {k: target.get(k) for k in ("version", "commit", "sequence", "archive_sha256")},
                    "current_version": body.get("current_version")}
        except Exception as exc:  # noqa: BLE001 - written for the browser and kept
            return {"ok": False, "why": self.error_text(exc)}

    def page_arm(self, request_id: str) -> dict:
        """arm()'s second half for the page's own request id: the observer intent and unit, armed or not within
        ARM_WAIT_SECONDS (the page's own start request times out after 15 s)."""
        started = time.monotonic()
        if not RID.fullmatch(request_id) or "check" not in self.state or self.state.get("request_id"):
            return {"armed": False, "why": "not a fresh 32-hex request id, no update check, or already armed"}
        try:
            self.state["request_id"] = request_id
            proofs = self.state["proofs"]
            role = self.role
            intent = {"schema": out.OBSERVER_INTENT_SCHEMA, "identity": self.identity, "operation_id": request_id,
                      "mode": "checkpoint" if self.cell.recovery_fault else "watch",
                      "baseline": {"version": out.BASELINE_VERSION, "commit": self.artifacts["baseline"]["commit"],
                                   "agent_sha256": proofs["baseline"]["agent_sha256"],
                                   "panel_sha256": proofs["baseline"]["panel_sha256"]},
                      "target": {"version": out.CANDIDATE_VERSION, "commit": self.candidate["commit"],
                                 "agent_sha256": proofs[role]["agent_sha256"], "panel_sha256": proofs[role]["panel_sha256"]},
                      "recovery_fault": self.cell.recovery_fault}
            name = f"owner-update-observer-{request_id}.json"
            saved = self.trial.save(self.root, self.node_name, name, out.encoded(intent))
            self.lab.put_file(self.root, self.record, self.plan, self.node_name, Path(saved["path"]), name)
            unit = f"celikpanel-lab-owner-update-observer-{request_id}.service"
            argv = ["systemd-run", f"--unit={unit}", "--no-block", "--property=RuntimeMaxSec=3700",
                    "--property=TimeoutStopSec=10", "--property=UMask=0077",
                    f"--property=ExecStopPost=-/usr/bin/systemctl thaw celikpanel-self-update-{request_id}.service",
                    "python3", "-I", f"{out.PRIVATE}/guest_owner_update_observer.py", "--lab-nonce", self.identity["nonce"],
                    "--vm-uuid", self.identity["vm_uuid"], "--cell-id", self.identity["cell_id"], "--node", self.node_name,
                    "--operation-id", request_id, "--execute"]
            self.guest(shlex.join(argv), timeout=30)
            self.s17["observer"] = {"intent": intent, "unit": unit}
            while time.monotonic() - started < ARM_WAIT_SECONDS:
                events = self.observer_events()
                if events and events[0].get("event") == "armed":
                    self.s17["armed"] = {"at": base.utc_now(), "seconds": round(time.monotonic() - started, 2)}
                    return {"armed": True, "request_id": request_id, "seconds": self.s17["armed"]["seconds"]}
                time.sleep(0.3)
            self.s17["armed"] = None
            return {"armed": False, "why": f"the observer did not report armed within {ARM_WAIT_SECONDS} s",
                    "seconds": round(time.monotonic() - started, 2)}
        except Exception as exc:  # noqa: BLE001
            return {"armed": False, "why": self.error_text(exc), "seconds": round(time.monotonic() - started, 2)}

    def arm(self, checks: dict) -> str:
        if not self.page_window or not os.environ.get("SET12_HAND"):
            return super().arm(checks)
        record = self.browser_window("page", PAGE_WINDOW_SECONDS, {
            "browser": BROWSER[self.set17_name], "cell": self.set17_name, "mode": "update-page", "preps": list(PAGE_PREPS),
            "target_version": out.CANDIDATE_VERSION, "baseline_version": out.BASELINE_VERSION})
        self.s17["page_hand"] = record.get("hand")
        checks.update(self.s17.get("arm_checks") or {})
        checks["page_window"] = {k: v for k, v in record.items() if k != "requests"}
        checks["page_window_preps"] = record.get("requests")
        self.record_json("page-window.json", record)
        if record.get("skipped"):
            self.s17["page_skipped"] = record["skipped"]
            return super().arm(checks)
        answer = self.start_answer(wait=30)
        self.s17["start_answer"] = answer
        if not self.state.get("request_id"):
            raise base.StepInconclusive(f"the owner's start in the browser did not reach the observer: {record.get('not_finished')}"
                                        f" {answer.get('why') if isinstance(answer, dict) else ''}")
        obs = self.s17.get("observer") or {}
        checks.update(request_id=self.state["request_id"], observer_intent=obs.get("intent"), observer_unit=obs.get("unit"),
                      observer_armed=self.s17.get("armed"), start_answer=answer)
        if not self.s17.get("armed"):
            # The page's request was released anyway (its own timeout is 15 s); the update is tracked like any other.
            self.finding(f"{self.set17_name}: the observer was not armed before the page's start request was released "
                         f"({self.s17.get('armed')}); the update was tracked without its early checkpoints")
        return "passed"

    def start_answer(self, wait: int) -> dict:
        hand = Path(self.s17.get("page_hand") or "")
        deadline = time.monotonic() + wait
        while time.monotonic() < deadline:
            path = hand / "start-answer.json"
            if path.exists():
                try:
                    return json.loads(path.read_text())
                except ValueError:
                    pass
            time.sleep(0.5)
        return {"missing": True, "why": f"no start-answer.json within {wait} s"}

    def owner_start(self, checks: dict) -> str:
        if not self.page_window or "start_answer" not in self.s17:
            return super().owner_start(checks)
        answer = self.s17["start_answer"]
        rid = self.state["request_id"]
        checks["by"] = "the owner's press in the browser: the page's own POST /api/v1/panel/update/start (held unchanged " \
                       "until the observer was armed, then released); the driver sent no start"
        checks["start"] = {"http": answer.get("status"), "body": answer.get("body"), "failed": answer.get("failed"),
                           "request_id": answer.get("request_id")}
        released = answer.get("released_ms")
        answered = answer.get("answered_ms")
        self.state["started_at"] = released / 1000.0 if isinstance(released, (int, float)) else time.time()
        status = self.api("GET", f"/api/v1/panel/update/status?request_id={rid}", purpose="SystemUpdateOperation status (read-only)")
        status_body = status.json() or {}
        checks["status_after_the_start"] = {"http": status.status, "body": status_body}
        accepted = isinstance(answer.get("status"), int) and out.start_accepted(answer["status"], answer.get("body"))
        if not accepted and status_body.get("status") not in ("queued", "running", "succeeded"):
            raise base.StepFailed(f"the page's update start was not admitted: {checks['start']} status {status_body}")
        if not accepted:
            checks["start_outcome"] = "unknown in the page; reconciled by the status read"
        self.state["start_answered_at"] = answered / 1000.0 if isinstance(answered, (int, float)) else time.time()
        if self.pk_on():
            checks["packagekit_after_answer"] = self.pk_observation("owner-start-after-answer", readiness=False)
        return "passed"

    def step(self, name: str, function, *, needs: tuple = ()) -> str:
        if name == "collect":
            label = "after-the-update" if self.cell.variant == "good" else "after-the-return"
            base.Trial.step(self, f"set17-readings-{label}", lambda checks: self.readings17(checks, label),
                            needs=("terminal",))
            base.Trial.step(self, "set17-browser-window",
                            lambda checks: self.window17(checks, self.set17_name, label, UPDATE_PREPS),
                            needs=("terminal",))
        return super(s16.Set16UpdateTrial, self).step(name, function, needs=needs)


def build_plan(name: str, artifacts: dict, work_root: str, local_port: int) -> dict:
    plan = {"schema": "celikpanel/set17-plan/v1", "native_evidence": False, "cell": name, "work_root": work_root,
            "local_port": local_port, "pinned_names": list(s6.PINNED_NAMES), "browser": BROWSER[name],
            "rule": "the driver calls only the Panel's HTTP API as the logged-in owner; every native fact is an SSH reading; "
                    "owner actions and lab preparation on the guest are recorded as such; no certificate authority and "
                    "no licence service is contacted by the driver"}
    if name in CELLS:
        settings = CELLS[name]
        plan.update(cell_kind=CELL_KIND, candidate={k: artifacts["baseline"][k] for k in ("version", "commit", "sha256")},
                    setup={"purpose": settings.purpose, "components": sorted(settings.components), "dns_mode": "external"},
                    preparations=list(FRESH_PREPS),
                    steps=[s6.PIN_STEP, "preflight", "origin", "baseline-install", "owner-login", "license", "setup",
                           "set17-readings", "set17-browser-window", s6.PIN_END_STEP, "collect"])
    else:
        base_cell, _, page = UPDATE_CELLS[name]
        plan.update(cell_kind=UPDATE_KIND, base_cell=base_cell, baseline_ref=(artifacts.get("baseline_ref") or {}).get("ref"),
                    preparations=list(UPDATE_PREPS) + (list(PAGE_PREPS) if page else []),
                    update_page_window=page, update=s6.build_plan(base_cell, artifacts, work_root, local_port),
                    added_steps=["set17-readings-after-the-update|after-the-return", "set17-browser-window"],
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
    labels = fx17.configure(document)
    wanted = fx17.SECOND_REF if args.cell in CELLS else UPDATE_CELLS[args.cell][1]
    if labels.get("ref") != wanted:
        parser.error(f"cell {args.cell} needs an artifacts document with baseline_ref {wanted}, not {labels.get('ref')}")
    cell = CELLS[args.cell].cell if args.cell in CELLS else base.validate_cell(UPDATE_CELLS[args.cell][0])
    if args.command == "plan":
        base.validate_cell_artifacts(document, cell, check_files=not args.dry_run)
        print(json.dumps(build_plan(args.cell, document, args.work_root, args.local_port), indent=2, sort_keys=True))
        return 0
    if not args.execute:
        parser.error("run mutates one registered disposable guest and requires --execute")
    base.validate_cell_artifacts(document, cell)
    if args.cell in CELLS:
        result = Set17Trial(CELLS[args.cell], document, args.work_root, args.local_port).execute()
    else:
        result = Set17UpdateTrial(cell, document, args.work_root, args.local_port, name=args.cell,
                                  page_window=UPDATE_CELLS[args.cell][2]).execute()
    print(json.dumps({"overall": result["overall"], "outcome": (result.get("outcome") or {}).get("classification"),
                      "steps": {s["name"]: s["verdict"] for s in result["steps"]}}, sort_keys=True))
    return 0 if result["overall"] != "failed" else 1


if __name__ == "__main__":
    raise SystemExit(main())
