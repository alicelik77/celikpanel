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

*20 November 2026, corrected 21 November 2026 after the independent review of
the release candidate · In source and component-tested; the distribution
configuration matrix checks CelikPanel's `opendkim.conf` with each release's
own OpenDKIM, asks it (with `miltertest`, Debian and Ubuntu) which domain it
signs each test message for, and runs CelikPanel's own decisions against each
release's real `postconf` and `opendkim -n`. The lab round is open (steps
below).*

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
  behind postscreen) gets its own `-o smtpd_milters=`: `main.cf`'s list as it
  is at every Panel start, the server owner's own filters included, with only
  OpenDKIM left out (joined by commas: `master.cf` takes no spaces there).
  Nothing that arrives on port 25, from outside or from a site over loopback,
  is signed by this server. The setting is removed again at the next Panel
  start or mail filter step (a component installed or removed) after
  OpenDKIM is no longer set up (its unit or its `opendkim.conf` gone) or
  `main.cf`'s list names no OpenDKIM; no component or button removes
  OpenDKIM by itself. CelikPanel recognises its own value by a record of what
  it wrote (`/var/lib/celikpanel-agent-private/mail-port25-milters.json`);
  without that record (22 November 2026), by its content: a value equal to
  `main.cf`'s list without OpenDKIM, or one made only of entries a CelikPanel
  release writes. A value written for an earlier list is then kept as the
  owner's (it leaves OpenDKIM out; a filter added later does not reach port
  25 until the owner removes it with `postconf -PX smtp/inet/smtpd_milters`
  and restarts the panel). `main.cf` keeps OpenDKIM, so submission, smtps and
  any smtpd the owner added keep it.
- OpenDKIM's address is recognised in `main.cf`'s list in every spelling
  Postfix accepts for the same endpoint (22 November 2026). Postfix takes the
  transport `inet` by exact comparison (`src/milter/milter8.c`,
  `milter8_connect`) and parses the rest with `host_port(buf, &host,
  "localhost", &port, NULL)` (`src/util/inet_connect.c`, `src/util/host_port.c`):
  `[host]:port` (host may be `IPv6:addr`), `host:port` split at the last
  colon, an empty host meaning `localhost`, or a bare port; names and
  addresses are resolved. Accepted: `inet:127.0.0.1:8891`,
  `inet:[127.0.0.1]:8891`, `inet:localhost:8891`, `inet:[localhost]:8891`,
  `inet::8891`, `inet:8891`, `inet:[IPv6:::ffff:127.0.0.1]:8891`,
  `inet:::ffff:127.0.0.1:8891`, any case of the host name, leading zeros of
  the port. Not taken for OpenDKIM: another address (`::1` is another
  endpoint), a service name instead of the number, a `unix:` socket.
- Where `main.cf`'s list names its filters through a `$variable` or a `{ }`
  group, CelikPanel cannot read it entry by entry, so it changes nothing on
  port 25 (22 November 2026). Where its own earlier value is on a port-25
  entry, that value stays in force: it leaves OpenDKIM out, but also every
  filter added to the list since; it is not removed, because port 25 would
  then follow a list that may name OpenDKIM. The state is
  `port25_variable_own`. Where there is no such value, port 25 follows the
  list and may still be signed: `port25_variable`. Problems now words each
  with the step (write the filters into `smtpd_milters` by name and restart
  the panel), never with the words of an entry the owner changed by hand.

**What keeps being signed, and by which identity.**

