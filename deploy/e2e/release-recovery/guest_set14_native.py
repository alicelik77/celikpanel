#!/usr/bin/env python3
"""set14: native readers, owner actions and lab preparation for the fourteenth native record (commit a5830dd3e: the
Dashboard's "System services" card checks the components by itself), on a marked disposable QEMU guest.

Built on ``guest_set13_native.py`` (every set13, set12, set11, set10 and set8 mode is reused unchanged). Added:

Reader (``read-probe14``) only inspects: the guest clock; the Panel's stored component scan (``service_scan_cache``
row 1 of the Panel database, opened read-only: ``scanned_at``, its age, the size of the stored text); the Agent's and
the Panel's systemd state (``ActiveState``, ``MainPID``, ``NRestarts``, ``CPUUsageNSec``, ``MemoryCurrent``), the
``usage_usec``/``user_usec``/``system_usec`` lines of each unit's cgroup ``cpu.stat`` and the main process's
``/proc/<pid>/stat`` CPU fields (``utime``, ``stime``, ``cutime``, ``cstime`` in clock ticks); ``/proc/loadavg``;
any nginx drop-in this harness wrote.

Owner action (``owner-agent14``): ``systemctl stop|start celikpanel-agent.service`` (what a server owner would type),
then a bounded wait (at most 60 s, inside the guest) until the unit is inactive or active.

Lab preparation (``lab-slow-reload14``, recorded as such): a drop-in for ``nginx.service`` whose ``ExecReload=`` is
``/bin/sleep N`` (a reload that takes N seconds and changes nothing), or its removal; ``systemctl daemon-reload`` after
either. It is how this harness makes one ordinary Panel operation (a reload of nginx from the Panel) hold the Panel's
change lease long enough to load the Dashboard meanwhile.

No mode prints a password, a private key or a hash of one, and no mode contacts another host.

set14: on dördüncü yerel kayıt için okuyucu, sahibin elle yaptığı işlem ve laboratuvar hazırlığı. Başka sunucuya
bağlanmaz.
"""
from __future__ import annotations

import argparse
import base64
import importlib.util
import json
import os
from pathlib import Path
import sqlite3
import subprocess
import sys
import time

HERE = Path(__file__).resolve().parent
SCHEMA = "celikpanel/set14-native/v1"


def _load(name: str):
    spec = importlib.util.spec_from_file_location("set14_" + name.replace(".", "_"), HERE / name)
    value = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(value)
    return value


g13 = _load("guest_set13_native.py")
g12, rid = g13.g12, g13.rid
g10 = g12.g10
run, utc, Refused = rid.run, rid.utc, rid.Refused

AGENT_UNIT = "celikpanel-agent.service"
PANEL_UNIT = "celikpanel-panel.service"
UNITS = {"agent": AGENT_UNIT, "panel": PANEL_UNIT}
SHOW = ("ActiveState", "SubState", "MainPID", "NRestarts", "CPUUsageNSec", "MemoryCurrent", "ControlGroup",
        "ActiveEnterTimestampMonotonic", "ExecMainStartTimestamp")
DROPIN_DIR = "/etc/systemd/system/nginx.service.d"
DROPIN = DROPIN_DIR + "/zz-set14-slow-reload.conf"
WAIT_LIMIT = 60


def show(unit: str) -> dict:
    out = subprocess.run(["systemctl", "show", unit, "-p", ",".join(SHOW)], capture_output=True, text=True, timeout=30)
    value = {}
    for line in out.stdout.splitlines():
        key, _, rest = line.partition("=")
        value[key] = rest
    return value


def cpu_stat(control_group: str) -> dict:
    if not control_group:
        return {"error": "no control group"}
    path = Path("/sys/fs/cgroup") / control_group.lstrip("/") / "cpu.stat"
    try:
        rows = dict(line.split() for line in path.read_text().splitlines() if line.strip())
    except OSError as exc:
        return {"error": type(exc).__name__}
    return {k: int(v) for k, v in rows.items() if k in ("usage_usec", "user_usec", "system_usec")}


def proc_cpu(pid: str) -> dict:
    try:
        text = Path(f"/proc/{int(pid)}/stat").read_text()
    except (OSError, ValueError) as exc:
        return {"error": type(exc).__name__}
    fields = text[text.rindex(")") + 2:].split()
    # fields[0] is field 3 (state); utime is field 14 -> index 11
    return {"utime": int(fields[11]), "stime": int(fields[12]), "cutime": int(fields[13]), "cstime": int(fields[14]),
            "clk_tck": os.sysconf("SC_CLK_TCK")}


