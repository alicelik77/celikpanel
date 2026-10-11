#!/usr/bin/env python3
"""set12: native readers and the owner's hand actions for the twelfth native record (D-031's second step: the PHP-FPM
pool and the application unit; the multi-file page; the restore fact; "apply"; the addresses the validation probe
asks) on a marked disposable QEMU guest.

Built on ``guest_set11_native.py`` (its readers, watchers, owner actions and lab preparation, and set10's and set8's
under it, are reused unchanged). Added:

Readers (``read-*``) only inspect: a site's PHP-FPM pool, its pool directory and the PHP-FPM unit (state, main PID,
the owner's files in the pool directory); the application unit with its drop-in directory and its systemd state;
the journal of the Panel, the Agent, nginx, PHP-FPM and every ``celikapp-*`` unit since a time, with systemd's own
lines about them; the guest's IPv4 and IPv6 addresses and the sysctl ``net.ipv4.ip_nonlocal_bind``.

Owner actions (``owner-*``) are what a server owner does by hand: add (or take back) a directive with a comment of
their own in a site's pool; put a pool file of their own with a directive PHP-FPM refuses into the pool directory (or
take it back); stop, start or restart PHP-FPM; bind the site's port-80 block to one address, to ``[::]:80`` only, or
back; lock or unlock a pool file (``chattr``). ``lab-*`` is lab preparation, recorded as such: a pool or a vhost
written back to recorded bytes; a drop-in that makes PHP-FPM's reload take a fixed time (so that one pool operation
holds the Agent's pool lock while the page reads); ``net.ipv4.ip_nonlocal_bind`` (nginx may then listen on an address
the guest does not have).

No mode prints a password, a private key or a hash of one, and no mode contacts another host.

set12: onikinci yerel kayıt için okuyucular ve sahibin elle yaptığı işlemler. Hiçbir kip parola ya da özel anahtar
yazdırmaz; başka bir sunucuya bağlanmaz.
"""
from __future__ import annotations

import argparse
import base64
import glob
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import re
import sys
import time

HERE = Path(__file__).resolve().parent
SCHEMA = "celikpanel/set12-native/v1"


def _load(name: str):
    spec = importlib.util.spec_from_file_location("set12_" + name.replace(".", "_"), HERE / name)
    value = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(value)
    return value


g11 = _load("guest_set11_native.py")
g10, g8, rid = g11.g10, g11.g8, g11.rid
run, utc, Refused = rid.run, rid.utc, rid.Refused

AVAILABLE = g8.AVAILABLE
INI = g10.INI_HEADER_PREFIX
OWNER_POOL_LINES = b"; set12: added by the server owner by hand (a comment and a directive of their own)\nphp_admin_value[memory_limit] = 256M\n"
BROKEN_POOL_NAME = "zz-set12-owner.conf"
BROKEN_POOL_TEXT = b"; set12: the server owner's own pool file, with a directive PHP-FPM refuses\n[set12owner]\nset12_not_a_directive = 1\n"
SLOW_DROPIN = "zz-set12-lab-slow-reload.conf"
NONLOCAL_SYSCTL = "/proc/sys/net/ipv4/ip_nonlocal_bind"


def php_unit_and_program(pool: str) -> tuple:
    unit = g10.php_unit_of(pool)
    program = "php-fpm" if unit == "php-fpm" else "php-fpm" + unit[3:-4]
    return unit, program


def unit_state(unit: str) -> dict:
    shown = run(["systemctl", "show", unit, "-p", "ActiveState", "-p", "SubState", "-p", "MainPID", "-p",
                 "ActiveEnterTimestamp", "-p", "ExecMainStartTimestamp", "-p", "NRestarts", "-p", "LoadState"], timeout=30)
    value = {}
    for line in (shown.get("stdout") or "").splitlines():
        key, _, rest = line.partition("=")
        if key:
            value[key] = rest
    return value


def single_pool(domain: str) -> str:
    site_id = g10.site_ids().get(domain)
    pools = g10.pool_paths(site_id) if site_id is not None else []
    if len(pools) != 1:
        raise Refused("the site has no single pool file")
    return pools[0]


def pool_dir_entries(pool: str) -> list:
    directory = os.path.dirname(pool)
    out = []
    for name in sorted(os.listdir(directory)):
        out.append(g8.file_record(os.path.join(directory, name), text=name.startswith("zz-set12")))
    return out


# -- readers --------------------------------------------------------------------------------------------------------

