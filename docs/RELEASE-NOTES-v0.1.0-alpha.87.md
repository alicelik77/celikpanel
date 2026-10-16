# v0.1.0-alpha.87 (draft, not published)

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.87.tr.md)

This is the draft of the release that follows v0.1.0-alpha.86. The number
v0.1.0-alpha.87 was chosen by the release preparer under the owner's standing
instruction of 2026-10-13 ("hem sürüm numaraları hem de commit işlemleri için
bana sormana gerek yok": no need to ask the owner about version numbers or
commits); it is not a number the owner named for this release. The owner's own
part in this release is the goal it starts on (the second installed server as a
full peer of the first) and the reply "devam. sorun yok." of 2026-10-15, which
put that work after alpha.86. Every choice listed under "Decisions the release
preparer made" below is the release preparer's, made under D-021 and D-033 and
that standing instruction, and is open for the owner to confirm or change; none
of them is an owner decision. The release preparation is commit `471377d60` (sequence 87 after 86,
with the alpha.86 release commit `8d260f317e58bed7fcd84c6cbb01823c4fe3f166` as
the previous one). Nothing of it is published, and this draft records no
installed server running it. This draft was written on 2026-10-16 (UTC), after
the native records set18 and set18b; the candidate's last product commit is
`6b363ac46`.

The owner wants the second installed server (Boston, the DNS secondary of the
pair) to host websites and mail as a full peer of the first (Frankfurt, the DNS
primary). A read-only investigation of the source at main `e3c8f735a` (the main of
2026-10-13, before alpha.85 and alpha.86; the investigation is not recorded as
a file in the repository) found
eight places where the panel could not take the owner there. Five of them need
no decision from the owner, and this release corrects them: this is **part 1**.
The other three need the owner's decision and are not in this release; they
are **part 2**, and in plain words they are:

1. Should Frankfurt's own setup also carry Boston's mail server name, so that
   Frankfurt publishes that DNS record itself?
2. Should the panel have an editor for the DNS records of the server
   infrastructure on the primary (today setup writes a minimal set once)?
3. Should Boston's own records travel to Frankfurt over the remote DNS
   connection automatically, or stay a step the owner takes by hand?

Until they are decided, the DNS record of a mail server name on Boston is made
by the owner by hand, on Frankfurt or at the DNS provider, and the panel waits
for it with its guidance (below).

