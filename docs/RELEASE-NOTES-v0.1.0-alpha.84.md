# v0.1.0-alpha.84 (draft, not published)

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.84.tr.md)

This is the draft of the release that follows v0.1.0-alpha.83. The number
v0.1.0-alpha.84 is the working name of the branch; the owner decides the
number at publication. Nothing of it is published, and no installed server runs
it. This draft was written on 2026-10-13.

It does three things, all on the Dashboard's "System services" card and the
answers behind it. The card now checks the components by itself, instead of
saying "Unknown" whenever the stored scan is more than five minutes old and
waiting for you to press a button on another page ([D-032](DECISIONS.md)). The
server now states how old a scan is, so the browser never compares its own
clock with the server's. And a Panel that cannot reach the CelikPanel Agent on
its own server says so, with the next step, instead of "an internal error".

What the card does rests on component tests, on a browser against a mock of the
API, and on one lab measurement (the fourteenth native record, on Debian 13 and
Ubuntu 24.04, a real headless Chrome against a real Panel), which measured the
first of the three product commits only. The two corrections made after that
measurement (the clock rule and the server-stated age) and the typed Agent
answer have component tests only. A short lab round that measures them on one
platform is running; it is not entered here (see "What was measured, and how").
The limits say what was never run.

## What changes for the server owner

**After the update, reload every CelikPanel page that was open.** A page loaded
before the update still has the old card.

