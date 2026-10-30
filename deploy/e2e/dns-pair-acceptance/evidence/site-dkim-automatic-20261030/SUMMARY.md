# Every site's mail DKIM-signed by itself — evidence (2026-10-30)

Branch `fix/site-dkim-automatic` (1c4cfefc7, d2323245d) from `release/alpha113`
550462123, merged into `release/alpha113`. Native check built from d2323245d
(acceptance-licence dist labelled `v0.1.0-alpha.113`, `dist.json`) on the
owner's topology: Debian 13 main (PowerDNS main DNS, mail, web) and Ubuntu
24.04 backup joined by code (PowerDNS backup). Both were installed through the
owner's `get.sh` command as in the final acceptance (`get.sh.diff`: pinned key
digest only), and set up through the panel API (`pair.out`: both
`succeeded`). Lab only.

The secret scan was clean (156 files plus the lab signing key and mailbox
passwords, `secret-scan.json`). Full logs and drivers are in
`site-dkim-automatic-evidence.tar.gz` (SHA-256
`ccbf949cf88c15b5661c38cc644af31d21a065a4fb4fb6ada26023f98ec3cb88`).

The Mail authentication card was never pressed in either cycle.

| Check | Result | Where |
|---|---|---|
| Site `shopdk.test` (DNS on the pair) with mailboxes: key, origin record, signing-table and key-table entries made by themselves | yes, about 10 s after creation; `[dkim-auto] shopdk.test: DKIM ready (domain_created; record published …)` | `ops.out`, `check-dk-a-before.log` |
| DKIM TXT served by main and backup DNS server | yes, same value; second cycle: p= digest `47d1fe03…` on both, matching the key's public half | `ops.out`, `post-dk-a.log` |
| Card while it works, and after | `automatic: pending`, then no automatic field and `dkim_signed: true` | `ops.out` |
| 587 mail signed and verifying | `d=shopdk.test s=celik`; second cycle verified independently: `body_hash_ok`, `signature_ok` | `verify-dk-a-site2.log` |
| PHP `mail()` signed and verifying | `opendkim-testmsg rc=0`; independently `signature_ok` | `check-dk-a-before.log`, `verify-dk-a-site2.log` |
| Missing file download | `404 FILE_NOT_FOUND` with guidance; the web client shows it in the file manager (`web/tests/site-dkim-automatic-alpha113.test.mjs`) | `ops.out` |
| Delete the site | `200 deleted` with no `dkim` note (the key was CelikPanel's); key directory gone; not in the signing or key table; TXT gone from both servers within 1 s | `check-dk-a-after.log`, `post-dk-a.log` |
| OpenDKIM still signs the main domain after a deletion | main domain added as a site: existing key kept (p= `4c0f3112…` the same before and after, and on both DNS servers); mailbox `carol`'s 587 mail `d=dkf113.test` gives `signature_ok` | `post-dk-a.log`, `verify-dk-a-main.log` |
| OpenDKIM active throughout | yes | check logs |

`opendkim-testmsg` cannot parse the CRLF 587 test message
(`dkim_chunk(): Syntax error`), as in the D3 run. Those signatures were checked
with the D3 run's independent RFC 6376 verifier (`verify.sh`) against the key.
The key's public half was compared with what both DNS servers serve.

**Run notes**
- The guests' QEMU processes end with the systemd unit that started them. The
  second cycle (`postwrap.sh`) therefore began by powering both guests on from
  their disks; both panels answered at once.
- On that boot, the Panel's startup milter wiring was refused once because
  systemd was still "starting". The existing startup retry path handles this;
  this change did not touch it.
- The card's live-DNS column shows "In zone — waiting for DNS" because the
  panel host does not resolve the lab's `.test` names. Both authoritative
  servers answer.

**Open here**
- A key made before this release has no origin record, so deleting its domain
  keeps the key and names it. This is by design.
- OpenDKIM is restarted on each signing change, as "Generate DKIM key" already
  did.
- Domains imported from cPanel archives, and mailboxes added by import, do not
  trigger automatic DKIM.
- An upgraded server does not mint keys for existing domains by itself; their
  cards offer "Generate DKIM key".
- Setup creates no `domains` row for the server's main domain, so that domain
  has no Mail authentication card until it is added as a site.
