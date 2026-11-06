# v0.1.0-alpha.125

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.125.tr.md)

> **Published** on 2026-11-06 (UTC): release commit `ed6086469`, GitHub pre-release at 03:00:52Z, download portal at about 03:05Z (public check 27/27).

Mail without a password is now turned off: sites send mail only with a
mailbox's user name and password. The panel is faster, an update no longer
fails on a timing race at its last database check, three lab findings from
alpha.124 are fixed, and two update faults found in this release's lab are
fixed (one stopped every update on Debian 12). **Only the owner updates installed panels, from the
panel's own update screen.**

## Mail without a password is turned off

**What is closed.** On the first panel start after this update, the server
stops two ways of sending mail without a password, for every site, PHP
version and script at once:

- **PHP `mail()` and the `sendmail` command** (also used by site cron jobs and
  scripts). Only `root` (system notices: cron, automatic updates, Fail2ban)
  and `vmail` (mailbox out-of-office replies) may still use it. For a site,
  PHP `mail()` now returns false and the mail log says
  `User <site user> is not allowed to submit mail`.
- **SMTP to the server itself without a password** (port 25 on 127.0.0.1).
  Mail into this server's own mailboxes is still delivered; mail to other
  servers needs a mailbox's user name and password ("Relay access denied").

This is done with two ordinary Postfix settings in `/etc/postfix/main.cf`
(`authorized_submit_users = root, vmail`, and `smtpd_relay_restrictions`
without `permit_mynetworks`), so it keeps working without CelikPanel. Postfix
is checked and reloaded; if it does not accept the change, the earlier
settings are put back, mail keeps working as before, the Postfix page says so,
and CelikPanel tries again at the next panel start. If you set one of these two
settings yourself, CelikPanel keeps your value and the Postfix page shows it
next to CelikPanel's value: you decide.

**Site cron jobs.** A site's cron output can no longer be mailed (that used
the closed `sendmail` command). Site crontabs therefore get `MAILTO=""` at the
top: once for existing crontabs, and whenever the panel writes one. A crontab
that sets its own `MAILTO` is kept as it is and named on the Postfix page;
root's crontab is never changed. A job whose output matters writes it to a
file.

**The panel's own mail** (password resets, invitations, notices) is handed to
Postfix by the Agent and keeps working.

