#!/usr/bin/env python3
"""set13: native readers, owner actions and lab preparation for the short native round on commit 816b8dd60 (a put-back is
reloaded only when the service's own check accepts it; an unread pool reaches the Domains row; the application unit's
PATH and ``/bin/sh -c``), on a marked disposable QEMU guest.

Built on ``guest_set12_native.py`` (every set12, set11, set10 and set8 mode is reused unchanged). Added:

Readers (``read-*``) only inspect: nginx's unit state and ``nginx -t``; an application unit with its text's
``Environment=PATH=`` and ``ExecStart=`` lines, its systemd state (``NRestarts``, ``ExecMainStatus``), its journal and
the answer of the application on its own port (127.0.0.1); ``systemd-analyze verify`` of a unit file run by the
harness itself; the lab call log written by the shims below; the Agent's ``PATH`` (that variable only).

Owner actions (``owner-*``): a file of the owner's own in ``/etc/nginx/conf.d/`` with a directive nginx refuses (or its
removal), without a reload; a line in the application unit naming a start-up program that does not exist
(``ExecStartPre=``), or its removal, followed by ``daemon-reload`` (the running application is not restarted).

Lab preparation (``lab-*``, recorded as such): logging shims ``/usr/local/sbin/systemctl`` and
``/usr/local/sbin/systemd-analyze`` (each call of any process appended to ``/run/set13-lab/calls.log`` with the caller's
PID and name, then the real program runs with the same arguments); one injected failure of the next ``systemctl
daemon-reload`` called by the Agent's process (the only way this harness has to make an application unit's take put
the previous file back); ``systemd-analyze`` made unavailable by binding an empty non-executable file over the real
program (and the bind removed again).

No mode prints a password, a private key or a hash of one, and no mode contacts another host.

set13: kısa yerel tur için okuyucular, sahibin elle yaptığı işlemler ve laboratuvar hazırlığı. Başka sunucuya bağlanmaz.
"""
from __future__ import annotations

import argparse
import base64
import importlib.util
import json
import os
from pathlib import Path
import re
import sys
import time
import urllib.request

HERE = Path(__file__).resolve().parent
SCHEMA = "celikpanel/set13-native/v1"


def _load(name: str):
    spec = importlib.util.spec_from_file_location("set13_" + name.replace(".", "_"), HERE / name)
    value = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(value)
    return value


g12 = _load("guest_set12_native.py")
g11, g10, g8, rid = g12.g11, g12.g10, g12.g8, g12.rid
run, utc, Refused = rid.run, rid.utc, rid.Refused

LAB_DIR = "/run/set13-lab"
CALL_LOG = LAB_DIR + "/calls.log"
FAIL_FLAG = LAB_DIR + "/fail-daemon-reload"
HIDE_FILE = LAB_DIR + "/no-systemd-analyze"
SHIM_DIR = "/usr/local/sbin"
SHIM_MARK = "# set13 lab shim"
NGINX_OWNER_FILE = "/etc/nginx/conf.d/zz-set13-owner.conf"
NGINX_OWNER_TEXT = b"# set13: the server owner's own file, with a directive nginx refuses\nset13_not_a_directive on;\n"
MISSING_PRESTART = b"# set13: added by the server owner by hand (a start-up program that does not exist)\nExecStartPre=/opt/set13-owner/missing-prepare\n"


def real_program(name: str) -> str:
    for directory in ("/usr/bin", "/bin", "/usr/sbin", "/sbin"):
        path = os.path.join(directory, name)
        if os.path.isfile(path) and not os.path.islink(path) and os.access(path, os.X_OK):
            return path
    for directory in ("/usr/bin", "/bin"):
        path = os.path.join(directory, name)
        if os.path.exists(path):
            return os.path.realpath(path)
    raise Refused(f"no {name} on this guest")


def agent_pid() -> int:
    shown = run(["systemctl", "show", g8.AGENT_UNIT, "-p", "MainPID", "--value"], timeout=30)
    return int((shown.get("stdout") or "0").strip() or 0)


