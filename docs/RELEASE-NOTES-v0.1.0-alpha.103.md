# v0.1.0-alpha.103 (candidate)

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.103.tr.md)

This is the candidate that follows v0.1.0-alpha.102. **It is not published.**
Written on 2026-10-23, as a candidate, on the branch
`fix/alpha103-fresh-update-check`, from `main` after the publication record of
alpha.102 (the alpha.102 release commit is
`085cc400131072f3bcda81b560bfcf98dad47f03`).

## Who should update, and why

Anyone who updates CelikPanel from its own update screen, once this release is
published. Two things the owner saw on 2026-10-23, with both servers on alpha.100:

- **Frankfurt.** The sidebar said "Update available · v0.1.0-alpha.101". The
  update screen it opened offered "Update to v0.1.0-alpha.101", although
  alpha.102 had been on celikpanel.net since about 10:02Z; only "Check for
  updates" showed alpha.102. In the owner's words: "güncellemede her zaman en son
  versiyonu göstermedi … sol alt kısımda 101 nolu güncellemeyi gösterdiğinde ve
  onu tıkladığımda güncelleme ekranında en son güncelleme görünmesi gerekmez
  miydi" (the update screen did not always show the newest version; when the
  bottom left showed update 101 and he clicked it, should the update screen not
  have shown the newest one?). He then updated Frankfurt to alpha.102 himself.
- **Boston.** The update to alpha.102 stopped without changing anything because
  the server's package manager was busy. While that notice was open, the
  "CelikPanel updates" card showed only the installed version, "Technical
  details" and the automatic-check switch, while the sidebar and the Settings
  item still said "Update available · v0.1.0-alpha.102", and the notice said
  that the card would tell when the package manager was free.

Only the owner of a server updates it, from the panel's own update screen.

## What changes on the update screen

**Opening it checks for the newest release** when the known result is older than
ten minutes and automatic checks are on (D-035 amendment). While it checks, the
card says "Checking celikpanel.net for the newest release…" with motion, "Known
before this check: v0.1.0-alpha.101 is available (found 13 hours ago).", and "The
update button waits for this answer, at most about a minute. Nothing starts by
itself."; the known offer stays in view and the Update button waits. Then the
newest release is offered, and the sidebar link, the Settings item and the
Dashboard row show the same version.

- Ten minutes, so that opening the screen several times in one sitting does not
  ask celikpanel.net each time (screen openings make at most one completed
  release read per ten minutes; a check that is skipped or does not finish keeps
  nothing, so the next opening may try again), while the screen no longer offers
  a 12-hour-old result when a newer release is out and the check can run. A
  completed check is kept even if the browser leaves before its answer arrives.
- It is the same check as "Check for updates" and the schedule, one at a time
  with them: an opening that waited for a check completed within ten minutes
  uses that result, and the schedule does not read again right after another
  check (the button's read is now bounded at 40 seconds, as the schedule's).
  The same rules apply: no check while an update was started on this
  server in the last hour, while the server reports a change in progress, while
  server setup is changing the server, or when the Agent does not answer. A busy
  package manager does not stop it.
