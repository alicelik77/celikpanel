# alpha.119 lab acceptance, run 5: update straight from 117 to 119 (2026-11-04, 03:56-04:13 UTC)

**Candidate.** `release/alpha119` at `1b4c32ae2` (identity 119). The guests were installed with
`v0.1.0-alpha.117` (`b22348ff8`).

**Lab portal.** It offered only 119 (`latest.txt`), so no guest stopped at 118. Lab only; panel API
like a user; SSH read-only.

**Guests.**
- D: Debian 13. PowerDNS primary and mail (one-server wizard), like Frankfurt.
- U: Ubuntu 24.04. BIND and mail, because PowerDNS as primary is refused on Ubuntu by design.
  Boston is a PowerDNS *secondary*, which this single-guest round does not reproduce.

**Data on 117 (both guests).** A customer with a plan, a reseller with a reseller plan, one PHP
site with a file, a mailbox, a MariaDB database and a full site backup.

**Result: no product defect.**

| Check | D Debian 13 | U Ubuntu 24.04 |
|---|---|---|
| Update 117 → 119 through the panel | PASS (102 s) | PASS (71 s) |
| Migrations 057-063 | PASS | PASS |
| Services active, `nginx -t`, no panics | PASS | PASS |
| 118 env migration is a no-op | PASS | PASS |
| Site answers 200 with the 117 file | PASS | PASS |
| Mailbox still receives mail | PASS | PASS |
| Database reachable | PASS | PASS |
| Backup still restores | PASS | PASS |
| Plans, reseller plan, users intact | PASS | PASS |
| Traffic limits unlimited + one admin notice (057) | PASS | PASS |
| Guide `/help` | PASS | PASS |
| Access tab SSH tools | — | PASS |

## Notes

**Update.** The first update start got the familiar short `409 PANEL_UPDATE_BUSY` from the 117
panel, right after the lab created data. The scripted retry was accepted.

**Services.** On D, `named` is inactive (it uses PowerDNS); on U, `pdns` is inactive (it uses BIND).

**118 env migration.** There is no `/etc/celikpanel-svcapps`, no related agent log line, and the
error log is empty.

**Mailbox.** A 2 KB message was stored after the update.

**Database.** The site's database user still connects.

**Backup.** The restore returned 200 with a safety backup, and the file still answers.

**Plans and users.** Plan117 still has a 2000 MB disk limit and the customer's domain limit is still
5. The customer is on Plan117, the reseller is on RPlan117, and the reseller plan's limits are
unchanged.

**Traffic limits (057).** `bandwidth_quota_mb` is 0 everywhere and `traffic_limit_bytes` is 0. There
is one rights notice, `traffic_measured`, for the owner.

**Guide.** `/help` returns 200; the tour state is pending.

**Access tab on U.**
- 117 left `zip` and `unzip` missing, and the Access tab lists them.
- Turning SSH on is refused with `409 FILE_ACCESS_SSH_TOOLS_MISSING` (unzip, zip).
- The first one-click install, at 04:11:08, returned `409 service_operation_busy` ("another package
  operation is already in progress"). At that moment no service operation was active; the agent was
  finishing its post-update startup jobs (`mail_tls_sync` and `mail_filter_wire`, until 04:11:32;
  `raw/u117-tools-busy.txt`).
- At 04:11:47 the install returned 200 with `installed [unzip, zip]`.
- After that, SSH-on returned 200, and the jail holds zip, unzip, nano, vi, rsync, less and bash.
- `sshd -t` is OK.
- Observation: the words "package operation" do not match the post-update startup work that made
  the server busy.

**Cleanup.** The guests, the lab portal, the dists and the work directory were removed.
