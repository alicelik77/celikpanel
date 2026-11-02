# v0.1.0-alpha.116

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.116.tr.md)

A fix release after v0.1.0-alpha.115: one security fix and the problems the
owner found while using the reinstalled servers. **Only the owner updates
installed panels, from the panel's own update screen.**

## Security

- **Proxy sites can no longer point inside the server.** A site switched to
  the proxy type could name any address as its upstream, including
  127.0.0.1, private networks, the cloud metadata address or the server's
  own addresses, and so expose internal-only services through a customer's
  domain. Saving such a target is now refused with the reason
  (`PROXY_TARGET_NOT_ALLOWED`); names are resolved and every address is
  checked. Existing proxy sites are not changed. Known limit: a name whose
  DNS later changes to an internal address is not caught (nginx resolves it
  again later).

## Fixes

- **Installer refuses unsupported systems first.** Kali, Linux Mint, older
  Debian/Ubuntu releases, openSUSE/SLES and the Red Hat family are refused
  before anything changes, with a bilingual message naming the system and the
  supported list (Debian 12/13, Ubuntu 22.04/24.04, Arch Linux).
- **Server setup gets the mail certificate before installing mail.** On a
  server without mail, setup now issues the trusted mail certificate first,
  so Postfix and Dovecot start with it. Servers that already run mail keep the
  previous order.
- **No misleading certificate warning during setup.** The "self-signed
  fallback" warning is no longer shown for setup steps; outside setup its text
  now says only what the install proves.
- **VPN page for resellers and customers.** The menu shows VPN only when it is
  offered to that account and WireGuard is installed; otherwise one plain
  sentence. The contradictory "needs attention / 0 pending, 0 failed" box and
  the administrator-only details are gone for them.
- **Quota gaps closed.** Creating a database from the domain page now counts
  against the plan; a cPanel import can no longer exceed mailbox or database
  limits; editing a plan below current usage shows the affected accounts and
  asks for confirmation (nothing is deleted).

## Not covered

Automatic checks (web, Go, release and distribution) and one short lab
install of a fresh Debian 13 server with mail. No other real-server trial.
