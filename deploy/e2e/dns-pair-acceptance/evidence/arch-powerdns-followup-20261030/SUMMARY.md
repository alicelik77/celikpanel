# Arch PowerDNS follow-up — evidence (2026-10-30)

Branch `fix/arch-powerdns-followup` (d17341bc3..8d9f6b97a), merged into
`release/alpha113` at 1146e70c3. Lab QEMU/KVM guests on the beta1 lab
network, acceptance-licence builds (`dists.jsonl`); nothing outside the lab
was contacted. Each cell ran once. The secret scan of 380 files was clean
(`secret-scan.json`). Full logs and the per-cell harness evidence are in
`arch-powerdns-followup-evidence.tar.gz` (SHA-256 beside it).

**Distro config matrix (build 6c63d7280 tree, podman).** Arch 73 pass,
1 warning (Arch's stock `nginx.conf`, not ours), 0 fail, 4 not applicable;
`powerdns.standalone`, `.paired-primary` and `.paired-secondary` start the
real `pdns_server` 5.1.5-1 as `powerdns` against the Agent's rendered files.
Debian 13: 82 pass (`matrix-report.md`).

| Run | Build | Setup | Result |
|---|---|---|---|
| apfu-a | dfab73d1 | Arch PowerDNS main + Debian backup; deterministic cut at the fresh primary's target-enable intent, machine reset | the undo refused its own staged database after the reboot: btrfs had renumbered the filesystem device (35 → 34; same inode and bytes). The setup page showed "held", not "undone" (d17341bc3 holding). Fixed in 9e1e31e13 |
| apfu-a2 | 9e1e31e1 | same topology, same cut | **native proof of both btrfs fixes**: undo completed, shown as rolled back, the owner's revised plan completed the Arch main. Records: add, edit, MX/TXT reached the backup within a second (`ops-apfu-a2.log`); a delete 8 s after an add was refused as "no DNS engine" (finding 1). Zone delete: both servers REFUSED at once; the proof stayed pending without peer inspection access and every later DNS change was held (finding 2). Management stopped and disabled, reboot: both servers answered authoritatively (UDP, TCP). `pacman -U` reinstall of 5.1.5-1 and restart: engine managed and running, pair ready, no untested-series flag |
| apfu-b | 9e1e31e1 | Debian PowerDNS main + Arch PowerDNS backup; cut on the backup after its target-staged journal write | undo completed, shown as rolled back, the revised plan completed the backup. The main then sat at "running verification" with no reason until the panel restarted (finding 3) |

Findings 1–3 are fixed in 8d9f6b97a (`DNS_PEER_CATCHING_UP` with a short
read-only wait; `DNS_PUBLICATION_HELD` naming the cause;
`server_setup_completion_waiting` rechecked every 20 s) with component tests
only; build 8d9f6b97a was not run on a VM here.

**Not evidenced here.** A DNS change after a paired zone deletion (blocked
by finding 2's hold; the hold itself is existing design and is addressed
separately); a 5.1.x version bump (only a same-version reinstall); a
PowerDNS outside 5.1 on Arch.
