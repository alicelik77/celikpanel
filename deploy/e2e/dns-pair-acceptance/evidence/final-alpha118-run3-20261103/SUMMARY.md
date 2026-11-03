# alpha.118 lab acceptance, run 3 (2026-11-03, 17:35-18:02 UTC)

Candidate `release/alpha118` at `5b872dfe0`, with release identity 118. Lab only, through the
panel API; SSH was read-only. One fresh Debian 13 guest, installed through get.sh, finished
setup (`raw/run-singleA.txt`). The run stopped at the first product defect: PrestaShop in item 3.

| Item | Result | Evidence |
|---|---|---|
| 1 ClamAV from Components | PASS | `raw/R3av-a118.log` |
| 2 WordPress refusal, then Components php8.4-fpm | PASS | `raw/R3wp-a118.log` |
| 3 PrestaShop | **FAIL (stop)** | `raw/R3apps-a118.log`, `raw/r3-defect-evidence.txt` |
| 3 MediaWiki | PASS | `raw/R3apps-a118.log`, `raw/r3-defect-evidence.txt` |
| 3 phpBB | **FAIL** | `raw/R3apps-a118.log`, `raw/r3-defect-evidence.txt` |
| 3 Drupal install/remove | not finished | stopped |
| 4 D4 PHP uploads | not finished | `raw/R2lim-a118.log` |
| 5 n8n, suspension, reseller | not run | stopped at the first defect |
| 6 file access warnings | not run | optional |

## Item 1: ClamAV

- The install succeeded: it waited for the signatures and finished at 17:50:09.
- `main.cvd` was in place at 17:49:45. The socket was listening at 17:49:57 and the daemon
  started at 17:49:59.
- The install never showed a failure, and no first-start path unit was needed (none present).
- The EICAR test file was found (`Eicar-Test-Signature`). After quarantine it left the document
  root and its state reads `quarantined`.

## Item 2: WordPress without the extensions

- The requirement check answered `unmet` and named `mysqli: php8.4-mysql`, with the hint
  `install_package php8.4-fpm`.
- The install returned `409 APP_EXTENSIONS_MISSING` with the Components guidance.
- Nothing changed: the document root still holds only `index.php`, there are 0 databases and
  no install record.
- Components `php-fpm`/`php8.4-fpm` succeeded from Debian sources only.

## Item 3: catalog apps

### PrestaShop 9.2.0 (the stop)

- The install returned `502 APP_SETUP_FAILED`: "PrestaShop is set up, but its administration
  folder could not be found (no renamed administration folder was found)".
- The installer's own output says the admin folder was renamed into `admin243uel9a76t8zr4wjhm/`
  and ends "Installation successful!".
- The site stays closed with a 503 "Being set up" page.
- On disk, `admin243uel9a76t8zr4wjhm` exists, next to PrestaShop 9's `admin-api` folder.
- Likely cause: the lookup for the renamed folder also matches `admin-api`, so it finds two
  candidates instead of one.

### MediaWiki 1.43.11

- Set up in 51 s.
- `/` answers 301 to the main page, and `index.php?title=Main_Page` answers 200.

### phpBB 3.3.19

- The install returned `APP_DOWNLOAD_FAILED` (HTTP 403).
- From the same NAT, download.phpbb.com answered:

  | User-Agent | Response |
  |---|---|
  | `CelikPanel/v0.1.0-alpha.118 (+https://celikpanel.net)` | 403 |
  | `Go-http-client/1.1` | 403 |
  | `curl/8.14.1` | 206 |

- Wikimedia accepts the CelikPanel user agent (206).

## Other notes

- **D2 (seen during prep):** quota was enforced (2.4) at 17:46:14, right after mail was
  installed. 552 came about 1 minute after the mailbox was created (`raw/R2a-a118.log`).
- **logrotate:** shows `written`.

## Cleanup

The guest, the lab portal, the dist and `/var/tmp/a118r1` were removed.
