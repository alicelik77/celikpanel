# a128 round 2: fix/a128-lab after the merge of release/alpha128 (808067d59)

*Lab only, the same lab as round 1 (QEMU/KVM in WSL `archlinux`, lab portal with a lab key, acceptance
licence, pebble, lab CA, lab Git server). Every install through `get.sh`, every update pressed once
through the panel's own update API. Boston, Frankfurt, real CAs, licence services and celikpanel.net
were not touched.*

## Heads

| Head | Role | Dist sha256 |
|---|---|---|
| `0c7aefe86` | v0.1.0-alpha.127 tag (lab copy), the pair's "before" | `097346b4…` |
| `ef891c290` | candidate (release/alpha128 808067d59 + round-1 fixes + A1–A7), carries 128 | `71e12c74…` |
| `af3c4996b` | lab 129 label of `ef891c290` (case 8 target, A7 update) | `69291eaa…` |
| `df1502cbc` | lab 130 label of `5edae8f22` (+ round-2 fixes 1–4 below) | `beb5eb7b…` |
| `efe09b2c1` | lab 131 label of `ab931ff0a` (+ round-2 fix 5) | `4a639c63…` |

Topology: `r9` main Debian 13 (PowerDNS main, mail, 192.0.3.190) + joined Ubuntu 24.04 (BIND backup,
192.0.3.191), installed at 127; `f9` fresh Debian 13 at 128 (192.0.3.194).

## Results

| Item | Result |
|---|---|
| Case 1 again: pair 127 → 128, one press each | PASS: joined 72 s, main 68 s, schema 65 → 72, 0 native-file differences (249 + 199 files), `postconf` unchanged, no failed unit, sites 200 |
| Case 8 again: fresh 128 → lab 129 forced failure | PASS: refused after services stopped, rollback complete 27 s later to 128, next press 61 s, `systemctl --failed` empty |
| A1 clamped own values | PASS on lab 130 (fix `6bc5bdec7`): plan 512 → 256 MB / 20 → 4: screen own 384/6, clamped, effective 256/4, PHP `memory_limit` 256M, `pm.max_children` 4; unchanged form accepted, 400 refused; plan raised: 384M / 6 back. On `ef891c290` PHP still said 384M (first admin value wins in PHP-FPM) |
| A2 password change reopens | PASS on lab 131: box3 over its limit, paused in 210 s, answered with the pause (550); password change answered `sending_resumed` in 6 s; its next message 250. On lab 130 the next message still got 550 (Debian's postfwd request cache, fix `ab931ff0a`); on `ef891c290` it got the old limit's 450 (rate object, fixes `01131c8c5`, `5edae8f22`) |
| A3 self-test reads a list | PASS: list unreadable for postfwd → `OUTGOING_MAIL_SELFTEST_FAILED`, Postfix not wired, mail flows; readable → applied, verified, wired |
| A4 Git in Components | PASS: listed (tool), deploy without git → the panel's refusal naming Components, install 10 s, HTTPS deploy succeeded |
| A5 bulk limit answers | PASS: saves that change nothing on the server answer `applied: true` |
| A6 token deletion finishes by itself | Unit test only: this time the token-made domain was published and deleted at once (no pending path) |
| A7 no Postfix .db re-hash | PASS: 128 → 129 and 129 → 130 on the main: 0 native-file differences |
| Changed limit applies now (fix `01131c8c5`) | PASS on lab 130: limit 3 → 4th message 450; raised to 100 → next message 250 at once |
| Notify-only over limit | PASS: new customer on a notify plan, 31 MB of 20 MB: `disk_100` + `expiry_soon` in 10 s, site 200, uploads blocked; no suspension day on lab 130 (fix `6f184247a`; `ef891c290` showed one) |
| Expiry warning, notifications, seen | PASS: `expiry_soon` customer (no action, /account) and administrator (action); administrator list: account, usage, site_limit ×2, mail_outgoing paused + limit_reached, monitoring counts; seen round trip (account + usage routes) → 0 items |
| Domain health strip | PASS: site/certificate/mail/disk/backup codes, cust2 disk `over` |
| Activity log | PASS: customer sees its own token/domain rows; reseller sees its customer's sign-in; customer `audit-logs` 403 |
| Sessions, idle, address list | PASS: admin sees all sessions, ending one gives 401; idle 15 min → `SESSION_IDLE`; own address missing → `PANEL_ACCESS_WOULD_LOCK_OUT`; admin token from the lab network → 403 `PANEL_ACCESS_ADDRESS_NOT_ALLOWED`, from the listed address 200; recovery file → allowed, `recovery_open`; removed → refused |
| Maintenance | PASS: clock, processes; joined server reboot from its own panel: 428 without request id, 202 with, back in 29 s, no failed unit, both DNS servers answer, DNS page ok, peer ready |
| Certificate request → install | PASS: ECDSA request, lab-CA certificate installed on csr8, served issuer lab CA, second install 409 `CERTIFICATE_REQUEST_ALREADY_INSTALLED` |
| Extra SFTP logins, Debian 13 + Ubuntu 24.04 | PARTIAL: create/remove, account, jail and bind mount right; with the server's `PasswordAuthentication no` (cloud image) the login cannot sign in at all and the panel does not say so (open); with password sign-in allowed (lab drop-in): only `/files`, upload as the site's uid, `/etc` unreachable, shell refused, removed login refused. FTPS not run (vsftpd not installed) |
| Go / web | cmd/panel ok, internal/... ok, cmd/agent only the 19 known guest-only failures; web 2638/2638, boot 356.58 / 109.16 KiB |

Updates in this round, each one press, no failed unit, 0 native-file differences: main 128 → 129 (76 s), 129 → 130 (87 s), 130 → 131 (67 s; the drop-in change restarted postfwd once).

## Defects found in round 2 and fixed on fix/a128-lab

1. `6bc5bdec7` A1 follow-up: a clamped site still ran at its own value (PHP-FPM keeps the first `php_admin_value`).
2. `01131c8c5` A changed mailbox/domain/account limit waited up to an hour (postfwd keeps the old rate object across a reload); now reset with `postfwd --delrate`.
3. `5edae8f22` A newly paused mailbox heard its hourly limit (450) instead of the pause (550).
4. `6f184247a` A notify-only account over its disk limit was shown a suspension day.
5. `ab931ff0a` Debian/Ubuntu postfwd ran with `--cache=600` (Debian ARGS): a lifted pause was answered from the request cache; the drop-in adds `--cache=0`.

## Observations (open)

- Extra SFTP logins on a server with `PasswordAuthentication no` cannot sign in (keys are disabled in their block) and nothing tells the owner; needs a decision: allow password in the login's own Match block, or say so on the screen.
- A mailbox resumed once is not paused again within the same hour (r2c: three more refusals, no pause).
- 127 setup (published): the main's first Start waited 10 min on "distribution updates unread", and the mail profile step failed once ("mail_tls_sync terminal state is uncertain"); a second Start finished.
- Lab harness: get.sh password needs upper/lower/special (kit copy patched); `sftp -b` needs `-oBatchMode=no`.
