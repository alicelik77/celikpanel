# alpha.107: vendor repositories confined, updates, data kept, webmail updated and rolled back (Debian 13, 2026-10-24)

A disposable Debian 13 (trixie) virtual machine, QEMU/KVM in the `archlinux` WSL
guest, booted as an overlay of the reusable base image
`/var/tmp/cp-base-images/debian13-base.qcow2` (nginx, the vendor PHP repository's
own source, no PHP; SHA-256
`e1c1173b677dcd4b19686241d3980804200106b6844868e9a0d55b4d4453caec`, in
`evidence/base-image.sha256`). Each overlay was deleted after its run; the base
image is kept. Not an installed server; no installed server was contacted.

The Agent's build-tagged test `cmd/agent/real_system_alpha107_test.go` was built
from the candidate branch at `5eceb9543294c2614fbd8b597af1824120fe73c8`
(`evidence/commit.txt`; test binary SHA-256 in `evidence/agent.test.sha256`) and
run as root in the VM with `CELIKPANEL_REAL_SYSTEM_A107=1`. It calls the Agent's
own code: `enableRepoWith`, `installServiceWith`, `ServiceInstallPreviewV1`,
`installPackagesGuardedContext` with the install guard, `OSUpdatesStatusV1`,
`WriteVendorUpdateOriginsV1` (inside a service mutation job),
`ServiceRemovalPreviewV1`, `uninstallServiceWithOps`, `installRoundcubeWith`,
`configureWebmailWith`, `updateRoundcubeTo`, `restartNeeds`, `osReleaseFacts`.

Archive: `cp-a107-evidence.tar.gz`, SHA-256
`1a317d7bc6cc9c2e10d0312978a284003b54c2250292721c348d3ebca9266913`
(`cp-a107-evidence.tar.gz.sha256`): the run log, the VM's output
(`evidence/vm.log`), the serial console and the build log.

## What the earlier runs found (fixed before the recorded run)

Three earlier runs on fresh overlays (logs in `cp-a107-earlier-runs.tar.gz`,
SHA-256 `eaf0992a71f585765afab1251fe2f432f0d71327f1e5bc089f50e1a5c43764ef`):

1. `postgresql-17` needs Debian's `perl-base` security update (5.40.1-6 to
   5.40.1-6+deb13u1); the install guard named it and refused it unconfirmed, as
   designed. The evidence now confirms exactly those names, as the install dialog
   does.
2. The exact-default install of `postgresql-17` failed with "did not provide its
   exact service unit", and a PostgreSQL removal refused with "the state of
   postgresql@ could not be read": `systemctl list-unit-files` lists the template
   `postgresql@.service`, never the instance. Fixed in the Agent: an instance
   exists when systemd loads it, and a template name is never a unit.
3. Under the confinement, PHP 8.3 from Sury could not be installed: Sury's
   `php8.3-common` Breaks the distribution's `php-common`. Fixed in the Agent: a
   vendor-only version names the vendor's shared packages exactly (Sury:
   `php-common`; PGDG: `libpq5`), and after the install the confinement keeps them
   following the vendor.

## Results of the recorded run (all five passed)

`TestRealSystemA107RepositoriesAreConfinedAndTheDefaultIsExact`:

- Sury and PGDG were enabled through the Agent's own code and confined (in this
  first design by the enable itself; see below)
  (`/etc/apt/preferences.d/celikpanel-sury.pref`, `-pgdg.pref`: `Package: *`,
  the origin, priority 100). `apt-cache policy` then names the distribution's
  `php8.4-fpm` 8.4.27-1~deb13u1 and `postgresql` 17+278 as candidates, not Sury's
  or PGDG's 18.
- A plain PHP-FPM install installed the distribution's `php8.4-fpm`; a plain
  PostgreSQL install installed `postgresql-17` exactly (no `postgresql`
  metapackage, no `postgresql-18`), after confirming `perl-base`.
- PHP 8.3 (vendor-only) installed from Sury with Sury's `php-common`
  2:101+0~20261021.78; the confinement then lists `php-common` as following Sury.
  The preview of PostgreSQL 18 from PGDG is known.
- With Sury's preferences file moved away (as on a server without confinement),
  installing `php8.4-gd` was **refused by name even when confirmed**: it would
  replace the distribution's php8.4-cli, -common, -fpm, -opcache and -readline
  with Sury's builds; nothing changed. The file was put back.

