# Mailbox recovery address and reset link: security review

*2026-11-08 · alpha.130 roadmap item 4 (ROADMAP.md, alpha.130 — Mail) ·
written before any code, as the roadmap requires.*

## 1. What is proposed

A person who forgot a mailbox's password (for example `anna@example.com`)
asks for a reset link. The link is mailed to a **recovery address** that was
set for that mailbox earlier, and whoever opens it can choose a new mailbox
password without knowing the old one.

That is a **sign-in path without a password**. Whoever controls the
recovery address controls the mailbox. The whole feature must therefore be
exactly as strong as that address, and never weaker.

## 2. Facts of the current system (read from source, 2026-11-08)

- **Panel accounts.** Mailbox users have no panel account. The panel roles
  are administrator, reseller and customer (with additional users). A mailbox
  is used through IMAP/POP3/SMTP and through webmail (Roundcube, which signs
  in over IMAP on `localhost:143`; there is no password plugin).
  - Sources: `cmd/panel/authz.go`, `cmd/agent/webmail_rpc.go` `renderRoundcubeConfig`.
- **Where the password lives.** The panel does not store the mailbox
  password. `email_accounts.password_hash` is the placeholder
  `managed-by-agent`. The real hash is a `SHA512-CRYPT` field in Dovecot's
  passwd-file `/etc/dovecot/users`, which only the Agent rewrites
  (`Agent.UpdateMailPassword`, `replaceDovecotPasswordHash`). The owner can
  change that file without the panel (D-022).
- **Who changes a password today.** Anyone who may manage the domain's mail
  changes a mailbox password directly, without knowing the old one:
  - the administrator;
  - the reseller;
  - the customer who owns the subscription;
  - an additional user with the mail capability in manage mode;
  - a `mail` write API token.

  Source: `PUT /api/v1/domains/{id}/mail/accounts/password`, `handleUpdateEmailPassword`.
- **What a change does not end.** A password change does not end open
  IMAP/POP3 sessions. Only the transfer rotation runs `doveadm kick`
  (`Agent.EndMailSessionsV1`).
- **What mailboxes do not have.** Mailboxes have no two-step verification,
  no app passwords and no per-mailbox suspension. Suspension is per
  subscription: `nologin=y` on every passwd-file line of the domain, plus a
  450 on delivery.
- **Pauses.** A mailbox paused by the outgoing limits (alpha.128) sends again
  only after its password changed (owner decision, 7 Nov 2026). The reason is
  `spike`, `refusals` or `manual`.
- **The panel's own reset path.** The panel's own forgot-password path for
  panel accounts (alpha.114, `account_recovery_handlers.go`,
  `account_links.go`) has these controls:
  - SHA-256 of a 256-bit token;
  - a 30-minute lifetime and single use, with consumption as a
    compare-and-set inside the change's own transaction;
  - the token in the URL fragment;
  - a generic `202` answer;
  - limits per client address and per account;
  - mail only to a confirmed address;
  - every session dropped after the reset.

  That reviewed pattern is the base here.
- **Mail sending.** Panel mail goes out through the Agent to Postfix's own
  sendmail (`Agent.SendAccountMailV1`), from `no-reply@<main domain>`, with
  links on the panel's own certificate host name.

## 3. Threat model

Each line names a threat, then the control this review requires, then the
risk that remains.

### T1 · Takeover through the recovery mailbox

- **Threat.** Whoever reads the recovery mailbox can reset the mailbox. That
  can be a compromised external mailbox, or an external domain that expired
  and was registered again by someone else.
- **Control.**
  - The address must be **confirmed** by a link mailed to it before it is
    ever used. A pending address never receives a reset link.
  - Every reset is announced **to the mailbox itself**, after the fact.
  - The link lives 30 minutes and works once.
- **Remains.** It is as strong as the recovery mailbox. This is the same
  residual risk every provider's recovery address has. It is opt-in per
  mailbox, and the screen says so in plain words.

### T2 · Recovery address on this same server

- **Threat.** The recovery address may be a mailbox or forwarder in a domain
  hosted on this server. That domain may be deleted and then added again by a
  different customer of the same server, who creates the address and
  receives the links.
