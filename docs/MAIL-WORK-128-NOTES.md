# Mail work in alpha.128: what shipped and what is a design note

Workstream "mail" of alpha.128 (gaps G1, G12, G20, G25, G9, G30 of the Plesk
gap list). Shipped items have their own commits; the rest are design notes
here, because each would have been a half-built security or delivery feature
tonight.

## Shipped

- **G1 outgoing limits and the security pause:** docs/OUTGOING-MAIL-LIMITS.md.
- **G12 plan mail settings:** the largest mailbox per plan (enforced for
  resellers and customers on create and resize; the administrator may go
  above), the plan's outgoing values (mailbox ceiling, domain and hosting
  account totals). Table `mail_plan_limits` (migration 067); keys for the
  generic ceilings model (066): `mail.mailbox_size_mb`,
  `mail.outgoing.mailbox`, `mail.outgoing.domain`, `mail.outgoing.subscription`.
  Editing is the administrator's (Mail server page, "Mail limits of hosting
  plans"); resellers' own plans and Custom subscriptions in the plan form come
  with the 066 integration.
- **G20 (part):** aliases shown under each mailbox with "Add an alias" (the
  forwarding form, prefilled); "Empty mailbox" through `doveadm expunge` with
  the panel's own confirmation, the server's steps and a request identity.
- **G1 off by default:** after the update a server sends exactly as before;
  nothing reaches the Agent and no log is read until the administrator saves
  the limits on (postfwd is installed only from Components, with its preview).
  Turning them off unwires Postfix first and leaves postfwd installed.
- **G20 bulk actions:** select mailboxes, then change the sending limit, empty
  or delete them; one confirmation, one server call and one step per mailbox;
  a refusal is shown and the rest go on, a lost answer stops the run.
- **G25 per-domain options:** "send only" (550 5.7.1 for mail from other
  servers; the domain's mailboxes keep sending) and an own reply for unknown
  addresses (known addresses DUNNO, the domain 550 5.1.1 <text>; refused
  beside a catch-all), one CelikPanel recipient access map named at the front
  of smtpd_recipient_restrictions only while it has lines; the map can never
  say OK; an owner-changed map is kept. Table domain_mail_options (067).
- **G30 (part):** the connections one sending server may hold at once
  (smtpd_client_connection_count_limit) in the versioned mail policy; the
  largest message size was already there.
- **G9 delivery lookup:** `GET /api/v1/domains/{id}/mail/delivery`, read from
  Postfix's journal (7 days), filtered by the domain inside the Agent (an
  incoming message shows only this domain's recipients) and again in the
  panel.

## Design notes (not built in 128)

### G12 autodiscover / autoconfig

- Clients ask `https://autoconfig.<domain>/mail/config-v1.1.xml`
  (Thunderbird) and `https://autodiscover.<domain>/autodiscover/autodiscover.xml`
  (Outlook, POST). The answer is static per domain: IMAP 993 SSL, SMTP 465
  SSL / 587 STARTTLS, username = full address, server = the mail host name
  (the one in the mail certificate).
- Native: one static file per domain under the site's own tree, served by an
  nginx `location` in the domain's vhost (autodiscover needs POST → a fixed
  `return 200` body with `default_type application/xml`), plus DNS records
  `autoconfig` / `autodiscover` CNAME to the mail host and an `_autodiscover._tcp`
  SRV, through the existing record path; certificates must cover the names.
- Why not tonight: it changes the vhost template the web workstream (068) is
  changing in the same release, and needs certificate SAN handling. It joins
  the 129 template change.

### G20 recovery address

- Recovery address and mailed reset link: an unauthenticated reset flow for a
  mailbox. Needs a token table (hashed, 30-minute, single use), rate limits per
  address and per IP, a public page outside the panel's session, and a mailed
  link only to an address confirmed by the mailbox owner. Security work; it is
  not to be shipped half-done, so it waits for its own review (131+).

### Lab round still open for G25

The map and restriction were tested against fakes of postconf and the mail
file transaction; a real Postfix run on the five systems (send-only refusal on
port 25, a domain mailbox still sending on 587, the unknown-address text) is
the lab round.

### G30 more mail server settings

One native setting at a time with the keep-owner-edits pattern, smarthost
first: `relayhost = [host]:587` with `smtp_sasl_auth_enable`,
`smtp_sasl_password_maps = hash:/etc/postfix/celikpanel_relay_passwd` (0600,
`postmap`), `smtp_tls_security_level = encrypt`; a test message before it is
kept, the earlier values put back if Postfix refuses. Then Dovecot connection
limits (`mail_max_userip_connections`), subaddressing
(`recipient_delimiter = +` with Dovecot `lmtp_save_to_detail_mailbox`), DKIM
selector rotation (new selector published and verified in DNS before signing
switches), mail antivirus (ClamAV milter, only with the spam filter seat
clear), allow/block lists (`check_sender_access`), custom quota warning text.
Of these only the Postfix connection limit shipped in 128.
