# alpha.105: a bundle with a chosen PHP is checked, confirmed and proved; a failing version tries the next (Debian 13, 2026-10-24)

A disposable Debian 13 (trixie) virtual machine, QEMU/KVM in the `archlinux` WSL
guest, booted as an overlay of the reusable base image
`/var/tmp/cp-base-images/debian13-base.qcow2` (built 2026-10-24 from
`debian-13-genericcloud-amd64-20260826-2582.qcow2`; nginx 1.26.3 and the vendor
PHP repository's key and source, no PHP; SHA-256
`e1c1173b677dcd4b19686241d3980804200106b6844868e9a0d55b4d4453caec`, in
`evidence/base-image.sha256`). The overlay was deleted after the run; the base
image is kept for later releases. Not an installed server; no installed server was
contacted.

The Agent's build-tagged test `cmd/agent/real_system_alpha105_test.go` was built
from the candidate branch at `53569e1a4efef7810f7ae192704f580cd4ae943e`
(`evidence/commit.txt`; test binary SHA-256 in `evidence/agent.test.sha256`) and
run as root in the VM with `CELIKPANEL_REAL_SYSTEM_A105=1`. It calls the Agent's
own code: `enableRepoWith`, `ServiceInstallPreviewV1`,
`installPackagesGuardedContext`, `installServiceWith`, `configureDBToolsWith`,
`provePHPToolWith`, `phpToolBindingsView`.

Archive: `cp-a105-evidence.tar.gz`, SHA-256
`183e4275b0a5811e3b7330f969965fdef03157d5696a6e836356829361d1f2f8`
(`cp-a105-evidence.tar.gz.sha256`): the run log with the VM's output
(`evidence/vm.log`), the serial console and the boot log.

## Results (both passed)

`TestRealSystemA105BundleWithAChosenPHPIsCheckedConfirmedAndProved`:

- The vendor PHP repository (Sury) was enabled through the Agent's own code, then
  the owner's bare PHP 8.3 (`php8.3-fpm`, `php8.3-cli`) was installed; the
  distribution's default PHP (8.4 from the same repository's php-defaults) was not.
- **A notable install is said before anything changes.** The simulation of
  phpMyAdmin alone named `php8.4-common`, `php8.4-mbstring`, `php8.4-mysql` and
  `php8.4-xml` as `new_php_version` 8.4 (its `php-mbstring`, `php-mysql`, `php-xml`
  dependencies pull the default PHP). The guarded real install with nothing
  confirmed was refused: "installing phpmyadmin would also install php8.4-common,
  php8.4-mbstring, php8.4-mysql, php8.4-xml, which was not confirmed; nothing was
  installed". The installed package set was unchanged.
- **The bundle with PHP 8.3 chosen brings no other PHP.** The simulation of
  PostgreSQL + PHP-FPM `php8.3-fpm` + phpPgAdmin on 8.3 together had no notable
  package: the tool's 8.3 extensions (`php8.3-pgsql`, `-mbstring`, `-xml`) satisfy
  its dependencies. A PHP version with no FPM package (5.1) was refused
  (`php_version_unavailable`).
- The three steps ran through the Agent's own install code, each checked against
  what was confirmed (nothing). PostgreSQL 17.11, the PHP 8.3 companions and
  phpPgAdmin 7.14.7 with its 8.3 extensions were installed; no `php8.4-*` and no
  `apache2*` package was. phpPgAdmin was a first install, bound to 8.3, configured
  and **proved on 8.3** (the PHP check through its socket, then its own page).

`TestRealSystemA105AFailingVersionTriesTheNext`:

- The owner added PHP 8.4 with the tool's extensions (`php8.4-fpm`, `-pgsql`,
  `-mbstring`, `-xml`); pgsql was disabled for 8.3's FPM only.
- A proof without fallback (a tool already bound) failed with the structured cause
  `missing_extensions` / `pgsql` / 8.3 ("PHP-FPM 8.3 lacks pgsql for phpPgAdmin"),
  did not move the tool, and the bindings view reported `proof_failed` on 8.3
  (Components reads "Not ready").
- A first-install proof tried 8.3 (same cause), then **8.4, which was proved**:
  phpPgAdmin's configuration names `php8.4-fpm.sock`, the record says 8.4 and the
  failed proof was cleared.

At the end nginx, php8.3-fpm, php8.4-fpm and postgresql were active.

## Limits

One distribution (Debian 13) and one vendor repository, apt only; the pacman
preview (`pacman -S --print`) is covered by component tests only and DNF stays
unreachable. The Panel's install-preview endpoint, the browser dialogue and the
bundle sequence were not part of this run (component and mounted tests cover
them). phpMyAdmin was simulated, not installed; Apache as a notable install is
covered by component tests. This is not acceptance of any P0 item.
