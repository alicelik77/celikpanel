# Mail programs that set themselves up (alpha.130)

Thunderbird, Outlook and most phone mail apps ask fixed addresses for a
domain's mail settings. When they get an answer, the person types only the
address and password.

## What is answered, and where

For a **top-level domain whose mail is on this server** (Postfix and Dovecot
installed, the server's mail name set, a website on this server, the
subscription not suspended), the domain's own nginx site file answers:

| Address | Who asks | Method |
| --- | --- | --- |
| `http(s)://autoconfig.<domain>/mail/config-v1.1.xml` | Thunderbird | GET |
| `http(s)://<domain>/.well-known/autoconfig/mail/config-v1.1.xml` | Thunderbird | GET |
| `http(s)://autodiscover.<domain>/autodiscover/autodiscover.xml` | Outlook (POX) | POST and GET |

The answers are fixed texts written into the site file with nginx `return
200` (no PHP, no panel process): they keep working with the panel stopped or
removed (D-022). They hold:

- the mail server name (the domain's `mail.<domain>` when the domain's
  certificate carries it for mail, otherwise the server's mail name, which
  always has a certificate);
- IMAP 993 over TLS; sending on 465 (TLS) and 587 (STARTTLS); sign-in
  required, user name = the whole address.

They never hold a password. The address an Outlook request carries is never
written back: the answer has no `LoginName`, so Outlook uses the address the
person typed; Thunderbird fills `%EMAILADDRESS%` itself. POP3 is not offered
in the answers (IMAP is); the manual settings still show it.

`autoconfig.<domain>` and `autodiscover.<domain>` get their own server block
in the same file: port 80 always (with CelikPanel's certificate validation
include), port 443 only when the site's active certificate covers both names.
Everything else on those names answers 404; the website is never served
there. A site without the feature renders byte for byte as before, and a
name that is already a domain, alias or the webmail address on this server is
never taken.

## DNS

- Every **new zone** gets `autoconfig` and `autodiscover` CNAMEs to
  `mail.<domain>` and `_autodiscover._tcp.<domain> SRV 0 0 443
  autodiscover.<domain>`. They go with the zone: deleting the domain deletes
  them (no leftovers). PowerDNS and BIND both publish them from the panel's
  zone records.
- An **existing zone** shows what is missing on the mailbox screen; **Set up
  now** adds only the missing records. A name that already holds another
  record is reported and never replaced.
- DNS run elsewhere (external provider or another server): the screen lists
  the records to add there.

## Certificate

A certificate request of the site adds the two names **only when the public
DNS shows them here** (the same check as the other names). Names that do not
point here yet are left out with a plain note and the certificate is issued
as before; requesting it again later adds them. A site file the owner kept
is not changed, so the names are left out then too. Once in the certificate,
the names are renewed with it (if they are later pointed elsewhere, renewal
reports it like any other name).

## Screens and API

- Mailbox screen (domain → Mail → Settings): the state at the top ("set
  themselves up" / what is missing and who acts), the manual settings below as
  the fallback.
- `GET /api/v1/domains/{id}/mail/setup` carries `autoconfig`: `state` (on,
  partial, off, unavailable), `reason`, `names`, `mail_host`, `served`/`secure`
  (yes, no, unknown — read locally from this server's nginx), `dns`, `records`,
  `can_fix`.
- `POST /api/v1/domains/{id}/mail/autoconfig` (steps: check, site, dns)
  writes the site file again and adds missing records on this server's zone.

## Known limits

- Subdomains are not set up (people use the manual settings there).
- When mail is installed on a server that already has sites, a site picks up
  the answers at its next render (panel start, any site save) or with **Set up
  now** on its mailbox screen.
- A site file the owner kept (D-031) is never changed; its owner decides.
