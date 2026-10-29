#!/usr/bin/env python3
"""Beta-1 VM lab (test harness only, not a product change).

Disposable QEMU/KVM guests on one isolated Linux bridge inside the archlinux
WSL host. One lab guest ("labnet", Debian 13) is the router of the lab
segment and runs the lab's DNS (an unbound resolver standing in for the
public resolvers, a BIND serving the lab parent zone ``test.`` and a lab DNS
provider), the lab ACME server (pebble) and a port-25 greeter. Server guests
have one NIC on the bridge only; they reach package mirrors through labnet's
NAT and nothing else of the internet except through the lab resolver's
deny-list (celikpanel.net, certificate authorities) and labnet's filters.

Addresses: the server guests use 192.0.3.0/24 because the product's join and
setup link refuse documentation ranges by design (no production seam:
cmd/panel/setup_dns_remote_transport.go remoteDNSExcludedNetworks,
cmd/panel/server_setup_peer_link.go serverSetupPeerAddressAllowed). The block
is never routed anywhere: it exists only on this bridge. Lab infrastructure
(resolver, parent nameservers, provider nameservers, ACME) uses TEST-NET-1.

usage (root, on the archlinux WSL host):
  lab.py net-up | net-down
  lab.py infra-start | infra-stop
  lab.py start CELL NODE OS IP | stop CELL NODE | reboot CELL NODE
  lab.py ssh CELL NODE -- COMMAND...      (stdin is passed through)
  lab.py wait-ssh CELL NODE [SECONDS]
  lab.py identity CELL NODE
"""
from __future__ import annotations

import hashlib
import json
import os
import shutil
import socket
import subprocess
import sys
import time
import uuid
from pathlib import Path

ROOT = Path("/var/tmp/cp-beta1/lab")
IMAGES = Path("/var/tmp/cp-v3n28/images")
KEY = Path("/var/tmp/cp-beta1/keys/id_ed25519")
BRIDGE = "cpb1"
HOST_IP = "192.0.3.2/24"
LAB_GW = "192.0.3.1"
LAB_DNS = "192.0.2.53"
NAMESPACE = uuid.UUID("718f806f-5f2f-4a43-8fa0-88b642d2d432")  # the acceptance seam's namespace
MARKER_SCHEMA = "celikpanel/dns-kill-fixture-plan/v1"            # internal/licensing/acceptance_fixture.go
IMAGE_FILES = {
    "debian13": ("debian-13-genericcloud-amd64-20260826-2582.qcow2", "qcow2", "sha512",
                 "184761b0dad0f9ace02f9298050ca96ce3caa39a461a47706d47ff9698b59933918b91b40177fbd4d392f6446af8b4d18ecb94caca988169b19641606bf34003"),
    "ubuntu2404": ("ubuntu-24.04-server-cloudimg-amd64-20260826.img", "qcow2", "sha256",
                   "d0fe84bb5f80853425fa6be28e2c106f30104c3cfe8611933f2e65c9b63f0e30"),
}
INFRA = {"cell": "beta1-lab", "node": "labnet", "os": "debian13", "ip": LAB_GW}
PATH_ENV = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"


def run(argv, check=True, **kw):
    result = subprocess.run(argv, text=True, capture_output=True, env={**os.environ, "PATH": PATH_ENV}, **kw)
    if check and result.returncode != 0:
        raise SystemExit(f"command failed ({result.returncode}): {argv}\n{result.stdout}\n{result.stderr}")
    return result


def node_dir(cell: str, node: str) -> Path:
    return ROOT / "cells" / cell / node


def tap_name(cell: str, node: str) -> str:
    return "cpt" + hashlib.sha256(f"{cell}/{node}".encode()).hexdigest()[:8]


def mac(cell: str, node: str, salt: str = "lab") -> str:
    digest = hashlib.sha256(f"{cell}/{node}/{salt}".encode()).digest()
    return "52:54:00:%02x:%02x:%02x" % (digest[0], digest[1], digest[2])


def vm_uuid(cell: str, node: str) -> str:
    return str(uuid.uuid5(NAMESPACE, f"{cell}\0{node}"))


def ssh_port(cell: str, node: str) -> int:
    # Only the lab guest has a management port forward (its own uplink); servers are reached on the bridge.
    return 22900


