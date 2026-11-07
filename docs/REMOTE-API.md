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

   The token (`cpk_…`) is shown **once**. Copy it then, and store it like a
   password. CelikPanel keeps only its SHA-256 hash.

Team members (additional users) cannot hold tokens. A token cannot be made
while signed in with "Log in as".

## Using a token

Send the token in the `Authorization` header, and nothing else. The token is
never accepted in a query string. A request that carries the panel's session
cookie as well as a token is refused.

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
| GET | `/api/v1/domains/{id}/dns/records` | DNS records |
| GET | `/api/v1/domains/{id}/mail/accounts` | mailboxes |
| GET | `/api/v1/domains/{id}/databases` | databases |
| GET | `/api/v1/domains/{id}/backups` | backups |

**Read and change** (adds):

| Method | Path | What |
|---|---|---|
| POST | `/api/v1/domains/create` | add a domain (`{"domain":"example.com","project_type":"php"}`) |
| DELETE | `/api/v1/domains/{id}` | delete a domain and its site |
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

## Limits and refusals

- 120 requests a minute per token. After that: `429 API_TOKEN_RATE_LIMITED`
  with `Retry-After: 60`.
- 10 failed token sign-ins a minute from one address. After that, that
  address gets `429 API_TOKEN_SIGN_IN_LIMITED` for the rest of the minute,
  even with a right token.

Every refusal is JSON `{"error": "...", "code": "..."}`. The sentence says
what happened, who acts and what to do next:

| Code | Status | Meaning |
|---|---|---|
| `API_DISABLED` | 403 | the administrator has not turned the remote API on |
| `API_TOKEN_INVALID` | 401 | not a token, or no such token |
| `API_TOKEN_REVOKED` | 401 | the token was removed |
| `API_TOKEN_EXPIRED` | 401 | the token's time is over |
| `API_TOKEN_ENDED` | 401 | the owner's password, two-factor sign-in or account changed after the token was made |
| `API_TOKEN_ROUTE_NOT_ALLOWED` | 403 | this address is not in the list above |
| `API_TOKEN_SCOPE` | 403 | a Read only token tried a change |
| `API_TOKEN_NOT_PERMITTED` | 403 | the plan does not include "Remote API" |
| `API_TOKEN_TEAM_MEMBER` | 403 | team members cannot use tokens |
| `API_TOKEN_WITH_SESSION` | 400 | the request carried a session cookie as well |
| `ACCOUNT_SUSPENDED` | 403 | the owner's account is suspended |

## What is recorded

- Making and removing a token is recorded in the audit log.
- So is every change made with a token: the token's public id (`cpk_<id>`,
  never its secret), the method and the path. The rows the change itself
  writes name the token too.
- The token's last use (time and address) is shown on My account. It is
  written at most once a minute.

## Security notes

- Only `SHA-256(secret)` is stored, and it is compared in constant time.
- A token is bound to the owner's sign-in state. A password change or reset,
  a two-factor change or an account change ends every token of that account,
  as it ends the account's sessions.
- The panel's cross-site request protection still applies to every browser
  session. It is skipped only for a request that carries a token and no
  session cookie. Such a request is authenticated by the token or refused.
- Turning the switch off stops every token at once.
