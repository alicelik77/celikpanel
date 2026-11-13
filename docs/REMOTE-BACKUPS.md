# Remote backups

*alpha.118 · owner request "uzak yedek" · [Türkçe](REMOTE-BACKUPS.tr.md)*

A backup on the server's own disk does not survive losing the server. A **destination** is
storage you control somewhere else. CelikPanel can send backups there, encrypted with a
passphrase that only you keep, and bring them back.

**What is built.** It supports two kinds of destination:

- **S3-compatible object storage.** This covers AWS S3, Backblaze B2 (S3 API), Wasabi, MinIO and
  Cloudflare R2.
- **SFTP servers.**

On top of these you get the following:

- A connection test that writes, reads and deletes a small object before a destination is saved.
- Sending a backup by hand ("Also send to…").
- Schedules per site, plus every-site schedules for the administrator, with a simple daily and
  weekly retention.
- A list of the backups held at the destination.
- Restore through the existing restore path.

Everything is native Go inside the panel. CelikPanel never runs rclone, restic or any other
external tool.

**What is not built.**

- **Incremental backups.** Every remote copy is a full package.
- **Resuming an interrupted upload part-way after a panel restart.** The operation runs again
  under the same id and sends the package again.
- **Restoring onto a different server or into another site through the panel.** For that, use
  the steps in [Restore without CelikPanel](#restore-without-celikpanel).

## Owner independence

- **Your local backups are not changed.** They and their schedules work exactly as before.
  Remote copies are an addition.
- **Scheduled sending is panel automation.** If the panel is removed, nothing more is sent. What
  was already sent stays at your destination and opens without CelikPanel: it is an OpenPGP file
  and the distribution's own `gpg` opens it.
- **CelikPanel never deletes a remote copy except under the retention you set.** The retention of
  a schedule removes only that schedule's own older copies. It never touches copies you sent by
  hand or copies made by another schedule. Removing a destination from the panel leaves
  everything stored there.

## Who can use it

| Who | What they can do |
|---|---|
| Administrator | Adds **server-wide** destinations. Uses them for any site. May **offer** each one to accounts. Sets every-site schedules. |
| Reseller, customer | Adds **own** destinations only with the permission **Own remote backup destinations** (`remote_backup_destinations`, D-042). Can always choose a destination the administrator offers. |
| Additional users (team) | Do not manage destinations. |

- **The server enforces every rule.** The screens only hide what is not allowed.
- **Removing a right does not stop running schedules (D-042 P5).** Only new choices are refused.
- **After a subscription transfer, the previous owner's destination stops holding the site's
  backups.** The next run stops with "destination not available", so the new owner's data never
  goes to someone else's storage.

## Encryption and the passphrase

Every object is encrypted on this server before it is sent. The format is OpenPGP symmetric
encryption (RFC 4880):

- AES-256 cipher.
- Iterated and salted SHA-256 key derivation.
- An integrity-protected data packet (MDC).

This is exactly what `gpg --symmetric --cipher-algo AES256` writes. `gpg` is in the official
repositories of every supported system: package `gnupg` in Debian 12/13, Ubuntu 22.04/24.04 main
and Arch core. It is normally already installed.

- **You choose the passphrase when you add the destination.** It must be at least 12 characters.
  The screen can also generate one. **Write it down and keep it away from this server.** Without
  it nobody can open these backups, and CelikPanel cannot recover it if the server is lost.
- **The panel stores the passphrase sealed** (AES-256-GCM under the panel's secret key, like
  other panel credentials), so it can encrypt scheduled backups and decrypt during a restore. It
  is never shown again, never written to logs and never sent to a screen.
- **The passphrase of a destination cannot be changed,** because the backups already there need
  it. For a new passphrase, add a new destination.
- **Each destination shows a short check tag,** for example `3F2A-91C0`. The same tag is written
  into every backup's side file. It tells you which passphrase opens a file without revealing
  the passphrase.

The storage credentials are sealed the same way: access key secret, SFTP password or private key.
The screens show only the start and end of an access key ID.

## SFTP: the server's key

**The first connection needs a person.** The panel shows the SFTP server's key fingerprint, for
example `SHA256:…`. Compare it with the server's own fingerprint before you confirm. On the SFTP
server, run:

```sh
ssh-keygen -lf /etc/ssh/ssh_host_ed25519_key.pub
```

Confirm only if the fingerprints match. CelikPanel then pins that key.

**If the server later shows a different key, nothing is sent and the operation stops.** It stops
with "The SFTP server shows a different key". Ask the server's owner whether the key really
changed. Confirm the new fingerprint in the destination only if it did.

## Where things are stored

Paths are relative to the bucket folder (prefix) or the SFTP folder:

```
<server>/sites/<domain>-d<domain id>/manual/<backup>.cpbak.gpg
<server>/sites/<domain>-d<domain id>/manual/<backup>.cpbak.gpg.json
<server>/sites/<domain>-d<domain id>/schedule-<n>/<backup>.cpbak.gpg
```

- `<server>` is this server's name, fixed the first time a remote backup is made. Several
  servers can share one bucket.
- The **`.json` side file** is plain text. It holds:
  - the site, the backup type and the creation time;
  - the sizes and SHA-256 of the encrypted file and of the decrypted file;
  - the passphrase check tag;
  - the decryption command.

  It holds no secret.

## Checks

- **Upload to S3.** Each request carries the SHA-256 and MD5 of its data, and the storage itself
  refuses damaged data. Large backups go up in parts of 16 MiB or more. A failed part is sent
  again from memory. A failed upload is cancelled, so no partial object is left. After the
  upload, the panel reads back the size and compares the storage's checksum (ETag) with the MD5
  of what was sent, where the storage reports one.
- **Upload to SFTP.** The file is written under a temporary hidden name and read back in full. It
  is compared by size and SHA-256, and only then renamed into place.
- **Restore.** The whole object is downloaded and checked:
  - against the SHA-256 in the side file;
  - by the integrity check inside the OpenPGP file;
  - against the decrypted SHA-256.

  The Agent then checks the package again before anything is restored: its size and SHA-256, that
  it belongs to this site and subscription, and every payload's SHA-256. The restore then uses
  the same path as a local restore: a safety backup of the current state is kept first, and the
  disk quota applies.

## Operations and what they tell you

- **One operation at a time.** Every send and every restore is an operation with a lasting id
  (`rb-…`), and the panel runs them one after another.
- **Watching never starts anything.** A page that follows an operation only reads it, so it can
  never start a second upload. The same request sent twice gets the same operation.
- **A restart continues the same operation.** If the panel restarts during an operation, the same
  operation is checked and continued.
- **Each operation shows the reason, who acts, the next action and how work resumes (D-024).**
  Every failure falls into one of these classes:

  - **Unmet prerequisite.** Something must be corrected first, for example:
    - credentials refused;
    - no permission in the bucket;
    - bucket or folder missing;
    - host key changed;
    - storage full;
    - server clock wrong;
    - disk quota reached;
    - destination removed or not available for this site.
  - **Verified failure.** For example:
    - the integrity check failed;
    - the passphrase does not open the file;
    - the package was refused by this server.
  - **Result unknown.** The storage could not be reached, or the panel restarted. CelikPanel tries
    again by itself, at most 3 attempts with growing waits. The last failure is kept and shown
    while it retries.

## Schedules and retention

- **What a schedule sets:** daily or weekly, files or full backup, and how many copies to keep.
  - **Keep N daily:** the newest copy of each of the newest N days.
  - **Keep M weekly:** the newest copy of each of the newest M calendar weeks.
- **What is never removed:** the newest copy is always kept, and a copy whose time cannot be read
  is never removed.
- **Separate local copy.** A schedule makes its own local backup for each run, sends it, checks
  it, then removes that local copy. Your local backups and their own schedule are not touched.
- **Every-site schedules (administrator):** these run the same way for every site on the server.

## Restore without CelikPanel

You need the `.cpbak.gpg` file, its `.json` side file (optional, for checking), the passphrase,
and `gpg` and `tar`. These come from the distribution's official packages (`gnupg`, `tar`).

1. **Download** both files from the bucket or SFTP folder with any tool you like: the provider's
   web console, `sftp`, `scp`, or the AWS CLI if you already use it.
2. **Optional: check the download.** Compare the output with `encrypted_sha256` in the `.json`
   file:

   ```sh
   sha256sum full-20261103T020000.000000000Z-0123456789abcdef.cpbak.gpg
   ```

3. **Decrypt.** Put the passphrase in a file only you can read, then:

   ```sh
   install -m 600 /dev/null pass.txt && nano pass.txt
   gpg --batch --pinentry-mode loopback --passphrase-file pass.txt \
       --output site.cpbak --decrypt full-20261103T020000.000000000Z-0123456789abcdef.cpbak.gpg
   shred -u pass.txt
   ```

   A wrong passphrase, or a damaged or altered file, makes `gpg` stop with an error. Optionally
   compare `sha256sum site.cpbak` with `plain_sha256`.
4. **Unpack the package:**

   ```sh
   mkdir restore && tar -xzf site.cpbak -C restore
   ```

   You get:
   - `manifest.json`: what the package holds, with the SHA-256 of each part.
   - `payload/files.tar.gz`: the site's files (files and full backups).
   - `payload/databases/<id>.sql.gz`: one compressed dump per database (database and full
     backups).
   - `payload/home.tar.gz` (only when `manifest.json` says `"version": 3`): the site home's
     program or Python app folder `app` and each service app's folder `apps/<app>`, as
     `manifest.json` lists them under `areas` (files and full backups).
5. **Put the files back** into the site's document root (example path):

   ```sh
   tar -xzf restore/payload/files.tar.gz -C /var/www/example.com/public_html
   ```

   **Program and app folders** (version 3 only): stop the program or app first and move its
   current folder aside, then unpack into the site home (the folder that holds
   `public_html`) and give the files to the site home's owner, because the archive stores no
   owners (do the same for each `apps/<app>` folder). Example path:

   ```sh
   site=/var/www/celikpanel/subscriptions/3/sites/7
   tar -xzf restore/payload/home.tar.gz -C "$site"
   chown -hR --reference="$site" "$site/app"
   ```

   Environment files (secret values) are never in a backup: enter them again where the
   program reads them.

6. **Load each database.**
   - MariaDB or MySQL:

     ```sh
     gunzip -c restore/payload/databases/12.sql.gz | mariadb dbname
     ```

   - PostgreSQL:

     ```sh
     gunzip -c restore/payload/databases/12.sql.gz | psql dbname
     ```

   The database name for each id is in `manifest.json`.

These steps work on any server with `gpg` and `tar`, with or without CelikPanel.

## Settings reference

- **S3:**
  - endpoint, for example:
    - `https://s3.eu-central-1.amazonaws.com`
    - `https://s3.us-west-004.backblazeb2.com`
    - `https://s3.eu-central-1.wasabisys.com`
    - `https://<account>.r2.cloudflarestorage.com`
    - your MinIO address
  - region: `us-east-1` when empty, `auto` for R2.
  - bucket.
  - folder (prefix).
  - access key ID and secret.
  - path-style addressing by default, or virtual-hosted.

  Only `https://` is accepted. The key needs permission to put, get, list and delete objects in
  the bucket, and to create, complete and abort multipart uploads. A lifecycle rule at the
  provider that removes unfinished multipart uploads after a few days is a good extra safety net.
- **SFTP:**
  - host and port.
  - user.
  - password or private key (unencrypted PEM).
  - an existing absolute folder.

  The panel creates sub-folders inside it.

## Open points

The following are not built:

- incremental or deduplicated backups;
- resuming an upload part-way after a restart;
- restoring through the panel onto a rebuilt server or into a different site;
- an administrator's choice of which accounts see an offered destination (today: all accounts);
- a reseller offering its own destination to its customers;
- plain `http://` endpoints and private certificate authorities for MinIO on a LAN.
