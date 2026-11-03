# alpha.118 lab acceptance, run 2 (2026-11-03, 16:30-16:50 UTC)

Candidate `release/alpha118` at `f19ba7be4`. The release identity is already 118, so there is
no lab commit. Lab only; panel API like a user; SSH read-only except the lab's own dpkg-lock load.
The run stopped at the first product defect (D6, 16:46). Lines still running then were stopped;
two read-only observations followed (`raw/r2-observe.log`).

| Item | Result | Evidence |
|---|---|---|
| 1 Fresh Debian 13 + setup | PASS | `raw/run-singleA.txt` |
| 1 D1 recheck | PASS on 2nd guest a118d (see note) | `raw/d1watch-a118d.log`, `raw/r2-observe.log` |
| 2 logrotate | PASS | `raw/R2a-a118.log` |
| 3 D2 mailbox quota | PASS | `raw/R2a-a118.log` |
| 4 D3 WordPress refusal | NOT SHOWN | `raw/R2apps-a118.log`, `raw/a118-guest-wp-prereq-unknown.txt` |
| 4 D3 Components PHP 8.4 | PASS | `raw/R2apps-a118.log` |
| 4 D3 apps | partial; stopped at D6 | `raw/R2apps-a118.log` |
| 5 D4 | not finished (stopped at D6) | `raw/R2lim-a118.log` |
| 6 D5 n8n | FAIL (new cause) | `raw/r2-n8nfail.log` |
| 7 D6 ClamAV | **FAIL** | `raw/R2av-a118.log`, `raw/r2-a118-guest-clamav.txt` |

## Item notes

- **D1 recheck.** Guest a118 used `flock`. That does not block apt, which uses `fcntl` locks,
  so a118 only shows that setup completed. On a118d the lab held the `fcntl` locks of dpkg
  `lock-frontend` and `lock`.
  - The step showed `waiting dns server_setup_host_busy` ("Nobody needs to act…").
  - The ledger stayed empty while the lock was held.
  - After the lock was released, setup continued by itself with exactly one job:
    `dns_engine_switch pdns succeeded`.
- **logrotate.** `/usr/sbin/logrotate` is installed (3.22.0-1) and traffic status shows
  `logrotate: written`.
- **D2.** Mail quota was `enforced` (2.4) at 16:39:19, right after mail was installed. The
  1 MB mailbox was created at 16:40:32 and `552 5.2.2` came at 16:41:48.
  - One 300 KB message 13 s after the 950 KB one was still stored, because the count index
    lags. The overshoot was 1.22 MB.
- **D3 WordPress refusal.** The check gave `503 APP_PREREQUISITES_UNKNOWN` ("the server could
  not be asked…"), not the missing-extension guidance. Agent work was concurrent at the time.
  It was not retried before the extensions were installed, and the a118d retry was stopped.
- **D3 Components PHP 8.4.** `php-fpm`/`php8.4-fpm` succeeded from Debian sources only, with no
  Sury repository. curl, gd, mysql, bcmath, intl and zip came with it.
- **D3 apps.** Nextcloud 35.0.2 was set up in 120 s and answers 302 to `/index.php/login`.
  Joomla 5.4.10 was set up in 22 s and answers 200 ("Home"). PrestaShop was installing when
  the run stopped. MediaWiki, phpBB and the Drupal install/remove were not run.
- **D4.** The disk limit of 1 MB was set at 16:45:27. The run stopped before the check round,
  so this item has no result.
- **D5 n8n.** The `${algo}` fix works. Node.js 24 was installed, n8n was offered, and the
  certificate was issued. The install now fails in npm:
  `npm error code EBADPLATFORM … Unsupported platform for @napi-rs/canvas-linux-arm64-musl@1.0.11:
  wanted {"os":"linux","cpu":"arm64","libc":"musl"} (current: x64 glibc)`.
  The state is `install=failed`, `verified_failure`. Because of this, suspend/resume and the
  reseller refusal were not run.
- **D6 ClamAV.** Install from Components ends **failed**: `service_install_failed`, "clamav-daemon
  did not become active (Result=success)" (16:44:19).
  - Timeline: freshclam started at 16:44:12. `daily.cvd` landed at 16:44:13. The daemon and
    socket were skipped at 16:44:14 and 16:44:19 on `ConditionPathExistsGlob=…/main.{c[vl]d,inc}`.
    `main.cvd` landed at 16:44:20.
  - Afterwards `clamav-daemon` and `clamav-daemon.socket` both stay **inactive**. freshclam
    logs "Clamd was NOT notified".
  - Cause: `clamAVSignaturesReady` (`cmd/agent/service_prepare.go:200`) accepts `daily.*` alone.
    Debian's units also need `main.*`, and the skipped socket never re-arms.
  - Scanning was not reached.

Cleanup: guests a118 and a118d were stopped and removed, as were the lab portal, the dist and
`/var/tmp/a118r1`.
