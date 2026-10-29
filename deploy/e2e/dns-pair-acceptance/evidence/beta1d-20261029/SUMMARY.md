# Beta-1 fourth VM re-run (2026-10-29)

Product commit: 68a9915a0e2387f85a900fb60dafe5a32fc94d81 (feat/setup-wizard-v2). Acceptance build (test licence seam): licence behaviour is **not** evidenced. Fresh installs, then the wizard, on disposable QEMU guests, with lab DNS root and parent, a lab resolver and lab ACME (pebble). The panels asked for celikpanel.net 12 times and got NXDOMAIN; nothing connected to it, and no CA name was asked. All 8 certificates came from pebble. Run time about 45 min (09:13Z–09:56Z).

## End to end
- (a) Debian main BIND, Ubuntu backup: both ready from a fresh install in one run. The main's card went pending → ready, and the link credential was emptied on both sides.
- (b) Option A: Ubuntu backup with mail, Debian main PowerDNS (DNS only). Both ready, and the link ended.

## Results
| Item | Outcome |
|---|---|
| Reboot during a real primary_dns wait (TCP 53 drop) | pass: `server_setup_host_starting`, then exactly one install |
| Reboot during the main's MariaDB step | pass, one operation per component; `host_starting` did not show (the panel answered after boot) |
| Second reboot later (access_dns) | pass: resumed, no count or audit change |
| Start during a Components install | pass: 409 `server_setup_host_busy`, no run created |
| Busy in the middle of a run | not measured (the parallel install never made the run wait) |
| Panel certificate, clock 200 s behind | pass: `server_setup_certificate_clock_skew` names 200 s and `timedatectl set-ntp true`; finished after the fix |
| Panel and mail certificates, 3–4 s behind | pass |
| Mail certificate, 200 s behind | not measured |
| Mail on both servers | refused at review with `server_setup_peer_mail_both` in (a) and (b) |
| Option A mail on the backup | pass: the main waited for the DKIM, it arrived over the link, served by both servers, Postfix answers; both overviews name the same receiver |
| Role texts | pass ("the other server", "Stop waiting for the other server") |
| Regressions: join code, registrar card, owner independence, (d) standalone, PowerDNS main after a reboot, Move mail | pass |

## Observation (lab-induced, not blocking)
Jumping the backup's clock back 200 s while its mail_dns step ran made that step wait `server_setup_host_busy` for about 3.5 min with nothing running, then fail with `server_setup_mail_dns_signing_failed`. Edit plan and start again passed. A backward jump of the clock during a step is not expected on a server with time sync. It is recorded as a known limit.

The secret scan of this evidence folder is clean (1349 files). Written from the run's report (the run could not write this file itself).

## Archive

The raw evidence of this run (1350 files: API exchanges, journals, versions, lab records) is kept as one archive next to this summary. Before it was committed, the archive's contents were scanned again for private keys, join codes, link credentials, enrollment codes, passwords and cookies, and for the secrets the lab still held (the fourth run's owner passwords, session cookies and join codes): clean.

- `cp-beta1d-evidence.tar.gz` — SHA-256 `e78ba7eba25b018f25a45cd909a67a7dc6eaa4d37925a8b1c62e23de04000f66`
