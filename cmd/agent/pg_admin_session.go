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

// pgAdminSessionEnv is the environment entry that carries it.
const pgAdminSessionEnv = "PGOPTIONS=" + pgAdminSearchPath

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
