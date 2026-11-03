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

- The Applications tab is a catalogue: PrestaShop, Nextcloud, Drupal, Joomla,
  MediaWiki and phpBB join WordPress. Each is the official release, checked
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

- If the server was busy when first-run setup began installing DNS, setup no
  longer stays on "result still unknown". It waits, says nothing was changed and
  continues by itself. If the server stays busy for more than 30 minutes, setup
  stops and asks you to review a revised plan.
- When Debian or Ubuntu's own PHP version is chosen in Components, it installs
  from the distribution's packages without the Sury repository, together with the
  PHP extensions the application catalogue needs.
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
fixtures extended to schema 61. <!-- LAB --> Lab round recorded in the release
record.
