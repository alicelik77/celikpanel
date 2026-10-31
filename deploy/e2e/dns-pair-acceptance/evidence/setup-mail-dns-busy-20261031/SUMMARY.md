# fix/setup-mail-dns-busy (D10–D12) — native evidence (2026-10-31)

Branch `fix/setup-mail-dns-busy` (2862f8d65, f58be5dcc) from
`release/alpha113` 5c1d16c6a, merged into `release/alpha113`.

**Result: pass.** Three fresh pairs finished server setup: a Debian 13 main
and an Ubuntu 24.04 backup, joined by code. In each pair, both servers serve
the main zone's `celik._domainkey` TXT authoritatively. A busy hold injected
at the mail DNS publication was waited out. After a failure injected after
the join, the owner's documented path resumed setup.

**Method.** The same as final acceptance run 5.
- **Build:** acceptance-licence dist labelled `v0.1.0-alpha.113`, from
  `2862f8d65` (`dist.json`).
- **Install:** the owner's `get.sh` against the lab portal; only the pinned
  key digest differs (`get.sh.diff`).
- **Setup:** through the panel API with run 5's wizard answers.
- **Sampler:** read-only, on each main. Every 0.5 s it logged the processes
  the Agent counts as package-manager activity.
- **Lab faults:** labelled; each is only a file or a process on the guest.
- **Secret scan:** clean (342 files, 32 secrets, plus the lab signing key and
  the panel passwords).
- **Logs:** `setup-mail-dns-busy-evidence.tar.gz`, SHA-256
  `b650a5ea28dfe97da57c532d7193e716df00058d989ecbcc69140e02552ef6b4`.

## Fixes
- **D10, busy host during setup.** A publication refused only because the
  host is busy now waits in the step (`server_setup_host_busy`), with a
  30-minute limit. Before, setup recorded it as a verified failure.
  - The pending-publication reconciler republishes a zone saved but refused
    as busy, with backoff.
  - Automatic DKIM starts no host work while setup runs.
  - The busy refusal now names the Agent's reason and the job holding the
    host (`held_by=`).
- **D11, plan review after a join.** The review now uses the backup that
  actually joined (`serverSetupJoinedReviewBinding`). Before, the joined
  backup's real address in the zone was refused as an infrastructure
  conflict. An owner's own change to that address is still a conflict.
- **D12, needless Panel restart at boot.**
  - A Panel stopped during the startup wait now logs a stop line, not the
    timeout text.
  - Cause of the restart: at every boot, the Agent took a Panel listener that
    was not listening yet for a wrong certificate, and restarted the Panel.
    An unreachable listener is now checked again after 10 s.

## Pairs
**A (normal)**
- Backup done 02:15:12, main 02:18:34.
- Step 15-mail_dns succeeded; the DKIM TXT is served by both servers; 0
  pending.
- Reboot of both servers:
  - one Panel start per boot;
  - "finished starting after 1 s / 3 s";
  - no timeout line, no failed units;
  - the DKIM TXT was still served.

**B (lab fault: a 45 s process named `dpkg`)**
- The hold landed in step 14 (mail profile).
- The step waited with `server_setup_host_busy` and continued.
- Setup succeeded; the DKIM TXT is served by both servers.

**C (lab faults: a key fault, then a busy hold)**
1. An unreadable file at the DKIM key path made 15-mail_dns fail after the
   join (`server_setup_mail_dns_dkim_failed`, naming the file).
2. The owner removed the file and reviewed the plan again: `can_start: true`,
   no blockers. Run 5 had answered `server_setup_infrastructure_dns_conflict`.
   The owner then started setup again.
3. In the resumed run, a 45 s `dpkg` hold began right after the signing
   reload. The publication was refused three times, each logged with
   `reason=package_manager_active`. The operation showed `waiting mail_dns
   server_setup_host_busy`.
4. The record was then published. Setup succeeded at 02:43:07, and the DKIM
   TXT is served by both servers.

## Open
- What held the host in run 5 is not proven.
  - In pair A, no package-manager process ran during the mail DNS window.
  - A fresh setup has no `domains` row for the main domain, so automatic
    DKIM had nothing to do.
  - The next occurrence will be named (`held_by=`).
- Each waiting pass rewrites the same DKIM value and advances the zone
  generation. This is harmless: one SOA serial per pass.
- The DNS card has no text of its own for a zone that waits only because the
  host was busy. The zone is republished within about 30–60 s.
