# v0.1.0-alpha.83 (published 2026-10-11)

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.83.tr.md)

This is the release that follows v0.1.0-alpha.82. It was published on
2026-10-11 by the owner (see "Publication record" at the end). The owner
decided that its number is v0.1.0-alpha.83. At the time of this record no
installed server has been updated to it: the owner of each starts that from the
panel's own update screen, and the outcome will be recorded separately. These
notes were written as a draft on 2026-10-11 and amended for the publication on
the same day.

It does two things. It stops CelikPanel from overwriting a site configuration
file you changed by hand: the file is kept, the site says so, and you choose
what happens to it. And it answers the four things the owner saw on the two
installed servers after the update to v0.1.0-alpha.82 on 2026-10-10 (listed
below under "The four observations after the last update").

What the server does with site files rests on component tests and on four lab
measurements: one of the first part of this work, on Debian 13, Ubuntu 24.04
and Arch; one of the corrections that followed it (the certificate validation
check against a real nginx, the locked file's page, the include directories),
on Debian 13 and Ubuntu 24.04, with Arch for only the include directories, the
locked file and one validation case; one of the pool, the application unit and
the update from v0.1.0-alpha.82, on all three; and a short round, on Debian 13
and Ubuntu 24.04 only, of the corrections the third one led to. The measured
updates started from v0.1.0-alpha.81 and
alpha.82 rebuilt from their tags' source as test-licence builds, not from the
published archives. The last changes, made after the short round, have
component tests only. What a screen shows rests on component tests and on a
browser against a mock of the API, and, for the first page load, the access
wait, the update notice and the Configuration file page, also on a real Chrome
against a real Panel in the lab. The limits say what was never run.

## What changes for the server owner

**After the update, reload every CelikPanel page that was open.** A page loaded
before the update does not have the new Configuration file page, ignores the
new update phase and draws the old waits. The new routes of this release
refuse a request without a request identity with `428`, and a refused request
changes nothing.

**During the update you still see the previous release's page.** The page that
is open while the update runs is v0.1.0-alpha.82's interface. Its layer over the
page may name the licence while the Panel restarts (seen in the lab with that
interface), and its corner notice may still say the update is being applied
while the card already shows the new version (reported by the owner after the
last update). This release corrects both; the corrections show
in an update started from a page of this release. After a successful update the
page reloads itself into the new interface.

**A site configuration file you changed is kept.** Until now every Panel start
and restart wrote every hosted site's nginx file again from CelikPanel's
records, and a saved setting wrote that site's file again, without looking at
what was there: an added `location`, a changed directive, your own include or a
whole replacement file was gone, and a file you had removed came back. This was
measured on v0.1.0-alpha.82 (its tag's source built as a test-licence build) on
Debian 13, Ubuntu 24.04 and Arch. Now every file CelikPanel writes for a site
starts with a line that carries the digest of the rest of the file:

```
# celikpanel-render v2 sha256=<64 hexadecimal characters>
```

Before it writes, CelikPanel compares the file with that line. Only its own
unchanged text is replaced, and only when the new text differs; an identical
file is not written and nginx is not reloaded for it. A file that was changed,
replaced, removed, locked or cannot be read is left exactly as it is, and the
operation that wanted to change it says so instead of reporting success.
CelikPanel's own text is held beside the file as `<file>.celikpanel-pending`,
which nginx does not read.

**The domain's page says what it found, and you choose.** Domain → Hosting →
Configuration file (administrators only). The Domains list marks the domain
("Configuration file: your choice needed", "Configuration file missing",
"Configuration file cannot be read", "Configuration file cannot be replaced"),
and a line above the domain's tabs points to the page. The page shows the
difference between your file and CelikPanel's text, with values that look like
credentials hidden, and offers three choices:

- *Keep mine.* Your file stays byte for byte, its first line included.
  CelikPanel records the choice and asks again only when the file changes.
  Changes you make to the site in CelikPanel (settings, certificates, hosting
  type, PHP version) are not applied to that file.
- *Take CelikPanel's.* Your file is first kept as a dated copy beside it
  (`<file>.celikpanel-backup-<date and time>`), then CelikPanel's text replaces
  it and nginx is reloaded. If nginx refuses the text, your file is put back;
  the dated copy stays.
- *Merge by hand.* The Panel does nothing. CelikPanel's text is in the
  `.celikpanel-pending` file; you edit your file on the server, check and reload
  nginx yourself, and reload the page. Copying the pending file over yours hands
  the file back to CelikPanel.

A file you removed is not recreated by a start or a saved setting; the page
offers "Recreate". A file locked against changes (for example with
`chattr +i`) is named as such, with the system error and the protection the
Panel found, and the text CelikPanel wanted to write. To let CelikPanel update
it, remove the protection (for example `sudo chattr -i <path>`) and choose
"Check again"; the page then offers "Write CelikPanel's text now", and the next
start or saved change would write it as well. To keep it locked, do nothing:
CelikPanel does not change it. A "Take CelikPanel's" that a protection refuses
leaves no dated copy behind.

**What you see after the first start of this release.** Files written by
v0.1.0-alpha.81 or alpha.82 have no such first line. At the first start
CelikPanel compares each of them byte for byte with the texts those two releases
wrote for the same site (the text written when the site was created, and the
text written at a later start or save). A file equal to one of them is
CelikPanel's own earlier text, so nothing of yours can be in it: it is written
again as this release's text, with the first line. Any other file is left
exactly as it is, shown as "not recognised as CelikPanel's", and never written.
As an administrator you see on the Domains list:

> Site configuration files: N recognised as CelikPanel’s and taken over; M left
> alone because they differ from every known CelikPanel text. Open a domain to
> see its file. Nothing is needed unless you want CelikPanel to manage a
> left-alone file again.

A file that was left alone need not be one you edited. A site whose certificate
path, aliases, PHP version or names changed after the earlier release last wrote
its file can also differ from every known text; that errs on the safe side, and
"Take CelikPanel's" resolves it. The line goes away once you have chosen "Keep
mine" for, or taken CelikPanel's text into, every file that was left alone.

**Certificates of a site whose file is kept.** CelikPanel now publishes the
certificate validation through a directory of its own,
`/etc/nginx/celikpanel-managed.d/<domain>/`, which its text includes; it no
longer writes the validation into the site's file. Before it requests or renews
a certificate for a kept file, CelikPanel checks whether nginx on this server
serves its validation under every name of the certificate (a request from the
server itself to port 80, on each address your file's port-80 blocks listen on
for that name, or the loopback for a wildcard). If it does, the certificate is obtained
without touching your file, but the site keeps serving the certificate your file
names until you take CelikPanel's text or change the two certificate lines
yourself; the SSL tab and the dashboard say so, with the days left on the
certificate in use. If it does not, nothing is requested: a request from the SSL
tab is refused with the line to add, and a renewal waits for you instead of
failing. If nginx does not answer that check, the result is "not known", never
"not ready". If your file makes the site listen on an address this server cannot
reach from itself, the check cannot run there: the request goes ahead, the
answer says the check could not run and names the address, and the certificate
authority's own attempt decides; nothing asks again by itself. A file kept from
v0.1.0-alpha.81 or alpha.82 has no line that includes the new directory;
whether it still lets the validation run is decided by that check. The check
was measured against a real nginx in the lab (see "What was measured"); no
certificate was issued there, and the choice of addresses came after that
measurement. Watch the dashboard: a certificate that waits for your choice is
listed first.

**The PHP-FPM pool and the application unit follow the same rule.** A PHP
site's pool file and a Node site's application unit get the same kind of first
line (`; celikpanel-render v2 sha256=…` in the pool, `# celikpanel-render v2
sha256=…` in the unit). A pool save, a PHP version switch or a hosting change
on a file you edited is refused, and your file stays in service. v0.1.0-alpha.82
dropped pool lines it had not written itself when the pool was saved through the
Panel (measured); now a pool with your lines is kept and shown. PHP-FPM has no
separate place for a pool's own settings, so your lines belong in the pool file
itself. An application's own settings belong in its systemd drop-in directory,
`/etc/systemd/system/celikapp-<N>.service.d/*.conf`, followed by
`sudo systemctl daemon-reload`; CelikPanel never writes or removes that
directory. A unit written by this release starts the application's command
with `/bin/sh -c` and an explicit `PATH` that names the Panel's Node directory,
no longer through a login shell, so `/etc/profile` no longer applies to it;
what your application needs from its environment belongs in the drop-in. A pool
or unit that is byte for byte what alpha.81 or alpha.82 wrote is recognised and
written as this release's text at its next write. These files are not looked at
when the Panel starts, and a pool or unit appears on the Domains list only after
its domain's Configuration file page was opened or after it was saved, switched
or changed; so the Domains list's count of files left alone can grow when you
open your domains. This part was measured in the lab on three platforms; two
findings were corrected afterwards and measured again on Debian 13 and Ubuntu
24.04 (see the next paragraph and "What was measured").

**Your own nginx lines have a place that CelikPanel never touches.** Each site's
file includes `/etc/nginx/celikpanel-sites.d/<domain>/*.conf`. Put your own
directives for that site in a `.conf` file there. CelikPanel never writes,
changes or removes anything in that directory, also not when the site is
deleted. The include line comes after the site's `location` blocks, so a file
there that repeats one of them (for example `location /`) makes `nginx -t`
refuse the whole configuration (nginx's own rule; not measured with
CelikPanel). The other directory, `/etc/nginx/celikpanel-managed.d/<domain>/`,
is CelikPanel's: if you edit a site's file, keep the line that includes it.

**Starts are quieter.** A Panel start that finds every site file unchanged
writes nothing and does not reload nginx. A new site's file is now the same text
a later start writes (the `www.` name is there from creation). Each write logs
one line naming the site and what was done, and the start line counts written,
unchanged, kept, missing, unreadable and failed files; you can read them with
`sudo journalctl -u celikpanel-panel | grep "site configuration"`.

**A locked file no longer stops the other sites.** In v0.1.0-alpha.82 a site
file locked with `chattr +i` made the whole start fail, the site's enabled link
was gone afterwards, and the next nginx reload took that site out of service,
with nothing shown (measured). Now only that file is refused; the other sites
are written with one check and one reload, and the link stays. The page names
the file as one CelikPanel cannot replace, with the system error and the
protection, and "Check again" only reads (both measured in the lab).

**A refused change does not take a service down.** When nginx, PHP-FPM or
systemd refuses CelikPanel's text, CelikPanel puts your previous file back.
Before it reloads the service with that file, it now runs the service's own
check (`nginx -t`, `php-fpm -t`, or `systemd-analyze verify` for an application
unit). If the check refuses the configuration because of another file, the
service is not reloaded and keeps running with the configuration it loaded
before; the answer says so and, for an administrator, names the line the check
printed. Fix that file, reload the service yourself, then choose again if you
still want CelikPanel's text. A service that was not running before is neither
reloaded nor started, and the answer tells you to start it (run in the lab with
PHP-FPM only; for nginx and an application it has component tests). The lab
measured the earlier behaviour on all three platforms (PHP-FPM reloaded with a
refused configuration ended `failed` while the answer said it kept running) and
then the correction, on Debian 13 and Ubuntu 24.04: PHP-FPM and nginx kept
their running process, and the application was left unrestarted by a refused or
unavailable unit check (there the lab had to force the put-back). A pool
or unit that CelikPanel could not read is shown on the Domains list as "A
configuration file could not be read; state not known" until a later read
answers (also measured).

**The first page load draws nothing until there is something to say.** A full
load of a CelikPanel page showed for a moment a full-page "Checking panel
access" with "Reload CelikPanel" before anything had answered. Now the page
stays empty for the first 1.5 seconds. If access is still being checked after
that, it says the Panel has not answered yet and that you need not do anything;
after 15 seconds it offers "Check now"; after 30 seconds also "Reload
CelikPanel". An answer that does not confirm access (no session, an error
status, the Panel starting), or a connection that failed, is shown at once, with
its cause. A check that reaches its own time limit is shown as still unknown,
never as a failure.

**The layer over the page names what it read.** When the Panel cannot confirm
access for a moment, the page stays under a layer, as in v0.1.0-alpha.82. The
layer now says which of these it read: the Panel did not answer; the Panel is
starting; the licence result could not be read; or your session could not be
confirmed. When the Panel did not answer, an update was started from the same
browser within the last 30 minutes and its end has not been seen there, it
says the Panel is not answering and an update may be restarting it, and that
nothing about the licence has been decided.

**The update notice and the card agree.** While the new version already runs
but the updater is still checking it, the notice and the card both say "Update
installed, being verified" and that you do not need to do anything. A state that
could not be read is said to be not known, with "do not start another update".
The time in the update window is your local time with its zone, no longer
labelled "UTC".

**A stopped setup says why, above its steps.** A setup run that stopped because
its plan was reopened for editing is no longer shown as a failure whose last
step is "In progress". The reason, who acts and the next step stand above the
list of steps, unfolded. The mail identity check names the condition that is
not met: the server's address is not public, the mail service uses another name,
the reverse DNS (PTR) of the address names something else, that name does not
lead back to the address, or the mail host name is not a full host name. A
reverse DNS lookup that got no answer is "could not be checked", no longer "no
reverse DNS". Setup's order and requirements are unchanged.

### The four observations after the last update

The owner reported these from the owner's own screens after updating the two
installed servers (one Ubuntu 24.04, one Debian 13) to v0.1.0-alpha.82 from the
panel's own update screen on 2026-10-10. No assistant looked at those servers.

1. *The full-screen "License status could not be checked" page during the
   Panel's restart (Ubuntu server).* That update was started from the
   v0.1.0-alpha.81 interface, which replaces the page when access is unknown; a
   lab measurement (with the releases rebuilt from their tags' source)
   reproduced this with that interface and saw the alpha.82 interface keep the
   page under a layer. Nothing can change alpha.81's
   interface. This release changes what the layer says (above). The earlier
   wording of the change was seen on a real Panel in the lab; its final wording
   has component tests and a mock browser only.
2. *The corner notice still said the update was being applied while the card
   showed the new version (Debian server, about two minutes after the start).*
   Changed: both now say "installed, being verified" in that window. Seen on a
   real Panel in the lab (Debian 13) during an update between two builds of the
   same code of this work, as an observation and not as a judged check. It takes
   effect for an update started from a page of this release (see above).
3. *A cold load of `/setup` showed a full-page "Checking panel access" for a
   moment.* Changed: nothing is drawn for the first 1.5 seconds. Measured in the
   lab on a real Panel (Debian 13): 18 of 18 loads passed. The 15- and 30-second
   steps that follow were made after that measurement and were measured in a
   later lab record on Debian 13 and Ubuntu 24.04: passed.
4. *The setup page said "This operation stopped" while its last step read "In
   progress", with the reason folded away (Debian server).* Changed: a run
   stopped by reopening its plan says so, and the reason stands above the steps.
   Component tests and a mock browser only; the owner's own setup record was not
   read.

### Returning to an older release

This release moves the Panel's database from schema 43 to 44: migration 044
adds one table, `managed_site_files`, and changes no existing table. There is no
step back. v0.1.0-alpha.82 refuses a database at 44, so a return, automatic or
not, restores the database snapshot taken before the update, as for every
migration.

A return loses this protection. The older release does not know the new first
line: at its start it writes every site's nginx file again from its records, so
a change you made, the first line and the line that includes your own directory
are gone from the file. Your files in `celikpanel-sites.d`, the dated copies and
the pending files stay on disk, but nginx stops reading your directory until this
release runs again. In the lab, after an automatic return, the older release's
start rewrote every site file, an owner's edit included: v0.1.0-alpha.81 on
three platforms in one record and on Debian 13 in another, and v0.1.0-alpha.82
on three platforms in a third (each rebuilt from its tag's source as a
test-licence build). The pools and the unit written by
alpha.82 were left byte for byte as they were. In every case the failing
candidate stopped before its own start, so no file carried the first line
yet. An automatic return follows a candidate that failed its
start or its start check; whether such a candidate can have written the first
line before it failed was not measured. What an older release does to a file
that already carries the first line was not measured; it is read from that
release's source. When this release runs again, it recognises the older
release's text and takes it over; an edit you made while the older release ran,
and before its next start, is kept. That second update after a return was not
run in the lab; this sentence is the design and the component tests.

### What you may need to do

- Reload open CelikPanel pages after the update.
- If scripts of yours call the API, read "API changes" before updating: several
  existing routes can now answer `409` for a site whose file is kept, and
  certificate issuance can answer `200` with `waiting_for_owner`.
- After the first start, open the Domains list as an administrator. If it shows
  the line about files left alone, open each marked domain's Configuration file
  page and choose. You do not have to: a file that was left alone keeps serving
  its site, but changes you make in CelikPanel are not applied to it, and a
  certificate for that site may wait for your choice.
- Put your own nginx lines for a site in `/etc/nginx/celikpanel-sites.d/<domain>/`.
  In a site file you edit, keep the line that includes
  `/etc/nginx/celikpanel-managed.d/<domain>/*.conf`.
- Put an application's own settings in
  `/etc/systemd/system/celikapp-<N>.service.d/`, then run
  `sudo systemctl daemon-reload`.
- The advice of v0.1.0-alpha.82 about Postfix still applies: on a server with
  mail, run `postconf -n >/dev/null`; if it prints a warning, correct the line of
  `main.cf` it names.
- Know before you update that a return to an older release removes this
  protection (above).

## API changes

Panel and Agent are installed together by one release. The new calls for the
pool and the unit are bound to the Panel's own build: an Agent of another build
answers "panel/agent build mismatch" and writes nothing, and there is no
fallback to the older pool calls. For the nginx files, the build match keeps an
Agent of another release from answering the Panel's writes, and the start
refuses its batch. An Agent that answers without a result per file is
recorded and shown as "not known" (`unknown`, `agent_does_not_report`), never as
unchanged.

Errors keep the shape `{"error", "code", "action"?, "reason"?, "detail"?,
"vars"?, "details"?}` and gain the additive `restore`, `service`,
`service_reason` and `service_active` (below). Screens translate by `code`.

### Site configuration files (new routes)

All are for an administrator (`403 {"error":"administrator access is
required"}` otherwise); a domain without a hosted site is `404`.

| Route | Body |
| --- | --- |
| `GET /api/v1/domains/{id}/site-config` | none |
| `POST /api/v1/domains/{id}/site-config/keep` | `{"file_sha256"}` |
| `POST /api/v1/domains/{id}/site-config/take` | `{"file_sha256", "render_sha256"}` |
| `POST /api/v1/domains/{id}/site-config/recreate` | `{}` or none |
| `POST /api/v1/domains/{id}/site-config/apply` | `{"file_sha256", "render_sha256"}` |

Each POST body may carry `"kind"`: `nginx_vhost` (the default), `php_pool` or
`app_unit`; another value is `400` ("kind is nginx_vhost, php_pool or
app_unit"); a kind the site does not have is `409 SITE_CONFIG_NOT_APPLICABLE`.
The digests are the ones the GET returned. The four POST routes require the
header `X-CelikPanel-Request-Id` of v0.1.0-alpha.82, with the same answers
(`428` without it, `400` if malformed, a replay answered from the stored answer
with `X-CelikPanel-Request-Replayed: 1`, `409 REQUEST_ID_REUSED`,
`409 REQUEST_IN_PROGRESS`, `409 REQUEST_OUTCOME_UNKNOWN`). With them, twelve
routes require the header.

**The GET answer.** The top-level fields describe the nginx file, as a client
that reads one file expects: `domain_id`, `domain`, `kind`, `path`, `state`,
`reason`, `detail`, `adopted_from`, `include_dir`, `enabled` (`link`, `absent`
or `other`), `file_sha256`, `render_sha256`, `pending_path`, `diff`,
`diff_truncated`, `actions`, `decision` (`kind`, `decided_at`, `current`) and
`ledger` (`written_release`, `written_at`, `observed_at`, `backup_path`). Added
beside them: `files[]` (the nginx file, then the pool of a PHP site, then the
unit of a Node site, each in the same shape with its own `kind` and `actions`),
`overall_state` and `overall_kind`.

- `state`: `absent`, `managed_unchanged`, `owner_edited`, `foreign`,
  `unreadable`, `unwritable`, `unknown_origin`, or `unknown` (`reason`
  `agent_does_not_report`, or `not_read` for a file in `files[]` the Agent did
  not answer for now).
- `reason` of `unreadable`: `symlink`, `not_regular`, `permission`,
  `too_large`, `read_failed`; of `unwritable`: `write_refused`. Additive on
  every answer, measured read-only by the Agent: `system_error` (for example
  `EPERM`, `EROFS`) and `protection` (`immutable`, `append_only`,
  `directory_immutable`, `read_only_filesystem`, `none`; absent when not
  measured). For `unwritable` on a file the owner changed, `file_state` names
  that file's own state.
- `actions` (always an array): `["keep","take","merge"]` for `owner_edited`,
  `foreign`, `unknown_origin`; `["recreate"]` for `absent`; `["recheck"]` for
  `unreadable` and `unwritable`; `["keep","take","merge","recheck"]` for
  `unwritable` with `file_state`; `["apply"]` for `managed_unchanged` with
  `reason` `protection_removed` while CelikPanel's text is held; `[]`
  otherwise. `merge` and `recheck` have no route: merge is done on the server,
  recheck is the GET again.
- `diff`: a unified diff of the file on disk against CelikPanel's text (first
  line left out), at most 4000 lines per side and 64 KiB. A line that mentions
  authorization, password, passwd, secret, token, api key, private key or
  cookie keeps its directive and shows its value as hidden; a comment line with
  such a word is replaced whole. A credential written without those words is
  not hidden.
- Certificate fields (nginx file): `managed_dir`, `managed_include` (the exact
  line a file needs), `challenge_file` (`absent`, `unchanged`, `differs`,
  `kept`; `written` and `failed` in the answer of an operation that wrote),
  `validation` (`ready`, `include_missing`, `names_missing`, `challenge_kept`,
  `challenge_failed`, `unknown`; with `validation_name` and
  `validation_status`; `validation_address`, the addresses asked or the one not
  this server's, and `validation_unknown_reason`), `resolved_reason`,
  `pending_reason` (`certificate`,
  `certificate_validation`) and `certificate` (`cert_path`, `key_path`,
  `expires_at`, `served_expires_at`, `served_days_left`, `referenced`), each
  only when known.
- `overall_state`: the highest presented state of the site's files, in the
  order unreadable, unwritable, missing, kept (owner-edited, replaced, not
  recognised), unknown, CelikPanel's unchanged text; the first file on a tie.
- The GET is not read-only while the file's recorded reason is
  `certificate_validation`: it repeats the validation check (a probe file in the
  challenge directory, removed again) and, when CelikPanel's own validation file
  is absent or out of date, writes it with one `nginx -t` and one reload; a
  check that finds every name served ends the reason and answers
  `resolved_reason: "certificate_validation"`.
- The GET also records what it found in the Panel's own database, so that the
  Domains list's row follows the file: a refused replacement that is gone, a
  file put back by hand, a pool or unit the Agent did not answer for (it never
  writes a site file otherwise).

**The POST answers.** `keep`: `200`, the GET's object (without `files[]`) after
the decision; nothing is written to the file. `take`: `200` with `outcome`
`taken` (or `unchanged`) and `backup_path`. `recreate`: `200` with `outcome`
`recreated` (or `unchanged`). `apply`: `200` with `outcome` `written` (or
`unchanged`), no dated copy. `take`, `recreate` and `apply` also carry
`managed_dir`, `managed_include` and `challenge_file` when the Agent reports
them.

| Refusal | Meaning |
| --- | --- |
| `400` | a digest missing or not 64 lowercase hexadecimal characters; an unknown `kind` |
| `409 SITE_CONFIG_CHANGED` | the file or CelikPanel's text changed after the GET; nothing was done |
| `409 SITE_CONFIG_NOT_APPLICABLE` | the choice is not offered for the file as it is now, or the site has no such file |
| `409 SITE_CONFIG_MISSING` | `take`: the file is gone |
| `409 SITE_CONFIG_UNWRITABLE` | `reason` `symlink`, `not_regular`, `permission`, `too_large`, `read_failed` or `write_refused`; also before anything is written when the Agent measures a protection now (no dated copy) |
| `502 SITE_CONFIG_NGINX_REFUSED` | `reason` `nginx_refused` or `reload_failed`; `restore` `confirmed` or `unconfirmed`; nginx's line in `details` |
| `502 SITE_CONFIG_SERVICE_REFUSED` | pool or unit: `detail` `config_refused` (the service's own check) or `reload_failed`; `restore` as above |
| `502 SITE_CONFIG_NOT_READ` | the Agent did not answer, answered with an error, or the input could not be prepared |

A `take` that a protection refuses leaves no dated copy: the Agent measures the
protection before the copy and removes the copy if the replacement is refused
anyway. For a pool or unit, CelikPanel's text is still put beside the file as the
pending file.

`restore` is `confirmed` when the Agent reports that the previous bytes are
back (read back after the put-back); otherwise it is `unconfirmed`, and the
sentence says that the server administrator checks the file on the server. Both
502 answers also carry, additively, what happened to the service after the
put-back: `service` (`reloaded`; `not_reloaded`, with `service_reason`
`config_refused_elsewhere` when the service's own check refused the put-back
configuration, or `check_unavailable`; `not_running` when the service was not
running before and was neither reloaded nor started; `reload_failed`, which
stays `unconfirmed`) and `service_active` (the strings `"true"`, `"false"` or
`"unknown"`, read from `systemctl is-active` after the operation). For an administrator the
check's line is in `vars.line` and `details`. A `systemd-analyze verify` that
times out, cannot be started or ends without output counts as
`check_unavailable`, not as a refusal. The Agent's own line is sent to
administrators only. Since the last change of this release (component tests
only), the `200` answer of a `take`, `apply` or `recreate` that reloaded the
service also carries `service: "reloaded"` and `service_active`.

### Existing routes that now refuse to change a kept file

Every operation that writes a site file classifies it first. Among them: the
general settings save (`POST /api/v1/domains/{id}/general`), the alias routes
(`POST /api/v1/domains/{id}/aliases`, `DELETE /api/v1/domains/{id}/aliases/{alias}`),
certificate issuance, the hosting change (`PUT /api/v1/domains/{id}/hosting`),
the PHP version switch (`POST /api/v1/domains/{id}/php`), the pool save
(`POST /api/v1/domains/{id}/php/pool`), site creation
(`POST /api/v1/domains/create`) and the cPanel import. On a file CelikPanel
keeps they answer:

- `409 SITE_CONFIG_OWNER_EDITED`: for the nginx file `reason` is the state
  (`owner_edited`, `foreign`, `unknown_origin`); for a pool or unit `reason` is
  the kind (`php_pool`, `app_unit`) and `detail` the state. Nothing was
  changed; a refused PHP switch switched nothing (pool, nginx file and the
  site's PHP version stay), and a refused hosting change keeps the previous
  settings.
- `409 SITE_CONFIG_MISSING` (for a pool or unit `reason` is the kind).
- `409 SITE_CONFIG_UNWRITABLE`: for the nginx file `reason` is the Agent's
  reason; for a pool or unit `reason` is the kind and `detail` the reason.
- `409 SITE_CONFIG_EXISTS`: site creation and import, when a file at the new
  site's path is not CelikPanel's unchanged text (the file is kept, only the
  records are removed); a PHP switch whose target version already has a pool
  file for the site (`reason` `php_pool`).
- `502 SITE_CONFIG_SERVICE_REFUSED` and `502 SITE_CONFIG_NGINX_REFUSED` as in
  the table above, with `restore`.

### Certificates

- `POST /api/v1/domains/{id}/ssl/letsencrypt` on a site whose file is kept:
  `200 {"status":"waiting_for_owner","expires_at":…,"pending_reason":"certificate"}`
  when the certificate was issued and stored but the kept file does not serve
  it yet (an ordinary success stays `{"status":"success","expires_at":…}`);
  before anything is requested, `409` with `code` `SITE_CONFIG_OWNER_EDITED`,
  `reason` `certificate_validation`, `detail` `include_missing`,
  `names_missing`, `challenge_kept` or `challenge_failed`, and `vars.include`
  (the line to add), `vars.name` and `vars.status` (the first name not served
  and nginx's HTTP status for it) and `vars.address` (the address that
  answered), each when known; `503 CERTIFICATE_VALIDATION_UNKNOWN` with
  `detail` `unknown` (and `vars.address`) when the check could not run for a
  local reason: nginx not answering on port 80 at an address asked, the
  challenge directory missing, the probe file not creatable, or this server's
  addresses not listable. When a name listens on an address that is not this
  server's, the request goes ahead, and the success answer (issuance, alias add
  and alias delete reissue) carries `validation_probe: "not_run"`,
  `validation_unknown_reason: "address_not_local"`, `validation_address` and
  `validation_note`; no record of it is stored apart from the Panel's journal
  line. A missing or unreadable file answers `409
  SITE_CONFIG_MISSING` or `SITE_CONFIG_UNWRITABLE` where v0.1.0-alpha.82 gave an
  untyped `409`. An Agent that does not say whether the validation can be
  published is answered as `include_missing`.
- The alias routes give the same refusals; a certificate reissue on a ready kept
  file answers `"status":"waiting_for_owner","pending_reason":"certificate"`
  instead of `"success"`.
- `GET /api/v1/domains/{id}/ssl`: each certificate carries `waiting_for_owner`
  (boolean); `renewal_status` gains the value `waiting_for_owner`; additive
  `waiting_for_owner_reason`: `certificate` or `certificate_validation`.
- `GET /api/v1/dashboard`: an entry of `expiring_certs` may carry
  `waiting_for_owner: true`, `domain_id` and `served_days_left` (the days left
  on the certificate in use, negative once it has expired). Such entries come
  first, whatever their days, ordered by `served_days_left`.

### Domains list

`GET /api/v1/domains`, for an administrator only: each domain may carry
`site_config` with `state`, and `kind` (absent for the nginx file),
`adopted_from`, `kept_by_choice`, `pending_reason`, `reason` and `read_error`
when they apply. A pool or unit the Agent did not answer for is recorded as
`unknown` with `read_error` (`timeout`, `agent_unreachable`, `agent_error`,
`render_input_unavailable`; the page's file carries the same field) until the
next answered read; a row with a known problem keeps it. It is
the Panel's last recorded observation, not a fresh read; a file with no
recorded row is not in it. `missing` appears here (the GET above answers
`absent` for the same file), and a recorded refused replacement is answered
`unwritable`, as on the page.

### Server setup

`GET /api/v1/setup` and `GET /api/v1/setup/operation`: the check `mail_identity`
in the state `action_required` carries, additively, `reason` and `vars`:
`server_address_not_public` (`ip`), `mail_name_differs` (`hostname`, `ip`,
`current`), `reverse_dns_mismatch` (`hostname`, `ip`, `ptr`),
`forward_dns_mismatch` (`hostname`, `ip`) or `mail_name_not_canonical`
(`name`), each var only when known. A reverse DNS lookup that got no answer
makes the check `state: "unknown"`, `code: "mail_identity_unavailable"`,
`reason: "reverse_dns_unknown"` with `vars.ip` and `vars.error` (`timeout`,
`no_resolver`, `refused`, `other`). A record written before this release keeps
its generic form.

### Update status

`GET /api/v1/panel/update/status` and `POST /api/v1/panel/update/abandon`
carry, additively, `phase: "verifying"` while the record is running and the
Panel that answers has the target's version and commit (the archive digest and
the release sequence are not compared). It never changes `status`.

Which of these answers were produced on a real system is said in the next two
sections and in the limits.

## Behaviour changes you will notice

- A Panel start that finds every site file unchanged writes nothing and does
  not reload nginx.
- A new site's nginx file is the text a later start writes.
- A saved setting, certificate, hosting change or PHP change on a site whose
  file is kept is refused and changes nothing.
- A removed site file is not recreated; the page offers "Recreate".
- A locked site file is refused alone; the other sites are written.
- Creating a site where a file that is not CelikPanel's already stands at its
  path is refused, and that file is kept.
- Deleting a site keeps a dated copy of an edited nginx file and leaves your
  include directory in place; CelikPanel's own directory is removed only when
  it holds nothing but CelikPanel's unchanged validation file.
- A new or recreated nginx file is `root:celikpanel 0644`; a kept or taken file
  keeps your mode, owner and group.
- The two include directories are created `0755`; a directory that already
  exists keeps its mode.
- A certificate for a kept file is obtained only when the server's own check
  finds the validation served; until you act, the site serves the certificate
  its file names.
- A certificate request for a kept file that listens on an address this server
  cannot reach from itself goes ahead with a warning instead of being refused.
- A pool save or PHP switch on a pool you edited is refused; your pool lines are
  no longer dropped.
- After a refused change, a service whose own check refuses the put-back
  configuration is not reloaded; a service that was not running is not started.
- An application unit starts its command without a login shell, with an
  explicit `PATH`.
- The Configuration file page's read records what it finds, so the Domains row
  follows a file you put back by hand.
- The setup page names why a run stopped above its steps, and the mail identity
  check names its condition.
- The update notice and the card say "installed, being verified" in the same
  window; the update window shows local time with its zone.
- A cold page load draws nothing for 1.5 seconds; "Check now" comes at
  15 seconds, "Reload CelikPanel" at 30 seconds.
- The layer over the page names the cause it read.
- The SSL tab, and the dashboard for a certificate that waits for your choice,
  say "expired N days ago" instead of a negative number of days.

Measured on real services (next section): the unchanged start, the creation
text, the refused save, the removed file, the locked file and its page, the
deletion of a site (its edited nginx file or pool kept as a dated copy, your
include directory left; the removal of CelikPanel's own directory was not
looked at), the owner and mode of written files, the include directories' mode,
the validation check on a kept file and the addresses it asks (without an
issued certificate), "Write CelikPanel's text now", the refused take without a
copy, the pool and the unit, the check before a reload after a put-back, the
unread file on the Domains list, the unit's `PATH`, the first-load quiet time
and the 15- and 30-second steps of the wait. The update notice's new words were
seen on a real Panel in the lab as an observation. The page read that records
what it finds and the `service` field of a successful choice have component
tests only; the setup page, the final wording of the layer and the SSL tab
wording have component tests and a mock browser only.

## What was measured, and how

Seven records made on 2026-10-10 and 2026-10-11 (UTC; the eleventh ran in the
night from 2026-10-10 to 2026-10-11), on disposable virtual machines on one
laptop host. The product ran as builds with the acceptance-test licence, not as
the signed archive. Every update was measured from an earlier release rebuilt
from its tag's source as a test-licence build, not from the published archive;
the digests of those rebuilds differ from the published ones. The directories are
under `deploy/e2e/release-recovery/evidence/`. Every result file marks itself as not
yet accepted evidence; the owner judges it.

**What the lab does and does not establish.** The virtual machines are not cut
off from the network: they run on QEMU user networking with outbound NAT, and
the distributions' packages are installed from their repositories during a run.
Nothing here shows what a machine could or could not reach. The name of the
release and licence host and four certificate-authority names were pinned to
the machine's own loopback in its hosts file before any product path existed,
and read back at the end (all labs of the seventh, eighth, ninth, twelfth and
thirteenth records; 11 of 12 labs of the tenth and every lab of the eleventh
that finished, where a stopped run has no read-back). In the eleventh and
twelfth records the certificate requests the Panel made went to the pinned
name, that is to the guest itself, and certbot refused the certificate it found
there, so no certificate authority took part. The licence step is
answered by the test build itself; that is what the build says about itself,
not an observation of traffic. No driver was pointed at an installed server. The
machines' traffic was not captured.

**What v0.1.0-alpha.82 does with a site file the owner edited (eighth record,
`set8-20261010`; Debian 13, Ubuntu 24.04, Arch).** The published tag's source,
rebuilt with the test licence under an alpha.81 label, not the published
archive; 48 runs (40 on the nginx file, 4 on the pool, 4 with a locked file).
In all 40 runs on the nginx file the owner's added `location`, changed `index`,
replacement file or own include was replaced by the Panel's own text at the next
start, restart or General settings save (36), or a removed file and its link
were recreated and served again (4); none of the Panel's API answers that were
read said so, and after a save no Panel or Agent journal line named the site. A
pool edit survived a start (4), and the pool save through the Panel then dropped
the owner's directive and comment (4). With `chattr +i` on one site file (4) the
file was kept, the start batch failed, the site's enabled link was absent
afterwards, and the next nginx reload, run by the lab as a log rotation would,
took that site out of service while the Panel still showed it active. Also
measured: the text written at creation differed from the text of the next start
(`www.` added); every start rewrote unchanged files; the Panel wrote
`root:celikpanel` over an owner's `root:root` file (owner and group varied in
one Debian run only).

**The first part of the site-file protection (tenth record, `set10-20261010`;
Debian 13 with nginx 1.26.3, Ubuntu 24.04 with nginx 1.24.0, Arch with nginx
1.30.5).** The code of the first part of this work: the nginx file with its
first line, the record per file, the three choices and your own include
directory. It did not yet contain CelikPanel's own include directory and the
validation check, the "cannot be replaced" answer, "Write CelikPanel's text
now", the order of states across files, the restore fact, the pool and the unit.
Installed fresh, and reached by an update from v0.1.0-alpha.81 rebuilt from
its tag's source as a test-licence build (not the published archive). 93 cells,
31 per platform: 84 passed, 6 did not pass (the same two findings on each
platform), 3 not measured.

- Passed on all three platforms: every owner edit (added location, changed
  directive, replaced file, own include) was kept byte for byte, with the same
  inode, at a start, a restart and a General save; CelikPanel's text was held
  beside it; the journal named the site; the page answered the state with the
  difference and the three choices; the Domains row carried the state; the save
  answered `409 SITE_CONFIG_OWNER_EDITED`; nginx was not reloaded; the owner's
  content stayed served. *Keep*: `428` without the request identity, then
  `200`, the file and its first line untouched, a repeat answered without
  running again, kept at the next start. An owner's `root:root 0640` file kept
  its mode and owner. *Take*: the dated copy with the owner's bytes, mode and
  owner, CelikPanel's text with the owner's mode and owner, a reload, the site
  served, a repeat answered without running again. A take with an out-of-date
  digest: `409 SITE_CONFIG_CHANGED`, nothing done. A take nginx refused: `502
  SITE_CONFIG_NGINX_REFUSED`, the owner's file back. A removed file and link:
  not recreated, the save `409 SITE_CONFIG_MISSING`, "Recreate" served the site
  again. A symlink: refused and not followed. Deletion: the edited file kept as a
  dated copy, the unchanged one removed, the include directories left. A start
  with every file unchanged: no write, no reload. Creation and start wrote the
  same text. A locked file: only it refused, its link intact, the other sites
  written, one reload; unlocked, the next start wrote it.
- An owner's `.conf` file in the include directory was served and survived a
  start and a save; the PHP version switch in that cell was not measured (each
  machine has one PHP version).
- The update from v0.1.0-alpha.81: the migration ran; the untouched files were
  taken over and rewritten as this release's text with the first line; the
  owner-edited file was left alone, byte for byte and without a first line:
  "4 adopted, 1 left alone" on Debian 13 and Ubuntu 24.04, "3 adopted, 1 left
  alone" on Arch. The three routes of that time (keep, take, recreate)
  answered `428` without a request identity. A database put back from before
  the migration was migrated again, and the start wrote nothing.
- The automatic return: a candidate whose migration fails returned to
  v0.1.0-alpha.81; the database was back at 42, and alpha.81's start rewrote
  every site file, the owner's edit included. The candidate never started in
  these cells, so no file carried the first line at the return.
- Did not pass, on each platform: the owner's include directory was created
  `root:celikpanel 0750`, not the documented 0755; and for a locked file the
  journal, the record and the Domains row named the lock while the page's own
  read answered "CelikPanel's text, unchanged" with no reason and no action.
  Both were corrected in the source afterwards and measured again in the
  eleventh record (below).
- Not measured: a PHP version switch, a site with a certificate, a second
  update after a return, the screens in a browser.

**The first page load, the access wait and an update in a real Chrome (ninth
record, `set9-20261010`; Debian 13).** A headless Chrome on the laptop host
against the real Panel of each guest; the interface of this work after its
first gate corrections, before the 15- and 30-second steps and before the
final wording of the layer.

- Cold full load of `/setup`, `/` and `/settings?section=updates`, plain and
  throttled to 2 Mbit/s and 300 ms: passed, 18 of 18 loads. No sentence and no
  button appeared before a read answered; the first read answered 71 to 97 ms
  (plain) and 0.92 to 0.98 s (throttled) after the first mount, inside the
  1.5 s.
- Every session read held 2.5 s: the explained wait appeared 1.57 to 1.62 s
  after the quiet surface began, in English and in Turkish. Passed.
- Every session read held 35 s: did not pass. An older 15-second limit showed
  "Your session could not be checked" with "Reload CelikPanel" at 15.18 s, and
  the older sentence again at the re-read. The 15- and 30-second steps
  described above were made afterwards and measured in the eleventh record.
- An update started from Settings: on one of three guests the layer appeared,
  with the wording of that time ("The Panel is not answering during an update
  … This is not a license problem."), and the Settings page stayed mounted and
  inert under it for at least 14.1 s and at most 18.1 s of a 29.3 s outage, then
  continued where it was. On the other two guests no layer was drawn (the page's
  access decision is taken not to have run out during the restart; its end time
  was not recorded there). The wording was changed afterwards; the final wording
  has component tests and a mock browser.
- Observation, not a judged check: on two guests the corner notice and the card
  said "Update installed, being verified" while the updater finished, and the
  update window showed local time with its zone. In the same window the card's
  "Current version" line and the sidebar still named the previous version until
  the product's own reload.
- No session, a session read failing as a closed connection, and a Panel
  starting were shown at once (passed). With the Panel stopped, Chrome showed its
  own error page: the recovery service worker was not registered in the
  signed-in browser contexts, although it controlled the no-session loads of a
  new context. The product's offline page was not reached here; the eleventh
  record narrowed the cause to the lab browser's profile (below).

**The baseline for the first load and the update (seventh record,
`set7-20261010`; Debian 13).** Also described in the notes of v0.1.0-alpha.82
(amended after its publication). On the alpha.82 tag's source, rebuilt as a
test-licence build, a cold load drew the full-page "Checking panel access" in
18 of 18 loads; during an update the alpha.81 interface (the tag's source,
rebuilt) replaced the page with the licence page for about 78 s of an 89 s
outage, the screen the owner described, while the alpha.82 interface kept the
page mounted under a layer for about 9 to 11 s of a 25 s outage, the layer's
first sentence naming the licence. The two outages differ (an update from
alpha.81 to alpha.82 in the first case, from an alpha.82 build labelled alpha.81
to alpha.82 in the second), so the figures compare the two interfaces, not the
updates.

**The corrections after the tenth record (eleventh record,
`set11-20261011`; Debian 13 and Ubuntu 24.04; Arch for the include
directories, the locked file and one validation case).** The code after the
locked-file and directory corrections: CelikPanel's own include directory, the
validation check (which then asked `127.0.0.1` only), the locked file's answer
and the 0755 mode. It did not yet contain "Write CelikPanel's text now", the
order of states across files, the pool and the unit, the restore fact, the
addresses the check asks, or the refused take without a copy. Installed fresh;
on Debian 13 also an update from v0.1.0-alpha.81, rebuilt from its tag's source
as a test-licence build, to a candidate that fails its start check. A headless Chrome 154 on the laptop host for the
browser cells.

- Include directories: both created `root:celikpanel 0755`, on all three
  platforms. Passed.
- The validation check against a real nginx (Debian 13, Ubuntu 24.04). One
  probe file appeared in the challenge directory, final mode `0644`, present for
  0.2 to 4.8 ms; nginx was asked once per name on `127.0.0.1:80` and answered
  `200` with the probe's body for every name; the request then went ahead (to
  the pinned name: certbot refused it, `502 CERTIFICATE_ISSUE_FAILED` /
  `authority_unreachable`) and the owner's file was not touched (CelikPanel
  wrote its pending text beside it). With nginx stopped: `503 CERTIFICATE_VALIDATION_UNKNOWN`. A kept file without
  CelikPanel's include line (also on Arch): `409 SITE_CONFIG_OWNER_EDITED` /
  `certificate_validation` / `include_missing`, with the line to add, the name
  and nginx's status. With the owner's own block for the `mail.` name: ready;
  with that block removed: `names_missing` for the `mail.` name, status `404`.
  The owner put the line back and reloaded nginx: the page's next read found
  the file ready and ended the reason without writing or reloading anything
  (CelikPanel's validation file was already in place; the case where it is
  absent or out of date, which the GET repairs, was not run). Passed; the
  dashboard part of that cell was not measured (no certificate could exist in
  this lab).
- Did not pass (Debian 13, Ubuntu 24.04): a site whose port-80 block the owner
  had bound to one address of the guest, while other blocks listen on the
  wildcard address. The check asked `127.0.0.1`, the default server answered
  `404`, and the request was refused as `include_missing` although the file
  carried CelikPanel's line. Changed afterwards: the check asks the addresses
  the site's file listens on (measured in the twelfth record, below).
- The locked file (all three platforms): at a start only that file was refused
  and the other sites were written; the page answered "cannot be replaced" with
  `EPERM` and the immutable attribute, the Domains row agreed, and "Check again"
  only read (no write, no reload). After unlocking, "Check again" still showed
  the file as not replaced, with no protection found (the behaviour of that
  code), and a General save then wrote it. Passed. The badge was not looked at in
  a browser: in this lab the setup page stands in front of the Domains list.
- Did not pass (all three platforms): a take on an owner-edited file that was
  locked between the page's read and the take was refused with `409
  SITE_CONFIG_UNWRITABLE` and the owner's file was untouched, but a dated copy
  was left beside it, and the page showed "edited outside CelikPanel" while the
  Domains row showed "cannot be replaced". Changed afterwards: one order of
  states for the page and the row, and no copy is left (measured in the twelfth
  record, below).
- The access wait in a real Chrome (Debian 13, Ubuntu 24.04; English). Every
  session read held 35 s: the explained wait after the quiet time, "Check now"
  at 15.0 s, the half-minute sentence and "Reload CelikPanel" at 30.0 s, and no
  other screen in between. Passed. A failed read followed by a held re-read: the
  failure shown at once; while the automatic re-read was held, the page's
  re-reading sentence ("Confirming your session and panel readiness") with the
  reload for 15 s; then the wait counted from that read, with "Last known,
  before this wait:" and the earlier failure, and the half-minute step 30 s after
  that read began; "Your session could not be checked" did not come back.
  Passed. A starting Panel followed by a held re-read: passed on Debian 13; on
  Ubuntu both loads hit Chrome's own error page right after the restart (not
  measured).
- The recovery service worker. In the Debian run the signed-in browser context
  used Chrome's profile on disk, about 177 characters deep (computed from the
  folder layout; the path itself was not recorded): the worker failed to
  install, with an error opening its cache storage, while a new in-memory
  context of the same browser registered it and was controlled by it. On the
  Ubuntu run, with the profile under a short path, the signed-in context
  registered it. With the Panel stopped, a context that had the worker showed the
  product's offline page ("The panel connection is unavailable"); the Debian
  signed-in context showed Chrome's own error page. So the missing worker of the
  ninth record points at the lab browser's profile, not at the product. That the
  Windows path-length limit is the mechanism is inferred, not read from a Chrome
  line; the Ubuntu run was another guest and another browser process, not a
  controlled comparison; a certificate the browser trusts without a pin was not
  measured. The lab's browser tools now keep the profile under a short path.
- The automatic return (Debian 13): the candidate failed its start check and the
  server returned to v0.1.0-alpha.81 by itself; the database was back at 42 and
  alpha.81's start rewrote every site file, the owner's edit included. The
  candidate failed before its own start, so no file ever carried the first line;
  what an older release does with such a file was not measured.

**The second part, the probe's addresses and the update from v0.1.0-alpha.82
(twelfth record, `set12-20261011`; Debian 13, Ubuntu 24.04, Arch; a headless
Chrome 154 on Debian).** The code with the pool and the unit, "Write
CelikPanel's text now", the order of states across files, the restore fact, the
addresses the check asks and the refused take without a copy. It differs from
the final code by the corrections listed after this record, the changes listed
after the thirteenth record and some screen texts.
Installed fresh, and reached by updates from v0.1.0-alpha.82 (three platforms)
and v0.1.0-alpha.81 (Debian 13), each rebuilt from its tag's source as a
test-licence build, not the published archive. Apart from the pinned names
nothing was blocked: the setup's packages came from the distributions'
repositories and the Panel's Node.js install from outside, through the outbound
NAT; that traffic was not recorded.

- Passed on all three platforms:
  - The pool: the owner's comment and directive were kept at a pool save
    (`409 SITE_CONFIG_OWNER_EDITED` / `php_pool`, CelikPanel's text with the
    saved value held beside the file); the page listed the nginx file and the
    pool, with the difference; "Take CelikPanel's" kept a dated copy of the
    owner's pool, wrote CelikPanel's text with the mode and owner kept and
    reloaded PHP-FPM. Deleting a site kept a dated copy of an edited pool.
  - The unit (Node.js 22.20.0 installed through the Panel): the owner's drop-in
    survived a hosting change and a take byte for byte; an owner's line in the
    unit made a hosting change answer `409` / `app_unit`; the take kept a dated
    copy, reloaded systemd and restarted the application.
  - A take nginx refused: `502 SITE_CONFIG_NGINX_REFUSED`, `restore:
    confirmed`, the owner's file back and served. A take with PHP-FPM stopped by
    the owner: `restore: unconfirmed` with its sentence, the file back.
  - "Write CelikPanel's text now": a file locked at the start, then unlocked,
    was offered it; the write left no dated copy and reloaded nginx; the page and
    the Domains row agreed. A take refused by a lock set after the page's read:
    `409 SITE_CONFIG_UNWRITABLE`, no dated copy, the page showed both facts, the
    row agreed.
  - The check's addresses: a port-80 block on the guest's own address was asked
    on that address, and a block on `[::]:80` only on `::1`; both ready, and the
    request went ahead (refused at the pinned name, `502
    authority_unreachable`; on Arch the packet reader saw only the status line,
    so those two cells were judged from nginx's access log). An address the
    guest does not have: no probe, the
    request went ahead, the journal line `validation_probe=not_run
    validation_unknown_reason=address_not_local` with the address, and the page
    read `unknown` / `address_not_local`. With nginx stopped: `503`.
  - The update from v0.1.0-alpha.82: database schema 43 to 44; "5 adopted, 1
    left alone" on Debian 13 and Ubuntu 24.04, "4 adopted, 1 left alone" on
    Arch; the owner-edited nginx file left alone and never written; alpha.82's
    pool and unit taken over, the owner-edited pool left alone; the four new
    routes answered `428` without a request identity and nothing changed. The
    Domains line's counts changed after the pages were read (5 and 1 became 4
    and 2; on Arch 4 and 1 became 3 and 2), because the pool's row began then.
  - A defective candidate from v0.1.0-alpha.82 returned by itself, the database
    at 43 and the Panel answering; the pools and the unit were byte for byte
    alpha.82's, and alpha.82's start rewrote every nginx file, the owner's edit
    included. The candidate failed before its own start, so no file carried the
    first line.
  - The update from v0.1.0-alpha.81 (Debian 13): the files were taken over as
    before, and a database put back from before the migration was migrated
    again.
- Did not pass, on all three platforms:
  - A pool the Agent could not read (its lock held for 75 s): the page said
    "not read" and ranked it above the unchanged nginx file, but the Domains row
    stayed "CelikPanel's text, unchanged", because a failed read wrote no row.
  - A take that `php-fpm -t` refused because of an owner's file elsewhere in the
    pool directory: the owner's pool was put back, PHP-FPM was then reloaded
    with the configuration the check still refused (attributed by timing in the
    journal) and ended `failed`, while the answer said `restore: confirmed` and
    that PHP-FPM keeps running.
- Not measured: a PHP version switch (one PHP version per guest), a put-back that
  itself fails, the success answer's fields of a check that did not run (no
  request could succeed).
- Observation (Debian 13): a bare `node` start command exited 127 under the
  unit's login shell, although the unit named the Node directory in `PATH`; that
  the login profile reset `PATH` is the likely cause, inferred.
- The real browser (Debian 13; English and Turkish, desktop and phone): the
  Domains badge, the Configuration file page with the nginx file and the pool,
  the pool's difference, and the "Write CelikPanel's text now" page; its result
  sentence was read from the page text. The recovery worker was registered with
  a short browser profile path. The two restore banners are in the page text,
  but the pictures taken for them show neither. At phone width the Domains list cut
  off its status column, and the badge sat under the domain name.

**Corrections after the twelfth record.** Made afterwards and measured in the
thirteenth record (next paragraph), on Debian 13 and Ubuntu 24.04:

- A pool or unit the Agent did not answer for is recorded as not read, with its
  error class, and the Domains row says "A configuration file could not be read;
  state not known" until a later read answers.
- After a put-back, a service that was not running before is left alone;
  otherwise its own check runs on the put-back configuration first, and only a
  configuration the check accepts is reloaded (a unit's application is
  restarted only when `systemd-analyze verify` accepts it). The answer says
  what happened to the service. The record's case of an owner who stopped
  PHP-FPM is now answered `confirmed` with `not_running`.
- The unit's start command uses `/bin/sh -c` and an explicit `PATH` with the
  Panel's Node directory; the earlier releases' unit texts stay as they were for
  recognition.
- The page's answer to a choice scrolls into view.

**The corrections after the twelfth record (thirteenth record,
`set13-20261011`; Debian 13 and Ubuntu 24.04; Arch not run; a headless Chrome
154 on Debian).** The code with those corrections, built as a test-licence
fixture that differs from it only in the release-sequence file; it does not
contain the changes listed after this record. Installed fresh; no update and no
certificate request in this round. The guests had outbound NAT as before (the
setup's packages, the Panel's Node.js install); that traffic was not recorded.
No product check failed; three cells of the first runs were not measured (two
preconditions not met in the harness's order, one harness error) and were
measured again in new labs.

- A pool take that `php-fpm -t` refused because of the owner's file elsewhere in
  the pool directory: `502 SITE_CONFIG_SERVICE_REFUSED`, `restore: confirmed`,
  `service: not_reloaded`, `service_reason: config_refused_elsewhere`,
  `service_active: true`, and the check's line in `vars.line` and in the
  sentence. 12 to 13 s later PHP-FPM was active with the same main process and
  no restart, and no reload of it followed the take; the owner's pool was
  byte-equal. With that file removed, the next take answered `200` and PHP-FPM
  was reloaded (read from the journal; that `200` carried no `service` field).
- PHP-FPM stopped by the owner: `restore: confirmed`, `service: not_running`,
  `service_active: false`, and the sentence that tells the owner to start
  PHP-FPM; nothing started it.
- A take nginx refused: with a conflict only in the site's include directory,
  the put-back passed `nginx -t` and nginx was reloaded (`service: reloaded`);
  with the owner's broken file in `conf.d`, `service: not_reloaded` with nginx's
  line, and nginx kept its main process. The owner's file was byte-equal and
  served in both.
- A pool the Agent could not read (its lock held for 75 s): the page answered it
  `unknown` / `not_read` with `read_error: timeout`, and the Domains row now said
  the same, also after the lock cleared and before the next read; the ledger row
  changed no digest. The next answered read made the row "CelikPanel's text,
  unchanged" again.
- The Node application unit: a bare `node` start command ran under
  `/bin/sh -c` with the explicit `PATH` and answered. After a put-back, the
  application was restarted when `systemd-analyze verify` accepted the unit,
  not restarted when it refused it (`service: not_reloaded` with the verify
  line), and not restarted when `systemd-analyze` could not be run
  (`check_unavailable`). (The lab forced the put-back by making one
  `daemon-reload` fail.)
- In the real browser (Debian 13 only): the refused take's answer was in view,
  as a status box at the top of the viewport, in English on a desktop and in
  Turkish on a phone; the
  Domains badge "A configuration file could not be read; state not known" /
  "Bir yapılandırma dosyası okunamadı; durumu bilinmiyor" was shown on desktop and
  phone and was gone after the page had been read.
- Observation, not a cell: after a refused take of an nginx file, the owner put
  CelikPanel's text back by hand; the page then said "CelikPanel's text,
  unchanged", while the Domains row kept "Configuration file: your choice
  needed" until the next write. Changed afterwards: the page read records what
  it finds (component tests only).

**After the thirteenth record.** Made afterwards; component tests only; not
measured on a real system: the sentences after a refused put-back say how work
resumes and, when the service is not running, tell the owner to start it; a
`systemd-analyze verify` that times out is `check_unavailable`; the page read
records what it finds on the row, also for a pool's or unit's row that still
carries an earlier refusal (above); a successful choice that reloaded the
service says so in its answer.

**Which check rests on which code.** Each later state of the code contains the
earlier corrections.

| What was checked | The code it ran on |
| --- | --- |
| The nginx file kept, the three choices, adoption after an update from v0.1.0-alpha.81, a return after a failed migration, a database put back, the locked file's effect on other sites | the first part of this work (tenth record); the adoption from alpha.81 and the database put back were repeated on Debian 13 in the twelfth record; none of it on the final code |
| CelikPanel's own include directory, the 0755 mode, the validation check on `127.0.0.1`, the locked file's page and "Check again", the access wait's 15- and 30-second steps, the service worker, a return after a failed start check | the code after the locked-file and directory corrections (eleventh record) |
| The pool and the unit, the multi-file page, the addresses the check asks, "Write CelikPanel's text now", a refused take without a copy, the restore fact, the update from v0.1.0-alpha.82 and its automatic return | the code before the last screen texts and the corrections after the twelfth record (twelfth record) |
| The check before a reload after a put-back (pool, nginx file, unit), a stopped PHP-FPM, the unread file on the Domains list, the unit's `PATH`, the answer in view | the code after the twelfth record's corrections (thirteenth record; Debian 13 and Ubuntu 24.04 only, the browser parts Debian 13 only) |
| The resume and "not running" sentences, the verify time-out, the page read that records what it finds, the `service` field of a successful choice | component tests only; the final code differs from the thirteenth record's code by these |
| The cold first load and the explained wait at 1.5 s | the interface after the first gate corrections (ninth record) |
| The layer's final wording, the setup page, the mail identity reason | component tests and a mock browser |
| The update notice "installed, being verified" | observed on a real Panel in the ninth record; component tests |
| What v0.1.0-alpha.82 does with an owner's edit | the published tag's source, rebuilt as a test-licence build (eighth record) |
| What the alpha.81 and alpha.82 interfaces do during an update and on a cold load | the two tags' source, rebuilt as test-licence builds (seventh record) |
| The whole code at the time of writing | component tests; CI on the head at the time named below |

**Screens.** The interface changes have component tests and were looked at in a
Chrome against a loopback mock of the API (desktop and phone, English and
Turkish; light and dark for the Configuration file page). No real Panel was
behind that browser. The ninth record put a real Chrome in front of a real
Panel for the first load, the access wait and an update (desktop size,
headless; Turkish for the access wait only), the eleventh for the access wait
and the service worker (English), the twelfth for the Domains list and the
Configuration file page (English and Turkish, desktop and phone), and the
thirteenth for the refused take's answer and the Domains badge of an unread
file (English and Turkish, desktop and phone; Debian 13 only).

**Tests.** The change records of the last states of the code give: 1315
interface tests passed; the Panel and shared Go packages passed; the Agent
package had the same 19 failures as before these changes, which the change
record attributes to the development machine's environment. These numbers come
from the change records, not from a retained test log; the interface tests were
run again on the head when these notes were checked (1315 passed, none failed),
the Go packages were not. The continuous
integration runs of the pull request on the heads of 10:20, 11:40, 11:44 and
11:49 UTC on 2026-10-11 each passed 23 of 23 checks (24 jobs: the job that
publishes assets runs only on a release tag and was skipped); they ended at
10:41, 11:57, 11:59 and 12:09 UTC. The head of 11:44 UTC carries the last
change to product code (the page read that records what it finds and the
`service` field of a successful choice); the head of 10:20 UTC is before it.
Six runs between 09:22 and 09:48 UTC had failed, all in the one job "Panel race
— A and B, exact and non-uppercase", on a data race in a test's fake Agent; the
change that made the next run pass is in test code. Commits made after 12:09 UTC
get their own runs, which are not entered here. A passing run is not the
owner's own test.

## Limits of this release

These are known. They are not hidden defects.

- **The final code was not measured on a real system as a whole.** The tenth
  to thirteenth records measured the code in four stages. The changes after the
  thirteenth record (the resume and "not running" sentences, the verify
  time-out, the page read that records what it finds, the `service` field of a
  successful choice) have component tests only. The release preparation
  (release sequence 83, bootstrap pins, version lines) is in none of the
  measured builds (the update candidates of the twelfth record carried a
  sequence-83 policy and an alpha.83 label made in the lab, as test-licence
  builds); apart from the continuous integration's reproducible-archive check on
  the pull request, no archive of this release had been built from the prepared
  branch when this was written.
- **Arch** was not in the thirteenth record: the check before a reload, the
  unread row and the unit's `PATH` were measured on Debian 13 and Ubuntu 24.04
  only.
- **The text when the screen texts are not loaded.** If the browser has not
  loaded the screen catalogue, the server's own fallback sentence for a stopped
  service still says that the service keeps running.
- **The updates were measured from rebuilds, not from the published
  archives.** v0.1.0-alpha.81 and alpha.82 were rebuilt from their tags' source
  as test-licence builds; their digests differ from the published archives'.
  The update from alpha.82 ran on three platforms, the update from alpha.81 on
  Debian 13 in the twelfth record and on three platforms in the tenth, and a
  failing update from alpha.81 on Debian 13 in the eleventh. An installed
  server's own archive and data were not used. The machines had outbound NAT
  (packages and Node.js came from outside; that traffic was not recorded).
- **No site with a certificate was measured, and no certificate was issued.**
  The validation check was run against a real nginx on `127.0.0.1` (eleventh
  record) and on a site's own address and `::1` (twelfth record), with a
  certificate authority never taking part: the requests went to the machine's
  own pinned name and certbot refused them. The success answer's fields for a
  check that did not run (`validation_probe: "not_run"` and its companions) were
  never seen on a real system, because no request could succeed.
- **The validation check.** It reads only the `listen` lines of the kept file
  itself (an `include` is not followed). A name that listens on an address that
  is not this server's cannot be checked from here: the request goes ahead, and
  the certificate authority's attempt is the test. Only the SSL tab's issuance
  answers a refused validation with a typed reason (`502
  CERTIFICATE_ISSUE_FAILED`, `reason` `validation`, when the Agent read
  certbot's output so); the alias reissue answers a plain `409` on a failure of
  the authority, and a renewal records its failure without a typed reason. That
  the check did not run is recorded only in the Panel's journal. At the page's
  read, a failed
  `nginx -t` or reload while CelikPanel publishes its validation file is
  reported as `challenge_failed`, although the cause may be another file. A
  missing or unreadable file records no reason, so a renewal it stopped waits
  until the next completed renewal. A ready check shows that nginx serves the
  path on the addresses asked from the server itself, not that DNS or a
  firewall lets the certificate authority in. That a waiting certificate's
  dashboard entry goes away when the reason ends was not measured. A first issuance is never retried; a renewal is retried only by
  the schedule (every 12 hours and at start, for certificates within 30 days of
  expiry). The pages were not drawn in a real hosting customer's session.
- **A return to an older release loses the protection** (see "Returning to an
  older release"). Measured for files without the first line (alpha.81 in two
  records, alpha.82 in a third); for files with it, read from the older
  release's source.
- **The first start renders hosted sites.** At its first start this release
  writes the nginx file of every hosted site that it recognises as an earlier
  release's own text (adoption, with one nginx check and reload); only a file
  that differs from every known text is left alone. That write was measured on
  rebuilt baselines in the lab and never on an installed server's files.
- **Deleting a site.** The removal of CelikPanel's own include directory
  (`celikpanel-managed.d`) at deletion has component tests only: the deletion
  measurements kept the dated copies and your directory, and did not look at it.
- **The pool and the unit.** A PHP version switch was never measured (each lab
  machine has one PHP version), and the removal and clean-up steps of its own
  put-back still reload without the check. In the lab only "Take CelikPanel's"
  was run on a pool or a unit (with the refusals of a pool save or a hosting
  change); "Keep mine", "Recreate" and "Write CelikPanel's text now" on those
  two files have component tests only. They are not
  looked at when the Panel starts, and the start line counts nginx files
  only. Site creation records no row for them,
  so a pool or unit nobody has opened or changed is not on the Domains list. The
  calls are bound to the Panel's build with no fallback for an Agent of another
  build. The generic PHP page's pool save by pool name gets the typed refusal
  only through the Agent's error text.
- **nginx judges every site together.** An owner's broken file elsewhere still
  fails every write of a site file (the written files are put back), as before.
  Your include directory is read after the site's `location` blocks.
- **Not covered:** the Panel's own nginx site for its certificate, and the nginx
  files of webmail and the database tools.
- **Screens not yet converted.** 30 source files of the interface (88 places;
  31 files in v0.1.0-alpha.82) still read the server the old way in at least
  one place: the
  application shell and its session read, the sign-in page, the setup wizard,
  the update card and the update and component operation overlays, the Services
  list and service pages, the sidebar, parts of the dashboard, add-ons, the
  audit log, VPN, team members, the licence page and a few smaller parts. On
  these a failed read can still look like an empty or negative state.
- **The access wait.** Each page load still reads the session twice. A load
  with the HTTP cache enabled was not measured. The offline page with a stopped
  Panel was seen only in lab browser contexts that had the recovery worker; the
  worker with a certificate the browser trusts without a pin was not measured.
  The locked file's badge and the Configuration file page were not looked at in
  a real browser.
- **The update notice.** The card's "Current version" line stays on the
  previous version until the page reloads itself (observed in the lab). On a
  phone the corner notice covered the card's status line in the mock browser.
  The verifying window was observed on a real Panel in the lab, not on an
  installed server. During the update to
  this release the open page is still v0.1.0-alpha.82's.
- **Setup.** The mail identity reason was never read from a real Agent. A failed
  forward lookup after a matching reverse DNS name is reported as the same
  unknown ("could not be checked"); the two lookups are not told apart. An Agent of an earlier release still
  reports an empty reverse DNS name as a mismatch.
- **Limits of v0.1.0-alpha.82 that still stand.**
  - Routes without a request identity: twelve routes now require it (the eight
    of v0.1.0-alpha.82 and the four POST routes above). Service and application
    restart, plans, enrollment codes, about 38 other routes, DNS records,
    hosting type, PHP settings, aliases, application install, mail
    authentication, accounts and files are not protected.
  - A mail renewal helper already installed on a server is not replaced by a
    panel update.
  - The mail certificate path behind a `postconf` warning was not measured on a
    server.
  - The packaging is unchanged since v0.1.0-alpha.82, whose archive carries the
    vendor's publishing tools and two one-time recovery scripts of an earlier
    incident; whether they leave the archive is an owner decision that is still
    open.
  - A Postfix restriction list written one restriction per line becomes one line
    after a DNSBL save.
  - The Panel has no control that sets a database user's password.
  - The other limits of [v0.1.0-alpha.82](RELEASE-NOTES-v0.1.0-alpha.82.md) that
    this release does not address still apply. Its limit "Your own edits of a
    generated site configuration file" is addressed here, for the nginx file,
    the pool and the unit, to the extent the measurements above and the limits
    in this list say.
- **Evidence scope.** One laptop host; builds with the acceptance-test licence,
  not the signed archive; machines with outbound NAT whose traffic was not
  captured; names pinned to loopback; each cell ran once, except where a record
  names a repeated run. The host logged modern standby during parts of the
  seventh to the thirteenth records; their disk watchers show no gap longer
  than 31 s. Checks
  that did not pass are listed in each directory's `checks-not-passed.txt`.
- **Open acceptance items.** All five foundation items of the
  [resilience contract](RESILIENCE-CONTRACT.md) remain partial; this release
  closes none. Decisions: D-021 (unchanged), D-022, D-024, D-025, D-029, D-031
  in [DECISIONS](DECISIONS.md).

## Publication record

Everything below is dated 2026-10-11 (clock date, UTC unless a time zone is
named). The steps were taken by the owner; the checks of the assets and of the
portal package were made by the person who prepared the release. This entry
does not store any of the results below as a file in the repository.

**What happened.**

1. The number: v0.1.0-alpha.83, the owner's decision of 2026-10-11. Done.
2. The release preparation was committed in a separate commit that changes no
   file under `cmd/`, `internal/` or `web/`: release sequence 83 after 82, the
   installer's bootstrap pins, the README version lines and the contract tests
   that pin them. (The owner's decision is dated 2026-10-11; that commit's
   message says 2026-10-12 by mistake.) The real archive was built and signed
   by CI on the tag.
3. The results of the eleventh, twelfth and thirteenth records were entered.
   The continuous integration runs of the pull request up to 12:09 UTC on
   2026-10-11 are named under "Tests".
4. The continuous integration splits one test group into three ("Panel
   race — S, except the service, setup, site and startup tests", "Panel race —
   Startup readiness", "Panel race — Site configuration"). A read of the
   branch protection of `main` on 2026-10-11 showed only one required check,
   "Release — reproducible archive and checksum", so no rename was needed
   before the merge; an earlier form of this item said one was.
5. The vendor's publishing tools and the two one-time recovery scripts stay in
   this release's archive (owner decision, 2026-10-11). They do nothing on a
   customer's server; removing them is left to a later release, where the
   packaging change needs its own archive build and content checks.
6. Pull request #206 was squash-merged by the owner into `main` on 2026-10-11.
   Main is commit `0447558e956d4c02c7bf06b368def0924856f8fc`, tree
   `7adde05327ecadb3c7d3451ec9b08fab9494a480`; that tree equals the tree of
   the pull request's last commit, `304da2466`. The continuous integration run
   on that main commit (run 38145660891) passed: 23 of 23 jobs that ran; the
   job that publishes assets was skipped, because it runs only for a tag.
7. Before tagging, the owner set the repository variable
   `CELIKPANEL_RELEASE_SEQUENCE` to 83. The owner then created and pushed the
   annotated tag `v0.1.0-alpha.83` (tag object
   `f5772a5b96fe8a341c63fd71a27a750bade4e382`) on the main commit above. The
   tag's CI run (38153704563) completed successfully with 24 jobs and
   published the release as a pre-release at 16:22:31 UTC with six assets: the
   two archives, their two checksum files, the signed manifest and its
   signature.
8. The assets were checked in a local build guest, from a clone checked out at
   the tag. The linux/amd64 archive
   `celikpanel-v0.1.0-alpha.83-linux-amd64.tar.gz` is 66,132,418 bytes,
   SHA-256 `6275bebed2b581bad58d4323217ed14defb54d06c23cef5af94218da573aefe5`.
   Both checksum files verify; the two archives are byte-identical. The signed
   manifest says `sequence=83`, `version=v0.1.0-alpha.83`, the main commit
   above, `published_at=2026-10-11T14:03:46Z`, that SHA-256 and that size. Its
   Ed25519 signature verifies against `deploy/release-signing-ed25519.pem`
   from the tag (key file SHA-256
   `7eadeb0b156f1a821575c4293fe664b44b8004bcdb5e9e770122cb5c144c68bb`, the
   same as the copy inside the archive and as the pin in the archive's
   `libexec/get.sh`). The `release.commit` and `release.tree` recorded in the
   archive equal the tag's. `install.sh`, `libexec/get.sh` and the public key
   in the archive are byte-identical to the tag's files.
   `deploy/release-content-guard.sh` and
   `deploy/release-acceptance-license-guard.sh` both exited 0 on the archive.
   The archive's bootstrap pins are sequence 83 and v0.1.0-alpha.83.
9. The download portal package was built from the tag checkout in pre-signed
   mode with the six downloaded assets. `deploy/test-download-portal-contract.sh`
   passed. The package was built twice with byte-identical results: 132,634,305
   bytes, SHA-256
   `6787d0b42ff70621fe1a2ee46c75d336ca24092108a2182e6c35a2da0f5c94d2`. Its
   file layout is identical to the v0.1.0-alpha.82 package apart from the
   version. In the tag checkout `deploy/test-download-portal-public-verifier.py`
   ran 17 tests, all OK, and `deploy/test-download-portal-promotion.py` ran 14
   tests, all OK. A local rehearsal ran the real promoter against a fixture
   copy of the alpha.82 site served on loopback: exit 0, status `committed`,
   26 requests, the success marker printed once, the previous version and the
   backup kept.
10. The owner published the portal with `deploy/publish-download-portal.ps1`
    (previous version v0.1.0-alpha.82, target v0.1.0-alpha.83). The result was
    status `committed`. The backup
    `httpdocs.before-v0.1.0-alpha.83.20261011T191704Z…` is kept on the server.
    The single public verification pass after the exchange reported
    `status: ok`, 26 requests, one archive download, 66,672,304 bytes
    downloaded. The marker `CELIKPANEL_DOWNLOAD_PORTAL_PUBLISHED` was printed
    once.
11. A read-only read of https://celikpanel.net from outside afterwards:
    `releases/latest.txt` is `v0.1.0-alpha.83`; `releases/latest.json` names
    the main commit and the SHA-256 above; the published signed manifest, its
    signature and the checksum file are byte-identical to the verified GitHub
    assets; the archive URL answers 200 with `Content-Length: 66132418` (the
    archive was not downloaded again in this read); `get.sh` pins sequence 83
    and v0.1.0-alpha.83; the alpha.82 manifest is still served.

**Not done, and what publication does not change.**

- The installed servers are not updated. At the time of this record nobody has
  updated them: the owner of each installed server starts that from the
  panel's own update screen, as a separate step from every step above, and no
  one else starts it. The outcome will be recorded separately. Nothing in this
  record describes what an installed server does with this release.
- Publication is not product acceptance. The limits listed under "Limits of
  this release" are unchanged by it, among them that the last changes made
  after the short round have component tests only. All five foundation items
  of the [resilience contract](RESILIENCE-CONTRACT.md) remain partial; no P0
  item is closed by the release or by its publication.

Install this release only through CelikPanel's update interface. Publishing it
does not update installed servers; only the owner of a server starts its
update, from the panel's own update screen.