def agent_path() -> str | None:
    pid = agent_pid()
    try:
        for item in Path(f"/proc/{pid}/environ").read_bytes().split(b"\0"):
            if item.startswith(b"PATH="):
                return item[5:].decode("utf-8", "replace")
    except OSError:
        return None
    return None


def call_log() -> list:
    try:
        return Path(CALL_LOG).read_text(errors="replace").splitlines()
    except OSError:
        return []


# -- readers --------------------------------------------------------------------------------------------------------

def read_nginx13(args: dict) -> dict:
    return {"at": utc(), "epoch": time.time(), "unit": g12.unit_state("nginx.service"),
            "is_active": (run(["systemctl", "is-active", "nginx.service"], timeout=20).get("stdout") or "").strip(),
            "nginx_test": run(["nginx", "-t"], timeout=60), "owner_file": g8.file_record(NGINX_OWNER_FILE)}


def unit_lines(text: str) -> dict:
    lines = (text or "").splitlines()
    return {"environment_path": [l for l in lines if l.startswith("Environment=PATH=") or l.startswith("Environment=\"PATH=")],
            "exec_start": [l for l in lines if l.startswith("ExecStart=")],
            "exec_start_pre": [l for l in lines if l.startswith("ExecStartPre=")],
            "port": next((m.group(1) for l in lines for m in [re.match(r'Environment="?PORT=(\d+)', l)] if m), None)}


def read_unit13(args: dict) -> dict:
    domain = rid.domain_name(args["domain"])
    site_id = g10.site_ids().get(domain)
    if site_id is None:
        return {"at": utc(), "domain": domain, "site_id": None}
    name = f"celikapp-{int(site_id)}"
    path = g10.unit_path(site_id)
    unit = g10.record_with(path, g10.HEADER_PREFIX)
    lines = unit_lines(unit.get("text") or "")
    shown = run(["systemctl", "show", name + ".service", "-p", "ActiveState", "-p", "SubState", "-p", "MainPID", "-p",
                 "NRestarts", "-p", "ExecMainStatus", "-p", "ExecMainCode", "-p", "Result", "-p", "ExecMainStartTimestamp"],
                timeout=30)
    state = dict(l.partition("=")[::2] for l in (shown.get("stdout") or "").splitlines() if "=" in l)
    value = {"at": utc(), "epoch": time.time(), "domain": domain, "site_id": site_id, "name": name, "unit": unit,
             "lines": lines, "state": state, "dropin_dir": g10.dropin_dir(site_id),
             "environment": (run(["systemctl", "show", name, "-p", "Environment"], timeout=30).get("stdout") or "").strip(),
             "journal": g8.journal([name + ".service"], "@" + str(int(args.get("since_epoch") or 0)), limit=200)}
    port = lines.get("port")
    if port:
        try:
            with urllib.request.urlopen(f"http://127.0.0.1:{int(port)}/", timeout=5) as response:
                value["http"] = {"status": response.status, "body": response.read(200).decode("utf-8", "replace")}
        except Exception as exc:  # noqa: BLE001 - the application's answer is data
            value["http"] = {"error": type(exc).__name__, "detail": str(exc)[:200]}
    if args.get("verify"):
        value["verify_by_harness"] = run([real_program("systemd-analyze"), "verify", path], timeout=60)
    return value


def read_lab13(args: dict) -> dict:
    """The shims' call log (from line ``from``), the shims and the hide state, the Agent's PID and PATH."""
    lines = call_log()
    start = int(args.get("from") or 0)
    pid = agent_pid()
    seen = {}
    for name in ("systemctl", "systemd-analyze"):
        shim = os.path.join(SHIM_DIR, name)
        seen[name] = {"shim": g8.file_record(shim), "real": real_program(name) if name != "systemd-analyze" or not os.path.exists(HIDE_FILE) else None}
    analyze = "/usr/bin/systemd-analyze"
    try:
        info = os.stat(f"/proc/{pid}/root{analyze}")
        in_agent = {"mode_octal": oct(info.st_mode & 0o7777), "executable": bool(info.st_mode & 0o111), "size": info.st_size}
    except OSError as exc:
        in_agent = {"error": type(exc).__name__}
    return {"at": utc(), "epoch": time.time(), "lines": lines[start:], "line_count": len(lines), "agent_pid": pid,
            "agent_path": agent_path(), "fail_flag": os.path.exists(FAIL_FLAG), "shims": seen,
            "systemd_analyze_seen_by_agent": in_agent,
            "mounts": [l for l in Path("/proc/self/mountinfo").read_text().splitlines() if "systemd-analyze" in l]}


