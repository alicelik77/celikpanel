# v0.1.0-alpha.128

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.128.tr.md)

Each site gets its own PHP settings, kept within limits that the server,
the reseller and the plan set. Plans can now warn instead of closing a site
when it goes over a limit. Outgoing mail can be limited per mailbox, domain
and hosting account. A mailbox that suddenly sends far more than usual is
paused until its password is changed. Every site gets web server settings,
error pages, a page cache and log retention. Node.js sites can have
environment variables and Git deployment, and a whole website can be
copied. A site can give a colleague an FTP/SFTP login that sees one
folder and nothing else. Databases get export, import, copy, check and repair. Sites get
visitor statistics. Your own programs can work with an account through a
remote API with tokens. Each person gets one list of notifications, with
what needs their action first. Accounts can see and end their signed-in
sessions, and the server's maintenance page can restart the server.
Everything new is off or unchanged until someone turns
it on, so an updated server works exactly as before. **Only the owner
updates installed panels, from the panel's own update screen.**

## Site settings within limits

- **PHP per site:** memory, run time, largest upload, input fields and the
  number of PHP processes, chosen on the site's PHP page. A site can only
  choose values within the limits set by the server, the reseller and the
  plan. Applications cannot raise them either. The page says which level
  set a limit and who can raise it.
- **The owner's server limits:** 512 MB memory, 300 seconds, 256 MB upload
  and 20 PHP processes. The administrator can change them on the server's
  limits page. The administrator's own sites have no limit.
- **Applications:** an application that needs more than the limit gets the
  limit, and the site's PHP page shows what it asked for. After an
  application is installed, the site's upload size follows the
  application's size, within the limit.
- **When a limit is lowered:** sites above it keep running at the new limit.
  The customer, their reseller and the administrators each get one notice
  saying what changed and what to do next.
- **Scheduled tasks:** plans can limit how many tasks a site has and how
  often a task may run (by default at least 5 minutes apart for new tasks).
- **Database connections:** plans can limit how many connections one
  database user may hold. The limit is set inside MariaDB or PostgreSQL, so
  it keeps working without the panel. For PostgreSQL it is set on the
  user's own running cluster.
- **Who may change what:** new permissions "PHP settings", "PHP version"
  and "Additional nginx directives". Everyone who could change PHP before
  keeps that right. The administrator must offer nginx directives before
  any plan can grant them.
- **Password level:** the administrator can make the password rule
  stricter for everyone: 10 characters with a digit, or 14 with a digit.

## Plans

- **Notify only:** when a site goes over a limit, a plan can warn instead
  of closing the site or suspending the account. A full disk still stops
  uploads to protect the server. The warning level (50–95 %, default 80 %)
  can be set per plan.
- **Preview:** before you save a plan, it shows which limits go down and
  how many sites use more than a new limit.
- **Copy a plan:** opens the form already filled in, as "<name> (copy)".
- **Overselling:** a reseller plan can forbid handing out more than its
  pool still holds. By default it is allowed, as before.
- **Custom accounts:** a Custom subscription or a Custom reseller has its
  own limits in the rights form.

## Outgoing mail limits

- **Off until the administrator turns them on.** After the update every
  server sends mail exactly as before. They are turned on from the Mail
  server page after installing "Outgoing mail limits (postfwd)" from
  Components. postfwd comes from each system's own package.
- **The owner's limits:** 500 recipients per hour per mailbox, 750 per
  domain and 1000 per hosting account. A new mailbox may send 50 per hour
  for its first day. Plans can set lower limits. When a limit is reached,
  the sender is told to try again later and when sending reopens. Nothing
  is lost.
- **Security pause:** a mailbox that suddenly sends far more than usual is
  paused, and the administrator is told at once. Only a password change
  reopens it.
- **Mail keeps flowing if postfwd stops.** Only authenticated sending is
  counted; incoming mail on port 25 is not.

## Mail work

- **Bulk actions:** select mailboxes, then change their sending limit,
  empty them or delete them, with one confirmation.
- **Empty a mailbox** and **aliases shown under each mailbox**, with "Add
  an alias".
- **Largest mailbox per plan.**
- **Per-domain options:** "send only" (the domain refuses mail from other
  servers) and your own reply text for unknown addresses.
- **Delivery lookup:** what happened to a message in the last 7 days, for
  that domain only.
