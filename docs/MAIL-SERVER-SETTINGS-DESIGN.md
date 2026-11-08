# Other mail server settings, one at a time (alpha.130)

Roadmap alpha.130 Mail item 2 asks for "the other mail server settings one at
a time, on demand". alpha.130 **ships subaddressing and the server's sender
allow/block list**. DKIM key rotation and Dovecot limits stay design notes
below; the owner's rule is that a finished, tested small item beats half of a
big one. All of it is administrator only, under Postfix → Other mail server
settings, server-wide.

## (a) Subaddressing (`user+tag@domain`) — shipped

- Postfix: `recipient_delimiter = +` in `main.cf`. Lookups in
  `virtual_mailbox_maps`, `virtual_alias_maps`, transport and access maps
  then try `user+tag@domain` first and `user@domain` second; mail lands in
  the user's inbox. A mailbox whose name really contains `+` still wins.
- Dovecot (mailboxes delivered through LMTP, i.e. with an autoresponder)
  must strip the same character: the Agent reads `doveconf -h
  recipient_delimiter` and refuses to turn it on when it is not `+` (or not
  readable). CelikPanel never writes Dovecot. Dovecot's default is `+`
  (2.4.1 on Debian 13 measured). No folder delivery
  (`lmtp_save_to_detail_mailbox` stays off).
- CelikPanel's line is marked by `/etc/postfix/celikpanel-subaddressing`.
  **Debian's and Ubuntu's own `main.cf` already set `recipient_delimiter =
  +`** (measured on Debian 13): the screen says subaddressing is on, set
  outside CelikPanel, and keeps that line; turning it off then says the
  command the administrator runs. Another value set by the owner is kept and
  reported; CelikPanel's line changed on the server is kept and reported.
- Off removes exactly CelikPanel's line and the marker; a reload Postfix
  refuses puts `main.cf` and the marker back.
- API: `GET /api/v1/mail/server-settings`, `POST /api/v1/mail/subaddressing`
  (steps: check, save, apply). Agent: `MailSubaddressingV1`,
  `MailServerSettingsStatusV1`.
- Per domain: not offered (Postfix has one delimiter for the server).

## (b) Sender allow and block list — shipped, server-wide

- `/etc/postfix/celikpanel_sender_access` (marker and digest, like the
  per-domain options map), named by `check_sender_access <type>:<map>` at the
  front of `main.cf` `smtpd_sender_restrictions`.
- Block: `550 5.7.1 …` for an address or a whole domain (Postfix's default
  `parent_domain_matches_subdomains` includes its subdomains). Allow: `DUNNO`,
  which only stops the lookup — an exception to a block (one address of a
  blocked domain). **The map never says OK**, so it can never open a relay or
  skip DNSBL/Rspamd checks; "allowed" senders are still spam-checked. That is
  said on the screen.
- A domain of this server (or an address of one, or a parent of one) cannot
  be listed: its mail is protected by sign-in and an entry would also stop
  this server's own mailboxes (submission reads the same sender
  restrictions).
- An empty list removes the element (and `smtpd_sender_restrictions` when
  nothing else is in it), the map and its index — no leftovers. A map changed
  on the server or a `smtpd_sender_restrictions` that names a variable is
  kept and reported. A refused reload puts the files back.
- API: `POST /api/v1/mail/sender-rules` `{"op":"add","value","action"}` /
  `{"op":"remove","id"}` (steps: check, save, apply; the whole list is
  applied). Agent: `MailSenderListV1`. Migration 077 tables
  `mail_server_options`, `mail_sender_rules`.
- **Per domain for customers: not built.** `check_sender_access` keys on the
  sender only; a list that applies only to mail for one recipient domain
  needs one `smtpd_restriction_classes` class (one main.cf parameter and map)
  per domain, selected by a recipient access map. That is a separate design
  with its own owner-edit and cleanup rules; it is not shipped half.

## (c) DKIM key rotation — design note

- A second selector (date-based, e.g. `cp202611`) is generated, its TXT is
  published and **verified in public DNS** before OpenDKIM's signing table
  switches to it; the old selector's TXT stays for a grace period (7 days) so
  mail in flight still verifies, then is removed together with its key file
  (no leftovers).
- Today one selector (`dkimSelector`) is fixed across the Agent
  (`EnsureDKIMKey`, signing table) and the Panel (records, health check); the
  rotation needs a stored per-domain selector, a resumable state machine
  (`publish → verify → switch → grace → remove`) and remote/external DNS
  handling (the owner publishes; the switch waits). Too large to finish safely
  in this release.

## (d) Dovecot limits — design note

- `mail_max_userip_connections` (IMAP default 10) and friends in a
  CelikPanel drop-in under `conf.d/`, with owner-edit detection like the
  other drop-ins, `doveconf -n` check before reload.
- Needs a decision on safe bounds (phones with several folders open hit 10
  quickly) and per-protocol values.
