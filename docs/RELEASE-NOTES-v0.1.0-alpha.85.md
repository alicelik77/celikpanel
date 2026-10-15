# v0.1.0-alpha.85 (draft, not published)

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.85.tr.md)

This is the draft of the release that follows v0.1.0-alpha.84. The number is
v0.1.0-alpha.85; the release preparation is commit 5443fe055 (sequence 85 after
84, with the alpha.84 release commit `f0157e6c8` as the previous one). Nothing
of it is published, and no installed server runs it. This draft was written on
2026-10-15 (UTC).

It does one thing the owner asked for on 2026-10-14, everywhere: a screen that
makes you wait now shows that it is waiting, or says plainly that you do not
need to do anything ([D-033](DECISIONS.md)). It started at the last step of
setup on the second installed server. There the final check waited for a
reverse DNS name that the owner had already changed at the provider and that
public DNS was still spreading. The step said "Waiting for you", and after every
check passed it showed a still sentence beside enabled buttons until the next
20-second pass. That step now finishes on the owner's passing check, shows
"Finishing setup" with motion, and names the reverse DNS wait for what it is.
The rest of the interface was then read for every state in which a page waits:
37 states, of which 18 already met the rule, 16 were changed at once, and the
3 left open were decided by the owner the same day and changed too.

