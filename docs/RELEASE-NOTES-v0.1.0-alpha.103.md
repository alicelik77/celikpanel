# v0.1.0-alpha.103 (published 2026-10-23)

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.103.tr.md)

This is the release that follows v0.1.0-alpha.102. It was published on 2026-10-23
(see "Publication record" at the end); nothing from it is on an installed server
yet. Written on 2026-10-23, as a candidate, on the branch
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

## Publication record

Everything below is dated 2026-10-23 (UTC unless a time zone is named). The release
commit carries 2026-10-23T16:23:04+03:00, which is the same instant as 13:23:04Z.
The merge, the checks, the portal publication and the read of the public site
were done by the release preparer under the owner's standing instruction, as for
the earlier releases; installed-panel updates are the owner's alone, from the
panel's own update screen. No native record for the publication, by the owner's
instruction. This entry stores none of the results below as a file in the
repository.

**What happened.**

1. Before the pull request, one independent review of the candidate found one
   must-fix (an open stop notice disabled the opening check across reloads) and
   smaller items; all were fixed. The gaps listed under "Known gaps" above
   stay recorded.
2. Pull request #244, "Release v0.1.0-alpha.103: the update screen checks for the
   newest release, and a stopped update keeps the card", was merged into `main`
   as commit `f780d5f08d0f295fbf7d249a853d9c130cd28779`, authored by
   Mehmet Ömer Efe Çelik. The manifest's `published_at` is 2026-10-23T13:23:04Z.
3. The release is a pre-release, `v0.1.0-alpha.103`, published on GitHub at
   2026-10-23T14:08:46Z.
4. Checked by the preparer before the publication (not recorded in a file here):
   the archives are 66,351,430 bytes, SHA-256
   `8a065499b59764cbefbdbe1dc16cfe754b240d865f00191484f75ec2e4ae9a00`; the
   checksums, the manifest and the archive members were checked; the manifest's
   signature verifies with the release key (SHA-256 `7eadeb0b…c68bb`); both content
   guards passed; `libexec/get.sh` pins 103; `install.sh` is the tag's; the policy
   inside says 103 after 102; the Panel binary carries the new opening-check
   route; the interface bundle is `app-DVVjTxvw.js`.
5. The portal package was built twice and came out byte-identical; the promoter's
   own extract and normalize step accepted it.
6. At about 14:17Z the preparer published the portal: status `committed`, public
   verification 26 of 26 requests ok.
7. A read from outside after the publication: `https://celikpanel.net/releases/latest.txt`
   says v0.1.0-alpha.103; `latest.json` says sequence 103 with commit `f780d5f…`.

**What the owner reported.** From the owner's screenshots of 2026-10-23: Frankfurt
was updated by the owner from alpha.100 to alpha.102 successfully (after the stale
offer of alpha.101 that led to this release). Boston's update to alpha.102 stopped
unchanged because the package manager was busy; Boston still runs alpha.100 as far
as is known. No installed server is known to run alpha.103, and nothing from
alpha.103 had been reported when this was written.

**Not done, and what publication does not change.**

- The installed servers were not updated to alpha.103 by the release preparer.
  Only the owner updates them, from the panel's own update screen. alpha.103 is
  on no installed server yet.
- Nothing from alpha.103 has been measured on an installed server or against the
  real release server. The owner's observation after he updates is still to come,
  including Boston's retry.
- The gaps listed under "Known gaps" above stay open for a later release.
- **Upcoming acceptance, not a claim:** at Beta 1 the owner will wipe and
  reinstall both servers for a clean first-install test.
- Whether the bundles move to a data catalogue is left to the owner (see the
  [alpha.101 notes](RELEASE-NOTES-v0.1.0-alpha.101.md), "Known gaps").
- Not reported and not recorded: anything from Boston or Frankfurt on alpha.103,
  and everything the earlier notes list as not reported. Each is "not recorded",
  not "worked" and not "failed". What the alpha.102 record leaves open stays open.
- The limits of the earlier publication records (see the [alpha.88
  notes](RELEASE-NOTES-v0.1.0-alpha.88.md)) are not reported as lifted.
- No code, schema (44) or recovery behaviour changed in this record.
- Publication is not acceptance. P0.1 to P0.5 stay partial; no P0 item is closed
  or advanced.
