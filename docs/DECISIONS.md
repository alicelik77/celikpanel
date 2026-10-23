# Strategic Decisions

*Project record · [Türkçe](DECISIONS.tr.md)*

Durable record of the **why** behind big directional choices — the reasoning
we do not want to re-derive from scratch each time the question resurfaces.
Code decisions live in git; this file is for strategy. Newest first.

> **Erratum on dates (recorded 2026-10-09).** The date line of D-029 ("October 10, 2026")
> and the reference in it to the "entry of 2026-10-10" are not calendar dates. D-029 was
> committed on 2026-10-09 at 05:57 (`faa5ef085`, local time, UTC+3). Work rounds were
> labelled 2026-10-10, 2026-10-11 and 2026-10-12 by raising the label once per round
> instead of reading the clock; the rounds were committed on 2026-10-09 (2026-10-10:
> 00:45-05:57; 2026-10-11: 08:05-09:12; 2026-10-12: 09:12-13:06). The labels stay, because
> entries cite each other by them. The mapping, the commits and the three evidence
> directories named with the same labels (`set1-20261010`, `set2-20261011`,
> `set3-20261012`, run on 2026-10-08 and 2026-10-09) are in the erratum at the top of
> [OPERATION-GUIDANCE.md](OPERATION-GUIDANCE.md). This file has one affected entry,
> D-029. Entries written after this note use the clock date; a dated entry from 10, 11 or
> 12 October 2026 other than D-029 is genuine.

---

## D-037 · Every page is where its name says it is: Settings holds settings; tools, security and maintenance live under Server; the person's own account lives in the personal menu

*October 22, 2026 (clock date) · Owner request of 2026-10-21 and approval of 2026-10-22; in source for the v0.1.0-alpha.98 candidate, published 2026-10-22; on 2026-10-22 the owner's screens showed the new menu on both installed servers after the owner's own update · Amended 2026-10-22: changing one's own email, in source for the v0.1.0-alpha.99 candidate; nothing of the amendment measured on an installed server · Amended again 2026-10-22: Server setup moves under Server, the removal dialog says what is affected, the first visit opens the installed view; in source for the v0.1.0-alpha.100 candidate*

On 2026-10-21 the owner asked: "ayarlardaki bu alt
menüdeki her şey ayarlar ile mi ilgili?" (In English: is everything in this
submenu of Settings really about settings?) and went on: "her işlemin yeri doğru
olmalı. saçma sapan alakasız yerlerde olmamalılar. user friendly yaklaşım önemli.
herkes aradığını kolayca bulsun." (In English: every operation's place must be
right; they must not sit in senseless, unrelated places; a user-friendly approach
matters; everyone should find what they look for easily.) A mockup of the new
layout was shown, then revised after a second thought (below), and on 2026-10-22
the owner answered: "menu düzeni taslağı ok." (In English: the menu layout draft
is OK.)

The question was fair. Settings had eight sections: Server setup, Account
security, Panel HTTPS, License, CelikPanel updates, Server security, DNS
infrastructure and System databases. Four of them were not settings of the panel:
the exposed-services audit is about the server's safety, DNS infrastructure is the
server's DNS role and its peer, System databases is maintenance of SQLite files,
and Account security is the signed-in person's own two-factor panel. A reseller or
a customer, who has no panel to set, saw a Settings whose only section was their
own account. D-019 already says empty management pages are forbidden; a Settings
that exists only to hold someone's own account is the same fault seen from the
menu.

The rules adopted:

1. **A page goes where its name says.** Settings is for choices made once that
   decide how the panel behaves. Tools, the server's security and its maintenance
   belong under Server. What belongs to the person and not to the server (their
   password and their two-factor authentication) belongs in the personal menu.
2. **The final layout (administrator).**
   - Panel (the Dashboard), unchanged.
   - **Hosting:** Domains, Databases, Users, Store, VPN, unchanged.
   - **Server:** Components · **DNS and peer servers** (moved from Settings → DNS
     infrastructure) · Monitoring · **Security** (new page) · **Maintenance** (new
     page) · Import · Audit log.
   - **Settings** (last in the rail) keeps four things: **Panel address and HTTPS**
     (the item was labelled "Panel HTTPS"), License, CelikPanel updates and Server
     setup.
   - **Personal menu** (the user name at the top right): **My account** first, the
     language switcher on a phone (as before), Sign out. Theme and language stay in
     the top bar.
3. **What each moved thing is, and why it is there.**
   - *DNS and peer servers* (`/dns-infrastructure`): today's DNS infrastructure
     content, unchanged: the shared nameservers, this server's role, its peer
     server and DNS replication. It is the server's DNS role, not a panel choice.
   - *Security* (`/security`): answers "is this server safe?" in one place. (1) The
     firewall with its settings (turn on or off, save for reboot, open ports, the
     SSH line), the full box that used to open from the Components page's one-line
     firewall bar. (2) Fail2ban's state, read from the stored component record
     (running; installed but not running; installed, running state not read yet;
     not installed; not checked yet; a failed read is "could not be read" with
     Retry), with a link "Manage Fail2ban" to its component page, or "Open
     Components" when it is not installed. (3) The read-only exposed-services audit
     that was Settings → Server security, moved as is.
   - *Maintenance* (`/maintenance`): Settings → System databases, moved as is
     (inspect and safely maintain the SQLite files CelikPanel and installed
     services keep). The page promises no future feature, such as backup.
   - *My account* (`/account`): the two-factor authentication panel that was
     Settings → Account security, moved as is, and a Password card that opens the
     same password dialog the menu used to open. Roles: administrator, reseller and
     customer, exactly the roles that had Settings → Account security.
   - *Settings* keeps what decides how the panel behaves and is changed rarely:
     the address and HTTPS certificate it is served on, its licence, its own
     updates, and the server's purpose with its setup.
4. **Second thoughts taken before the approval.** The first draft moved Import to
   Hosting and Server setup out of Settings; both were reversed. *Import* stays
   under Server: it is a tool used rarely, and it is looked for among server tools.
   *Server setup* stays in Settings: it is run once and then rarely, for "extend";
   the server's purpose is a configuration choice; a thing done about once a year
   deserves no permanent place of its own in the rail. *Store* and *VPN* stay under
   Hosting because resellers and customers use them.
5. **Old addresses keep working.** Every old `/settings?section=` address is
   replaced (a replace-redirect, so no extra history entry) by its new page:
   `?section=account` to `/account`, `?section=security` to `/security`,
   `?section=dns` to `/dns-infrastructure`, `?section=system-databases` to
   `/maintenance`. `/settings` for a reseller or a customer, whose Settings would
   be empty, goes to `/account` whatever section the address names (as Settings
   used to open their account section for any section it could not show them), and
   they no longer see Settings in the rail. An additional user (team member) never
   had Settings or the account section: the route guard still sends them home, and
   they keep "Change password" in the personal menu. Bookmarks, old guidance links
   and the links of an older Panel (which the browser redirects) therefore reach
   the right place.
6. **Roles do not change.** The role that could see a page or call an API before
   can still; only the place moved. No API, Agent, updater, schema, polling or
   component behaviour changed. Nothing hosted depends on where a page is (D-022).

Implementation decisions taken while building this (stated so they are not
mistaken for owner decisions):

- **Settings has four sections.** The brief allowed a fifth, administrator security
  rules, only if all-administrator policy settings existed. None exist (Settings →
  Account security held only the signed-in person's own two-factor panel), so none
  was invented.
- **The password change moved into My account**, opening the same dialog. The menu
  no longer repeats it for roles that have My account; additional users keep it in
  the menu. The dialog is fetched when it is opened, not on every page load; its
  words stay in the shell catalogue (`profile.*`), so it never shows raw keys.
- **Icons** come from the lucide set: DNS and peer servers a network, Security a
  shield with a check, Maintenance a wrench, My account a person with a check
  (UserCheck, already in the boot bundle).
- **The firewall box still shows on Components when it needs attention** (off,
  unreadable, no engine, an SSH note, persistence), because that is where the owner
  is when it matters; only the healthy state is the one calm line, whose link
  opens `/security#firewall` and says so ("Manage on Security"; it said
  "Settings" before, which now names a place that holds no firewall setting).
  Browser batch 23 found that label, and a running Fail2ban drawn as "installed
  but not running" because the stored record says systemd's "active (running)";
  both were corrected before this was recorded, and an optional Fail2ban that is
  not installed is drawn calm, not amber.
- **The Security page reads Fail2ban from the stored record only**; managing it
  stays on its component page. It reads host-mutation readiness every 5 seconds
  while open, as Components does, and shows the same "changes are held" line ("Server changes are temporarily
  unavailable.") when the server is busy (D-024; D-033).
- **Every in-product link points to the new place directly:** the Dashboard start
  guide and account card, Domains' "DNS needed", the Add domain dialog, Components'
  DNS buttons and firewall line, the server setup's next step ("Open DNS and peer
  servers"), the setup recovery links and the start guide. Guidance texts that
  named "Settings > DNS infrastructure", "Settings > Server security" or "in
  Settings" for the DNS pair now say "Server → DNS and peer servers" or "Server →
  Security". On the server side, every guidance action that pointed at
  `/settings?section=dns` (DNS engine workflow, DNSSEC, DNS-only children, import,
  remote DNS authority, DNS setup) now points at `/dns-infrastructure`; the panel
  certificate's missing contact email carries no action (see the review corrections
  below; from the v0.1.0-alpha.99 candidate it points at `/account#email`, see the
  amendment at the end).
- **The setup gate keeps the four addresses reachable** while setup is required
  (`/account`, `/security`, `/dns-infrastructure`, `/maintenance`), as `/settings`
  was, and the offline recovery shell's route list includes them.
- **The personal menu is a disclosure:** `aria-expanded`, Escape closes it and
  returns focus to its button.

What this does not authorise: no new feature, no change of who may see or do what,
no policy setting invented for the sake of filling a page, and no claim about
installed servers before the owner sees them there. Installed panels are still
updated only by their owners from the panel's own update screen.

Relation to the rest. D-019 (empty management pages are forbidden): a Settings
with nothing for a reseller or a customer is gone. D-022 (the owner's
infrastructure does not depend on the panel): only the place of a screen moved;
nothing the server runs depends on it. D-024 (every operation explains the current
state and next action): the Security page's Fail2ban states and its "changes are
held" line follow it, and the guidance texts and action links name the new place.
D-033 (a screen that makes the owner wait says so): the Security page's readiness
read is the visible, bounded one Components already has. D-035: CelikPanel updates
stays in Settings, where the update card and its dot in the navigation are.

**Review corrections (2026-10-22, before any publication).** An independent review
of the candidate asked for these; they are in source:

- **The boot path.** The coded refusals that only screens show (126 per language:
  DNS, mail, domains, sites, components, repositories, quotas, the firewall) moved
  from the shell catalogue into a screen part "errors", loaded with the other parts
  before any screen renders. The codes the shell can show before then (sign-in and
  access, the licence, the update tracker, the operation overlay, request identity)
  stay in the shell. A refusal whose words have not arrived shows the server's own
  sentence or the generic error, never its key. The critical boot path went from
  360.75 to 329.16 of 361 KiB; the new part has its own ceiling with at least 25%
  headroom. The password dialog's words stay in the shell (`profile.*`), so the
  dialog the menu opens for an additional user never shows raw keys.
- **The firewall box has its own module** (`FirewallPanel.tsx`): the Security page no
  longer loads the Components page to draw it. The remote-state ratchet records the
  move; its totals did not change.
- **One running rule** (`lib/serviceStatus.ts`) for the Dashboard, Components, a
  component's page and Security: "running", or a status that says "(running)".
  "active (exited)" is a finished one-shot unit, not running; before, the Dashboard
  and a copy's row counted every "active ..." as running. A known component list
  without a Fail2ban record says "not installed".
- **Old place names in the remaining texts** now name the new place: a domain's
  nameserver step, a managed DNS record, the start guide's DNS step, the setup
  migration blocker, "Connect to the DNS server", "Panel address and HTTPS" in the
  setup recovery link and renewal blocker, and two setup messages of the Panel.
- **The certificate's contact email.** The refusal pointed at Settings → Account
  security, and in the first draft of this release at My account; neither can
  change it. No screen edits the administrator's own email (the administrator is
  managed only when created on the server), so the refusal has no action now and
  says there is no screen for it yet. **Open item** (open in alpha.98): a way to
  change one's own email from My account. *Now built in source for the
  v0.1.0-alpha.99 candidate* (amendment below); it was open when alpha.98 was
  published.
- **"Open sessions"**, named in the mockup's description of My account, is not
  built: the product has no session list (the roadmap keeps "Active sessions" as a
  later item). As with the administrator rules, nothing was invented.
- The personal menu closes when focus leaves it and names its panel only while it
  is open; Maintenance keeps one subtitle (its card's); the settings catalogue
  part's block markers are fixed; two browser-inspection scripts use addresses that
  still exist.

Not done in alpha.98: nothing was measured on an installed server when it was
written (on 2026-10-22, after the owner updated them, the owner's screens of Boston
and Frankfurt showed the new menu); no screen changed the administrator's own email
(open item above, open in alpha.98 and built in source for the v0.1.0-alpha.99
candidate by the amendment below); open sessions are not built.

### Amendment (2026-10-22, clock date): changing one's own email is built in source for the v0.1.0-alpha.99 candidate

*Owner approval of 2026-10-22 ("ok") of the open item alpha.98 left; in source for the v0.1.0-alpha.99 candidate; nothing measured on an installed server*

Why. The panel certificate gives the signed-in administrator's email address to the
certificate authority as its contact. Until alpha.99 that address was set only when
the administrator was created on the server, and no screen changed it, so an
administrator without a usable address could only read, since alpha.98, that there
was no screen for it yet. alpha.98 recorded "a way to change one's own email from
My account" as an open item. On 2026-10-22, after the alpha.98 publication, the
owner approved building it ("ok").

The rules adopted:

1. **Only one's own address, by every role that has a session.** The signed-in
   person changes their own email address, nobody else's. The route is `PUT
   /api/v1/auth/email`, beside `POST /api/v1/auth/password`, and it is open to
   exactly the roles that may change their own password: administrator, reseller,
   customer and additional user (team member). It is neither administrator-only
   nor in the list of routes refused to additional users.
2. **The current password is asked again.** The request carries the new address
   and the current password; a body with any other field, or with more than one
   JSON object, is refused; the keys must be spelled exactly so. Every attempt
   counts against one re-authentication limit shared with the password change
   (`reauth`, per account and client address). A wrong or missing password is `403
   account_email_wrong_password`; too many attempts are `429
   account_email_too_many_attempts`.
3. **One identity per action; a replay never changes twice (D-029).** The
   request-identity guard now protects this route; it is the first `PUT` it
   guards (it guarded `POST` routes only). The request carries
   `X-CelikPanel-Request-Id` (32 lowercase hexadecimal characters). The same
   identity is answered from the first arrival's stored answer (with
   `X-CelikPanel-Request-Replayed: 1`) and never changes the address a second
   time; the same identity with another address is `409 REQUEST_ID_REUSED`; a
   request without one is `428 REQUEST_ID_REQUIRED`. The route's time limit is
   30 seconds. The digest the guard stores is of the new address alone,
   re-encoded (a digest hook per route), and the stored answer carries no address,
   so the `request_identities` table holds neither the password nor the address. The handler refuses to run if it is reached without the guard.
4. **One validator.** An address is accepted only by
   `mutationpayload.CanonicalContactEmail`, which is the panel certificate's
   existing contact check, exported and reused: surrounding spaces removed;
   printable ASCII only; exactly one `@`; a local part of at most 64 bytes and a
   whole of at most 254; a domain that is a fully qualified DNS name, stored in
   lower case; no display name. An address accepted here is therefore always one
   the certificate request accepts. Anything else is `400 account_email_invalid`;
   an unreadable request is `400 account_email_request_invalid`.
5. **One account per address.** An address another account uses is `409
   account_email_taken`, compared without regard to letter case. The `UNIQUE`
   constraint on `users.email`, in the schema since `001_full_schema.sql`, stays
   the last word. No schema change, no new index. Whether the address is already
   the person's own (`409 account_email_unchanged`) or another account's is told
   only after the password has been proven, so the route does not reveal who
   uses an address.
6. **A change made elsewhere is reported, never overwritten.** The write is a
   compare-and-set on the address and password hash that were verified. If either
   changed in between, nothing is written and the answer is `409
   account_email_changed_elsewhere`.
7. **Sessions stay.** Signing in uses the user name; the address is not a
   credential, so open sessions are not ended.
8. **The audit trail names the account, not the address.** One entry
   `account.email.changed` on the user, without the old or the new address (the
   trail names accounts, never addresses). The Dashboard's "Recent activity" says
   "changed their own email address" ("kendi e-posta adresini değiştirdi").
9. **No automatic certificate action.** Changing an address requests, renews or
   retries nothing about any certificate. The panel certificate request reads the
   signed-in administrator's address from the database when it is made (no
   cache). Its refusal `panel_certificate_contact_email_invalid` now carries the
   action `/account#email` and says to set the email on My account (top-right
   menu) and then request the certificate again. The server setup plan keeps the
   contact address it was reviewed with as part of the plan's identity: after an
   email change, starting an already-reviewed plan is refused as "requirements
   changed, review the current plan", and the new review picks up the new
   address; a setup that is already running keeps the address of the plan it
   started with.

Implementation decisions taken while building this (stated so they are not
mistaken for owner decisions):

- **The licence gate is unchanged.** The email change is *not* added to the routes
  a licence-locked panel still accepts (the password change is one of them). While
  the panel is licence-locked, the email change is refused like the other routes.
  What a licence-locked panel accepts is licence policy, which this release has no
  authority to change.
- **Where it is.** My account gets a third card, "My email" ("E-posta adresim",
  anchor `id="email"`, so `/account#email` scrolls to it), between Password and
  Two-step sign-in. It shows the current address read from `/api/v1/auth/me`
  ("checking" while it reads; "could not be read" with Retry when the read fails).
  "Change email" opens a small dialog with the new address and the current password.
  Additional users, who have no My account page, get "Change email" beside "Change
  password" in the personal menu, opening the same dialog.
- **The wait is visible and bounded (D-033).** While saving, the button shows a
  spinner, the fields and buttons are locked, and a line says the change is being
  saved and nothing else is needed. After 20 seconds the dialog says it is not
  known whether the change was saved, that this is not a failure, and the card
  reads the address again.
- **A lost answer is asked again once.** One identity per click; when the
  connection failed or a gateway answered, the dialog asks once more with the same
  identity. A refusal the Panel itself sent is not a lost answer.
- **Refusals in the D-024 order.** Each code above is shown in the dialog as what
  happened, that nothing was changed, and what to do next. Success shows "Your
  email address was changed." and the card shows the new address.
- **The setup plan's contact blocker and the panel certificate refusal** say "Set
  your email on My account (top-right menu)" and show an "Open My account" button
  to `/account#email`.
- **No message is sent to the old or the new address.** The panel sends no mail to
  confirm the new address; the person proves who they are with their password.

What this does not authorise: changing another account's address from this route;
signing in with an email address; any certificate request, renewal or retry; any
licence-policy, schema, Agent or updater change; and any claim about installed
servers before the owner sees it there. Installed panels are still updated only by
their owners from the panel's own update screen.

Relation to the rest. D-024: every refusal says what happened, who acts and what
to do; an unknown result is not called a failure. D-029: the first `PUT` under the
request-identity guard, with a digest of the new address alone. D-033: the
dialog's wait is visible and bounded. D-022: the change writes only the panel's
own user record; no hosted service is reconfigured, and the panel certificate
reads the address only when someone requests a certificate.

**Security review corrections (2026-10-22, before any publication).** An
independent security review of the candidate asked for these; they are in source:

- **Both contact gates use the one validator.** The panel certificate request and
  the setup plan's contact blocker checked the administrator's address with Go's
  `mail.ParseAddress`, which accepts `admin@localhost`, `root@myhost` or a
  non-ASCII address that the certificate request's own check refuses. Such an
  address (possible, because `--create-admin` does not check the address it is
  given) passed the gate and failed later without the My account guidance. Both
  gates now use `CanonicalContactEmail`: every address the certificate would
  refuse gets the coded refusal with `/account#email`, and the setup blocker.
- **The stored digest holds the new address alone.** Go's JSON decoding matches
  keys without regard to letter case (and folds `ſ` to `s`), so a body with
  `"Current_Password"` reached the password field while the digest removed only the
  exact key `current_password`. The body's keys must now be exactly `email` and
  `current_password`; any other key (`Current_Password`, `user_id`, `id`, …) is
  `400 account_email_request_invalid`. The digest is the new address alone,
  re-encoded; an unreadable body digests to a constant.
- **The success answer carries no address** (the guard keeps answers for 24
  hours); the page reads the address again.
- **One re-authentication limit.** The email change and the password change share
  one limiter key (`reauth`) per user and client address, so a stolen session gets
  one budget of guesses across both (10 per 5 minutes), not one per route.
- **After an unknown result, the next Save of the same address carries the same
  identity** until a definite answer arrives (not after the Panel's `REQUEST_OUTCOME_UNKNOWN`, which is final for that identity: the next Save is then a new action); another address is
  a new action. A key given twice in the body keeps its last value, in the handler
  and the digest alike.
- **The refusal for an unusable address** says that a domain with letters outside
  English goes in its ASCII form (`xn--…`).
- Tests added: both gates with `admin@localhost`, `root@myhost` and a non-ASCII
  address; bodies with `Current_Password`, `current_paſsword`, `user_id` and `id`;
  a wrong password told before "taken" and "already yours"; an administrator
  impersonating a customer refused without the customer's password; one identity
  with another password answered from the stored answer; the limit shared by both
  routes; the same identity after an unknown result (web).

Known limits, recorded: two simultaneous changes to addresses that differ only in
letter case can both pass the case-insensitive check, because the schema's `UNIQUE`
constraint compares exactly; no case-insensitive index is added in this release.
The certificate authority may still refuse an address at a domain that is not
public (`.test`, `.local`, `.internal`, `example.com`): the panel checks the form,
the authority judges the domain. `--create-admin` still does not check the address
it is given; the panel's gates now catch it.

Not done: nothing is measured on an installed server; the new address is not
confirmed by a message; open sessions are still not built.

### Amendment (2026-10-22, clock date): Server setup moves under Server; the removal dialog says what is affected; the broad Components redesign is withdrawn

*Owner decisions of 2026-10-22 ("1 evet, 2 b, 3 evet"); in source for the v0.1.0-alpha.100 candidate; nothing measured on an installed server*

Why. After alpha.99 was published the owner looked at a proposed redesign of the
Components page (a mockup with a "⋯" menu on every row, a single action column and
Server setup as one line at the head of the catalogue) and asked why it would be
better. The honest answer was that most of it would not be: the page already
remembers the view the owner chose (installed or catalogue) and folds the
catalogue when switching to it, so the broad redesign was **withdrawn**. Three
things from that review were kept, and the owner approved exactly those three: the
removal dialog of the mockup, which names what a removal affects on this server
(1, "evet"); the first visit opening the installed view, which D-010 already
promised (2, option b); and Server setup leaving Settings (3, "evet").

The rules adopted:

1. **Server setup is an operation and lives under Server.** It reverses the
   second thought of rule 4 above ("Server setup stays in Settings"). The owner's
   reading: choosing and extending what the server does is work done on the
   server, like installing components, not a setting of how the panel behaves. It
   is its own item in the Server group, **directly under Components**, labelled
   "Server setup" / "Sunucu kurulumu", at `/server-setup`. It shows what Settings →
   Server setup showed: the setup's state, the server's purpose, the "Extend this
   server's setup" button and the link to the setup wizard (`/setup`, which is
   unchanged). The Dashboard's "Server purpose" card links to it.
   `/settings?section=setup` is replaced (no extra history entry) by
   `/server-setup`, as the alpha.98 moves were; every in-product link to the old
   section, and the Panel's guidance action for a record managed by server setup,
   point to the new page. **Settings keeps three sections:** Panel address and
   HTTPS, License, CelikPanel updates. **Roles do not change:** administrators
   only, as before.
2. **The first visit opens the installed view (D-010).** With nothing stored the
   Components page opened the full catalogue. Now only a remembered "catalogue"
   choice opens the catalogue; a missing, unknown or unreadable choice opens the
   installed view, and a stored choice is still honoured. Nothing else on the
   page changes: the rows, their buttons (restart, stop, Manage and remove stay
   visible; no "⋯" menu), the bundle strips, the badges and the folding are as
   they were.