Beside that: observations O4 to O6 of the alpha.84 notes are corrected (the
Components firewall banner and the Dashboard's "Needs attention" box), the
Dashboard's Mail stacks card checks with the page, mail profile sentences are
typed and said in the page's language, the HSTS retirement no longer compares
the browser's clock with the server's, the sidebar's build id stays in one
piece, and the Agent asks the next resolver when the first reverse DNS answer
is stale.

What it does rests on component tests, on a browser inspection against a mock
of the API (the twelfth batch: 31 states in 8 configurations), and on one lab
measurement: the eighteenth native record (set16, 2026-10-14 UTC), a fresh
install on Debian 13 and the owner's update from alpha.84 on Debian 13 and
Ubuntu 24.04, good and defective, in a real headless Chrome. The limits say
what was never run. Two of the waiting states, setup finishing (the one the
owner met) and the update card's busy state, could not be reached in the lab.

## What you will see

Plain words first, for the server owner. After the update, reload every
CelikPanel page that was open; a page loaded before the update still has the
old screens. When a page waits for the server, a provider or a background job,
you now see either something moving (a spinner beside the sentence, with the
buttons that would get in the way switched off) or a sentence that says nothing
needs to be done and the page updates by itself. A wait has an end: if nothing
is confirmed after about a minute, the page says the result is not known yet,
that this is not a failure, and what to check. If setup is waiting for a
reverse DNS name you have already changed at your server provider, the step
now reads "Waiting for reverse DNS" and tells you that the change can take from
a few minutes to several hours to reach public DNS and that you do not need to
do anything more in CelikPanel. When you press "Check requirements again" and
every check passes, the step reads "Finishing setup" and completes within
seconds instead of after the next 20-second pass. With the CelikPanel Agent
stopped, the Turkish Components page and the Dashboard's "Needs attention" box
now say so in the page's language, with the commands to run. Nothing here
changes your server's services. Publishing this release updates no installed
server; you start that yourself, from the panel's own update screen.

## What changes for the server owner

**The last step of setup finishes on your passing check.** Until now, after
"Check requirements again" passed every check, the waiting setup run noticed
only on its next 20-second pass. Now the owner's explicit check
(`GET /api/v1/setup?check=1&manual=1`) wakes the waiting run, and only when
every check passed while the latest run waits at its final verification
(`wakeServerSetupVerification` in `cmd/panel/server_setup_operations.go`,
called from the setup handler in `cmd/panel/server_setup.go`; one buffered wake
signal per panel). The handler neither completes setup nor starts a change;
the runner rechecks and completes in its own single pass. Page polls do not
send `manual=1`, and the unattended 20-second cadence is unchanged. Tests:
`TestServerSetupManualCheckWakesWaitingVerification` and
`TestServerSetupManualCheckWakesOnlyPassingVerification` (a negative control
with the flag renamed failed after 5.6 s).

**Finishing is shown as active and bounded.** After a passing check the heading
and the final step read "Finishing setup", with a spinner on the step and
beside the sentence "Every check passed. The panel is completing setup now,
and you do not need to do anything. This page updates by itself when it is
done." The page reads the operation every 1.5 s instead of 3 s
(`web/src/components/ServerSetup.tsx`, `setupFinishingLimit` of 60 000 ms). It
ends when the run leaves the final check, or when the run records a newer,
different unmet result, which is then shown. After 60 s without confirmation
the page says the result is not known yet (not a failure), to reload the page,
and that if it persists `sudo systemctl restart celikpanel-panel` resumes the
same setup at its final check without repeating finished steps; while that
notice stands the step reads "Not confirmed yet" (it read "Waiting for
requirements" until the screenshot round, below). "Edit plan", the one control
on that step that stops the run, is not offered while finishing. The passive
"These checks passed. Waiting for the server to confirm setup completion." is
removed. Tests: `web/tests/setup-finishing-mounted.test.mjs`, English and
Turkish: spinners, no "Check requirements again" or "Edit plan", 1.5 s reads,
completion with no click and only GET requests, the 60 s fallback, a newer
unmet result ending finishing, and the "Not confirmed yet" label.

**Waiting for reverse DNS is said as such.** A final check waiting only on a
reverse DNS mismatch reads "Waiting for reverse DNS" instead of "Waiting for
you"; a plan correction open beside it still reads "Waiting for you". The
guidance adds that a change made at the server provider can take from a few
minutes to several hours to reach public DNS, because resolvers keep the old
name until their saved copy expires, and that once the change is made nothing
more is needed in CelikPanel (`web/src/lib/serverSetupGuidance.ts`; tests in
`web/tests/setup-final-check-mounted.test.mjs`).

**The Agent asks the next resolver when the first reverse DNS answer is
stale.** In the Agent's mail identity lookup (`mailDNSIdentityAt` in
`cmd/agent/mail_health_dns.go`), a PTR answer that is not the expected name, or
no PTR, now also asks the next resolver. Any resolver that returns the expected
name with a matching forward A/AAAA record proves the identity. Otherwise the
first observation is reported as before, preferring a PTR that names the host
but whose forward record does not match yet. A matching first answer asks
nothing more. Test: `TestMailDNSIdentityAsksNextResolverWhenFirstAnswerIsStale`
(six cases). This takes effect only after the Agent is updated (see the
limits).

**Every waiting state was read, and 19 were changed.** The web interface was
read from source for every state in which a page waits for the server, a
background operation, a poll, a provider, the licence or an external check:
37 states across setup (the phases other than the final check), the
Components and component pages, the Dashboard, the licence and access layers,
the panel update flow, domain certificates and HSTS, DNS, imports, PHP,
backups and the sidebar build stamp. 18 already met the rule; 16 did not and
were changed; 3 did not, were decided by the owner on 2026-10-14 and were
changed after that. Every row was read from code with a file and line; none
was measured on an installed server. The audit is in
[OPERATION-GUIDANCE](OPERATION-GUIDANCE.md), entry "Waiting screens that look
idle" (2026-10-14). The changes, by screen:

- *Setup, reconnecting after a lost read, and the planned certificate
  restart* (`ServerSetup.tsx`, `2e273cc55`). A spinner beside the sentence,
  which now says that the page asks the server again every few seconds by
  itself and nothing needs to be done (it used to ask the owner to press
  "Reconnect to this operation"). After 60 s the spinner stops and the page
  says the result is not known (not a failure), that it keeps asking, and how
  to check the panel on the server (`sudo systemctl status celikpanel-panel`,
  then `sudo systemctl start celikpanel-panel`), with "Reload this page".
- *Setup, "Check requirements again" and "Use this connection and continue
  setup".* A spinner while the request runs.
- *The component operation lock after a lost connection* (row 16,
  `OperationOverlay.tsx`, `8e68b3ce2`). A spinner beside "Reconnecting…" and
  the sentence that the page asks again every few seconds by itself. After
  60 s without a usable answer the spinner stops and the lock says the result
  is not known yet, that this does not mean it failed, that the server keeps
  the operation's record, and how to check the panel on the server; "Reload
  page" stays. The reads go on every 3 s; a recorded failure keeps its own
  wording.
- *The panel update card while the server is busy* (row 25,
  `PanelUpdateCard.tsx`, `3abbe3f76`). It reads readiness again every 5 s:
  GET only, one read at a time, only while the tab is visible, without drawing
  its checking look; it stops when the tab is hidden or the card leaves the
  busy state. Nothing in these reads starts or admits an update. Retry stays.
- *A domain's DNS change spreading* (row 35, `DomainConnection.tsx`,
  `0ad26074d`). While the tab is visible the connection card checks again
  every 30 s with the same GET as "Check again", for at most 60 checks
  (30 minutes of a visible page). It says that a change at the provider can
  take from a few minutes to several hours, that the page checks by itself and
  nothing needs doing, and when it last checked (the answer's `checked_at`,
  else the moment the page received it; never compared with the browser's
  clock). Then it stops and says the change was still spreading at the last
  check, that this is not a failure, and to use "Check again" or come back
  later.