- **Control.**
  - When the recovery address's domain is hosted here, the confirmation
    records that domain's row identity.
  - A reset link is sent only while the same domain row exists. Deleted and
    added again means a new row, so the address is void.
- **Remains.** None beyond T1.

### T3 · Who may set the address

- **Threat.** A panel user sets their own address as a back door.
- **Control.**
  - Only the people who may already change the mailbox password without
    knowing it may set or remove the address: administrator, reseller, the
    customer, or an additional user with mail manage. Setting it therefore
    gives no new power.
  - The address is shown on the recovery dialog of every mailbox to everyone
    who may manage the mailbox.
  - The mailbox is told when an address is set, with the masked address.
  - It is audited (`mail.account.recovery.set` / `.removed` / `.confirmed`).
- **Remains.** See T4.

### T4 · Persistence after access ends

- **Threat.** A team member or reseller loses access, or the domain is
  transferred to another customer, and the earlier address still works.
- **Control.**
  - The address is bound to the domain's subscription and that
    subscription's owner at the time it was set. A transfer (another
    subscription or another owner) voids it.
  - A removed team member's address stays visible to the customer on the
    mailbox, and the customer can remove it.
- **Remains.** A team member's address outlives the member until the
  customer removes it. It is visible, and the mailbox was told when it was
  set.

### T5 · Enumeration

- **Threat.** The request reveals whether a mailbox exists or has a recovery
  address.
- **Control.**
  - The request always answers `202 {"accepted":true}`.
  - The lookup, the Agent read and the mail all run after the answer.
  - Only server-wide facts answer differently: no mail system (503), and this
    client address's own limit (429).
- **Remains.** None known.

### T6 · Token theft and replay

- **Threat.** The token leaks through logs, a Referer header, link scanners
  or browser history, or it is used twice.
- **Control.**
  - The token is 32 random bytes; only its SHA-256 is stored.
  - It travels in the URL fragment, so it never reaches a server log or a
    Referer, and the page removes it from the address bar.
  - Opening the page changes nothing. Only *Save* with a new password does,
    so link scanners are harmless.
  - Consumption is a compare-and-set inside the transaction. A newer link,
    and every reset, revokes the older links.
- **Remains.** Mail in transit is protected only as far as the recipient's
  servers use TLS. The 30-minute lifetime and single use bound that.

### T7 · Binding to the password version

- **Threat.** The link is used after the password was changed some other way:
  by the panel, by the owner in `/etc/dovecot/users`, or by a transfer
  rotation.
- **Control.**
  - When the link is made, the Agent reads a fingerprint of the mailbox's
    current password field (SHA-256 of the stored hash, never the hash). The
    link records it.
  - The reset is a compare-and-set in the Agent: the hash is replaced only
    while the fingerprint is unchanged. Otherwise nothing changes and the
    link is void.
  - A login-disabled line (`nologin=y`, suspension) is refused.
- **Remains.** None known.

### T8 · Brute force and mail flooding

- **Threat.** The request form is used to flood a recovery address, or the
  panel is used as a mail relay.
- **Control.**
  - Requests: 5 per 15 minutes per client address.
  - Reset mails: 3 per hour per mailbox, with excess dropped silently.
  - Link reads and uses: 30 per 15 minutes per client address.
  - Confirmation mails: 3 per hour per mailbox, and 20 per hour per signed-in
    person.
  - The mail text is fixed. No user-supplied text is placed in it except the
    validated addresses.
- **Remains.** None known.

### T9 · Sessions after a reset

- **Threat.** The attacker who caused the reset stays connected.
- **Control.**
  - After a reset the Agent ends the mailbox's IMAP/POP3 sessions
    (`doveadm kick`, `EndMailSessionsV1`).
  - Roundcube signs in over IMAP again on every request, so its sessions end
    with the old password.
  - There are no app passwords to revoke.
  - The answer says so when the sessions could not be ended.
- **Remains.** An SMTP submission connection already authenticated may finish
  its current session.

### T10 · Suspension, deletion, pause

