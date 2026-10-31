# D7/D9 peer challenge context fix — native check (2026-10-31)

Branch `fix/peer-challenge-context` (322391d78, df7e2cc63) from
`release/alpha113` 8e9c0f72f, merged into `release/alpha113`.

**Result: pass.** Final acceptance run 4 had a site deletion stuck after
reboots (D7). With this build, a site deleted after the same steps was
removed in 30 s:

1. Both servers rebooted.
2. Panel and Agent turned off on the main, and the main rebooted.
3. Management turned back on.

The status read `running` throughout and never escalated. Two more site
create and delete cycles without reboots finished in 32 s each. Every proof
was verified on its first wave.

**Method.** Run 4, lean.
- Acceptance-licence dist `v0.1.0-alpha.113` built from `322391d78`
  (`dist.json`).
- Installed with the owner's `get.sh` against the lab portal; only the pinned
  key digest differs (`get.sh.diff`).
- Debian 13 main (PowerDNS) and Ubuntu 24.04 backup, joined by code
  (`pairA.out`).
- One site with a certificate, 301 to https and one mailbox with automatic
  DKIM. The TXT record was served by both servers (`site.out`).
- Secret scan clean (225 files, `secret-scan.json`). Tarball SHA-256
  `b4a952935f417ac1166c759178630a101c614e5f4797f930de1806535b3b8d47`.

**D7 cause.** The challenge was written with the catalog serial that the
deletion had proved. While it was being written, the publish rechecks
admitted the PowerDNS daemon's re-stamp to a higher serial. Every later
challenge carried the new serial, and the journal's `sameContext` refused it
as `pdns_peer_challenge_context_mismatch`, without end.

**D7 fix.** A later challenge of the same operation may carry a strictly
higher serial when its catalog member set is identical. The members must
reproduce both the stored digest and the new one. Everything else must be
unchanged: operation, enrollment, endpoints, peer identity and zone. Attempt
+1, a new nonce and a ledger attempt that does not go down are still
required. A challenge superseded by a re-stamp is never sent. A server
already stuck recovers at its next re-check or "Retry this deletion". The
text for this state now starts as waiting.

**Deletion after the reboots** (`after.out`)
- `running` at +3 s, then 200 `deleted` at 30 s.
- Zone and TXT answer REFUSED on both servers.
- The backup logged `read 1 of 3 failed (unclassified): native observation 2:
  PowerDNS control listener changed`, then `answered on read 2 of 3`.

**D9 cause.** A connection the daemon has not accepted yet appears under the
control socket's path in state `02`. The parser accepted only state `03`, so
the check failed as "control listener changed".

**D9 fix.**
- Connection rows in state 02 or 04 are ignored.
- An unclassified native failure is read again: at most three times, 750 ms
  apart, never within 5 s of expiry.
- Each failed read is logged on the backup (`journalctl -t
  celikpanel-pdns-peer-inspect`) with no secrets.
- A real-kernel test reproduces the state-02 row.

**Cycles.** `cycle1.out` and `cycle2.out`: deleted in 32 s each, with no
inspector failure.

**Open**
- No daemon re-stamp fell inside a challenge write in this run. That path,
  and recovery of an already stuck server, are covered by unit tests only.
- Commit df7e2cc63's parser change was not run natively. The next full
  acceptance covers it.
- An owner-edited catalog member set during a pending deletion is still
  refused, by design.
- P0.4 stays open.
