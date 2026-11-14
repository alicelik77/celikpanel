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
  the account ("unchecked"). Every right that names the deleted database
  goes from EVERY grantee (alpha.134): the database's own users that stay,
  accounts made by hand under any name, and roles (a role's rows have the
  empty host) - the accounts and roles themselves stay. MariaDB keeps such
  rights after DROP DATABASE (KB) and a later database of the same name would
  hand its tables to whoever holds them (to a role's members too). The Agent
  first asks the engine that no database of that name exists (and every
  statement repeats that condition); then it deletes the rows of mysql.db
  of the name's two forms (as written, and exact with `_` escaped) row by
  row, and of tables_priv, columns_priv and procs_priv by the literal name -
  every value as a hexadecimal literal, so the server's sql_mode does not
  matter - runs FLUSH PRIVILEGES on its own, and reads what the server has
  loaded and stored (information_schema for every grantee, and the grant
  tables, roles' column rights included). Whenever something is still
  counted it flushes once more and counts again (a FLUSH that failed after
  the DELETEs, on this press or an earlier one, is repaired this way);
  anything still left keeps the database listed ("press Delete again"). A
  pattern that merely matches the name (`shop\_%`, `sh_p`) is another text
  and is left alone; so are server-wide rights. A row in the name's raw form
  (a pattern) that ALSO reaches another existing database is kept, not
  deleted: it is the owner's (CelikPanel's own rows are exact after the
  rewrite below); the answer lists it with what it still reaches. The
  user steps report whose rights they removed too, so the answer and the
  audit row name every grantee once. Only an administrator is told whose
  rights went, how many, which were kept and what may remain; a customer's
  answer says nothing about other accounts. On a MariaDB reached with the
  record's own account the same removal is tried with that account; when
  it cannot, the deletion still finishes and an administrator is told
  rights may remain there for that server's administrator to remove.
  PostgreSQL leaves nothing that names a dropped database (dropdb removes its
  role settings and shared dependencies with the pg_database row). Open:
  after a deletion that kept a user's accounts, a database of the same name
  cannot be created again until those accounts are removed on the engine,
  and the refusal does not yet say so.
- Rights by exact name (alpha.134). MariaDB reads the database name of a
  database-level grant (`GRANT … ON name.*`) as a pattern: `_` is any one
  character, `%` any run (MySQL 8.0 manual, GRANT, "Object Quoting
  Guidelines"; the alpha.133 lab measured it on MariaDB 11.8.6 and 10.11.14:
  a grant on `labd_1__wp`.* let its user create a table in labd_12_wp).
  Every name CelikPanel makes holds `_`, so subscription 5's `s5__x` covered
  subscription 51's `s51_x`. Since alpha.134 every grant is written by the
  exact name (`s5\_\_x`.*: creating a database or a user, giving a user to
  another database, remote copies), and every place that takes a right away
  or counts it handles both forms. Table, column and routine grants name the
  database literally and do not change.
  - The grants written before are rewritten on each MariaDB the panel
    manages (this machine's, through the Agent; a registered one, with its
    record's account), only those the panel's records prove its own: a
    recorded user given a recorded database, on localhost or a remote copy
    the Agent's remote access record lists. For each such row the
    before-image (account, host, stored name, privileges) is written to
    `database_grant_forms` (migration 079) first; then the same privileges
    are granted by the exact name and checked, and the pattern form is
    revoked and checked - access is never lost, and an interruption at any
    step is continued from the engine on the next round. A row that is not
    what was captured is captured again first; a row failing three times
    waits a day (the page lists it with the statement to narrow it by
    hand). A row holding the grant option, or a privilege CelikPanel never
    gives, is the owner's: listed, not changed. On a server with a privilege
    column CelikPanel cannot name, the privileges are written word by word,
    never ALL PRIVILEGES (which would also give that one). Every rewritten
    row gets an audit row with the other databases its pattern reached ("it
    no longer reaches …"), and the MariaDB page lists them. The engine's gate
    (the one a database deletion or a grant takes) is taken for one row at a
    time. An Agent of another build, or one that does not answer, makes the
    engine wait; it is not counted against a row. A remote copy's
    pattern-form grant is rewritten by the remote access apply only while
    this rewrite may run, and never at the same moment as a row of it.
  - Every GRANT CelikPanel writes on MariaDB runs with NO_AUTO_CREATE_USER
    in its session and is read back: the stored row must hold exactly the
    exact form. Measured on MariaDB 11.8.6: without the mode, a GRANT to an
    account dropped meanwhile creates it again with an empty password; with
    it the GRANT is refused (ERROR 1133). MariaDB 10.11 and 11.8 have it in
    their default sql_mode; MySQL 8 removed it, so it is not sent there.
  - Names whose exact form is longer than 64 characters (each `_` counts
    twice). Measured on MariaDB 11.8.6: for a 64-character name with eleven
    `_` (75 characters exact) and for a 60-character one with six `_` (66;
    CelikPanel's own names are at most 63 characters), GRANT on the exact
    form is accepted, SHOW GRANTS shows it whole, but mysql.db.Db keeps it
    cut to 64 characters - after the next
    reload a right on another, shorter name. So a new database of such a
    name is refused with a plain sentence (DATABASE_NAME_TOO_LONG), a user
    is not given an existing one by the exact form, and existing pattern
    rows of such a name stay as they are and are listed with that reason
    (remote copies keep the same pattern form as the local account).
  - Rows the rewrite cannot record (the anonymous account ''@host, a
    pattern that reaches hundreds of databases) are recorded and listed (at
    most 50 database names a row, the rest counted); a failure to record
    makes the engine "blocked", never silent.
  - Rows that cannot be proven CelikPanel's (made by hand, or left by a
    database CelikPanel no longer records) are never changed. The MariaDB
    page lists them by why they are left (said once above the rows) with the
    databases they reach now and, under Details, a statement and what it
    does: narrow it to one database's exact name, or - for a wildcard on
    purpose or a name too long for its exact form - remove the right
    entirely. Roles are named as roles, the anonymous account as such. The
    administrators get one notification while such rows exist, worded by
    why they are left.
  - When it runs: in the panel's own loop (hourly), never within the first
    minutes after a start, never while the latest update attempt to this
    build is not verified (the release recovery observation says
    "succeeded", or none was recorded), never while the server is busy, on
    this machine only while the Agent is the same build. Until then the
    page says "waiting" and why. A refused credential, an unreachable
    server or an account that may not read or change the grant tables is
    "blocked" with that reason; each round asks again (read only) and
    continues when the cause is gone. A server that is not MariaDB (MySQL)
    gets exact new grants but no automatic rewrite: its rows are listed.
  - Going back to alpha.133 (snapshot restore of the panel database,
    schema 079 back to 077: two steps, this migration and 078): the
    exact-form grants stay on the engine and keep working. alpha.133 code
    then (documented, not built): drops both forms when a database is
    deleted; but removing an added user from a live database looks for the
    pattern form only, so the exact grant stays (the account is kept with
    it) or a registered server's revoke answers "no such grant"; and remote
    copies of such a user are not refreshed (their grant name is refused).
    Updating to alpha.134 again continues from the engine.
  - Open: a one-click narrowing for the listed rows; the exact behaviour of
    `SHOW GRANTS`/`mysql.db` on MySQL 8 with `partial_revokes` (no lab).
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
