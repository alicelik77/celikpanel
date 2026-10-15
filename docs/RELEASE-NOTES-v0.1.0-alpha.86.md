# v0.1.0-alpha.86 (draft, not published)

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.86.tr.md)

This is the draft of the release that follows v0.1.0-alpha.85. The number
v0.1.0-alpha.86 was chosen by the release preparer under the owner's standing
instruction of 2026-10-13 ("hem sürüm numaraları hem de commit işlemleri için
bana sormana gerek yok": no need to ask the owner about version numbers or
commits); it is not a number the owner named for this release. The release
preparation is commit `313f3eea2` (sequence 86 after 85, with the alpha.85
release commit `36c33c4531ee6c1306d324db7a666403176f3bf7` as the previous
one). Nothing of it is published, and this draft records no installed server
running it. This draft was written on 2026-10-15 (UTC).

It started with what the owner saw on 2026-10-15 while updating the second
installed server (Frankfurt) from v0.1.0-alpha.84 to v0.1.0-alpha.85 from the
panel's own update screen. While the panel restarted, as the update's own plan
says it will, the update page showed a warning-coloured notice, "The state of
the update to v0.1.0-alpha.85 could not be read just now, so it is not known
whether it has finished…", and at the same time a toast said the same thing
with "T+02:59". The update then completed normally. In substance, the owner's
words: this looks like a problem although nothing is wrong; while the operation
does what it said it would, a less alarming message is right, and the problem
wording belongs to a real problem only. This release shows the update's own
restart as calm progress, keeps the "not known" wording for after a bound, and
says it in one notice instead of two ([D-033](DECISIONS.md) and
[D-024](DECISIONS.md)).

Beside that it corrects the open items that the alpha.85 notes and the owner's
report listed: the page's language attribute, component names and descriptions
in Turkish, "Recent activity" in sentences instead of raw audit ids, server
sentences that still asked for a rescan, the platform in a mail-profile
refusal, role names in mail-profile requirements, the resolver behind the
setup's reverse DNS sentence, the reads that ran while the tab was hidden, and
the publishing script's relative-path defect. It reserves the username `system`,
because the audit log now says that actor's name as "System".

What it does rests on component tests and on two browser inspections against a
mock of the API (the thirteenth batch: the restart notice; the fourteenth
batch: the smaller corrections). When this draft was written no native record
of the candidate had been made; the section "Native record set17" is to be
filled before publication. The limits say what was never run.

## What you will see

Plain words first, for the server owner. After the update, reload every
CelikPanel page that was open; a page loaded before the update still has the
old screens. The next time you update CelikPanel from its own update screen,
the moment the panel restarts as part of that update reads "The panel is
restarting as part of the update. This page reconnects by itself; nothing needs
to be pressed.", with a spinner and the time counter, and one notice instead of
two. Only if the panel has not answered after about 6 minutes from the start of
the update (and at least 2 minutes after it stopped answering) does the notice
say that the state is not known yet, that this does not mean it failed, and
what to check on the server. That change is in the new version's screens, so
you see it from the update after this one: the update to this release is still
followed by the screens of the version you are running. On Turkish pages the
component names and their descriptions are now in Turkish, and "Recent
activity" on the Dashboard says what happened in a sentence ("System …"
for actions without a signed-in user) instead of ids such as
`server.setup.child:nginx`. No screen tells you to press Rescan any more: when
the panel's record of the components cannot be verified, the Dashboard, the
Components page or the component's own page checks by itself and says so. If
setup is waiting for a reverse DNS name, it names the public resolver that gave
the old answer once the CelikPanel Agent is updated too. Nothing here changes
your server's services. Publishing this release updates no installed server;
you start that yourself, from the panel's own update screen.

## What changes for the server owner

### The update's own restart

**Calm progress inside the expected restart.** Until now every status read that
lost the connection set the followed update's phase to unknown at once
(`systemUpdateOperationPhase(undefined)` in
`web/src/components/SystemUpdateOperation.tsx`), with no notion of the restart
the update plans: the Panel's status answer knows only queued, running (with
"verifying" once the new Panel answers), succeeded and failed, and the Agent's
updater restarts the Panel and the Agent while the record is running. Now a
read that loses the connection, or meets a 5xx answer, after the server last
said that this exact request is active (the start was accepted, or a read
answered queued or running) is the phase "restarting": a spinner, "The panel is
restarting as part of the update. This page reconnects by itself; nothing needs
to be pressed." (`panelUpdate.tracking.restarting`, and on the card
`panelUpdate.card.restarting` with the version), the T+ counter, and the update
lock unchanged. While it lasts the page reads at most every 3 s, so it
reconnects promptly (`8e27dd0eb`).