3. **The removal dialog says what is affected, by name, and what is kept.** It is
   the only part of the mockup that was approved. Rules for it:
   - **Truth first.** Every sentence about what is removed or kept follows the
     uninstall path in the code, not the mockup (the mockup's "configuration
     files stay in place" was wrong: the purge removes them). The Panel answers
     from the same small conditions its uninstall uses, so the dialog cannot
     promise what the removal does not do.
   - **From what the Panel already holds.** `GET /api/v1/service/removal-impact`
     (administrators only, read-only: no new Agent call and no mutation; the host
     profile may come from cached Agent facts, and for the firewall engine the
     firewall's state is read through the existing read, bounded) reads the
     catalogue, the stored component record, the Panel's database and the panel
     certificate file. It names: what the Panel hosts on the component (sites for
     the web server; PHP or Node.js sites for a runtime; databases for an engine;
     mail addresses and forwardings for Postfix and Dovecot; mail domains for a
     spam filter; mailboxes for Roundcube; for certbot the certificates the Panel
     renews with it — the panel's unless the owner serves their own, the mail
     host certificates server setup recorded as issued, and every Let's Encrypt
     site certificate with automatic renewal on — whose renewal stops; for the
     firewall engine the panel firewall's state (on with N open ports, off, or
     not known) and that CelikPanel can no longer apply, change or restore it,
     while rules already loaded may stay until the server restarts; for WireGuard
     the VPN peers the Panel keeps active, which can no longer connect; for
     vsftpd the FTP accounts it records; for Redis, Valkey and Memcached only
     that CelikPanel does not record which sites use them), that server setup
     requires certbot and the
     firewall engine on every server it prepares, the installed components that
     need it, the ready bundles it leaves incomplete and the firewall ports it
     asked for; only with none of these, "CelikPanel records nothing else on this server that depends on X". Hosted names are shown up to five, then "+N more".
   - **Read once, visibly, and never in the way.** The dialog reads when it opens,
     shows that it reads, and after 10 seconds, on a refusal or on an answer it
     cannot read, says that the effects could not be read, that nothing was
     changed, that removal is still allowed and where to look first (Retry reads
     again). A failed read never holds the removal, and the read never starts a
     change.
   - **Existing safeguards stay.** The Panel still refuses to remove a component
     while sites, databases or mail addresses it knows of depend on it, and the
     dialog says so beside those names; conflict and requirement refusals, the DNS
     engines' own workflow (BIND and PowerDNS are not in this dialog) and the
     operation guidance after Remove are unchanged. The Remove button stays the
     outlined danger button.

What the uninstall path does, as the dialog says it (from the code of this
candidate):

| Component class | Removed | Kept |
| --- | --- | --- |
| A package component (services and tools) | Its units, when the catalogue names any, are stopped and disabled (`systemctl disable --now`); a tool without units (certbot, nftables, phpMyAdmin, phpPgAdmin) has nothing to stop. Then its catalogue packages are removed. **apt** (`apt-get purge -y --auto-remove`): the packages, their configuration files, and any automatically installed package that nothing needs any more — system-wide, not only those installed for this component. **pacman** (`pacman -Rns --noconfirm`): the packages, their configuration files and their dependencies that nothing else needs. Ports only it asked for close when the firewall is on (the firewall is set again from what stays installed). | CelikPanel deletes nothing else itself (site files, mailboxes, database files). A package's own removal script, which runs without questions, may delete data that package created; which data is the distribution package's decision, not CelikPanel's, and is not claimed. |
| PHP-FPM and PostgreSQL on apt | As above, plus every installed versioned package matching the catalogue's vendor pattern (`php8.3-fpm`, `postgresql-17`, …) and its companions. On pacman only the catalogue packages. | As above. For a database engine the dialog says, as its own warning line under what is removed, that the database files may be deleted by the package's removal script and to back them up first. |
| A spam filter (Rspamd, SpamAssassin) | As a package component, and Postfix's filter chain is composed again without it. | As above; mail stays, it is no longer checked for spam. |
| phpMyAdmin, phpPgAdmin | As a package component, and the database tools' server is set again without it. | As above. |
| certbot | As a package component (no units). The certificates the Panel renews with it (the panel's unless the owner serves their own, the recorded mail host certificates, the sites' with automatic renewal) stop being renewed. | As above. |
| The firewall engine (nftables) | As a package component (no units). CelikPanel can no longer apply, change or restore the panel firewall; rules already loaded may stay until the server restarts. | As above. |
| A component without catalogue packages for this package manager (Redis, SpamAssassin, phpPgAdmin on pacman) | Nothing: the Agent refuses ("cannot be removed automatically on this system yet"), and the dialog says so instead of a purge. | Everything. |
| Roundcube | Not a package: its application tree under `/var/lib/celikpanel-webmail` (its settings and address-book database included) and the web server entry that served `/webmail/`. | Mailboxes and Maildir data, other sites, and the Nginx and PHP-FPM packages. |
| A refused removal (sites, databases or mail addresses the Panel knows of) | Nothing: the Panel refuses with the names. | Everything. |

Implementation decisions taken while building this (stated so they are not
mistaken for owner decisions):

- **Roundcube's leftovers offer stays in sight.** That offer sits on a
  not-installed Roundcube row. With the installed view as the first view it
  would have been hidden behind the switch, so the installed view also shows the
  Roundcube row while the Panel's record of an unfinished install or removal makes
  the offer.
- **The blocking hosted kinds are the existing guard's own answer**
  (`serviceDependents`); mail domains for a spam filter and mailboxes for Roundcube
  are counted only for the dialog and never block. No guard was added or relaxed.
- **The Remove button is not disabled by a read that says the removal will be
  refused:** the Panel is the authority, the facts can change between the read and
  the press, and its refusal is shown in the dialog as before.
- **The package manager's command is named** under the removed sentence (for apt
  and pacman only), so an owner can recognise what runs.

**Review corrections (2026-10-22, before any publication).** An independent review of
the candidate found the removal behaviour unchanged and asked for these; they are in
source:

- "Nothing else on this server depends on X" was false for certbot and the firewall
  engine, which server setup requires and through which the Panel renews every
  certificate and enforces its firewall; the dialog now names those certificates
  and the firewall's state, and says that server setup requires both. For Redis,
  Valkey and Memcached it says only that CelikPanel does not record which sites
  use them.
- apt's `--auto-remove` is system-wide; the dialog says so per package family.
- The database warning is its own line under what is removed, not "what stays".
- A tool without units is not said to be stopped.
- A package family without catalogue packages is said to be refused, as the Agent
  refuses it, with no purge and no command.
- Roundcube's kept sentence names the Nginx and PHP-FPM packages (its `/webmail/`
  entry is removed), and a failed read for Roundcube shows what its removal
  deletes, not the package purge.
- The Agent package's own tests (in a clean mount namespace) are run
  and recorded, since the Agent's uninstall and update tests changed.

**Second review round (2026-10-22, before any publication).** A re-check asked for
these; they are in source: a firewall answer that carries its own error or error
code, or an engine that cannot be read, is "not known", never a state; the firewall
engine's sentence says what is true of a ruleset loaded with `nft -f` (CelikPanel
can no longer apply, change or restore the panel firewall; rules already loaded may
stay until the server restarts); WireGuard names the VPN peers the Panel keeps
active, which can no longer connect, and vsftpd the FTP accounts it records;
certbot's list counts only the site certificates the renewal job renews
(`letsencrypt`, `auto_renew = 1`, `status = 'active'`), leaves out the panel
certificate when the owner serves their own (`CELIKPANEL_TLS_CERT`/`KEY` or an
unmanaged TLS directory), and names a mail host certificate only when server setup
recorded its certificate step as succeeded; the firewall read never waits for a
firewall change (it answers "not known" at once while one holds the lock); and the
empty case is narrowed to what CelikPanel records: "CelikPanel records nothing else
on this server that depends on X".

What this does not authorise: no other change to the Components page (the rows,
buttons, strips, badges and folding stay as they are), no "⋯" menu, no new Agent
call, no change of who may see or do what, no claim about installed servers before
the owner sees them there. Installed panels are still updated only by their owners
from the panel's own update screen.

Relation to the rest. D-010 (installed-first) is now what a first visit does.
D-024: the dialog says what will happen and who acts, and keeps a verified refusal,
an unmet prerequisite and an unknown read apart. D-033: the read is visible and
bounded. D-034: a ready bundle a removal leaves incomplete is named. D-022: the
removal sentences say what the package manager and CelikPanel do and nothing the
owner's own configuration does.

Not done: nothing is measured on an installed server; what a distribution
package's own removal script deletes is not read or claimed.

---

## D-036 · A version shown is the package database's own answer; unknown stays unknown

*October 21, 2026 (clock date) · Follows the owner's alpha.94 screens of 2026-10-17 and the open item of the D-034 amendment; in source for the v0.1.0-alpha.97 candidate; nothing measured on an installed server*

The owner's alpha.94 screenshots showed a version column of dashes on the
Components page for every component with a single version (Nginx, PostgreSQL,
MariaDB, Postfix, Dovecot, Certbot, Redis…). alpha.95 removed the column and
recorded "read installed package versions for every component" as an open item
(D-034 amendment): the Panel knew versions only for the multi-version runtimes
(PHP-FPM, Node.js), the Agent's per-unit binary probe is not a package version and
is unreliable, and tools such as Certbot have no unit at all.

The rules adopted:

1. **The source is the system package database, asked read-only.** A new Agent
   read, `Agent.InstalledPackageVersionsV1`, runs during the existing component
   scan, once per scan, one query for the whole catalogue: `dpkg-query -W` with a
   fixed format (package, state, version) on Debian and Ubuntu, `pacman -Q` on Arch.
   Fixed argv, no shell, no network, a 10-second timeout and an 8 MiB output
   ceiling. Which packages count is the Agent's own compiled catalogue (the
   component's packages for this family; for a component with a versioned
   repository pattern such as PostgreSQL's `postgresql-17`, the versioned packages
   when any is installed, because the catalogue name is then a meta-package whose
   version is not the server's). The read takes no package-manager lock.
2. **Never while a package operation runs.** Before the query the Agent reads the
   process table (pacman and the apt/dpkg tools among it) and the kernel's lock
   table for the apt/dpkg lock files; it never takes a lock itself, not even for a
   moment. A pacman lock file left behind by a killed pacman, with no pacman
   process alive, does not count, or the versions would never be read again. A busy
   host, or one whose state cannot be read, answers "busy": nothing is queried, and
   the Panel keeps the previous scan's versions, marked as kept, with the time they
   were read.
3. **Unknown stays unknown.** A component with no installed catalogue package is
   not listed. Only a package the database reports as installed counts: in dpkg,
   current state "i" with a blank error flag, whatever the wanted action ("ii",
   "hi" held, "ri"/"pi" marked for removal or purge); a package flagged "reinstall
   required" ("iiR"), half-installed, unpacked or with only its configuration
   left ("rc") has no version. A version text that
   is longer than 64 characters or carries characters a package database does not
   print is dropped, never shortened, and the whole component's version is then
   unknown. A component whose packages report different versions is "several
   versions", never one of them. A failed or unsupported read leaves every version
   unknown. No guessed version, no dash, no "default".
4. **What is shown.** The full version exactly as printed, epoch and revision kept
   (`1:11.8.3-0+deb13u1`), on the component's own page with its source ("Read from
   the system package database."; for a kept reading, when it was read and that the
   package manager was busy at the last check), in the header shared by every
   component page. On the Components rows the Panel's short form, the upstream part
   (`11.8.3`: the epoch, the packaging revision after the last hyphen and a
   `+dfsg`/`~deb` style suffix removed), in mono, with the full version and its
   source in the tooltip. The desktop version column comes back only when at least
   one installed component on the page has a known package version; otherwise
   alpha.95's line under the description stays. On a phone the version is under the
   description. PHP-FPM and Node.js keep their per-copy versions and are not given
   a package version.
5. **Authorisation and storage follow the existing scan.** The Panel admits the
   method as a read (quick read timeout, no host capability), like the other scan
   reads; it calls it once per scan and a failure never fails the scan. The answer
   travels with the existing scan cache as an additive JSON field of each
   observation; a cache row written before alpha.97 has none and reads as unknown.
   No schema change (44).

What this does not authorise: no package operation of any kind, no version read
outside the scan, no network read for versions, no "latest available" or update
offer from these versions, and no claim about installed servers before the owner
sees them there. Installed panels are still updated only by their owner from the
panel's own update screen.

Relation to the rest. D-024 (unknown is not absent; say who acts and how it
resumes): a kept reading says so, and nothing is said where nothing is known.
D-032 (the panel looks by itself): the read rides the existing conditional and
manual scans and adds no timer. D-034 amendment: its open item "read installed
package versions for every component" is implemented in source by this decision;
it is not measured on an installed server.

