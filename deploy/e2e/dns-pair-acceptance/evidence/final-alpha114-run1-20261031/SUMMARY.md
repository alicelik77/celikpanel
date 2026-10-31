# alpha.114 final lab acceptance, run 1 (31 Oct 2026)

**Result: not releasable.** We found 8 product defects (E1–E8) and 24 screen findings (S1–S24). Defect E7 blocked the update path (topology C) for PowerDNS first servers. All of them were fixed on `fix/a114-f1..f4` and merged into `release/alpha114-integration`. Run 2 re-checks them.

- **Candidate:** `release/alpha114-integration` 8303c0a87.
- **Lab dist:** lab-only commit 49c7efd5b. It is the candidate plus the version-114 policy and `get.sh` lines that a release adds.
- **How it was installed and updated:** with the owner's `get.sh`, from a lab copy of the download portal on labnet. Updates went through the panel's own update API (`/panel/update/check`, `/start`, `/status`).

## Topologies
| | Setup | Result |
|---|---|---|
| A | Debian 13 first server (PowerDNS main, web + mail + DNS) and Ubuntu 24.04 server joined by code | Run; E1, E3, E4, E6 |
| B | Arch on its own | Run; E5 (redirect) |
| C | Debian 13 + Ubuntu 24.04 pair installed on alpha.113, then updated to the candidate | Joined server updated (schema 46 → 51, stays on port 2083). First server was refused (E7) |
| D | alpha.113 first server on 2083, with a candidate joined server on 1923 | PASS; Components mail E8 |

## Checks
| # | Check | Result |
|---|---|---|
| 1 | Wizard first screen | PASS |
| 2 | Main domain as a website | PASS: the Yes path serves https on the apex and www; the No path also behaves correctly |
| 3a | Mail on the joined server, set up in the wizard | PARTIAL: the name is refused and the proposal is right, main records unchanged, mail flows both ways. FAIL E6 |
| 3b | Mail added later from Components | FAIL E8: the dialog opens in place, but the final check never ends |
| 4 | Panel port | PASS, apart from E2 |
| 5 | OS updates | PASS on Debian, Ubuntu and Arch |
| 6 | Accounts, resellers, customers | PASS, apart from E3 |
| 7 | Site features | Protected folder, redirect injection and `nginx -t` PASS. E4, E5 |
| 8 | DNS overview | PARTIAL: the state settles by itself. The "waiting for backup" wording and the IPv6 glue case were not reached |
| 9 | No manual refresh | PASS where tried |
| 10 | Screenshot review | 501 shots, 24 findings |

## Defects
| ID | Severity | What happened |
|---|---|---|
| E1 | High | The joined server's setup stopped at `dns_publisher` with "code expired or used". The code was valid; the first server was busy |
| E2 | Medium | The panel port can be changed on a joined server; it should be refused |
| E3 | Medium | Adding a DNS record returned 500 SQLITE_BUSY although the record was saved |
| E4 | High | On Debian 13 (Dovecot 2.4), turning on the autoresponder makes every mail to that mailbox bounce (LMTP 550) |
| E5 | Medium | A folder redirect drops the rest of the path when the URL has a query |
| E6 | High | A customer domain on the joined server gets no MX, SPF, DMARC or DKIM records |
| E7 | Critical | Updating from alpha.113 is refused on PowerDNS first servers at `application_compatibility` |
| E8 | High | After mail is added in place, the final check waits forever on "dns" |

The exact sentences, file locations and screenshot names are in the logs tarball.

## Not covered in run 1
- **Blocked by E7:** updating a PowerDNS first server, and every check that comes after it.
- **Never run:**
  - Components mail with an alpha.114 first server.
  - Port rollback after a restart during the trial.
  - Reboots, panel-off and deletes on A and B.
  - Autoresponder on Arch.
  - The "waiting for backup" DNS wording and the IPv6 glue case.

## Files
- `final-alpha114-run1-evidence.tar.gz` (logs). SHA-256 `f594fea6c2bf6a454d252fbdb7477b080b1b4ed2acc08671e0f38558b4bdfe96`
- `final-alpha114-run1-screenshots.tar.gz` (WebP images and page text, 23 MB). SHA-256 `acc0a570edb1fb2c47a20f7fc7f7d0a21f7134d0a0a275f4cdc062bf7a5a3a1f`
- `secret-scan.json`: clean. It scanned 1237 files against 57 known secrets.
- Run logs: `pairA.out`, `pairC.out`, `pairD.out`, `opsA.out`, `opsB.out`, `archB.out`, `cd.out`, `stopA.out`, `wiz.out`, `dist.json`, `get.sh.diff`.
