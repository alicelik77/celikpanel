# v0.1.0-alpha.108

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.108.tr.md)

This is the release that follows v0.1.0-alpha.107. **It is a candidate, written on
2026-10-25; it is not published and nothing from it is on an installed server.
Only the owner updates installed panels, from the panel's own update screen.**
Written on the branch `feat/alpha108-safe-lanes` from `main` after the publication
record of alpha.107 (the alpha.107 release commit is
`2e68e6039e5d21160c28944ad9fd74b83747ec4d`).

## Who should update, and why

Every server. After updating Boston and Frankfurt to alpha.107 the owner found that
a vendor repository could still be enabled without its confinement and automatic
updates, that the PHP-FPM version list installed without showing what it brings,
that Boston's phpPgAdmin was reported not answering while it answered, that Sury's
key read "expired in 2021", and that PostgreSQL had no way to run 16, 17 and 18 side
by side. He decided: **security and stability always come before freedom**
([D-038 amendment](DECISIONS.md)). What the owner does on each server is in the
[operation guidance](OPERATION-GUIDANCE.md#two-lanes-postgresql-side-by-side-and-the-owners-findings-d-038-2026-10-25).

## Two lanes: the operating system's packages by default, a vendor repository only by informed choice

- PHP-FPM's and PostgreSQL's pages say when the component comes from the
  operating system. Versions only a vendor repository ships are listed as
  **locked**, with where they come from and who patches them.
- **Use Sury PHP… / Use PGDG…** opens one dialog that shows the three steps
  together — the repository and its pinned key, the confinement rule, its origin
  in the operating system's automatic updates — each with its file, text and
  SHA-256; one confirmation writes them, the source last. With automatic updates
  off, it says the command to turn them on first.
- **Already enabled (Boston, Frankfurt):** nothing is switched off. The
  repository card names what is missing and **Complete…** writes only that. The
  Security updates card has the same button next to an uncovered repository.
- Customers never enable repositories. Disabling one removes nothing; what came
  from it is marked **no longer updated**.
- phpMyAdmin, phpPgAdmin and webmail stay on the distribution's PHP by default; a
  vendor version is marked as meant for sites.

## PostgreSQL 16, 17 and 18 side by side

- **PostgreSQL → Clusters** lists every cluster with its port, status, data
  directory, databases and the panel's records.
- **Install PostgreSQL NN…** shows the port its cluster will get first and goes
  through the install preview.
- **Change port…** shows the changed line, who is connected and the interruption,
  restarts, proves the new port and puts the old one back otherwise.
- **Move a database…** copies it (roles included, passwords never shown), proves
  every table's row count on both sides, and keeps the source. **Switch**,
  **Switch back** and **Remove the source** are separate confirmed steps.
- Removing a major is refused while it holds databases; its data is kept.
- MariaDB cannot run side by side from packages; its page says so.

## The owner's findings, corrected

- Every install path shows the preview and asks to confirm notable packages,
  including the PHP-FPM version list and Repair. The refusal says to press Install
  again.
- **phpPgAdmin on Boston:** the likely cause is the Agent's `UMask=0027`, which
  makes CelikPanel's check file unreadable to PHP-FPM. It was reproduced on a
  disposable Debian 13 VM, not measured on Boston: pressing **Check again** after the
  update confirms it there (it says the real cause if it is another). It repairs it and
  proves the tool; "not running" is said only when PHP-FPM really is stopped; using
  the same PHP version only proves; a lost answer is read back, never resent.
- **Sury's key:** its expiry is read from its newest signature (early 2028);
  "apt no longer accepts" appears only when apt said so. **Fetch its current key…**
  is on the card when a key is expiring.
- The install refusal no longer squeezes into a column; Components rows keep their
  names whole at medium widths; the sidebar collapses to icons (remembered in this
  browser, keyboard accessible, phone unchanged); Roundcube's row has a direct
  **Update**; owner commands start with `sudo`, with "if you are root, leave out
  sudo".

## Remaining D-038 items

- **Offered to customers** (PHP-FPM and Node.js pages): the administrator hides or
  offers each version line; a site already on a hidden version keeps it, and its
  owner sees a note.
- Version lists mark **tested** with this release or **installable, not tested**.
- Removal dialogs name what the server's own traces show: web-server lines naming
  the socket or port, services that depend on it, programs connected now.

## Upgrade and recovery

- Schema 45 to **46** (additive: `database_servers.cluster`, table
  `postgresql_database_moves`). An alpha.107 Panel refuses schema 46, so going
  back is the pre-update snapshot restore. The release-recovery harness pins
  migration 46.
- Sequence 107 to 108. Nothing on the server changes by itself after the update:
  every lane step, port change and move waits for the administrator.

## Evidence

- Agent 4822 passed, 29 skipped (in a clean mount namespace); Panel and internal
  packages 7945 passed, 45 skipped, 0 failed; web 1906 passed, and the production
  build within its budgets (two new screen parts, `databaseClusters` and
  `vendorLanes`, each with its own ceiling). The release-recovery harness pins
  schema 46; its four database suites pass, and its controller suites show the same
  environment-only errors as on alpha.107 in that guest (no git there).
- One real-system run on a disposable Debian 13 VM from the kept base image: four
  tests passed (PostgreSQL side by side, the PHP tool proof under the Agent's umask,
  removal traces, the Sury vendor lane). Its first run found that a PGDG major
  installed on the distribution's postgresql-common came without its cluster; the
  install now creates it on the shown port
  ([evidence](../deploy/e2e/d038/alpha108-debian13-20261025/README.md), archive
  SHA-256 `effaf7ce…a1604698`).
- After the release review, a short second run of the PostgreSQL and vendor-lane
  tests on the same base image: an UPDATE after the copy refuses the drop of the
  source, and the image's own `php.list` is named as the owner's with no second
  source (archive SHA-256 `79430a30…0b1429b`).
- A browser look (EN and TR, 1440 and 390, light) on a mock (batch 31).

Not measured on an installed server. A candidate is not acceptance; P0.1–P0.5 stay
**partial**; no P0 item is closed or advanced.
