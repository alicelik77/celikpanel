#!/usr/bin/env python3
"""set11: native readers, two read-only watchers and the owner's hand actions for the eleventh native record (the
corrections committed after the tenth: the certificate-validation probe, the unwritable state, the include directory
mode) on a marked disposable QEMU guest.

Built on ``guest_set10_native.py`` (its readers, owner actions and lab preparation are reused unchanged). Added:

Readers (``read-*``) only inspect: CelikPanel's own include directory of a site
(``/etc/nginx/celikpanel-managed.d/<domain>/``, its ``acme-http-01.conf`` with the challenge root named in it), the
challenge directory and every component of its path (mode, owner, group), the include bases, nginx's listening
sockets and its worker user, the new bytes of nginx's log files since recorded offsets, the guest's own addresses.

Watchers (``watch-start`` / ``watch-stop``) observe only: an inotify watcher (create, write, attribute change,
removal) on the directories named, with each entry's mode, owner, size and (for a file under 4 KiB) its bytes, and a
packet reader on the loopback interface that records, for TCP port 80, each HTTP request line with its Host header and
each answer's status line with the first bytes of its body, and every connection attempt (SYN) to port 80 or 443.
Both run as transient units on this guest and write JSON lines under /root/celikpanel-release-recovery-lab/; nothing
is sent anywhere. A probe body is a random value the Agent writes into a public validation path: not a secret.

Owner actions (``owner-*``) are what a server owner does by hand: remove or put back the line of a site's vhost that
includes CelikPanel's own directory; add or remove a validation-only server block for ``mail.<domain>``; bind the
site's port-80 block to one address only; stop or start nginx. Each edit is written in place and followed by
``nginx -t`` and a reload when asked.

No mode prints a password, a private key or a hash of one, and no mode contacts another host.

set11: onbirinci yerel kayıt için okuyucular, iki salt-okur izleyici (inotify ve geri döngü arayüzünde paket okuyucu)
ve sahibin elle yaptığı işlemler. Hiçbir kip parola ya da özel anahtar yazdırmaz; başka bir sunucuya bağlanmaz.
"""
from __future__ import annotations

import argparse
import base64
import glob
import importlib.util
import json
import os
from pathlib import Path
import re
import sys
import time

HERE = Path(__file__).resolve().parent
SCHEMA = "celikpanel/set11-native/v1"


def _load(name: str):
    spec = importlib.util.spec_from_file_location("set11_" + name.replace(".", "_"), HERE / name)
    value = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(value)
    return value


g10 = _load("guest_set10_native.py")
g8, rid = g10.g8, g10.rid
run, utc, Refused = rid.run, rid.utc, rid.Refused

NGINX = g8.NGINX
AVAILABLE = g8.AVAILABLE
INCLUDE_BASE = g10.INCLUDE_BASE
MANAGED_BASE = NGINX + "/celikpanel-managed.d"
LAB_DIR = "/root/celikpanel-release-recovery-lab"
FS_UNIT, NET_UNIT = "celikpanel-lab-set11-fswatch", "celikpanel-lab-set11-netwatch"
LOG_DIR = "/var/log/nginx"
MAIL_MARK = "# set11: the owner's validation-only block for "
LABEL_RE = re.compile(r"[a-z0-9-]{1,40}\Z")

# -- the two watchers (written to the guest and run as transient units) -------------------------------------------

