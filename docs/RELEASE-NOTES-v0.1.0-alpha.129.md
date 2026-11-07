# v0.1.0-alpha.129

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.129.tr.md)

Databases get the everyday work they were missing: a new password for a
database user, extra users that only read or that read and write, a guide
for desktop tools, and clearer cards. A site's customer can connect to their
database from their own computer, only from addresses they add, always
encrypted, once the plan allows it. This release also fixes what the lab
found when it ran alpha.128 on Debian 12, Ubuntu 22.04 and Arch Linux; the
most important fix lets Ubuntu 22.04 servers update again. **Only the owner
updates installed panels, from the panel's own update screen.**

## Databases

- **New password for a database user**, on the domain's Databases tab and
  on the Databases page: the shared password field (generate, show/hide,
  copy, strength), the database user rule (at least 12 characters, upper and
  lower case and a special character), and the server's steps while it
  runs. MariaDB changes the password on every host of the account;
  PostgreSQL stores it on the role's own cluster (SCRAM). The last step says
  what happens to the site: a WordPress or catalogue application that
  CelikPanel installed gets the new password in its configuration; for
  anything else the screen says to enter it there by hand.
- **More users for one database:** "read only" or "read and write". A
  read-only user reads every schema of the database, including tables that
  the owner or a read-and-write user makes later, and cannot change data.
  Removing such a user never touches the database.
- **Connection guide** under "Connection details": host, port, database and
  user for the site; for pgAdmin, DBeaver and HeidiSQL the fields to type.
  The administrator also gets the SSH tunnel route.
- **Cards:** each database shows its engine with its own mark and colour,
  the real engine name (a MariaDB database is no longer labelled "MySQL"),
  size, tables and open connections in three boxes, and an "Import" button
  that stays a normal, disabled button until a file is chosen.
- **Export file names carry the engine** (`<name>-<engine>-<time>.sql.gz`),
  so two databases of different engines with the same name never overwrite
  each other's download.
- **Safety:** creating a database or user no longer takes over one that
  already exists on the engine, whether CelikPanel made it or not; the
  answer names what is taken (`DATABASE_NAME_TAKEN`). Before this release
  the account-level create could adopt an existing database and grant the
  new user on it.

## Connecting to a database from outside

The owner's decision of 7 November 2026: off by default; the customer
connects only from addresses they added; connections are always encrypted;
wrong passwords are slowed down; a plan permission grants it.

- **A plan permission (`database_remote`)**, off until the administrator
  offers it. Withdrawing it closes everything that was open.
- **A rule is one database, one user and one address.** At most 10 addresses
  per user. Whole-internet, wide (wider than /24 for IPv4 or /64 for IPv6),
  local, link-local and multicast addresses are refused.
- **PostgreSQL:** `hostssl` lines with SCRAM in a block of `pg_hba.conf` that
  CelikPanel keeps; an owner's own network line elsewhere in that file means
  CelikPanel opens nothing on that cluster and names the line.
- **MariaDB:** a separate account for each address, with `REQUIRE SSL`, the
  same password, rights and connection limit as the local account, and the
  same lock when the site is suspended.
- **Firewall:** the database port is open only to the listed addresses, and
  only while CelikPanel's filter is proven loaded; a guard reloads the
  filter if it disappears.
- **Waiting, with the reason:** a rule waits until TLS is on, the engine
  listens on the network after a restart (the administrator restarts it in
  Components) and wrong passwords are slowed down (MariaDB: CelikPanel's
  Fail2ban jail; PostgreSQL: `auth_delay`). The screen says what is missing
  and who does it.
- **Client settings** to type into pgAdmin, DBeaver and HeidiSQL, including
  "SSL mode: require".
- Remote API tokens cannot change these rules. An independent security
  review checked the feature three times before release; every finding was
  fixed (docs/DATABASE-REMOTE-ACCESS.md).

## Mail

