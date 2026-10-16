# v0.1.0-alpha.91 (candidate, not published)

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.91.tr.md)

This is the release that follows v0.1.0-alpha.90. The number v0.1.0-alpha.91
was chosen by the release preparer under the owner's standing instruction of
2026-10-13. Written on 2026-10-16 (UTC), on the branch
`feat/alpha91-second-server-part2`, from `main` at
`1743ac69edb0e5723dc60d992b53094dcc4b1431` (the alpha.90 release commit, merged
into the branch). The product commits of the branch are `7cd9232db`,
`5a749b5c6` (code, tests, batch 18) and `c99c48b70` (the operation-guidance
record).

## The owner's decisions and remark

On 2026-10-16 the owner decided the three questions left open by part 1 of the
second-server work (alpha.87), in the words **"önerin gibi olacak"** (it will be
as you proposed):

1. The second server can be a **full web and mail peer** of the first.
2. The **infrastructure zone** (for example `celikhost.com`, the panels' own
   zone) is shown on the primary as a **normal entry of Domains**, with the
   existing DNS record editor. Records that setup created stay managed by
   setup; the owner's own records are preserved.
3. The second server's **host records travel to the primary automatically**
   over the existing authorized remote DNS connection, and manual entry stays
   possible.

The owner also remarked that users configure their own servers: **the panel
offers; the user decides.** Hence nothing here is imposed. A refused or
unavailable request leaves the exact record to enter by hand, and the owner's
own records are never overwritten.

## What changed

Three of the gaps found in the alpha.87 investigation (4, 5 and 6) are
corrected in source.

**Gap 4: the peer's mail host name in the primary's plan.** The primary's
infrastructure DNS selection gains `peer_mail_hostname`, prepared as an A
record to the reviewed peer address, so it appears in the plan review. Typed
plan blockers: `server_setup_infrastructure_dns_peer_mail_own_name` and
`server_setup_infrastructure_dns_peer_mail_outside_zone`. An older plan has no
such field and keeps its identity. Code: `cmd/panel/server_setup.go`,
`server_setup_infrastructure_dns.go`, `server_setup_operations.go`;
`web/src/components/ServerSetup.tsx`. Tests: `server_setup_peer_mail_test.go`
(three); `web/tests/server-setup-runtime.test.mjs`.

**Gap 5: the infrastructure zone in Domains.** Administrator only:
`GET /api/v1/dns/infrastructure-zones`, and `GET …/{zone}/zone` and
`GET|POST|PUT|DELETE …/{zone}/records` on the existing record handlers. Each
record says who keeps it (`managed_by`: `setup` or `peer`). Changing or
removing a setup record is refused with
`INFRASTRUCTURE_DNS_RECORD_MANAGED_BY_SETUP` (the action is Settings, Server
setup); the owner's records are edited as before, and a setup address the
owner changed is no longer setup's. Adding a website for the zone's own name
adopts the existing zone untouched; removing that website keeps the zone. Code:
`cmd/panel/infrastructure_dns_zone.go`, `domain_dns_handlers.go`,
`domain_delete_saga.go`; `web/src/components/Domains.tsx`,
`InfrastructureZone.tsx`. Tests: `infrastructure_dns_zone_test.go` (five);
`web/tests/remote-state-mounted.test.mjs` (five, each with a control).