- **Threat.** The reset bypasses a suspension or an administrator's pause.
- **Control.**
  - No link is sent, and no reset is applied, while the subscription is
    suspended, the domain is being deleted or the passwd line is
    login-disabled.
  - A reset ends a **security pause** (`spike`, `refusals`), just like the
    owner's own password change (owner decision, 7 Nov 2026).
  - It **never ends an administrator's manual pause**. The page says sending
    stays paused and who to ask.
- **Remains.** None known.

### T11 · Two-step verification

- **Threat.** The reset bypasses a second factor.
- **Control.**
  - Mailboxes have no second factor, so there is none to bypass.
  - The recovery is single-factor, equal to the recovery mailbox. This is
    stated on the setting.
- **Remains.** As T1.

### T12 · Phishing with look-alike mail

- **Threat.** An attacker sends fake "reset your mailbox" mail.
- **Control.**
  - The link host is always the panel's own certificate name.
  - The mail is sent only after a request.
  - The text tells the reader to ignore it if they did not ask.
- **Remains.** As with any mailed link.

### T13 · CSRF and public routes

- **Threat.** Cross-site requests reach the public endpoints.
- **Control.**
  - The public endpoints take JSON bodies with a token, need no cookie and
    change nothing without a valid token.
  - They are listed exactly (path and method) like the account routes.
- **Remains.** None known.

### T14 · Secrets

- **Threat.** The new password leaks.
- **Control.**
  - The new password goes only in the one Agent request. It is never
    logged, stored or shown again.
  - The passwd field fingerprint is a hash of a salted hash, which helps no
    offline guess.
- **Remains.** None known.

### T15 · Owner independence (D-022)

- **Threat.** The feature makes the mailbox depend on the panel.
- **Control.**
  - The feature writes nothing to the host except the same one passwd field
    a normal change writes.
  - Mail keeps working without the panel. Removing the panel removes only
    the convenience.
- **Remains.** None.

## 4. Conditions the build must meet

1. The address is set and removed only through the same authorization as a
   password change.
2. It is used only after its own confirmation link was opened. The link lives
   24 hours and works once.
3. The address cannot be the mailbox itself.
4. It is bound to the subscription and owner of the moment. When its domain
   is hosted here, it is also bound to that domain row.
5. Reset links are hashed, live 30 minutes, work once, are revoked by a newer
   link and by any reset, and carry the Agent's password-field fingerprint.
6. The public request always answers the same. It works only after the
   answer, and has limits per client address and per mailbox.
7. The reset follows the alpha.127 password rule (`mailbox` field;
   `PASSWORD_WEAK`) and `PasswordInput`. It is an Agent compare-and-set.
   Sessions are ended. The mailbox gets a notice. It is audited.
8. It is refused while the subscription is suspended, the domain is being
   deleted or the line is login-disabled. A manual pause stays; a security
   pause ends.
9. Unknown results are told as unknown, never as success (D-024). If the
   Agent call did not answer, the person tries the new password; if it does
   not work, they ask for a new link.

## 5. Verdict

**Passes, with the conditions of section 4.** The feature adds no power that
the people who may set the address do not already have.

Its new risk is the recovery mailbox itself (T1). That risk is bounded by
four controls:

- the confirmation;
- the notice to the mailbox;
- the short, single-use link;
- the binding to the password version, the subscription owner and the local
  domain row.

Not built in this release, by choice:

- **Mailbox users setting their own recovery address.** They have no panel
  login. A signed-in webmail setting would need a Roundcube plugin, and that
  is a separate review.
- **A "forgot password" link on the Roundcube login page.** The request page
  lives at `/account-link/mailbox-recovery` on the panel's address, and the
  confirmation and notice mails name it.

## 6. Where it is built

- **Migration and code.**
  - Migration `080_mailbox_recovery.sql`.
  - `cmd/panel/mailbox_recovery*.go`.
  - `cmd/agent/mail_password_cas.go`, which adds `Agent.MailPasswordFingerprintV1`
    and `Agent.SetMailPasswordIfUnchangedV1`.
- **Screens.**
  - The mailbox list's recovery dialog.
  - The public pages under `/account-link/mailbox-*`.
- **Tests.** As listed in the release notes.
