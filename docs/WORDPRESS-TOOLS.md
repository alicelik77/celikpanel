# WordPress tools

*Feature note · [Türkçe](WORDPRESS-TOOLS.tr.md)*

*alpha.118 (owner request: "WordPress araçları"). Source change; tested with focused unit
tests only, not yet on a real server. The release's lab round is still open.*

For every PHP site where the panel installed WordPress or finds it in the web folder, the
Applications tab has a **WordPress tools** section, separate from the application catalogue.

## What it does

- **Status:** WordPress version, plugins and themes with their versions and whether
  wordpress.org offers an update. The versions are read from the site's files by a small
  panel-shipped PHP script that runs **as the site's own account, with the site's own PHP**,
  reads version headers only and runs no WordPress, plugin or theme code. Updates are asked
  only from **wordpress.org's official API** (`api.wordpress.org`); nothing else is contacted.
- **Updates, backup first:** before any update the panel makes a backup of the site with the
  existing backup engine (origin "Before a WordPress update"; files, plus the site's databases
  for a core update). Then:
  - **Core:** the release is downloaded from wordpress.org and checked against its published
    SHA-1 before use; the files are copied by the site's own account (never `wp-content` or
    `wp-config.php`); WordPress's own `wp_upgrade()` runs as the site account with plugins and
    themes not loaded.
  - **Plugins:** every package is downloaded and verified **file by file** against the SHA-256
    list wordpress.org publishes for that plugin version, before the site is touched. The old
    plugin folder is set aside and put back if anything fails.
  - **Themes:** wordpress.org publishes no checksum for theme packages, so CelikPanel does not
    install theme updates (security before freedom). The screen shows the available version
    and says to update from WordPress's own Dashboard > Updates page, after a backup.
  - WordPress's own maintenance page is shown during the change; the site's front page is
    asked locally before and after. If it answered before and fails after, the update counts
    as failed.
- **Rollback:** if an update changed files and did not succeed, the panel restores the backup
  it made first, automatically. If the Agent itself put the old plugin folders back, no
  restore is needed. If the update's answer was lost or the panel restarted during it, the
  result is **unknown**: nothing is guessed or repeated; the screen names the backup to
  restore from Advanced > Backups if the site does not work.
- **Automatic minor and security releases (per site switch):** once a day the panel looks for
  a newer release of the installed branch (6.6.1 → 6.6.2, never 6.6 → 6.7) and installs it
  the same way, backup first. Major releases are always the person's choice. WordPress's own
  background updater is not changed by the panel (owner independence): if the owner has not
  disabled it in `wp-config.php`, WordPress also installs minor releases itself.
- **Staging copy:** copies the site to another, empty PHP site of the same subscription:
  files (the backup engine's safe archive), a new database (counted against the database
  limit; the disk limit is checked first) and the address. The address is changed inside the
  database dump with a **serialization-safe** replace: PHP serialized values get their string
  lengths written again, nested values are walked, JSON-escaped and URL-encoded forms are
  covered, `www.` is mapped too, and a value that cannot be parsed is left untouched (counted)
  rather than corrupted. `wp-config.php` of the copy gets only its four database lines and the
  address changed. A failed copy removes exactly the database it created (ownership token).

## Decision: no WP-CLI

WP-CLI is in Arch Linux's official `extra` repository, but **not** in Debian 12/13 or Ubuntu
22.04/24.04 (checked on packages.debian.org and packages.ubuntu.com, 2026-11-03). CelikPanel
installs only official distribution packages on every supported system, so WP-CLI is not used.
The needed operations are done with the site's PHP running two small panel-shipped scripts
(`/var/lib/celikpanel-wordpress-tools/inventory.php`, `upgrade.php`), WordPress's own
`wp_upgrade()`, standard file tools run as the site account, and the existing backup engine.

## Rights

Permission `wordpress_tools` (D-042, [PLANS-AND-RIGHTS.md](PLANS-AND-RIGHTS.md) section 4.2).
The server enforces it on every request; the screen hides the section without it. The
administrator always may. Unlimited plans and sets with "all permissions" include it; a Custom
set only when someone adds it (no silent change). Additional users (team members) do not get
these tools. The application installer keeps its own permission.

## Guidance on screen

Every state says the reason, who acts, the next step and how it goes on: waiting for the
server's single operation slot, making the backup, updating, restoring, done; a verified
failure ("nothing was changed"); restored ("the site was put back"); unknown ("check the
site; restore backup X"); prerequisites (PHP command line missing → the administrator; the
site's database not linked to the domain → a core update is refused because the backup would
not include it). The screen reads the operation until it ends and never starts a second one;
the server refuses a second open operation for the same site.

## Limits and open items

- **Push staging to live** is not offered (out of scope this release).
- Theme updates are not done by CelikPanel (no published checksum).
- Plugins that are not on wordpress.org, single-file plugins and must-use plugins are listed
  but not updated by the panel.
- A core update leaves files that the new release no longer ships in place (WordPress's own
  updater deletes a list of old files; the panel does not yet).
- A copy needs `wp-config.php` inside the web folder, standard `define()` database lines and
  the site's database linked to the domain in the panel.
- One WordPress operation runs at a time on the server.
