# Site backup contents: programs, Python apps and service apps

*Design note, 12 November 2026, for the release after alpha.133. Status: implemented in
source as described below; not yet run on a real system (the lab steps are at the end).*

## The defect

A site backup ("Dosya yedeği" and "Tam yedek") archived only `<site home>/public_html`
(`internal/hostingpath/path.go` `DocumentRoot`, `cmd/agent/backup_create.go`
`buildFilesPayload`). A full backup added the domain's databases. Everything else a site
keeps in its home was outside every backup, and the backup screen did not say so:

| Site kind | Where its files and data live | Covered before this change |
|---|---|---|
| Hosting (PHP, static, WordPress, Node.js) | `public_html` (Node.js keeps its app and `node_modules` there too) | yes |
| Program (`runtime_app`, kind `program`) | `app/bin`, `app/config`, `app/data` (`internal/siteruntime/siteruntime.go` `Layout`, docs/RUNTIME-APPS.md §2) | **no** |
| Python app (`runtime_app`, kind `python`) | `app/src`, `app/data`, `app/config`, `app/venv-<n>` (docs/RUNTIME-APPS.md §18.3) | **no** |
| Service apps (n8n, Open WebUI) | `apps/<app>/data`, `apps/<app>/config`, `apps/<app>/dist`, `apps/<app>/app-<version>` (`internal/serviceapps/render.go` `Layout`) | **no** |
| Environment of a program | `/etc/celikpanel-runtime/celikrt-<id>.service.env`, root 0600; public values also in the Panel's `runtime_apps.env_public`, secret values only in the file | no (and stays out, see below) |
| Environment of a service app | `/etc/celikpanel-svcapps/celiksvc-<id>.env`, root 0600; secrets only in the file | no (stays out) |

After a disaster the Panel would have restored an empty web folder and, at best, the
database; the owner's program, its configuration and its data were gone.

## What a site backup contains now

A files or full backup holds `public_html` exactly as before, and in addition the
**site-home areas** that exist on the site:

- `app` — the program or Python app folder, whole, except the build leftovers below.
- `apps/<app>` — each service app's folder, except the parts the Panel installs again.

The areas are found by the Agent when the backup is made (a folder called `app`, and the
folders under `apps`, directly in the site home, opened without following links). They
are not taken from the Panel's records: an `app` folder whose program record was removed
is still the owner's data ("the app folder stays until someone deletes it",
docs/RUNTIME-APPS.md §13), and scheduled backups, the safety backup made before a restore
and the backup made before a WordPress update all get the same content without the Panel
naming anything.

### Deliberately left out, and why

Each left-out item is named in the backup's manifest with its reason and size, and the
backup screen shows it. Nothing is skipped without being listed.

| Left out | Reason code | Why |
|---|---|---|
| `app/.pip-cache` | `cache` | pip's download cache; a deploy fills it again. |
| `app/.build-tmp`, `app/venv-<n>.partial` | `build_temp` | A build in progress or its temporary folder; never the running app. |
| `apps/<app>/.cache` | `cache` | npm/pip cache of the install. |
| `apps/<app>/dist`, `apps/<app>/app-<version>` | `reinstalled` | The verified upstream file and the installed application; the Panel installs them again from the catalogue. The app's own data (`data`, `config`) is kept. |
| A socket, FIFO or device file anywhere in an area | `special` | It has no content to archive; the program makes it again when it starts. |
| A file or folder in an area that does not belong to the site's account | `not_site_owned` | Root reads the site as root; a file of another account (for example one the server owner put there as root) must not reach a downloadable archive. The owner sees its path and decides. |
| Files directly in the site home outside `public_html`, `app` and `apps` | — | As before. Named in this note as an open item, not hidden: the screen says what a backup holds, by area. |
| The environment files | — | See "Secrets" below. |

Nothing else is excluded: no log rule (site logs are under `/var/log`, not in the site
home), no size rule. A large data folder is backed up, not cut.

### Links and hard links in an area

- A symbolic link is stored **as a link** (its target text), never followed. A Python
  environment is made of links (`venv-<n>/bin/python` points at the distribution's
  Python), and an owner may use links for his own versions. Restore makes the same link
  again without following it, with the site's account as its owner. A link can name a
  place outside the site; that gives the site's account nothing it could not make itself,
  and no root step follows it (every open is `O_NOFOLLOW`/`RESOLVE_NO_SYMLINKS`).