- *Server-change pauses* (the Components banner, the Dashboard firewall
  dialog; `5403eed88`). They add "This page checks again every few seconds and
  lifts this pause by itself; you do not need to reload it." The unverified
  state no longer asks for a reload. A component page's install dialog now
  reads readiness every 5 s while open (GET only), not once.
- *Buttons that ran a long request behind a disabled, still label*
  (`f643f838d`, `875e9c2bd`): domain certificate issue, activation retry and
  upload, panel certificate, zone publish, PHP version, import inspection and
  run, repository enable, version removal. Each shows its spinner for the
  length of the request.

**The Components firewall banner (O4 and O5 of the alpha.84 notes).** A coded
answer is said in the page's language: on the Turkish page the banner shows
the translation of `err.AGENT_UNREACHABLE`, not the server's English sentence.
The banner reads the firewall again with each scan the page receives, so it
no longer keeps "Status unknown" after the Agent is back and Rescan succeeded
(`FirewallBar` in `web/src/components/ServiceList.tsx`, `723d4894f`). After the
screenshot round the refused read stands on the attention surface with ink
text, not on the failure surface, and the unavailable control goes under the
text on a narrow card (`795f31674`). Tests:
`web/tests/remote-state-mounted-batch3.test.mjs`.

**The Dashboard's "Needs attention" box names the Agent (O6).** When one of its
reads was refused with `AGENT_UNREACHABLE`, the notice names the Agent and the
owner's commands (`err.AGENT_UNREACHABLE`) instead of saying that this does not
mean something is wrong (`web/src/components/Dashboard.tsx`, `6c2b9e3f3`).

**The Dashboard's Mail stacks card follows the page's own check.** It shows
checking (with a spinner), held by a change, could not check, or not checked
recently, from the same check the System services card runs. "Open Components
to rescan" and "…rescan before trusting mail status" are removed. This was an
open item of the alpha.84 notes (`Dashboard.tsx`, `11762f96f`; tests in
`web/tests/dashboard-mail-stack-card-contract.test.mjs`).

**Mail profile sentences are typed.** Names and descriptions come from the
catalogue by profile id. The Panel now types its blocked and warning sentences
(`blocked_code` with `blocked_vars`, and `warning_code`, beside the old text,
which stays for older pages), and the page says the code in its language
(`mailProfilesView` in `cmd/panel/mail_profiles.go`, `1c5454c53`;
`web/src/lib/mailProfileText.ts`). Three server sentences no longer say "run a
fresh scan" or "run another scan"; they say that the next component check
verifies the state. The last attempt's error keeps the server's own words
behind "The last attempt reported:". This also answers the untranslated mail
profile text on the Turkish Components page that the alpha.84 notes listed.
Tests: `TestMailProfileTextCodesAreListed` (every constant of the code type is
in the list), `TestMailProfileViewTypesItsSentences`, and
`web/tests/mail-profile-text.test.mjs`, which reads `mailProfileTextCodes` from
the Panel's source and requires an English and a Turkish sentence for every
code, with no scan wording.

**The HSTS retirement no longer compares two clocks.** The domain SSL answer
adds `settings.hsts_retire_remaining_seconds`, the time left on the server's
clock when the answer is written (`stampHSTSRetireRemaining` in
`cmd/panel/domain_ssl_handlers.go`, `f643f838d`). The page
(`DomainSSLSettings.tsx`) dates the end from the moment it received the answer
and no longer compares its own clock with the server's `hsts_retire_after`,
which is kept. An older Panel without the field keeps the old rule. This
closes the alpha.84 limit that the HSTS retirement in `DomainSSLSettings`
compared a server timestamp with the browser's clock. Tests:
`TestStampHSTSRetireRemaining`, `TestDomainSSLAnswerCarriesHSTSRetireRemaining`,
and mounted tests in `web/tests/remote-state-mounted.test.mjs` with the server's
clock behind and ahead and with an older Panel.

**The sidebar's build id stays in one piece.** "build" and the 12-character id
are one unit, so the line no longer breaks inside the id; the full id is in the
title and in the screen-reader text (`web/src/components/Layout.tsx`,
`942623e1d`; `web/tests/layout-server-identity-contract.test.mjs`, which fails
with the old class). The owner observed the break after the alpha.83 and
alpha.84 updates.

**Copy corrections from the screenshot round** (`89cf88397`). The Turkish HSTS
term "emeklilik" becomes "kaldırma süresi" in the three SSL keys; the
Dashboard's antivirus line and two services and firewall hints address the
owner formally ("siz"); the phone menu button's accessible label uses the
catalogue's `nav.menu`; the HSTS time left is a duration ("1 h 30 min",
"2 days 3 h") instead of a rounded relative time (`web/src/lib/hstsRemaining.ts`,
`web/tests/hsts-remaining.test.mjs`).

