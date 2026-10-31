# Certificates and mail — evidence (2026-10-30)

Branch `fix/cert-mail-known-gaps` (4153fbe63, 0ee5255da), merged into
`release/alpha113`. No installed server, real CA, licence service or
celikpanel.net was contacted. `matrix/` holds the distro config matrix
logs; `boot-ubuntu-24.04/` and `boot-arch/` the real-boot runs
(`deploy/e2e/cert-mail-boot/lab.sh` + `guest.sh`).

**Mail SNI on every release (matrix).** Native certbot archive/live layout,
real Dovecot and Postfix started per release. For each SNI name IMAPS 993 and
SMTP STARTTLS serve that name's certificate, a client without SNI gets the
default one, and after the live links switch as `certbot renew` does, the
real site deploy hook reloads and the renewed certificate is served.

| Release | Dovecot | Result |
|---|---|---|
| Debian 12 | 2.3.19 | pass |
| Ubuntu 22.04 | 2.3.16 | pass |
| Ubuntu 24.04 | 2.3.21 | pass |
| Debian 13 | 2.4.1 | pass |
| Arch | 2.4.5 | pass |

The Dovecot 2.3 `ssl_cert = </…` form needed no product change. Arch Dovecot
and Arch MariaDB are now measured (no N/A labels); the full five-release
matrix exits 0.

**Real boot, no route out, local Pebble CA.** The Agent's own code issued the
certificate and configured mail, then the CelikPanel binary was removed.

| System | certbot | Result |
|---|---|---|
| Ubuntu 24.04 | 2.9.0 deb, `certbot.timer` | the timer fired on its own and renewed |
| Arch | 5.8.0, `certbot-renew.timer` (enabled by issuance) | the timer fired on its own and renewed |

On both the hook record read `nginx/mail/dovecot=reloaded`; https, SMTP and
IMAPS served the renewed certificate; mail configuration stayed
byte-identical; after a reboot the timer was still enabled and the renewed
certificate still served. Lab limits: Pebble forgets accounts on restart, so
renewal happened in the issuing boot; Arch used 10-minute certificates
(certbot 5 renews at one third of the lifetime), Ubuntu 1-day ones.

**Not evidenced here.** The move of a certificate from an earlier release's
panel store into a native lineage (component tests only; a fresh install has
no such certificate); ZeroSSL/Google certificates are not moved automatically
(their EAB credentials are not stored — the card says to reissue once);
snap certbot.
