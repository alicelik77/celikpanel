# v0.1.0-alpha.118

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.118.tr.md)

Real plan limits, an application catalogue, WordPress tools, malware scanning,
remote backups and the first AI and automation applications, as the owner
decided on 3 November 2026 (D-044). **Only the owner updates installed panels,
from the panel's own update screen.**

## Plan limits that work (D-044)

- **Traffic.** Each site's web traffic is counted per calendar month (data sent
  to visitors). At 100% of the subscription's limit, or of its reseller's pool,
  only the websites show "traffic limit reached"; mail, databases and file access
  keep working. They reopen on the 1st, or within a few minutes after the limit
  is raised. The count can run a few minutes behind; the screen says so.
- **Disk.** Real usage is measured: site files, mail and databases. When it is
  full, nothing new can be created and uploads stop (file manager, FTP, SFTP and
  PHP uploads); sites keep running. After 7 days over the limit the subscription
  is suspended; it reopens by itself when it is back under the limit.
- **Mailboxes.** A full mailbox refuses new mail (Dovecot's own quota), and the
  mailbox gets one warning message at 90%.
- Notices at 80% and 100% go to the customer, its reseller and the
  administrator.
- Traffic limits were stored before but never applied. They start as
  unlimited, and the administrator is told once, so no site closes because of an
  old number nobody saw.

## Application catalogue

- The Applications tab is a catalogue: PrestaShop, Nextcloud, Drupal, Joomla
  and MediaWiki join WordPress. Each is the official release, checked
  against a fingerprint pinned in CelikPanel before it is unpacked.
- The administrator chooses which applications are offered (Plans and rights →
  Offered). WordPress stays offered; the others start closed.
- Before installing, the screen says what is missing (PHP version, PHP packages,
  an empty site, disk space). The application sets itself up and its admin
  password is shown once. A failed setup can be retried; removal keeps the
  database unless you tick it.

## WordPress tools

- See the WordPress version, plugins and themes with available updates.
- Update the core and plugins with a backup taken first, which is put back if the
  update fails. An optional switch installs WordPress security and maintenance
  releases automatically.
- Copy a WordPress site to another empty site of the same subscription (staging).

## Malware scanning

- With ClamAV (installed from Components): scan one site or a whole
  subscription, or every site weekly. Infected files can be moved to quarantine
  and put back; nothing is deleted automatically. The customer, its reseller and
  the administrators are told. Old signatures give a warning, not a refusal.

## Remote backups

- Backups can also go to storage you control: S3-compatible storage (AWS,
  Backblaze B2, Wasabi, MinIO, Cloudflare R2) or an SFTP server.
- Every copy is encrypted with your passphrase and opens without CelikPanel
  using `gpg` (steps in docs/REMOTE-BACKUPS.md).
- Connection test, "Also send to…", daily and weekly schedules with retention,
  restore from the destination. Accounts can add their own destinations if their
  rights allow it.

## AI and automation applications

- n8n and Open WebUI run on a site as a closed service: their own account,
  memory and processor limits, reachable only through the site over HTTPS.
  Nothing opens until the administrator offers it after reading its risk
  summary, and the administrator can put an application on a security hold.
- n8n starts without the parts that run commands or read server files.
- n8n's licence limits selling it as a hosted service, so for now the
  administrator can offer n8n only to the administrator's own customers, not
  through resellers. The screen says why.
- Applications that run commands by themselves (OpenClaw, Hermes Agent,
  Paperclip) are listed but not offered yet; the catalogue says why.

## Fixed

- SSH command line: the tools it needs (zip, unzip, nano, vi, rsync…) are now
  installed with the panel. On existing servers SSH cannot be turned on while one
  is missing; the administrator installs them with one click on the domain's
  Access tab. Accounts that already have SSH keep it.

- If the server was busy when first-run setup began installing DNS, setup no
  longer stays on "result still unknown". It waits, says nothing was changed and
  continues by itself. If the server stays busy for more than 30 minutes, setup
  stops and asks you to review a revised plan.
- When Debian or Ubuntu's own PHP version is chosen in Components, it installs
  from the distribution's packages without the Sury repository, together with the
  PHP extensions the application catalogue needs.
- PostgreSQL can again be installed on its own, choosing its version (since
  alpha.101 only the PostgreSQL + phpPgAdmin bundle was visible in the first
  view). Removing a database offers to remove its admin tool too, and an admin
  tool left without its database says so and offers to reinstall the database or
  remove the tool.
- The security emergency section under MariaDB and PostgreSQL is a quiet tool
  unless an emergency is open, and clicking it no longer opens the settings page.
- After saving FTP/SFTP/SSH access, its warnings appear on the page instead of
  pop-ups over the page's buttons; what the page already says is not repeated,
  and missing command-line tools are named, with who installs them.
- A file-handle bug that could break large unpacks and other file reads running
  at the same time is fixed.

## New rights

WordPress tools, malware scan results, remote backup destinations and AI and
automation applications are new rights. Unlimited plans get them; accounts with
a custom rights list get them only when someone chooses them.

## Known limits

- Backups are not counted in disk usage; scheduled backups still run when a
  subscription is over its disk limit. Files in malware quarantine leave the
  disk count.
- SSH access cannot be made read-only when the disk is full.
- Traffic counts websites only (not mail or FTP).
- phpBB is not offered yet: its download server refuses the panel's downloads,
  and CelikPanel does not pretend to be another program to get past that.
- Matomo and Moodle are not offered yet. PrestaShop's friendly URLs are off;
  Nextcloud background jobs are not set up as scheduled tasks.
- WordPress theme updates are left to WordPress's own screen (wordpress.org
  publishes no checksum for themes).