- **Mail server:** a limit on how many connections one sending server may
  hold at once.

## Web server settings for each site

- **Settings and additional nginx directives:** the administrator can
  always add directives. Resellers and customers can only when the
  administrator has offered them and their plan allows it. Directives on a
  forbidden list are refused. Every change is checked with `nginx -t` and
  rolled back if nginx refuses it. Edits you make on the server yourself
  are kept.
- **Custom error pages** per site.
- **Page cache** per site, with "Clear cache".
- **Log retention:** per plan or account. Only the site's old rotated log
  files are removed. The system's own log rotation is not changed.
- **Node.js:** environment variables (secret values are hidden) and package
  scripts.
- **Copy a website** to another site, with an optional MariaDB database. A
  site that already has content is backed up first.
- **Git deployment:** from an HTTPS or SSH repository, with a deploy key
  made on the server (its public half is shown once). Builds run in the
  site sandbox.

## More FTP/SFTP logins per site

- **A login for one folder:** the site's owner can give a designer or a
  colleague their own user name and password that sees only one folder of
  the site. It works over SFTP, and over FTP with TLS where vsftpd is
  CelikPanel's. A login can never open a command line, and it can be made
  read only.
- **Files stay the site's:** uploads belong to the site, so PHP can work
  with them as before.
- **Plan limit:** 10 logins per hosting account by default. A plan, a
  reseller plan or a Custom account can lower it; 0 allows none.
- **Escape tests:** 64 of 64 passed on all five supported systems. The tests
  covered other folders, other sites, links, a swapped folder, reboots,
  forwarding and plain FTP.
- **Without the panel:** each login is ordinary system configuration (a
  systemd unit, an sshd block, a vsftpd entry) and keeps working; the
  guide shows how to remove one by hand (docs/FTP-LOGINS.md).

## Databases and statistics

- **Database tools:** export, import (a safety copy is taken first), copy,
  check and repair, and size and tables, for MariaDB and PostgreSQL. An
  uploaded file is loaded as the database's own user, never as the
  administrator.
- **Restore refusals** now give the database engine's own reason instead
  of a general error.
- **Visitor statistics** per site, from the site's own access log. Finished
  days are kept. Visitors' addresses are never stored.
- **IP addresses:** the administrator can see the server's addresses
  (read only).

## Notifications, sessions and server maintenance

- **One notifications list per person** (Dashboard and My account): what
  needs your action comes first, then what is for your information. Each
  notice has its reason in plain words, a button to the fix, and "Mark as
  seen". The usage notices, the "a lowered limit brought a site's PHP
  setting down" notices and the outgoing mail notices are all in this list,
  so there is one place to look; the separate blocks on My plan, Rights and
  the Mail server page are gone. Each notice is still mailed only once, as
  before. The site's PHP page still marks a value a lowered limit brought
  down.
- **Your email choice** (My account): every notice, only what needs your
  action, or nothing, plus one extra address. The administrator can add an
  extra address per kind of event.
- **Subscription end date:** the account is warned a set number of days
  before the end date and again on the last day.
- **Signed-in sessions** (My account): every browser where you are signed
  in, with its address and browser; end one, or all the others. The
  administrator sees every account's sessions (Security), and a reseller
  sees and can end its customers' sessions.
- **Idle timeout:** a session unused for 60 minutes (15 to 480, set under
  Security, Panel access) ends at its next request.
- **Administrator addresses:** when the administrator lists addresses, the
  administrator can sign in, and use API tokens, only from them. The server
  itself is always allowed, so an SSH tunnel always works, and a local file
  opens access again (docs/PANEL-ACCESS.md). Customers and resellers are
  never affected.
- **Your own activity:** each account can read its own action log; resellers
  also see their customers'.
- **Domain health:** one card on a domain's Overview shows whether the
  website answers, the certificate, DNS, mail, disk use and the last backup,
  each with its fix.
- **Server maintenance:** restart the server (refused while another change
  is running), see the server's clock and its busiest processes.
- **Certificates on this server:** one read-only list of every certificate,
  with who uses it, days left and whether it renews by itself. You can also
  make a certificate request (CSR) for a certificate you buy; its key stays
  on the server and is never shown. When the certificate authority sends the
  certificate, install it on the site it is for: the panel first checks that
  it matches the request's key, is valid now and covers every name, then
  installs it like any uploaded certificate, and the key never leaves the
  server. The panel's own certificate and the mail server's certificate are
  renewed by Let's Encrypt, so a bought certificate is refused there with
  the reason; install it on the site and its domain's mail uses it too.

