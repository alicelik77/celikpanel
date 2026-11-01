#!/usr/bin/env python3
"""Lab guest "labnet" (test harness only, NOT a product change).

Runs as root on the disposable Debian 13 lab guest. It is the router of the
isolated lab segment and holds the lab's stand-ins for the internet:

* router: 192.0.3.1 on the lab bridge; NAT to its QEMU uplink only for TCP 80/443 (package mirrors); every other
  forwarded packet is dropped and counted. DNS to the public resolvers the product hardcodes (1.1.1.1, 8.8.8.8,
  9.9.9.9, 208.67.222.222) is redirected to the lab resolver; outbound TCP 25 to non-lab addresses is redirected to a
  local greeter (no SMTP leaves the lab).
* lab resolver (unbound, 192.0.2.53): iterates the lab parent zone ``test.`` from the lab TLD servers (stub), and the
  reverse zone of the lab server block from the lab provider; forwards other names upstream except a deny-list
  (celikpanel.net and certificate authorities answer NXDOMAIN).
* lab TLD (BIND view "tld", a.nic.test 192.0.2.54, b.nic.test 192.0.2.55): the parent zone ``test.``. Changing a
  delegation here is the owner's registrar step (NS + glue).
* lab DNS provider (BIND view "provider", ns1/ns2.provider.test 192.0.2.56/57): the zones an external DNS provider
  hosts (cell c), the old mail host, the old hosting of cell a's domain and the reverse zone of 192.0.3.0/24.
* lab ACME (pebble, https://192.0.2.60:14000/dir), validating HTTP-01 through the lab resolver.

usage: infra_guest.py prepare | state | delegate ZONE NS1 IP1 NS2 IP2 | undelegate ZONE
       | parent-block on|off | provider-zone ZONE JSONFILE | provider-drop ZONE | ptr IP NAME | wrong-server on|off
"""
from __future__ import annotations

import json
import os
import re
import subprocess
import sys
import time
from pathlib import Path

LAB = Path("/root/beta1")
ZONES = Path("/var/lib/bind/beta1")
TLD_ZONE = ZONES / "test.zone"
DENY = ["celikpanel.net", "celikbros.com", "letsencrypt.org", "lencr.org", "pki.goog", "zerossl.com", "buypass.com",
        "buypass.no", "ssl.com", "sectigo.com", "digicert.com", "globalsign.com", "identrust.com", "godaddy.com",
        "actalis.it", "harica.gr"]
PUBLIC_RESOLVERS = ["1.1.1.1", "8.8.8.8", "9.9.9.9", "208.67.222.222"]


def now() -> str:
    return time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())


def sh(argv, check=True, timeout=900, input_text=None):
    r = subprocess.run(argv, text=True, capture_output=True, timeout=timeout, input=input_text)
    rec = {"argv": argv, "rc": r.returncode, "out": r.stdout[-2500:], "err": r.stderr[-1500:]}
    if check and r.returncode != 0:
        raise SystemExit(json.dumps(rec))
    return rec


def log(event: dict) -> None:
    LAB.mkdir(parents=True, exist_ok=True)
    with (LAB / "events.jsonl").open("a") as f:
        f.write(json.dumps({"at": now(), **event}, sort_keys=True) + "\n")


# -- zones ----------------------------------------------------------------------------------------------------------

def serial() -> int:
    return int(time.time())


def tld_text(delegations: dict) -> str:
    lines = [f"$TTL 60", f"@ IN SOA a.nic.test. hostmaster.nic.test. ({serial()} 60 30 600 30)",
             "@ IN NS a.nic.test.", "@ IN NS b.nic.test.", "a.nic IN A 192.0.2.54", "b.nic IN A 192.0.2.55",
             "provider IN NS ns1.provider.test.", "provider IN NS ns2.provider.test.",
             "ns1.provider IN A 192.0.2.56", "ns2.provider IN A 192.0.2.57",
             "oldmail IN NS ns1.provider.test.", "oldmail IN NS ns2.provider.test."]
    for zone, servers in sorted(delegations.items()):
        label = zone[: -len(".test")]
        for name, ip in servers:
            lines.append(f"{label} IN NS {name}.")
            if ip and name.endswith("." + zone):
                lines.append(f"{name[: -len('.test')]} IN A {ip}")
    return "\n".join(lines) + "\n"


