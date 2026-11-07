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
- Creation is exclusive: a name the engine already has changes nothing.
  Removal takes away the user and its rights and never the database: MariaDB
  `DROP USER` on every host; PostgreSQL `REASSIGN OWNED BY` the user to the
  database's owner, `DROP OWNED BY`, `DROP ROLE` (a table the user made stays,
  owned by the owner). The user made with the database goes only with it.
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
