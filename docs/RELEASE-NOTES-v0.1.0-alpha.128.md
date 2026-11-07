# v0.1.0-alpha.128

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.128.tr.md)

Each site gets its own PHP settings, kept within limits that the server,
the reseller and the plan set. Plans can now warn instead of closing a site
when it goes over a limit. Outgoing mail can be limited per mailbox, domain
and hosting account. A mailbox that suddenly sends far more than usual is
paused until its password is changed. Every site gets web server settings,
error pages, a page cache and log retention. Node.js sites can have
environment variables and Git deployment, and a whole website can be
copied. Databases get export, import, copy, check and repair. Sites get
visitor statistics. Everything new is off or unchanged until someone turns
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

## Lab results

_To be filled in after the lab round._

## Known limits

- **Site settings within limits:** each supported PHP version still needs
  lab proof that applications cannot raise a limit (through `ini_set`,
  `.user.ini` or `set_time_limit`). The password check in the browser still
  uses the owner's rule. The server's answer names the stricter level.
  Limit hints from real use are planned for alpha.131.
- **Outgoing mail limits:** not yet tested end to end with a real Postfix
  on the five systems: install, reaching a limit, panel stopped, postfwd
  stopped. Restarting postfwd starts the hour's count from zero.
- **Mail:** send-only and the reply for unknown addresses were tested
  against simulated Postfix only. A real Postfix run is part of the lab
  round.
- **Website copy:** a known application's configuration is not rewritten
  yet; the copy still names the source database. PostgreSQL databases are
  not offered.
- **Git deployment:** no releases or rollback folder and no webhook yet.
- **Log retention:** only shortens what the system already keeps. Keeping
  logs longer needs a log folder move, which is not built.
- The lab round on the five systems is still open for log retention,
  copying a large site, Git deployment, web settings and database tools.
- **Not built, design notes only:**
  - **Mail autoconfiguration** (Thunderbird, Outlook): it changes the site
    template that this release also changes, and it needs certificate work.
    Planned with the alpha.129 template change.
  - **Mailbox recovery address and reset link:** this is a sign-in flow
    without a password. It waits for its own security review.
  - **More mail server settings** (smarthost, Dovecot limits,
    subaddressing, DKIM key rotation, antivirus, allow and block lists):
    only the connection limit shipped.
  - **More FTP/SFTP accounts per site:** the current mechanism cannot
    confine a login to a folder on any system. The sandbox's folder binding
    is the planned route (docs/A128-DATA-ACCESS-DESIGN.md).
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

As with every migration, alpha.127 refuses a database that has these
migrations, and going back means restoring the snapshot taken before the
update. Release sequence 127 → 128.
