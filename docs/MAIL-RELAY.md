# Sending mail through another server (smarthost, alpha.130)

The administrator can make every message this server sends to other servers
leave through one relay (an ISP's or a mail service's SMTP server), signed in
with a user name and password over an encrypted connection.

**Server-wide only.** A relay for one domain only (Postfix
`sender_dependent_relayhost_maps`) is not offered: it needs per-sender
authentication and a second map whose lines must match every sending address
form, and a wrong line sends a customer's mail through another customer's
relay. It can be designed later on its own.

## What CelikPanel writes on the server

| Where | What |
| --- | --- |
| `/etc/postfix/celikpanel-relay-sasl` (+ its postmap index) | `[host]:port user:password`, root-only (0600; the index is built under `umask 077`) |
| `master.cf` service `celikpanel-relay/unix` | Postfix's `smtp` client with its own `-o` options: SASL login (`noanonymous`), `smtp_tls_security_level=encrypt`, the map above, and `smtp_tls_wrappermode=yes` for port 465 |
| `main.cf` | `default_transport = celikpanel-relay`, `relayhost = [host]:port` |

The distribution's own `smtp_*` settings stay as they were. Order: map,
service, `default_transport`, `relayhost`; removal is the reverse. So an
interrupted run never looks like the owner's relay, and a `relayhost` without
`default_transport = celikpanel-relay` is always the owner's.

Postfix reads all of it natively and keeps relaying with the panel stopped or
removed (D-022). Outgoing mail limits (postfwd) keep counting: they are asked
at the end of each message on 587/465, before the message leaves.

## Owner edits

- A `relayhost`, `default_transport`, `sender_dependent_relayhost_maps` or
  `sender_dependent_default_transport_maps` the owner set: CelikPanel refuses
  to turn its relay on and changes nothing; turning off leaves them alone.
- CelikPanel's own service or map changed on the server: kept, the save is
  refused with the command that moves it aside.
- A reload Postfix refuses puts the earlier `main.cf`, `master.cf` and map
  back and reloads them; if even that fails the screen says what to run.

## Turning it off

Removes `relayhost`, `default_transport`, the service, the map and its index
(no leftovers), and forgets the sealed password in the panel.

## Prerequisite

Postfix must be able to sign in: Cyrus SASL client support and a PLAIN or
LOGIN module (Debian/Ubuntu `libsasl2-modules`, Arch `libsasl`). Missing, the
save is refused with the command; nothing is installed behind the owner's
back.

## Panel

- `GET /api/v1/mail/relay` — settings (never the password: `has_password`),
  last apply, what the server holds (`off`, `on`, `owner`, `edited`,
  `partial`, `sasl_ready`).
- `POST /api/v1/mail/relay` — save and apply (steps: check, save, apply). An
  empty password keeps the saved one only for the same host and user name; a
  saved password is never sent to another server.
- `POST /api/v1/mail/relay/test` — connects, starts TLS as Postfix will,
  signs in and says QUIT. It has no message commands at all, so it cannot send
  mail. It reports whether the relay's certificate is valid for its name
  (Postfix's `encrypt` level encrypts without checking that).
- Migration 079 `mail_relay_settings`: one row; the password sealed with the
  panel's secret key (`internal/secrets`).