**The Dashboard looks by itself.** Until now the "System services" card said
"Unknown. Component status is outdated. Open Components to rescan." whenever
the stored scan was older than five minutes, and made you press "Rescan" on
the Components page. Now the Dashboard runs the same cached, conditional scan
that the Components page runs (one scan at a time on the server, cached,
refused while a change holds the server's lock), under rules that keep its
cost small:

- It checks after a successful read when the stored scan is missing or older
  than 300 seconds, and at most once per five-minute window, whatever the
  answer was.
- It checks only while the tab is visible. A hidden tab runs nothing; a tab
  shown again checks once if a check is due.
- It never starts a check while another is in flight, and there is no timer
  that scans at an interval regardless. There is no retry loop after a
  refusal.
- If a change is in progress on the server (a refusal with the code
  `server_setup_busy` or `service_operation_busy`), the card says so and sends
  nothing more. One follow-up check runs when the page's existing readiness
  poll sees the server go from busy to ready (package operations only; after a
  setup refusal the next check waits for the window).
- Your own press of "Scan services" stays unconditional: it always scans.

The card shows four distinct states: "Checking components…", the result,
"Could not check: <reason>" with the in-place "Scan services" button, and
"A change is in progress; the status is shown when it finishes." The "Needs
attention" sentence follows them. "Not checked in the last 5 minutes. This page
checks again by itself while it is open." is now a short state between a result
that has aged and the next check, not a state that waits for you.

What it costs: one check is one conditional scan. Measured on two lab guests,
one check cost the Agent about 0.2 to 0.4 seconds of CPU time (an estimate from
two 47-second windows; see "What was measured, and how"). At most one check per
five minutes for an open, visible Dashboard; none for a hidden tab.

**The scan's age is the server's statement.** The three managed-services
answers carry `scan_age_seconds`, the scan's age on the server's clock when the
answer is written. The Dashboard and the Components page date the scan as the
moment they received the answer minus that age, so neither compares the
server's clock with the browser's. This closes a defect the lab found in the
first version (below). Without the field (an older server) the page falls back
to the rule of the previous step.

**An unreachable Agent is named.** When the Panel cannot open the Agent's local
socket (nothing is listening, or the socket file is missing), it answers `503`
with the code `AGENT_UNREACHABLE` and says: the reason, that nothing was
changed, that the server owner checks the Agent on the server with
`sudo systemctl status celikpanel-agent` (log: `sudo journalctl -u
celikpanel-agent`) and, if it is stopped, starts it with `sudo systemctl start
celikpanel-agent`, and that you try again once it runs. This replaces "The
server hit an internal error" on the card and on every screen that shows the
answer of a refused admission. Only a failed dial counts. A connection that
broke after a request was sent is an unknown result and keeps the generic
answer.

**What does not change.** The mail-stack card on the Dashboard still has its
own copy of the old stale text and its "Open Components" button (open; see the
limits). The Components page behaves as before. No database schema change and
no new route.

### The defect the lab found, and the two steps that fixed it

The fourteenth record measured the first version of the card. With the guest's
clock one to two seconds ahead of the browser's (1.05 to 2.16 s measured), the
card showed "Not checked in the last 5 minutes" for 20 to 29 seconds after
every check, because the page rejected a scan stamp later than its own clock
and only its 30-second tick moved past it (defect D1 of that record).

Step one: the page records when it received each answer that carries a scan
stamp, and takes the earlier of the stamp and that moment as the scan's time.
Step two (above): the server states the age. This also removes the limit step
one left, that a server clock more than five minutes behind the browser's still
read as stale.

## API changes

No new route. One new field and one new error code.

### `scan_age_seconds`

On the three answers that carry the stored scan: `GET
/api/v1/managed-services`, the cached answer of `POST
/api/v1/managed-services/scan?max_age_seconds=N`, and that route's fresh scan.

```json
{
  "scanned_at": "2026-10-12T21:41:48Z",
  "scan_age_seconds": 46.7,
  "services": [ ... ],
  "profiles": [ ... ],
  "dns_identity_ready": true,
  "mail_hostname": { ... }
}
```

(The values are an example; `...` stands for the fields that did not change.)

`scan_age_seconds` is the scan's age on the server's clock when the answer is
written, to the millisecond. It is `null` when no scan has run (`scanned_at` is
`null` too). The stored stamp keeps whole seconds, so the age can read up to a
second older than the scan. A negative value means the stamp is later than the
server's clock (the clock moved back); like the conditional scan, a page does
not take it as fresh. A client that does not know the field ignores it. A page
that finds the field malformed does not trust it.

### `503 AGENT_UNREACHABLE`

```json
{
  "error": "The CelikPanel Agent on this server is not reachable, so this request was not carried out and nothing was changed. The server owner checks the Agent on the server with sudo systemctl status celikpanel-agent (its log: sudo journalctl -u celikpanel-agent) and, if it is stopped, starts it with sudo systemctl start celikpanel-agent. Once the Agent is running, try again.",
  "code": "AGENT_UNREACHABLE"
}
```

The screens show their own translation of the sentence (the Turkish one is in
the Turkish notes). Where it applies:

- The scan handler (`POST /api/v1/managed-services/scan`), when the Panel
  cannot open the Agent's socket.
- The shared admission check that eleven mutation handlers run before they
  change anything. A refused admission now says this; admission runs before any
  change, so nothing was changed.

The answer carries no socket path; the cause is written to the Panel's journal.
Only a failed dial of the Agent's Unix socket counts (nothing listening, or no
socket file). A connection that broke after a request was sent keeps the
generic path and is an unknown result. An ordinary scan failure still answers
`500`.

## Behaviour changes you will notice

- The Dashboard's "System services" card no longer waits for you. A stored
  scan older than five minutes is checked once when you look, then in the next
  five-minute window while the tab stays visible.
- Right after a check, the card shows the result at once. It no longer drops
  to "Not checked in the last 5 minutes" for up to 30 seconds when the server's
  clock is ahead of the browser's.
- A server whose clock is more than five minutes behind the browser's no
  longer makes a fresh scan read as stale.
- With the Agent stopped, a refused scan or change says the Agent is not
  reachable and what to run, in English and Turkish, instead of an internal
  error.
- The card's strings are in English and Turkish;
  `dashboard.systemServicesScanStale` is removed.

## What was measured, and how

**Component tests.** On the last product commit: `gofmt`, `go vet` and the
arm64 build clean; `go test ./cmd/panel/... ./internal/...` 7025 passed, 44
skipped, 0 failed (7 new: the field on each answer, the helper accepting only a
failed dial, `503` at admission with no socket text in the body, `503` when
the Agent drops after admission, an ordinary scan failure still `500`);
`npm test` 1336 of 1336 (8 new in that commit, among them: an age of 10 s with
a stamp seven minutes behind shows the result at once and sends nothing; an
age of 400 s sends exactly one conditional scan; a malformed age is not
trusted; the `503` shows "could not check" with the reason and is not
repeated); `npm run build` within budget (Dashboard chunk 37.67 KiB of
160.25). The web test count was 1324 after the first commit and 1328 after the
clock correction.