What this release does rests on component tests, on the sixteenth
browser-inspection batch against a mock of the API (36 states in 8
configurations; seven defects found and corrected), and on two native records
made in a lab with its own DNS and its own certificate authority: set18 at
`5d54aba1e` and set18b at `6b363ac46` (section "Native records set18 and
set18b"). set18 found one defect (D1: an extension opened on the completed
setup's run, so the owner could not review, start or stop it) and two
observations that were also corrected (O1, O3); set18b, on the candidate that
carries those corrections, observed all three fixed and no new defect. The
limits say what was never run.

## What you will see

Plain words first, for the server owner, on what the screens will show once a
server is updated to this release (it is installed nowhere yet). After the
update, reload every
CelikPanel page that was open; a page loaded before the update still has the
old screens. When a server's setup is finished, the "Your server is ready"
screen and Settings → Server setup now offer **"Extend this server's setup"**.
It adds what you choose, for example mail, through the same steps setup uses,
and says first what it does not do: it never removes, stops or replaces
anything installed, your websites keep running, and nothing starts before you
confirm the review. The review lists what changes, what will be installed and
what is kept as it is. **"Stop extending"** returns the server to its setup as
it was completed and removes nothing. On the Components page, adding a mail
stack to a server whose setup is finished now opens that same extension, so
mail gets the public DNS check, a trusted certificate and its renewal, the
reverse DNS check and the outgoing port 25 check with setup's guidance; on a
server that has not finished setup, mail is still installed directly and the
dialog now says that this ends with a temporary self-signed certificate. A
saved mail server name can be changed with "Change", and the note under the
field says what the change means before you confirm.

**Once Boston is updated to this release, it can be extended with mail from the
panel.** (This rests on component tests and the mock: no native record reached
a DNS secondary, and nothing ran on Boston.) One thing still has
to be done by hand until part 2: the DNS record of Boston's mail server name
(for example `mail.example.com`, type A, Boston's public address) must be made
on Frankfurt or at your DNS provider, because Boston is the DNS secondary and
keeps only copies of Frankfurt's zones. On a server that is the DNS secondary
of a pair, as Boston is, the Components page now says exactly this, names the
primary and shows the exact record, instead of sending you to a DNS settings
page where a secondary cannot create records (not yet seen on Boston itself). The
extension's public DNS step waits for that record, says what it waits for, and
continues by itself once the record is visible. The reverse DNS (PTR) of
Boston's address is set at your server provider, as before. Nothing here
changes a server's services by itself. Publishing this release updates no
installed server; you start that yourself, from the panel's own update screen.

## What changes for the server owner

The headings below keep the gap numbers of the investigation as pull request
#215 uses them: this release corrects gaps 1, 2, 3 and 7 and the remainder of
gap 8; the three gaps left are the owner decisions above. (The operation
guidance numbers the same five changes 1 to 5.) Every change follows D-021
(purpose-led setup that preserves what is installed), D-024 (the current
reason, who acts, the next action and how work resumes), D-032 (the panel reads
a state it can read) and D-033 (a screen says what it does and what it waits
for). D-022 is unchanged: every step uses the native service configuration
setup already used.

### 1. Extend a completed setup

**Before.** Once setup was ready, the setup page and Settings → Server setup
showed only "Your server is ready"; adding mail meant the Components page,
which did not run setup's checks (item 2).

**Now.** The ready screen and Settings → Server setup offer "Extend this
server's setup" with the sentence of what it does and does not do (it installs
and configures only what is added, never removes, stops or replaces anything
installed, the websites keep running, and nothing starts before the review).
"Extend setup" opens the wizard with the completed values: the purpose step,
the components step with the installed components marked "Installed" / "Keep
existing installation", the access step, and a review with three blocks: "What
changes" (each setting before and after), "To be installed" and, folded,
"Already installed, kept as they are". The run is the existing setup runner,
with its single flight and its "setup in progress" refusals. "Stop extending"
returns the setup to the completed draft. An extension never routes the owner
into setup; the Dashboard says that the server is being extended and keeps
working, with "Continue extending" for a draft and "Show its progress" for a
running extension.

Code. `cmd/panel/server_setup_extend.go`: `POST /api/v1/setup/extend` with the
actions `begin` and `cancel` and the typed refusals `server_setup_busy`,
`server_setup_extend_not_ready`, `server_setup_extend_not_active` and
`setup_conflict`; begin and cancel each change the state row and one setting
(`server_setup_extension_base` in `panel_settings`) in one transaction, and two
new audit actions, `server.setup.extend` and `server.setup.extend.cancel`, say
"started extending the server setup" and "stopped extending the server setup".
`cmd/panel/server_setup.go`: the setup state answers `extension` and
`extension_base` (both additive); an extension keeps `completed_at` and is not
`required`; a draft saved over a ready setup keeps the completed draft as the
base, and completing clears it. `cmd/panel/server_setup_revise.go`: reopening
an extension run's plan keeps it an extension (`99e90ac74`). The web:
`web/src/components/ServerSetup.tsx`, `ServerSetupChoice.tsx`,
`ServerSetupGate.tsx`, `web/src/lib/serverSetup.ts` (`f51d98e4b`, with the
batch 16 corrections in `b67987865`).

Tests. `cmd/panel/server_setup_extend_test.go`: six tests at `99e90ac74`
(`TestServerSetupExtendKeepsTheCompletedSetupAndItsBase`,
`TestServerSetupExtendRefusals`,
`TestServerSetupExtendAddsAMailProfileWithItsHostName`,
`TestServerSetupDraftWithMailProfile`,
`TestServerSetupExtensionMarkerFollowsTheCompletionRecord`,
`TestServerSetupReviseKeepsAnExtension`), each with a control or a refusal; the
extension marker and the revise rule were each broken once to see the tests
fail. `web/tests/server-setup-runtime.test.mjs`: mounted and decoder tests with
controls (four fail when the extension is switched off in the wizard and the
routing rule is removed).

### 2. Mail from the Components page gets setup's steps

**Before.** `POST /api/v1/service/profile/install` installed the mail profile
and ended with the self-signed fallback certificate: no public DNS check of the
mail server name, no trusted mail certificate, no renewal enrolment, no reverse
DNS or port 25 check.

**Now.** On a server whose setup has finished (or is being extended with no
run active), the Components dialog adds the profile to a setup extension and
opens setup on its access step, with the mail server name filled in and "Stop
extending". The dialog says so first ("On this server mail is added through
server setup, so it gets the same steps and checks as setup: …") and its
button reads "Review in server setup". The review and the run are setup's own:
the public DNS wait for the mail server name, the trusted certificate and its
renewal, and at the final check the reverse DNS wait with the resolver named
and an honest "not read". Nothing starts in the dialog. While the request is
answered the dialog is locked (Cancel, the name field and "Keep {hostname}"
are off); a refusal, for example a setup run in progress, keeps the dialog open
with its reason; a lost answer is settled by reading the setup again. If no
mail server name is saved, the dialog asks for it. On a server without a
finished setup the dialog installs directly as before and now says that this
ends with a temporary self-signed certificate and that the full checks are
setup's.

Code. `serverSetupDraftWithMailProfile` in `cmd/panel/server_setup_extend.go`
(`mail_profile` and `mail_hostname` on `begin`); `addProfileThroughSetup` and
`MailProfileInstallDialog` in `web/src/components/ServiceList.tsx`
(`450af0d88`; the lock and the refusal in view in `c40f14f87`). Tests:
`TestServerSetupExtendAddsAMailProfileWithItsHostName`,
`TestServerSetupDraftWithMailProfile`;
`web/tests/remote-state-mounted-batch3.test.mjs` (the setup route and its
control, a refused extension with the lock while pending and its release
after).

### 3. A saved mail server name can be corrected

**Before.** Once a mail server name was saved, the dialog only stated it.

**Now.** "Change" opens the field with the saved name. Before anything is
confirmed, the note under the field says what the change does: the mail
certificate is made for the new name, Postfix answers as the new name, and the
reverse DNS (PTR) of the server's address must point to it, set at the
provider. The name is validated as in setup. On a prepared server the
correction goes through the setup extension (the review shows the old and the
new name). On the direct path the name is now saved only once the operation is
admitted, so a refusal because a setup run or another operation holds the
server leaves the saved name unchanged; before, the name was saved first.

Code. `handleMailProfileInstall` in `cmd/panel/mail_profiles.go`
(`eddc11610`); `MailProfileInstallDialog` (`450af0d88`; the note said the new
name twice, found in a browser inspection before batch 16 and corrected in
`32f0a3766`). Tests: `TestMailHostnameCorrectionIsSavedOnlyWhenAdmitted`
(fails on the previous order), and one mounted test in
`remote-state-mounted-batch3.test.mjs`.

### 7. No "Configure DNS" dead end on a secondary DNS server

**Before.** On a server whose DNS engine is the secondary of a pair (local
mode), the mail box sent the owner to "Configure DNS", where a secondary cannot
create a record.

**Now.** The box says that this server keeps copies of the primary's zones and
cannot create records, names the primary DNS server and its address from the
saved DNS settings, says that the record is made there or at the DNS provider,
shows the exact record (the mail server name, A, this server's reviewed
address) and offers "Extend this server's setup" (or "Open server setup"
before setup has finished) to choose how this server's hosting records are
managed; there is no DNS-settings button. The profile cards then say "Waiting
for the mail DNS record" instead of "Ready to install" beside a disabled
button (`c40f14f87`).

Code. `mailProfileDNSGuidanceFor` in `cmd/panel/mail_dns_identity.go` and the
additive `mail_dns_guidance` field of the Components payload
(`cmd/panel/managed_service_handlers.go`, `60ea75c1c`); `MailProfileCards` in
`ServiceList.tsx`. The record's value is setup's reviewed address, then the
server's detected address. Tests:
`TestMailProfileDNSGuidanceOnASecondaryNamesThePrimaryAndTheRecord` and
`TestMailProfileDNSGuidanceIsAbsentOnAStandaloneServer` in
`cmd/panel/mail_hostname_edit_test.go` (the secondary with its controls: a
ready identity, external mode, a standalone server); one mounted test with a
control, extended for the waiting status.

### 8. "Check requirements again" says the typed reason

**Before.** After the owner's check, the box under the button said the generic
sentence for each open check, while the guidance above it said the typed one
(for a reverse DNS wait: the address, the name found, the resolver).

**Now.** Both use the same sentence (`setupCheckLine` in
`web/src/lib/serverSetupGuidance.ts`, `f51d98e4b`); a check that could not be
read is said as not read. The answer is scrolled into view when it arrives
(`b67987865`). Tests in `web/tests/server-setup-runtime.test.mjs`: one new and
one updated (both fail with the previous box), and a mounted re-check test (no
scroll while reading, one scroll after).

### The set18 corrections: D1, O1 and O3

The native record set18 (below) followed a fresh setup and an extension adding
Core Mail to ready in a real headless Chrome. Three of its findings are
corrected in `6b363ac46`; set18b then observed all three fixed.

- **D1 (verified failure in the lab, both routes, two browsers): an extension
  opened on the completed run.** `GET /api/v1/setup/operation` without a
  request id answers the latest run, which during an extension in draft is the
  completed setup's own succeeded run; the page then reloaded the setup, saw
  `status: draft` and showed "Verifying the result" and "Completion could not
  be verified… the server has not been declared ready", reading every 3 s,
  with the access step, the review, Start and "Stop extending" out of reach: a
  false alarm on a server that was ready. The operation answer now carries the
  additive field `superseded: true` exactly when the run succeeded and the
  setup is an extension in draft (`serverSetupRunSuperseded` in
  `cmd/panel/server_setup_operations.go`, the field in
  `server_setup_execution_context.go`); an older page ignores it and the run
  itself is still answered. The page treats a superseded run as no current
  operation (`readExecution` in `ServerSetup.tsx`) and shows the extension's
  wizard; a start marker left from that run is dropped. The decoder keeps the
  flag only on a succeeded run (`web/src/lib/serverSetupOperation.ts`), so it
  can never hide a run in progress or a failure. A first setup whose run
  succeeded without the state becoming ready still says "Completion could not
  be verified". Tests:
  `TestServerSetupOperationMarksTheCompletedRunSupersededByAnExtension` (first
  setup not ready, ready, extension in draft by both read routes, stopped,
  again, running; fails when the flag is switched off), and mounted tests in
  `server-setup-runtime.test.mjs` (the wizard and "Stop extending" with the
  flag on both routes and with a stale marker, the D1 screen without it as the
  control; both mounted D1 tests fail when the page ignores the flag).
- **O1: for about three seconds after "Start reviewed setup" the guidance said
  the generic "No more specific requirement has been confirmed for this
  step…".** The guidance now names the run's own first step and says "Setup
  has started with the step shown here. The panel begins it in a moment and
  this page updates by itself; you do not need to do anything."
  (`setup.guide.queued`, `setupExecutionGuidance`). Mounted test (fails
  without the sentence).
- **O3: "Continue recorded operation" stayed enabled for about 4 s while the
  mail enrollment was running; the tool pressed it on four samples and three
  POSTs reached the Panel** (one request id, no second enrollment). The press is now remembered until a read shows
  that the operation moved on: the button stays disabled with its spinner and
  reads "Checking the same operation" (`setup.mailEnrollment.sent`); a press
  no read confirms is offered again after 15 s. The server already answered a
  repeat of the same request without a second operation. Mounted test (fails
  when the press is not remembered).

### Corrections from the sixteenth browser batch

The sixteenth batch (below) photographed the changes above against the mock.
It found seven defects, corrected before the native records:

- while "Review in server setup" waited for its answer, Cancel, the name field
  and "Keep {hostname}" stayed usable, so the dialog could be closed or the
  name changed while setup was being asked; the dialog is now locked until the
  answer (`c40f14f87`);
- on a phone the refusal was drawn below the dialog's visible body; it is
  brought into view (`c40f14f87`);
- on a server whose mail DNS record is not ready the cards said "Ready to
  install" beside a disabled "Unavailable" button; they say "Waiting for the
  mail DNS record", and the disabled button looks disabled (`c40f14f87`);
- the answer to "Check requirements again" opened mostly below the screen; it
  is scrolled into view (`b67987865`);
- while an extension ran, the Dashboard notice and Settings said "nothing needs
  to be done here" next to "Continue extending"; the link says "Show its
  progress" (`b67987865`);
- the review drew the word "none" in the monospace face beside a host name; a
  word stays in the normal face (`b67987865`);
- "E-posta" and "PHP-FPM" broke at their hyphen at 390 px; they keep whole
  (`b67987865`).

**What does not change.** No database schema change (the ledger stays 44; no
migration between the alpha.86 tag and this candidate: `git diff --stat
8d260f317..HEAD -- internal/ cmd/` names no migration file). The extension base
is one row in `panel_settings`. Nothing under `internal/` or `cmd/agent/`
changed: the CelikPanel Agent is the same as alpha.86's. The update path, the
recovery runner, the release floor and the native services are unchanged.

## API changes

One new route, additive fields and two audit actions.

### `POST /api/v1/setup/extend`

Body `{"revision": n, "action": "begin" | "cancel"}`, and on `begin`
optionally `mail_profile` and `mail_hostname`. `begin` on a ready setup with no
run active turns it into an extension in draft (keeping `completed_at`) and
stores the completed draft as the base; on an extension already in draft (or
failed) it adds to it. `cancel` restores that draft and the ready state.
Nothing on the host changes before the reviewed start. Refusals, each `409`
with its code: `server_setup_busy` (a setup run is active),
`server_setup_extend_not_ready` (setup has not finished),
`server_setup_extend_not_active` (cancel without an extension) and
`setup_conflict` (a stale revision); a malformed request is `400`. Audit
actions `server.setup.extend` and `server.setup.extend.cancel`.

### `extension`, `extension_base` (setup state)

On the setup state answer: `extension: true` while a completed setup is being
extended, and `extension_base`, the completed draft. Absent otherwise; a client
that does not know them ignores them.

### `superseded` (setup operation)

On `GET /api/v1/setup/operation`: `superseded: true` when the answered run
succeeded and the setup is an extension in draft. Absent otherwise.

### `mail_dns_guidance` (Components payload)

On the managed-services answer, when the mail DNS record cannot be made on
this server because it is the secondary of a DNS pair: `reason`
(`dns_secondary`), `record_type`, `primary_nameserver`, `primary_ip`,
`record_name`, `record_value`. Absent otherwise.

## Behaviour changes you will notice

- The ready screen and Settings → Server setup offer "Extend this server's
  setup"; its wizard keeps installed components and its review says what
  changes, what is installed and what is kept; "Stop extending" removes
  nothing.
- On a server whose setup is finished, a mail stack from the Components page
  goes through setup's extension and its checks; on a server without a
  finished setup the direct install says it ends self-signed.
- A saved mail server name can be changed; a refused change leaves it as it
  was.
- On a DNS secondary, the mail box names the primary and the exact record
  instead of "Configure DNS", and the cards say "Waiting for the mail DNS
  record".
- The answer to "Check requirements again" says the typed reason, in view.
- A started setup names its first step at once; a pressed "Continue recorded
  operation" stays disabled as "Checking the same operation".
- Adding mail to a server set up for web hosting through "Customize selection"
  changes its purpose label to "Custom setup" (set18b O5; see the limits).

## What was measured, and how

**Component tests.** The product code of this candidate is that of
`6b363ac46`. The commits after it change no file under `web/src`, `cmd/` or
`internal/`: `0be5c8e5a` (set18b's record and harness), `90b3fe2f2` (the
intake corrections to the set18 and set18b READMEs), the merge of `main`
(`bef12820e`, the alpha.86 publication record, documents only), the release
preparation `471377d60` and these documents.

- Web: `npm test` 1499 of 1499, recorded after the set18 corrections at
  `6b363ac46` in [OPERATION-GUIDANCE](OPERATION-GUIDANCE.md) (with `npx tsc
  --noEmit` clean and `npm run build` within its bundle budget), and run again
  on 2026-10-16 at `bef12820e` with the same count (21.7 s), and again by the fact-check of these documents at
  `b6c43a4b5` (1499 of 1499; `npm run build`, which runs `tsc` first, within its
  bundle budget). The web count was
  1483 at alpha.86, 1494 when part 1 was first written (OPERATION-GUIDANCE
  entry of 2026-10-16) and 1499 after the set18 corrections.
- Go, in the WSL guest `CelikPanel-S2-Debian` (Go 1.26.5): `go test
  ./cmd/panel` 1557 top-level tests passed, 6 skipped, 1675 subtests passed,
  recorded at `60ea75c1c`; after the set18 corrections `go test -run Setup
  ./cmd/panel` 181 top-level tests passed (recorded in OPERATION-GUIDANCE),
  with `go vet` and gofmt clean. A full run after `6b363ac46` was not recorded
  before these notes, so it was made for them on 2026-10-16 at `bef12820e`
  (the same Go code as `6b363ac46`): `go test -count=1 -v ./cmd/panel/...`
  passed 1558 top-level tests, skipped 6, failed 0, and passed 1675
  subtests (272.7 s for the package, 288 s with the build), with `go vet
  ./cmd/panel/...` and gofmt clean. This run is the release preparer's
  measurement in the WSL guest; no file in the repository records it, and the
  fact-check of these documents, without a WSL guest, did not repeat it. The
  fact-check confirmed that `git diff --stat 6b363ac46 HEAD -- cmd internal
  go.mod go.sum` is empty. `./cmd/panel/...` is the single package
  `./cmd/panel`; the one top-level test more than at `60ea75c1c` is the D1
  test of `6b363ac46`.
- The Agent (`cmd/agent/`) and `internal/` were not changed on this branch and
  were not run again; their counts are alpha.86's.
- The release preparation's contract test passes: the sequence-policy contract
  in the Windows Git Bash checkout, which has the `v0.1.0-alpha.86` tag it
  reads ("86 -> 87; historical bootstrap sequence 86"; run again by the fact-check of
  these documents with the same output). The download-portal and
  signed-manifest contracts refuse in the Windows checkout, where the key-file
  checks fail (as for alpha.86); in the WSL guest, on a `git archive` of
  `471377d60`, both pass ("download portal contract passed", "signed release
  manifest contract passed", for v0.1.0-alpha.87), as the release preparer
  reported (not recorded in a file, and not re-run by the fact-check, which had
  no WSL guest).

**The sixteenth browser-inspection batch** (`x16*`,
`web/tools/browser-inspect/scenarios-batch16.mjs` and `mock-batch16.mjs`,
`5d54aba1e`). A real Chrome on Windows against the loopback mock: the ready
screen and the extension wizard (purpose, components kept, access with a new
mail server name, the review's three blocks, "Stop extending", a refused
begin), the Dashboard notice and Settings for an extension in draft and
running, the Components mail dialog on a prepared and an unprepared server
(setup route, direct route, "Change", a held then refused request), the
DNS-secondary box with and without a primary name, and the answer to "Check
requirements again" with the typed reverse DNS sentence and a check not read:
36 states (counted from the scenario source) in 8 configurations (1280×800 and
390×844, English and Turkish, light and dark). Its seven defects are listed
above. The batch's pictures are not stored in the repository and its run
results are not recorded in a file.

### Native records set18 and set18b

The source of truth for every figure in this section is the record's own
README: `deploy/e2e/release-recovery/evidence/set18-20261015/README.md` (the
twenty-first native record; `ddea67344`) and
`deploy/e2e/release-recovery/evidence/set18b-20261016/README.md` (the
twenty-second; `0be5c8e5a`). Where this summary and a README differ, the README
holds. Both are measurements, not acceptance evidence: QEMU/KVM guests,
test-licence (`acceptance_license`) builds, every `result.json` with
`native_evidence: false`, Debian 13 only. No installed server, licence service
or certificate authority was addressed, and celikpanel.net was not contacted;
the release origin's name and the certificate-authority names were pinned to
the guest's loopback as the first driver step and read back at its end (passed
in both records). They close no P0 row. The figures below follow the READMEs
as corrected at intake in `90b3fe2f2` (no raw file changed).

**The lab (lab changes, not product changes; `guest_set18_lab_dns.py`).** For
the first time in this evidence folder a lab let setup's public steps run
natively, by standing in for the internet inside the guest:

- *Lab DNS.* A small responder on 127.0.0.1 answered the zone `set18.test` and
  the reverse zone of 192.0.2.0/24, and nftables DNAT sent the public resolvers
  the product dials (1.1.1.1, 8.8.8.8, 9.9.9.9, 208.67.222.222, found by a
  search of `cmd/` and `internal/`) to it. The server's address was
  `192.0.2.10` (TEST-NET-1, "public" to the product). Two later record changes
  stood for the owner's actions at the DNS provider (the mail A record) and at
  the server provider (the PTR).
- *Lab ACME.* pebble 2.6.0 inside the guest, chosen through certbot's own user
  configuration (`/root/.config/letsencrypt/cli.ini`), with the lab root in the
  system trust; the product runs certbot without `--server`, so this needed no
  product change. Both certificates (panel and mail) came from pebble.
- *Port 443 blocked.* Outbound TCP and UDP 443 was refused except to loopback,
  the lab networks and the package hosts; the only refused 443 connection in
  each record is the lab's own self-test (reject counter `packets 1`). **No
  ACME request left the guest.**
- *Port 25.* The product's outbound-25 check was answered by the guest's own
  Postfix through DNAT, so "Outgoing mail (port 25)" passing is a property of
  the lab, not a measurement of any network; no SMTP left the guest.

**set18: the candidate at `5d54aba1e`, 2026-10-15 (UTC).** Run directory made
at 22:38:28Z, last lab ended at 23:27:49Z. The installed archive is a
policy-only fixture over `5d54aba1e` (`c84ebee3…`, labelled v0.1.0-alpha.87 /
87, previous the alpha.86 tag commit), archive SHA-256
`708e2030d5c46b9074c94a1ced72cc122d96beb5ba77419e8f9df64e6e87b0a6`. Lab s18-a
stopped before any install (H1: `nft` absent from the cloud image); lab s18-b
ran, overall `inconclusive` only because the driver's API readings failed after
the certificate handover (H2, a harness defect).

- *Setup to ready, for the first time in this evidence folder* (no earlier
  README there records "Your server is ready"). Purpose Web hosting, panel
  address `panel.set18.test`, external DNS. Every step ran natively, including
  "Verify public DNS before certificate issuance" (it succeeded between two
  samples 3 s apart and was never sampled as its own state) and "Secure panel
  access" with a trusted certificate from the lab CA and the planned Panel
  restart ("The Panel restarts at this step"), then "Your server is ready", in
  115 s. No waiting state was sampled at the final check, so "Finishing setup"
  did not appear here.
- *The Components mail route.* The dialog showed the setup route note and
  "Review in server setup"; the press answered `extension: true`, revision 4,
  with postfix and dovecot and the mail server name in the draft. It did not
  land on the access step (D1).
- *The extension to ready.* Through D1 the wizard's own requests were made by
  the page's session as recorded stand-ins (begin, plan, start). The run was
  then followed on screen: "Setup is waiting for a requirement" at "Verify
  public DNS before certificate issuance: mail.set18.test" with "Certificate
  issuance requires this public A record: mail.set18.test → 192.0.2.10. …
  Create or correct this record at the DNS provider or primary that manages it.
  … This prerequisite is checked automatically. After it passes, the same
  setup continues." for 9 min 11 s, until the A record was added; the mail
  certificate; the mail renewal enrollment (O3); at the final check "The public
  resolver 1.1.1.1 reports host-192-0-2-10.provider.set18.test as the reverse
  DNS (PTR) name of this server's address 192.0.2.10; mail servers expect
  mail.set18.test. You set this at your server provider, not in CelikPanel: …";
  after the PTR change and "Check requirements again", **"Finishing setup"**
  (1.3 s after that press: "Every check passed. The panel is completing setup
  now, and you do not need to do anything. …"; the first time in this evidence
  folder), then "Your server is ready". Followed for 689 s in the tool's
  samples (705 s from the answer to the start request to ready). Afterwards
  Postfix `myhostname = mail.set18.test`, the lab-CA certificate on 587 (993:
  the same issuer and validity, its SAN not recorded), the system hostname
  kept.
- *Findings.* **D1** (defect, verified in the lab on both routes and in two
  browsers; above). **O1** (the generic sentence for about three seconds
  after Start) and **O3** (the continue button enabled for about 4 s; three POSTs,
  one request id, no second enrollment). **O2**: after the handover the Panel
  serves its original self-signed certificate to a client that sends no SNI;
  not judged. **O4**: the handover notice offers the lab forward's port, not
  2083, before the run has a server URL; a lab artifact, not judged. H1–H4 are
  harness notes.

**set18b: the candidate at `6b363ac46`, 2026-10-16 (UTC).** Build
23:59:40Z–00:03:04Z, lab 00:03:56Z–00:17:33Z, set18's lab unchanged; installed
archive the same kind of policy-only fixture (`a97050d4…`), SHA-256
`6d645d8def2316853d7b1c191b4771c48d51fc65c925d3b0aae3e3312ad0464b`. One lab,
overall `complete-for-review`; the driver's readings after the handover
answered 200 (H2 fixed).

- *Setup to ready* in 119 s (no waiting state sampled at the final check, so
  no "Finishing setup" here); about 0.5 s after Start the guidance named "Configure
  DNS: Use an external DNS provider" with "Setup has started with the step
  shown here. …" (O1 fixed). The ready screen in Turkish at 390×844 ("Sunucunuz
  hazır", "Bu sunucunun kurulumunu genişletin", "Kurulumu genişlet"), no
  horizontal overflow.
- *The Components route* landed on the extension's access step: "Extend this
  server's setup", the mail server name prefilled `mail.set18.test`, "Stop
  extending" with "Stopping returns this server to its setup as it was
  completed. Nothing installed is removed."; in twelve samples over 6 s no mark
  of the completed run. "Stop extending" returned "Your server is ready", with
  Postfix and Dovecot not installed (from the recorded Components answer: the
  tool's own summary read a field the answer does not have, H6).
- *The extension from the ready screen*, every step pressed in the page (no
  stand-in): the purpose step with Web hosting checked, the components step
  with Nginx, PHP-FPM and MariaDB "Installed" / "Keep existing installation"
  and Postfix + Dovecot checked by the owner, the access step, and the review:
  "What changes: Server purpose: Web hosting → Custom setup; Mail server name:
  none → mail.set18.test", "To be installed: Prepare mail profile: Core Mail",
  "Already installed, kept as they are (6)"; the same in Turkish at 390×844.
  The run: the first step named at once (O1); the mail A record wait (57 s,
  until the tool's own trigger added the record; H3 fixed); the mail
  certificate; "Continue recorded operation" pressed once, then disabled as
  "Checking the same operation" 0.3 s and 2 s after the press, one POST in the
  whole run (O3 fixed); the reverse DNS sentence naming 1.1.1.1 (58 s);
  "Finishing setup"; "Your server is ready" with the label "Custom setup". Run
  205 s. Afterwards Postfix and Dovecot installed and running (the recorded
  Components answer, H6), `myhostname =
  mail.set18.test`, the lab-CA certificates on 2083 (with SNI), 587 and 993,
  certbot.timer active, the system hostname kept, ledger version 44.
- *Verdicts.* D1, O1 and O3 **fixed (observed)**. O2 and O4 unchanged, not
  judged. **No new defect.** **O5** (observation, not judged): adding Core
  Mail through "Customize selection" turns the purpose label into "Custom
  setup", in the review before Start and on the ready screen after. It is
  truthful (the selection is no longer the Web hosting preset) and stated
  before Start; the release preparer leaves the label as it is and records it
  here. The release preparer's decision to leave the label as it is rests on D-021
  and D-033 and the owner's standing instruction of 2026-10-13; it is not an
  owner decision. Whether a web server with mail added should instead keep
  "Web hosting" or read "Web and mail hosting" is open for the owner to change;
  no owner decision on it exists.

**Harness and lab-tool changes on this branch.**

- `5d54aba1e`: the sixteenth browser batch (`scenarios-batch16.mjs`,
  `mock-batch16.mjs`).
- `ddea67344`: set18's harness (`set18_fixture.py`, `build-set18-artifacts.sh`,
  `set18_trial.py`, `run-set18.sh`, the lab preparation
  `guest_set18_lab_dns.py`, `web/tools/browser-inspect/cold-load-set18.mjs`);
  `0be5c8e5a`: set18b's (`set18b_fixture.py`, `build-set18b-artifacts.sh`,
  `set18b_trial.py` with the H2 correction, `run-set18b.sh`,
  `cold-load-set18b.mjs` with the H3 correction). Lab code only.

## Decisions the release preparer made

Made by the release preparer under D-021 and D-033 and the owner's standing
instruction of 2026-10-13, and recorded in the operation guidance for the owner
to confirm; none is an owner decision.

- The extension is a new endpoint, `POST /api/v1/setup/extend`, rather than a
  flag on the existing PUT.
- An extension keeps `completed_at` and is not `required`: it never sends the
  owner back into setup.
- "Stop extending" exists and restores the completed draft; whatever a partly
  run extension installed stays installed and is managed from Components.
- The extension opens on the purpose step; one started from Components opens
  on the access step.
- On a prepared server the Components dialog no longer installs mail directly;
  it routes to setup. A server without a finished setup keeps the direct
  install, with the self-signed sentence.
- Adding a profile to a draft without components turns it into the matching
  component selection; a DNS-only purpose becomes "Custom setup"; adding mail
  to Web hosting through "Customize selection" reads "Custom setup" (O5, left
  as it is).
- On a secondary, the record's value is setup's reviewed address, then the
  server's detected address.
- The "Custom setup" label that Web hosting reads after mail is added through
  "Customize selection" (set18b O5) is left as it is.
- The commits of the branch before the release documents carry the attribution
  of the model that wrote them.

No new entry is added to [DECISIONS](DECISIONS.md): extending a completed setup
applies D-021 (setup resumes and preserves what is installed; clearing a
selection never removes an installed service), and the screens apply D-033.
Dated notes under D-021 and D-033 record how.

## Limits of this release

These are known. They are not hidden defects.

- **Part 2 is not in this release.** The three owner decisions above are open.
  On Boston (a DNS secondary) the DNS record of a mail server name is made by
  the owner by hand, on Frankfurt or at the DNS provider; the extension's
  public DNS step waits for it with its guidance and continues by itself. The
  panel does not create that record anywhere.
- **The secondary-DNS boxes were not reached natively.** Neither the
  Components box for a secondary in a DNS pair (`mail_dns_guidance`; it needs
  a completed secondary setup with a primary, not attempted) nor the setup
  access step's secondary box (in set18 D1 made it unreachable; in set18b the
  tool's click on "This server" did not change the selection, not
  investigated) was seen rendered on a native server. They rest on component
  tests and the sixteenth batch against the mock.
