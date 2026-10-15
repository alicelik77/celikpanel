# v0.1.0-alpha.86 (published 2026-10-15)

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.86.tr.md)

This is the release that follows v0.1.0-alpha.85. It was published on
2026-10-15 (see "Publication record" at the end). The number
v0.1.0-alpha.86 was chosen by the release preparer under the owner's standing
instruction of 2026-10-13 ("hem sürüm numaraları hem de commit işlemleri için
bana sormana gerek yok": no need to ask the owner about version numbers or
commits); it is not a number the owner named for this release. The owner's own
decisions in this release are the rule (D-033, 2026-10-14), the observation of
2026-10-15 described below, and the reply "devam. sorun yok." (publish
v0.1.0-alpha.86 with the items in hand; the second-server work comes later). The
F1 lock decision below is the release preparer's. The release
preparation is commit `313f3eea2` (sequence 86 after 85, with the alpha.85
release commit `36c33c4531ee6c1306d324db7a666403176f3bf7` as the previous
one). When the publication record was written on 2026-10-15, no installed
server had been updated to it; the owner of each starts that from the panel's
own update screen. These notes were written as a draft on 2026-10-15 (UTC),
completed the same day (UTC) after the native records set17 and set17b and the
three web corrections that followed from them, and amended for the publication
the same day; the candidate's last product commit is `7265d3c99`.

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

What it does rests on component tests, on three browser inspections against a
mock of the API (the thirteenth batch: the restart notice; the fourteenth: the
smaller corrections; the fifteenth: the licence gate, the session gate and the
navigation lock during the restart), and on two native records made on earlier
commits of this candidate: set17 at the release preparation `313f3eea2` and
set17b at `ecd2a5f08` (section "Native records set17 and set17b"). set17 found
one defect (D1: the licence gate drew its own dialog over the restart); set17b
recorded one behaviour that the release preparer then decided to change, under
D-033 and the owner's standing instruction of 2026-10-13 and not by an owner
decision (F1: the navigation lock ended in the middle of a long restart). Both
are corrected in this
candidate; the F1 correction (`7265d3c99`) came after the last native record.
The limits say what was never run.

## What you will see

Plain words first, for the server owner. After the update, reload every
CelikPanel page that was open; a page loaded before the update still has the
old screens. The next time you update CelikPanel from its own update screen,
the moment the panel restarts as part of that update reads "The panel is
restarting as part of the update. This page reconnects by itself; nothing needs
to be pressed.", with a spinner and the time counter, and one notice instead of
two. No licence or session dialog covers the page during that restart, and if
the panel is still restarting when the usual two-minute navigation lock would
end, the lock stays until the panel answers again, at the latest 6 minutes
after the update started. Only if the panel has not answered after about 6 minutes from the start of
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
(`systemUpdateOperationPhase(outcome.operation)` called with no record, in
`web/src/components/SystemUpdateOperation.tsx`; the function is in
`web/src/lib/systemUpdateTracking.ts`), with no notion of the restart
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
thirteenth browser batch, and against real Panels in QEMU guests in set17 and
set17b (below).

The native records then showed that two other parts of the page could still
replace the calm restart, and that the navigation lock could end in the middle
of a long one. Three web-only corrections follow; none changes a Go file, the
schema or a version transition, and each reads only and uses the browser's
clock only (D-032).

**The licence gate stays quiet during the restart (set17 D1, `e8aa9c5c0`).**
In one of set17's two candidate runs the licence access gate drew its own
dialog, "The Panel is not answering; an update may be restarting it", over the
calm restart at T+55.9 s, and the update's navigation lock did not come back.
The cause found in the source: the gate's last positive decision lapsed during
the outage; the gate then drew its hold and raised its observation recovery,
which `App.tsx` turns into `publishSystemUpdateAuthentication(false)`; the
update provider paused its tracking, the lock was no longer requested, and the
navigation lease releases an engaged lock that is no longer requested for good
(`useSystemUpdateNavigationLease.ts`, by design; unchanged). The provider now
publishes the followed update, once the server has said it is active, with its
first lost read (`web/src/lib/systemUpdateRestartSignal.ts`). While that update
restarts the Panel, an access read that gets no answer keeps the gate's last
positive decision until the update's own restart window ends (6 minutes from
the start, at least 2 minutes from the first lost read; never later than the
decision's own end plus 6 minutes). The gate draws nothing then, reads the
access every 5 s while the tab is visible, and once at once when the tracking
reaches the Panel again; any answer from the Panel is applied as before, and
after the window the existing dialog appears (`LicenseOnboarding.tsx`,
`SystemUpdateOperation.tsx`). No new strings. Tests: five mounted cases with a
controlled clock in `web/tests/license-onboarding-runtime.test.mjs` (four fail
against the previous gate; the no-update control passes on both) and three in
`update-restart-notice-mounted.test.mjs`. The set17 record itself says that its
folder does not establish the mechanism (the failed access reads and the
moment the gate appeared do not fit one rule); the cause above is read from the
source. set17b, on a candidate that carries this change, did not show the
dialog in two runs.

