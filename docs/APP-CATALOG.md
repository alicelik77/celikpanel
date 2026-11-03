# Application catalog

*[Türkçe](APP-CATALOG.tr.md) · alpha.118 · Code: `internal/appcatalog`, `cmd/agent/apps_catalog_*.go`,
`cmd/panel/app_catalog_*.go`, migration 058. Rights: [PLANS-AND-RIGHTS.md](PLANS-AND-RIGHTS.md) 4.5.*

The Applications tab of a PHP site installs a ready-made web application into the site's empty
document root: WordPress (its own older installer, unchanged), PrestaShop, Nextcloud, Drupal,
Joomla, MediaWiki and phpBB. Each is one release pinned in one reviewed file.

This document describes code that has component tests only. Every application's real install
on Debian, Ubuntu and Arch is proven in the release's lab round; until then the installers are
"built, not yet proven on a real server".

## The pin-and-verify rule

- `internal/appcatalog/pinned.go` holds, per application: the official release archive's
  address, its **SHA-256**, the archive format and top folder, size limits, the files every
  release has (markers), the PHP range, the PHP extensions, the database engine, the disk room,
  the setup recipe, the administration path, the configuration file that holds the database
  password, the nginx rule set and a short risk note.
- The Panel names an application by its ID only. The Agent reads the same compiled list, so the
  Panel can never send it an address or a digest.
- The Agent downloads over HTTPS (redirects only to HTTPS), compares the SHA-256 and **refuses
  on mismatch**: the file is deleted, nothing on the site changed, and the screen says not to
  install it by hand. Composer, npm or any other fetch at install time is never used: every
  pinned archive already contains its `vendor` folder.
- Where the upstream publishes a checksum, the pinned digest was checked against it (Nextcloud
  `.sha256`, phpBB `.sha256`, GitHub's asset digest for Joomla, drupal.org's MD5 for Drupal).
  PrestaShop and MediaWiki publish none (MediaWiki publishes a GPG signature); their digest is
  the one of the official download taken on 2026-11-03. Each entry's `Upstream` field says which.

### Adding or updating an application

1. Download the official archive; compare its SHA-256 with what the upstream publishes.
2. Read the release's own requirement file for the PHP range (for example Nextcloud's
   `lib/versioncheck.php`, PrestaShop's `install/install_version.php`, Drupal's `core/lib/Drupal.php`).
3. List the archive: top folder, entry count, unpacked size, any symbolic links (only links to a
   file in the same folder are accepted; they are written as copies).
4. Edit the entry in `pinned.go` (address, digest, version, PHP range, limits). A new application
   also needs a setup recipe (`cmd/agent/apps_catalog_setup.go`), nginx rules
   (`internal/appcatalog/nginx.go`), a configuration format for password rotation
   (`cmd/agent/transfer_rotation_appconfig.go`) and its name in `web/src/lib/rights.ts`.
5. Run `go test ./internal/appcatalog/ ./cmd/agent/ -run Catalog` and the lab install.

## What happens on Install