- **Variants not produced natively.** The reverse DNS sentence when resolvers
  disagree (both lab responders answered the same), the slow "Finishing setup"
  variant, and a final check that fails after "Finishing setup". The Panel's
  own restart was not caught as its own page state in set18b's setup.
- **A lab, not the internet.** Every public step ran against the lab's DNS
  responder and the lab's pebble; the public ACME and DNS path (Let's Encrypt,
  real resolvers, a real provider's PTR) was not measured. "Outgoing mail (port
  25)" passed because the guest's own Postfix answered.
- **Platforms.** Debian 13 only. Ubuntu 24.04 and Arch were not run.
- **The direct Components install still ends self-signed** on a server that has
  not finished setup; the dialog says so.
- **Observed in the raw files at intake, not judged** (`90b3fe2f2`; none is
  in the page-text samples, and none was investigated for this release):
  1. The Panel's journal in set18
     (`deploy/e2e/release-recovery/evidence/set18-20261015/debian13/fresh-setup-b/driver/steps/11-collect/journal-product.txt`)
     logs at each Panel start, including its restart during setup at
     22:57:19Z, `certificate startup reconcile: certificate dependents:
     publish full mail SNI snapshot: mail server hostname is not a valid FQDN:
     invalid hostname "dns-debian13"`, on a web-only setup with no mail host
     name saved (set18b's journal has the same line at each of its three Panel
     starts).
  2. The same journal logs `(CRON) DEATH (can't lock /var/run/crond.pid,
     otherpid may be 15418 …)` 128 times, from 22:56:58Z (then 22:56:59Z,
     22:57:19Z and on) to 23:27:27Z; set18b's journal holds 23 such lines.
  3. During set18's extension, while it waited for the mail A record, a
     warning toast "core-mail installed with a self-signed fallback
     certificate. Mail clients will show a trust warning until a matching
     trusted certificate is configured." was on screen
     (`set18-20261015/debian13/fresh-setup-b/browser/resume/c2-state-02-viewport.png`), although the trusted
     lab-CA mail certificate followed in the same run. A screen that says
     more than is true for that moment: a candidate D-033 item for the next
     release.
  4. The licence gate's banner "The latest access check failed. Retrying
     automatically; the current page is preserved while the last verified
     access remains valid." with "Check again" and "Reload page" was drawn
     around setup's own planned Panel restart: in set18 at the handover
     (`set18-20261015/debian13/fresh-setup-b/browser/setup/c1-state-10-viewport.png`, 22:57:18Z) and in
     set18b at the final check right after it
     (`set18b-20261016/debian13/fresh-setup-a/browser/setup/c1-state-09-viewport.png`, 00:08:29Z). The restart
     grace of alpha.86 covers the update's own restart only, not setup's
     planned restart: an item for the next release.
  Also: the browser tools of set18 and set18b summarised Components with a
  field the answer does not have (`installed` instead of `is_installed`), so
  the installed / not installed values in the READMEs and here come from the
  recorded API answers (set18b H6).
