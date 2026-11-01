# alpha.114 lab run 5, targeted check (1 Nov 2026)

**Result: not releasable.** The run stopped on a High failure, as the fail-fast rule requires.

I1 is fixed: both servers finished setup. A new defect appeared at the Debian 13 sanity check: one mail bounced right after a refused autoresponder save.

- **Candidate:** `release/alpha114-integration` d70c1d7ce.
- **Lab dist:** c3f4b328b on `lab/alpha114-label-r5` (local only). It is the candidate plus the version-114 lines; the budget check passed.
- **115-lab build:** 77cee8678. It was never used, and it is deleted.

| Item | Result |
|---|---|
| I1 topology A setup | **PASS**: Debian 13 first, Ubuntu 24.04 joined, `13-mail_profile` included, no `firewall_sync_failed`, "fail-closed" 0 |
| `firewall_waiting` | not observed; the busy race was not met this run |
| Sanity, Debian 13 | **FAIL (High) J1**: main check fine (2 delivered, 1 reply), but the mail after the `lmtp_rejected` refusal bounced |
| E-new-3 Dovecot stopped | **PASS**: 409 `dovecot_stopped`, then `applied` after Dovecot started |
| Sanity Ubuntu, E-new-4 a/b/c, E-new-5, S-new-1..3, S23 | NOT COVERED (stopped) |

## Defect
- **J1 (High):** after the route is removed, `postfix/lmtp ... status=bounced (550 5.1.1 User doesn't exist)`.
  - **Cause:** the save is refused as `lmtp_rejected` and the mailbox's route is removed from `celikpanel_transport`, but Postfix is not reloaded. trivial-rewrite served that mail from the old table and only then logged "table ... has changed -- restarting".
  - **Code:** `setAutoresponderTransport` (`cmd/agent/mail_autoresponder.go`) reloaded Postfix only when it first set `transport_maps`.
  - **Fix:** every change to a table that is in use is followed by a reload.
  - It is timing-dependent: run 2 passed the same check.

## Files
- Evidence tarball SHA-256: `344ca539f52f71d274ac2e79a4d2d3ddb24fff1df231a372d7618019a14874a8`
- Screenshots tarball SHA-256: `690551795d7ffb61193f9090700317f10e679ce27f0b90c34fe396026bc9a5c3` (empty; no screen step was reached)
- Dist SHA-256: `374f223990e685ac583f4b3e1e4d02d3960d0651fb7a071bb340352085b37f3d`
- Secret scan: clean (340 + 15 files against 25 secrets).
- Harness: `beta1/a114/run5/`. Lab commit 3824bb8cc.