- **A reopened mailbox can be stopped again within the same hour.** In
  alpha.128 a mailbox reopened by a password change could not be paused
  again until the next hour. Now only messages after the change count
  within that hour, and a second spike pauses it again and notifies at once.
- **Debian 12 and Ubuntu 22.04:** a changed sending limit did not apply
  until postfwd was restarted by hand, and a reopened mailbox stayed refused
  for about four minutes. postfwd 1.x is now restarted to take changed
  rules, keeps its hourly counts, and mail keeps flowing meanwhile.

## Deleting a domain

- **A deletion waiting for DNS finishes by itself**, also when it was
  started with a remote API token: the answer says so and gives a status
  address (`/api/v1/domains/{id}/deletion-status`) that tokens may read. A
  second press starts nothing new.
- **A finishing step that keeps failing** is tried again after 5 minutes, 15
  minutes and 1 hour, then every 6 hours; from the third failure the status
  names the step and the error, with "Retry this deletion"; one notice per
  run of failures.

## Fixes from the lab on Debian 12, Ubuntu 22.04 and Arch

- **Ubuntu 22.04 servers can update again.** Since alpha.123, every update
  on Ubuntu 22.04 stopped at "panel could not be frozen" (systemd 249 does
  not know an option the update used); the server stayed healthy on its
  version. The update always runs the new release's own scripts, so an
  Ubuntu 22.04 server on alpha.123–128 updates to 129 normally. Automatic
  rollback on Ubuntu 22.04 is fixed the same way.
- **DNS is never stopped because a check did not finish.** On a busy Debian
  12 server a slow package check made the panel stop BIND. A check that runs
  out of time is now "unknown": it is asked again (5, 15 and 30 seconds
  later) and never treated as a failure. Setup and the DNS page now show a
  stopped DNS engine with the one command that starts it.
- **Arch Linux:** MariaDB statements use the `mariadb` client (the `mysql`
  name now prints a warning that broke the database size view and the
  clean-up of a refused site copy).
- **Site copy:** when the copy's database cannot be removed, the panel keeps
  its records and shows it, instead of forgetting a database that still
  exists.

## Lab results

_To be filled in after the lab round._

## Known limits

- **Remote database access:**
  - the engine starts listening on the network only after the
    administrator restarts it;
  - withdrawing the plan permission closes access at the next apply, up to
    about 35 seconds;
  - table, column or role grants an administrator adds by hand to a remote
    MariaDB account are not removed by CelikPanel;
  - MariaDB accepts single IPv6 addresses only, no IPv6 ranges;
  - an owner's own `shared_preload_libraries` in PostgreSQL is never
    rewritten; the administrator gets the exact value to set for
    `auth_delay`;
  - the Fail2ban rule that reads MariaDB's journal is checked by automatic
    tests; the lab round checks it on real systems.
- **Database users:** what a read-only PostgreSQL user can and cannot see is
  listed in docs/DATABASE-USERS.md. Moving a database to another PostgreSQL
  cluster reuses a role of the same name on the target.
- **DNS on a fresh setup:** if a check still cannot finish after its
  retries during the very first DNS configuration, setup goes back to "no
  DNS engine" (nothing was serving yet), shows the reason and checks again
  by itself.
- **Applications:** when the sandbox of a Node.js or service application
  cannot be read, the application is stopped rather than run unconfined.
- Earlier limits still apply (see alpha.128).

## Evidence

Migrations (both additive):

- **073 `mail_outgoing_resume_baseline`:** two columns on `mailbox_outgoing`
  (`resume_base_recipients`, `resume_base_refusals`, default 0).
- **074 `database_remote_rules`:** the `database_remote_rules` table.

As with every migration, alpha.128 refuses a database that has these
migrations, and going back means restoring the snapshot taken before the
update. Release sequence 128 → 129.

Lab evidence for the alpha.128 runs on Debian 12, Ubuntu 22.04 and Arch:
`deploy/e2e/dns-pair-acceptance/evidence/a128-other-systems/` on the lab
branch.
