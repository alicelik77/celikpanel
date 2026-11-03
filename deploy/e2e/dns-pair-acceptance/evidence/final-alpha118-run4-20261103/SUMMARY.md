# alpha.118 lab acceptance, run 4 (2026-11-03, 18:30-19:15 UTC)

**Candidate:** `release/alpha118` at `42091f3f9` (identity 118). The build came from the remote ref
through a bundle; no branch was created in the shared repository. Previous release:
`v0.1.0-alpha.117` (`b22348ff8`).

**Method:** lab only, through the panel API. SSH was used read-only.

**Result:** all items pass. No product defect was found, so the stop rule was never triggered.

## A: Debian 13, fresh install

Installed through get.sh, then setup succeeded (`raw/run-singleA.txt`).

| # | Item | Result | Evidence |
|---|---|---|---|
| 1 | SSH jail tools | PASS | `raw/R4ssh-a118.log` |
| 2 | Catalogue apps | PASS | `raw/R4apps-a118.log` |
| 3 | D4: PHP uploads after a disk limit | PASS | `raw/R4lim2-a118.log` |
| 4 | n8n | PASS | `raw/R3n8n-a118.log`, `raw/run-R4n8n.txt` |
| 5 | PostgreSQL and phpPgAdmin | PASS (API) | `raw/R4pg-a118.log` |

### 1. SSH jail tools

- The Access view reports `jail_checked:true` and `jail_missing:null`.
- Turning on file access with SSH returned 200.
- The jail `/var/www/celikpanel/subscriptions/3/sites/bin` holds zip, unzip, nano, vi, vim, rsync,
  less and bash.
- `sshd -t` is ok.

### 2. Catalogue apps

- Components `php8.4-fpm` came from distribution packages.
- **phpBB is withheld:**
  - The catalogue does not list it, for the administrator or the customer.
  - Offering it gets `400 unknown application`.
  - Installing it gets `400 APP_UNKNOWN`.
- **PrestaShop 9.2.0:**
  - Set up in 108 s.
  - `admin_url` is `https://pres.apps118.test/admin018xthag4wiyj4oomjg/`.
  - The site answers `302 -> /index.php`.
- **Drupal 11.4.11:**
  - The site answers 200.
  - Removal returned 200 with the database removed.
  - The installed list is now empty, and the document root is back to the placeholder `index.php`.

### 3. D4: PHP uploads after a disk limit

- First attempt (`raw/R2lim-a118.log`): it never blocked. That subscription's only measurement was
  taken before the 2.5 MB upload, and disks are re-measured every 6 hours. This is a lab-procedure
  issue, not a product defect.
- Second attempt, on a new subscription whose first measurement was 2.5 MB:
  - Set the limit to 1 MB. After the next round, `uploads_blocked=true` and `site8.conf` has
    `file_uploads = off`. A PHP upload was refused.
  - Lifted the limit. After the next round (19:11:38) `site8.conf` was rewritten without the flag,
    and a PHP upload succeeded (`file_uploads=true files=1`).

### 4. n8n

- Node.js 24.22.2 was installed and n8n was offered.
- For the administrator's own customer:
  - The install finished in about 8 minutes.
  - The unit is active and listens on `127.0.0.1:21000/21001`.
  - HTTPS answers 200 with the title "n8n.io - Workflow Automation".
  - The owner login (`/rest/login`) returned 200 as `global:owner`, with the session cookie set.
  - The journal has no native-module errors. It ends with "Editor is now accessible".
- Suspend: the unit became inactive and disabled, and the site showed 503 "Site suspended".
- Resume: the unit became active again and the site answered 200.
- Licence refusal (409 `SERVICE_APP` / `licence_reseller`), each time:
  - a reseller plan that names `app:n8n`;
  - an install under a reseller's customer;
  - the same install sent by the administrator.
  - The reseller-customer view shows `refused_by: licence`.

### 5. PostgreSQL and phpPgAdmin

- PostgreSQL first appeared as `not_installed`, with no `requires_missing`. The distribution
  candidate is `17`, and PGDG is available but not enabled.
- PostgreSQL 17 installed from the distribution (no package chosen). phpPgAdmin installed.
- The removal review shows `companion_tools:["phppgadmin"]`.
- Removed PostgreSQL alone, which is the unticked choice in the API. phpPgAdmin's row then showed
  `needs_missing:["postgresql"]`.
- Removed phpPgAdmin.
- The tick state and the two buttons exist only in the web page, so they were not seen here.

## B: Ubuntu 24.04, alpha.117 (BIND) updated to the candidate

| # | Item | Result | Evidence |
|---|---|---|---|
| 6 | Update to 118 | PASS | `raw/Bupd-u117.log`, `raw/run-Bupd2.txt`, `raw/Bbusy-u117.log` |
| 7 | SSH jail tools after the update | PASS | `raw/R4ssh-u117.log` |

### 6. Update to 118

- The update succeeded in 72 s through the panel.
- Migrations 057 to 061 are present, services are active, `nginx -t` is ok, and there are no panics.
- The first start, sent to the alpha.117 panel right after the lab created a domain, got
  `409 PANEL_UPDATE_BUSY`. No operation was active a moment later, and the retry was accepted.
  This also happened in run 1.

### 7. SSH jail tools after the update

- The Access view showed `jail_missing` = `["unzip","zip"]`.
- Turning SSH on got `409 FILE_ACCESS_SSH_TOOLS_MISSING`, `details ["unzip","zip"]`.
- The administrator's `POST /api/v1/service/file-access-tools {"packages":["unzip","zip"]}`
  returned 200 with `installed ["unzip","zip"]`, and nothing else was installed.
- After that, turning SSH on returned 200, and the jail has all the tools.

## Also seen

- The mailbox quota was enforced right after mail was installed: 552 came 77 s after the mailbox
  was created (`raw/R2a-a118.log`).

## Cleanup

Guests a118 and u117, the lab portal, the dists and `/var/tmp/a118r1` were removed.
