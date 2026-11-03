---
id: backups
title: Backups on the server and at a remote destination
roles: [admin]
topics: []
keywords: [backup, restore, remote, s3, sftp, passphrase, gpg, schedule, retention, destination]
---

Backups are taken per site. A copy on the server's own disk does not survive losing the server, so send copies elsewhere too.

## On the server {#local}

Each domain has **Backups** on its **Advanced** tab: files, databases or both, by hand or on a schedule. A restore first takes a safety backup. There is no whole-server backup in the panel; cover every site with an every-site schedule to a remote destination, and use your provider's snapshots for the system itself.

## Remote destinations {#destination}

In **Settings**, tab **Remote backups**, press **Add a destination**: **S3-compatible storage** over HTTPS, or an **SFTP server** after you confirm its fingerprint. Choose a passphrase or **Generate a strong passphrase**. **Test and save** writes, reads and deletes a test file before saving. **Offer to accounts** lets resellers and customers use it.

:::warning
Write the passphrase down and keep it away from this server. Without it nobody can open the backups; it cannot be changed and is never shown again.
:::

## Schedules and restore {#schedules}

**Every-site schedules** send every site daily or weekly and keep the copies you set. On a site's **Backups**, **Add a schedule** and **Also send to…** work for that site alone, and **Backups at the destination** lists its copies with **Restore**. A restore goes into the same site, after a safety backup.

A schedule removes only its own older copies. **Remove from CelikPanel** deletes nothing at the destination. A failed send is retried up to three times; a result that is not known is read again, never sent twice.

## Without CelikPanel {#without-panel}

A remote copy is a standard OpenPGP file. Any computer opens it with `gpg`, your passphrase and `tar`:

```
gpg --output site.cpbak --decrypt example.cpbak.gpg
mkdir restore && tar -xzf site.cpbak -C restore
```

Inside are the site's files and one dump per database.
