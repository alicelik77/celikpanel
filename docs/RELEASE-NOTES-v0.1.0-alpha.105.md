# v0.1.0-alpha.105 (candidate)

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.105.tr.md)

This is the candidate that follows v0.1.0-alpha.104. **It is not published.**
Written on 2026-10-24, as a candidate, on the branch
`feat/alpha105-bundle-versions`, from `main` after the publication record of
alpha.104 (the alpha.104 release commit is
`98c4750b4a5d4ad67447468bcfdde3a3b848465a`).

## Who should update, and why

Anyone who installs ready bundles or database and webmail tools from Components.
The owner asked how a bundle chooses its versions and whether it should force
compatible ones, and recalled CelikPanel's principle: **recommend, warn, never
impose** (D-034 amendment, 2026-10-24).

Only the owner of a server updates it, from the panel's own update screen. What the
owner sees in each case is in the [operation
guidance](OPERATION-GUIDANCE.md#choosing-versions-in-a-bundle-installs-that-bring-more-than-you-chose-a-tool-that-is-not-ready-after-its-install-d-034-amendment-2026-10-24).

## A bundle shows its versions, and you can change them

- Each part the bundle installs says the version that will come, for example
  "8.4 · the distribution’s version · recommended". **Change** lists the versions
  that part's own install already offers ("8.3: from Sury PHP
  (packages.sury.org)"; PostgreSQL from its own repository); no new repository is
  added. A part already installed says its version and is left as it is.
- The recommended version is the distribution's own. It is marked, never forced.
- When you choose a PHP version, the bundle's database tool runs on it ("Runs on PHP
  8.3").
- A PHP version that cannot work is refused before anything changes, for example
  "phpPgAdmin cannot run on PHP 8.6 here: php8.6-pgsql is not available from this
  server’s repositories. Choose another PHP version for PHP-FPM." A version that
  usually works but is not tested, or an old PHP, is only a warning ("… You may go
  ahead.").

## You see what an install brings before it starts

- The package manager is asked first what the install would bring. If it would
  bring another PHP version, Apache, another component or a database server, the
  dialogue says "This also installs packages you did not choose:" with the names
  and a plain reason, and Install waits until you tick "I have read this; install
  these packages as well." This is in the bundle dialogue and in a component's own
  install dialogue.
- If that check cannot be made (the package manager is busy), the dialogue says so
  and does not stop you: the server checks again right before installing and
  refuses, installing nothing, if something you did not confirm would come.

## Tools are checked when they are first installed

- phpMyAdmin, phpPgAdmin and Roundcube are checked right after their install, the
  same way as a move in alpha.104 (CelikPanel's PHP check, then the tool's own
  page). If the check fails, other installed PHP versions that have the tool's
  extensions are tried, each put back if it fails.
- If none works, the tool stays installed and reads "Not ready" with the cause and
  the next action: choose a PHP version in the tool's Manage window. A tool that
  was already installed is checked but never moved.

## Underneath

- New read-only Agent check of what an install would bring, and a new Agent step
  that proves a tool inside its install; installs are checked again under the
  package lock right before they run.
- Panel: `GET /api/v1/service/install-preview`; every install is checked on the
  server (a component's own page, Server setup, mail profiles, a PHP version added
  from the versions list, Repair and an older open tab included); the browser only
  says what you confirmed. An install keeps the chosen PHP version and what you
  confirmed in its record, so one that is interrupted and resumes runs as you asked.
- Continuous integration: the "Panel race — Server setup" job was split in two
  ("(1/2)" and "(2/2)"); its time limit was not raised.

## Unchanged

Sites still choose their PHP version in their own settings. The updater, the
schema (44) and every installed server are unchanged; only the owner updates.
Every install is checked, and alpha.104's rule that an install never removes a
package stands. Where no window lists the packages (Server setup, a mail profile, a
PHP version added from the versions list, Repair), an install that would bring a
notable package is refused with nothing installed, and its guidance names the
packages and says to install that component from Components, where they are listed
for you to confirm.

## Evidence on the candidate

- Agent: 4723 passed, 29 skipped (in a clean mount namespace).
- Panel and internal packages: 7243 passed, 44 skipped.
- Web: 1796 passed, and the production build within its budgets.
- Real system, one disposable Debian 13 virtual machine (QEMU/KVM, deleted after the
  run): phpMyAdmin alone on a bare PHP 8.3 was found to bring PHP 8.4's packages,
  and its unconfirmed install was refused with nothing installed; PostgreSQL +
  PHP-FPM 8.3 + phpPgAdmin brought no other PHP and no Apache, and phpPgAdmin was
  proved on 8.3; a PHP version without an FPM package was refused; with pgsql off
  for 8.3, an already installed tool's check failed with the cause and did not
  move, while a first install fell back to PHP 8.4 and was proved
  ([evidence](../deploy/e2e/bundle-versions/alpha105-debian13-20261024/README.md),
  archive SHA-256 `183e4275…61f2f8`). The machine started from a reusable Debian 13
  base image (SHA-256 `e1c1173b…53caec`), kept for later releases.
- The release-recovery harness was not run: no update file changed.

## Known gaps

- Nothing is measured on an installed server.
- pacman's check is covered by component tests only; the DNF path stays
  unreachable.
- The Panel's check route, the browser dialogue and the bundle sequence were not
  part of the virtual-machine run (component and mounted tests cover them); Apache
  as an unexpected install is covered by component tests only.
- The Apache rule knows Debian's and Ubuntu's names only (apache2, apache2-bin,
  libapache2-mod-php*), not Arch's (apache, php-apache); and on Arch the check
  (`pacman -S --print`) is not the real `-Syu` transaction, which also upgrades the
  system.
- If branch protection requires the old check name "Panel race — Server setup", the
  owner must replace it with the two new names.
- P0.1–P0.5 stay partial; no P0 item is closed or advanced.
