# v0.1.0-alpha.137

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.137.tr.md)

A small release about one thing: an update that meets the server's package
manager at work. On Ubuntu the first press of the panel's update could stop
with "the server's package manager was busy" although nothing was really
changing packages; and when a real package installation started just after
the press, the update stopped in a way that left the update page locked with
no way to start again. Now the update waits for a package run, at most a
minute at each of its checks, and if it is still running the update stops
unchanged and can be started again at once.
**Only the owner updates installed panels, from the panel's own update
screen.**

## Updating from alpha.136

1. Update from the panel's own update screen, as before. This release does
   not change the panel's database layout (it stays at the alpha.136 form),
   so going back to alpha.136 later needs no database restore.
2. The fixes are in the update that installs alpha.137 itself, so they
   already help this update: where it waits, the package check (the update
   runs the new version's own checks from the downloaded release, as the lab
   saw), and that a busy server ends as a stop you can start again. Only the
   panel's readiness check before you press (the update card that keeps the
   button off while packages are being installed) is still the installed
   version's until the update finishes. If the update still stops with "the
   server's package manager was busy", the server really was installing or
   checking packages for longer: press **Update** again when it has
   finished.
3. **If the update page of your current version is stuck** (it says the
   update failed and the owner must inspect the update log, the page stays
   locked, and there is no update button), see "If an earlier update left the
   page locked" below before you update.
4. There is nothing to press or set after the update. The card shows the new
   version; an earlier attempt's "failed" service mark is cleared by itself.
   From now on the card's readiness check before the press follows the new
   package check too (step 2), including the hardening below.
