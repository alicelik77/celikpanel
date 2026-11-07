# Outgoing mail limits (alpha.128, gap G1)

Owner decisions of 7 Nov 2026: 500 recipients per hour per mailbox, 750 per
domain, 1000 per hosting account, counting recipients. An ordinary limit is a
temporary refusal (4xx) naming the limit and when sending reopens; an unusual
spike pauses the mailbox with a permanent refusal (5xx) until its password is
changed, and the administrator is told at once. postfwd from the official
package of every supported system; mail keeps flowing if postfwd is down.

## Package evidence (7 Nov 2026)

| System | Repository | Version | How it was checked |
|---|---|---|---|
| Debian 12 bookworm | main (section mail) | 1.35-8 | packages.debian.org search, file list |
| Debian 13 trixie | main (section mail) | 1.35-10 | packages.debian.org; **installed and run** in the Debian 13 development guest |
| Ubuntu 22.04 jammy | universe | 1.35-7 | packages.ubuntu.com search |
| Ubuntu 24.04 noble | universe | 1.35-8 | packages.ubuntu.com search |
| Arch Linux | extra | 2.03-3 | archlinux.org file list; package downloaded and its program run |

Ubuntu's universe has no Canonical security commitment without Ubuntu Pro; the
Mail settings screen says so.

Native layout:

- Debian/Ubuntu: `/etc/default/postfwd` (CONF=/etc/postfix/postfwd.cf,
  INET=127.0.0.1, PORT=10040, RUNAS=postfw, ARGS="--summary=600 --cache=600
  --cache-rdomain-only --cache-no-size"), unit `postfwd.service` (not enabled
  by the package; CelikPanel's drop-in adds `--cache=0 --keep_rates` after `$ARGS`:
  the request cache answered a mailbox with the pause's 550 for minutes after
  its password change, alpha.128 lab), no rule file shipped, `/usr/sbin/postfwd` is an
  alternative pointing at `postfwd2` (the prefork program).
- Arch: `/etc/postfwd/postfwd.cf` (packaged, `id=DEFAULT; action=DUNNO`,
  sha256 9810fac0…8662), units `postfwd.service` / `postfwd2.service` with the
  arguments in the unit (`--cache=600`, user `postfwd` from sysusers),
  `/usr/bin/postfwd` → `postfwd3` (2.03).

## What was measured

On the real Debian 13 package (installed in the development guest, rules
written to `/etc/postfix/postfwd.cf`, requests sent to 127.0.0.1:10040 in the
Postfix policy protocol, package removed afterwards):

- `rcpt(sasl_username/N/3600/…)` counts `recipient_count` per mailbox and
  refuses the request that passes N; refused requests do not count further.
- `sender_domain` works as the domain key.
- `set(cp_subscription=sN)` followed by `rcpt(cp_subscription/N/3600/…)`
  counts all domains of a hosting account together.
- `file:` lists match without regard to case and accept `#` comments.
- A plain `systemctl reload postfwd` clears the counters; with `--keep_rates`
  on the start line it keeps them.
- A kept rate object keeps the limit and text of the rule that made it, so a
  changed limit would wait up to an hour for it to end (alpha.128 lab). After a
  reload the Agent removes the rate objects of the mailboxes, domains and
  hosting accounts whose limit changed, and of resumed mailboxes
  (`postfwd --delrate <item>`); their hour counts again from then.
- A semicolon inside an action text splits the rule (texts avoid it).
- `postfwd -C` (postfwd2 1.35) ends with `exit 1` whatever the file holds
  (alpha.128 lab), so the check is the "Rule N:" lines it prints; `postconf -P` refuses a value
  with whitespace (`check_policy_service,inet:127.0.0.1:10040` is used).

On the Arch 2.03 program (run from the downloaded package with the same
rules): with Arch's `--cache=600` identical end-of-message requests were
answered from the request cache and not counted; with `--cache=0` they were.

## How CelikPanel wires it

