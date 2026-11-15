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
   already help this update. If it still stops with "the server's package
   manager was busy", the server really was installing or checking packages
   for longer: press **Update** again when it has finished.
3. **If the update page of your current version is stuck** (it says the
   update failed and the owner must inspect the update log, the page stays
   locked, and there is no update button), see "If an earlier update left the
   page locked" below before you update.
4. There is nothing to press or set after the update.

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
says so as a stop you can close: "the server's recovery tool could not
confirm its own preparation for the new version, so the update stopped
before stopping any service. Nothing was changed.", with one command that
only reads, `sudo /usr/libexec/celikpanel/recovery runtime-status`, which
says whether anything of that preparation is left and what to do. Before,
this page stayed locked.

## If an earlier update left the page locked

This can happen on alpha.136 and earlier when a package installation
started right after the update was pressed. The installed version and the
data were not changed. The lock is kept by the browser that followed the
update, not by the server:

1. Open the panel in another browser or a private window and sign in. The
   update card shows the earlier attempt as failed and offers the update
   again. (In the same browser, removing the panel site's stored data does
   the same.)
2. If you want to be sure first, run on the server
   `sudo /usr/libexec/celikpanel/recovery runtime-status`. It only reads.
   "No recovery kit transition is recorded" means nothing is left: update
   from the panel. If it says the replacement kit is prepared, updating from
   the panel again finishes that check. If it names an installed launcher or
   a selected kit, run `sudo /usr/libexec/celikpanel/recovery recover`
   first, as it says.
3. Optional: `sudo systemctl reset-failed 'celikpanel-self-update-*'`
   removes the "failed" mark of that attempt's service. It changes nothing
   else; alpha.137 does this by itself for such attempts.

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
- **Pages already locked on an earlier version** are unlocked from another
  browser, as described above; the server cannot unlock a browser.

## Also in this release

- **The disk-usage worker uses one clock.** It stamped its measurements
  with one clock and decided what was due with another, and it measured
  its time budget against the second one. Five of CelikPanel's own tests
  started failing on every machine from 15 November 2026 12:00 because of
  it (PR #328). Its tests no longer depend on the calendar date. Nothing
  changes on a server: there both clocks are the same.

## Checked in the lab

*To be filled in after the lab check of the release candidate.*

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
languages. The full Go test packages were compared with alpha.136: no new
failure. **No P0 item is closed.**
