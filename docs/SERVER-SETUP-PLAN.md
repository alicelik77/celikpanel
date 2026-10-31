# Guided server setup

*Approved product direction: September 10, 2026 · [Türkçe](SERVER-SETUP-PLAN.tr.md)*

**Status: approved source implementation and local validation completed; not released.**
See [implementation status and evidence](SERVER-SETUP-STATUS.md) for the current
boundary. The operator approved this direction after reviewing the activation,
Dashboard and Components screens.
This document records that agreement and the implementation acceptance criteria.
It does not authorize an assistant to update any installed panel; the user starts
every installed-panel update from CelikPanel itself, as required by [AGENTS.md](../AGENTS.md).

## October 31 follow-up: mail on the server that joined by code (alpha.114 source)

Found live by the owner on alpha.113: Boston, joined to Frankfurt by code,
could not have mail at all, because Frankfurt already receives the main
domain's mail and review refused `server_setup_peer_mail_both`. A hosting
company's servers each host their own customers' web, databases and mail.

- **A joined server may run mail for the domains created on it.** The join's
  shared answer still names the one server that receives the main domain's
  mail. When that is the first server, the joined server's mail never claims
  it: no MX, SPF, DMARC or DKIM of the main domain is written or sent from
  there (on a backup DNS server no `mail_dns` step; on a joined main DNS
  server the zone's MX/SPF/DMARC keep naming the first server's mail name and
  its DKIM is still accepted over the link). The joined server publishes only
  its own mail host name (an A record to its address): through its names to
  the first server during its first setup, and through its access DNS step
  over the authorized DNS connection (`host-record`) otherwise. It gets its
  own mail certificate, Postfix/Dovecot/Rspamd and Roundcube. Customer
  domains created on it get their MX, SPF, DKIM and DMARC in their own zones,
  published to the main DNS server like every hosted zone.
- **Adding mail later.** A completed joined setup (for example web and DNS
  only, installed on alpha.113 and updated by the owner) is reopened in
  Server -> Server setup and mail is added. The extension's names step sends
  nothing (the link ended when that setup completed, and the first server
  already has the names); a first setup over an ended link keeps its
  `server_setup_peer_link_revoked` guidance.
- **Names.** On a joined server the mail name is proposed as
  `mail.<this server's panel name>`. A mail name that the first server, the
  main domain or a nameserver already uses is refused at review with
  `server_setup_peer_mail_hostname_taken` (you, the owner: go back to This
  server's names and give this server its own mail name; nothing was
  changed). The panel name itself may be the mail name: the panel listens on
  its own port with its own certificate lineage, the mail certificate is a
  separate lineage issued through the same port-80 check, and webmail is
  nginx on 443; one A record serves both names (the first server is then
  sent only the panel name).
- **Texts.** On a joined server the Names step and the review say that the
  main domain's records stay on the first server and that this setup does
  not change them, instead of asking to import them.

Remaining: proven only by component tests; the two-server lab run (first
server with mail, second joined with websites and DNS, update, mail added
later, mail both ways for a customer domain) is owed.

## October 29 follow-up: fixes from the third targeted Beta-1 re-run (alpha.115 source)

The third targeted re-run of product commit cfcadd3f1 found the defects
below. Each fix has a component test and none changes the schema; a further
targeted VM re-run is owed before any is called proven.

- **A booting host is a wait, for every step.** After a reboot the runner
  resumed the DNS step while systemd still reported "starting"; the Agent's
  host identity could not be read and the step failed with the generic
  `server_setup_dns_failed`. One classifier now treats an Agent call refused
  before dispatch because the host identity is unavailable while systemd is
  starting (or the Agent does not answer yet) as `server_setup_host_starting`:
  nothing was sent, nobody acts, and the runner asks the same step again
  without a new mutation. A setup child operation is created only after the
  host identity is read. A host still starting after 10 minutes fails with
  `server_setup_host_not_started` (systemd still starting; the commands to
  check) or `server_setup_agent_not_answering` (the Agent never answered;
  `sudo systemctl status celikpanel-agent`). Each boot is its own episode: any
  other outcome ends the previous one.
