# a128: alpha.128 lab acceptance round (`release/alpha128` head `215ed6cae`)

*Lab only: QEMU/KVM in WSL `archlinux`, labnet 192.0.3.x, a lab portal signed with a lab key, the
acceptance licence, the lab ACME CA (pebble), a lab Git server (git.lab.test, lab-CA certificate) and
the lab resolver. Every guest was installed through `get.sh` in a root terminal; every update was
pressed once through the panel's own update API. No real server, celikpanel.net, licence service or
real CA was contacted. Boston and Frankfurt were not touched.*

## Heads and dists

| Head | Role | Dist sha256 |
|---|---|---|
| `0c7aefe86` | v0.1.0-alpha.127 tag (lab copy), the pair's "before" | `097346b4…` |
| `a48a12e3e` | lab 128 label of the candidate `215ed6cae` (cases 1–8) | `f3c3025…` |
| `91482b386` | lab 129 label of `215ed6cae` (case 8 target) | `e97cfebe…` |
| `2e1790160` | lab 129 label of `9004d098c` (fix/a128-lab: page cache, postfwd check, DB tools) | `fe64dd2e…` |
| `6e2e1af46` | lab 130 label of `401ca0298` (fix/a128-lab: + postfwd list folder) | `1286bcd8…` |

Topology: `r8` main Debian 13 (PowerDNS main, mail, `host1.r8.test`, 192.0.3.190) + joined Ubuntu
24.04 (BIND backup, `host2.r8.test`, 192.0.3.191), installed at 127. `f8`: fresh Debian 13 at 128
(192.0.3.194), cases 7 and 8.

## Results

