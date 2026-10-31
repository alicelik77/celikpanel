# Account access: forgot password, invitations, email confirmation

*v0.1.0-alpha.114. Türkçe: [ACCOUNT-ACCESS.tr.md](ACCOUNT-ACCESS.tr.md).*

Customers and resellers look after their own sign-in. Nobody chooses or sees
another person's password.

## How the panel sends account mail

The panel has no mail system of its own and adds no outside dependency. It
hands each message to **this server's own mail system** over SMTP on
`127.0.0.1:25` (Postfix accepts mail from the server itself). Nothing about the
mail system is configured or changed by this feature.

Account mail works only when all three are true; otherwise the screens say
which one is missing and who can help (`reason`):

| reason | meaning | who acts |
| --- | --- | --- |
| `no_mail_server` | nothing answers SMTP on 127.0.0.1:25 | the administrator installs the mail stack, or sets a temporary password |
| `no_panel_address` | the panel certificate names no domain, so links cannot point to the panel | the administrator finishes the panel address in Server setup |
| `no_sender` | no domain to send from (setup's main domain, the saved mail host name or the panel name) | the administrator finishes server setup |

Links use the panel certificate's name and the **configured** panel port
(`CELIKPANEL_LISTEN`), never the request's `Host` header. Messages come from
`no-reply@<main domain>`. "Handed to the mail system" is what the panel can
know; whether a message reached an inbox is not known to it.

Not done: an optional outside SMTP relay (host, port, user, password). A
server without a local mail system uses the guidance above instead.

## Forgot password

1. The sign-in card has **Forgot password?**. It first reads
   `GET /api/v1/auth/recovery/mail-status`; when mail cannot be sent it says so
   before anyone types.
2. `POST /api/v1/auth/recovery/request {identifier, lang}` takes a user name or
   an address and **always answers `202 {"accepted":true}`**: for an unknown
   name, a suspended account, an unconfirmed address, an account with an open
   invitation and a real match alike. The message is handed over after the
   answer, so timing does not tell either.
3. A reset is sent only to a **confirmed** address — for every role, the
   administrator included. An administrator without one recovers with the
   panel's create-administrator command on the server (it keeps the account
   and replaces its password).
4. The link (`/account-link/reset#t=<token>`) works **once, for 30 minutes**.
   The page checks it, asks for the new password twice, and saves it.
5. Saving writes the password with a new credential epoch, **removes every
   session of that account** and its pending two-step sign-ins, revokes its
   other open links and does not sign the person in. Two-step verification
   stays on.

Limits: 5 requests per 15 minutes per client address (then `429`); 3 reset
messages per hour per account (beyond that the answer is the same and nothing
is sent); 30 link reads and uses per 15 minutes per client address.

## Invitations

- `POST /api/v1/users` with `"invite": true` (administrator, reseller) or
  `POST /api/v1/team-members` with `"invite": true` (customer, for an
  additional user) creates the account **without a password** (a random one
  nobody sees) and an invitation link valid for **7 days**. Giving a password
  together with `invite` is refused.
- When the server can send mail the link is emailed and the answer says
  `delivery: "sent"`. When it cannot — or the mail system refuses the message —
  the answer gives the inviter alone, once, `link_path` and the `reason`; the
  page builds the full link from the address the inviter is using.
- `GET /api/v1/invitations` lists open invitations the caller manages
  (administrator: every non-administrator account; reseller and customer: the
  accounts whose `parent_id` they are). `POST /api/v1/invitations/{id}/resend
  {delivery: "email"|"link"}` makes a new link and voids the old one; `DELETE
  /api/v1/invitations/{id}` cancels it (the account stays). An account that got
  a password another way is not re-invited.
- Accepting (`/account-link/invitation`) sets the password; an emailed
  invitation also confirms the address.

## Email confirmation

Migration 047 adds `users.email_verified_at`; every existing address starts
unconfirmed, and a trigger clears the confirmation whenever an address changes
by any path. My account shows the state and **Send confirmation link**
(`/api/v1/auth/email/verification`, valid 24 hours, 3 per hour). Changing one's
own address sends a confirmation link to the new address and a notice, without
the new address, to the old one.

## Storage and audit

`account_links` keeps only the SHA-256 of each 256-bit token (a deterministic
hash is what lets the panel find the row; the token's entropy makes a slow
hash unnecessary), the user, purpose, address, credential epoch and
timestamps. A link is void once the account's password, suspension or
two-step state changed or its address differs. Used, voided and expired rows
are removed after 30 days. The token travels in the URL fragment, so it never
reaches a server log or a `Referer` header, and a mail scanner that only
fetches the address changes nothing.

Audit actions: `account.password_reset.requested|mail_failed|completed`,
`account.invitation.sent|mail_failed|revoked|accepted`,
`account.email.verification_sent|verified`. No address is written to the trail.

## Owner independence

Nothing here runs on the mail system's behalf or changes it; with the panel
stopped, mail, sites and DNS keep working as before. Recovery of the server
owner's own administrator account never depends on mail.