**Mock browser.** A headless Chrome against a loopback mock of the API drew the
four states in English on a desktop and in Turkish on a phone, with exactly one
`POST …/scan?max_age_seconds=300` per load (first commit).

**The fourteenth native record**
([set14](../deploy/e2e/release-recovery/evidence/set14-20261012/README.md),
2026-10-12 UTC). A test-licence build of the first product commit on two
disposable guests, with a real headless Chrome 154 on the host: Debian 13 with
an English desktop window (1440 by 900), Ubuntu 24.04 with a Turkish phone
window (390 by 844). The guests' clocks ran 1.04 to 1.62 s (Debian) and 1.23 to
2.15 s (Ubuntu) ahead of the host's. Every request was recorded with its status
and time, and the card's text every 100 ms with the page's visibility. Results:

- A stale load (stored scan 317.4 s and 319.2 s old) sent exactly one
  conditional scan (`max_age_seconds=300`, 200, 27 services; 1.34 s on Debian,
  4.45 s on Ubuntu, through the lab's SSH forward). A fresh load (stored scan
  about 45 s old) sent none.
- The Agent's CPU for one scan, as the difference between a stale and a fresh
  load of about 47 s each: about 189 ms (Debian) and about 417 ms (Ubuntu),
  including its child processes. An estimate, not a direct measurement of one
  scan.
- A page kept visible for 282 s and 284 s sent exactly one more conditional
  scan, at the page's first 30-second tick after the stored scan was 300 s old
  (317.4 s on Debian; about 314 s on Ubuntu): not before 300 s.
- A hidden tab (the browser's own hidden state, 303.9 s and 301.8 s) sent no
  scan while hidden, and one scan 13 ms and 34 ms after the page became
  visible, the stored scan then being about 61 s and 64 s past due.
- A load while a slow nginx reload held the server's lock (the owner's reload
  from the Panel, stretched to about 90 s by a lab setting) got `409
  service_operation_busy` in 12 ms and 8 ms, showed "a change is in progress"
  for 89.9 s, and sent nothing more until the readiness poll followed the
  change; then one cached follow-up (200, 4 ms and 45 ms).
- With the Agent stopped, one load sent one scan and got `500 INTERNAL` (the
  answer that `503 AGENT_UNREACHABLE` now replaces); the card showed "Could not
  check" with the button for 57.1 s and 56.0 s; after the Agent was started
  again, one press sent one unconditional scan (200, 1.34 s and 2.34 s) and the
  card showed the result.
- The Components page still sent its one conditional scan per load.
- Every card state was pictured in English (Debian) and Turkish (Ubuntu).
- The defect above: after each of the four checks per guest the card showed
  "Not checked in the last 5 minutes" for 28.6, 28.3, 25.9 and 1.5 s (Debian)
  and 25.5, 26.9, 20.0 and 1.5 s (Ubuntu).

The record states its own limits: the readiness answers' bodies were not
recorded (status codes only), so "busy, then ready" is read from the code and
from the follow-up's timing; two Turkish phone pictures are cut off at the
card's top. Nothing of the product was changed by the run, and every
`result.json` carries `native_evidence: false`.

**Pending, not measured: the fifteenth record (set14b).** One platform, a short
run that re-measures defect D1 and observation O1 on the code after the two
corrections: whether the card still drops to "Not checked in the last 5
minutes" after a check when the guest's clock is ahead of the browser's, and
what the card and the answer say with the Agent stopped (`503
AGENT_UNREACHABLE`). **It is running and has not been measured when this was
written. Its numbers are entered here after its intake. Until then every
statement in these notes about the two corrections and about the typed Agent
answer rests on component tests only.** [numbers to be entered]

## Limits of this release

These are known. They are not hidden defects. The first group is new with this
release; the second is the list of v0.1.0-alpha.83's limits, copied, each
marked "still open" because nothing in this release addresses it.

