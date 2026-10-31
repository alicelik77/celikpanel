# Final alpha.113 acceptance through get.sh — run 1 (2026-10-30)

Release head f81a4dfd5, acceptance-licence build (`dist.json`), lab QEMU/KVM
guests only, local Pebble CA. Secret scan clean (277 files,
`secret-scan.json`); full logs in `final-alpha113-evidence.tar.gz` (SHA-256
`20ae9c8f6282c942187c68683cb02a9bc626d6036987accc79a0f76972fc2a74`).

**Owner path.** On each guest, in a root terminal:
`curl -fL --proto '=https' -o celikpanel-install.sh https://celikpanel.net/get.sh && sh celikpanel-install.sh`,
admin prompts answered on the terminal. `celikpanel.net` was a lab portal
copy (signed `release-manifest-v2`, lab Ed25519 key) served by a `curl`
stand-in on that session's PATH (the seam of
`test-fresh-install-entry-boundary-contract.sh`); `get.sh` differed from the
release's only in the pinned key digest (`get.sh.diff`). Installs: Debian
53 s, Ubuntu 58 s, Arch 105 s (Arch asked for a restart).

**A. Debian 13 main (PowerDNS, mail, web) + Ubuntu 24.04 joined by code.**

| Step | Result |
|---|---|
| 1 install, licence, setup, deletion check | pass (setup about 6 and 4 min; check ready on both) |
| 2 site with SSL at creation | certificate pass after a lab clock correction (Pebble does not backdate; the guest trailed the CA by about 2 s, the request answered a bare 500); welcome page discloses nothing; **switching the site to PHP fails (D2)**; http is not redirected (site default `force_https: false`) |
| 3 mail | submission 587 STARTTLS + auth pass; 587/465/993 serve the right certificate; **IMAP read fails (D1)**; webmail enabled from its card, https 200 |
| 4 DNS | records reach the backup in 0–3 s; zone delete verified with the proof in 28 s |
| 5 reboot both | pass (IMAP still D1) |
| 6 panel and Agent off, main rebooted | https, mail, DNS keep working; certbot.timer active; management back: pair ready, 0 pending |
| 7 waits | every wait carried a reason; no busy state |
| 8 journals | no cron lock lines, no failed units; notes below |

**B. Arch standalone (PowerDNS).** Setup, https site (same clock correction),
mail send and read, reboot, authoritative answers: pass. **PHP switch fails
(D2).**

**Defects.**
- **D1** Debian 13 (Dovecot 2.4.1): `mail_inbox_path = /var/mail/%{user}`
  inherited from Debian's `conf.d/10-mail.conf`; the managed
  `99-celikpanel.conf` sets only `mail_driver` and `mail_path`, so INBOX
  autocreate fails (Permission denied, `NO [SERVERBUG]`). Arch 2.4.5 and
  Ubuntu 2.3 unaffected.
- **D2** A site from Add Domain is created static; `PUT
  /api/v1/domains/{id}/hosting {"project_type":"php"}` answers 409: no
  per-site PHP-FPM pool or socket is made in the transition, and the Agent
  refuses the empty socket.

**Observations.** A not-yet-valid issued certificate gives a bare 500 and is
discarded; Debian logs `postmap: fatal: unsupported map type: lmdb` once per
Agent start (map-type probe); Arch sends `Server: nginx/1.30.6`; Roundcube
exposes `rcversion`; webmail http answers 404 instead of redirecting.

Not published; defects to be fixed and the run repeated.