**What does not change.** No database schema change (the ledger stays 44; no
migration between the alpha.84 tag and this candidate) and no new route. The
Dashboard's System services check, the licence check and the Agent answer of
alpha.84 are unchanged. Server sentences outside the mail profiles that still
say "rescan" are unchanged (see the limits).

## API changes

No new route. One new query parameter, one new answer field and three new
profile fields, all additive.

### `GET /api/v1/setup?check=1&manual=1`

`manual=1` beside `check=1` marks the owner's explicit "Check requirements
again". The answer is the same setup snapshot as before. When every check
passed and the latest run waits at its final verification, the Panel also
wakes the waiting run, which then rechecks and completes in its own pass. The
request itself completes nothing and starts nothing. Without `manual=1`
nothing is woken. A client that does not send it sees no change.

### `hsts_retire_remaining_seconds`

On the domain SSL answer's `settings`, next to the unchanged
`hsts_retire_after`:

```json
{
  "settings": {
    "hsts_retire_after": "2026-10-15T01:30:00Z",
    "hsts_retire_remaining_seconds": 5399.512
  }
}
```

(The values are an example; the other fields did not change.)

The time left on the server's clock when the answer is written, to the
millisecond, never below zero (0 once the retirement has passed). Absent when
there is no retirement. A page takes the end as the moment it received the
answer plus this value; it does not compare `hsts_retire_after` with its own
clock. A page that finds the field absent uses `hsts_retire_after` as before.

### `blocked_code`, `blocked_vars`, `warning_code`

On each mail profile in the `profiles` list of the managed-services answers,
next to the unchanged `blocked_reason` and `warning`. The codes are
`state_unverified`, `state_incomplete`, `catalogue_incomplete`,
`hostname_unknown`, `dns_identity_not_ready`, `component_not_installable`,
`component_conflict` and `component_requires` (blocked), and
`repair_to_verify`, `webmail_unverified`, `roundcube_not_ready` and
`partly_present` (warning); `blocked_vars` carries the values a sentence names.
A page says the code in its own language and keeps the English text only for a
code it does not know. A client that does not know the fields ignores them.

## Behaviour changes you will notice

- The last step of setup completes within seconds of a passing "Check
  requirements again", shows "Finishing setup" with a spinner meanwhile, and
  does not offer "Edit plan" while finishing.
- A final check waiting only for reverse DNS reads "Waiting for reverse DNS",
  not "Waiting for you", and says the change can take hours to spread and
  nothing more is needed in CelikPanel.
- Every wait the audit found either moves or says nothing needs doing, and
  ends with an honest "not known yet" after its bound: setup and the component
  lock after 60 s, a spreading domain after 60 checks.
- The update card, a component's install dialog, the pauses and a spreading
  domain read the server again by themselves while visible (reads only).
- On the Turkish Components page with the Agent stopped, the firewall banner
  speaks Turkish and is read again after Rescan; the Dashboard's "Needs
  attention" names the Agent.
- The Mail stacks card no longer asks you to open Components to rescan.
- The HSTS time left is a duration, and it is right whatever the browser's
  clock says.
- The sidebar's build id no longer breaks in the middle.

## What was measured, and how

**Component tests.** The product code of this release is that of `89cf88397`.
The commits after it change evidence, documents, one lab tool
(`cold-load-set16.mjs`) and the release preparation; none touches
`cmd/panel/`, `cmd/agent/`, `internal/` or `web/src/`.

- Go, in the WSL guest `CelikPanel-S2-Debian`: `go test ./cmd/panel/...` 1542
  tests and 1665 subtests passed, 6 skipped, 0 failed, recorded at `1c5454c53`
  (no Go file changed after it); `go test ./cmd/agent/...` 1798 tests and 2822
  subtests passed, 29 skipped, 0 failed, recorded with the commits `a94f83cb0`
  to `e358ebe3c` (no file under `cmd/agent/` or `internal/` changed after the
  run). The Agent count was measured in a private mount namespace that showed
  the tests a clean host (empty directories over
  `/var/lib/celikpanel-agent-private` and `/run/celikpanel`, an `/etc/group`
  without `celikpanel`): that guest is itself a running CelikPanel host, on
  which the package fails the same 19 tests on this branch and on `e3c8f735a`.
  The guest was not changed. Both counts are those of
  [OPERATION-GUIDANCE](OPERATION-GUIDANCE.md), entries of 2026-10-14.
  `./internal/...` was not run on this branch; no file under `internal/`
  changed.
