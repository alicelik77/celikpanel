# Standalone zone delete proof (D5) — native check (2026-10-31)

Branch `fix/standalone-zone-delete-proof` (e6b9493d0, af289147d, eeeff7237,
8feee463b) from `release/alpha113` dc0946c14, merged into `release/alpha113`.
Lab only (beta1 QEMU on the archlinux WSL host). Dist of `af289147d` labelled
v0.1.0-alpha.113, installed through `get.sh` in a root terminal (lab portal;
`get.sh` differs only in the pinned key digest, `lab/get.sh.diff`). Setup
wizard via the panel API (custom components, local DNS, PowerDNS main,
standalone); Pebble CA. Secret scan clean (`secret-scan.json`); tarball
`standalone-zone-delete-proof-20261031.tar.gz` (SHA-256
`da3916a1c909ff728b8dc146bbe109cc4f00408465c9c10e49955decf4b5f765`).

**Cause.** Final acceptance run 3 found that deleting a site on standalone
PowerDNS held every later change on the server. The deleted-zone proof
accepted only an authoritative NXDOMAIN or NODATA that carries a parent
zone's SOA. A standalone server does not serve the parent zone, so it answers
an empty REFUSED, and the proof could never pass. Startup recovery re-ran the
same proof, so restarting the Agent kept the hold. Paired mains already used
the native primary's proof (the PowerDNS database's own deletion record plus
an empty REFUSED). BIND was not affected.

**Fix.** Standalone PowerDNS uses the same proof as the native primary:

- The exact deletion record exists and no zone row exists, read before and
  after the DNS check.
- The DNS check runs over UDP and TCP from the server's own address and gets
  an empty REFUSED, or the strict answer form.
- The following still hold the server: a timeout, SERVFAIL, the zone's own
  SOA in the answer, REFUSED with the AA bit or with records, an answer from
  a foreign address, or a zone row that reappears during the check.

When the Agent holds a publication, the Panel now names reason
`agent_mutations_held` with the owner's next step (restart the Agent, then
retry the same change). It no longer says "already in progress" or
"unknown".

| | Arch (PowerDNS 5.1) | Debian 13 (PowerDNS 4.9.18) |
|---|---|---|
| Setup (18 steps) | succeeded | succeeded |
| Site with mailbox: DKIM TXT appears by itself | 11 s | 11 s |
| DELETE | 200 `deleted` in 12 s | 200 `deleted` in 18 s |
| After the delete | SOA REFUSED; TXT and A empty; DKIM lines 0; only the setup zone left | same |
| Agent hold | none; every `dns_zone_sync` job succeeded; 0 fail-closed or ambiguous lines | same |
| Next site | created; A answers at once; SOA NOERROR aa; http 200 | same |

Logs: `lab/setup-*`, `install-*`, `plan-*`, `d5site-*`, `d5diag-*`. Component
tests: `tests/*.out`. The rc 127 in Arch provisioning came from the lab trust
helper, which is part of the harness, not the product.

**Open**

- A server already held by the earlier build was not reproduced natively.
  Its recovery is shown only by the component test. That build was never
  released.
- Legacy paired primaries keep the strict proof.
- An engine switch or adopt that carries a pending deletion uses the strict
  proof and fails closed.
- The BIND legacy rollback repair path uses the strict rule.
- BIND standalone was not re-run natively.
