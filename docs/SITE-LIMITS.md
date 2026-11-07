# Site settings within limits

*Design and implementation notes · alpha.128 · migration 066*

This page describes what alpha.128 builds for the gaps G2, G5, G10, G13, G17 and G21 of the
Plesk gap list, how the pieces fit together, and the interface other features use to add their
own limited settings. It names what is **not** done yet. It is not a statement that the lab round
has passed: the per-PHP-version checks in "Evidence still to collect" are open.

## 1. The model: one chain for every limited setting

A limited setting has a key (`php.memory_limit_mb`) and follows the rights chain of D-042:

```
server  ⊇  reseller (reseller plan or Custom reseller)  ⊇  plan or Custom subscription  ⊇  site
```

- Each level may hold a ceiling for a key. No ceiling at a level is "up to the level above".
- The **effective ceiling** is the tightest level, together with the level that set it
  (`set_by`: `server`, `reseller`, `plan`, `subscription`) and its name. A refusal
  (`SETTING_ABOVE_LIMIT`, vars `key`, `value`, `limit`, `set_by`, `set_by_name`) names who can
  raise it.
- A **floor** key is the opposite: the smallest value a site may choose
  (`cron.min_interval_minutes`); its tightest level is the largest value.
- Plans hold ceilings and permissions, never preset values. A site's own values live in the
  native files (pool, nginx), never in the panel's database (D-022).
- A subscription bound to a plan reads the plan's ceilings. Leaving the plan (P3: any change makes
  it Custom) copies the plan's ceilings and overuse policy to the subscription; binding a plan or
  Unlimited removes the subscription's own ceilings. The same for resellers and reseller plans.

Storage (migration 066, additive):

| Table / column | Meaning |
|---|---|
| `setting_policy(key, server_ceiling, server_default, customer_editable)` | The server level. Seeded with the owner's ceilings: 512 MB memory, 300 s, 256 MB upload, 10000 input fields, 20 PHP processes; scheduled tasks at least 5 minutes apart; no ceiling for database connections and task counts. |
| `setting_ceilings(owner_kind, owner_id, key, value)` | Ceilings of a plan, reseller plan, Custom subscription or Custom reseller; removed with the owner by triggers. |
| `service_plans` / `subscriptions`: `overuse_policy`, `notify_percent_disk`, `notify_percent_traffic` | G10. |
| `reseller_plans` / `reseller_pools`: `overselling` | G17. |
| `rights_set_items` | `permission:php_settings` and `permission:php_version` added to every listed set, so nobody loses what they could do before. |

### 1.1 Interface for other features (mail, web, apps)

A feature with its own limited setting:

1. Registers its key in Go, in an `init` function of its own file:

   ```go
   func init() {
       registerSettingKey(settingKeyDef{Key: "mail.outgoing_per_hour", Group: "mail", Unit: "count",
           Min: 1, Max: 100000, Presets: []int{100, 250, 500, 1000}})
   }
   ```

2. Inserts its server row in its own migration:
   `INSERT INTO setting_policy (key, server_ceiling, server_default) VALUES ('mail.outgoing_per_hour', NULL, 500);`
3. Reads the effective limit where it renders or checks:
   `subscriptionSettingLimits(ctx, db, subID)[key]` (`Ceiling`, `SetBy`, `SetByName`, `Default`,
   `Fixed`, `allows(v)`, `clamp(v)`), and refuses with `writeSettingAboveLimit`.
4. Adds the screen words `limits.key.<key>` (EN and TR).

Nothing else changes: plans, reseller plans and Custom sets carry every registered key in their
`ceilings` object, the plan form lists it, the plan preview reports it as lowered, and
`GET /api/v1/rights/setting-limits` returns it. The plan preview's "sites above the new limit"
reads PHP values only; a feature that wants its own count adds it to `previewLoweredCeilings`.