# -- owner actions ------------------------------------------------------------------------------------------------

def owner_nginx13(args: dict) -> dict:
    """The owner puts a file of their own with a directive nginx refuses into /etc/nginx/conf.d/ (no reload), or takes
    back only that file (no reload)."""
    value = {"action": "owner-nginx13", "path": NGINX_OWNER_FILE, "before": g8.file_record(NGINX_OWNER_FILE)}
    if args.get("remove"):
        if os.path.isfile(NGINX_OWNER_FILE) and not os.path.islink(NGINX_OWNER_FILE):
            os.unlink(NGINX_OWNER_FILE)
            value["removed"] = True
    else:
        if not os.path.isdir("/etc/nginx/conf.d"):
            raise Refused("no /etc/nginx/conf.d on this guest")
        with open(NGINX_OWNER_FILE, "wb") as stream:
            stream.write(NGINX_OWNER_TEXT)
        os.chmod(NGINX_OWNER_FILE, 0o644)
    value["after"] = g8.file_record(NGINX_OWNER_FILE)
    value["nginx_test"] = run(["nginx", "-t"], timeout=60)
    value["unit"] = g12.unit_state("nginx.service")
    value["at"], value["epoch"] = utc(), time.time()
    return value


def owner_unit13(args: dict) -> dict:
    """The owner adds (or takes back) a line in the application unit naming a start-up program that does not exist,
    then ``systemctl daemon-reload``; the running application is not restarted."""
    domain = rid.domain_name(args["domain"])
    site_id = g10.site_ids().get(domain)
    if site_id is None:
        raise Refused("unknown site")
    path = g10.unit_path(site_id)
    data = Path(path).read_bytes()
    value = {"action": "owner-unit13", "what": args["action"], "path": path, "before": g10.record_with(path, g10.HEADER_PREFIX)}
    if args["action"] == "add-missing-prestart":
        marker = b"[Install]"
        if marker not in data:
            raise Refused("the unit has no [Install] section")
        g8.write_in_place(path, data.replace(marker, MISSING_PRESTART + b"\n" + marker, 1))
    elif args["action"] == "remove-missing-prestart":
        g8.write_in_place(path, data.replace(MISSING_PRESTART + b"\n", b"").replace(MISSING_PRESTART, b""))
    else:
        raise Refused("unknown unit action")
    value["daemon_reload"] = run(["systemctl", "daemon-reload"], timeout=90)
    value["after"] = g10.record_with(path, g10.HEADER_PREFIX)
    value["verify_by_harness"] = run([real_program("systemd-analyze"), "verify", path], timeout=60)
    value["at"], value["epoch"] = utc(), time.time()
    return value


# -- lab preparation ----------------------------------------------------------------------------------------------