**No session hold while the new Panel starts (`b7b0f4766`).** The same class of
defect, found in the source while correcting D1; neither record shows a session
hold (set17b looked for one in all four runs; set17's sampling of other dialogs
was partial: its Debian control run did not sample them, and its tool sampled
every visible dialog only after run a of the candidate's own page). While the
new Panel starts, its management answers
`503 PANEL_STARTING`, the update's own status read included, and the fetch
interception in `App.tsx` (AuthGate) marked the session unavailable on the
first such answer: the pages were held with "The panel is starting", the
tracking paused and the lock was released for good. While the followed update
restarts the Panel such an answer no longer marks the session
(`web/src/lib/useUpdateRestartStarting.ts`; bound
`systemUpdateSessionGraceEnd`: the same window, counted from the first lost
read of the tracking or the page's first `PANEL_STARTING` answer, which is
never moved later). Inside it nothing is drawn, the tracking keeps running, and
the Panel's readiness (`/api/v1/panel/availability`) is read every 5 s while
the tab is visible; a ready answer ends the wait, a 401 still ends the session,
and when the window ends without a ready answer the session is marked as
before. No new strings. Tests: three mounted cases with a controlled clock in
`web/tests/access-hold-runtime.test.mjs` (the first two fail against the
previous `App.tsx`; the no-update control passes on both) and one in
`update-restart-notice-mounted.test.mjs`. Still open
([OPERATION-GUIDANCE](OPERATION-GUIDANCE.md), items 16 and 17): an
`AUTH_STATUS_UNAVAILABLE` answer still marks the session at once, and pages
the owner opens after the lock has ended and before the window has may show
their own refusals of a still-starting Panel (now only for a restart that
begins after the two-minute lease, or between reconnection and the update's
outcome).

**The navigation lock covers the restart (set17b F1, `7265d3c99`).** In set17b,
whose restart outage the lab made about 3 minutes long, the update's navigation
lock (its full-page dialog) ended by its two-minute lease at T+120.3 s and
T+120.5 s while the Panel was still down; the card stayed calm and "Check for
updates" stayed disabled. The release preparer decided on 2026-10-15 that the
lock covers the update's restart while its restart signal is active
(OPERATION-GUIDANCE item 17): an application of D-033 under the owner's standing
instruction of 2026-10-13, not an owner decision (set17b's README leaves the
question to the release preparer). Now, when the tracking has lost its read of
the followed update
before the two-minute lease ends, a lock that is still held is kept until the
tracking reaches the Panel again and reads the update's record, or until the
restart window ends, whichever comes first (`systemUpdateNavigationDeadline` in
`web/src/lib/systemUpdateWatchdog.ts`, `systemUpdateNavigationRestart` in
`systemUpdateRestartSignal.ts`, `useSystemUpdateNavigationLease.ts`). It never
lasts past 6 minutes from the start; a read lost at or after the two-minute
lease extends nothing; a released lock is never re-opened; the deadline is
measured against a monotonic anchor, so a wall-clock step back cannot lengthen
it. Without a restart the two-minute lease is unchanged. One string changed,
the lock line (`panelUpdate.interactionLocked`): "Navigation is temporarily
locked while this update is checked. It unlocks by itself after two minutes;
if the panel is still restarting then, once it answers again, at the latest
six minutes after the start. Tracking continues." Tests: seven mounted tests
with a controlled clock in
`web/tests/system-update-navigation-lease-mounted.test.mjs` (three fail against
the previous lease; the unchanged lease, the control and the clock test pass on
both), one wiring test in `update-restart-notice-mounted.test.mjs`, and the
line's bounds in `panel-update-ui-contract.test.mjs`. `npm test` 1483 of 1483
at `7265d3c99`. This change came after the last native record and is not
measured natively (see the limits).

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
letter case (`ErrReservedUsername`). Every path that takes a username from a
person goes through it: account creation, team-member creation and rename, the
admin CLI and its prompt (the demo-mode seed creates three fixed accounts, none
named `system`, without it). The two HTTP paths answer `400 USERNAME_RESERVED`
(typed; English and
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
defects, corrected before the release:

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
are unchanged. The Agent changes only its mail health answer. The three
corrections after set17 (`e8aa9c5c0`, `b7b0f4766`, `7265d3c99`) are web only.

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
- During that restart neither the licence dialog nor "The panel is starting"
  covers the update page, and the navigation lock lasts until the panel
  answers again when the restart is still going at two minutes (at the latest
  6 minutes from the start); its line says so.
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
`7265d3c99`: the code of `21f07e7a8` plus the three web-only corrections
`e8aa9c5c0`, `b7b0f4766` and `7265d3c99`. The release preparation `313f3eea2`
changes no file under `cmd/`, `internal/` or `web/`, and the commits after
`7265d3c99` change no file under `web/src`, `cmd/` or `internal/` (only
`web/tools`, the evidence READMEs and documents).

- Web: `npm test` 1483 of 1483, recorded at `7265d3c99` in
  [OPERATION-GUIDANCE](OPERATION-GUIDANCE.md) (counts after item 17, with
  `npx tsc --noEmit` clean and `npm run build` within its budget), and run again
  on 2026-10-15 at `155a984ea` and at `70870dfad` with the same count. Before
  the three
  corrections: 1463 of 1463, recorded at `00138c530` and run again at the
  release preparation commit `313f3eea2` (no file under `web/src` or
  `web/tests` changed between them); `npm run build` then passed with its type
  check and the bundle budget (the largest asynchronous chunk, the Turkish
  screen catalogue, 152.44 KiB; the same at `70870dfad`, where `npm run build`
  passed again). The web
  count was 1391 at alpha.85, 1404 after the restart notice, 1409 after the
  visibility gating, 1435 at `1abe1717d`, 1457 at `297f1f0b1`, 1463 at
  `00138c530` and 1483 at `7265d3c99`.
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
  again. No file under `cmd/` or `internal/` (nor `go.mod` or `go.sum`)
  changed after `21f07e7a8` (`git diff --stat 21f07e7a8 155a984ea -- cmd
  internal go.mod go.sum` is empty), so Go was not run again after the
  web-only corrections; the Go counts above are the recorded ones.
- The release preparation's contract tests pass: the sequence-policy contract
  in the Windows Git Bash checkout, which has the `v0.1.0-alpha.85` tag it
  reads ("85 -> 86; historical bootstrap sequence 85"), and, as the release
  preparer reported, the download-portal and signed-manifest contracts in the
  WSL guest (not recorded in a commit, and not re-run by the fact-check of these
  documents: they refuse in the Windows checkout, where the key-file checks
  fail).

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

**The fifteenth browser-inspection batch** (`d1restart`, `d1starting`;
`web/tools/browser-inspect/scenarios-batch15.mjs`, `59e793b2b`, `389ce2831`,
`155a984ea`), on the built candidate in the same 8 configurations. `d1restart`:
an update this browser started, its record 40 s old, access valid for 45 s,
the Panel down for 90 s across the two-minute lease, then back on the new
version. At 60 s of outage (T+01:43) and 88 s (T+02:11) the update's calm
restart dialog with the lock line and the application made inert; no licence
gate dialog, no held pages, no warning mark, no connection banner, no
horizontal overflow; after reconnection (T+02:16) no dialog and no inert
application, the card "being verified"; then the new version, in all eight.
`d1starting`: the Panel down for 25 s, then 20 s of `503 PANEL_STARTING`
(readiness "starting"), then back. At 10 s and 18 s of the start only the
update's calm restart dialog with its lock (4 and 2 requests refused with
`PANEL_STARTING` in the two intervals); no session hold, no "The panel is
starting", no gate dialog; then "Update installed, being verified" with the
lock, then the new version. The results are recorded in
[OPERATION-GUIDANCE](OPERATION-GUIDANCE.md) items 16 and 17 and in the commit
messages; the pictures are not stored in the repository.

### Native records set17 and set17b

The source of truth for every figure in this section is the record's own
README: `deploy/e2e/release-recovery/evidence/set17-20261015/README.md` (the
nineteenth native record; `4a29f4754`, seal completed in `0bf1ee601`) and
`deploy/e2e/release-recovery/evidence/set17b-20261015/README.md` (the
twentieth; `1f9495f7b`), both with their wording corrected at intake in
`d72e4528a` (no raw file changed). Where this summary and a README differ, the
README holds. Both are measurements, not acceptance evidence: QEMU/KVM guests
with outbound NAT, test-licence (`acceptance_license`) builds with
`release: false`, every `result.json` with `native_evidence: false`. No
installed server, licence service or certificate authority was addressed; the
release origin's name and the certificate-authority names were pinned to each
guest's loopback as the first driver step of every lab and read back at its
end (passed in all 7 labs of set17 and all 4 of set17b). They close no P0 row.

**set17: the candidate at the release preparation `313f3eea2`.** Run directory
made at 13:50:54Z, last lab ended at 15:57:57Z (2026-10-15, UTC). Go 1.26.5,
builds from `git clone`. No fixture commit labels the candidate: it is the
source commit itself, with its own policy (86 after 85) and the bootstrap pins
86, and `git diff --stat 21f07e7a8 313f3eea2 -- cmd internal web` is empty.

| Build | Version / seq | Commit | Archive SHA-256 |
| --- | --- | --- | --- |
| baseline | v0.1.0-alpha.85 / 85 | `36c33c45` (tag commit, local test-licence build) | `31b03ddd762e20d0a841e728b75df321ba0fe96b15e9d1a568843d49d3245c16` |
| candidate | v0.1.0-alpha.86 / 86 | `313f3eea` | `6c64289048f9e3fd98655bf761665c3b1f500ec654007da776d422654f747d39` |
| defective candidate | v0.1.0-alpha.86 / 86 | `6544ee8b1186ad42b38bd2b3b0cac1c8f07f50b1` (313f3eea + set5's migrate-only defect) | `0397f4e85a3d60b2f82391166a95909cf04e6884ed7e5a2ecb33eb105011dfbb` |
| second update's target | v0.1.0-alpha.87 / 87 | `c48678d34583868ab583db4cfb6c97c36e423db6` (policy only) | `887a16c0d3c7c1ce30371dbdc91661aadf6a9bf95663233ef8c35464d026ccfc` |
| its defective twin (never installed) | v0.1.0-alpha.87 / 87 | `8c6ac88b290b6662c8618878661b02857ff80348` | `8227bac036ac5ccdeb7fc29a4f00e1d56fad2d0b98bd2a93023458ffc56d2f86` |

The local alpha.85 build is not byte-identical to the published archive
(`cb6424b712052df2768f6c16dd6a66c8ea05a13393e71933c38ae639c3fb0792`, read once
from the published release manifest outside the labs).

| Cell | Debian 13 | Ubuntu 24.04 |
| --- | --- | --- |
| fresh install of the candidate, sign-in, licence, setup `web` | PASS (14:30:23Z–14:40:33Z) | not planned |
| update alpha.85 → candidate, good | `update_verified` (13:59:03Z–14:14:40Z) | `update_verified` (15:12:18Z–15:34:38Z) |
| update, defective → automatic return | `recovered` / `rollback_verified` (14:56:24Z–15:12:04Z) | same (15:34:51Z–15:57:57Z) |
| the candidate's own update page (candidate → v0.1.0-alpha.87 label) | twice, `update_verified` (run a 14:15:12Z–14:30:06Z, run b 14:40:47Z–14:56:04Z) | not planned |

All seven labs ran to `complete-for-review`; the six update labs' `verdicts`
step passed. The fresh install reported `v0.1.0-alpha.86`, `agent_matches true`,
`schema_version 44`, ledger `as-expected` and installed pins 86. The defective
update had one guest reset injected (Debian 15:07:28Z, SSH back after 8.91 s;
Ubuntu 15:53:19Z, 11.62 s), made two automatic attempts and no owner attempt,
and ended `phase: recovered, reason: rollback_verified, previous_failure:
update_failed`, with alpha.85 (`36c33c45…`) running and login, site marker and
firewall equal. The ledger after each return: `equal-except-volatile`,
`unexpected []`, one `managed_site_files` row before and after, changed in
`observed_at` only, `stable_sha256` equal before and after (Debian `7efbb2c8…`,
Ubuntu `45c5e796…`); 67 tables compared, five volatile tables left out. The
good updates were also `equal-except-volatile` in `observed_at` only.

*The update page during a real update* (sampled every 500 ms; T+ from the
owner's press in a real headless Chrome; the page's own start request was held
unchanged for 880 to 1202 ms until the driver's read-only observer was armed,
below the page's 15 s start timeout):

| | alpha.85, Debian (control) | alpha.85, Ubuntu (control) | candidate run a | candidate run b |
| --- | --- | --- | --- | --- |
| first lost status read | T+41.0 s | T+25.5 s | T+25.6 s | T+25.5 s |
| page outage (first lost → first answered read) | 30.1 s | 26.0 s | 35.2 s | 30.3 s |
| during the outage | "could not be read just now … not known", warning surface (59 samples) | the same (51 samples) | "The panel is restarting as part of the update…", spinner on the primary colour; 0 "not known", 0 warning | the same; 0, 0 |
| corner notice during the outage | none | none | none | none |
| other dialog over the page | none seen (not sampled in that run) | licence gate, T+36.5–51.3 s | **licence gate, T+55.9–61.0 s (D1)** | none |
| update dialog (navigation lock) | until T+77.9 s | until T+36.0 s (licence gate) | until T+55.4 s (licence gate) | until T+70.6 s |
| after reconnection | corner notice **and** card "being verified" (T+78.4–83.0 s) | both (T+51.8–60.5 s) | card "verifying", then the corner notice for the known outcome only (T+70.2–71.2 s) | dialog "Update installed, being verified", then "The update completed…" |
| new version on the page | T+85.6 s | T+63.0 s | T+71.7 s | T+71.2 s |

"Check for updates" stayed disabled until the new version showed in all four.
The bound was not approached: the longest page outage was 35.2 s from the first
lost read (bound 120 s) and the latest reconnection T+71.0 s from the press
(bound 360 s); the driver's host loop saw the Panel down for 25–35 s in each
good update.

*Browser checks on the candidate* (TR and EN; in the fresh cell and after all
four candidate updates): the Components page with 27 of 27 names and 27 of 27
descriptions equal to the catalogue; `document.documentElement.lang` `tr` on
Turkish pages and `en` on English; Recent activity in sentences, no raw id (7
rows each); the sidebar build id on one line at 1280×800 and 390×844; the
Store's filter lists "State not known yet" apart from "Unsupported here" (no
offering was in that state); and a component's own page on
`SERVICE_STATE_UNVERIFIED`, produced by a lab preparation in the fresh Debian
cell only: the quiet checking line from 0.5 s, one
`POST /api/v1/managed-services/scan?max_age_seconds=300` answered 200, the
record read again, the page filled at 1.3 / 1.6 s, no alert or toast. After the
two returns alpha.85 behaved as alpha.85 (`lang` `en`, raw ids in 6 of 7 rows,
its English server names). The setup final step's resolver sentence was not
reachable (no lab guest has a public reverse DNS name; not imitated).

*Findings.* **D1** (the licence gate during the restart; above): a verified
observation in this lab, in one of two candidate runs and one of two control
runs; not a measured failure of the update; the folder does not establish when
the gate appears. **O1** (harness): Chrome's network events do not report the
answer of the held start request (it leaves through the page's service
worker); each start was reconciled by a status read. **O2** (alpha.85): the
corner notice and the card repeat "being verified" after the lock closes; not
seen on the candidate. Open in the record: after each return the foundation
file still names the defective candidate (`release-commit=6544ee8b…`, sequence
86) while alpha.85 runs, and the floor reads 86, as in set12, set15 and set16.

**set17b: the candidate at `ecd2a5f08`, through a restart outage of about 3
minutes.** Run directory made at 17:05:07Z, last lab ended at 18:16:23Z
(2026-10-15, UTC). Against `313f3eea2` the candidate changes five product
files, all under `web/src` (D1's licence gate grace and the session gate's
`PANEL_STARTING` grace); again the source commit itself with its own policy.
Debian 13 only, four labs, one at a time. Builds (Go 1.26.5): the candidate
`ecd2a5f0` →
`4569b5eb0dba432799be5f63abeec4afca48669ead46151dd10d8ae80ccd72d0`; alpha.85
`31b03ddd…`, byte-identical to set17's local build; the v0.1.0-alpha.87 label
`20a9baa5d0ccb269528a02ec8c14229bc2ee2493` →
`34c753c6158ecedfea501d3f9c5494557f12609b61e989bbc00cecdba82cb93e`; the
defective archives (`8acfb74a842488c46a8083cca05e2bb10fc1fb97` →
`ab140cf3…`, `0aa2bb3c1394aeee5cc7e153a8b96a0e4d0ee51b` → `1df2f933…`) were
built only because the artifacts documents require them and never installed.

*The long outage is a lab change, not a product property.* A drop-in with
`ExecStartPre=/bin/sleep 150` would have been refused by the updater's own
transaction guard (it requires that systemd loads only its guard drop-in), so
no drop-in and no persistent unit file was written. A transient helper unit
(`guest_set17b_restart_delay.py`) froze the update's own worker unit with
`systemctl freeze` once the updater had renamed its `active` marker to
`completion.pending` (candidate installed, Agent and Panel stopped), held it
for 150.2 s, 150.7 s and 150.3 s (runs a, b, control), and thawed it; the
updater then started the Agent (0.6–1.1 s after the thaw) and the Panel
(1.6–1.7 s) itself. The first attempt, run a0, held nothing (H1) and is kept as
an ordinary short-outage run.

| | run a0 (not held) | run a | run b | control (alpha.85's page) |
| --- | --- | --- | --- | --- |
| pressed | 17:23:22.3Z | 17:39:19.5Z | 17:56:27.3Z | 18:11:23.3Z |
| first lost → first answered read | T+25.5 → 62.0 s | T+25.5 → 202.8 s | T+27.0 → 193.6 s | T+25.5 → 194.6 s |
| page outage | 36.5 s | **177.2 s** | **166.5 s** | **169.1 s** |
| during the outage | "restarting", 72 samples | "restarting", 347 samples | "restarting", 328 samples | "could not be read just now … not known", 333 samples from T+25.6 s |
| "not known" / warning surface | 0 / 0 | **0 / 0** | **0 / 0** | 333 / 333 |
| licence gate dialog | none | **none** | **none** | T+59.8–194.3 s (266 samples), pages held |
| failed licence access reads during the outage | 7 | 39 | 35 | 31 |
| `PANEL_STARTING` answers to the page | 0 | 0 | 0 | 0 |
| update dialog with the lock line | until the reload (T+76.6 s) | **T+0.5–119.8 s (F1)** | **T+0.5–120.0 s (F1)** | until T+59.3 s (the gate) |
| after the dialog, during the outage | – | card alone: "restarting", spinner, T+ running, no corner notice | the same | the gate's dialog; card "unknown", warning |
| "Check for updates" disabled until | T+76.6 s | T+217.3 s | T+208.2 s | T+210.5 s |
| after reconnection | "being verified", then "The update completed… Reloading" | card "verifying", then corner "completed… Reloading" | the same | corner **and** card "being verified" (T+194.8–209.0 s) |
| new version on the page | T+77.1 s | T+217.8 s | T+209.2 s | T+211.0 s |
| driver's final state | `update_verified` 17:24:33Z | 17:42:50Z | 17:59:51Z | 18:14:48Z |

Every lab's `verdicts` step passed (web, SMTP and cron never interrupted;
Panel `down-only-during-transaction`), every update ended `update_verified`
with `post-update-facts` passed and schema ledger 44 `as-expected`, and every
database comparison was `equal-except-volatile`, `managed_site_files` in
`observed_at` only. Runs a0 and a end `inconclusive` only because a harness
step's unit reading stopped early (H2); runs b and control end
`complete-for-review`. Against the bounds: the window ended at T+360 s;
reconnection came 157 s and 166 s before it, so "not known" was correctly not
reached; the 6-minute overdue wording was not produced (the outage was held
below it by design). The last good access read was 205 s and 195 s before the
last failed one in the candidate runs, without the gate's dialog; the control
drew it 60.3 s after its last good access read.

*Findings.* **D1** was not reproduced on the candidate (0 of 2 runs) and was
seen again on alpha.85 (1 of 1). Qualifiers from the record: set17b's candidate
is not set17's (it carries the gate grace written after D1); D1 was
timing-dependent in set17 (1 of 2 candidate runs), so two runs without it do
not show that it cannot occur; the mechanism is not established. **F1**: the
lock ended at T+120.3 s and T+120.5 s by the documented two-minute lease of the
measured commit `ecd2a5f08`, while the Panel was down for another 82 s and
73 s; what followed was calm. It was changed afterwards in `7265d3c99` (above);
that change is component-tested and inspected in batch 15, not measured
natively. **O1**: `PANEL_STARTING` never reached the page in four runs
(systemd reported the Panel active 1.6–1.7 s after the thaw, and the page's
next reads were already answered `200`), so the session gate's grace was not
exercised natively. **O2** (not investigated): the Panel
unit read `failed` while the updater had it stopped, and returned to `active`
at the updater's own start. **O3** (alpha.85): two notices after
reconnection, as set17's O2. **H1–H3** (harness): the updater's checkpoint
line is not in the journal of an owner's update (fixed with the marker
trigger); the unit reading under `set -eu` (fixed with `set +e`); the held
start's answer not reported by Chrome (as set17 O1). Only the update page was
observed; set17's browser checks were not repeated. Console: only
`ERR_CONNECTION_CLOSED` during the outage and two `401` before sign-in.

**Harness and lab-tool changes on this branch.**

- `15d80851e`, `a45c1bfcb`: the thirteenth and fourteenth browser-inspection
  batches; `59e793b2b`, `389ce2831`, `155a984ea`: the fifteenth.
- `4a29f4754`: set17's harness (`set17_fixture.py`, `set17_trial.py`,
  `build-set17-artifacts.sh`, `run-set17.sh`,
  `web/tools/browser-inspect/cold-load-set17.mjs`); `1f9495f7b`: set17b's
  (`set17b_trial.py`, `run-set17b.sh`, the lab preparation
  `guest_set17b_restart_delay.py`, `cold-load-set17b.mjs`). Lab code only;
  the changes made to them during each run are recorded in the READMEs.
