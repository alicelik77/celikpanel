package main

// Every session the engine's administrator opens (security lane S1, 20
// November 2026).
//
// A database's owner can set its database's search_path (ALTER DATABASE ...
// SET search_path = public, pg_catalog) and create functions and operators
// in public with the names of pg_catalog's: count(*), <>(name, name),
// !~~(name, text), pg_get_userbyid(oid). A session of the postgres account
// that then runs an unqualified name inside that database runs the owner's
// function as the engine's superuser (the CVE-2018-1058 class). So every
// administrator session - psql, pg_dump, pg_restore, pg_dumpall, as postgres
// through sudo or as a cluster's owner account - starts with the search path
// fixed to pg_catalog and then pg_temp, given as a connection option
// (PGOPTIONS): it outranks ALTER DATABASE/ROLE ... SET and holds for every
// connection the client makes, a psql \connect included. psql never reads a
// start-up file (--no-psqlrc).
//
// The guard test (pg_admin_session_test.go) fails when a PostgreSQL client is
// started anywhere else.
//
// Yöneticinin açtığı her PostgreSQL oturumu arama yolunu pg_catalog ve
// pg_temp'e sabitleyerek başlar; veritabanı sahibinin public şemasına koyduğu
// işlevler bu oturumlarda çalışmaz.

// pgAdminSearchPath is the connection option every administrator session
// starts with.
const pgAdminSearchPath = "-c search_path=pg_catalog,pg_temp"

// pgAdminSessionPins (second review of alpha.143, B5, 22 November 2026): a
// database's owner can also set, for his database (ALTER DATABASE ... SET),
// the timeouts, read-only mode, the role and exit_on_error, and these apply
// to the administrator's session inside it as well: the close became
// unknown or failed and the scan unreadable, and with "role" the session
// ran as the owner. Connection options outrank every ALTER DATABASE/ROLE ...
// SET, so the administrator session pins them to PostgreSQL's own defaults:
//   - statement_timeout=0, lock_timeout=0: no limit from the owner. A pin to
//     a short limit would cut the panel's own long loads (restore, copy,
//     dump), so the bounded uses set their own limit inside their
//     transaction (SET LOCAL in the close and the undo, pg_public_access.go).
//   - idle_in_transaction_session_timeout=0: a restore or dump transaction
//     is never ended between statements.
//   - default_transaction_read_only=off: the panel's writes are not refused.
//   - role=none: the session acts as the account it signed in as.
//   - exit_on_error=off: an expected error (a guard's refusal) answers, it
//     does not end the session.
//
// idle_session_timeout (PostgreSQL 14+) and transaction_timeout (17+) are
// not in every supported version, and an unknown option refuses the
// connection, so the bounded uses set those right after connecting
// (pgPublicSessionPins).
const pgAdminSessionPins = " -c statement_timeout=0 -c lock_timeout=0 -c idle_in_transaction_session_timeout=0" +
	" -c default_transaction_read_only=off -c role=none -c exit_on_error=off"

// pgAdminSessionEnv is the environment entry that carries them.
const pgAdminSessionEnv = "PGOPTIONS=" + pgAdminSearchPath + pgAdminSessionPins

// pgAdminSudoArgs is the sudo command line that runs a PostgreSQL client
// as the postgres account with the fixed search path.
func pgAdminSudoArgs(client string, args ...string) []string {
	return append([]string{"-u", "postgres", "env", pgAdminSessionEnv, client}, args...)
}

// pgAdminPSQLArgs is pgAdminSudoArgs for psql: no start-up file, and every
// failure stops it (callers read success from the exit status). extra
// follows; a pg_wrapper --cluster option goes first.
func pgAdminPSQLArgs(cluster string, extra ...string) []string {
	args := []string{}
	if cluster != "" {
		args = append(args, "--cluster", cluster)
	}
	args = append(args, "--no-psqlrc", "--set", "ON_ERROR_STOP=on")
	return pgAdminSudoArgs("psql", append(args, extra...)...)
}

// pgAdminPSQLArgsFor is pgAdminPSQLArgs for one named client (B1): every
// connection it makes carries application_name (PGAPPNAME, read by libpq at
// every connection, a psql \connect included), so its sessions can be found
// on the engine, and it gives up connecting after 10 seconds.
func pgAdminPSQLArgsFor(application string, extra ...string) []string {
	args := append([]string{"--no-psqlrc", "--set", "ON_ERROR_STOP=on"}, extra...)
	return append([]string{"-u", "postgres", "env", pgAdminSessionEnv, "PGAPPNAME=" + application, "PGCONNECT_TIMEOUT=10", "psql"}, args...)
}
