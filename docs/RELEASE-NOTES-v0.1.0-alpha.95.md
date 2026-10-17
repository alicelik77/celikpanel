# v0.1.0-alpha.95 (candidate, not published)

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.95.tr.md)

This is the release that follows v0.1.0-alpha.94. The number was chosen by the
release preparer under the owner's standing instruction of 2026-10-13. Written
on 2026-10-17, on the branch `fix/alpha95-bundle-consistency`, from `main` at the
alpha.94 release commit `d506a05e8c003dffee8de2e1268402865520bd6b`.

## Who should update, and why

Everyone who runs alpha.94. After updating alpha.94 the owner looked at
Components and the Dashboard on Boston and Frankfurt and asked for the problems to
be evaluated again and fixed (owner decision of 2026-10-17, D-034 amendment). This
release is that correction. Only the owner of a server updates it, from the
panel's own update screen.

## What changes on screen

**The same bundle list on every server.** Every Components category (Email,
Databases, Web servers) shows the same ready bundles in the same order on every
server. Only the state on each chip changes. This withdraws alpha.94's rule
"the installed bundle plus at most one more offer", which was the release
preparer's mistake: on Boston it hid the MariaDB bundle although it could be
installed (and the Dashboard counted it), and Core Mail was a chip on Boston and
missing on Frankfurt.

A chip says its state in words and in form, never by colour alone:

- a check mark and the name: installed and verified;
- "Core Mail · included": a verified mail bundle contains it;
- a grey dot and "not verified": installed, not proven yet;
- outlined "Name · Install" (add-ons: "· Add"): can be installed;
- "n of m · Continue": a mail install was started and is not complete;
- muted text with a short reason and no button ("phpMyAdmin · needs MariaDB"):
  cannot be installed yet; add-ons always appear;
- "2/3 · PHP-FPM installing" with a moving indicator while a bundle installs; the
  other chips are disabled meanwhile.

Pressing an installed chip opens a dialogue: **Repair** for a mail bundle (it
completes what is missing and verifies the setup; nothing installed is removed),
**Verify** for a component bundle (it reads the components again and changes
nothing). The strip has no Repair links and no hint sentence. If the components
were not checked recently, the strip is one muted line and the page checks by
itself.

**The Dashboard card** is one summary line ("7 installed and verified · 1 can be
installed"), then only the problem lines (a failed or running mail attempt with
its reason, a bundle installed but not verified), then "Open in Components". It
counts the way the chips are drawn: every bundle including add-ons; "can be
installed" is available or partial, blocked is not counted; "included" counts as
installed and verified. The sentences for never checked, checking, held, could not
check and not recent are kept.

**Three review items of alpha.94 are closed.**

1. The check between a bundle's install steps now reads the "state could not be
   verified" flag the moment it changes, so one screen refresh can no longer miss
   it.
2. Verify of a component bundle is held, with the reason shown, while a component
   operation is running on the server; you press it again when the operation ends.
   It stays usable while changes are held for other reasons, because it only reads.
3. While the page checks the server before an install and the request has not been
   accepted yet, the chip says "Checking the server before installing X…"; it says
   "installing X" only after the install is accepted.

**Rows on Components.**

- The Stop icon is a stop sign in a circle (the empty square read as a checkbox).
  Restart, stop, start, repair and delete share one outlined icon button, grey at
  rest and coloured only under the pointer or keyboard focus. The same Stop icon
  is used on the component page and the hosting type panel.
- The desktop version column of dashes is gone. A version appears as small
  monospaced text under the description where the scan knows it (PHP-FPM and
  Node.js today).
- "PHP-FPM" no longer breaks at its hyphen in requirement badges and chips.
- "Repair webmail" stands on the Roundcube row only when the server reports a
  webmail problem; otherwise repair is in the Webmail chip's dialogue.
- **Roundcube leftovers.** The Panel cannot tell whether files from an
  interrupted Roundcube install or removal are still on the server, so the row
  carries a muted note and a neutral "Clean up leftovers…" button. The
  confirmation lists what the Agent removes: `/var/lib/celikpanel-webmail`
  (Roundcube's files, its own settings and address-book database),
  `/var/lib/.celikpanel-webmail.stage`, `/var/lib/.celikpanel-webmail.retired*`,
  `/var/lib/..celikpanel-webmail.stage.retired*` (copies from an interrupted install
  or removal), `/etc/nginx/conf.d/celikpanel-webmail.conf` (then Nginx is reloaded)
  and `/run/celikpanel-webmail.sock`; and it says that mailboxes and Maildir data,
  other sites, Nginx, PHP-FPM and packages are not touched. The red danger style is
  only on the dialogue's confirm button.

**Counts.** The sidebar counts for Components and Domains are plain muted numbers
instead of filled pills (a count that is not known is still a dash, never 0). The
Dashboard's Domains count now includes the infrastructure DNS zone the Domains page
lists (on Frankfurt the Dashboard said 0 while the page listed the zone). The
Dashboard sentence when nothing needs attention now reads "Nothing needs your
attention. CelikPanel checked the certificates, the firewall and the installed
components." and, as before, appears only when every read was answered and the
components scan is fresh.

## What does not change

- Installing a database or web bundle: still one ordinary component install at a
  time, each only after the previous one has ended with a fresh scan; the first
  failure, another operation or a closed gate stops it with the reason; Continue
  installs only what is missing; nothing continues by itself after a reload.
- A mail bundle still opens the existing mail install dialogue and runs the same
  route.
- Schema 44, no migration; no Agent change; no new Agent job kind; the updater,
  rollback and recovery behave as in alpha.94. No native record, because the
  updater is unchanged (and by the owner's instruction of 2026-10-16).
- Installed panels are updated only by their owner, from the panel's own update
  screen.

## Known gaps

- **Versions are not read for every component.** The Panel gets a version only
  for PHP-FPM and Node.js. The Agent also probes a binary per service unit, but that
  is not a package version, is unreliable for several units (Apache, `named`,
  PostgreSQL) and does not cover tools without a unit (Certbot). Real package
  versions need a new Agent read; this release adds none. Open roadmap item: read
  installed package versions for every component.
- **The sidebar Domains badge** still counts only domains, while the Dashboard and
  the Domains page now include the infrastructure DNS zone. A small remaining
  difference (the badge shows no number for 0).
- **Roundcube leftovers cannot be detected.** The Panel has no field that says
  whether leftovers exist, so the button is always offered on the Roundcube row and
  the note says "may still be".
- Nothing was measured on an installed server; the bundle sequence has not run
  against a real Agent. P0.1-P0.5 stay **partial**; no P0 item is closed or
  advanced; a candidate is not acceptance.

## Evidence

Web: the changed and new contract and mounted tests (ready-bundles-ui-contract and
ready-bundles-mounted on Boston- and Frankfurt-shaped payloads; installed-alpha94-
corrections; the Dashboard card, row layout, remote-state, mail profile and service
availability contracts); Go: the bundles test (an add-on before its parent needs
MariaDB or PostgreSQL) and the mail profile frontend contract. The release
contracts (sequence policy 94 -> 95, download portal, signed manifest) pass. Final
test counts are recorded by the release preparer when the candidate is merged.
Browser mock only; nothing on an installed server.

Install this release only through CelikPanel's update interface. Publishing it does
not update installed servers; only the owner of a server starts its update, from
the panel's own update screen.