- **Another change on the server holds the start, and a run waits.** A start
  while a Components install ran created a run whose first component the
  Agent refused at admission (`HOST_MUTATION_BUSY`), and the run failed. The
  start is now refused before a run exists with `server_setup_host_busy`
  (another change is running; wait until it finishes, then start). A run that
  meets it waits with the same code: nothing is created while another
  operation runs. A child is admitted again only with proof that nothing ran
  (it failed at its first lease because the host was busy, and the Agent's
  ledger has no job for it): the same child and operation row, with the
  attempts kept on the step and no audit row per attempt, for at most 30
  minutes; then `server_setup_host_stayed_busy` names what kept the server
  busy. A resumed child or a mail profile whose later mail TLS sub-step was
  refused (its own code, `mail_tls_sync_host_busy`) is never admitted again.
- **A clock behind the certificate authority is said, for every certificate.**
  With the clock 200 s behind, the panel certificate step said only
  `panel_certificate_issue_failed`, and the mail certificate (which had no
  wait at all) the generic `server_setup_step_failed`. One shared Agent
  function now waits, at most two minutes, for any certificate the setup
  issues (panel and mail host) to become valid by this clock, and a larger
  skew is coded with its seconds through the operation into the step:
  `server_setup_certificate_clock_skew` (the clock is N seconds behind; you,
  the owner: turn on time sync, `timedatectl set-ntp true`, or correct the
  clock; then review a revised plan to issue it again).
- **The finished main still takes the backup's DKIM over a live link.** In
  Option A the main finished first; the backup's DKIM publish was then refused
  with `server_setup_peer_dkim_not_running` for ever. While the main's link is
  live (it lasts until the backup reports ready), the main accepts the DKIM
  for the bound join whether its own run is still running or finished, and
  writes and publishes it through the same reviewed DNS path. An ended link
  refuses it.
- **One server receives the main domain's mail.** In Option A with mail
  chosen on both servers, both plans claimed the main domain's mail: the join
  document said the first server (the one that created the code) receives it,
  but the main's draft dropped that answer because it had mail of its own, and
  the backup's overview showed itself as the receiver. The join's shared
  answer is now the single source of truth: it is kept in the joined draft,
  a joined server that would also receive the main domain's mail is refused
  at review with `server_setup_peer_mail_both` (you, the owner: remove mail
  from this server's plan, or remove it on the other server and join again;
  then review again), and the DNS overview names the receiver the shared
  answer names. This release has no mail for a server's own host name only,
  so the second server cannot keep mail beside it.
- **The setup link's texts name servers by role.** In Option A the backup
  DNS server is the server that created the join code, and
  `server_setup_peer_link_revoked` told it to make a new code "on the first
  server". The link's texts in English and Turkish now say "the server that
  created the join code", "the other server" or "the main DNS server", and
  the ended-link message tells each side where the new code is made.
A certificate step whose reply was lost still names a clock behind the authority: the child's job records the clock code and sentence, and both the panel certificate and the mail certificate steps map it again when they read the job back.

## October 29 follow-up: fixes from the targeted Beta-1 re-run (alpha.115 source)

The targeted re-run of product commit b86ab6fa4 found the defects below. Each
fix has a component test and none changes the schema; a further targeted VM
re-run is owed before any is called proven.

- **An unread update list holds the start.** While the review cannot read
  which installed distribution packages the plan's installs would also update
  (`package_updates_state: unknown`, for example a busy package manager),
  setup does not start: the confirm button is disabled with the reason, and
  the start handler refuses with `server_setup_distribution_updates_unread`.
  Refresh the review reads the list again and changes nothing on the server;
  the plan keeps its identity. The start sends the digest of the list its
  page showed; a list replaced by another review in between is refused with
  `server_setup_distribution_updates_changed_before_start` (read the review
  again, then confirm), and a stored list is replaced only while it is
  unread. A component step that waits on
  `server_setup_distribution_updates_changed` (or `_unread`) offers Review the
  updates: a read of the list for the run's remaining component installs, then
  Confirm and continue. The confirmed list is recorded for that run, and the
  same run continues from the waiting step; nothing already done is repeated,
  and no plan is reopened. A running step or a known failure is never changed.
