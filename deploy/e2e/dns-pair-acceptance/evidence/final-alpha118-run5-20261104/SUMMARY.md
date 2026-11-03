# alpha.118 lab acceptance, run 5 (2026-11-03 21:45 to 22:12 UTC; folder dated 2026-11-04)

Candidate: `fix/a118-svcapps-rootfiles` at `31acebb77`, release identity 118. It was built from
the remote ref through a bundle; no branch was created in the shared repository.

Rules followed: lab only, panel API like a user, SSH read-only. Two exceptions:
- the step 2 attack setup, where the lab acts as a malicious customer;
- one `systemctl start` of the quota-warning service, as lab load.

| Item | Result | Evidence |
|---|---|---|
| 1 n8n env file is root's | PASS | `raw/R5svc-a118.log`, `raw/run-R5n8n.txt` |
| 2 malicious link swap | PASS | `raw/R5svc-a118.log` |
| 3 backup/restore + WP staging | **FAIL: setgid bit lost** | `raw/R5bk-a118.log`, `raw/R5mode-a118.log` |
| 4 MediaWiki | PASS | `raw/R5bk-a118.log` |
| 5 mailbox 90% warning | PASS | `raw/R5bk-a118.log` |
| 6 B update 117 to candidate | PASS | `raw/run-Bupd2.txt`, `raw/Bupd-u117.log` |
| 7 B n8n env path | PASS | `raw/R5n8nB-u117.log` |

## Item notes

1. **n8n env file.**
   - `EnvironmentFile=/etc/celikpanel-svcapps/celiksvc-1.env`.
   - The file is `root:root 600`; the folder is `root:root 700`.
   - n8n runs: HTTPS answers 200 and the owner login answers 200.

2. **Malicious link swap.** Using its own account, the site's user linked
   `apps/n8n/config` to `/etc/celikpanel`. It also linked `config/app.env` to a root-only
   canary file. Then, through the panel, the app's keys were changed (owner password → 200) and
   the app was restarted (`state running`).
   - `/proc/<pid>/environ` (main process and child) has `LAB_CANARY: 0` and no `CELIKPANEL_`
     variables.
   - The panel shows running/serving, and the new password logs in.
   - Agent log line: "the earlier environment file … could not be removed: rm … Permission
     denied". The old-file cleanup followed the planted link but ran unprivileged, so nothing
     was removed. The log line is noisy but harmless.

3. **Backup/restore + WP staging.**
   - Restore itself works: content is back to the backed-up state, the file added afterwards is
     gone, files keep `owner:www-data` and the site answers 200.
   - **Defect:** after the restore, the document root changed from `drwxr-s--- 2750` to
     `drwxr-x--- 750`; the setgid bit was lost.
   - The WordPress staging copy shows the same: `stage.wp118.test` docroot is 750, while every
     site that was never restored is 2750. The copy itself works: 8 values changed, site 200,
     title "Lab WP".
   - Impact: files the site's own account or PHP creates later get the account's group instead
     of www-data. Under the SFTP umask 0027 the web server then cannot read them. File-manager
     uploads still get www-data.
   - This is likely the new descriptor-based restore staging (Fchmod without the setgid bit). It
     was not compared with the build before the fix.
   - No link swap during the restore stage was tried.

4. **MediaWiki** 1.43.11 is set up, and the main page answers 200.

5. **Mailbox 90% warning.**
   - The hourly timer at 22:02 delivered "Your mailbox is 119% full / Posta kutunuz %119 dolu"
     once.
   - The manual start at 22:10 did not warn again.
   - The marker `/var/lib/celikpanel-quota-warning/q@c118.test` is a directory, `root:root 700`,
     inside the `vmail 700` folder.

6. **B update.**
   - The update succeeded in 61 s. Migrations 057-061 are present, services are active,
     `nginx -t` is OK and there are no panics.
   - The first start got the transient `409 PANEL_UPDATE_BUSY` again. The alpha.117 panel sent
     it right after the lab created data; the retry was accepted.

7. **B n8n.**
   - Node.js 24.22.2 and n8n are installed after the update.
   - `EnvironmentFile` is `/etc/celikpanel-svcapps/celiksvc-1.env` (root 600, folder 700).
   - HTTPS answers 200 and the owner login answers 200.

## Cleanup

Guests, the lab portal, the dists and the canary went with the guests.
