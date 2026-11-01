# alpha.114 lab run 3, targeted check (1 Nov 2026)

**Result: not releasable.** The run stopped early on a Critical failure, as the fail-fast rule requires. Fresh installs with mail never finish setup on Debian 13 or Ubuntu 24.04. A second, High defect appeared on Arch.

- **Candidate:** `release/alpha114-integration` aa84d2307.
- **Lab dist:** 650d4becf on `lab/alpha114-label-r3` (local only). It is the candidate plus the version-114 lines.
- **Scope:** only the items fixed after run 2. This was not a full acceptance run.

| Item | Result |
|---|---|
| E-new-1 bundle budget | **PASS.** The budget now stops the build when exceeded. `setupLink/tr.ts` is 9.29 of 13.00 KiB. |
| E-new-2 Arch autoresponder | NOT COVERED (stopped) |
| E-new-3 Dovecot stopped | NOT COVERED (stopped) |
| E-new-4 joined-server delete | NOT COVERED (setup never finished) |
| E-new-5 rollback record | **PARTIAL.** On 113 → candidate, the reason names the installer's error. `rolled_back` and the detail exist only from 114 onward. |
| S-new-1..4, S23 | NOT COVERED |
| Debian/Ubuntu autoresponder sanity | **FAIL.** Blocked by H1. |

## Defects
- **H1 (Critical, regression from the run 2 fixes).** The mail step of setup never ends on Debian 13 (Dovecot 2.4.1) or Ubuntu 24.04 (2.3.21).
  - The new Dovecot reload check reads `doveadm log errors` through the mail TLS command runner, which never allowed `doveadm`.
  - The unreadable record was treated as a failure, so the mail certificate setup and its rollback failed.
  - The Agent then held every change ("service mutation manager is fail-closed").
  - Separately, the check could not read Dovecot 2.3's log order (`master: Warning: SIGHUP received`).
- **H2 (High), Arch, alpha.113 → candidate.**
  - An update started 11 s after a verified rollback was correctly refused at `agent_idle`.
  - Recovery then stopped with "Quiesce abort verification failed".
  - The panel was left `failed` and the Agent `inactive`, while hosted services kept running.
  - The recovery unit asks the owner to run `recovery recover --retry` by hand.

## Lab note
On Arch, cloud-init rewrites `/etc/hosts` at the install restart and drops the lab portal name. The harness now adds it back. The first attempt is kept as `opsR-attempt1-hosts-lost.out`.

## Files
- Evidence tarball SHA-256: `1b54e830490c816327154e7ebd3ee22f181eb5257ad4784da2210ba4466cf388`
- Screenshots (3) SHA-256: `b1712f3e0193b0dcdc85a4f6c2b40f389c9daa9e9c8809a1f72077ff32ff3ed4`
- Dist SHA-256: `077e193852ab991e68ef185f6101ab5668ca47d4c7af403bd70fa5bcab314296`
- Secret scan: clean (401 files checked against 27 secrets).
- Harness: `beta1/a114/run3/`.
