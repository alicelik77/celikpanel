# v0.1.0-alpha.96 (candidate, not published)

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.96.tr.md)

This is the release that follows v0.1.0-alpha.95. It is a candidate: it has not
been tagged or published, and no installed server has been updated to it. Written
on 2026-10-17, on the branch `feat/alpha96-update-notice`, from `main` after the
publication record of alpha.95 (the alpha.95 release commit is
`5f3cb1a2fa336c529740a20e9a543b8ac086dc8d`).

## Who should update, and why

Everyone who runs alpha.95 or an earlier release, once this release is published.
While updating to alpha.95 the owner asked, in the owner's own words,
"belirli periodlarda güncellemeleri kontrol etmeli ve güncelleme var diye
uyarmalı. güncelleme konusunda yardımcı olmalı." In English: it should check for
updates at regular intervals and warn that an update exists; it should help with
the update (D-035). This release does that. A panel can only look for updates
once it runs this release, so alpha.96 cannot announce itself to a panel that
still runs alpha.95; the first update to it is found with the "Check for updates"
button as before. Only the owner of a server updates it, from the panel's own
update screen.

## What changes on screen

**The panel looks by itself.** About 2 minutes after the panel starts, and then
every 12 hours, the panel asks celikpanel.net whether a newer signed release
exists. It is the same read-only check as the "Check for updates" button, made by
the panel and not by the browser. It installs nothing, changes nothing on the
server. One check is cut off after 40 seconds. After a failed check the next try
is in 1 hour, then in 3 hours, then back to the 12-hour period, so a passing
outage does not become a stream of requests. The check is skipped, and tried
again in 1 hour, while an update was started in the last hour, while the server
reports that another change is in progress or that it cannot verify that state,
while server setup is changing the server (not while it waits for a DNS record),
or when the Agent does not answer; a skip is not counted as a failure, is kept in
memory only and never replaces the known result. A busy package manager does not
skip it. **The panel never writes the result while an update is admitted or
running:** right before writing it reads again that no update was admitted and
that the server reports no change in progress, and otherwise keeps the result in
memory until the next idle pass, because a write to the panel's database during
the updater's read-only check makes that check stop. The Agent is asked with no
lock held; the panel's own change lease is held only for a quick in-process
re-check and the write itself.

**The owner can switch it off.** Settings → CelikPanel updates has "Check for
updates automatically", on by default: "Once every 12 hours the panel asks
celikpanel.net whether a newer signed release exists. Nothing is installed by
itself." Off means the schedule makes no network read at all; the "Check for
updates" button still works.

**Administrators are told, calmly, only when a release is available.**

- a small link under the version line in the sidebar, "Update available ·
  v0.1.0-alpha.96", which opens Settings → CelikPanel updates;
- one quiet row at the top of the Dashboard, "A new CelikPanel version is
  available: v0.1.0-alpha.96", with "Review and update" and "Remind me later" (the
  row stays hidden for 24 hours in that browser for that version; the sidebar link
  stays). It is not placed under "Needs attention", because an update is not a
  problem;
- in Settings, a small dot and "Update available · v0.1.0-alpha.96" beside
  "CelikPanel updates".

No alarm colour is used for an available update. A failed, unknown or skipped
automatic check shows nothing outside the update card, and a release found earlier
stays offered after a failed check. People who are not administrators see and read
none of this.

