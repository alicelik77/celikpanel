# v0.1.0-alpha.109

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.109.tr.md)

This is the release that follows v0.1.0-alpha.108. **It is a candidate, written on
2026-10-26; it is not published and nothing from it is on an installed server.
Only the owner updates installed panels, from the panel's own update screen.**
Written on the branch `fix/alpha109-webmail-update` from `main` after the
publication record of alpha.108 (the alpha.108 release commit is
`f99ae9f3ec045ea45c7eada690d052c0767848f5`).

## Who should update, and why

Every server with webmail, and every server where installs are confirmed. On
Frankfurt the Roundcube update to 1.6.20 failed and was put back, the lock then
read "another CelikPanel operation is still running", the row offered "Update
available" again without saying what had happened, and webmail opened with plain
inputs. The owner also did not know how to open webmail, and found the install
confirmation confusing as soon as it named more than one package. What the owner
does on each server is in the
[operation guidance](OPERATION-GUIDANCE.md#frankfurts-webmail-update-the-webmail-link-and-install-confirmations-2026-10-26).

## Webmail updates that work on servers set up by older releases

- **The cause on Frankfurt.** Its webmail web-server configuration was written by an
  older release and has no location for CelikPanel's PHP check, so the proof after
  the swap read HTTP 404 and a working 1.6.20 was rolled back (correctly, byte for
  byte). Before an update, and before any PHP tool's proof, the check and its
  location are now put in place: only in a file in CelikPanel's own form, keeping
  its listen address, roots and sockets; nginx checks it and reloads, and a refusal
  puts the previous file back. A file the owner changed is not touched; the update
  stops before Roundcube changes and says who acts and what to do.
- **The update runs on the server, not in the browser.** Confirming records an
  operation and returns at once; the update goes on if the page is closed or the
  connection drops; the screen follows it by reading, re-attaches after a reload,
  and never sends a second update. A panel restart during an update is reconciled
  read only from the installed version.
- **The outcome stays said.** The Roundcube row and its Manage card say what the
  last update did until the server runs that release: updated, failed and put back
  with the cause, not changed (with the reason), put back but not answering, could
  not be put back, interrupted, or not known, each with who acts and the next step.
- **The proof also loads webmail's stylesheet** and requires it as CSS, so a webmail
  that would open without its layout never passes.

## Webmail opens with its interface

Webmail behind the panel received the panel's own security policy, which allows
scripts only from files and forbids frames. Roundcube starts itself with an inline
script and shows messages in same-origin frames, so its interface never started:
plain inputs on the login page and, after signing in, no working mailbox. This was
not caused by the rollback; it was so since webmail was served through the panel.
Webmail's answers now carry their own policy (frames only from the same origin,
Roundcube's own `X-Frame-Options`); nosniff, the referrer policy and HSTS stay.

## Open webmail

The Roundcube row and its Manage page show **Open webmail** with the full address
while webmail answers there, and say that mailbox users sign in with their email
address and password and need no panel account. The domain's mail screen says the
same address.

## Install confirmations people can read

Every install dialog (a component, the versions list, a bundle, Repair) now says,
one package per line: **Packages that will be replaced** (the distribution's version
and the repository's version, why, and where its updates come from afterwards) and
**Additional packages** under their plain reason; then, when the distribution has
its own version, how to choose it instead. Versions are shown short (`2:93`, not
`2:93ubuntu2`); the exact versions stay in the tooltip and in what the server
checks. Refusals that list packages use the same structure.

## Upgrade and recovery

No schema change (schema 46): the update uses the existing service operations
table (kind `webmail_update`). Recovery to alpha.108 is the ordinary one; an
alpha.108 Panel shows a finished `webmail_update` row as a past operation. A
webmail configuration rewritten to today's form stays CelikPanel's own form for
alpha.108.

## Evidence

- Agent 4827 passed, 29 skipped (as root in a clean mount namespace); the Agent
  package also passes as the unprivileged user `nobody`, as CI runs it; Panel 3448
  passed, 6 skipped; every internal package passes; web 1920 passed, and the
  production build within its budgets. The release-recovery harness is unchanged
  (schema 46) and is not part of this candidate's evidence. The race shards could not be run locally (no C
  compiler in that guest); CI runs them.
- One real-system run on a disposable Debian 13 VM from the kept base image, under
  the Agent unit's umask: Frankfurt's HTTP 404 reproduced word for word from the
  older configuration; an update with an injected failed proof put 1.6.15 back and
  it answered; the real update to 1.6.20 proved through the check, the page and its
  stylesheet (`text/css`) ([evidence](../deploy/e2e/d038/alpha109-debian13-20261026/README.md), archive SHA-256 `e77d1ab4…960f7ee`).
- A browser look (EN and TR, 1440 and 390, light) on a mock (batch 32): the grouped
  confirmation, the webmail row and card; no sideways scrolling, no raw keys.

Not measured on an installed server. A candidate is not acceptance; P0.1–P0.5 stay
**partial**; no P0 item is closed or advanced.