- Web: `npm test` 1391 of 1391, run again on 2026-10-15 on the release
  preparation commit; `npm run build` passes with its type check and the bundle
  budget (Dashboard chunk 38.28 KiB of 160.25 KiB). The web count was 1348
  after the setup change, 1375 after the audit's sixteen changes, 1386 after
  rows 16, 25 and 35, and 1391 after the screenshot round's corrections. Each
  mounted test of a changed state failed against the screen before its change,
  except as the guidance entry names (the repository and version-removal
  buttons have a source check only; two wording checks read the catalogues).
- The release preparation's three contract tests pass: the download-portal and
  signed-manifest contracts in the build guest, and the sequence-policy
  contract in the Windows checkout, because it reads the `v0.1.0-alpha.84` tag
  with git, which the guest lacks ("84 -> 85; historical bootstrap sequence
  84").

**The screenshot round (the twelfth browser-inspection batch).** A real Chrome
on Windows against the loopback mock, `web/tools/browser-inspect/
scenarios-batch12.mjs` with `mock-batch12.mjs` (`0b43ee81e`), drove every state
changed by the two 2026-10-14 guidance entries: 31 states in 8 configurations
(1280 and 390 wide, light and dark, English and Turkish), with no scenario
errors and no page scrolling sideways, as recorded in that commit. The 60 s
bounds and the 30 s cadence were reached with a test-side clock control, not a
product change. It found defects that were corrected before the native record:

- after the 60 s finishing bound the step read "Waiting for requirements",
  which reads as an unmet requirement while the result is unknown; it now
  reads "Not confirmed yet" (`8dfc3ff69`);
- on a 390 px phone the component lock's spinner stood apart from its
  sentence; it is now inline (`733b798db`);
- the refused firewall read stood on the failure surface in the failure colour,
  and the disabled control squeezed its sentence on a phone (`795f31674`);
- the Agent's commands broke at their hyphen ("celikpanel-" / "agent"), and a
  spreading domain's resolver names broke inside the host name; `KeepWords`
  (`web/src/components/ui.tsx`) keeps such tokens whole (`8dfc3ff69`,
  `733b798db`, `795f31674`);
- the four copy defects above (`89cf88397`).

The pictures of that round are not stored in the repository.

**The eighteenth native record**
([set16](../deploy/e2e/release-recovery/evidence/set16-20261014/README.md),
2026-10-14 22:16:46Z to 2026-10-15 00:01:17Z UTC). The pre-publication
measurement of the candidate commit `89cf88397`, built from a clone of the
committed head, never from the working tree. Test-licence builds, none a
release; QEMU/KVM guests with outbound NAT; a real headless Chrome on the host.
Every `result.json` carries `native_evidence: false`; the record closes no P0
row. The harness addressed no installed server, no licence service, no
certificate authority and no celikpanel.net.

- **What was installed.** The candidate archive is built from a fixture commit,
  `d5d673eb`, which is `89cf88397` plus `deploy/release-sequence-policy` only
  (sequence 85 after 84), SHA-256
  `3913d51374ba8db47fd4acce0af21d0c4150690664809d8932e355dbfdff40ad`. The
  baseline is a local test-licence build of the alpha.84 tag commit
  `f0157e6c`, `9f86eb4352ce685e064c51c68aab9b592f99612ff3dcca7f71983c9305708dd4`,
  not byte-identical to the published archive
  (`6dc899e5dabaf471978cde47bfde4a0a649b7c3f6941b1431284c84c9743c301`). The
  defective candidate is `524f86dd` (`d5d673eb` plus the migrate-only defect of
  an earlier record in `cmd/panel/main.go`),
  `77f39013cb3ef7fff68e3495794dd0034bcb4a396f7e8c2bd649db710119be3a`. Because of
  the fixture, every candidate page reads `build d5d673ebd95e`, and the
  bootstrap pins inside every candidate archive still read 84 and
  `v0.1.0-alpha.84` (the fixture changes the policy only); the installed copy on
  every guest read the same. There is no migration between the alpha.84 tag and
  the candidate; the ledger stays 44.
- **Fresh install, Debian 13 (English desktop, 1440×900), 22:24:54Z to
  22:44:41Z.** Install, owner sign-in and licence passed; setup purpose `web`
  (DNS external) waited at its public-DNS step, `access_dns`, and the owner
  chose to set up by hand. The version route answered `v0.1.0-alpha.85`,
  commit `d5d673eb…`, `agent_matches true`, `schema_version 44`; ledger
  `as-expected`; domains `[]`; `valid_for_seconds` and `scan_age_seconds`
  present.
- **The update from alpha.84, good candidate.** Debian 13 (22:45:01Z to
  23:00:07Z; armed 22:55:05Z, final 22:55:58Z) and Ubuntu 24.04 (23:16:39Z to
  23:38:07Z; armed 23:33:07Z, final 23:33:58Z), each from a baseline set up as
  `web_mail` with one static site, through the Panel's own update path. Both
  ended `succeeded / update_verified`; Agent and Panel at `v0.1.0-alpha.85 /
  d5d673eb…`; floor sequence 85; ledger 44; the HSTS readings before and after
  passed; Domains one row, `managed_unchanged`; the database comparison
  `equal-except-volatile`, `managed_site_files` volatile in `observed_at` only.
- **The defective candidate: the automatic return.** One guest reset during the
  update (Debian 23:11:06Z, SSH back after 8.26 s; Ubuntu 23:56:04Z, 8.77 s).
  Both recovered by themselves: `phase: recovered`, `reason: rollback_verified`,
  `previous_failure: update_failed` (Debian 23:12:07Z, Ubuntu 23:57:09Z), two
  automatic attempts each, no owner attempt. Agent and Panel then reported
  `v0.1.0-alpha.84 / f0157e6c…`; login, the site marker, SMTP and the firewall
  were equal. The terminal step passed this time (set15's had failed on
  `managed_site_files`), so the readings and the browser window after the
  return ran.
- **The ledger after the return.** On both distributions: verdict
  `equal-except-volatile`, nothing unexpected, one row before and after, only
  `observed_at` changed (Debian 1792019311 → 1792019522; Ubuntu 1792022021 →
  1792022224), the stable digest equal (Debian `a39f9b86…`, Ubuntu
  `985eed03…`) and the full digest different. 67 tables were compared and five
  left out as volatile. This answers set15's open finding for this fixture:
  only `observed_at` changed after the return. The probe that made this reading
  is the harness change `a87ce0b97` (below).
- **The Dashboard checks by itself, without clicks.** One conditional scan per
  load within 90 s in every lab: Debian fresh (stored scan 317.3 s old, answer
  in 864 ms), Debian after the update (413.5 s, 1025 ms), Ubuntu after the
  update (557.7 s, 2622 ms), Debian after the return (462.7 s, 1908 ms), Ubuntu
  after the return (641.4 s, 1492 ms); the System services card went to
  checking and then to the result. On the candidate the Mail stacks card went
  to "Checking…" with one spinner and then to "Ready"; "Open Components to
  rescan" was never seen. After the returns alpha.84's own old wording appeared
  until the scan answered, as expected of a return. On the fresh install the
  Mail stacks card is drawn only once the profiles show mail activity, so its
  checking state could not be seen there. The Components page right after the
  Dashboard sent no scan; with the scan stale (317.1 s) it sent one conditional
  scan, 200, 873 ms, with no alert and no toast.
- **The licence check with the guest clock moved** (fresh cell). With the guest
  +91.065 to +91.070 s and −88.937 to −88.932 s from the browser, the page
  mounted each time and read the access check three times, every answer
  `can_use_panel: true`; by calculation the old rule (`valid_until` against the
  browser's clock) would have refused 0 of 3 and 3 of 3. The clock was
  restored to about +1.05 s and NTP switched back on at the end.
- **The Turkish Components page with the Agent stopped.** `GET
  /api/v1/firewall` answered `503 AGENT_UNREACHABLE`. The banner read
  "Güvenlik duvarı: Durum bilinmiyor" with the Turkish Agent sentence; the
  English sentence was absent (O4 corrected). Its surface was the attention one
  (amber border and background, ink text), with no element in the failure
  colour; `celikpanel-agent` occurred three times, each on one line, at
  1440×900 and at 390×844. After the owner started the Agent, one press of
  "Yeniden tara" sent one scan (200, 850 ms) and one firewall read (200,
  16 ms), and the banner read "Güvenlik duvarı: Açık" with no alert (O5
  corrected).
- **The Dashboard with the Agent stopped.** "İlgi istiyor" in Turkish and
  "Needs attention" in English named the Agent with its sentence and three
  commands, on the attention surface, `celikpanel-agent` unbroken (O6
  corrected). In the English load the stored scan passed 300 s while the page
  was open; the page sent one conditional scan, `503` in 3 ms; System services
  went from checking to "Could not check" with the Agent sentence, and Mail
  stacks from "Checking…" to "Unknown: The components could not be checked just
  now, so the mail status is not known. The System services card above says
  why." The mail card's "held by a change" state was not produced.
- **The sidebar's build id.** On the candidate, `build d5d673ebd95e` stood on
  one line, the id one rectangle, with no overflow, at 1280×800 and at 390×844
  with the menu opened. After the returns alpha.84's own stamp broke at
  1280×800, as alpha.84 does; at 390×844 after the returns the stamp was found
  with empty boxes, so nothing is claimed there.
- **Not reachable or not measured.** "Finishing setup": the fresh setup waits
  at its public-DNS step, and the final step needs a reverse DNS name that no
  lab guest has; the record does not imitate it. The update card's busy state:
  the browser tool has no Settings phase and no step holds the server busy
  while the card is open. Arch; a fresh Ubuntu install of the candidate (not
  planned).
- **Observations, pre-existing and not judged** (the files behind them are
  unchanged since alpha.84). O1: Turkish pages carry `<html lang="en">`
  (`web/index.html:2`; the app sets the language only on the offline page).
  O2: "Firewall engine" and its description stay English on the Turkish
  Components page; they come from the server catalogue
  (`internal/core/managed_services.go:915-916`). O3: the Dashboard's "Recent
  activity" shows raw audit action ids such as `server.setup.guidance`, because
  `auditActionText` (`web/src/components/Dashboard.tsx:120`) maps only login
  and mail-profile events.
- **Open items of the record.** After each return the recovery foundation file
  still names the defective candidate (`524f86dd…`, sequence 85) while the
  running build is alpha.84, and the floor reads 85: the same pattern as set12
  and set15, not judged. The fresh cell's `result.json` carries a fixed harness
  sentence about `web_mail` on a "published v0.1.0-alpha.85 baseline" that is
  not a measurement of this run (it installed purpose `web`). The PHP-site and
  M10 mail sections of set4/set5 were switched off.

Cells 1 and 2 took 51 min 18 s together. The record's README was corrected at
intake against its raw files (`2957b5196`, `f24d33a51`); the figures here are
those of the README as it stands, and the README wins over this summary. No
defect of the candidate was observed in these five labs. The run changed no
product file.

**Harness and lab-tool changes on this branch.**

- `a87ce0b97`: the release-recovery probe (`guest_probe.py`,
  `owner_update_trial.py`) records the `managed_site_files` rows, the full
  digest and a stable digest without `observed_at`, and lists a table that
  differs only in `observed_at` as volatile rather than unexpected, with
  offline tests (`test_guest_probe.py`, `test_owner_update_trial.py`). set16 is
  its first guest run.
- `c2144b785`: the lab mock answers the licence access check with
  `valid_for_seconds`, as the Panel does, and has a `clockSkew` control; the
  cold-load tools for sets 9 and 11 plan their timed click on the page's
  deadline. This closes the alpha.84 limit that the lab tools sent only
  `valid_until`. The mock was measured by hand on a local port (59.2 s left for
  an active licence; with `clockSkew` −420 the stamp moved 420 s and the time
  left stayed 59.1 s; a missing licence answered 0); the cold-load tools were
  not run.
- `0b43ee81e`: the twelfth browser-inspection batch (above) and a 1280×800
  viewport in `run.mjs`.
- `a945a4dbf`, `e3feee06d`: the set16 harness (`build-set16-artifacts.sh`,
  `run-set16.sh`, `set16_fixture.py`, `set16_trial.py` with 11 offline tests,
  `cold-load-set16.mjs`) and the record itself.

**A correction of the alpha.84 records** (`8b09e4e61`). The alpha.84 release
notes, ROADMAP and resilience contract said that the job that publishes
release assets depends only on the packaging job, not on the test jobs. That
was wrong: `publish-release` needs `package`, which needs every test and build
job of the same run, so a tag's assets are published only after that run has
passed. A dated correction was added in English and Turkish; the process note
about the order between the run on `main` and the tag stands.

## Limits of this release

These are known. They are not hidden defects. The first group is new with this
release; the second is the alpha.84 list, condensed.

New with this release:

- **No installed server was measured.** Nothing in this release was seen on an
  installed server; the audit's rows were read from code, and the native record
  ran on disposable guests.
- **"Finishing setup" was not reached natively.** The state the owner met on
  the second installed server needs a reverse DNS name that no lab guest has.
  It has mounted tests and the mock browser only. When the runner's own
  recheck after a passing check finds exactly the same unmet result as before,
  the page cannot tell that from no recheck yet; it then ends at the 60 s "not
  known yet" notice rather than at once.
- **The update card's busy state was not measured natively.** It has mounted
  tests and the mock browser only. The same holds for the component lock's
  reconnect, the spreading-domain checks, the install dialog's re-read, the
  pauses, the certificate, zone, PHP and import spinners and the HSTS time
  left: none of them was produced in the native record.
- **The resolver that answered is not named.** The Agent does not report which
  resolver gave the reverse DNS answer, so the page cannot say it.
- **The Agent change needs an Agent update.** Asking the next resolver takes
  effect only after the Agent is updated. Until then a stale first resolver can
  still read as a mismatch, although the wording now covers that wait.
- **The candidate was measured as a fixture commit.** set16 ran `d5d673eb`
  (`89cf88397` plus the sequence policy), so its pages read
  `build d5d673ebd95e` and its archives carried bootstrap pins 84. The release
  preparation commit 5443fe055 (pins 85, version lines, contract tests) and the
  documents after it are in no measured build; their product code is that of
  `89cf88397`.
- **The alpha.84 baseline was a local build, not byte-identical to the
  published archive.** The update was measured from a test-licence rebuild of
  the alpha.84 tag (`9f86eb43…`), not from `6dc899e5…`; the ordinary archive of
  that commit was not built, so it was not compared.
- **set16 is a measurement, not acceptance.** QEMU/KVM guests, test-licence
  builds, one run per cell on one laptop host, outbound NAT not captured; every
  `result.json` carries `native_evidence: false`.
- **The recovery foundation file after a return** still names the defective
  candidate while the running build is the previous release, and the floor
  reads the candidate's sequence (set12, set15 and set16). Recorded, not
  judged; open.
- **Observations O1 to O3 of set16** (the page's language attribute, the
  English firewall engine name on the Turkish Components page, raw audit action
  ids on the Dashboard). Open.
- **Templated text.** `component_not_installable` does not carry the platform
  detail, which only the old text has, and a requirement that names a role
  rather than a service is shown as the role's token. Server sentences outside
  the mail profiles still say "rescan" (the cached-state refusal in
  `managed_service_handlers.go`, the offering states in `store_backend.go`).
- **Not run:** Arch; a fresh Ubuntu install of the candidate; the Mail stacks
  card's "held by a change" state; the cold-load tools for sets 9 and 11 after
  their change.

Closed by this release from the alpha.84 list: O4, O5 and O6; the Mail stacks
card's stale copy; the untranslated mail profile text on the Turkish Components
page; the sidebar build id wrapping mid-word; the HSTS retirement comparing two
clocks; the lab tools sending only `valid_until`; and the `managed_site_files`
finding after an automatic return, answered for set16's fixture (only
`observed_at` changed).

Limits of v0.1.0-alpha.84 that this release does not address (each still
open), condensed; the full list is in the
[alpha.84 notes](RELEASE-NOTES-v0.1.0-alpha.84.md):

- `server_setup_busy`, the never-checked state, the scan's server-side wall
  time and the "may have completed" banner after an action were not measured;
  the held state is reachable only by a change that does not scan first.
- "Nothing was changed" in the Agent answer assumes no earlier change in the
  same request; no handler-by-handler audit is claimed.
- The guest's NTP settings in the sixteenth record; the clock cases of the
  sixteenth record were measured on Debian 13 only.
- Screens that still read the server the old way (the count was not recomputed
  for this draft); the access wait; the update notice; setup's mail identity
  reason never read from a real Agent; and the limits of v0.1.0-alpha.83 and
  v0.1.0-alpha.82 that the alpha.84 notes list as still open.
- **Open acceptance items.** All five foundation items of the
  [resilience contract](RESILIENCE-CONTRACT.md) remain partial; this release
  closes none and advances none. P0.1–P0.5 stay **partial**. Decisions: D-022,
  D-024, D-025, D-032 and D-033 in [DECISIONS](DECISIONS.md).

## Publication record

Not published. Nothing is recorded here until the owner publishes. Open before
publication:

1. The pre-publication measurement: done (set16, the eighteenth record,
   2026-10-14 UTC), on the candidate's product code through a fixture commit:
   a fresh install on Debian 13 and the update from alpha.84 on Debian 13 and
   Ubuntu 24.04, good and defective. Its limits are above.
2. The release preparation: done in commit 5443fe055, which changes no file
   under `cmd/`, `internal/` or `web/`: the release sequence policy 85 after 84
   with the alpha.84 release commit `f0157e6c8` as previous, the
   `download-portal/get.sh` pins (sequence 85 and version v0.1.0-alpha.85; the
   public-key pin unchanged), the README version lines and three contract tests,
   which pass. Still open: the repository variable
   `CELIKPANEL_RELEASE_SEQUENCE=85`, which the owner sets before the tag. The
   real archive is built and signed by CI on the tag.
3. The pull request's last head gets its own CI run.
4. The owner merges, tags the release, checks the six signed assets as the
   signed release contract describes, and publishes the download portal.
5. After the release is published, the owner of each installed server updates
   that server from the panel's own update screen. This is a separate step from
   every step above; no one else starts it.

Install this release only through CelikPanel's update interface. Publishing it
does not update installed servers; only the owner of a server starts its
update, from the panel's own update screen.
