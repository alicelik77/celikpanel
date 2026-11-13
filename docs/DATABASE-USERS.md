# Database users, connections and names (alpha.129)

What a database's users can do, how people connect, and how names stay
unique. Written with the alpha.129 database work; the code comments name the
same rules.

## A user's new password

- Domain's Databases tab: every user of a database has "New password";
  the Databases page's key button opens the same form. The password is
  typed or generated in the shared password field (database user rule:
  12+ characters, upper, lower, special). The server never sends it back.
- Engine: MariaDB `ALTER USER` on every host the account has (CelikPanel
  makes `'name'@'localhost'`; a host the owner added keeps the same
  password); PostgreSQL `ALTER ROLE … PASSWORD` on the role's own cluster,
  stored as a SCRAM-SHA-256 verifier. The user's open sessions are ended.
- Sites using the old password: a WordPress or catalogue application that
  CelikPanel installed gets the new one in its configuration file through
  the existing, file-checked mechanism (`wp-config.php` only when it still
  names this database and user). For anything else the panel says plainly
  that its configuration must get the new password; it does not rewrite
  other files.

## More users for one database

- "Read only" or "read and write", named `<database>_<name>` (name: 1–16
  lowercase letters, digits, `_`, not `user`; at most 63 characters in all).
- Recorded with the Databases page's model: one `database_users` row and one
  grant (`SELECT` for read only, `ALL` for read and write). No schema change.
- MariaDB: `SELECT` or `ALL PRIVILEGES` on that database only, no
  `GRANT OPTION`.
- PostgreSQL. "Every schema" means every schema of the database except the
  system's (`pg_*`, `information_schema`) at the moment the user is added;
  "later" means objects made afterwards by the named roles anywhere in the
  database, new schemas included (`ALTER DEFAULT PRIVILEGES FOR ROLE …`
  without `IN SCHEMA`).
  - read only: `CONNECT`; in every schema `USAGE` and `SELECT` on its tables
    and sequences; later, `SELECT` on tables and sequences and `USAGE` on
    schemas made by the database's owner and by every read-and-write user.
  - read and write: `CONNECT`, `TEMPORARY`; in every schema `USAGE`, `CREATE`,
    all rights on tables and sequences, `EXECUTE` on functions; later, all
    rights both ways between this user and the owner and the other
    read-and-write users, and `SELECT` (`USAGE` on schemas) on what this user
    makes for every read-only user.
  - Kept up to date when users are added: a new reader is given the owner's
    and every writer's later objects; a new writer gives its later objects to
    every reader and every other writer. When a writer is removed its objects
    become the owner's (`REASSIGN OWNED`) and keep their grants, so readers
    go on reading them.
  - Not covered: objects made later by a role that is not the owner or an
    added user (for example the owner's administrator account), objects in a
    schema the owner makes with explicit `REVOKE`s, and schemas or objects
    made by the owner and then given to another role. A schema made later by
    the owner is covered; one made by another role is not.
  - Never `SUPERUSER`, `CREATEDB`, `CREATEROLE`, `REPLICATION`, `BYPASSRLS`.
  - Found in the alpha.133 lab (open): a user added to another database got
    rights only on tables CelikPanel's own account made there later, not on
    the tables the database's own user owns. The statements above describe
    the intent; this gap is not fixed yet.
- Creation is exclusive: a name the engine already has changes nothing.
  Removal takes away the user and its rights and never the database: MariaDB
  `DROP USER` on every host; PostgreSQL `REASSIGN OWNED BY` the user to the
  database's owner, `DROP OWNED BY`, `DROP ROLE` (a table the user made stays,
  owned by the owner). The user made with the database goes only with it.
  Open (alpha.133): when something else still uses the user, its account
  stays on the engine with no rights, yet the result says the user was
  removed; a hand-made account of that name keeps its rights on the live
  database (by design) and the result does not mention it.
