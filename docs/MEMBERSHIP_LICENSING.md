# Membership and annual server licensing

*Current-policy reconciliation: October 30, 2026 (seven-day offline window, D-039) · [Türkçe](MEMBERSHIP_LICENSING.tr.md) · D-025*

This document distinguishes the annual license term, the access verification
window (seven days offline since D-039, approved by the owner on 2026-10-29), and
historical behavior. It records existing code and required recovery work; it does
not authorize an update on an installed panel.

Approved product model: one CelikPanel product, four existing authorization
roles, and a celikpanel.net member account independent of panel administrators.
Initially a verified member may issue free server licenses. Each license binds
to one verified public server IP and lasts one calendar year from creation.
Existing activated licenses retain their already-granted expiry; legacy unactivated
licenses receive creation plus one calendar year during the additive migration.
Retrying activation preserves the original term. Server transfer preserves the
term and requires the member's password. No payments or automatic billing yet.

The central service uses PHP 8.3/PDO SQLite on the existing portal host. Its
database, sessions and Ed25519 license signing secret live outside httpdocs and
outside atomic website releases. Each portal wrapper pins an immutable private
application release by its source manifest hash, so website rollback selects
the prior application too. Stage it with deploy/stage-membership.py before the
supported portal publisher; never put the private state in a portal archive. Release-signing and license-signing keys are
separate. Clients receive signed entitlements, never database access. Mail is
sent through the configured authenticated SMTP transport; verification/reset
tokens are short-lived, single-use, hashed and never written to access URLs.

Member UI: registration, email verification, sign-in, password reset, licenses,
issue, copy key, renewal and server release. Match the portal's existing
navy/white typography and provide Turkish and English. License secrets can be shown again after owner-password verification. Stored hashes
authenticate activation; authenticated secretbox encryption permits redisplay. The
storage key is domain-separated from the private license signing secret. A signing
secret rotation must re-encrypt existing keys before retiring the old secret.
Legacy hash-only keys cannot be reconstructed: valid activation stores the supplied
key, or the owner can explicitly save their existing key without rotating it. Panel credentials
remain on the customer's server and are still created in the terminal.

License authentication is not panel authentication. The installer requests a
license without echo or command-line arguments. Activation persists before
continuing installation; an interrupted installation reuses its identity and
entitlement. Panel administrators may activate/refresh in Settings. The current
HTTP access gate applies to normal panel management across all four roles, with
the exact recovery exceptions listed below. Older provisioning-only descriptions
and broad maintenance exemptions are historical, not the current API contract.
A central-service outage retains the existing signed receipt; it cannot extend
that receipt's effective access deadline (at most seven days after its last
successful check, never past the annual expiry).

Server transfer, key replacement and license removal invalidate the old binding
centrally; the panel learns of it at its next check, about hourly while in use.
Signed cached receipts remain usable only until the effective access deadline
described below. If the annual term is still valid but that deadline has passed,
an unreachable verifier produces `verification_unavailable` with reason
`offline_ended` (or `clock_behind`, below). The access endpoint answers
`can_use_panel=false` with that reason, and the browser shows the recovery screen
with the cause, who acts and what to do; it does not route to activation. Existing
workloads are not stopped by this HTTP gate.

Before publication: transactional activation/transfer/renewal tests, auth/CSRF/
token abuse tests, signed entitlement and server-binding tests, provisioning
route coverage, interrupted installation, role isolation, and one bounded
desktop/mobile TR/EN browser inspection plus one correction pass. Production
mail delivery and PHP web-handler capabilities must be verified explicitly;
CLI PHP availability alone does not prove the web handler.

## Current enforced access policy

Source: [license manager](../internal/licensing/license.go),
[panel access gate and exceptions](../cmd/panel/license.go), and
[browser access gate](../web/src/components/LicenseOnboarding.tsx).
The annual term and this access-verification window are separate:

| Decision | Current implementation |
|---|---|
| Effective authorization deadline | The earliest of annual expiry, the signed `offline_until` and `issued_at + 7 days`. The service signs `offline_until = min(now + 7 days, expiry)`. A receipt from a service before D-039 (`offline_until = issued_at + 60 s`) keeps exactly that minute. |
| Ordinary refresh eligibility | At the earlier of signed `refresh_after` (service: `now + 1 hour`) and `issued_at + 1 hour`, minus up to a fifth as deterministic per-server jitter (48–60 minutes). Only authenticated use triggers it; there is no server timer polling an idle installation. Concurrent requests share one refresh. |
| Request wait for a due refresh | A receipt that no longer opens management waits for the shared refresh. A still-valid receipt waits at most 3 seconds, then the request proceeds on it while the refresh finishes in the background; a reachable service's rejection therefore applies to the discovering request. |
| Failed refresh retry | While the receipt is valid: 1, 2, 4, 8, 16, then 30 minutes (each shortened by up to a fifth per server), and never later than the moment the receipt lapses. After it lapses: every 30 seconds on use. The administrator's "Check again" bypasses the backoff, never signature or binding verification. |
| Explicit rejection | `invalid_license`, `invalid_activation`, `license_in_use` or `license_expired` on refresh is persisted at once (`rejected`) and closes management immediately, inside or outside the offline window. |
| Clock safety | `issued_at` is the service's clock. A server clock more than 5 minutes before `issued_at`, more than 5 minutes below the highest clock reading recorded with the receipt (`seen_at`, local, unsigned), or set back while the panel runs (wall clock vs. monotonic time) gives `verification_unavailable` with reason `clock_behind` until a check succeeds; a reachable service is asked at once. A fresh answer dated more than 5 minutes after this clock is refused with check `clock_behind`. |
| HTTP refresh budget | The license client has a 15-second timeout; the shared background refresh is bounded at 20 seconds. |
| Missing/expired/rejected/unverifiable entitlement | Normal management is denied. The access endpoint answers `can_use_panel=false` with state, observation and, for `verification_unavailable`, the reason. The license page (`/api/v1/panel/license`, administrators) also reports `check` (last failed attempt: `unreachable`, `rate_limited`, `invalid_answer`, `clock_behind`), `offline_for_seconds` and `verified_seconds_ago`. |
| Browser behavior | A failed access request retains only a still-valid decision for the same user, without extending its deadline. Unknown access shows connection recovery; a server denial or `license_required` response locks management. |