| # | Case | Result |
|---|---|---|
| 1 | Pair 127 → 128, Update pressed once each: joined 93 s, main 76 s; schema 65 → 70 (066–070); `systemctl --failed` empty; native files byte-identical (main 250, joined 199 files under nginx/php/postfix/dovecot/systemd: 0 differences); `postconf -n/-M/-P` identical, no policy wiring; three sites 200 | PASS |
| 2 | Customer site on plan Basic: 384 MB/120 s/64 MB/3000/6 saved; 1024 MB and 30 processes → 409 `SETTING_ABOVE_LIMIT` (set_by server); pool `php_admin_value`, `request_terminate_timeout 130s`, nginx `fastcgi_read_timeout 130s`; `ini_set`/`set_time_limit`/`.user.ini` cannot raise (8.4 and 8.3); 90 s script 200, 140 s 504 at 130 s; plan preview names "1 site(s) above", PUT needs `confirm_narrowing`, pool clamped to 256 MB/4; 6 processes → `SETTING_ABOVE_LIMIT` (set_by plan "Basic"); clamp notices; `WEB_SERVER_STOPPED` with nginx stopped, /etc/php+/etc/nginx hash unchanged | PASS (notify-only: policy saved, site 200; over-limit not reached because the disk measurement did not re-run in 10 min) |
| 3 | Index, MIME, 7-day cache, header, IP deny (lab network 403) and lab-CA issue + reissue while denied; missing error page refused; custom 404; directives: field-owned refused, `nginx -t` refusal put back, customer offer: alias/root/path/proxy_pass refused, header applied; page cache MISS/HIT/BYPASS + clear; Node env (secret hidden, not in unit files), package script, Git over HTTPS (smart HTTP) and SSH with the generated key, build on | PASS after fix (page cache 500 on the candidate); git must be installed from the terminal |
| 4 | postfwd from Components; limits on → applied, verified, wired on 587/465 only; mailbox limit 3: 4th–6th → `450 4.7.1 This mailbox reached its sending limit of 3 recipients per hour…`; 3 refusals → paused in 240 s, `550 5.7.1 Sending from this mailbox was stopped for security…`, administrator notices; password change → `sending_resumed`; postfwd stopped → mail flows; send-only 550 5.7.1; own unknown-user reply 550 5.1.1; empty mailbox; delivery lookup scoped (customer 404 on another domain) | PASS after two fixes (limits could not turn on; lists unreadable) |
| 5 | PostgreSQL 16 (joined) and MariaDB (main): export + one-time download (410 after), import with safety copy, failing import rolled back, client command refused, copy, check (+ MariaDB repair); connection limit 3 → 5 follows the plan (admin's user none); visitor statistics from the access log (customer 404 on another site); IP addresses | PASS after fix (MariaDB copy 500, repair refused on the candidate) |
| 6 | Switch off by default; create while off 403; step-up: no request id 428, no password 403 `API_TOKEN_REAUTH`; read token reads, change → 403 `API_TOKEN_SCOPE`; other account's domain 404; manage token adds a record; not-listed routes 403; cookie+token 400; non-cpk Bearer/Basic header at sign-in 200; delete without/with wrong `confirm_domain` 400, right 202; switch off → working token 403 `API_DISABLED`, made-up token 401 | PASS (as documented: a working token gets 403, not 401) |
| 7 | Fresh Debian 13 through get.sh as typed: 71 s, schema 70, `systemctl --failed` empty | PASS |
| 8 | Forced failure (128 → lab 129, unsupported panel.env key): refused after services stopped, rollback complete in 2 min to 128, panel/agent active; next press 98 s; `systemctl --failed` empty | PASS |
| + | Fix labels: pair 128 → lab 129 (71 s / 75 s) and main → lab 130 (67 s), each pressed once, `systemctl --failed` empty, no native file changed (except postmap `.db` re-hash of unchanged maps once) | PASS |

## Defects found and fixed on `fix/a128-lab`

1. **Page cache: every PHP answer 500** (`open() … pagecache/2/… (13: Permission denied)`): the Agent's umask 0027 made `/var/cache/celikpanel` and `pagecache` 0750 root:celikpanel. `b3cc2df34` makes them 0755 (and corrects 0750 at the next save). Lab 129: 0750 → 0755, EXPIRED/HIT, BYPASS with a cookie, clear → MISS/HIT.
2. **Outgoing limits could never turn on on Debian/Ubuntu**: postfwd 1.x `--showconfig` always exits 1. `5ec4bdae0` checks the printed rules. Lab 129: applied, verified, wired.
3. **No outgoing limit ever applied**: list folder 0750, postfwd (postfw) read no list; the self-test (pattern, not list) passed. `401ca0298` keeps the folder 0755 at every apply. Lab 130: 0750 → 0755, 450 on the 4th message, pause, 550.
4. **MariaDB copy 500 and repair refused** (engine record "mariadb", tools compared "mysql"). `9004d098c`. Lab 129: copy and repair succeed.
5. **PostgreSQL import named line 1 for a refusal on line 2** (batched statements). `e8bc84913` names the range.

## Observations (not fixed)

- `git` is not a Components entry; Git deploy says "the administrator installs the distribution's git package", which needs a terminal (D-024 gap).
- After a password change the answer says `sending_resumed` at once; the next message 70 s later was still refused 550 by postfwd's children, the one after (~2 min) passed.
- The self-test does not read a list (would have caught defect 3 by itself).
- Bulk mailbox limits (the screen's loop): `called false, applied false` for quick successive saves; the worker applies later.
- A plan clamp rewrites the site's own value (384 → 256): `clamped` is then false and a raised ceiling does not bring 384 back.
- A domain made with a token (and `bsite` on the joined server, already on 127) answered 409 `DNS_PUBLICATION_PENDING`; the token's delete stopped at DNS clean-up and needs "Retry this deletion" in the panel.
- Notify-only over-limit behaviour not established (disk measurement interval).
- Postfix `.db` maps re-hashed (sources unchanged) at the 128 → 129 update.
- Dumb-HTTP Git repositories cannot be deployed (shallow clone); smart HTTP and SSH work.
- Lab harness: `getsh_install.py` needed a password with upper/lower/special (127 rule) — kit copy patched.