- Deleting a database (alpha.133, the same for the left-menu Databases page
  and the domain's Databases section): the database is dropped first (on
  PostgreSQL its open sessions are ended first), then every user recorded for
  it that nothing else uses, with the removal above; the records are removed
  last, in one transaction, so an interrupted deletion keeps the database
  listed and the same Delete finishes it. A user stays when another database
  uses the same engine account (the engine and the name, across every server
  record of that engine), when it is recorded for another server record or
  subscription, and when it is an engine's or CelikPanel's own account
  (`root`, `postgres`, `pg_*`, `debian-sys-maint`, `celikpanel_admin`, a name
  CelikPanel does not manage). On this machine the Agent also keeps an
  account its owner gave rights by hand - on another database, globally, on
  a routine, through a role or on a host CelikPanel did not create (MariaDB);
  owning or holding anything elsewhere, or a membership (PostgreSQL) - and
  takes away only CelikPanel's grant on this database; the answer says it is
  kept on the engine and its record goes. Giving an existing account to a
  database waits while a deletion on the same engine runs (at most 10 s,
  then DATABASE_ENGINE_BUSY; a deletion waits at most 30 s for such a step).
  On an engine reached with the record's own account (another machine, an
  administrator's own record) only the database is dropped and every user
  stays ("stays": "server"); removing users there needs a rights check that
  account may not be able to perform (open). The PostgreSQL check does not
  count the membership PostgreSQL 16+ grants a role's creator by itself;
  on MariaDB a routine in the deleted database is not a right elsewhere, the
  role table is asked for only where it exists, and a check that fails keeps
  the account ("unchecked"). A user that stays - kept for its owner's
  rights, or shared with another database - loses every right that names the
  deleted database, on every listed account of its name (hand-made ones too,
  never the empty host of a MariaDB role of the same name): MariaDB keeps such
  rights after DROP DATABASE (KB) and a later database of the same name would
  inherit them. The Agent first asks the engine that no database of that name
  exists (and every statement repeats that condition); then it deletes the
  rows of mysql.db by the name's two pattern forms (as written, or with
  `_`/`%` escaped) and of tables_priv, columns_priv and procs_priv by the
  literal name, flushes, and reads what the server has loaded
  (information_schema for those accounts, procs_priv) - anything left is a
  partial result ("rights"). A pattern that covers several databases, a
  server-wide right and a role are left alone. PostgreSQL leaves nothing that
  names a dropped database (dropdb removes its role settings and shared
  dependencies with the pg_database row). Open: an account with another name,
  made outside CelikPanel, keeps its rights on a deleted database, and so
  does a hand-made account under the name of a user removed from the
  database earlier (alpha.134 removes every right naming a deleted database
  from every account); after a deletion that kept a user's accounts, a
  database of the same name cannot be created again until those accounts
  are removed on the engine, and the refusal does not yet say so; database
  rights are still given by name pattern (`_` = any one character), exact
  names come in alpha.134.
- Plans: there is no database-user quota in the plans
  (docs/PLANS-AND-RIGHTS.md), so none is counted; the databases permission
  and the engine offering apply, and team members need "manage".

## Connecting

Databases listen on the server only. "Connection details" gives the site's
values (localhost, port, database, user). From a person's own computer:

- the administrator: an SSH tunnel (SSH host = the panel's address, port 22,
  root or their administrator account; then 127.0.0.1 and the database's
  port) with the place of the SSH settings in pgAdmin, DBeaver, HeidiSQL;
- a reseller or customer: not available yet; phpMyAdmin/phpPgAdmin in the
  panel (or "your provider installs it").

Connections from outside wait for the owner's decision; nothing here opens
them.

## Names and files

Audit of 7 November 2026:

- Export downloads were named `<database>-<minute>.sql.gz`: a MariaDB and a
  PostgreSQL database of the same name exported in the same minute got the
  same file name. Now `<database>-<engine>-<minute>.sql.gz`.
- Safety copies before an import and backup archives are named by the
  database's id (`database-<id>-…cpbak`, `payload/databases/<id>.sql.gz`),
  and the manifest records name and engine for the restore: no collision,
  unchanged. Copy and transfer work files live in random directories.
- Account-level (domain-less) databases are `s<subscription>_<name>`, domain
  databases `<domain with . and - as _>_<name>`. Both schemes can produce the
  same name on one engine (`s5.com` + `x` and subscription 5's `com_x`; the
  domains `a.b` and `a-b`), and every subscription has its own record of the
  same engine. The account-level create took over an existing database or
  user ("IF NOT EXISTS"; an existing PostgreSQL role was success) and granted
  its new user on it; the domain create compared only its own record. Now
  both compare every record of the same engine (localhost and 127.0.0.1 are
  one engine) and refuse with `DATABASE_NAME_TAKEN`, and the panel-side
  drivers create exclusively. Different engines may share a name.
- The Agent's create (`Agent.CreateDatabase`: domain databases, database
  copies, catalogue and WordPress installs, WordPress clones, site copies)
  is exclusive on both engines. PostgreSQL used to take "role already
  exists" as success and made the new database owned by that role; now an
  existing role or database changes nothing and the answer names it
  (`NameTaken`, shown as `DATABASE_NAME_TAKEN`). A create that carries an
  operation's ownership proof marks its role (`COMMENT ON ROLE` with the
  proof's hash), so a replay of the same operation resumes on its own role;
  another operation or a plain create never adopts it. A role made by a
  failed create is dropped again. Replays of the same request are answered
  by the panel's request identity before the Agent.
- Not changed: moving a database to another PostgreSQL cluster
  (`CopyPostgreSQLDatabaseV1`) creates the roles the target lacks and reuses
  roles of the same name already on the target, by design of the move.