def delegations() -> dict:
    path = LAB / "delegations.json"
    return json.loads(path.read_text()) if path.exists() else {}


def write_tld(state: dict) -> dict:
    (LAB / "delegations.json").write_text(json.dumps(state, indent=2, sort_keys=True) + "\n")
    TLD_ZONE.write_text(tld_text(state))
    sh(["chown", "bind:bind", str(TLD_ZONE)])
    check = sh(["named-checkzone", "test", str(TLD_ZONE)], check=False)
    reload = sh(["rndc", "reload", "test", "IN", "tld"], check=False)
    return {"zone_text": TLD_ZONE.read_text(), "checkzone": check, "reload": reload}


def provider_zone_text(zone: str, records: list) -> str:
    lines = ["$TTL 60", f"@ IN SOA ns1.provider.test. hostmaster.provider.test. ({serial()} 60 30 600 30)",
             "@ IN NS ns1.provider.test.", "@ IN NS ns2.provider.test."]
    for rec in records:
        name, rtype, value = rec["name"], rec["type"], rec["value"]
        owner = "@" if name in ("@", zone) else (name[: -len("." + zone)] if name.endswith("." + zone) else name)
        if rtype == "TXT" and not value.startswith('"'):
            parts = [value[i:i + 250] for i in range(0, len(value), 250)]
            value = " ".join('"' + p.replace('"', '\\"') + '"' for p in parts)
        lines.append(f"{owner} IN {rtype} {value}")
    return "\n".join(lines) + "\n"


def provider_conf(zones: list[str]) -> str:
    out = []
    for zone in zones:
        out.append(f'    zone "{zone}" {{ type primary; file "{ZONES}/provider-{zone}.zone"; }};')
    return "\n".join(out)


def provider_zones() -> dict:
    path = LAB / "provider-zones.json"
    return json.loads(path.read_text()) if path.exists() else {}


def write_named_conf(prov: dict) -> None:
    conf = f"""// beta1 lab only
options {{
    directory "/var/cache/bind";
    listen-on {{ 192.0.2.54; 192.0.2.55; 192.0.2.56; 192.0.2.57; }};
    listen-on-v6 {{ none; }};
    recursion no;
    allow-query {{ any; }};
    allow-transfer {{ none; }};
    notify no;
    dnssec-validation no;
    querylog yes;
}};
logging {{ channel q {{ file "/var/log/named/beta1-queries.log" versions 3 size 20m; print-time yes; }}; category queries {{ q; }}; }};
view "tld" {{
    match-destinations {{ 192.0.2.54; 192.0.2.55; }};
    zone "test" {{ type primary; file "{TLD_ZONE}"; }};
}};
view "provider" {{
    match-destinations {{ 192.0.2.56; 192.0.2.57; }};
{provider_conf(sorted(prov))}
}};
"""
    Path("/etc/bind/named.conf.local").write_text("")
    Path("/etc/bind/named.conf.options").write_text("")
    Path("/etc/bind/named.conf").write_text(conf + 'include "/etc/bind/rndc.key";\ncontrols { inet 127.0.0.1 port 953 allow { 127.0.0.1; } keys { "rndc-key"; }; };\n')


def set_provider_zone(zone: str, records: list) -> dict:
    prov = provider_zones()
    prov[zone] = records
    (LAB / "provider-zones.json").write_text(json.dumps(prov, indent=2, sort_keys=True) + "\n")
    path = ZONES / f"provider-{zone}.zone"
    path.write_text(provider_zone_text(zone, records))
    sh(["chown", "bind:bind", str(path)])
    write_named_conf(prov)
    check = sh(["named-checkzone", zone, str(path)], check=False)
    reconfig = sh(["rndc", "reconfig"], check=False)
    reload = sh(["rndc", "reload", zone, "IN", "provider"], check=False)
    flush = sh(["unbound-control", "flush_zone", zone], check=False)
    log({"event": "provider-zone", "zone": zone, "records": records})
    return {"zone": zone, "text": path.read_text(), "checkzone": check, "reconfig": reconfig, "reload": reload, "flush": flush}


