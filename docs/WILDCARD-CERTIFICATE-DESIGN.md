# Wildcard certificate (`*.domain`) through DNS: design note

*2026-11-08 · alpha.130 roadmap item 3 (ROADMAP.md, alpha.130 — Mail).
**Not built in alpha.130.** This note records why, and the design a later
release can build and prove.*

## 1. What the owner can already do

**The certificate request route.** A wildcard certificate is already
possible today through the certificate request route (alpha.128 G31;
docs/CERTIFICATE-REQUESTS.md):

- The request names `*.example.com` and `example.com`.
- The key stays in the Agent.
- The owner has any CA sign it.
- Installing the issued certificate puts it on the site. Coverage of
  subdomains by a `*.` name is already recognised by `certificateCoversHostname`.

**What is missing** is the automatic part: issuing through ACME DNS-01 from
the panel's own PowerDNS, and renewing without anyone.

## 2. Why it is not shipped in alpha.130

The alpha.114 postponement named the problem. Reading the current source
confirms it. Every simple design breaks a rule the product already promised.

1. **Renewal must outlive the panel (D-022, D-040).**
   - Website certificates are certbot lineages that `certbot.timer` renews
     with no CelikPanel program.
   - A wildcard renews every 60–90 days. Its DNS-01 answer must therefore be
     written by something that is not the Panel or the Agent.
   - A design in which the Panel writes the TXT record through its zone
     ledger works only while the Panel runs. Within 90 days of a panel
     removal, every wildcard certificate would expire. That is the exact
     defect D-040 corrected for HTTP-01.
2. **The panel's zones are exact commitments.**
   - The Agent proves that a live PowerDNS zone equals the durable receipt of
     its last publication: `verifiedDNSZoneCommitment` in
     `cmd/agent/dns_zone_sync_commit.go` compares every record.
   - A TXT written beside it, by a certbot hook or by an RFC 2136 update,
     makes the live zone differ from the receipt. The Agent treats that
     difference as split publication authority.
3. **SOA serials would go silently wrong.**
   - The Panel computes the next serial from its own ledger
     (`nextSOASerial`, `cmd/panel/dns_serial.go`).
   - An out-of-band change also raises the serial in the backend (PowerDNS
     `SOA-EDIT-DNSUPDATE`). The ledger's next serial can then equal the
     served one.
   - A secondary sees no newer serial and never transfers the panel's next
     change. Replication stops without any error.
4. **A separate challenge zone breaks the engine's own checks.**
   - The primary switch requires the exact zone count and zero TSIG keys
     (`dns_engine_pdns_switch.go`: `domainCount == expectedDomains`, the
     `tsigkeys` count).
   - The catalog producer enumerates every native zone
     (`dns_engine_pdns_catalog.go`).
   - The secondary rollback snapshot does the same
     (`dns_engine_pdns_secondary_rollback_db.go`).
   - An extra zone in the same backend, or the TSIG key an RFC 2136 path
     needs, therefore makes those safety checks refuse.
5. **The PowerDNS drop-in is byte-exact.**
   - `internal/pdnsmanagedconf` renders `celikpanel.conf` exactly.
   - The owner's `pdns-peer-inspect` recognises a CelikPanel server by
     rendering it again.
   - A second backend changes what the inspector must know.
6. **It cannot be proven in this workstream.** The proof needs a real
   PowerDNS primary and secondary pair on the supported systems and a Pebble
   CA. That is the lab's VM work, not a component test. The release brief
   asks for a smaller finished item over a half-done one. The DNS engine is
   also the subsystem with the most incident history.

## 3. Design for a later release

### 3.1 Delegation: one permanent record through the ledger

When the owner turns on a wildcard for `example.com`, the normal DNS write
path publishes one record:

```
_acme-challenge.example.com.  NS  <the primary's own name server name>.
```

- It is an insecure delegation (no DS). PowerDNS proves the absent DS in a
  signed parent.
- The secondary serves it as part of the parent zone, so a resolver that
  asks the secondary gets a referral to the primary.
- The secondary needs no new zone and no catalog change.

### 3.2 Challenge zone outside the panel's database

- The zone `_acme-challenge.example.com` lives only on the primary, in
  PowerDNS's **bind backend**.
