# v0.1.0-alpha.146

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.146.tr.md)

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

## Checked in the lab

OPEN.

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
