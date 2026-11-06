# v0.1.0-alpha.127

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.127.tr.md)

Every confirmed change now shows the server's real steps while it runs. The
domain page no longer waits for slow DNS and mail checks: it shows the
last verified answer at once and checks again behind the screen. PostgreSQL
database changes go only to a cluster that runs, so a removed version's kept
cluster no longer breaks new databases. Every password field shares one
rule, a generator and a strength meter, and a weak password is refused the
same way everywhere. Screens tell the server's administrator the fix itself
and tell everyone else who acts. Monitoring gains alerts (in the panel and by
email), service health, website checks and the busiest processes at a
spike. Node.js sites join the site sandbox. **Only the owner updates
installed panels, from the panel's own update screen.**

## Every confirmed change shows its real steps

alpha.126 showed real steps only for domain deletion. Now the progress
window follows the server's own steps for the other confirmed changes too.

- **Certificates:** the Let's Encrypt request (checks, DNS check, preparing
  the validation, the certificate authority, verifying, installing,
  reloading, mail), certificate upload and removal.
- **Backups:** restoring (check, inspect, restore, finish) and creating a
  backup, which now also runs in the locked window.
- **PHP version, php.ini and hosting type** changes; **application install**
  and the **WordPress tools** follow their own saved status.
- **Mail encryption, DNS records** (add, change, delete), **PowerDNS
  repair**, **service start/stop/restart**, **malware** quarantine, restore
  and delete, and **database, database user and account deletion**.
- **A failed step is marked** and said as what it is: a check that refused
  is a prerequisite to meet (with who acts and the next step), a lost answer
  is "not known", never "failed".
- **Reload keeps the window.** After a reload during a certificate request,
  backup, PHP version change, application install or WordPress tool, the
  window opens again at the server's current step and says how it ended.
  Checking the step only reads: it never starts or repeats a change.
- The first-login tour now waits while a question or progress window is
  open.

## The domain page answers at once

On Boston the Databases tab waited about 10 seconds and the Overview 5–10
seconds, because each opened the full DNS pair check (and, on a joined
server, a check at the main DNS server too).

- **Screens show the last verified answer immediately.** A DNS check result
  is used for 30 seconds; after that the page shows it with its time and
  "checking again", while one check runs behind the screen. The page reads
  again by itself (only while the tab is visible, at most six times), so the
  newer answer appears without reloading. Refreshing the page never starts a
  second check, and a check that failed is never shown as the last answer.
- **Changes still use the live check.** Creating or importing a domain,
  publishing DNS and setup always ask the servers themselves; any DNS change
  makes the screens check again.
- **Faster checks.** The DNS questions to both servers, the connection
  card's lookups, mail health and mail authentication lookups now run at the
  same time instead of one after another.
- **Usage and programs.** Opening a page never measures disk use or scans
  access logs; it shows the last measurement with its time and starts one
  new measurement in the background when it is older than 15 minutes. A
  disk that was never measured is said as such, never as 0. Programs' sandbox
  level is the last verified answer; every change still asks the Agent.

## PostgreSQL: changes go to the cluster that runs

On Boston, creating a PostgreSQL database answered "internal error" twice:
PostgreSQL 16 had been removed with its data kept, so its cluster still
claimed port 5432, and PostgreSQL 18 ran on 5433. The panel sent every
statement to the cluster with no program.

- **Database changes go only to a running cluster.** A record that names a
  removed or stopped cluster is refused with its facts. An empty record is
  moved to the running cluster; a record that holds anything is never moved,
  and kept clusters are never touched.
- **When no PostgreSQL runs**, a new database is refused with the reason,
  who acts and the next step (no more "internal error").
- **A removed version's kept cluster says so honestly** on the page: data and
  settings kept, port still reserved, and how the owner removes it.
- **Backups name their cluster.** A PostgreSQL backup records its major
  version and port. A restore goes back to that database's cluster only when
  it runs and is the same major version; another major version is refused
  before anything is written, with the backup kept. Backups made before
  alpha.127 restore as before.
- **Restoring a PostgreSQL database that still has its tables works.**
  Before, every such restore stopped at "relation already exists" (and so
  did the automatic return to the safety copy). The restore now first drops
  what the database's owner owns in that database, then recreates it from
  the backup, owned by the same owner. Objects of other roles are not
  touched; a database owned by a superuser is restored as before.
- **phpPgAdmin follows the running clusters.** After it is installed, moved
  to another PHP version, or a PostgreSQL version is installed or removed,
  its server list names every running cluster ("PostgreSQL 18 (port 5433)").
  A list you edited yourself is never rewritten: the phpPgAdmin card says
  what it names, what runs and what to set.
- **A refused change is said once**, in the banner above the form with its
  fix, brought into view and announced; no notice repeats it.

## One password rule, everywhere

- **Every password field** (mailboxes, database users, panel accounts,
  invitations and reset links, team members, FTP/SFTP/SSH access, protected
  folders, service application owners) shows the rule, a strength meter and
  what is still missing, offers **Generate** and **Copy**, and keeps the
  button off until the rule is met. Fields for existing secrets only get
  show/hide.
