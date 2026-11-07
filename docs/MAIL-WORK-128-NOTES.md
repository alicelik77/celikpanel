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

### G20 bulk actions and recovery address

- Bulk: a checkbox column, "Remove selected" and "Set the sending limit of
  selected" as one confirmed operation with one step per mailbox; the server
  side exists per mailbox, so this is screen work plus a batch endpoint that
  records each mailbox's outcome (no partial silence).
- Recovery address and mailed reset link: an unauthenticated reset flow for a
  mailbox. Needs a token table (hashed, 30-minute, single use), rate limits per
  address and per IP, a public page outside the panel's session, and a mailed
  link only to an address confirmed by the mailbox owner. Security work; it is
  not to be shipped half-done, so it waits for its own review (131+).

### G25 per-domain mail service options

- "Incoming off, send only": the domain leaves `virtual_mailbox_domains` while
  its mailboxes keep SASL login (Dovecot passdb) so submission still works;
  port 25 then refuses RCPT for the domain (`550 5.1.1`). Preview: the MX
  record is shown as no longer used; mailboxes and their mail are never
  deleted. Owner edits of the map files kept (D-031).
- "Bounce unknown users with a message": a `check_recipient_access` map per
  domain returning `550 5.1.1 <text>` for unknown local parts, only when no
  catch-all forward is set; the text is fixed and bilingual.
- Why not tonight: both touch the mailbox-domain maps that mail enrollment and
  domain deletion also write; they need the lab round on all five systems.

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
None was started in 128.
