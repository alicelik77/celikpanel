#!/usr/bin/env python3
"""set17b LAB PREPARATION (not a product change): lengthen the Panel's outage during one owner update on a
disposable lab guest, so that the update page is seen through a restart as long as the one on the owner's server.

The first plan was a drop-in for celikpanel-panel.service with ``ExecStartPre=/bin/sleep 150``. The updater refuses
that: before it publishes its active marker it installs its own guard drop-in and requires that systemd loads ONLY
that drop-in for the Panel's unit (deploy/release-transaction-guard.sh, ``release_txn_install_unit_guards``: "systemd
did not load only the exact transaction guard for celikpanel-panel.service"; ``release_txn_verify_unit_guard_files``:
"guard directory contains unrecognized entries"). So no unit file and no drop-in is touched here.

Instead, for exactly one update (its request id), this helper watches the release transaction's marker directory
``/var/lib/celikpanel-release-transaction`` (read-only, every 50 ms) until the updater renames its ``active`` marker to
``completion.pending`` (update.sh ``release_txn_mark_completion_pending``: the candidate is installed, the Panel and the
Agent are stopped; what follows is the offline database step, the artifact verification and then the controlled Agent
and Panel starts). If, and only if, the update's own worker unit ``celikpanel-self-update-<id>.service`` is then
active and the Panel's unit inactive or failed, the worker unit is frozen (``systemctl freeze``, the cgroup freezer; no
signal is sent, nothing is killed), held for DELAY seconds, and thawed (``systemctl thaw``). The updater then continues
exactly where it was and starts the Agent and the Panel itself. Between that marker and the Panel start update.sh runs
no ``timeout`` command and no clock-bounded wait (its Panel start wait begins after ``systemctl start``), and the release
recovery watchdog leaves a held transaction lock alone, so the freeze only lengthens the time the Panel is down. If the
Panel is already starting when the marker is seen, or becomes so right after the freeze, nothing is held (the worker is
thawed at once) and that is recorded.

(The updater's informational line ``CELIKPANEL_UPDATE_CHECKPOINT database_verified_before_start`` is not used: in an
owner's update from the Panel the Agent's worker keeps the updater's output in memory, so the line never reaches the
journal; set17b run a, 2026-10-15, saw the worker end without it.)

Every step is written to ``/root/celikpanel-release-recovery-lab/set17b-restart-delay-<id>.jsonl`` (root only). The
unit that runs this helper is transient (``systemd-run``) with ``ExecStopPost=-/usr/bin/systemctl thaw <worker>``,
so the worker is thawed even if this helper is stopped.

  guest_set17b_restart_delay.py --operation-id <32 hex> --delay-seconds 150 [--limit-seconds 900]

set17b: Laboratuvar hazırlığı; ürün değişikliği değildir. Güncelleme işçisi, Panel durmuşken işlem işaretçisi
completion.pending olduğunda DELAY saniye dondurulur ve çözülür; birim dosyası ve drop-in değiştirilmez.
"""
from __future__ import annotations

import argparse
import json
import os
from pathlib import Path
import re
import signal
import subprocess
import sys
import time

TRANSACTIONS = Path("/var/lib/celikpanel-release-transaction")
TRIGGER = TRANSACTIONS / "completion.pending"
PRIVATE = Path("/root/celikpanel-release-recovery-lab")
SCHEMA = "celikpanel/set17b-restart-delay/v1"
HEX32 = re.compile(r"[0-9a-f]{32}")
QUIET = ("inactive", "failed")


def utc(epoch: float | None = None) -> str:
    value = time.time() if epoch is None else epoch
    return time.strftime("%Y-%m-%dT%H:%M:%S", time.gmtime(value)) + f".{int((value % 1) * 1000):03d}Z"


def show(unit: str, *props: str) -> dict:
    result = subprocess.run(["systemctl", "show", unit, "--no-pager", *(f"--property={p}" for p in props)],
                            capture_output=True, text=True, timeout=10)
    values = {}
    for line in result.stdout.splitlines():
        key, _, value = line.partition("=")
        values[key] = value
    return values