FS_WATCH = r'''
import ctypes, ctypes.util, json, os, struct, sys, time
out, dirs = sys.argv[1], sys.argv[2:]
libc = ctypes.CDLL(ctypes.util.find_library("c") or "libc.so.6", use_errno=True)
IN_MODIFY, IN_ATTRIB, IN_CLOSE_WRITE, IN_MOVED_FROM, IN_MOVED_TO, IN_CREATE, IN_DELETE = 0x2, 0x4, 0x8, 0x40, 0x80, 0x100, 0x200
NAMES = {IN_MODIFY: "modify", IN_ATTRIB: "attrib", IN_CLOSE_WRITE: "close_write", IN_MOVED_FROM: "moved_from",
         IN_MOVED_TO: "moved_to", IN_CREATE: "create", IN_DELETE: "delete"}
fd = libc.inotify_init1(0)
watched = {}
with open(out, "a") as f:
    for d in dirs:
        wd = libc.inotify_add_watch(fd, d.encode(), IN_MODIFY | IN_ATTRIB | IN_CLOSE_WRITE | IN_MOVED_FROM | IN_MOVED_TO | IN_CREATE | IN_DELETE)
        watched[wd] = d
        f.write(json.dumps({"t": time.time(), "watch": d, "wd": wd, "errno": ctypes.get_errno() if wd < 0 else 0}) + "\n")
while True:
    data = os.read(fd, 65536)
    t = time.time()
    rows, offset = [], 0
    while offset + 16 <= len(data):
        wd, mask, cookie, length = struct.unpack_from("iIII", data, offset)
        name = data[offset + 16: offset + 16 + length].rstrip(b"\0").decode("utf-8", "replace")
        offset += 16 + length
        path = os.path.join(watched.get(wd, "?"), name)
        row = {"t": t, "dir": watched.get(wd), "name": name, "events": [v for k, v in NAMES.items() if mask & k]}
        try:
            info = os.lstat(path)
            row.update(mode=oct(info.st_mode & 0o7777), uid=info.st_uid, gid=info.st_gid, size=info.st_size,
                       inode=info.st_ino, regular=os.path.isfile(path) and not os.path.islink(path))
            if row["regular"] and info.st_size < 4096 and name.startswith("celikpanel-probe-"):
                with open(path, "rb") as stream:
                    row["bytes"] = stream.read(4096).decode("utf-8", "replace")
        except OSError as exc:
            row["lstat"] = type(exc).__name__
        rows.append(row)
    with open(out, "a") as f:
        for row in rows:
            f.write(json.dumps(row) + "\n")
'''

NET_WATCH = r'''
import json, socket, struct, sys, time
out = sys.argv[1]
s = socket.socket(socket.AF_PACKET, socket.SOCK_RAW, socket.ntohs(0x0003))
s.bind(("lo", 0))
with open(out, "a") as f:
    f.write(json.dumps({"t": time.time(), "started": "AF_PACKET on lo"}) + "\n")
while True:
    packet, address = s.recvfrom(262144)
    if address[2] == 4:      # PACKET_OUTGOING: the same packet is read again as it comes in
        continue
    t = time.time()
    kind = struct.unpack("!H", packet[12:14])[0]
    data = packet[14:]
    if kind == 0x0800 and len(data) >= 20 and data[9] == 6:
        header = (data[0] & 0x0F) * 4
        src, dst = socket.inet_ntop(socket.AF_INET, data[12:16]), socket.inet_ntop(socket.AF_INET, data[16:20])
        tcp = data[header:]
    elif kind == 0x86DD and len(data) >= 40 and data[6] == 6:
        src, dst = socket.inet_ntop(socket.AF_INET6, data[8:24]), socket.inet_ntop(socket.AF_INET6, data[24:40])
        tcp = data[40:]
    else:
        continue
    if len(tcp) < 20:
        continue
    sport, dport = struct.unpack("!HH", tcp[:4])
    flags = tcp[13]
    payload = tcp[(tcp[12] >> 4) * 4:]
    row = None
    if flags & 0x02 and not flags & 0x10 and dport in (80, 443):
        row = {"t": t, "syn": True, "src": src, "sport": sport, "dst": dst, "dport": dport}
    elif flags & 0x04 and sport in (80, 443):
        row = {"t": t, "rst_from": sport, "to_port": dport, "src": src}
    elif dport == 80 and payload[:4] in (b"GET ", b"HEAD", b"POST"):
        head = payload.split(b"\r\n\r\n", 1)[0].decode("latin-1", "replace").split("\r\n")
        host = next((l.split(":", 1)[1].strip() for l in head[1:] if l.lower().startswith("host:")), None)
        agent = next((l.split(":", 1)[1].strip()[:80] for l in head[1:] if l.lower().startswith("user-agent:")), None)
        row = {"t": t, "request": head[0][:300], "host": host, "user_agent": agent, "src": src, "sport": sport}
    elif sport == 80 and payload[:7] == b"HTTP/1.":
        parts = payload.split(b"\r\n\r\n", 1)
        head = parts[0].decode("latin-1", "replace").split("\r\n")
        body = parts[1][:160].decode("utf-8", "replace") if len(parts) > 1 else ""
        location = next((l.split(":", 1)[1].strip()[:200] for l in head[1:] if l.lower().startswith("location:")), None)
        row = {"t": t, "answer": head[0][:120], "to_port": dport, "location": location, "body_start": body}
    if row:
        with open(out, "a") as f:
            f.write(json.dumps(row) + "\n")
'''


def label_of(args: dict) -> str:
    label = str(args.get("tag", ""))
    if not LABEL_RE.fullmatch(label):
        raise Refused("a watch tag is lowercase letters, digits and dashes")
    return label