**The bound.** The warning mark and the "not known" wording appear only after
the window has passed: 6 minutes from the update's start, and at least 2
minutes from the first lost read (`systemUpdateRestartDeadline` in
`web/src/lib/systemUpdateTracking.ts`, the later of
`SYSTEM_UPDATE_EXPECTED_RESTART_MS` from the start and
`SYSTEM_UPDATE_RESTART_AFTER_LOSS_MS` from the first lost read; both on the
browser's clock, nothing compared with the server's). Under the unknown line
the notice then says that the panel has not answered for longer than an update
normally needs to restart it, that this does not mean the update failed, that
the server keeps the update's record and the notice keeps reading it, and what
the owner checks (`sudo systemctl status celikpanel-panel`)
(`panelUpdate.tracking.restartOverdue`). A verified failure is a terminal
outcome and keeps its own wording. "Do not start another update" stays.

*Where the 6 minutes come from.* The eighteenth native record (set16,
`deploy/e2e/release-recovery/evidence/set16-20261014/*/upd84-good-a/driver/steps/12-owner-start`
and `13-track`): a good update from alpha.84 to the alpha.85 candidate took
51 s on Debian 13 (start 22:55:07Z, `update_verified` 22:55:58Z) and 48 s on
Ubuntu 24.04, with the Panel's API unanswered for at most 32 s and 25 s; the
lab served the archive from a local origin. The owner's real update, with the
archive fetched over the internet, still had no answer at T+02:59. The window
is about twice the longest of these (2 × 3 min); the 2 minutes from the first
lost read are about four times the longest measured outage, for a restart that
begins late in a slow update. It rests on two lab updates and one owner
observation (see the limits).

**One notice.** After the two-minute navigation lock
(`SYSTEM_UPDATE_BLOCKING_DEADLINE_MS`) the corner notice was drawn on every
page, so on the update page the card and the corner notice both said it. Now,
while the update card is on screen it carries the T+ counter and the operation
ID, and the corner notice is not drawn for a followed update; on other pages
the corner notice is the one notice. A known outcome keeps the corner notice.
The card's unknown line now says "This page reads it again by itself" instead
of pointing at the corner notice, and the T+ counter and the ID wrap as two
units, so a phone does not break the ID mid-line (`15d80851e`).

