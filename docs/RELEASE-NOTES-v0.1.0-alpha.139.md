# v0.1.0-alpha.139

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.139.tr.md)

A small release of two jobs, both from the alpha.135 lab. When the backup DNS
server of a pair asks its main DNS server to add one of its own host records,
the main server now says truthfully, and in time, what it did. On top of
that, webmail's address check on the backup DNS server gets **Add…** beside
**Copy**, which asks the main DNS server to add webmail's record.
**Only the owner updates installed panels, from the panel's own update
screen.**

## Updating from alpha.138

1. Update from the panel's own update screen, as before. This release does
   not change the panel's database layout (it stays at the alpha.138 form),
   so going back to alpha.138 later needs no database restore.
2. There is nothing to press or set after the update. Update both servers of
   a pair when you can; each answer below also works with a server that still
   runs alpha.138 (see "With a server on an older release").

## What changes for you

### The main DNS server answers a host-record request truthfully

The backup DNS server of a pair can ask the main DNS server, over their
authorized DNS connection, to add one of its own names (for example its mail
name) to the main domain's zone. Server setup does this on the backup server,
and **Add…** below uses the same request. This never replaces the ordinary
zone transfer between the two servers; it only adds a record.

In the alpha.135 lab this went wrong twice: the main server said the name was
already taken although it held no such record, and once it gave no answer at
all while it was busy deleting a domain. Now:

- **A customer's domain under the main domain no longer blocks it.** A
  customer domain such as `shop.example.com` beside the main domain
  `example.com` was enough to refuse every name. Now only a zone that really
  holds the requested name refuses it, and the answer says so.
- **Each refusal says what it is.** "Another record uses this name" now means
  exactly that (another record, an alias in the way, a disabled record, or
  this record changed or removed by hand on the main server). When the zone
  itself cannot take the record (for example it is DNSSEC-signed, or the
  main server cannot confirm the zone is its own), the answer says the zone
  needs a review by the main server's administrator.
- **A busy main server says so, quickly.** The main server waits at most
  three seconds for another DNS change to finish, then answers "busy, nothing
  was added". Asking again is safe: the record is never added twice. Server
  setup on the backup server asks again by itself after two minutes.
- **Every answer is written down on the main server.** Each request that adds
  nothing leaves a line in the panel's log and a row in the activity log,
  with the reason, also when the asking server stopped waiting.

### "Add…" for webmail's address on the backup DNS server

On the backup DNS server of a pair, Components → Roundcube → Webmail address
→ **Check** shows the record webmail's name needs. When that name lies in the
main domain and the DNS connection to the main server is ready, **Add…**
appears beside **Copy**. Everywhere else **Copy** stays as before.

- **Add…** first shows exactly the record that will be asked for. After you
  confirm, the page is locked and shows the steps: asking the main server,
  the record being written there, and public DNS showing it.
- **Added** (or already there): the page then reads public DNS every 20
  seconds for at most five minutes, by itself.
- **The main server is busy:** nothing was added. **Ask again…** opens the
  same question again; asking again never adds the record twice.
- **No answer within 10 seconds:** the main server may still be adding it.
  The page waits 25 seconds and asks once more by itself; if the record is
  there then, it ends as added. If there is still no answer, it says the
  result is not known and offers **Ask again…**.
- **Refused:** the sentence says why and who acts: another record or a hand
  change on the main server (look at the name there, or Copy), the zone there
  needs its administrator's review, or the name is not in a zone the main
  server keeps for this pair (add it where the name is managed, with Copy).
- An IPv6 (AAAA) record is also asked for when this server has a public IPv6
  address; it is optional, and webmail works with the IPv4 record alone.

### Server setup says a zone overlap as what it is

When another DNS zone hosted on the main server lies inside the main domain's
zone or around it (for example a customer's domain under the main domain),
server setup still does not add its names to that zone; that rule is
unchanged. What changes is what the screen says:

- **The plan's summary** says that another zone lies inside or around the
  main domain's zone, that this is not a clash with a record or a name you
  chose, and that the administrator reviews that zone in Domains and then
  reads the plan again. Before, it said an existing record clashed with the
  names setup adds.
- **While the second server joins,** the setup page says the same, and that it
  is not about the joining server's names; setup continues by itself once the
  zone no longer overlaps. Before, it told you to change that server's names.

### No other company's product name

The Panel port card in Settings no longer says that port 2083 is "the port
cPanel uses"; it says the port is also used by other hosting control panels.
The import of cPanel backups keeps its name.

## With a server on an older release

- **A backup on alpha.138 or older asking a main server on alpha.139:** a
  busy answer is "not answered yet" there, and setup asks again after two
  minutes. A zone that needs a review is shown as a refusal with its generic
  sentence ("did not create this record; add it by hand").
- **A backup on alpha.139 asking a main server on alpha.138 or older:** that
  server still says "another record uses this name" also for the zone-wide
  reason above (the **Add…** sentence is written to be true for both), and
  it may still not answer while it is busy (the page then asks once more by
  itself, and then says the result is not known).

## Known limits

- **The setup rule itself is unchanged.** A main domain with a customer's
  zone under it still cannot extend its setup or bind a second server; only
  the words are new. A narrower rule is planned separately, with its own lab.
- **The card needs Roundcube.** The webmail address card (and **Add…**) is on
  Roundcube's page, so it appears only where Roundcube is installed.
- **The first answer can be "not known".** If the main server takes longer
  than 10 seconds to publish the record, the first answer is "not known"; the
  automatic re-ask 25 seconds later then reads "present".
- **Sign-in refusals of the DNS connection itself** (an unknown or revoked
  connection, refused before the request is read) still leave no log line on
  the main server.

## Checked in the lab

The lab check on a primary and backup DNS server pair is **open**; it is
written here when it has run.

## Evidence

No schema change: schema 079, as in alpha.138. No persisted record changes;
the main server's answers are additive (a new refusal code,
`REMOTE_DNS_HOST_RECORD_ZONE_REVIEW`, and an uncoded "busy" answer), and the
backup server's own answer to the page has one more outcome (`busy`). The
changes are in the host-record receiver and the remote DNS publication
receiver (`cmd/panel/remote_dns_host_record.go`,
`setup_dns_remote_receiver.go`, `server_setup_infrastructure_dns.go`), the
backup server's request (`webmail_address_dns_record.go`), the setup review
and peer join wording (`server_setup_operations.go`,
`server_setup_peer_join.go`), and the page (`WebmailAddressCard.tsx`, now
loaded only on Roundcube's page, `lib/webmailAddressRecords.ts`, the
`dnsAdd`, `setupDNS`, `setupLink`, `problems` and `panelPort` texts). Go
tests run the request over the real TLS transport against the production
receiver: a customer zone beside the main zone ends added; a name under it is
refused, nothing written; busy within the bound, nothing written; no answer,
then present, with one record; an older main server's "taken"; every refusal
leaves a log line and an activity row; the setup review and the peer join say
the overlap as its own cause. Mounted web tests cover every end of **Add…**
(busy, the one re-ask, not known, each refusal) and both setup sentences.
`./cmd/panel` was compared name by name with alpha.138 in a Debian guest.
**No P0 item is closed.**
