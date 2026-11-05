# v0.1.0-alpha.124

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.124.tr.md)

> **Published** on 2026-11-05 (UTC): release commit `8f782e664`, GitHub pre-release at 15:14:59Z, download portal at about 15:58Z (public check 27/27).

Installing a component now works at component level and installs what it
needs in the same operation; audit entries are never lost while the panel's
database is busy; only one Agent can ever run. **Only the owner updates
installed panels, from the panel's own update screen.**

## Installing a component

- The install window lists what the component **requires**, by component, not
  by package: each requirement is shown as installed (with its version), to
  be installed together with it, or not installable here with the reason (for
  example: mail engines come with the mail setup; not offered on this system;
  another component already has that role). While a requirement cannot be
  installed here, **Install** is off and the window says who acts and what to
  do next.
- Pressing **Install** installs the requirements first and then the component,
  on the server, as one operation. It goes on if the page is closed or the
  panel restarts, and a page opened meanwhile shows the same progress. If a
  step fails, what was already installed stays installed and the window names
  it.
- If the plan changed between opening the window and pressing Install (for
  example another administrator installed something), nothing is installed
  and the window asks to look at the list again and confirm it.
- phpMyAdmin, phpPgAdmin and Roundcube ask which installed PHP version they
  run on; the distribution's own PHP (or the newest) is chosen by default.
- Package names now appear only where the owner must consent to something: a
  repository change names the repository plainly (PostgreSQL 18 on Debian 12
  no longer says "vendor").
- Removing a PHP version that sites use is refused as before, and the refusal
  now also names phpMyAdmin, phpPgAdmin or Roundcube when they run on it, each
  with a link to its page. A PHP version that only one of these tools runs on
  is refused too: choose another PHP version on the tool's page first.

## Panel database busy

- When another program holds the panel's database for a while, audit entries
  are no longer dropped. An entry that cannot be written yet is kept, also on
  disk, and written later in its original order and with its original time.
  The audit log page says how many entries are waiting and why; nobody needs
  to act. If they still wait after about ten minutes, the page names the one
  command to run.
- A component operation now shows **waiting for the panel's database**
  instead of staying on "running", and resumes by itself. Server setup shows
  the same waiting state.

## One Agent only (the Frankfurt incident of 5 November)

**What happened.** On Frankfurt, `agent --version` was typed to read the
Agent's version. The Agent did not know that argument, ignored it and started
a second full Agent. The second Agent re-created the panel's connection point
to the Agent (its socket) with permissions the panel could not use, so for
about 40 minutes the panel could not reach the Agent and the domain's DNS tab
only said "an internal error". It was fixed by stopping the second process
and restarting the Agent.

**What prevents it now.**

- `agent --version` (and the panel's and recovery tool's) prints the version
  and stops. Any other unknown argument is refused before anything is
  started.
- The running Agent holds a lock for its whole life. A second Agent sees it,
  says how to check and restart the running one, and stops without touching
  anything.
- The socket's group is always the same, whether systemd or a person started
  the Agent.
- If the panel cannot reach the Agent, the screen says so and gives the one
  safe command: check the status, then
  `sudo systemctl restart celikpanel-agent`.

## DNS and server setup

- DNSSEC: on a pair where PowerDNS is the main server and another server is
  the backup (as on Frankfurt and Boston), signing a zone is **not supported
  yet**. The **Sign the zone** button is off with the plain reason beside it.
  Previously it was on, and pressing it gave a general internal error while
  nothing was changed. The DNSSEC card's text is also simpler.
- First server setup now adds the machine's own name (for example
  `frankfurt.example.com`) as an A record to the zone it creates. An existing
  record at that name is kept. Servers set up before this release are not
  changed; their dashboard keeps offering **Add the record**.

## Mail sent without a password (first step)

- The Postfix page says that mail sent without a password will be closed in
  the **next release**, and lists the sites that sent such mail in the last
  30 days (read from Postfix's own log). **This release changes nothing on the
  server**: those sites keep sending as before.
- Each listed site has **Create a sending mailbox**: it makes
  `noreply@<domain>` and shows once the server, port, user name and password
  to put into the site's mail settings. The list updates itself afterwards.

## Other

- The Agent clears the "failed" mark that systemd kept for its own earlier
  self-update attempts once a later attempt succeeded, so they no longer
  appear under `systemctl --failed`. Attempts that are not resolved keep it.
- Programs uploaded by users, Python apps and service apps behave on Ubuntu
  22.04 exactly as on Ubuntu 24.04; a test now holds this.

## Lab results

Four fresh installs through the owner's install command (Debian 12, Debian
13 with PowerDNS, Ubuntu 22.04 and Ubuntu 24.04 with BIND) and an update from
alpha.123 through the panel's own update screen. All seven checks pass:

- Update from alpha.123: succeeds on one press; `agent --version` prints the
  version and exits; a second Agent started by hand stops at once and the
  running Agent's socket keeps its owner.
- Setup adds the machine's own name: `host1.<zone>` resolves on all four
  systems; a joined backup server's name gets no record.
- Component window: phpPgAdmin installs PostgreSQL with it as one operation and
  survives a page reload and a panel restart; phpMyAdmin runs on the chosen
  PHP; removing that PHP names phpMyAdmin and the site; PostgreSQL 18 on
  Debian 12 names the PGDG repository.
- Panel database held for about 30 seconds: the screen shows the wait, the
  install finishes after release, and no history row is lost.
- Agent stopped or its socket refusing: 503 with the sentence that says what
  to do; it recovers after a restart.
- DNSSEC on the PowerDNS pair: refused, and the zone's serial does not move.
- Mail without a password: the sending site is listed; the sending mailbox
  works on port 587; nothing is blocked.

The round found two defects, fixed before publication: failed self-update
marks stayed visible after a successful update (now cleared once the success
is read), and a refused DNSSEC request moved the zone's serial (now refused
before the zone is touched).

## Known limits

- DNSSEC signing for a PowerDNS main server with a backup server is not
  implemented; the button stays off with its reason.
- Mail without a password is only reported in this release; closing it
  follows in alpha.125.
- Not every place that saves an operation's state waits for a busy database
  yet. Still to follow: runtime and service apps, remote backups, the
  security emergency and suspension, and the result of adding a peer
  server's host record.
- Planned for later: dependency rules for ClamAV, vsftpd and Python apps;
  usage records for Redis, Valkey and Memcached; removing a single
  PostgreSQL major version.
- AAAA (IPv6) for the machine's own name is not added in setup.
- Seen in the lab, to be fixed in alpha.125: pressing **Update** right after
  a rollback can fail once with nothing changed (pressing it again works);
  creating a mailbox that already exists answers with an internal error
  instead of "already exists"; changing PHP on a static main site answers
  with an internal error.

## Evidence

No schema change and no migration. Release sequence 123 → 124.
Go tests (whole module) and web tests pass, the web bundles stay within their
budgets. Lab evidence is kept on the lab branch (`evidence/a124`).