5. **If an update ever stops with "the server's recovery tool could not
   confirm its own preparation"** (rare): run the read-only command the
   window shows and do what its answer says, usually start the update again.
   The server's own recovery service may finish the recovery tool's switch
   by itself as soon as the package manager is free; it then logs one
   alarming line ("the selected recovery entry could not be verified …
   runtime_changed") and is marked failed for about half a minute until its
   next run succeeds. If the command then says the transition is verified
   complete, that line needs nothing from you.

## What changes for you

### What was wrong

- **The first press on Ubuntu.** While it prepares, the update reloads the
  server's service definitions (systemd) a few times. On Ubuntu and Debian,
  such a reload can start the system's daily package check at once
  (`apt-daily`: it refreshes the package lists and, on Ubuntu, starts two
  small helpers of Ubuntu Pro, the news and the ESM cache services). The
  update's check for package activity did not recognise the daily check's
  first steps, only the `apt-get` it runs a few seconds later; and the
  update's one-minute wait came after another check that asks the same
  question and never waits. So the update stopped at once, before changing
  anything, and a second press a few minutes later worked.
- **A package installation that starts just after the press** (found in the
  alpha.137 lab, on Debian 13). Before the update stops anything, the
  server's recovery tool prepares its own new version and checks that the
  server is idle. That check waited a minute for the package manager, and
  then the update stopped with "independent recovery runtime preparation is
  unconfirmed" instead of the usual "the package manager was busy". Nothing
  had been changed, but the update page took it for a failure that the
  server's recovery would still handle: it stayed locked, offered no update
  button, and the attempt's service stayed marked as failed.
- **About one check in a thousand.** When a program ended at exactly the
  moment the package check read its name, the check refused ("no such
  process") instead of answering, and an update could stop at once. It
  stopped safely, before changing anything.

### What changes

- The update now waits first, before it prepares anything: if the package
  manager or another server operation of the panel is working, it waits for
  it, at most a minute, and then continues. It waits once more after
  reloading the service definitions, before its other checks.
- If the recovery tool's own check still finds the server busy after its
  wait, before it has selected anything, the update now stops with the
  usual "the package manager was busy" (or "another server operation was
  still running"): nothing changed, the update can be started again at once,
  and the attempt's service is not left marked as failed. Any other problem
  of that preparation keeps its existing, careful report.
- The daily package check is recognised from its first step
  (`apt.systemd.daily` and `apt-helper`), so the update waits for the whole
  run instead of meeting it half-way.
- An update still never proceeds while packages are really being installed
  or the package manager holds its lock, automatic security updates
  included. Every later check before the update changes anything still
  refuses package activity; only the first ones wait.
- A program that ends while the package check reads it is treated as gone,
  not as a reason to refuse.
- Ubuntu Pro's news and ESM cache services are not counted as package
  activity: they take none of the system's package locks.
- **Hardening:** a process that only carries the package manager's name,
  run by an ordinary account, no longer holds an update back. On a hosting
  server any account can start a program called `apt` or `dpkg`; only a
  process that runs as root (the only user that can change the system's
  packages) is counted now. A process whose owner cannot be read is still
  counted.
- A retry after such a stop no longer leaves a copy of the recovery kit
  behind.

### What you see

In the update window, under the step **Kurmadan önce sunucuyu denetleme**
(Check the server before installing), the note now names the package
manager:

- TR: "Başka bir sunucu değişikliği ya da sunucunun paket yöneticisi
  (örneğin otomatik güncellemeler) hâlâ çalışıyorsa güncelleme onu bekler
  (her denetimde en çok bir dakika), sonra kendiliğinden sürer ya da hiçbir
  şeyi değiştirmeden durur. Bir şey yapmanız gerekmiyor."
- EN: "If another server change or the server's package manager (for
  example automatic updates) is still working, the update waits for it (up
  to a minute at each check), then continues by itself or stops without
  changing anything. Nothing needs to be done."

This note is shown for the whole check step. The window does not show the
moment the update starts or ends a wait: nothing in the window or in the
server's logs says it today (see "Known limits").

If the package manager is still working after the wait, the update stops
with the same words as before ("the server's package manager was busy, for
example with automatic security updates. Nothing was changed."), the
version is unchanged, the update button is there again, and you start the
update again when it has finished.

If the recovery tool's preparation stops for another reason, the window now
shows it as a stop you can close, instead of staying locked. It says:

> "The update to v0.1.0-alpha.137 stopped before the installed version was
> changed or any service was stopped. The server is running the current
> version as before."
>
> "Reason: the server's recovery tool could not confirm its own preparation
> for the new version, so the update stopped before stopping any service.
> The installed version and its data were not changed; the recovery tool's
> own preparation for the new version may be partly done. The server owner
> runs the command below once on the server and does what its answer says.
> Nothing starts by itself."
>
> `sudo /usr/libexec/celikpanel/recovery runtime-status --lang en`
>
> "This command only reads; it changes nothing."
>
> "If it says no recovery kit transition is recorded, that the replacement
> kit is prepared, or that the transition is verified complete: start the
> update to v0.1.0-alpha.137 again; it repeats the same check first. If it
> says the replacement launcher is installed or the replacement kit is
> selected: run sudo /usr/libexec/celikpanel/recovery recover, as it says,
> then start the update again. If it says the transition evidence could not
> be verified: keep the recovery files and this update's log unchanged and
> do not start the update; the evidence problem named in brackets at the
> end of its answer is resolved first. Until then the server runs the current
> version as before."

On a Turkish page the command is the same with `--lang tr`, and the tool
answers in Turkish. After the window is closed, the update card says the
same of that attempt ("stopped before changing the installed version or
its data", with the same command), not "did not complete".

## If an earlier update left the page locked

This can happen on alpha.136 and earlier when a package installation
started right after the update was pressed. The installed version and its
data were not changed; the recovery tool's own preparation may be partly
done (step 2 says what is left).

The lock is kept by the browser that followed the update (its stored record
of the operation), not by the server. The lab confirmed the way out on a
server left stuck this way by an earlier build:

1. Open the panel in another browser or a private window and sign in. In
   that fresh session the page was not locked, "Check for updates" worked,
   the server was "ready for the update" and the update was offered;
   pressing it updated the server, and the new version cleared the "failed"
   mark of the earlier attempt by itself. The fresh session did not show a
   notice about the earlier attempt. That the original browser stays locked
   was not checked again (its profile no longer existed); in that browser,
   removing the panel site's stored data should have the same effect.

2. Before you update, run on the server
   `sudo /usr/libexec/celikpanel/recovery runtime-status --lang en` once and
   do what its answer says, as described for the window above: no
   transition recorded, the replacement kit prepared, or the transition
   verified complete: update from the panel; the replacement launcher
   installed or the replacement kit selected: run
   `sudo /usr/libexec/celikpanel/recovery recover` first; the evidence could
   not be verified: keep the files and the update log and do not update yet.
3. Nothing else is needed. When the update to alpha.137 succeeds, the
   panel removes the "failed" mark of that earlier attempt's service by
   itself.

## Known limits

- **A package run that starts in the last seconds.** If a package run
  starts in the few seconds between the update's waits and its final checks
  (or while the recovery tool is switching to its new version), the update
  still stops without waiting; it stops unchanged and can be started again.
- **No live "waiting for …" line.** The update's own messages, including
  the lines it writes when it waits for the package manager, are not kept
  anywhere the owner can read them: the panel keeps only the last line of a
  failure. Showing a live waiting line needs a new way for the update to
  tell the panel what it is doing, which is not in this release; the window
  keeps the standing note above.
- **What was not established about Ubuntu.** The chain (a reload starts
  `apt-daily`, whose `apt-get update` starts Ubuntu Pro's news and ESM cache
  services) is established from systemd's, apt's and Ubuntu Pro's sources
  (what the lab saw is in "Checked in the lab"). Which of the update's
  checks stopped the alpha.136 lab's first press is not known.
- **Pages already locked on an earlier version** are opened from another
  browser or a private window, as described above (confirmed in the lab);
  the server cannot unlock a browser.
- **Copies left by earlier retries.** Each earlier retry of the same
  version after a stop of the recovery tool's preparation could leave one
  copy of the recovery kit in a hidden `.promote-…` folder under
  `/usr/libexec/celikpanel/recovery-runtimes/v1/`. alpha.137 no longer leaves
  them; nothing removes the ones already there yet (listed for alpha.139).
  They are inert: nothing selects or runs them.

## Also in this release

- **The disk-usage worker uses one clock.** It stamped its measurements
  with one clock and decided what was due with another, and it measured
  its time budget against the second one. Five of CelikPanel's own tests
  started failing on every machine from 15 November 2026 12:00 because of
  it (PR #328). Its tests no longer depend on the calendar date. Nothing
  changes on a server: there both clocks are the same.
- **CelikPanel's own race tests have room again.** One group of them
  reached its eight-minute limit on slower test machines although no test
  failed; the groups are split further by measured time, and a check lists
  every panel test and fails unless each runs in exactly one group (PR
  #329, measured, merged here). Nothing changes on a server.

## Checked in the lab

Lab checks finished: two runs, on Debian 13 and Ubuntu 24.04 test servers.

**Run 1 (first build of this release).** On Ubuntu the update's own reload
of the service definitions started the daily package check, and the first
press completed the update. 2,000 package checks while programs kept ending
never refused. It also found the defect this release then fixed: a real
package installation that started seven seconds after the press left the
update page locked with no update button.

**Run 2 (the release's final product code).**
- Debian: an update with nothing running completed without waiting.
- Debian: a real package installation that started seven seconds after the
  press. The update waited about a minute, stopped without changing
  anything, the update button came back, the attempt's service was not left
  marked as failed, and the next press with nothing running succeeded.
- The same when the recovery tool's own check met the busy server, pressed
  twice: each time a typed "package manager was busy" stop, nothing left
  behind (no kit copy, no recorded transition).
- The "preparation unconfirmed" stop produced on purpose: the window, the
  read-only command's answer in Turkish and in English, and one retry
  completed the update.
- Ubuntu: the first press completed while the daily package check ran at the
  same time.
- A program merely named like the package manager (`apt`, `dpkg`), run by an
  ordinary account, did not hold the update back (50 of 50 checks idle);
  the same under root did.
- A server left stuck by the earlier build: a fresh browser session offered
  the update, and it succeeded (see "If an earlier update left the page
  locked").
- In every busy case the update never proceeded while the package lock was
  held.

**Not checked:** the update's waiting lines themselves (they are kept
nowhere; the waits were seen by their timing); creating a mailbox in the
final smoke check; that the same browser stays locked on a server stuck by
an earlier version.

### Found in the lab, left for the next release

- After the "preparation unconfirmed" stop, the server's recovery service
  finished the recovery tool's switch by itself once the package lock was
  free, but first logged one alarming line ("the selected recovery entry
  could not be verified … runtime_changed") and was marked failed until its
  next run half a minute later, while the read-only command had just said
  the old launcher cannot finish this by itself.
- A browser that still holds an unclosed notice of an older stopped update
  shows that older attempt on the update card instead of the newer one.
- The update's own waiting lines are kept nowhere the owner can read them.

## Evidence

No schema change: schema 079, as in alpha.136. No persisted record
changes. The changes are in `update.sh` (where and how it waits, and the
typed stop before its exit handler exists), the recovery tool
(`internal/recoveryruntime`, `cmd/recovery`: a busy host before any
transition is named), the Agent's package scan and its rule for clearing
the failed mark of an attempt that changed nothing (`cmd/agent`), the update
window's texts and its reading of the unconfirmed-preparation stop
(`web/src`), and `cmd/panel/disk_usage.go` (one clock, PR #328). Component
tests: the package scan with a fake `/proc`; the recovery tool's checker and
preparation with a busy host before and after its intent; the update's
first checks (a package run waited for, a lasting one stopped unchanged with
its typed line, before the preparation and before the BIND check); the
recovery runtime shell contract (a busy preparation is a typed stop, any
other failure keeps its report); the Agent's clearing rule on the lab's
exact record; the update window on the lab's exact record in both
languages, and the update card after it is
closed; the package scan counting only root-owned package processes (an
ordinary account's same-named process is not counted, an unreadable owner
is); a busy host met twice leaves no kit copy; one contract keeps the
"preparation is unconfirmed" line, the Agent's and the window's readers of
it and the recovery tool's typed reports one text
(`deploy/test-update-preparation-stop-contract.sh`). The full Go test packages were compared with alpha.136: no new
failure. **No P0 item is closed.**