def address_of(cell: str, node: str) -> str:
    return json.loads((node_dir(cell, node) / "node.json").read_text())["ip"]


def ssh_argv(cell: str, node: str, command: str | None = None, connect_timeout: int = 8) -> list[str]:
    nd = node_dir(cell, node)
    argv = ["ssh", "-i", str(KEY), "-o", "BatchMode=yes", "-o", f"ConnectTimeout={connect_timeout}",
            "-o", "StrictHostKeyChecking=accept-new", "-o", f"UserKnownHostsFile={nd / 'known_hosts'}",
            "-o", "ServerAliveInterval=10", "-o", "ServerAliveCountMax=6", "-o", "LogLevel=ERROR"]
    argv += [f"celik@{address_of(cell, node)}"]
    if command is not None:
        argv.append(command)
    return argv


# -- network --------------------------------------------------------------------------------------------------------

def net_up() -> dict:
    record = {}
    if run(["ip", "link", "show", BRIDGE], check=False).returncode != 0:
        run(["ip", "link", "add", BRIDGE, "type", "bridge"])
    run(["ip", "addr", "replace", HOST_IP, "dev", BRIDGE])
    run(["ip", "link", "set", BRIDGE, "up"])
    # Lab infrastructure addresses (TEST-NET-1) live on labnet; the host reaches them through it.
    run(["ip", "route", "replace", "192.0.2.0/24", "via", LAB_GW, "dev", BRIDGE])
    # Bridged frames must not traverse the host's own netfilter: the lab's filters are labnet's.
    run(["modprobe", "br_netfilter"], check=False)
    for key in ("net.bridge.bridge-nf-call-iptables", "net.bridge.bridge-nf-call-ip6tables"):
        run(["sysctl", "-w", f"{key}=0"], check=False)
    record["bridge"] = run(["ip", "-br", "addr", "show", BRIDGE]).stdout.strip()
    record["routes"] = run(["ip", "route"]).stdout
    record["ip_forward"] = Path("/proc/sys/net/ipv4/ip_forward").read_text().strip()
    return record


def net_down() -> dict:
    run(["ip", "route", "del", "192.0.2.0/24"], check=False)
    run(["ip", "link", "del", BRIDGE], check=False)
    return {"bridge": "removed"}


# -- cloud-init -----------------------------------------------------------------------------------------------------

def cloud_init(cell: str, node: str, os_name: str, ip: str, hostname: str, infra: bool) -> dict[str, str]:
    public_key = Path(str(KEY) + ".pub").read_text().strip()
    marker = "" if infra else f"""  - path: /etc/celikpanel-dns-kill-matrix
    owner: root:root
    permissions: '0444'
    content: |
      schema={MARKER_SCHEMA}
      cell_id={cell}
      node={node}
"""
    user_data = f"""#cloud-config
hostname: {hostname}
fqdn: {hostname}
manage_etc_hosts: true
users:
  - default
  - name: celik
    gecos: CelikPanel beta1 lab
    groups: [sudo]
    sudo: ["ALL=(ALL) NOPASSWD:ALL"]
    shell: /bin/bash
    lock_passwd: true
    ssh_authorized_keys:
      - {json.dumps(public_key)}
disable_root: true
ssh_pwauth: false
package_update: false
package_upgrade: false
write_files:
{marker}  - path: /etc/celikpanel-beta1-lab
    owner: root:root
    permissions: '0444'
    content: |
      cell={cell}
      node={node}
runcmd:
  - [touch, /etc/celikpanel-beta1-lab-ready]
"""
    meta = f"instance-id: beta1-{hashlib.sha256(f'{cell}/{node}/{time.time()}'.encode()).hexdigest()[:16]}\nlocal-hostname: {hostname}\n"
    if infra:
        network = f"""version: 2
ethernets:
  up:
    match: {{macaddress: "{mac(cell, node, 'up')}"}}
    set-name: up0
    dhcp4: true
    dhcp6: false
  lab:
    match: {{macaddress: "{mac(cell, node)}"}}
    set-name: lab0
    dhcp4: false
    dhcp6: false
    addresses: [{ip}/24, 192.0.2.53/24, 192.0.2.54/24, 192.0.2.55/24, 192.0.2.56/24, 192.0.2.57/24, 192.0.2.60/24, 192.0.2.25/24]
"""
    else:
        network = f"""version: 2
ethernets:
  lab:
    match: {{macaddress: "{mac(cell, node)}"}}
    set-name: lab0
    dhcp4: false
    dhcp6: false
    accept-ra: false
    addresses: [{ip}/24]
    routes:
      - {{to: default, via: {LAB_GW}}}
    nameservers:
      addresses: [{LAB_DNS}]
"""
    return {"user-data": user_data, "meta-data": meta, "network-config": network}


