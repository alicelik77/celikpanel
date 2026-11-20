# v0.1.0-alpha.142

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.142.tr.md)

**A security release: please update soon.** It closes ways in which a
customer or a reseller could take names, ports and files that belong to
another account or to the server itself: a domain name inside or above
another customer's domain or the server's own names, another site's system
account, a port of the server's own services, and another account's files
through a link.
**Only the owner updates installed panels, from the panel's own update
screen.**

## Updating from alpha.141

1. Update from the panel's own update screen, as before.
2. **The panel's database layout changes** (schema 80): six rules that check
   every new name are added; no table, column or row changes. A database
   that already holds overlapping names updates normally (see "Names that
   already overlap" below). Going back to alpha.141 later means restoring
   the snapshot the update takes, as for every release that changes the
   layout.
3. At its first start after the update, the panel writes every site's web
   server file again with the new link rule, and corrects the modes of its
   own per-site files once (see "Files"). There is nothing to press.
4. **Going back to alpha.141 opens the closed ways again.**

## Security: what is closed

- **A name of another account or of the server.** A customer's or
  reseller's subscription could receive a domain, subdomain or alias equal
  to, inside or above another account's domain, or the server's own names
  (the main domain's zone, the panel's, nameservers', mail and webmail
  addresses), and with it that name's DNS, website, certificate and mail.
  Now such a name is refused before anything is created, by the panel and
  again by the panel's database. A subdomain of one's own domain is still an
  ordinary subdomain. The administrator's own subscription may still hold
  such a name on purpose, for example the main domain's website.
- **One system account for two domain names.** Two different domain names
  can lead to the same system account name. A domain whose account another
  site or the system already has is now refused before anything is made,
  and the server's agent proves that an account belongs to the site before
  it removes anything by the account's name (scheduled tasks, extra FTP
  logins, processes, the account). Before, a refused creation could remove
  another customer's scheduled tasks and extra FTP logins.
- **A Node.js site's port.** A site could be pointed at a port of the
  server's own services (for example the database web tools), putting them
  on the internet. Now a Node.js site's port comes from 3001 to 3999 and is
  never a port of another site or of the server's services; the server's
  agent refuses such a port again whatever the panel sends.
- **A link that made the web server serve another account's file.** The
  web server followed a link in a site's folder to another account's file,
  or to the server's own files it can read. Now it follows a link only to
  a file of the link's own owner.
- **Configuration files readable by every account.** Each site's web server
  and PHP files could be read by every account on the server; they are now
  readable only by the services that use them, and the hosting folder can
  be passed through but not listed.
- **Directives that lifted limits.** The extra web server directives a
  customer may be given can no longer switch the link rule off, list
  folders, raise the site's PHP limits or make the web server read a file
  as the administrator.
- **What Add domain read before checking access.** It no longer looks into
  a subscription named in the request before checking that the caller may
  use it.

## What changes for you

### One answer when a name cannot be used

A customer or reseller whose name is refused reads one sentence, whatever
the reason: the name, or a name above or under it, is already in use on this
server or reserved by it; if it is theirs, they ask the server's
administrator. It never says who holds the name or whether the server
reserved it: otherwise the refusal would tell anyone which domains are
hosted here. Their own names are told as theirs. The administrator is told
the exact reason and the other name.

The same holds for an extra FTP login name that is already in use, and a
subscription transfer whose names the receiving account may not hold is
refused with the names listed to the administrator.

### Names that already overlap

An update changes nothing that exists. If a customer already holds a name
inside or above another account's name, or one of the server's names, or a
Node.js site already uses a port of the server's services, the
administrator sees it in **Problems now** (Dashboard and Monitoring), with
what to do:

- a name of another account: remove it, or transfer the subscription that
  holds it to the account that holds the other name;
- a name of the server: unless that account really may hold it, remove it;
  only an administrator's own subscription holds the server's names;
- a Node.js site on a server service's port: save its Node.js settings
  again; it gets a free port, and its program must listen on that port.

Each line goes away by itself once it is solved. The panel also writes them
to its log at start.

### Links to another account's files

