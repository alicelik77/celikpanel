#!/usr/bin/env python3
"""set18 lab preparation (NOT a product change): a lab-only DNS and ACME fixture inside one disposable guest, so that a
fresh CelikPanel can finish its setup and an extension of it without public DNS and without any certificate authority.

What it does on the guest, and nothing else (every rule and command is printed into the JSON record):

* a small DNS responder (this file, mode ``serve``, a transient systemd unit) on 127.0.0.1, TCP and UDP, one port per
  "resolver" it stands in for (5300 pebble, 5301 1.1.1.1, 5302 8.8.8.8, 5303 9.9.9.9, 5304 208.67.222.222). It
  answers the lab zone ``set18.test`` and the reverse name of the server address from a JSON record table, NXDOMAIN
  for unknown names inside them and REFUSED for every other name; every question is logged.
* nftables table ``ip set18lab``: OUTPUT DNAT of udp/tcp 53 to 1.1.1.1, 8.8.8.8, 9.9.9.9 and 208.67.222.222 (the
  public resolvers hardcoded in cmd/ and internal/) to those ports; OUTPUT DNAT of tcp 25 to any non-local address to
  127.0.0.1:25 (the lab's "open outbound port 25"; no SMTP leaves the guest); an OUTPUT filter that rejects tcp/udp
  443 to every address except loopback, the lab networks and the package hosts of the apt sources (resolved, kept in
  a set). Table ``ip6 set18lab6`` does the same 443 filter for IPv6. A guard unit re-applies a missing table.
* routes for those four resolver addresses through the default gateway with source 192.0.2.10 (the address the
  Panel reads as this server's, `ip route get 192.0.2.1`), so that the Agent's own address reading (`ip route get
  1.1.1.1`) names the same address, as on a server with one public address.
* pebble (Debian's ``pebble`` package, the ACME test server) as a transient unit on 127.0.0.1:14000 with its own CA,
  resolving through the lab responder (port 5300), validating HTTP-01 on port 80 of this guest; its root installed into
  the system trust (``/usr/local/share/ca-certificates/set18-lab-pebble-root.crt``); certbot pointed at it by its own
  user configuration file ``/root/.config/letsencrypt/cli.ini`` (``server`` and ``no-verify-ssl``).

usage (as root, from the lab's private directory):
  guest_set18_lab_dns.py prepare --server-ip 192.0.2.10
  guest_set18_lab_dns.py set-record --what mail-a|ptr-mail
  guest_set18_lab_dns.py state
  guest_set18_lab_dns.py serve | guard        (started by prepare as transient units)
"""
from __future__ import annotations

import argparse
import json
import os
from pathlib import Path
import re
import selectors
import socket
import struct
import subprocess
import sys
import time

LAB = Path("/root/celikpanel-release-recovery-lab/set18-lab")
RECORDS = LAB / "records.json"
QUERY_LOG = LAB / "resolver-queries.jsonl"
GUARD_LOG = LAB / "guard.jsonl"
RECORD_LOG = LAB / "record-changes.jsonl"
PEBBLE_DIR = LAB / "pebble"
ZONE = "set18.test"
PANEL = "panel.set18.test"
MAIL = "mail.set18.test"
PROVIDER = "host-192-0-2-10.provider.set18.test"
PORTS = {5300: "pebble", 5301: "1.1.1.1", 5302: "8.8.8.8", 5303: "9.9.9.9", 5304: "208.67.222.222"}
RESOLVERS = {"1.1.1.1": 5301, "8.8.8.8": 5302, "9.9.9.9": 5303, "208.67.222.222": 5304}
ALWAYS_ALLOWED_HOSTS = ("deb.debian.org", "security.debian.org", "packages.sury.org")
CLI_INI = Path("/root/.config/letsencrypt/cli.ini")
CA_FILE = Path("/usr/local/share/ca-certificates/set18-lab-pebble-root.crt")
HERE = Path(__file__).resolve()
TABLE4, TABLE6 = "set18lab", "set18lab6"


