# a125: alpha.125 pre-release lab round (PR #298, `release/alpha125`)

*Lab only: QEMU/KVM in WSL `archlinux`, 192.0.3.x, a lab portal signed with a lab key, the
acceptance licence. Every guest was installed through `get.sh` in a root terminal, as the owner
installs. Every update was pressed through the panel's own update API, as the owner's Update button
does. No real server, celikpanel.net, licence service or real CA was contacted.*

**Topology (mirrors the owner's servers).**
- `p5` main: Debian 13, PowerDNS main, mail stack.
- `p5` backup: Ubuntu 24.04, BIND, joined by code.
- Both were installed at a lab copy of the published alpha.124 tag (`8f782e664`, dist `70ed7e89…`)
  and then updated to alpha.125.
- `d5`: a fresh Debian 12 install of alpha.125 (BIND, mail).
- Case 2 needs an update target beyond alpha.125, so a lab-only "alpha.126" label of the same
  candidate is used. It differs only in the release-sequence bump.

**Heads tested, and the alpha.125 dist each produced.**

| Head | Role | Dist sha256 | Lab 126 label |
|---|---|---|---|
| `bf0a5b9ee` | candidate as handed over | `cb4a7964…` | `3b25dad42` |
| `3f58cd02c` | fix 1 | `5b073ad8…` | `64c22f5de` |
| `1600949e8` | fix 2 | `7193c203…` | `0c9b9d2cc` (dist `8c0ddc88…`) |

**Lab loads, named where used.**
- `hostnamectl` and `/etc/hosts` set to the owner's machine name before install.
- An unsupported key in `panel.env`, to force a rollback.
- A process named `dpkg`, standing for a package task that starts during an update.
- `apt-get install redis-server` outside the panel.
- The owner's own `postconf -e` values, followed by `systemctl restart celikpanel-panel`.
- A hand-set `MAILTO` line in a site crontab.

**Lab environment incidents (not product).**
- Run 1: the WSL host's network adapter failed under memory pressure. The kernel logged `hv_netvsc
  … sub channel open failed: -12` (an order-8 allocation failure) and then held the kernel's network
  lock. The owner approved `wsl --shutdown`, and everything was rebuilt with less guest memory.
  Run 1's logs are in `raw/run1/`.
- On the published alpha.124, the main's first setup stopped at `mail_profile` with
  `mail_tls_sync_host_busy` (a typed refusal with guidance). Pressing "review the plan" and Start
  again finished it.

## Results

| # | Case | Result |
|---|---|---|
| 1 | Update 124 → 125 on both servers | **Defect found and fixed (`3f58cd02c`); re-run PASS** |
| 2 | Update right after a rollback | **Defect found on Debian 12 and fixed (`1600949e8`); PASS on Debian 13; Debian 12 update PASS** |
| 3 | Faster panel | PASS |
| 4 | Mail phase 2 | PASS |
| 5 | Typed 409 refusals | PASS |
| — | Fresh Debian 12 install of 125 (smoke) | PASS |

### 1. Update 124 → 125

**Backup (Ubuntu 24.04).** Succeeded on the first press:
- verify done 00:00:32, restart done 00:01:22, confirm done 00:01:34;
- `systemctl --failed` empty; no "database publication is unverified".

**Main (Debian 13).** Defect.
- The first press was at 23:59:58. The attempt failed at 00:00:11 with `!! independent recovery
  runtime preparation is unconfirmed; panel services have not been stopped`.
- At that moment Debian's midnight `apt-listchanges` (00:00:02–00:00:19) and `dpkg-db-backup` were
  running.
- Reproduced exactly (`C1repro-p5.log`): a process named `dpkg` started the moment the staged
  runtime's checker ran (00:06:54), and the attempt failed at 00:06:57.

**Cause.** update.sh's `prepare-runtime` step runs before `verify-compatibility`.
- `prepare-runtime` promotes the new recovery runtime. The promotion runs the same Panel and Agent
  read-only checkers, but re-reads a refusal only once, after 2 s.
- The alpha.125 wait for a finishing operation (`37ee2ed14`) covers only the preliminary probes and
  `verify-compatibility`.

**Fix `3f58cd02c`** (`internal/recoveryruntime`, `runPromotionChecker`).
- An Agent checker refusal that names a running privileged mutation or the host package manager is
  re-read every 3 s for up to 60 s. The phrases and the bound are the same as update.sh's.
- Every other refusal keeps its single re-read.
- The boundary is re-proved around every read.

**Re-run** (`C1fix-p5.log`), with the same `dpkg` load held for 20 s:
- Update pressed 00:18:59; checker seen 00:19:12; the load ended 00:19:32.
- **Succeeded on that press at 00:20:31**, agent commit `3f58cd02c`.
- The Agent cleared both earlier failed self-update units ("cleared the failed mark …; record kept").
- `systemctl --failed` empty; no "unverified" line.
- The panel's start-up mail work logged that it waits for the update ("retries by itself … nothing
  needs to be done now").
- The late database check did not need to retry in any run, so there is no reason line to show.
- An update finalized by release recovery was not forced. It is covered by Go test only.

### 2. Update right after a rollback

**Debian 12 (`d5`, candidate `bf0a5b9ee` → lab 126).** Defect.
- The update failed after the panel and Agent had stopped: `CELIKPANEL_UPDATE_FAILURE …
  state=recovery_required reason=Certbot scheduler state could not be added to the service ledger`.
- Release recovery then refused every 30 s with `service-state ledger must contain exactly three or
  five rows`. **The panel and Agent stayed down and nothing rolled back**
  (`C2-d5-debian12-update-failure.log`).

**Cause** (present since alpha.92):
- systemd 252 prints nothing for `systemctl is-enabled certbot-renew.timer`, a unit that does not
  exist on Debian 12 (exit 1). systemd 257 prints `not-found`.
- The scheduler capture appended `certbot.timer` before reading the second timer, so its refusal
  left a four-row ledger.

**Fix `1600949e8`** (`deploy/panel-tls-snapshot.sh`).
- An empty answer with a non-zero exit is read as `not-found`, but only when LoadState says
  `not-found`.
- Both rows are validated before a single append.
- New contract-test cases fail on the old helper and pass now.

**Re-run on Debian 12** (fresh install of `1600949e8`; `C1-d5-main.log`): Update pressed once,
**succeeded in 71 s**, agent `0c9b9d2cc`, `systemctl --failed` empty. Ubuntu 22.04 (systemd 249) is
affected the same way but was not run.

**Debian 13 main** (`C2-p5.log`; 125 at `3f58cd02c` → lab 126 `0c9b9d2cc`):
- The forced attempt failed with `installer stopped: panel.env line 5 has an unsupported key …
  (remove the line, then retry)` and rolled back.
- The panel answered at 00:53:58. The panel's start-up mail work ran at 00:53:55 and yielded
  ("another server change … retries by itself … nothing needs to be done now").
- Update was pressed at 00:53:59, **one second after the panel answered**. It succeeded at
  00:55:02.
- One new attempt record and no duplicate operation. The failed unit of the rolled-back attempt was
  cleared; `systemctl --failed` empty.
- The update screen shows its waiting note while the "prepare" step runs. That step ran from
  00:54:00 to about 00:54:50.
- The sentence "the update waits for it (up to a minute) … Nothing needs to be done" is in the
  served bundle in both EN and TR (`C5extra-p5.log`). No screenshot was taken in the lab.

### 3. Faster panel

Timings are 5 reads each, as `elapsed` from the panel API. The 124 numbers were taken before the
update and the 125 numbers after it, on the same servers.

| | Main 124 → 125 | Backup 124 → 125 |
|---|---|---|
| Components scan (POST) | 1.75–1.97 s → 0.69–0.86 s | 2.05–2.19 s → 0.58–0.66 s |
| `/hosting/capabilities` | 1.44–2.32 s → 0.55–0.73 s | 3.25–3.33 s → 1.85–1.95 s |
| `/setup/components` | 0.84–0.90 s → 0.08–0.10 s | 1.19–1.30 s → 0.06–0.08 s |

- **Same answers** (`C3-same-answers.txt`, time stamps dropped): all identical. The one exception
  is nginx on the main, which lists `shop5.test.conf`, a site created between the two reads.
- **apt outside the panel:** `redis-server` installed by apt was shown as installed by Rescan after
  1.4 s and by the page read after 5.0 s.
- **Controls during a scan:**
  - On 124, all 12 nginx reloads pressed during a scan were refused (`409
    service_operation_busy`).
  - On 125, the backup's reloads were 6/6 accepted.
  - On the main, the first two right after the update met the start-up mail work, and then
    reloads were accepted. The scan, not the control, gets `409` when a change already holds the
    server, as designed.
- **Compression:**
  - `Content-Encoding: gzip` on JS, CSS, `/managed-services`, the scan POST and
    `/setup-components`.
  - Not on answers under 1 KiB.
  - Not on secret-word paths: `/database-servers/1/admin-account`, `/auth/me`, and
    `/system-databases` (1534 B, uncompressed).

### 4. Mail phase 2 (main, after the update; `C45-p5.log`)

**Postfix settings.**
- After the update: `authorized_submit_users = root, vmail`, and `smtpd_relay_restrictions =
  permit_sasl_authenticated, defer_unauth_destination`.
- The record keeps the before values: `static:anyone` (default), and `permit_mynetworks …` (set).

**What is refused.**
- The site user `shop5_test`, via PHP-FPM `mail()`, php-cli `mail()` and `sendmail`: `bool(false)`
  or exit 75, logged as `fatal: User shop5_test(5002) is not allowed to submit mail` (three journal
  lines).
- Port 25 without a password to another server: `454 4.7.1 Relay access denied`.

**What still works.**
- Port 25 without a password into the server's own mailbox: delivered to maildir.
- root `sendmail` and vmail `sendmail`: delivered to `ext5@d5.test` on the other lab server.
- Port 587 with the mailbox password: 235/250.
- Panel account mail (an address change, handed over by the Agent as root): "Confirm your email
  address for CelikPanel" was delivered to `acct5@d5.test`.

**Crontabs.**
- The site crontab written before the update got `MAILTO=""` once.
- The owner's `MAILTO="webmaster@p5.test"` was kept, including after a later panel write.
- A new panel-written crontab on Debian 12 starts with `MAILTO=""`. root's crontab was untouched.

**Owner's own values.**
- The owner set `authorized_submit_users = static:anyone` and a relay list that includes
  `permit_mynetworks`, then restarted the panel.
- Both values were kept. The report shows `state: owner` with CelikPanel's form beside each value.
  The journal says "kept as they are".

### 5. Typed 409 refusals

- An existing mailbox: `409 MAIL_ACCOUNT_EXISTS`.
- On the static main site: `409 SITE_HAS_NO_PHP` (reason `static`, action "Open Hosting") for a PHP
  version change, a pool read and a pool save. The PHP site still answers 200.
- The EN and TR sentences for both codes are in the served bundle.

### Fresh Debian 12 install of 125

Setup succeeded and the machine-name A record resolves, at both `bf0a5b9ee` and `1600949e8`.

On the `bf0a5b9ee` install (`run-case2.txt`, smoke part):
- the Components scan answered 200 with 12 components installed;
- a customer site was created and served from the lab network;
- its crontab got `MAILTO=""`.

The `1600949e8` install was used for the update check above.

## Open items
1. Ubuntu 22.04 updates were not run. The same systemd behaviour applies there, and fix 2 covers it.
2. A server already left with a four-row ledger by an earlier release keeps release recovery
   refused. It needs the owner's recovery path; that path was not changed here.
3. Not forced in the lab: an update finalized by release recovery, and a retry with a reason line in
   the late database check.
4. Run 1's setup on the published 124 met `mail_tls_sync_host_busy` once during `mail_profile`. It
   is typed and recoverable, and was not investigated further.

Files: `raw/` holds the logs (`run-*.txt` are the driver outputs), `raw/perf/` holds the 124/125
answers, and `raw/kit/` holds the scripts.
