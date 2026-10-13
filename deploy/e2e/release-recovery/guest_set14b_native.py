#!/usr/bin/env python3
"""set14b: native readers and lab preparation for the short follow-up of the fourteenth native record (commit
1a74eeda5: the server reports a scan's age; an unreachable Agent is named), on a marked disposable QEMU guest.

Built on ``guest_set14_native.py`` (every set14 mode and the modes it reuses are kept unchanged). Added:

Reader (``read-probe14b``) only inspects: everything ``read-probe14`` reads, plus the guest clock's NTP settings
(``timedatectl show``) and nginx's systemd state (``MainPID``, ``ActiveEnterTimestampMonotonic``,
``ExecMainStartTimestamp``, ``NRestarts``), so that a refused change can be shown to have changed nothing.

Lab preparation (``lab-clock14b``, recorded as such): the guest's clock moved by a number of milliseconds relative
to its own current reading (``timedatectl set-ntp false`` first, then ``date -u -s @<epoch>``), bounded to 15
minutes either way; or (``ntp: true``) ``timedatectl set-ntp true``. It is how this harness puts the server's clock
behind the browser's. Each call reports the guest clock before and after and every return code.

No mode prints a password, a private key or a hash of one, and no mode contacts another host.

set14b: on dördüncü kaydın kısa devamı için okuyucu ve laboratuvar hazırlığı (konuk saatini kaydırma). Başka
sunucuya bağlanmaz.
"""
from __future__ import annotations

import argparse
import base64
import importlib.util
import json
from pathlib import Path
import subprocess
import sys
import time

HERE = Path(__file__).resolve().parent
SCHEMA = "celikpanel/set14b-native/v1"


def _load(name: str):
    spec = importlib.util.spec_from_file_location("set14b_" + name.replace(".", "_"), HERE / name)
    value = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(value)
    return value


g14 = _load("guest_set14_native.py")
rid = g14.rid
utc, Refused = rid.utc, rid.Refused

NGINX_UNIT = "nginx.service"
NGINX_SHOW = ("ActiveState", "SubState", "MainPID", "NRestarts", "ActiveEnterTimestampMonotonic", "ExecMainStartTimestamp")
CLOCK_SHOW = ("NTP", "NTPSynchronized", "CanNTP", "TimeUSec", "Timezone")
SHIFT_LIMIT_MS = 15 * 60 * 1000


def show(unit: str, fields: tuple) -> dict:
    out = subprocess.run(["systemctl", "show", unit, "-p", ",".join(fields)], capture_output=True, text=True, timeout=30)
    value = {}
    for line in out.stdout.splitlines():
        key, _, rest = line.partition("=")
        value[key] = rest
    return value


def clock_state() -> dict:
    out = subprocess.run(["timedatectl", "show", "-p", ",".join(CLOCK_SHOW)], capture_output=True, text=True, timeout=30)
    value = {"returncode": out.returncode}
    for line in out.stdout.splitlines():
        key, _, rest = line.partition("=")
        value[key] = rest
    if out.returncode:
        value["stderr"] = out.stderr[-300:]
    return value


def read_probe14b(args: dict) -> dict:
    value = g14.read_probe14(args)
    value["nginx"] = show(NGINX_UNIT, NGINX_SHOW)
    value["clock"] = clock_state()
    return value


def lab_clock14b(args: dict) -> dict:
    ntp_on = args.get("ntp") is True
    shift_ms = args.get("shift_ms")
    if not ntp_on and (not isinstance(shift_ms, int) or isinstance(shift_ms, bool) or not 0 < abs(shift_ms) <= SHIFT_LIMIT_MS):
        raise Refused("shift_ms must be a non-zero whole number of milliseconds within 15 minutes")
    value = {"action": "lab-clock14b", "before_epoch": time.time(), "before_clock": clock_state()}
    if ntp_on:
        done = subprocess.run(["timedatectl", "set-ntp", "true"], capture_output=True, text=True, timeout=60)
        value.update(command="timedatectl set-ntp true", returncode=done.returncode, stderr=done.stderr[-300:])
    else:
        ntp = subprocess.run(["timedatectl", "set-ntp", "false"], capture_output=True, text=True, timeout=60)
        value.update(ntp_off_returncode=ntp.returncode, ntp_off_stderr=ntp.stderr[-300:])
        target = time.time() + shift_ms / 1000.0
        command = ["date", "-u", "-s", "@%.6f" % target]
        done = subprocess.run(command, capture_output=True, text=True, timeout=30)
        value.update(command=" ".join(command[:3]) + " @<guest epoch %+d ms>" % shift_ms, shift_ms=shift_ms,
                     target_epoch=target, returncode=done.returncode, stdout=done.stdout.strip()[-200:],
                     stderr=done.stderr[-300:])
    value["after_epoch"] = time.time()
    value["after_clock"] = clock_state()
    value["at"], value["epoch"] = utc(), time.time()
    return value


MODES = dict(g14.MODES)
MODES.update({"read-probe14b": read_probe14b, "lab-clock14b": lab_clock14b})


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