## Remote API

- **Off until the administrator turns it on** (Settings → Panel → Remote
  API). A plan must also include the new permission "Remote API".
- **Tokens** are made on My account → API tokens: read only, or read and
  change, ending after 7, 30, 90 or 365 days. Making one asks for your
  password again (and your two-factor code). The token is shown once;
  CelikPanel keeps only its hash.
- **What a token reaches:** the account's domains, DNS records, mailboxes,
  databases, backups, usage and visitor statistics, and nothing else. A
  token reaches only what the account itself may, under its plan. Deleting
  a domain needs the domain's name as confirmation.
- **Safety:** 120 requests a minute per token; repeated wrong tokens from
  one address are slowed down; a password change ends the account's
  tokens; team members cannot hold tokens. Every change made with a token
  is in the audit log.
- Guide: docs/REMOTE-API.md.

## Lab results

Two lab rounds were run on a joined pair: a Debian 13 main server with
PowerDNS and mail, and an Ubuntu 24.04 joined server with BIND. A fresh
Debian 13 server was also installed. Every server was installed with the
owner's install command, and every update was pressed once in the panel.
The lab used its own CA and Git server; no real server was touched.

- **Update 127 → 128:** one press on each server: 68–93 seconds. No
  service failed. The native configuration files of existing sites and
  mail stayed byte for byte the same (450 files compared).
- **Fresh install:** 71 seconds, no failed service.
- **Broken update:** a forced failure was rolled back by itself to 128; the
  next press worked.
- **Site settings:** values above a limit are refused with the name of
  whoever set the limit. On PHP 8.4 and 8.3, `ini_set`, `set_time_limit` and
  `.user.ini` could not raise a limit. A 140-second script stopped at the
  130-second limit. When a plan is lowered, the site runs at the plan's
  limit and its own value is kept: when the plan is raised again, the own
  value comes back.
- **Notify only:** the full-disk and expiry notices arrived within 10
  seconds; the site stayed up and uploads were blocked.
- **Web settings:** error pages, caching, headers, address blocks, extra
  nginx directives (refused ones are put back after `nginx -t`), the page
  cache, Node.js variables and Git deployment over HTTPS and SSH.
- **Outgoing mail with a real Postfix:** a mailbox over its limit got a
  temporary refusal naming the limit. An unusual spike paused the mailbox
  with a permanent refusal and notified the administrator. A password
  change reopened sending within 6 seconds. With postfwd stopped, mail kept
  flowing. Send-only and the own reply for unknown addresses work.
- **Databases:** export with a one-time download, import with a safety
  copy (a failing import is rolled back), copy, check and MariaDB repair,
  on PostgreSQL 16 and MariaDB. Visitor statistics work.
- **Remote API:** off by default; token creation asks for the password;
  scopes, other accounts' domains and the off switch are enforced.
- **Sessions and access:** the idle timeout ends a session; a panel
  address list that would lock the administrator out is refused; the
  recovery file opens access and removing it closes it again.
- **Server maintenance:** the joined server restarted and was back in 29
  seconds, with both DNS servers answering.
- **Extra SFTP logins:** created and removed on Debian 13 and Ubuntu 24.04.
  A login sees only its folder, uploads belong to the site, and it cannot
  open a command line or reach `/etc`.
- **Defects found and fixed before release (14):** every PHP page failed
  with the page cache on; outgoing limits could not turn on on
  Debian/Ubuntu, and once on applied no limit; MariaDB copy and repair
  failed; a PostgreSQL import named the wrong line; a lowered plan erased
  the site's own value; sending stayed closed for about 2 minutes after a
  password change; a changed limit waited up to an hour; a new pause showed
  the limit text; the postfwd self-test did not read a list; git could not
  be installed from Components; bulk limit saves gave unclear answers;
  unchanged Postfix maps were rebuilt at each update; a notify-only account
  was shown a suspension date.

## Known limits

- **Site settings within limits:** the lab proved on PHP 8.4 and 8.3 that
  applications cannot raise a limit (through `ini_set`, `.user.ini` or
  `set_time_limit`); other PHP versions use the same pool setting but were
  not tried. The password check in the browser still
  uses the owner's rule. The server's answer names the stricter level.
  Limit hints from real use are planned for alpha.131.
