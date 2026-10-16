# v0.1.0-alpha.94 (candidate, not published)

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.94.tr.md)

This is the release that follows v0.1.0-alpha.93. The number was chosen by the
release preparer under the owner's standing instruction of 2026-10-13. Written
on 2026-10-17, on the branch `feat/alpha94-ready-bundles`, from `main` at the
alpha.93 release commit `0897ee9238bdfee295149adfe8b16968dd6fe0de`.

## What changed

**Ready bundles (owner decision of 2026-10-17, D-034).** The three "One-click
mail stacks" cards at the top of Components are gone. In their place, each
category has one line, "Ready bundles", between its header and its rows:

- **Mail:** Core mail, Webmail, Spam-protected mail. They install through the
  existing mail install dialogue, exactly as before.
- **Databases:** a PostgreSQL bundle and a MariaDB bundle; once one is
  installed, its browser tool (phpPgAdmin, phpMyAdmin) is offered as "Add …".
- **Web:** Nginx + PHP-FPM.

A bundle is a shortcut over the components listed below it, not a separate
product: a database or web bundle installs its missing components one after
another through the ordinary component install, each with its own progress
("2/3 · installing PHP-FPM"). If one fails, its own guidance is shown and the
rest wait; the bundle then offers Continue, which installs only what is
missing. An installed bundle collapses to one line: "… installed and verified"
(Repair as a small link for mail), or "Installed, not verified" with Verify.
The Dashboard's Mail stacks card is replaced by one calm "Ready bundles" card
that lists what is installed and opens the bundles on Components.

**Calmer Components page** (the design review of 2026-10-16):

- Restart, Stop and Repair icons in a row are grey until you point at them;
  Stop's red stays in its confirmation.
- A stopped service has a red dot and "Stopped"; "Not installed" is a hollow
  ring.
- Row Install buttons are outlined; the page keeps one filled button.
- With the firewall on and healthy, it is one line: "Firewall on · Open ports:
  4 · Settings". The full box stays for off, unreadable or needing attention.
- Component icons are line icons from one map instead of emoji.
- Versions and states line up in columns; on a phone, row buttons are 44 px tall
  with their names written, and the version is shown under the description.

## What you will see after updating to alpha.94

- Boston (nothing but the DNS engine installed): each of Web servers,
  Databases and Email starts with its bundles.
- Frankfurt: "Webmail installed and verified · Repair" with "Add spam
  protection"; "MariaDB bundle installed and verified" with "Add phpMyAdmin";
  the firewall as one line; the Dashboard's Ready bundles card.

## Limits

- No native record, by the owner's instruction of 2026-10-16 (quick releases).
  Go, component, mounted and mock-browser checks only; nothing measured on an
  installed server; the bundle sequence has not run against a real Agent.
- Schema 44, no migration; no new Agent job kind; recovery behaviour unchanged.
  The managed-services answer gains an additive `bundles` field; the page
  shows no strips when an older Panel does not send it.
- P0.1-P0.5 stay **partial**; no P0 item is closed or advanced; publication is
  not acceptance.

## Evidence

Go `./cmd/panel/` and `./internal/...` (66 packages): vet and gofmt clean; 2843 top-level tests pass, 42 skip, 4278 subtests pass, none fail; in `./cmd/panel/` 1593 top-level tests pass and 6 skip (eight new ready-bundle tests; the mail profile tests unchanged). Web: `npm test` 1575 of 1575; `tsc --noEmit` and the
production build with the bundle budget pass. Browser batch 19 (mock only):
Components on Boston-like and Frankfurt-like mocks, a database bundle at step
2/3 and the Dashboard card, at 1440 and 390 px in both themes, Turkish; one fix
round (the hint line beside an installed bundle, PHP-FPM kept whole on a
phone). The release contracts pass: sequence policy (93 -> 94), download
portal, signed manifest.

## Publication record

Not published.

Install this release only through CelikPanel's update interface. Publishing it
does not update installed servers; only the owner of a server starts its
update, from the panel's own update screen.