def read_pool12(args: dict) -> dict:
    """A site's pool (with its side files), its directory's entries and PHP-FPM's unit."""
    domain = rid.domain_name(args["domain"])
    site_id = g10.site_ids().get(domain)
    pools = g10.pool_paths(site_id) if site_id is not None else []
    value = {"at": utc(), "epoch": time.time(), "domain": domain, "site_id": site_id,
             "pools": {p: g10.record_with(p, INI) for p in pools}}
    if pools:
        unit, program = php_unit_and_program(pools[0])
        value.update(php_unit=unit, php_unit_state=unit_state(unit + ".service"),
                     pool_dir=dict(rid._stat(os.path.dirname(pools[0])), entries=pool_dir_entries(pools[0])),
                     php_fpm_test=run([program, "-t"], timeout=60))
        lsattr = run(["lsattr", "-d", os.path.dirname(pools[0])], timeout=20)
        value["pool_dir_lsattr"] = ((lsattr.get("stdout") or "").split() or [None])[0]
    value["php_fpm_programs"] = g10.php_versions()
    return value


def read_unit12(args: dict) -> dict:
    domain = rid.domain_name(args["domain"])
    site_id = g10.site_ids().get(domain)
    if site_id is None:
        return {"at": utc(), "domain": domain, "site_id": None}
    name = f"celikapp-{int(site_id)}"
    return {"at": utc(), "epoch": time.time(), "domain": domain, "site_id": site_id,
            "unit": g10.record_with(g10.unit_path(site_id), g10.HEADER_PREFIX), "dropin_dir": g10.dropin_dir(site_id),
            "state": unit_state(name + ".service"),
            "environment": (run(["systemctl", "show", name, "-p", "Environment"], timeout=30).get("stdout") or "").strip(),
            "dropin_paths": (run(["systemctl", "show", name, "-p", "DropInPaths"], timeout=30).get("stdout") or "").strip()}


def read_window12(args: dict) -> dict:
    """set8's window (the Panel, the Agent, nginx, PHP-FPM) plus every celikapp-* unit and systemd's own lines."""
    value = g8.read_window(args)
    since = "@" + str(int(args["since_epoch"]))
    units = sorted({os.path.basename(p) for p in glob.glob("/etc/systemd/system/celikapp-*.service")})
    for unit in units:
        value["by_unit"][unit] = g8.journal([unit], since)
    manager = run(["journalctl", "--no-pager", "-o", "short-iso-precise", "--since", since, "_PID=1", "-n", "3000"], timeout=60)
    value["systemd_all"] = [l[:400] for l in (manager.get("stdout") or "").splitlines()
                            if l.strip() and not l.startswith("-- ") and ("celikapp-" in l or "Reload" in l or "php" in l)][:400]
    value["units"] = list(value.get("units") or []) + units
    return value


def read_paths12(args: dict) -> dict:
    """Named site files (a pool or unit of a site that may be gone) with their side files."""
    out = {"at": utc(), "epoch": time.time(), "files": {}}
    for path in args.get("paths") or []:
        if not isinstance(path, str) or not re.fullmatch(r"/etc/[A-Za-z0-9._/+-]{1,200}", path) or ".." in path:
            raise Refused("not a path this harness reads")
        out["files"][path] = g10.record_with(path, INI if path.endswith(".conf") else g10.HEADER_PREFIX)
    return out


def read_addresses12(args: dict) -> dict:
    v4 = run(["ip", "-4", "-o", "addr", "show", "scope", "global"], timeout=20)
    v6 = run(["ip", "-6", "-o", "addr", "show"], timeout=20)
    nonlocal_bind = Path(NONLOCAL_SYSCTL).read_text().strip() if os.path.exists(NONLOCAL_SYSCTL) else None
    return {"at": utc(), "ipv4": re.findall(r"inet (\d+\.\d+\.\d+\.\d+)/", v4.get("stdout") or ""),
            "ipv6": re.findall(r"inet6 ([0-9a-f:]+)/", v6.get("stdout") or ""), "ip_nonlocal_bind": nonlocal_bind,
            "listening_80": g11.listening((":80",))}


# -- owner actions ------------------------------------------------------------------------------------------------