# -- prepare --------------------------------------------------------------------------------------------------------

def nft_text(blocked: list[str]) -> str:
    resolvers = ", ".join(PUBLIC_RESOLVERS)
    blocked_set = ("elements = { " + ", ".join(blocked) + " }") if blocked else ""
    return f"""table ip beta1lab {{
    set blocked {{ type ipv4_addr; {blocked_set} }}
    chain pre {{
        type nat hook prerouting priority -100; policy accept;
        iifname "lab0" ip daddr {{ {resolvers} }} udp dport 53 counter dnat to 192.0.2.53
        iifname "lab0" ip daddr {{ {resolvers} }} tcp dport 53 counter dnat to 192.0.2.53
        iifname "lab0" tcp dport 25 ip daddr != {{ 192.0.3.0/24, 192.0.2.0/24 }} counter dnat to 192.0.2.25:25
    }}
    chain post {{
        type nat hook postrouting priority 100; policy accept;
        oifname "up0" ip saddr 192.0.3.0/24 counter masquerade
    }}
    chain forward_lab {{
        type filter hook forward priority 0; policy drop;
        ct state established,related counter accept
        iifname "lab0" ip daddr @blocked counter log prefix "beta1-blocked " drop
        iifname "lab0" oifname "up0" ip saddr 192.0.3.0/24 tcp dport {{ 80, 443 }} counter accept
        iifname "lab0" oifname "lab0" counter accept
        counter log prefix "beta1-fwd-drop " drop
    }}
    chain inp {{
        type filter hook input priority 0; policy accept;
    }}
}}
"""


UNBOUND = """# beta1 lab only
server:
    interface: 192.0.2.53
    interface: 127.0.0.1
    access-control: 192.0.3.0/24 allow
    access-control: 192.0.2.0/24 allow
    access-control: 127.0.0.0/8 allow
    do-not-query-localhost: no
    module-config: "iterator"
    harden-referral-path: no
    qname-minimisation: no
    cache-max-ttl: 60
    cache-max-negative-ttl: 15
    serve-expired: no
    prefetch: no
    log-queries: yes
    log-replies: yes
    logfile: "/var/log/unbound/beta1.log"
    verbosity: 1
    use-syslog: no
    so-sndbuf: 0
    so-rcvbuf: 0
    local-zone: "test." nodefault
    domain-insecure: "test."
    local-zone: "3.0.192.in-addr.arpa." nodefault
    domain-insecure: "3.0.192.in-addr.arpa."
    private-domain: "test."
{deny}
remote-control:
    control-enable: yes
    control-interface: 127.0.0.1
    control-use-cert: no
stub-zone:
    name: "test."
    stub-addr: 192.0.2.54
    stub-addr: 192.0.2.55
stub-zone:
    name: "3.0.192.in-addr.arpa."
    stub-addr: 192.0.2.56
forward-zone:
    name: "."
    forward-addr: {upstream}
"""

GREETER = r'''
import asyncio, json, time
LOG = "/root/beta1/greeter.jsonl"
async def handle(reader, writer):
    peer = writer.get_extra_info("peername")
    with open(LOG, "a") as f:
        f.write(json.dumps({"at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()), "peer": peer[0]}) + "\n")
    writer.write(b"220 lab-greeter ESMTP (beta1 lab: nothing is delivered)\r\n")
    try:
        while True:
            line = await asyncio.wait_for(reader.readline(), 30)
            if not line:
                break
            word = line.strip().split(b" ")[0].upper()
            if word in (b"EHLO", b"HELO"):
                writer.write(b"250 lab-greeter\r\n")
            elif word == b"QUIT":
                writer.write(b"221 bye\r\n"); await writer.drain(); break
            else:
                writer.write(b"502 not in this lab\r\n")
            await writer.drain()
    except Exception:
        pass
    writer.close()
async def main():
    server = await asyncio.start_server(handle, "192.0.2.25", 25)
    async with server:
        await server.serve_forever()
asyncio.run(main())
'''


def start_unit(name: str, argv: list[str], env: dict | None = None) -> dict:
    sh(["systemctl", "stop", name], check=False)
    sh(["systemctl", "reset-failed", name], check=False)
    args = ["systemd-run", "--unit", name, "--property", "Restart=always", "--collect"]
    for k, v in (env or {}).items():
        args += ["--setenv", f"{k}={v}"]
    return sh(args + argv, check=False)