A link in a site's folder to the site's own file keeps working. A link to a
file of another account or of the server is no longer served; the visitor
gets "not found" or "forbidden". The site's **Logs** screen explains such a
line in one sentence: put a copy of the file in the site instead of a link.
If the owner set the link rule for a site himself, CelikPanel leaves his
line in place.

### Files corrected once

Once, at the first start after the update, CelikPanel's own unchanged
per-site files that were readable by everyone become readable only by
their services (0640), and the hosting folder and its subscriptions folder
become passable but not listable (0711). A mode the owner changed before is
kept and named in the agent's log; a mode the owner sets later is never
changed back.

### Node.js ports

A Node.js site gets a free port from 3001 to 3999. A port that another site
or one of the server's services uses is refused with one sentence, whatever
uses it; leaving the port empty lets the panel choose. A site's existing
port is kept as long as it is not a port of the server's services.

## Known limits

- The web server's group can still read every site's folder; the link rule
  is the barrier between sites until each site gets its own group.
- Every site's account is listed in the system's account list, and its name
  is made from the domain name; site accounts named by number come later.
- Mail's address maps are still readable by every account.
- Mail is still signed for any hosted domain whose name is in the "From"
  line; signing only for the sender's own domain is the next release.
- PHP and scheduled tasks still run outside the per-site sandbox and its
  limits.
- Running processes and their command lines are visible to every account.
- A site deployed from a Git repository may clone from an address on the
  server itself (it runs in the site's own sandbox, which can already reach
  those addresses).
- Testing a remote backup destination can reach addresses inside the
  server's network.
- The name and port checks run inside the panel's database and the panel;
  a name or port set by hand on the server is not checked.

## Checked in the lab

OPEN.

## Evidence

Schema 079 → 080: six database rules (on new domains, renamed or moved
domains, new or changed aliases, and a subscription's new owner); no table,
column or row. A database holding overlaps made before upgrades and keeps
working (`TestSchema79DatabaseWithOverlapsUpgradesAndKeepsWorking`). New, all
additive: the refusal codes `HOSTNAME_NOT_AVAILABLE`, `APP_PORT_NOT_ALLOWED`
and `TRANSFER_NAME_OVERLAP`, the agent's answer `site_identity_exists`, the
site part outcome `refused`, three kinds in Problems now, one agent receipt
file.

Tests that perform each closed way and fail on alpha.141's code, passing
on this release's: `TestNameRuleProofAnotherAccountsNameIsRefused`,
`TestNameRuleProofTheServersNamesAreRefused`,
`TestNameRuleProofTheInfrastructureZoneIsNotAdopted`,
`TestNameRuleProofASharedSystemAccountIsRefusedBeforeCreation`,
`TestNameRuleProofARefusedCreationIsNotCompensatedByAccountName`,
`TestNameRuleProofANodePortOfAServerServiceIsRefused`,
`TestNameRuleParentLookupWaitsForTheAccessCheck` (panel);
`TestDeleteSiteLeavesAnotherSitesAccountCrontabAndLoginsAlone`,
`TestNodeVhostRefusesAPortOfTheServersOwnServices`,
`TestTenantIsolationFTPLoginNameTakenSaysNothingOfWhoHasIt`,
`TestTenantIsolationHostingBaseIsNotListable` (agent);
`TestTenantIsolationEveryBlockThatServesFilesRefusesForeignLinks`,
`TestTenantIsolationCustomerDirectivesCannotLiftTheRules`,
`TestTenantIsolationNewSiteFilesAreNotWorldReadable` (services). The link
rule was run against Debian 13's own nginx 1.26.3: another account's file
through a link refused, the site's own file through a link served.

Tests, each package's full suite in the test machine, before (alpha.141)
and after:

| Package | Before | After |
|---|---|---|
| `internal/transport` | 62 | 62 |
| `internal/panellisten` | 4 | 4 |
| `internal/sitesandbox` | 18 | 19 |
| `internal/db` | 180 | 182 |
| `internal/services` | 364 | 371 |
| `cmd/agent` | 5937 | 5948 |
| `cmd/panel` | 4713 | 4726 |

(Passed tests and subtests.)

No new failure. Web: `npm test` 3079 → 3090 passed, 0 failed; the type check is clean and the build meets every size budget (first-load path 109.37 KiB of 110.00 KiB compressed; alpha.141: 109.36).
