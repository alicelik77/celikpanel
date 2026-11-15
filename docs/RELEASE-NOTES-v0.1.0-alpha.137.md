# v0.1.0-alpha.137

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.137.tr.md)

A small release of two fixes found in the alpha.136 lab. On Ubuntu, the
first press of the panel's update could stop with "the server's package
manager was busy" although nothing was really changing packages. Now the
update waits for a short package run, at most a minute, says what it is
waiting for, and continues by itself.
**Only the owner updates installed panels, from the panel's own update
screen.**

## Updating from alpha.136

1. Update from the panel's own update screen, as before. This release does
   not change the panel's database layout (it stays at the alpha.136 form),
   so going back to alpha.136 later needs no database restore.
2. The fix is in the update that installs alpha.137 itself, so it already
   helps this update. If the update still stops with "the server's package
   manager was busy", the server really was installing or checking
   packages for more than a minute: press **Update** again when it has
   finished.
3. There is nothing to press or set after the update.

## What changes for you

### What was wrong

- **The first press on Ubuntu.** While it prepares, the update reloads the
  server's service definitions (systemd) a few times. On Ubuntu and Debian,
  such a reload can start the system's daily package check at once
  (`apt-daily`: it refreshes the package lists and, on Ubuntu, starts two
  small helpers of Ubuntu Pro, the news and the ESM cache services). The
  update's check for package activity did not recognise the daily check's
  first steps, only the `apt-get` it runs a few seconds later. The update
  does wait, up to a minute, for a package run, but that wait came after
  another check that repeats the same question and never waits. So the
  update stopped at once, before changing anything, and a second press a
  few minutes later worked.
- **About one check in a thousand.** When a program ended at exactly the
  moment the package check read its name, the check refused ("no such
  process") instead of answering, and an update could stop at once for
  that reason. It stopped safely, before changing anything.

### What changes

- The update's waiting check comes first, before any check that never
  waits, and its message names what it waits for: the server's package
  manager or another server operation of the panel.
- The daily package check is recognised from its first step
  (`apt.systemd.daily` and `apt-helper`), so the update waits for the whole
  run instead of meeting it half-way.
- An update still never proceeds while packages are really being installed
  or the package manager holds its lock, automatic security updates
  included. Every later check before the update changes anything still
  refuses package activity; only the first one waits.
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
  (en çok bir dakika), sonra kendiliğinden sürer ya da hiçbir şeyi
  değiştirmeden durur. Bir şey yapmanız gerekmiyor."
- EN: "If another server change or the server's package manager (for
  example automatic updates) is still working, the update waits for it (up
  to a minute), then continues by itself or stops without changing
  anything. Nothing needs to be done."

The update's own log (the worker log on the server) says when it waits
for the package manager and when it continues:

- "==> The server's package manager is working (for example automatic
  updates); waiting up to 60 s for it before the update continues (nothing
  needs to be done)" / "==> Sunucunun paket yöneticisi çalışıyor (örneğin
  otomatik güncellemeler); güncelleme devam etmeden önce en çok 60 sn
  bekleniyor (bir şey yapmanız gerekmiyor)"
- "==> The package manager finished after about N s; the update continues"
  / "==> Paket yöneticisi işini yaklaşık N sn sonra bitirdi; güncelleme
  sürüyor"

If the package manager is still working after the minute, the update
stops with the same words as before ("the server's package manager was
busy, for example with automatic security updates. Nothing was changed."),
the version is unchanged, and you start the update again when it has
finished.

## Also in this release

- **The disk-usage worker uses one clock.** It stamped its measurements
  with one clock and decided what was due with another, and it measured
  its time budget against the second one. Five of CelikPanel's own tests
  started failing on every machine from 15 November 2026 12:00 because of
  it (PR #328). Its tests no longer depend on the calendar date. Nothing
  changes on a server: there both clocks are the same.

## Known limits

- **A package run that starts in the last seconds.** If the daily package
  check (or any package run) starts in the few seconds between the wait
  and the update's final check, the update still stops unchanged, without
  waiting. Press **Update** again when it has finished.
- **No live "waiting for …" line in the window.** The window shows the
  standing note above for the whole check step; the exact moment the
  update waits, and for what, is only in the update's log. A live line
  needs a new channel from the update to the panel.
- **What was not established about Ubuntu.** The chain (a reload starts
  `apt-daily`, whose `apt-get update` starts Ubuntu Pro's news and ESM
  cache services) is established from systemd's, apt's and Ubuntu Pro's
  sources, not measured on a server. Which of the update's checks stopped
  the alpha.136 lab's first press is not known (the stop reports only
  "package manager busy"), and whether `apt-daily` or the lab's clock jump
  started that run is not known. The names the daily check's programs
  carry on Ubuntu 24.04 were not read on a server.

## Checked in the lab

*To be filled in after the lab check of the release candidate.*

## Evidence

No schema change: schema 079, as in alpha.136. No persisted record
changes. The change is in `cmd/agent/service_mutation_lock_linux.go` (the
package scan), `update.sh` (the order and wording of the wait),
`cmd/panel/disk_usage.go` (one clock, PR #328) and the update window's
note (`web/src/i18n`). Component tests: the package scan with a fake
`/proc` (a process ending during the read, the daily check's first steps,
Ubuntu Pro's readers not counted, other read errors still refusing); the
shell contract test of the update's first check (a package run is waited
for at most 60 s with its own lines, the wait comes before the BIND
check); the disk-usage tests with the clock a year before and a year after
today. The full Go test packages were compared with alpha.136: no new
failure. No native update run yet. **No P0 item is closed.**
