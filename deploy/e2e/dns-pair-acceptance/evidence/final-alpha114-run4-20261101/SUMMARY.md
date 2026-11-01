# alpha.114 lab run 4, targeted check (1 Nov 2026)

**Result: not releasable.** The run stopped on a High failure, as the fail-fast rule requires.

Both run 3 defects are fixed in the lab:
- **H1:** no "could not be checked" or "fail-closed" lines, and the Ubuntu joined server finished setup.
- **H2:** an update started 11 s after a rollback succeeded, and a busy update was refused while the panel kept running.

A new timing race stopped the Debian first server's setup.

- **Candidate:** `release/alpha114-integration` 449701242.
- **Lab dist:** 7e5cd5ab0 on `lab/alpha114-label-r4` (local only). It is the candidate plus the version-114 lines; the budget check passed.
- **115-lab build:** 04e0494ec, for E-new-5. It is deleted.

| Item | Result |
|---|---|
| H1 Dovecot reload check | **PASS** for the run 3 defect. Setup itself stopped on I1. |
| H2 refused update leaves panel running | **PASS** |
| E-new-2(a) Arch 113 → candidate autoresponder | **PASS**: "restarted once" warning, the next save is clean, 2 delivered, 1 reply |
| E-new-2(b) fresh Arch | **PASS**: pigeonhole installed, no warning, the reply works |
| E-new-5 rollback record | **FAIL (Medium) I2**: `rolled_back` is shown and the reason names the error, but the detail is empty |
| S-new-4 one-server names step | **PASS** |
| Sanity, E-new-3, E-new-4 a/b/c, S-new-1..3, S23 | NOT COVERED (stopped) |

## Defects
- **I1 (High):** the Debian 13 first server's setup failed at `13-mail_profile` (webmail) with `firewall_sync_failed`.
  - The firewall sync after the webmail operation met a busy host: opendkim was installing and restarting, and Postfix was reloading.
  - That busy refusal was turned straight into a failed step instead of a wait.
  - Suspect code: `syncFirewallAfterOperation` → `firewallSyncFailure` in `cmd/panel/service_operations.go`.
- **I2 (Medium):** `boundedPanelUpdateFailure` (`cmd/panel/system_update_handlers.go`) drops the detail for `update_failed`.
- **Cosmetic:** the Arch "restarted once" warning ends in a stray `"; "`.

## Harness note
The first H2 pass was scored FAIL because the harness counted the update's own restart step as a stopped panel. The rule is now "down for longer than the 360 s restart bound, or not running at the end".

## Files
- Evidence tarball SHA-256: `f4f8097f7ee4725c80ef5ef2380d8801c03e20f37e964ea9f2672d47eb7571ca`
- Screenshots SHA-256: `5b6735a57ff59412c87c8c35c1e323e5c31b8d13b9a3d1e90b3c3689466e7abf`
- Dist SHA-256: `bda8dd83e53f88acec5c2ebec0c9021149f071f8dfa64c0bf21d6c57a9d413c1`
- Secret scan: clean (517 files checked against 32 secrets).
- Harness: `beta1/a114/run4/`. Lab commit 7cb2a7e4b.