- Component "Outgoing mail limits (postfwd)" (Components; requires the SMTP
  seat; never installed by a panel update). The install writes a rule file that
  only says DUNNO (Debian ships none) and a systemd drop-in
  `/etc/systemd/system/postfwd.service.d/celikpanel.conf`: Debian keeps the
  owner's `/etc/default/postfwd` and adds `--keep_rates`; Arch adds
  `--cache=0 --keep_rates`.
- The panel sends the whole wanted state; the Agent renders the rule file
  (marker + body digest) and lists under `/etc/postfix/celikpanel-postfwd/`
  (Arch: `/etc/postfwd/celikpanel/`), checks them with `postfwd -C`, reloads
  postfwd and sends two self-test requests for a name no mailbox can have (the
  self-test rule reads a list too, `selftest.domains`, so a list postfwd cannot
  read fails it; the list folder is kept 0755 because postfwd runs as its own
  account and the Agent's umask is 0027 — alpha.128 lab)
  (`celikpanel-selftest-…@selftest.invalid`): the first must pass, the second
  must be refused. Only then submission and smtps get
  `smtpd_end_of_data_restrictions=check_policy_service,inet:127.0.0.1:10040`,
  `smtpd_policy_service_default_action=DUNNO`,
  `smtpd_policy_service_timeout=10s`, and Postfix's verified reload. Port 25 is
  not touched.
- Rule order: self-test, paused mailboxes (550 5.7.1), subscription marks,
  mailbox limits, domain limits, subscription limits, DUNNO.
- A rule file, drop-in or master.cf value changed on the server is kept and
  reported (D-031). A failed check, reload or self-test puts the earlier files
  and master.cf values back; Postfix is never told to ask a postfwd that does
  not count.

## The pause

The panel reads Postfix's submission/smtps lines every five minutes (accepted
recipients per mailbox per UTC hour, refusals by a limit or the pause,
sender-address mismatches) into `mail_outgoing_hourly`. A mailbox is paused
when one hour reaches 5× its normal hour (median of its active hours of the
last 7 days) and at least 100 recipients, or collects 100 limit refusals. A
mailbox without history is judged by volume only after the panel has watched
for a day, so turning the limits on does not pause busy ordinary mailboxes.
A new mailbox sends at most 50 per hour in its first 24 hours. The pause is in
postfwd's `paused.list` (native); the only way out is a password change. The
apply that lifts the pause asks postfwd about the mailbox with a request that
counts nothing until it is answered 12 times in a row without the pause (at
most 30 s): postfwd's prefork children take a reload one by one (alpha.128 lab:
a message 70 s after the change was still refused). Only then does the answer
say sending is open again; otherwise it says the stop is lifted and sending
opens within minutes by itself. Notices: "limit reached" at most once per mailbox
per hour, "paused" at once, to the customer, the reseller and administrators,
in the panel and by mail.

## Sender check

`reject_sender_login_mismatch` with `smtpd_sender_login_maps` has been set on
submission and smtps since mail sending was set up
(`cmd/agent/mail_submission_rpc.go`), so the "who would have been refused"
preview phase does not apply; the report counts the refusals it makes.

## Resilience contract

- Invariants: owner independence (native rule file, lists, drop-in and
  master.cf values keep working with the panel removed); fail-open (DUNNO);
  detect-don't-overwrite (D-031).
- Schema: migration 067 (additive; five tables, one seeded row, triggers).
- Recovery: snapshot and put-back of every file; master.cf values put back on a
  failed Postfix reload; refusals recorded with their code; the worker retries
  a failed state after 30 minutes, a save retries at once.
- Evidence: Go tests with a fake host (render, owner file kept, check refused,
  self-test failure put back, owner master.cf kept, Postfix refusal put back,
  disable removes only CelikPanel's values), the package measurements above.
- Open: an end-to-end run with a real Postfix on the five systems (install,
  limit hit, panel stopped, postfwd stopped = mail still flows) is the lab
  round; component tests alone do not establish it. A postfwd restart starts
  the hour from zero (`--save_rates` not used).