def watch_files(label: str) -> tuple:
    return f"{LAB_DIR}/set11-fs-{label}.jsonl", f"{LAB_DIR}/set11-net-{label}.jsonl"


def watch_start(args: dict) -> dict:
    label = label_of(args)
    dirs = [d for d in args.get("dirs") or [] if isinstance(d, str) and d.startswith("/") and os.path.isdir(d)]
    os.makedirs(LAB_DIR, mode=0o700, exist_ok=True)
    fs_out, net_out = watch_files(label)
    for name, body in (("set11-fswatch.py", FS_WATCH), ("set11-netwatch.py", NET_WATCH)):
        path = f"{LAB_DIR}/{name}"
        Path(path).write_text(body)
        os.chmod(path, 0o600)
    for out in (fs_out, net_out):
        Path(out).write_text("")
    value = {"tag": label, "dirs": dirs, "at": utc(), "epoch": time.time()}
    for unit in (FS_UNIT, NET_UNIT):
        run(["systemctl", "stop", unit + ".service"], timeout=30)
        run(["systemctl", "reset-failed", unit + ".service"], timeout=30)
    value["fs"] = run(["systemd-run", f"--unit={FS_UNIT}", "--property=RuntimeMaxSec=3600", "/usr/bin/python3", "-I",
                       f"{LAB_DIR}/set11-fswatch.py", fs_out] + dirs, timeout=30)
    value["net"] = run(["systemd-run", f"--unit={NET_UNIT}", "--property=RuntimeMaxSec=3600", "/usr/bin/python3", "-I",
                        f"{LAB_DIR}/set11-netwatch.py", net_out], timeout=30)
    time.sleep(1.5)
    value["active"] = {u: (run(["systemctl", "is-active", u + ".service"], timeout=20).get("stdout") or "").strip()
                       for u in (FS_UNIT, NET_UNIT)}
    return value


def jsonl(path: str, limit: int = 4000) -> list:
    rows = []
    try:
        with open(path) as stream:
            for line in stream:
                if line.strip():
                    try:
                        rows.append(json.loads(line))
                    except ValueError:
                        rows.append({"unparsed": line[:200]})
                if len(rows) >= limit:
                    rows.append({"truncated_at": limit})
                    break
    except OSError as exc:
        return [{"error": type(exc).__name__}]
    return rows


def watch_stop(args: dict) -> dict:
    label = label_of(args)
    time.sleep(float(args.get("settle", 1.0)))
    value = {"tag": label, "at": utc(), "epoch": time.time(),
             "active_before_stop": {u: (run(["systemctl", "is-active", u + ".service"], timeout=20).get("stdout") or "").strip()
                                    for u in (FS_UNIT, NET_UNIT)}}
    for unit in (FS_UNIT, NET_UNIT):
        run(["systemctl", "stop", unit + ".service"], timeout=30)
    fs_out, net_out = watch_files(label)
    value["fs"], value["net"] = jsonl(fs_out), jsonl(net_out)
    return value


# -- readers --------------------------------------------------------------------------------------------------------

def stat_record(path: str) -> dict:
    value = dict(rid._stat(path))
    try:
        info = os.lstat(path)
        value.update(inode=info.st_ino, uid=info.st_uid, gid=info.st_gid, mode_octal=oct(info.st_mode & 0o7777),
                     mtime_ns=info.st_mtime_ns)
    except OSError:
        pass
    return value


def path_chain(path: str) -> list:
    parts, chain = Path(path).parts, []
    for index in range(1, len(parts) + 1):
        chain.append(stat_record(str(Path(*parts[:index]))))
    return chain


def challenge_root_of(text: str | None) -> str | None:
    match = re.search(r"^\s*root\s+(/\S+);", text or "", re.M)
    return match.group(1) if match else None


def read_acme(args: dict) -> dict:
    value = {"at": utc(), "epoch": time.time(), "sites": {},
             "bases": {"include_base": stat_record(INCLUDE_BASE), "managed_base": stat_record(MANAGED_BASE)}}
    for domain in [rid.domain_name(d) for d in args.get("domains", [])]:
        directory = f"{MANAGED_BASE}/{domain}"
        site = {"managed_dir": stat_record(directory), "include_dir": stat_record(f"{INCLUDE_BASE}/{domain}")}
        if os.path.isdir(directory):
            site["managed_entries"] = [g10.record(os.path.join(directory, n)) for n in sorted(os.listdir(directory))]
        challenge = f"{directory}/acme-http-01.conf"
        text = Path(challenge).read_text() if os.path.isfile(challenge) else None
        root = challenge_root_of(text)
        site["challenge_root"] = root
        if root:
            target = os.path.join(root, ".well-known", "acme-challenge")
            site["challenge_dir"] = target
            site["challenge_chain"] = path_chain(target)
            site["challenge_entries"] = sorted(os.listdir(target)) if os.path.isdir(target) else None
        value["sites"][domain] = site
    workers = run(["ps", "-o", "user=,pid=,args=", "-C", "nginx"], timeout=20)
    value["nginx_processes"] = (workers.get("stdout") or "").splitlines()[:12]
    value["listening_80_443"] = listening((":80", ":443"))
    value["nginx_active"] = (run(["systemctl", "is-active", "nginx.service"], timeout=20).get("stdout") or "").strip()
    return value


