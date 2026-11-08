# Other mail server settings, one at a time (design note, alpha.130)

Roadmap alpha.130 Mail item 2 asks for "the other mail server settings one at
a time, on demand". In alpha.130 **none of them is shipped**: the release put
its time into the smarthost and the self-configuring mail programs, and the
owner's rule is that a finished, tested small item beats half of a big one.
This note is what each one needs, in the order they should be built.

## (a) Subaddressing (`user+tag@domain`) — next, small

- Postfix: `recipient_delimiter = +` in `main.cf`. Lookups in
  `virtual_mailbox_maps`/`virtual_alias_maps` then try `user+tag@domain`
  first and `user@domain` second; `virtual(8)` delivers into the user's
  mailbox. A mailbox whose name really contains `+` still wins (exact match
  first).
- Dovecot (mailboxes delivered through LMTP, i.e. with an autoresponder):
  `recipient_delimiter` must be the same character, otherwise the userdb
  lookup of `user+tag@domain` fails. Dovecot 2.3's default is already `+`;
  the Agent must read it (`doveconf -h recipient_delimiter`) and refuse when
  it differs, never rewrite it silently. `lmtp_save_to_detail_mailbox` stays
  `no` (no surprise folders).
- Owner edits: an existing `recipient_delimiter` with another value is the
  owner's: kept and reported. Off removes only CelikPanel's line.
- Catch-all and forwardings: unchanged (lookups fall back to `user@domain`).
- Screen: one switch under Mail server settings, administrator only.
- Tests: the Postfix double used by `mail_relay_test.go`; a lab delivery to
  `user+tag@` on Debian 13 and Ubuntu 24.04, both delivery paths.

## (b) Allow and block lists for senders

- Server-wide: a CelikPanel `check_sender_access` map placed in
  `smtpd_sender_restrictions` (or `smtpd_recipient_restrictions` before
  `permit_mynetworks` decisions), `OK`/`REJECT` per address or domain, with
  Rspamd's own allow/deny maps kept apart.
- The existing per-domain options (`mail_domain_options`, recipient access
  map) show the shape; per-domain sender lists need a restriction class per
  recipient domain (`smtpd_restriction_classes`) — a separate design.
- Risk: an allow entry bypasses spam checks for forged senders; it must match
  on SPF-authenticated senders only or be documented as such.

## (c) DKIM key rotation

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

## (d) Dovecot limits

- `mail_max_userip_connections` (IMAP default 10) and friends in a
  CelikPanel drop-in under `conf.d/`, with owner-edit detection like the
  other drop-ins, `doveconf -n` check before reload.
- Needs a decision on safe bounds (phones with several folders open hit 10
  quickly) and per-protocol values.
