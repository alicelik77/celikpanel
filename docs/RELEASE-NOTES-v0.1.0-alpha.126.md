# v0.1.0-alpha.126

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.126.tr.md)

> **Published** on 2026-11-06 (UTC): release commit `14a90360c`, GitHub pre-release at 16:26:12Z, download portal at about 16:31Z (public check 27/27).

The panel now asks its questions in its own window and shows every confirmed
change step by step while it runs. A domain deletion on a joined server that
stayed "being deleted" for ever is fixed and finishes by itself after the
update. Monitoring shows honest CPU charts, 30 days of history and a "Problems
now" list, and the recovery watchdog is much quieter on an idle server.
**Only the owner updates installed panels, from the panel's own update
screen.**

## The panel's own confirmations and progress window

- **No more browser pop-ups.** Every "Are you sure?" question is now the
  panel's own window, in the panel's language (the browser's
  "…:1923 says" dialogue is gone everywhere). Escape or a click outside
  cancels; a second press never sends the change twice.
- **A progress window for every confirmed change.** After you confirm, the
  same window shows what is being done, step by step, with the time it has
  taken, like the update screen. The page is locked while the work runs, so
  nothing can be pressed twice. At the end it says **Completed**, or why it
  stopped, who must act and the next step (with the fix button when there is
  one). Lists update by themselves; there is no need to reload the page.
- **Domain deletion shows its real steps as they happen:** mail settings →
  website → DNS → certificates → finishing. When the DNS step waits for
  another DNS server (the backup confirming, the backup catching up, or the
  main DNS server not answering), the window says what it waits for. It keeps
  the page locked for at most 10 minutes; after that it says the deletion
  **continues by itself**, the window can be closed and the row keeps saying
  "Being deleted".
- The same toast is no longer shown twice while it is on screen.

## A stuck domain deletion on a joined server

On a joined server whose main DNS server runs PowerDNS with a BIND backup
(Frankfurt and Boston in our tests), deleting a domain removed it from DNS,
but the main server's final check never finished, so the domain stayed
"being deleted" for ever. Two faults caused it: the backup's check refused
every new attempt after PowerDNS re-stamped its list of zones, and the
backup's DNS check tool did not accept a DNS server listening more than once
on the same address. Both are fixed.

- **The stuck deletion finishes by itself after the update.** The backup's
  check tool is CelikPanel's own program, and until now no update replaced
  it. The Agent now puts the current copy in place when it starts after an
  update (only when the installed file is one CelikPanel shipped; a file
  you changed yourself is kept, and the DNS page shows the one command to
  replace it). Update the joined server first: when it restarts it sends the
  removal again. If the main DNS server is updated later, the deletion
  finishes within about five minutes after that update. Nobody needs to
  press anything.
- **Pressing delete again starts nothing.** While a domain is already being
  deleted, another press answers "This domain is already being deleted.
  Nobody needs to do anything" and runs nothing again. Before, each press
  re-ran the whole deletion and re-sent the removal to the main server.
- **The plan-limit warning names the website.** While a domain was being
  deleted, the Users page could warn "1 website(s) are not counted". A
  domain being deleted is no longer measured, and the warning now names each
  website whose traffic is not counted, says what that means and gives one
  step with an **Open** button; file paths are only under technical details.
- **Plan software lists only what an account can choose:** PHP and Node.js
  versions, Python, MariaDB, PostgreSQL, certificate authorities,
  applications and own programs. Server parts (nginx, BIND, Postfix, Dovecot,
  certbot, cron and others) are no longer offered or counted. A saved plan
  that named them keeps working; they disappear when the plan is next saved.
- **Service plans:** while a plan form is open, **Add plan** is hidden and the
  other rows cannot be edited.

## Monitoring, stage 1

- **Honest CPU charts.** Each point is the average over the last 30 seconds;
  a faint band shows the short peaks. Before, a 0.2-second snapshot could
  draw an idle server as 50–100 % spikes. The CPU card names the number of
  vCPUs.
- **1 hour, 6 hours, 24 hours, 7 days or 30 days**, each drawn with at most
  360 points. History is kept for 30 days. A fault that made a range show up
  to a day too much is fixed.
