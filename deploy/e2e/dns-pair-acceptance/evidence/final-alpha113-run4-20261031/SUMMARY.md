# Final acceptance run 4 (alpha.113 candidate 480848132) — evidence (2026-10-31)

**Result: not clean.** Arch standalone passed every step. The pair failed one step: a site deleted after the reboots stays pending (D7). D5, D6 and the PHP `mail()` Rspamd scan are fixed and passed.

**Method:** the same as run 3.
- **Install:** the owner's `get.sh` against the lab portal; only the pinned key digest differs (`get.sh.diff`).
- **Build:** acceptance-licence dist labelled `v0.1.0-alpha.113` (`dist.json`).
- **DKIM card:** read only, never pressed.
- **Secret scan:** clean (352 files, 26 secrets).
- **Logs:** `final-alpha113-run4-evidence.tar.gz`, SHA-256 `95de330890b2f06c368de225f360269a31ffdee871c2538d74d3c0e729f776a1`.

## Defects
**D7: pair, site deleted after the main rebooted.**

Sequence: both servers rebooted, then panel-off with the main rebooted, then management back on, then `DELETE /domains/1` (`r4shop113.test`).
- **Panel answer:** `deletion-status` read `running` for about 2 s. DELETE then answered 202 `deletion_pending`, reason `dns_peer_journal_unknown`, with an administrator escalation text.
- **State:**
  - The zone and the TXT are gone on both servers (REFUSED).
  - The domain stays listed.
- **Agent log, in order:**
  1. `DNS peer proof admitted the PowerDNS daemon's re-stamp of catalog … serial 1793402914 -> 1793403447`
  2. `stopped at step exchange as dns_peer_inspection_unknown: peer_inspection_unavailable`
  3. At every re-check (23:37:34 to 23:41:19, with back-off) and after "Retry this deletion" at 23:44:30: `stopped at step challenge_write as dns_peer_journal_unknown: challenge publish: pdns_peer_challenge_context_mismatch`
- **Suspected cause:** `internal/pdnspeerjournal/store_linux.go` publish `sameContext`. The first attempt's outstanding challenge keeps the catalog context from before the re-stamp, so every retry is refused.
- **Comparison:** a DNS-only zone deleted before the reboots completed in 34 s.

**D8 (minor): after every reboot, the Panel's startup certificate reconcile runs while systemd is still "starting", and is not retried.**
- Logged line: `certificate startup reconcile: staged lineages: … systemd is not ready: systemd state "starting"`.
- On Frankfurt and Arch, also: `restore hosted vhost batch`.
- Sites served normally after every reboot.

**D9: the cause of `peer_inspection_unavailable` cannot be read on the backup.**
- The inspector writes one generic stderr line and logs nothing on the backup.
- In this run, the failing inspections ran 8 s after the backup committed the catalog deletion.

## Observations
- **DKIM card:** shows `dkim.status: "pending"` while `dkim_signed: true` and the TXT is served by both servers.
- **Arch standalone:** the `running` deletion text mentions a paired backup DNS server, although Arch has none.
- **PHP `mail()`:** Rspamd scores `FORGED_SENDER` (0.30), still "no action". The envelope sender is the site user.
- **Expected in the lab:**
  - a MariaDB postinst warning on Ubuntu;
  - `release_unverified` update-check lines;
  - `owner_configured` renewal state.

## Pair (Debian 13 main + Ubuntu 24.04 joined by code)
- **Setup:** pass. Install 59 s / 56 s; both setups succeeded; deletion check ready; every wait carried a reason.
- **Site:** pass.
  - Certificate in 3 s.
  - 301 to https.
  - PHP page with no `X-Powered-By`.
- **Mail:** pass.
  - 587 and 465 send; IMAPS read; Roundcube 3/3.
  - DKIM by itself in 3 s: 587, 465 and PHP `mail()` verify against both DNS servers.
  - Rspamd scanned all three.
  - Bounce delivered.
- **DNS-only zone created and deleted at once:** pass. `running`, then "deleted" in 34 s; `[dkim-auto] … stopped`; 0 `dkim_automatic` rows; 0 FOREIGN KEY lines.
- **Reboot and panel-off:** pass.
- **Journals:** pass. 0 milter, FQDN, panel 500, fail-closed, ambiguous, FOREIGN KEY or lmdb lines; no failed units.
- **Site delete:** fails (D7).

## Arch standalone (PowerDNS)
- **Setup and site:** pass. Setup; certificate in 3 s; 301; PHP 8.5.
- **Mail:** pass.
  - 587, 465, IMAPS and Roundcube.
  - DKIM for 587, 465 and PHP `mail()` verifies against Arch's own PowerDNS.
  - Rspamd scanned all three.
  - Bounce delivered.
- **Reboot:** pass.
- **Site delete:** pass. `running`, then "deleted" in 11 s, with no Agent hold.
- **New site afterwards:** pass. Created, certificate issued, https served.
- **Journals:** clean over 4 boots.
