# What a domain's certificate protects (alpha.134)

*2026-11-12 · lane certcover. The owner asked, looking at his certificate
screen: "the certificate does not cover the mail prefix; how are we supposed
to know what it covers and what it does not?"*

## 1. What was true before this change

- Mail programs are told the **server's** mail name (`mail_autoconfig.go`),
  and every mail port presents the server's trusted mail certificate for that
  name. Mail was not insecure.
- The domain's MX and every hand-typed setting use `mail.<domain>`. A program
  that connects to that name got the server's certificate: a name warning.
- `mail.<domain>` entered the certificate only when someone ticked it at
  request time, and mail used it only after a second switch ("Maili bu
  sertifikayla koru"). That switch also changed the name mail programs are
  told, without rewriting the auto-setup answers.
- No screen said which name mail programs use, or whether a certificate
  covers it.

## 2. The list "Neler korunuyor"

One list on the certificate screen (`GET /api/v1/domains/{id}/ssl/coverage`,
same gate as the certificate screen; team members with the SSL view right).
Each line: what it is, the name, and either **Korunuyor** (by which
certificate, until when) or **Korunmuyor** with one reason and at most one
action. Lines:

| Line | Name | Shown when |
| --- | --- | --- |
| Web site | the domain, `www`, each other name of the site | the domain has a site |
| Web mail | the server's web mail address | mail is on this server and web mail has an address |
| Mail programs | the name they are told (server's mail name, or `mail.<domain>` for a domain that chose it before alpha.134) | mail is on this server |
| Mail name | `mail.<domain>` | mail is here, the name exists in public DNS or is in the certificate, and it is not already the line above |
| Mail auto-setup | `autoconfig.<domain>`, `autodiscover.<domain>` | the domain answers mail programs (MAIL-AUTOCONFIG.md) |

Reasons, each one sentence: no certificate; the name is not in the
certificate (it points here); the name points elsewhere (the addresses are
named; it cannot be included); the name has no address; the certificate is
not usable now (expired, not trusted, not in use); the certificate covers the
mail name but mail does not use it yet; the server's mail certificate is not
ready; web mail answers without a certificate for its name; could not check.
"Covered by the server's mail certificate" is **Korunuyor**, and says so.

Actions: *Posta adını da kapsa* (request the certificate again with the mail
name and switch mail to it), *Bu adla yeniden al* (request again: every name
of the site and every name that points here is included), *Postada da kullan*
(mail starts using the certificate that already covers the name), *DNS
kayıtlarını aç* (only where this server holds the zone), *Posta ayarlarını
aç* (auto-setup records), *Yeniden denetle* (the zone is run elsewhere: the
list asks the public DNS again; it only reads). An uploaded certificate
cannot be requested again: its action opens the upload form.

A name that points elsewhere or has no address is an **unmet prerequisite,
not a failure** (D-024): its badge says *Başka yeri gösteriyor* / *DNS’te
adresi yok* in the warning tone instead of *Korunmuyor*, and under the
reason one sentence says who acts and what to do: point the name's A record
to this server's address (`server_ip` of the answer) in this server's DNS
records or where the domain's DNS is run, and what then appears in the list.
For `mail.<domain>` it also says that nothing is needed when the domain's
mail is elsewhere (mail being installed is a server fact, not a domain one).
No certificate authority is asked for such a name.

The list stands right under the certificate's facts and its state notes,
before the renewal switch and the buttons, so a gap and its one fix are seen
first. Inside the list, every name that is not protected (a gap, a name
waiting for DNS, one that could not be checked) comes first, then the
protected names, each group in the server's order: one list, always open,
nothing folded.

The mail screen states the same line under the server name, with the same
words ("Bu adı güvenilir bir sertifika koruyor (… tarihine kadar)").

## 3. Right by default

A certificate request (new or again) for a domain whose mail is on this
server includes `mail.<domain>` **when the public DNS shows it here**, without
a tick. The form shows the name ticked; the person can leave it out
(`exclude_mail`). A name that points elsewhere, has no address or could not be
checked is not included and the request goes on (the list says why). A site
file the owner kept (D-031) leaves the name out, as for the auto-setup names.
`include_mail: true` keeps its old API meaning (include, or refuse when the
name points elsewhere).

When the issued certificate covers `mail.<domain>`, mail serves it for that
name (`secure_mail = 1`). Serving and telling are separate:

- **Serving** adds one name entry to Postfix's SNI map and one Dovecot
  `local_name` block, both naming certbot's stable live files of the site's
  lineage. The default certificate of every mail port (the server's mail
  name) is unchanged, so programs set up with the server's name see exactly
  what they saw. A program that connects to `mail.<domain>` now gets a
  certificate for that name instead of a warning.
- **Telling** (auto-setup answers, the mail screen's settings) stays as it
  is. The server's name is told unless the domain chose `mail.<domain>`
  before alpha.134 (the new `domains.mail_clients_use_own_name`, set by the
  migration from the old switch). A newly served name never changes what
  programs are told, so nothing that works starts to warn.

If mail cannot start serving the name (the mail step fails, with a known
result), mail is put back as it was and the request ends as success with a
plain note; the list then offers *Postada da kullan*. A step whose result is
unknown keeps today's "waiting" state and its retry.

The switch "Postayı da bu sertifikayla koru" now means serving only. When it
changes what a domain of the old choice tells programs, the site file (the
auto-setup answers) is written again in the same operation, and a failure
there puts mail back as it was.

## 4. Existing domains

Nothing changes silently. A certificate without the mail name shows the line
**Korunmuyor** with *Posta adını da kapsa*. That one press requests the
certificate again (same authority, same renewal setting, the account's
address) in the progress window with the real steps. It asks for the mail
name by name (`include_mail`): if the name no longer points here, the server
refuses before any authority is asked (`CERTIFICATE_DOMAIN_NOT_POINTED`), and
the list is read again and shows the name's next step. It never gets the same
certificate again without the name it was pressed for. If the authority
refuses, the old certificate and mail stay as they were (the request is
staged; D-029). Authorities that need account keys (EAB) open the request
form instead of a one-press request.

## 5. Renewal with the panel absent (P0.5)

Unchanged mechanism: the lineage is certbot's, `certbot.timer` renews it, and
the native deploy hook (`site_cert_native.go`) rebuilds the SNI map and
reloads Postfix and Dovecot when the map or drop-in names the lineage's live
files. The per-name blocks name only those live paths; no CelikPanel program
is involved. A renewal fails as a whole if any name stops pointing here
(certbot validates every name): the existing renewal guidance reports it,
like any other name of the site. A certificate of an earlier release that
the panel still renews itself (`cert_renewal.go`) keeps `mail.<domain>` the
same way: the renewal validates the name when the current certificate
carries it and requires the renewed one to carry exactly the site's names and
it, and `secure_mail` is carried over.

## 5a. The certificate it replaced (lab check, 14 Nov 2026)

Every request makes a new certbot lineage (D-040: a failed request never
touches the lineage being served), so after *Posta adını da kapsa*, *Bu adla
yeniden al*, an alias change or a detach, the lineage served before stayed in
`/etc/letsencrypt`. Until this change the Panel only wrote `autorenew =
False` into it at its 12-hour pass (D-040 point 9): its files, renewal file
and deploy record stayed, and until that pass certbot could renew it.

Now the Panel asks the Agent (`RetireSiteCertificateLineagesV1`) right after
the new certificate is served (request, alias add and remove) and again at
every 12-hour pass, under the domain's certificate lock. Nothing is asked
while the active certificate waits for its activation, its mail step or the
owner's site file (it is not served yet, so the old one may be). The Agent
runs `certbot delete --cert-name <lineage>` and removes the lineage's deploy
record only when:

- the lineage is a native one of this domain (`celikpanel-site-<id>-…`), only
  revoked or expired ledger rows name it, and it is not the active one;
- its renewal file is still CelikPanel's: the same rule the certificate page
  uses for "changed outside CelikPanel" (authenticator webroot, the domain's
  own validation directory, the authority it was issued from or the one
  certbot recorded for the domain's newest certificate);
- no file under `/etc` other than certbot's own store names it (nginx,
  Postfix, Dovecot, another site, anything wired by hand). A lineage name
  carries 24 random hex digits, so a file that contains it names it.

Each lineage ends as `retired`, `absent`, `in_use` (the file is named),
`owner_changed`, `not_checked` (a file or directory could not be read, or more
than 50 000 files) or `failed` (certbot's own error). Everything but `retired`
and `absent` keeps the lineage, is logged and is asked again at the next pass.
An administrator who made the request sees one warning naming what was kept
and why (`certcover.previousKept.*`); a customer or reseller sees nothing,
because there is nothing they can do. The active certificate and its renewal
are never touched, so renewal with the panel and the Agent stopped (§5) is
unchanged. The ledger rows stay as the record they are; no row is ever made
active again after its operation ended.

An owner who pressed the button on a lab build of alpha.134 (and every
server with replaced native lineages from alpha.113 on, which carry
`autorenew = False`) has two lineages; the first pass after the update
removes the replaced one by itself when nothing names it.

Limits: a configuration file outside `/etc` (an nginx `include` of a file
under `/opt`, a program that reads the live files directly) is not read; such
a lineage would be removed although that file names it. The Agent reads the
files under `/etc` only to look for the name and keeps nothing.

## 5b. A request that gave no new certificate (lab check, 14 Nov 2026)

The answer is `CERTIFICATE_ISSUE_FAILED` with the kind as `reason`, and the
page draws one guidance block: what happened (the current certificate stays
in use, or the site is served as before), why, who acts for the reader's role,
the next step and how it goes on. Kinds read from certbot's own output:
the authority refused the proof of a name (`validation`), one of its limits
(`rate_limited`), it could not be reached (`authority_unreachable`), it does
not know this server's account (`account_unknown`, RFC 8555
`accountDoesNotExist`), certbot ran out of time (`timeout`), otherwise "not
issued, the reason could not be established" (`tool`). An issued certificate
that the server's clock does not accept is `server_clock`; one that does not
pass the check before use is `issued_check` with `vars.check`: `untrusted`
(the chain is not trusted by this server: the lab's case, formerly a bare
500), `names` (other names than were asked for) or `unusable`. In every case
the new certificate is removed and the previous one keeps serving.

A customer or reseller is told what they can do (point the names here) or to
contact the server's administrator; they are never sent to a file on the
server. An administrator gets certbot's line and where certbot keeps its
record (`/var/log/letsencrypt/letsencrypt.log`; the earlier sentence named
`/var/log/celikpanel/certbot/`, which a native request does not use) as
closed technical details. Nothing asks the authority again by itself.

Still a bare 500 on this route: database errors, a lost pending-state write,
and an Agent refusal that is not a certbot run (`writeAgentError` after
`Agent.IssueLetsEncryptCertificate` without a failure kind: an invalid
domain, the challenge directory, EAB characters, a lineage identity error).

## 6. Out of scope

- **Names added later.** A name that starts to point here after the request
  (a new alias, `mail.<domain>` created later) is shown **Korunmuyor** with
  *Bu adla yeniden al*; it is not added automatically. Doing it would need a
  watcher that re-requests on DNS change and a rate-limit budget.
- **An all-sub-names (wildcard) certificate** when DNS is managed here: see
  WILDCARD-CERTIFICATE-DESIGN.md, not built. For this list it would need: the
  certificate's `*.<domain>` name (already understood by the coverage check),
  a reason "covered by the all-sub-names certificate", and its own renewal
  state per line, because its renewal depends on the challenge zone rather
  than on each name pointing here.

## 7. Schema

Migration 078 adds `domains.mail_clients_use_own_name INTEGER NOT NULL
DEFAULT 0 CHECK (0, 1)` and sets it to 1 for every domain with an active
certificate that secures mail (the domains that told `mail.<domain>` before).
Additive. The release before refuses a ledger with this entry; going back is
the pre-update snapshot restore (D-025).

## 8. Real-system check (lab, Pebble as the authority)

Component tests do not prove these; the lab runs them on a server with mail
(Postfix, Dovecot) and the panel's DNS:

1. A domain whose `mail.<domain>` A record points to the server: request a
   certificate with nothing ticked or unticked. The certificate carries
   `mail.<domain>`; `openssl s_client -connect <ip>:993 -servername
   mail.<domain>` (and `:465`, and `:587 -starttls smtp`) returns the domain's
   certificate; the same with `-servername <server mail name>` returns the
   server's certificate. The auto-setup answer
   (`/mail/config-v1.1.xml` on `autoconfig.<domain>`) still names the server's
   mail name. The list shows every line **Korunuyor**.
2. A domain whose `mail.<domain>` points elsewhere: the list says
   **Korunmuyor** with the address; the request succeeds without the name.
3. An existing certificate without the mail name (issued with "leave it out"):
   the list offers *Posta adını da kapsa*; one press, real steps; afterwards
   the line is **Korunuyor** and step 1's handshakes hold.
4. A failed request: make the authority refuse (stop Pebble's challenge
   server or point the name away mid-request). The old certificate, its
   files and the mail answers are unchanged.
5. Renewal with the panel stopped: `systemctl stop celikpanel-panel
   celikpanel-agent`, then `certbot renew --dry-run` and `certbot renew
   --force-renewal --cert-name <lineage>`; the deploy hook's record
   (`/var/lib/celikpanel-site-certificates/<lineage>`) says `mail=reloaded`
   and `dovecot=reloaded`, and step 1's handshake returns the renewed
   certificate.
