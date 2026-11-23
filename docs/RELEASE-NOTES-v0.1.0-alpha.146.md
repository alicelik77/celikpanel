# v0.1.0-alpha.146

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.146.tr.md)

> **Published** on 2026-11-23 (UTC): release commit `8917ed407`, GitHub pre-release at 13:27:13Z, download portal at about 13:31Z (public check 27/27).

**A release for the public website only.** It brings the new celikpanel.net
with this release's download portal. Nothing in the panel's or the agent's
code changes, and the panel's database layout does not change.
**Only the owner updates installed panels, from the panel's own update
screen.**

## Updating from alpha.145

1. Update from the panel's own update screen, as before.
2. **The panel's database layout does not change** (it stays schema 81).
   Going back to alpha.145 is the ordinary rollback; no snapshot has to be
   restored.
3. There is nothing to press after the update and nothing to change by hand.

## The new celikpanel.net

The site is rebuilt as a datasheet, in Turkish and English, with every page
in both languages:

- **Home:** what CelikPanel is ("a hosting control panel for Linux servers"),
  a specification list beside a real screen of the panel with sample data, the
  install command, the supported systems, the separation between customers as
  the published release notes state it (with what is not done yet), what keeps
  running when the panel stops, dated measurements, the licence today and from
  version 1.0, and what is not offered yet.
- **Install:** the steps, the command, the setup wizard's DNS choices and the
  rules a change on the server follows.
- **Systems:** the Linux systems supported today with the distributions' own
  end dates, what each offers and does not, and how updates work.
- **Licence:** what the licence does today, the terms from version 1.0 as
  decided on 14 November 2026, what is not decided yet, and what never depends
  on the licence.
- **Releases:** the current release with its files, and how to check a
  download by hand (now with the step that compares the signed manifest's
  archive digest with the package's).
- **Changelog** (new): what each release changed, from v0.1.0-alpha.141 on, in
  a few plain lines with a link to its full notes.
- **Known problems** (new): numbered cases (CP-2026-1 onwards) in our own
  words, with their state, what to do meanwhile and the release that fixed
  them; and how to report a problem today (security problems privately, other
  problems in the public feedback repository). No reporter's name, server or
  words are ever shown.

Every number and every statement on the site is taken from the repository; the
site's own tests check that each cited line still says what the site relies
on.

## What changes outside the site

This release changes no file the panel or the agent runs. Compared with
alpha.145's release commit, the files outside `site/` and the generated pages
in `download-portal/` that differ are:

- `download-portal/get.sh`: the pinned release, now alpha.146 (as in every
  release).
- `deploy/release-sequence-policy`, `README.md`, `README.tr.md`: the version
  pins of this release.
- `deploy/build-download-portal.sh`, `deploy/promote-download-portal.py`,
  `deploy/verify-download-portal-public.py`: how the download portal is built,
  published and checked from outside. They run when celikpanel.net is
  published, never on a server running CelikPanel.
- `deploy/test-*` (the portal, command, promotion, verifier, release sequence
  and signed manifest contracts): tests; they are not part of the release
  archive.
- `web/tools/browser-inspect/`: the tool that took the site's screens; it runs
  on a developer's machine only.
- `DESIGN.md` and `.gitattributes`: the site's type sizes, and how git stores
  the site's list and screens.
- `ROADMAP.md`, `ROADMAP.tr.md`, `docs/RELEASE-NOTES-v0.1.0-alpha.145*.md` and
  `internal/recoveryruntime/dns_application_compatibility_linux.go` differ
  only by alpha.145's own publication record, which is already on main.

## What to expect after updating

- The panel shows the new version; no schema step.
- No service is restarted beyond what every update does.
- Going back to alpha.145 is the ordinary rollback.

## Known limits

- Everything alpha.145's notes list as known limits stays as it is
  ([alpha.145 release notes](RELEASE-NOTES-v0.1.0-alpha.145.md)).
- On Ubuntu, if the system's own automatic package update is running when
  the update is pressed, the update is refused without changing anything and
  can be pressed again when that has finished (the card may say the server is
  ready a little before the package manager is really idle).

## Checked in the lab

One run on 23 November 2026 (UTC about 10:30 to 11:10) on the pair of test
servers: a Debian 13 main DNS server with PowerDNS, mail and webmail, and an
Ubuntu 24.04 backup DNS server with BIND. Both started on alpha.145's code and
were updated to this release's code, built under the lab-only label
v0.1.0-alpha.154, through the panel's own update page. The lab's download
portal served this release's own new site. No hard failure.

- **Both updates finished by themselves.** The main server: pressed at
  10:35:54, "updated" with every step ticked at 10:41:22. The backup server:
  the first press at 10:43:18 was refused at 10:45:17 because Ubuntu's own
  daily package job had started at 10:44:05; the window said the package
  manager was busy and nothing had been changed, and the server kept running
  alpha.145's code. The second press at 10:50:22, after that job had ended at
  10:47:55, ended "updated" at 10:53:59.
- **The schema did not change:** both servers stayed on schema 81, with no
  new step.
- **Webmail and the web server were left alone:** on both servers webmail's
  configuration and its local door kept their content and times, the web
  server was not reloaded and PHP was not restarted.
- **What the panel's start refreshes, as at every update:** Postfix was
  refreshed twice and Dovecot reloaded once on each server when the panel
  started again, the same as at the panel's starts in alpha.145's lab run.
- **Sites, DNS and mail kept answering:** during the main server's update
  756 requests to three sites from outside all answered; after both updates
  the sites, both panels and webmail's page answered as before, mail answered
  on ports 25, 587 and 993, and both servers gave the same DNS answers as
  before. The panel's usual pages showed the new version on both servers.
- **The new site, as the lab's portal served it:** home, install, systems,
  licence, releases, changelog and known problems, in English and Turkish, at
  desktop and phone width: every page answered in its own language with the
  version and date filled in and no placeholder left; the style sheets, both
  fonts and the home's screens loaded, with no failed file, no browser error
  and no sideways scrolling on the phone. Every link between the pages and
  every anchor resolved. The changelog shows this release, dated 23 November
  2026. The Releases page's check commands, run by hand as printed (English
  and Turkish) on the files downloaded through the page's own links: the
  checksum matched, the signature was verified, and the signed manifest's
  archive digest equalled the archive's. (The lab's portal serves the lab's
  signing key, so the key's digest differed from the one the page prints, as
  expected.)
- **Compared with the earlier portal:** `latest.txt` has the same form and
  names the new version, the signing key file is the same byte for byte, and
  `get.sh` differs only in the two lines that name the release. The member
  area's selector files (`account/`) are the same as alpha.145's and name the
  same member-app release.

**Not checked in the lab:** the member sign-in pages themselves (the lab's
portal does not run PHP; they are checked read-only on the live site when
this release is published); a fresh install with the page's command; links
to other sites; nothing on Arch Linux.

## Evidence

No schema change: no file under `internal/db/migrations/` changes; the
layout stays 81. No agent call, route, audit action or screen of the panel
changes.

The site's tests (`go test ./site/...`): the committed pages equal what the
generator writes; both languages have the same texts and every text is used;
well-formed pages under the portal's content policy; every link and anchor
resolves; every number has a source whose quoted phrase is on the cited line;
no other control panel's name; titles and headings say "Linux servers";
an optional feature says it is optional; no decoration in the styles; every
changelog line and case cites a release-notes line; every published release
from alpha.141 on has its changelog file; no reporter field in the cases. The
portal contract tests check the built portal: every listed page and asset
shipped with its reviewed bytes, the release values filled in, the install
command, the language links.