def now() -> str:
    return time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()) + f"{time.time() % 1:.3f}"[1:]


def log(path: Path, value: dict) -> None:
    with path.open("a") as handle:
        handle.write(json.dumps({"at": now(), "epoch": round(time.time(), 3), **value}, sort_keys=True) + "\n")


def run(argv: list[str], timeout: float = 120, check: bool = False, input_text: str | None = None) -> dict:
    result = subprocess.run(argv, capture_output=True, text=True, timeout=timeout, input=input_text)
    record = {"argv": argv, "returncode": result.returncode, "stdout": result.stdout[-3000:], "stderr": result.stderr[-2000:]}
    if check and result.returncode != 0:
        raise RuntimeError(json.dumps(record))
    return record


# -- the DNS responder --------------------------------------------------------------------------------------------

def reverse_name(ip: str) -> str:
    return ".".join(reversed(ip.split("."))) + ".in-addr.arpa"


def load_records() -> dict:
    return json.loads(RECORDS.read_text())


def encode_name(name: str) -> bytes:
    out = b""
    for label in name.rstrip(".").split("."):
        raw = label.encode("ascii")
        out += bytes([len(raw)]) + raw
    return out + b"\x00"


def decode_question(message: bytes) -> tuple[str, int, int, int]:
    offset, labels = 12, []
    while True:
        length = message[offset]
        if length == 0:
            offset += 1
            break
        if length & 0xC0:
            raise ValueError("compressed question")
        labels.append(message[offset + 1:offset + 1 + length].decode("ascii").lower())
        offset += 1 + length
    qtype, qclass = struct.unpack("!HH", message[offset:offset + 4])
    return ".".join(labels), qtype, qclass, offset + 4


def answer(message: bytes, records: dict, server_reverse_zone: str) -> tuple[bytes, dict]:
    ident, flags = struct.unpack("!HH", message[:4])
    name, qtype, qclass, end = decode_question(message)
    question = message[12:end]
    rd = flags & 0x0100
    in_zone = name == ZONE or name.endswith("." + ZONE) or name == server_reverse_zone or name.endswith("." + server_reverse_zone)
    a = records.get("A", {})
    ptr = records.get("PTR", {})
    rrs: list[bytes] = []
    values: list[str] = []
    if not in_zone or qclass != 1:
        rcode = 5
    else:
        known = name in a or name in ptr or name in (ZONE, server_reverse_zone)
        rcode = 0 if known else 3
        if qtype == 1 and name in a:
            for value in a[name]:
                rrs.append(b"\xc0\x0c" + struct.pack("!HHIH", 1, 1, 60, 4) + socket.inet_aton(value))
                values.append(value)
        elif qtype == 12 and name in ptr:
            target = encode_name(ptr[name])
            rrs.append(b"\xc0\x0c" + struct.pack("!HHIH", 12, 1, 60, len(target)) + target)
            values.append(ptr[name])
    header = struct.pack("!HHHHHH", ident, 0x8000 | 0x0400 | rd | 0x0080 | rcode, 1, len(rrs), 0, 0)
    return header + question + b"".join(rrs), {"qname": name, "qtype": qtype, "rcode": rcode, "answers": values}