- That backend is loaded by its own drop-in `celikpanel-acme.conf` with
  `launch+=bind`.
- The zone list is `/etc/powerdns/celikpanel-acme/named.conf`, and the zone
  files are under `/var/lib/celikpanel-acme-dns/`.
- PowerDNS answers from the most specific zone of any backend. The gsqlite3
  database, its receipts, the catalog and the switch never see this zone.
- The inspector learns the third drop-in.
- Removing the wildcard removes these things in the same operation, so
  nothing is left behind:
  - the delegation record;
  - the zone file;
  - its `named.conf` line;
  - the drop-in, when it is the last zone.

### 3.3 Renewal with certbot alone

- The lineage is issued with
  `certbot --manual --preferred-challenges dns --manual-auth-hook <A> --manual-cleanup-hook <C>`.
- certbot stores both hooks in the renewal configuration, so `certbot.timer`
  renews with nothing of CelikPanel running.
- `<A>` and `<C>` are plain POSIX sh, with no CelikPanel program:
  - **`<A>` (authorise).** It writes the zone file with SOA, NS and the TXT
    from `$CERTBOT_VALIDATION`, and runs
    `pdns_control bind-reload-now <zone>`. It then waits until the primary
    serves the TXT, asking the primary's address and giving up after a bound.
  - **`<C>` (clean up).** It writes the zone back to SOA and NS only. certbot
    calls it after every validation, a failed one included.
- The existing deploy hook reloads nginx and the mail services as today
  (D-040 §4).
- If certbot is killed between the two hooks, the Agent removes a stale TXT
  at its next start.

### 3.4 First issuance, by the Panel

1. Check the prerequisites, in this order:
   - The engine is PowerDNS.
   - This server is the domain's primary.
   - The site is on this server.
   - The parent zone is published.
2. Publish the delegation record and wait until every name server of the
   domain serves the referral. The check uses `internal/dnswire` against
   each NS address, including the secondary.
3. Write the challenge zone. The hooks then do the rest through certbot.
4. Treat states as D-024 states, with the same failure and unknown guidance
   as other certificates:
   - "delegation not yet served by `<ns>`" (waiting);
   - "the CA refused the TXT" (verified failure);
   - "no answer" (unknown).

### 3.5 Certificate authorities

The flow uses certbot's own ACME, with each CA handled as today:

- **Let's Encrypt:** as today.
- **ZeroSSL and Google Trust Services:** the existing EAB handling, through
  a 0600 config file that is removed after the run (D-040 §5).
- **Tests:** Pebble only, with `PEBBLE_VA_ALWAYS_VALID=0` and a test
  resolver that points at the test PowerDNS.

### 3.6 Refusals, in plain words (EN/TR on the screen)

- **The engine is BIND.** The panel publishes BIND zones as whole files it
  owns. A record written by a renewal hook would be overwritten or would
  break the serial order. The owner can:
  - use a certbot DNS plugin for a DNS provider they own;
  - use the certificate request route;
  - switch the server to PowerDNS.
- **The site is on a joined server.** The domain's DNS is on the first
  server. A renewal on this server could not write there without a remote
  API, and D-022 forbids depending on one. The owner can:
  - move the site;
  - use the certificate request route.
- **Remote DNS mode (the DNS is elsewhere).** As above.

### 3.7 Acceptance before release (lab)

1. Pebble with DNS-01 against a PowerDNS primary and secondary pair, on
   Debian 13 and Ubuntu 24.04:
   - The challenge is asked of both name servers. The secondary refers to the
     primary.
   - No TXT is left after success.
   - No TXT is left after a forced failure, using a wrong token.
2. Renewal with the Panel and the Agent stopped (`certbot renew --force-renewal`).
3. Primary switch, rollback and the peer inspector with the acme backend
   present.
4. A DNSSEC-signed parent, which needs the insecure delegation.
5. Removal leaves no file, line or record.

## 4. Decision needed from the owner

The design adds a second PowerDNS backend that CelikPanel owns on the
primary, plus certbot manual hooks. Both are native and keep working
without the panel.

It still changes the PowerDNS configuration of every primary that uses a
wildcard. It should be approved as a direction before it is built, the way
D-040 was.
