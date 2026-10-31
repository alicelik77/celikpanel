# Final acceptance run 6 (alpha.113 candidate e8e7ab4e6) — evidence (2026-10-31)

**Result: fully clean.** Every check passed on both topologies and no new
defect was found. The defects of runs 3–5 (D5–D12) did not recur. The case
that blocked run 4 (deleting a site after the reboots, while the daemon
re-stamps its catalog) now completes by itself.

## How it was run
- Same method as runs 3–5.
- Installed with the owner's `get.sh` against the lab portal; only the
  pinned key digest differs (`get.sh.diff`).
- Acceptance-licence dist labelled `v0.1.0-alpha.113` (`dist.json`).
- The DKIM card was read only, never pressed. The guest clocks were left as
  the lab gives them.
- Secret scan clean: 308 files, 26 secrets, plus the lab signing key and the
  mailbox passwords (`secret-scan.json`).
- Logs: `final-alpha113-run6-evidence.tar.gz`, SHA-256
  `89fc387f2b849c6c6c25260322c796f4ca83c885f4ac27f5a35b8cada144f7b7`.

## Install and setup
- Install times: 40 s on Debian 13, 53 s on Ubuntu 24.04, 86 s on Arch.
  Licence active on all three.
- All three setups succeeded, including the pair main.
- The deletion check is ready on both pair servers.
- Every wait carried a reason.
- No `server_setup_host_busy` wait and no `held_by=` line appeared.

## Pair: Debian 13 main + Ubuntu 24.04 joined by code
**Site**
- Certificate issued in 3 s.
- 301 on the site, on www and on the PHP page.
- PHP page: `Server: nginx`, no `X-Powered-By`.

**Mail**
- 587 and 465 send, IMAPS read and Roundcube (3/3) all work.
- A bounce was delivered.

**DKIM, set up by itself**
- Ready 8 s after the mailboxes were created.
- The 587, 465 and PHP `mail()` messages verify `d=r6shop113.test s=celik`
  against the TXT from both DNS servers.
- The main zone's `celik._domainkey` TXT is served by both servers.

**Rspamd**
- Scanned the 587 and 465 mail (0.90) and the PHP `mail()` (0.19), all "no
  action".

**DNS-only zone, created and deleted at once**
- Status `running`, then "deleted" in 30 s.
- REFUSED on both servers.
- 0 `dkim_automatic` rows, 0 FOREIGN KEY lines.

**Run 4 sequence, then delete**
- Sequence: site with mailbox → reboot both → panel-off and reboot of the
  main → management back on → delete.
- With the panel off, https, PHP, mail and DNS kept working, and nothing
  listened on 2083.
- The delete read `running` throughout, then "deleted" in 36 s. TXT and zone
  are gone on both servers; 0 key-table lines.
- Agent log: `catalog re-stamp … admitted`, then `challenge superseded before
  its exchange`, then `accepted`, then `verified`.

**One more site, created and deleted without reboots**
- "deleted" in 26 s.

**Inspector**
- No inspector read failed. The backup's `celikpanel-pdns-peer-inspect`
  journal is empty.

## Arch standalone (PowerDNS)
- **Site:** certificate in 3 s; 301; PHP 8.5 page without version headers.
- **Mail:** 587 and 465 send, IMAPS read, Roundcube and the bounce all work.
- **DKIM:** on 587, 465 and PHP `mail()`, verified against Arch's own
  PowerDNS.
- **Rspamd:** 1.40 / 1.40 / 0.70, all "no action".
- **Reboot:** pass.
- **Site delete:** `running`, then "deleted" in 11 s, with no Agent hold.
- **New site afterwards:** created; certificate issued; https works; 0
  pending.

## Reboots (all three servers)
- One Panel start per reboot, with no `Stopping celikpanel-panel` right after
  boot.
- Each start shows the wait line, then "finished starting": 3 s on
  Frankfurt, 1 s on Boston, 3 s on Arch.
- 0 timeout lines.

## DKIM card and journals
- DKIM card: `served`, then `ok`, on the pair and on Arch.
- Journals, on every boot of all three servers: 0 milter warnings, 0 FQDN,
  0 panel 500, 0 fail-closed or ambiguous, 0 FOREIGN KEY, 0 lmdb lines, and
  no failed units. The only err-priority line is the known Ubuntu MariaDB
  postinst warning.

## Observations (not defects)
- PHP `mail()` gets Rspamd's `FORGED_SENDER` (0.30), still "no action". This
  is a known limit in the release notes.
- Renewal state is `owner_configured`: the lab `cli.ini` points at Pebble.
- `release_unverified` update-check lines appear, because the lab uses its
  own key and does not reach celikpanel.net.