def owner_pool12(args: dict) -> dict:
    """The owner adds a comment and a directive of their own to the site's pool (in place, as an editor saves), or
    takes back exactly those lines; then php-fpm -t and, when asked and it passed, a reload."""
    path = single_pool(args["domain"])
    unit, program = php_unit_and_program(path)
    value = {"action": "owner-pool12", "what": args["action"], "path": path, "before": g10.record_with(path, INI),
             "php_unit_before": unit_state(unit + ".service")}
    data = Path(path).read_bytes()
    if args["action"] == "append":
        g8.write_in_place(path, data.rstrip(b"\n") + b"\n" + OWNER_POOL_LINES)
    elif args["action"] == "remove":
        g8.write_in_place(path, data.replace(OWNER_POOL_LINES, b""))
    else:
        raise Refused("unknown pool action")
    value["php_fpm_test"] = run([program, "-t"], timeout=60)
    if args.get("reload") and value["php_fpm_test"].get("returncode") == 0:
        value["reload"] = run(["systemctl", "reload", unit + ".service"], timeout=120)
    value["after"] = g10.record_with(path, INI)
    value["php_unit_after"] = unit_state(unit + ".service")
    value["at"], value["epoch"] = utc(), time.time()
    return value


def owner_broken_pool(args: dict) -> dict:
    """The owner puts a pool file of their own into the pool directory whose directive PHP-FPM refuses (php-fpm -t
    then fails for every pool), without reloading; or takes back only that file."""
    path = single_pool(args["domain"])
    unit, program = php_unit_and_program(path)
    target = os.path.join(os.path.dirname(path), BROKEN_POOL_NAME)
    value = {"action": "owner-broken-pool", "path": target, "before": g8.file_record(target)}
    if args.get("remove"):
        if os.path.isfile(target) and not os.path.islink(target):
            os.unlink(target)
            value["removed"] = True
    else:
        with open(target, "wb") as stream:
            stream.write(BROKEN_POOL_TEXT)
        os.chmod(target, 0o644)
    value["after"] = g8.file_record(target)
    value["php_fpm_test"] = run([program, "-t"], timeout=60)
    value["php_unit"] = unit_state(unit + ".service")
    value["at"], value["epoch"] = utc(), time.time()
    return value


def owner_php12(args: dict) -> dict:
    """The owner stops, starts or restarts PHP-FPM (the site's version)."""
    action = args["action"]
    if action not in ("stop", "start", "restart"):
        raise Refused("unknown action")
    path = single_pool(args["domain"])
    unit, _ = php_unit_and_program(path)
    value = {"action": "owner-php12", "what": action, "unit": unit, "before": unit_state(unit + ".service")}
    value["systemctl"] = run(["systemctl", action, unit + ".service"], timeout=120)
    time.sleep(1.0)
    value["after"] = unit_state(unit + ".service")
    value["at"], value["epoch"] = utc(), time.time()
    return value


def owner_pool_chattr(args: dict) -> dict:
    path = single_pool(args["domain"])
    flag = args["flag"]
    if flag not in ("+i", "-i"):
        raise Refused("only +i or -i")
    value = {"action": "owner-pool-chattr", "flag": flag, "path": path, "chattr": run(["chattr", flag, path], timeout=20)}
    value["after"] = g8.file_record(path, text=False)
    value["at"], value["epoch"] = utc(), time.time()
    return value


def owner_vhost12(args: dict) -> dict:
    """The owner binds the site's port-80 block: to one address (``bind-address``, set11's edit), to ``[::]:80``
    only (``ipv6-only``: the ``listen 80;`` line removed); each followed by nginx -t and a reload when asked."""
    domain = rid.domain_name(args["domain"])
    action = args["action"]
    path = f"{AVAILABLE}/{domain}.conf"
    value = {"action": "owner-vhost12", "what": action, "domain": domain, "before": {"vhost": g10.record(path)}}
    text = Path(path).read_text()
    if action == "bind-address":
        new, changed = g11.edit_text(text, domain, "bind-address", args)
    elif action == "ipv6-only":
        lines = text.split("\n")
        end = g8.server_block_end(text, domain)
        if end is None:
            raise Refused("no server block names the domain")
        start = max(i for i in range(end) if re.match(r"\s*server\s*\{", lines[i].split("#", 1)[0]))
        out, removed = [], []
        for index, line in enumerate(lines):
            if start <= index <= end and line.strip() == "listen 80;":
                removed.append(line.strip())
                continue
            out.append(line)
        if not removed or not any(l.strip() == "listen [::]:80;" for l in lines[start:end + 1]):
            raise Refused("the block has no plain port-80 pair of listen lines")
        new, changed = "\n".join(out), {"removed_lines": removed, "kept": "listen [::]:80;"}
    else:
        raise Refused("unknown owner action")
    g8.write_in_place(path, new.encode())
    value["changed"] = changed
    value["after"] = {"vhost": g10.record(path)}
    value.update(g8.nginx_reload(bool(args.get("reload"))))
    value["listening_80"] = g11.listening((":80",))
    value["at"], value["epoch"] = utc(), time.time()
    return value


