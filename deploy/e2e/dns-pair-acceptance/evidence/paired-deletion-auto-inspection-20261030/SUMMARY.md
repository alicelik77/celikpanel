# Paired zone deletion without terminal work — evidence (2026-10-30)

Branch `fix/paired-deletion-auto-inspection` (99b04149c..0d1a83734), merged
into `release/alpha113`. Disposable QEMU/KVM guests on the archlinux WSL host,
beta1 harness, acceptance-licence build (not a release; `dist.json`); lab
network only (labnet DNS delegation, Pebble CA). Secret scan clean (99 files,
`secret-scan.json`). Logs: `run-pair.out`, `run-ops.out`,
`inspect-pdel-a-{main,backup}.log`; full set in
`paired-deletion-evidence.tar.gz` (SHA-256 beside it).

Topology as the owner's: Debian 13 first server with PowerDNS main DNS
(192.0.3.80) and a Debian 13 second server joined by code as PowerDNS backup
(192.0.3.81).

**Accepted run 4 (04c5a9aa7).**

1. Install on both: rc 0, licence active on both.
2. Pairing: both setups succeeded. The deletion check was set up by the
   product (main enrollment active, backup channel installed, probe
   reachable); both setup pages show it ready. The owner tool's status
   commands report configured on both sides; channel files root:root,
   `sshd_config` still root:root 0644, `celikpeer` locked.
3. Zone add: served by both servers at once.
4. Record add, then delete 8 s later: both 200, gone on both; nothing said
   "no DNS engine".
5. Zone delete: 200 deleted. The inspector's answer (catalog transferred,
   zone absent and unloaded) was accepted and the proof verified in 18.5 s,
   with no terminal action; both servers refuse the zone.
6. A DNS change after the deletion: 200, published on both at once.
7. Both setups end "succeeded, complete" (the main's intermediate states were
   not sampled).

**Earlier runs (logs not kept).** (1) f0d9d5d06: the backup refused its own
new directory under the Agent's umask 027; fixed in a1c710c15. (2) a1c710c15:
the deletion stayed pending (`dns_peer_inspection_unknown`) because channel
files and `sshd_config` had become root:celikpanel; fixed in 645e55451.
(3) 645e55451: still pending; "catalog record malformed" from PowerDNS's NULL
catalog row (also affected the manual owner tool); fixed in 04c5a9aa7.

**Not shown here.** Arch, Ubuntu and BIND backups; SSH on another port or
behind a provider firewall (component tests only); installations updated
from an earlier release (no `/opt/celikpanel/dns-owner-tools` there, so their
setup page points to the owner tool); revocation without the panel. P0.4
stays open.
