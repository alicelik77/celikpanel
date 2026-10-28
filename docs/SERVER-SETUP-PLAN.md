# Guided server setup

*Approved product direction: September 10, 2026 · [Türkçe](SERVER-SETUP-PLAN.tr.md)*

**Status: approved source implementation and local validation completed; not released.**
See [implementation status and evidence](SERVER-SETUP-STATUS.md) for the current
boundary. The operator approved this direction after reviewing the activation,
Dashboard and Components screens.
This document records that agreement and the implementation acceptance criteria.
It does not authorize an assistant to update any installed panel; the user starts
every installed-panel update from CelikPanel itself, as required by [AGENTS.md](../AGENTS.md).

## October 29 follow-up: setup wizard v2, phase 5 (v0.1.0-alpha.115 source)

- **The DNS page reads one picture.** `GET /api/v1/dns/overview`
  (administrator only, read only, no schema change) answers the whole DNS
  picture at once: one D-024 sentence (state `ok`, `waiting`, `prerequisite`,
  `failed` or `unknown`, a stable code, why, who acts, the next action and
  what happens then), this server and the bound second server (role main,
  backup, standalone or none; short name, panel name, DNS software; whether it
  answers for the main domain when asked directly: answers, answers but not
  for the domain, could not be asked, not joined), the names with the
  expected value and what the public resolvers answer (panel and mail names,
  in-zone nameserver names, the registry's nameservers and IP records (glue)
  from the registrar check), the main domain's MX, SPF, DKIM (selector
  `celik`) and DMARC with who receives the mail, where the owner's own records
  are edited, and whether a second server can join here. With the list at a
  provider no server claims DNS software and no registrar check runs. A failed
  lookup is unknown, never missing; a verified failure comes first, then what
  someone must do, then what could not be checked, then a change that is
  spreading. One bounded answer per panel is shared for a minute (15 seconds
  when part of it is unknown), computed apart from the caller; polling never
  changes anything. Link credentials, enrollment and join codes never appear.
- **The page.** Server → DNS opens with that sentence, then the servers (named
  "This server (name)" and "Second server (name)", never a guessed name) or
  where the list is kept, the "Join a server" entry, the name table with a
  per-row internet check, the mail records and "Records you added" with a
  link to the existing record editor. Today's DNS settings and engine card are
  kept whole under Advanced, collapsed and mounted only when opened.
- **Joining from the DNS page.** While setup waits for the second server the
  entry is the existing join card. Otherwise it says why there is no join
  here: setup not finished; one server on BIND (a running BIND cannot gain a
  backup DNS server in this release); PowerDNS pairs under Advanced; the list
  at a provider or another CelikPanel (the second server runs its own setup);
  this server is the backup.
- **Move mail.** When the owner kept the old mail server (MX "later") and this
  panel keeps the list, "Move mail to …" changes the one apex MX record
  through the existing record editor after an in-page confirmation that says
  what changes (new mail arrives on this server; create the mailboxes first;
  mail on the old server stays). Several MX records are left to the editor;
  a lost answer is never sent again and the page reads the state again.
- **The first site.** A finished setup with websites offers "Add your first
  site": the existing Add domain form opens with the main domain filled in and
  creates nothing until the owner submits it; "Later" hides the offer.

