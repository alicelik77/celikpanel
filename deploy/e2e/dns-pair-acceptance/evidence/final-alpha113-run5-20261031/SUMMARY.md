# Final acceptance run 5 (alpha.113 candidate dbf20b9bd) — evidence (2026-10-31)

**Result: not clean.** Arch standalone passed every step. On the pair, the
Debian main's server setup failed at its mail DNS step (D10) and could not
be resumed (D11), so the pair's site, mail, DKIM and deletion steps did not
run. The D8 startup wait works, but on every reboot it logs one false
timeout line (D12).

**Method:** the same as runs 3–4.
- Install with the owner's `get.sh` against the lab portal; only the pinned
  key digest differs.
- Acceptance-licence dist labelled `v0.1.0-alpha.113` (`dist.json`).
- The DKIM card was read only, never pressed.
- Secret scan clean (249 files, 24 secrets).
- Logs are in `final-alpha113-run5-evidence.tar.gz`, SHA-256
  `23da95203357f8290c940662f31055530008b4d9c0c3629ab25a603fb06c7ff7`.

## Defects
**D10: pair main (Debian 13), setup step `15-mail_dns` fails because the
host is briefly busy.**
- The step should wait while the host is busy. Instead the operation ended
  `server_setup_mail_dns_publish_failed`.
- Panel log, both at `01:18:28`: `dns V3 sync r5f113.test generation 15:
  another server change or package-manager task is still running`, then
  `server setup step 15-mail_dns failed`.
- The `protected-mail` profile finished at 01:18:20.7. Postfix was refreshed
  again at 01:18:26–27. No log line names the change that held the host.
- The main zone's `celik._domainkey` TXT was not published, and nothing
  retries it. The same step passed in runs 3–4, so the failure depends on
  timing.

**D11: a paired main cannot resume after a failed step.**
- "Review the plan again" answers `can_start: false` with
  `server_setup_infrastructure_dns_conflict` ("infrastructure address for
  ns2 differs from the reviewed server address").
- Start then answers 409 `server_setup_plan_blocked`.
- Cause: the reviewed draft has `peer_ip: ''`, because the joined peer's
  address lives only in the execution binding (`serverSetupEffectiveDraft`).
  The zone holds `ns2 → 192.0.3.114`, and the infrastructure check expects
  an empty address.

**D12: the startup wait logs a false timeout line.**
- About 0.2 s after boot, systemd stops the first Panel process
  (agent-driven restart).
- Before stopping, that process logs the "still starting after 2m0s" text.
- The second process then waits correctly and continues.
- Management stayed closed after a reboot for 1.9 s (Frankfurt), 1.6 s
  (Boston) and 2.5 s (Arch).
- No `systemd is not ready` refusal was left unretried.

## Pair (Debian 13 main + Ubuntu 24.04 joined by code)
- Installed in 50 s / 57 s; licence active.
- Backup setup: pass. Every wait carried a reason.
- Main setup: fails (D10, D11).
- One reboot of both servers: the Panel API answered, and DNS answered with
  the authoritative flag from both servers. Journals clean: 0 milter, FQDN,
  panel 500, fail-closed, ambiguous or FOREIGN KEY lines, and no failed
  units. The only err-priority line is the known Ubuntu MariaDB postinst
  warning.
- Not run: site, mail, DKIM, Rspamd, the DNS-only zone create and delete,
  the run 4 delete-after-reboot sequence, the extra site create and delete,
  and panel-off.

## Arch standalone (PowerDNS)
- **Install and site:**
  - Install and setup in 108 s.
  - Certificate in 3 s; 301; PHP page without `X-Powered-By`.
- **Mail and DKIM:**
  - 587 and 465 send, IMAPS read and Roundcube (3/3) work.
  - DKIM was set up by itself. Mail sent on 587, on 465 and from PHP
    `mail()` verifies against Arch's own PowerDNS.
  - The DKIM card read `served` 4 s after the mailboxes were created, then
    `ok`.
  - Rspamd scanned all three messages.
  - A bounce was delivered.
- **Reboot:** pass.
- **Site delete:**
  - Status read `running` with the neutral text, then "deleted" in 11 s.
  - No Agent hold.
- **New site afterwards:** created; certificate issued; https and 301 work.
- **Journals:** clean over 4 boots, apart from the D12 lines.
