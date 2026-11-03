# alpha.118 lab acceptance, run 1 (2026-11-03)

Candidate `release/alpha118` at `02542ef24`; previous release `v0.1.0-alpha.117` (`b22348ff8`).
QEMU/KVM lab only (archlinux WSL host; labnet router, lab resolver, pebble ACME). No real
servers, no celikpanel.net, no licence service, no real CA. Acceptance-licence builds.
Lab time 13:45-15:20 UTC. Everything went through the panel API; SSH was used to observe,
except the two lab actions listed below.

## Lab differences

- **Unbumped release identity.** The candidate still carries the alpha.117 identity in
  `deploy/release-sequence-policy` and `download-portal/get.sh`. The first fresh install
  stopped in install.sh with `fresh recovery foundation differs from authenticated release
  identity` (`raw/run-singleA.txt`). The lab then built from a lab-only commit `dbd4dfe68`
  on top of `02542ef24`. It changes only those two files, to 118 (previous 117/`b22348ff8`);
  see `raw/lab-identity-bump.diff`. The release commit must make the same change.
- Lab portal signed with a lab key; `get.sh` differs only in the key digest (`raw/get.sh.diff`).
- B ran on Ubuntu 24.04 with BIND, because PowerDNS main is refused there by design.
- The SFTP backup target was a key-only user on labnet (192.0.3.1), not inside the guest.
- **Owner action on A, as the product's guidance asks.** After `APP_EXTENSIONS_MISSING` (see
  D3), the owner ran `apt-get install php8.4-curl php8.4-gd php8.4-mysql php8.4-bcmath`
  (`raw/php-a118.log`).

## Topologies

- **A: Debian 13** (Dovecot 2.4.1), get.sh install and one-server wizard (PowerDNS, web+mail).
  The second fresh guest succeeded. The first one stuck at `01-dns` (D1).
- **B: Ubuntu 24.04** (Dovecot 2.3.21). alpha.117 install and wizard, then a panel update to
  the candidate. The update succeeded in 82 s. Migrations 057-061 are present, services are
  active, `nginx -t` passes, no panics. No rollback was needed or tested.
- **C: Arch** (Dovecot 2.4.5). Install and wizard succeeded. `nginx -t` passes with the
  traffic include. `doveconf` is OK. The quota file is absent, as in D2.

## Passed

1. Traffic:
   - The log grows: 7.5 MB counted after the 5-minute collection.
   - A 1 MB limit gave only the site the 503 "Traffic limit reached" page. IMAP login still
     worked. Raising the limit reopened the site in one round.
   - On B the traffic was measured (13 KB).
2. Disk:
   - The web, mail and DB measurement appears.
   - With a 1 MB limit, a new domain and a file-manager upload are both refused with
     `QUOTA_DISK_EXCEEDED`.
   - SFTP gets `ForceCommand internal-sftp -R`, and the PHP pool gets `file_uploads = off`
     (PHP upload refused).
   - After lifting the limit, the domain, the upload and SFTP read-write all reopened. PHP did
     not reopen (D4).
3. Mail quota:
   - B (2.3): a 950 KB message is stored; the next one gets `552 5.2.2 Mailbox is full`. The
     90% warning (EN+TR) was delivered once by the 15:02 timer run, with a marker file written.
   - A (2.4): the 2.4 dialect was written at the hourly reconcile, then `552 5.2.2` was
     returned. This was late (D2).
4. Apps:
   - Drupal 11.4.11 installed in 47 s and the site answers.
   - Missing-extension refusals are clear.
5. WordPress:
   - Install and setup, status (core 7.1.4, plugins).
   - Clone to `stage2.wp118.test` was applied ("9 values") and answers.
   - Plugin update not exercised: the bundled plugins had no update.
6. ClamAV:
   - EICAR found (`Eicar-Test-Signature`).
   - Quarantine moved the file and restore put it back.
7. Remote backup:
   - Host-key confirm step, then the key was pinned.
   - Send verified size+sha256 and restore verified sha256+mdc.
   - Manual `gpg --decrypt` matched the sidecar's `plain_sha256`.
8. n8n offer and Open WebUI "os_unsupported (Python 3.13)" guidance on Debian 13 are correct.