*Corrected the same day after an independent review of the candidate:* the first
form counted only dpkg's "ii" (rule 3 now counts every installed state); an answer
over the output ceiling was reported as a generic failure and the query kept
running (now the query's process group is killed at the ceiling and the answer is
"larger than the read ceiling"); the query inherited the Agent's environment (now
`LC_ALL=C`, `LANG=C` and a fixed `PATH`, nothing else); the lock-table read was
unbounded and refused a lock without an inode (now at most 1 MiB, and such a line
is skipped); a stale pacman lock file meant busy for ever (now it needs a live
pacman process); a reading kept for more than a day had no guidance (now the page
adds: "The package manager has been busy at every check for more than a day. If no
installation or update is running on the server, the server administrator checks
whether a package operation is stuck."); and the page's own scan held the Panel's
mutation lease while the package database answered (now it reads the versions
before taking the lease; the fresh scan at the end of an operation still reads
them inside that operation's lease).

*Note of 2026-10-22 (alpha.101 candidate), after the owner's Frankfurt screen on
alpha.100:* the phpMyAdmin row read "5.2.2-really…". A Debian "really" version
belongs to a package that had to go back to an older upstream: it keeps a higher
number before "really" so the package manager still sees an upgrade, and the
version actually shipped follows "really". The short form of rule 4 removed the
epoch and the packaging revision and then cut only at "+" or "~", so
"-really5.2.x" stayed in it, and the narrow column clipped it. The short form is
now the real version after "really", when "really" follows a version separator
("+", "-", "~" or ".") and is followed by a digit, after the epoch and the
revision are removed as before (`5.2.2-really5.2.1-1` reads 5.2.1,
`2:4.1.0+really4.0.9-2` reads 4.0.9; `4:5.2.2+dfsg-1` still reads 5.2.2 and
`3:7.2` reads 7.2). The full version, in the tooltip and on the component page, is
unchanged. The short form is derived when the Panel builds its answer, so a stored
scan reads correctly at once; the Agent and what it reports are unchanged. A
single runtime version on a row now also ends in an ellipsis with its full text as
the tooltip, so no version cell is clipped silently. Nothing measured on an
installed server.

---

## D-035 · The panel looks for updates by itself and says so calmly; only an administrator starts an update

*October 17, 2026 (clock date) · Owner request while updating to v0.1.0-alpha.95; in source for the v0.1.0-alpha.96 candidate; nothing measured on an installed server*

While updating the installed servers to v0.1.0-alpha.95 the owner found two
things in the update screen. The panel learned that a newer release existed only
when someone pressed "Check for updates", so a server could stay on an old
version without anyone being told. And the update card itself was hard to read:
two amber boxes (the alpha warning and "An earlier update to this version
stopped…"), a green "A signed update is available." and a second identical row
below the Start button. In the owner's words: "belirli periodlarda güncellemeleri
kontrol etmeli ve güncelleme var diye uyarmalı. güncelleme konusunda yardımcı
olmalı." In English: it should check for updates at regular intervals and warn
that an update exists; it should help with the update.

The rules adopted:

1. **The panel itself looks, on a schedule, and only reads.** A loop in the
   Panel process (not in the browser) makes the same read-only discovery as the
   "Check for updates" button, through one shared function: the Agent's
   availability and build pair, the host's own authority for starting an update,
   the Agent's check of the signed release at celikpanel.net, the identity match
   and the validation of the target. The first check runs about 2 minutes after
   the Panel starts (plus or minus 30 seconds), then every 12 hours (plus or
   minus 30 minutes). One check is bounded at 40 seconds. It changes nothing
   on the host and writes no update record. Its readiness read is the readiness
   endpoint's: it takes the Panel's own mutation lease with `TryLock` only for
   the in-process check of an active operation and releases it before its
   advisory Agent read.
2. **A failed check backs off, it does not retry in a storm.** After a failure
   the next try is in 1 hour, then in 3 hours, then the 12-hour period again. A
   successful check resets this.
3. **Some conditions skip the check; a skip is not a failure.** The check is not
   run, and tried again in 1 hour, while the Panel admitted an update start in
   the last hour, while the Agent's advisory readiness read reports a change in
   progress or a state it cannot verify, while a server setup run is changing
   the server (a run waiting for a DNS record does not hold it), and when the
   Agent does not answer. A busy package manager does not skip it, because the
   release read does not touch packages. A skip is kept in memory only: it is
   never written and never replaces a known result; the card says it beside
   that result. The check never runs while the Panel shuts down; an interrupted
   check keeps nothing. Turned on again, the schedule continues 12 hours after
   the last completed check, not two minutes later.
4. **The owner can switch it off.** Settings → CelikPanel updates has "Check for
   updates automatically", on by default, with the text "Once every 12 hours the
   panel asks celikpanel.net whether a newer signed release exists. Nothing is
   installed by itself." Off means no network read by the schedule at all; the
   manual check still works.
5. **The result is stored and cleared by rule; never during an update.** The
   latest completed result (outcome, time, whether it was automatic, a reason
   code for a failure, and the release found with the time it was found) is kept
   in the Panel's settings table; no schema change. A failed check keeps the
   release found before it, so a passing outage does not hide an update that is
   still waiting. **The scheduled check never writes while an update is
   admitted or running**, and neither does the button: right before a write the
   Panel reads the Agent's readiness with no lock held, then takes its mutation
   lease with `TryLock` only to read again the facts it holds itself (no update
   admitted, no Panel operation active) and to write the setting; otherwise the
   result is kept in memory, shown, and written on the next idle pass. (A write to the
   Panel's database during the updater's pinned read-only preflight makes that
   preflight stop.) A rebuild with the same version and another commit is what
   the Agent's signed answer says it is. At start, before management opens, a
   record made by another build, or naming a release this build already is or
   is older than this build, is cleared; after an update the notice disappears.
6. **Where the owner sees it, and where not.** Administrators only: a small link
   under the version line in the sidebar ("Update available · v0.1.0-alpha.96");
   one quiet row at the top of the Dashboard content, not inside "Needs
   attention", because an update is not a problem, with "Review and update" and
   "Remind me later" (hidden for 24 hours in this browser for that version);
   a small dot and the same words in the Settings navigation. The update card
   opens already knowing the stored result. A failed or unknown automatic check
   shows nothing outside the update card.
7. **Never an alarm.** An available update has no signal colour: no amber, no
   red, no badge colour. A failed check is calm, says what is and is not known,
   who acts (nobody, or the owner may check now), when the next check runs and
   that the panel and the hosted services work as before.
8. **An update never starts by itself.** Only an administrator starts an
   update, from the panel's own update screen, with the one filled button
   "Update to <version>". Start re-runs discovery and requires the exact same
   target. The card tells the owner what happens before they press (one restart
   of about N seconds, hosted websites, mail, DNS and databases keep running, a
   current backup is recommended for an alpha release, nothing starts by
   itself).

Consequences. A new read, `GET /api/v1/panel/update/discovery` (administrators
only; no Agent call and no network read), returns the stored result, the switch,
the wait to the next scheduled check and ages measured on the server's clock; the
switch is saved with `PUT` on the same path. The first plan named
`/api/v1/panel/update/status` for this read, but that path already answers one
update request's status by its request id, so a separate path was used. The
existing `/update/check`, `/update/start` and `/update/status` keep their shape;
the check now also records its result. No Agent change, no schema change (44),
no migration. The updater's mutation path is unchanged. The browser reads the
stored result when its copy is older than 5 minutes, again every 30 minutes and
when the tab is shown after that long, only while visible. The wording, the
states and how a failed check resumes are in
[OPERATION-GUIDANCE](OPERATION-GUIDANCE.md), "Update discovery and the update
screen".

What this does not authorise:

- No automatic update. The panel finds and says; it never installs, and no
  schedule, poll or notice starts an update.
- No polling that starts a mutation. The scheduled check and every browser
  re-read are reads; a read never starts a second change.
- No alarm colour for an available update, and no badge colour on its link.
- No notice for a failed check outside the update card. A failed or unknown
  check is not shown on the Dashboard, in the sidebar or in the Settings
  navigation.
- No claim measured on an installed server. The scheduled check has not run
  against a real Agent or the real release server; Go tests fake the Agent and
  the browser batch uses a loopback mock.
- No P0 item is closed or advanced.
- No change to who updates installed panels: only their owner, from the panel's
  own update screen.

Relation to the rest. D-022 (the panel works without the portal; nothing
depends on the check) is why a failed check is calm and never blocks anything.
D-024 requires the reason, who acts, the next action and how work resumes, and a
distinction between a verified failure, an unmet prerequisite and an unknown
result: the status line keeps a failure (no verified answer), an unknown result
(a timeout) and a skip (an unmet condition) apart. D-032 said that the panel
looks by itself and that no background timer runs when nobody is looking, which
it applied to the component scan; D-035 is the owner's explicit, narrow exception
for update discovery: one read-only release read every 12 hours on the server,
bounded, switchable, never a mutation. D-032's clock rule is kept: the browser
never compares clocks, the server states the ages. D-033: the card's waiting
states are visible (a checking line with motion while the button's check runs)
or say that nothing needs to be done.

*Corrected the same day after an independent review of the candidate:* the
first form of rule 5 wrote a skip and could write a result while an update was
being admitted; a setup run waiting for a DNS record held the check; the
admission window was 6 minutes. The rules above are the corrected ones.

### Amendment (2026-10-23, clock date): opening the update screen checks again; a stopped update keeps the offer and says why

*Owner observations of 2026-10-23 on Frankfurt and Boston (both on alpha.100); in
source for the v0.1.0-alpha.103 candidate; nothing measured on an installed server*

Why. Frankfurt's sidebar said "Update available · v0.1.0-alpha.101"; the update
screen it opened offered "Update to v0.1.0-alpha.101" although alpha.102 had
been on celikpanel.net since about 10:02Z, and only "Check for updates" showed
alpha.102. In the owner's words: "güncellemede her zaman en son versiyonu
göstermedi … sol alt kısımda 101 nolu güncellemeyi gösterdiğinde ve onu
tıkladığımda güncelleme ekranında en son güncelleme görünmesi gerekmez miydi"
(the update screen did not always show the newest version; when the bottom left
showed update 101 and he clicked it, should the update screen not have shown the
newest one?). He is right: the card showed the stored result of the 12-hour
schedule, and opening the screen did not look again. Start would have refused
the changed target (PANEL_UPDATE_TARGET_CHANGED), so nothing unsafe could happen,
but the owner would have met an error instead of the newest version. On Boston
the update to alpha.102 stopped unchanged because the package manager was busy;
while that notice was open, the update card showed only the installed version,
"Technical details" and the switch, while the sidebar and the Settings item said
"Güncelleme var · v0.1.0-alpha.102", and the notice itself promised that the
card would say when the package manager was free. The cause: the card hid its
status line, the offer, the earlier attempt and its readiness read whenever the
browser's update lock was held, and a failed outcome waiting to be acknowledged
holds that lock.

The rules added:

1. **Opening the update screen checks again when the known result is older than
   ten minutes, only with automatic checks on.** One read,
   `GET /api/v1/panel/update/discovery/refresh` (administrators only), made once
   per opening of the card after the stored result was read. The Panel decides
   again: it runs the same discovery as the button and the schedule, under the
   schedule's skip rules (no check while it admitted an update start in the last
   hour, while the server reports a change in progress, while setup is changing
   the server, or when the Agent does not answer) and its write rules, bounded at
   50 seconds for the whole request (the check itself at 40); the check and its
   save run on even when the browser leaves, so a completed read is kept. Ten
   minutes, so that a person who opens the screen several times in one sitting
   does not ask celikpanel.net each time (screen openings make at most one
   completed release read per ten minutes, whoever opens them; a check that is
   skipped or does not finish within its bound keeps nothing, so the next
   opening may try again), while the screen no longer offers a 12-hour-old
   result when a newer release is out and the check can run.
2. **One release check at a time.** The schedule, the button and the opening
   wait for each other through one gate. An opening that waited for a check that
   completed within the last ten minutes uses that result instead of reading
   again; the schedule, after waiting, does not read again when a check completed
   within the last ten minutes, and skips when a start was admitted meanwhile.
   The button always makes its own read, after any check it waited for; it is
   bounded at 40 seconds like the schedule's.
3. **Waiting is visible and keeps what is known.** "Checking celikpanel.net for
   the newest release…" with motion, "Known before this check: …", and "The
   update button waits for this answer, at most about a minute. Nothing starts
   by itself."; the offer stays in view and Start waits. A failed check keeps the
   offer with its age and the existing failure line; a skipped one, one that did
   not answer in time (result not known) and one whose answer could not be read
   (the Panel answered with an error or an unreadable reply) each say so and how
   old what is shown is.
4. **Automatic checks off: no contact.** Opening the screen asks nothing; an
   offer says "Last checked …", and the next-check line points to "Check for
   updates".
5. **A stopped update keeps the offer and says why.** While the notice of an
   update that ended unchanged (or verified back) is open, the card shows the
   status line, the offer, the earlier attempt with its cause ("The server's
   package manager was busy …"), the package-manager state from its own readiness
   read and its quiet re-read ("still busy" / "has finished"), and at the top:
   "Close the notice about the stopped update first ("Close notification"); this
   button then starts the update to v0.1.0-alpha.102. Nothing starts by itself."
   Start waits for the notice to be closed; the update lock itself is unchanged.
   No check is made on opening while the notice holds the lock; once it is
   closed the card checks if the known result is older than ten minutes (a newer
   release may have been published while the notice stayed open). Within an hour
   of the start the Panel answers that check as skipped, because it admitted an
   update start; the offer then stays with its age and that reason, and "Check
   for updates" reads the release at once.
6. **The sidebar, the Settings item and the Dashboard row follow** the answer of
   the opening check: they read the same shared copy, which the answer replaces.

Unchanged: rules 1 to 8 above (calm notice, nothing installs by itself, only an
administrator starts an update), the start handler's re-check of the exact
target, the Agent, the updater's mutation path and the schema (44). No native
record, by the owner's instruction of 2026-10-16. The browser inspection batch 26
uses a loopback mock; nothing was measured on an installed server.

---

## D-034 · Ready bundles are shortcuts, not products: a general registry, shown at the head of each Components category, one door on the Dashboard

*October 17, 2026 (clock date) · Owner decision after the design review of the Components page (alpha.92 source, 2026-10-16) and the owner's mockup of 2026-10-16; in source for the v0.1.0-alpha.94 candidate; nothing measured on an installed server*

The three "One-click mail stacks" cards stood at the top of Components, above
every component row. On a phone the first row came after about 1,300 px of
cards; the cards drew a verified Webmail as "Partially installed" in amber; and
the idea behind them, installing a known-good group of components in one go,
existed only for mail. The owner asked for the same thing to be general: like
the one-click stacks, but for mail, databases and web alike, and in the place
where the components themselves are. WordPress and other site applications are
a separate, later topic. The Store stays the place for what is acquired or
licensed (D-015 item 2 is upheld: the catalogue is the single install address).

The rules adopted:

1. **A ready bundle is a shortcut over existing catalogue components.** It is
   not a store product, not a component of its own and not a page. A general
   registry in the Panel (`cmd/panel/bundles.go`) names each bundle, its
   category, its components in install order and the bundles offered after it.
   The first entries: Core mail, Webmail and Spam-protected mail (through the
   existing mail profiles); a PostgreSQL bundle and a MariaDB bundle, each with
   one optional add-on (phpPgAdmin, phpMyAdmin, with the Nginx and PHP-FPM they
   need); Nginx + PHP-FPM.
2. **Where.** Each Components category draws one line, "Ready bundles",
   between its header and its rows. An installed and verified bundle collapses
   to "… installed and verified" (Repair, where it exists, is a text link);
   installed but not verified reads "installed, not verified" in grey with
   Repair (mail) or Verify (database and web); a bundle installed contains the
   smaller ones it covers (Webmail covers Core Mail), and at most one more
   offer follows (the registry's add-on first). A blocked bundle says why, in
   grey, with no button; a host-wide mail refusal (the DNS identity) is one
   line for the category, and an installed mail bundle stays visible beside it.
   A mail bundle's last failed or running attempt and the Panel's warning are
   said again under it (D-024). Mail bundles carry the product's own mail
   profile names. The Dashboard has one calm "Ready bundles" card, always
   shown, that lists what is installed, how many bundles can be installed and
   any failed or running mail attempt with its reason, says "not checked",
   "could not check", "held" and "not recent" each in its own words (with the
   check itself on a host never checked), and is a door to
   `/services#bundles`; it installs nothing.
3. **States and actions are the Panel's.** Unknown (no fresh scan), available,
   partial, installed and not verified, installed and verified, blocked, with
   the mail profiles' typed reasons. Actions: install, continue, verify, repair,
   none. Mail bundles take their state from the mail profiles; a mail profile
   whose components are all present but whose proof is missing is "installed,
   not verified", not partial. **Repair and Verify mean different things per
   route.** There is no read-only verification of a mail setup: for a mail
   bundle the action is Repair, which runs the profile install; it completes
   what is missing and verifies the setup, and nothing installed is removed
   (the dialogue says so before it is pressed). For a database or web bundle
   Verify only reads the components again and changes nothing, so it stays
   usable while the page holds its changes. A database or web bundle is never
   partial: it is verified when all its components are ready on a fresh scan,
   and otherwise offered with Install, which installs only the missing parts
   (the parts present are listed as kept, and a part whose role is already
   filled, such as Nginx where Apache serves the web, as not needed).
4. **How it installs.** A mail bundle opens the existing mail install dialogue
   and runs exactly today's route (setup extension or the profile install). A
   database or web bundle asks once and then sends one ordinary component
   install per missing component, in order, each only after the previous
   operation, this step's own component install, has ended with a fresh scan
   (one active operation per host), and only after the page's change gate (an
   unverified page state, a host not ready for changes) has been read again.
   The first failure, an operation that was not this step's, or a closed gate
   stops the sequence with its reason and a Continue that installs only what
   is missing; a failed component's own guidance (D-024) stays on screen. That
   Continue lives in the page: a poll never sends an install, and after a
   reload the bundle is simply offered again. No schema change (schema 44) and
   no new Agent job kind.

Consequences. The Components colour corrections of the same review are applied
as existing DESIGN.md rules, not new ones: row action icons are neutral at rest
and take their tone only under the pointer or focus (State Colour Rule,
No-Adjacency Rule); the status dot has four tones, so a stopped service is red
with its word and "not installed" is a hollow ring (State Colour Rule); the
amber Layers tile and the amber "partial" sentences went with the cards, and
the strip uses Amber Ink only for a webmail runtime that is installed but not
ready (Two Yellows Rule); "Repair webmail" stays neutral unless the server
reports a webmail problem; a healthy firewall is one line; component icons come
from one map of line icons instead of emoji; the row is a fixed-column grid,
and on a phone row actions are 44 px with their names written.

What this does not authorise: no installed-server change, no new store item,
no licence-policy change, and no WordPress or site-application bundle. Installed
panels are still updated only by their owner from the panel's own update
screen.

### Amendment 2026-10-17 (alpha.95): the same list on every server, with the state on each item

*Owner decision of 2026-10-17 after looking at Components and the Dashboard on Boston and Frankfurt (alpha.94): "ok. sorunları tekrar değerlendir ve düzelt" ("ok, evaluate the problems again and fix them"); in source for the v0.1.0-alpha.95 candidate; nothing measured on an installed server*

What the owner saw. On Boston the MariaDB bundle was not drawn at all although it
could be installed, and the Dashboard still counted it ("5 more bundles can be
installed") while it did not count the phpPgAdmin add-on that was visible. Core
Mail was an item on Boston and absent on Frankfurt. Two servers showed two
different lists, and the number on the Dashboard did not match the list below it.
The cause was rule 2 above: "installed line plus at most one more offer" hid
sibling bundles. That rule is **withdrawn**. It was the release preparer's
mistake, not an owner requirement.

The rule that replaces it:

1. **One list.** Every Components category shows the same ready bundles in the
   same registry order on every server. Only the state on each chip changes. The
   strip is a left-aligned, wrapping row of chips, not an installed line plus an
   offer. Add-ons always appear.
2. **State in words and in form, never by colour alone.** Installed and verified
   is a check mark and the name (accessible name "…, installed and verified");
   "Core Mail · included" when a verified mail bundle contains it and the contained
   bundle is itself installed and verified with no failed or running attempt of its
   own (otherwise it reads "not verified" with its reason); a grey dot and
   "not verified"; an outlined "Name · Install" (add-ons: "· Add"); mail with an
   attempt "n of m installed · Continue"; blocked is muted text with a short reason
   ("phpMyAdmin · needs MariaDB") on a quiet button: pressing it, by keyboard or
   touch, says the full reason on a line under the chips (the tooltip stays but is
   no longer the only way); a running bundle is "2/3 · installing PHP-FPM" with a
   moving indicator and the other chips disabled. A stale scan is one muted line.
3. **One counting rule.** The Dashboard card and the chips count the same way:
   every registry entry, add-ons included; "can be installed" means available or
   partial (blocked is excluded); "included" counts as installed and verified.
   The card is one summary line ("7 installed and verified · 1 can be installed"),
   then only the problem lines (a failed or running mail attempt with its reason,
   a bundle installed but not verified; a bundle that is both is one line, with the
   reason), then "Open in Components". A bundle whose own last attempt failed or
   runs is never counted as installed and verified. The never-checked, checking,
   held, could-not-check and not-recent sentences are kept.
4. **The installed chip holds Repair and Verify.** Pressing an installed chip
   opens a dialogue: Repair for a mail bundle (the profile install; it completes
   what is missing and verifies the setup, nothing installed is removed), Verify
   for a component bundle (it reads the components again and changes nothing).
   The strip carries no Repair links and no hint sentence. Roundcube's "Repair
   webmail" stays on the component row only when the server reports a webmail
   problem; otherwise repair lives in the Webmail chip's dialogue.
5. **Three review items of alpha.94 are closed.** The gate between a bundle's
   steps reads the state-unverified flag through a reference set at the moment it
   changes, before and after the readiness read. Verify of a component bundle is
   held, with its reason, while a component operation runs (a locked tracker, an
   active bundle sequence, or the host reporting `panel_operation_active` or
   `agent_mutation_active`); it stays usable when changes are held for other
   reasons, because it only reads (`host_lock_busy` and `package_manager_active`
   among them: the rescan endpoint refuses only for a running server setup, an
   active service operation or an active Agent mutation). The chip says "Checking the server before
   installing X…" until the install request is accepted, and "installing X" only
   afterwards.

No schema change (44), no Agent change, no new Agent job kind. The rest of D-034
(shortcuts over catalogue components, one sequence at a time, Continue only on the
owner's action, no continuation after a reload) stands.

**Versions: no new Agent RPC in this release.** The owner's screens showed a
version column of dashes. Investigation: the Panel fills `Versions` of a managed
service only from runtime instances (`Agent.ListServiceInstances`, so only PHP-FPM
and Node.js). `Agent.GetServices` returns a binary-probe `Version` per systemd
unit (`internal/services/version_detector.go`) that the Panel ignores. It is not a
package version, it is unreliable for several units (Apache, `named` and
PostgreSQL run the unit name as a binary) and it does not cover tools without a
unit (Certbot). Real package versions would need a new Agent RPC or extended
`dpkg`/`pacman` queries, which this release does not add. Decision: the desktop
version column is removed; a version is shown as small monospaced text under the
description only where it is known; the browser mock no longer invents versions.
**Open roadmap item:** read installed package versions for every component.

**Stop icon.** The stop button was an empty square, which read as a checkbox. It is
Lucide `StopCircle` (a stop sign in a circle) on the Components rows, the component
page and the hosting type panel, and restart, stop, start, repair and delete share
one outlined icon button. The visuals are in DESIGN.md, "Ready bundle chips, row
actions and versions (alpha.95)".

Owner-requested cleanups of the same review, no new rule: sidebar counts
(Components, Domains) are plain muted numbers, not filled pills (a count nobody took
is still a dash, never 0); the Dashboard's "nothing needs your attention" sentence
is plain language and still requires every read answered and a fresh components
scan; the Dashboard's Domains count adds the standalone infrastructure DNS zones
the Domains page lists (the alpha.91 owner decision of 2026-10-16); the Dashboard
tile, the sidebar Domains badge and the Domains page now agree, because the tile
and the badge use one helper (`domainsCount`, `web/src/lib/infrastructureZones.ts`):
domains plus the DNS-only infrastructure zones the Domains page lists (a zone that
became a website counts once, as that website); a number only when both reads
answered; the loading mark while either is read; unknown, never "domains + 0",
when one could not be read (an earlier number is kept only when both reads had
one); non-administrators, who have no zone read, count domains alone.

**Independent review of the candidate (2026-10-17).** An independent review found
ten problems; they are fixed in the candidate, and this
amendment was corrected to match. (1) Roundcube leftovers: the Components row shows
the muted note and the neutral "Clean up leftovers…" button only when the Panel's
own record shows an unfinished Roundcube install or removal (the newest Roundcube
install or removal entry in the Panel's audit log is a failed install or a partial
or failed removal, sent as the additive payload field `webmail_cleanup`) or when
the webmail profile's last attempt failed; a later clean install or removal ends
the offer, and without a record the row says nothing. Roundcube's own component
page always has a "Clean up leftovers…" link that opens the same confirmation
(`/services?cleanup=roundcube`). The confirmation is unchanged. Known gap that
remains: the Panel still cannot see whether files are left on disk (no Agent
read); it relies on its own record. (2) The installed bundle dialogue describes the
bundle ("PostgreSQL for your sites' databases.") instead of "Installs …". (3) Under
the chips: only attempts that failed or run, and typed warnings on chips that are
not verified or only partly installed; never the "some parts are already present"
note. (4) "included" only for a contained bundle that is itself installed and
verified with no failed or running attempt of its own; otherwise "not verified"
with its reason, and the Dashboard does not count it. (5) One Domains count for
tile and badge (above). (6) A bundle that is not verified and has a failed or
running attempt is one Dashboard line. (7) A blocked chip is a quiet button that
shows the full reason under the chips. (8) Verify stays usable on
`host_lock_busy` and `package_manager_active` (above). (9) Wording: "2/3 ·
installing PHP-FPM"; "n of m installed" (Turkish "2/3 bileşen kurulu"). (10) A mail
chip that says "Install" (Spam-Protected Mail beside an installed Webmail, which
the Panel calls partial only because the parts are shared) opens the mail
dialogue in its install form, not "continue". No schema change (44), no Agent
change, no native record: the new payload field is additive JSON read from the
existing audit log.

What this does not authorise: no installed-server change and no licence-policy
change. Installed panels are still updated only by their owner from the panel's own
update screen.

### Amendment 2026-10-22 (alpha.101): a bundle is a service with its companion tools

*Owner decision of 2026-10-22 after looking at Frankfurt on v0.1.0-alpha.100; in source for the v0.1.0-alpha.101 candidate (the code on 2026-10-22, this record on 2026-10-23, clock dates); nothing measured on an installed server*

What the owner saw. On Frankfurt, on alpha.100, the database category offered a
"PostgreSQL" bundle and a "MariaDB" bundle that were each one component, the same
component that has its own row right under the strip, and the browser tools
phpPgAdmin and phpMyAdmin as separate add-ons. The owner's words: "bundle diyip
tek servisi kurmak nedir. saçmalık. postgresql ve phpPgAdmin bundle dır mariadb ve
phpMyAdmin bundle dır. Diğer tüm bundle lar da böyle dir." (In English: what is
calling it a bundle and installing a single service? Nonsense. PostgreSQL and
phpPgAdmin is a bundle, MariaDB and phpMyAdmin is a bundle. All the other bundles
are like this too.) Rule 1 above had listed "a PostgreSQL bundle and a MariaDB
bundle, each with one optional add-on"; the tool is now part of the bundle.

The rules adopted:

1. **A bundle is a service together with its companion tools.** A one-component
   bundle only repeats its component's row, so there is none any more. Every
   component bundle holds at least two components, and at least one of them is
   there for itself, not only as a prerequisite of another; a mail bundle is
   exactly its mail profile. A Go registry contract test refuses a registry that
   breaks this, and refuses the retired ids and name codes.
2. **The registry, in order.** Names are the same in English and Turkish except
   where a Turkish name is shown.

   | Id | Category | Name | Components, in install order | Notes |
   | --- | --- | --- | --- | --- |
   | `mail.core` | email | Core Mail (Temel Posta) | Postfix, Dovecot | mail profile `core-mail`; offers Spam-Protected Mail after it |
   | `mail.webmail` | email | Webmail | Postfix, Dovecot, Nginx, PHP-FPM, Roundcube | mail profile `webmail`; offers Spam-Protected Mail after it |
   | `mail.protected` | email | Spam-Protected Mail (Spam Korumalı Posta) | Postfix, Dovecot, Rspamd | mail profile `protected-mail` |
   | `db.postgresql` | database | PostgreSQL + phpPgAdmin | PostgreSQL, Nginx, PHP-FPM, phpPgAdmin | Nginx and PHP-FPM are prerequisites (rule 4) |
   | `db.mariadb` | database | MariaDB + phpMyAdmin | MariaDB, Nginx, PHP-FPM, phpMyAdmin | Nginx and PHP-FPM are prerequisites (rule 4) |
   | `web.nginx_php` | web | Nginx + PHP-FPM | Nginx, PHP-FPM | |

   The add-on bundles `db.postgresql.admin` and `db.mariadb.admin` are gone;
   what they installed is inside the database bundles. The mail bundles, their
   Spam-Protected Mail add-on and Nginx + PHP-FPM are unchanged. Every registry
   entry is drawn and counted as before (alpha.95 amendment, rules 1 and 3); there
   are six now.
3. **Ids kept, names changed.** `db.postgresql` and `db.mariadb` keep their ids, so
   a link to them still lands on the same bundle. Their name codes changed
   (`db_postgresql_phppgadmin`, `db_mariadb_phpmyadmin`), so a page never names an
   older Panel's one-component bundle as one with a tool: the page has no words
   for the retired codes and names such an answer by its components.
4. **Prerequisites fill a role.** The web server and PHP-FPM a database tool runs
   on are in the bundle for the role they fill, not for themselves. Where another
   installed component fills that role (Apache instead of Nginx), it serves, and
   the dialogue lists Nginx as "not needed", not as a conflict. Nginx + PHP-FPM
   names Nginx itself and stays blocked beside Apache, as before.
5. **A part the system does not offer blocks the whole bundle.** It is blocked with
   the existing typed reason, and a subset is never installed. phpPgAdmin has no
   pacman package, so on Arch "PostgreSQL + phpPgAdmin" is blocked and its chip
   says "phpPgAdmin is not available on this system" ("phpPgAdmin bu sistemde
   kullanılamıyor"); MariaDB + phpMyAdmin stays installable there.
6. **The installed bundle's dialogue says each part's own state; Verify is
   withdrawn.** This amends rules 3 and 4 above and rule 4 of the alpha.95
   amendment ("The installed chip holds Repair and Verify"). Each part says its
   row's own state and version ("Running · 1.26.3", "Installed · 5.2.1",
   "Stopped"; "Çalışıyor · …", "Kurulu · …", "Durdu"), from the same data the rows
   use, with the full package version as its tooltip, instead of "Already
   installed". The Verify button and its sentence are gone: it only read the
   components again, which the page's Rescan does. Close stays. Mail bundles keep
   Repair, which does real work (the profile install). An installed component
   bundle that is not verified shows the part that is not running in the row's red
   and says, in words, to start it from its row or to use Rescan at the top of the
   page, and that the window changes nothing on the server; it has no button of
   its own. The Panel no longer sends the `verify` action (it sends `none`); the
   page still accepts `verify` from an older Panel and draws no button for it.
   The page's reading of the answer now leaves out an id the answer does not
   carry (an add-on reference to an unknown bundle is dropped, and an add-on of a
   bundle that is not present is not drawn) instead of refusing the whole list.
7. **How a bundle installs is unchanged** (rule 4 above): one ordinary component
   install per missing part, in order, each after the previous one has ended with
   a fresh scan; the first failure stops the sequence with its reason and a
   Continue that installs only what is missing; nothing installed is removed.

What the owner's servers show, as the Panel derives it (from their alpha.100
screens; not measured on alpha.101). **Boston** (PostgreSQL and Nginx installed,
no PHP-FPM, no phpPgAdmin): "PostgreSQL + phpPgAdmin · Install"; its dialogue
keeps PostgreSQL and Nginx and installs PHP-FPM, then phpPgAdmin, one at a time.
The Dashboard's "Ready bundles" card counts the same chips: "None installed yet ·
6 can be installed". **Frankfurt** (everything installed): both database bundles
are installed and verified, and the card says "6 installed and verified".

**Where the bundles are defined.** In code, in `bundleDefinitions` in
`cmd/panel/bundles.go`, compiled into the Panel. They are not read from a file or
from the database and cannot be changed while the panel runs; changing a bundle is
a code change and needs a release. Moving them to a data catalogue later is
possible if the owner wants it; that is not decided here.

No schema change (44), no Agent change, no new Agent job kind or RPC, no change to
the updater or release recovery, so no native record.

What this does not authorise: no installed-server change, no licence-policy
change, no new store item, no WordPress or site-application bundle and no
run-time editing of bundles. Installed panels are still updated only by their
owner from the panel's own update screen.

Relation to the rest. D-015 item 2 stands: the catalogue is still the single
install address and a bundle installs catalogue components only. D-024: a bundle
that is not verified says which part, who acts (the administrator), the next
action (start the part from its row, or Rescan) and how it resumes. D-033: the
install sequence and its visible waiting are unchanged. D-036: the dialogue's
versions are the rows' versions. The removal dialog of the D-037 amendment names
the merged bundles (removing Nginx now leaves both database bundles incomplete).

Not done: nothing is measured on an installed server; whether bundles should move
to a data catalogue is left to the owner.


---

## D-033 · A screen that makes the owner wait shows that it waits, or says that nothing needs to be done

*October 14, 2026 (clock date) · Owner decision after the final setup step on the second installed server (Frankfurt, v0.1.0-alpha.84); applied in source to every waiting state found by a read of the web interface (37 states), with mounted tests and a mock browser (2026-10-14); partly measured in the eighteenth native record (set16, 2026-10-14 UTC: the Dashboard's Mail stacks card and the sidebar build id on Debian 13 and Ubuntu 24.04; the Components firewall banner and the "Needs attention" box with the Agent stopped on Debian 13 only); setup finishing and the update card's busy state not reached natively; nothing measured on an installed server*

On 2026-10-13 and 2026-10-14 the owner worked through the last step of setup,
"Verify the prepared server", on the second installed server. The final check
waited for the reverse DNS (PTR) name of the server's address. The owner had
already changed it at the VPS provider; the provider was still spreading the
change, and the public resolver asked still answered the old name from its
cache. The step read "Waiting for you", although the owner's part was done, and
nothing said that a change at the provider takes time to reach public DNS or
that nothing more was needed in CelikPanel. After "Check requirements again"
passed every check, the step read "Waiting for requirements" under a still
sentence ("These checks passed. Waiting for the server to confirm setup
completion."), with the buttons enabled and no motion; the run completed only on
its next 20-second pass. In substance, the owner's words: this confuses people;
they will not know what to do and will press things impatiently; if they must
wait, make them wait or tell them clearly to wait; nobody waits two minutes
while nothing seems to happen. The owner then set the rule for every screen.

The rules adopted:

1. A screen that makes the user wait either visibly waits (motion, with the
   controls that would interfere disabled) or says plainly that nothing needs
   to be done and that the page updates by itself. Where it can, it does both.
2. It finishes promptly. A wait that the owner's own action can end (a passing
   check) ends on that action, not on the next timer.
3. A wait has a bound. After it, the screen stops its motion and says honestly
   that the result is not known yet, that this is not a failure, who acts and
   what to check, and how work resumes; a known failure keeps its own wording.
4. A static waiting sentence beside enabled buttons is a defect.
5. A screen never says "Waiting for you" when the owner's part is already done,
   for example while a change made at a provider spreads. It names what it
   waits for (for instance "Waiting for reverse DNS") and how long that can
   take.
6. A screen that waits for a state it can read reads it again by itself while
   it is visible (reads only), instead of asking the owner to press a button.

Consequences. The web interface was read from source for every state in which
a page waits for the server, a background operation, a poll, a provider, the
licence or an external check: 37 states. 18 already met the rule, 16 were
changed on 2026-10-14, and 3 (the component operation lock after a lost
connection, the panel update card's readiness and a domain's spreading DNS
change) were decided by the owner the same day and changed after that. The
bounds chosen: 60 seconds for setup finishing, setup reconnecting and the
component lock; readiness re-read every 5 seconds while the update card says
the server is busy; a spreading domain checked every 30 seconds for at most 60
checks. The last step of setup now finishes on the owner's passing check. The
rows, the commits and the tests are in
[OPERATION-GUIDANCE](OPERATION-GUIDANCE.md), the two entries of 2026-10-14; the
release notes of v0.1.0-alpha.85 say which check rests on which code.

What this does not authorise:

- No polling that starts or admits a mutation. Every automatic re-read added by
  this decision is a read (GET); a re-read never starts a second change. The
  update card's and the spreading domain's re-reads run one at a time and only
  while the page is visible; the component install dialog's 5-second re-read
  and the setup page's reads do not check visibility (open; see the release
  notes).
- No new timer that runs when nobody is looking, and no unbounded wait shown as
  motion: the bound ends the motion and the notice says the result is not known
  (setup's and the component lock's reads go on after it). The install dialog's
  re-read runs while the dialog is open whatever the tab's visibility; that is
  a gap, not an authorisation.
- No "not known yet" in place of a verified failure or an unmet prerequisite:
  the three stay apart (D-024).
- No claim that every screen was measured. The audit was read from code; the
  native record reached part of it; nothing was measured on an installed
  server.
- No change to installed panels. Panel updates remain the owner's, from the
  panel's own update screen; no P0 item is closed or advanced.

Relation to the rest. D-024 requires every operation to show the current
reason, who acts, the next action and how work resumes, and to distinguish a
verified failure, an unmet prerequisite and an unknown result; this decision
applies it to waiting, and adds that a wait must look like a wait and end. D-032
(the panel looks by itself and does not ask the owner to press a button to learn
a state it can read) is why rule 6 reads again instead of asking. The
known-state rule is why the bound ends in "not known yet" and not in a negative
state. D-022 (the owner operates the server) is why the bounded notices say how
the owner checks the panel on the server rather than restarting anything.

Status. In source on the branch of the v0.1.0-alpha.85 candidate (written
before its publication on 2026-10-15), with mounted tests. The Components firewall banner, the Dashboard's
attention notice and Mail stacks card and the sidebar build id were measured in
the eighteenth native record. Setup finishing, the update card's busy state,
the component lock's reconnect and the spreading-domain checks were not reached
natively. Open for a native run, for an installed server and for the
visibility gating of the install dialog's and setup's reads.

*Note of 2026-10-15 (clock date): applied in v0.1.0-alpha.86 (written while it
was a candidate; published 2026-10-15).* No new decision is recorded for that candidate; what it does
follows this decision and D-032, and the text above is unchanged.

- Rules 1 and 3, after the owner's observation of 2026-10-15 during the real
  update of the second installed server to alpha.85: the update's own planned
  restart is shown as progress ("The panel is restarting as part of the
  update…", a spinner, the T+ counter), and the warning and "not known" wording
  come only after a bound (6 minutes from the update's start and at least 2
  minutes from the first lost read) or with a verified failure, which keeps its
  own wording. The update card and the corner notice no longer say the same
  thing twice. The bound rests on two lab updates (set16) and that one owner
  observation.
- Rules 1 and 3 after the native records set17 and set17b: inside the same
  bound neither the licence gate nor the session gate replaces that progress
  with its own hold (set17 D1), and the update's navigation lock covers the
  restart while its restart signal is active, at the latest 6 minutes from
  the start (set17b F1; the release preparer's decision of 2026-10-15, under the
  owner's standing instruction of 2026-10-13, an application of this rule; not
  an owner decision and not a new decision).
- The gap named under "What this does not authorise" is closed in source: the
  component install dialog's re-read and the setup page's reads now run only
  while the tab is visible (`web/src/lib/visibleReads.ts`), and the two re-reads
  of this decision that already stopped while the tab was hidden, the update
  card's readiness re-read and the spreading domain's re-check, now read once at
  once when the tab is shown again (rule 6); the other re-reads of this decision
  were not audited for this.
- With D-032: no server or page sentence asks the owner to rescan; a stored
  component record that cannot be verified is checked by the Dashboard, the
  Components page and a component's own page themselves, and a check held by a
  change resumes by itself while the tab is visible (reads only, one follow-up,
  no loop).
- Not a decision of its own: the username `system` is reserved
  (`USERNAME_RESERVED`), because the audit log now says that actor as "System"
  and a person must not be shown as the system (D-024: a screen says what is
  true). It is a validation rule, not a policy change; an existing row is not
  touched.

Status of the note: in source with component tests and a mock browser
(browser-inspection batches 13 to 15). Native records set17 (at `313f3eea2`)
and set17b (at `ecd2a5f08`) under `deploy/e2e/release-recovery/evidence/`
measured the calm restart; the lock change made after them (`7265d3c99`) and
the session gate's `PANEL_STARTING` path were not measured natively, and
nothing was measured on an installed server. The
[alpha.86 release notes](RELEASE-NOTES-v0.1.0-alpha.86.md) say which check rests
on which code. No P0 item is closed or advanced.

*Note of 2026-10-16 (clock date): applied in the v0.1.0-alpha.87 candidate (not
published).* No new decision is recorded for that candidate; the text above is
unchanged.

- Rule 1 and D-024 for the setup extension: the ready screen and Settings say
  what "Extend this server's setup" does and does not do before anything is
  pressed, and nothing starts before the review; while an extension runs, the
  Dashboard and Settings link to its progress ("Show its progress") instead of
  saying "nothing needs to be done" beside "Continue extending".
- Rules 1 and 4 after the native record set18: a started setup run names its
  own first step and says that the page updates by itself (O1); a pressed
  "Continue recorded operation" stays disabled as "Checking the same
  operation" until a read shows the operation moved on, at most 15 s (O3; reads
  only, the server answers a repeat of the same request without a second
  operation); an extension in draft is no longer shown as the completed run's
  "Completion could not be verified" (D1, a false alarm on a server that was
  ready).
- Rule 5 for a DNS secondary: the mail box names what it waits for (the record
  on the primary or at the DNS provider, with the exact record) instead of
  sending the owner to a DNS page that cannot create it, and the profile cards
  say "Waiting for the mail DNS record". The answer to "Check requirements
  again" says the same typed reason as the guidance.
- The Components mail dialog is locked while setup is asked (no silent exit,
  rule 1) and keeps a refusal in view.

Status of the note: in source with component tests and a mock browser (batch
16); set18b (at `6b363ac46`) observed D1, O1 and O3 fixed in a lab; the
secondary-DNS boxes were not reached natively; nothing measured on an installed
server. No P0 item is closed or advanced.

---

## D-032 · The panel looks by itself; it never asks the owner to press a button to learn a state it can read

*October 12, 2026 (clock date) · Owner decision after the update to v0.1.0-alpha.83; in source with component tests and a mock browser; the first step measured in the fourteenth native record (Debian 13, Ubuntu 24.04, 2026-10-12 UTC); the scan-age clock rule and the typed Agent answer at the scan and the admission check measured in the fifteenth record (Debian 13, 2026-10-13 UTC); the licence check's time left, the Agent answer in the shared error writer and the read banner (2026-10-13) component-tested, then measured on Debian 13 in the sixteenth record (set14c, 2026-10-13 UTC), where the licence-check defect found in the fifteenth record is gone*

The Dashboard's "System services" card said "Unknown. Component status is
outdated. Open Components to rescan." whenever the stored scan was older than
five minutes. The Components page already ran the cached, conditional scan the
Panel offers (`POST /api/v1/managed-services/scan?max_age_seconds=N`: one scan
at a time, cached, refused while a change holds the server's lock); the
Dashboard did not, so the owner had to carry out, by hand, a read the Panel
could make itself. After the update to v0.1.0-alpha.83 the owner decided on
2026-10-12: the Dashboard should check by itself, without wasting resources.

The rules adopted:

1. After a successful read, if the stored scan is missing or older than 300
   seconds, the Dashboard runs the conditional scan, at most once per
   five-minute window whatever the answer was.
2. Only while the tab is visible. A hidden tab runs nothing; a tab shown again
   checks once if a check is due.
3. Never with another check in flight. No interval timer that scans
   regardless; no retry loop after a refusal.
4. A refusal because a change is in progress (`server_setup_busy`,
   `service_operation_busy`) is shown as "a change is in progress" and sends
   nothing more. One follow-up runs when the existing readiness poll sees the
   server go from busy to ready (package operations only; after a setup
   refusal the next check waits for the window).
5. A failed check says "Could not check: <reason>" with the in-place button.
   The owner's press stays unconditional.
6. The card draws four distinct states (checking, the result, could not check,
   held) and the "Needs attention" sentence follows them.

The resource bound is part of the decision, not an afterthought: at most one
conditional scan per five minutes for a Dashboard someone is looking at, none
for one nobody looks at. Measured on two lab guests (fourteenth record, first
step): a stale load sent exactly one scan, a fresh load none, a page kept
visible one more not before 300 seconds, a hidden tab none for five minutes
and one when shown, and one check cost the Agent about 0.2 to 0.4 seconds of
CPU (an estimate from two 47-second windows).

The clock rule. The server states a scan's age (`scan_age_seconds`, on the three
managed-services answers, measured on the server's clock when the answer is
written); the page dates the scan as the moment it received the answer minus
that age. The browser never compares its own clock with the server's. The rule
has two steps because the lab found the defect in the first version: with the
guest's clock one to two seconds ahead, the card showed "Not checked in the last
5 minutes" for 20 to 29 seconds after every check. The first step (the earlier
of the stamp and the moment of receipt) removed it for a server ahead; the
second (the server's age) also removes it for a server more than five minutes
behind. An older server without the field keeps the first rule. A negative age,
or one that is not a finite number, is not taken as fresh. Measured (fifteenth
record, Debian 13, guest 1.5 s ahead): one conditional scan on a stale load and
the card going straight to the result, with no "Not checked in the last 5
minutes" frame. The case of a server clock behind was measured later, in the
sixteenth record (set14c, Debian 13, 2026-10-13, clock seven minutes behind):
one conditional scan, then the result to the end of 90 s.

The same rule holds for the licence check, which the lab showed breaking it.
`/api/v1/license/access` gave the end of access (`valid_until`) as a time on
the server's clock, and the page refused a positive answer whose end was not
after the browser's clock; with the server's clock more than about a minute
behind, no Panel page loaded (fifteenth record, defect D2: 418 s behind, the
check read 19 times in 90 s). The server now states the time left
(`valid_for_seconds`: `valid_until` minus the server's clock when the answer is
written, never below zero; `valid_until` stays for older pages), and the
browser takes the end as the moment it sent the request plus that time, so it
never compares the two clocks and never ends later than the server allows.
Without the field the absolute rule applies. The fix was measured on Debian 13
in the sixteenth record (2026-10-13): with the guest clock seven minutes behind
and seven minutes ahead of the browser's, the page mounted and read the access
four times in 90 s, as with a correct clock.

The typed-error rule. A Panel that cannot open its own Agent's local socket
answers `503 AGENT_UNREACHABLE`: the reason, that nothing was changed, who acts,
the next action and how work resumes (D-024). The answer is written once, in the
shared server-error writer, so it applies to every handler that reaches that
writer with a failed dial of the Agent's socket as the only cause (the scan, the
admission check (`beginServiceMutation`) of the mutation handlers (nine call
sites besides the scan handler at commit 68b79b372), the firewall read, the
system check), not handler by handler. It is for a failed dial only. A
connection that broke after a request was sent is an unknown result and keeps
the generic path; an unknown result is never rewritten into a verified failure.
Neither is a joined error, such as a failure together with its compensation: an
earlier part may have changed something, and "nothing was changed" would be
false. The one join accepted is the platform-identity sentinel with its cause,
because that read runs before the call it guards. The DNSSEC status, the update
status and start, domain deletion and mail password rotation stay on their own
typed paths. An ordinary failure still answers `500`. Measured at the scan and
at the admission check (fifteenth record) and at the firewall read, the system
check and a restart request (sixteenth record, `503` with only `code` and
`error`, nginx unchanged). Open: a handler that changes something and later
fails on a separate dial that is not joined to the earlier outcome would still
claim that nothing was changed.

The read-banner rule. A failed read never claims an action. A screen that
could not read a state says that it could not read it and why, with the next
action; it does not say that an action may have completed unless one was sent.
The cause is kept: an action that was sent and not confirmed keeps its "may
have completed" wording, and a later failed read never replaces it. Found in
the lab on the Components page with the Agent stopped (fifteenth record,
observation O2) and applied there; measured in the sixteenth record (the read
banner in English and Turkish, the action wording absent, gone after one press
of Rescan once the Agent was back). No other screen is converted by this rule.

What this does not authorise:

- No background timer that runs when nobody is looking. This decision adds no
  scan on a schedule for the sake of a fresh card; the Dashboard looks when a
  person has it open and visible.
- No polling that starts a mutation. The check is the read-only scan the
  Components page already offers; a poll that sees a state never starts a
  change, and the readiness poll's one follow-up is that same scan.
- No other card, page or screen is converted by this decision. The mail-stack
  card on the Dashboard keeps its own stale text and its "Open Components to
  rescan" button until a change of its own; screens that still read the server
  the old way stay on the list in the release notes.
- No claim that the foundation is finished: no P0 item is closed or advanced.

Relation to the rest. D-024 requires every operation to show its reason, who
acts, the next action and how work resumes, and to distinguish a verified
failure, an unmet prerequisite and an unknown result; this decision applies it
to the component check and to the unreachable Agent. The known-state rule (a
failed or missing read is not drawn as an empty or negative state) is why the
card has "could not check" and "held" as states of their own and why the stale
state is not collapsed into "Unknown". D-022 (the owner operates the server;
the Panel detects and reports) is why the Panel says how the owner checks the
Agent instead of restarting it. Panel updates remain the owner's, from the
panel's own update screen.

---

## D-031 · A site configuration file the owner changed is kept and named, never overwritten by a render

*October 10, 2026 (clock date) · Owner decision on a finding of the read-only audit [SITE-CONFIG-OWNER-EDITS-2026-10-10](audit/SITE-CONFIG-OWNER-EDITS-2026-10-10.md); design decided; first step measured in the tenth native record (two findings corrected 2026-10-11); the apply action and the one precedence (2026-10-11) and the second step (pool, application unit; 2026-10-11) component-tested and mock browser, native measurement pending*

The audit read, from source only, that the Panel's start renders every hosted
site's nginx configuration again and writes it with no comparison, no digest
and no backup (`cmd/panel/cert_startup_reconcile.go`, `internal/services/
nginx_generator.go`); every other render path (certificate, settings, hosting
type, PHP version, import) does the same. By that reading an owner's added
`location`, changed directive, own include or replaced file is overwritten
without a word, and a removed site file is recreated at the next start. The
published v0.1.0-alpha.81 and v0.1.0-alpha.82 both do this. This is the
behaviour D-022 forbids: the Panel must detect the owner's change, not
overwrite it. Nothing of this was measured on a real system when the decision
was taken; the eighth native record measures the published behaviour first.

The owner decided on 2026-10-10 that the correction enters the next release:

1. Every file the Panel writes for a site carries a header with the digest of
   its body, and the Panel keeps a ledger row per file (schema transition 43 →
   44, migration 044, table `managed_site_files`; D-025 names it).
2. Before every render the Agent classifies the file on disk: absent, managed
   and unchanged, owner-edited, foreign, unreadable, unknown origin. Only an
   absent or managed-and-unchanged file is written; identical bytes are not
   rewritten and nginx is not reloaded for them.
3. Any other file is kept. The site is shown as "configuration edited by the
   owner" with the difference; the Panel's pending text is held beside the
   file; the owner chooses "keep mine", "take CelikPanel's" (with a dated
   backup) or merges by hand. A site file the owner removed is not recreated
   at start; the Panel offers to recreate it.
4. The template gains a per-site include directory as the supported place for
   the owner's additions, so a later render never has to touch them. (Step 1b,
   2026-10-10, planner's decision:) it also gains a second, Panel-owned
   include directory (`/etc/nginx/celikpanel-managed.d/<domain>/`) that holds
   the ACME HTTP-01 location instead of the vhost. Whether a kept file lets
   the validation run is measured, not read from its text (second round, same
   day): the Agent puts a probe file with a random name and body in the
   challenge root, asks nginx on 127.0.0.1 port 80 (since 2026-10-11: on each address the
   kept file's port-80 blocks of the name listen on) for it under every name
   the certificate validates, and removes it; ready means every name served it.
   On a ready kept file (with an unchanged challenge file) a certificate is
   issued or renewed without touching the file, and the
   certificate the file names, if any, is served until the owner takes
   CelikPanel's text or updates the certificate lines (`waiting_for_owner`);
   on a kept file that is not ready, issuance stops with a typed refusal
   before anything is requested, and renewal waits for the owner instead of
   failing; a probe nginx does not answer is unknown, never "not ready". The
   alias-certificate path follows the same two paths.
5. Writes preserve mode and owner, refuse symlinks, and one site's failure
   does not fail the others; the start line counts written, unchanged, kept,
   foreign and unreadable separately.
6. Files written by alpha.81 and alpha.82 carry no digest. The migration
   renders each site in memory with the frozen templates of those releases
   and compares: a byte-equal file is adopted, any other file is "unknown
   origin" and is never overwritten. The honest first state after the update
   is "N adopted, M left alone because they differ from every known
   CelikPanel text", and the Panel shows it. (Tenth native record, 2026-10-11,
   planner's decision:) an adopted file is rendered again as this release's
   text and gets the header — it is not the old bytes plus a header; the
   owner's additions could not have been in it, because it was byte-equal to
   CelikPanel's own text. `adopted_from` names the newest release whose text
   matches (alpha.82 when both match). Measured in the tenth native record.
7. An automatic return to an older release loses the protection; the release
   notes say so.
8. (Second step, 2026-10-11; in source, component-tested and mock browser;
   not measured on a real system.) The site's PHP-FPM pool file (kind
   `php_pool`) and its application unit (kind `app_unit`) follow points 1-7.
   The pool is INI, so its header is a `;` comment line
   (`; celikpanel-render v2 sha256=…`); the unit's is
   `# celikpanel-render v2 sha256=…`. Each is classified before every write
   (pool: save, PHP version switch, creation, deletion; unit: hosting change
   and its compensation, removal; both: the page's choices); identical bytes
   are not rewritten and nothing is reloaded for them; a kept file gets
   CelikPanel's text beside it and the operation answers
   `409 SITE_CONFIG_OWNER_EDITED` with `reason` the kind and `detail` the
   state. Writes go through a temporary file and one rename, keep mode, owner
   and group, and refuse a symlink. PHP-FPM has no include for a pool, so the
   owner's directives are supported in place: a pool with CelikPanel's header
   and lines CelikPanel does not render is owner-edited and kept, and no save
   or switch drops them. The unit's owner extension point is its systemd
   drop-in directory (`<unit>.service.d/*.conf`), which CelikPanel never
   writes or removes. No database column holds a pool's process-manager
   values: CelikPanel's pool text takes them from the save, else from its own
   held text beside the file, else from the file's values for those keys,
   else the defaults; the identity lines are always the site's. Adoption: a
   headerless pool byte-equal to the frozen alpha.81/82 form for the site's
   identity and the values it carries, or a headerless unit byte-equal to
   the alpha.81/82 unit for the same request, is adopted (`adopted_from` the
   newest matching release) and written as this release's text at its next
   write; any other is unknown origin. Pools and units are not rendered at
   the Panel's start (no new start-time write); the start line counts vhosts
   only. No schema change: migration 044's `kind` column already names
   `php_pool` and `app_unit`. The Configuration file page lists every file of
   the site with its own actions; the Domains row shows the worst state
   across the files (the vhost first on a tie).
   Limits, stated plainly (2026-10-11): the Panel's two pool calls are bound
   to the Panel's own build, and an Agent that does not have them gives an
   error with nothing written (no fallback to the older pool calls;
   `MigratePHPPool` is gone from the Panel's policy); the unit's two calls
   are bound to the Panel's build the same way (corrected 2026-10-11). Site
   creation writes no pool or unit ledger row: a row begins at the page's
   first read or at the first save, switch or unit change, so a pool nobody
   has read is not in the Domains list. Corrected 2026-10-11 (component-tested
   and mock browser; not measured on a real system): a file the Agent did not
   read (`unknown`) outranks CelikPanel's unchanged text in the worst-state
   rule (every observed problem still outranks it), so the Domains row says a
   configuration file could not be read; a service refusal answers `restore:
   confirmed | unconfirmed`, and unconfirmed (the Agent's "rollback
   incomplete", or no restore fact) says the server administrator checks the
   file on the server instead of saying it was put back. The two adoption bases differ: the pool of alpha.81 listened
   on `/var/run/php/php<v>-fpm-<pool>.sock` owned by `www-data`, alpha.82 on
   this host's socket owned by the web server's account; the unit of alpha.81
   wrote `User=` as asked, alpha.82 the web server's account that exists on
   the host.

Not done until measured natively on Debian 13, Ubuntu 24.04 and Arch: the
audit's §9 cells (owner edit kept and named at start and at every render path;
immutable file does not stop other sites; update, return and forward again;
database restore from before the header; keep-mine then certificate issue;
take-CelikPanel's with its backup; owner include directory survives renewals
and switches; removed file not recreated; pool directives survive). The PHP
pool and the application unit follow in a second step under the same rule
(point 8, in source 2026-10-11; its cells are in set10's harness as sections
m, n and o, not yet run on a guest).
Tenth native record (set10, 2026-10-10, `cd46ca595`, Debian 13, Ubuntu 24.04,
Arch): every cell held but two. Corrected 2026-10-11 (component-tested; not
measured): the owner include directory is created 0755 explicitly (measured
`root:celikpanel 0750` under the Agent's `UMask=0027`), and so is CelikPanel's
own; a locked file (`chattr +i`, EPERM) is answered `unwritable` by the
domain's page, its list row and badge alike, with the system error, the
protection measured read-only, the text CelikPanel wanted to write and "check
again" (the page answered `managed_unchanged` with no reason). The ledger keeps
migration 044's state set (`unreadable` + `write_refused`); no schema change.
New and recreated vhosts are `root:celikpanel 0644`; kept and taken files keep
the owner's mode, owner and group (measured).
Second correction (2026-10-11; component-tested and mock browser; not measured
on a real system): the page's read is the "check again" and records what it
found on a refused replacement's ledger row. The same bytes with no protection
measured: CelikPanel's own text is `managed_unchanged` with the reason
`protection_removed`, and the page offers "Write CelikPanel's text now"
(`POST …/site-config/apply`, both digests, request identity, administrator
only). It writes this one site's held text, keeps no dated copy (the file is
CelikPanel's own earlier text; nothing of the owner's is in it), and an nginx
refusal puts the previous bytes back. Still protected: `unwritable` stays; other
bytes: the file is recorded as it is and never written. One order decides what
the row, the badge and the page show when two facts hold for the same bytes:
unreadable, unwritable, missing, kept (owner-edited, foreign, unknown origin),
then CelikPanel's unchanged text (`services.SiteFileShownState`). An
owner-edited file whose "take" was refused shows both facts, and "take" is
answered `409 SITE_CONFIG_UNWRITABLE` before anything is written while the
protection stands, and a refused "take" leaves no dated copy (the Agent
measures the protection before the copy and removes the copy if the
replacement is refused anyway; vhost and pool tested, the unit shares the
pool's code, set11 cell B4; component-tested; not measured on a real system).
No schema change.
Open in the probe of the second round (2026-10-10), the first answered: (1)
a port-80 block bound to one address was not reached from the loopback —
answered 2026-10-11 (set11 cell A6; component-tested; not measured on a real
system): the probe asks the addresses the kept file's port-80 blocks of each
name listen on (`internal/services/managed_vhost_listen.go`); an address that
is not this server's is unknown and the certificate request goes ahead,
`validation_probe` `not_run`, journal only, no schema change, the 503 stays
for local causes; the rest are still open and none is fixed: a failed
`nginx -t` or reload at the Configuration file page's read is reported as
`challenge_failed`; a missing or unreadable file writes no reason, so a
renewal it stopped waits until the next completed renewal; (fixed 2026-10-11:
the SSL tab says "expired N days ago" instead of negative days); no real
hosting-customer session was
drawn; a ready probe does not show that DNS or the firewall lets the
certificate authority in. The probe has never asked a real nginx.

Corrections from the twelfth native record (set12, 2026-10-11; two findings
measured in the twelfth record, corrected in commit `816b8dd60`, the new code
component-tested; not measured on a real system): a pool or unit the Agent did not answer for is recorded on its
ledger row as an observation (`unknown`, `not_read` or `agent_does_not_report`
with the error class, no digest changed), so the Domains row says it could not
be read, until the next answered read; a row with a known problem keeps it.
After a put-back, a service that was not running before is left alone
(`service: not_running`), and otherwise the service's own check (`php-fpm -t`,
`nginx -t`, `systemd-analyze verify` for a unit) runs on the put-back
configuration first: refused, the service is not reloaded and keeps the
configuration it loaded before (`restore: confirmed`, `service: not_reloaded`,
`config_refused_elsewhere`, the line for an administrator); passed, it is
reloaded; every such answer carries `service_active`. The application unit's
start command no longer depends on a login shell (`/bin/sh -c` with an explicit
`Environment=PATH=` naming the Panel's Node directory; observed: a bare `node`
exited 127 under `/bin/sh -lc` although the PATH line was there, the login
profile resetting it is an inference, and the new unit has not run on a real
system); the earlier releases' unit texts stay frozen for adoption. The PHP
version switch's removal of the source pool and its clean-up still reload
without the check. The page's answer to a choice scrolls into view. No schema
change.

---

## D-030 · The Panel's secure-connection rule covers the Panel's own host name only

*October 10, 2026 (clock date) · Owner decision; in source with a component test; read on the running Panel on three platforms and, as reported, on two installed servers (2026-10-11 note below); the browser's replacement of the old rule not measured*

Over HTTPS the Panel sent `Strict-Transport-Security: max-age=31536000;
includeSubDomains`. A browser that had opened the Panel at a name therefore
refused plain HTTP on every name under that name for a year. Those names are
not the Panel's: the owner may serve them elsewhere, without a certificate, or
not through CelikPanel at all (D-022). Hosted sites never carried
`includeSubDomains` or `preload`; a test already forbade it there. No decision
record covered the Panel's own header.

The owner decided on 2026-10-10 that the rule is limited to the Panel's own
host name and that the change enters v0.1.0-alpha.82. The Panel now sends
`Strict-Transport-Security: max-age=31536000` over HTTPS and nothing over plain
HTTP. `preload` stays out.

What an owner sees: nothing on the Panel itself. A browser replaces the rule it
holds for the Panel's host with the one it reads on its next visit to the
Panel, so the wider rule ends for that browser at that visit; a browser that
does not open the Panel again keeps the old rule until its year runs out, and
the owner can clear it in that browser's own settings. This was not measured in
a browser; it is how the header is specified (RFC 6797, section 8.1).

Not changed: the header's duration, the Panel's other security headers, hosted
sites' headers.

*Note, 2026-10-11 (what has been read since).* (1) The sixth native record
(`deploy/e2e/release-recovery/evidence/set6-20261009/README.md` and
`part-b-headers.md`) read the header from the running Panel on Debian 13,
Ubuntu 24.04 and Arch: the candidate answered exactly `max-age=31536000` on
every HTTPS answer read (pages, assets, public and signed-in API answers,
401/403/404 answers, the preflight), and the published v0.1.0-alpha.81 before
the update answered `max-age=31536000; includeSubDomains`; guest-local reads on
disposable lab guests. (2) After the owner updated the two installed servers on
2026-10-10, an external read of each showed the narrowed header; this is
reported by the planner's own read and is observed on installed servers, not
recorded in a file here (see the entry in `docs/RESILIENCE-CONTRACT.md`,
"Publication of v0.1.0-alpha.82 …"). **Not measured:** a browser replacing the
rule it holds with the narrowed one on its next visit (the paragraph above is
still the header's specification, RFC 6797 section 8.1, not an observation);
a browser that keeps the old rule; a phone or other browser; a third-party
client of the Panel. No decision, code or schema changed with this note.

---

## D-029 · Every state-changing request carries one identity; a replay never runs twice

*October 10, 2026 · Planner decision on the owner's standing direction (D-024: a lost reply never starts a duplicate mutation; D-025 invariant 4); batch 1 in source with component tests, native cell pending*

A real-browser inspection measured that when a connection is reset while a
state-changing request is being sent, Chrome sends the POST again by itself:
one click reached the Panel three times. A read-only inventory of the source at
`7a64bda91` then classified about 115 state-changing routes: 12 are harmful when
they run twice, 7 could not be classified, about 38 leave the state right but
report it wrongly, and about 58 are safe. The subsystems that already carry a
request identity in the body and keep an operation row (component install, mail
profile install, panel certificate, server setup, DNS engine switch, panel
update) were safe.

A second hazard sat under the first. Most handlers passed the connection's
context to the Agent call and to the database writes after it. A reset cancels
that context: the Panel returned while the Agent kept working, so the host was
changed and the Panel never recorded it, and the replay then ran against that
torn first attempt.

**Decision.** One mechanism for every state-changing request that does not
already name itself.

- *Identity.* The header `X-CelikPanel-Request-Id`: 32 lowercase hexadecimal
  characters, the product's existing operation-id format. The web interface's
  one fetch interceptor adds it to every non-GET `/api/` call whose body does
  not already carry `request_id`; one identity per user action, a new click is
  a new identity. It is never sent to another origin.
- *The guard.* Inside the authenticated chain, in front of the router, backed
  by the table `request_identities` (migration 43). A row holds the identity,
  the acting user, the method and route pattern, the SHA-256 of method, path,
  query and body (never the body itself), the status `running`, `done` or
  `interrupted`, the answer's status code and, when it is kept, its content
  type and body, and its creation and expiry times. Rows live 24 hours and are
  removed by the hourly sweep.
- *First arrival.* The row is written as `running`, and the handler runs on a
  context the connection cannot cancel, bounded by the route's own time limit.
  Its answer is stored, then sent. A handler never starts without its row.
- *Replay.* The same identity with the same actor and the same hash is answered
  from the row, byte for byte. While the first arrival is still running the
  replay waits up to 20 seconds for it, then is answered `409
  REQUEST_IN_PROGRESS` with the same identity.
- *Reuse.* The same identity with another body, path, query or actor is `409
  REQUEST_ID_REUSED`; nothing runs and the first answer is not shown.
- *Interrupted.* A row found `running` when the Panel starts, or whose handler
  failed unexpectedly, becomes `interrupted`. Its replay is `409
  REQUEST_OUTCOME_UNKNOWN` with the next step (check the current state); the
  request is never executed again under that identity.
- *Secrets.* An answer that carries a one-time secret, and any answer larger
  than 64 KiB, is not kept: only its status code is. Its replay is `409
  REQUEST_COMPLETED_RESULT_NOT_RETAINED` (the change was made; its one-time
  result was shown only once), or the same code with the reason `failed` when
  the first attempt ended with an error.
- *Not wrapped.* Routes that carry `request_id` in the body and keep an
  operation row; routes that carry a settings version, in this batch.

**Scope of batch 1.** Exactly eight of the harmful routes, each with its
confirmed outcome:

| Route (POST) | Running twice, or cut by a reset | Time limit |
|---|---|---|
| `domains/{id}/backups/restore` | the Agent had no lock: two or three restores extracted into the same document root and imported the same database side by side | 40 min |
| `import/cpanel/apply` | the first import stopped between two steps with the site half imported (the domain created; files, mail, DNS or databases not), and the replay was then refused as "domain already exists" | 2 h |
| `domains/{id}/ssl/letsencrypt` with reissue | every arrival forced another issuance against the certificate authority's duplicate-certificate allowance | 25 min |
| `domains/{id}/backups` | no job key was sent, so the Agent took no job lock and built one archive per arrival | 35 min |
| `database-servers/{id}/admin-account` | a new password per arrival; two arrivals interleaving, or a reset between the engine and the record, left the engine on one password and the Panel holding another | 12 min |
| `vpn/peers` | new keys and a new address per arrival | 10 min |
| `database-servers/{id}/databases` | MariaDB accepts the second `CREATE DATABASE IF NOT EXISTS`; the replay then failed on the existing record and its compensation dropped the database the first request had created and recorded | 15 min |
| `domains/{id}/databases` | a reset between the Agent's create and the Panel's record left a database on the engine that the Panel does not list | 15 min |

Beside the guard: the Agent refuses a second restore of a domain while one is
running, before it reads or writes anything (`409 BACKUP_RESTORE_IN_PROGRESS`),
so two restores cannot interleave even without the header; a manual backup
carries its request identity as the Agent's job key; changes of one database
server's own account run one at a time. The admin-account and VPN peer answers
are never stored; the database route's answer is not stored when it carries a
password minted by that request.

On these eight routes a request without the header is refused with `428
REQUEST_ID_REQUIRED` before the handler: a page opened before the update must
not be able to run them unprotected. On these eight the web interface asks once
more for a lost answer, after a short delay and with the same identity, and
uses that answer. A refusal the Panel itself sent is not a lost answer,
whatever its status. When the second asking brings no answer either, the screen
says that the result is not known and that the answer was asked for again,
reads the state again and keeps its changing controls off until that read
answers. Every other route behaves as before, with or without the header.

**Not covered yet, deliberately.** The other harmful routes found by the
inventory (service and application restart, plans, enrollments); the about 38
routes that leave the state right and report it wrongly; the 7 routes the
inventory could not classify; the version-token routes. The guard is general;
each of these is a later batch with its own verification of the bad outcome.

**Consequences.**

- A page that was open during the update is refused on the eight routes until
  it is reloaded. Release notes say so. A client that is not the web interface
  sends the header itself.
- Leaving or closing the page no longer cancels these eight changes: once
  accepted, a restore, import, issuance, backup or database change runs to its
  end or to its time limit.
- One-time secrets are not retained, so a lost answer that carried one cannot
  be shown again: a VPN peer whose configuration did not arrive is revoked by
  its existing delivery receipt and a new peer is created; a minted database
  password is set again. The screens say so in place. The Panel has no control
  that sets a database user's password yet, so that one is done on the engine.
- The table is not owner data. An older Panel refuses to start on a ledger that
  carries entry 43; rollback is the pre-update snapshot restore, as for every
  earlier migration, and losing the table at restore is harmless: a replay of a
  forgotten identity is then a first arrival.
- A manual backup now reads the manifests of the domain's existing archives to
  find one this job already published, as a scheduled backup always has.

**Evidence state.** Component tests of the guard, of each of the eight routes
through the product's dispatcher with a fake Agent (three sends in a row and
three at once: one effect, the same answer), of the detached context, of the
Agent's restore lock and of the backup job key; web tests of the header, the
second asking and the texts. A real Chrome against a loopback mock that keeps
the guard's contract: with the connection really reset, one click arrived up to
10 times under one identity and each change was made once (resilience contract,
entry of 2026-10-10). Not yet measured: a real browser re-sending a request
against a real Panel, a Panel restart in the middle of one of the eight, and
the real services. Installed servers were not touched; the decision
does not authorise updating them.

---

## D-028 · The fresh paired PowerDNS primary is offered, within the measured envelope

*October 1, 2026 · Planner decision on the owner's standing direction (D-026 addendum: open the gate when row 6 has native evidence)*

Until today the Panel and the Agent refused the first install of PowerDNS
as the paired primary on a server with no DNS engine
(`pdns_primary_switch_paused`), because its V3 journal had no native
recovery evidence. Every acceptance run since 2026-09-30 was made on a
branch that flipped the two gate constants; the main line stayed closed.

**Evidence.** Kill-matrix batches 8r, 9 and 12 (cuts before and after the
first start, with one zone and with zero zones, forward recovery, pre-start
rollback, owner edits held, a parentless deletion resumed after the owner
enrollment, management-disabled reboots) and pair runs 5 and 7 (the whole
product flow on two CelikPanel servers: setup, zone add, record edit, zone
delete with the product's proof after the owner enrollment, re-add, reboot
with the panel off, management return) passed on the acceptance branch with
no harness workaround. The same runs found and closed eleven product defects
along the way; each is in the acceptance register with its evidence.

**Decision.** The two gate constants are `true` on the main line. What is
offered is exactly what was measured: PowerDNS as the paired primary on a
server with no DNS engine, Debian 13 amd64, the measured PowerDNS package
version, enforced by the Agent's host-profile and package-version
preflights. Every other PowerDNS paired-primary manifest stays refused; a
serving BIND source keeps the D-026 refusal
(`bind_source_pdns_switch_unsupported`); nothing changes for BIND or for a
PowerDNS secondary.

**Limits stated.** One run per cell or topology, disposable guests, orderly
reboots, a test-only license build (D-027), a laptop host. The daemon
re-stamp admission, the proof time-out code and the composite owner-edit
code are in source with component tests and did not occur natively. The
step-time bounds of the native proof are single samples. Installed servers
were not touched; the decision does not authorise updating them (the owner
starts every panel update from the panel's own update screen).

---

## D-027 · Acceptance runs use a test-only fixture license; license policy is unchanged

*September 30, 2026 · Owner decision*

Product-flow acceptance on disposable guests must drive the real Panel: setup
wizard, domain screens, both servers running CelikPanel. The Panel refuses
almost every authenticated route until a license is active and re-verifies an
active license against the license service about every minute; the
disposable guests are offline. The owner chose a test-only build over using a
real key on the experiments or skipping the product flow.

**Decision.** A build tag, `acceptance_license`, compiles a seam that accepts
exactly one fixture license labelled "ACCEPTANCE FIXTURE — NOT FOR
PRODUCTION", only on a guest that carries the e2e fixture marker, verified
locally, with every connection attempt to the license service refused and
counted. Ordinary builds compile a stub with no fixture, no alternate
verifier and no environment or file lookup, so an ordinary binary cannot be
switched into acceptance mode at run time; tests prove this from the sources
and from the built binaries. Release packaging refuses any artifact built
with the tag or containing the fixture text. The acceptance archive is
produced only by an explicitly named script and is marked not a release.

This does not change license policy, the verification window, or any
behaviour of a customer build. A run made with the fixture never counts as
evidence about license behaviour. Known limit: the License screen shows state
and expiry only, so on screen the fixture is not distinguishable without a
web change; the API payload, the logs, the evidence and the panel version
are.

---

## D-026 · DNS engine recovery: refuse the unrecoverable switch, accept same-operation recovery for first installs

*September 29, 2026 · Owner decision on the handoff's item 1 (DNS recovery contract)*

A read-only audit of the source at `e9d1019d` bound every DNS engine mutation
the product can start to one of three states: passed with native evidence,
named gap, or unsupported. One path fell into a fourth, unacceptable state:
switching a **serving BIND** source to a **PowerDNS** target on a standalone
host or a paired secondary was still reachable from the engine card. It wrote
the legacy V1 journal, its only recovery was the Agent's own inverse, the V4
pre-start inverse and its owner CLI had no producer wired to them, and no
native interruption trial existed. The existing gate covered only the paired
primary.

**Decision 1.** The BIND→PowerDNS engine switch is unsupported in this
release, for every topology. Panel preview/commit, server setup and the Agent
RPC refuse it before any mutation with the blocker
`bind_source_pdns_switch_unsupported`, and the screen states the reason, that
BIND keeps serving, and the next action (install PowerDNS on a host without a
DNS engine, or wait for a release that names the switch as supported).
Fresh PowerDNS installs, PowerDNS→BIND switches, PowerDNS adoption and
reconfiguration, BIND reinstall and every installed server are unchanged.
Reopening the gate requires a wired producer, a pre-start and post-start
Agent-independent inverse, and native interruption evidence — not removing the
check.

**Decision 2.** For a first engine install (BIND or PowerDNS, any role) the
prior state is "no DNS engine"; there is no owner data to restore. Recovery
through the Agent on the same operation identity at the next start, with the
read-only status command and the screen naming the owner's next action, is the
accepted contract for this release. An Agent-independent owner CLI is not
required for first installs. What remains for item 1 is evidence, not code:
native interruption cells for fresh standalone PowerDNS, fresh paired
secondary (BIND and PowerDNS), and the post-start cut on fresh BIND.

**Addendum, same day.** Native cells found two product defects that
component tests had not (a first-install rollback and the V2 BIND switch
inverse both rejected a target that had never started) and one recovery dead
end (owner inverse commands refused the Agent's own deliberate release).
All three were fixed and re-run natively. The owner chose to run the
post-stop PowerDNS→BIND cells before closing; they passed, and roadmap item 1
is closed with the limits named in the
[acceptance register](DNS-RECOVERY-ACCEPTANCE.md). For a released job the
owner command does not rewrite the finished ledger entry; what the owner is
shown is computed at read time. Closing item 1 does not close P0.4.

Decision 1 narrows promised scope and is stated in release notes as an
explicit unsupported operation, not silently. Neither decision changes an
installed server, the license policy, owner-operated recovery or user-only
installed-panel updates.

---

## D-025 · Resilience is a core contract, not an incident patch

*September 14, 2026 · Owner-requested constitutional audit; required direction, implementation open*

The owner requested examination of the system's constitution after repeated
Frankfurt failures. The source audit found coupled failure boundaries, ambiguous
evidence roles, incompatible artifact readers and recovery dependent on the
candidate it must recover. Similar failures had already been documented on
August 26. A passing reproducer did not establish whole-lifecycle reliability.

**Direction.** Preserve the Panel/Agent privilege split, while separating normal
mutation authority, state observation and recovery availability. Stop unsafe
actions at their affected boundary; preserve working native services and a narrow
authenticated recovery path within the supported host fault model. Unknown status
must not become false license expiry, missing service or completed work.

Every durable artifact needs one versioned producer/reader/restore contract.
Every mutation, including metadata normalization, needs read-only discovery,
explicit durable checkpoints, bounded continuation/compensation and terminal
proof. Recovery must be able to outlive ordinary Agent and candidate startup.
A separately versioned minimal executor and manifest protocol are proposed;
existing exact retained-release rollback rules continue until that replacement
is implemented and verified. No unrestricted shell or AI-based evidence repair.

The [resilience contract](RESILIENCE-CONTRACT.md) contains source evidence,
invariants, the open P0 acceptance register and implementation sequence. The
[constitution](../ROADMAP.md) now makes survival, truthful state and fault evidence
mandatory. PR review must name affected contracts, migration/recovery behavior
and exact test evidence. Full support requires disposable native upgrade and
automatic-restore drills, including failure during recovery itself. An emergency
incident correction may ship with explicit scope and limitations; it does not
close the architectural work or justify unrelated feature expansion.

This decision changes no installed server and does not implement the new
executor, recovery UI, schema split or independent workload renewal. Alpha80 is a
scoped BIND/recovery correction. D-021, D-022, D-024, owner-operated recovery and
user-only installed-panel updates remain binding. Older one-binary, only-panel
and historical bootstrap wording cannot override these boundaries.

---

## D-024 · Every operation explains the current state and next action

*September 13, 2026 · User-approved requirement; product-wide implementation and audit incomplete*

Progress, waiting, failure and recovery guidance apply to every managed program,
service, runtime and integration. The user must see the current reason, who needs
to act, the concrete next action and how the operation will resume before a long
list of steps. DNS topology, license, credentials, permissions and provider access
are examples of dependencies that require specific guidance, not silent waiting.

Separate verified failures, unmet prerequisites and unknown results. Preserve
known failures while checking their outcome, retain the accepted plan and exact
operation identity, never expose secrets and never start a duplicate mutation
because a page refreshed or a reply was lost. Every supported adapter must supply
stable state/error codes and actionable translated messages for its own cases.

The current source work provides setup DNS role context, panel-license guidance
and generic service-error handling. Third-party license adapters and an exhaustive
review of all operation screens are not implemented by this decision. The
[operation guidance contract](OPERATION-GUIDANCE.md) defines acceptance and limits.
D-021 and D-022 remain binding; installed panels are still updated only by the user.

---

## D-022 · Owner-controlled infrastructure independent of CelikPanel

*September 12, 2026 · User-approved requirement; first audit recorded, removal not certified*

CelikPanel is an operator acting for the server owner. Owners must be able to
configure and operate services without it and remove it while retaining workloads.
Service operation must not depend on panel availability, licensing, or retaining
its management agent. Native configuration and standard protocols must support
independent DNS, websites, mail, databases, scheduled jobs and certificate renewal.

Removing management software must preserve service packages, data, identities,
configuration and necessary renewal/scheduling mechanisms. Audit existing
dependencies and separate them safely before claiming removal is supported.
Direct owner administration must be accommodated: detect configuration conflicts
and reconcile explicitly rather than silently overwrite owner changes.

Standard primary/secondary DNS transfer must be configurable through the panel
without requiring a remote CelikPanel or its HTTPS endpoint. Remote record
creation/editing is a separate optional authorized automation feature. The current
central publication connector is an implementation choice, not a DNS prerequisite.
Preserve installed configurations when evolving this architecture.

This direction does not authorize assistant-side live changes, panel removal or
installed-panel updates. The user-only update rule remains binding.

[Source implementation and remaining dependencies](OWNER-INDEPENDENCE.md).

---

## D-021 · Purpose-led, resumable server setup with explicit DNS ownership

*September 10, 2026 · Approved direction; [working-tree implementation status](SERVER-SETUP-STATUS.md)*

**Decision.** A confirmed new server enters an administrator-only setup guide
following license activation. Incomplete setup resumes; prepared servers return
to the Dashboard. Licensing, setup history and current health are independent.
Web hosting is the recommended default, with mail explicitly selected rather
than installed automatically. Other purposes cover web-and-mail hosting,
application servers and DNS infrastructure.

Hosting chooses local DNS, an authorized existing CelikPanel DNS infrastructure,
or externally managed DNS with verifiable manual records and optional API
automation. This supersedes D-009's blanket requirement for a local DNS engine.
It does not relax local DNS ownership, topology or recovery invariants, and it
must not be implemented by merely deleting current guards. Existing DNS ownership
is preserved until an explicit supported migration.

Initial public setup requires a panel FQDN with trusted renewable HTTPS and a
firewall policy that preserves management access. Further readiness gates apply
to the corresponding feature. The user reviews and starts a concrete, durable
plan; read-only discovery performs no configuration changes. Existing workloads
are preserved. Installed-panel updates remain user-initiated inside the panel.

**September 11 follow-up (source implementation).** On first use, fresh and
unassessed legacy administrators explicitly choose guided or manual setup.
The server remembers manual mode and offers re-entry under Settings → Server
setup. This changes navigation only: readiness, license enforcement, persisted
drafts and active-operation recovery remain independent. See
[guidance preference](SERVER-SETUP-GUIDANCE.md).

**September 11 customization follow-up (Alpha67 release source).** Profiles can be
customized and an explicit Custom setup choice keeps component selection inside
the wizard. Dependencies and conflicts use the managed-service policy; clearing
a selection never removes an installed service. The final review and readiness
checks follow the selected components. See [scope and local
evidence](SERVER-SETUP-CUSTOMIZATION.md).

**Scope and proof.** The approved sequence, current implementation gaps,
acceptance cases and implementation order are in
[Guided server setup](SERVER-SETUP-PLAN.md). This decision records product intent,
not a claim that the new wizard or external DNS mode is already available.

*Note of 2026-10-16 (clock date): applied in the v0.1.0-alpha.87 candidate (not
published).* No new decision is recorded for that candidate; the text above is
unchanged. A completed setup can now be extended
(`POST /api/v1/setup/extend`): the wizard reopens with the completed values,
the review says what changes, what is installed and what is kept, and the run
is the existing reviewed runner. This applies the decision's own rules:

- **Existing workloads are preserved.** An extension installs and configures
  only what is added; it never removes, stops or replaces an installed
  component (the customization rule that clearing a selection never removes an
  installed service holds for an extension), and "Stop extending" restores the
  completed draft without removing anything a partly run extension installed.
- **Setup history and current health stay independent.** An extension keeps the
  completion record (`completed_at`) and is not required, so a prepared server
  is never routed back into setup; the Dashboard says that the server is being
  extended and keeps working.
- **Mail is explicitly selected, with setup's checks.** On a server whose setup
  has finished, adding a mail profile from the Components page reuses setup: the
  profile is added to an extension and goes through setup's review and steps
  (public DNS of the mail host name, trusted certificate and renewal, reverse
  DNS, port 25). A server without a finished setup keeps the direct install,
  which says that it ends with a temporary self-signed certificate.
- **DNS ownership is not changed.** On the secondary of a DNS pair the panel
  says that the mail host name's record is made on the primary or at the DNS
  provider and shows the exact record; it creates none. Whether the primary
  should publish a peer's records itself, an editor for the infrastructure zone
  and automatic transfer of host records over the remote DNS connection were
  three open owner decisions (OPERATION-GUIDANCE, entry of 2026-10-16). The
  owner decided all three the same day ("as proposed"): the primary's plan
  can carry the peer's mail host name, the infrastructure zone is a DNS-only
  entry of Domains with the existing editor, and the secondary asks the
  primary for its host records over the authorized connection, with manual
  entry still possible; replication stays the native zone transfer (D-022).
  In source on branch `feat/alpha91-second-server-part2`, not measured
  natively by the owner's instruction (OPERATION-GUIDANCE, part 2).

Status of the note: in source with component tests and a mock browser (batch
16); measured in a lab with its own DNS and ACME in the native records set18
(at `5d54aba1e`, where the extension opened on the completed run, D1) and
set18b (at `6b363ac46`, D1 observed fixed, the extension reached ready) under
`deploy/e2e/release-recovery/evidence/`; Debian 13 only; nothing measured on an
installed server. The [alpha.87 release notes](RELEASE-NOTES-v0.1.0-alpha.87.md)
say which check rests on which code. No P0 item is closed or advanced.

### Amendment (2026-10-23): the administrator's email stays mandatory and is checked when the administrator is created

*Owner decisions of 2026-10-23; in source for the v0.1.0-alpha.102 candidate (not
published); nothing measured on an installed server*

Why. Since alpha.99 an open item said that the setup plan's contact-email blocker
applies to every plan. The owner decided: "iletişim epostası zorunlu olarak
gerekiyor ise zorunlu olmalı. kurulumda eposta belirtiyoruz" (if the contact email
is genuinely required it must be mandatory; an email is already given during
installation). It is genuinely required: initial public setup requires trusted
renewable HTTPS for the panel's name (above), so the first setup always requests
the panel's certificate, and the certificate authority takes the administrator's
address as its contact. The only plan without a certificate step is one for a
server that already has a valid certificate, and then the address is already on
file. The blocker therefore stays for every plan. The owner then approved ("ok.
yap") checking the address where it is first given, so that a typo is caught at
install time and not later on the Server setup screen.

Decision.

- **One rule.** The installer checks the first administrator's email with the
  validator the setup plan, the panel certificate request and the own-email change
  already use (`mutationpayload.CanonicalContactEmail`): printable ASCII, exactly
  one `@`, a local part of at most 64 bytes, a fully qualified domain name (not
  `localhost`, not an IP address, no trailing dot) and at most 254 bytes in all.
  `ali`, `ali@`, `@x.com`, `a b@x.com` and `ali@localhost` are refused.
- **Interactive prompt.** On a refused address the installer prints "That is not a
  valid email address (example: you@example.com). The certificate authority needs
  it as a contact. Please try again. / Bu geçerli bir e-posta adresi değil (örnek:
  siz@ornek.com). Sertifika yetkilisi bunu iletişim adresi olarak ister. Lütfen
  yeniden deneyin." and asks again. The typed value is never printed. The prompt
  still comes where it always came, after the packages are installed; it asks
  again rather than stopping, so a typo there does not stop the installation.
- **Credentials file.** A file given with `CELIKPANEL_ADMIN_CREDENTIALS_FILE` is
  checked by the candidate panel's validate-only mode before anything is
  installed, as before; that check now applies the same rule. The installer then
  says that the input file was left untouched and nothing was installed, and what
  to correct: a username the panel accepts, a real email address such as
  you@example.com (the certificate authority needs it as a contact) and a password
  of at least 8 characters. The panel's own refusal names the email field and
  never a value. The signed entry script's own form check is unchanged; the
  downloaded panel remains the authority.
- **Stored form.** The address is stored in the validator's canonical form (the
  domain in lower case), as the own-email change stores it.
- **The existing-administrator path.** Running the create-administrator command
  again for an existing user name is held to the same rule; a refused address
  changes neither the address nor the password.
- **Existing installations are unaffected.** No migration and no re-validation of
  stored addresses. An older invalid address is still caught by the Server setup
  blocker, whose "Open My account" link leads to the own-email change.
- **No other path makes an administrator.** The account API refuses to create or
  manage administrator-role accounts; team members are customer-role additional
  users and cannot open server setup; a control-plane restore brings the
  archive's administrators unchanged; demo mode's accounts are demo-only. The
  team-member address check is therefore not changed.

Not changed. The certificate authority may still refuse an address at a domain
that is not public (`.test`, `.invalid`, `example.com`): the panel checks the form
and the authority judges the domain. This closes the limit recorded with D-037's
alpha.99 amendment that the create-administrator command did not check the
address; it does in source for alpha.102.

Upcoming acceptance, not a claim. At Beta 1 the owner will wipe and reinstall both
servers for a clean first-install test; that run is where this check will first be
seen on a real installation. Installed panels are still updated only by their
owners from the panel's own update screen.

---

## D-020 · Managed-server support follows proven capabilities, not distribution names

*August 11, 2026*

**Decision.** The CelikPanel Agent targets managed Linux servers only. Platform
selection is based on proven host capabilities, not an allowlist of distribution
names, versions, or codenames. The current package adapters are `apt`, `dnf`, and
`pacman`; a narrow layout override exists only where a package, repository,
service, security policy, or path genuinely differs.

**Detection and trust boundary.** `ID`, `ID_LIKE`, and `VERSION_ID` from
`/etc/os-release` are parsed as inert metadata and never sourced. A single
unambiguous `ID`/`ID_LIKE` family hint may choose which complete toolchain to
verify, so an unrelated package manager installed by an operator cannot redirect
bootstrap. With no usable hint, or with conflicting hints, exactly one complete
trusted toolchain must exist. APT means the fixed `apt-get` + `apt-cache` +
`dpkg-query` set, pacman means fixed `pacman`, and DNF means fixed `dnf` + `rpm`.
Fixed `systemctl` and `timeout` are always required, together with a root-owned,
non-writable `/run/systemd` chain, a direct `/run/systemd/private` Unix socket,
and a bounded `systemctl is-system-running` proof of `running` or `degraded`.
Every present candidate tool, its target, and its directory chain must satisfy
the root-owned, non-writable fixed-path contract. Missing, incomplete, ambiguous,
symbolic outside a pinned vendor alternative, unsafe, or non-live capabilities
fail before mutation. Names and versions therefore describe the host; verified
capabilities authorize the adapter.

**Certification boundary.** Selecting a package adapter is not a blanket feature
claim. The UI exposes only capabilities whose package, path, security-policy,
lifecycle, and readiness recipes prove their own preconditions. A distribution
may use any name or version and reuse a proven adapter; an unsupported package
ecosystem or an unproven feature remains blocked because the required capability
proof is missing, not because the distribution label is absent from a list.

**Current DNF preview gate.** The test-covered DNF transaction, RPM inventory,
host-identity, and external-package-lock primitives may be developed and tested
before any DNF capability is exposed. Every host whose verified toolchain selects
DNF still stops before its first bootstrap mutation because the
SELinux-Enforcing panel/agent lifecycle is not implemented. This blocker follows
the missing security capability, independent of distribution name or version.
The same live SELinux probe runs before every APT or pacman bootstrap and
rollback mutation. Those adapters proceed only when SELinux is inactive; an
active permissive or enforcing state fails closed until its label lifecycle is
implemented. Package-manager capability never implies a security-policy
capability.

**Default-deny exact-method prefilter.** For every DNF preview host, the panel
carries the complete verified host identity into a prefilter keyed by the exact
Agent RPC method immediately before the sole raw dispatcher. This is a
fail-closed foundation, explicitly not an activation surface, and exact method
plus host identity is insufficient to authorize a parameterized RPC: the same
method can carry different service, package, or target arguments. No DNF grant
entry may be added until the RPC arguments are bound to the durable lease's
exact `Kind`, `Target`, and `PackageName`, and that complete lifecycle has
passed live-machine end-to-end certification. The DNF grant set remains empty,
so every DNF host mutation remains denied. Detection, identity
resolution, cross-compilation, and smoke coverage are neither a support claim
nor a capability claim.

**Out of scope.** A package ecosystem without an implemented adapter does not
become supported by imitating APT, DNF, or pacman. Proven demand may add distinct
adapters later. Linux distributions are never rejected merely for their name;
they fail closed when no unique complete trusted toolchain and feature-specific
layout/security proof exists. Windows and FreeBSD are not managed-server targets.
`!linux` files preserve build portability and a no-mutation, fail-closed contract
only; they do not promise product support.

**Install-source policy is unchanged.** A package adapter enforces D-018:
system services are distro packages installed through `apt`/`dnf`/`pacman`,
while portable applications and runtimes use verified official releases. A
truly necessary source build happens in central CI, not on a customer's
server, and is distributed as a signed native package for that ecosystem. Security
updates, file ownership, uninstall, and rollback therefore remain integrated
with the package manager as capability coverage grows.

**Why.** One installer copied per distribution multiplies code while dividing
quality. Building everything from source only unifies binary production; it
does not unify systemd/SELinux/AppArmor, users, paths, firewall, or update
integration. A package-capability adapter is the correct reuse boundary.

---

## D-019 · The panel does not scare: every management page carries its own help; empty pages are forbidden

*25 July 2026*

**Decision.** The operator asked for it directly: user-friendly, reassuring
pages with a help button that explains the page itself. Three rules are
permanent:

1. **Every management page has a Help button.** It comes from the single
   shell (ServiceShell), so nobody writing a new page has to remember help
   separately. Content has three sections: *What is this?* (2-3 plain
   sentences), *Tips* (practical panel-first tasks), *Something looks
   wrong?* (symptom → fix). The language is deliberately non-technical:
   "background process", "databases have no trash bin", "reading is always
   safe".
2. **Help content is CONTENT, not chrome.** It is not smeared over hundreds
   of `help.x.tip3` i18n keys; both locales sit side by side in one entry
   per component (`web/src/help/serviceHelp.ts`), so updating one language
   and silently forgetting the other is structurally impossible. A component
   without a specific entry gets its kind's generic one — a page without
   help cannot exist.
3. **Empty management pages are forbidden.** The panel already knows a
   component's status, unit, versions, packages, ports, config files and
   journal; a "coming soon" page is a refusal to show what is known. The
   derived panels (ComponentPanels) are every page's floor; specialised
   pages build ON TOP of it, never instead of it.

**Why.** A page that scares goes unused, and an unused panel is untrusted.
The help button must be there at exactly the moment a page frightens its
reader — so it shows on not-installed, failed and half-configured services
too. Hand-written page lists (the nine-page switch) rotted the same way the
scanner's hand-written unit list did; the derived floor closes that class.

---

## D-018 · The install source is fixed by kind: system service = distro package, app/runtime = official release, Docker is not the base

*July 24, 2026*

**Decision.** The operator asked: "wouldn't it be better to always pull from
git and build for our system, or use Docker?" The answer is not "one method
for everything" — chaos comes from having no rule, not from this one. The
source is chosen by the item's KIND, and each kind's rule is fixed:

1. **System service** (nginx, PostgreSQL, MariaDB, Postfix, Dovecot, Redis…)
   → the **distro's package repo** (apt/pacman/dnf). Why: the distro's
   security team owns the patching (`apt upgrade` fixes it); systemd
   integration and config paths come ready. Building from source puts CVE
   tracking on us; Dockerizing breaks start/stop/status-read over systemd.
2. **Runtime + portable app** (Node.js, Roundcube, later Python/Ruby, webmail
   alternatives) → the **vendor's official release** (download + verify a
   pinned SHA-256 + unpack). Why: the distro freezes one version, we want
   many + current; and the distro package is distro-specific (live proof in
   Roundcube: apt `roundcube` / pacman `roundcubemail` differ in layout →
   worked on Debian, not Arch). The official release is one path on every
   Linux (D-004). This is NOT "pull from git + build" but "signed/frozen
   release" — none of the composer/build-step fragility.
3. **Docker is NOT the base.** The shared-hosting model runs in-system
   directly (nginx vhost + per-site FPM pool); 100 sites = 100 containers of
   waste + port/network/volume chaos + loss of systemd integration. Docker
   may later be a KIND (a project type for a customer's own container, or a
   store option for an isolated/heavy stack) but never the substrate — in
   Plesk too it is an add-on, not the model.
4. **Building from source**: only when genuinely required (not in the distro
   AND no official binary — rare); then build tools are installed
   temporarily and removed after (clean-server).

**Honest limit (proven in the same slice).** The "one path on every Linux"
ideal is not free: even a portable tarball touches distro reality in its
SUB-dependencies — Roundcube's SQLite needed PHP's pdo_sqlite (a separate
package on each distro, and on Arch manually enabled), the FPM socket path
differed, the nginx conf layout was Debian-only (A10). The right architecture
is not pure but HYBRID: a portable body + distro-aware sub-details (package
name, socket path, extension enable) — and that distro-specific truth lives
in the agent; the panel/catalog knows no distro.

**Rejected alternatives.**
- **Build everything from source:** most things (PHP/scripts) don't compile;
  taking on CVE-patching for system services; build tools clutter the "clean
  server".
- **Put everything in Docker:** clashes with shared-hosting DNA; every
  systemd-based mechanism (status read, start/stop) breaks.
- **Install everything via distro package:** single-version prison for
  runtimes; distro-specific layout for portable apps (the Roundcube trap).
- **"Decide case by case" (no written rule):** the same debate on every new
  item; plus a class-slip risk (packaging an app like a system service).

---

## D-017 · Offering is universal: everything choosable — component, version, integration, product — flows through the chain in one offering-ID space

*July 24, 2026*

**Decision.** The operator's sentence reads like a statute: *"when a customer
picks an SSL provider, or a reseller decides which SSL providers their
customers may pick, they in fact move within the options WE offer them."*

1. **The chain's subject generalizes.** D-014's `installed ⊇ offered ⊇ used`
   applies not only to installed components but to EVERYTHING choosable —
   including integrations that install nothing on the server (SSL CAs;
   later Cloudflare, backup targets). To a customer, "which CA" is an
   offering exactly like "which PHP version".
2. **One offering-ID space.** Every offerable item carries a canonical id:
   `component:<id>` (whole) · `component:<id>:<version>` (D-014: the unit
   is the version) · `integration:acme:<id>` · `product:<id>` (D-012's
   `subscription_entitlements.product_id` is this space's first resident).
   There is NO separate table/model per offering type — that would be the
   "two owners of one fact" disease at the offering layer.
3. **Three layers, three questions; effective set = intersection.**
   *Available* (admin): installed components + registry entries left ON
   server-wide — the admin must be able to disable a CA panel-wide.
   *Offered* (reseller): plan contents — D-014's "missing link" is born
   here as `plan_offerings` (plan ↔ offering-id).
   *Used* (customer): the choice at use time (version per site, CA per
   certificate).
4. **One enforcement pattern at every chooser.** Listing endpoints (ssl
   providers, php versions, node versions…) filter to the caller's
   effective set; acting endpoints re-verify server-side and refuse with a
   coded error (the `NOT_OFFERED` family). The UI draws only what is
   offered — the general form of D-012 #7 ("visibility follows the right").
5. **Generous by default, narrowing deliberate.** A plan that declares no
   restriction offers everything (today's behavior, unchanged). The
   mechanism breaks nothing retroactively; a reseller narrows WHEN they
   choose to — D-014's "right to simplify" made real.
6. **Timing honesty.** The full mechanism ships with the v0.3 tenant/plan
   slice (a behaviorless skeleton without a plan UI would be theater). The
   one obligation effective today: every NEW chooser endpoint is written to
   this pattern, and offering ids are named canonically from now on.

**Why.** Three choosers were born in two days (PHP version, Node version,
SSL CA) and all three had no answer to "who narrows this list?". The
operator saw the answer must be the SAME for all three. Three separate
narrowing mechanisms would mean maintaining three authorization models.

**Rejected alternatives.**
- **Keeping integrations outside the chain** ("nothing gets installed"): to
  the customer a CA pick is an offering; excluding it births a second
  authorization model.
- **A table per offering type:** one space, one offering ledger; the D-012
  lesson.
- **Full implementation now:** a skeleton that changes no behavior while
  the plan UI does not exist — a recorded decision beats half-built code.

---

## D-016 · The page is named Components; the glossary is pinned; the service definition is refined

*July 23, 2026 — 5-consultant panel (market, TR-UX, information architecture, devil's advocate, global/localization) + judge synthesis; operator approved*

**Decision.**
1. **The page is renamed NOW:** EN **Components** · TR **Bileşenler**. The
   `/services` route stays. Four of five consultants independently concluded
   the current name is already false (industry-wide, Services = daemons; the
   page holds PHP, phpMyAdmin, nftables) and the rename cost only grows as
   users and docs accumulate.
2. **Glossary** (the store is born into these words; the umbrella is never
   re-litigated): umbrella **component/bileşen** · kinds: **Service/Servis** ·
   **Runtime/Çalışma Ortamı** (short badge form "Ortam") · **Tool/Araç** ·
   future **Module/Modül**, **Task/Görev**, **Integration/Entegrasyon** ·
   store: EN **Store**, TR **Mağaza** (page title "Bileşen Mağazası").
   "Service" lives on as the default kind filter — support language ("restart
   the service") keeps working.
3. **The service definition is refined** (the operator's "start/stop-able =
   service" test is directionally right, not literal): *a service is a
   component that owns a resident process whose being down is an incident in
   itself.* Two counterexamples from our own catalog: nftables accepts
   start/stop via a oneshot unit yet leaves no resident process (correctly a
   tool; oneshots deserve an "applied/not applied" state, never a running
   dot), and php-fpm ships real daemons yet is a runtime because its primary
   job is per-site version choice. "Has a controllable unit" is an ORTHOGONAL
   capability, not a kind: start/stop buttons draw from that capability on
   any row, including inside the version drawer.
4. **Recorded dissent** (the global consultant, stated fairly): no major
   panel uses "Components" in its nav (cPanel's umbrella is "Software",
   aaPanel's is "App Store"); Components is the most abstract candidate for
   ESL-heavy growth markets and collides with Joomla's
   Components/Modules/Plugins. Its alternative was "Software". The judge
   rejected it because "Software" goes false on a known schedule — the day
   the integration kind (a Cloudflare connection, not software on the
   server) is born, the exact disease that broke "Services" — and it offers
   no countable row/doc noun. IF findability or ticket-language problems
   surface, the fallback candidate is Software; this sentence exists for
   that day.

**Why now.** The item count only grows (module/task/integration are mapped,
the store is coming); letting habit and docs pile onto a wrong name does
nothing but make the correction day more expensive. That was the devil's
advocate's own collapse condition: "I concede once non-service rows
multiply" — the roadmap promises exactly that.

---

## D-015 · An extension is a package, a kind is a nature: the store grants but never installs, the core catalog's boundary is responsibility not price, and the kind list is open

*July 23, 2026*

**Decision.** Six linked decisions, prompted by the operator's car analogy
(standard vs optional equipment, engine swap) and the consultant's breakdown
of the Plesk/cPanel extension world:

1. **The two axes are final and never mix.** An *item/extension* is the unit
   of acquisition and distribution (free/paid, first/third party — D-012's
   axis). A *kind* is the mechanical nature of what gets installed
   (service/runtime/tool — D-010's axis). The consultant's summary IS the
   model: *"extensions usually manage services; they are not services
   themselves."* One extension may bring 0..N parts (Plesk Email Security:
   one extension → manages postfix+dovecot+clamav); every part shows up
   under ITS OWN kind — the extension itself is not a kind.
2. **The store grants, it never installs.** Purchase / license key / seat
   pool live in the store; an acquired item appears in the Services
   catalogue and is installed from there. The catalogue is the single
   install address — a second Install button would repeat the two-address
   mistake B3b just deleted (AdminNodeInstall).
3. **The core-catalogue/store boundary is RESPONSIBILITY, not price.** The
   core catalogue is the small curated set the panel vouches for (package
   names on both distro families, firewall ports, uninstall, tests — all
   our debt). Rarely-used FREE items (a niche database, say) would mean
   bloat plus a warranty we cannot honor — they come through the store
   too. (This corrects the Jul 22 statement "free items never enter the
   store": the criterion is the warranty boundary, not the price tag.)
4. **The kind list is open, but a kind is born with its FIRST item.** Three
   kinds carry three real item families today. The remaining shapes in the
   consultant's breakdown map cleanly and join as explicit kinds when their
   day comes: *module* (lives inside a host service, no unit of its own —
   ModSecurity, Brotli; needs a "host service" field) · *task/timer* (runs
   and exits — backups, certificate renewal) · *integration* (nothing
   installed on the server; its state is "connected?", its secret a sealed
   API key — Cloudflare, S3 backup targets, UptimeRobot). An *agent* is NOT
   a separate kind: mechanically it is a service (you start/stop the
   Imunify/Acronis agent); the difference is commercial.
5. **A compiled language needs nothing installed.** Go/Rust "support"
   cannot be a catalogue item because the multiplication test (D-011) says
   N: the runtime ships INSIDE the customer's binary. App mode (reverse
   proxy + unit) already runs any binary today — Go/Rust support exists in
   fact, no row needed. Interpreted languages (Python, Ruby) are Node's
   class: runtime items when their day comes. (If "pull from Git + build
   on server" ever ships, the toolchain becomes a *tool* item — if.)
6. **Two pages, two questions.** Services (possibly renamed "Components"
   later) = "what is on my server and how is it doing?" Store = "what can
   I acquire, from whom, under which right?" The same product may appear
   on both; that is two moments, not a contradiction.

**Why.** The operator stress-tested the model with three concrete questions
(where do rare-free items go; what about Go/Rust; the consultant counts 7
working shapes) and the model answered all three with its existing axes —
only the core/store boundary had been justified wrongly (price instead of
responsibility). The consultant's "verify on a real server" section
(list-unit-files, ss -lntup) is exactly what the scan already does —
recorded as external validation.

**Addendum (Jul 24 — on the operator's boundary-keeping): the core's measure.**
The operator asked: "wasn't everything except vital services supposed to sit
in the store first and enter the catalog on demand? won't hundreds of
programs in Components be chaos?" They remember correctly, and the boundary
now gets a number:
- **Core = one default + 1-2 named alternatives per role.** (web:
  nginx+Apache · DNS: PowerDNS+BIND · SMTP: Postfix+Exim · spam:
  SpamAssassin+Rspamd · webmail: Roundcube …) Target: TENS of items; never
  hundreds. The Jul 23-24 additions (rspamd, exim) are of this class — seat
  members, not long tail.
- **The long tail lives in the store** (free items acquired with one click
  and then appearing in the catalog — clause 2 already said so; the size
  discipline is now explicit). Large groupware like SOGo does NOT enter the
  core and waits for the store; netdata is borderline (the monitoring role
  is covered by the panel's own page) and is a candidate to move there.
- Today's chaos brake stays on record too: the default view shows installed
  items only (D-010, installed-first); the catalog sits behind a button.
  But the scale insurance is the store gate, not the view.

**Rejected alternatives.**
- **Making the consultant's 7 shapes 7 kinds today:** with zero module,
  timer or integration ITEMS on hand the schema would be speculative; a
  kind is born with its first item, otherwise we recreate the inverse of
  the problem D-010 solved (empty categories).
- **A "support row" for Go/Rust:** conceptual error — the same "a Go row
  would be a category mistake" already rejected in D-011; where there is
  nothing to install, a row lies.
- **Drawing the boundary at free/paid:** a rare free item is a warranty
  burden in the core but finds natural distribution in the store; price is
  the wrong axis.
- **A page per kind:** would split "everything installed, one list" across
  seven pages; kind changes how a row is DRAWN, not where it lives.

---

## D-014 · The authority chain is a narrowing filter: admin installs, reseller offers, customer uses — and the unit is the version

*July 21, 2026*

**Decision.** Who gets a capability is settled by three separate decisions, and
each layer can only **narrow**, never widen:

    installed (admin)  ⊇  offered (reseller)  ⊇  used (customer)

- **Admin** decides one thing only: does this service/extension **exist on the
  server**. Having installed it does not mean having given it to anyone.
- **Reseller** decides one thing only: which of the admin's installed items do I
  **offer to my customers**.
- **Customer** decides one thing only: which of the reseller's offered items do I
  **use** — and that decision is per site, not per account.

**The unit of the chain is the version, not the service.** "I offer PHP" means
nothing; a reseller offers **PHP 8.3** or does not, and a customer picks a
version per site. This does not contradict D-010; it adds a second axis to it:
in the catalog the **display** unit is the runtime (one row, versions in a
drawer), while the **entitlement** unit is the version itself.

**Settings run through the same chain**, not just presence: the admin sets the
server default and the ceiling, the reseller decides which knobs a customer may
touch, and the customer adjusts within those bounds.

**Why.** The operator's concrete case: the admin installed only PHP 8.4; a site
owner may need 8.3 and will never touch 8.4. A single "is it installed" flag
cannot answer those three different questions at once. Today the code has one
grant layer (`hasEntitlement`: does the subscription hold the product); there is
no admin→reseller vs reseller→customer distinction, so a reseller's right to say
"I do not offer that" cannot be represented.

**Where the layers live in code** (two of the three already exist):
- Admin layer = the installed set (`InstalledServiceIDs` + catalog). ✅ exists
- **Reseller layer = service plans** (`plan_handlers.go`, `004_accounts.sql`).
  No new structure is needed: "I offer it" is the plan's content — *"this plan
  includes PHP 8.2 and 8.3, not 8.4"*. Plesk does the same (Service Plans).
  ⬜ this is the missing link
- Customer layer = the per-site choice (`site.php_version`, `CreatePool(siteID,
  username, phpVersion)`). ✅ exists

**Consequences.**
- **Removal is a chain-wide event.** If an admin wants to delete a version, the
  plans offering it and the sites using it must block that. D-010's
  `RUNTIME_IN_USE` refusal must not stop at "in use" — it must count *who*
  blocks (how many plans, how many sites), or the admin decides without seeing
  what they are about to break.
- **Server-wide PHP settings are incompatible with this model.** Today's PHP-FPM
  page keeps extension toggles and php.ini values server-wide:
  `disable_functions` is a security control, so one customer's need becomes
  everyone's policy; a server-wide `memory_limit` lets one heavy site set the
  ceiling for all; `ffi` cannot be on for one customer and off for another. The
  page should name itself the **server default**, and a per-site layer must
  exist alongside it. The plumbing is ready: every site already gets its own FPM
  pool, and a pool can carry its own `php_admin_value` directives.

**Precondition (honesty note).** For PHP this model is currently **vacuous**:
without the Sury repo the admin could not install a second version even if they
wanted to (see the D-002 correction, D-010). Debating "who offers what" while
there is only one thing to give is theory. So B3c (Sury + multi-version) moves
to the front of the queue — not as polish, but as what makes the chain mean
anything.

**Rejected alternatives.**
- **A separate "offering" table for the reseller layer**: plans already answer
  exactly this question; a second structure would give the same fact two owners
  (the very mistake B3 exists to close).
- **Auto-offering everything the admin installs**: least code and wrong — it
  erases the reseller's right to simplify (a reseller may deliberately offer
  only 8.3 to cut their support load).
- **Keeping the entitlement unit at service level** ("has PHP / has no PHP"):
  solves none of the operator's case; it would tell a customer who needs 8.3
  that they "have PHP" because 8.4 is installed.
- **Treating server-wide settings as sufficient**: in a multi-tenant panel that
  means imposing one customer's `disable_functions` need on everyone.

---

## D-013 · Creating a domain is not picking a runtime: PHP is a site toggle, not an installation decision

*July 21, 2026*

**Decision.** The "Add domain" screen stops asking **"what will run here?"**. It
asks one thing — purpose: **website · mail only · DNS only**. The runtime (PHP
on/off + version, Node/proxy mode, port, startup file) becomes a **setting on
the site** after creation: a property that can be flipped and flipped back.

A direct consequence: **`static` and `php` are not separate project types.**
They are the same thing with PHP off and on; the template difference is three
lines (does `index.php` count as an index · does an unmatched URL fall through
to `/index.php` or 404 · does `.php` go to fastcgi). `node`/`proxy` genuinely is
a separate mode — no document root is served, everything goes to the proxy.

**PHP-FPM stays optional** (operator's call, Jul 21). The constitution's "nothing
is installed at setup; what isn't installed is invisible" applies to PHP too: a
DNS-only or Node-only machine carries no PHP. When PHP is absent the toggle on
the site says so honestly and shows an admin the way to install it — but it does
that in its own place, **not on the creation screen**.

**Why.** The operator asked: "why are we forcing PHP on someone who will never
use it — isn't that unnecessary and confusing? Not everyone has to install
WordPress." They were right, and the diagnosis was: the screen advertised an
uninstalled capability with an orange call to action — the exact opposite of the
"not installed = invisible" rule we had just enforced on the Services page. It
made someone building a static site feel their server was deficient. The server
was not deficient; that person did not want PHP.

Plesk's real flow was examined (9 operator screenshots of a live install) and it
showed this: **Plesk's creation dialog is not a runtime picker.** Its first
screen asks how the content will arrive (blank page · upload files · WordPress ·
deploy from Git · Node.js · mail only). The second screen never mentions a
runtime. The third runs "Configuring PHP" as an **unconditional** step. PHP is
afterwards a checkbox on the site's settings page (version list + per-site
php.ini). The telling detail: **the "Node.js" option does not change creation at
all** — the form is identical to "Blank website", and when creation finishes Node
is **still not enabled**; you must press "Enable Node.js". So even in Plesk that
first screen is a routing menu; the real decision is made in the site's settings.

**We take Plesk's shape, not its bundling.** In Plesk the question never arises
because it ships PHP itself (`/opt/plesk/php/8.3`); PHP can never be "not
installed". For us it can. So "PHP on by default for every site" cannot be
copied directly — copying it would violate D-003 and the constitution.

**Side finding (closed).** This discussion surfaced a real hole: the static vhost
had no PHP handler but also no deny rule for `.php`, so nginx served the file as
`application/octet-stream` and the browser **downloaded the source**. Same family
as A6. Measured against live nginx and closed (`b1a6aac`). The lesson goes on
record: *"X does not run here" must not mean "X's source is published here".*

**Rejected alternatives.**
- **Make PHP-FPM part of the base install** (the Plesk model): it would end the
  question outright and match market expectation exactly, but it means giving up
  "not installed = invisible" for PHP. The operator chose optional.
- **Ask the operator at install time** ("will this server serve PHP?"): one
  decision per machine was tempting, but it adds a permanent branch to
  install.sh and strands anyone who changes their mind later.
- **One option per language** ("Node.js site", "Python site", "Go site"): to
  nginx and to the panel all three are the same thing — reverse proxy to
  `127.0.0.1:PORT` + a unit. Same reasoning as D-010's "a Go row would be a
  conceptual error". Application mode is **one** option; the runtime is a field
  inside it.
- **Soften or shorten the orange warning on the creation screen**: treats the
  symptom. The problem was not the warning's tone but that the question did not
  belong there.
- **Treat mismatch detection as sufficient on its own** (warn when a static site
  contains `.php`): a good feature and it will be built, but alone it would keep
  asking the wrong question and clean up afterwards. The question gets fixed
  first; detection is the safety net.

**Implementation order.** (1) ✅ the static `.php` leak is closed — security,
independent of the design debate · (2) Add Domain drops to "what is this for?",
the orange nag goes, copy moves from brands to behaviour ("WordPress, Laravel…"
goes) · (3) a PHP toggle + version on the site's settings; `static`/`php` merge
into one mode · (4) mismatch detection + one-click fix · (5) Application mode
(Node/proxy) becomes visible in post-creation settings — the backend
(`hosting_handlers.go`, the template's proxy branch, `HostingTypePanel`) is
already there; only the shared language is missing.

---

## D-012 · Licensed products and the resale chain: an entitlement is a pool; compliance is between the operator and the vendor

*July 20, 2026*

**Decision.** The operator's model ("everything is an item; some free, some paid;
the admin buys and sells to resellers or directly to customers") is met by the
following — and **third-party commercial products are in scope from the start**
(operator's decision):

1. **"The thing" and "the right to use it" are separated.** The product itself is
   installed once on the server (multiplication test = 1 → catalog item, D-011).
   The **entitlement** is per subscription and flows down the tree. These are
   independent axes: installation is a binary state, an entitlement is a countable
   resource.
2. **An entitlement is a pool, like disk/domains.** No new mechanism: v0.3's
   `reseller_pools` pattern extends to products. The admin's quota → allocated to a
   reseller → allocated to a customer; on overflow, a coded 409 + remaining pool.
   The admin may also sell **directly to a customer** (the subscription ownership
   tree already supports this).
3. **The license model enters the product definition:** `license_model ∈ {server,
   seat}`. *server* → one price, unlimited users; the admin's pool is infinite and
   resale is pure margin. *seat* → the admin pays the vendor per unit, so
   **over-allocation is real money and a licence breach** — the pool is enforced
   hard (a refusal, not a warning).
4. **`seat_unit` is mandatory** (`mailbox | site | subscription | server`). Vendors
   count different things; without the unit we count the wrong one and the panel's
   number will not match the vendor's invoice — the subtlest trap here.
5. **The licence key is stored as a secret** — A4's `enc:v1` mechanism (sealed at
   rest, opened at use). Most commercial products need the key at install time; an
   install without it is half an install.
6. **A price is not one number.** The chain has three: vendor→admin (cost),
   admin→reseller, reseller→customer. Today's single `MonthlyPriceCents` is not
   enough; the pattern matches v0.3's reseller-owned plans (`service_plans.owner_id`).
7. **Visibility follows entitlement.** If a reseller has not bought the product,
   their customers **never see it** ("what isn't installed is invisible", applied to
   entitlements). Subtlety: a reseller may switch on a "buy" prompt — visibility is
   the reseller's call, hidden by default.
8. **A product that is not installed cannot be sold.** Selling an entitlement for a
   product absent from the server sells the customer nothing → coded refusal.
9. **Revocation follows the SAME rule as subscription suspension.** When a reseller
   stops paying: new allocations in their tree are 403, existing usage lives until
   the end of grace, no data is deleted. There are not two different "cut off"
   behaviours.

**Why this burden was accepted.** Opening the chain to third parties later would
mean rewriting the allocation and pricing model; adding `license_model`/`seat_unit`
up front costs two fields. The operator deliberately chose the harder path.

**THE HONESTY LIMIT — what the panel does NOT guarantee.** The panel enforces only
**its own allocation records**; it does **not** guarantee compliance with a
vendor's licence terms:
- The vendor may count differently (e.g. domains instead of mailboxes); the panel
  shows a reconciliation view ("vendor: 50 · allocated: 47 · free: 3"), but **the
  invoice is the vendor's truth**, not the panel's.
- **The right to sublicense a third-party product (to a reseller or customer) is
  between the operator and the vendor**; most vendors bind it to a partner
  agreement. The panel cannot verify that right and does not claim to. This is
  written plainly in the UI and the docs — otherwise we would ship a feature that
  encourages licence violations.

**Rejected alternatives.** Silent over-allocation (warn and continue — under the
*seat* model it runs up a debt for the operator) · putting per-vendor API
integrations in the core (each vendor is a separate integration, against the
no-dependency rule and simplicity; we start with a **manual key + manual quota**,
and automate only if real demand appears for one specific vendor) · burying
entitlements inside plans (a plan change would silently drop rights; an entitlement
is its own record, a plan only supplies defaults) · deferring third parties (the
operator refused: opening the chain later means rewriting the allocation model).

---

## D-011 · A framework is not a service: the multiplication test, a preset budget, and the limit of version ownership

*July 20, 2026*

**Decision.** Four linked decisions in one record:

1. **The boundary rule becomes countable — the multiplication test.** One question
   decides whether something is a catalog item or site-internal: *when N sites are
   created on the server, does the number of copies of this thing on disk stay at
   1, or become N?* 1 → catalog item (its version is managed by the distro/vendor
   repo); N → site-internal (its version is managed by the customer's lock file or
   the application's own updater). **The catalog = L1 (services) + L2 (multi-version
   runtimes), nothing else.** Laravel, Symfony, Django and Next.js are **not in the
   catalog and never will be**; Composer, Node and certbot are catalog items (a
   single copy). The class is set by **deployment shape**, not by the software:
   the very same phpMyAdmin, if copied into every customer's docroot, would become
   N and fall out of the catalog. The test is not arguable because it is numeric.

2. **The panel never owns the version of anything that exists N times.** The panel
   does not **write** an updater; it **triggers** the application's own updater as
   the site user. It does not pin versions, patch files, write `DISALLOW_FILE_MODS`
   or display a "supported version". This binds the roadmap's "WordPress Toolkit
   depth (updates, hardening)": no hardening preset may disable automatic updates
   by default.

3. **Framework support = site primitives (L3) + a preset; never a type, never an
   installer.** The only thing the panel may own about Laravel is a *set of
   defaults* (a `public` docroot suggestion, the cron line text, a queue command
   builder) — not an installer. Provisioning contains not one `if type == laravel`
   branch.

4. **`appCatalog` is not a type axis but a reversible install action.** The site
   type is PHP; WordPress is **not** an option in the domain-creation flow — it is
   an action run afterwards and **removable**.

**Why.** The operator asked: "Is Laravel a service, does it belong in the catalog —
and if we open that door, does it go on forever?" Five competing panels' real
record was examined:

- **The engine of drift is not the catalog, it is presets.** A catalog entry is
  expensive (package name, two distro families, a removal path, an init step); a
  preset costs three strings and has no argument against it. "Symfony comes for
  free" also holds for Django, Ghost and Strapi; by the eighth preset the screen is
  a framework picker. **cPAddons, too, began as "just a configuration recipe", not
  a one-click installer.**
- **The second engine of drift is version ownership.** What killed cPAddons was not
  installation: the panel took ownership of N-instance WordPress versions, patched
  `update.php`, and users were stranded on WP 3.9. The same pattern repeated at
  aaPanel (a one-click frozen on Laravel 5.4/PHP 7.4; their own staff say "delete
  the files we generated") and Plesk (the skeleton generator broke on an upstream
  change; the APS catalog was **removed entirely** in 18.0.77). The industry
  retreated from this model.

**Three mechanisms that make breaking the rule expensive** (a rule that lives only
in Markdown will not survive two years — unlike D-003/D-010, this decision needs
enforcement in code):

- **The structural purity test.** A preset is accepted only if it can be expressed
  as *pre-filled values of already existing generic fields*. If it needs one new
  field or one `if framework ==` branch, the preset is **rejected**. Presets live
  as pure data in one file; in the UI they are not a selectable "type" but a "fill
  the form" button above the form. Passing 3 presets is not a code change but a
  **strategy change**.
- **A framework-name ban + a CI gate.** A framework name may not appear in any enum
  constant, DB column, API field value or systemd unit name — only in i18n strings.
  The docroot dropdown lists **path values**, not framework names
  (`(root) | public | public_html`). `validProjectTypes`
  (php/static/node/proxy/forwarding) defines the *transport shape*, not the
  application — it is **considered locked**. In CI, grepping Go sources for
  `laravel|symfony|django|nextjs|ghost` must return zero matches (i18n excluded).
- **Every entry's exit is written first.** `appCatalog` entries carry a mandatory
  decision-record ID, maintainer and source checksum — "adding an entry in three
  lines" becomes impossible. Each admission is written together with its removal
  path and the answer to "what happens to sites already installed" (always: **the
  files belong to the customer; the panel only removes the button**).

**Commercial pressure is closed too.** The `app_installer` entry today says
"One-click installs for WordPress **and other apps**" — selling a one-entry list as
a plural plan feature. Two harms: sales sees an empty bucket to fill (**the
strongest engine of drift is a promise already sold**, not engineering), and once
the list is bound to a plan feature, deleting an entry becomes a contract breach.
The product is brought back to reality: "One-click WordPress install".

**Positioning (sales and product use the same sentence).**
*"CelikPanel does not install your application; it manages the server underneath
it. Your code's version is yours — and we give you that not as a limit but as a
guarantee."* For the technical buyer: *"Everything the panel owns exists exactly
once on the server; we never touch the version of anything that sits in your site
as N copies."* A competitor cannot make that promise honestly. **Note:** "we have
one-click too, it's called `composer create-project`" may only be used once the
run-as-site-user command endpoint **ships**; it does not exist today.

**The real work is not in the catalog.** This decision fixes what we will not do;
the gap is at L3 — today we cannot host Laravel even without a catalog entry:
- the docroot is pinned to `public_html` in `site_orchestrator.go` (Laravel needs `public/`)
- ~~the vhost served `.env` in plain text~~ → **closed** (commit `0088c67`, same day;
  the same fix also repaired ACME for static sites)
- `composer`/`artisan` appear zero times in the codebase (no endpoint runs a command as the site user)
- a queue worker is impossible behind three independent blocks (the `RunAsUser: "www-data"`
  constant, the `req.Port <= 0` rejection, the `project_type == "node"` lock)

**Rejected alternatives.** A "Laravel" project type (once an enum carries a
framework name the catalog is implicitly born, and it is irreversible because it
persists in the DB) · a panel-generated skeleton/tarball (aaPanel frozen on an old
version, Plesk broken by an upstream change) · a Softaculous-style "Frameworks"
category (15 entries: Bootstrap is a CSS framework, Kohana dead since 2016,
Symfony 2.3.42 EOL 2017 — still installable) · schematizing `.env` field by field
(it silently corrupts the customer's file on multi-line values; Forge and Ploi both
keep an opaque blob — the only legitimate "smartness" is to look at what is on
disk: copy `.env.example` if it exists) · making the scheduler a first-class
concept (it is an ordinary crontab line) · applying a preset automatically
(consent belongs to the user; otherwise ghost crons/units appear).

---

## D-010 · Catalog kinds (service/runtime/tool) + an "installed-first" default; real multi-PHP via Sury

*July 20, 2026*

**Decision.** Three linked decisions in one record:

1. **One catalog, but `ManagedService` gains a `Kind`:** `service` (a systemd
   daemon), `runtime` (a versioned interpreter), `tool` (a daemonless web tool).
   PHP-FPM and Node.js become two members of the same kind; phpMyAdmin/phpPgAdmin
   stop being "services". Row rendering branches on `Kind`, and today's
   `Daemonless = len(SystemNames)==0` heuristic is deleted — that one flag marks
   three different things today. **Versions are not rows; they live inside the
   row** (a version drawer). No separate `/runtimes` or `/apps` page.
2. **The Services page becomes "installed-first":** "hide not installed" defaults
   to ON. The catalog is the same list with the filter off (no second screen, no
   second search box). *Implementation correction (Jul 20, same day): collapsing
   is not a fixed default, it follows the view* — categories are EXPANDED in the
   installed view (the list is already short; folding it meant three clicks to
   see three services) and COLLAPSED once the catalog is shown (a long list wants
   folding). ✅ shipped: commit `312e378`.
3. **Real multi-PHP via the Sury vendor repo** (applying D-007 to PHP): on
   Debian/Ubuntu, side-by-side `php8.x-fpm` packages become installable from the
   panel. Arch has no clean path (AUR only) — there we honestly say "the distro's
   single version" (D-004: apt first-class, pacman dev-test).

**Why.** The operator's question was: "there is no Node.js/Go option; should it
go here, and won't the page get very long?" Five competing panels were studied
from their real documentation, and two things came out:

- **Page length is not a taxonomy problem, it is a default problem.** Length must
  be measured by **installed count**, not catalog size; with the filter on by
  default a clean server shows 3-4 rows whether the catalog holds 19 items or 40.
  This turns the constitution's "a service that isn't installed is invisible" from
  a checkbox into product mechanics.
- **List explosion comes from making versions into rows.** Plesk's "PHP
  interpreter versions (2 of 12 selected)" tree is the proof. One row per
  language with a version manager inside cuts the explosion at its source.

Concrete mistakes we avoid: **cPanel**'s Service Manager / Application Manager
split (the user memorizes which screen answers which question); **Plesk**'s
inconsistent runtime model (Node is an extension on Linux but a component on
Windows; Ruby lives in the CLI; Python is a checkbox); **HestiaCP**'s bug #5050
(the Edit of N version rows all lands on one version-less page — our version
endpoints are parameterized); **aaPanel**'s PM2 duplication (one capability, two
entry points — our equivalent is `AdminNodeInstall`, which is deleted: the single
address for runtime installation is Services).

**Go/Java are out of scope as runtimes.** Go compiles to a single binary; there is
no runtime to install, and the correct support already exists as the `proxy`
project type + a systemd unit. A "Go" row in the catalog would be a conceptual
error. Java/Tomcat additionally clashes with the rest of the panel through its WAR
deployment model. Ruby, PM2, Supervisor, Docker, Elasticsearch and
MongoDB/RabbitMQ/Varnish also stay out of scope.

**Three linked sub-decisions** (fixed here because the pattern will repeat across
five languages):
- **The "system interpreter" escape hatch is removed:** the panel runs only the
  runtime it installed itself (the same discipline as PHP). Accepting the `node`
  on PATH — which the panel neither installs, updates, nor removes — creates an
  invisible dependency.
- **A Node project without a web server is refused** (symmetric with php/static,
  as a coded refusal + action button under the B1 contract): today the unit comes
  up but the domain does not answer without a reverse proxy — the user sees a
  green status and a broken site at the same time.
- **A version/service in use cannot be removed:** a coded refusal + the list of
  blocking sites (`RUNTIME_IN_USE`, `SERVICE_HAS_DEPENDENTS`). Bulk version
  migration is separate work (v0.3).

**Correction to D-002.** D-002 claimed "PHP — side-by-side packages, chosen
per-site ✅ built"; the code delivers only half of that: **detection and per-site
selection were built, multi-version INSTALLATION was not** — the `php-fpm` catalog
entry has no `Repo`, and there is not one line mentioning `sury`/`ondrej` in the
codebase. The panel carries a multi-PHP interface while being able to offer a
single version: "a picker with nothing to pick". This decision closes that gap.

**Implementation corrections (Jul 21).** Writing the code sharpened two points
of this decision; both go on the record:

- **`Kind` decides CONTROL, not status.** "Row rendering branches on Kind" read,
  at first pass, as "a runtime has no running/stopped state either". Wrong:
  `php-fpm` has real units (`php8.3-fpm`…) and the scan aggregates them. A dead
  php-fpm breaks every PHP site; removing its alarm from the dashboard would be
  blindness. The right split is twofold: **only a `tool` is exempt from status**
  (it has no daemon of ours); **inline start/stop belongs to `service` alone** —
  a runtime has one unit per version, so a single "Stop" button would lie about
  "which PHP?"; its control lives in the version drawer. Field evidence:
  `nftables` (a tool) reads "inactive (dead)" while the firewall is on with 12
  rules loaded on both servers — the old behaviour raised a "1 service stopped"
  false alarm about a working firewall.
- **Catalog truth is never cached** (A7). The kind separation shipped as
  `kind:""` on the first attempt: `service_scan_cache` stored whole API
  responses, catalog fields included. The rule is now explicit: **the cache
  stores only what the scan discovered** (installed / unit up / versions /
  config files); name, description, icon, category, package names and `Kind` are
  joined from code on every read. This is the runtime counterpart of D-010's
  "the catalog stays single" — single ownership cannot coexist with a stored
  copy.

**Rejected alternatives.** A separate `/runtimes` page (a second screen = cPanel's
split) · a separate `/apps` page (the counter+list under the Node row suffices;
revisit past 30 apps) · a server-role/profile UI (the `Role` field enters the data
model now, but the filter is weighed only past 25 catalog items) · a batch-commit
planner (not written until the dependency chain runs deeper than 2 items) ·
**"just press Scan once after the kind separation"** (it would have papered over
A7 instead of fixing it: the bug was not specific to `kind` and would return on
every catalog edit).

---

## D-009 · No DNS server, no domains: the panel is authoritative for its domains

*July 9, 2026*

**Decision.** A domain can only be added when a DNS server (PowerDNS/BIND) is
installed, and the DNS server cannot be uninstalled while domains exist. Every
domain on a CelikPanel server is served by that server's own DNS — there is no
"or manage DNS at your external provider" branch.

**Why.** The operator's verdict after living with the either/or messaging:
"dns yoksa domain de olmamalı — kafa karıştırıyor." A record list that nothing
serves is a trap dressed as a feature; two valid-but-different mental models in
one dialog is one too many. One rule reads instantly: install DNS first, then
domains. This consciously supersedes the earlier "DNS can be external" stance
in D-008's first-day notes — external DNS can return later as an explicit
advanced mode if a real user needs it; the alpha optimises for coherence.

**How it holds up.** Preflight blocks creation of every domain type with an
actionable 409 (and the dialog disables all choices with one clear banner);
the mirror guard refuses uninstalling the dns-server group member while any
domain exists — otherwise every domain would silently go dark, the exact trap
the rule prevents. All "or manage externally" copy removed. Proven live:
creation without DNS → 409.

**Addendum (July 26, 2026; superseded August 18 — two-server authority).** The
old PowerDNS-only, per-zone dual-writer model is rejected because a mixed
BIND/PowerDNS pair cannot make a zone created on the directional secondary
converge back to the primary. **Paired** is now machine-directional: one primary
owns every panel-local DNS mutation and publishes an engine-neutral Catalog Zone
v2; one secondary consumes that catalog and is locally read-only. Both
nameservers remain authoritative for the resulting zones. Both panels carry the
same shared name pair; exactly one name resolves to the local IP and the other to
the peer IP. Glue is registered once under the domain that owns those names; no
child nameservers are created under customer domains. The exact NS names,
addresses and directional roles are staged before either engine is installed and
become immutable when the first engine is activated.

**Addendum (August 18, 2026 — direct directional DNS engine ownership).** PowerDNS
and BIND are alternatives behind one panel-owned DNS-engine lifecycle, not
ordinary component rows that an operator starts, stops or removes independently.
Exactly one engine owns TCP and UDP port 53. The inactive alternative may remain
installed, but it stays stopped as a rollback standby.

Selecting an engine is deliberately two-step. The first action obtains a
read-only preview containing the exact action, topology, zone and DNSSEC counts,
expected interruption, impacts and coded blockers. A separate confirmation
must present the same one-use preview authority, source, target and state
revision; a stale or blocked preview cannot mutate the server. A live-engine
cutover also requires a meaningful interruption acknowledgement, never a typed
version or product-name phrase.

PowerDNS↔BIND switching is bidirectional in both **Standalone** and verified
**Paired** topology. Topology and engine selection are independent: the primary
and secondary may use PowerDNS/PowerDNS, BIND/BIND, BIND/PowerDNS, or
PowerDNS/BIND, and either engine may be the first engine activated on either
role. No PowerDNS-first bootstrap and no temporary-engine hop is allowed. The
saved nameserver identity deterministically selects the immutable primary and
secondary roles. The primary publishes an engine-neutral Catalog Zone v2; the
secondary transfers the exact catalog and every member zone using standard
AXFR/NOTIFY.

Initial primary activation is permitted before the peer exists, but ownership
and publication readiness are separate facts: the engine remains exactly
managed and running in **pair-pending**, while every panel-local zone write is
blocked. PairReady opens publication only after all of these independent
authorities agree:

- a local catalog AXFR exactly matches the durable serial and sorted membership;
- a peer catalog AXFR, bound to the configured local paired address as its
  source, returns the same serial and membership;
- the peer catalog returns that authoritative SOA serial over both UDP and TCP;
  and
- every member zone returns its durable expected authoritative SOA serial
  locally and on the peer over UDP and TCP.

This proof is non-vacuous even with zero member zones. A deletion is complete
only when the peer zone AXFR is absent; if transfer still succeeds, the peer is
serving a stale copy and PairReady fails. The secondary never receives
panel-local write authority.

Managed BIND global options default to `allow-transfer { none; };`. A
directional BIND primary's generated catalog and member zones permit AXFR only
from exact self `LocalIP` for trusted local proof plus the exact peer, and notify
only that peer. A BIND secondary permits only its exact primary `/32`.
Directional PowerDNS primary uses exact `allow-axfr-ips=LocalIP,PeerIP` and
`also-notify=PeerIP`; directional secondary uses peer-only AXFR and omits
`also-notify`. Released legacy paired policy is accepted only as byte-exact
peer-only-plus-notify compatibility under its narrow proof. The first successful
BIND V3 publication migrates that policy transactionally with generation pointer
and state rollback. Legacy PowerDNS is not silently migrated; it stays on the
compatibility proof until an explicit reviewed switch or reconfiguration.
Legacy V2 mutation entrypoints are restricted to exact tuple-less
producer/standalone compatibility. Directional receipts and tuple-less
consumers are read-only and require the reviewed V3 switch/reconfiguration path.
A released populated consumer may leave PowerDNS only after its exact
catalog-bound member set and local/peer SOA serials are proven.
A fixed dedicated peer IPv4 is required. TSIG is not implemented; shared or
dynamic NAT endpoints are unsupported.

The catalog serial is topology authority, not engine-local state. A primary
BIND↔PowerDNS switch preserves it exactly. A membership add, delete or re-add
advances it once; a record-only update does not. Advancing the maximum serial
would wrap stale state into an apparently current value, so overflow fails
closed before publication. A released `v0.1.0-alpha.27` source receipt predates
the serial field. Such a receipt can be upgraded only when exact durable state
and the live source backend independently yield the same positive catalog
serial and membership; the derived value is then bound into the new switch
journal and target receipt. Missing or divergent evidence blocks the switch.

DNSSEC zones, pending zone publication, unmanaged DNS, a port-53 conflict, a
degraded source or another operation block a switch or reconfiguration. The
transaction freezes the complete engine-and-epoch-bound zone snapshot, stages
and validates the target before stopping the source, verifies every applicable
zone over both UDP and TCP, and retains the old package as the stopped standby.

An unresolved host left by an older release still has the narrow
**registration-only adoption of an existing panel-managed PowerDNS authority**.
Adoption verifies exact config bytes and modes, unit state, SQLite data, owned
zones, topology and TCP/UDP authority without changing them; there is no BIND
adoption shortcut. The frozen Boston path then reviews its existing **empty,
managed Standalone PowerDNS** and reconfigures that same engine directly as the
directional secondary. It takes an exact database/config/unit snapshot, writes
the catalog-consumer target, performs the bounded restart, proves the target and
commits, or restores and proves the snapshot. It never installs or activates a
temporary BIND authority. Non-empty, DNSSEC-bearing, unmanaged or divergent
legacy state fails closed instead of being silently repurposed.

Both the panel ledger and an agent-owned host journal carry the operation
identity, immutable pair identity, snapshot and phase. After interruption,
recovery proves the exact target and commits it or restores and proves the exact
pre-operation files, database, generation pointer and unit state. If neither can
be proved, DNS mutations remain locked for recovery; the panel never guesses
which daemon owns authority.

---

## D-008 · Alpha: the operator drives the panel; every gap becomes a product feature

*July 9, 2026*

**September 14 clarification.** The panel-first rule is a product acceptance
discipline, not a restriction on the owner's native administration (D-022).
For installed-server recovery, the September 13 user-operated recovery rule
applies: prefer the panel, then short verified owner-run commands when the panel
cannot recover. This does not authorize silent assistant-side configuration or
installed-panel updates. Manual recovery is not evidence that the corresponding
product flow is complete. Read the original decision below in that historical
context and subject to this clarification.

**Decision.** From the Debian 13 reinstall onward, the operator uses CelikPanel
exactly like a real customer: every install, every setting, every domain goes
through the panel, by their hand. The developer never configures the server —
not even with permission. When the operator hits a wall, the wall is the
product's fault: the missing capability is built into the panel (or the broken
one fixed) and shipped as a product update. Diagnosis over SSH stays read-only.

**Why.** Hand-fixing a server over SSH makes the product look more finished
than it is — the gap disappears from view instead of from the product. This is
the final escalation of a line that started with D-005 (install nothing) and
sharpened through "never install unasked": now the developer installs and
configures *nothing at all*. Alpha quality comes from walking into the walls.

**How it held up on day one.** One afternoon of real operator use surfaced and
fixed, in order: a domain wrongly requiring PHP (creation predated the type
system) → role-aware Add Domain with php/static/**DNS-only** types (Plesk's
"no web hosting" equivalent) and live prerequisite checks; ghost settings
pages for services that are not installed → every tab, engine list and version
dropdown now reads the server's real capability set (one endpoint,
`/api/v1/hosting/capabilities`); dead phpMyAdmin/pgAdmin links → a general
`Requires` catalogue concept (the inverse of conflict groups): dependent tools
are locked until their parent service exists, and once installed are served
loopback-only behind an admin-gated panel proxy; no way to get the panel a
real certificate → one-click Let's Encrypt in Settings with automatic renewal;
and the quietest, biggest one — the panel served `index.html` with no cache
headers, so browsers kept showing the OLD interface after every update. The
operator saw ghosts that had already been fixed; now the entry point is
`no-cache` and fingerprinted assets are immutable.

---

## D-007 · Version choice via managed vendor repositories, not a fork of the distro

*July 9, 2026*

**Decision.** A service may declare an official upstream repository (PostgreSQL
→ PGDG). Enabling it is a first-class panel action that unlocks a version pick
at install time: instead of the single major the distro froze, the admin
chooses from every current major the vendor ships. It is opt-in, curated,
signed and reversible — never on by default.

**Why.** A distro release pins one major of each database/runtime (Ubuntu noble
ships only PostgreSQL 16; Debian bookworm only 15). Customers need to land on a
specific version and stay there — this was the operator's open worry ("PG has
many versions; which one does install pick, and does it lock us to one?"). The
two bad answers are *fork the distro* (own the whole package tree forever) or
*let the OS decide* (no choice at all). The industry-standard middle path is the
vendor's own signed apt repo, which carries all current majors side by side.

**Why it stays consistent with minimal-install (D-005/D-006).** A third-party
repo widens the trust boundary, so it gets the same discipline as installing a
service: only catalog-declared repos can be enabled (the UI never passes a URL);
the signing key is pinned per-repo with apt's `signed-by=` (no global `apt-key`
trust); and disabling removes the source + key cleanly. The armoured key is used
directly, so **no `gpg` is pulled in** — the minimal footprint holds. A version
pick is bounded by the repo's package pattern, so it can never become an
arbitrary package install.

**How it holds up.**
- Catalog: `ManagedRepo` (id, name, key URL, source template with `{codename}`,
  package pattern); `postgresql` carries PGDG. Agent `EnableRepo`/`DisableRepo`/
  `RepoStatus`/`RepoPackages`; the repo — not our code — is the source of truth
  for which versions exist (discovered via `apt-cache`, newest major first).
- Panel `GET/POST /api/v1/repo` (admin-only, audited); the allowlist lives here
  (resolve repo from catalog by service id). Install accepts a version package
  only when it matches that service's repo pattern.
- Proven on the production VPS: baseline offered **only `postgresql-16`**;
  enabling PGDG unlocked **9 majors (10–18)**, and picking `postgresql-17`
  resolved to `17.10-1.pgdg24.04+1` from PGDG; disabling returned to baseline.
- Bug caught in testing: the agent runs with `UMask=0027`, so a `0644` keyring
  landed as `0640 root:celikpanel` — unreadable by apt's unprivileged `_apt`
  verifier, failing as "not signed". Fixed by `chmod 0644` after write.

**Correction (Jul 21) — the second vendor exposed the single-vendor assumptions.**
This decision was written and implemented against exactly one repo (PGDG); when
Sury was added for PHP, three of its assumptions turned out to be PGDG-specific.
The general rule: *a mechanism validated against a single example mistakes what
is specific to that example for what is general.*

1. **"KeyURL is ASCII-armoured" was wrong.** The text above states it as fact
   ("The armoured key is used directly"). PGDG publishes armoured; **Sury
   publishes a binary keyring** (`0x99` = OpenPGP public-key packet, measured on
   the server) and has no `.asc` variant (418). `fetchArmoredKey` would have
   rejected it — the decision locked out the second vendor inside its own scope.
   Both forms are now accepted and the file is named for **what it contains**
   (`.asc`/`.gpg`); apt reads both directly, so no gpg dependency is pulled in.
   Acceptance is bounded by checking the packet tag (tag 6) — an HTML error page
   cannot become a trusted keyring.
2. **Version ordering read the trailing integer.** That yields 17 for
   `postgresql-17` and 0 for `php8.3-fpm` ("fpm" is not a number). Code that
   looked correct against one vendor left every PHP version tied, so the drawer's
   order was whatever apt-cache happened to print. It now reads `(major, minor)`.
3. **The "already installed" wall.** Install asked whether the SERVICE was
   present before looking at the version pick. Harmless for PostgreSQL (one
   cluster); fatal for PHP: with 8.4 present, a request for 8.3 was refused with
   "PHP-FPM is already installed". For a plain install the question is "is this
   service here"; for a version pick it is "is **this version** here" — this was
   what actually blocked D-014's chain.

**`VersionCompanions`** was also added: a version pick installs bare
(`php8.3-fpm` alone has no database driver, mbstring or curl), so the panel would
report success for a runtime that cannot serve a site. PGDG never needed this
because `postgresql-17` works alone — the fourth face of the same single-vendor
blindness. Companions are best-effort and anything skipped is **reported** (Sury
publishes no `php8.5-opcache`; a strict install would refuse PHP 8.5 entirely
over one optional extension).

---

## D-006 · Attack-surface management: every install is reversible

*July 8, 2026*

**Decision.** Whatever the panel can install, it can uninstall. Service
removal (stop + disable + `apt purge --auto-remove`) is a first-class action
next to install, not a manual SSH chore. This is the first of a three-part
"attack-surface" line: **reversible installs**, automatic security patching,
and a firewall that only opens ports in use.

**Why.** The operator's principle, stated plainly: *every installed service or
package is a security risk — a CVE waiting, a port open.* Minimalism is not
tidiness, it is a smaller attack surface. But minimalism is only real if the
surface can shrink as well as grow: if you can add nginx but never remove it,
a mistaken or outgrown install is permanent risk. So install has a mirror.

**How it holds up.**
- `Agent.UninstallService`: stops + disables every unit, then `removePackages`
  purges (config gone, `--auto-remove` drops orphaned deps). Mirror of
  `InstallService`; same whitelist, same honest "not on this distro yet".
- Panel `POST /service/uninstall` (admin-only, audited); UI shows a
  destructive-confirm dialog listing the exact packages and warning that
  dependent sites/mail will stop.
- Proven on the production VPS: SpamAssassin installed (dpkg present) then
  uninstalled (dpkg absent, unit gone) — the surface measurably shrank.

**This line's progress (complete):** reversible installs ✅ · auto security
updates ✅ · firewall ✅. The firewall is default-deny inbound; the panel
opens exactly the panel port + each installed service's declared ports, and
the agent always keeps SSH (auto-detected from live listeners) + loopback +
established open, so a rule can never lock the operator out. Install/uninstall
re-syncs it, so the open-port set always equals the running-service set.
Proven on the VPS: enabling it left SSH, panel, web and DNS reachable while
the policy dropped everything else.

---

## D-005 · Install installs nothing but the panel; a server can be panel-only

*July 8, 2026*

**Decision.** `install.sh` installs only the panel + agent (self-contained Go
binaries + SQLite) and four tiny fetch tools (tar, xz, curl, ca-certificates)
— **nothing** for hosting. nginx, PHP, MariaDB, Postfix, PowerDNS, mail: every
one is added later from the panel, on demand. A server may stay panel-only
(e.g. someone who wants only the built-in VPN) and be managed over its IP with
no domain at all.

**Why.** The constitution: "what isn't installed is invisible." A freshly
installed server carries zero attack surface and zero cruft beyond the panel.
The operator composes exactly the server they want — a full hosting box, or
just a VPN endpoint, or just DNS. Nothing unwanted is ever present.

**How it holds up.**
- Verified: `install.sh` runs only `apt-get install tar xz-utils curl
  ca-certificates`. On the production VPS, nginx/PHP/etc. exist only because
  they were installed *from the panel* to prove the golden path — a bare
  install has none.
- Domain-less access works by design: the panel's self-signed certificate puts
  the machine's IPs in its SAN (`tmpl.IPAddresses = hostIPs()`), so IP access
  presents a matching cert (self-signed warning only, never a name mismatch).
  A domain + Let's Encrypt is an upgrade, never a requirement.

---

## D-004 · Operating system: Ubuntu LTS first, BSD option preserved (never a fork)

*July 8, 2026*

**Decision.** Ubuntu 24.04 LTS is the first-class, only-tested target. BSD is
a deliberate non-goal **for now** — but the option to support it later is
preserved by architecture, and it will **never** be a separate product.

**Why Ubuntu, not BSD.**
- The whole product sits on Linux-specific pieces: systemd (the agent's core —
  services, `celikapp-*`, `wg-quick@`), apt + package whitelists, nftables
  (VPN NAT), and the Ubuntu layout of postfix/dovecot/opendkim. Switching OS
  = rewriting the agent's "hands," months of work for zero user-visible gain.
- The stability the user admired ("versions locked, stable") **is Ubuntu LTS
  itself**: 5 years of security-only patches. BSD would not add stability we
  don't already have.
- Market reality: no competitor is on BSD (cPanel = RHEL family, Plesk =
  Linux+Windows, aaPanel = Linux). VPS images, customer habits, the
  WordPress/PHP ecosystem, Let's Encrypt tooling are all Linux-first. The
  customer asks "does my WordPress work, does my mail avoid spam?" — not
  "are you on BSD?"

**Why the option is still cheap to keep — and why NOT a fork.**
- The panel↔agent RPC split (built for security) is also OS-independence. The
  **panel** (HTTP, SQLite, UI, business logic) is portable Go; the **agent**
  is the only OS-touching layer. The panel says *what* ("create site"); only
  the agent knows *how* (systemctl/apt/nftables).
- Proven, not claimed: as of this date `make freebsd-cross` compiles **both**
  panel and agent. One portability fix was needed (Statfs_t field types differ
  Linux vs BSD — explicit `uint64` casts in `cmd/panel/system_stats_disk_unix.go`) and is
  done. The whole codebase cross-compiles to FreeBSD today.
- Two CelikPanels would be the exact bloat the user feared: every feature
  twice, every bug twice, two mediocre products. Instead, a hypothetical BSD
  future = a **BSD agent backend behind the same RPC surface** (rc.d, pkg, pf).
  Estimate: weeks, not years — the whole stack (nginx, postfix, dovecot,
  opendkim, PowerDNS, MariaDB, WireGuard) exists in FreeBSD ports; only the
  "hands" change. Panel, UI, DB, business logic: zero change.

**What we do today.** Almost nothing — and that's the point. The only cost is
discipline: new agent features keep *what* (RPC surface) separate from *how*
(exec calls), which is how the code is written anyway. No speculative
abstraction layer, no "might need it" code. The decision is made when real
demand appears (e.g. a Linux trust/security crisis pushing hosts to BSD).

**Other distros.** `detectPkgFamily` recognises apt / dnf / pacman. Today only
apt (Ubuntu/Debian) is fully supported and tested; dnf/pacman are recognised
but installs honestly say "not automatable on this distro yet" rather than
guess. Note: dnf module streams (`postgresql:16` vs `:18`) would make a real
version picker meaningful there — the UI is ready for it if we add dnf support.

*Amendment (Jul 16, operator decision):* Arch is now a **dev-test target** —
we keep a second test server on Arch precisely to keep the portability promise
honest. `install.sh` supports pacman (prerequisites, toolchain arch via
`uname -m`, honest "no security-only channel" note).

*Amendment 2 (Jul 16, widened the same day):* Testing actively on both servers,
the operator hit the "apt-only catalog" wall three times (certbot install, bind
removal, a confirm dialog showing the package name `bind9`). Decision: **the
agent's package layer now drives pacman too** (install `-S --needed`, remove
`-Rns`, presence `-Q`; never `-Syu` — no surprise system upgrade). Catalog
pacman package names are filled only where the mapping is certain AND no
distro-specific init step is needed; MariaDB/PostgreSQL (need initdb),
phpPgAdmin (AUR-only) and Redis (Valkey fork on Arch) stay deliberately empty —
they keep saying an honest "not supported yet". The UI shows package names for
the family the agent reports (`Agent.PkgFamily`).

*Amendment 3 (Aug 3, 2026):* Debian 13 joins Ubuntu 24.04 as an explicit apt
acceptance target. This does not declare an unrun test successful: a tagged,
prebuilt release must pass a clean Debian 13 installation through the exact
documented operator workflow before the release is accepted. Source checkouts,
developer-only bundles and post-install SSH fixes do not count as evidence.

*Amendment 4 (Aug 23, 2026):* D-020 supersedes the distribution/version gate in
this older decision. Ubuntu 24.04 and Debian 13 remain acceptance fixtures, not
an authorization allowlist. Any Linux name/version may select APT or pacman when
the complete fixed-path capability contract is proven; DNF remains preview-only
because its SELinux lifecycle capability is incomplete.

---

## D-003 · Service catalogue: show everything, block by conflict — not by "unmanaged"

*July 8, 2026*

**Decision.** The Services page (admin-only) lists the whole catalogue,
installed or not, with a one-click Install. We do **not** hide services we
don't deeply manage, and we do **not** clutter them with "unmanaged" labels.
The only thing that blocks an install is a **real conflict**.

**Why.** The user's insight: gating on "do we manage it?" is needless
complexity. The real constraint is whether two services can coexist.
- **Conflict groups** (`ConflictGroup` in the catalogue): services that hold
  the same role/port are mutually exclusive — a web server on :80 (Nginx ↔
  Apache), an active DNS authority on :53 (BIND ↔ PowerDNS). Generic component
  installation still refuses this conflict. The dedicated DNS-engine flow is
  the narrow exception: it may install the alternative under a no-start guard
  and keep it stopped as a verified rollback standby; it never lets both serve
  port 53.
- **No group = coexists**: MariaDB + PostgreSQL run side by side (different
  ports), so both are installable. Redis + Memcached likewise.

**Consequences.** Every install is the admin's explicit, single-click consent
(honouring the "install only with permission" principle). The catalogue is
grouped by category with an accordion (expand/collapse all) so it stays
scannable as it grows toward dozens of services.

DNS-engine installation and switching are intentionally stronger than this
generic rule: the first click is read-only review and a separate explicit
confirmation starts the durable operation. See D-009.

---

## D-002 · Service versions: honest single version, real multi-version only where it exists

*July 8, 2026*

**Decision.** The install modal shows the **real** version apt would install
(read live via `apt-cache policy`), not a version *picker*. A picker is only
offered where multiple versions genuinely exist.

**Why.** A Debian/Ubuntu release ships exactly one version of each package and
freezes it (security patches only) — that is the stability model, by design.
A dropdown with one option is a lie that misleads the user. So:
- **Single-version services** (PostgreSQL, MariaDB, nginx…): show what will
  land ("Version to install: 16"), no picker.
- **Genuinely multi-version, hosting-critical** → solved outside the distro,
  with a real picker:
  - **PHP** — side-by-side packages (`php8.1-fpm`…`php8.4-fpm`), chosen
    per-site under runtimes. ⚠️ *Correction (Jul 20, see D-010): only HALF was
    built.* Detection (`DetectInstalledPHPVersions`) and per-site selection
    work; but with no `Repo` defined for `php-fpm` in the catalog the panel
    **cannot install** multiple versions — it is limited to the distro's single
    one. The Sury vendor repo is added by D-010.
  - **Node.js** — official tarballs independent of the distro, chosen
    per-project. ✅ built.
- **Future option**: a customer wanting e.g. PostgreSQL 18 on Ubuntu 24.04
  would mean adding the vendor's own apt repo (PGDG) as a deliberate feature —
  real repo management, never a fake picker. In backlog, not built.

Pattern: **the distro's single version = safe baseline; real multi-version
(PHP, Node) via our own mechanism, independent of the distro.** Same as
cPanel/Plesk (they compile PHP themselves, leave the rest to the distro).

---

## D-001 · Update & rollback: never re-image the server

*July 8, 2026 · amended July 28, 2026*

**Current installed-panel boundary — September 14 clarification.** The
bootstrap commands below document internal/historical release mechanics; they
do not authorize updating an installed panel through SSH. Since September 10,
the user initiates every installed-panel update through CelikPanel's update UI.
Publishing and installing are separate. Exact retained-release rollback remains
the supported owner-operated recovery path until D-025's replacement executor
is implemented and verified; another release's rollback must not be substituted.

**Decision.** Production updates start only with
`sudo /bin/bash ./bootstrap-update.sh --normal`, or the one-time
`sudo /bin/bash ./bootstrap-update.sh --bootstrap-pre-ledger` transition. The
bootstrap exports the clean reviewed commit and publishes an immutable,
root-owned release below
`/var/backups/celikpanel/releases/<commit>-<nonce>/`; only that release's
`update.sh`, invoked with the explicit mode, may perform the update. A privileged
update never runs `git pull` and never executes an updater or installer from the
mutable checkout.

Every update first takes and verifies a rollback snapshot. Recovery uses only
the exact command and `VERIFIED_SNAPSHOT` printed by the update:
`sudo /bin/bash /var/backups/celikpanel/releases/<commit>-<nonce>/rollback.sh "$VERIFIED_SNAPSHOT"`.
A checkout `rollback.sh` or a rollback script from
another release is not a recovery path. The service-mutation ledger is never
created, edited, truncated, or migrated by hand; only the product's controlled
one-shot initializer may create it during the pre-ledger install flow.

**Why.** Customers on the box mean data must survive every update. Customer
data (SQLite DB, site files, mail, DNS, certs, DKIM keys) lives outside the
replaced paths (`bin/`, `web/`). After the snapshot, updates run schema
migrations offline through the staged panel's `--migrate-only` mode while both
coordinators are stopped; neither unit may restart before that step completes. The snapshot
(panel DB + binaries + unit files + source commit) makes "an update made it
worse" a recoverable event, not a disaster. Fix flow: reproduce on the dev
box (the VPS's mirror) → prove → push → stage and verify the immutable release
with [`bootstrap-update.sh`](../bootstrap-update.sh) on the VPS → use only the
printed root-trusted rollback command if needed. See
[`update.sh`](../update.sh) and [`rollback.sh`](../rollback.sh) for the internal
release and snapshot contracts.
