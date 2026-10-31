# User Roles & Permissions

*Design document · July 3, 2026 · [Türkçe](ROLES.tr.md)*

This is a **design specification**, not yet fully implemented. It is the blueprint the Phase 0.2 authentication work builds toward. It stays true to the [constitution](../ROADMAP.md): four clear roles, one obvious way, secure by default.

---

## The core idea

CelikPanel has **four roles** arranged in a hierarchy. Each level can only see and act within what the level above it granted. This single, predictable chain of ownership covers every scenario from a one-person website to a distributor running many resellers — without the maze of overlapping permission screens that make Plesk and cPanel intimidating.

```
Administrator            the server itself — full power
   │  creates & funds
   ├── Reseller          sells hosting from a resource pool
   │      │  creates & funds
   │      └── Customer   owns subscriptions, runs their sites
   │             │  delegates
   │             └── Additional User   a scoped subset of one customer
   │
   └── Customer          (Administrator can also create customers directly)
```

The rule that makes it simple: **a resource always has exactly one owner, and you can only manage what you own or were delegated.** No shared ownership, no ambiguous cross-links.

---

## The four roles

### 1. Administrator (`admin`)
The server owner. There is normally **one**. Equivalent to cPanel's *root/WHM* and Plesk's *Administrator*.

- Installs, starts, stops, configures services (Nginx, PHP, MariaDB, PostgreSQL, mail, DNS, …)
- Manages server-wide settings, IP addresses, panel SSL, updates
- Creates and manages **resellers** and **customers**
- Sees everything on the server; the only role that touches the OS layer (via the Agent)

### 2. Reseller (`reseller`)
Sells hosting to their own customers from a **resource pool** the administrator granted (e.g. "100 GB disk, 50 domains total, distribute as you like"). Equivalent to cPanel's *Reseller (limited WHM)* and Plesk's *Reseller*.

- Creates and manages **customers** and their subscriptions, within their pool
- Sets each customer's quotas (never exceeding what remains in the pool)
- Suspends/unsuspends, resets passwords for their own customers
- **Cannot** touch the server, services, other resellers, or customers they didn't create
- Can have their own branding (future: white-label login)

### 3. Customer (`customer`)
The person who actually runs websites. Equivalent to a *cPanel account* / Plesk *Customer*. This one role covers two of your scenarios by design:

- **One website:** one subscription, one domain. The panel shows only what they need.
- **Many websites:** one subscription can hold many domains, or the customer can hold several subscriptions. Same role, more resources — no new concept to learn.

A customer manages, **within their quota**: domains & sites, DNS records, email accounts, databases, files, backups, cron jobs, SSL, PHP settings, logs and statistics — but only for their own subscriptions.

### 4. Additional User (effective role `additional_user`)
A **scoped** login that belongs to a single customer, for delegating part of the work without sharing the master password. Equivalent to cPanel *sub-accounts / User Manager* and Plesk *Additional Users*.

For schema compatibility, this identity is stored with `users.role = 'customer'` and the immutable marker `users.account_type = 'additional_user'`. Authentication derives the effective `additional_user` role from that pair and the owning `parent_id`; malformed combinations fail closed.

Examples:
- A developer who may edit files and databases but not email or billing
- An office manager who only manages email accounts
- A read-only accountant who only views statistics

An additional user never sees resources outside the one customer they belong to, and only the capabilities that customer delegated (see the permission model below).

---

## Which role for your scenarios

| Scenario | Role |
|---|---|
| A person with a single website | **Customer** (one subscription, one domain) |
| A person managing many websites | **Customer** (many domains / many subscriptions) — or delegate parts to **Additional Users** |
| A hosting seller with many customers | **Reseller** |
| Someone who owns/oversees several resellers | **Administrator** — resellers roll up to the admin. A dedicated "distributor above reseller" tier is a deliberate *non-goal for now* (see below). |

---

## Permission model (for Additional Users)

Administrators, resellers and customers have their capabilities fixed by their role — that is what keeps the panel simple. **Granularity exists in exactly one place: the additional user**, where a customer delegates a subset of *their own* capabilities.

Capabilities are grouped by resource. A customer delegating an additional user picks from **their own** set:

| Capability group | View | Manage |
|---|:---:|:---:|
| Files (per domain or all) | ☐ | ☐ |
| Databases | ☐ | ☐ |
| Email accounts | ☐ | ☐ |
| DNS records | ☐ | ☐ |
| SSL / certificates | ☐ | ☐ |
| Cron jobs | ☐ | ☐ |
| Backups | ☐ | ☐ |
| PHP settings | ☐ | ☐ |
| Statistics & logs | ☐ | ☐ |

Two properties keep this safe and simple:
- **Scope narrows going down, never widens.** An additional user can never gain a capability its parent customer doesn't have. A customer can't exceed its subscription. A reseller can't exceed its pool.
- **Delegation is per-domain-optional.** A capability can be granted for one domain or all of the customer's domains — nothing finer, to avoid a permission maze.

---

## Enforcement — where the rules actually live