def listening(ports: tuple) -> list:
    """TCP sockets listening on the given ports (``ss -ltn``, filtered here)."""
    shown = run(["ss", "-H", "-ltn"], timeout=20)
    rows = []
    for line in (shown.get("stdout") or "").splitlines():
        fields = line.split()
        if len(fields) >= 4 and any(fields[3].endswith(p) for p in ports):
            rows.append(" ".join(fields[:5]))
    return rows[:20]


def log_files() -> list:
    return sorted(p for p in glob.glob(LOG_DIR + "/*.log") if os.path.isfile(p))


def log_offsets(args: dict) -> dict:
    return {"at": utc(), "epoch": time.time(), "offsets": {p: os.path.getsize(p) for p in log_files()}}


def read_logs(args: dict) -> dict:
    offsets = args.get("offsets") or {}
    value = {"at": utc(), "epoch": time.time(), "logs": {}}
    for path in log_files():
        start = int(offsets.get(path, 0))
        size = os.path.getsize(path)
        if size < start:
            start = 0
        if size == start:
            continue
        with open(path, "rb") as stream:
            stream.seek(start)
            data = stream.read(min(size - start, 65536))
        value["logs"][path] = {"from": start, "to": size, "text": data.decode("utf-8", "replace")}
    return value


def read_addresses(args: dict) -> dict:
    shown = run(["ip", "-4", "-o", "addr", "show", "scope", "global"], timeout=20)
    addresses = re.findall(r"inet (\d+\.\d+\.\d+\.\d+)/", shown.get("stdout") or "")
    return {"at": utc(), "addresses": addresses, "ip": (shown.get("stdout") or "")[:600]}


# -- owner actions --------------------------------------------------------------------------------------------------

def managed_include_line(domain: str) -> str:
    return f"include {MANAGED_BASE}/{domain}/*.conf;"


def remove_lines(text: str, wanted: str) -> tuple:
    kept, removed = [], []
    for line in text.split("\n"):
        (removed if line.strip() == wanted else kept).append(line)
    return "\n".join(kept), removed


def block_bounds(lines: list, start: int) -> int | None:
    depth = 0
    for index in range(start, len(lines)):
        code = lines[index].split("#", 1)[0]
        depth += code.count("{") - code.count("}")
        if depth == 0 and "}" in code:
            return index
    return None


def mail_block(domain: str) -> list:
    return [MAIL_MARK + "mail." + domain + " (as CelikPanel writes it)", "server {", "    listen 80;", "    listen [::]:80;",
            f"    server_name mail.{domain};", "    # CelikPanel's own parts of this site (certificate validation)",
            f"    {managed_include_line(domain)}", "", "    location / {", "        return 404;", "    }", "}"]