**What to do if a site still sends without a password.** The Postfix page
lists every site that sent or tried to send mail without a password in the
last 30 days (from Postfix's own log), as in alpha.124. For each listed site:

1. Press **Create a sending mailbox**. It makes `noreply@<domain>` and shows
   once the server, port (587 with STARTTLS or 465 with SSL/TLS), user name,
   From address and password.
2. Enter these in the site's mail settings (for example its SMTP plugin). The
   site must send from that address.

Senders that are not sites in this panel are listed separately; the server
owner moves them to a mailbox with a password. On both test servers (Boston
and Frankfurt) no site sent mail without a password in the last 30 days, so
nothing there needs to change.

## Updates

- At its last database check (after the new panel has started) an update
  could fail when the new panel wrote to its database at the same moment;
  release recovery then finished the update anyway, but the screen showed a
  failure without a reason. That check now reads again when, and only when,
  the database was being written while it was read; every other problem
  stops at once as before. A refused check now shows its reason. Rolling
  back has the same fix.
- Pressing **Update** right after an automatic rollback could fail once
  (pressing it again worked): start-up mail work of the panel was still
  changing the server. The update's first server check now waits up to a
  minute for a change that is finishing, and the step says that it waits and
  that nobody needs to act. The panel's start-up mail work waits while an
  update it accepted is queued or running.
- The "failed" mark of a self-update attempt that release recovery finished
  after the Agent had started is now cleared too, so it no longer stays under
  `systemctl --failed`.
- An update pressed while the server's own package work was running (for
  example Debian's nightly package tasks just after midnight) could fail at
  once with "independent recovery runtime preparation is unconfirmed"
  (nothing had changed; pressing again later worked). Preparing the new
  recovery program now waits up to a minute for that work, like the update's
  other read-only checks.
- On Debian 12 (and Ubuntu 22.04, which has the same older systemd) every
  update stopped after the panel had already been stopped, while reading the
  Certbot renewal timers, and release recovery then could not roll it back:
  the panel stayed down. The timer that does not exist on these systems is now
  read correctly, and a refusal at that step can no longer leave a record that
  recovery does not accept. This fault dates from alpha.92; Debian 13 and
  Ubuntu 24.04 were never affected.

## A faster panel

- The Components page, hosting capabilities and server setup read the
  server's packages and services with a few batched commands instead of
  dozens, and keep the result briefly while nothing changed on the server (a
  change made outside the panel is seen at once). In the lab the Components
  check went from about 1.1 s to about 0.15 s.
- The Components page stays usable while its check runs in the background.
  Starting or stopping a component shows the new state at once.
- The panel's files and larger answers are sent compressed (gzip) to the
  browser. Answers that may carry a password, key or one-time code are never
  compressed. The panel compresses each file itself the first time it is
  asked for, so the release package stays the same on every build machine.
- Pages that refresh themselves (Dashboard, Components, Security, Monitoring,
  a domain's log) pause while the browser tab is hidden and read once when it
  is shown again.

## Clearer refusals

- Creating a mailbox that already exists now says so, that nothing was
  changed, and what to do (use it, set a new password from the list, or choose
  another name), instead of an internal error.
- Changing the PHP version or the PHP pool settings of a site that does not
  run PHP (for example the static main site that setup creates) now says the
  site has no PHP, that nothing was changed, and opens its Hosting tab, where
  its hosting type is changed. Before, it was an internal error.

## Lab results

Lab round on 6 November 2026 with fresh virtual servers (Debian 13 main with
PowerDNS and mail, Ubuntu 24.04 backup DNS server, a Debian 12 server and a
second mail server), each updated with the **Update** button:

- **Update 124 → 125:** the Ubuntu backup updated on the first press. The
  Debian main failed once because Debian's midnight package task was running
  (the fault above); after the fix it updated on the first press, even with a
  package task started on purpose at that moment. Old failed marks were
  cleared; `systemctl --failed` was empty.
- **Update right after a rollback:** pressed one second after a rollback, the
  update waited for the start-up mail work and succeeded, with no duplicate
  operation. On Debian 12 it found the Certbot timer fault above; after the
  fix Debian 12 updated on the first press.
- **Faster panel:** on the lab servers the Components check went from
  1.8–2.2 s to 0.6–0.9 s and server setup's component answer from 0.8–1.3 s to
  under 0.1 s, with the same results. A package installed with `apt` outside
  the panel appeared after 1.4 s. Files and answers were compressed; answers
  that may carry secrets were not.
- **Mail without a password:** every item checked, including delivery with a
  mailbox password to a second server.
- **Clearer refusals:** both refusals came back as described, in English and
  Turkish.

## Known limits

- Site cron output is no longer mailed and the panel does not keep it
  elsewhere.
- A Debian 12 or Ubuntu 22.04 server whose panel was already left stopped by
  the Certbot timer fault above is not repaired by this release, because it
  cannot reach it; its owner's recovery path is still needed. The update on
  Ubuntu 22.04 itself was not run in the lab.
- The update's last database check still copies the live database. Asking
  the running panel instead is a structural simplification planned for
  alpha.126.
- Monitoring stage 1 and a quick path for the recovery watchdog when nothing
  is happening are planned for alpha.126.
- The Node.js site sandbox moves to alpha.127.
- Earlier limits still apply: DNSSEC signing for a PowerDNS main server with a
  backup server is not implemented; not every place that saves an operation's
  state waits for a busy database yet; no AAAA (IPv6) record for the machine's
  own name in setup.

## Evidence

No schema change and no migration. Release sequence 124 → 125.
Go tests (whole module) and web tests pass, the web bundles stay within their
budgets. Lab evidence: `deploy/e2e/dns-pair-acceptance/evidence/a125` on the
lab branch.