## Defects

- **D1 (intermittent) A, 1 of 2 fresh Debian 13 installs.**
  - What happened: setup `01-dns` stayed `server_setup_dns_result_unknown` with no progress.
  - What the evidence shows:
    - The audit shows `setup.accepted`, then `setup.reconcile_required` in the same second.
    - `recovery dns-switch-status` says "exact DNS switch job is absent from the mutation
      ledger".
    - The switch phase stayed `activating`, pdns was never installed, and nothing was logged.
  - Evidence: `raw/a118-setup-operation-stuck-dns*.json`, `raw/a118-guest-stuck-dns.txt`,
    `raw/a118-guest-audit-dns.txt`.
- **D2 Mailbox quota not enforced for up to one hour after a fresh install** (A Debian 13,
  C Arch).
  - What happened: the first usage round ran before mail existed (`state no_mail`), and
    `ReconcileMailQuotaV1` runs hourly. So there was no `90-celikpanel-quota.conf`, no policy
    service and no warning timer.
  - Result: a 1 MB mailbox accepted 1.25 MB.
  - Evidence: `raw/run-A0.txt`, `raw/L1-a118.log`, `raw/warnA-a118.log`, `raw/C1-a118c.log`.
- **D3 App catalog recipes.** Evidence: `raw/run-L2.txt`, `raw/run-L2b.txt`,
  `raw/n8nfail-a118.log`.
  - Nextcloud: `APP_VERIFICATION_FAILED` "archive link ...NcRelatedResourcesPanel...license
    points at another link".
  - Joomla: `APP_SETUP_FAILED`, "Joomla CLI installer: exit code 255". A retry failed the same
    way; the installer's own message is not shown.
  - PrestaShop: `APP_VERIFICATION_FAILED` "write .../En_243.php: bad file descriptor".
  - MediaWiki and phpBB: `APP_DOWNLOAD_FAILED` HTTP 403. Upstream returns 403 to the Go
    default and empty User-Agent and 206 to curl's, from the same NAT.
  - Drupal removal: "some of Drupal's files could not be removed". `sites/default` is
    `dr-xr-x---`, the app status becomes `needs_review` and the site returns 403
    (`raw/a118-guest-drupal-remove.txt`).
  - On a distribution-only server, Components cannot add `php8.4-curl/gd/mysql`. Picking
    `php8.4-fpm` demands Sury.
  - WordPress (legacy path) installs without `mysqli` and ends at 500 "Requirements Not Met".
- **D4 PHP uploads stay off after the disk limit is lifted.**
  - The DB shows `upload_blocked=0` and applied, and SFTP reopened.
  - But `site2.conf` (14:27:36) keeps `php_admin_flag[file_uploads] = off` two rounds later.
  - Evidence: `raw/L1b-a118.log`.
- **D5 n8n can never install.**
  - The install unit prints `Referenced but unset environment variable ... algo` and
    `sum: invalid option -- 'c'`.
  - Cause: systemd expands `"${algo}sum"` in `internal/serviceapps/render.go:488`; it needs
    `$${algo}`.
  - Evidence: `raw/n8nfail-a118.log`, `raw/run-L3c.txt`.
- **D6 ClamAV install from Components reports failure.**
  - The detail is "clamav-daemon did not become active".
  - Verification ran at 14:24:24, before freshclam finished at 14:24:35, so the
    `ConditionPathExistsGlob` was unmet.
  - Scanning still works with `clamscan`, and the screen keeps the failure.
  - Evidence: `raw/a118-guest-joomla-clamav.txt`.

## Not run

- Item 9 (suspension stops and restarts n8n): blocked by D5.
- Open WebUI: Debian 13 is unsupported, and it is several GB on Ubuntu.
- WordPress plugin update: nothing to update.
- 90% warning on Dovecot 2.4: next timer run at 16:04.
- SFTP write test: the guests' sshd is key-only. Read-only was shown by the sshd config.
- Rollback.

Other observations:
- `logrotate` is missing on the Debian 13 and Arch images; usage status shows `missing_tool`.
- A transient update `409 PANEL_UPDATE_BUSY` happened right after the B setup, and the retry
  passed.