New with this release:

- **Arch was not measured.** The fourteenth record ran Debian 13 and Ubuntu
  24.04. The Debian lab's idle Arch peer was not measured.
- **`server_setup_busy` was not measured.** The setup ran before the browser
  window opened, so no Dashboard load happened during it. Only
  `service_operation_busy` was seen.
- **The never-checked state was not measured.** The setup had already scanned,
  so the stored scan was never absent in the lab.
- **The scan's server-side wall time was not measured.** Only the browser's
  request time through the lab's forward; cache answers took 4 to 45 ms and the
  refused answers 8 to 14 ms through the same forward.
- **The held state is reachable only in some cases.** A service install scans
  in its preflight, so the stored scan is fresh while the install holds the lock
  and the Dashboard asks nothing. The held state shows for a change that does
  not scan first (a service reload was used) or one that lasts past the window.
- **The mail-stack card on the Dashboard still has its own stale copy** of the
  old text and its "Open Components" button. It does not check by itself.
- **The sidebar's build identifier wraps in the middle of a word.** Observed by
  the owner after the update to v0.1.0-alpha.83; not fixed.
- **The two corrections and the typed Agent answer are not measured on a real
  system** in the current state of these notes (see the pending fifteenth
  record). Neither a server clock several minutes behind nor the `503` answer
  was seen in a browser against a real Panel.
- **No update to this candidate was measured.** The fourteenth record's guests
  ran a disposable fixture build, not an update from v0.1.0-alpha.83. The
  release preparation (sequence number, bootstrap pins, version lines) is not
  done and is in no measured build.
- **Evidence scope.** One laptop host; a test-licence build, not the signed
  archive; guests with outbound NAT whose traffic was not captured; each cell
  ran once; English was seen on a desktop window on Debian and Turkish on a
  phone window on Ubuntu, not both languages on both.
- **Other cards when the Agent is down.** With the Agent stopped the
  Dashboard's CPU, memory and disk cards showed "—" with the amber banner, and
  the firewall read answered `500`. This was seen in the intake of the
  fourteenth record, not measured further, and is not changed here.

Limits of v0.1.0-alpha.83 that this release does not address (each still
open):

- **The final code was not measured on a real system as a whole.** (Still open.
  The tenth to thirteenth records measured alpha.83 in four stages; the last
  changes of alpha.83 had component tests only.)
- **Arch** was not in the thirteenth record: the check before a reload, the
  unread row and the unit's `PATH` were measured on Debian 13 and Ubuntu 24.04
  only. (Still open.)
- **The text when the screen texts are not loaded.** If the browser has not
  loaded the screen catalogue, the server's own fallback sentence for a stopped
  service still says that the service keeps running. (Still open.)
- **The updates were measured from rebuilds, not from the published
  archives.** v0.1.0-alpha.81 and alpha.82 were rebuilt from their tags' source
  as test-licence builds; their digests differ from the published archives'. An
  installed server's own archive and data were not used. (Still open.)
- **No site with a certificate was measured, and no certificate was issued.**
  The success answer's fields for a validation check that did not run were
  never seen on a real system. (Still open.)
- **The validation check** reads only the `listen` lines of the kept file (an
  `include` is not followed); a name on an address that is not this server's
  cannot be checked from here; only the SSL tab's issuance answers a refused
  validation with a typed reason; a failed `nginx -t` or reload at the page's
  read is reported as `challenge_failed` although the cause may be another
  file; a missing or unreadable file records no reason; a ready check does not
  show that DNS or a firewall lets the certificate authority in; that a waiting
  certificate's dashboard entry goes away when the reason ends was not
  measured; a first issuance is never retried and a renewal only by the
  schedule; the pages were not drawn in a real hosting customer's session.
  (Still open.)
- **A return to an older release loses the protection** of the site-file
  header. Measured for files without the first line; for files with it, read
  from the older release's source. (Still open.)
- **The first start renders hosted sites.** At its first start, alpha.83 writes
  the nginx file of every hosted site it recognises as an earlier release's own
  text; measured on rebuilt baselines, never on an installed server's files.
  (Still open.)
