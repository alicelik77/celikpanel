# alpha.104 real-system evidence: removals and PHP tool moves on Debian 13 (2026-10-23)

A disposable Debian 13 (trixie) virtual machine, QEMU/KVM in the `archlinux` WSL
guest, base image `debian-13-genericcloud-amd64-20260826-2582.qcow2` (SHA-256
prefix `cbf3e1f588f02f8d`), destroyed after the run. Not an installed server; no
installed server was contacted. The Agent's build-tagged test
`cmd/agent/real_system_alpha104_test.go` was built from a fresh clone of the
candidate branch with the candidate's final code (its commit is in
`evidence/commit.txt`; the test binary's SHA-256 in `evidence/agent.test.sha256`)
and run as root inside the VM with `CELIKPANEL_REAL_SYSTEM_A104=1`.

Archive: `cp-a104-evidence.tar.gz`, SHA-256
`617147598c5cdbb67e2356aa1cc486a1c649837433c142101933d6c536982ec1`
(`cp-a104-evidence.tar.gz.sha256`). It holds the run log, the VM's command output
(`evidence/vm.log`), the build log, the serial console and the disk use before and
after.

## What was set up

Frankfurt's package state from the distribution only: nginx, php8.4-fpm, php-fpm,
the eleven PHP 8.4 packages the Panel installs beside a version, redis-server,
then phpmyadmin `4:5.2.2-really+dfsg-1+deb13u1` and phppgadmin `7.14.7+dfsg-1`
(`--no-install-recommends`; phpMyAdmin's database configuration declined). The
log also records APT's own simulations of the old command: `apt-get -s purge
--auto-remove` of the eleven PHP 8.4 packages purges 74 packages, among them
phppgadmin, php-fpm, phpmyadmin, php-mysql and php8.4-readline.

## Results

- `TestRealSystemA104FrankfurtRemovalIsRefused` **passed**: the guarded removal of
  the eleven PHP 8.4 packages was refused before anything changed
  (REMOVAL_WOULD_REMOVE_OTHERS) naming php-fpm, phpmyadmin, phppgadmin,
  php-mysql, php-phpmyadmin-sql-parser, php-symfony-config,
  php-symfony-filesystem, php-twig and php-twig-i18n-extension; no unit was
  stopped and the installed package set was unchanged.
- `TestRealSystemA104CleanRemovalRunsTheSimulatedList` **passed**: removing
  redis-server ran; the packages that disappeared (liblzf1, redis-server,
  redis-tools) are exactly the simulated list.
- `TestRealSystemA104ToolMoveIsProvedAndRolledBack` **passed** after PHP 8.5 was
  installed from the vendor repository (packages.sury.org): phpMyAdmin and
  phpPgAdmin answered on 8.4 through the loopback tools server; phpPgAdmin was
  moved to 8.5 (the Agent installed php8.5-pgsql, php8.5-mbstring and php8.5-xml)
  and the PHP check through its socket and its own page proved it; it was moved
  back to 8.4; then, with pgsql disabled for PHP 8.5's FPM only, a move to 8.5 was
  refused by the proof ("PHP-FPM 8.5 lacks pgsql for phpPgAdmin"), the previous
  configuration was restored byte for byte and phpPgAdmin answered on 8.4 again;
  phpMyAdmin and phpPgAdmin were still bound to 8.4.

## What the earlier runs found (kept in the record, not in the archive)

Two earlier runs of the same harness on the same day failed the move test and led
to two fixes in the candidate: the tool's front page alone did not prove its
PHP extensions (phpPgAdmin's page passed without pgsql), so a PHP check served
through the tool's own socket was added; and right after `systemctl reload nginx`
the old workers still answered for the previous socket (PHP 8.4.27 answered a
binding just moved to 8.5), so the proof now waits up to 20 seconds for the
reloaded configuration.

## Limits

One distribution (Debian 13), apt only; the pacman path is covered by component
tests only. Webmail (Roundcube) was not installed in the VM; its move uses the same
code with its own socket and page. The Panel, the browser and the Agent RPCs were
not part of this run (component tests cover them). This is not acceptance of any
P0 item.
