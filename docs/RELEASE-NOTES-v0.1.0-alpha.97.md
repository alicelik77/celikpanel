# v0.1.0-alpha.97 (candidate, 2026-10-21)

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.97.tr.md)

This is the candidate that follows v0.1.0-alpha.96. It is not published yet, and no
installed server has been updated to it. The number was chosen by the release
preparer under the owner's standing instruction of 2026-10-13. Written on
2026-10-21, on the branch `feat/alpha97-component-versions`, from `main` after the
publication record of alpha.96 (the alpha.96 release commit is
`08faa91e5a8c6622072b40f0f87c9c388035c9a3`).

## Who should update, and why

Everyone who runs alpha.96 or an earlier release, once this release is published.
On the owner's alpha.94 screens of 2026-10-17 the Components page showed a dash as
the version of every single-version component; alpha.95 removed that column and
left "read installed package versions for every component" open. This release reads
them (D-036). It also explains, and corrects how the page says, why Frankfurt's
update to alpha.95 stopped: the server's package manager was busy with Debian's
automatic package updates. Only the owner of a server updates it, from the panel's
own update screen.

## What changes on screen

**Real versions on Components.** An installed component's row says the version its
package database reports, in its short form, in mono: Nginx `1.26.3` for
`1.26.3-3+deb13u1`, MariaDB `11.8.3` for `1:11.8.3-0+deb13u1`. The tooltip has the
full version and "from the system package database". On a desktop the versions
stand in their own column, drawn only when at least one installed component on the
page has a known version; when none is known the page looks as in alpha.95, with no
column of dashes. On a phone the version is under the description. PHP-FPM and
Node.js keep the versions of their installed copies.

**The full version on every component page.** Under the component's name and
status: "Installed version: 1:11.8.3-0+deb13u1" and "Read from the system package
database." This is on the generic component page and on the specialised ones
(Nginx, Postfix, Fail2ban, PowerDNS, MariaDB, PostgreSQL…) alike.

