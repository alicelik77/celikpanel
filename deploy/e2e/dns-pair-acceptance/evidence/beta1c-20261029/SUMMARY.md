# Beta-1 third targeted VM re-run (2026-10-29)

Product commit: cfcadd3f1823e433c734956f9855cc530d421b2c (feat/setup-wizard-v2). Acceptance build (test licence seam): licence behaviour is **not** evidenced. Fresh installs, then the wizard, on disposable QEMU guests with lab DNS root and parent, a lab resolver and lab ACME (pebble). The panels asked for celikpanel.net 13 times and got NXDOMAIN; nothing connected to it or to a real CA. 13 certificates, all from pebble. Run time about 1 h 35 min (06:07Z–07:41Z).

## Results
| Item | Outcome |
|---|---|
| P5 busy review (`_unread`, refresh, `_changed_before_start`) | not reproduced: held dpkg locks and a Components install still gave a known list |
| P5 update appearing mid-run, confirmed from the wait | pass: the same run continued |
| P6 BIND uninstallable → rolled back → PowerDNS | pass, no panel restart |
| P7 backup fails after main completes, retry | pass: pending → ready, link credential emptied on both sides |
| P7 backup cannot tell the main it is ready | pass: `reporting` / `peer_ready_unreachable`, then it reported by itself |
| P7 Stop waiting for the second server | pass; the backup's `peer_link_revoked` text names the wrong server in Option A (N6) |
| Panel certificate, clock 2–4 s behind | pass |
| Panel certificate, clock 200 s behind | fail: generic `panel_certificate_issue_failed`, clock reason lost (N2) |
| P8/P9 rejoin through dns_publisher, re-review | pass |
| Mail a2 | pass after its clock was corrected (N4) |
| Mail (b) Option A | fail: the backup's DKIM never crosses the link (N3) |
| Vendor PowerDNS / `peer_join_changed` at the top | pass |
| Real primary_dns wait (TCP 53 drop) + reboot | the wait happened; after the reboot the DNS step failed (N1) |
| Lost ready acknowledgement | pass: one `peer_ready`, the backup finished |
| Bare 401 → `peer_ready_refused` | not measured (needs a database edit) |

## New defects (hypotheses)
- **N1**: a DNS step resumed during boot fails instead of waiting. The Agent reports "systemd is not ready: state starting", which maps to `server_setup_dns_failed`.
- **N2**: the clock-skew reason from cmd/agent/panel_cert_source_wait.go:36 never reaches the step.
- **N3**: in Option A with mail on both servers, the backup's DKIM over the link is refused with `server_setup_peer_dkim_not_running` once the main has finished. Both servers claim the main domain's mail.
- **N4**: the mail certificate has no not-yet-valid wait; a few seconds of skew gives the generic `server_setup_step_failed`.
- **N5** (minor): the backup's DNS overview shows itself as the main domain's mail receiver.
- **N6** (minor): `peer_link_revoked` says to make a new code "on the first server", but in Option A the backup is the first server.
- Side effect: a start while another install runs failed with `HOST_MUTATION_BUSY` and needed Edit plan.

The secret scan of this evidence folder is clean. Written from the run's report (the run could not write this file itself).

## Archive

The raw evidence of this run (489 files: API exchanges, journals, versions, lab records) is kept as one archive next to this summary. Before it was committed, the archive's contents were scanned again for private keys, join codes, link credentials, enrollment codes, passwords and cookies, and for the secrets the lab still held (the fourth run's owner passwords, session cookies and join codes): clean.

- `cp-beta1c-evidence.tar.gz` — SHA-256 `53039d80d1674630a64c2f61280aa799c8ee3a6595c525c3fc6d3c01d2e0111b`
