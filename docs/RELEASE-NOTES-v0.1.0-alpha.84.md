# v0.1.0-alpha.84 (draft, not published)

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.84.tr.md)

This is the draft of the release that follows v0.1.0-alpha.83. The owner decided
on 2026-10-13 that the number is v0.1.0-alpha.84 (commit 1636b73f0). Nothing of
it is published, and no installed server runs it. This draft was written on
2026-10-13.

It does three things on the Dashboard's "System services" card and the answers
behind it, and two beside it. The card now checks the components by itself,
instead of saying "Unknown" whenever the stored scan is more than five minutes
old and waiting for you to press a button on another page
([D-032](DECISIONS.md)). The server now states how old a scan is, so the browser
never compares its own clock with the server's. And a Panel that cannot reach
the CelikPanel Agent on its own server says so, with the next step, instead of
"an internal error".

Found by the lab on the way, and also corrected: a server whose clock was more
than about a minute behind the browser's kept every Panel page from loading,
because the licence check compared the two clocks. The server now states the
time left itself. The second thing beside the card is the Components page's
banner for a failed read, which now gives the reason.

What the card does rests on component tests, on a browser against a mock of the
API, and on three lab measurements, each a real headless Chrome against a real
Panel: the fourteenth native record (Debian 13 and Ubuntu 24.04), which
measured the first of the product commits only; the fifteenth (set14b, Debian
13 only, 2026-10-13), which measured the two corrections that followed (the
clock rule and the server-stated age) and the typed Agent answer at the scan
and at the admission check; and the sixteenth (set14c, Debian 13 only,
2026-10-13), which measured, on the last product commit, the licence check's
correction, the typed Agent answer on the firewall read, the system check and
a restart request, and the Components page's read banner. The licence-check
fault was found in the lab by the fifteenth record and is gone in the
sixteenth (see "What was measured, and how"). The limits say what was never
run.

After those, the seventeenth record (set15, 2026-10-13) was the
pre-publication measurement: the candidate commit itself, installed fresh on
Debian 13 and Ubuntu 24.04, updated from v0.1.0-alpha.83 on Debian 13, and a
defective candidate that returned by itself. It left two open items (see the
limits).

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
  than 300 seconds, and at most once per five-minute window, whatever the answer
  was. The one exception: after a refusal because a change was in progress, one
  follow-up check runs when the readiness poll sees the host ready, which can
  add one check inside a window (`heldSawBusy` resets the window).
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
five minutes for an open, visible Dashboard, except that after a refusal because
a change was in progress one follow-up check runs when the readiness poll sees
the host ready, which can add one check inside a window (`heldSawBusy` resets
the window); none for a hidden tab.

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
answer of a refused admission. The answer is now written once, in the Panel's
shared server-error writer, so it applies to every handler that reaches that
writer with a failed dial of the Agent's socket as the only cause: besides the
scan and the admission check, the firewall read and the system check, which
still answered a generic internal error with the Agent stopped. A joined error
(a failure together with its compensation, for instance) keeps the generic
answer, because an earlier part of the request may have changed something and
"nothing was changed" would then be false. Left on their own typed paths, not
changed: the DNSSEC status, the update status and start, domain deletion and
mail password rotation. Only a failed dial counts. A connection that broke
after a request was sent is an unknown result and keeps the generic answer.

**A server clock that is behind no longer keeps the Panel from loading.** The
Panel's access check (`GET /api/v1/license/access`) gave the end of access as a
time on the server's clock, and the page refused a positive answer whose end
was not after the browser's own clock. A server clock more than about a minute
behind the browser's therefore made every answer look expired: no Panel page
loaded, and the page read the check again every five seconds (found in the
lab; see "What was measured, and how"). Now the server states the time left
itself (`valid_for_seconds`), and the page takes the end as the moment it sent
the request plus that time. It never compares the two clocks: a server clock
behind or ahead of the browser's changes nothing, and the end can never come
out later than the server allows. The absolute time (`valid_until`) is still
sent. Without the field (an older server) the old rule applies. The quiet
read, the recheck and the 15 s / 30 s stages are unchanged.

**The Components page no longer says an action may have completed when none
ran.** With the Agent stopped, the page's automatic check failed and the page
showed "Service state needs verification" and "A server action may have
completed", although it had sent no action. The page now keeps why the state is
unverified, and shows one of two banners:

- A component action was sent from this page and its result could not be
  confirmed: the banner is the old one. Title: "Service state needs
  verification". Hint: "A server action may have completed, but the latest
  state could not be verified. Run Rescan before changing another component."
  A later failed read never replaces it.
