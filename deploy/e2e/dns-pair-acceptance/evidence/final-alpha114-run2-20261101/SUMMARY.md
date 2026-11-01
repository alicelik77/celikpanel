# alpha.114 lab acceptance, run 2 (1 Nov 2026)

**Result: not releasable.** All of run 1's defects (E1–E8) and screen findings (S1–S24) are fixed in the lab. Run 2 found one release blocker (E-new-1) and two new High defects (E-new-2, E-new-4). The run then stopped under the owner's new fail-fast rule, so the items under "Not covered" were not run.

- **Candidate:** `release/alpha114-integration` 7d1d4ac7a.
- **Lab dist:** local commit b6800f082 on `lab/alpha114-label-r2`.
- **Host reboot mid-run:** the host restarted during the run, and topology A was reinstalled from the dist. The logs from before the restart are in `raw/attempt1-before-host-reboot`.

## Run 1 items
| Item | Result |
|---|---|
| E1 | PASS. The first server's panel restarted while the joined server was publishing. The joined server waited with `server_setup_dns_publisher_peer_busy` and carried on by itself. |
| E2 | PASS: both servers answer 409 `PANEL_PORT_PAIRED`. |
| E3 | PASS: 8 records were added while DKIM was running. All answered 200 and are present on both DNS servers. |
| E4 Debian 13 | PASS: 2 delivered, 1 reply, no bounce. The strip line gives `lmtp_rejected` and mail still arrives. The stopped-Dovecot case fails: see E-new-3. |
| E4 Ubuntu 24.04 | PASS |
| E5 | PASS |
| E6 | PASS: MX, SPF, DMARC and DKIM are on both servers, DKIM verifies, and the records are removed when the domain is deleted. |
| E7 | PASS: 113 → 114 on the D first server and both C servers. Schema 46 → 51, port 2083 kept, v3 files byte-identical. Rollback was `rollback_verified`, and a second update succeeded. |
| E8 / 3b | PASS, for both an alpha.114 first server and the Boston case. |
| Check 8 | PARTIAL: the "waiting for backup" state is never reached by design. |
| B, D, reboots, panel-off, deletes, checks 6/7/9, 2FA | PASS |
| S1–S24 | Fixed in EN and TR. No sideways scroll at 390 px. |

## New defects
- **E-new-1 (blocker):** the web bundle budget fails. `i18n/screens/setupLink/tr.ts` is 13.35 KiB against a 13.00 KiB limit.
- **E-new-2 (High), Arch, Dovecot 2.4.5:** the autoresponder drop-in fails on reload with "Unknown section name: sieve_script", yet `systemctl reload` still reports success. The owner gets the wrong guidance.
- **E-new-3 (Medium):** with Dovecot stopped, saving gives `lmtp_rejected` instead of `dovecot_stopped`. The cause is a leftover socket file.
- **E-new-4 (High):** deleting a domain on the joined server stays at `dns_cleanup` / `unknown`, even though the zone is already gone.
- **E-new-5 (Medium):** after a verified rollback, the update record says `failed` / `recovery_required` and the cause is empty.
- **Screens:**
  - **S-new-1:** the joined server's ready page offers "Host a website" for the main domain.
  - **S-new-2:** a raw "(pdns)" label appears in Add domain.
  - **S-new-3:** the audit log shows raw action codes.
  - **S-new-4:** a one-server setup mentions "the server that will join".

## Not covered
- Running the S23 command in a terminal.
- A long preflight refusal line under Details.
- The mail dialog's "Check again" button (no wait happened, so it never appeared).

## Files
- Evidence tarball. SHA-256: `107d308c4561818edbc9168a643efd4ac1df6c5d8db820f61590a7ba71578763`
- Screenshots (552 WebP). SHA-256: `23f4d897478a1daa0aad0081c1d543ba3e72c4dbee69c30b28f9acad38219cff`
- Secret scan: clean (2488 files checked against 61 secrets).
- Harness: `beta1/a114/run2/`.