1. **Gates** (Panel): the App installer permission, the application's offering `app:<id>`
   (administrator ∩ reseller ∩ subscription), `component:mariadb`, the database count limit and
   the disk limit (with the application's room), one application per domain.
2. **Prerequisites** (Agent, read only): the site's PHP version against the range; each missing
   PHP extension with the **distribution package that brings it** (`php8.3-intl` on Debian and
   Ubuntu, `php-intl` on Arch, or "switch it on in php.ini"); the empty document root (or only
   CelikPanel's placeholder); free disk where sites live. The installer never installs a
   package: the administrator does, from Components or the system's package manager.
3. **Database** and its user, recorded and counted like any database (the WordPress saga).
4. **Files**: download, verify, unpack into a root-private staging folder with the WordPress
   limits, give to the site's user, write the **setup lock** (every address answers "being set
   up", so nobody can finish the application's web installer first), publish with the same
   atomic exchange as WordPress.
5. **Setup**: the application's own non-interactive installer runs **as the site's user** with
   the site's PHP: `occ maintenance:install` (Nextcloud), Drupal's install API (as Drupal's own
   `drupal install` command uses it, with MariaDB), `installation/joomla.php install`,
   `install/index_cli.php` (PrestaShop), `maintenance/run.php install` (MediaWiki),
   `install/phpbbcli.php install` (phpBB). Secrets never appear on a command line (other users
   can read those): the command line holds `@@CELIKPANEL:NAME@@` tokens and a fixed PHP file
   loaded first (`/var/lib/celikpanel-app-setup/prepend.php`) reads the values from standard
   input. Installer output is returned with every secret removed and is never logged.
6. **Rules**: the application's nginx rules are written to the site's CelikPanel include
   directory, then the setup lock is removed.

The generated administrator password is returned once, and kept sealed only until the owner
confirms it was saved (or reveals it once after a lost answer).

### Failure, retry and recovery

- A refusal before step 3 changes nothing and names who acts (owner: PHP version, empty folder;
  administrator: packages, MariaDB, disk; provider: offering, limits).
- A verification or download failure after step 3 removes the operation's database (only with
  the operation's ownership proof) and records `failed`.
- A failed setup leaves the files and the setup lock: `setup_failed`, **Retry setup** runs only
  step 5–6 again with the same secrets; a finished setup is detected and not repeated.
- A lost answer is `needs_review` (or `setup_failed` for the setup, which is safe to repeat).
  After a Panel restart, an interrupted setup becomes `setup_failed`; any other interrupted step
  becomes `needs_review`. Nothing is guessed; reading the state never changes anything.
- A domain holds one live application; a second install is refused by the Panel and by a
  partial unique index (migration 058).

## Removal

Type the domain to confirm. The Agent checks the document root still holds the application
(otherwise nothing is removed), deletes its files **as the site's user**, puts the placeholder
page back and removes the application's nginx rules and setup lock (a file the owner edited is
kept and named). The database is removed only when ticked; Nextcloud's data folder only when it
is also ticked.

## nginx rules (native, readable without the panel)

`/etc/nginx/celikpanel-managed.d/<domain>/app-<id>.conf`, with CelikPanel's render header. The
vhost includes that directory in every server block of the site, including the suspended,
maintenance and redirect-only blocks; therefore the rules never serve anything on their own:
no `root`, no `fastcgi_pass`, only deny, redirect or an internal rewrite back to the vhost's own
PHP location, and only regular-expression or exact locations (a prefix location could collide
with the owner's protected folders). "Path info" addresses (`/remote.php/dav/…`,
`/index.php/apps/…`, `/admin…/index.php/sell/…`) are rewritten to the script; the catalog's
applications read the rest from `REQUEST_URI`. Per application: Nextcloud's `.well-known`
redirects and denied code/config/data paths, Drupal's denied PHP under files and private files,
PrestaShop's denied code folders, MediaWiki's denied maintenance and web installer, phpBB's
denied config/cache/store, Joomla's no-script upload folders; upload sizes per application.
nginx is checked before each change and the file is put back when nginx refuses it.

## Suspension, security emergency, transfer

- **Suspension** (alpha.117) and the **security emergency** (D-043) act on the site: the vhost
  serves the suspended or maintenance page (and the emergency stops the PHP version). The
  application's rules cannot bypass that (see above); the setup lock answers 503 too. A suspended
  subscription cannot install (the quota gate refuses).
- The emergency's **bulk move** to another PHP version does not check an application's PHP
  range yet; an application may stop working on an unsupported version (open item).
- A **transfer**'s credential rotation and a database user's **New password** now also rewrite
  the password inside the application's own configuration file (Nextcloud `config/config.php`,
  Drupal `sites/default/settings.php`, Joomla `configuration.php`, PrestaShop
  `app/config/parameters.php`, MediaWiki `LocalSettings.php`, phpBB `config.php`), in the form
  the installer wrote it; anything else changes nothing and says so.

## Per-application notes

| Application | Version | PHP | Notes |
|---|---|---|---|
| PrestaShop | 9.2.0 (Classic 9.2.0-6.0) | 8.1–8.5 | The Classic edition bundles PrestaShop SA modules that connect to PrestaShop's services once enabled. The admin folder is renamed to `admin<random>`; the install folder is removed. No demo products. Friendly URLs are off (they need image rewrite rules this release does not write). |
| Nextcloud | 35.0.2 | 8.3–8.5 | Data in `<site home>/nextcloud-data`, outside the document root. Trusted domains: the domain and www. Background jobs stay on AJAX until the owner adds a cron job. |
| Drupal | 11.4.11 | 8.3+ | Standard profile; `trusted_host_patterns` set to the domain. |
| Joomla | 5.4.10 | 8.1+ | Joomla 6 needs PHP 8.3 and would not run on Debian 12; 5.4 is supported. |
| MediaWiki | 1.43.11 (LTS) | 8.1+ | The web installer (`mw-config`) is denied after setup. Anyone may edit until the owner changes `$wgGroupPermissions`. |
| phpBB | 3.3.19 | 7.2+ | The install folder is removed after setup. |
| WordPress | latest | 7.4+ | Unchanged installer (newest release, SHA-1 from WordPress.org); finished in its own setup page. |

Not in this release: **Matomo** (no supported command-line installer in its core; a web
installer would leave the site open to whoever finishes it first) and **Moodle** (needs
`max_input_vars` ≥ 5000 in the site's PHP and a cron job; both are site settings this release
does not make). A half-working application is not offered.

## Service applications (later)

The model has a `kind`: every entry here is `php-site`. Long-running Node/Python applications
(`service`) share the `app:<id>` offering space and are a separate workstream.
