# Final acceptance run 3 (alpha.113 candidate dc0946c14) — evidence (2026-10-30)

**Result: not clean.** Run 3 found one serious defect (D5) and one smaller one (D6).

## How it ran
- **Install:** the owner's `get.sh` command against the lab portal; only the pinned key digest differs (`get.sh.diff`).
- **Build:** acceptance-licence dist labelled `v0.1.0-alpha.113` (`dist.json`).
- **Clocks:** the guest clocks were left as the lab gives them, about 1 s behind the lab CA.
- **DKIM card:** read only, never pressed.
- **Install times:** Debian 64 s, Ubuntu 66 s, Arch 176 s (Arch asked for a restart and was restarted).
- **Secret scan:** clean (447 files, 26 secrets; lab signing key and mailbox passwords also checked; the Roundcube request token is redacted). `secret-scan.json`.
- **Full logs:** `final-alpha113-run3-evidence.tar.gz`, SHA-256 `0139fbfdb5ce564456fb1c3b9bfac06a7b030a5b0cfb34db8d243adbc7abadbc`.

## Defects
**D5 (serious): Arch standalone (PowerDNS).** Deleting a site leaves the Agent refusing every change.
- **What happened:** `DELETE /domains/1` (`archr3shop113.test`) answered 202 `deletion_pending`, stage `dns_cleanup`. The zone, the DKIM key and the TXT were gone; the server answers REFUSED. The Agent logged:
  - `deleted zone archr3shop113.test absence could not be verified over udp`;
  - then the service mutation manager went fail-closed (every change held);
  - the Panel logged `ledger_ambiguous`.
- **Recovery did not work:** restarting the Agent repeated the same proof failure; six automatic re-checks failed; "Retry this deletion" answered `DNS V3 publication is already in progress`.
- **Suspected cause:** `validDeletedDNSZoneProof` accepts only an authoritative answer carrying a parent zone's SOA. A server that does not serve the parent zone answers REFUSED, so the proof can never pass on a standalone server. A pair deletion goes through the peer proof and passed.
- **Consequence:** the Arch PHP `mail()` DKIM check was not done.

**D6: pair main (Debian 13).** Automatic DKIM raced the deletion of a DNS-only zone created and deleted at once.
- `[dkim-auto] r3gone113.test: failed (dns_publish_failed) constraint failed: FOREIGN KEY constraint failed (787)`.
- A failed `dkim_automatic_v1:2` record was left for the deleted domain; the key and the table lines were removed.
- In the same window, one deletion proof first showed `peer_inspection_unavailable` and verified by itself 19 s later.

## Observations
- **Pair deletions:** they logged "the peer administrator must check native zone state" before verifying by themselves 18–19 s later.
- **PHP `mail()`:** passes through OpenDKIM only (`non_smtpd_milters`), not Rspamd.
- **Rspamd:** adds no headers; its verdict is only in its log.
- **Expected in the lab:**
  - renewal state `owner_configured` (the lab `cli.ini` points at Pebble);
  - `release_unverified` update-check lines;
  - a MariaDB postinst warning on Ubuntu.

## A. Debian 13 main + Ubuntu 24.04 backup (joined by code)
1. **Setup:** pass. Licence active; both setups succeeded; deletion check ready on both servers.
2. **Site:** pass.
   - Certificate in 3 s.
   - http answers 301 to https (site, www, PHP page).
   - PHP 8.4 page: `Server: nginx`, no `X-Powered-By`.
   - A missing-file download answers 404 `FILE_NOT_FOUND` with guidance.
3. **Mail:** pass.
   - 587 and 465 send; IMAPS read; Roundcube shows 2/2; webmail http answers 301.
   - DKIM by itself: the 587, 465 and PHP `mail()` messages carry `d=r3shop113.test s=celik`. dkimpy verifies each against the TXT from both DNS servers.
   - Rspamd scanned the 587 and 465 mail.
4. **DNS:** pass. Record add, edit and delete reached the backup in 0–1 s; zone delete was proven.
5. **Reboot of both servers:** pass.
6. **Panel and Agent off, main rebooted:** pass. Nothing listened on 2083; https, PHP, mail, DNS and DKIM signing kept working; `certbot.timer` was present. With management back on: `pair_ready`, 0 pending.
7. **Waits:** pass. Every wait carried a reason.
8. **Journals:** pass. On every boot of both servers: 0 milter warnings, 0 lmdb lines, 0 FQDN lines, 0 panel 500 lines, no "can't lock" lines, no failed units.

**Site delete:** pass. "deleted" in 35 s; TXT and zone gone on both servers; no key-table or signing-table lines left.

## B. Arch standalone (PowerDNS)
- **Setup and site:** pass. Setup, certificate in 3 s, 301, PHP 8.5 page without `X-Powered-By`, 404 guidance.
- **Mail:**
  - 587 and 465 send, IMAPS read and Roundcube work.
  - DKIM is signed by itself and verifies against Arch's own PowerDNS.
  - Rspamd scanned the mail.
  - 0 milter warnings over 4 boots.
- **Site delete:** fails (D5).