- **A DNS install that placed nothing is a verified failure at once.** When
  the DNS software's first install fails before any package is placed (for
  example APT has no installation candidate for bind9), the Agent now proves
  that rollback too: its exact install receipt, none of the packages it found
  missing installed, no unit of that software running, failed or masked, and
  nothing on public port 53, read twice. The setup step then shows
  `server_setup_dns_rolled_back` on its next pass, with both next actions
  (keep the software and try again, or choose the other one in the revised
  plan), and no panel restart is needed. Before, it stayed at
  `server_setup_dns_agent_failed` until startup recovery released it.
- **The setup link outlives the main DNS server's own setup.** The main
  keeps its side of the link after its setup is ready, until the backup
  reports over the link that its setup is ready (`POST
  /api/v1/setup/peer/ready`, authenticated by the link credential in its
  role's namespace, from the joined address on the first server, naming the
  same join: a report for another binding is refused), or until the owner
  stops waiting for it on the main (Stop waiting for the other server, `POST
  /api/v1/setup/peer/retire`, confirmed). Only then does the main end it. The
  join code still ends with the main's setup. The backup reports once it is
  durably ready and keeps its side of the link until the main was told; until
  then it says so (`peer_setup: reporting`) and waits
  (`server_setup_peer_ready_unreachable`) and reports again by itself. Only an
  acknowledgement or the main's `server_setup_peer_link_ended` counts as told:
  an ended side keeps a keyed hash of the credential it accepted (never the
  value), and the main says the link ended only for exactly that credential
  from the joined address. Any other refusal is the wait
  `server_setup_peer_ready_refused` (you, the owner: on the main open setup
  and, if needed, choose Stop waiting for the other server); another TLS key
  is `server_setup_peer_ready_identity_changed` (create a new join code on the
  main). A report acts only on the record of the namespace its credential
  authenticates in, and a report whose state cannot be read is an error, so
  the backup reports again. So a
  backup that fails after the main finished can retry and finish. Meanwhile
  the main is ready for its own services, and its setup page says the second
  server has not finished (`peer_setup: pending`; you, on the second server:
  open setup there; then both are ready).
- **A just-issued panel certificate is waited for, not failed.** The run's
  `panel_certificate_issue_failed` after a successful issue was not name
  resolution: the ACME server stamped notBefore with its issue time and the
  backup's clock was about 1.5 s behind, so the agent's read right after
  certbot found the leaf "not yet valid". Issuance now waits, at most two
  minutes and reading only, until the leaf is valid by this server's clock,
  then reads and checks it as before. A larger skew still fails, and its
  reason now names the clock (`timedatectl`).
- **A rejoined backup goes through its publisher step.** When the backup DNS
  server is the first server (the main joined it), its plan is reviewed
  before the main joins, so its publishing step names no endpoint yet; the
  binding supplies it. The step's gate compared the empty reviewed endpoint
  with the bound one and failed every such run with the generic
  `server_setup_step_failed` (in the run, right after the P4 rejoin). The
  gate now takes the bound endpoint for that step, so a run that adopted the
  replacing join's binding continues. If a join were replaced under a run
  that had already used its binding, the run stops with
  `server_setup_plan_changed_by_rejoin` (you, the owner: Review a revised
  plan; setup then continues with the new join; completed host changes are
  kept), never the generic failure.
- **The setup's own DNS software is not "existing DNS".** Reviewing a new
  plan after a run (for example the P4 rejoin) was refused with
  `server_setup_existing_dns_requires_migration` because of the PowerDNS that
  the earlier run had installed. Now an active engine is this setup's own
  when a run of this panel recorded it: a committed first install (no source
  engine) whose request is the DNS step of a setup run on this panel that
  succeeded, at the engine's current epoch (no later switch), with no switch
  or marker pending. Such an engine is never "existing DNS that needs
  migration": when it matches the plan the run's DNS step adopts it without
  installing anything; when it does not, the review says so
  (`server_setup_dns_installed_differs`: choose that engine in the plan, or
  change it in Server → DNS); a second server bound after the review is
  compared at the DNS step, with the same code. An engine without that record
  still needs the
  migration as before.
