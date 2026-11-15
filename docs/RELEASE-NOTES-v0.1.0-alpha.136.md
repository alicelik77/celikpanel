# v0.1.0-alpha.136

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.136.tr.md)

A resilience fix that ships alone. Before the panel starts certain work, it
asks "is something changing this server right now?". On a busy server that
check could answer "no" while a change was in fact running. It now looks at
every process before it answers.
**Only the owner updates installed panels, from the panel's own update
screen.**

## Updating from alpha.135

1. Update from the panel's own update screen, as before. This release does
   not change the panel's database layout (it stays at the alpha.135 form),
   so going back to alpha.135 later needs no database restore.
2. There is nothing to press or set after the update. The change is inside
   the check; no screen changes.

## What changes for you

### What was wrong

Before an update, before the recovery tool is switched to a new version,
when a page asks whether the server is ready for a change, and before mail
setup is handed to its worker, the panel checks whether another change
already holds the server's change lock.

To answer, it read the system's list of all file locks. On a server where
many programs take and drop locks all the time (databases, mail, other
services), that list can be read in a moment when a line is skipped. If
the skipped line was the one for the running change, the check answered
"nothing is changing" although something was. This defect was in every
release since alpha.113, when the check stopped taking the lock for an
instant and began reading the list. It was found because one of
CelikPanel's own automatic
checks failed now and then for this reason.

### What it could cause

The check answering "free" does not itself change anything, and most
changes then try to take the lock themselves and stop if it is held. But
some steps rely on the check alone:

- an update's first check could let the update continue while an Agent
  operation was still running, and the update then waited or stopped
  later;
- the switch of the recovery tool to its new version during an update
  relies on this check (see "Known limits" for what is still open there);
- the record of who held the lock could say the holder had "released it
  before it could be named" while it still held it.

### What changes

- When the lock list names a holder, the answer is "busy", as before.
- When the lock list names nobody, the panel now also looks at the open
  files of every process before it says "free". This read does not take
  the lock, so it cannot itself make a real change wait. It takes about
  half a second on a server with four thousand processes and stops after
  five seconds at most.
- If that full read cannot be made, for example on a system that does not
  give the panel the right to look at other processes, the panel answers
  as releases before alpha.113 did: it takes the lock for an instant and
  lets it go. The answer is still exact. See "Known limits".
- The record of who held the lock says "released it before it could be
  named" only when the full read found nobody; otherwise it says the
  holder could not be established.

### What you see

Nothing new. When something really is running, an update waits for it up
to 60 seconds with the message that the Agent is finishing another server
operation, and if it is still running then, it stops before changing
anything with the existing explanation and "Starting the update again is
safe". These messages are unchanged; they are now shown only when they are
true.

## Known limits

- **A holder in another process namespace.** A program that holds the lock
  from a separate process namespace, which the panel cannot see, is not
  found by either read. CelikPanel's own programs run where the panel sees
  them.
- **A lock kept without an open file.** A lock kept alive by a program
  that no longer has the file open (a rare programming technique) is
  visible only in the lock list, as before.
- **Systems where the full read cannot be made.** There, the check takes
  the lock for an instant. A real change that starts in exactly that
  instant is answered "busy" and waits or is retried, and two checks at
  the same instant can make one of them answer "busy". Neither can make the
  check answer "free" while the lock is held. CelikPanel's Agent runs with
  the rights the full read needs.
- **Open for its own change: the recovery tool's switch does not hold the
  lock.** When an update switches the recovery tool to its new version, it
  checks that nothing is changing but does not hold the change lock while
  it switches. A panel operation that starts in between (a DNS engine
  switch, a mail setup, a firewall change) is not stopped. A later recovery
  could then meet records the new recovery tool was never checked against,
  and refuse. This release makes the check itself exact; the gap between
  the check and the switch stays and is planned as its own change.
- A recovery check that cannot even be started is still reported as
  "unsupported protocol" (listed for alpha.137).

## Checked in the lab

**To be filled in after the lab check.** No lab run has been made on this
release yet.

## Evidence

No schema change: schema 079, as in alpha.135. No persisted record
changes. The change is in `internal/hostmutationlock`
(`descriptors_linux.go`, `holders_linux.go`, `lock_linux.go`) and one
comment in `cmd/agent/mail_enrollment_worker_linux.go`.
Component tests: `internal/hostmutationlock/descriptors_linux_test.go`
(a holder missing from the lock list is found; an incomplete read decides
with the momentary lock; no capability still answers exactly; what a
writer meets inside the momentary lock) and a regression test that takes
and drops locks on purpose while every check must name the holder. On the
code of alpha.135 that test failed 20 of 20 runs as root and as an
unprivileged user, and 5 of 5 with the race detector; on this release it
passed every run. CelikPanel's existing test of naming another process,
unchanged, failed 47 and 38 of 300 runs under the same kind of load on
alpha.135 and 0 on this release. A new CI step runs these tests as root.
No run on a real server yet (see "Checked in the lab").
**No P0 item is closed.**