| Mail | Who hands it over | Signed for |
|---|---|---|
| A mail program or webmail over 587/465; a site's code or plugin with a mailbox login | the SASL login | the login's own domain |
| Out-of-office replies | Dovecot's Sieve, as `vmail`, through `sendmail` | its `From:`: the mailbox's own address, or an additional address forwarded to the mailbox from a domain of the same account, checked when the reply is saved, at every start, every hour, and right after every change of who holds a name or where an address goes (below); for the seconds between such a change and the rewrite, or while a rewrite cannot be made (Problems now), the earlier reply can still answer |
| The panel's notices (invitations, password mail) | the Agent, as `root`, through `sendmail` | `no-reply@` of the owner's main domain |
| Cron and system mail of the server | `root`, through `sendmail` | their `From:` domain (the server's own name) |
| Forwarders, catch-alls, "keep a copy and forward" | Postfix's virtual aliases, inside the queue of the message that arrived on port 25 | not signed; the original signature passes through untouched |
| A site's `mail()` or `sendmail` | refused (`authorized_submit_users = root, vmail`, alpha.125) | — |
| Mail a site hands to port 25 without a login | — | not signed |

Mail handed over on the server without a login has no login, so OpenDKIM
signs it by its `From:`. Who can make the server do that (21 November 2026):

- Customers cannot supply a Sieve script of their own: no ManageSieve listener
  is installed (Debian's and Ubuntu's `dovecot-managesieved` is not one of the
  packages CelikPanel installs), webmail has no filter plugin, the maildirs
  belong to `vmail`, and the reply's settings are fields the Agent renders,
  never script text. The only script CelikPanel writes is the out-of-office
  reply (Sieve `vacation`, `date`, `relational`).
- The reply's `From:` is the mailbox's address or the additional address the
  incoming message named (Pigeonhole's `vacation :addresses`). Since this
  correction an additional address must be forwarded to the mailbox from a
  domain of the same account; the Panel checks it from its records and the
  Agent again from the server's own alias map, and the refusal says only that
  the address is not one of the mailbox's own. Pigeonhole's
  `sieve_vacation_use_original_recipient` stays off, so the original
  recipient of a forwarded copy never becomes the `From:`. Replies saved
  before are rendered again without other addresses at the first start after
  the update; the reply's window says once which addresses were removed, and
  it is audited.
- 22 November 2026 (second review): the same check and rewrite runs right
  after, and inside, every operation that changes who holds a name or where
  an address delivers: a subscription transfer, a domain deletion (after its
  mail step), a domain added (also by an import), a forwarder or catch-all
  added, changed or removed (also by an import), a mailbox added (also by an
  import); and every hour, as a net for anything else. Each mailbox is
  checked under the Panel's mail lock with its row read again, so a
  customer's save of the same reply and the rewrite never overwrite each
  other. A rewrite the Agent cannot make (no answer, another build, Dovecot
  stopped, a refusal) turns the reply off where that can be applied (turning
  a reply off needs neither Dovecot running nor the addresses checked); the
  window then says it was turned off and that saving it turns it back on.
  Where even that cannot be applied, the foreign address stays active:
  Problems now shows one line per mailbox (`mail_autoresponder_foreign`,
  administrator, with the step: the Agent does not answer, is another
  version, or refused), the window says so, and it is tried again at every
  start and every hour until done. After a normal update the Agent is never
  the earlier build at the Panel's first start: `update.sh` installs and
  verifies both binaries, starts the Agent and waits for it before it starts
  the Panel.
- Forwarders never pass `sendmail` or OpenDKIM again: an outside message with
  a forged `From:` sent to a forwarding address is not signed here.
- Where `authorized_submit_users` is not limited to `root` and `vmail` (the
  owner's own value, or the change put back), programs of sites can send mail
  signed for any hosted domain. Signing is then not reported as in place:
  Problems now says so with the line to set, and nothing is changed in the
  owner's file.

A mailbox cannot send as an alias domain today (its envelope sender must be
its own address), so nothing of that changes.

**Arriving mail.** OpenDKIM still runs in sign-and-verify mode, but it is no
longer on port 25, so it no longer checks the signatures of arriving mail or
adds its `Authentication-Results` line to it. Rspamd, where installed, keeps
checking DKIM (and DMARC) of arriving mail for its spam score; spamass-milter
scores signatures only where SpamAssassin's DKIM plugin runs; with no spam
filter nothing on this server checks them. Nothing of CelikPanel reads the
`Authentication-Results` line.

**OpenDKIM without the option.** `SenderMacro` is compiled in only when a
distribution enables it. Measured in the packages on 20 November 2026:
Debian 11, 12 and 13 and Ubuntu's 2.11.0 builds (24.04: `2.11.0~beta2-9build4`)
have it; Arch Linux's `opendkim 2.11.0beta2-2` has neither it nor Lua. The
Agent asks the installed OpenDKIM first (`opendkim -n` on a file with only that
line); where it does not know it, CelikPanel keeps the earlier configuration,
port 25 is still closed to signing, and Problems now says that mail sent with
a mailbox's password is still signed by its `From:` there (a known limit).
An owner's `opendkim.conf` on such a system is kept and nothing is put beside
it: the server owner is never told to add a line his OpenDKIM refuses.

**Existing servers.** At every Panel start (the same mail repair step as the
Dovecot drop-in and the submission chroot), in this order:

1. The milter lists: `main.cf` first (where the lists are CelikPanel's,
   `milter_default_action = accept` with them), then the port-25 entries
   (`postconf -P`, or `-PX` to remove), then `postfix check` and a reload
   Postfix must verifiably take. If it does not, every setting written is put
   back exactly (a setting that was at Postfix's default is removed again, not
   written out) and Postfix reloaded with the earlier ones.
2. `opendkim.conf`: when it is exactly what an earlier release wrote,
   CelikPanel's new text is written beside it and checked with
   `opendkim -n`; the restart is recorded as owed; only then is the text
   renamed over the file and OpenDKIM restarted and verified. If OpenDKIM
   does not run with it, the earlier file is put back and OpenDKIM restarted
   with that. A restart recorded as owed by a start that was interrupted is
   made at the next start, even when the file is already CelikPanel's, and
   signing is not reported as in place until OpenDKIM is verified running.
   Setting up a domain's signing (`ConfigureDKIMSigning`) takes the same
   order since 22 November 2026: the tables first, then the new text beside
   the file and `opendkim -n`, the owed restart, the rename, and a restart
   that puts the earlier text back when OpenDKIM does not run with it.
   While OpenDKIM restarts, Postfix passes mail on unsigned where both lists
   are CelikPanel's (`milter_default_action = accept`, step 1). Where either
   list is the owner's, `accept` is not written, so his
   `milter_default_action` applies, or Postfix's default `tempfail`, and the
   panel's note says what it does: a sending mail server gets a temporary
   error and tries again by itself; a mail program on 587 shows its user an
   error and the person sends again.

**The owner's own settings (D-022).** An `opendkim.conf` the owner changed is
kept as he wrote it; CelikPanel's text is put beside it as
`opendkim.conf.celikpanel-pending`. Read as OpenDKIM reads it (one `Name
value` per line, names in any case, `#` a comment), a file of his that has
`SenderMacro {auth_authen}`, no other `SenderMacro` value and no `Include`
is in place: signing counts as bound, the other states are decided
normally, and the pending text is removed (alpha.143 lab, 22 November
2026). A milter list the owner wrote in `main.cf`
is kept, and port 25 follows it without OpenDKIM. His own `-o smtpd_milters`
on a port-25 entry is kept and, when it names OpenDKIM, reported. Every step
that can change OpenDKIM or the milters (the start, a component's install or
removal, the mail setup, DKIM signing) records the signing state, and an
answer that did not arrive is recorded as unknown. Problems now names the
file or setting and the server owner's concrete step; the line goes away when
a later step finds everything in place.

**Lab steps** (the pair's main server hosts mail, on Debian 13 and on Ubuntu
24.04; a receiver inside the lab shows the `From:` and `DKIM-Signature: d=` of
each message):

1. Before the update, path a: with mailbox `anna@a.test`,
   `swaks --server <main>:587 --tls --auth-user anna@a.test --from anna@a.test --header 'From: ceo@b.test' --to <receiver>`:
   the receiver shows `d=b.test`. Path b: from a site of `a.test`, connect to
   127.0.0.1:25 and send `MAIL FROM:<x@b.test>`, `From: x@b.test` to a
   forwarder of `a.test` that points to the receiver: `d=b.test`. Out-of-office
   reply on `anna@a.test` with additional address `ceo@b.test` (customer B's
   domain): send in from outside with `To: ceo@b.test`: the reply shows
   `From: ceo@b.test`, `d=b.test`.
2. The owner updates the panel from its own update screen.
3. `/etc/opendkim.conf` has the SenderMacro line; `postconf -P smtp/inet/smtpd_milters`
   names only the spam filter (or nothing); `ls /var/lib/celikpanel-agent-private/opendkim-restart-owed`
   finds nothing.
4. Path a again: `d=a.test`. A login of a domain with no DKIM key: no
   signature (not the `From:` domain). The null sender with a login: `d=a.test`.
   Path b again, to 127.0.0.1:25 and to the public address: no signature.
5. Webmail with an identity of another domain: `d=` of the login's domain. A
   panel notice: `d=` of the owner's main domain. A message forwarded from
   outside with a forged `From:` of a hosted domain: delivered, no signature
   added.
6. The reply of step 1: the reply's window says once that `ceo@b.test` was
   removed; the audit log has the row; the same message again gets a reply
   from `anna@a.test` (or none, when it does not name the mailbox). Saving
   `ceo@b.test` again is refused with the neutral sentence.
7. `postconf -e 'authorized_submit_users = static:anyone'`, restart the panel:
   Problems now shows the line to set; `mail()` from a site with
   `From: ceo@b.test` leaves with `d=b.test` (what the line says). Set it back
   to `root, vmail`, restart: the line is gone.
8. The owner adds a filter to `main.cf` `smtpd_milters` (for example
   `inet:127.0.0.1:7777`), restart the panel: `postconf -P
   smtp/inet/smtpd_milters` names the spam filter and that filter, not
   OpenDKIM; Problems now shows no port-25 line. There is no component that
   removes OpenDKIM: as root, `systemctl disable --now opendkim` and move
   `/etc/opendkim.conf` aside (`mv /etc/opendkim.conf /root/`), then restart
   the panel: no CelikPanel `-o smtpd_milters` remains. Put the file back and
   restart the panel before the next steps.
9. Kill test: stop the Agent between the `opendkim.conf` rename and the
   restart (for example `systemctl kill -s KILL celikpanel-agent` while the
   start runs, with `strace` or a breakpoint), restart the panel: OpenDKIM is
   restarted, runs with SenderMacro (`/proc/<pid>` start time after the file's
   modification time), the record is gone.
10. With an owner-kept milter list and no `milter_default_action`, send mail
    over 587 during the reconcile restart: the mail program gets a temporary
    error and the person sends again; the panel's log of that start says so.
11. On a copy of the main server: edit `/etc/opendkim.conf` by hand, restart
    the panel: the file is unchanged, `opendkim.conf.celikpanel-pending` is
    beside it and starts under `opendkim -n -x`, Problems now names the file
    and the line, mail flows. Add `SenderMacro {auth_authen}` to the owner's
    file, `systemctl restart opendkim`, restart the panel: the line goes,
    `opendkim.conf.celikpanel-pending` is gone, the owner's file is
    unchanged (`sha256sum`), and with `authorized_submit_users =
    static:anyone` set, Problems now shows that state instead.

Added 22 November 2026 (second review of alpha.143):

12. Customer A holds `a.test` and `a2.test` in two subscriptions; forwarder
    `x@a2.test` → `anna@a.test`; reply on `anna@a.test` with additional
    address `x@a2.test`. Transfer the subscription of `a2.test` to customer
    B: before the transfer answers, the reply's window says `x@a2.test` was
    removed and the audit log has the row with `subscription_transfer`; a
    message from outside to `anna@a.test` with `To: x@a2.test` gets a reply
    from `anna@a.test`, never `From: x@a2.test`.
13. The same with the `a2.test` domain deleted instead (row with
    `domain_deleted`), and with the forwarder removed (row with
    `forwarding_changed`).
14. Stop Dovecot (`systemctl stop dovecot`), set a reply on `anna@a.test`
    naming `x@a2.test` again in the database (`email_autoresponders`), restart
    the panel: the reply is turned off, its window says so and that saving
    turns it back on. Then stop the Agent (`systemctl stop celikpanel-agent`)
    with the same row, restart the panel: Problems now shows "anna@a.test:
    the out-of-office reply still answers from a foreign address" with the
    Agent step; the reply's window names `x@a2.test`. Start the
    Agent and Dovecot, wait for the hourly round (or restart the panel): the
    line goes.
15. `postconf -e 'owner_milters = inet:127.0.0.1:8891, inet:127.0.0.1:11332, inet:127.0.0.1:7777'`
    and `postconf -e 'smtpd_milters = $owner_milters'`, restart the panel:
    `postconf -P smtp/inet/smtpd_milters` still shows CelikPanel's earlier
    value; Problems now says "Port 25 skips the filters added to main.cf
    since", never "changed by hand". On a copy without an earlier value:
    nothing is written for port 25 and Problems now says arriving mail may
    still be signed. Put the list back by name, restart: the line goes.
16. `postconf -e 'smtpd_milters = inet:[127.0.0.1]:8891, inet:127.0.0.1:7777'`,
    restart the panel: `postconf -P smtp/inet/smtpd_milters` names only
    `inet:127.0.0.1:7777`; port 25 mail of path b is not signed.
17. `sha256sum /etc/opendkim.conf`; hold port 8891 (`systemctl stop opendkim`,
    then `python3 -m http.server 8891 --bind 127.0.0.1`) and add a domain
    that gets a DKIM key: the domain's signing step reports OpenDKIM not
    running, `sha256sum` is unchanged, Problems now shows the signing
    failure line. Stop the program and restart the panel: the line goes.

The PostgreSQL databases made before alpha.141 (`docs/DATABASE-USERS.md`,
second review of the fix round), on the same servers, with customer A's
old database `a_shop` (owner `a_owner`, recorded reader `a_reader`) made by
alpha.140 or earlier and watched for a day:

18. A holds the database's row: `psql -U a_owner a_shop` then `BEGIN; GRANT
    CONNECT ON DATABASE a_shop TO a_reader;` and leaves the session open.
    The administrator presses Close: the answer comes within about 30
    seconds and says the database server did not close it, or its result
    is not known; the page never shows it closed without its Undo, and
    nothing is closed while the session is open
    (`SELECT datacl FROM pg_database WHERE datname = 'a_shop'` unchanged).
    `ps -ef | grep celikpanel-pgpublic` shows no client after the answer.
    A ends the session (`ROLLBACK;`); Read again: the page says what holds
    (open, with the failure and its time); Close again: closed, with Undo.
19. Undo, then repeat step 18 with the panel's own close: wait for the
    hourly round with the session open. The PostgreSQL page shows the
    database as open with "a close was sent and its result is not confirmed
    yet" or as a failed close, never as watched again, and no second close
    is sent at the next round (`journalctl -u celikpanel` shows one).
20. As `a_owner`: `ALTER DATABASE a_shop SET default_transaction_read_only =
    on; ALTER DATABASE a_shop SET role = 'a_owner'; ALTER DATABASE a_shop SET
    lock_timeout = '1ms';`. Read again: the database is read; Close: closed;
    Undo: open again. A backup and restore of `a_shop` from the domain page
    still works.
21. Stop the cluster for four hours (or, on a copy, make `a_shop` refuse
    every connection with `ALTER DATABASE a_shop ALLOW_CONNECTIONS false`,
    undone afterwards with `true`): the
    page shows "Could not be read now" with the reason; after the fourth
    hourly round Problems now and the alert mail list it as not readable,
    with the reason and the step. Start the cluster, Read again: the line
    goes.
22. Delete a database that has a record (`SELECT key FROM panel_settings
    WHERE key LIKE 'postgresql_public_v1:%'` before and after, read with
    python3's sqlite3 module): its record goes with it.
23. (alpha.143 lab corrections) After a round closed `a_shop` by itself,
    open the PostgreSQL page without pressing Read again: it shows the close
    (by CelikPanel, its time, Undo), never the database as open, and no
    finding ends in an empty colon.
24. As the administrator, `DROP ROLE a_reader` by hand (after `REASSIGN
    OWNED`/`DROP OWNED` in `a_shop`), then press Close on the Turkish page:
    the window says nothing was changed, why in Turkish, and the database
    server's own message quoted; never "not known"; the list's failed line
    says the same.
25. Undo a close: `datacl` is the default written out
    (`{=Tc/a_owner,a_owner=CTc/a_owner}` in some order); the database is
    listed under "Opened again by an administrator" without "changed
    outside CelikPanel".
26. A reply that named one foreign address says it in the singular, one
    with two in the plural (Turkish and English).

## 7. Side doors: the database tools, webmail, the stock PHP-FPM pool, `at` and site crontabs

*23 November 2026 (sandbox slice R1, pull request #348, after three
independent reviews) · In source and component-tested; the access-control
list on webmail's tree was checked with real files, owners and access checks
on the test guest's ext4. The lab round on Debian 13 and Ubuntu 24.04 is
open (steps in the pull request).*

**The problem.** phpMyAdmin, phpPgAdmin and webmail (Roundcube) ran in the
distribution's stock PHP-FPM pool as the web server's account, whose group
reads every site's home; a fault in any of them read every site. The stock
pool stayed on even where nothing used it. A site's own code could plant
`at` jobs and crontab lines that run outside its limits, and nothing showed
them.

**What CelikPanel does once after each Panel start (the side doors step);
it then reads the states again, without changing anything, every hour and
after every move in a tool's Manage window:**

1. *Each tool on its own account.* `celikpanel-webmail`,
   `celikpanel-phpmyadmin` and `celikpanel-phppgadmin` (no login, no home, in
   no other group), each with its own pool on the PHP version it is bound to
   (`/etc/php/<v>/fpm/pool.d/celikpanel-<tool>.conf`; the socket is the web
   server's account's, 0660). The move is the verified step of the Manage
   window: own pool, nginx checked and reloaded, the tool must answer as
   itself, otherwise everything comes back. A configuration the owner
   changed is kept (D-022).
2. *Webmail's tree.* The root of `/var/lib/celikpanel-webmail` stays
   `root:<web group> 0750`, closed to others, as the install leaves it.
   Webmail's account alone gets through it, by one access-control-list
   entry, `user:celikpanel-webmail:--x`, set with `fsetxattr` on a
   descriptor opened without following any link (no dependency on the `acl`
   package). The web server reaches `public_html` (whose `plugins`, `skins`
   and `program/js` are links inside the tree) through the root's group.
   `db/`, `temp/` and `logs/` become the account's; `config/` becomes
   `root:<account> 0750`; a plugin configuration the web group read gets the
   account's group. No website account enters the tree at all. A file
   system that refuses access-control lists stops the move before anything
   changes: webmail stays on the web server's account, and Problems now says
   why and what the owner can do. Webmail's updates put the entry on the new
   tree before it is published.
3. *The stock pool* (`www.conf`) of each PHP version is turned off when it is
   the package's untouched file (dpkg's own MD5), no web server
   configuration that is loaded names its socket, and another pool remains.
   "Loaded" is nginx's effective configuration (`nginx -T`) and, only when
   the Apache program is installed, the Apache files Apache loads: every
   file under `/etc/apache2` outside the `*-available` folders, the enabled
   links, and what any of them includes. The PHP-FPM package's own
   `/etc/apache2/conf-available/php<v>-fpm.conf`, which nothing enables on a
   standard server, is not a user; an Apache include that cannot be followed
   makes every Apache file count. In what is loaded, any line outside a
   full-line comment that names the socket, a `unix:` path that reaches the
   same socket by another spelling or link (compared by device and inode) or
   cannot be followed at all, or a `*_pass` whose address is a variable,
   keeps the pool on. The one exception is CelikPanel's own reverse-proxy
   line `proxy_pass $celikpanel_upstream;` in a site file that carries
   CelikPanel's seal and is unchanged: that variable only ever holds
   `http(s)://host[:port]`. So on a standard Debian 13 or Ubuntu 24.04
   server with nginx the pool is turned off once nothing CelikPanel did not
   write points at it and a site or tool pool runs beside it; it stays on
   where Apache loads the packaged snippet, where an owner's configuration
   names the socket or a socket path that cannot be followed, or where a
   `*_pass` other than CelikPanel's own uses a variable. Turning off is a
   rename to `www.conf.off-by-celikpanel`, checked with `php-fpm -t` and a
   reload; a refusal puts it back. A version is
   never left without a pool: before its last other pool goes (a site
   deleted or moved, a tool moved or removed) the stock pool is put back.
   One host-wide lock covers every keep, write, remove, test and reload of
   a version's pools.
4. *`at`* is closed to website accounts: where the at package is installed
   and no `/etc/at.allow` exists, CelikPanel writes one listing every account
   that may use at today minus the website accounts (and every account that
   shares a website account's id). Pending at jobs of site accounts are
   listed (number, account, time), never removed.
5. *Site crontabs.* Lines not made in CelikPanel are marked on the site's
   Scheduled tasks list (Delete only; the administrator may Keep one) and
   counted in Problems now. The Agent refuses to edit or enable such a line
   until it is kept.

**What is kept on disk** (all root's, in `/var/lib/celikpanel-agent-private/`):

| File | Holds | Removed when |
|---|---|---|
| `php-tool-accounts.json` | a tool's failed move (version, reason) | the move succeeds |
| `php-tool-files-<tool>.json` | each changed path's owner, group, mode (special bits included), the root's access-control list, and what CelikPanel set | the tool answers on its own pool, or the undo is done |
| `stock-pools-off.json` | the versions CelikPanel turned off, and a turn-off begun and not finished | never as a file; a version's entry goes when it is put back |
| `at-allow.sha256` | the digest of CelikPanel's `/etc/at.allow`, or `pending <new> <earlier>` while it writes | never (it is how the owner's deletion is recognised) |
| `site-crontabs/<account>.json`, `site-crontabs/baseline-v1` | the digests of the crontab lines CelikPanel accounts for | the site account is deleted |

The Panel keeps its own records in its database: the side doors' last
answer, and the administrator's last "close it again" with its time (a run
a Panel restart cut is said as not known).

**How each step recovers.** Wherever the other order would make a power
loss look like the owner's doing, a record is written before the change it
describes: a tool's files before they change, the "turning off" mark of a
stock pool before its rename, the pending record of `at.allow` before the
file. A power loss in between then reads at the next start as CelikPanel's
unfinished step. The finished records come after the change they confirm
(a stock pool recorded off, `at.allow` recorded as CelikPanel's), and a
crontab change that adds a line writes the crontab first and its record
after, by design: a power loss in between shows the new line as not made in
CelikPanel, never hides one. A tool move cut
before nginx was switched is tried again, and its failure puts back what the
first attempt recorded; one cut after is reloaded once and asked again; a
tool that still does not answer is a Problems line (who acts, what to do).
The undo puts a path back only while it is exactly as CelikPanel set it; a
file the move itself created (phpMyAdmin's
`conf.d/celikpanel-own-account.php`) is removed while it still holds what
the move wrote; a path already gone has nothing to put back and is said so;
a path changed since is kept and named. A stock pool marked "turning off" is
finished (check and reload) or decided again; a refused turn-off renames the
pool back before the mark goes; a pool found renamed under CelikPanel's
name without its record is checked (`php-fpm -t` and a reload) before it is
recorded off. `at.allow` with a pending record is finished or written again.
A crontab change that removes a line writes the record without it first.
The hourly read takes the step's lock,
never replaces a newer answer, and calls a tool down only after two probes
in a row failed.

**How the owner undoes each thing without the panel (D-022).**

- A tool's own pool: delete `/etc/php/<v>/fpm/pool.d/celikpanel-<tool>.conf`,
  point the tool's nginx file back at the stock socket and reload both. The
  accounts are removed with `userdel`.
- Webmail's entry: `setfacl -x u:celikpanel-webmail /var/lib/celikpanel-webmail`
  (`getfacl` reads it). Webmail then cannot read its files; CelikPanel does
  not put the entry back by itself and says so; choosing webmail's PHP
  version in its Manage window puts it back.
- The stock pool: rename `www.conf.off-by-celikpanel` back to `www.conf` and
  reload PHP-FPM. CelikPanel leaves it on and lists it; only the
  administrator's explicit "Turn it off again" (that one pool only) turns it
  off again.
- `at`: delete `/etc/at.allow`; `/etc/at.deny` applies again. CelikPanel does
  not write it again by itself; "Close again" (that file only) writes it
  anew.
- A crontab line: edit the crontab as root. Lines CelikPanel did not write
  are listed, never removed by it.

**What a site account can still reach.** Nothing under webmail's tree. The
tools' sockets are the web server's (0660), and a site account is not in
that group; customers cannot write any `*_pass` directive. A customer who
names the stock socket in a harmless directive keeps that pool on and gains
nothing by it.