def serve() -> int:
    records_mtime, records = 0.0, {}
    server_ip = json.loads((LAB / "lab.json").read_text())["server_ip"]
    reverse_zone = ".".join(reversed(server_ip.split(".")[:3])) + ".in-addr.arpa"
    selector = selectors.DefaultSelector()
    for port, label in PORTS.items():
        udp = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
        udp.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        udp.bind(("127.0.0.1", port))
        selector.register(udp, selectors.EVENT_READ, ("udp", label))
        tcp = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        tcp.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        tcp.bind(("127.0.0.1", port))
        tcp.listen(64)
        selector.register(tcp, selectors.EVENT_READ, ("tcp", label))
    log(QUERY_LOG, {"event": "listening", "ports": PORTS})

    def respond(message: bytes, proto: str, label: str, client: str) -> bytes | None:
        nonlocal records_mtime, records
        try:
            mtime = RECORDS.stat().st_mtime
            if mtime != records_mtime:
                records, records_mtime = load_records(), mtime
            reply, info = answer(message, records, reverse_zone)
        except Exception as exc:  # noqa: BLE001 - a malformed question is logged and dropped
            log(QUERY_LOG, {"event": "dropped", "as": label, "proto": proto, "client": client, "error": str(exc)[:200]})
            return None
        log(QUERY_LOG, {"event": "query", "as": label, "proto": proto, "client": client, **info})
        return reply

    while True:
        for key, _ in selector.select(timeout=5):
            sock, (proto, label) = key.fileobj, key.data
            if proto == "udp":
                message, address = sock.recvfrom(4096)
                reply = respond(message, proto, label, address[0])
                if reply:
                    sock.sendto(reply, address)
            else:
                conn, address = sock.accept()
                conn.settimeout(3)
                try:
                    size = conn.recv(2)
                    if len(size) == 2:
                        length = struct.unpack("!H", size)[0]
                        message = b""
                        while len(message) < length:
                            chunk = conn.recv(length - len(message))
                            if not chunk:
                                break
                            message += chunk
                        reply = respond(message, proto, label, address[0])
                        if reply:
                            conn.sendall(struct.pack("!H", len(reply)) + reply)
                except OSError:
                    pass
                finally:
                    conn.close()


# -- nftables, routes, allow-list -----------------------------------------------------------------------------------

def apt_https_hosts() -> list[str]:
    hosts = set(ALWAYS_ALLOWED_HOSTS)
    paths = [Path("/etc/apt/sources.list")] + sorted(Path("/etc/apt/sources.list.d").glob("*"))
    for path in paths:
        try:
            text = path.read_text()
        except OSError:
            continue
        hosts.update(re.findall(r"https://([A-Za-z0-9.-]+)", text))
    return sorted(hosts)


def resolve(hosts: list[str]) -> tuple[list[str], list[str]]:
    v4, v6 = set(), set()
    for host in hosts:
        try:
            for family, _, _, _, sockaddr in socket.getaddrinfo(host, 443, proto=socket.IPPROTO_TCP):
                (v4 if family == socket.AF_INET else v6).add(sockaddr[0])
        except OSError:
            continue
    return sorted(v4), sorted(v6)


def ruleset(server_ip: str) -> str:
    dnat = "\n".join(f"        ip daddr {address} {proto} dport 53 counter dnat to 127.0.0.1:{port}"
                     for address, port in RESOLVERS.items() for proto in ("udp", "tcp"))
    return f"""table ip {TABLE4} {{
    set allow443 {{ type ipv4_addr; }}
    chain out_nat {{
        type nat hook output priority -100; policy accept;
{dnat}
        tcp dport 25 ip daddr != {{ 127.0.0.0/8, {server_ip}, 10.0.2.0/24 }} counter dnat to 127.0.0.1:25
    }}
    chain out_filter {{
        type filter hook output priority 0; policy accept;
        tcp dport 443 ip daddr {{ 127.0.0.0/8, 192.0.2.0/24, 10.0.2.0/24 }} counter accept
        tcp dport 443 ip daddr @allow443 counter accept
        tcp dport 443 counter log prefix "set18-lab-443-block " reject with tcp reset
        udp dport 443 counter reject
    }}
}}
table ip6 {TABLE6} {{
    set allow443 {{ type ipv6_addr; }}
    chain out_filter {{
        type filter hook output priority 0; policy accept;
        tcp dport 443 ip6 daddr ::1 counter accept
        tcp dport 443 ip6 daddr @allow443 counter accept
        tcp dport 443 counter log prefix "set18-lab-443-block6 " reject with tcp reset
        udp dport 443 counter reject
    }}
}}
"""