- `f89551e9b`: the publisher's own test (above).
- `498dc49e1` (pull request #212, merged from `main`): the WAL-trace
  tracer-death test treats a task that vanished between two `/proc` reads as
  not running (`trace_fixture.is_running`), instead of failing CI on that race.
  Lab code only.

## Limits of this release

These are known. They are not hidden defects.

New with this release:

- **The native records measured earlier commits.** set17 measured the release
  preparation `313f3eea2`, set17b measured `ecd2a5f08`. The final candidate
  carries web-only changes after them: `7265d3c99` (product: the navigation
  lock through the restart), `155a984ea` (the fifteenth browser batch's
  scenario and the operation guidance; no product file) and `d72e4528a` (the
  two READMEs' wording). Not natively measured therefore: the F1 lock
  extension, that is, the lock kept past two minutes until the tracking reads
  the update's record again (at the latest 6 minutes from the start), and the
  changed lock line. It rests on component tests and the fifteenth browser
  batch against the mock. Everything else in the candidate's product code is
  the code set17b measured.
- **The session gate's `PANEL_STARTING` path was not exercised natively.**
  `PANEL_STARTING` reached the page 0 times in set17b's four runs; the grace
  rests on component tests and the batch-15 scenario `d1starting`.
- **The licence gate's quiet path rests on two native runs.** set17b did not
  show D1 in two candidate runs; D1 was timing-dependent (1 of 2 in set17),
  and its mechanism is not established by either record.
- **The 6-minute "overdue" wording was not produced natively.** Every native
  candidate run reconnected before the restart window ended (set17: page outages
  of at most 35.2 s; set17b: at most 177.2 s, held below it by design, with the
  latest reconnection at T+202.8 s from the press against a window ending at
  T+360 s). set17b's 3-minute outages were made by the
  lab (a frozen update worker): a lab change, not a product property, and not
  evidence of how long real restarts last.
- **The 6-minute bound rests on lab updates and one owner observation.**
  set16's two good updates (51 s and 48 s), the four updates set17 observed
  on the page (page outages of 26.0 to 35.2 s, reconnection at the latest
  T+71.0 s), and the owner's real update
  (T+02:59 with no answer). A real update whose restart lasts longer than 6
  minutes from its start, and longer than 2 minutes from the first lost read,
  shows the unknown wording while still being normal. The restart provider is
  not mounted in a component test; it ran in the mock browser and in the QEMU
  guests of set17 and set17b.
- **Platforms.** set17 ran Debian 13 (fresh install, update, return, the
  candidate's own page) and Ubuntu 24.04 (update and return only); set17b ran
  Debian 13 only. Ubuntu 24.04 and Arch were not run in set17b; Arch and a
  fresh Ubuntu install of the candidate were run in neither.
- **The resolver sentence was not reachable natively.** It needs the server's
  public reverse DNS name, which no lab guest has; it was not imitated.
- **Not investigated:** why the Panel unit reads `failed` while the updater
  has it stopped (set17b O2); it returned to `active` at the updater's own start
  in each of the three held runs (a, b, control).
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
  an installed server. The native records are QEMU/KVM guests with
  test-licence builds; they are measurements, not acceptance.

Answered by this release, in source and with component tests: the update
screen's problem-looking notice during the expected restart (the owner's
observation of 2026-10-15, items (b) and (c) of the operation guidance's entry
on the installed servers; also observed natively in set17 and set17b on the
earlier commits `313f3eea2` and `ecd2a5f08`, with alpha.85's page as the
control); set17's D1 (the licence gate, not reproduced in set17b's two runs);
the session gate's `PANEL_STARTING` hold (not exercised natively); set17b's F1
(not measured natively); the publishing script's `$PSScriptRoot`
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
  reads the candidate's sequence (set12, set15, set16, and again set17).
  Open.
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

Everything below is dated 2026-10-15 (clock date, UTC unless a time zone is
named). Items 1 to 3 are the preparation, written before the publication and
kept as history. The merge (item 4), the repository variable and the tag (item
6), the checks of the assets, of the portal package and of the public site, and
the portal publication (item 9) were all done by the person who prepared the
release, under the owner's instruction of 2026-10-15 recorded in the
alpha.85 notes ("bunları neden sen yapmıyorsun… benden istiyorsun": why do you
not do these yourself, why do you ask me): the merge, the repository variable,
the tag and the portal publication are the release preparer's;
installed-panel updates remain the owner's only. This entry does not store any
of the results below as a file in the repository.

**What happened.**

1. The pre-publication measurement: made. set17 at `313f3eea2` and set17b at
   `ecd2a5f08`, summarised in "Native records set17 and set17b" above from
   their READMEs. The final candidate's web change after them (`7265d3c99`)
   has no native record; the limits say so.
2. The release preparation: done in commit `313f3eea2`, which changes no file
   under `cmd/`, `internal/` or `web/`: the release sequence policy 86 after 85
   with the alpha.85 release commit
   `36c33c4531ee6c1306d324db7a666403176f3bf7` as previous, the
   `download-portal/get.sh` pins (sequence 86 and version v0.1.0-alpha.86; the
   public-key pin unchanged), the README version lines and three contract
   tests, which pass. The repository variable is item 6. The real archive was
   built and signed by CI on the tag.
3. The number: v0.1.0-alpha.86, chosen by the release preparer under the
   owner's standing instruction of 2026-10-13. The owner's reply "devam. sorun
   yok." of 2026-10-15 decided that this release ships the items in hand and
   that the second-server work follows.
4. Pull request #213 (the release candidate; CI: 24 jobs, all success or
   skipped) was squash-merged by the release preparer into `main` on
   2026-10-15. Main is commit `8d260f317e58bed7fcd84c6cbb01823c4fe3f166`, tree
   `5aaee3d01a4c80632ee3e1abec2c11c986da90df`.
