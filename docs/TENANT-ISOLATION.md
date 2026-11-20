# Tenant isolation: files, links, names and mail signing

*20 November 2026 · In source and component-tested. The link rule was also
run against Debian 13's own nginx 1.26.3 binary in a private test folder. The
lab round on Debian 13 and Ubuntu 24.04 with two customers is still open
(steps at the end).*

Turkish: [TENANT-ISOLATION.tr.md](TENANT-ISOLATION.tr.md).

The owner's rule of 20 November 2026: only the administrator has root. A
reseller or customer must not read, change or even list what belongs to
another customer or to the server. This page covers what CelikPanel does
about a customer reading another customer's files through the web server,
and about a site account learning which other sites are hosted.

## 1. Links in a site's folder

**The problem.** nginx's worker reads every site's files through the web
server's group (`www-data`, `nginx` or `http`): each site home is
`<site account>:<web group> 0750`. Before this change, a link a customer put
in their own folder (with a scheduled task, SFTP, PHP's `symlink()` or a
Node.js site) was followed by nginx to any file that group can read: another
site's configuration file with its database password, or the secrets of
webmail and the database tools. nginx's default is `disable_symlinks off`.

**What CelikPanel writes now.** Every server block that serves a site's files
(PHP and static sites, a Node.js site's `public/` folder, with their
password-protected folders, error pages and the certificate check location
inside them) has

```nginx
disable_symlinks if_not_owner;
```

nginx then follows a link only when the link and the file or folder it points
to have the same owner. There is no `from=` part on purpose: with
`from=<document root>` nginx does not check the document root itself, and the
site's account owns its home, so it can replace `public_html` with a link to
another site's folder. Without `from=` the whole path is checked.

Source: nginx documentation, `disable_symlinks` (ngx_http_core_module,
<https://nginx.org/en/docs/http/ngx_http_core_module.html#disable_symlinks>,
read 20 November 2026). The directive exists since nginx 1.1.15, so Debian 13
(1.26) and Ubuntu 24.04 (1.24) both have it; the distribution configuration
matrix checks the rendered file with each release's own `nginx -t`. "Parameters
`on` and `if_not_owner` add a processing overhead": each component of the path
is opened (O_PATH) and its owner compared, a few system calls per request.

Measured with Debian 13's nginx 1.26.3 binary on a private test tree (two
sites owned by different accounts):

| Request | Before | Now |
|---|---|---|
| A link in site A to a file of site B | 200, B's file | 404 (403 without `try_files`) |
| A link in site A to a root-owned shared file | 200 | 404 |
| A link in site A to A's own file | 200 | 200 |
| Site A's `public_html` replaced by a link to B's folder | 200 | 404 (403); with `from=` naming the document root's path, 200 |
| A's `public_html` is a link to another folder of A (a deployment layout) | 200 | 200 |

**What changes for a legitimate site.** A link to the site's own file or folder
keeps working. A link to a file owned by another account or by root (for
example a shared library under `/usr/share`) is no longer served: copy the
file into the site instead. Files a backup restores keep working: restore
gives the restored links to the site's account.