- **Deleting a site.** The removal of CelikPanel's own include directory has
  component tests only. (Still open.)
- **The pool and the unit.** A PHP version switch was never measured; "Keep
  mine", "Recreate" and "Write CelikPanel's text now" on a pool or a unit have
  component tests only; pools and units are not looked at when the Panel
  starts; a pool or unit nobody opened is not on the Domains list; the calls
  are bound to the Panel's build with no fallback for an Agent of another
  build. (Still open.)
- **nginx judges every site together.** An owner's broken file elsewhere still
  fails every write of a site file (the written files are put back). (Still
  open.)
- **Not covered:** the Panel's own nginx site for its certificate, and the nginx
  files of webmail and the database tools. (Still open.)
- **Screens not yet converted.** 30 source files of the interface (88 places)
  still read the server the old way in at least one place, among them the
  application shell, the sign-in page, the setup wizard, the update card and
  overlays, the Services list and pages, the sidebar, parts of the dashboard,
  add-ons, the audit log, VPN, team members and the licence page. On these a
  failed read can still look like an empty or negative state. (Still open; the
  count was not recomputed for this draft.)
- **The access wait.** Each page load still reads the session twice; a load
  with the HTTP cache enabled was not measured; the offline page was seen only
  in lab contexts with the recovery worker; the locked file's badge and the
  Configuration file page were not looked at in a real browser. (Still open.)
- **The update notice.** The card's "Current version" line stays on the
  previous version until the page reloads itself; on a phone the corner notice
  covered the card's status line in the mock browser; the verifying window was
  observed in the lab, not on an installed server. (Still open.)
- **Setup.** The mail identity reason was never read from a real Agent; a
  failed forward lookup after a matching reverse DNS name is reported as the
  same unknown; an Agent of an earlier release still reports an empty reverse
  DNS name as a mismatch. (Still open.)
- **Limits of v0.1.0-alpha.82 that still stand.** Twelve routes require a
  request identity; the rest (service and application restart, plans,
  enrollment codes, about 38 other routes, DNS records, hosting type, PHP
  settings, aliases, application install, mail authentication, accounts and
  files) do not; a mail renewal helper already installed is not replaced by a
  panel update; the mail certificate path behind a `postconf` warning was not
  measured on a server; the archive carries the vendor's publishing tools and
  two one-time recovery scripts, whose removal is an owner decision still
  open; a Postfix restriction list written one restriction per line becomes
  one line after a DNSBL save; the Panel has no control that sets a database
  user's password; and the other limits of
  [v0.1.0-alpha.82](RELEASE-NOTES-v0.1.0-alpha.82.md) that alpha.83 did not
  address. (Still open.)
- **Open acceptance items.** All five foundation items of the
  [resilience contract](RESILIENCE-CONTRACT.md) remain partial; this release
  closes none. No P0 item is closed or advanced by this candidate. Decisions:
  D-021 (unchanged), D-022, D-024, D-025, D-029, D-031, D-032 in
  [DECISIONS](DECISIONS.md).

## Publication record

Not published. Open before publication:

1. The fifteenth record (set14b): one platform, the re-measurement of defect
   D1 and observation O1 on the final product code. Running; its numbers are
   entered under "What was measured, and how" once its intake is done.
2. The release preparation, committed separately and changing no file under
   `cmd/`, `internal/` or `web/`: the release sequence number after the
   previous one, the installer's bootstrap pins, the README version lines and
   the contract tests that pin them. Not done. At publication the owner sets
   the repository variable `CELIKPANEL_RELEASE_SEQUENCE` before the tag. The
   real archive is built and signed by CI on the tag.
3. The number: the owner decides it. v0.1.0-alpha.84 is the branch's working
   name.
4. The pull request's last head gets its own CI run.
5. The owner merges, tags the release, checks the six signed assets as the
   signed release contract describes, and publishes the download portal.
6. After the release is published, the owner of each installed server updates
   that server from the panel's own update screen. This is a separate step from
   every step above; no one else starts it.

Install this release only through CelikPanel's update interface. Publishing it
does not update installed servers; only the owner of a server starts its
update, from the panel's own update screen.