def shim_text(name: str, real: str) -> str:
    if name == "systemctl":
        return f"""#!/bin/sh
{SHIM_MARK} (disposable guest): every call is logged, then the real systemctl runs with the same arguments.
# While {FAIL_FLAG} names the caller's parent PID (the Agent), that process's next `daemon-reload` fails once.
printf '%s systemctl ppid=%s comm=%s args=%s\\n' "$(date -u +%Y-%m-%dT%H:%M:%S.%NZ)" "$PPID" "$(cat /proc/$PPID/comm 2>/dev/null)" "$*" >> {CALL_LOG}
if [ "$1" = daemon-reload ] && [ -f {FAIL_FLAG} ] && [ "$(cat {FAIL_FLAG})" = "$PPID" ]; then
  rm -f {FAIL_FLAG}
  printf '%s systemctl ppid=%s injected-failure daemon-reload rc=1\\n' "$(date -u +%Y-%m-%dT%H:%M:%S.%NZ)" "$PPID" >> {CALL_LOG}
  echo "Failed to reload daemon: set13 lab preparation (one injected failure)" >&2
  exit 1
fi
exec {real} "$@"
"""
    return f"""#!/bin/sh
{SHIM_MARK} (disposable guest): every call is logged with its exit status; the real systemd-analyze runs.
printf '%s systemd-analyze ppid=%s comm=%s args=%s\\n' "$(date -u +%Y-%m-%dT%H:%M:%S.%NZ)" "$PPID" "$(cat /proc/$PPID/comm 2>/dev/null)" "$*" >> {CALL_LOG}
{real} "$@"
rc=$?
printf '%s systemd-analyze ppid=%s rc=%s\\n' "$(date -u +%Y-%m-%dT%H:%M:%S.%NZ)" "$PPID" "$rc" >> {CALL_LOG}
exit $rc
"""


def lab_shims13(args: dict) -> dict:
    """install: both shims; remove: both shims (only files carrying the mark); arm: the Agent's next daemon-reload fails
    once; hide / unhide: systemd-analyze unavailable (an empty non-executable file bound over the real program, the
    analyze shim removed) and back (bind removed, analyze shim written again)."""
    action = args["action"]
    os.makedirs(LAB_DIR, mode=0o755, exist_ok=True)
    value = {"action": "lab-shims13", "what": action}
    if action == "install":
        for name in ("systemctl", "systemd-analyze"):
            target = os.path.join(SHIM_DIR, name)
            if os.path.exists(target) and SHIM_MARK not in Path(target).read_text(errors="replace"):
                raise Refused(f"{target} exists and is not this harness's shim")
            Path(target).write_text(shim_text(name, real_program(name)))
            os.chmod(target, 0o755)
    elif action == "remove":
        for name in ("systemctl", "systemd-analyze"):
            target = os.path.join(SHIM_DIR, name)
            if os.path.isfile(target) and SHIM_MARK in Path(target).read_text(errors="replace"):
                os.unlink(target)
                value.setdefault("removed", []).append(target)
        if os.path.exists(FAIL_FLAG):
            os.unlink(FAIL_FLAG)
    elif action == "arm":
        pid = agent_pid()
        if pid <= 1:
            raise Refused("the Agent is not running")
        Path(FAIL_FLAG).write_text(str(pid))
        value["agent_pid"] = pid
    elif action == "disarm":
        if os.path.exists(FAIL_FLAG):
            os.unlink(FAIL_FLAG)
            value["was_armed"] = True
    elif action == "hide":
        real = real_program("systemd-analyze")
        Path(HIDE_FILE).write_bytes(b"")
        os.chmod(HIDE_FILE, 0o644)
        shim = os.path.join(SHIM_DIR, "systemd-analyze")
        if os.path.isfile(shim) and SHIM_MARK in Path(shim).read_text(errors="replace"):
            os.unlink(shim)
        value["real"] = real
        value["mount"] = run(["mount", "--bind", HIDE_FILE, real], timeout=30)
    elif action == "unhide":
        real = "/usr/bin/systemd-analyze"
        value["umount"] = run(["umount", real], timeout=30)
        if os.path.exists(HIDE_FILE):
            os.unlink(HIDE_FILE)
        if args.get("shim", True):
            target = os.path.join(SHIM_DIR, "systemd-analyze")
            Path(target).write_text(shim_text("systemd-analyze", real_program("systemd-analyze")))
            os.chmod(target, 0o755)
    else:
        raise Refused("unknown lab action")
    value["state"] = read_lab13({"from": 10 ** 9})
    value["at"], value["epoch"] = utc(), time.time()
    return value


MODES = dict(g12.MODES)
MODES.update({"read-nginx13": read_nginx13, "read-unit13": read_unit13, "read-lab13": read_lab13,
              "owner-nginx13": owner_nginx13, "owner-unit13": owner_unit13, "lab-shims13": lab_shims13})


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
