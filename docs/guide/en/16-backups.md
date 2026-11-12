---
id: backups
title: Backups, remote copies and scheduled tasks
roles: [admin, reseller, customer]
topics: [domain.backups#backups, domain.advanced#cron-logs]
keywords: [backup, restore, remote, s3, sftp, backblaze, wasabi, gpg, passphrase, cron, scheduled task, logs]
---

A domain's backups are in its **Backups** section. **Scheduled tasks** and **Logs** are in its **Advanced** section, and the malware scan is in **Security** (see [Malware scan](guide:apps-wordpress#malware)).

## Taking a backup {#backups}

Under **Create a new backup**, choose:

- **Files backup**: the website's files only;
- **Database backup**: one database linked to the domain;
- **Full backup**: the files and every linked database.

Then press **Create backup**. **Existing backups** lists them with **Restore**, **Download** and **Delete**. They are kept in a protected area of this server and do not count against your disk limit.

**Automatic backups** takes a backup by itself: choose **How often** (**Daily** or **Weekly**), **What to back up** and **Copies to keep**, then **Turn on**. Older copies beyond that number are removed.

## Restoring {#restore}

Press **Restore** beside a backup and confirm. The current files and data are replaced. Before that, CelikPanel keeps a safety backup of the current state, which appears in the list; if the result is not what you wanted, restore that one. A restore that does not fit within your disk limit is refused before anything is replaced.

## Remote copies {#remote}

A backup on this server does not survive losing the server. A remote destination is storage you control elsewhere: S3-compatible storage (AWS, Backblaze B2, Wasabi, MinIO, Cloudflare R2) or an SFTP server.

**Adding a destination.** Customers and resellers do this under **My account**, **Remote backup destinations**, if their plan includes it; the administrator, under **Settings**, and can offer a destination to every account.

1. Press **Add a destination**, give it a **Name** and choose the **Type**.
2. Enter the storage's address and credentials.
3. Choose a **Passphrase** of at least 12 characters, or **Generate a strong passphrase**. Every copy is encrypted with it before it leaves the server.
4. Tick that you wrote it down, and save. The panel first tests the connection by writing, reading and deleting a small file. For SFTP, confirm the server's key fingerprint only if it matches the one on that server.

:::warning
Keep the passphrase somewhere safe, away from this server. Without it nobody can open these backups, and CelikPanel cannot give it back if the server is lost. It cannot be changed later; for a new one, add a new destination.
:::

**Using it.** On **Backups**, the **Remote copies** section shows the destinations this site can use:

- **Also send to…** beside a backup sends that backup. The local backup stays.
- **Add a schedule** sends a new backup daily or weekly and keeps the daily and weekly copies you set. It removes only its own older copies.
- **Show backups at the destination** lists what is stored there; **Restore** brings one back into this site.

Removing a destination stops sending to it; what is stored there stays.

## Opening a remote copy without the panel {#gpg}

A remote copy is a standard encrypted file (`.cpbak.gpg`), with a plain `.json` description beside it. On any computer with `gpg` and `tar`:

```
gpg --output site.cpbak --decrypt backup.cpbak.gpg
mkdir restore && tar -xzf site.cpbak -C restore
```

`gpg` asks for the passphrase. The site's files are then in `restore/payload/files.tar.gz`, and each database dump in `restore/payload/databases/`.

## Scheduled tasks and logs {#cron-logs}

**Scheduled tasks** runs a command at set times, as the site's own account. **Add task**, write the **Schedule** in the usual five fields (minute, hour, day of month, month, day of week) and the **Command**, and save.

**Logs** shows the site's **Access log**, **Error log** and **PHP error log**, with a filter and a download.