def start(cell: str, node: str, os_name: str, ip: str, hostname: str, memory: int = 2560, cpus: int = 2,
          infra: bool = False) -> dict:
    nd = node_dir(cell, node)
    if nd.exists():
        raise SystemExit(f"fresh node directory already exists: {nd}")
    nd.mkdir(parents=True, mode=0o700)
    filename, fmt, algorithm, digest = IMAGE_FILES[os_name]
    base = IMAGES / filename
    seed = nd / "seed"
    seed.mkdir()
    for name, text in cloud_init(cell, node, os_name, ip, hostname, infra).items():
        (seed / name).write_text(text)
    run(["qemu-img", "create", "-f", "qcow2", "-F", fmt, "-b", str(base), str(nd / "overlay.qcow2"), "24G"])
    run(["genisoimage", "-quiet", "-output", str(nd / "seed.iso"), "-volid", "cidata", "-joliet", "-rock",
         *[str(seed / n) for n in ("user-data", "meta-data", "network-config")]])
    tap = tap_name(cell, node)
    run(["ip", "tuntap", "add", "dev", tap, "mode", "tap"], check=False)
    run(["ip", "link", "set", tap, "master", BRIDGE])
    run(["ip", "link", "set", tap, "up"])
    argv = ["qemu-system-x86_64", "-name", f"beta1-{cell}-{node}", "-uuid", vm_uuid(cell, node),
            "-accel", "kvm", "-cpu", "host", "-smp", str(cpus), "-m", str(memory),
            "-drive", f"file={nd / 'overlay.qcow2'},if=virtio,format=qcow2,cache=none,discard=unmap",
            "-drive", f"file={nd / 'seed.iso'},if=virtio,format=raw,media=cdrom,readonly=on",
            "-netdev", f"tap,id=lab,ifname={tap},script=no,downscript=no",
            "-device", f"virtio-net-pci,netdev=lab,mac={mac(cell, node)}"]
    if infra:
        argv += ["-netdev", f"user,id=up,hostfwd=tcp:127.0.0.1:{ssh_port(cell, node)}-:22",
                 "-device", f"virtio-net-pci,netdev=up,mac={mac(cell, node, 'up')}"]
    argv += ["-qmp", f"unix:{nd / 'qmp.sock'},server=on,wait=off", "-pidfile", str(nd / "qemu.pid"),
             "-serial", f"file:{nd / 'serial.log'}", "-display", "none", "-monitor", "none", "-daemonize"]
    run(argv)
    info = {"cell": cell, "node": node, "os": os_name, "ip": ip, "hostname": hostname, "uuid": vm_uuid(cell, node),
            "tap": tap, "mac": mac(cell, node), "image": filename, "image_digest": {algorithm: digest},
            "memory_mb": memory, "cpus": cpus, "started_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())}
    (nd / "node.json").write_text(json.dumps(info, indent=2) + "\n")
    return info


def qmp(cell: str, node: str, command: str) -> None:
    path = node_dir(cell, node) / "qmp.sock"
    with socket.socket(socket.AF_UNIX) as s:
        s.settimeout(10)
        s.connect(str(path))
        f = s.makefile("rwb")
        f.readline()
        f.write(b'{"execute":"qmp_capabilities"}\n'); f.flush(); f.readline()
        f.write(json.dumps({"execute": command}).encode() + b"\n"); f.flush()
        try:
            f.readline()
        except OSError:
            pass


def pid_alive(cell: str, node: str) -> bool:
    try:
        pid = int((node_dir(cell, node) / "qemu.pid").read_text())
        os.kill(pid, 0)
        return True
    except (OSError, ValueError):
        return False


def stop(cell: str, node: str, delete: bool = True) -> dict:
    nd = node_dir(cell, node)
    if not nd.exists():
        return {"node": node, "state": "absent"}
    if pid_alive(cell, node):
        try:
            qmp(cell, node, "quit")
        except OSError:
            pass
        for _ in range(40):
            if not pid_alive(cell, node):
                break
            time.sleep(0.5)
    info = json.loads((nd / "node.json").read_text())
    run(["ip", "link", "del", info["tap"]], check=False)
    if delete:
        shutil.rmtree(nd)
    return {"node": node, "state": "stopped", "deleted": delete}


def wait_ssh(cell: str, node: str, seconds: int = 600) -> dict:
    deadline = time.monotonic() + seconds
    last = ""
    while time.monotonic() < deadline:
        r = subprocess.run(ssh_argv(cell, node, "test -e /etc/celikpanel-beta1-lab-ready && cat /proc/sys/kernel/random/boot_id"),
                           text=True, capture_output=True, timeout=30, env={**os.environ, "PATH": PATH_ENV})
        if r.returncode == 0 and r.stdout.strip():
            return {"node": node, "boot_id": r.stdout.strip(), "waited_seconds": round(seconds - (deadline - time.monotonic()), 1)}
        last = (r.stderr or r.stdout).strip()[-200:]
        time.sleep(3)
    raise SystemExit(f"ssh to {cell}/{node} not ready in {seconds}s: {last}")


def identity(cell: str, node: str) -> dict:
    r = run(ssh_argv(cell, node, "sudo -n cat /sys/class/dmi/id/product_uuid; cat /proc/sys/kernel/random/boot_id; cat /etc/celikpanel-beta1-lab"))
    lines = r.stdout.split()
    want = vm_uuid(cell, node)
    if not lines or lines[0].lower() != want:
        raise SystemExit(f"identity mismatch for {cell}/{node}: {lines[:1]} != {want}")
    return {"node": node, "product_uuid": lines[0], "boot_id": lines[1]}


def reboot(cell: str, node: str, seconds: int = 900) -> dict:
    before = identity(cell, node)
    started = time.monotonic()
    subprocess.run(ssh_argv(cell, node, "sudo -n systemctl reboot"), capture_output=True, timeout=60,
                   env={**os.environ, "PATH": PATH_ENV})
    deadline = started + seconds
    while time.monotonic() < deadline:
        time.sleep(3)
        try:
            after = identity(cell, node)
        except (SystemExit, subprocess.SubprocessError, OSError):
            continue
        if after["boot_id"] != before["boot_id"]:
            return {"action": "reboot", "node": node, "boot_id_before": before["boot_id"], "boot_id_after": after["boot_id"],
                    "elapsed_seconds": round(time.monotonic() - started, 1)}
    raise SystemExit(f"{cell}/{node} did not come back within {seconds}s")


def main(argv: list[str]) -> int:
    if os.geteuid() != 0:
        raise SystemExit("run as root")
    cmd = argv[0]
    if cmd == "net-up":
        out = net_up()
    elif cmd == "net-down":
        out = net_down()
    elif cmd == "infra-start":
        out = start(INFRA["cell"], INFRA["node"], INFRA["os"], INFRA["ip"], "labnet", memory=1536, cpus=2, infra=True)
    elif cmd == "infra-stop":
        out = stop(INFRA["cell"], INFRA["node"], delete=len(argv) > 1 and argv[1] == "--delete")
    elif cmd == "start":
        cell, node, os_name, ip, hostname = argv[1:6]
        memory = int(argv[6]) if len(argv) > 6 else 2560
        out = start(cell, node, os_name, ip, hostname, memory=memory)
    elif cmd == "stop":
        out = stop(argv[1], argv[2], delete=not (len(argv) > 3 and argv[3] == "--keep"))
    elif cmd == "reboot":
        out = reboot(argv[1], argv[2], int(argv[3]) if len(argv) > 3 else 900)
    elif cmd == "wait-ssh":
        out = wait_ssh(argv[1], argv[2], int(argv[3]) if len(argv) > 3 else 600)
    elif cmd == "identity":
        out = identity(argv[1], argv[2])
    elif cmd == "ssh":
        cell, node = argv[1], argv[2]
        rest = argv[4:] if len(argv) > 3 and argv[3] == "--" else argv[3:]
        os.execvpe("ssh", ssh_argv(cell, node, " ".join(rest)), {**os.environ, "PATH": PATH_ENV})
    else:
        raise SystemExit(__doc__)
    print(json.dumps(out, indent=2))
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