**Unknown stays unknown.** A component that is not installed, or whose version could
not be read, says nothing: no dash, no "default", no guess. A component whose
packages report different versions (for example SpamAssassin and its milter) says
"several versions" on its row, and its page names each package with its version.
When the server's package manager was busy at the last component check, the earlier
reading is shown and said as such ("Read from the system package database on
{time}. At the last check the package manager was busy, so this earlier reading is
shown; the next check reads it again.").

**Why an update stopped, in plain words.** Frankfurt's update to alpha.95 stopped on
2026-10-17 because the server's package manager was busy (Debian's automatic package
updates were running). The stop dialog now says so: "Reason: the server's package
manager was busy, for example with automatic security updates. Nothing was
changed." and "Start the update to {target} again when it has finished; the update
card in Settings says when the package manager is free. Nothing starts by itself."
The update card's notice about such an earlier attempt says the same, calmly, and
its readiness check, which already re-reads quietly while the server is busy, says
"The package manager has finished. The update can be started again." when it is.
A stop because the server's recovery tool could not be started or verified now has
its own reason ("Reason: the server's recovery tool could not be started or its
installation could not be verified. The server administrator checks it, using the
server's line below; …") with the server's first line under it. A long diagnostic
line is no longer dropped: the panel keeps its first line, at most 240 characters,
without paths outside the product's own, addresses, values of assignments or
token-like strings. While a step's words are still loading, the reason line says
"Reading the reason…" rather than the generic reason.

## What changes underneath

- **A new read-only Agent capability.** `Agent.InstalledPackageVersionsV1` asks the
  system package database once per component scan, for the whole catalogue in one
  query: `dpkg-query -W` with a fixed format on Debian and Ubuntu, `pacman -Q` on
  Arch. Fixed arguments, no shell, no network, a 10-second timeout and an 8 MiB
  output ceiling. The packages asked about are the Agent's own catalogue. It never
  runs while a package operation holds the package manager: it reads the process
  table and the kernel's lock table first, takes no lock itself, and answers
  "busy" when the package manager is in use or its state cannot be read.
- **The Panel** admits the method as a read in its Agent RPC policy (like the other
  scan reads), calls it once per scan without ever failing the scan, and keeps the
  answer with the existing component scan cache as an additive JSON field. A cache
  row written before alpha.97 reads as unknown. No schema change.
- **The screen catalogue is split.** The two Turkish screen catalogue files were
  within 0.29 and 1.00 KiB of their size ceiling. The screen copy is now nine parts
  per language, along groups of screens (setup, sites, siteConfig,
  dnsCertificates, components, mailDatabases, security, dashboard, settings), each
  with its own ceiling set with at least 25% headroom; the rule for where a new text
  goes and how a part is split again is in the header of
  `web/scripts/check-bundle-budget.mjs`. Every part is still loaded before a screen
  is drawn, and no text changed in the split.
- **Browser mocks** report versions only as the new fields would, with sample Debian
  13 strings for Frankfurt-shaped data and Ubuntu 24.04 strings for Boston-shaped
  data, labelled as sample data.

## What does not change

- The updater's mutation path: `update.sh`, the behaviour of `get.sh`, the Agent's
  system update worker, rollback and recovery. The Agent change is a read-only
  capability and its RPC policy. Therefore no native record is made, by the owner's
  instruction of 2026-10-16.
- Schema 44, no migration. No new Agent job kind. No package is installed, upgraded
  or refreshed by the version read, and no update offer is made from a version.
- Hosted websites, mail, DNS, databases, scheduled jobs and certificate renewal do
  not depend on the version read.
- Installed panels are updated only by their owner, from the panel's own update
  screen.

## Known gaps

- **Nothing was measured on an installed server.** The version read ran against a
  faked runner and lock-table fixtures in the Go tests and against a mock server in
  the browser batch; the real `dpkg-query`/`pacman` answer on Boston and Frankfurt
  is not yet seen. The open item "read installed package versions for every
  component" is closed only when the owner sees the versions there.
- **The critical boot path has about 1 KiB of its budget left.** The update dialog
  is mounted above the pages, so its new texts are boot copy. The limit was not
  raised.
- **Boston's result of its alpha.95 update** has not been reported.
- On a desktop the component page header is a little taller with the version line,
  so the action buttons sit slightly below the title line; on a phone a long kept
  sentence takes three lines. Readable; not changed.
- P0.1-P0.5 stay **partial**; no P0 item is closed or advanced; a candidate is not
  acceptance.

## Evidence

Everything below was run on the candidate tree on 2026-10-21 in a development
environment; none of it was run on an installed server.

- Go: `./cmd/panel/` and `./internal/...` 7153 passed, 0 failed, 44 skipped; the
  Agent `./cmd/agent/` 4652 passed, 0 failed, 29 skipped, run in a clean mount
  namespace in the `CelikPanel-S2-Debian` WSL guest (empty agent state, no
  `celikpanel` group) as earlier Agent runs were; `go vet` and gofmt clean
  (`-timeout 45m`). New Agent tests cover the dpkg and pacman parsers (normal,
  missing package, removed and half-installed packages, several architectures,
  odd characters, overlong text, epoch), the catalogue mapping (meta-package versus
  versioned package, several packages, absent components), the one fixed query,
  the timeout, the busy and unreadable probe (no query is made), the lock-table
  reading, and failures carrying no output. New Panel tests cover the RPC policy
  (a read on every family; no similar mutation admitted), the short form, several
  versions, unknown, the busy keep, old cache rows, and the version passing through
  the scan, the cache and both managed-services answers; and the bounded,
  sanitized update diagnostic.
- Web: `npm test` 1671 passed, 0 failed; `npm run build` succeeded within the
  budgets: the largest Turkish screen part (setup) 55.84 of 80 KiB, components
  44.26 of 64 KiB, settings 31.24 of 44 KiB; critical boot path 359.95 of 361 KiB.
  Mounted tests cover the version column appearing only when a version is known, the
  short and full forms, several versions, unknown showing nothing, the component
  page header, the busy package manager stop (with the record shape Frankfurt had),
  the `compatibility` step, the reason waiting for its words, the update card's
  notice and its "free"/"still busy" line, and every key of every screen part
  resolving through the real provider in English and Turkish.
- Release-recovery harness (it mirrors the update card's texts and outcome logic):
  `python3 -m unittest discover -s deploy/e2e/release-recovery -p 'test_*.py'` in a
  fresh clone in the `archlinux` WSL guest: 1109 tests, OK (1 skipped). Its mirror
  of a busy package manager stop was updated to the calm stop.
- Browser batch 22 (mock server): Components on Boston-shaped and Frankfurt-shaped
  data, with and without known versions, and component pages (Nginx, SpamAssassin
  with several versions, BIND with a kept reading, phpMyAdmin with none), in English
  and Turkish, light and dark, at 1440 and 390 px. The first look found that the
  specialised component pages (Nginx, Postfix, Fail2ban, PowerDNS) never showed the
  version, because it was in the generic page's overview card; it moved to the
  header every component page shares. The second look: the version line on every
  page, no raw key, no English on a Turkish page, no horizontal overflow at 390 px,
  no signal colour on a version, no column and no dash when no version is known.
- Release contracts (sequence policy 97 after 96, download portal, signed manifest):
  sequence policy (Git Bash) "96 -> 97" passed; download portal and signed manifest
  (WSL guest) passed.

Component, mounted and mock-browser tests do not establish native behaviour.