- **Problems now**, at the top of Monitoring and on the Dashboard: a stopped
  service (with **Start**), a disk 85 % full or more, CPU or memory at 90 % or
  more for 10 minutes, a long mail queue, a certificate that expires soon or
  could not be renewed, pending security updates. Each says who acts and the
  next step; what could not be checked is said as such, never as "no
  problem". The list refreshes itself every minute (paused while the tab is
  hidden).

## A quieter recovery watchdog

The recovery watchdog runs about every 30 seconds. When nothing is pending it
now finishes in about 11–15 ms of CPU instead of 0.3–0.9 s, so it no longer
shows up as CPU spikes. The full check of the recovery files still runs at
every boot and at least once an hour. If that check fails, its log line now
says that no recovery step was started, that the owner acts only if it
repeats outside an update, the command to run
(`sudo /usr/libexec/celikpanel/recovery runtime-status --lang en`), and that
it clears by itself.

## Lab results

Tested on a pair of lab servers set up like the owner's (PowerDNS main on
Debian 13, BIND backup on Ubuntu 24.04, whose BIND listens twice on each
loopback address), built from the release candidate:

- **Stuck deletion from alpha.125 finishes after the update.** On alpha.125
  a domain deletion stuck exactly as on the owner's servers. Updating the
  backup server first replaced its DNS check tool (the old copy and a record
  kept) and the deletion finished by itself about 32 seconds after the
  agent restarted. In the other order it stayed pending until the backup
  server was updated, then finished in 40 seconds. The users-page traffic
  warning disappeared.
- **Fresh deletion:** the panel's own confirm window; three clicks sent one
  request; live steps; "Completed" after 36 seconds; the list refreshed
  without reloading. A repeated request got "deletion already running" and
  only one run happened.
- **No browser dialogs:** none in any of the 347 script files the panel
  serves.
- **Plans:** a plan saved on alpha.125 with 14 software items now shows the
  5 that can be chosen. "Add plan" is hidden while the form is open.
- **Monitoring:** one fully busy core on a 2-core server reads about 50 %;
  a stopped mail service appears with a Start button and clears after it.
- **Background watchdog:** 4–11 ms of processor time per idle run. A forced
  failed update was rolled back by itself ("Rollback complete") and the next
  attempt succeeded with no failed services.
- **Update 125 → 126** succeeded on the first press on both servers in
  63–72 seconds, with no failed services.

Evidence: `deploy/e2e/dns-pair-acceptance/evidence/a126` on the lab branch
(commit `c816cb3ed`).

## Known limits

- The progress window shows real steps for domain deletion. Issuing an SSL
  certificate, restoring a backup, installing an application, changing the
  PHP version and the WordPress tools still show one "Working on the
  server…" step with the elapsed time; their own steps follow in alpha.127.
- The update's last database check still copies the live database; asking
  the running panel instead is a structural simplification planned for
  alpha.127.
- Rolling back to alpha.125 or earlier keeps the newer DNS check tool on the
  backup DNS server (it speaks the same request format; the older program is
  kept in `/var/lib/celikpanel-agent-private/dns-check-tool/`).
- The update on Ubuntu 22.04 was not run in the alpha.125 or alpha.126 lab.
- An emptied site-traffic folder left behind by a deletion stuck on alpha.125
  stays in place (it is harmless and holds nothing); new deletions remove
  theirs.
- The first-login tour can open above the progress window.
- On a server without a mail server, the system log after an update notes
  that publishing the mail certificate was skipped because Postfix is not
  installed. Nothing fails; the wording will be quieter in alpha.127.
- Earlier limits still apply: DNSSEC signing for a PowerDNS main server with
  a backup server is not implemented; site cron output is not mailed or
  kept; no AAAA (IPv6) record for the machine's own name in setup.

## Evidence

Migration 064 (`metrics_history`): one new column for CPU short peaks and a
new table for the 5-minute and 1-hour monitoring summaries; nothing existing
changes. As with every migration, alpha.125 refuses a database with it, and
going back is the pre-update snapshot restore. Release sequence 125 → 126.
Go tests (whole module) and web tests pass, and the web bundles stay within
their budgets.