- **A vendor PowerDNS build says where it comes from.** The origin is checked
  before the version: a build from repo.powerdns.com (for example
  4.9.18-1pdns.trixie), backports or another repository is refused with
  `server_setup_pdns_main_not_debian_archive`, as documented, not with
  `server_setup_pdns_main_untested_version`.
- **A changed join is the run's current state.** When the second server's
  answers change while the first server's run waits at a step that has
  started nothing yet (for example primary_dns before its DNS install), the
  run itself now says `server_setup_peer_join_changed` at the top, not only
  the peer card. A step with an operation in progress or a known failure
  keeps its own state.

## October 29 follow-up: fixes from the Beta-1 VM run (alpha.115 source)

A native run of setup wizard v2 on fresh Debian 13 and Ubuntu 24.04 guests
found the defects below; each fix has a component test, none changes the
schema, and a targeted VM re-run is still owed before any is called proven.

- **Distribution point updates are listed, not refused.** A component's
  install that also updates already installed packages within the same
  release no longer stops a fresh setup. A point update needs all three: the
  same epoch and upstream version, a newer version in dpkg's order, and every
  archive APT names for it being the distribution's own released suite or its
  security/updates pocket (for example perl-base 5.40.1-6 →
  5.40.1-6+deb13u1 from Debian-Security). The review reads the install
  simulation once and lists them under "Distribution packages also updated"
  ("Ayrıca güncellenecek dağıtım paketleri"); starting setup confirms exactly
  those (package=version), and only setup's own component steps send them to
  the Agent, which accepts only exact matches. An unread review cannot start.
  Before each component's first install the step asks again: an update the
  review did not list waits as `server_setup_distribution_updates_changed`
  (the owner reviews and confirms it from that wait), an unread answer waits as
  `server_setup_distribution_updates_unread`; nothing is installed. A PPA, a
  vendor or third-party repository, backports, a downgrade, a new upstream
  version, a database server, Apache, another PHP version or another
  component keeps today's confirmation.
- **One server with its own DNS finishes its zone.** The plan and every zone
  sync decide the zone kind with one rule (`NATIVE` alone, `MASTER` in a
  pair), so the reviewed digest no longer loops on
  `infrastructure_dns_changed`.
- **Mail follows the DNS identity.** In the join flow only components that
  need no DNS run before the join; the mail profile and everything that needs
  a mail server (SpamAssassin) run after the DNS steps and waits on both
  servers and in every layout.
- **PowerDNS as the main DNS server: one tested series.** The wizard offers,
  the review accepts and the DNS step installs Debian's own archive build of
  the tested PowerDNS series (4.9.x, no epoch, a Debian archive revision, from
  Debian's archive; D-028 amendment). A candidate outside it is not offered
  (`server_setup_pdns_main_untested_version`, or
  `server_setup_pdns_main_not_debian_archive` for repo.powerdns.com, backports
  or another repository) and is refused at review, never mid-run.
- **Changed second-server answers recover with a new code.** A binding still
  belongs to one run. While no run with it has succeeded, answers the second
  server changed afterwards (for example its DNS software) mark the join
  changed: the first server's run says `server_setup_peer_join_changed` (the
  owner creates a new join code there and enters it on the second server), a
  new code is allowed, and the run binds the new answers and continues.
  Unchanged answers re-bind as before; a run that already went on with the
  earlier answers asks to reopen the plan
  (`server_setup_peer_join_reopen_required`). After a run succeeded the
  answers are final and a new code is refused, both said plainly
  (`server_setup_peer_names_final`, `server_setup_join_finished`).
- **Extending after a succeeded pair** reuses its binding: no new join step.
  An extension that would change what the second server received (shared
  answers, names, mail) is refused at review
  (`server_setup_extension_needs_second_server`): keep the shared answers, or
  set up both servers again.