- A file with several hard links is stored as an ordinary file when it belongs to the
  site's account (its own data under another name); it comes back as separate files.
  `public_html` keeps its old rule (links and hard links stop the backup with a named
  error) so that its archive stays byte-compatible with older releases.

### Size, before the backup starts

`GET /api/v1/domains/{id}/backups/contents` asks the Agent to measure what a backup of
this site would hold **with the same walk and the same rules that make the archive**, so
the numbers shown are the numbers archived (uncompressed bytes of the files kept). It
answers per area, the left-out items with their sizes, and the free space on the
server's backup disk. The screen shows "Bu yedek şunları içerir: Site dosyaları (12 MB),
program dosyaları ve verisi (1,1 GB), veritabanı: …". When the files to archive are
1 GiB or more, creating a files or full backup asks first in the panel's own dialog,
naming the size; when they are more than the free space on the backup disk the screen
says so before anything starts. The measurement stops after 45 seconds and then says
the size is "at least" what was counted.

The plan's disk allowance: backup archives live in `/var/backups/celikpanel`, outside
the subscription's measured usage (site files, mail, databases; `cmd/panel/quota.go`).
They do not count against the plan today, and this change does not start counting
them: a storage allowance for backups is a plan decision outside this lane (open).
The existing rule stays: a subscription at its disk limit takes no new manual backup.

## Restore

`public_html` and the databases restore exactly as before. An area restores like
`public_html`: the archive is unpacked into a staging folder beside the area, owned by
root and 0700 while it is written; then every file, folder and link gets the site's
account as owner (the owner of the site home) and the mode stored in the archive (the
area folder keeps its current mode, or the archived one when it did not exist), the
setgid bit on folders where the area had it; then the staging folder replaces the area
in one rename and the old one is removed after the rename is proven. Every open is made
relative to a held descriptor without following a link; a name with `..`, an absolute
name, a duplicate, a hard-link entry or a device entry stops the restore before
anything is replaced. Several replacements (site files, `app`, databases) are published
in order; when one fails, the ones already published are put back and the databases are
loaded from the safety backup, as a full restore did.

**Programs and Python apps.** The Panel orchestrates around the Agent's restore, with
real steps in the progress window:

1. *check* — a program operation (deploy, removal) still running refuses the restore
   before anything changes (`PROGRAM_OPERATION_RUNNING`, "wait for it to end").
2. *program_stop* — only when the backup holds `app` and the program runs: the existing
   stop of the Programs tab (`RuntimeAppControlV1` stop). A running program would keep
   the old folder open through its sandbox bind mount.
3. *restore* — the Agent's restore, as above.
4. *program_start* — only when the Panel stopped it: the existing start
   (`RuntimeAppControlV1` start, with the same hold, suspension and sandbox-level gates
   as the Programs tab), then the status is read until the program listens on its port
   or 2 minutes pass.

The answer says which happened, in the person's language: started and listening;
started but did not listen / stopped again (with the Programs tab's reason, for example
`restart_loop`); left stopped because a gate refused the start (hold, suspension); left
stopped because the restore could not put the old files back (`rollback_failed`: the
files are not in a known state, the safety backup is kept); or not started because it
was not running before. A program whose record no longer exists gets its files back and
the answer says to set it up again on the Programs tab. A restore that fails after the
Panel stopped the program starts it again on the old files.

A Python app's environment comes back with its links; on the same server and the same
Python it runs as it ran. On a server with another Python the Programs tab already says
`python_moved`, and a deploy builds a new environment.