**Mail keys (alpha.128 integration).** `mail.mailbox_size_mb` and
`mail.outgoing.{mailbox,domain,subscription}` are registered in
`cmd/panel/setting_limits_mail.go`; their server rows (no ceiling, no default) are inserted by
the outgoing mail migration (067). The mail workstream keeps its own plan mail settings in
`mail_plan_limits` (the Mail server page); every mail value a server enforces or a screen shows
is the tighter of that row (or, without one, the Mail server page's value) and the chain's
effective ceiling (`foldMailChain`). Nothing is copied between the two stores. A change of
ceilings asks the outgoing mail worker to apply again.

## 2. PHP per site (G2)

- **Where the values live:** CelikPanel's text of the site's pool carries one block after the
  pool's own lines. Limits are `php_admin_value[...]`, so `ini_set()` and `.user.ini` cannot raise
  them; free choices are `php_value`/`php_flag`. `request_terminate_timeout` = run time + 10 s is
  the pool's own backstop. A site without values of its own gets no block: its pool stays
  byte-identical (test `TestSitePoolWithoutOwnValuesStaysByteIdentical`).
- **Upload** drives `upload_max_filesize`, `post_max_size` and nginx `client_max_body_size`.
- **nginx:** `celikpanel-managed.d/<domain>/site-settings.conf` holds `client_max_body_size` and
  `fastcgi_read_timeout` (run time + 10 s; nginx's 60 s default would answer 504 first). A
  directive another file of the site already sets (an application's include such as
  `app-nextcloud.conf`, the owner's `celikpanel-sites.d` file) is not written, because nginx refuses
  it twice; the screen names that file and its value applies. Owner edits are kept (D-031). If
  nginx already refuses the server's configuration before the change, nothing is written and the
  answer says the administrator must fix it first (unmet prerequisite).
- **Every render clamps:** the panel sends the effective ceilings with every pool operation
  (`SitePoolRequest.PHPLimits`), so a lowered plan ceiling takes effect at the site's next write;
  the process count is clamped too (the old 1–200 bound stays the fixed range). The administrator's
  own workspace has no ceiling. When a plan's (or reseller's) ceiling is below what the server's
  php.ini gives a site that chose nothing, the ceiling is written in a second block marked as the
  plan's; it is never read back as the site's own value, so a raised ceiling takes it away.
  A site whose own value is above a lowered ceiling keeps it in its own block and the ceiling is
  written after it in the plan's block (the later line wins in PHP-FPM); a process count above
  the limit is kept as a comment line of the own block while `pm.max_children` holds the limit.
  The screen reads the own values back and marks them clamped; a raised or removed ceiling of a
  plan writes its sites again and gives the own values back (alpha.128 lab: 384 MB had become
  256 MB for good). Server and reseller ceiling changes still reach a site at its next write.
- **A save** runs: check against the limits → write the pool (`php-fpm -t`, put back on refusal) →
  write the nginx file (`nginx -t`, put back on refusal). When nginx refuses, the pool is written
  back with its previous values, so PHP and nginx never disagree silently. Steps are reported to
  the progress window (`check`, `pool`, `web_server`).
- **Process manager details** (`pm`, spares, `max_requests`) are the administrator's: for anyone
  else the pool route takes only the process count, within the limit (closes the finding that a
  customer could ask 200 processes).
- **Permissions:** `php_settings` (change values), `php_version` (change version),
  `nginx_directives` (opt-in: closed for everyone, Unlimited included, until the administrator
  offers it with `PUT /api/v1/rights/offered-permissions`; the directives box itself is the web
  workstream's G3).
- **Applications' upload size** (owner decision 3): after a catalog application is installed, a
  site without its own upload size gets the application's size within its limit. The site
  setting then owns `client_max_body_size`: CelikPanel's unchanged text of the application's
  include is written again without its line (checked with nginx together with
  `site-settings.conf`, both put back on refusal), and an update of the application leaves the line
  out while the site setting owns it. An application's include edited by hand is kept and named on
  the screen. An application that wants more than the limit gets the limit, and the PHP screen
  names its wish, the limit and who can raise it.
- **New sites:** the pool is created within the plan's process limit (5 or the limit when lower).
- **A lowered limit that clamps a site** leaves one notice per person (customer, its reseller,
  administrators): what changed, that the site keeps running at the new limit, who acts and the
  next step; the site's PHP screen marks the clamped value (its own value stays stored), and a
  form sent back with that unchanged value is accepted (P5).
- **PHP version change with nginx stopped** (alpha.127 lab): a web server known to be stopped
  refuses the change before anything moves (`WEB_SERVER_STOPPED`, role-aware next step); a vhost
  that does not follow the new socket moves the pool and the record back
  (`PHP_VERSION_UNDONE`), so the site keeps answering on its previous version.

Endpoints:

| Route | Who | What |
|---|---|---|
| `GET /api/v1/domains/{id}/php/settings` | site's account, team (view) | fields, limits with `set_by`, effective values with source |
| `PUT /api/v1/domains/{id}/php/settings` | `php_settings`, team (manage) | save; guarded request identity, steps |
| `GET/PUT /api/v1/rights/setting-limits` | everyone reads their field; PUT administrator | the server level |
| `GET/PUT /api/v1/rights/offered-permissions` | administrator | opt-in permissions |

Agent RPCs (bound to the panel's build): `Agent.SitePHPSettingsV1` (read, many sites in one call),
`Agent.ApplySiteSettingsIncludeV1` (nginx file), and `Agent.ApplySitePool` operation `settings`.

## 3. Plans (G10, G13, G17)

- **Preview** of a plan edit adds the lowered ceilings and, for PHP keys, the sites whose own value
  is above a new ceiling ("N sites use more than a new limit; they keep running at the new
  limit"). If the sites' values cannot be read, the preview says so and the edit asks for
  confirmation. After saving, the plan's PHP sites are written again in the background so the
  lowered limit reaches them now; an owner-edited pool is kept (logged).
- **Overuse policy:** "Close" is D-044. "Notify only" closes and suspends nothing; a full disk
  still stops uploads (the server's disk). The notice level (50–95 %, default 80) replaces the fixed
  80 % for disk and traffic. The reseller pool keeps its own 80 %.
- **Clone:** the plan list's Copy opens the form filled in, named "<name> (copy)"; the rights set,
  ceilings and policy are copied, accounts are not.
- **Overselling (reseller plans and Custom resellers):** "Allowed" is the owner's K2 rule and the
  default. "Not allowed" refuses an allocation (plan assignment, Custom numbers) that would hand
  out more than the pool still holds, or an unlimited one under a limited pool
  (`POOL_OVERSELLING_NOT_ALLOWED`). What was handed out before is kept.
- **Custom sets:** the rights form of a Custom subscription edits its own ceilings (starting from
  its plan's when it leaves one), overuse policy and notice levels; the form of a Custom reseller
  edits its own ceilings and overselling choice (`GET /api/v1/rights/setting-limits?reseller_id=`,
  administrator). Only what was touched is sent.

## 4. Database connections and scheduled tasks (G5)

- **Scheduled tasks (done):** `cron.max_jobs` (tasks per site) and `cron.min_interval_minutes`
  (a floor; server default 5) are checked when a task is added or its schedule changes
  (`cron_limits.go`): the smallest gap between two runs within a day is read from the minute and
  hour fields (`@hourly`, `@daily`… too). A task whose schedule is kept is not held by a newer
  limit (P5), the crontab lines stay plain, and the administrator is not held by them.
- **Database connections (done):** `db.max_user_connections` is the engine's own limit on the
  user's role: MariaDB `ALTER USER 'u'@'localhost' WITH MAX_USER_CONNECTIONS n`, PostgreSQL
  `ALTER ROLE "u" CONNECTION LIMIT n` (on the cluster the server's driver reaches, as every
  statement). It is written when a user is created (a user the engine did not take the limit for
  is removed again and the creation is refused), and for every existing user of the subscriptions
  a changed limit reaches (a plan, a reseller plan, a Custom set, the server's policy) in the
  background through the same function; no limit removes an earlier one. The driver functions are
  separate (`internal/services/database_connection_limits.go`, `ConnectionLimiter`). It keeps
  working without the panel. A user whose server cannot be reached keeps its limit until the next
  change (logged).

## 5. Password level (G21)

`GET/PUT /api/v1/rights/password-level` (PUT administrator): `owner` (default), `strong` (10
characters and a digit), `very_strong` (14 and a digit). It can only raise the owner's rule and
applies when a password is set or changed. The generator always adds a digit. Refusals use
`PASSWORD_WEAK` with reason `needs_digit` or `too_short` (vars carry the level's minimum).

## 6. Open items (not done in alpha.128)

- Lab evidence per supported PHP version: `ini_get` after `ini_set`, after `.user.ini` and after
  `set_time_limit()` returns the admin value; `php-fpm -t` accepts each line.
- The browser-side password check still uses the owner's rule; the server refusal names the level.
- Limit hints from real use (G2, alpha.131).

## 7. Resilience contract

- Invariants: owner independence (values in native pool and nginx files, working without the
  panel), detect-don't-overwrite (D-031), P5 (lowering never breaks what runs).
- Schema: one additive migration (066); going back is the pre-update snapshot restore.
- Recovery: every write is validate-then-reload with put-back (`php-fpm -t`, `nginx -t`); an nginx
  refusal also writes the pool back; a PHP version switch whose vhost fails moves back.
- Evidence: Go tests in `internal/services/site_php_settings_test.go`,
  `cmd/panel/setting_limits_test.go`, `cmd/panel/php_version_web_server_test.go`,
  `internal/db/site_setting_limits_migration_test.go`; lab evidence listed in section 6 is open.