def edit_text(text: str, domain: str, action: str, args: dict) -> tuple:
    """The owner's text edit; returns the new text and what changed."""
    if action == "remove-managed-include":
        new, removed = remove_lines(text, managed_include_line(domain))
        if not removed:
            raise Refused("the file has no line including CelikPanel's own directory")
        return new, {"removed_lines": removed}
    if action == "add-managed-include":
        if managed_include_line(domain) in text:
            raise Refused("the line is already in the file")
        lines = text.split("\n")
        end = g8.server_block_end(text, domain)
        if end is None:
            raise Refused("no server block names the domain")
        start = max(i for i in range(end) if re.match(r"\s*server\s*\{", lines[i].split("#", 1)[0]))
        name = next(i for i in range(start, end) if lines[i].strip().startswith("server_name"))
        added = ["    # set11: put back by the server owner", "    " + managed_include_line(domain)]
        return "\n".join(lines[:name + 1] + added + lines[name + 1:]), {"added_lines": added, "after_line": name + 1}
    if action == "add-mail-block":
        if MAIL_MARK in text:
            raise Refused("the owner's mail block is already there")
        added = [""] + mail_block(domain)
        return text.rstrip("\n") + "\n" + "\n".join(added) + "\n", {"added_lines": added}
    if action == "remove-mail-block":
        lines = text.split("\n")
        start = next((i for i, l in enumerate(lines) if l.startswith(MAIL_MARK)), None)
        if start is None:
            raise Refused("the owner's mail block is not there")
        end = block_bounds(lines, start + 1)
        if end is None:
            raise Refused("the owner's mail block has no end")
        removed = lines[start:end + 1]
        while start > 0 and lines[start - 1] == "":
            start -= 1
        return "\n".join(lines[:start] + lines[end + 1:]), {"removed_lines": removed}
    if action == "bind-address":
        address = str(args.get("address", ""))
        if not re.fullmatch(r"\d+\.\d+\.\d+\.\d+", address):
            raise Refused("an IPv4 address is needed")
        lines = text.split("\n")
        end = g8.server_block_end(text, domain)
        if end is None:
            raise Refused("no server block names the domain")
        start = max(i for i in range(end) if re.match(r"\s*server\s*\{", lines[i].split("#", 1)[0]))
        changed = []
        out = []
        for index, line in enumerate(lines):
            if start <= index <= end and line.strip() == "listen 80;":
                new = line.replace("listen 80;", f"listen {address}:80;")
                changed.append({"from": line.strip(), "to": new.strip()})
                out.append(new)
            elif start <= index <= end and line.strip() == "listen [::]:80;":
                changed.append({"from": line.strip(), "to": "(removed)"})
            else:
                out.append(line)
        if not changed:
            raise Refused("the block has no plain port-80 listen line")
        return "\n".join(out), {"changed": changed}
    raise Refused("unknown owner action")


def owner_vhost11(args: dict) -> dict:
    domain = rid.domain_name(args["domain"])
    action = args["action"]
    path = f"{AVAILABLE}/{domain}.conf"
    value = {"action": "owner-vhost11", "what": action, "domain": domain, "before": {"vhost": g10.record(path)}}
    new, changed = edit_text(Path(path).read_text(), domain, action, args)
    g8.write_in_place(path, new.encode())
    value["changed"] = changed
    value["after"] = {"vhost": g10.record(path)}
    value.update(g8.nginx_reload(bool(args.get("reload"))))
    value["at"] = utc()
    value["epoch"] = time.time()
    return value


def owner_nginx(args: dict) -> dict:
    action = args["action"]
    if action not in ("stop", "start"):
        raise Refused("unknown action")
    value = {"action": "owner-nginx", "what": action, "at": utc(), "epoch": time.time()}
    value["systemctl"] = run(["systemctl", action, "nginx.service"], timeout=90)
    time.sleep(1.0)
    value["is_active"] = (run(["systemctl", "is-active", "nginx.service"], timeout=20).get("stdout") or "").strip()
    value["listening_80"] = listening((":80",))
    return value


def owner_chattr(args: dict) -> dict:
    """The owner sets or clears the immutable flag of a site's vhost (``chattr +i`` / ``-i``), nothing else."""
    domain = rid.domain_name(args["domain"])
    flag = args["flag"]
    if flag not in ("+i", "-i"):
        raise Refused("only +i or -i")
    path = f"{AVAILABLE}/{domain}.conf"
    value = {"action": "owner-chattr", "flag": flag, "domain": domain, "before": g10.record(path, text=False)}
    value["chattr"] = run(["chattr", flag, path], timeout=20)
    value["after"] = g10.record(path, text=False)
    value["at"], value["epoch"] = utc(), time.time()
    return value


def read_stat(args: dict) -> dict:
    """One vhost's stat and attribute flags, without its text (the recheck's before/after)."""
    domain = rid.domain_name(args["domain"])
    path = f"{AVAILABLE}/{domain}.conf"
    return {"at": utc(), "epoch": time.time(), "vhost": g10.record(path, text=False),
            "side": {"pending": g10.record(path + g10.PENDING_SUFFIX, text=False),
                     "backups": [g10.record(p, text=False) for p in sorted(glob.glob(glob.escape(path) + g10.BACKUP_MARKER + "*"))]}}


MODES = dict(g10.MODES)
MODES.update({"read-acme": read_acme, "log-offsets": log_offsets, "read-logs": read_logs, "read-addresses": read_addresses,
              "read-stat": read_stat, "watch-start": watch_start, "watch-stop": watch_stop,
              "owner-vhost11": owner_vhost11, "owner-nginx": owner_nginx, "owner-chattr": owner_chattr})


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