- **The rule:** at least 6 characters (or the field's stricter minimum), an
  uppercase and a lowercase letter and a special character. The server
  checks the same rule and refuses a weak password as `PASSWORD_WEAK`,
  naming the first rule it breaks. Passwords the server generates always
  meet it.
- **The first administrator.** `get.sh` checks the password in the
  credentials file against the rule before it downloads or changes anything
  and names the unmet rule; the terminal prompt states the rule first and
  asks again on a weak password.

## The administrator gets the fix; others are told who acts

On Boston the Access tab said "ask your provider" about missing unzip/zip and
FTP, and the Mail tab only said webmail was not available — to a customer
account. Screens now say who acts for each role.

- **The administrator gets the action itself:** "Install the tools" on the
  Access tab (in the panel's own window, with locked progress steps),
  "Install webmail", "Open the webmail settings" or "Open Roundcube" on the
  webmail card, install links beside cron, database web tools, Python and
  Node.js runtimes and ClamAV signatures, and links to PHP-FPM's page, the
  Catalog management, Users or the mail setup's Repair where those are the
  fix.
- **Customers are told their provider (the server's administrator) acts**,
  and no longer get a button to a page they cannot open. An administrator
  signed in as a customer is told to use the administrator account.
- **Plans and reseller pools.** The administrator or reseller who manages an
  account sees its name and "Open <account>'s rights"; the reseller pool
  notice opens the reseller's rights in place.
- **Where CelikPanel has no install path** (automatic updates,
  unattended-upgrades, logrotate), the administrator gets the whole command
  with a Copy button.
- A PHP tool's note after an install links to the tool's own page.

## Monitoring: alerts, services, websites

- **Alerts.** Under "Problems now", a problem that lasts longer than your
  delay (5–60 minutes, default 10) becomes one alert with since when, and is
  resolved 10 minutes after it is gone. A source that could not be checked
  neither opens nor closes an alert. "Mark all as seen" clears the Dashboard
  line.
- **Email to confirmed administrators.** Alerts are emailed only to active
  administrators whose account address is confirmed (My account), through
  the server's own mail: one digest at a time, at most four an hour, the same
  alert not again within six hours. Each alert says what happened to its
  email. With the panel or server stopped there are no alerts; the page
  recommends an outside uptime checker.
- **Services and websites.** Each service shows since when it runs, restarts
  in the last hour, CPU and memory; three restarts in an hour is a problem.
  The server checks its own websites every 5 minutes (answer, down since and
  why, certificate days left).
- **More charts.** Disk read/write and network in/out; at a CPU or memory
  spike the five busiest processes are captured (only their short name and
  owner, never a command line) and shown under the chart.

## Node.js sites in the site sandbox

- **Node.js sites run in the same sandbox as other sites:** their own
  document root, one Node.js version, the site's one port, and outgoing mail
  blocked, at the level the server's system supports.
- **Install packages and build.** The Hosting tab runs `npm ci` (or
  `npm install` without a lock file) and `npm run build` inside the sandbox,
  with confirmation and a progress window.
- **Existing Node.js sites are not rewritten silently.** Components →
  Node.js shows the server's level, its gaps and the sites still to move; the
  administrator moves them in the panel's own window, one site at a time.
  Each moved running application is checked; one that stops answering is put
  back on its earlier setup and started again. Sites whose service file you
  edited yourself are kept and listed.

## Smaller fixes

- A server without a mail server no longer logs a mail certificate
  "failure" after every update; it notes once that there is nothing to
  publish.
- Empty site folders left behind by a deletion stuck on alpha.125 are
  removed when the Agent starts (only empty folders no site can use).

## Lab results

_To be filled in after the lab round._

## Known limits

- **Progress steps:** removing a team member still shows one general step.
  The changes that gained steps in this release's second round (certificate
  upload and removal, mail encryption, DNS records, PowerDNS repair,
  php.ini, hosting type, services, malware, deletions) do not reopen their
  window after a page reload. Domain deletion keeps its own step record
  from alpha.126 rather than the shared one. After a panel restart the steps
  of a change that was running are gone and its result is "not known".
- **Domain page:** a slow check's answer can be up to about 30 seconds old
  before the page checks again; the time shown says how old it is.
- **PostgreSQL:** a database in a cluster that is neither the running default
  nor its own recorded one is not moved automatically; phpPgAdmin's server
  list stays as you wrote it when you edited it yourself.
- **Administrator next step:** some notices still read the provider wording
  for every role; they are listed in docs/OPERATION-GUIDANCE.md.
- **Monitoring:** websites are checked only from the server itself (a
  check from the paired DNS server is not implemented); process capture is
  off when `/proc` hides other users' processes (hidepid).
- **Node.js sandbox:** tried only on WSL Debian 13 so far. Still open for the
  lab: the five supported systems, that a wildcard Node.js listener cannot be
  reached from outside with the egress table loaded (escape kit item 10),
  and a real Node.js application (Express, Next.js) moved by the
  administrator's action on each system.
- The update's last database check still copies the live database. Asking
  the running panel instead was not done: the check must stay the updater's
  own, independent proof (docs/UPDATE-LATE-DATABASE-PROOF.md names a safer
  direction).
- Rolling back to alpha.125 or earlier keeps the newer DNS check tool on the
  backup DNS server (it speaks the same request format).
- Earlier limits still apply: DNSSEC signing for a PowerDNS main server with
  a backup server is not implemented; site cron output is not mailed or
  kept; no AAAA (IPv6) record for the machine's own name in setup; the
  update on Ubuntu 22.04 was not run in the recent labs.

## Evidence

Migration 065 (`monitoring_alerts`): four new optional columns on
`metrics_samples` (network and disk speeds) and three new tables
(`metrics_process_snapshots`, `monitoring_alerts`, `site_uptime`); nothing
existing changes. As with every migration, alpha.126 refuses a database
with it, and going back is the pre-update snapshot restore. Release sequence
126 → 127. Go tests (whole module) and web tests pass, and the web bundles
stay within their budgets.