- **O2, O4 and O5 not judged.** O2: a client without SNI gets the Panel's
  original self-signed certificate after the handover. O4: a lab artifact in
  the handover address. O5: the "Custom setup" label after adding mail, left as it is by the release
  preparer's decision (the wording is open for the owner).
- **The Agent is unchanged.** Nothing under `cmd/agent/` or `internal/`
  changed; the Agent's limits of alpha.86 (the resolver sentence needs an Agent
  update) stand as they were.
- **No installed server was measured.** Nothing in this candidate has reached
  an installed server. The native records are QEMU/KVM guests with
  test-licence builds and a lab DNS and ACME; they are measurements, not
  acceptance.

Limits of v0.1.0-alpha.86 that this release does not address (each still
open), condensed; the full list is in the
[alpha.86 notes](RELEASE-NOTES-v0.1.0-alpha.86.md):

- The F1 lock extension of `7265d3c99` and the session gate's `PANEL_STARTING`
  path were not measured natively; the 6-minute overdue wording was not
  produced; the 6-minute bound rests on lab updates and one owner observation.
- The new update screen of alpha.86 is seen on an installed server only from
  the update after alpha.86, that is, on the update to this release. What the
  owner sees during that update is to be reported by the owner.
- Not reported by the owner after the alpha.86 publication: each server's
  update result and what the update page showed during the restart, the
  full-screen licence page, the Agent's version, Frankfurt's setup final step
  after the update, the output of `postconf -n >/dev/null`.
