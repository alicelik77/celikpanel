package services

import (
	"strconv"
	"strings"
)

// Every PostgreSQL database CelikPanel creates is UTF8 with an explicit,
// language-neutral locale, whatever the cluster's own templates are
// (20 Nov 2026, from a production server: a cluster initialised under locale
// C has SQL_ASCII templates, and CREATE DATABASE without options inherits
// template1, so every database the panel made there stored text as unchecked
// bytes). The statement copies template0, the only template whose encoding
// and locale may differ from the new database's, and names everything:
//
//   - PostgreSQL 17 and later: the builtin provider with its C.UTF-8 locale.
//     Unicode code-point order and simple case mapping, built into
//     PostgreSQL, so an operating system's collation update never changes
//     its order. LC_COLLATE and LC_CTYPE are set to C, which every system
//     has and which allows every encoding, so the statement needs nothing
//     from the operating system.
//   - PostgreSQL 15 and 16: libc named explicitly (LOCALE_PROVIDER, new in
//     15), C.UTF-8 for LC_COLLATE and LC_CTYPE.
//   - PostgreSQL 14 and earlier, and a server whose version could not be
//     read: LC_COLLATE and LC_CTYPE C.UTF-8 only (both exist since 8.4;
//     LOCALE only since 13).
//
// Before 17, C.UTF-8 is a libc locale the system must have (Debian and
// Ubuntu always do: libc-bin ships it). A server that refuses it as an
// invalid locale name gets the same statement once more with libc C: still
// UTF8, the text still checked, ordered by byte value, which for UTF-8 is
// code-point order; only upper()/lower() then change ASCII letters alone.
//
// Sources (read 20 Nov 2026): https://www.postgresql.org/docs/{13,14,15,16,17,18}/sql-createdatabase.html
//
// CelikPanel'in oluşturduğu her PostgreSQL veritabanı, kümenin şablonları ne
// olursa olsun UTF8'dir ve yerel ayarı açıkça yazılır: 17 ve sonrasında
// yerleşik C.UTF-8, öncesinde libc C.UTF-8 (yoksa libc C, yine UTF8).

// PostgreSQLDatabaseLocale is the locale every new database gets.
const PostgreSQLDatabaseLocale = "C.UTF-8"

// PostgreSQLBuiltinLocaleMajor is the first major with the builtin provider.
const PostgreSQLBuiltinLocaleMajor = 17

// PostgreSQLMajor is the major of a server_version_num (180001 -> 18;
// 90624 -> 9); 0 when unknown.
func PostgreSQLMajor(versionNum int) int {
	if versionNum <= 0 {
		return 0
	}
	return versionNum / 10000
}

// ParsePostgreSQLVersionNum reads current_setting('server_version_num');
// 0 when it is not a positive number.
func ParsePostgreSQLVersionNum(text string) int {
	n, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

// PostgreSQLCreateDatabaseStatement is the one CREATE DATABASE CelikPanel
// sends. ident and ownerIdent are already quoted identifiers; ownerIdent ""
// names no owner. libcC is the fallback for a server without a C.UTF-8 libc
// locale (ignored from 17, where nothing comes from the system).
func PostgreSQLCreateDatabaseStatement(ident, ownerIdent string, versionNum int, libcC bool) string {
	var b strings.Builder
	b.WriteString("CREATE DATABASE ")
	b.WriteString(ident)
	if ownerIdent != "" {
		b.WriteString(" OWNER ")
		b.WriteString(ownerIdent)
	}
	b.WriteString(" TEMPLATE template0 ENCODING 'UTF8'")
	major := PostgreSQLMajor(versionNum)
	switch {
	case major >= PostgreSQLBuiltinLocaleMajor:
		b.WriteString(" LOCALE_PROVIDER builtin BUILTIN_LOCALE '" + PostgreSQLDatabaseLocale + "' LC_COLLATE 'C' LC_CTYPE 'C'")
	default:
		locale := PostgreSQLDatabaseLocale
		if libcC {
			locale = "C"
		}
		if major >= 15 {
			b.WriteString(" LOCALE_PROVIDER libc")
		}
		b.WriteString(" LC_COLLATE '" + locale + "' LC_CTYPE '" + locale + "'")
	}
	// Made shut: nobody signs in before PUBLIC's rights are revoked
	// (PostgreSQLClosePublicStatement opens it). CONNECT is checked only at
	// sign-in, so a session opened in between would outlive the revoke.
	b.WriteString(" ALLOW_CONNECTIONS false;")
	return b.String()
}

// PostgreSQLCreateDatabaseNeedsFallback: the statement failed only because
// this server (before 17) has no C.UTF-8 libc locale. Nothing was created
// then, so the fallback statement is safe to send once.
func PostgreSQLCreateDatabaseNeedsFallback(versionNum int, err error) bool {
	if err == nil || PostgreSQLMajor(versionNum) >= PostgreSQLBuiltinLocaleMajor {
		return false
	}
	text := err.Error()
	for _, marker := range []string{"invalid LC_COLLATE locale name", "invalid LC_CTYPE locale name", "invalid locale name"} {
		if strings.Contains(text, marker+`: "`+PostgreSQLDatabaseLocale+`"`) {
			return true
		}
	}
	return false
}

// A new database is closed to every other role (20 Nov 2026, security). A
// database is created with PostgreSQL's PUBLIC defaults - CONNECT and
// TEMPORARY for every role on the cluster - so one customer's database user
// could sign in to another customer's database over localhost, read its
// catalogue (table and column names, function bodies), call functions open
// to PUBLIC and make temporary tables there (seen read-only on a production
// server). Right after CREATE DATABASE those defaults are revoked; the owner
// keeps its own rights, and every user CelikPanel adds to the database is
// granted CONNECT (read and write: CONNECT, TEMPORARY) explicitly, as it
// always was. The creation is complete only when the read-back says PUBLIC
// has neither right; otherwise the new database is removed again.
//
// Yeni veritabanı başka hiçbir role açık değildir: PUBLIC'in CONNECT ve
// TEMPORARY hakları oluşturmanın hemen ardından geri alınır ve okunarak
// doğrulanır.

// The database is created with ALLOW_CONNECTIONS false and opened only after
// the revoke, so there is no moment in which another role can sign in
// (PostgreSQL advises the revoke "in the same transaction that creates the
// object", which CREATE DATABASE cannot be part of).

// PostgreSQLClosePublicStatement revokes PUBLIC's rights on the database and
// then lets connections in (ident: a quoted identifier).
func PostgreSQLClosePublicStatement(ident string) string {
	return "REVOKE ALL ON DATABASE " + ident + " FROM PUBLIC;\nALTER DATABASE " + ident + " ALLOW_CONNECTIONS true;"
}

// PostgreSQLPublicReachQuery is true while the database named $1 is not
// finished: PUBLIC still has CONNECT or TEMPORARY, or it does not take
// connections yet.
const PostgreSQLPublicReachQuery = "SELECT pg_catalog.has_database_privilege('public', $1, 'CONNECT') OR pg_catalog.has_database_privilege('public', $1, 'TEMPORARY') OR NOT COALESCE((SELECT datallowconn FROM pg_catalog.pg_database WHERE datname = $1), false)"

// PostgreSQLEncodingIsUTF8 reports whether pg_encoding_to_char's name is
// UTF8, the only server encoding whose text PostgreSQL checks as Unicode.
func PostgreSQLEncodingIsUTF8(name string) bool {
	return strings.EqualFold(strings.TrimSpace(name), "UTF8")
}