`TestRealSystemA107VendorOriginsJoinAutomaticUpdates`: before, unattended-upgrade's
dry run allowed only Debian's origins. The confirmed drop-in
`/etc/apt/apt.conf.d/52celikpanel-vendor-origins` was written (`effective: true`);
the dry run then allowed `origin=apt.postgresql.org` and `origin=deb.sury.org`, and
the card's read reported Sury covered.

`TestRealSystemA107PostgreSQLRemovalKeepsTheCluster`: with a database
`a107evidence` in cluster 17/main, the removal preview said it keeps the data
(`/var/lib/postgresql/17`); the removal ran `apt-get remove`; the data directory
and `/etc/postgresql/17/main` stayed; the install preview then named the existing
data; the reinstall started the same cluster (17 main, online) and the database
was there. **Debian's own purge of `postgresql-17`, run by hand afterwards for the
record, deleted the cluster's data and configuration** (`PG_VERSION` and
`postgresql.conf` gone, no cluster listed). CelikPanel therefore keeps `remove`.

`TestRealSystemA107WebmailUpdateRollsBackThenUpdates`: Roundcube 1.6.15 (its
signature verified with Roundcube's key) was installed and configured, and
answered. The update check named 1.6.20 on the 1.6 line. With the new tree's
proof forced to fail, the update archived the current webmail, swapped, and put
1.6.15 back (`rolled_back`, `PROOF_FAILED`); webmail answered and its
configuration was unchanged. The real update then installed 1.6.20, proved it
and kept the configuration byte for byte.

`TestRealSystemA107RestartsAndReleaseAreRead`: with needrestart installed, the
read used needrestart, knew the services and the reboot state (none waiting);
the release facts read Debian 13 trixie, Sury's suite trixie, PGDG's
trixie-pgdg, PHP lines and the clusters.

## After the review: confinement is the owner's action (second recorded run)

The recorded run above was made with the first design, in which enabling a
repository (and the Panel's first start) confined it by itself. After the
review that was removed: the repository card shows the plan and only the
confirmed text is written. One short run on a fresh overlay of the same base
image recorded the new design, from the candidate at
`f6c44f2b65b6cce4fdd0ed49668c90600e233a8e`. Archive
`cp-a107-confine-evidence.tar.gz`, SHA-256
`6a360711fe6b1db35140f04e3fd6bd76a710dcc3f280ec31e253f8fe624f72f7`.

`TestRealSystemA107ConfinementIsTheOwnersShownFirstAndKeptAsTheOwnerLeftIt`
(passed):

- Sury and PGDG were enabled through the Agent's own code; both read
  `not_written` afterwards: nothing confined them.
- Before confining, an older PGDG client was installed by hand
  (`postgresql-client-18` 18.3-1.pgdg13+1, PGDG listing 18.6-1.pgdg13+2), and
  `apt-get -s upgrade` upgraded it.
- The plan for PGDG named the exact text and its SHA-256, kept `libpq5`,
  `postgresql-client-18` and `postgresql-client-common` following PGDG, said the
  client is still upgraded from PGDG, and that `apt-get upgrade` would do the same
  with and without the rule (39 updates either way). Confirmed with that SHA-256,
  the written file was byte for byte the shown text. Sury was confined the same
  way.
- Confined, `apt-get -s upgrade` still upgraded `postgresql-client-18` from PGDG,
  and no upgrade replaced a distribution package with a vendor build.
- PHP 8.3 (Sury only) was refused unconfirmed with `php-common` named as
  `vendor_shared`; confirmed, it installed, and Sury's rule then keeps `php-common`
  following Sury.
- The owner deleted `/etc/apt/preferences.d/celikpanel-pgdg.pref`: the state read
  `removed_by_owner`. Neither the post-install refresh nor installing
  `postgresql-18` from PGDG wrote it again.

## Limits

One distribution (Debian 13), apt only; pacman and DNF by component tests only.
The Panel's endpoints and a Panel restart, the browser cards and dialogs,
Node.js patches and an application's account move were not part of these runs
(component and mounted tests cover them; the Panel no longer confines anything
at its start). Sury keeps only its newest builds, so the older-vendor-version
case was run with PGDG. This is not acceptance of any P0 item.