Permissions are **not** a UI concern. Following the "API-first" principle, every rule is enforced on the server, in one place:

1. **Authentication** (Phase 0.2): who are you? → session tied to a `user`.
2. **Ownership resolution:** every resource (domain, database, …) resolves to an owning subscription → owning customer → (optional) reseller → admin. A request is allowed only if the caller is in that chain, or is the admin.
3. **Capability check:** for additional users, the specific capability is checked on top of ownership.

The UI simply hides what the API would reject — that is how "services you didn't install are invisible" and "you only see what you own" become the same mechanism.

---

## Data model implications (for the 0.2 sprint)

The current schema already has the backbone: `users(role)` and `subscriptions(owner_id)`. To realize this design:

- Add an immutable `users.account_type` marker (`account` or `additional_user`). Additional users retain the stored `customer` role; authorization uses the derived effective role instead of trusting either column alone.
- Use `users.parent_id` (nullable, references `users.id`) for who created/owns the user. For an additional user this must identify one active, real customer account.
- Add a `reseller_pools` concept (or columns on the reseller's own record) for the resource pool a reseller distributes.
- Store grants in two explicit scope tables: `additional_user_subscription_permissions` and `additional_user_domain_permissions`, each with a closed capability set and `view|manage` mode. A domain's effective access is the additive union of its direct-domain and subscription grants; `manage` outranks `view`.
- Every data-access repository gains an ownership filter; no handler queries by raw ID without an ownership check.

These changes are **additive** and land alongside authentication in Phase 0.2, so the model and its enforcement ship together — never a login without the rules behind it.

---

## What is implemented (v0.1.0-alpha.114)

This section states what the code does today; the design above remains the target.

- **Roles and gates.** `admin`, `reseller`, `customer` and the derived `additional_user` are enforced on the server. A route × role test (`cmd/panel/route_role_matrix_test.go`) reads every route registered in `cmd/panel/main.go` and checks its gate for an anonymous visitor and each role; a route registered without a row fails the test. Account routes are additionally driven with their real handlers (`TestAccountRoutesRoleMatrix`).
- **Plans.** The administrator's plans are global. A reseller creates its own plans (`service_plans.owner_id`); it sees global plans and its own, edits only its own, and its plans never promise more than its pool. A reseller's plan can be given only to that reseller's customers.
- **0 means unlimited** for domains, databases, mailboxes and disk — in the plan form, the plan validation, every quota check and every usage answer.
- **Plan change after creation** (`PUT /api/v1/users/{id}/plan`, admin; reseller for its own customers). Limits are re-copied from the plan. A plan below current usage is refused with the list of what is over (`PLAN_BELOW_USAGE`) until the operator confirms; then nothing is deleted, and new items of the over kinds are refused with guidance (`QUOTA_*_EXCEEDED`, pointing at My plan).
- **Reseller pool** (`reseller_pools`, set by the administrator at `/api/v1/users/{id}/pool` or when creating the reseller): total domains, databases, mailboxes, disk and number of customers over the reseller's tree. The pool limits *allocation* (the sum of plan limits handed out), so it cannot be oversold. Exceeding it is refused with `RESELLER_POOL_EXCEEDED` / `RESELLER_CUSTOMER_LIMIT`. A pool lowered below what is handed out is saved with a warning; nothing is taken away. A reseller without a pool row has no limits (every reseller from before alpha.114).
- **Suspend/resume.** A reseller suspends and resumes its own customers; the administrator any account. Both are audited (`user.suspend`, `user.resume`).
- **My plan** (`GET /api/v1/my/subscription`, a card on the Dashboard and on My account): plan, limits, live usage (the same numbers the quota checks use), whom to contact for a larger plan, and for a reseller its pool. Team members do not see it.
- **"Log in as" is accountable.** The session records its operator (`sessions.impersonator_id`). Every audit row written inside it names the real person in `user_id` and the account in `acting_as`; `impersonate.start` and `impersonate.stop` are recorded. The session ends when the operator is suspended, changes password, or may no longer manage the account. Nested "Log in as" is refused. A persistent bar says "Viewing as X" with the way back.
- **Password change** ends every session of that person, including "Log in as" sessions they opened as an operator.

**Not implemented yet:** traffic (bandwidth) is stored but **not measured**, so no traffic limit is applied and the screens say so; subscription suspension/expiry effects, prices, periods and payments; the reseller suspension "chain rule"; OS-level disk enforcement (disk is a soft gate at creation, from cached measurements); the per-capability additional-user model beyond the existing team-member grants.

---

## Deliberate non-goals (simplicity)

- ❌ **Arbitrary nesting** (resellers under resellers under resellers). The chain is fixed at four levels. A "distributor" tier above resellers can be added later *only if* real demand appears — until then, the administrator fills that role.
- ❌ **Custom roles with hand-built permission sets for customers/resellers.** Only additional users are granular; everyone else is their role. This is the single biggest source of cPanel/Plesk confusion, and we refuse it on purpose.
- ❌ **Per-record ACLs finer than "one domain."** The smallest scope is a domain.
