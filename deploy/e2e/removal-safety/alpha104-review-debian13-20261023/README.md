# alpha.104 review: the real uninstall refuses, and webmail is installed and moved, on Debian 13 (2026-10-23)

The second real-system run of the alpha.104 candidate, after the independent
review. The first run is in [`../alpha104-debian13-20261023/`](../alpha104-debian13-20261023/README.md)
and stays as it was.

A disposable Debian 13 (trixie) virtual machine, QEMU/KVM in the `archlinux` WSL
guest, base image `debian-13-genericcloud-amd64-20260826-2582.qcow2` (SHA-256
prefix `cbf3e1f588f02f8d`), destroyed after the run. Not an installed server; no
installed server was contacted. The Agent's build-tagged test
`cmd/agent/real_system_alpha104_test.go` was built from a fresh clone of the
candidate branch (its commit is in `evidence/commit.txt`, the test binary's SHA-256
in `evidence/agent.test.sha256`) and run as root inside the VM with
`CELIKPANEL_REAL_SYSTEM_A104=1`, in two phases: before PHP 8.5 exists, then after
it is installed from the vendor repository (packages.sury.org).

Archive: `cp-a104r-evidence.tar.gz`, SHA-256
`8f9dca047f4412fa1196269ca02a199ac740bcb879b6846aa57b892492a44ce2`
(`cp-a104r-evidence.tar.gz.sha256`). It holds the run log, the VM's command output
(`evidence/vm.log`), the build log, the serial console and the disk use before and
after.

## What was set up

As in the first run: nginx, php8.4-fpm, php-fpm, the eleven PHP 8.4 packages the
Panel installs beside a version, redis-server, then phpmyadmin
`4:5.2.2-really+dfsg-1+deb13u1` and phppgadmin `7.14.7+dfsg-1`
(`--no-install-recommends`; phpMyAdmin's database configuration declined). APT's own
simulation of the old command (`apt-get -s purge --auto-remove` of the eleven
packages) purges 74 packages.

## Results (all passed)

Phase one (PHP 8.4 only):

- `TestRealSystemA104FrankfurtRemovalIsRefused`: refused before anything changed
  (REMOVAL_WOULD_REMOVE_OTHERS) naming php-fpm, phpmyadmin, phppgadmin, php-mysql,
  php-phpmyadmin-sql-parser, php-symfony-config, php-symfony-filesystem, php-twig
  and php-twig-i18n-extension; no unit stopped (the stop hook did not run); the
  installed package set unchanged.
- `TestRealSystemA104CleanRemovalRunsTheSimulatedList`: removing redis-server
  removed exactly the simulated list (liblzf1, redis-server, redis-tools).
- `TestRealSystemA104WebmailInstalledOnItsPHP` (new): Roundcube 1.6.15 downloaded,
  checked and installed by the Agent's own install code, its PHP extensions for the
  only PHP (8.4) ensured through the install guard (simulated first; nothing would
  be removed), the webmail server configured on `/run/celikpanel-webmail.sock` with
  `fastcgi_pass unix:/run/php/php8.4-fpm.sock`, and Roundcube answered as itself
  through its socket on 8.4.

Phase two (PHP 8.5 installed):

- `TestRealSystemA104ToolMoveIsProvedAndRolledBack`: phpPgAdmin moved to 8.5 (the
  Agent installed php8.5-pgsql, php8.5-mbstring and php8.5-xml) and proved; moved
  back; with pgsql off for 8.5's FPM only, a move was refused by the proof ("PHP-FPM
  8.5 lacks pgsql for phpPgAdmin"), restored byte for byte, and phpPgAdmin answered
  on 8.4 again. **Step three (new): the Agent's real uninstall of PHP-FPM 8.4**
  (`uninstallServiceWithOps` with the host's own operations) was refused with
  REMOVAL_TOOLS_BOUND naming phpmyadmin, phppgadmin and webmail ("… run on PHP 8.4;
  move them to another PHP version first or keep this version; nothing was
  changed"), the installed package set was unchanged and php8.4-fpm stayed active.
- `TestRealSystemA104WebmailMoveIsProvedAndRolledBack` (new): webmail moved to 8.5
  (the Agent installed php8.5-intl, php8.5-sqlite3 and php8.5-zip) and proved
  through its socket; moved back to 8.4; with intl off for 8.5's FPM only, a move
  was refused by the proof with the structured cause `missing_extensions` /
  `intl` / `8.5` ("Webmail (Roundcube) did not work on PHP 8.5 (PHP-FPM 8.5 lacks
  intl for Webmail (Roundcube)); it was put back on PHP 8.4"), the webmail
  configuration was restored byte for byte and webmail answered on 8.4 again.

At the end both tool configurations named `php8.4-fpm.sock`, and php8.4-fpm,
php8.5-fpm and nginx were active.

## Limits

One distribution (Debian 13), apt only; pacman's path is covered by component tests
only, and DNF stays unreachable. The Panel's "move, then remove" flow, the browser
and the Agent RPCs were not part of this run (component tests cover them). Roundcube
answered its login page; no mail account was used. This is not acceptance of any P0
item.