Not in this phase: "Move main DNS" (alpha.116–.118); the existing-installations
mapping was dropped by the owner's rule of 2026-10-26 (nothing real is
installed). Remaining limits: the other server's DKIM key is checked only as
published (its value is that server's); reverse DNS stays an open task of
setup, not a row of this page; the record editor itself is unchanged. Measured
only by component tests on loopback servers; not on installed servers.

## October 28 follow-up: setup wizard v2, phase 4 (v0.1.0-alpha.114 source)

- **One server, its own DNS.** One server keeping its own DNS is standalone:
  both nameserver names point to it, there is no backup DNS server, and the
  wizard does not promise adding one later for BIND (a running BIND cannot
  move from standalone to paired). Readiness needs both names published to
  this server; the registrar card asks this server once.
- **The second server joins by code at the start.** With two servers and the
  owner's own DNS the first server's plan runs its components (services,
  runtimes, mail profiles) and then waits at a "the second server joins" step;
  the second server's address is never typed. Its administrator creates a
  join code there: 24 characters, the first 16 carrying 80 bits of the first
  panel's TLS key fingerprint (SHA-256 of the SubjectPublicKeyInfo), the last
  8 a 40-bit single-use secret, valid 24 hours and 5 wrong tries, kept only as
  a salted hash, audited without the secret, shown once. On the second server
  the first step offers "Join the first server with a code": the address and
  the code. The second server checks the fingerprint in the TLS handshake
  before anything is written, so a wrong address never receives the secret.
  The first server checks the secret in constant time, the caller's source
  address against the address it declares, a rate limit (20 a minute, 5 from
  one address), and consumes the code once. Answers: joined; refused with the
  reason (wrong code with the tries left, used, expired, exhausted, not
  waiting, source mismatch); wrong server (another key; nothing sent);
  unreachable (nothing sent; the code unused); unknown (the answer was lost:
  the request is kept and the next try is the same request, which the first
  server answers with the same join and fresh secrets, never a second join).
- **The join document.** The second server's shared answers come from the
  first: main domain, two servers, its own DNS, which server is the main DNS
  server, ns1 (always the main) and ns2, the first server's address, short
  name and names, and either the first server's publishing endpoint with a
  one-time enrollment code (first server is the main) or the list the first
  server prepared (second server is the main). The second server's review
  refuses a draft whose shared answers differ ("join again to change them").
- **Names over the link, bound once.** The second server's plan starts by
  sending its names (panel, mail, its main-domain mail choice when it
  receives the main domain's mail, its address and key fingerprint) over the
  link; the first server checks them against its own names and records at
  once (a clash is refused before anything is written) and wakes. Its join
  step then binds the second server to that exact execution, once and
  immutably (INSERT OR IGNORE, like the DNS publisher binding), computing the
  zone records against the zone at binding time; every later step runs on the
  plan completed by the binding, the reviewed plan itself never changes. A
  runner restarted mid-wait binds the same way; names that change after the
  binding are refused with the reason.
- **Choosing the main DNS server (Option A).** When the second server is the
  main, the first prepares the list (records, mail choice) and installs no DNS
  software until the second joined; the list goes with the join; the second
  keeps the zone with the first server's names, and the first installs as
  backup with an enrollment code the second issued with its names.
- **Publishing without the administrator.** A joined backup DNS server makes
  its publishing connection by itself with the handed-over enrollment code
  (the join was the main administrator's consent, Q5); until the main DNS
  server accepts (its pair ready and nameservers public) the step waits with
  that reason. The connection itself keeps the existing rules (HTTPS name,
  public addresses, a certificate from a public authority).
- **Mail of the other server.** When the backup DNS server receives the main
  domain's mail (the main has none), its MX, SPF and DMARC are written on the
  main from its answers and its DKIM record goes over the link; "add them by
  hand" is gone. When both servers have mail the main DNS server receives the
  main domain's mail and the other server's mail serves its own domains.
- **The link.** The link dials the peer's IPv4 address and trusts only its
  key fingerprint (the full SHA-256 SPKI after the join), never a
  certificate name. The panel keeps serving its first self-signed identity on
  its IP address across domain certificates, so the pin holds during setup.
  The first server keeps only keyed hashes (HMAC under the panel's key file,
  outside the database) of the code's secret and of the second server's
  credential; the second server derives the credential it accepts and keeps
  the typed code, sealed under the key file, only while a join's outcome is
  unknown (it is deleted on every known outcome). The link credential is
  removed on both servers when their setup completes. Nothing secret leaves
  through an administrator API, a log or the audit.

Remaining limits: the IPv4 address is the only address checked for the
source; the backup's own key fingerprint is sent once with its names; a
second server that lost its link record joins again with a new code.
Measured only by component tests on loopback servers; not on installed
servers.

## October 28 follow-up: setup wizard v2, phase 3 (v0.1.0-alpha.113 source)

The registrar card checks itself. `GET /api/v1/setup/delegation?zone=` (the
main domain on the owner's own DNS servers only; admin-only, read-only, one
bounded answer shared for a minute) finds the parent zone by its SOA (so
`.com.tr`-style suffixes need no list), asks the parent's own servers without
recursion which nameservers and IP records (glue) they hand out, asks each of
our servers whether it answers the zone authoritatively (with its SOA serial)
and compares the imported records with what our servers answer. Each item is
correct, not yet, waiting (the backup DNS server before it is set up) or
could not be checked; an unread answer is never "not yet". With two servers
the card opens fully once the backup answers or setup waits for the panel's
name; "Do it now anyway" is refused with the reason until the main DNS server
answers, and warns that `.tr` registries refuse a change before both answer.
Glue is asked only for nameservers inside the domain; IPv6 glue is optional.
Not measured on an installed server.

Review corrections: the comparison uses the records as the zone publishes
them (imported records plus the mail records: the merged SPF, the MX and
DMARC as the owner chose), not the raw imports; a value the owner added
beside ours (a backup MX) is allowed. Only an authoritative NOERROR or
NXDOMAIN answer lacking a published value is "not yet"; no reply, a reply
that is not authoritative, or REFUSED/SERVFAIL is "could not be checked".
The parent walk and our servers' answers run side by side in their own time
budgets. A lame parent server (a non-authoritative NXDOMAIN, an answer
without the domain's nameservers) is skipped for the next one; parent
servers are reached over IPv4 only, so a parent with only IPv6 servers is
"could not be checked". An answer with an unknown item is not shared, a
second look waits for the check in flight (or gives up with its own
request), and a card where everything is correct stops asking until the
step is opened again.

## October 28 follow-up: setup wizard v2, phase 2 (v0.1.0-alpha.112 source)

- **Existing records.** On the main DNS server a step after the DNS step reads
  what the public internet answers now for the main domain's usual names (the
  domain itself, www, mail, webmail, autodiscover, _dmarc) and any name the
  owner types (`GET /api/v1/setup/public-records`: two fixed public resolvers,
  read-only, shared for a minute; an unread name is unknown). The internet
  gives no complete list, and the screen says so. Each record is imported or
  skipped; a skipped one stops working once the nameserver change spreads. A
  name the setup gives to a server is never taken over; when the old mail
  server holds `mail.<domain>`, one button gives this server `mail2`.
- **Mail records.** With email, MX, SPF and DMARC for the main domain are
  written into the zone, DKIM by a `mail_dns` step after the mail profile
  (key, signing, TXT record; idempotent). MX moves only when no old MX was
  imported or the owner chose "now"; SPF is merged (this server is added,
  nothing removed); an old DMARC is kept or replaced as chosen. Imported and
  mail records are the owner's records in the zone (editable in Domains);
  only the setup's own server addresses stay locked.
  Review correction: records are compared in the zone renderer's own form.
  The setup keeps a list of the imported and mail records it wrote
  (`panel_settings`, no schema change); on a revised plan a once-per-name
  record it wrote (the merged SPF after a mail host change, DMARC, the old
  imported MX when mail moves now) is replaced by its exact stored row,
  never refused. A different SPF or DMARC the setup did not write (or one
  the owner changed in Domains) refuses the plan; another MX the owner added
  (a backup MX) stays beside ours, since a name may have several. Records
  the setup wrote that a revision no longer wants (a skipped import, our MX
  after "now" becomes "later") are left for the owner in Domains.
- **External provider.** The provider list adds MX, the merged SPF, DMARC and
  DKIM rows with their public state ("kept" for an old MX the owner keeps,
  "pending" until the DKIM key exists).
- **Reverse DNS.** The mail step shows the PTR rows for IPv4 and IPv6 to give
  the server provider. Reverse DNS for IPv4 and, with public IPv6, for IPv6
  is an open task setup does not wait for (owner decision 2026-10-28): mail
  works without it but may be marked as spam; it stays listed in the mail
  step, on the install screen and after completion until the internet shows
  the right name (an unanswered lookup is unknown).
- **Helpers.** SpamAssassin and ClamAV are wizard choices again: the scanner
  now sees their helper units (spamass-milter, clamav-freshclam), which never
  stand for the service itself.

No schema change. Not measured on an installed server.

## October 26 follow-up: setup wizard v2, phase 1 (v0.1.0-alpha.111 source)

The owner approved a redesigned first-run wizard (main domain first, roles,
names proposed from the main domain, an explicit DNS home). Phase 1 ships it
on today's DNS backend; joining the second server by code, choosing the
second server as main, importing existing records, mail records, the live
registrar check and moving the main DNS server come in later releases.

- **Steps.** Main domain (asked once, first) → number of servers (two
  recommended) → the server's short name (prefilled only when the host name
  reads like a name a person chose; never invented) → roles (Websites, Email,
  DNS; combinable; each lists what it installs) → components (optional; the
  same catalogue as the Components page, with installed versions, the
  distribution version recommended, vendor versions only from an enabled
  repository and a reason for every component the wizard does not install) →
  names proposed from the main domain and short name (an owner edit is kept)
  → where the DNS list lives → summary → install.
- **DNS home.** *On my own servers*: this server is the main DNS server (the
  only main-server choice in this release) or the backup DNS server of a main
  server already set up; BIND or PowerDNS with the review's own gate reasons.
  The main domain is the zone; it is no longer an unticked option. With two
  servers the second server's nameserver name and IPv4 are still typed, and
  the screen says so. One server cannot keep its own DNS yet (refused with
  `server_setup_local_dns_requires_two_servers`). *At an external provider*:
  no DNS software, port 53 is not opened; after the start the records to add
  are shown with their live public state (`GET /api/v1/setup/provider-records`,
  read-only, about one check a minute, a failed lookup is unknown). *On
  another CelikPanel*: today's connection.
- **Install screen.** The panel is reachable at `https://IP:2083` with a
  temporary certificate until its name resolves, said plainly. The registrar
  card lists the nameservers and, for names inside the main domain, their IP
  records (glue), with today's timing: after the backup DNS server is ready.
- **Backend.** Additive draft fields `main_domain`, `server_count`,
  `short_name`, `main_dns_server` and `customization.versions` (a chosen
  vendor package becomes the service step's package; the install preflight
  still requires its repository). `GET /api/v1/setup/components` lists every
  catalogue component with what the wizard does with it and the per-role DNS
  software offer. Services whose helper unit setup's final check cannot see
  (SpamAssassin, ClamAV) and Netdata (vendor repository required) install
  from Components after setup. No schema change.

Not measured on an installed server; the Beta 1 reinstall is the first real
run. Installed panels are still updated only by their owners.

## September 11 follow-up: explicit guidance preference (Alpha66 release source)

After updating Alpha65, the operator could not find the setup entry or decline
the wizard. On first use, an administrator with a fresh or unassessed legacy
state now chooses **Set up with the wizard** or **I'll configure it myself**.
This is a short choice, not a claim that an upgraded host is empty. Completed
installations and existing in-progress operations retain their state.

The choice persists for the server across sessions. Manual mode suppresses the
automatic wizard and Dashboard invitation; **Server → Server setup** (Settings → Server setup until
v0.1.0-alpha.100) remains
available to reopen it. Switching the preference does not complete setup, alter
its draft, remove readiness checks, change licensing, or run host operations.
An accepted active/waiting execution must be resolved before changing preference.
See [implementation and checks](SERVER-SETUP-GUIDANCE.md).

## September 11 follow-up: custom component selection (Alpha67 release source)

The operator approved customizing a purpose profile and a fifth **Custom setup**
choice inside the wizard. A dedicated Components step resolves dependencies,
explains conflicts and preserves installed software. Access, DNS, review and
verification remain in the same guided flow. Components remains the place for
later service administration. See the [customization contract and validation
boundary](SERVER-SETUP-CUSTOMIZATION.md).

## September 13 implementation note: access DNS inside setup

The working-tree correction prepares explicitly reviewed infrastructure DNS
records inside the wizard before public hostname verification and certificate
issuance. It does not require leaving setup to create a tenant domain. Selecting
local DNS does not authorize taking over an inferred parent zone: the owner
chooses the exact zone and reviews its minimal record additions. Existing DNS
ownership and records are preserved; a secondary receives native zone transfers
and never becomes a local primary merely because it also hosts applications.

This implements the existing outcome and ownership requirements below; it does
not change their approval scope. Previously accepted plans retain their exact
steps and identities; new infrastructure changes require a new review. See the
[access DNS implementation note](SERVER-SETUP-ACCESS-DNS.md) for ordering, recovery,
local test evidence and unsupported cases. This correction is not yet released
or verified on the installed Frankfurt/Boston pair.

## 1. Outcome and audience

A server administrator should reach a usable, secured environment without
interpreting a component catalogue or moving between unrelated settings pages.
The first question is the server's purpose. Use the existing CelikPanel visual
language, Turkish and English, responsive layouts and accessible controls.
Resellers, customers and additional users never receive host setup controls.

License activation and server setup are independent states:

| Observed server state | Destination after valid licensing |
|---|---|
| Confirmed fresh installation | Guided setup |
| Persisted incomplete setup | Resume the current step |
| Previously prepared installation | Dashboard |
| Existing installation with no setup record | Preserve access; assess read-only and offer setup without assuming the host is new |

Replacing a license, signing in again, restarting or updating must not reset
setup. An empty domain list alone is not evidence of a fresh installation.
Readiness failures on an established server must not force it through a new
installation or stop existing workloads.

## 2. Purpose profiles

| Choice | Intended outcome |
|---|---|
| Web hosting — recommended | Web and PHP hosting with the required database infrastructure; no mail by default |
| Web and mail hosting | Web hosting plus mailboxes, webmail and mail protection |
| Application server | The selected application's runtime, web access and required dependencies |
| DNS server | New authoritative DNS infrastructure or joining an existing setup as a secondary |

Profiles resolve to compatible offerings already supported on the actual host.
Customization is secondary; users need not select every package or install
unrelated tools. Do not expose an executable profile until its full lifecycle
has been implemented and verified. Profiles configure infrastructure, not a
customer's first website or mailbox. Those follow after infrastructure is ready.

## 3. DNS policy

Hosting setup must choose a DNS management model. A local authoritative DNS
engine is not universally required:

1. **Manage DNS here:** configure a supported local engine, nameserver identity
   and the required topology. Production readiness for this model includes
   independently hosted authoritative redundancy; two names on one host do not
   satisfy it.
2. **Use an existing CelikPanel DNS infrastructure:** establish authorized
   publishing/consuming relationships and verify the remote authority. A peer IP
   alone grants no management permission. A web host using remote DNS need not
   become a DNS secondary or install an authoritative engine.
3. **Use an external DNS provider:** show the exact records to create and verify
   them. Manual management is supported by the design; provider API credentials
   are optional automation, not a prerequisite.

Choose the infrastructure/default model during setup; verify each domain's
necessary records when that domain is published. An operating-system resolver
is not evidence of authoritative DNS readiness. A secondary serves transferred
DNS zones; it does not back up websites and is not automatically a publisher for
new locally created domains.

This approved direction supersedes D-009's blanket local-DNS product requirement.
The existing engine ownership, publication, topology, DNSSEC and recovery
invariants remain binding for local authority. Implement an explicit external
mode before relaxing guards; removing a frontend check alone is not this feature.
Existing domains retain their DNS ownership until an explicit supported migration.

## 4. Secure access and mandatory checks

The initial implementation uses a public panel FQDN, for example
`panel.example.com`, with a trusted certificate and verified automatic renewal.
A new domain purchase is not required. The starting self-signed certificate is
an entry/recovery mechanism, not evidence that public panel setup is complete.
The panel address, operating-system hostname, nameserver names and mail identity
are distinct; collecting one must not silently rewrite the others.

IP certificates are a possible later path, not a current CelikPanel capability.
Private-only deployments and their trust model require a separate explicit design;
they must not become an unchecked bypass of the public hosting profile.

| Requirement | Enforcement point |
|---|---|
| Trusted panel HTTPS and working renewal | Before marking initial public setup complete |
| Firewall policy preserving panel/SSH access and allowing required services | Before marking initial setup complete |
| Installed, healthy and compatible dependencies | Before enabling the corresponding feature |
| Correct domain DNS and website certificate | Before reporting the corresponding HTTPS site as published |
| Mail identity, TLS, necessary DNS/authentication records and delivery prerequisites | Before reporting the corresponding mail service/domain as ready |

Mail preparation includes SPF, DKIM and DMARC; reverse DNS and connectivity must
be checked where relevant. Separate host readiness from per-domain readiness.
An external PTR or registrar change remains an explicit operator task when the
panel has no authorized integration to perform it.

Checks are enforced on the server, not only through disabled buttons. Missing
mail prerequisites do not block web hosting. Recovery/configuration access stays
available when a check fails. No check automatically stops existing websites,
mail, databases, scheduled work or certificate renewal.

## 5. User flow and operation contract

**Purpose → Panel address and DNS method → Review plan → Start setup → Verify result**

- Read current state automatically using bounded read-only checks. Never install
  software, rewrite settings or start a scan with configuration side effects just
  because the page opened.
- Present one primary action per step. Show only relevant inputs. Keep advanced
  details available without turning the first screen into Components.
- Before mutation, show the concrete components, configuration changes, access
  rules and outstanding external tasks. The user starts the reviewed plan.
- Use durable, resumable operations with exact plan identity and dependency order.
  Page refresh, reconnect or agent/panel restart must neither lose the operation
  nor execute it twice. Conflicting operations must not run concurrently.
  A lost HTTP response does not prove failure; reconcile the saved operation
  before retrying. Unmanaged configurations and existing customer data are never
  overwritten or deleted by automatic recovery.
- Reuse existing audited service/DNS/certificate operations. Setup does not gain a
  new arbitrary-command path to the privileged agent.
- Show honest stages, results and actionable failures. External DNS propagation
  is a waiting state, not a guessed success or a percentage countdown.
- Changes to material inputs invalidate the previous review. Recovery must
  distinguish completed, not-started and uncertain steps; do not promise that a
  package installation can always be fully undone.
- Report completion only from fresh evidence. End with one purpose-specific next
  action: add a first site, deploy an application or connect/manage DNS.

Afterward the Dashboard summarizes the selected role, existing resources and
actionable issues. It must not keep showing a fresh-server checklist once setup
has completed. Components remains available for later administration.

## 6. Implementation order

1. **Persisted state and routing:** fresh-install provenance, conservative upgrade
   behavior, admin-only setup APIs, resumable drafts and independent license gating.
2. **Readiness and DNS models:** evidence schema; local/existing/external ownership;
   replace D-009 gates consistently across domain, capability and mail paths.
3. **Secure-access workflow:** panel address, trusted TLS/renewal and safe firewall
   changes using existing operation machinery.
4. **Purpose plans and execution:** supported offering resolution, exact plan
   review, durable orchestration, recovery and purpose-specific verification.
5. **Guided UI and Dashboard:** bilingual steps, progress, external actions,
   resume/error states and a single useful completion action.
6. **Validation and publication:** meet the acceptance cases below, document the
   supported paths, then publish a reviewed release. Users update their panels.

These are implementation dependencies, not permission to ship a wizard that
claims unsupported profiles or treats incomplete checks as ready.

## 7. Acceptance cases

- Fresh, resumed, established and ambiguous legacy installations route correctly;
  reactivation never causes reinstallation or resets an existing setup.
- All non-admin roles are denied host setup APIs; licensing remains independently
  enforced and license loss does not stop already running workloads.
- Every offered purpose completes on its supported host capabilities, including
  required dependency and mutually exclusive service handling.
- Local DNS, verified existing DNS and manually managed external DNS each have a
  complete working path. External DNS does not trigger a local DNS installation.
- Primary/secondary responsibilities, authorization, redundancy and publication
  are tested; a configured but unreachable or stale peer is not marked ready.
- Invalid/mismatched/expired panel certificates, failed renewal, stale evidence,
  incorrect DNS and missing mail prerequisites cannot produce false completion.
- Firewall application preserves current management access. Applicable real-VM
  reboot and recovery gates in [OPERATIONS.md](OPERATIONS.md) must pass.
- Refresh, repeated clicks, logout, concurrent sessions, restart and interruption
  cannot duplicate an installation or discard a committed operation.
- Existing workloads/configuration survive setup assessment and unrelated
  failures. A changed profile never silently uninstalls or resets live services.
- Turkish/English, desktop/mobile, keyboard navigation and actionable errors are
  verified against the actual UI. No success requires a hidden extra button.

- Every setup operation follows [D-024 actionable guidance](OPERATION-GUIDANCE.md):
  show the current reason, actor, next action and continuation behavior before
  lengthy progress lists; distinguish prerequisites, observed failures and unknown
  results. Cover both DNS start orders and the chosen DNS mode, relevant license,
  credential and provider failures, preserved failures during reconciliation,
  secret-free diagnostics and refresh without duplicate mutations.
## 8. Baseline at approval

At approval, Dashboard contained `StartGuide`; there was no persisted general
purpose wizard. Mail profiles provide an existing review/execution foundation.
Domain creation currently requires local DNS under D-009; detection of an external
nameserver is not an external-provider management mode. Panel certificate input
uses `CanonicalFQDN` and rejects IP addresses.

This historical baseline does not describe the current working tree; see
[implementation status](SERVER-SETUP-STATUS.md).

Relevant sources: `web/src/components/StartGuide.tsx`,
`web/src/components/AddDomainModal.tsx`, `cmd/panel/mail_profiles.go`,
`cmd/panel/dns_engine.go`, `cmd/panel/domain_connection.go`,
`cmd/panel/panel_cert_handler.go`, `internal/hostname/hostname.go`.

## Note, 2026-09-30: mail on Arch and component install failures (upd1 P2)

The `web_mail` purpose (and any plan with mail components) is refused on Arch
Linux at review with `server_setup_service_unsupported:dovecot`: Arch's Dovecot
2.4 package keeps its mail, login and TLS settings in one `dovecot.conf` that the
panel does not manage, so an admitted plan used to stop mid-setup at the mail
step. A component install failure now names the component, the failed step and
the host's own line, and the wizard states who acts and how setup continues.
Details and remaining work: [operation guidance](OPERATION-GUIDANCE.md). Component
tests only; native evidence is pending.
