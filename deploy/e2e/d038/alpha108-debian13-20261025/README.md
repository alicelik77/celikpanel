# alpha.108 real-system evidence (Debian 13, 2026-10-25)

One disposable Debian 13 virtual machine (QEMU/KVM inside the `archlinux` WSL
guest), an overlay of the kept base image `debian13-base.qcow2`
(SHA-256 `e1c1173b677dcd4b19686241d3980804200106b6844868e9a0d55b4d4453caec`:
nginx, unattended-upgrades, the owner's own Sury source, no PHP, no PostgreSQL).
The Agent's `realsystem` test binary was built from branch
`feat/alpha108-safe-lanes` at `dcbc8a4a75c56e26b0d90215282e9f96a21f66df`
(binary SHA-256 `88089da725ff932c5e8aa9e8cda93ee2d8340dc66e59b9433fa3c2ddf98f2d6a`)
and run with `CELIKPANEL_REAL_SYSTEM_A108=1`, in this order, on the same VM. Nothing
ran on an installed server; the overlay was deleted afterwards and the base image
kept.

| Test | Result | What it proves |
|---|---|---|
| `TestRealSystemA108PostgreSQLSideBySide` | PASS (70 s) | PostgreSQL 17 from Debian by its exact name (Debian's `perl-base` security update named and confirmed); a database `a108shop` with its own role and 1000 + 37 rows; **PGDG through its vendor lane** (repository, confinement, origin in one confirmed operation); PostgreSQL 18 through the install preview (PGDG's `libpq5` named and confirmed) as cluster 18/main **on the shown port 5433**; port 5433 → 5440 after the plan (one changed line, its SHA-256), restart and proof (`5440 180006`); the database copied 17 → 18 with every table's row count equal (orders 1000/1000, shop.users 37/37), the role created and signing in with its own password; removing postgresql-17 **refused** (`REMOVAL_HOLDS_DATABASES`) while it held the database; the source dropped only as the explicit step; 17 then removed (`apt-get remove`) with its data directory kept. |
| `TestRealSystemA108PHPToolProof` | PASS (44 s) | Under the Agent unit's `UMask=0027`: the PHP check file is written 0755/0644 and phpPgAdmin's proof passes; Boston's state (0750/0640) reproduced: PHP-FPM 8.4 could not open the check (HTTP 403), said as `check_unreadable` while the page answers; **Check again** repairs it and passes; a same-version "Use PHP 8.4" only proves, nothing rewritten. |
| `TestRealSystemA108RemovalTraces` | PASS (10 s) | The removal preview of PHP-FPM 8.4 names the owner's nginx block (`/etc/nginx/sites-enabled/a108-owner.conf` line 6, `unix:/run/php/php8.4-fpm.sock`), the owner's unit `a108-owner.service` (Requires, After) and the connected process; once they are gone it names none. Nothing removed. |
| `TestRealSystemA108VendorLane` | PASS (143 s) | Without Sury PHP 8.5 has no candidate (the default lane). The Sury lane plan shows three steps with their files, texts and digests; one confirmation writes them (`completed: repository, confinement, updates`; `effective: true`); the written confinement and drop-in are byte for byte the confirmed ones; `unattended-upgrade --dry-run` lists `origin=deb.sury.org`; the same confirmation again writes nothing (`LANE_PLAN_CHANGED`). The key reads `valid` until 2028-02-04 (newest self-signature), not 2021. PHP 8.5 from the version list: the preview names Sury's `php-common` (`vendor_shared`), the unconfirmed install is refused, the confirmed one installs. Disabling Sury removes nothing: `php8.5-fpm` stays installed and active, and the repository reports it as no longer updated. |

**A defect found and fixed by this run.** The first run of the same tests (commit
`60794bfab`, archive `cp-a108-first-run.tar.gz`) failed the PostgreSQL test:
with PGDG confined, Debian 13's own `postgresql-common` 278 stays, and the
`postgresql-18` package from PGDG created no cluster in its postinst, so its unit's
assertion failed ("Assertion failed on job for postgresql@18-main.service"). A
manual diagnosis in a separate overlay confirmed it and that `pg_createcluster 18
main -p 5433` works with that `postgresql-common`. The install now creates NN/main
on the first free port before starting the unit (never over an existing cluster,
never against `create_main_cluster = false`); commit `dcbc8a4a7`. The vendor-lane
test of the first run failed on its own assertion (it expected the preview state
`checked`; the Agent answers `known`); corrected in the same commit. The tools and
traces tests passed in both runs.

**Not covered here.** Ubuntu 24.04: no VM run; Boston's phpPgAdmin cause is the
Agent's umask, reproduced on Debian 13, and the Ubuntu page shape is covered by the
real package's page fixtures in the Agent tests. Automatic updates off (the lane's
refusal) by component tests only: the base image has them on. A Panel restart is
not part of the run. Nothing measured on an installed server.

Archives:

- `cp-a108-evidence.tar.gz` — SHA-256 `effaf7cee44bbcba9465405d41f2ba0d6e43cbe924d68570072d4b19a1604698`
  (run log, VM log, build log, serial log, commit, binary and base image digests)
- `cp-a108-first-run.tar.gz` — SHA-256 `80e0917acac6cf76dde9e88075c5b38bc4180795f286964e8bbea715aa4d90e7`

## Review run (2026-10-25)

After the release review, `TestRealSystemA108PostgreSQLSideBySide` and
`TestRealSystemA108VendorLane` ran again on the same base image at commit
`e138da242a6a7a6b9d49e7a09bcba21d7b57ccc8` (binary SHA-256
`69b95731320570b2ad7c3d4d49fe3ededf827f7e420e80f1046b6d5dc8c6f244`); both PASS. New in
them: the copy keeps the source's write counters; an unchanged source reads
unchanged; an `UPDATE` of one row after the copy (row counts the same) reads
`SOURCE_CHANGED`, and the drop of the source with the expectation is refused with it
and the source stays. With the base image's own `/etc/apt/sources.list.d/php.list`,
the Sury lane names the repository step `owner_source` and offers only the
confinement and the automatic updates; no CelikPanel source is written. The run
script now stops the image's apt timers and waits for a running unattended-upgrade
before the tests (an earlier attempt of this run met the dpkg lock held by it); two
earlier attempts found the Agent's private directory missing on a VM without an
installed Agent (fixed in `e138da242`) and that lock.

- `cp-a108-review-evidence.tar.gz` — SHA-256 `79430a30150f932a00b9dfce497c8b195fa2419719f31a6a5753bbb150b1429b`