**How a person learns why a link stopped working (D-024).** nginx writes the
reason to the site's error log: `openat() "<path>" failed (40: Too many
levels of symbolic links)`. Hosting → Logs → Error log shows these lines with
one plain sentence above them: what the line means and what to do.

**The owner's own rule is kept (D-022).** When the server owner set
`disable_symlinks` for a site himself (a file in
`/etc/nginx/celikpanel-sites.d/<domain>/`, a site setting file he edited, or the
administrator's additional directives), CelikPanel leaves its own line out: nginx refuses the directive
twice in one block, and the owner's choice stands.

**Existing sites.** At the Panel's start after the update every site's vhost
is rendered again. A vhost that is still CelikPanel's unchanged text gets the
new line. A vhost the owner edited is kept as it is, with CelikPanel's new text
beside it in `<file>.celikpanel-pending` and the site listed in Hosting →
Configuration; the owner decides when to take it (D-031).

Webmail's and the database tools' vhosts are not changed: their folders are
package files that only root can change, so no customer can put a link there.
The validation-only blocks (`mail.<domain>`, the mail set-up names) serve only
the certificate check from a root-owned folder.

## 2. Additional nginx directives of resellers and customers

Only when the administrator offers them and the plan includes them. These are
now refused for a reseller's or customer's text (the administrator's text is
not limited):

| Directive | Why |
|---|---|
| `disable_symlinks` | it would lift the link rule above in a location |
| `autoindex`, `random_index` | nginx's documentation: they ignore `disable_symlinks`, so through a link they would list another site's folder |
| `xslt_stylesheet` | nginx's master process reads the file as root whenever it loads its configuration |
| `fastcgi_param` / `scgi_param` / `uwsgi_param` `PHP_VALUE` or `PHP_ADMIN_VALUE` | php-fpm applies them as php.ini settings for the request, so they could lift the plan's PHP limits ([php.net, FPM configuration](https://www.php.net/manual/en/install.fpm.configuration.php); php-src `sapi/fpm/fpm/fpm_main.c` reads exactly these two) |

`auth_basic_user_file` stays allowed for a file inside the site's home. The
Agent now also proves, before it writes the text, that the file is a regular
file reached from `/` without any link (openat2 with RESOLVE_NO_SYMLINKS);
before, the check was only on the path's letters. A file that becomes a link
later is refused at the next render of the site. The Protected folders feature
needs none of this.

A text saved before the update that the new rules refuse is not used any
more: at the next render CelikPanel removes its own copy of it (a copy the
owner edited is kept), and the Web server screen says which line is no longer
allowed. Nothing else of the site changes.

## 3. What a site account can learn about other sites

- **Per-site configuration files.** The vhosts, CelikPanel's include files of
  each site (`/etc/nginx/celikpanel-managed.d/<domain>/`) and the PHP-FPM pools
  are written `0640` now (root and the Agent's group, `celikpanel`), no longer
  `0644`. nginx's and PHP-FPM's master processes read their configuration as
  root. systemd units stay `0644`: systemd tells every local user a unit's
  settings over D-Bus anyway.
- **The hosting base.** `/var/www/celikpanel` and its `subscriptions` folder
  are `0711`: every account passes through them, only root lists them.
- **Additional logins (FTP/SFTP).** A name that is in use gets one answer,
  "the name … cannot be used on this server", whatever holds it (another
  subscription's login, a system account or group).

**Existing servers.** The Agent corrects modes once, at its first start after
the update, before it serves anything: CelikPanel's own unchanged files still
at `0644` become `0640`; the hosting base and the subscriptions folder at `0755`
become `0711`. Any other mode is the owner's choice and is kept, and the
Agent's journal says so (`tenant file modes: kept …`). A receipt
(`/var/lib/celikpanel-agent-private/tenant-file-modes-v1.done`) records that
it ran, so a mode the owner sets later is never changed back. Undo for an
owner who wants the old modes: `chmod 0644` on the files, `chmod 0755` on the
two folders; CelikPanel keeps a file's mode on every later rewrite.

**Still visible, and why.** `/etc/passwd` lists every site account, and the
account name is made from the domain; directory names in `/etc/nginx` and
`/var/log/nginx`, and every process with its command line in `/proc`. These
need the changes in section 4.

## 4. Open (proposals, not built)

- One group per site for nginx instead of the web server's group on every
  home, so a link is not the only thing between sites.
- `/proc` mounted with `hidepid`, with a group for monitoring.
- Each site's PHP-FPM pool and scheduled tasks in the same systemd sandbox
  as programs (per-site slice, hidden hosting tree, the outgoing-mail block,
  limits).
- Site accounts named by id rather than by domain.
- Postfix's maps (`/etc/postfix/vmailbox`, `virtual`, …) are still `0644`:
  they list every mailbox and forwarding address. Postfix's readers and the
  map type differ per release; this needs its own lab proof.

## 5. Lab steps (Debian 13 and Ubuntu 24.04, two customers A and B)

1. Before the update: A adds a scheduled task
   `ln -s <B's home>/public_html/<a file> <A's document root>/x.txt`;
   `curl https://a.test/x.txt` serves B's file. Note `stat -c '%a %n'` of
   `/etc/nginx/sites-available/*.conf`, `/etc/php/*/fpm/pool.d/*.conf`,
   `/var/www/celikpanel` and `/var/www/celikpanel/subscriptions`.
2. The owner updates the panel from its own update screen.
3. `curl https://a.test/x.txt` is refused (404); A's error log shows the
   line and the Logs screen the sentence. A link to A's own file is served.
4. A replaces its `public_html` with a link to B's `public_html`: refused.
5. Sites, PHP, webmail, phpMyAdmin/phpPgAdmin, a password-protected folder,
   a custom error page and `certbot renew --dry-run` still work.
6. The modes from step 1 are now `640`/`711`; a mode the owner had changed
   before the update is unchanged; the Agent's journal lists both.
7. A, with the directives right: `location /x/ { disable_symlinks off; }`,
   `autoindex on;`, `fastcgi_param PHP_ADMIN_VALUE "memory_limit=-1";` and an
   `auth_basic_user_file` that is a link are each refused with a plain reason.
8. A adds an additional login named like one of B's: the answer does not say
   whose it is.

## 6. Mail is signed only for the sender's own domain

*20 November 2026 · In source and component-tested; the distribution
configuration matrix checks CelikPanel's `opendkim.conf` with each release's
own OpenDKIM and, where `miltertest` is installed (Debian and Ubuntu), asks
that OpenDKIM which domain it signs each test message for. The lab round is
open (steps below).*

**The problem.** OpenDKIM signed every message whose `From:` named a hosted
domain, when the client was authenticated or connected from the server
itself, and it was on every smtpd, port 25 included. A mailbox could send
mail with another customer's address in `From:`, and a website's code could
hand a message to port 25 over loopback; either left this server with that
customer's DKIM signature, so receivers took it as genuine.

**What CelikPanel sets now.**