def add_allowed(v4: list[str], v6: list[str]) -> list[dict]:
    done = []
    if v4:
        done.append(run(["nft", "add", "element", "ip", TABLE4, "allow443", "{ " + ", ".join(v4) + " }"]))
    if v6:
        done.append(run(["nft", "add", "element", "ip6", TABLE6, "allow443", "{ " + ", ".join(v6) + " }"]))
    return done


def default_route() -> tuple[str, str]:
    text = run(["ip", "-4", "route", "show", "default"])["stdout"]
    match = re.search(r"default via (\S+) dev (\S+)", text)
    if not match:
        raise RuntimeError("no IPv4 default route")
    return match.group(1), match.group(2)


def apply_routes(server_ip: str) -> list[dict]:
    gateway, device = default_route()
    return [run(["ip", "route", "replace", f"{address}/32", "via", gateway, "dev", device, "src", server_ip])
            for address in RESOLVERS]


def tables_present() -> bool:
    return (run(["nft", "list", "table", "ip", TABLE4])["returncode"] == 0
            and run(["nft", "list", "table", "ip6", TABLE6])["returncode"] == 0)


def apply_tables(server_ip: str) -> dict:
    for family, table in (("ip", TABLE4), ("ip6", TABLE6)):
        if run(["nft", "list", "table", family, table])["returncode"] == 0:
            run(["nft", "delete", "table", family, table])
    text = ruleset(server_ip)
    (LAB / "set18lab.nft").write_text(text)
    applied = run(["nft", "-f", str(LAB / "set18lab.nft")])
    v4, v6 = resolve(apt_https_hosts())
    return {"ruleset": text, "nft_f": applied, "allowed_hosts": apt_https_hosts(), "allowed_v4": v4, "allowed_v6": v6,
            "add_elements": add_allowed(v4, v6)}


def guard() -> int:
    server_ip = json.loads((LAB / "lab.json").read_text())["server_ip"]
    seen4: set[str] = set()
    seen6: set[str] = set()
    last_refresh = 0.0
    log(GUARD_LOG, {"event": "started"})
    while True:
        try:
            if not tables_present():
                record = apply_tables(server_ip)
                seen4, seen6 = set(record["allowed_v4"]), set(record["allowed_v6"])
                log(GUARD_LOG, {"event": "tables-reapplied", "nft_f_rc": record["nft_f"]["returncode"]})
            routes = run(["ip", "-4", "route", "show"])["stdout"]
            missing = [a for a in RESOLVERS if not re.search(rf"^{re.escape(a)} via .* src {re.escape(server_ip)}", routes, re.M)]
            if missing:
                log(GUARD_LOG, {"event": "routes-reapplied", "missing": missing,
                                "result": [r["returncode"] for r in apply_routes(server_ip)]})
            if time.time() - last_refresh > 10:
                last_refresh = time.time()
                v4, v6 = resolve(apt_https_hosts())
                new4, new6 = sorted(set(v4) - seen4), sorted(set(v6) - seen6)
                if new4 or new6:
                    add_allowed(new4, new6)
                    seen4.update(new4)
                    seen6.update(new6)
                    log(GUARD_LOG, {"event": "allow443-added", "v4": new4, "v6": new6})
        except Exception as exc:  # noqa: BLE001 - the guard keeps going and says why
            log(GUARD_LOG, {"event": "error", "error": str(exc)[:300]})
        time.sleep(2)


# -- prepare --------------------------------------------------------------------------------------------------------

def dns_probe(resolver: str, name: str, qtype: int, proto: str = "udp") -> dict:
    ident = int.from_bytes(os.urandom(2), "big")
    query = struct.pack("!HHHHHH", ident, 0x0100, 1, 0, 0, 0) + encode_name(name) + struct.pack("!HH", qtype, 1)
    try:
        if proto == "udp":
            sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
            sock.settimeout(3)
            sock.sendto(query, (resolver, 53))
            reply = sock.recv(4096)
        else:
            sock = socket.create_connection((resolver, 53), timeout=3)
            sock.sendall(struct.pack("!H", len(query)) + query)
            length = struct.unpack("!H", sock.recv(2))[0]
            reply = b""
            while len(reply) < length:
                reply += sock.recv(length - len(reply))
        sock.close()
        flags, _, ancount = struct.unpack("!HHH", reply[2:8])
        return {"resolver": resolver, "proto": proto, "name": name, "qtype": qtype, "rcode": flags & 15, "answers": ancount}
    except OSError as exc:
        return {"resolver": resolver, "proto": proto, "name": name, "qtype": qtype, "error": str(exc)[:120]}


