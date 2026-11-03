# v0.1.0-alpha.117

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.117.tr.md)

> **Published** on 2026-11-03 (UTC): release commit `b22348ff8`, GitHub pre-release at 02:08:57Z, download portal at about 02:13Z (public check 27/27).

The plans-and-rights release (D-042) and the security emergency (D-043), as the
owner approved them on 2 November 2026. **Only the owner updates installed
panels, from the panel's own update screen.**

## Plans, rights and the playing field (D-042)

- **Installed is not granted.** What an account may use is the intersection of
  what is installed, what the administrator offers, the reseller's rights and
  the account's rights. A reseller can only narrow.
- **Every account shows its state by name:** Unlimited, Plan: <name>, or
  Custom. Changing one item of a plan-bound account makes it Custom (asked
  first). Two built-in plans, Unlimited and Unlimited reseller, take everything
  newly offered.
- **Rights have three parts:** quantities (domains, databases, mailboxes,
  disk), permissions (DNS editing, DNSSEC, mail, databases, scheduled tasks,
  file manager, backups, free and own certificates, protected folders and
  redirects, proxy sites, app installer, VPN, team members, file access, SSH)
  and software (PHP and Node.js versions, database engines, webmail,
  certificate providers). The server enforces them; screens hide what an
  account cannot use.
- **The reseller pool limits real usage**, not the sum handed out; warnings at
  80% and 100%.
- **Creating an account** always shows a Rights section (Unlimited preselected
  by name, a plan, or custom). Plan edits that narrow show the affected
  accounts first.
- **Taking a right away keeps what runs:** existing sites, mailboxes and
  databases keep working; nothing new of that kind can be created.

## Subscriptions

- **Suspension.** When an administrator or reseller suspends a subscription, or
  its end date passes: sites show "This site is suspended", mail is refused
  temporarily (450, senders retry), and mailbox logins, database users, VPN
  devices, scheduled tasks, Node.js apps and file access stop. Nothing is
  deleted; resume restores exactly. Suspending a reseller suspends its
  customers. Suspended subscriptions appear in a Deletable list; deletion
  stays manual.
- **End dates** can be set or cleared (empty = never ends), ready for a later
  billing system.
- **Transfer.** An administrator transfers any subscription; a reseller between
  its own customers; a customer sends a request that its reseller or the
  administrator approves. The receiving side's rights and pool are checked
  first. "Change all passwords and keys" is on by default: site accounts, SSH
  keys, mailbox and database passwords (a WordPress installed by the panel is
  updated), protected-folder users and VPN devices.

## File access

- Per site: FTP over TLS and SFTP (one right), and SSH (a separate right)
  inside a jail of the site's own folder with a small tool set. Plain FTP is
  never offered. New site users have no login shell.

## Security emergency (D-043)

- For a PHP or Node.js version or a database engine: withdraw (no new use),
  emergency stop (service locked, sites on a maintenance page), bulk move to
  another version, emergency remove. Strong confirmation (affected list,
  reason, typed name, password). Affected customers and resellers get a panel
  notice and an e-mail with a one-click switch. Customer data is never
  deleted. Re-enable restores.

## Other fixes

- Setup starts Postfix and Dovecot on the trusted mail certificate from the
  first moment; packages no longer start with their own defaults during
  installation (the FTP server included).
- "Get certificate" is closed, with the reason and the fix, when the domain
  does not point to this server yet; "behind a proxy such as Cloudflare —
  request anyway" is available and audited.

## Known limits

- Traffic is not measured, and disk is not limited by the operating system;
  limits act on new creation.
- A rollback to alpha.116 restores the database copy taken before the update:
  anything done in the panel after the update is lost, and a suspension or
  emergency applied on the server in that time is not undone by the rollback.

- A command sent over SSH without an interactive login (for example
  `ssh host 'pwd'`) starts at the jail's root, which cannot be listed; an
  interactive login starts in the site's own folder.

## Evidence

Linux Go suites (82 packages; the known guest-only Agent failures also fail on
main), 2305 web tests, release-recovery fixtures extended to schema 56, and a
lab runs 1–3 (fresh install, rights, file access, suspension, transfer, update from alpha.116, SSH jail) recorded in the release record.
