# alpha.121 lab check, run 1 (2026-11-04, 11:32-11:50 UTC)

**Candidate:** `release/alpha121` at `850ade435` (identity 121; previous 120 / `bb3aa249d`).

**Guests:**
- **U:** Ubuntu 24.04, installed at 120 (BIND, mail, MariaDB) to recreate Boston.
- **D:** Debian 13, installed fresh at 121.

**Rules:** lab only, panel API like a user. SSH changes were lab load only: the dpkg lock hold and
deleting the PGDG list files.

**Result:** no product defect found. One scenario deviation (item 1) and two observations (items 7
and 8).

Evidence: `raw/R11U-u120.log`, `raw/R11D-d121.log`, `raw/db-servers-clusters.txt`.

## Results

| # | Check | Result |
|---|---|---|
| 1 | U on 120: distribution PG 16 + phpPgAdmin + one PG database | PASS (phpPgAdmin brings php8.3-pgsql; `php -m` shows pgsql) |
| 1 | Remove PostgreSQL | **Refused:** `409 SERVICE_HAS_DEPENDENTS ["pg11_test_keep16"]` (the review says `hosted.blocks_removal`). PG 16 stayed. |
| 1 | Enable PGDG from Components (vendor lane) | PASS: enabled and confined (pin file); PG 10-18 offered |
| 6 | Update with dpkg's fcntl lock held | `409 HOST_MUTATION_BUSY / package_manager_active`, no update record, readiness `false` |
| 6 | After releasing the lock | readiness `true`; one start with a new request id → 202 → succeeded in 84 s |
| 2 | Package install right after the update | Not refused (202), so the busy text did not appear |
| 4 | PGDG 17 preview (18 not installed) | `libpq5` only (`vendor_shared`, 16.15-0ubuntu0.24.04.1 → 18.6-1.pgdg24.04+2) |
| 5 | PGDG list files deleted, then preview 18 | `state known`, `lists_refreshed: pgdg`; no "could not be checked" |
| 3 | PGDG 18 preview | `libpq5` 16.15→18.6, `postgresql-client-common` and `postgresql-common` 257build1.1→293.pgdg24.04+1, all `vendor_shared` (shown as "replaced"), so confirmation is needed |
| 3 | PGDG 18 install with the three confirmed | PASS |
| 7 | D: PGDG 18 preview | names only `libpq5`; common stays 278 |
| 7 | D: PGDG 18 install | PASS: 18 main online on 5432; database-servers lists PostgreSQL 18.6 |
| 8 | D: host-name notice | `GET /dns/server-hostname` → `state: not_a_domain_name`, hostname `frankfurt` |

## Notes

- **Item 1 deviation.** Because PostgreSQL removal was refused, U ran with the 16 cluster still
  present.
- **Item 3, U after the PG 18 install.**
  - `pg_lsclusters` shows 16 main (5432) and 18 main (5433), both online.
  - The clusters API lists both clusters; `database-servers` lists the default PostgreSQL (16.15).
  - `php -m` shows pgsql.
  - The old 16 data directory is still there (`/var/lib/postgresql/16/main`, PG_VERSION, base).
- **Item 7 observation.** On D the `libpq5` note has no from or to version: `libpq5` was not
  installed before.
- **Item 8 observation.** The lab guest's hostname is short (`frankfurt`, not an FQDN), so the
  missing-record notice and the "Add the record" review could not be exercised.

## Cleanup

Guests, lab portal, dists and work directory removed.
