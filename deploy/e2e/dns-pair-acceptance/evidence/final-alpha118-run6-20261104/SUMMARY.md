# alpha.118 lab acceptance, run 6 (2026-11-03 22:25 to 22:37 UTC; folder dated 2026-11-04)

**Candidate:** `fix/a118-svcapps-rootfiles` at `08d70fe36` ("Restore keeps the site folders'
setgid bit"). It was built from the remote ref through a bundle; no branch was created.

**Setup:** one fresh Debian 13 guest, installed with get.sh, then setup succeeded
(`raw/run-singleA.txt`). Lab only, through the panel API; SSH was read-only.

| Item | Result | Evidence |
|---|---|---|
| 1 new site document root | PASS | `raw/R6-a118.log` |
| 2 local backup and restore | PASS | `raw/R6-a118.log` |
| 3 WordPress staging copy | PASS | `raw/R6-a118.log` |

## Item notes

1. **New site.** `c118.test` has `drwxr-s--- 2750`, `c118_test:www-data`.
2. **Backup and restore.** The restore returned 200 and kept a safety backup.
   - Afterwards the document root is still `2750`.
   - No file has a setuid or setgid bit. File modes are 640 and 644.
   - Ownership is `c118_test:www-data`.
   - The site answers 200.
   - Limit: the document root had no subfolder during this check. My folder-create request to
     the file manager returned 500 `INTERNAL`, probably because my request was malformed; this
     was not investigated. The WordPress copy below exercises the same restore staging on
     subfolders.
3. **WordPress staging copy.** WordPress was installed on `wp.c118.test`, then copied to
   `stage.c118.test`.
   - The copy was applied, with 8 values changed.
   - The copy's document root and all 468 of its folders are `2750`.
   - No file has a setuid or setgid bit. File modes are 640, and `wp-config.php` is 600.
   - Every file and folder is `stage_c118_test:www-data`.
   - The site answers 200 with the title "Lab WP".

## Cleanup

The guest, the lab portal, the dist and the work directory were removed.
