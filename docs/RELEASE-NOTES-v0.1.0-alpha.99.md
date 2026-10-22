# v0.1.0-alpha.99 (candidate, 2026-10-22)

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.99.tr.md)

This is the release that follows v0.1.0-alpha.98. It is a candidate: when this was
written it had not been tagged or published, and nothing from it was on an installed
server. Written on 2026-10-22, on the branch `feat/alpha99-own-email`, from `main`
after the publication record of alpha.98.

## Who should update, and why

Everyone who runs alpha.98 or an earlier release, once this release is published.
Until now nobody could change their own email address in the panel: an
administrator's address was the one given when the administrator was created on the
server. The panel certificate gives that address to the certificate authority as its
contact, so an administrator without a usable address was stuck: since alpha.98 the
panel said honestly that there was no screen for it yet. This release adds that
screen (D-037 amendment); the owner approved building it on 2026-10-22. Only the
owner of a server updates it, from the panel's own update screen.

## What changes on screen

**My account has a "My email" card.** Open the menu at the top right (your user
name) → My account. Between Password and Two-step sign-in there is a third card, "My
email" ("E-posta adresim"). It shows your current address. While it reads the
address it says so; if the read fails, it says the address could not be read and
offers Retry. The address `/account#email` opens the page at this card.

**Changing it.** "Change email" opens a small dialog: your new address and your current
password. Enter saves, Escape closes. While it saves, the button shows a spinner, the fields
and buttons are locked, and a line says the change is being saved and nothing else
is needed. When it is done you see "Your email address was changed." and the card
shows the new address. You stay signed in: you sign in with your user name, not your
address.

**If it takes long.** The dialog waits at most 20 seconds. After that it says: "It is
not known yet whether your new address was saved. This is not a failure. The address
shown on this page was read again; if it is still the old one, try again." One click
never changes your address twice, even if the connection drops and the answer is
asked for again.

**When something is not right, nothing is changed, and the dialog says what to do:**

- "This is not an email address the panel can use, so nothing was changed. Enter one
  address such as name@example.com; a domain with letters outside English goes in
  its ASCII form (xn--…)."
- "This is already your email address, so nothing was changed."
- "Another account already uses this email address, so nothing was changed. Enter a
  different address."
- "Your current password is not correct, so nothing was changed. Enter the password
  you sign in with."
- "Too many attempts, so nothing was changed. Wait a few minutes, then try again."
- "Your account was changed somewhere else while this was being saved, so nothing
  was changed. Reload the page and try again."
- "The request could not be read, so nothing was changed. Reload the page and try
  again."

**Team members too.** An additional user (team member) has no My account page; they
find "Change email" in the personal menu, beside "Change password". It opens the same
dialog. Administrators, resellers and customers use the card.

**The panel certificate and server setup point to it.** When the administrator has no
usable address, the panel certificate and the server setup plan say: "The
administrator account has no valid email address, which the certificate authority
needs as a contact. Nothing was requested. Set your email on My account (top-right
menu), then try again." with an "Open My account" button that opens the card.
Changing your address does not request or renew any certificate by itself: request
the certificate again when you are ready. A server setup plan you reviewed before
the change must be reviewed again; the new review uses the new address. A setup that
is already running keeps the address it started with.

**Refusal buttons on a phone.** The button beside a refusal that names the place
that fixes it (such as "Open My account") is now at least 44 px tall on a phone,
everywhere in the panel. Browser batch 24 found it 28 px tall.

## What changes underneath

- **One new route,** `PUT /api/v1/auth/email`, beside `POST /api/v1/auth/password`.
  Body `{"email", "current_password"}`, keys spelled exactly so; any other key (a
  differently spelled one, `user_id`, `id`), or more than one JSON object, is
  refused. Open to every role that may change its own password:
  administrator, reseller, customer and additional user.
- **The current password is required.** Every attempt counts against one
  re-authentication limit shared with the password change (`reauth`, per account
  and client address, 10 per 5 minutes): `403 account_email_wrong_password`, then `429
  account_email_too_many_attempts`.
- **One identity per action (D-029).** The request-identity guard now protects this
  route; it is the first `PUT` it guards. The same identity is answered from the
  first arrival (`X-CelikPanel-Request-Replayed: 1`) and never changes the address
  twice; the same identity with another address is `409 REQUEST_ID_REUSED`; a
  request without one is `428 REQUEST_ID_REQUIRED`. Time limit 30 seconds. The digest
  the guard stores is of the new address alone, re-encoded, and the success answer
  carries no address, so the stored rows hold neither the password nor the address.
  After an unknown result the dialog's next Save of the same address carries the
  same identity.