def resolve4(name: str) -> list[str]:
    r = sh(["getent", "ahostsv4", name], check=False)
    return sorted({line.split()[0] for line in r["out"].splitlines() if line.strip()})


def prepare() -> dict:
    LAB.mkdir(parents=True, exist_ok=True, mode=0o700)
    rec = {"started_at": now()}
    upstream = Path("/etc/resolv.conf").read_text()
    m = re.search(r"^nameserver\s+(\S+)", upstream, re.M)
    rec["upstream_resolver"] = m.group(1) if m else "10.0.2.3"
    env = {**os.environ, "DEBIAN_FRONTEND": "noninteractive"}
    rec["apt_update"] = subprocess.run(["apt-get", "update"], capture_output=True, text=True, env=env, timeout=900).returncode
    inst = subprocess.run(["apt-get", "install", "-y", "--no-install-recommends", "unbound", "bind9", "bind9-dnsutils",
                           "pebble", "nftables", "openssl", "curl", "tcpdump", "python3"], capture_output=True, text=True,
                          env=env, timeout=1800)
    rec["apt_install"] = {"rc": inst.returncode, "tail": inst.stdout[-1500:], "err": inst.stderr[-800:]}
    if inst.returncode != 0:
        raise SystemExit(json.dumps(rec))
    rec["versions"] = sh(["sh", "-c", "dpkg-query -W unbound bind9 pebble nftables"])["out"]
    # deny-list addresses, resolved upstream before unbound takes over
    # Address-level block only for the panel vendor's own names (CDN addresses of the CAs are shared with mirrors).
    blocked = sorted({ip for name in ("celikpanel.net", "www.celikpanel.net") for ip in resolve4(name)})
    rec["blocked_addresses"] = blocked
    sh(["sysctl", "-w", "net.ipv4.ip_forward=1"])
    Path("/etc/sysctl.d/90-beta1.conf").write_text("net.ipv4.ip_forward=1\n")
    nft = nft_text(blocked)
    (LAB / "beta1lab.nft").write_text(nft)
    sh(["nft", "delete", "table", "ip", "beta1lab"], check=False)
    rec["nft"] = sh(["nft", "-f", str(LAB / "beta1lab.nft")])
    # BIND
    ZONES.mkdir(parents=True, exist_ok=True)
    sh(["chown", "bind:bind", str(ZONES)])
    Path("/var/log/named").mkdir(exist_ok=True)
    sh(["chown", "bind:bind", "/var/log/named"])
    rev = [{"name": f"{i}.3.0.192.in-addr.arpa", "type": "PTR", "value": f"host-192-0-3-{i}.provider.test."} for i in (10, 11, 20, 21, 30, 31, 40)]
    prov = {"provider.test": [{"name": "ns1.provider.test", "type": "A", "value": "192.0.2.56"},
                              {"name": "ns2.provider.test", "type": "A", "value": "192.0.2.57"}]
            + [{"name": f"host-192-0-3-{i}.provider.test", "type": "A", "value": f"192.0.3.{i}"} for i in (10, 11, 20, 21, 30, 31, 40)],
            "oldmail.test": [{"name": "mx.oldmail.test", "type": "A", "value": "192.0.2.81"}],
            "3.0.192.in-addr.arpa": rev}
    (LAB / "provider-zones.json").write_text(json.dumps(prov, indent=2) + "\n")
    for zone, records in prov.items():
        (ZONES / f"provider-{zone}.zone").write_text(provider_zone_text(zone, records))
    (LAB / "delegations.json").write_text("{}\n")
    TLD_ZONE.write_text(tld_text({}))
    sh(["sh", "-c", f"chown bind:bind {ZONES}/*"])
    write_named_conf(prov)
    rec["named_checkconf"] = sh(["named-checkconf"], check=False)
    sh(["systemctl", "enable", "named"], check=False)
    rec["named_restart"] = sh(["systemctl", "restart", "named"], check=False)
    # unbound
    Path("/var/log/unbound").mkdir(exist_ok=True)
    sh(["chown", "unbound:unbound", "/var/log/unbound"], check=False)
    deny = "\n".join(f'    local-zone: "{d}." always_nxdomain' for d in DENY)
    Path("/etc/unbound/unbound.conf.d/beta1.conf").write_text(UNBOUND.format(deny=deny, upstream=rec["upstream_resolver"]))
    for extra in Path("/etc/unbound/unbound.conf.d").glob("*.conf"):
        if extra.name not in ("beta1.conf",):
            extra.rename(extra.with_suffix(".conf.disabled"))
    rec["unbound_checkconf"] = sh(["unbound-checkconf"], check=False)
    sh(["systemctl", "reset-failed", "unbound"], check=False)
    rec["unbound_restart"] = sh(["systemctl", "restart", "unbound"], check=False)
    rec["unbound_active"] = sh(["systemctl", "is-active", "unbound"], check=False)["out"].strip()
    if rec["unbound_active"] != "active":
        raise SystemExit(json.dumps({**rec, "error": "lab resolver did not start"}))
    # pebble
    pdir = LAB / "pebble"
    pdir.mkdir(exist_ok=True)
    sh(["systemctl", "disable", "--now", "pebble.service"], check=False)
    sh(["openssl", "req", "-x509", "-newkey", "ec", "-pkeyopt", "ec_paramgen_curve:P-256", "-nodes", "-days", "30",
        "-subj", "/CN=beta1-lab-pebble-listener", "-addext", "subjectAltName=IP:192.0.2.60",
        "-keyout", str(pdir / "listener-key.pem"), "-out", str(pdir / "listener-cert.pem")])
    cfg = {"pebble": {"listenAddress": "192.0.2.60:14000", "managementListenAddress": "192.0.2.60:15000",
                      "certificate": str(pdir / "listener-cert.pem"), "privateKey": str(pdir / "listener-key.pem"),
                      "httpPort": 80, "tlsPort": 443, "ocspResponderURL": "", "externalAccountBindingRequired": False}}
    (pdir / "pebble-config.json").write_text(json.dumps(cfg, indent=2) + "\n")
    rec["pebble_unit"] = start_unit("beta1-pebble.service", ["/usr/bin/pebble", "-config", str(pdir / "pebble-config.json"),
                                                            "-dnsserver", "192.0.2.53:53"],
                                    {"PEBBLE_VA_NOSLEEP": "1", "PEBBLE_WFE_NONCEREJECT": "0"})
    root = ""
    for _ in range(40):
        r = sh(["curl", "-fsSk", "--max-time", "5", "https://192.0.2.60:15000/roots/0"], check=False)
        if "BEGIN CERTIFICATE" in r["out"]:
            root = r["out"]
            break
        time.sleep(1)
    if not root:
        raise SystemExit(json.dumps({**rec, "error": "pebble root unreadable"}))
    (LAB / "pebble-root.pem").write_text(root)
    rec["pebble_root"] = sh(["openssl", "x509", "-noout", "-subject", "-fingerprint", "-sha256", "-in", str(LAB / "pebble-root.pem")])["out"]
    # greeter
    (LAB / "greeter.py").write_text(GREETER)
    rec["greeter_unit"] = start_unit("beta1-greeter.service", ["python3", "-I", str(LAB / "greeter.py")])
    time.sleep(2)
    rec["self_checks"] = {
        "tld_soa": sh(["dig", "+norec", "+short", "@192.0.2.54", "test.", "SOA"], check=False)["out"],
        "resolver_provider": sh(["dig", "+short", "@192.0.2.53", "ns1.provider.test", "A"], check=False)["out"],
        "resolver_deny": sh(["dig", "@192.0.2.53", "celikpanel.net", "A"], check=False)["out"][-600:],
        "resolver_mirror": sh(["dig", "+short", "@192.0.2.53", "deb.debian.org", "A"], check=False)["out"],
        "acme_dir": sh(["curl", "-fsSk", "--max-time", "5", "https://192.0.2.60:14000/dir"], check=False)["out"][:600],
    }
    rec["finished_at"] = now()
    (LAB / "prepare.json").write_text(json.dumps(rec, indent=2) + "\n")
    return rec


