# v0.1.0-alpha.120

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.120.tr.md)

A small release with the Components fixes the owner asked for after updating
Boston and Frankfurt to alpha.119. **Only the owner updates installed panels,
from the panel's own update screen.**

## Components

- A core server program that is not installed yet (Nginx, PHP-FPM, MariaDB,
  PostgreSQL) now has its own row with **Install**, like the installed ones.
  The install window shows the versions. The small "or install one on its own"
  link is gone; the ready bundles stay.
- The security emergency steps left the list. They are now on each
  component's own page (PHP, Node.js, MariaDB, PostgreSQL) and in one
  "Security emergency" area on the Security page, which says why only these
  four: they are the programs customer sites run on directly. A row shows a red
  "Security emergency open" badge only while an emergency is open.
- Python shows the version the operating system provides.
- The page's address is now `/components`. Old `/services` links and
  bookmarks still work: they open the same place.

## Known limits

- Store items saved by earlier releases still link to `/services/…`; they keep
  working through the redirect.
- Mail and DNS engines are not listed as "not installed" rows; they are chosen
  in mail setup and in DNS infrastructure.

## Evidence

Go tests for the panel pass; 2432 web tests pass within the bundle budgets;
release sequence 119 → 120. <!-- LAB -->