- The recovery foundation file after a return still names the defective
  candidate (set12, set15, set16, set17); the audit log stores no actor kind;
  Arch; a fresh Ubuntu install; and the limits of earlier releases that the
  alpha.86 notes list as still open.
- **Open acceptance items.** All five foundation items of the
  [resilience contract](RESILIENCE-CONTRACT.md) remain partial; this release
  closes none and advances none. P0.1–P0.5 stay **partial**. Publishing this
  release would not be acceptance of it. Decisions: D-021, D-022, D-024,
  D-025, D-032 and D-033 in [DECISIONS](DECISIONS.md).

## Publication record

Not published. Nothing is recorded here until it is published. Open before
publication:

1. The pre-publication measurement: made. set18 at `5d54aba1e` and set18b at
   `6b363ac46`, summarised in "Native records set18 and set18b" above from
   their READMEs. The candidate's product code is that of `6b363ac46`, which
   set18b measured.
2. The release preparation: done in commit `471377d60` (the number was chosen
   by the release preparer under the owner's standing instruction of
   2026-10-13), which changes no file under `cmd/`, `internal/` or `web/`: the
   release sequence policy 87 after 86 with the alpha.86 release commit
   `8d260f317e58bed7fcd84c6cbb01823c4fe3f166` as previous, the
   `download-portal/get.sh` pins (sequence 87 and version v0.1.0-alpha.87; the
   public-key pin unchanged), the README version lines and three contract
   tests. Still open: the repository variable
   `CELIKPANEL_RELEASE_SEQUENCE=87`, set before the tag. The real archive is
   built and signed by CI on the tag.
3. Pull request #215's last head gets its own CI run.
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