- If it cannot check, the offer stays with its age and the reason: a failed
  check keeps its line ("Last automatic check: just now — could not get a
  verified answer from the release server (celikpanel.net)."), a condition not
  met says so ("Not checked again on opening, because an update was started on
  this server less than an hour ago."; "Not checked again on opening: another
  change was in progress on the server."), no answer in time says "The check
  made on opening did not answer in time, so its result is not known.", and an answer that could not be read (the panel answered with an error or an unreadable reply) says "The check made on opening could not be read: the panel answered with an error or an unreadable reply.", each
  followed by "What is shown is from the last check, 13 hours ago; you can check
  now."
- **With automatic checks off, opening the screen contacts nothing.** An offer
  says "Last checked 13 hours ago.", and the next line says "Automatic checks are
  off. Press "Check for updates" to look now."

**A stopped update leaves the card full.** While the notice of an update that
stopped without changing anything (or was verified back) is open, the card shows
the offer, "An earlier update to this version stopped before changing the
installed version" with "The server's package manager was busy, for example with
automatic security updates. Nothing was changed; …", the package manager's state
from the card's own read ("The package manager is still busy. This card checks
again by itself and says when the update can be started; nothing starts by
itself.", then "The package manager has finished. The update can be started
again."), and, at the top of the card where the corner notice does not cover it,
"Close the notice about the stopped update first ("Close notification"); this
button then starts the update to v0.1.0-alpha.102. Nothing starts by itself." The
Update button waits until the notice is closed.

On a phone the readiness box's Retry button is now 44 px tall.

## What does not change

- Nothing installs by itself; the screen, the schedule and every re-read only
  read. Start still checks the exact target again before anything is installed
  and refuses a changed one.
- The Agent, the updater (`update.sh`, the system update worker, rollback and
  recovery), the update lock in the browser, roles, the schema (44, no
  migration). No native record is made, by the owner's instruction of 2026-10-16.
- The 12-hour schedule, its back-off and its switch.

## Known gaps

- Nothing is measured on an installed server. The opening check has not run
  against a real Agent or the real release server; Go tests fake the Agent and
  the browser batch uses a loopback mock.
- While the stopped update's notice is open, the corner notice still covers part
  of the card on a wide screen; the sentence that says what to do is at the top
  of the card, where it stays visible.
- While a stopped update's notice is open no check is made on opening; once it is
  closed the card checks if the known result is older than ten minutes. Within an
  hour of the start the Panel answers that check as skipped ("an update was
  started on this server less than an hour ago"); "Check for updates" reads the
  release at once.
- For a failed update whose notice is in an unknown, recovery or attention state,
  the card still shows only the version and the switch while the sidebar says
  "Update available". Holding Start there is right; the card saying nothing is a
  gap for a later release.
- The opening check is a GET that reads the release and writes the stored result.
  A signed-in administrator's browser can be sent to it by a top-level link from
  another site (the session cookie is SameSite=Lax); it is bounded to one
  completed read per ten minutes, the same exposure as the existing "Check for
  updates" read.

## Evidence

On the candidate tree:

- Go: the opening check (a stale result is checked again, a fresh one within ten minutes is not, automatic checks off make no Agent call, no check while an update was admitted or the host lock is held, a busy package manager does not hold it, a failed check keeps the offer, an unanswered Agent is a skip, a request that ends while waiting for the gate keeps nothing, a completed opening check is kept when the browser leaves, one release check at a time across the button, the openings and the schedule, a scheduled check that waited reads its conditions again (no read right after a completed check, a skip for a start admitted meanwhile), the button's read bounded at 40 seconds, administrators only, readable while the licence is locked); passed on the final Go tree in the `CelikPanel-S2-Debian` guest (the Panel and the internal packages, 45-minute limit, CGO off): 7,229 passed, 44 skipped, 0 failed (2,973 top-level tests), no package failed; gofmt and go vet clean
- Web: 1,757 tests passed, 0 failed, including mounted tests of a stale stored
  offer and a newer release (waiting, then the newest version offered and the
  sidebar following it), a fresh result (no check), automatic checks off (no
  request, the age shown), a failed, skipped, unanswered or unreadable check (the
  offer kept with the reason; an HTTP error or unreadable reply said apart from no
  answer in time), no check while an update is followed, Boston's stopped state
  (the offer, the earlier attempt with its cause, "still busy" and "has finished",
  Start waiting for the notice, the sidebar saying the same version), the check
  running once the stopped update's notice is closed (and not when the result is
  recent), and the attempt drawn from the open notice carrying the recorded cause;
  the production build passed within its bundle budgets (no limit raised).
- Browser inspection batch 26 (loopback mock; Frankfurt's stale offer and
  Boston's stopped-busy state; English and Turkish, light and dark, 1440 and 390
  px; 40 states, and after the review the closed-notice state at 1440 EN light
  and 390 TR dark, where the opening check ran and Start opened): looked once, fixed (the "close the notice first" sentence moved
  to the top of the card because the corner notice covered it, the version kept
  whole in it, Retry 44 px on a phone), looked again; no raw keys, no English on
  the Turkish page, no sideways scrolling, no phone target under 44 px.
- Shell: in a fresh clone in the `archlinux` guest the offline release-recovery
  harness passed (1,109 tests, 1 skipped; its mirrored outcome texts and logic
  did not change, so nothing in it needed updating; it was run before the review round, which changed none of them either), and the release sequence
  policy ("102 -> 103; historical bootstrap sequence 102"), download portal and
  signed release manifest contracts passed.

## How to update

Once published: from the panel's own update screen (Settings → CelikPanel updates),
by the owner of the server. Nothing needs to be done before or after.