def fetch(url: str) -> str | None:
    """A lab-loopback HTTPS read (pebble's own listener certificate is not trusted, by design)."""
    import ssl
    import urllib.request
    context = ssl.create_default_context()
    context.check_hostname = False
    context.verify_mode = ssl.CERT_NONE
    try:
        with urllib.request.urlopen(url, timeout=5, context=context) as response:
            return response.read().decode()
    except OSError:
        return None


def start_unit(unit: str, argv: list[str], environment: dict | None = None) -> dict:
    run(["systemctl", "stop", unit])
    run(["systemctl", "reset-failed", unit])
    command = ["systemd-run", f"--unit={unit}", "--property=Restart=on-failure", "--property=RestartSec=2",
               "--property=UMask=0077"]
    for key, value in (environment or {}).items():
        command.append(f"--setenv={key}={value}")
    return run(command + argv)


def prepare(server_ip: str) -> dict:
    LAB.mkdir(parents=True, exist_ok=True, mode=0o700)
    record: dict = {"what": "set18 lab preparation (not a product change): lab-only DNS, ACME and outbound rules",
                    "at": now(), "server_ip": server_ip}
    seen_src = run(["ip", "-4", "route", "get", "192.0.2.1"])["stdout"]
    record["route_to_192_0_2_1"] = seen_src.strip()
    if f"src {server_ip}" not in seen_src:
        raise RuntimeError(f"this guest does not read {server_ip} as its address: {seen_src!r}")
    (LAB / "lab.json").write_text(json.dumps({"server_ip": server_ip}) + "\n")
    rev = reverse_name(server_ip)
    records = {"A": {PANEL: [server_ip], PROVIDER: [server_ip]}, "PTR": {rev: PROVIDER},
               "note": f"{PANEL} prepared before setup; {MAIL} absent until the owner adds it; PTR at the provider's default name"}
    RECORDS.write_text(json.dumps(records, indent=2, sort_keys=True) + "\n")
    log(RECORD_LOG, {"event": "initial", "records": records})
    record["records_initial"] = records
    # nft (the nftables package: not in the Debian cloud image; setup's own nftables component then reads it as
    # already installed) and pebble, from the distribution's repository, before any rule exists.
    env = {**os.environ, "DEBIAN_FRONTEND": "noninteractive"}
    update = subprocess.run(["apt-get", "update"], capture_output=True, text=True, timeout=600, env=env)
    install = subprocess.run(["apt-get", "install", "-y", "--no-install-recommends", "nftables", "pebble"], capture_output=True,
                             text=True, timeout=900, env=env)
    record["packages_install"] = {"update_rc": update.returncode, "install_rc": install.returncode,
                                  "packages": ["nftables", "pebble"], "install_tail": install.stdout[-1500:],
                                  "stderr_tail": install.stderr[-800:],
                                  "nftables_service": run(["sh", "-c", "systemctl is-enabled nftables.service 2>&1; systemctl is-active nftables.service 2>&1 || true"])["stdout"]}
    record["pebble_install"] = record["packages_install"]
    record["resolver_unit"] = start_unit("set18-lab-resolver.service", ["python3", "-I", str(HERE), "serve"])
    record["apt_sources"] = run(["sh", "-c", "grep -rhE '^(deb |URIs:)' /etc/apt/sources.list /etc/apt/sources.list.d/ 2>/dev/null"])["stdout"]
    record["tables"] = apply_tables(server_ip)
    record["routes"] = apply_routes(server_ip)
    record["route_get_1_1_1_1"] = run(["ip", "-4", "route", "get", "1.1.1.1"])["stdout"].strip()
    time.sleep(1)
    record["dns_checks"] = [dns_probe(r, PANEL, 1, proto) for r in RESOLVERS for proto in ("udp", "tcp")] + \
        [dns_probe("1.1.1.1", rev, 12, "tcp"), dns_probe("8.8.8.8", MAIL, 1, "tcp"), dns_probe("1.1.1.1", "example.com", 1)]
    blocked = run(["sh", "-c", "timeout 8 bash -c 'exec 3<>/dev/tcp/203.0.113.1/443' 2>&1; echo rc=$?"], timeout=20)
    record["block_check_203_0_113_1_443"] = blocked["stdout"].strip()
    record["pebble_version"] = run(["sh", "-c", "dpkg-query -W pebble; command -v pebble"])["stdout"]
    record["pebble_packaged_units"] = run(["sh", "-c", "systemctl list-unit-files --no-pager 'pebble*' 2>&1; systemctl is-active pebble.service 2>&1 || true"])["stdout"]
    run(["systemctl", "disable", "--now", "pebble.service"])
    PEBBLE_DIR.mkdir(parents=True, exist_ok=True, mode=0o700)
    run(["openssl", "req", "-x509", "-newkey", "ec", "-pkeyopt", "ec_paramgen_curve:P-256", "-nodes", "-days", "30",
         "-subj", "/CN=set18-lab-pebble-listener", "-addext", "subjectAltName=IP:127.0.0.1,DNS:localhost",
         "-keyout", str(PEBBLE_DIR / "listener-key.pem"), "-out", str(PEBBLE_DIR / "listener-cert.pem")], check=True)
    config = {"pebble": {"listenAddress": "127.0.0.1:14000", "managementListenAddress": "127.0.0.1:15000",
                         "certificate": str(PEBBLE_DIR / "listener-cert.pem"), "privateKey": str(PEBBLE_DIR / "listener-key.pem"),
                         "httpPort": 80, "tlsPort": 443, "ocspResponderURL": "", "externalAccountBindingRequired": False}}
    (PEBBLE_DIR / "pebble-config.json").write_text(json.dumps(config, indent=2) + "\n")
    record["pebble_config"] = config
    pebble_env = {"PEBBLE_VA_NOSLEEP": "1", "PEBBLE_WFE_NONCEREJECT": "0"}
    record["pebble_unit"] = start_unit("set18-lab-pebble.service",
                                       ["/usr/bin/pebble", "-config", str(PEBBLE_DIR / "pebble-config.json"),
                                        "-dnsserver", "127.0.0.1:5300"], pebble_env)
    record["pebble_environment"] = pebble_env
    root = None
    for _ in range(30):
        root = fetch("https://127.0.0.1:15000/roots/0")
        if root and "BEGIN CERTIFICATE" in root:
            break
        root = None
        time.sleep(1)
    if root is None:
        raise RuntimeError("pebble's root could not be read")
    CA_FILE.write_text(root)
    record["pebble_root"] = run(["openssl", "x509", "-noout", "-subject", "-fingerprint", "-sha256", "-in", str(CA_FILE)])["stdout"]
    record["update_ca_certificates"] = run(["update-ca-certificates"])
    record["directory"] = (fetch("https://127.0.0.1:14000/dir") or "")[:800]
    CLI_INI.parent.mkdir(parents=True, exist_ok=True)
    CLI_INI.write_text("# set18 lab only (disposable guest): certbot uses the lab's own ACME server inside this guest.\n"
                       "server = https://127.0.0.1:14000/dir\nno-verify-ssl = true\n")
    record["certbot_cli_ini"] = {"path": str(CLI_INI), "text": CLI_INI.read_text()}
    record["guard_unit"] = start_unit("set18-lab-guard.service", ["python3", "-I", str(HERE), "guard"])
    record["finished_at"] = now()
    return record


