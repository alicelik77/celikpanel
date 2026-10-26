# v0.1.0-alpha.110

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.110.tr.md)

This is the release that follows v0.1.0-alpha.109. **It is a candidate, written on
2026-10-26; it is not published and nothing from it is on an installed server.
Only the owner updates installed panels, from the panel's own update screen.**
Written on the branch `feat/alpha110-webmail-origin` from `main` at the alpha.109
release commit `c7a05c7dc04cd133528e9aa4b67f21c0132369cb`.

## Who should update, and why

Every server with webmail. In alpha.109 webmail stayed on the panel's own address
behind the panel's strict security policy, so Roundcube's interface could not
start there. A script running on the panel's address could use the panel's API as
the signed-in administrator, and Roundcube has had such cross-site-scripting flaws
(CVE-2023-5631, CVE-2024-37383, CVE-2024-42009). Webmail now opens on **its own
address**, for example `https://mail.celikhost.com/`, served by the server's own
web server. What the owner does on each server is in the
[operation guidance](OPERATION-GUIDANCE.md#webmail-on-its-own-address-2026-10-26).

## Webmail on its own address

- **You choose the name.** On Components → Webmail (Roundcube) → Manage, the new
  **Webmail address** card takes any host name; it suggests the mail host name from
  Server setup. No server prefixes (such as `frankfurt.`) are needed. The panel's
  own name is refused: a browser sends the panel's sign-in cookie to every port of
  the same name.
- **Check first, nothing changes.** **Check** asks the public DNS resolvers what
  the name points to and compares it with this server's addresses. No record yet:
  the card shows the exact record to add; when this panel hosts the zone, it can
  add the record for you after you confirm it. The name points to another server:
  refused, with what DNS answered and what it should answer (the other DNS server
  is named when it is that one). DNS could not be asked: "not known", check again.
  Ports 80 or 443 closed in the firewall: the card says which and where to open
  them; CelikPanel never opens ports by itself.
- **One confirmed operation sets it up.** The server obtains a certificate for the
  name from Let's Encrypt (the same way as the mail certificate: through the web
  server on port 80; the operating system's own certbot timer renews it, and
  certbot reloads the web server after each renewal), writes one web server file
  for the name, checks it, reloads, and proves webmail answers there over HTTPS
  (its certificate, its PHP, the login page and its stylesheet). If anything fails,
  the web server file is put back as it was; the certificate is kept. The
  operation runs on the server; the screen follows it and never starts it twice.
- **Webmail no longer depends on the panel.** The address is served by nginx and
  PHP-FPM with Roundcube's own security policy for its own origin; it keeps
  answering while the panel is stopped.
- **The panel's `/webmail/`** now redirects to the address. Without an address it
  is a short page saying where to set it. Roundcube's pages are no longer served
  on the panel's address at all.
- **Links are back.** **Open webmail** on the Roundcube row, its Manage page and the
  domain's mail screen point to `https://<address>/`. Mailbox users sign in there
  with their email address and password; they need no panel account.
- **Changing or removing the address.** Changing the name sets up the new one first
  and removes the old one only after the new one works. Removing it is confirmed
  and says what happens; the certificate is kept unless you tick "Also delete its
  certificate". A web server file you changed by hand is never overwritten or
  removed; the card says so.

Not built yet (recorded as next steps): one address for mailboxes on several
servers, and customers' own `webmail.<domain>` names.

## Upgrade and recovery

No schema change (schema 46): the address is one row in `panel_settings`; the
operations use the existing service operations table (kinds `webmail_address` and
`webmail_address_remove`). Recovery to alpha.109 is the ordinary one; an alpha.109
Panel leaves an enabled address's web server file and certificate as they are
(webmail keeps answering on its address) and its `/webmail/` goes back to the
held-back proxy. To remove an address by hand: `sudo rm
/etc/nginx/conf.d/celikpanel-webmail-origin-<name>.conf`, `sudo nginx -t`, `sudo
systemctl reload nginx`.

## Evidence

- Agent 4840 passed, 29 skipped (as root in a clean mount namespace; also as the unprivileged user `nobody`); Panel 3450 passed, 6 skipped; every internal package passes; web 1947 passed, and the production build within its budgets; the race shards were not run locally (no C compiler in that guest; CI runs them). The release-recovery harness is unchanged (schema 46).
- One real-system run on a disposable Debian 13 VM from the kept base image, under
  the Agent unit's umask, with a disposable test CA instead of Let's Encrypt:
  Roundcube installed with the distribution's PHP-FPM; certbot's check answered
  through nginx on port 80; the address set up and proved; `http://` redirected;
  the login page with its own policy and secure, strict cookies; the stylesheet as
  `text/css`; CelikPanel's PHP check refused from another address; the panel's
  `/webmail/` redirected there and the redirect reached Roundcube with the
  certificate verified; removal stopped webmail on the name and kept the
  certificate ([evidence](../deploy/e2e/d038/alpha110-debian13-20261026/README.md), archive SHA-256 `cf485342…888497b6`).
- A browser look (EN and TR, 1440 and 390, light) on a mock (batch 33): the Webmail address card without an address, with no record yet, ready and its set-up confirmation, enabled and its removal confirmation; no sideways scrolling, no raw keys.

Not measured on an installed server. A candidate is not acceptance; P0.1–P0.5 stay
**partial**; no P0 item is closed or advanced.
