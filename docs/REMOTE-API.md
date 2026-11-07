# Remote API

*alpha.128 (G15) · [Türkçe](REMOTE-API.tr.md)*

The remote API lets an account's own programs and scripts work with that
account without a browser: list and change domains, DNS records, mailboxes,
databases and backups. It is optional panel automation (D-022). The hosted
services never depend on it, and with the panel stopped they run on.

## Turning it on

1. **The server switch.** The remote API is **off** on a new server. The
   administrator turns it on in **Settings → Panel → Remote API**. While it is
   off, no token works. Tokens are kept and work again when it is turned on.
2. **The plan permission `remote_api`** ("Remote API" in the plan's rights).
   An administrator always has it. A reseller needs it offered by the
   administrator and in the reseller's own rights. A customer needs it in
   **every** subscription the account holds: a token acts for the whole
   account. Plans with "all permissions" (the Unlimited plans) include it. To
   keep an account off the API, leave the permission out of its plan.
3. **A token.** On **My account → API tokens**, choose:
   - a name;
   - a scope: **Read only** or **Read and change**;
   - an expiry: 7, 30, 90 or 365 days. A token always ends.

   Making a token asks for your current password again, and for your
   two-factor code when two-factor sign-in is on. The token (`cpk_…`) is
   shown **once**. Copy it then, and store it like a
   password. CelikPanel keeps only its SHA-256 hash.

Team members (additional users) cannot hold tokens. A token cannot be made
while signed in with "Log in as".

## Using a token

Send the token in the `Authorization` header, and nothing else. The token is
never accepted in a query string. A request that carries the panel's session
cookie as well as a token is refused.

Only a header of the form `Authorization: Bearer cpk_…` is treated as a
token. Any other `Authorization` header is ignored by the token layer, for
example an identity proxy's bearer ID token or a proxy's Basic sign-in in
front of the panel. The panel then behaves as if the header were not there.

```sh
TOKEN='cpk_abcdefghijkl_…'
PANEL='https://panel.example.com:1923'

# Your domains
curl -s -H "Authorization: Bearer $TOKEN" "$PANEL/api/v1/domains"

# The DNS records of domain 12
curl -s -H "Authorization: Bearer $TOKEN" "$PANEL/api/v1/domains/12/dns/records"

# Add a record (Read and change token)
curl -s -X POST -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"name":"www","type":"CNAME","content":"example.com.","ttl":3600}' \
  "$PANEL/api/v1/domains/12/dns/records"

# Make a backup. This route also needs a new X-CelikPanel-Request-Id for
# each action: 32 lowercase hexadecimal characters.
curl -s -X POST -H "Authorization: Bearer $TOKEN" \
  -H "X-CelikPanel-Request-Id: $(openssl rand -hex 16)" \
  "$PANEL/api/v1/domains/12/backups"
```

The routes marked "request id" below need the `X-CelikPanel-Request-Id`
header. If the same request is sent again with the same id, the first
answer is returned and the change is not run a second time.

## Routes a token reaches

Every other address answers `403 API_TOKEN_ROUTE_NOT_ALLOWED`. That includes:

- token management;
- password, two-factor and e-mail changes;
- every route that shows a secret;
- VPN, licence, setup and updates;
- every server and administrator route.

The owner's usual rights still apply on top of this list. A token reaches
only what the account itself may reach, under the plan's limits and rights.
A customer's token never reaches another account's domain.

**Read only** (both scopes):

| Method | Path | What |
|---|---|---|
| GET | `/api/v1/domains` | the account's domains |
| GET | `/api/v1/domains/{id}/hosting` | a domain's hosting settings |
| GET | `/api/v1/domains/{id}/usage` | a domain's disk and traffic use |
| GET | `/api/v1/domains/{id}/stats/visitors` | a site's visitor statistics |
| GET | `/api/v1/domains/{id}/dns/records` | DNS records |
| GET | `/api/v1/domains/{id}/mail/accounts` | mailboxes |
| GET | `/api/v1/domains/{id}/databases` | databases |
| GET | `/api/v1/domains/{id}/backups` | backups |

**Read and change** (adds):

| Method | Path | What |
|---|---|---|
| POST | `/api/v1/domains/create` | add a domain (`{"domain":"example.com","project_type":"php"}`) |
| DELETE | `/api/v1/domains/{id}?confirm_domain={name}` | delete a domain and its site; `confirm_domain` must be the domain's name, otherwise `400 API_TOKEN_CONFIRM_DOMAIN` and nothing is deleted |
| POST | `/api/v1/domains/{id}/dns/records` | add a record (`name`, `type`, `content`, `ttl`, `prio`) |
| PUT | `/api/v1/domains/{id}/dns/records` | change a record (`id`, `content`, `ttl`, `prio`, `disabled`, optional `expected_content`) |
| DELETE | `/api/v1/domains/{id}/dns/records?id={record}` | delete a record |
| POST | `/api/v1/domains/{id}/mail/accounts` | add a mailbox (`address`, `password`, `quota_mb`) |
| PUT | `/api/v1/domains/{id}/mail/accounts` | change a mailbox quota (`id`, `quota_mb`) |
| PUT | `/api/v1/domains/{id}/mail/accounts/password` | set a mailbox password (`id`, `new_password`) |
| DELETE | `/api/v1/domains/{id}/mail/accounts?id={mailbox}` | delete a mailbox |
| POST | `/api/v1/domains/{id}/databases` | add a database (`name`, `type`: `mysql` or `postgresql`, `password`) · request id |
| DELETE | `/api/v1/domains/{id}/databases/{database}` | delete a database |
| POST | `/api/v1/domains/{id}/backups` | make a backup · request id |
| DELETE | `/api/v1/domains/{id}/backups?name={file}` | delete a backup |

A password you send (a mailbox or a database) follows the panel's password
rule. A token never reads a password back.

**Mailbox passwords.** A Read and change token can set a mailbox's password.
Setting a password gives access to that mailbox, so such a token is as
powerful as access to every mailbox of the account. Every change made with a
token is in the audit log, with the token's public id. Give scripts a Read
only token unless they must make changes.

## Limits and refusals

- 120 requests a minute per token. After that: `429 API_TOKEN_RATE_LIMITED`
  with `Retry-After: 60`.
- 10 failed token sign-ins a minute from one address. After that, a request
  from that address whose token does not verify gets
  `429 API_TOKEN_SIGN_IN_LIMITED` for the rest of the minute. A right token
  from the same address keeps working, for example from an office behind one
  shared address.

Every refusal is JSON `{"error": "...", "code": "..."}`. The sentence says
what happened, who acts and what to do next:

| Code | Status | Meaning |
|---|---|---|
| `API_DISABLED` | 403 | the administrator has not turned the remote API on (told only to a working token) |
| `API_TOKEN_INVALID` | 401 | not a token, or no such token. This is also the answer when the remote API is off, so a caller without a working token cannot tell whether it is on |
| `API_TOKEN_REVOKED` | 401 | the token was removed |
| `API_TOKEN_EXPIRED` | 401 | the token's time is over |
| `API_TOKEN_ENDED` | 401 | after the token was made, the owner's password, e-mail address or two-factor sign-in changed, or an administrator changed the account |
| `API_TOKEN_CONFIRM_DOMAIN` | 400 | a domain deletion without the right `confirm_domain` |
| `API_TOKEN_ROUTE_NOT_ALLOWED` | 403 | this address is not in the list above |
| `API_TOKEN_SCOPE` | 403 | a Read only token tried a change |
| `API_TOKEN_NOT_PERMITTED` | 403 | the plan does not include "Remote API" |
| `API_TOKEN_TEAM_MEMBER` | 403 | team members cannot use tokens |
| `PANEL_ACCESS_NOT_ALLOWED` | 403 | an administrator's token from an address outside the administrator address list (Security → Panel access), as for an administrator's session |
| `API_TOKEN_WITH_SESSION` | 400 | the request carried a session cookie as well |
| `ACCOUNT_SUSPENDED` | 403 | the owner's account is suspended |

## What is recorded

- Making and removing a token is recorded in the audit log.
- So is a failed token sign-in: at most one row a minute for each token id
  and address, with the reason (`malformed`, `unknown`, `wrong_secret`,
  `revoked`, `expired`, `ended`). The next row counts the failures folded
  into it. A wrong secret for a real token, and a removed, expired or ended
  token, are recorded under the token's owner.
- So is every change made with a token: the token's public id (`cpk_<id>`,
  never its secret), the method and the path. The rows the change itself
  writes name the token too.
- The token's last use (time and address) is shown on My account. It is
  written at most once a minute.

## Security notes

- Only `SHA-256(secret)` is stored, and it is compared in constant time.
- A token is bound to the owner's sign-in state. These end every token of
  that account:
  - a password change or reset;
  - a change of the account's e-mail address;
  - a two-factor change;
  - an administrator's change of the account.
- The panel's cross-site request protection still applies to every browser
  session. It is skipped only for a request that carries a token and no
  session cookie. Such a request is authenticated by the token or refused.
- Turning the switch off stops every token at once.