def set_record(what: str) -> dict:
    server_ip = json.loads((LAB / "lab.json").read_text())["server_ip"]
    records = load_records()
    before = json.loads(json.dumps(records))
    if what == "mail-a":
        records["A"][MAIL] = [server_ip]
        meaning = f"the owner adds the A record {MAIL} -> {server_ip} at the DNS provider (lab record table)"
    elif what == "ptr-mail":
        records["PTR"][reverse_name(server_ip)] = MAIL
        meaning = f"the owner sets the reverse DNS of {server_ip} to {MAIL} at the server provider (lab record table)"
    else:
        raise ValueError("unknown record change")
    temporary = RECORDS.with_suffix(".tmp")
    temporary.write_text(json.dumps(records, indent=2, sort_keys=True) + "\n")
    os.replace(temporary, RECORDS)
    log(RECORD_LOG, {"event": what, "meaning": meaning, "before": before, "after": records})
    return {"at": now(), "what": what, "meaning": meaning, "after": records,
            "checks": [dns_probe("1.1.1.1", MAIL, 1, "tcp"), dns_probe("8.8.8.8", reverse_name(server_ip), 12, "tcp")]}


def tail(path: Path, lines: int) -> list:
    try:
        return [json.loads(line) for line in path.read_text().splitlines()[-lines:]]
    except (OSError, ValueError):
        return []


