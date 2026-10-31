# A pending paired publication completes by itself — evidence (2026-10-30)

Branch `fix/pending-publication-self-recovery` (476dcdf94, e647a6735,
e65f0f60b), merged into `release/alpha113`. QEMU/KVM guests on the archlinux
WSL host, beta1 harness, acceptance-licence build of e65f0f60b (not a
release; `dist.json`); lab network only. Secret scan clean (109 files,
`secret-scan.json`). Logs: `run-pair.out`, `run-ops.out`,
`inspect-ppub-a-{main,backup}.log`; full set in
`pending-publication-evidence.tar.gz` (SHA-256 beside it).

Topology as the owner's servers: Debian 13 first server with PowerDNS main
DNS (192.0.3.82); Ubuntu 24.04 second server joined by code as PowerDNS
backup (192.0.3.83).

**Accepted run 2 (e65f0f60b).**

1. Install rc 0 and licence active on both; both setups succeeded.
2. The deletion check on Ubuntu was set up by itself and shows ready on both
   pages. OpenSSH there is socket-activated (`ssh.socket` active and enabled,
   `ssh.service` disabled); the channel worked through it. Channel files and
   `sshd_config` root:root.
3. Zone add: two DNS-only domains served by both servers at once.
4. Backup unable to reach the main's DNS port (lab nft rule, a network blip):
   a record change was saved and served by the main; 409
   `DNS_PUBLICATION_FAILED`, reason `dns_peer_catching_up`, after 19 s; the
   DNS card shows it with its next check.
5. Backup PowerDNS also stopped: a change to the other zone got one attempt,
   then 409 `DNS_PUBLICATION_HELD`, reason `dns_peer_catching_up`, zone named,
   after 17 s; saved, not yet served. The panel kept re-checking by itself.
6. Backup back, no owner action: the scheduled re-check completed the same
   operation 27 s later; the card cleared at 33 s; the held change was
   published by itself 4 s after that. Both records served by both servers.
7. Zone delete: 200; the Ubuntu inspector's answer was accepted; proof
   verified in 16.1 s.
8. A change after it: 200, on both servers at once.

**Run 1 (e647a6735, logs not kept).** Every step completed by itself, but
step 4 answered after 31 s with the generic text and no reason (an immediate
second check exceeded the 30 s request limit), and step 5 was held as
pending verification. Fixed in e65f0f60b.

**Not shown here.** BIND or Arch backups; outages longer than the 5-minute
backoff cap; a panel restart while a publication is pending, and causes that
need a person (component tests only); installations updated from an earlier
release. P0.4 stays open.
