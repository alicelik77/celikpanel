# Arch Linux PowerDNS — real-system evidence (2026-10-30)

Branch `feat/arch-powerdns`, merged into `release/alpha113`. Lab QEMU/KVM
guests on the beta1 lab network with a local Pebble CA. Acceptance builds
d24d9965 (runs a–e) and 2a5f7405 (f, g); licence behaviour is not evidenced.
Nothing outside the lab was contacted. Each cell ran once; the secret scan of
566 files was clean. The archive, its SHA-256, driver logs, `measure51.out`
and `dists.jsonl` sit beside this file.

**Measurement without the panel (`measure51.out`).** Arch `extra/powerdns`
5.1.5-1 as a fresh primary changes its database at first start exactly as
4.9 does (notified_serial, last_check NULL, one producer SOA rewrite, one
CATALOG-HASH, schema unchanged). As a consumer it writes the same member rows
as 4.9 (SLAVE, options `{"consumer":{"unique":"<label>."}}`). Package facts:
`/usr/bin/pdns_server`, user and group `powerdns`, `pdns.conf` 0644
root:root, no `pdns.d`, no `/var/lib/powerdns`.

| Run | Setup | Result |
|---|---|---|
| a | Arch alone: PowerDNS main + web + mail, through the wizard | 19/19 in about 5 minutes; authoritative SOA/MX from outside |
| b | Arch PowerDNS main + Debian 13 PowerDNS 4.9.18 backup | 13/13 + 12/12; state marker `pdns-fresh-paired-primary/arch-5.1/v1`; same SOA 2026103000, authoritative |
| c | Debian 13 BIND main + Arch PowerDNS backup | both complete; the Arch backup took in the BIND catalog; same SOA |
| d | Arch main cut 2 s after the package arrived (Agent killed, forced reboot) | cut before the switch journal; rolled back with the owner's next action; revised plan completed; `pacman -Syup` then one `-Su`; no pacman lock left |
| e | the same cut at 5 s | **defect**: undo stopped because a directory on btrfs reports link count 1; journal kept, retry blocked. Fixed in 2a5f7405 |
| f, g | the 5 s and 4 s cuts on the fixed build | cuts landed after the unit was enabled; PowerDNS started at boot; the Agent finished forward; both servers complete with the same SOA |

Not evidenced here: the exact pre-start cut on the fixed build (the btrfs fix
has unit-level proof only); a PowerDNS outside 5.1 on Arch; an in-series
`pacman -Syu` on an installed server; record edits and zone deletion on Arch;
a reboot with management disabled; a cut on the Arch backup. Product finding
(not Arch-specific): after a kept journal the setup page said the change was
undone and the owner's retry showed "running" without guidance.