- For n8n and Open WebUI only the main package is pinned; packages they fetch
  while installing are not each verified. Open WebUI is several GB and runs on
  Debian 12 and Ubuntu 24.04 only; n8n uses the panel's Node.js 24 runtime.
- On Arch Linux, missing PHP extensions are named but the panel cannot switch
  them on yet (the owner adds `extension=` in `/etc/php/conf.d`). A PHP version
  other than the distribution's own still needs the Sury repository.
- Servers installed earlier without logrotate show "logrotate is not installed"
  until the administrator installs it (`sudo apt install logrotate`, or
  `sudo pacman -Syu --needed logrotate`); traffic is still counted. On Arch, a
  failed first ClamAV signature download still fails the ClamAV install.
- Remote backups are full copies (no incremental backups); restore through the
  panel works into the same site.
- A rollback to alpha.117 restores the database copy taken before the update;
  anything done in the panel after the update is lost.

## Evidence

Integrated tree: Go build and vet clean; `internal` and `cmd/panel` suites pass;
the Agent suite shows only the known guest-only failures that also fail on
alpha.117; 2348 web tests pass within the bundle budgets; release-recovery
fixtures extended to schema 61.

Lab (QEMU guests, branch `lab/alpha114-acceptance`, commit `1316e2f05`, evidence
`final-alpha118-run1..run4-20261103`). Four short rounds; rounds 1–3 stopped at
defects that are fixed above. Round 4 on the final candidate passed every item:

- Debian 13 fresh install with the owner's command: setup, SSH tools present,
  PrestaShop (admin path found), Drupal install and removal, phpBB not offered,
  disk limit set and lifted (PHP uploads closed and reopened), n8n installed and
  running with owner login, stopped by suspension and started again, refused for
  resellers with the licence reason; PostgreSQL alone (distribution 17),
  phpPgAdmin offered with its removal and shown as left without its database.
- Ubuntu 24.04 updated from alpha.117 in 72 seconds: migrations 057–061, all
  services active; SSH refused while zip and unzip were missing, the
  administrator's one-click install added exactly those, then SSH was accepted.
- Earlier rounds on this code: traffic limit (site only closes, mail works),
  disk limit refusals, mailbox quota within one round on Dovecot 2.3 and 2.4,
  Nextcloud, Joomla, MediaWiki, WordPress (refused without mysqli, then
  installed), ClamAV (waits for signatures, EICAR quarantined and restored),
  SFTP remote backup with `gpg` decryption, setup DNS waiting while the server
  was busy. Not run: rollback, Open WebUI, Arch beyond install and setup.
