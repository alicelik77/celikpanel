# v0.1.0-alpha.107

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.107.tr.md)

This is the release that follows v0.1.0-alpha.106. **It is a candidate, written on
2026-10-24; it is not published and nothing from it is on an installed server.
Only the owner updates installed panels, from the panel's own update screen.**
Written on the branch `feat/alpha107-d038` from `main` after the record of D-038
(the alpha.106 release commit is `a8f90616f6472300fe6bc29868fa962247da51ad`).

## Who should update, and why

Every server, and before anyone removes PostgreSQL or MariaDB. On 2026-10-24 the
owner asked whether security patches should be forced and whether one old version
puts the whole server at risk, and asked for one principle for every component
(D-038). A read-only audit found that PHP from the Sury repository was never
installed by Frankfurt's automatic updates, that a vendor repository could replace
the distribution's own packages, that a plain PostgreSQL install with PGDG followed
a moving default to a new major, that removing a database engine purged it, that
webmail had no update path and that nothing said which services wait for a
restart. This release closes those gaps. What the owner sees is in the
[operation guidance](OPERATION-GUIDANCE.md#versions-updates-and-restarts-of-every-component-d-038-2026-10-24).

## Security updates are the operating system's, and now visible

- **Security → Security updates**: whether automatic updates are on, the last run,
  waiting updates and security updates, which installed packages come from origins
  the automatic updates do not cover, and what CelikPanel installed outside the
  package manager (Roundcube, Node.js). Off is a warning with the command to turn
  it on; CelikPanel never switches it off.
- **Add to the automatic updates…** writes one file,
  `/etc/apt/apt.conf.d/52celikpanel-vendor-origins`, after you confirm its text;
  your own files are never edited. Delete it to undo.

## Vendor repositories stay in their lane

- Enabling Sury PHP or PGDG writes an apt preferences file first: the repository
  provides only what the distribution does not ship; packages already installed
  from it keep following it. Repositories enabled before this release are confined
  when the Panel starts after the update; an owner's own rule is kept and named.
- An install that would replace a distribution package with a vendor version is
  refused by name. A version only the vendor ships takes the vendor's shared
  packages it needs (Sury's `php-common`, PGDG's `libpq5`), named in the install. An install that would upgrade a package you did not ask for
  names it and waits for your confirmation (for example postgresql-17 needs
  Debian's perl-base security update).
- With a vendor repository, a plain install takes the exact version the
  distribution's default brings (`php8.4-fpm`, `postgresql-17`), never the moving
  default package.
- The repository card says who enabled it and when, the signing key and its
  expiry, the list's date and what apt reported.

## Removing keeps data; webmail and Node.js get their updates

- **PostgreSQL and MariaDB are removed, not purged:** the dialog says where the
  data stays, and a later install says it found the data and uses it.
- **Webmail (Roundcube)**: "Update to 1.6.x…" downloads the release, checks its
  signature with Roundcube's key, keeps configuration, plugins and the database,
  archives the current webmail, swaps it in one step, and puts it back if webmail
  does not answer. No service restarts. This release installs 1.6.20 for new
  installs.
- **Node.js**: each installed line says its newer patch; install it, then move each
  site on request: the application restarts on the new version and must answer,
  otherwise it is put back.

## Restarts, support dates, accounts and the operating system

- **Maintenance → Restarts and reboot**: services still running replaced code and
  whether a reboot waits, each restart saying what it interrupts first.
- **Support dates**: a dated table (checked 2026-10-24) marks versions that no
  longer get security fixes, from whoever ships them; unknown stays unknown.
  Choosing such a PHP for a site asks for confirmation.
- **Node.js applications** made from now on run under the site's own user; an
  existing one says it runs as the shared web user and moves only on request,
  proved or put back. Shared isolation (webmail and database tools on one PHP
  pool, mailboxes under one user) is said plainly.
- **Server setup → Operating system**: this server's place in the supported list.
  After the owner upgrades the operating system, it says what changed and what to
  do, until marked reviewed. CelikPanel never starts that upgrade.
- **Audit log**: rows carry details (component, versions from and to,
  confirmations, outcome); site PHP and Node.js changes are recorded.

## Limits

- Schema 44 to 45 (two additive columns); an older release cannot open the new
  database, so going back is the pre-update snapshot restore.
- Not measured on an installed server. pacman and DNF paths by component tests
  only. Moving Roundcube to another line, a major database upgrade flow, hiding
  versions from customers and per-component "tested" marks are not built.
- Evidence on the candidate tree: Agent 4767 passed, 29 skipped (clean mount
  namespace); Panel and internal packages 7924 passed, 45 skipped, 0 failed; the
  release-recovery harness pins migration 45 and its tests passed; web 1842
  passed and the production build within its budgets (the new "lifecycle" words
  part has its own ceiling); browser batch 30 (EN and TR, 1440 and 390, light);
  one real-system run on a disposable Debian 13 VM
  ([evidence](../deploy/e2e/d038/alpha107-debian13-20261024/README.md), archive
  SHA-256 `1a317d7b…9266913`). Three earlier runs found and this release fixes:
  PostgreSQL's cluster unit not found after an exact install and a template name
  taken for a unit on removal, and Sury PHP versions blocked by the confinement
  (now the vendor's `php-common`, PGDG's `libpq5` are named in such an install).
  Debian's own purge of `postgresql-17`, run by hand in the VM, deleted the
  cluster; CelikPanel removes without purging.