**Gap 6: host records over the remote DNS connection.** The primary accepts
`POST /api/v1/dns/remote/receiver/host-record` (one new machine endpoint on the
remote DNS receiver, using the connection's existing machine credential): one
name strictly inside an infrastructure zone, type A with the DNS pair's
verified peer address. It never overwrites; refusals are typed:
`REMOTE_DNS_HOST_RECORD_CONFLICT`, `…_INVALID`, `…_OUTSIDE_ZONE`, `…_ADDRESS`,
`…_PENDING` (saved, publication not yet verified; the same request may be sent
again). It runs under the same locks as every DNS publication and is audited
(`dns.remote.host_record.*`). The secondary sends it from setup's public DNS
step once per step identity, an unknown result again after two minutes, five
times at most; a read of the page sends nothing. Zone replication stays the
native transfer and never depends on this channel. Code:
`cmd/panel/remote_dns_host_record.go`, `server_setup_access_dns.go`,
`setup_dns_remote.go`, `remote_dns_auth.go`, `mail_dns_identity.go`. Tests:
`remote_dns_host_record_test.go` (ten, among them the zone working with no
connection at all); `web/tests/server-setup-guidance.test.mjs` (two),
`remote-state-mounted-batch3.test.mjs` (one).

## What the owner will see

**On Frankfurt (the primary).** Domains lists the infrastructure zone as a row
marked "Infrastructure zone · DNS only", with one action, "Manage DNS records".
The zone's page says what the zone is, which records setup keeps and where they
are changed, that the rest are the owner's and which are the peer's, then
shows the usual record editor. Records setup created read "managed by setup";
records the other server asked for read as the peer's. A zone list that cannot
be read is one line with Retry and never hides the domains.

**On Boston (the secondary), when adding mail.** If the two panels' remote DNS
connection exists, setup asks Frankfurt to create the mail host's record by
itself, and says what happened: asking; applied or already present ("nothing
more is needed from you"); refused, with the reason; the primary does not
accept such requests; not answered yet. If it does not exist, the page shows
the exact record to enter by hand (on the primary under Domains, in the
infrastructure zone, or at the DNS provider). How to tell: the Components mail
box on Boston says either "This server has an authorized DNS connection to the
primary…" or "This server has no authorized DNS connection to the primary, so
nothing creates this record automatically. Add it by hand…".

## Decisions the release preparer made

Made by the release preparer under D-021 and D-033 and the owner's standing
instruction of 2026-10-13, recorded in the operation guidance to be confirmed;
none is an owner decision.

- No schema change: the infrastructure zone is not a tenant `domains` row; it
  is listed from setup's ownership record and edited through its own
  administrator route.
- A website removed from the infrastructure zone's own name keeps the zone.
- "Managed by setup" means the A/AAAA records setup prepares with their exact
  value; SOA and the apex NS stay "managed by DNS settings".
- The peer's records can be changed or removed by the owner; the channel then
  refuses to overwrite or recreate them.
- The primary accepts an A record only with the DNS pair's peer address, and an
  AAAA only with the request's own source address; the secondary sends the A
  record only.
- The request is sent from the setup's access DNS step, not from a separate
  step or button, so existing plans keep their steps.
- An unknown result is asked again after two minutes, five times at most; a
  refusal is not sent again for the same step.
- Once the primary applied the record, the setup page no longer draws the
  generic "follow the record instructions" line.

## What was measured

- `npm test` in `web/`: 1534 of 1534 (before the merge of alpha.90; not re-run
  after the merge). `npx tsc --noEmit` clean; `npm run build` within its budget.
- `go test ./cmd/panel` in the WSL guest at `5a749b5c6`: 1581 top-level tests
  pass, 6 skip, 1687 subtests pass; gofmt and vet clean. Eighteen new Go tests
  and eleven new web tests; each new rule was switched off once and its test
  failed. Not re-run after the merge.
- Mock browser batch 18 (desktop and phone, English and Turkish, light and
  dark): no English in Turkish, no raw key, no horizontal overflow; two
  interface defects found and corrected. The Components box with and without a
  connection was not drawn by the mock and rests on a mounted test.
- The release sequence policy contract test passes ("90 -> 91").

## Limits

- **No native record**, by the owner's instruction of 2026-10-16. The evidence
  is component, mounted and Go tests and one mock browser batch.
- Nothing was measured on an installed server; no installed panel was updated
  or contacted.
- The channel between the two installed panels (the authorized remote DNS
  connection) must exist for the automatic record; without it the record is
  entered by hand, as shown above. An older primary refuses the new request
  plainly; the secondary then shows the manual record.
- Schema 44 is unchanged and no migration runs; recovery behaviour is
  unchanged. New `panel_settings` keys and JSON fields are additive.
- P0.1-P0.5 stay **partial**; no P0 item is closed or advanced; publication is
  not acceptance.

## Publication record

Not published.

Install this release only through CelikPanel's update interface. Publishing it
does not update installed servers; only the owner of a server starts its
update, from the panel's own update screen.
