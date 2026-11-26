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
    go on reading them - except on a database owned by CelikPanel's own
    account (`celikpanel_admin`), which never receives them: there a user
    that owns anything is not removed (see "Privileged database accounts and
    the Agent" below).
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
  owned by the owner; on a database owned by `celikpanel_admin` no `REASSIGN`
  at all - `DROP OWNED BY` only, or a refusal when the user owns something
  there). The user made with the database goes only with it.
  Open (alpha.133): when something else still uses the user, its account
  stays on the engine with no rights, yet the result says the user was
  removed; a hand-made account of that name keeps its rights on the live
  database (by design) and the result does not mention it.
- Deleting a database (alpha.133, the same for the left-menu Databases page
  and the domain's Databases section): the database is dropped first (on
  PostgreSQL its open sessions are ended first), then every user recorded for
  it that nothing else uses, with the removal above; the records are removed
  last, in one transaction, so an interrupted deletion keeps the database
  listed and the same Delete finishes it. The drop happens at most once
  (alpha.140): before it, the deletion reads the database's identity on the
  engine (PostgreSQL: its oid; this machine's MariaDB: the database
  directory's device, inode, inode generation and birth time) and keeps it
  in a marker of the record (panel_settings `database_delete_v1:<id>`); the
  Agent drops only while the engine shows that identity. A later press
  after a lost answer, a stop or a step that did not finish never drops a
  database of the same name made in the meantime: it finishes the users and
  records, and while such a database is there it leaves it and its users
  alone ("exists": the server administrator decides about it, then Delete
  finishes). Without an identity (no birth times on the filesystem; a
  MariaDB reached with a record's own account) a later press drops nothing
  it cannot tell apart (DATABASE_DELETE_UNVERIFIED). The marker goes with
  the records. A user stays when another database
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
  deleted, when its account is not on record for the deleted database; the
  answer lists it (once) with what it still reaches. The accounts on record
  for the deleted database (every one, also one another database shares, on
  this machine's MariaDB or on a registered one) lose their rows naming it
  in both forms, from every host, even where the pattern reaches another database - as on
  alpha.133; CelikPanel gave them (second review: before the rewrite runs,
  such a row is CelikPanel's own pattern and must not outlive the deletion). The
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
    engine wait; it is not counted against a row. Inside the gate the pair
    is read again from the records and the Agent checks that the database
    exists (BINARY) before it writes the exact right: a database deleted
    since the scan leaves the round ("database_gone") and gets no right. A
    remote copy's pattern-form grant is rewritten by the remote access apply
    only while this rewrite may run, and never at the same moment as a row
    of it; while it is held, a change of rules is still applied to the copy
    in the pattern form (added, narrowed or taken away), as on alpha.133.
    A row CelikPanel records whose database no longer exists on the engine
    (dropped by hand) is never rewritten nor marked done: it is listed and
    counted as left ("the database no longer exists, the record still
    does"), with what to do - delete the record in Databases, which runs the
    removal, or create the database again; the round deletes no engine row.
    When CelikPanel's own records cannot be read, the engine shows that the
    round's result is unknown (records_unread). While an engine waits (a
    panel update not confirmed or not readable, an Agent that does not
    answer or is another build, unread records) or is blocked, for one
    cause, for more than 24 hours, the administrators get one notification
    with the cause, who acts and what next; it is cleared when the wait
    ends, and a later, separate wait is told again. A listed pattern-form
    right goes from the list when its grant is revoked or its database is
    deleted. A row holding a privilege
    CelikPanel cannot name is offered only its removal, never a narrowing
    that would drop that privilege.
  - Every GRANT CelikPanel writes on MariaDB runs with NO_AUTO_CREATE_USER
    in its session and is read back: the stored row must hold exactly the
    exact form. The length rule below, checked before any statement, is what
    prevents a cut-short right; the read-back is a second line. An account
    that may not read mysql.db (a registered server's restricted account;
    ERROR 1142) cannot read a right back: the right stands on the length
    rule, nothing is refused, and the engine's state records once that rights
    there cannot be verified (the MariaDB page says so and what the account
    would need: SELECT on mysql.db); a later verified read-back clears it. A
    read-back that failed in any other way is an unknown result, never taken
    for either answer. Measured on MariaDB 11.8.6: without the mode, a GRANT to an
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
    name is refused before anything is made (DATABASE_NAME_TOO_LONG: choose
    a name at least N characters shorter, or with fewer underscores), and a
    database copy checks it before the copy starts. A cPanel import names
    such databases in its preview, with why and what to do (a shorter name
    in cPanel, or create it here under a shorter name), and leaves them out:
    nothing is created under such a name. Such an import is complete with
    those databases left out: the result says so and names them, and it is
    audited as import.cpanel.complete_with_skips with their names. Names CelikPanel makes
    itself (application installs, WordPress installs and clones, site
    copies) are shortened at their base until they fit, so they never fail
    late. An existing database of such a name keeps working: a user given
    to it gets the pattern form it always had, stored whole and read back
    (never an exact form cut short); its pattern rows are listed with that
    reason, and remote copies keep the same form as the local account.
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
    exact-form grants stay on the engine and keep working. What alpha.133
    then does with them (read from the v0.1.0-alpha.133 code; the first
    point measured in the alpha.134 lab):
    - Removing an added user from a live domain database (this machine's
      MariaDB, through the Agent) works: alpha.133 counts both forms of
      the name as rights on this database, so an account holding only
      CelikPanel's rights is dropped whole on every host, its exact grant
      with it (lab: `{"kept_on_engine":false,"status":"removed"}`). Only an
      account alpha.133 keeps - one with rights elsewhere, a host CelikPanel
      did not create, or a check that failed - has its grant looked up in
      the pattern form alone: the exact grant on this database then stays
      with the account, and the answer says the account was kept.
    - Taking a right away through the panel's own driver (the API's grant
      removal, DELETE /api/v1/grants/{id}, which no alpha.133 screen calls,
      on any MariaDB record, this machine's included) revokes the pattern
      form only: a right held in the exact form is answered "no such grant"
      (ERROR 1141), and the right and its record stay.
    - Deleting a database on this machine's MariaDB (through the Agent)
      removes the rights of both forms; on a MariaDB reached with its
      record's own account (a registered one) alpha.133 drops only the
      database, and every right naming it stays.
    - Remote copies of such a user are not refreshed (alpha.133 refuses the
      exact name's backslash in the copy's grant).
    Updating to alpha.134 again continues from the engine.
  - Open: a one-click narrowing for the listed rows; the exact behaviour of
    `SHOW GRANTS`/`mysql.db` on MySQL 8 with `partial_revokes` (no lab).
- Plans: there is no database-user quota in the plans
  (docs/PLANS-AND-RIGHTS.md), so none is counted; the databases permission
  and the engine offering apply, and team members need "manage".

## SQL from a file (20 November 2026)

SQL that comes from a file runs with the rights of the target database's own
account and nothing more: a backup's dump (made on this server or brought back
from a remote destination), the safety copy put back after a failed restore,
the dump of a database, site or WordPress copy, a cPanel archive's dump and a
file a customer imports. Its text is the customer's even when CelikPanel made
the dump. Before, a restore and a copy ran it through `mysql` as root without
sandbox mode or through `psql` as the postgres account, so a line of the file
could reach a shell or the database server's administrator rights.

- One mechanism (`cmd/agent/database_owner_load.go`) loads every such file.
  - MariaDB: the client with `--no-defaults --sandbox` (no shell, no local
    files), signed in as the database's own user, under the site's own system
    account (`nobody` for a domain without hosting), never root. DEFINER
    clauses of views, triggers and routines are written as `CURRENT_USER`: the
    object belongs to the database's own user (a view an added user made comes
    back as the database user's). The cPanel import, whose database has no user
    of its own yet, loads through a temporary account with every right on the
    new database only (`celikpanel_load_…`), removed afterwards.
  - PostgreSQL: no psql. The Agent signs in as the database's owner role over
    127.0.0.1 and sends the statements itself (the import's loader), in one
    transaction after `DROP OWNED BY CURRENT_USER`; psql commands are refused,
    `SET ROLE`/`RESET ROLE`, `ALTER … OWNER TO` and `COMMENT ON EXTENSION` are
    left out.
- The account is the user recorded first for the database, with its stored
  password (as the import). It is proved before the safety backup and before
  anything is written. A restore is refused with what to do when:
  - the database has no user with a stored password, or the server refuses it
    (it was changed outside the panel), or on PostgreSQL that user is not the
    database's owner: set a new password for the user on the Databases page,
    then restore again (`BACKUP_RESTORE_DATABASE_ACCOUNT`);
  - the server's MariaDB client has no sandbox mode (older than 10.5.25,
    10.6.18, 10.11.8): the administrator updates the MariaDB packages
    (`BACKUP_RESTORE_CLIENT_UNSAFE`); a client without it is never replaced by
    one that would obey the file's client commands;
  - something only the server's administrator may create here:
    MariaDB with binary logging and `log_bin_trust_function_creators` off and a
    dump holding stored functions or triggers (the dump and the safety copy
    are read first), or a PostgreSQL user that is an administrator role
    (`BACKUP_RESTORE_NEEDS_ADMINISTRATOR`).
- What the database's own rights cannot replay (measured on MariaDB 11.8.6 and
  PostgreSQL 17.12 with dumps made by the panel's own backup code): a
  PostgreSQL extension only the administrator may create, when the target does
  not have it yet (a copy into a new database, a database the extension was
  removed from): the load stops, the transaction leaves the database as it
  was, the engine's reason names the extension. An extension the
  administrator made in the database before is kept and the restore goes on.
- Not changed by this, still open: on PostgreSQL a restore drops and
  recreates the owner's objects, so the rights the panel gave to the
  database's added users on them (and their default privileges) do not come
  back with the restore (also before; measured on 17.12). MariaDB events are
  not in the backup's dump (no `--events`).
- Every session the engine's administrator opens on PostgreSQL (psql,
  pg_dump, pg_restore, as postgres or as a cluster's owner) starts with
  `search_path = pg_catalog, pg_temp` given as a connection option
  (`cmd/agent/pg_admin_session.go`), and psql never reads a start-up file: a
  database's owner can no longer place functions or operators that such a
  session would run with the administrator's rights. A test fails when a
  PostgreSQL client is started another way.

## Moving a PostgreSQL database to another cluster

A move (PostgreSQL page → a cluster's Move) still loads the copy with
`pg_restore` as the cluster's administrator, as in alpha.141: code a
database's user placed in a table (a CHECK constraint, a domain's constraint,
a stored generated column) runs with the administrator's rights while the
rows are loaded. Only the administrator can start a move; do not move a
database whose owner you do not trust. Loading the copy with its owners'
rights is an open item of the security series (ROADMAP).

## Old PostgreSQL databases closed to other accounts (20 November 2026)

alpha.141 makes every new PostgreSQL database closed to the server's other
roles. A database made before keeps PostgreSQL's default rights: every role
of the server may connect to it and make temporary tables in it, so one
customer's database user can sign in to another customer's database and see
how it is built (`cmd/panel/postgresql_public_access.go`,
`cmd/agent/pg_public_access.go`). MariaDB has no such default: nothing to do
there.

- What CelikPanel closes by itself: a database it records on this machine's
  clusters, only when nobody else can be seen using it: its own rights are
  still PostgreSQL's untouched default (`datacl` empty); no role outside its
  records for that database has any footprint inside it (owners and rights
  of relations, columns, schemas, functions, types, large objects,
  extensions, languages, operators and their classes and families,
  collations, conversions, text search objects, extended statistics,
  foreign data wrappers and servers, publications and subscriptions; default
  rights; user mappings; row security policy roles; per-database role
  settings); every recorded user has its role; and its cluster has no
  monitoring login role (`pg_monitor` and the like) and no role with the
  `REPLICATION` attribute.
- The watch (the exact rule). Such a database is watched first. It is closed
  by itself at the first hourly round that comes at least 24 hours after its
  watch began and after at least 24 rounds have read it, each finding all of
  the above. The watch begins at the first reading that finds it so (a
  round, or the page's reading of a new scan). A round that cannot read it
  does not count. A change of its rights, of the roles inside it, of its
  owner or of its recorded users (and their read or read-and-write right)
  starts the count again. A session of a role outside its records seen
  connected to it in any reading (`pg_stat_activity`) ends the watch: the
  database is listed for the administrator with that role's name, also after
  the session has ended; so is a role the close's own last check finds
  connected (the close then changes nothing). Sessions are hourly snapshots:
  a role that connects only between two readings is not seen. The page says,
  for a watched database, since when and the time after which it closes by
  itself if no other role is seen connected at an hourly check; the
  administrator can close it at any time.
- The close is one transaction inside the database: it changes nothing unless
  the database is still as read; it gives `CONNECT` to the recorded users
  (and `TEMPORARY` to a read-and-write user; the owner keeps its rights as
  owner), takes `CONNECT` and `TEMPORARY` from `PUBLIC`, and reads back that
  `PUBLIC` holds neither and every recorded user and the owner may connect,
  before `COMMIT`. Role names and rights reach its checks as transaction
  settings, never inside a dollar-quoted body. Before its first statement
  that can wait it sets `lock_timeout = 30s` and `statement_timeout = 60s`
  for its transaction (`SET LOCAL`, which outranks the owner's own settings),
  so a session that holds the database's row makes it end inside the engine.
  The Agent ends a close (or undo) client still running after 3 minutes (the
  panel waits 10); its answer is then unknown, with the time it was ended.
- Every administrator session (psql, pg_dump, pg_restore, as `postgres` or a
  cluster's owner, so also alpha.141's restore and copy) pins, as connection
  options that outrank `ALTER DATABASE/ROLE ... SET`: `search_path`,
  `statement_timeout=0`, `lock_timeout=0`,
  `idle_in_transaction_session_timeout=0`, `default_transaction_read_only=off`,
  `role=none` and `exit_on_error=off` (PostgreSQL's own defaults: a short
  limit would cut the panel's long loads, so the bounded close sets its own
  inside its transaction). `idle_session_timeout` (PostgreSQL 14+) and
  `transaction_timeout` (17+) are reset by the closing sessions right after
  they connect, where the server knows them.
- Its answers are three (D-024): closed; verified not closed (the database
  was no longer as read, its read-back refused, it could not be entered, or
  psql stopped at a server error before `COMMIT` was acknowledged, without
  a fatal error or a lost connection, so the transaction was rolled back:
  a recorded user's missing role, a lock or statement timeout), with the
  database server's own message, which the page shows quoted after its own
  words; and unknown (anything else after `BEGIN`: a lost connection, an
  error after `COMMIT`, no read-back, or its client was ended at the
  deadline). On an unknown answer, or when the
  panel's call got no answer at all, the panel keeps its record ("sending"),
  sends nothing again, and reads the database only after the Agent confirms
  that no client of a close or undo of that database runs (its own, or a
  process with that close's `application_name`) and no session with that
  name is on the engine (a `COMMIT` already sent), from a scan that begins
  after that confirmation: closed (the record is finished, with who closed
  it); unchanged (a failure with the reason, never redone by itself); or
  still unknown (listed as such, never closed by itself). A watch that holds
  a close found not applied keeps that close; if the database is found
  closed later, it becomes that close's full record (when, by whom, the
  Undo), never a deleted one.
- A close that failed is kept with its reason and time, listed on the page
  and in Problems now with who acts and what to do; it is not tried every
  hour, only when the database's facts change or when the administrator
  presses Close. Closes, undos and the panel's records of one database are
  done one at a time under that database's own lock: the hourly round and an
  administrator never both act on it, and the second reads the first's
  result; with a close still possibly running, no second close is sent.
  Another database's close and "Read again" do not wait for it (a reading
  shows a database whose lock is held without changing its record). A round
  whose scan began before a record changed shows that database from the
  record and writes nothing for it.
- Everything else the panel can read is left exactly as it is and listed (Problems now, and the
  PostgreSQL page under "Databases other accounts can enter") with what was
  found (rights changed by hand, other roles inside, other roles seen
  connected, recorded users without a role, monitoring or replication roles,
  a failed close, an unknown result), what every account can do there now
  (connect, or only make temporary tables) and what closing changes; the
  administrator closes it there after reading that. The confirmation names
  the roles outside the records connected at the last reading. A database
  that cannot be read is shown on the page as not read, with the reason;
  after more than three rounds in a row it is also listed in Problems now
  and the alert mail (`postgresql_databases_unreadable`) with the reason and
  the step.
- Undo: on the PostgreSQL page beside each closed database (when, by whom),
  and as the plain statements for an owner without the panel (run as the
  postgres account). It gives `PUBLIC` back what it held and takes away
  exactly the rights the close gave; it is refused, changing nothing, when the
  database's rights changed since. Its unknown answer is settled from the
  database the same way, after the Agent confirms nothing of it runs; the
  record of a close is never deleted while the database may be closed. A database an administrator opened again stays on
  the page as the administrator's choice ("Opened again by an
  administrator"), out of Problems now and the alert mail, and is never
  closed again by itself. The rights written out as the default for the
  database's owner (`{=Tc/owner,owner=CTc/owner}`, what an undo's `GRANT`
  leaves) count as PostgreSQL's untouched default everywhere the panel asks
  (what was found, the watch, the opened-again entry); rights that are the
  default again after a close are said as such. Right after a round closed
  a database, the page (which reads the last scan) shows that close from
  its record.
- When: after the panel has been up a few minutes, an update of it is
  verified, the server is not busy and the Agent is the same build (the
  database-rights loop's admission), then hourly. A scan reads every recorded
  database of a cluster in one session; the page shows the last scan with its
  time and changes nothing; "Read again" starts one scan (a press while one
  runs waits for it; an administrator's action reads through the same one
  scan). A database or cluster that cannot be read is shown as not read,
  never as safe. Whether each database exists is asked in the session's
  first connection, before any database is entered. The Agent ends a scan
  after 45 seconds and keeps the facts of every database it read; the
  others are not read, with that reason. Measured on a private PostgreSQL 17.12: 300
  databases in one session in about 5 seconds (the panel's bound for the
  read is 60 seconds).
- Kept in `panel_settings` (no schema change):
  `postgresql_public_v1:<database id>` per database CelikPanel watches,
  closed or is closing (state, by whom, when, the rights before and after,
  what `PUBLIC` held, what the close gave; the watch's start, rounds and
  facts; the roles seen connected; the last failed close; when it was
  written; an undo whose answer is not settled; a close found not applied),
  removed with the database's record (and by the hourly round for any
  database that is gone), and `postgresql_public_list_v1` for the lists
  Problems now shows (with the rounds each unread database was not read).
- Not covered: registered PostgreSQL servers on other machines (their own
  administrator's), and the `postgres`/`template1` databases.

## Privileged database accounts and the Agent (alpha.149)

The Agent runs as root and, on the engines, as their superuser; the panel is
the less trusted side. No Agent call changes a database account the panel
names (a password, a lock, a drop, a revoke, limits, a database's owner as the
owner and recipient of objects) unless the Agent itself finds it an ordinary
account, read before the write and again in the write's own transaction
(PostgreSQL) or compound statement (MariaDB); a state that cannot be read
refuses. The rule is in `cmd/agent/database_account_guard.go`. A database's
owner passes the same rule; the one exception is CelikPanel's own
`celikpanel_admin` (DATABASE-ADMIN-ACCOUNT.md), while it is not a superuser,
has neither `REPLICATION` nor `BYPASSRLS`, and is a member of no `pg_*` role
and no superuser role. Even then nothing is ever handed to it: removing a user
added to a database `celikpanel_admin` owns sends no `REASSIGN OWNED`; in the
same transaction, before `DROP OWNED`, the Agent refuses (`owns_objects`,
rolled back, the user as it was) when the user owns anything there that
`REASSIGN OWNED` would move - every `pg_shdepend` owner row of the user in
that database or among shared objects, except default-privilege entries and
user mappings, which `REASSIGN OWNED` skips and `DROP OWNED` removes. A site
user's function given to `celikpanel_admin` would run as `celikpanel_admin`
(SECURITY DEFINER, or from an index expression). The server's administrator
hands the objects to the database's own user, then removes the user again
(the user guide's Databases chapter):
`sudo -u postgres psql -d <database> -c 'REASSIGN OWNED BY "<user>" TO "<the database's own user>";'`.
A database owned by a site's own role keeps `REASSIGN OWNED` to that role,
which passed the rule. No Agent call drops,
empties, loads into, gives a user or opens or closes the engine's own system
databases (MariaDB `mysql`, `information_schema`, `performance_schema`,
`sys`; PostgreSQL `postgres`, `template0`, `template1`). A MariaDB database
is not made with a user whose name already exists on any host or as a role.

A refusal answers its reason as a code (`absent`, `privileged`, `member`,
`unreadable`, `system_database`, `unsupported_engine`, `owns_objects`); nothing was changed,
the server's administrator decides, and the Databases screens say it in the
person's language (`DATABASE_ACCOUNT_REFUSED`).

What this boundary is worth:

- On PostgreSQL 16 and later it is a real boundary against a taken-over
  panel: the panel's own account (`celikpanel_admin`, CREATEDB CREATEROLE)
  cannot make itself or another role privileged there, and the Agent will not
  do it on the panel's word.
- On MariaDB, and on PostgreSQL 15 or older, the panel's own database account
  already holds rights of that weight (MariaDB: `ALL PRIVILEGES ON *.* WITH
  GRANT OPTION`; PostgreSQL 15 or older: CREATEROLE can grant itself the
  `pg_*` roles). There the refusal guards against mistakes, not against a
  taken-over panel.
- A database of another site is protected by the panel's own checks only;
  the Agent does not check which site a database belongs to.

Limits of the rule:

- The PostgreSQL rule looks at role attributes and memberships. It does not
  look at parameter privileges (PostgreSQL 15 and later, `GRANT ALTER SYSTEM
  ON PARAMETER`), at EXECUTE granted directly on administrator functions
  (`pg_read_file`, `lo_import`, `lo_export`), or at roles that own objects a
  superuser runs. Each needs the server owner to have made such a grant by
  hand.
- Adding a PostgreSQL user grants it rights on every table, sequence and
  (read and write) function of the database's non-system schemas
  (`GRANT … ON ALL … IN SCHEMA`, run by the Agent as the superuser),
  including objects the server's administrator installed there: an
  extension put into a site database's `public` gives the new user EXECUTE
  on its functions, even one whose EXECUTE the extension took from `PUBLIC`
  on purpose (`dblink_connect_u`, adminpack's `pg_file_write`). Such an
  extension needs the administrator's own installation into the site's
  database; install administrator extensions elsewhere.
- Objects that earlier releases already handed to `celikpanel_admin` when an
  added user was removed stay as they are. The server's administrator can
  list what `celikpanel_admin` owns in a site's database (default-privilege
  entries left out):

  ```
  sudo -u postgres psql -d <database> -c "SELECT pg_catalog.pg_describe_object(d.classid, d.objid, d.objsubid) FROM pg_catalog.pg_shdepend d WHERE d.dbid = (SELECT oid FROM pg_catalog.pg_database WHERE datname = pg_catalog.current_database()) AND d.deptype = 'o' AND d.refobjid = 'celikpanel_admin'::pg_catalog.regrole AND d.classid <> 'pg_catalog.pg_default_acl'::pg_catalog.regclass;"
  ```

  and hand each to the database's own user (`ALTER TABLE … OWNER TO`,
  `ALTER FUNCTION … OWNER TO` and so on). Not `REASSIGN OWNED BY
  celikpanel_admin`: that would also hand over every database
  `celikpanel_admin` owns on the server.
- A local MySQL server running in MariaDB's place is not managed: MySQL has
  no `mysql.global_priv`, so the rule cannot be read there and every account
  change is refused as `unsupported_engine` ("this server runs MySQL in
  MariaDB's place; CelikPanel manages database accounts on MariaDB only").
  The server's administrator makes the change on the server itself. An
  account the administrator removed by hand counts as gone, so deleting the
  database or the site then goes through; rights that name a deleted database
  are then removed by the engine's rule for every grantee, without taking the
  account names from the panel.

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
- Lengths (alpha.134): every name a create makes - the database and the
  user made with it (`<database>_user`), a copy and its user, an added
  user - holds at most 63 characters on both engines, and on MariaDB the
  database name's exact form fits 64 (above). One rule
  (`services.DatabaseNamesOver`) is checked by the panel before anything is
  sent and by the Agent before anything runs; the domain's create form
  reads the same test cases, shows the full name as the server makes it
  (the list's `name_prefix`) and stops such a name beside the field. A name
  too long is DATABASE_NAME_TOO_LONG with by how much; the words about
  underscores come only when MariaDB's exact form is the limit exceeded
  most. A name holding another character than a letter, digit or `_` is
  refused before the Agent too. Names CelikPanel makes itself (installers,
  clones, site copies) are shortened by FitBase, which keeps them within 63
  as well (their user has the database's own name), on both engines.
- A failed create (alpha.134): what it made is taken back - the user
  before the database, and only what this request made itself; an account
  that existed before is never touched. The panel driver's last statement
  for a new MariaDB user is CREATE USER (an account statement needs no
  FLUSH PRIVILEGES; an account without RELOAD used to have the new user
  left behind, alpha.133 lab). When the clean-up cannot finish, the engine
  is asked what is still there and the answer (DATABASE_CREATE_LEFT_OVER)
  names it, says the server's administrator removes it, and that the same
  name is refused until then. The domain create also takes back what the
  Agent made when its records cannot be written, and a database copy that
  could not be removed again says so (it stays listed; Delete removes it).
  Open: a CREATE DATABASE or CREATE USER that gets no answer (a timeout, a
  lost connection) is not taken back - whether it ran is unknown and the
  panel keeps no record proving the name its own; the next press is then
  refused as taken.