- **One validator.** `mutationpayload.CanonicalContactEmail` is the panel
  certificate's existing contact check, exported and reused: trimmed, printable ASCII
  only, exactly one `@`, a local part of at most 64 bytes and a whole of at most 254,
  a fully qualified domain stored in lower case, no display name. An address
  accepted here is always one the certificate request accepts.
- **One account per address.** An address another account uses is `409
  account_email_taken`, compared without regard to letter case; the `UNIQUE`
  constraint on `users.email` (in the schema since `001_full_schema.sql`) is the
  last word. Whether the address is already yours (`409 account_email_unchanged`) or
  someone else's is told only after the password was proven.
- **A change made elsewhere is not overwritten.** The write is a compare-and-set on
  the verified address and password hash: `409 account_email_changed_elsewhere`.
- **Sessions are kept; the audit trail names the account, not the address.** One
  entry `account.email.changed` on the user; the Dashboard's "Recent activity" says
  "changed their own email address".
- **The panel certificate** reads the signed-in administrator's address from the
  database when it is requested (no cache). Its refusal
  `panel_certificate_contact_email_invalid` carries the action `/account#email`
  again. The server setup plan keeps the contact address it was reviewed with in the
  plan's identity.
- **The licence gate is unchanged.** The email change is not added to the routes a
  licence-locked panel still accepts (the password change is one of them): while the
  panel is licence-locked, it is refused like the other routes. What a locked panel
  accepts is licence policy, outside this release.

## What does not change

- No Agent, updater, schema, polling, component or catalogue-part split change. No
  new index; schema 44, no migration.
- The updater's mutation path (`update.sh`, `get.sh`, the system update worker,
  rollback and recovery) is not touched, so no native record is made, by the owner's
  instruction of 2026-10-16, and the release-recovery harness is not needed: no
  update-related file changed.
- No mail is sent to the old or the new address; no certificate is requested,
  renewed or retried by an email change.
- Hosted websites, mail, DNS, databases, scheduled jobs and certificate renewal do
  not depend on a panel user's address.
- Installed panels are updated only by their owner, from the panel's own update
  screen.

## What the owner reported

On 2026-10-22 the owner reported that both installed servers are on alpha.98: the
Dashboards of Boston and Frankfurt show the new menu. Frankfurt's update from
alpha.94 went through; its earlier alpha.95 update had stopped in the read-only check
on a busy package manager (Debian's automatic package updates, recorded with
alpha.97). This answers two open items of the alpha.98 publication. Frankfurt's
package versions on Components were not reported, so the open item "read installed
package versions for every component" stays open for Frankfurt.

## Security review corrections

An independent security review of the candidate asked for these on 2026-10-22,
before any publication; they are in source.

- **Both contact gates use the one validator.** The panel certificate request and
  the setup plan's contact blocker checked the administrator's address with Go's
  `mail.ParseAddress`, which accepts `admin@localhost`, `root@myhost` or a
  non-ASCII address that the certificate request's own check refuses. Such an
  address (possible, because `--create-admin` does not check the address it is
  given) passed the gate and failed later without the My account guidance. Both
  gates now use `CanonicalContactEmail`: every address the certificate would
  refuse gets the coded refusal with `/account#email`, and the setup blocker.
- **The stored digest holds the new address alone.** Go's JSON decoding matches
  keys without regard to letter case (and folds `ſ` to `s`), so a body with
  `"Current_Password"` reached the password field while the digest removed only the
  exact key `current_password`. The body's keys must now be exactly `email` and
  `current_password`; any other key (`Current_Password`, `user_id`, `id`, …) is
  `400 account_email_request_invalid`. The digest is the new address alone,
  re-encoded; an unreadable body digests to a constant.
- **The success answer carries no address** (the guard keeps answers for 24
  hours); the page reads the address again.
- **One re-authentication limit.** The email change and the password change share
  one limiter key (`reauth`) per user and client address, so a stolen session gets
  one budget of guesses across both (10 per 5 minutes), not one per route.
- **After an unknown result, the next Save of the same address carries the same
  identity** until a definite answer arrives; another address is a new action.