**The update card helps with the update.** It opens already knowing the last
result, without a button press and without a call to the release server, and has
one status line: "You are on the latest version (checked 2 hours ago,
automatically)", or "v0.1.0-alpha.96 is available (found 2 hours ago)". After a
failed automatic check it says what is known and what is not, for example "Last
automatic check: 2 hours ago — could not get a verified answer from the release
server (celikpanel.net)." and "The next check runs by itself at 15:30; you can also
check now." A check that did not finish in time says that its result is not known,
which is not a failure. A skipped check says why ("another change was in progress
on the server"). When automatic checks are off it says "Automatic checks are off.
Press "Check for updates" to look now."

When an update is available the card shows at most one notice about an earlier
attempt to that version (neutral when it stopped before changing anything; an
attempt that was rolled back or failed keeps the attention mark and its own
words), then "Before you update": the panel and its Agent restart once for about
N seconds (taken from the last measured restart, at least 90); hosted websites,
mail, DNS and databases keep running; this is an alpha release, so a current backup
of important data is recommended (plain text, not an amber box); nothing starts by
itself; you can follow every step on the page. Then the readiness result ("The
server is ready for the update." with a check mark; a busy server keeps its
attention box) and one filled button, "Update to v0.1.0-alpha.96". "Check for
updates" is outlined. The version, commit, platform, sequence, package size and
SHA-256 are in a collapsed "Technical details". The screen the owner sent from
Frankfurt on 2026-10-17 showed two amber boxes, a green "A signed update is
available." and a second identical row below the Start button; all of that is gone.

**Nothing updates by itself.** Start is unchanged: the panel runs the discovery
again and requires the exact same target before any start. Only an administrator
starts an update, from the panel's own update screen.

**The stop dialog says more about a read-only check that stopped.** Two steps of
the recovery tool's read-only check, `mode` and `owner`, had no words and fell back
to "Reason: a read-only check could not verify the current installation." They now
say what they mean ("the recovery tool was asked for a kind of check it does not
know" and "the read-only check was not running with the server's administrator
(root) rights"). For any other step the page has no words for, the server's own
first diagnostic line is shown under the generic reason ("The server reported: …")
instead of only inside the collapsed Details. This is why a stop like Frankfurt's
update to alpha.95 (below) will say more the next time; these texts do not claim
they are the cause of that stop.

**Four alpha.95 open items are closed.**

1. Verify of a component bundle is now also held, with the server's own readiness
   reason, while the server is not ready for any reason (a busy host lock, a running
   package manager, a state that cannot be verified), not only while a component
   operation runs; it comes back by itself when the page's readiness re-read says
   the server is ready. This reverses alpha.95's note that Verify stayed usable on a
   busy host lock or package manager.
2. The Roundcube cleanup offer follows the newer of the Panel's newest Roundcube
   install or removal entry and the webmail profile's last attempt (at an equal
   second the Roundcube entry decides), so a later clean removal ends an offer that
   a failed webmail attempt raised.
3. `/services?cleanup=roundcube` removes `?cleanup` from the address after it opens
   the confirmation, so a reload does not reopen it.
4. Buttons at the foot of a dialog are at least 44 px high (and centred) on phones,
   639 px and narrower.

The browser screen check that was not re-run after alpha.95's review is replaced by
batch 21 in this candidate (see "Evidence").

**What the first look at batch 21 corrected (2026-10-17).** On a phone the
Settings page was about 730 px wide when an update was available (the
screen-reader note of the CelikPanel updates item was laid out against the page,
not the item); the Dashboard row broke the version as "v0.1.0-" / "alpha.96"; in
the dark theme the selected Settings item's icon was white on a pale square; the
phone menu button was under 44 px. The note is now placed inside its item, a
version is kept whole in the row, the sidebar link and the Settings note, the
selected icon uses the primary pair of colours, and the menu button is 44 px. In
Turkish the manual check has one verb everywhere: "Güncellemeleri denetle" (the
button, "Güncellemeleri otomatik denetle", "denetlendi" and the related messages),
where the button used to say "Güncellemeyi kontrol et".

**What the independent review of the candidate corrected (2026-10-17).** The
first form of the candidate wrote a skipped check to the panel's settings and
could write a result while an update was being admitted, which could stop the
updater's read-only check; it treated a setup waiting for a DNS record as a
running setup; it assigned the scheduler after management opened. Now: a skip is
memory only and is said beside the known result ("The last automatic check was
skipped …"); a result is written only when no update is admitted or running; only
a setup that changes the server holds the check; the scheduler is prepared and a
stale record cleared before management opens; switching the check on again
continues 12 hours after the last completed check; a rebuild with the same version
and another commit is stored as the Agent's signed answer says; the stop dialog
keeps the whole server message behind Details when the line shown is cut (with an
ellipsis) or differs; a snoozed Dashboard row is never painted for a frame; the
switch says "not known yet" instead of "on" before its setting was read; the card
redraws its ages every minute while visible; and the Turkish busy and restart
messages use "denetle" and "bu yaklaşık N saniye sürer".

## What does not change

- No Agent change (`cmd/agent` is untouched), no new Agent RPC and no new Agent job
  kind. The updater's mutation path is unchanged: `update.sh`, the behaviour of
  `get.sh`, the Agent's system update worker, rollback and recovery. The release
  touches update discovery (read-only) and the update screen only. Therefore no
  native record is made, by the owner's instruction of 2026-10-16.
- Schema 44, no migration. The stored discovery result is a setting in the Panel's
  existing settings table (the mechanism of the last-restart record).
- The existing `/api/v1/panel/update/check`, `/update/start` and `/update/status`
  keep their shape (the check now also records its result). The new read is
  `GET /api/v1/panel/update/discovery`; it is a separate path because
  `/update/status` already exists and answers the status of one update request by
  its request id, and the read for administrators makes no Agent call and no network
  read. `PUT` on the same path saves the switch.
- Hosted websites, mail, DNS, databases, scheduled jobs and certificate renewal do
  not depend on the check. If the check cannot reach the release server, nothing on
  the server changes.
- Installed panels are updated only by their owner, from the panel's own update
  screen.

## Known gaps

- **Nothing was measured on an installed server.** The scheduled check has not run
  against a real Agent or the real release server: the Agent is faked in the Go
  tests, and the browser batch uses a loopback mock. No contact was made with
  celikpanel.net, licence services or a certificate authority from tests.
- **The cause of the stop of Frankfurt's update to alpha.95 is still unknown.** The
  update stopped in its read-only check on 2026-10-17 and the server stayed on
  alpha.94, so nothing installed was changed. The stop dialog showed only the
  generic reason, and the Details text is pending from the owner. No cause is
  guessed here, and the texts added in this release are not claimed to be it.
  Nothing about Boston's update to alpha.95 is recorded.
- **Reading installed package versions for every component remains open.** The
  Panel reads a version only for PHP-FPM and Node.js; a real answer needs a new
  Agent read, which this release does not add.
- The Roundcube files left on disk are still not seen by the Panel, which relies on
  its own record (unchanged from alpha.95).
- **The Turkish screen catalogues are close to their size ceiling.** To keep the
  server screens' Turkish part under its 160.25 KiB limit (the limit was not
  raised), the update card's reasons, its "Before you update" list and the switch
  texts moved to the general screen catalogue, and three texts the card no longer
  uses were removed. After this, the Turkish server part has about 0.3 KiB left
  and the general Turkish part about 1.1 KiB; the next text added there needs the
  catalogue split again.
- P0.1-P0.5 stay **partial**; no P0 item is closed or advanced; a candidate is not
  acceptance.

## Evidence

Everything below was run on the candidate tree on 2026-10-17 in a development
environment; none of it was run on an installed server.

- Go: `./cmd/panel/` and `./internal/...` 7147 passed, 0 failed, 43 skipped on the final tree after the review corrections (run with `-timeout 45m`; the panel package alone takes about 16 minutes in the WSL guest); `go vet ./cmd/panel/` and gofmt clean; the discovery tests also passed under `-race` (Go 1.27 in the `archlinux` WSL guest, which has a C compiler; the Debian guest has none)
- Web: `npm test` 1623 passed, 0 failed on the final tree; `npm run build` succeeded with the bundle budget unchanged (Turkish server screen part 159.96 of 160.25 KiB, Turkish general screen part 159.25 of 160.25 KiB)
- Browser batch 21 (mock server; replaces batch 20): six scenarios (current, available, an earlier attempt that stopped, a failed automatic check, switched off, the Dashboard row before and after "Remind me later") in English and Turkish, light and dark, at 1440 and 390 px: 8 configurations, 100 screenshots. The first look found the phone Settings page about 730 px wide, the version broken at its hyphen in the Dashboard row, the selected Settings icon nearly invisible in the dark theme, a phone menu button under 44 px and two Turkish verbs for the manual check; all fixed (see above). The second look, on the final build: no horizontal overflow, no phone target under 44 px, no amber or red on these screens, no untranslated key and no English on a Turkish page, the version whole, one filled action button, no update notice outside the card after a failed check. Batch 21 was not run again after the review corrections; they are covered by component and mounted tests
- Release contracts (sequence policy 96 after 95, download portal, signed
  manifest): sequence policy (Git Bash) "95 -> 96" passed; download portal and signed manifest (WSL guest) passed

Go tests of the discovery loop use a faked Agent and cover the jitter bounds, the
back-off of 1, 3 and 12 hours, the skip reasons (a skip makes no network read, and
an unanswering Agent is a skip, not a failure), no check while the switch is off,
no record when shutdown interrupts a check, an automatic check keeping the release
found earlier, the manual check writing the same record, the record being cleared
when the build reaches the target, and the administrators-only read that makes no
Agent call. Web contract and mounted tests cover the update card, the notices and
the English and Turkish texts. Component, mounted and mock-browser tests do not
establish native behaviour.