5. The continuous integration run on that main commit (run 38795120470)
   concluded success, 24 jobs. The tag was pushed only after it finished, so
   the order "the run on main finished, then the tag" was followed.
6. The release preparer set the repository variable
   `CELIKPANEL_RELEASE_SEQUENCE` from 85 to 86, then created and pushed the
   annotated tag `v0.1.0-alpha.86` (tag object
   `8399668a3fc600ed3951c1c3c22739b3f4ffa35d`) on the main commit above. The
   tag's CI run (38797990342) completed successfully with 24 jobs, and the
   release was published at 2026-10-15T20:18:29Z as a pre-release with six
   assets: the two archives, their two checksum files, the signed manifest and
   its signature.
7. The assets were checked by the release preparer from a download
   (`gh release download`; checks in the `CelikPanel-S2-Debian` guest;
   reported by the preparer, not recorded in a file here). The platform
   archive and the generic archive are byte-identical, 66,159,282 bytes,
   SHA-256 `5949a0fd36f8a9f9f0308749962ec92c641df0e8cdbce72ed6b6316578a16a7d`.
   Both `.sha256` files verify. The signed manifest is 332 bytes and says
   format v2, sequence 86, version v0.1.0-alpha.86, the main commit above
   (`8d260f31…`), `published_at` 2026-10-15T19:35:12Z, linux/amd64, with the
   archive's SHA-256 and size matching. Its 64-byte Ed25519 signature verifies
   with `openssl pkeyutl -verify -rawin -pubin` against the tracked
   `deploy/release-signing-ed25519.pem` (key SHA-256
   `7eadeb0b156f1a821575c4293fe664b44b8004bcdb5e9e770122cb5c144c68bb`, the same
   as alpha.85's). The archive has 237 members. Inside it, `libexec/get.sh`
   pins sequence 86 and v0.1.0-alpha.86 (SHA-256
   `0a71748503e8e8b67766577a269379eb48f92da968f1daf00f5f6c47a286dbf7`);
   `release-sequence-policy` says 86 with previous 85,
   `36c33c4531ee6c1306d324db7a666403176f3bf7`; `release.commit` and
   `release.tree` equal the tag's. The binaries are go1.26.5, trimpath,
   CGO_ENABLED=0. The content guard and the acceptance-licence guard both
   returned 0. The interface bundle is `web/dist/assets/app-DAGE4avW.js`
   (248,116 bytes, SHA-256
   `262f8434444ad325249c8949b34f9d4cd3ee7bc2839aa088986ad8a166dba33c`;
   alpha.85's was `app-Br53Jd-B.js`). Limits: the tag's files were taken with
   `git archive` on Windows, not from a clone in a build guest, and the
   archive was not compared file by file with alpha.85's package.
8. The download portal package was built pre-signed with
   `deploy/build-download-portal.sh` (no private key used): 132,691,628 bytes,
   SHA-256 `a36496cdca0547609b12e2aaa0d103127d5ea3ed9cebd976a857dc865151e72b`,
   54 members, built twice byte-identical; `releases/latest.txt` says
   v0.1.0-alpha.86. The promoter's package rules accepted it. At the tag, the
   portal contract test returned 0, the public-verifier tests passed 17 of 17
   and the promotion tests 14 of 14. Limits: no promoter rehearsal against a
   copy of the live site was made, and `deploy/test-download-portal-publisher.ps1`
   was not run.
9. The portal was published by the release preparer with
   `deploy/publish-download-portal.ps1`, started with its absolute path and an
   explicit `-Promoter` and `-PublicVerifier`, and `-PreviousVersion
   v0.1.0-alpha.85 -TargetVersion v0.1.0-alpha.86 -TransferTimeoutSeconds
   1800`. Output: `CELIKPANEL_PORTAL_UPLOAD_READY`,
   `CELIKPANEL_PORTAL_UPLOAD_PINNED`, the promoter's result `status: committed`
   with previous_version v0.1.0-alpha.85 and the backup
   `httpdocs.before-v0.1.0-alpha.86.20261015T202726Z…`, public verification
   `status: ok` (26 of 26 requests), then
   `CELIKPANEL_DOWNLOAD_PORTAL_PUBLISHED`. Because the script was started with
   its absolute path, this invocation did not exercise the correction of the
   relative-path defect (see "The publishing script" above).
10. A read from outside at 2026-10-15 20:27:35Z (read-only; release preparer):
    `releases/latest.txt` is v0.1.0-alpha.86 (it read v0.1.0-alpha.85 at
    20:24:59Z, before the publication); `get.sh` is served with
    `Strict-Transport-Security: max-age=15768000; includeSubDomains` and pins
    sequence 86; `releases/latest.json` names sequence 86, commit `8d260f31…`
    and SHA-256 `5949a0fd…`. Not reported: a byte comparison of the other
    published files with the GitHub assets.
11. Installed servers, at the time of this record: not updated. Only the owner
    starts that, from the panel's own update screen. The outcome will be
    recorded separately.

**Not done, and what publication does not change.**

- The installed servers were not updated when this record was written. The
  owner of each starts that from the panel's own update screen, as a separate
  step from every step above, and no one else starts it. To be reported by the
  owner later, and not recorded until then: both servers' update result and
  what the update page showed during the restart (the calm notice, no gate
  dialog, the lock); whether the full-screen licence page appeared during the
  update; the Agent's version; Frankfurt's setup final step after the update;
  the output of `postconf -n >/dev/null`. Nothing in this record describes
  what an installed server did with this release. The update from alpha.85 to
  this release runs on alpha.85's update screen; the new restart notice is
  seen on an installed server only from the update after this one (see "Limits
  of this release").
- Limits of the checks above: the assets' files were taken with `git archive`
  on Windows, not from a clone in a build guest, and the archive was not
  compared with alpha.85's package file by file (item 7); no promoter rehearsal
  against a copy of the live site was made and the publisher's own test was not
  run (item 8).
- Publication is not product acceptance. The limits listed under "Limits of
  this release" are unchanged by it, among them that the F1 lock extension of
  `7265d3c99` and the session gate's `PANEL_STARTING` path are not measured
  natively. All five foundation items of the
  [resilience contract](RESILIENCE-CONTRACT.md) remain partial; P0.1 to P0.5
  stay partial and no P0 item is closed or advanced by the release or by its
  publication.

Install this release only through CelikPanel's update interface. Publishing it
does not update installed servers; only the owner of a server starts its
update, from the panel's own update screen.