- **Outgoing mail limits:** tested with a real Postfix on Debian 13 only.
  Restarting postfwd starts the hour's count from zero. A mailbox that was
  reopened is not paused again within the same hour; its ordinary limit
  still applies.
- **Website copy:** a known application's configuration is not rewritten
  yet; the copy still names the source database. PostgreSQL databases are
  not offered.
- **Git deployment:** no releases or rollback folder and no webhook yet.
  Repositories served over plain ("dumb") HTTP cannot be deployed; smart
  HTTP and SSH work.
- **FTP/SFTP logins:** where sshd accepts keys only (common on cloud
  servers), logins use FTP with TLS, and the screen says so. If FTP with TLS
  is not installed either, the screen says why no login can sign in and
  hides Add; the administrator is pointed to the FTP server in Components.
  CelikPanel never turns on passwords in the server's SSH settings. This
  notice was checked by automatic tests, not in the lab. On Arch, the system's own sign-in rule refuses password SFTP for
  these logins, so they use FTP over TLS. On Arch, vsftpd's extra seccomp
  sandbox is turned off because it breaks TLS data connections (this
  affected every FTP account since alpha.117); chroot and privilege
  separation stay on. Where the owner runs their own vsftpd, logins use SFTP only. A login
  shares the site account's user id, so the system's own tools show it as
  the site.
- **Log retention:** only shortens what the system already keeps. Keeping
  logs longer needs a log folder move, which is not built.
- **Lab coverage:** the new features were tried on Debian 13 and Ubuntu
  24.04. Debian 12, Ubuntu 22.04 and Arch Linux were covered by the
  automatic tests only. Log retention and copying a large site were not
  tried in the lab.
- **Remote API:** deleting a domain with a token, when its DNS publication
  is still pending, is unit-tested only.
- **Not built, design notes only:**
  - **Mail autoconfiguration** (Thunderbird, Outlook): it changes the site
    template that this release also changes, and it needs certificate work.
    Planned with the alpha.129 template change.
  - **Mailbox recovery address and reset link:** this is a sign-in flow
    without a password. It waits for its own security review.
  - **More mail server settings** (smarthost, Dovecot limits,
    subaddressing, DKIM key rotation, antivirus, allow and block lists):
    only the connection limit shipped.
  - **Dedicated IP per site:** changing one site's address changes nginx's
    default site on that address for every site. It needs rendering tests
    and a lab round first.
  - **Web application firewall (ModSecurity):** there is no official
    package on Arch, and Ubuntu has it only in its community repository.
    This needs an owner decision.
- Earlier limits still apply (see alpha.127).

## Evidence

Migrations (all additive; nothing existing changes, except the new
columns' defaults on plans and subscriptions):

- **066 `site_setting_limits`:** `setting_policy`, `setting_ceilings`. On
  plans and subscriptions: `overuse_policy` ('close'),
  `notify_percent_disk` and `notify_percent_traffic` (80). On reseller plans
  and pools: `overselling` ('allowed'). The permissions `php_settings` and
  `php_version` are added to every listed rights set.
- **067 `mail_outgoing`:** `mail_outgoing_settings` (one row, off),
  `mailbox_outgoing`, `mail_plan_limits`, `mail_outgoing_hourly`,
  `mail_outgoing_notices`, `domain_mail_options`, and four mail rows in
  `setting_policy` (no limit).
- **068 `site_web_settings`:** `site_web_settings`, `node_site_env`,
  `log_retention_plans`, `log_retention_subscriptions`,
  `git_deploy_settings`.
- **069 `site_visitor_stats`:** `site_visitor_stats_daily`.
- **070 `api_tokens`:** `api_tokens` (only a hash of each token is kept).
- **071 `ftp_logins`:** `ftp_logins` (no password is kept), and the
  `ftp.max_logins` row in `setting_policy` (10).
- **072 `panel_operations`:** `notification_preferences`, `account_notices`,
  `certificate_requests`; three new empty columns on `sessions` (last use,
  address, browser); two indexes on `audit_logs`.

As with every migration, alpha.127 refuses a database that has these
migrations, and going back means restoring the snapshot taken before the
update. Release sequence 127 → 128.