The current authenticated exceptions are deliberately narrow. All role,
ownership and method checks still apply:

- Session identity and access-status reads; logout, password change and ending
  impersonation remain available.
- License reads, activation and refresh remain available to administrators.
- Administrators can read panel version, update check/status and host-mutation
  readiness, and use the signed-update start/abandon endpoints. Release trust,
  operation identity and mutation admission remain enforced. The user must
  initiate every installed-panel update in the panel interface; API capability
  does not authorize assistant-side installation.

These exceptions do **not** provide general panel access to backups, deletion,
service configuration or other maintenance when the entitlement cannot authorize
management. Existing native workloads and their independent schedules are not
stopped by this HTTP license gate. Full operation after removal of management
binaries is a separate, unfinished [owner-independence requirement](OWNER-INDEPENDENCE.md).

An unreachable verifier after the effective deadline produces
`verification_unavailable` in the manager; it is not proof that the annual
license expired. Explicit rejection of this installation's refresh can invalidate
its local receipt. A mistyped replacement key must not revoke a currently valid
binding. The seven-day window is the whole offline allowance; nothing extends it.

Clock rollback, stated plainly: the window is anchored on the service's
`issued_at` and capped by the signed `offline_until`, so moving the clock forward
never helps and a clock before `issued_at` is refused. Setting the clock back
inside the window is detected while the panel runs and, across a restart, down to
the granularity of `seen_at` (written at every successful check and every failed
attempt, i.e. at least every 30 minutes of use during an outage). The owner is
root on their own server: restoring an old license file together with a clock set
back, or patching the binary, is outside what a local check can stop; that is the
same limit as before D-039, with the window now seven days instead of one minute.

## Required recovery behavior — not yet fully implemented

[D-025 and the resilience contract](RESILIENCE-CONTRACT.md) require typed access
reasons and separately scoped authenticated status/recovery capabilities.
A failed observation must not be converted into a different diagnosis: verifier
unavailability must not imply that the owner needs a new license key; a failed
session lookup must not be treated as a confirmed unauthenticated response.

The current boolean access response and shared `license_required` denial do not
preserve that distinction across the frontend/backend boundary. An authenticated
recovery/status surface independent of the normal Agent and candidate startup
also remains open work. Its implementation must preserve identity, server and
operation scope, sensitive-data boundaries, current license deadlines and the
user-only panel update rule. This is not permission to extend leases, bypass
licensing or expose unrestricted privileged actions.

## Historical activity policy (Alpha 59) — superseded

The following timing and admission description records Alpha59 behavior. It is
retained to explain older installations and receipts; use the current policy
above for present code. The daily refresh, 15-minute failure backoff, seven-day
cached access and broad maintenance access are no longer the current client
contract.

Alpha59 had no startup/hourly licensing poll. Authorized API requests from admin,
reseller, customer and additional-user sessions used one shared activity hook.
The first request after the signed daily refresh deadline started one coalesced,
bounded refresh, with a 15-minute backoff after failure and no idle retry timer.
Provisioning used the local signed receipt; login and maintenance remained open.
This paragraph is historical evidence, not a promise of those exemptions today.

## IP binding and retained receipt compatibility

The central API takes the source IP only from web-server REMOTE_ADDR, never client
JSON or forwarded headers. Its front proxy must reliably restore the real peer IP.
IPv4-mapped IPv6 is normalized. Private/reserved source addresses are rejected.
The explicit loopback development mail fixture maps its local peer to a fixed test
public IP; this is not enabled on the production HTTPS origin.

The fixed public IP identifies a license's server assignment, not physical hardware.
Shared NAT is unsuitable for distinguishing servers; use a dedicated fixed egress
IP. Dual-stack egress changes require a binding change just like any IP change.
Receipts remain v1 and machine-bound locally for compatibility with installed
clients. Reinstalling at the same bound IP with the key replaces the machine binding
and invalidates the previous refresh credential, preserving key and expiry.
The signed v1 format permits an offline deadline up to seven days after
issuance, bounded by annual expiry; since D-039 the service signs exactly that
and current panels honour it. Panels from v0.1.0-alpha.64 (#134) up to the
release carrying D-039 still apply their own one-minute cap to a seven-day
receipt; panels before alpha.64 already honoured the signed window. Older receipt formats do not grant
a current client a longer authorization window.
Legacy machine-bound licenses bind the observed IP after proving the original
machine and key/refresh credential; an already reinstalled legacy server requires
the owner to release its old binding first.

Changing server/IP requires the member password, clears the binding and increments
credential generation, but keeps the key and term. Key replacement is separate and
preserves IP and term. None of these operations stop existing workloads.