- **Join code tries** are said the same everywhere: at most 5 wrong tries from
  one address, and the code's own remaining total (50 at creation), the same on
  re-read and in a refusal.
- **An owner's native DNS edit** is one notice on the DNS software card and the
  DNS page (`engine_owner_changed` / `dns_overview_engine_owner_changed`):
  changed outside CelikPanel, the owner acts, keep it (nothing to do; never
  overwritten); never "Nothing to do" as a calm state and never a foreign DNS
  server. Known limit: while the change stays CelikPanel publishes no DNS
  change there (the record editor cannot publish either), and handing the DNS
  software back after a native edit is not available in this source; the
  notice says so.

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

Review corrections (same phase):

- **The shared check.** Its timing is set under its lock (a data race was
  measured with `-race`); a check that panics wakes its readers, who say the
  result is not known. "Check now" (`?fresh=1`) starts a real new check at
  most once per panel every 15 seconds; inside that window it returns the
  check in flight or the latest answer and `fresh_after` says when the next
  is possible. The page reads again when the shared answer expires
  (`next_check_after`) and waits while the tab is hidden. Any write to the
  main domain's zone (a digest of its records is part of the key) is a new
  check, so a change made in the editor or by "Move mail" shows at once.
- **Truthful states.** Setup neither finished nor running asks nothing of the
  internet and points to Server setup. A DKIM key that could not be read is
  unknown; no key after a finished setup with mail is a task. A backup DNS
  server that answers but not for the domain before its first copy is proven
  (pair ready on the main, catalog consumption on the backup) waits, on the
  page and on the registrar card alike (one rule). A server name the
  internet shows wrong is compared with our own DNS server's answer first:
  if our list has another value it is the owner's change, not a delay. After
  setup is edited (an extension under review) the picture keeps the last
  completed run and its second server. Only the sentence's title is a live
  region; it is an alert only when the state turns into a verified failure.
- **Mail.** The MX is judged by this panel's own list. "Move mail" is offered
  only while the list still has exactly the old MX setup imported; an MX the
  owner set is never called the old mail server. The record update API
  takes an optional `expected_content` and refuses a mismatch
  (`DNS_RECORD_CHANGED`); the confirmation names the record's real TTL. A
  record setup wrote that the owner removed or changed is never added back
  by a later plan or extension.
- **First site in the infrastructure zone.** Adding the main domain as a
  website when it is already the infrastructure zone adds only the missing
  apex and www addresses (A, and AAAA with a global IPv6 address); a name
  that points elsewhere is left unchanged and reported with who decides.

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
  hand" is gone. Exactly one server receives the main domain's mail, and the
  join's shared answer says which (superseded on 2026-10-29: mail chosen on
  both is refused at review with `server_setup_peer_mail_both`).
- **The link.** The link dials the peer's IPv4 address and trusts only its
  key fingerprint (the full SHA-256 SPKI after the join), never a
  certificate name. The panel keeps serving its first self-signed identity on
  its IP address across domain certificates, so the pin holds during setup.
  The first server keeps only keyed hashes (HMAC under the panel's key file,
  outside the database) of the code's secret and of the second server's
  credential; the second server derives the credential it accepts and keeps
  the typed code, sealed under the key file, only while a join's outcome is
  unknown (it is deleted on every known outcome). Every secret a server must
  use again is sealed under its own key file (AES-GCM, `secret.key`): on the
  second server the link credential, the enrollment code the first server
  handed over and the one it issued itself as main DNS server; on the first
  server the credential it uses on the second and, in Option A, the
  enrollment code the main sent with its names. A copy of either database
  alone therefore yields no bearer, code or enrollment. Hashes and sealed
  values both carry the key generation they were made with (`k1:`), so a
  later key rotation can tell old values from new; there is no rotation yet,
  and a value of another generation is not read. The link credential and the
  join code are removed on both servers when their setup completes. Nothing
  secret leaves through an administrator API, a log or the audit.

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
