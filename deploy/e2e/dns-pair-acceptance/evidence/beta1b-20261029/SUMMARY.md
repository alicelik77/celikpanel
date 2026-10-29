# Beta-1 targeted VM re-run (2026-10-29)

Product commit: b86ab6fa468ada361da8f179fa9584b6a4bc5d51 (feat/setup-wizard-v2). Acceptance build (test licence seam): licence behaviour is **not** evidenced. Disposable QEMU guests on a lab bridge, with lab DNS root and parent, a lab resolver and lab ACME (pebble). No real servers, registrars or CAs were contacted; celikpanel.net got NXDOMAIN from the lab and nothing connected. Run time about 1 h 35 min (02:28Z–04:03Z).

## Results
| Item | Outcome |
|---|---|
| (d) standalone BIND | pass: infrastructure_dns succeeded, ready; registrar card not_yet → unknown (parent blocked) → ok |
| MariaDB point updates in the review | pass on Debian (a1, d) and Ubuntu (b1); a2 read "unknown" → P5 |
| Mail after DNS | pass on a1 (DKIM published); a2 not reached (P7); b not reached (P8/P9) |
| Move mail | a1 shows mx_move for the kept old MX |
| PowerDNS main 4.9.18-0+deb13u1 | pass: offered, installed, pair ready, ready after reboot, recovery check reports the job succeeded |
| Vendor PowerDNS build | refused, but with `server_setup_pdns_main_untested_version` instead of `not_debian_archive` |
| P4 engine change | recovered: peer_names_changed → new code → join_replaced; the first's execution continued |
| Owner change notice | pass: the same notice on the engine card and the overview; record kept |
| Join code tries | pass: 50 at creation and on re-read, 49 after one wrong try |
| Extension | pass: refused when shared answers change; the unrelated change reached ready, binding kept |
| Reboot in a real primary_dns wait | not reached (the step never waited); reboot at access_dns resumed, no duplicate |

## New defects (hypotheses)
- **P5**: an unread review (`package_updates_state: unknown`) still lets setup start. The step then waits on `server_setup_distribution_updates_changed` and says "open the review", but revise refuses the phase "service" (`setup_conflict`). Only the old Components detour unblocks it. See server_setup_operations.go:845-846, 891, 897-899, server_setup_revise.go:49 and web ServerSetup.tsx:679.
- **P6**: a DNS step that failed on a package stays paused at `server_setup_dns_agent_failed` until a panel restart. The recovery check only reports what it observes (server_setup_dns_outcome.go).
- **P7**: the backup's panel certificate was issued, yet the step failed with `panel_certificate_issue_failed`. The likely cause is a lab negative cache 17 s after delegation, but this is unproven. The main had already finished and revoked the link, so the backup can never finish: the retry got `peer_link_revoked`, and a new code got `server_setup_join_finished`.
- **P8/P9**: after the P4 rejoin, the backup's dns_publisher failed with the generic `server_setup_step_failed` (journal: "server setup changed; reload the current plan", probably server_setup_dns_publisher_commit.go:17-25). Re-review is then blocked by `server_setup_existing_dns_requires_migration`, caused by the setup's own PowerDNS.
- Minor: the first server's run showed primary_dns rather than `peer_join_changed`; only its peer card showed the change. The P4 detour needed a panel restart (P6).

## Not measured
- Licence behaviour.
- DKIM over the link and mail in (b).
- Mail on a2.
- A reboot during a real primary_dns wait.
- Backports (covered by component tests only).
- IPv6 and reverse DNS.

The secret scan of this evidence folder is clean. Written from the run's report (the run could not write this file itself).

## Archive

The raw evidence of this run (376 files: API exchanges, journals, versions, lab records) is kept as one archive next to this summary. Before it was committed, the archive's contents were scanned again for private keys, join codes, link credentials, enrollment codes, passwords and cookies, and for the secrets the lab still held (the fourth run's owner passwords, session cookies and join codes): clean.

- `cp-beta1b-evidence.tar.gz` — SHA-256 `3c15c78260fa80b3b90a67877f4035ab90ad50f5dd1690f29a2eda545416d5b4`
