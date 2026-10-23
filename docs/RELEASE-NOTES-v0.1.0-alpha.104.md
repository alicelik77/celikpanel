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

## Corrections after the independent review

An independent review of this candidate found gaps; the owner's rule was that all of
them ship in alpha.104. Each is in source with tests:

- **No tool moves before the package manager accepted the removal.** "Move them to
  …, then remove" first asks the package manager, for the same version, what the
  removal takes, and moves nothing unless it answered and takes nothing else
  ("No tool was moved and nothing was removed: the package manager could not be
  asked now …"). The move is offered only when the dialog's own check answered.
- **Moves, then a refusal or a lost answer, are both said.** The dialog names the
  tools that moved, then the refusal, then that the version was not removed — or,
  when the removal's answer was lost, that whether it was removed is not known —
  and reads the versions, the check and the components again. The removal request
  carries a request identity: a lost answer is asked for again once, never run
  twice.
- **"No other PHP version is installed"** is said only when the installed versions
  were read; otherwise "could not be read now" with Retry.
- **A failed move's reason is said in English or Turkish** from its facts (missing
  extensions, the PHP version that answered, an HTTP status, nginx refusing or not
  reloading); a reason the screen does not know is quoted as "the server said: …".
  When the previous configuration could not be fully put back, the sentence says
  which step failed. A gateway's page in place of the answer is "not known".
- **A tool whose PHP extension packages are missing for its version is "Not
  ready"**, with "choose PHP x.y again in Manage: the move installs them". This
  reads installed packages, not whether PHP loaded them; the move's proof checks
  the loaded extensions.
- **Another tool's PHP problem no longer fails an install.** Installing phpMyAdmin
  or phpPgAdmin while another tool's PHP needs attention installs it and shows a
  warning naming that tool and the fix.
- **A tool reinstalled after its PHP version was removed** gets the default PHP,
  not the removed version it was once bound to.
- **Owner edits are not overwritten (D-022).** A database tools or webmail
  configuration changed outside CelikPanel (not one of the forms CelikPanel wrote
  since alpha.1, nor an empty file) is not regenerated; the answer names the
  file and the next action. The database tools configuration is written atomically
  and put back with its mode and owner when nginx refuses it.
- **A removal leaves alone** PHP extension packages of other PHP versions (sites
  may use them) and packages that were already unused (no false refusal from an
  old orphan). It is checked again before and after its services are stopped; if
  the answer changed, each service is put back exactly as the owner left it
  (enabled or running only if it was) and nothing is removed.
- **An install never removes a package.** The component install and a tool's
  extension install are simulated first and refused, naming the packages, if they
  would remove any.
- **Wording.** "Could not be checked" no longer names apt on pacman systems; a
  refusal for packages that depend on the removed one no longer says "remove the
  component they belong to".
- **Phone targets.** The component page's back link, Help and the log's filter and
  Refresh are 44 px tall on a phone.

## Unchanged

Sites still choose their PHP version in their own settings. The updater, the
schema (44) and every installed server are unchanged; only the owner updates.

## Evidence on the candidate

- Agent: the whole package in a clean mount namespace, 4,709 tests passed (1,864
  top-level), 29 skipped, 0 failed; besides the tests listed for the first round,
  new tests for other versions' extension packages and old orphans left alone, the
  check again after the stop with the services started again, an install that
  would remove packages refused, the stale record, owner-edited configurations
  kept (every form CelikPanel wrote since alpha.1, and an empty file, recognised), the exact restore step,
  the structured cause and the extension-package presence.
- Panel and internal packages (45-minute limit, CGO off): 7,239 passed, 44
  skipped, 0 failed, 64 packages; gofmt and go vet clean (also with the
  real-system build tag).
- Web: 1,783 tests passed (new: the move offered only after the check answered,
  moves then a refusal, moves then a lost answer, unread versions with Retry, the
  missing-extensions row and card, the cause in English and Turkish, a gateway's
  page as "not known", the install warning, the removal route asked again, the
  phone targets); the build within every budget.
- Real system, two disposable Debian 13 virtual machines (QEMU/KVM, destroyed after
  each run). First run: the Frankfurt removal refused by apt's own simulation with
  phpmyadmin and phppgadmin named and nothing changed; a clean removal removed
  exactly the simulated list; phpPgAdmin moved to PHP 8.5 and proved, and a move
  whose proof failed put back byte for byte
  ([evidence](../deploy/e2e/removal-safety/alpha104-debian13-20261023/README.md),
  archive SHA-256 `61714759…2ec1`; two earlier runs failed and led to two fixes,
  recorded there). Second run, after the review, on the final Agent code: the same
  two removals; Roundcube installed by the Agent's own code and answering on PHP
  8.4; webmail moved to PHP 8.5 and proved, moved back, and a move whose proof
  failed (intl off for 8.5's FPM) refused with the cause `missing_extensions`/intl
  and restored byte for byte; the Agent's real uninstall of PHP-FPM 8.4 refused
  with REMOVAL_TOOLS_BOUND naming phpmyadmin, phppgadmin and webmail, the package
  set unchanged and php8.4-fpm still active
  ([evidence](../deploy/e2e/removal-safety/alpha104-review-debian13-20261023/README.md),
  archive SHA-256 `8f9dca04…2a44ce2`).
- Browser inspection batch 27: 17 states in each of 8 configurations (EN/TR,
  light/dark, 1440/390), including the review states; no raw key, no English on a
  Turkish page, no control under 44 px on a phone, no sideways overflow; looked at,
  fixed (the refusal after moves now stands beside them on a phone; a card lacking
  extensions preselects its version), looked at again.
- In a fresh clone in the `archlinux` guest: the release sequence (103 → 104),
  download portal and signed manifest contracts, and the offline release-recovery
  harness (1,109 tests, 1 skipped) passed.

## Known gaps

- Nothing is measured on an installed server.
- pacman's removal and install-guard paths are covered by component tests only; the
  DNF path stays unreachable.
- The Panel's "move, then remove" request, the browser and the Agent RPCs were not
  part of the virtual-machine runs (component and mounted tests cover them).
- "Not ready · missing extensions" reads installed packages; a PHP that has the
  packages but does not load an extension is found by a move's proof, not by the
  scan.
- P0.1–P0.5 stay partial; no P0 item is closed or advanced.
