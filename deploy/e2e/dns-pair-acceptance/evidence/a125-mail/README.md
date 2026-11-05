# a125-mail — no mail without a password (D-046), phase 2 lab check

*2026-11-05 · lab only · branch `feat/a125-no-passwordless-mail`, Agent code of commit 7d652d893 (phase 2)*

**Machine.** One throwaway Debian 13 VM (QEMU/KVM in the WSL `archlinux` guest), an
overlay on the ready base image `/var/tmp/cp-base-images/debian13-base.qcow2`, user-mode
network. The base image's third-party PHP repository was removed first; everything came
from Debian 13's own archive: Postfix 3.10.13, Dovecot 2.4.1-4, PHP 8.4.27 (`php-cli`),
swaks. The server was shaped like a CelikPanel mail server (Debian's stock `main.cf`,
vmail 5000 virtual mailboxes for `shop.test`, Dovecot SASL on submission 587 with
STARTTLS required, site user `shop_test`). `outside.test` stands for the Internet: a
transport map sends it to a small SMTP sink on 127.0.0.1:2526 that keeps each message.
No CelikPanel process ran on the VM: the closing step and the report are the Agent's own
functions run from the `cmd/agent` test binary (`TestLabPasswordlessMail`, skipped unless
`CELIKPANEL_LAB_PASSWORDLESS` is set), so the result also shows the change needs no
panel to keep working. The VM was stopped and its overlay deleted afterwards.

**Result: pass.**

| Path | Before | After |
|---|---|---|
| Site PHP `mail()` as `shop_test` | `bool(true)`, reached the outside sink | `bool(false)`; stderr and mail log: `fatal: User shop_test(5001) is not allowed to submit mail` |
| SMTP to 127.0.0.1:25, no password, to another server | `250 queued`, reached the sink | `454 4.7.1 <relay@outside.test>: Relay access denied` |
| SMTP to 127.0.0.1:25, no password, into this server's own mailbox | delivered | delivered (inbound mail is unchanged) |
| Submission 587 with the mailbox password | `235`, `250 queued`, reached the sink | same |
| System notice as root (`sendmail`) | reached the sink | reached the sink |
| `vmail` (`sendmail`; Dovecot Sieve out-of-office replies) | reached the sink | reached the sink |
| Panel account mail (`Agent.SendAccountMailV1`, root `sendmail`) | — | `sent=true`, reached the sink |

**Closing step** (`4-close.txt`, `5-maincf-after.txt`): one run wrote
`authorized_submit_users = root, vmail` and `smtpd_relay_restrictions =
permit_sasl_authenticated, defer_unauth_destination` (from Debian's
`permit_mynetworks permit_sasl_authenticated defer_unauth_destination`), passed
`postfix check` and a verified reload, and recorded before/after in
`/var/lib/celikpanel-agent-private/mail-passwordless.json`. A second run changed
nothing and reloaded nothing. After the owner set `authorized_submit_users =
static:anyone`, the step kept the owner's value and reported state `owner` with
CelikPanel's form beside it (`10-idempotent-and-owner.txt`).

**Report** (`3-report-before.json`, `8-report-after.json`): before, state `open`, the
site account `shop_test` and the loopback sender `contact@shop.test` listed, root and
vmail marked system; after, state `closed` and the two refusals counted
(`refused: 1` each), matching the journal lines in `9-journal-refusals.txt`.

**Not covered here.** The rollback after a failed `postfix check` or reload is covered
by Go tests only (`TestPasswordlessCloseRollsBackWhenPostfixRefuses`). The panel screen
and the "Create a sending mailbox" button were not driven in this VM (no panel
installed); they are covered by Go and web tests. Debian 12, Ubuntu and Arch were not
run; Arch's `main.cf` sets neither setting, which the Go tests cover with Postfix's
defaults.

Files: `g_setup.sh` (guest setup), `g_probe.sh` (the paths), `run.sh` (driver),
numbered outputs in order.
