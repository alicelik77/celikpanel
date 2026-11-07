# Site log retention, website copying and Git deploy — design note (alpha.128)

Status (updated later the same night): **built in alpha.128 in a narrower,
safer form; the full designs below stay the target.** What shipped:

- **G27:** no log move and no second logrotate rule. The distribution's
  rule stays the only rotation; a subscription, plan or server limit makes
  the Agent remove only the site's *rotated* files older than the limit
  (`cmd/agent/site_logs.go`). Longer-than-system retention still needs the
  `LogDir` move below.
- **G8:** files plus an optional MariaDB database into a new database of
  the target, a full backup of a non-empty target first
  (`cmd/panel/site_copy.go`, `Agent.SiteCopyV1`). No configuration rewrite
  of known applications yet (the answer says the copy's config still names
  the source database); PostgreSQL databases are not offered.
- **G32:** HTTPS or SSH repositories, a deploy key made on the server (public
  half shown once), init/fetch/reset of one branch and an optional Node.js
  build in the site sandbox (`internal/nodesite/gitdeploy.go`). The key is
  handed over as a systemd credential (LoadCredential=, systemd 247+;
  systemd 249 accepts it for transient units, so Ubuntu 22.04 works like
  24.04); no releases/rollback folder and no webhook yet.

Open for all three: the lab round on the five systems (logrotate forms,
backup + copy of a large site, LoadCredential inside the sandbox).

Original note: PLESK-GAP G27, G8 and G32 were in the alpha.128 "web"
workstream after G3, G19, G26 and G28. Owner independence (D-022),
detect-don't-overwrite (D-031) and actionable guidance (D-024) apply to all
three.

## 1. Log retention per plan (G27)

**Fact that decides the design.** Site logs are
`/var/log/nginx/<domain>-{access,error}.log`. Every supported distribution
rotates `/var/log/nginx/*.log` with its own logrotate rule. A second rule for a
file that glob already matches is a logrotate error ("duplicate log entry")
that stops rotation *for the whole server*. So per-site retention cannot be a
second rule over today's paths.

**Design.**
1. A template variable `LogDir` (default `/var/log/nginx`, file names
   unchanged). A site gets `/var/log/celikpanel/sites/<domain>/` only when
   its plan (or the administrator) sets a retention; every other vhost stays
   byte-identical (same frozen-template test as G3).
2. The directory is root:adm 0750 (Debian/Ubuntu) or root:log (Arch), created
   by the Agent before the vhost that names it (nginx -t opens every log).
3. One CelikPanel logrotate file `/etc/logrotate.d/celikpanel-sites`
   (render header, owner edits kept) with one stanza per moved site:
   `daily`, `rotate <N>`, `compress`, `delaycompress`, `missingok`,
   `notifempty`, `sharedscripts`, and the distribution's own nginx
   `postrotate` (USR1 to the master), copied from the package rule at render
   time, never invented.
4. Readers follow the path: the Logs viewer (`domain_logs_handlers.go`),
   fail2ban's nginx jails (they read `/var/log/nginx/*error.log`: add the new
   glob through a CelikPanel jail drop-in, never by editing the jail), the
   malware scanner's exclusion list. The traffic log is unaffected.
5. Plan key `logs.keep_days` (1–365, empty = the system's rule) with the
   ceiling/notify model of SITE-SETTINGS-DESIGN §2.2; the 130 read-only view
   ("kept 14 days, rotated daily, by the system") stays for unmoved sites.
6. Moving back (retention cleared) renders the old path and leaves the moved
   files where they are, with a note naming them.

**Evidence needed:** logrotate `-d` on the five systems with the rule set;
nginx -t with the new directory; fail2ban still bans from a moved log;
rotation still runs server-wide after an owner edits the CelikPanel file.

## 2. Website copying (G8)

"Copy this site to <another domain>": files, database, configuration of known
applications, within subscriptions the caller may manage.

**Design.**
- **Who and where:** the caller must own both domains (same chokepoint as
  `authorizeDomain`); a reseller within their tree; the target subscription's
  plan must offer the source's project type and PHP version (rights gate),
  and the target's disk must hold the copy (D-044 usage).
- **Target checked empty first:** document root holds nothing but the
  default index page; otherwise the screen says what is there and offers
  "copy into an empty folder" only (never a silent overwrite).
- **Steps (one recorded operation, D-024, request identity, progress
  window):** check → copy files → copy database → rewrite configuration →
  web server → done. Polling never starts a second copy.
- **Files:** the Agent copies as root with `cp -a --reflink=auto` into a
  staging folder beside the target, then `chown -R` to the target site's
  account and one rename. No link is followed out of the source home;
  `.git`, `node_modules/.cache` and the home's `tmp/` are skipped.
- **Database:** depends on G7 "copy to a new database" (database
  workstream, migration 070): a new database and user in the target
  subscription, counted against its plan; the source is never locked.
- **Configuration rewrite:** only for catalogue applications the Panel
  installed (WordPress `siteurl`/`home` via the WordPress tools' existing
  search-replace, PrestaShop shop URL, Nextcloud `trusted_domains`); other
  sites get the files and a plain note that their own config names the old
  address.
- **Failure:** a failed step removes the staging folder and the new
  database; the source is read-only throughout.

## 3. Git deploy (G32)

**Design.**
- A per-site deploy key (ed25519) made by the Agent, private half root-owned
  in the runtime folder (0600), public half shown **once** with "copy";
  a new key replaces it (the old one is shown as revoked).
- Repository URL (SSH or HTTPS read-only), branch; no credentials stored
  beyond the deploy key.
- Deploy = a transient unit in the **site sandbox** (alpha.127 profile, same
  as the Node.js install step): `git fetch` into `<home>/repo` as the site's
  account with `GIT_SSH_COMMAND` naming the key file through a root-owned
  `ssh_config` bind (the account never reads the key file itself), then the
  chosen build (package script from G19, or none), then a rename-switch of
  the document root content (`releases/<sha>` + symlink-free copy), keeping
  the previous release for one-click rollback.
- Live steps and output exactly like the G19 script runner; webhook trigger
  later (needs a signed, rate-limited public endpoint: separate decision).

**Evidence needed:** the sandbox denies the key file to the site account;
a failing build leaves the live site untouched; rollback restores the
previous release byte for byte.