- No action was sent and reading the state failed (the automatic check, a page
  load or Rescan): the banner is new, with a Rescan button. Title: "Component
  state could not be read". Hint: "This page did not send any component action;
  it could not read the current state of the components. {reason} Component
  changes stay locked until a scan reads the state; Rescan reads it again."
  `{reason}` is the sentence of the answer that failed (with the Agent stopped,
  the `AGENT_UNREACHABLE` sentence above), or "Service scan failed." when no
  answer came back.

After a reload only the kind of cause is remembered, not the answer, so a read
banner then gives "Service scan failed."; a record kept from before this change
keeps the old wording.

**What does not change.** The mail-stack card on the Dashboard still has its own
copy of the old stale text and its "Open Components to rescan" button (open; see
the limits). The Components page checks as before; only its banner for a failed
read changes. No database schema change and no new route.

### The defect the lab found, and the two steps that fixed it

The fourteenth record measured the first version of the card. With the guest's
clock one to two seconds ahead of the browser's (1.04 to 2.15 s measured), the
card showed "Not checked in the last 5 minutes" for 20 to 29 seconds after each
of the four automatic checks (1.5 s after the operator's press), because the
page rejected a scan stamp later than its own clock and only its 30-second tick
moved past it (defect D1 of that record).

Step one: the page records when it received each answer that carries a scan
stamp, and takes the earlier of the stamp and that moment as the scan's time.
Step two (above): the server states the age. This also removes the limit step
one left, that a server clock more than five minutes behind the browser's still
read as stale.

## API changes

No new route. Two new fields and one new error code.

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

### `valid_for_seconds`

On `GET /api/v1/license/access`, next to the unchanged `valid_until`.

```json
{
  "can_use_panel": true,
  "valid_until": 1791871840,
  "valid_for_seconds": 55.4,
  "state": "active",
  "observation": "known"
}
```

(The values are an example.)

`valid_until` is the end of access as a time on the server's clock (the earlier
of the licence's expiry and its offline allowance), as before.
`valid_for_seconds` is the same end as the time left when the answer is
written, measured on the same clock: `valid_until` minus the server's clock, to
the millisecond, never below zero (0 when there is no end or it has passed). A
page takes the end as the moment it sent the request plus `valid_for_seconds`;
it does not compare `valid_until` with its own clock. Because that moment is not
later than the server's measurement, the end cannot come out later than the
server allows. A page that finds the field absent, or not a finite number, uses
`valid_until` as before. A positive answer whose time left is 0 is not
accepted. A client that does not know the field ignores it.

### `503 AGENT_UNREACHABLE`

```json
{
  "error": "The CelikPanel Agent on this server is not reachable, so this request was not carried out and nothing was changed. The server owner checks the Agent on the server with sudo systemctl status celikpanel-agent (its log: sudo journalctl -u celikpanel-agent) and, if it is stopped, starts it with sudo systemctl start celikpanel-agent. Once the Agent is running, try again.",
  "code": "AGENT_UNREACHABLE"
}
```

The screens show their own translation of the sentence (the Turkish one is in
the Turkish notes). Where it applies:

- The Panel's shared server-error writer. Every handler that reports a failure
  through it answers `503 AGENT_UNREACHABLE` when a failed dial of the Agent's
  Unix socket (nothing listening, or no socket file; the error may be wrapped)
  is the only cause. That includes the scan handler (`POST
  /api/v1/managed-services/scan`), the shared admission check
  (`beginServiceMutation`) that the mutation handlers run before they change
  anything (nine call sites besides the scan handler at commit 68b79b372; a
  refused admission changed nothing), the firewall read (`GET /api/v1/firewall`)
  and the system check. The system check's other failures now answer the coded
  `INTERNAL` error instead of plain text.
- Not for a joined error. One made of a failure together with its
  compensation, for instance, may carry a change that did happen, so it keeps
  the generic path. The one join accepted is the platform-identity sentinel
  with its cause, because that read runs before the call it guards.
- Left on their own typed paths, not changed: the DNSSEC status, the update
  status and start, domain deletion and mail password rotation.

The answer carries no socket path; the cause is written to the Panel's journal.
A connection that broke after a request was sent (a reset, an end of stream)
keeps the generic path and is an unknown result. An ordinary failure still
answers `500`.

## Behaviour changes you will notice

- The Dashboard's "System services" card no longer waits for you. A stored
  scan older than five minutes is checked once when you look, then in the next
  five-minute window while the tab stays visible.
- Right after a check, the card shows the result at once. It no longer drops
  to "Not checked in the last 5 minutes" for up to 30 seconds when the server's
  clock is ahead of the browser's.
- A server whose clock is more than five minutes behind the browser's no
  longer makes a fresh scan read as stale (component-tested; measured once, on
  Debian 13, with the clock seven minutes behind: one conditional scan, then
  the result to the end of 90 s).
- A server whose clock is more than about a minute behind the browser's used to
  keep every Panel page from loading. Now the server states the time left and
  the browser never compares the two clocks (`valid_for_seconds` on
  `/api/v1/license/access`). Found in the lab; measured after the fix on
  Debian 13 only: with the clock seven minutes behind, and seven minutes
  ahead, the Dashboard loaded at once.
- With the Agent stopped, a refused scan or change, the firewall read and the
  system check say the Agent is not reachable and what to run, in English and
  Turkish, instead of an internal error.
- On the Components page, a failed read says the state "could not be read" and
  why, with Rescan, instead of saying a server action may have completed when
  none was sent. That wording stays for an action whose result is unknown.
- The card's strings are in English and Turkish;
  `dashboard.systemServicesScanStale` is removed.

## What was measured, and how

**Component tests.** On `1a74eeda5`: `gofmt`, `go vet` and the
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

On `7024534bc`: `gofmt`, `go vet` and the arm64 build
clean; `go test ./cmd/panel/... ./internal/...` 7028 passed, 44 skipped, 0
failed (3 new: the time left on every access state; the central writer
answering `503` for a refused dial, a missing socket, a wrapped dial and an
identity-plus-dial error through all three writers, and `500` for an ordinary
error, a reset, an end of stream, a joined dial and a failure with its
rollback; the firewall and system-check reads naming the Agent); `npm test`
1339 of 1339 (3 new: an answer whose `valid_until` is seven minutes past by the
browser's clock but whose `valid_for_seconds` is 55 is accepted, 0 is not, and
without the field the old rule holds; the two banners); `npm run build` within
budget. The commit after it that touched product code, `68b79b372`, changed
two class names only (keys, strings and behaviour unchanged): `npm test` 1339
of 1339, `npm run build` within budget. Measured on a real server once, Debian
13 only, on `68b79b372`: see the sixteenth record below.

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

**The fifteenth native record**
([set14b](../deploy/e2e/release-recovery/evidence/set14b-20261013/README.md),
2026-10-13 UTC). A test-licence build of `1a74eeda5` (the clock rule, the
server-stated age and the Agent answer at the scan and the admission check) on
one disposable Debian 13 guest, with a real headless Chrome 154 on the host: an
English desktop window (1440 by 900), and Turkish for one Dashboard load. The
harness moved the guest's clock to reproduce the fourteenth record's skew; the
offset was read from the answers themselves (each carries `scanned_at` and
`scan_age_seconds`, whose sum is the server's clock when it wrote the answer),
so it is bounded to the request's duration. Results:

- The guest's clock was 1.48 to 1.55 s ahead of the browser's. A stale load
  (stored scan 316.8 s old) sent exactly one conditional scan
  (`max_age_seconds=300`, 200, 27 services, 959 ms), and the card went from
  "Checking components…" straight to the result, 4 ms after the answer, and
  stayed until the page closed 45 s later. No "Not checked in the last 5
  minutes" frame appeared at any time in the run (the fourteenth record
  measured 28.6 s of it at this point on Debian): defect D1 is gone. A fresh
  load (stored scan 46 s old) sent none.
- `scan_age_seconds` was a number in all 19 browser-recorded answers that
  carried a scan, and `0` in the driver's own scan; it is absent from the `503`
  bodies, which carry no scan. It was never `null`, because a scan always
  existed.
- The Components page, on a stored scan 835.2 s old, sent exactly one
  conditional scan (200, 1557 ms) and showed no toast.
- With the Agent stopped, the Dashboard in English, the Components page and the
  Dashboard in Turkish each sent one scan and got `503 AGENT_UNREACHABLE` (5,
  158 and 4 ms). The three bodies are identical: `{"error": <the sentence>,
  "code": "AGENT_UNREACHABLE"}`. The message is in `error` (there is no
  `message` field), and there is no socket path; the cause stayed in the
  Panel's journal. The Dashboard card showed "Unknown / Could not check:" with
  the sentence and the "Scan services" button, in Turkish "Bilinmiyor /
  Kontrol edilemedi:" with "Servisleri tara", held until the page closed with no
  second scan. The Components page showed the sentence in an error toast 28 ms
  after the answer.
- A service restart request for nginx with the Agent stopped (`POST
  /api/v1/service/action`, as the logged-in owner) was refused with the same
  `503`. Nothing changed: nginx's main process id, its start timestamp, its
  restart counter and its exec-start time were identical before and after.
- After the Agent was started again, one press of "Scan services" sent one
  unconditional scan (200, 1037 ms, `scan_age_seconds` 0.001) and the card
  showed the result. The Turkish page had sent nothing by itself in the 15 s
  before the press.
- Six scans were sent in the whole run: the stale load, the Components page,
  the three refused scans and the press.

The record's intake (commit fb5ec06f8) corrected three figures of its README
against the raw files and recorded one wrong `phases.json` field; the figures
here are those of the README as it stands. Nothing of the product was changed by
the run, and its `result.json` carries `native_evidence: false`.

**Found in the lab by the fifteenth record, gone in the sixteenth: the licence
check (D2).** In the same record, with the guest's clock moved back (measured
418.5 s behind the browser's), no Panel page loaded in 90 s. The page read
`/api/v1/license/access` 19 times, every 5 s, all 200, and nothing else: no
managed-services read, no scan, no Dashboard. The cause was read from the code,
not from the answers: `valid_until` is a time at most 60 s after the last
verification on the server's clock, and the page refused a positive answer
whose `valid_until` was not after the browser's clock. The record did not keep
the access answers' bodies and took no picture of the held page. The correction
(`valid_for_seconds`, above) was made after the record; the sixteenth record
measured it. For the same reason the measurement the fifteenth record set out
to make for a clock behind (one conditional scan, the result at once, no second
scan) could not be made there; the sixteenth record made it.

**Observed in the same record, then corrected in source, then measured in the
sixteenth.**
With the Agent stopped, the Components page showed "A server action may have
completed" although none had run (O2), and the firewall read answered `500`
(O3: the Components page showed "Firewall: Status unknown" with an internal
error text, the Dashboard "Part of this server's state could not be read").
The read banner and the shared error writer, above, answer them; they had
component tests, and the sixteenth record measured both.

**The sixteenth native record**
([set14c](../deploy/e2e/release-recovery/evidence/set14c-20261013/README.md),
2026-10-13 UTC). A test-licence build of `68b79b372`, the last product commit,
on one disposable Debian 13 guest, with a real headless Chrome 154 on the host:
an English desktop window (1440 by 900) and, for the Components page, one
Turkish phone window (390 by 844). The installed archive was a disposable
fixture build whose tree differs from `68b79b372`'s in one file,
`deploy/release-sequence-policy`. The harness moved the guest's clock. The
offset was read from the answers themselves: from a managed-services answer
(`scanned_at` plus `scan_age_seconds`) and, more tightly, from each licence
access answer (`valid_until` minus `valid_for_seconds`, the server's clock when
it answered). Offsets, guest minus browser:

- Natural, before the first load: -78 to +64 ms.
- Seven minutes behind: -420,002 to -419,945 ms.
- Seven minutes ahead: +419,996 to +420,058 ms.
- Restored: +3 to +52 ms, and 30 s later -4 to +64 ms. The harness's net shift
  was 0 at the end.

Results:

- **The licence check, three loads.** The same Dashboard load three times, each
  with a stored scan older than 300 s (316.8, 317.1 and 930.9 s) and watched
  for 90 s: with the clock as it was, 420 s behind, and 420 s ahead. The
  Dashboard mounted (first managed-services read) 192, 130 and 132 ms after
  the load, and the card appeared 253, 165 and 172 ms after it. The page read
  `/api/v1/license/access` 4 times in each 90 s (at 0.11, 44.1, 60.1 and
  89.1 s; 0.07, 44.0, 60.1 and 88.9 s; 0.07, 42.6, 60.1 and 87.6 s), every
  answer 200 with `can_use_panel: true`, `state: active`,
  `observation: known`. Each load sent exactly one conditional scan
  (`max_age_seconds=300`, 200; 1197, 956 and 1047 ms) and the card went from
  absent to checking to "3 / 3 running / installed" and stayed so to the end.
  No second scan came (none seen 88.8, 89.0 and 89.1 s after the first). With
  the server seven minutes behind, the page behaved as with the clock correct:
  no hold, no read every 5 s (the fifteenth record: 19 reads at 5 s in 90 s
  and no Dashboard). The same with the server seven minutes ahead.
- **The access answers.** All 12 answers of the three loads have exactly the
  keys `can_use_panel`, `observation`, `state`, `valid_for_seconds` and
  `valid_until`; `valid_for_seconds` is a JSON number, 42.5 to 60.0 s. (In the
  whole run 17 answers carried the same five keys; the four of the Agent phase
  ranged from 16.7 to 59.99 s.) In all three loads the page read again 15 s
  before the deadline computed as the moment it sent the request plus
  `valid_for_seconds`. With the server behind, the absolute `valid_until` was
  already in the past by the browser's clock: 361.1 s before the first answer
  was received and 360.0 to 376.1 s before the next three (not 420 s, because
  `valid_until` is the server's clock plus about 60 s); by calculation the old
  rule would have refused them, its deadline lying about 419.9 s before the
  one the page used. With the server ahead, the absolute `valid_until` would
  have let the page accept access 420.05 s longer than the server said; the
  page's own deadline was not later. The browser's internal deadline is not
  readable from outside; it is inferred from the timing of the reads.
- **The Agent stopped, the reads.** The firewall read (`GET /api/v1/firewall`),
  the system check (`GET /api/v1/system/check`) and a restart request for
  nginx (`POST /api/v1/service/action`) each answered `503 AGENT_UNREACHABLE`.
  The three bodies are the same, hold only `code` and `error`, and name no
  socket path; the cause stayed in the Panel's journal. The refused restart
  changed nothing: nginx's main process id, restart counter and start
  timestamps were identical before and after. (The fifteenth record measured
  the firewall read as `500`.)
- **The Dashboard with the Agent stopped.** One conditional scan, `503` in
  3 ms; the card went from absent to checking to "Unknown / Could not check:"
  with the Agent sentence and the "Scan services" button, held to the end of
  40 s, with no second scan. The page's own firewall read answered the same
  `503`. The other cards (CPU, memory, disk, hosting, recent activity) were
  shown. The Dashboard showed no firewall banner, and its "Needs attention" box
  did not name the Agent (O6, below).
- **The Components page with the Agent stopped.** English desktop: one
  conditional scan, `503` in 107 ms; the banner "Component state could not be
  read" with the Agent sentence as the reason, the line that the page sent no
  component action, and a "Rescan" button, seen 1007 ms after the load and
  still there 5 s later. Turkish phone: one scan, `503` in 88 ms; the banner
  "Bileşen durumu okunamadı" with "Yeniden tara", seen 1012 ms after the load.
  Neither page showed the "Service state needs verification" or "may have
  completed" wording (in Turkish "Servis durumu doğrulanmalı", "tamamlanmış
  olabilir"). Each page also showed the Agent sentence as an error toast.
- **The Agent started again, and the press.** The Turkish page sent nothing by
  itself in the 5 s before the press. One press of "Yeniden tara" sent one
  unconditional scan (200, 1014 ms, `scan_age_seconds` 0, 27 services); the
  banner was gone 1092 ms after the press and still gone 10 s later.
- The browser sent 7 scans in the whole run: one per measured load, the three
  refused ones and the press.

The record found no defect in the code those commits changed. Its intake
checked 273 files listed in four checksum lists (all present, no secrets or
operator identity) and corrected two README statements against the raw files.
Nothing of the product was changed by the run, and its `result.json` carries
`native_evidence: false`. It closes no P0 row. Its observations O4, O5 and O6
and the untranslated mail-stack text are in the limits.

**The seventeenth native record**
([set15](../deploy/e2e/release-recovery/evidence/set15-20261013/README.md),
2026-10-13 UTC). The pre-publication measurement, and the first record to run
`1636b73f0` itself, the commit this release is prepared in, with no fixture
commit between. It is also the first to run an update from v0.1.0-alpha.83.
Test-licence builds, none a release: the candidate (version v0.1.0-alpha.84,
sequence 84) was built from `1636b73f0`; the baseline is a rebuild of the
alpha.83 tag's source (the tag commit, unpatched), not the published archive;
the defective candidate is `1636b73f0` plus the migrate-only defect of an
earlier record, in `cmd/panel/main.go`. Disposable guests, a real headless
Chrome on the host, an English desktop window (1440 by 900) on Debian 13
and a Turkish desktop window on Ubuntu 24.04.

- **Fresh installs, Debian 13 and Ubuntu 24.04.** The version route answered
  `v0.1.0-alpha.84`, commit `1636b73f0979…`, `agent_matches: true`,
  `schema_version: 44`; the sidebar read "CelikPanel · v0.1.0-alpha.84 ·
  build 1636b73f0979". The bootstrap pins read 84 and `v0.1.0-alpha.84` in the
  archive's `libexec/get.sh` (lines 6 and 7) and in the installed copy of that
  file on both guests. Sign-in, the licence and the setup purpose `web` passed;
  the setup waits at the DNS step and was left there. The domains list was
  empty (`GET /api/v1/domains` answered `[]`) and the page showed no
  first-state row. `GET /api/v1/license/access` carried `valid_for_seconds`
  with `can_use_panel: true`, `state: active`, `observation: known`;
  `GET /api/v1/managed-services` carried `scan_age_seconds`.
- **The Dashboard with a stale scan.** The stored scan was 317.1 s old
  (Debian) and 317.2 s old (Ubuntu) at the load. In 90 s each Dashboard sent
  exactly one `POST /api/v1/managed-services/scan?max_age_seconds=300`: 200 in
  1126 ms (Debian) and 1590 ms (Ubuntu). The card went from absent to
  checking (1145 ms and 1611 ms after the load) to "3 / 3 running / installed"
  ("3 / 3 çalışan / kurulu" in Turkish). "Open Components to rescan" was not
  seen. The Components page loaded right after, with a scan 90 s old, sent no
  scan, as designed. On Ubuntu the Components page with a stale scan (317 s)
  sent one conditional scan (200, 2026 ms) and showed no alert and no toast.
  On Debian that load was not measured.
- **The update from v0.1.0-alpha.83, Debian 13, good candidate, three runs
  (a, b, c).** A baseline with a hosted static site, updated through the
  Panel's own update path. Every run ended `phase: succeeded`,
  `reason: update_verified`, `terminal_proof: update_verified`, with the build
  identity of the Agent and the Panel at `v0.1.0-alpha.84`, `1636b73f0…`, the
  release floor at sequence 84, and the schema ledger `as-expected`. The schema
  was 44 before (alpha.83 answered 44) and 44 after. No migration file changed
  between the alpha.83 tag and `1636b73f0` (9 files under `cmd` and
  `internal`, none a migration). Domains showed 1 row,
  `site_config: managed_unchanged`.
- **The Dashboard after the update (run c).** With a stored scan 441 s old,
  from before the update, it sent one conditional scan by itself (200,
  1210 ms) and went from absent to checking (1240 ms) to "6 / 6 running /
  installed". The Components page sent no scan right after and one conditional
  scan (200, 1187 ms) once the scan was stale. The Dashboard's Mail stacks
  card showed "Status needs refresh" and "Open Components to rescan" for about
  1.2 s, until the scan answered. That is the known open item in the limits
  (the card has its own copy of the old text); it is not new.
- **The defective candidate: the automatic return.** With the fault
  "reboot at `payload_restored`", the guest reset once. The update recovered
  by itself: `phase: recovered`, `reason: rollback_verified`,
  `previous_failure: update_failed`, two automatic attempts. Afterwards the
  Agent and the Panel reported the alpha.83 build (`v0.1.0-alpha.83`,
  `0447558e…`), and the Panel answered: the single-page root and an API route
  200, one route 401 or 403, an unknown path 404, and the version route named
  alpha.83's build.
- **Open finding: `managed_site_files` after the return.** The driver's
  terminal step failed on its database comparison: the Panel's database after
  the return differs from the copy taken before the update in one table,
  `managed_site_files` (one row, 21 columns both times; only digests were
  recorded, not the rows). The three good runs list the same table as
  differing and pass, because the update path does not require equality. The
  likely cause, from the source and **not confirmed from the rows**: the Panel
  re-observes each site's files at every start and rewrites the row's
  observation fields, `observed_at` among them
  (`internal/services/site_file_ledger.go` lines 263 to 273, an upsert with
  `observed_at = excluded.observed_at`, and lines 297 to 300, 327, 391 and
  426; the column is in `internal/db/migrations/044_managed_site_files.sql`
  line 31). After the return alpha.83 starts and observes again, so the
  table's digest changes. The driver's check did not treat the table as
  volatile because it did not exist in the alpha.82 baselines of the set12
  and set13 records (ledger 43). Because the step failed, the readings after the
  return and the browser window after it did not run, so there is no sidebar
  picture or Dashboard reading after the return. The product-level facts
  above stand. The finding is recorded, not judged.
- **The recovery foundation file after the return.** After the automatic
  return the release floor reads sequence 84 and the recovery foundation file
  still names the defective candidate's commit, while the running build is
  alpha.83. Recorded, not judged. This is not new as a pattern: the
  [upd2](../deploy/e2e/release-recovery/evidence/upd2-20261001/README.md) and
  [upd7](../deploy/e2e/release-recovery/evidence/upd7-20261001/README.md)
  records of 2026-10-01 already show the floor and the recovery foundation
  staying at the candidate's values after a rollback (upd2 calls the floor's
  case by design), and the set12 record showed the floor at 83 after its
  return to 82. The set12 and set13 READMEs do not mention the foundation
  file. It is new only as a reading with an alpha.83 baseline.
- **The host's sleep.** The laptop host slept from about 10:20Z to 12:35Z
  (modern standby; the keep-awake request did not prevent it) during the
  third good run. The guest was frozen and its clock ended 8050 s behind the
  browser's. That run's workload verdict failed ("workload interruption outside
  the expected windows: web, smtp, cron, panel"); this is the environment's
  doing. Runs a and b, without the sleep, passed that verdict. The update of
  run c itself ran after the wake.
- **Not run or not measured:** Arch; the stale Components load on Debian fresh;
  the readings after the defective return (see the open finding).

The intake checked 1578 listed files, all present, with no secrets or operator
identity; one host path was redacted and four README statements were corrected
against the raw files. The run changed no product code, and its `result.json`
files carry `native_evidence: false`. It closes no P0 row. The open finding is
recorded, not judged a product defect.

## Limits of this release

These are known. They are not hidden defects. The first group is new with this
release; the second is the list of v0.1.0-alpha.83's limits, condensed, each
marked "still open" because nothing in this release addresses it. Two alpha.83
sentences were dropped in the condensing (the evidence-scope host-standby
sentence and "During the update to this release the open page is still
alpha.82's"); both still hold.

New with this release:

- **Open finding: `managed_site_files` after an automatic return.** In the
  seventeenth record the lab's database comparison after the return from the
  defective candidate found one table, `managed_site_files`, differing from the
  copy taken before the update. The cause is not confirmed from the rows (only
  digests were kept). The likely cause, from the source, is that the Panel
  rewrites each row's observation fields, `observed_at` among them, at every
  start, so the return's restart changes them
  (`internal/services/site_file_ledger.go` lines 263 to 273, 297 to 300, 327,
  391 and 426; `044_managed_site_files.sql` line 31). The lab's check must
  exclude the field or the table in the next record. Not judged a product
  defect; the return itself happened and the Panel answered as alpha.83.
- **The recovery foundation file names the defective candidate after the
  return.** After the automatic return the file still names the defective
  candidate's commit while the running build is alpha.83, and the release floor
  reads 84. Recorded, not judged. Already known as a pattern from the upd2 and
  upd7 records (2026-10-01) and the floor from the set12 record; open.
- **Ubuntu and Arch were not measured after the changes.** The fourteenth
  record ran Debian 13 and Ubuntu 24.04 (the first commit only), the fifteenth
  and sixteenth Debian 13 only. The Debian labs' idle Arch peer was not
  measured. The seventeenth record installed the candidate fresh on Ubuntu
  24.04 and measured the Dashboard and the Components page there; no update
  ran on Ubuntu, and Arch was not run.
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
  old text and its "Open Components to rescan" button. It does not check by
  itself.
- **The sidebar's build identifier wraps in the middle of a word.** Observed by
  the owner after the update to v0.1.0-alpha.83; not fixed.
- **Measured on Debian 13 once, nowhere else:** the licence check's fix (D2),
  the typed Agent answer on the firewall read, the system check and a restart
  request, and the Components page's read banner, all in the sixteenth record.
  Ubuntu and Arch were not run after these changes. The "may have completed"
  banner after an action whose result is unknown was not measured: producing it
  needs a component action whose answer is lost, which the harness has no cheap
  way to do. It has component tests only.
- **O4: the Turkish Components page shows the firewall banner's server sentence
  in English.** Seen in the sixteenth record with the Agent stopped:
  "Güvenlik duvarı: Durum bilinmiyor" followed by the English Agent sentence.
  `FirewallBar.load` takes the answer's `error` string as the message
  (`web/src/components/ServiceList.tsx:3259-3271`, `:3286`) and shows it
  (`:3393`) instead of the translation of the code (`err.AGENT_UNREACHABLE`),
  which the scan banner and the toast use. Open for the next release. Line
  numbers here are those of `68b79b372`.
- **O5: the firewall banner is not read again after the Agent returns.** After
  the press and 10 s later, the Turkish page still showed "Durum bilinmiyor"
  with the Agent sentence although the Agent was running and the scan had
  succeeded: `FirewallBar` reads only on mount (`ServiceList.tsx:3289-3291`)
  and after its own action (`:3318`); Rescan does not read it. A reload would.
  Open for the next release.
- **O6: the Dashboard's "Needs attention" box does not name the Agent.** With
  `GET /api/v1/firewall` answering `503 AGENT_UNREACHABLE`, the box says only
  that part of the state could not be read (`dashboard.attentionUnread`,
  `web/src/components/Dashboard.tsx:895`, `:1163-1165`), while the System
  services card on the same page names the Agent and the owner's commands.
  Open for the next release.
- **Untranslated English text on the Turkish Components page.** The mail stack
  descriptions ("Postfix SMTP and Dovecot IMAP/POP3 with authenticated
  submission.", "Core mail plus Nginx, PHP-FPM and Roundcube webmail.") and
  "Some components are already present. Installing this profile will reconcile
  the full plan without removing completed services." Seen in the sixteenth
  record; not looked into. Open for the next release.
- **The guest's NTP settings were not measured.** In the sixteenth record
  `timedatectl show` printed no fields, so only the return codes of the clock
  changes are recorded; whether the time service stepped the clock after it was
  switched on again cannot be told.
- **The scan's age with the server's clock behind the browser's was measured
  once.** The fifteenth record could not make that measurement (D2 held the
  page). The sixteenth made it on Debian 13 only, once each with the clock seven
  minutes behind and seven minutes ahead (see above); Ubuntu and Arch were not
  run.
- **The fifteenth record ran Debian 13 only.** Ubuntu and Arch were not
  re-measured; the window rule, the hidden tab, the held (`409`) state,
  `server_setup_busy` and the never-checked state were not asked for (the
  window rule is unchanged since the fourteenth record). Its access answers'
  bodies were not recorded (the sixteenth recorded them), and the guest's
  time-service settings were not.
- **The lab tools still send only `valid_until`.** The mock server `mock.mjs`
  and the cold-load tools `cold-load-set9.mjs` and `cold-load-set11.mjs` answer
  the access check with `valid_until` only, so they exercise the old rule and
  not `valid_for_seconds`.
- **The HSTS retirement in `DomainSSLSettings` still compares a server timestamp
  with the browser's clock.** The same class as the defects above; not changed
  here. Open.
- **"Nothing was changed" assumes no earlier change in the same request.** The
  writer treats the failed dial as the Agent answer only when it is the only
  cause, and its joined-error guard covers aggregate errors (a failure with its
  compensation). A handler that changes something and later fails on a separate
  dial that is not joined to the earlier outcome would still claim it. Open; no
  handler-by-handler audit is claimed.
- **The candidate itself was measured before publication, once.** The
  seventeenth record ran commit `1636b73f0` itself, not a fixture: fresh
  installs on Debian 13 and Ubuntu 24.04, and the update from a rebuild of the
  v0.1.0-alpha.83 tag on Debian 13. This closes the earlier limit that the
  release preparation (sequence number, bootstrap pins, version lines) was in
  no measured build. What it does not cover: the archives were test-licence
  rebuilds, not the signed ones; the baseline was a rebuild of the alpha.83
  tag, not the published archive; the update ran on Debian 13 only; Arch was
  not run; the defective candidate's readings after the return did not run
  (see the open finding).
- **Evidence scope.** One laptop host; a test-licence build, not the signed
  archive; guests with outbound NAT whose traffic was not captured; each cell
  ran once; English was seen on a desktop window on Debian and Turkish on a
  phone window on Ubuntu, not both languages on both. The fifteenth record
  added Debian 13 only: an English desktop window, Turkish for one Dashboard
  load, the guest's clock moved by the harness. The sixteenth record added
  Debian 13 only again: an English desktop window, Turkish on one phone window
  for the Components page, the guest's clock moved seven minutes each way, each
  cell once. The seventeenth record added fresh installs on Debian 13 (English
  desktop window) and Ubuntu 24.04 (Turkish desktop window), each once, and the
  update on Debian 13 (good candidate three times, defective candidate once);
  the clock was not moved.
- **Other cards when the Agent is down.** In the fourteenth record, with the
  Agent stopped, the Dashboard's CPU, memory and disk cards showed "—" with the
  amber banner; the firewall read answered `500` in the fourteenth and fifteenth
  records. In the sixteenth record the firewall read answered `503
  AGENT_UNREACHABLE` naming the Agent, and the CPU, memory and disk cards showed
  values (0 %, 15 %, 11 %); this release does not change those cards, and the
  difference was not investigated. The Dashboard's "Needs attention" box does
  not name the Agent (O6), and the Turkish Components page's firewall banner
  shows an English sentence (O4).

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

1. The next native record: done (set14c, the sixteenth, Debian 13 only,
   2026-10-13): the licence check's fix with the server's clock seven minutes
   behind and ahead, the scan's age with the clock behind, the Agent answer on
   the firewall and system-check reads, and the Components page's read banner,
   on a real server. No longer open here. Not measured there, and still open:
   Ubuntu, Arch, the "may have completed" banner after an action, the guest's
   NTP settings, and its observations O4 to O6 (see the limits).
2. The pre-publication measurement: done (set15, the seventeenth record,
   2026-10-13), on commit 1636b73f0 itself: fresh installs on Debian 13 and
   Ubuntu 24.04, the update from v0.1.0-alpha.83 on Debian 13, and the
   automatic return of a defective candidate. It left the two open items in the
   limits. The owner's steps below are unchanged.
3. The release preparation: done in commit 1636b73f0, which changes no file
   under `cmd/`, `internal/` or `web/`. It holds the release sequence policy 84
   after 83 with the alpha.83 release commit as previous, the
   `download-portal/get.sh` pins, the README version lines and three contract
   tests; the three contract tests pass in the build guest. Still open: the
   repository variable `CELIKPANEL_RELEASE_SEQUENCE=84`, which the owner sets
   before the tag. The real archive is built and signed by CI on the tag.
4. The number: done, 2026-10-13. The owner decided that it is v0.1.0-alpha.84.
5. The pull request's last head gets its own CI run.
6. The owner merges, tags the release, checks the six signed assets as the
   signed release contract describes, and publishes the download portal.
7. After the release is published, the owner of each installed server updates
   that server from the panel's own update screen. This is a separate step from
   every step above; no one else starts it.

Install this release only through CelikPanel's update interface. Publishing it
does not update installed servers; only the owner of a server starts its
update, from the panel's own update screen.