def parent_block(on: bool) -> dict:
    sh(["nft", "delete", "table", "ip", "beta1parent"], check=False)
    rule = None
    if on:
        rule = ("table ip beta1parent {\n    chain input_lab {\n        type filter hook input priority -10; policy accept;\n"
                "        iifname \"lab0\" ip saddr 192.0.3.0/24 ip daddr { 192.0.2.54, 192.0.2.55 } meta l4proto { tcp, udp } th dport 53 counter drop\n    }\n}\n")
        (LAB / "parent-block.nft").write_text(rule)
        sh(["nft", "-f", str(LAB / "parent-block.nft")])
    log({"event": "parent-block", "on": on})
    return {"parent_block": on, "rule": rule, "table": sh(["nft", "list", "table", "ip", "beta1parent"], check=False)["out"]}


def wrong_server(on: bool) -> dict:
    sh(["systemctl", "stop", "beta1-wrongserver.service"], check=False)
    if on:
        d = LAB / "wrong"
        d.mkdir(exist_ok=True)
        if not (d / "cert.pem").exists():
            sh(["openssl", "req", "-x509", "-newkey", "ec", "-pkeyopt", "ec_paramgen_curve:P-256", "-nodes", "-days", "5",
                "-subj", "/CN=beta1-wrong-server", "-addext", "subjectAltName=IP:192.0.3.1",
                "-keyout", str(d / "key.pem"), "-out", str(d / "cert.pem")])
        start_unit("beta1-wrongserver.service", ["openssl", "s_server", "-quiet", "-accept", "192.0.3.1:" + os.environ.get("BETA1_PANEL_PORT", "1923"),
                                                 "-cert", str(d / "cert.pem"), "-key", str(d / "key.pem"), "-www"])
    log({"event": "wrong-server", "on": on})
    return {"wrong_server": on}


