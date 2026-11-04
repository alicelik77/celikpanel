# v0.1.0-alpha.121

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.121.tr.md)

> **Published** on 2026-11-04 (UTC): release commit `ba0a53d11`, GitHub pre-release at 13:40:33Z, download portal at about 13:45Z (public check 27/27).

A small release with fixes for what the owner hit on Boston and Frankfurt after
updating to alpha.119 and 120. **Only the owner updates installed panels, from
the panel's own update screen.**

## PostgreSQL 18 on Ubuntu 24.04

- PostgreSQL 18 from the PostgreSQL (PGDG) repository could not be installed on
  Ubuntu 24.04. It needs a newer `postgresql-common` (and
  `postgresql-client-common`) than Ubuntu ships, and the repository's
  confinement kept Ubuntu's. The install now asks for PGDG's versions of those
  two packages by name, only when Ubuntu's version is too old. The install window
  lists them with `libpq5` as packages whose updates then come from PGDG, and you
  confirm them first. Nothing is removed; PHP's PostgreSQL extension keeps
  working.
- Debian 13, and PostgreSQL 17 or 16 on Ubuntu, install exactly as before.

## Clearer messages

- When the install window cannot check what an install would bring, it now says
  why (for example "unmet dependencies: postgresql-common"), who needs to act and
  what to do next. If a package that an enabled repository should offer is
  missing, the panel refreshes that repository's package lists once and checks
  again.
- Right after an update, a one-click install refused because the server was
  still finishing its start-up work now says exactly that: nothing needs to be
  done, and it ends by itself within a minute or two.

## Update screen

- After an update stopped on a busy package manager, the "Update to …" button
  now closes the old notice and starts the update in one press. The server is
  checked again first. Nothing starts by itself.

## Server host name

- The Dashboard says when this server's own host name (shown in the sidebar)
  does not resolve in public DNS, and who needs to act. If this panel serves
  that zone, **Add the record…** shows the exact address record for review and
  adds it through the zone's normal record editor. The check only reads and is
  cached.

## Known limits

- Only PGDG's `postgresql-common` and `postgresql-client-common` are taken this
  way. Another package that the distribution ships too old is still reported,
  not replaced.
- The host-name notice does not offer one click for a zone served elsewhere.
  For a name that points to another address, it only explains.

## Evidence

Go tests for the new Agent and Panel code pass. 2,444 web tests pass within the
bundle budgets. Release sequence 120 → 121. Apt's behaviour was checked with
`apt-get -s` only, with a confined test repository (apt 3.0.3, classic and 3.0
solvers): an Ubuntu-like host fails as Boston did, and succeeds when the two
common packages are named (3 upgraded, 2 new, 0 removed). A Debian-like host
needs only `libpq5`.

Lab (`final-alpha121-run1-20261104`, lab branch commit `31fb2aec0`): Ubuntu 24.04 at alpha.120 with Ubuntu's PostgreSQL 16, phpPgAdmin and a database, PGDG enabled, updated to 121 in 84 seconds. PGDG 18's preview named libpq5 (16.15 → 18.6), postgresql-common and postgresql-client-common (257build1.1 → 293) for confirmation; 18 installed and runs beside 16; both clusters are listed; PHP still has its PostgreSQL extension; 16's data is intact. PGDG 17 named only libpq5. With PGDG's package lists deleted, the preview refreshed them and checked normally. Debian 13: PGDG 18 named only libpq5 and installed. With the package manager held, an update start was refused with nothing left behind, and one press started it once free. Not exercised on a real system: the host-name notice (the lab host name is not a domain name) and the start-up busy wording (the first install after the update was not busy).