# -- lab preparation ----------------------------------------------------------------------------------------------

def write_back(path: str, data: bytes, uid: int, gid: int, mode: int) -> None:
    temporary = path + ".set12-lab"
    with open(temporary, "wb") as stream:
        stream.write(data)
        stream.flush()
        os.fsync(stream.fileno())
    os.chown(temporary, uid, gid)
    os.chmod(temporary, mode)
    os.rename(temporary, path)


def lab_pool_restore(args: dict) -> dict:
    """Lab preparation: the site's pool written back to recorded bytes (owner, group and mode as recorded), its
    pending file removed, PHP-FPM reloaded."""
    path = single_pool(args["domain"])
    unit, program = php_unit_and_program(path)
    data = base64.b64decode(args["content_b64"])
    before = g8.file_record(path, text=False)
    write_back(path, data, int(args["uid"]), int(args["gid"]), int(str(args["file_mode"]), 8))
    pending = path + g10.PENDING_SUFFIX
    if os.path.isfile(pending) and not os.path.islink(pending):
        os.unlink(pending)
    value = {"action": "lab-pool-restore", "path": path, "before": before, "after": g8.file_record(path, text=False),
             "sha256_matches": hashlib.sha256(data).hexdigest() == hashlib.sha256(Path(path).read_bytes()).hexdigest(),
             "php_fpm_test": run([program, "-t"], timeout=60)}
    if value["php_fpm_test"].get("returncode") == 0 and unit_state(unit + ".service").get("ActiveState") == "active":
        value["reload"] = run(["systemctl", "reload", unit + ".service"], timeout=120)
    value["at"] = utc()
    return value


def lab_slow_reload(args: dict) -> dict:
    """Lab preparation: a drop-in for the PHP-FPM unit whose reload waits ``seconds`` (and does nothing else), so that
    one pool operation of the Agent holds its pool lock that long; or its removal. daemon-reload after either."""
    path = single_pool(args["domain"])
    unit, _ = php_unit_and_program(path)
    directory = f"/etc/systemd/system/{unit}.service.d"
    target = os.path.join(directory, SLOW_DROPIN)
    value = {"action": "lab-slow-reload", "path": target}
    if args.get("remove"):
        if os.path.isfile(target) and not os.path.islink(target):
            os.unlink(target)
            value["removed"] = True
        try:
            os.rmdir(directory)
            value["directory_removed"] = True
        except OSError:
            pass
    else:
        seconds = int(args.get("seconds", 75))
        if not 10 <= seconds <= 85:
            raise Refused("10 to 85 seconds")
        os.makedirs(directory, exist_ok=True)
        with open(target, "w") as stream:
            stream.write("# set12 lab preparation: PHP-FPM's reload waits %d s and does nothing else\n"
                         "[Service]\nExecReload=\nExecReload=/bin/sleep %d\n" % (seconds, seconds))
        os.chmod(target, 0o644)
        value["text"] = Path(target).read_text()
    value["daemon_reload"] = run(["systemctl", "daemon-reload"], timeout=90)
    value["exec_reload"] = (run(["systemctl", "show", unit + ".service", "-p", "ExecReload"], timeout=30).get("stdout") or "")[:400]
    value["at"], value["epoch"] = utc(), time.time()
    return value


def lab_nonlocal_bind(args: dict) -> dict:
    """Lab preparation: net.ipv4.ip_nonlocal_bind set to 1 (nginx may listen on an address this guest does not have)
    or back to the value recorded."""
    before = Path(NONLOCAL_SYSCTL).read_text().strip()
    wanted = str(int(args["value"]))
    if wanted not in ("0", "1"):
        raise Refused("0 or 1")
    Path(NONLOCAL_SYSCTL).write_text(wanted + "\n")
    return {"action": "lab-nonlocal-bind", "before": before, "after": Path(NONLOCAL_SYSCTL).read_text().strip(), "at": utc()}


MODES = dict(g11.MODES)
MODES.update({"read-pool12": read_pool12, "read-unit12": read_unit12, "read-window12": read_window12,
              "read-addresses12": read_addresses12, "read-paths12": read_paths12, "owner-pool12": owner_pool12, "owner-broken-pool": owner_broken_pool,
              "owner-php12": owner_php12, "owner-pool-chattr": owner_pool_chattr, "owner-vhost12": owner_vhost12,
              "lab-pool-restore": lab_pool_restore, "lab-slow-reload": lab_slow_reload,
              "lab-nonlocal-bind": lab_nonlocal_bind})


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