def state() -> dict:
    return {"at": now(),
            "delegations": delegations(),
            "tld_zone": TLD_ZONE.read_text() if TLD_ZONE.exists() else None,
            "provider_zones": provider_zones(),
            "nft_counters": sh(["nft", "list", "ruleset"], check=False)["out"][-6000:],
            "units": sh(["sh", "-c", "systemctl is-active named unbound beta1-pebble beta1-greeter 2>&1"], check=False)["out"],
            "forward_drops": sh(["sh", "-c", "journalctl -k --no-pager -o short-iso | grep -E 'beta1-(fwd-drop|blocked)' | tail -n 30"], check=False)["out"],
            "pebble_orders": sh(["sh", "-c", "journalctl -u beta1-pebble --no-pager -o short-iso | grep -Ei 'issued|order|validat' | tail -n 60"], check=False)["out"],
            "greeter": (LAB / "greeter.jsonl").read_text()[-3000:] if (LAB / "greeter.jsonl").exists() else ""}


def main(argv: list[str]) -> int:
    cmd = argv[0]
    if cmd == "prepare":
        out = prepare()
    elif cmd == "state":
        out = state()
    elif cmd == "delegate":
        zone, ns1, ip1, ns2, ip2 = argv[1:6]
        st = delegations()
        st[zone] = [[ns1, "" if ip1 == "-" else ip1], [ns2, "" if ip2 == "-" else ip2]]
        out = write_tld(st)
        out["flush"] = sh(["unbound-control", "flush_zone", zone], check=False)
        sh(["unbound-control", "flush_infra", "all"], check=False)
        log({"event": "delegate", "zone": zone, "servers": st[zone]})
    elif cmd == "undelegate":
        st = delegations()
        st.pop(argv[1], None)
        out = write_tld(st)
        out["flush"] = sh(["unbound-control", "flush_zone", argv[1]], check=False)
        log({"event": "undelegate", "zone": argv[1]})
    elif cmd == "parent-block":
        out = parent_block(argv[1] == "on")
    elif cmd == "provider-zone":
        out = set_provider_zone(argv[1], json.loads(Path(argv[2]).read_text()))
    elif cmd == "wrong-server":
        out = wrong_server(argv[1] == "on")
    elif cmd == "flush":
        out = sh(["unbound-control", "flush_zone", argv[1]], check=False)
    else:
        raise SystemExit(__doc__)
    print(json.dumps(out, indent=2, sort_keys=True))
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
