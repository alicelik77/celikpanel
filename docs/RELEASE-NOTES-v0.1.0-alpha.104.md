# v0.1.0-alpha.104 (candidate)

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.104.tr.md)

This is the candidate that follows v0.1.0-alpha.103. **It is not published.**
Written on 2026-10-23, as a candidate, on the branch `fix/alpha104-safe-removal`,
from `main` after the publication record of alpha.103 (the alpha.103 release
commit is `f780d5f08d0f295fbf7d249a853d9c130cd28779`).

## Who should update, and why

Anyone who removes components or PHP versions from Components, and anyone with
Roundcube webmail, phpMyAdmin or phpPgAdmin. What happened on Frankfurt on
2026-10-23 (Debian 13, alpha.103): PHP-FPM 8.6, 8.5 and 8.4 were installed (8.4
is Debian 13's own PHP), with phpMyAdmin, phpPgAdmin and Roundcube. The owner
removed PHP-FPM 8.4. The dialog had said only that sites would be checked. Two
minutes later phpMyAdmin was "Not installed" and webmail was "not verified —
Roundcube is installed, but its Unix-socket runtime is not ready". The server's apt
history showed that the removal (`apt-get purge -y --auto-remove`) had also purged
phpMyAdmin, the distribution's PHP packages and about 35 libraries, and that all
three tools had been served from PHP 8.4. The owner decided that every correction
ships in this release ("Tüm düzeltmeleri 104'te yap. 105'e bir şey bırakma.").

Only the owner of a server updates it, from the panel's own update screen. How
Frankfurt is brought back is in the [operation
guidance](OPERATION-GUIDANCE.md#frankfurt-on-alpha103-removing-php-fpm-84-took-phpmyadmin-and-stopped-webmail-removals-and-php-tools-in-alpha104-d-037-amendment-2026-10-23).

## A removal never removes anything it did not name

Before any package removal (a component, one PHP or PostgreSQL version; apt and
pacman) the Agent asks the package manager what the removal would do, without
changing anything. It may remove: the packages it names; the packages of the same
version (`php8.4-…`); and the automatically installed packages that only this
removal leaves unused. Anything else — a package of another component (phpMyAdmin),
the component's own distribution package (which means the version is the system's
default), any other package that depends on what is removed, or anything it would
install — refuses the removal by name, before any service is stopped. Packages
that were unused before (an old kernel) are left alone. The real removal runs the
exact list that was simulated, after the same simulation once more, under one
package lock; if the answer changed in between, it is refused.

The removal dialogs say it before the button: "The package manager checked this
removal: it takes exactly these N packages."; or "The package manager would also
remove packages this removal does not name, so CelikPanel refuses it and nothing
changes:" with, for each, what to do ("phpmyadmin belongs to phpMyAdmin: remove
phpMyAdmin first, or keep PHP-FPM 8.4."; "php-fpm — the distribution's own package
— depends on it: PHP-FPM 8.4 is this system's default. …"); Remove is not offered
then. When the check could not run: "Another package operation is running, so the
package manager could not be asked now. You can still press Remove: it is checked
again then and refused if it would remove anything it does not name." A failed
read never holds a removal.

## Each PHP tool runs on a PHP version you choose

- **The tool's Manage window** (Roundcube, phpMyAdmin, phpPgAdmin now have one)
  shows "PHP version": which version it runs on, and a choice of the other
  installed PHP-FPM versions. "Use PHP 8.6" moves it: its PHP extensions are
  installed for that version if missing, the web server's configuration is
  written, checked and reloaded, and then the tool must answer as itself through
  its own socket (a CelikPanel check confirms the PHP version and the tool's
  extensions in that PHP-FPM, then the tool's own page); otherwise it is put back
  on its previous version and the card says why. Waiting is shown; nothing needs
  to be done meanwhile.
- **Nothing moves a tool by itself any more.** Before, every reconfiguration chose
  the alphabetically last PHP socket (which also put 8.9 after 8.10), so
  installing a newer PHP and then reinstalling phpMyAdmin or repairing webmail
  moved the tools to it. Now a tool stays on its version while that version is
  installed; a tool with nothing bound yet gets the system's default PHP.
- **The PHP-FPM versions list** says who uses each version: "Used by 2 sites,
  phpPgAdmin, Roundcube" or "No site or tool uses it".
- **Removing a PHP version that tools run on** names them and offers "Move them to
  [version], then remove": each tool is moved and tested, then the version is
  removed; if a move fails nothing is removed and the dialog says what moved, what
  was put back and that the version stays. Sites on the version are named with
  "Open Domains" (they are moved in each site's settings, as before).
- **A tool whose PHP is missing** reads "Not ready" with the reason and the fix,
  not "Installed".

## Smaller

- phpMyAdmin on Debian 13 (`4:5.2.2-really+dfsg-1+deb13u1`) reads **5.2.2**, not
  "5.2.2-really" (alpha.101's rule needed a version after "really").

## Unchanged

Sites still choose their PHP version in their own settings. The updater, the
schema (44) and every installed server are unchanged; only the owner updates.

## Evidence on the candidate

- Agent: the whole package in a clean mount namespace, 4,702 tests passed (1,857
  top-level), 29 skipped, 0 failed; new tests for APT's simulation output (Purg,
  Remv, Inst, foreign architectures), the Frankfurt refusal, version packages and
  new orphans accepted, old orphans left alone, refusal before any unit stops, the
  same arguments for the last simulation and the removal, the package lock held,
  a changed answer refused, a failed simulation refused, pacman's print and its
  "breaks dependency" refusal, bound tools refusing a version removal, the
  read-only preview never waiting for the lock, the binding rules (kept, gone,
  defaults, numeric order), the verified move with its rollback and its typed
  refusals, the PHP check, the step policy.
- Panel and internal packages (45-minute limit, CGO off): 7,238 passed, 44
  skipped, 0 failed, 64 packages; gofmt and go vet clean.
- Web: 1,772 tests passed (new mounted tests of the version-removal dialog, the
  component removal dialog with collateral, the versions list, the not-ready row,
  the phpMyAdmin version cell and the tool's PHP version card, in English and
  Turkish); the build within every budget.
- Real system: a disposable Debian 13 virtual machine (QEMU/KVM, destroyed after
  the run): the Frankfurt removal refused by apt's own simulation with phpmyadmin
  and phppgadmin named and nothing changed; a clean removal removed exactly the
  simulated list; phpPgAdmin moved to PHP 8.5 and proved, and a move whose proof
  failed put back byte for byte ([evidence](../deploy/e2e/removal-safety/alpha104-debian13-20261023/README.md),
  archive SHA-256 `fb17d18c…ef50`). Two earlier runs failed and led to two fixes,
  recorded there.
- Browser inspection batch 27: 11 states in each of 8 configurations (EN/TR,
  light/dark, 1440/390), looked at, fixed, looked at again.
- In a fresh clone in the `archlinux` guest: the release sequence (103 → 104),
  download portal and signed manifest contracts, and the offline release-recovery
  harness (1,109 tests, 1 skipped) passed.

## Known gaps

- Nothing is measured on an installed server.
- pacman's removal path is covered by component tests only; DNF stays unreachable.
- Roundcube's move was not run in the virtual machine (same code, its own socket).
- The Components row says "Not ready" when a tool's PHP socket is missing; a PHP
  that runs without the tool's extensions is found by a move's proof, not by the
  scan.
- The uninstall request is not guarded by a request identity (as before); a
  repeated "move, then remove" repeats verified moves and finds nothing to remove.
- In Turkish, the cause of a failed move is quoted as the Agent says it (English).
- The component page's back link, Help and log Refresh are smaller than 44 px on
  a phone (as before this release).
- P0.1–P0.5 stay partial; no P0 item is closed or advanced.