**Service apps.** Their `data` and `config` are in the backup, but this release does not
put them back: a restore replaces `public_html`, `app` and the databases and leaves every
`apps/<app>` folder as it is. The answer and the backup row name those apps ("not
restored by the panel in this release"). Doing it safely needs the service app's own
stop/start orchestration and a decision about restoring data onto a different installed
version; until then the archive is a standard `tar.gz`: the owner stops the app, takes
`apps/<app>/data` from `payload/home.tar.gz` and starts it again.

**Secrets.** No environment file is written into an archive. Archives are downloadable by
the site's owner and copied to remote storage; a secret value is write-only in the Panel
today ("never shown again", docs/RUNTIME-APPS.md §5, docs/AI-APPS.md §5). Consequence,
said on the screen: a restore on the same server keeps the server's current environment
and unit, so the program starts as before; after the server itself is lost, the public
values come back with the Panel's record (a deploy writes them), and secret values, the
service apps' keys and n8n's encryption key must be entered again.

## Compatibility and format

- **Manifest version 3** is written only when an area is present. A site without `app`
  and `apps` gets the same version 2 manifest and payloads as before, byte for byte, so
  every release can still read it. Version 3 adds `home` (the payload
  `payload/home.tar.gz`), `areas` (name, bytes, files) and `left_out` (path, reason,
  bytes; at most 100 named, then a count). The RPC protocol version stays 2; the new
  `Info` and `RestoreResponse` fields are additive.
- **Older archives** (version 2 and pre-v2) restore exactly as today: they hold no area,
  so no area is touched and no program is stopped. The screen labels a files or full
  backup without the program, on a site that has one, "Bu yedek programı içermiyor"
  (and "eski biçim" when it was made before this change).
- **Newer archives on an older release (rollback): fail closed.** An older Agent reads
  the version, refuses the manifest, so it does not list the archive, refuses to restore
  or import it before anything is written, and leaves the file in place. Restoring the
  old subset is not possible: an older Agent refuses any payload its manifest reader
  does not know, and a version-2 label on an archive that holds more would make the old
  release show a program backup as a plain site backup. The archives reappear when the
  owner updates again; an older release does not prune them meanwhile.
- **Remote copies** (S3/SFTP) are the same `.cpbak` bytes, so they carry the same
  content; bringing one back verifies the version-3 manifest and every payload's hash.

## Invariants, evidence, open items

Constitution 2 (continuity and recoverability) and 3 (truthful state; one versioned
producer/reader/restore contract per durable artifact), 6 (backups in standard formats);
D-022 (the native unit and environment are not touched by a restore; a program the
owner stopped stays stopped); D-024 (every skip, left-out item and partial result is
named with who acts next); D-025 (versioned artifact contract, fail-closed reader).

Evidence: component tests in `cmd/agent` (area detection, rules, links, ownership,
special files, size equals archive, manifest v2/v3 both ways, restore of old and new
archives on a temporary tree with owner and modes, no escape) and `cmd/panel` (contents
endpoint, restore orchestration states with a fake Agent), and mounted web tests. No
real system has run it.

Open: the lab steps below; service-app restore; files in the site home outside the three
areas; a plan allowance for backup storage; the narrow window in which a program
operation can start after the restore's check (the runtime operation lock is not shared
with the restore); a Panel stopped between *program_stop* and *program_start* leaves the
program stopped and disabled (the request's outcome is "not known" and the owner starts
it on the Programs tab); Node.js sites whose `node_modules` holds links still cannot be
backed up (the `public_html` rule above), unchanged by this note.

## Lab steps (real system)

1. Program site: upload a small Go server to `app/bin/server` that serves the content of
   `app/data/note.txt`, put a file in `app/config`, deploy, open the site: it shows the
   note. Open Yedekler: the contents line names site files, program files and data with
   sizes; note the program size.
2. Create "Dosya yedeği". The row shows "Program dosyaları ve verisi (<size>)"; the size
   equals step 1's. Create "Tam yedek" with one database: the row also names the database.
3. Download the backup; `tar -xzf` it and `tar -tzf payload/home.tar.gz`: `app/bin/server`,
   `app/data/note.txt`, `app/config/...` are there; no environment file.
4. On the server delete `app/data/note.txt` and `app/bin/server` (as the site's account).
   The site fails. Restore the files backup: the progress window shows stopping the
   program, restoring, starting the program; the result says it started and listens;
   the site shows the note again. `stat` shows the site's account as owner and the
   original modes.
5. Stop the program on the Programs tab, restore again: it stays stopped and the result
   says so.
6. Restore an archive made by alpha.132 or 133 on the same site: it restores
   `public_html` only, the row says "Bu yedek programı içermiyor (eski biçim)", the
   program is not stopped.
7. Full backup restore: the database comes back (change a row first).
8. Python app: deploy a Flask app, back up, delete `app/src`, restore: it starts without
   a rebuild. Check `venv-<n>/bin/python` is a link again.
9. A site with n8n: the contents line names n8n's data; a restore says n8n's data was
   not restored by the panel.
10. Roll the panel back is not in scope for the lab without the owner's update path; the
    fail-closed reading of version 3 by an older release is covered by component tests.
