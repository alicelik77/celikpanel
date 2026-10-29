# Beta-1 VM run — setup wizard v2 on fresh installs (2026-10-29)

Product commit: c2d60e57485d13d207f72ec8425e85729b144ff0 (feat/setup-wizard-v2). Acceptance build (test licence seam): licence behaviour is **not** evidenced. Disposable QEMU guests in a lab network; lab DNS root/parent, lab resolver, lab ACME (pebble). No real servers, registrars or CAs contacted. Run time about 1 h 40 min (22:45Z–00:23Z).

Server guests used 192.0.3.0/24 (lab bridge only), because the product refuses TEST-NET addresses for join/link (setup_dns_remote_transport.go:89-115, intended). Update discovery resolved celikpanel.net by name; the lab answered NXDOMAIN, nothing connected.

## Cells
| Cell | Layout | Outcome |
|---|---|---|
| a | Debian 13 first = main BIND, Ubuntu 24.04 backup, join by code, mail on first | DNS pair passed, both ready; mail blocked (P2) |
| b | Option A: Ubuntu first, Debian 13 second = PowerDNS main | blocked (P3, then P4) |
| c | External provider, two servers | passed, both ready |
| d | One Debian 13 server, standalone BIND | blocked (P1) on every retry |

## Measured (a)
- Join: code shown once; wrong code, unreachable, wrong server, source mismatch refused or reported correctly; dropped answer → unknown; replay and double submit → exactly one join.
- Names: clash refused with nothing written; main and backup answered identically.
- Registrar card: not_yet → ok after the parent edit; unknown when the parent was unreachable; never not_yet on unknown.
- Polling: no mutations.
- Reboots at peer_join, at the backup's access_dns, and at primary_dns in (b): resumed, nothing duplicated.
- Certificates: both domain certificates issued by the lab ACME; the IP pin unchanged; the backup's publisher connection succeeded.
- Owner independence: panel and agent stopped, a native rndc edit transferred to the backup; detected, not overwritten.
- DNS page: ok.

## Product defects (hypotheses)
- P0 MariaDB refused on fresh images: base point updates counted as notable installs (service_install_preview.go:187-201). Owner remedy (Components → confirm → Edit plan) worked.
- P1 standalone plan fixes zone type MASTER (server_setup_infrastructure_dns.go:429), sync stores NATIVE (dns_cluster.go:491) → digest check (:744-746) fails with infrastructure_dns_changed; review loops.
- P2 join flow orders mail_profile before peer_join/dns (server_setup_peer_join.go:189-203); mail needs DNS identity (mail_dns_identity.go:84-114) → mail_profile_dns_identity_not_ready in (a), (b).
- P3 PowerDNS main offered at 4.9.18 on Debian 13; DNS step accepts only 4.9.17 and rolls back.
- P4 after the main changed engine: server_setup_peer_names_bound (server_setup_join.go:1108-1117); reopening re-binds at once; new code refused (join_bound): no recovery path.
- Minor: join code attempts_left 5 at creation vs 50 on re-read; after a native edit the engine card says degraded/unmanaged while the overview says "Nothing to do"; extending the first server re-adds a peer_join step.

## Not measured
Licence behaviour (acceptance build); mail, DKIM over the link, Move mail (P2); rest of (b) (P3/P4); (d) after infrastructure_dns (P1); second-server reboot during a real primary_dns wait in (a) (never waited); IPv6 and reverse DNS.

The secret scan of this evidence folder is clean. Written from the run's report (the run could not write this file itself).

## Archive

The raw evidence of this run (541 files: API exchanges, journals, versions, lab records) is kept as one archive next to this summary. Before it was committed, the archive's contents were scanned again for private keys, join codes, link credentials, enrollment codes, passwords and cookies, and for the secrets the lab still held (the fourth run's owner passwords, session cookies and join codes): clean.

- `cp-beta1-evidence.tar.gz` — SHA-256 `735c74edd62a493b6856c6d265166de8f9b48884f200a44cd0f131be71e1ac34`
