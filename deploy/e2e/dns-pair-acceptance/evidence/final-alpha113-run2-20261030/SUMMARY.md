# Final alpha.113 acceptance through get.sh — run 2 (2026-10-30)

Release commit 132e895cf, acceptance-licence build labelled
`v0.1.0-alpha.113` (`dist.json`); same owner-path method as run 1 (the
owner's command typed unchanged; lab portal copy reached through a `curl`
stand-in; `get.sh` differs only in its pinned key digest, `get.sh.diff`).
Secret scan clean (305 files, `secret-scan.json`); full logs in
`final-alpha113-run2-evidence.tar.gz` (SHA-256
`8a11b00747a2344fb1a7c585fd8e35f1435c51a4eb0c355e2c196f908ce3e160`).
Installs: Debian 44 s, Ubuntu 52 s, Arch 124 s. No guest clock was
corrected (guests 0.8 s behind the CA).

**A. Debian 13 main + Ubuntu 24.04 backup: every step passed.**

1. Licence active, both setups succeeded, deletion check ready on both.
2. Certificate issued in 4 s; http → https 301 on site and www; switch to
   PHP 8.4, a PHP page served with `Server: nginx` and no `X-Powered-By`;
   welcome page discloses nothing. Renewal state `owner_configured` is the
   lab's certbot `cli.ini` (Pebble), expected.
3. Mail submitted on 587 read over IMAPS after 8.7 s (D1 fixed); 587/465/993
   serve the mail certificate; webmail http → https 301; Roundcube login
   lists the message.
4. Records reach the backup in 0–1 s; zone delete verified in 23 s.
5. Reboot of both: everything came back.
6. Panel and Agent off, main rebooted: https, PHP, mail, DNS keep working;
   certbot.timer active; management back: pair ready, 0 pending.
7. Every wait carried a reason; no busy state.
8. No cron lock lines, no failed units, no postmap lmdb line.

**B. Arch standalone (PowerDNS): passed except D3.** Setup, certificate in
4 s, 301, PHP 8.5 page without version headers, mail send and IMAPS read,
Roundcube, reboot, authoritative answers.

**Defect D3 (Arch).** Submission (587) and smtps (465) run chrooted
(`cmd/agent/mail_submission_rpc.go` writes chroot `y`), but Arch has no
`/var/spool/postfix/etc`, so `localhost:8891` (OpenDKIM) and
`localhost:11332` (rspamd) do not resolve inside the chroot: 58 warnings,
and with `milter_default_action=accept` user-submitted mail is neither
DKIM-signed nor spam-checked. Port 25 (not chrooted) is unaffected; Debian
has the chroot files and shows no warning.

**Observations.** `mail server hostname is not a valid FQDN: invalid
hostname "frankfurt"` logged once at the first panel start during setup; the
file manager answers a bare 500 for a path that does not exist.

**Harness note.** The two topologies first shared one temporary password
file and check script, which spoiled the first mail checks; each cell then
got its own files and the mail, PHP and Arch reboot checks were redone
(`redo-a.out`, `redo-b.out`, `archreboot2.out`). Those failures were the
harness's, not the product's.
