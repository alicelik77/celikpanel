# a124 — alpha.124 pre-release lab round (PR #296, `release/alpha124`)

*2026-11-05 · lab only (QEMU/KVM in WSL `archlinux`, 192.0.3.x, lab portal signed with a lab key,
acceptance licence, Pebble CA). Every guest was installed through `get.sh` in a root terminal (the
owner's command). No real server, celikpanel.net, licence service or real CA was contacted.*

**Heads tested.** `dd6f65752` (candidate as handed over) → `9561b2df9` (fix 1) → `4881738d3` (fix 2).
Dist sha256: dd6f65752 `4f2c60ae…`, 9561b2df9 `52c3c53d…`, 4881738d3 `a182e25d…`; lab copy of
alpha.123 (`d7b5f1ab5`) `a9d9f3aa…` (`raw/dist-*.json`, `raw/release-manifest-v2-*`, `raw/get.sh.diff-*`).

**Lab load (said where used).** `hostnamectl` + `/etc/hosts` for the owner's machine name before
install; dpkg lock holder; an unsupported key in `panel.env`; SQLite `BEGIN IMMEDIATE` holder;
`systemctl stop`/`chown` of the Agent socket; a hand-started second Agent; Sury lane + PHP 8.3/8.4.

## Results

| # | Case | Result | Evidence |
|---|---|---|---|
| 1 | Update 123 → 124 (Debian 13) | **Defect found, fixed (9561b2df9), re-run PASS** | `C1-u123.log`, `C1b-u123.log`, `C1r-u123b.log`, `u123*-journal-update.txt` |
| 2 | Fresh installs + machine FQDN A record | PASS on Debian 13 (PowerDNS main of a pair), Debian 12, Ubuntu 22.04, Ubuntu 24.04 (BIND) | `single-*.log`, `pair-p124.log`, `op-*.json`, `setup-*.json` |
| 3 | Component install window | PASS | `R3-p124.log`, `R3b-p124.log`, `R3-u24.log`, `R2-d12.log`, `preview-pgdg18-d12.json` |
| 4 | Panel DB write lock held 30 s | PASS | `R4-p124.log` |
| 5 | Agent unreachable guidance | PASS (one typed exception, below) | `R5-p124.log`, `R5b-p124.log` |
| 6 | DNSSEC gate on PowerDNS main + backup | **Defect found, fixed (4881738d3), re-run PASS** | `R6-p124.log`, `R6r-p2.log`, `pair-p2.log` |
| 7 | Passwordless mail phase 1 | PASS | `R7-p124.log`, `R7b-p124.log` |

### 1. Update path
- `u123` (dd6f65752): dpkg lock held → `409 HOST_MUTATION_BUSY / package_manager_active`, no record, no
  unit. Unsupported key in `panel.env` → attempt failed `installer stopped … (remove the line, then
  retry)`, automatic rollback, unit `celikpanel-self-update-132f…` failed. Pressed again 20 s after the
  rollback → failed `recovery_runtime_preflight_failed state=unchanged … agent_ledger_check: service
  mutation state is not idle`: the panel's start-up mail work (`mail_tls_sync` 12:24:00, then
  `mail_filter_wire`) began one second after the Agent admitted the update (12:23:59). Third press →
  succeeded in 101 s (`v0.1.0-alpha.124`, agent commit matches).
- After the update: `agent --version` prints `CelikPanel agent v0.1.0-alpha.124 (commit …)` rc 0;
  `agent bogus` rc 2 "nothing was started or changed"; a hand-started second Agent rc 1 with the plain
  message, socket stays `root:celikpanel`, same inode, systemd PID unchanged.
- **Defect:** both earlier failed units stayed under `systemctl --failed`. The new Agent started at
  12:31:11 while the attempt was still active (worker finished 12:31:25), so the start-time
  `Reconcile` returned before clearing and nothing ran it again. **Fix `9561b2df9`:** `Status`
  clears once per successful attempt with the same rule (no other attempt active; the success and
  earlier attempts; only units systemd reports failed; records kept). Test
  `TestSystemUpdateStatusClearsResolvedUnitsOnceAfterTheSuccess`.
- Re-run `u123b` (fresh Debian 13 at 123 → 9561b2df9): busy 409, failed attempt, update pressed once
  after 120 s → succeeded 155 s; journal `self-update: cleared the failed mark of
  celikpanel-self-update-05653…service`; `systemctl --failed` lists no self-update unit; Agent checks as
  above. PASS.
- Note: before the update my check order ran the hand-started Agent on **123** — it ignored
  `--version`, re-created the socket `root:root` (the Frankfurt defect, reproduced on the old
  version); `sudo systemctl restart celikpanel-agent` restored `root:celikpanel`.

### 2. Machine name record
Owner machine names `host1.<zone>` (set before install). Draft carries `host_name`; after setup
`dig @server host1.<zone> A` answers the server address on Debian 13 (both pair servers answer),
Debian 12, Ubuntu 22.04, Ubuntu 24.04. PowerDNS refused on Debian 12 / Ubuntu 22.04
(`server_setup_pdns_untested_system`), as before. The joined (backup) server's own name gets no
record (the change covers the first server only).

### 3. Component install window (Debian 13 `p124`, Ubuntu 24.04 `u24`, Debian 12 `d12`)
- `install-plan?id=phppgadmin`: `together ["postgresql"]`; one `service_install_together` operation
  (`installing:postgresql`). Reload: `active=1` and `request_id` reads re-attach (running). Panel
  restart 12 s in: the panel answered again after 58 s; the operation had finished at 12:41:14 during
  start-up; the tab marker read `succeeded`, `installed_together ["postgresql"]`, audit
  `service.install.together.recovered`. PostgreSQL 17 online, phpPgAdmin on 8.4.
