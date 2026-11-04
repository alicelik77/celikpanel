# alpha.120 lab check, run 1 (2026-11-04, 08:08-08:28 UTC)

**Candidate:** `release/alpha120` at `74121094f`, identity 120 (previous 119, `7e4ebeb49`).

**Guest:** one Ubuntu 24.04 VM installed fresh at alpha.119 from the lab portal, with BIND, mail
and MariaDB, to mirror Boston. The lab portal offered only 120 as the update.

**Method:** lab only; everything went through the panel API like a user, and SSH was used read-only.

**Result:** no product defect found. Evidence: `raw/R10-u119.log`.

| Check | Result |
|---|---|
| 1 Python installed on 119 from Components (point updates confirmed); update 119→120 through the panel | PASS (82 s); services active, `nginx -t` ok, panics 0 |
| 2 PostgreSQL row | `not_installed`, kind `service`. The row's `versions` field is empty; the distribution candidate (`/service/candidate`) is `16`, the same shape as in 118/119 |
| 2 Python row | PASS: `package_version_short 3.12.3` (python3 / python3-venv `3.12.3-0ubuntu2.1`), one instance `3.12` |
| 3 Security emergency list | PASS: `GET /security-emergencies` answers 200 (empty list) |
| 3 Withdraw step for PostgreSQL, then undo | Opened: `stage: withdrawn`. Cancelled: `resolved`. **The `/managed-services` row carries no emergency or withdrawn field while it is open.** In 120 the emergency moved to its own page, so this may be intended. |
| 4 SPA routes `/components`, `/services`, `/help` | PASS: 200 each |

**Cleanup:** the guest, the lab portal, the dists and the work directory were removed.