def markers() -> list[str]:
    try:
        return sorted(name for name in os.listdir(TRANSACTIONS) if name in ("active", "completion.pending", "scheduler.pending"))
    except OSError:
        return []


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--operation-id", required=True)
    parser.add_argument("--delay-seconds", type=float, required=True)
    parser.add_argument("--limit-seconds", type=float, default=900.0)
    args = parser.parse_args(argv)
    if not HEX32.fullmatch(args.operation_id) or not 0 < args.delay_seconds <= 600 or not 0 < args.limit_seconds <= 1800:
        parser.error("operation id must be 32 hex; delay in (0, 600]; limit in (0, 1800]")
    worker = f"celikpanel-self-update-{args.operation_id}.service"
    events = PRIVATE / f"set17b-restart-delay-{args.operation_id}.jsonl"
    started = time.monotonic()

    def emit(event: str, **fields) -> None:
        line = json.dumps({"schema": SCHEMA, "event": event, "at": utc(), "epoch": round(time.time(), 3),
                           "operation_id": args.operation_id, "worker": worker, **fields}, sort_keys=True)
        fd = os.open(events, os.O_WRONLY | os.O_CREAT | os.O_APPEND, 0o600)
        try:
            os.write(fd, (line + "\n").encode())
        finally:
            os.close(fd)

    def states() -> dict:
        panel = show("celikpanel-panel.service", "ActiveState", "SubState", "MainPID")
        agent = show("celikpanel-agent.service", "ActiveState", "SubState")
        unit = show(worker, "ActiveState", "SubState", "FreezerState")
        return {"panel": panel.get("ActiveState"), "panel_sub": panel.get("SubState"), "panel_pid": panel.get("MainPID"),
                "agent": agent.get("ActiveState"), "worker": unit.get("ActiveState"), "worker_freezer": unit.get("FreezerState"),
                "markers": markers()}

    frozen = False

    def thaw(reason: str) -> None:
        nonlocal frozen
        if not frozen:
            return
        result = subprocess.run(["systemctl", "thaw", worker], capture_output=True, text=True, timeout=30)
        frozen = False
        emit("thawed", reason=reason, returncode=result.returncode, stderr=result.stderr.strip()[:300], **states())

    def stop(signum, frame) -> None:  # noqa: ARG001
        thaw(f"signal {signum}")
        emit("released", reason=f"signal {signum}")
        sys.exit(3)

    signal.signal(signal.SIGTERM, stop)
    signal.signal(signal.SIGINT, stop)
    emit("armed", delay_seconds=args.delay_seconds, limit_seconds=args.limit_seconds, trigger=str(TRIGGER),
         what="lab preparation: hold the update's own worker once its transaction is completion-pending, while the "
              "Panel is stopped", **states())

    seen = None
    saw_worker = False
    last_check = 0.0
    last_markers = None
    while time.monotonic() - started < args.limit_seconds:
        if TRIGGER.exists():
            seen = time.time()
            break
        if time.monotonic() - last_check >= 1.0:
            last_check = time.monotonic()
            now = markers()
            if now != last_markers:
                emit("markers", markers=now)
                last_markers = now
            worker_state = show(worker, "ActiveState").get("ActiveState")
            saw_worker = saw_worker or worker_state in ("active", "activating", "deactivating", "reloading")
            if saw_worker and worker_state in QUIET:
                emit("released", reason="worker-ended-before-completion-pending", **states())
                return 2
        time.sleep(0.05)
    if seen is None:
        emit("released", reason="completion-pending-not-seen-within-the-limit", **states())
        return 2
    before = states()
    emit("completion_pending_seen", seen_epoch=round(seen, 3), **before)
    if before["panel"] not in QUIET or before["worker"] != "active":
        emit("released", reason="not-held: the Panel was not stopped (or the worker not active) when the marker was seen",
             **before)
        return 2
    result = subprocess.run(["systemctl", "freeze", worker], capture_output=True, text=True, timeout=30)
    frozen = result.returncode == 0
    after = states()
    emit("frozen" if frozen else "freeze_failed", returncode=result.returncode, stderr=result.stderr.strip()[:300],
         after_marker_ms=round((time.time() - seen) * 1000), **after)
    if not frozen:
        emit("released", reason="freeze-failed")
        return 2
    if after["panel"] not in QUIET:
        thaw("the Panel was already starting when the worker was frozen; nothing is held")
        emit("released", reason="not-held")
        return 2
    held_from = time.monotonic()
    last = None
    while time.monotonic() - held_from < args.delay_seconds:
        now = states()
        if now != last:
            emit("held", held_s=round(time.monotonic() - held_from, 1), **now)
            last = now
        time.sleep(1.0)
    thaw(f"the delay of {args.delay_seconds:g} s ended")
    # After the thaw: when the updater started the Agent and the Panel (read-only, at most 180 s).
    after_thaw = time.monotonic()
    last = None
    while time.monotonic() - after_thaw < 180:
        now = states()
        if now != last:
            emit("after_thaw", after_thaw_s=round(time.monotonic() - after_thaw, 1), **now)
            last = now
        if now["worker"] in QUIET:
            break
        time.sleep(0.5)
    emit("released", reason="held and thawed", held_seconds=args.delay_seconds)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