- **The refusal for an unusable address** says that a domain with letters outside
  English goes in its ASCII form (`xn--…`).
- Tests added: both gates with `admin@localhost`, `root@myhost` and a non-ASCII
  address; bodies with `Current_Password`, `current_paſsword`, `user_id` and `id`;
  a wrong password told before "taken" and "already yours"; an administrator
  impersonating a customer refused without the customer's password; one identity
  with another password answered from the stored answer; the limit shared by both
  routes; the same identity after an unknown result (web).

Known limits, recorded: two simultaneous changes to addresses that differ only in
letter case can both pass the case-insensitive check, because the schema's `UNIQUE`
constraint compares exactly; no case-insensitive index is added in this release.
The certificate authority may still refuse an address at a domain that is not
public (`.test`, `.local`, `.internal`, `example.com`): the panel checks the form,
the authority judges the domain. `--create-admin` still does not check the address
it is given; the panel's gates now catch it.

## Known gaps

- **Nothing was measured on an installed server.** The email change has been
  exercised by component, mounted and mock-browser tests only.
- **The new address is not confirmed by a message.** The panel sends no mail to the
  new address; the person proves who they are with their password.
- **While the panel is licence-locked**, the email change is refused like the other
  routes (the password change stays reachable).
- **"Open sessions"** is still not built; the product has no session list.
- **alpha.99 back to alpha.98.** An address changed on alpha.99 stays; alpha.98 has
  no screen to change it again and its certificate refusal says so. Nothing is lost
  and nothing needs repairing.
- **Frankfurt's package versions** were not reported.
- P0.1-P0.5 stay **partial**; no P0 item is closed or advanced; a candidate is not
  acceptance.

## Evidence

Everything below was run on the candidate tree on 2026-10-22 in a development
environment; none of it was run on an installed server.

- Go: `go test ./cmd/panel/ ./internal/...` (-timeout 45m, CGO off) in the `CelikPanel-S2-Debian` guest on the candidate tree: 64 packages ok, 2894 top-level tests passed, 42 skipped, 0 failed (4279 subtests passed); gofmt and go vet clean. The new tests are in `cmd/panel/account_email_handler_test.go`
  and `internal/mutationpayload/panel_certificate_issue_test.go`: the change for all
  four roles (sessions kept, one audit row, no address in it); every refusal
  changing nothing (invalid forms, the same address, a taken one including another
  letter case, a wrong or missing password, an unknown field, two JSON objects, no or
  a bad identity, a wrong method); cross-origin and origin-less `PUT` blocked by the
  CSRF check; wrong passwords counted toward the lockout (the test's third attempt is
  refused even with the right password; another account is unaffected); the replay
  of one identity (the same answer, one audit row; a reused identity with another
  address refused; the stored digest without the password); the compare-and-set
  against a stale read; the handler refusing to run without the guard; the route
  gates; and the panel certificate request refused with `/account#email`, then
  carrying the new address after the change, with no certificate requested by the
  change; and the validator.
- Web: `npm test` 1708 of 1708 passed (the new `tests/own-email-mounted.test.mjs`: 10 tests in English and Turkish; one new mounted test of the certificate refusal's banner on Settings; the additional user's menu item), `npm run build` passed within every budget (critical boot path 329.93 of 361 KiB).
- Release-recovery harness: not run, because no update-related file changed (it
  mirrors the update card's texts and outcome logic, which this release does not
  touch).
- Deploy contracts: `deploy/test-release-sequence-policy.sh` passed (98 -> 99) in
  Git Bash against the candidate worktree; `deploy/test-download-portal-contract.sh`
  and `deploy/test-signed-release-manifest-contract.sh` passed in the
  `CelikPanel-S2-Debian` guest on a copy of the candidate's tracked files.
- Browser batch 24 (mock answers, loopback only), the email card's states in
  English and Turkish, light and dark, at 1440 and 390 px: 13 states in each of 8 configurations (104 screenshots), looked at twice: the first look found the refusal banner's button 28 px tall on a phone, now at least 44 px; the second look found nothing else; the language switcher inside the personal menu (28 px on a phone) is older than this release and unchanged; screenshots and the browser driver deleted afterwards.

Component and mounted tests do not establish native behaviour.

## How to update

Install this release only through CelikPanel's update interface (Settings →
CelikPanel updates). Publishing it does not update installed servers; only the owner
of a server starts its update, from the panel's own update screen.
