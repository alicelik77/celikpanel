# DKIM deletion race, waiting deletion text, PHP mail() scanned — native check (2026-10-31)

**Result:** pass for what this branch fixes.

Branch `fix/dkim-deletion-race` (829b51ee5, 57379a058, 694ce3cfa, ce5f049a7,
ceb29d31f, b3b558bfe), from `release/alpha113` 3494254f4, merged into
`release/alpha113`.

- **Build:** dist of `ce5f049a7`, labelled `v0.1.0-alpha.113`.
- **Install:** through the owner's `get.sh` against a lab portal; only the pinned key digest differs (`get.sh.diff`).
- **Lab:** Debian 13 main + Ubuntu 24.04 backup joined by code (PowerDNS), and Arch standalone. All setups succeeded; the deletion check was ready on both pair servers.
- **Secret scan:** clean (430 files, 26 secrets, plus the lab signing key and the mailbox passwords; `secret-scan*.json`).
- **Logs:** `dkim-deletion-race-evidence.tar.gz`, SHA-256 `9b501bc34e4bfc8eb0067719a0eced03bc0f166731d50b979c9cf16166575e04`.

## What changed
- **Automatic DKIM no longer writes for a domain being deleted.**
  - Its record and TXT writes check, in the same SQL statement, that the domain exists and its deletion has not started.
  - The zone is published under the deletion's own locks, after one more check.
  - A deleted domain stops the work quietly, with one "stopped" line.
  - Records of deleted domains are removed before every pass, and at Panel start.
- **Waiting text instead of an early "act now".**
  - The Agent's pending line for a pair deletion says it is waiting for the backup and re-checking.
  - While a DELETE runs, deletion-status answers `running` / `deletion_running`.
- **PHP `mail()` is spam-checked.**
  - With Rspamd installed, `non_smtpd_milters` is OpenDKIM and then Rspamd. The Panel-start repair migrates existing servers; an owner-changed list is kept and named.
  - `milter_default_action=accept`.
  - `internal_mail_filter_classes` stays empty, so bounces skip the milters.

## Native results
**D6: zones created and deleted at once (d6gone1/2/3.test)**
- 200 "deleted" in 56, 38 and 62 s; both DNS servers answer REFUSED.
- During each DELETE, status was read 42, 30 and 45 times; every read said running.
- One `[dkim-auto] … stopped` line each; 0 FOREIGN KEY lines; no record, key or table line left (`d6opsA.out`, `delpoll-d6-a-*.txt`).
- gone1's first proof round timed out under the load of a matrix run. It logged the waiting text and verified 29 s later.

**Site deletion (d6shop113.test)**
- "deleted" in 62 s; status read "running" 49 times, never the escalation text.
- The first backup check answered `peer_inspection_unavailable`, was logged with the waiting text, and verified 29 s later.
- TXT, zone, key and table lines are gone on both servers.

**PHP `mail()` and bounces, pair (d6mail113.test)**
- The 587, 465 and PHP `mail()` messages carry `d=d6mail113.test s=celik` and verify against both DNS servers.
- Rspamd logged the PHP message (no action, 0.19).
- `non_smtpd_milters = inet:127.0.0.1:8891, inet:127.0.0.1:11332`.
- The mail to `nosuch-d6@` bounced (unknown user). The bounce notice reached the sender's INBOX and has no Rspamd line (`d6opsA-mail.out`).

**PHP `mail()` and bounces, Arch** (`archB.out`)
- The same results; Rspamd scanned the PHP message (no action, 0.70).
- Submission chroot `n`; 0 milter warnings over 3 boots.

**Distro config matrix**
- 0 failures on five releases.
- On each, non-SMTP mail reached the milters from cleanup.

## Open
- The first `pdns-peer-inspect` exchange of a pair deletion can still fail once, with no DKIM activity in its window. It now reads as waiting and verifies on the automatic re-check. The backup-side cause has not been captured yet.
- In one case a queued DKIM record is not restored:
  - a background pass has already removed the domain's queued DKIM record;
  - the deletion is then rolled back, because mail-TLS preparation failed on its first attempt.

  The card then offers the manual buttons.
- DNS-only zones still get a DKIM key while mail is installed on the server (product rule `domainHasProductMailRuntime`, kept). The branch author suggests limiting this to the first-mailbox trigger.