- `SenderMacro {auth_authen}` in `opendkim.conf`: OpenDKIM chooses the signing
  key by the SASL login, not by the `From:` header. Sources: opendkim.conf(5)
  "SenderMacro: Use the milter macro string to determine the sender of the
  message"; `opendkim.c` (2.11.0, `mlfi_eoh`): a non-empty macro value is the
  address looked up in the signing table, an empty one falls back to `From:`;
  Postfix sends `{auth_authen}` with MAIL FROM (`postconf -d
  milter_mail_macros`). Submission (587/465) requires a login, and on this
  server a login is the mailbox's full address, so submitted mail is signed
  for the login's own domain or not at all. The envelope sender
  (`{mail_addr}`) was not used: Postfix skips `reject_sender_login_mismatch`
  for the null sender (`smtpd_check.c`), and an empty value would fall back
  to `From:`.
- The smtpd that receives mail on port 25 (`smtp/inet`, or `smtpd/pass`
  behind postscreen) gets its own `-o smtpd_milters=` with the spam filter
  only. Nothing that arrives on port 25, from outside or from a site over
  loopback, is signed by this server. `main.cf` keeps OpenDKIM, so
  submission, smtps and any smtpd the owner added keep it.

**What keeps being signed.**

| Mail | Who sends it | Signed for |
|---|---|---|
| A mail program or webmail over 587/465; a site's code or plugin with a mailbox login | the SASL login | the login's own domain |
| Out-of-office replies | Dovecot, as `vmail`, through `sendmail` | the mailbox's own domain (its `From:`) |
| The panel's notices, cron and system mail | `root`, through `sendmail` | their `From:` domain, as before |
| Forwarded mail | arrives on port 25 | not signed again; the original signature passes through untouched |
| Mail a site hands to port 25 without a login | — | not signed |

`sendmail` is open only to `root` and `vmail` (`authorized_submit_users`,
alpha.125), and that mail has no login, so OpenDKIM signs it by its `From:`
as before. A mailbox cannot send as an alias domain today (its envelope sender
must be its own address), so nothing of that changes.

**OpenDKIM without the option.** `SenderMacro` is compiled in only when a
distribution enables it. Measured in the packages on 20 November 2026:
Debian 11, 12 and 13 and Ubuntu's 2.11.0 builds (24.04: `2.11.0~beta2-9build4`)
have it; Arch Linux's `opendkim 2.11.0beta2-2` has neither it nor Lua. The
Agent asks the installed OpenDKIM first (`opendkim -n` on a file with only that
line); where it does not know it, CelikPanel keeps the earlier configuration,
port 25 is still closed to signing, and Monitoring says so.

**Existing servers.** At every Panel start (the same mail repair step as the
Dovecot drop-in and the submission chroot), in this order:

1. `opendkim.conf`: when it is exactly what an earlier release wrote,
   CelikPanel's new text is written beside it and checked with
   `opendkim -n`; only then is it renamed over the file and OpenDKIM
   restarted. If OpenDKIM does not run with it, the earlier file is put back
   and OpenDKIM restarted with that. While OpenDKIM restarts, Postfix passes
   mail on unsigned (`milter_default_action = accept`): it never bounces or
   holds it.
2. The milter lists: `main.cf` first, then the port-25 entry (`postconf -P`),
   then `postfix check` and a reload Postfix must verifiably take. If it does
   not, every setting written is put back and Postfix reloaded with the
   earlier ones.

**The owner's own settings (D-022).** An `opendkim.conf` the owner changed is
kept as he wrote it; CelikPanel's text is put beside it as
`opendkim.conf.celikpanel-pending`. A milter list the owner wrote in `main.cf`,
or his own `-o smtpd_milters` on the port-25 entry, is kept. In each case, and
when a change was refused, Monitoring → Problems now names the setting and
what the server owner does; the line goes away when the next Panel start finds
everything in place.

**Lab steps** (the pair's main server hosts mail; a receiver inside the lab
shows the `DKIM-Signature: d=` of each message):

1. Before the update, path a: with mailbox `anna@a.test`,
   `swaks --server <main>:587 --tls --auth-user anna@a.test --from anna@a.test --header 'From: ceo@b.test' --to <receiver>`:
   the receiver shows `d=b.test`. Path b: from a site of `a.test`, connect to
   127.0.0.1:25 and send `MAIL FROM:<x@b.test>`, `From: x@b.test` to a
   forwarder of `a.test` that points to the receiver: `d=b.test`.
2. The owner updates the panel from its own update screen.
3. `/etc/opendkim.conf` has the SenderMacro line; `postconf -P smtp/inet/smtpd_milters`
   names only the spam filter (or nothing).
4. Path a again: `d=a.test`. Path b again: no DKIM signature.
5. Mail from a mail program (587 and 465) and from webmail: `d=` is the
   mailbox's domain. An out-of-office reply and a panel notice (an invitation
   or a password mail): signed for their `From:` domain. A message forwarded
   from outside keeps its original signature and is delivered.
6. On a copy of the main server: edit `/etc/opendkim.conf` by hand, restart
   the panel: the file is unchanged, `opendkim.conf.celikpanel-pending` is
   beside it, Monitoring shows the problem, mail flows.