def scan_cache() -> dict:
    value = {"database": g10.PANEL_DB}
    try:
        connection = sqlite3.connect("file:" + g10.PANEL_DB + "?mode=ro", uri=True, timeout=20, isolation_level=None)
    except sqlite3.Error as exc:
        return dict(value, error=type(exc).__name__ + ": " + str(exc)[:200])
    try:
        connection.execute("PRAGMA query_only=ON")
        row = connection.execute("SELECT scanned_at, length(data) FROM service_scan_cache WHERE id = 1").fetchone()
    except sqlite3.Error as exc:
        return dict(value, error=type(exc).__name__ + ": " + str(exc)[:200])
    finally:
        connection.close()
    if row is None:
        return dict(value, exists=False)
    scanned_at, size = row
    value.update(exists=True, scanned_at=scanned_at, data_bytes=size)
    try:
        from datetime import datetime
        scanned = datetime.fromisoformat(scanned_at.replace("Z", "+00:00")).timestamp()
        value.update(scanned_epoch=scanned, age_seconds=round(time.time() - scanned, 1))
    except ValueError:
        value["age_seconds"] = None
    return value


def read_probe14(args: dict) -> dict:
    value = {"at": utc(), "epoch": time.time(), "units": {}, "cgroup_cpu": {}, "proc_cpu": {}}
    for name, unit in UNITS.items():
        state = show(unit)
        value["units"][name] = state
        value["cgroup_cpu"][name] = cpu_stat(state.get("ControlGroup", ""))
        if state.get("MainPID") not in (None, "", "0"):
            value["proc_cpu"][name] = proc_cpu(state["MainPID"])
    value["scan_cache"] = scan_cache()
    value["loadavg"] = Path("/proc/loadavg").read_text().strip()
    value["nginx_dropin"] = {"path": DROPIN, "exists": os.path.exists(DROPIN),
                             "text": Path(DROPIN).read_text() if os.path.exists(DROPIN) else None}
    return value


def owner_agent14(args: dict) -> dict:
    action = args.get("action")
    if action not in ("stop", "start"):
        raise Refused("action must be stop or start")
    value = {"action": "owner-agent14", "what": action, "unit": AGENT_UNIT, "before": show(AGENT_UNIT),
             "command": f"systemctl {action} {AGENT_UNIT}"}
    started = time.time()
    done = subprocess.run(["systemctl", action, AGENT_UNIT], capture_output=True, text=True, timeout=120)
    value["returncode"], value["stderr"] = done.returncode, done.stderr[-400:]
    want = "active" if action == "start" else "inactive"
    for _ in range(WAIT_LIMIT * 2):
        state = show(AGENT_UNIT)
        if state.get("ActiveState") in (want, "failed"):
            break
        time.sleep(0.5)
    value["after"] = show(AGENT_UNIT)
    value["waited_seconds"] = round(time.time() - started, 2)
    value["at"], value["epoch"] = utc(), time.time()
    return value


def lab_slow_reload14(args: dict) -> dict:
    value = {"action": "lab-slow-reload14", "path": DROPIN}
    if args.get("remove"):
        existed = os.path.exists(DROPIN)
        if existed:
            os.unlink(DROPIN)
        value["removed"] = existed
    else:
        seconds = int(args.get("seconds") or 90)
        if not 5 <= seconds <= 300:
            raise Refused("seconds must be between 5 and 300")
        os.makedirs(DROPIN_DIR, exist_ok=True)
        text = ("# set14 lab preparation (disposable guest): a reload of nginx that takes %d s and changes nothing\n"
                "[Service]\nExecReload=\nExecReload=/bin/sleep %d\n") % (seconds, seconds)
        Path(DROPIN).write_text(text)
        value["text"] = text
    reload = subprocess.run(["systemctl", "daemon-reload"], capture_output=True, text=True, timeout=120)
    value["daemon_reload_returncode"] = reload.returncode
    shown = subprocess.run(["systemctl", "show", "nginx.service", "-p", "ExecReload,ActiveState,MainPID"],
                           capture_output=True, text=True, timeout=30)
    value["nginx"] = shown.stdout.strip().splitlines()
    value["at"], value["epoch"] = utc(), time.time()
    return value


MODES = dict(g13.MODES)
MODES.update({"read-probe14": read_probe14, "owner-agent14": owner_agent14, "lab-slow-reload14": lab_slow_reload14})


def main(argv=None) -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("mode", choices=sorted(MODES))
    for name in ("lab-nonce", "vm-uuid", "cell-id", "node"):
        parser.add_argument("--" + name, required=True)
    parser.add_argument("--args-b64", default="e30=")
    args = parser.parse_args(argv)
    rid._load_probe().guard_guest(args)
    request = json.loads(base64.b64decode(args.args_b64, validate=True))
    if not isinstance(request, dict):
        parser.error("--args-b64 must hold a JSON object")
    value = MODES[args.mode](request)
    value.update(schema=SCHEMA, mode=args.mode, owner_action=args.mode.startswith("owner-"))
    print(json.dumps(value, sort_keys=True, default=str))
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except Refused as exc:
        print(json.dumps({"refused": "Refused", "reason": str(exc)[:300]}), file=sys.stderr)
        raise SystemExit(3)
    except Exception as exc:  # noqa: BLE001 - reported as data; the driver decides
        print(json.dumps({"refused": type(exc).__name__, "reason": str(exc)[:300]}), file=sys.stderr)
        raise SystemExit(2)
