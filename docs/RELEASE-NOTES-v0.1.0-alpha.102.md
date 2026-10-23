# v0.1.0-alpha.102 (candidate, not published)

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.102.tr.md)

This is the release candidate that follows v0.1.0-alpha.101. It is not published;
nothing from it is on an installed server. Written on 2026-10-23 on the branch
`fix/alpha102-install-email`, from `main` after the publication record of
alpha.101.

## Who should update, and why

Anyone who installs CelikPanel on a new server, once this release is published.
An existing installation gains nothing visible: the change is in the installer's
first-administrator step.

Since alpha.99 an open item said that Server setup refuses to start any plan while
the administrator has no valid email address. The owner decided on 2026-10-23
that it stays that way: "iletişim epostası zorunlu olarak gerekiyor ise zorunlu
olmalı. kurulumda eposta belirtiyoruz" (if the contact email is genuinely
required it must be mandatory; an email is already given during installation). It
is genuinely required: the first setup always requests the panel's HTTPS
certificate, and the certificate authority needs the administrator's address as
its contact. The only plan without a certificate step is one for a server that
already has a valid certificate, and then the address is already on file. The
owner then approved ("ok. yap") checking the address where it is first typed, so
a typo such as `ali` or `ali@` is caught during installation and not later on the
Server setup screen (D-021 amendment).

Only the owner of a server updates it, from the panel's own update screen.

## What changes during installation

**The administrator's email is checked with the certificate request's rule.**
The installer uses the same check as the setup plan, the panel certificate
request and the own-email change on My account: printable ASCII, exactly one `@`,
a local part of at most 64 characters, a full domain name (not `localhost`, not
an IP address, no trailing dot), at most 254 characters in all. For example
`ali`, `ali@`, `@x.com`, `a b@x.com` and `ali@localhost` are refused;
`you@example.com` is accepted.

- **At the prompt** ("Admin email / Yönetici e-posta:"), a refused address prints

  > That is not a valid email address (example: you@example.com). The certificate
  > authority needs it as a contact. Please try again. / Bu geçerli bir e-posta
  > adresi değil (örnek: siz@ornek.com). Sertifika yetkilisi bunu iletişim adresi
  > olarak ister. Lütfen yeniden deneyin.

  and the same question is asked again. What was typed is not printed. An empty
  answer keeps its own line ("Email must not be empty…"). The prompt is where it
  was, after the packages are installed; it asks again instead of stopping, so a
  typo does not stop the installation.
- **With a credentials file** (`CELIKPANEL_ADMIN_CREDENTIALS_FILE`), the installer
  checks the file with the downloaded panel before anything is installed, as it
  did; that check now refuses such an address. The installer stops with

  > Administrator credentials are invalid; the input file was left untouched and
  > nothing was installed. Correct the file, then run the installer again: a
  > username the panel accepts, an email that is a real address such as
  > you@example.com (the certificate authority needs it as a contact), and a
  > password of at least 8 characters

  and its Turkish line. Run the same command again after correcting the file.
  The panel's own refusal (seen when the panel binary is run by hand) names the
  email field and never a value.
- **The address is stored as the own-email change stores it:** with the domain in
  lower case (`Owner@Example.COM` is stored as `Owner@example.com`).
- **Running the create-administrator command again** for an existing user name is
  held to the same rule; a refused address changes neither the address nor the
  password.

## What does not change

- **Existing installations.** No migration; stored addresses are not checked again
  or rewritten. An older address that the certificate request would refuse is
  still caught by Server setup's contact blocker, whose "Open My account" button
  leads to the own-email change.
- **Server setup's contact blocker** stays for every plan.
- **The certificate authority's own judgement.** The panel checks the form of the
  address; the authority may still refuse an address at a domain that is not
  public (`.test`, `.invalid`, `example.com`).
- **Team members and other accounts.** No other path makes an administrator: the
  account screens refuse to create or manage administrator accounts, team members
  are customer-role additional users who cannot open Server setup, and a
  control-plane restore brings back the archive's administrators unchanged. Their
  address checks are not changed.
- **The signed entry script** (`get.sh`) keeps its own narrower form check; only
  its version pins change. The downloaded panel's check is the authority.
- The interface, roles, the Agent, the updater, release recovery, the schema (44,
  no migration) and polling. No native record is made, by the owner's instruction
  of 2026-10-16 (no native record unless the updater's mutation path changes).

## Known gaps

- Nothing is measured on an installed server or a real first installation.
- **Upcoming acceptance, not a claim:** at Beta 1 the owner will wipe and
  reinstall both servers for a clean first-install test; that is where this check
  will first be seen on a real installation.

## Evidence

On the candidate tree:

- Go: the prompt takes exactly what the certificate contact validator takes and
  asks again otherwise (`ali`, `ali@`, `@x.com`, `a b@x.com`, `ali@localhost`, an
  IP address, a display name; accepted addresses returned in canonical form); the
  strict credentials parser refuses the same addresses and a padded one; the
  create path refuses before any database change, stores the canonical form, and
  leaves an existing administrator's address and password unchanged on a refused
  address; the validate-only mode exits with an error naming the email field and
  no value; the credentials-file create path writes no user. Passed on the final tree in the `CelikPanel-S2-Debian` guest (the Panel and the internal packages, 45-minute limit, CGO off): 7,219 passed, 44 skipped, 0 failed in 64 packages (2,921 top-level tests); gofmt and go vet clean.
- Shell: In a fresh clone in the `archlinux` guest the release sequence policy ("101 -> 102; historical bootstrap sequence 101"), download portal, signed release manifest, bootstrap update, first-install signed trust, fresh-install entry boundary and install localization contracts passed, and the offline release-recovery harness passed (1,109 tests, 1 skipped), run because `install.sh` changed; the Go toolchain contract passed in the `CelikPanel-S2-Debian` guest (it refuses the `archlinux` guest's Go 1.27 by design, as only the pinned 1.26.5 is accepted). No fixture needed changing: every sample administrator address in the shell and harness fixtures already passes the rule.
- No web change, so no web build or web tests were needed.

## How to update

Once published: from the panel's own update screen (Settings → CelikPanel updates),
by the owner of the server. Nothing needs to be done before or after. A new
installation uses the installer as before.