def state() -> dict:
    return {"at": now(),
            "nft": run(["sh", "-c", f"nft list table ip {TABLE4}; nft list table ip6 {TABLE6}"])["stdout"],
            "routes": run(["ip", "-4", "route", "show"])["stdout"],
            "route_get_1_1_1_1": run(["ip", "-4", "route", "get", "1.1.1.1"])["stdout"].strip(),
            "units": run(["sh", "-c", "systemctl show set18-lab-resolver.service set18-lab-pebble.service set18-lab-guard.service --no-pager -p Id -p ActiveState -p SubState -p NRestarts"])["stdout"],
            "records": load_records() if RECORDS.exists() else None,
            "record_changes": tail(RECORD_LOG, 20),
            "guard_events": tail(GUARD_LOG, 40),
            "queries_total": sum(1 for _ in QUERY_LOG.open()) if QUERY_LOG.exists() else 0,
            "queries_last": tail(QUERY_LOG, 60),
            "pebble_journal": run(["journalctl", "-u", "set18-lab-pebble.service", "--no-pager", "-o", "short-iso", "-n", "120"])["stdout"][-12000:],
            "block_log": run(["sh", "-c", "journalctl -k --no-pager -o short-iso | grep -F set18-lab-443-block | tail -n 40"])["stdout"],
            "cli_ini": CLI_INI.read_text() if CLI_INI.exists() else None,
            "letsencrypt_renewal": run(["sh", "-c", "for f in /etc/letsencrypt/renewal/*.conf; do echo \"== $f\"; grep -E '^(server|no_verify_ssl|authenticator|account) ' \"$f\"; done 2>/dev/null"])["stdout"],
            "ca_file": run(["openssl", "x509", "-noout", "-subject", "-fingerprint", "-sha256", "-in", str(CA_FILE)])["stdout"] if CA_FILE.exists() else None}


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("mode", choices=("prepare", "serve", "guard", "set-record", "state"))
    parser.add_argument("--server-ip")
    parser.add_argument("--what")
    args, _ = parser.parse_known_args()
    if os.geteuid() != 0:
        print(json.dumps({"error": "run as root"}))
        return 2
    if args.mode == "serve":
        return serve()
    if args.mode == "guard":
        return guard()
    if args.mode == "prepare":
        if not args.server_ip or not re.fullmatch(r"\d+\.\d+\.\d+\.\d+", args.server_ip):
            parser.error("prepare needs --server-ip")
        value = prepare(args.server_ip)
    elif args.mode == "set-record":
        value = set_record(args.what or "")
    else:
        value = state()
    print(json.dumps(value, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