- Debian 13: Sury lane → PHP 8.3 beside 8.4; phpMyAdmin with `php_version 8.3` → proof `8.3`, nginx
  `fastcgi_pass …php8.3-fpm.sock`; removing 8.3 → `409 REMOVAL_TOOLS_BOUND ["phpMyAdmin"]`.
  Ubuntu 24.04 (distribution 8.3): phpMyAdmin on 8.4, a customer site on 8.4; removing 8.4 →
  `409 RUNTIME_IN_USE ["php84.test (cust3u)", "phpMyAdmin"]` (site and tool, each linked).
- Debian 12: PGDG lane enabled; PostgreSQL 18 preview notable `libpq5`, `postgresql-client-common`,
  `postgresql-common`, all `vendor_shared` with repository `pgdg` (no longer "vendor"); confirmation
  required.

### 4. Held DB lock
Install of Redis; lock held 12:44:45–12:45:15. Reads: `installing` → `panel_busy` (12:44:57) →
`scanning` → `succeeded` 12:45:19. Audit rows 42 → 43 (`service.install:redis`, the one row an install
writes); no `audit-pending.json` left; no row lost.

### 5. Agent unreachable
Agent stopped → `503 AGENT_UNREACHABLE` "…is not running … sudo systemctl status celikpanel-agent …"
(Postfix queue, passwordless report, candidate, DNSSEC sign). Socket `root:root` → `503
AGENT_UNREACHABLE` socket-denied sentence with `sudo systemctl restart celikpanel-agent`. After that
command everything answers again. Install preview answers `state unknown, reason_code
agent_unreachable`. The DNSSEC status read answers its own typed `502 DNSSEC_STATUS_UNAVAILABLE`
("could not be verified"), not the Agent sentence (no 500 anywhere).

### 6. DNSSEC gate
`p124` (dd6f65752): GET `signing {available:false, reason DNSSEC_PAIR_UNSUPPORTED, message …}` for
admin and customer; POST → `409 DNSSEC_PAIR_UNSUPPORTED`, no keys, zone unsigned on both servers.
**Defect:** the refusal came after `prepareDNSZoneSyncV3Plan` advanced the zone's SOA serial and the
refused path republished it (serial moved 2026110501 → 04 over two clicks plus the domain's own
follow-up publications), while the answer says "Nothing was changed". **Fix `4881738d3`:** the POST
reads the Agent's read-only status first and refuses before any publication is prepared (older Agent:
unchanged path). Test `TestDNSSECPairRefusalChangesNothingInTheZone`.
Re-run `p2` (fresh Debian 13 PowerDNS main + backup at 4881738d3, quiet zone `sec2.test`): GET closed with the reason; admin and customer POST → `409 DNSSEC_PAIR_UNSUPPORTED`; SOA `2026110501` before and after on both servers, `Last SOA serial number we notified: 0` (no republish, no NOTIFY), no keys. PASS. The pair also showed `host1.p2.test` on both servers (case 2).

### 7. Passwordless mail phase 1
Site `p124.test` (user `p124_test`): PHP `mail()` (php-cli as the site user, same sendmail path)
`bool(true)`; SMTP to 127.0.0.1:25 without a password accepted (`250`); both to another server.
Postfix page list: `p124.test messages 2, ways ["sendmail","smtp"]`. "Create a sending mailbox" path
(`POST /domains/1/mail/accounts noreply@p124.test`) → 200, settings `smtp mail.p124.test:587
STARTTLS auth_required`; the list then shows `sender_mailbox noreply@p124.test`. Authenticated 587:
`235`, `250`, `sasl_username=noreply@p124.test`, not counted as passwordless. Nothing blocked
(`authorized_submit_users` default, `permit_mynetworks` kept). Local deliveries are not listed (by
design: only mail that left for another server counts).

## Not fixed (open, for the owner / alpha.125)
1. **Update right after a rollback:** the Agent admits the update while idle; the panel's start-up
   mail work starts a mutation a second later; the worker's preflight then refuses (`state=unchanged`).
   A second press works. Needs a design choice (hold new mutations while an update is queued, or
   refuse the start during the panel's start-up window as `agent_startup` does for installs).
2. Creating a mailbox that already exists → `500 INTERNAL` (UNIQUE constraint) instead of a typed
   409 (pre-existing; the new notice hides the button once the mailbox exists).
3. Changing the PHP version of the setup-created static main site → `500 invalid PHP version ""`
   (pre-existing).
4. Commit 45a2b095f says "tools alone still never refuse"; the lab shows removing PHP 8.3 used only by
   phpMyAdmin refused (`REMOVAL_TOOLS_BOUND`). Behaviour is safe; the wording or the rule needs a look.
5. Agent unreachable on the DNSSEC status read gives `DNSSEC_STATUS_UNAVAILABLE`, not the Agent
   sentence.

## Lab notes
- The labnet resolver had backed off the new pair's servers (queried before the zone existed); setup
  waited at `access_dns` until its infra cache expired (~10 min), then went on by itself.
- `p124` backup was stopped after case 6; new domains on the main then waited (`pair_pending`), so
  the site parts of cases 3 and 7 used the main site and the Ubuntu 24.04 guest.
- WSL disk (archlinux): before 41G used (vhdx 64.0 GB); after: 42G used (vhdx 64.0 GB, unchanged). All guests stopped, overlays
  deleted.