Tests: `web/tests/update-restart-notice-mounted.test.mjs`, 13 tests (the read
classification and the window, the wording in English and Turkish, the mounted
card in the restart and past the window, the single-notice rule, and the
provider's wiring as a source check), each failing against the previous card,
provider or library; `npm test` 1404 of 1404 at `8e27dd0eb`. The provider
itself is not mounted in a test (it needs IndexedDB, Web Locks and the
authentication signal); it ran in a real Chrome against the lab mock in the
thirteenth browser batch (below).

### The publishing script

**`deploy/publish-download-portal.ps1` works from any working directory.** On
2026-10-15 the portal publication of alpha.85 first stopped at parameter
binding ("Join-Path : Cannot bind argument to parameter 'Path' because it is an
empty string"), because `$PSScriptRoot` is empty in a parameter default when
the script is started with `-File` and a relative path. `-Promoter` and
`-PublicVerifier` now default to empty and are resolved in the script body from
`$PSScriptRoot`, else `$PSCommandPath`, else `$MyInvocation.MyCommand.Path`
(`Resolve-PublisherScriptDirectory`); with none set the script refuses and asks
for the explicit parameters. Explicit values are used as given (`f89551e9b`).
Test: `deploy/test-download-portal-publisher.ps1` (CI job `publisher-windows`)
checks that no parameter default depends on the script's location,
unit-checks the resolver's order and its refusal, and runs the publisher with
`-File` from the temporary directory with a deliberately invalid version, which
must stop at the first validation (before any file, network or remote access)
and never at binding. It passes from the repository root and from `%TEMP%` and
fails against the previous publisher. The publisher was not run against any
target.

### Reads only while the page is visible

The alpha.85 notes listed as a limit that the component install dialog's 5 s
readiness re-read (`ServiceShell.tsx`) and the setup page's operation reads
(1.5 s while finishing, 3 s otherwise; `ServerSetup.tsx`) ran whatever the
tab's visibility. `web/src/lib/visibleReads.ts` (`startVisibleReads`) now runs
both: while the tab is hidden the interval is cleared and nothing is read; when
it is shown again one read runs at once and the interval resumes; a tick that
finds a read in flight is skipped; with no document the page counts as
visible. Both callers send GET only. The dialog keeps its opening read and the
setup page its mount read; the operation runs on the server and its record
stays there, so nothing needs reading while hidden (`0082c9d08`). Tests:
`web/tests/visible-reads.test.mjs` (three unit tests) and mounted tests in
`setup-finishing-mounted.test.mjs` and `remote-state-mounted.test.mjs`
(hidden: no interval; shown: one read and the interval; hidden again:
stopped), both failing against the previous components; `npm test` 1409 of
1409 at that commit.

The two re-reads that already stopped while hidden, the update card's quiet
readiness re-read and a spreading domain's re-check, used to wait their normal
delay (5 s, 30 s) when the tab was shown again. They now read once at once, as
`visibleReads.ts` does, then resume the delay from that read; a read already in
flight is that read, the domain check still counts toward its 60-check bound,
and both stay GET only (`1abe1717d`; `domain-connection-spread-mounted` and
`panel-update-readiness-poll-mounted` tests, both failing against the previous
components). This is D-033 rule 6.

### Turkish pages say Turkish

**The page's language attribute (set16 observation O1).** Turkish pages carried
`<html lang="en">`, because only `web/index.html` set it. The i18n provider now
sets `document.documentElement.lang` to the language the page is drawn in: the
target language on the boot screen and, after a switch, only once the new
catalogue has arrived (`d2c0add01`, `web/src/i18n/index.tsx`). Test:
`web/tests/document-language.test.mjs` over the real catalogues; three of its
four cases fail against the previous provider.

**Component names and descriptions (set16 observation O2).** Every catalogue id
of `internal/core` (27 ids) has an English and a Turkish display name
(`services.component.name.<id>`) and one-line description
(`services.component.description.<id>`). Product names stay as they are;
descriptive names are translated ("Firewall engine" is "Güvenlik duvarı
motoru", "Intrusion Prevention" is "Saldırı önleme"); the ids do not change,
and an id the page does not know keeps the server's name and text
(`web/src/lib/componentName.ts`, `componentName` and `componentDescription`).
They are used on the Components list, its dialogs, operation labels and search,
the component page, setup's component step and review, and the Dashboard's
stopped-service line; the cron and firewall-engine guidance name the
Components row as the page shows it (`151969774`, `ef7690fc0`). Test:
`web/tests/component-names.test.mjs` reads the ids from the Go catalogue, so a
component without both names and both descriptions fails.

### Recent activity in sentences

**Every audit action is a sentence (set16 observation O3; seen on both
installed servers after the alpha.85 update).** The Dashboard showed raw audit
ids for every action except sign-in and the mail profiles.
`cmd/panel/audit_actions.go` now lists the 179 action names the Panel writes.
A Go test, `TestAuditActionNamesListEveryActionThePanelWrites`, derives the
names from the source of every `INSERT INTO audit_logs` with go/ast, following
the action argument through parameters (per call site), closures, locals,
concatenation, `fmt.Sprintf`, string helpers and the service-operation writer
passed as a value, and fails when a writable name is missing or a listed name
is not writable; one unreachable shape is exempted with its reason
(`TestAuditActionAnalysisFollowsParametersClosuresAndLocals` checks the
analysis itself). `web/src/lib/auditAction.ts` says each name in English and
Turkish after the user's name, with the target as `{target}` (component ids by
their display name, mail profiles by their name). An action the page has no
words for is said as words, never as its id, and its target follows unchanged
("… example.com", `7956001d0`) (`8a5f299fa`). Test:
`web/tests/audit-actions.test.mjs` reads the Go list and requires a sentence in
both languages for every name; four of its five tests failed against the
previous Dashboard and catalogues. The 179 sentences live in the server
catalogue part (`web/src/i18n/screens/server/{en,tr}.ts`), because in the
general screen catalogue they pushed the Turkish chunk 10 KiB over the
per-chunk ceiling and `npm run build` failed its bundle budget; the ceiling was
not raised (`8a3729cbe`).

**The system actor, and `system` as a reserved username.** The Panel writes
the actor `system` for an action without a signed-in user. The Dashboard and
the audit log now say it as "System" / "Sistem"; any other name is shown as it
is (`4372fc86e`). So that no person can take that name,
`auth.ValidateUsername` (`internal/auth/validate.go`) refuses `system` in any
letter case (`ErrReservedUsername`). Every creation path goes through it:
account creation, team-member creation and rename, the admin CLI and its
prompt. The two HTTP paths answer `400 USERNAME_RESERVED` (typed; English and
Turkish sentence `err.USERNAME_RESERVED`); the CLI prints its usual
invalid-name message. No migration or seed creates a user named `system`; an
existing row is not touched, and a team member who already has the name can
still be edited while the name is unchanged (`8fe746582`). Tests:
`TestValidateUsernameReservesSystemActor`,
`TestReservedUsernameIsRefusedWithItsOwnCode`,
`TestReservedUsernameDoesNotBreakAnExistingMember`. Not done: the audit log
does not store an actor kind, so a pre-existing row with the username `system`
would still be drawn as "System" (it cannot arise from this Panel).

### No sentence asks for a rescan; the panel checks by itself

These follow D-032 (the panel looks by itself and never asks the owner to
press a button to learn a state it can read) and close the "Templated text"
limit of alpha.85 for the rescan wording.

- **The unverified-record refusal** (`503 SERVICE_STATE_UNVERIFIED`, already
  typed) now says that the stored component record could not be verified, so it
  is not shown and nothing was changed, and that CelikPanel checks the
  components again by itself while the Dashboard, the Components page or a
  component's page is open (`serviceStateUnverifiedMessage` in
  `cmd/panel/managed_service_handlers.go`, `cd2275e47`, `a6bdf410b`; the web
  says it from the code, `err.SERVICE_STATE_UNVERIFIED`). Test:
  `TestUnverifiedComponentRecordRefusalSaysThePanelChecksByItself`.
- **That is now true on the Dashboard and the Components page.** They answer
  this code with their own conditional check (`POST
  /api/v1/managed-services/scan?max_age_seconds=…`, a read), once, with no
  toast, as they do for a stale record. A failed check keeps the existing
  "could not check" state and is not asked again by itself (`cd2275e47`;
  mounted tests in `remote-state-mounted-batch3.test.mjs`, three of which fail
  against the previous pages).
- **And on a component's own page** (`/services/<id>`). There a `503
  SERVICE_STATE_UNVERIFIED` answer used to be the generic "This component's
  record could not be read", whose Retry kept failing until the Dashboard or
  the Components page ran its check. The page now runs the same conditional
  check (one `POST /api/v1/managed-services/scan?max_age_seconds=300`, shared by
  the reloaded-page lookup, the sections and the header; no toast), then reads
  the record again. While it runs a quiet line says that CelikPanel is checking
  the components now and that nothing needs to be done, with no Retry
  (`component.checkingStored`). A failed check keeps the existing "could not
  check" state and is not asked again by itself; a refusal that returns after a
  successful check is not checked a second time (`297f1f0b1`). A check held by
  a change in progress (`409 server_setup_busy` or `service_operation_busy`)
  says so and resumes by itself, as the Dashboard does: while the held state is
  on screen the page reads `GET /api/v1/host-mutation-readiness` (reads only;
  every 5 s, only while the tab is visible, one at a time, one poll however many
  readers the page has) and, when that read goes from busy to ready, asks for
  the record once more (one follow-up check, no loop). A change the poll never
  sees as busy, or a server that stays busy, leaves the held state in place;
  Retry stays and asks at once. `component.checkHeld` reads "This page checks
  again by itself when the change finishes; Try again asks now." (`00138c530`;
  `useStoredRecordCheck` and `runStoredRecordCheck` in
  `web/src/lib/managedServices.ts`, `ServiceRecordLookup`, `ComponentPanels`,
  `ServiceShell`). Tests: sixteen mounted cases in
  `web/tests/remote-state-mounted.test.mjs` (two entry points, eight cases
  each: the check, checking, failed, held, held resumes by itself, held not
  followed up when never or always busy, held reads only while the tab is
  visible, no loop) plus a negative control (any other refusal is unchanged);
  the resume cases fail when the follow-up or the visibility rule is removed.
- **`err.SERVICE_STATE_REFRESH_FAILED`** no longer carries a button
  instruction. It says that the page assumes neither the outcome nor the state
  and that CelikPanel checks the components again by itself while the
  Dashboard, the Components page or a component's page is open (`d2b58e7e9`,
  `00138c530`).
- **`services.stateUnverifiedHint`**, the banner after an action whose outcome
  is unknown, no longer says "Run Rescan". It says the action may have
  completed, that component changes stay locked for now, that CelikPanel checks
  the components again by itself each time the page is opened once its last
  check is a few minutes old, and that the lock lifts when that check shows the
  new state. That is what the load flow does (the conditional scan with
  `AUTO_SCAN_MAX_AGE_SECONDS`); there is no timer, so the sentence promises
  none (`8fe746582`; the test pins the words and the constant they rest on).
- **The Store's three `component_state_*` sentences** (typed by
  `blocker_reason`, localised by the server in `cmd/panel/store_backend.go`)
  say that CelikPanel checks the components by itself while an administrator
  has the Dashboard, the Components page or a component's page open, and to
  open the Store again after that check (`cd2275e47`, `21f07e7a8`). Test:
  `TestStoreComponentRecordStatesSayWhatThePanelDoes` (both languages, no
  rescan wording).
- **The Store's badge and filter for an unknown state.** When the Panel has no
  usable record of the components (none yet, older than 15 minutes,
  unreadable) it marks an offering "unsupported" with `blocker_reason
  component_state_*`. The badge beside that sentence said "Unsupported here",
  a claim about a state nobody knows (D-024). Those offerings now carry "State
  not known yet" / "Durum henüz bilinmiyor" with a clock; a real unsupported
  offering keeps its badge (`1f564fdb0`). The state filter lists them under
  their own entry, "State not known yet", not under "Unsupported here"; the
  default view still shows them (`297f1f0b1`). Test:
  `web/tests/store-state-not-known-mounted.test.mjs`, both languages, failing
  against the previous badge and grouping.

### Mail profile sentences

**The platform in `component_not_installable`.** The typed sentence of alpha.85
dropped the platform detail that only the old English text carried. The Panel
now adds `blocked_vars.platform` when the package ecosystem is verified
("Debian/Ubuntu (apt)", "Arch Linux (pacman)", "Fedora/RHEL (dnf)"; product
names, the same in both languages) and omits it when the platform is not
verified (`cmd/panel/mail_profiles.go`, `80d763949`). The page then says
"{component} cannot be installed automatically on this server's platform:
{platform}." and keeps the shorter sentence otherwise (an unverified platform
or an older Panel). Tests: in `TestMailProfileViewTypesItsSentences` (pacman
names Dovecot and the platform, dnf names the platform, an unverified platform
names none) and `web/tests/mail-profile-text.test.mjs`, each failing against
the previous source.

**Role names in a requirement.** In `component_requires`, `required` can hold a
role token (`web-server`, `smtp-server`, `imap-server`, `spam-filter`,
`dns-server`, `kv-store`: the catalogue's `ConflictGroup` values) next to
component names. The page says each role by its display name
(`services.mailProfiles.role.<token>`, English and Turkish); a role it does not
know is said as words, never as the token (`0a81043cf`; the test reads the
roles from the Go catalogue). Several requirements are now said as a list
ending in "and" / "ve" with their own plural sentence
(`component_requires.many`), and the English role names carry their article
("requires a web server"); the single sentence of the fourteenth batch read
"Rspamd requires Redis, key-value store, mail relay host, which is not part of
this profile." (`370afa2c9`).

### The resolver behind the setup's reverse DNS sentence

The final setup check said which reverse DNS name it saw, but not which public
resolver gave it, so while a change made at the provider spread the owner
could not tell a stale resolver from an unchanged record (the alpha.85 limit
"The resolver that answered is not named", and the owner's report (f) of
2026-10-15). The Agent's mail health answer now carries two additive fields,
`reverse_dns_resolver` (the IP of the resolver whose answer the reported PTR
is) and `reverse_dns_resolvers_disagree` (two resolvers gave different
answers), set only when the lookup answered (`cmd/agent/mail_health_dns.go`,
`cmd/agent/mail_health_rpc.go`, `internal/transport/mail_observability_contracts.go`).
`mailDNSIdentityAt` keeps its result and every existing field is unchanged.
The Panel's `setupMailIdentityReason` (`cmd/panel/server_setup_readiness.go`)
adds `resolver` (a valid IP only) and `resolvers=disagree` to the
`reverse_dns_mismatch` vars. Setup's final check names the resolver and, when
resolvers disagree, says that they do while a change spreads (four new English
and Turkish sentences, `setup.check.mailIdentity.reverseDNSFrom`,
`reverseDNSMissingFrom`, `reverseDNSDisagree`, `reverseDNSMissingDisagree`, in
`web/src/lib/serverSetupGuidance.ts`); the step still reads "Waiting for
reverse DNS". An older Agent sends neither field and the page keeps the
previous sentence (`e2c630faa`). Tests:
`TestMailHealthNamesTheResolverAndSaysWhenResolversDisagree` (resolver named,
agreement and disagreement, a verified identity, a failed lookup names none),
the Panel's vars in `TestServerSetupMailIdentityCheckNamesTheTypedReason` (a
non-address resolver is dropped) and mounted web tests in
`setup-final-check-mounted.test.mjs` (both languages; six of eight fail against
the previous page). This takes effect only after the Agent is updated (see the
limits).

### Corrections from the fourteenth browser batch

The fourteenth batch (below) photographed the corrections above. It found six
defects, corrected before this draft:

- an unknown audit action lost the dots of its target ("backup offsite verify
  partial example com"); the target now follows unchanged (`7956001d0`);
- after `SERVICE_STATE_REFRESH_FAILED` the Components banner said changes are
  locked and the row's fieldset did disable every button, but Manage, Manage
  DNS engine, Use built-in SFTP, the versions toggle and Uninstall kept their
  enabled look; they now dim and show the not-allowed cursor (`01861bfc9`);
- a translated description was cut with an ellipsis at 390 px with no title to
  read it whole; it now wraps (`01861bfc9`);
- the Dashboard's stopped-service link said "Go to services" / "Servislere
  git" for the page the menu calls Components; it says "Open Components" /
  "Bileşenleri aç" (`e2853c873`);
- several mail-profile requirements read as one run-on sentence; they are a
  list in the plural (`370afa2c9`);
- a Store offering whose component state is unknown carried "Unsupported here"
  (`1f564fdb0`).

A seventh correction came from `npm run build`, not from the pictures: the
bundle budget (`8a3729cbe`, above).

**What does not change.** No database schema change (the ledger stays 44; no
migration between the alpha.85 tag and this candidate) and no new route. The
update path, the recovery runner, the release floor and the native services
are unchanged. The Agent changes only its mail health answer.

## API changes

No new route. Additive fields and one new refusal code.

### `reverse_dns_resolver`, `reverse_dns_resolvers_disagree` (Agent mail health)

On the Agent's mail health answer to the Panel, next to the unchanged fields.
`reverse_dns_resolver` is the IP address of the public resolver whose answer
the reported PTR is; `reverse_dns_resolvers_disagree` is `true` when two
resolvers gave different answers. Both are omitted when the lookup did not
answer, and an older Agent never sends them. On the setup snapshot, a
`reverse_dns_mismatch` check's `vars` gain `resolver` (only a valid IP address)
and `resolvers: "disagree"`. A client that does not know them ignores them.

### `blocked_vars.platform`

On a mail profile whose `blocked_code` is `component_not_installable`, when the
server's package ecosystem is verified: `"Debian/Ubuntu (apt)"`, `"Arch Linux
(pacman)"` or `"Fedora/RHEL (dnf)"`. Absent otherwise.

### `400 USERNAME_RESERVED`

Account creation, team-member creation and team-member rename refuse the
username `system` in any letter case with `400` and the code
`USERNAME_RESERVED`. A rename that keeps an existing member's name unchanged is
not refused.

### Reworded server sentences

The English text of `503 SERVICE_STATE_UNVERIFIED` and the Store's three
`component_state_*` sentences (English and Turkish) were reworded; their codes
did not change.

## Behaviour changes you will notice

- During the panel's own restart inside an update, the update page shows a
  spinner, "The panel is restarting as part of the update…" and the T+
  counter, and the corner notice is not drawn beside the update card. The
  warning mark and "not known" come only after 6 minutes from the start and
  2 minutes from the first lost read. You see this from the update that
  follows this release.
- The page's language attribute follows the language the page is drawn in.
- On Turkish pages, component names and descriptions are Turkish.
- "Recent activity" says each action as a sentence; "System" for actions
  without a signed-in user.
- A new account or team member cannot be named `system`.
- No sentence asks you to press Rescan. A component record that cannot be
  verified is checked by the page itself, on the Dashboard, the Components page
  and a component's own page; a check held by a change resumes by itself.
- The Store shows "State not known yet" for offerings whose component state is
  unknown, with its own filter entry.
- The install dialog's and the setup page's reads stop while the tab is hidden
  and read at once when it is shown; the update card and a spreading domain
  read at once when the tab is shown again.
- Mail-profile refusals name the platform and the roles.
- Setup's reverse DNS sentence names the resolver, after an Agent update.

## What was measured, and how

**Component tests.** The product code of this candidate is that of
`21f07e7a8`; the release preparation `313f3eea2` changes no file under
`cmd/`, `internal/` or `web/`.

- Web: `npm test` 1463 of 1463, recorded at `00138c530` in
  [OPERATION-GUIDANCE](OPERATION-GUIDANCE.md) (entry of 2026-10-15) and run
  again on 2026-10-15 at the release preparation commit `313f3eea2` (same
  count; no file under `web/src` or `web/tests` changed after `00138c530`);
  `npm run build` passes with its type check and the bundle budget (the largest
  asynchronous chunk, the Turkish screen catalogue, 152.44 KiB). The web count
  was 1391 at alpha.85, 1404 after the restart notice, 1409 after the
  visibility gating, 1435 at `1abe1717d`, 1457 at `297f1f0b1` and 1463 at
  `00138c530`.
- Go, in the WSL guest `CelikPanel-S2-Debian`:
  `go test -v ./cmd/panel/... ./internal/transport/... ./internal/auth/...`
  1602 top-level tests and 1655 first-level subtests passed, 6 skipped, 0
  failed, recorded at `8fe746582`, and run again on 2026-10-15 at
  `21f07e7a8` with the same counts (after the recorded run `a6bdf410b` and
  `21f07e7a8` reworded the refusal and the Store sentences and one test under
  `cmd/panel/`);
  `go test ./cmd/agent/...` 1799 tests and 2829 subtests passed, 29 skipped,
  0 failed, recorded at `1abe1717d`, in the clean mount namespace of the
  2026-10-14 audit entry (the guest is itself a running CelikPanel host).
  After `1abe1717d` nothing under `cmd/agent/` changed and, under `internal/`,
  only `internal/auth`, which only `cmd/panel` imports; the Agent was not run
  again.
- The release preparation's contract tests pass: the sequence-policy contract
  in the Windows Git Bash checkout, which has the `v0.1.0-alpha.85` tag it
  reads ("85 -> 86; historical bootstrap sequence 85"), and the download-portal
  and signed-manifest contracts in the WSL guest.

**The thirteenth browser-inspection batch** (`u86restart`,
`web/tools/browser-inspect/scenarios-batch13.mjs`, `15d80851e`). A real Chrome
on Windows against the loopback mock ran the real update provider with the
status read dropped after a "running" read: three states (the calm restart line
on the update page, the same page past the window with the page's `Date.now`
moved 7 minutes ahead on the test side, and the corner notice on Domains during
the restart) in 8 configurations (1280×800 and 390×844, English and Turkish,
light and dark), 24 records. In the restart: a spinner, no warning mark, no
corner notice on the update page, the corner notice calm on Domains; past the
window: the warning mark and the unknown wording; no page scrolled sideways.

**The fourteenth browser-inspection batch** (`c86*`,
`web/tools/browser-inspect/scenarios-batch14.mjs`, `a45c1bfcb`). Six scenarios,
23 states (the whole catalogue of 27 components at the top and whole, a search
by a translated description, the install and uninstall dialogs, a component
page refused with `SERVICE_STATE_UNVERIFIED`, `SERVICE_STATE_REFRESH_FAILED`
with its locked rows, a stored record checked on load; the mail profile cards;
Recent activity and the stopped-service lines; five reverse DNS sentences; the
Store's three `component_state_*` sentences, at the top and whole; the document
language after a switch and back), in the same 8 configurations. Each record
adds the document language, English catalogue text on a Turkish page, raw ids,
codes and tokens, visible toasts, the disabled state of the buttons and
sideways scrolling. Its six defects are listed above. Neither batch's pictures
are stored in the repository, and the batches' run results are not recorded in
a file; the 23 states are counted from the scenario source.

### Native record set17

*To be filled from the set17 README before publication.*

**Harness and lab-tool changes on this branch.**

- `15d80851e`, `a45c1bfcb`: the thirteenth and fourteenth browser-inspection
  batches.
- `f89551e9b`: the publisher's own test (above).
- `498dc49e1` (pull request #212, merged from `main`): the WAL-trace
  tracer-death test treats a task that vanished between two `/proc` reads as
  not running (`trace_fixture.is_running`), instead of failing CI on that race.
  Lab code only.

## Limits of this release

These are known. They are not hidden defects.

New with this release:

- **No native run yet.** When this draft was written no native record of this
  candidate existed; everything above rests on component tests and the mock
  browser. The section "Native record set17" is filled before publication.
- **The 6-minute bound rests on two lab updates and one owner observation.**
  set16's two good updates (51 s and 48 s) and the owner's real update
  (T+02:59 with no answer). A real update whose restart lasts longer than 6
  minutes from its start, and longer than 2 minutes from the first lost read,
  shows the unknown wording while still being normal. The restart provider is
  not mounted in a component test; it ran in the mock browser only.
- **The new update screen is seen only from the next update.** The update to
  this release is followed by the screens of the version the server runs
  (alpha.85 on both installed servers), so the owner sees the alpha.85
  behaviour during that update.
- **The resolver sentence needs an Agent update.** The Agent fields reach a
  server only with an Agent update; until then the page keeps the previous
  sentence. The resolver comparison was not produced against real resolvers.
- **The audit log has no actor kind.** A pre-existing user row named `system`
  (which this Panel cannot create) would still be drawn as "System".
- **Store sentences are server text.** The Store's `component_state_*`
  sentences are localised by the server and promise a check only while an
  administrator has one of the three pages open; nothing checks while nobody
  looks (by design, D-032).
- **No installed server was measured.** Nothing in this candidate has reached
  an installed server.

Answered by this release, in source and with component tests (no native run):
the update screen's problem-looking notice during the expected restart (the
owner's report (c) of 2026-10-15); the publishing script's `$PSScriptRoot`
defect; the reads not gated on visibility; set16's O1 to O3, and "Recent
activity" raw ids on both installed servers (h); the "Templated text" limit
(`component_not_installable` platform, roles shown as tokens, "rescan" outside
the mail profiles); "The resolver that answered is not named" (in source; needs
an Agent update).

Limits of v0.1.0-alpha.85 that this release does not address (each still
open), condensed; the full list is in the
[alpha.85 notes](RELEASE-NOTES-v0.1.0-alpha.85.md):

- "Finishing setup" and the update card's busy state were not reached
  natively, nor the component lock's reconnect, the spreading-domain checks,
  the install dialog's re-read, the pauses and the HSTS time left; when the
  runner's own recheck finds the same unmet result, finishing ends at the 60 s
  notice.
- The next-resolver change of alpha.85 takes effect only after an Agent
  update; the owner has not reported the Agent's version.
- The recovery foundation file after a return still names the defective
  candidate while the running build is the previous release, and the floor
  reads the candidate's sequence (set12, set15, set16). Open.
- Not reported by the owner after the alpha.85 update: the full-screen licence
  page during the update, the Agent's version, Frankfurt's setup final step
  after the update, the output of `postconf -n >/dev/null`, and the result of
  Frankfurt's component check after the update.
- Arch; a fresh Ubuntu install; the Mail stacks card's "held by a change"
  state; the cold-load tools for sets 9 and 11 after their change; and the
  limits of v0.1.0-alpha.84 that the alpha.85 notes list as still open.
- **Open acceptance items.** All five foundation items of the
  [resilience contract](RESILIENCE-CONTRACT.md) remain partial; this release
  closes none and advances none. P0.1–P0.5 stay **partial**. Publishing this
  release would not be acceptance of it. Decisions: D-022, D-024, D-025, D-032
  and D-033 in [DECISIONS](DECISIONS.md).

## Publication record

Not published. Nothing is recorded here until it is published. Open before
publication:

1. The pre-publication measurement: set17, not made when this draft was
   written. Its figures go into "Native record set17" above, from its README.
2. The release preparation: done in commit `313f3eea2` (the number was chosen
   by the release preparer under the owner's standing instruction of
   2026-10-13), which changes no file under `cmd/`, `internal/` or `web/`: the
   release sequence policy 86 after 85 with the alpha.85 release commit
   `36c33c4531ee6c1306d324db7a666403176f3bf7` as previous, the
   `download-portal/get.sh` pins (sequence 86 and version v0.1.0-alpha.86; the
   public-key pin unchanged), the README version lines and three contract
   tests, which pass. Still open: the repository variable
   `CELIKPANEL_RELEASE_SEQUENCE=86`, set before the tag. The real archive is
   built and signed by CI on the tag.
3. The pull request's last head gets its own CI run.
4. Merge, the run on the main commit, tag, the check of the six signed assets
   as the signed release contract describes, and the download portal's
   publication. Under the owner's instruction of 2026-10-15 these are the
   release preparer's.
5. After the release is published, the owner of each installed server updates
   that server from the panel's own update screen. This is a separate step from
   every step above; no one else starts it.

Install this release only through CelikPanel's update interface. Publishing it
does not update installed servers; only the owner of a server starts its
update, from the panel's own update screen.
