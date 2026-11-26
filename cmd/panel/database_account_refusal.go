package main

import "github.com/alicelik/celikpanel/internal/transport"

// The Agent refuses to change a database account it finds to be an
// administrator account, or whose rights it cannot read (alpha.149 security
// fix, review of PR #370 finding 1; cmd/agent database_account_guard.go). Its
// answer names the reason as a code (AccountRefused); the panel says it in
// one plain sentence of its own: what was refused, that nothing was changed,
// and that the server's administrator acts (D-024: the reason, who acts, the
// next action).
//
// Agent'ın yönetici hesabını değiştirmeyi reddi panelde tek, düz bir cümleyle
// söylenir: hiçbir şey değişmedi, sunucunun yöneticisi karar verir.

// databaseAccountRefusalSentence is the panel's sentence for a refusal code.
func databaseAccountRefusalSentence(code, username string) string {
	switch code {
	case transport.DatabaseAccountRefusedPrivileged:
		return "The database user " + username + " holds administrator rights on the database server, so CelikPanel did not change it. " +
			"Nothing was changed. The server's administrator decides about this account (" + code + ")."
	case transport.DatabaseAccountRefusedMember:
		return "The database user " + username + " is a member of another PostgreSQL role, which no site database user CelikPanel makes is, so CelikPanel did not change it. " +
			"Nothing was changed. The server's administrator decides about this account (" + code + ")."
	case transport.DatabaseAccountRefusedAbsent:
		return "The database user " + username + " is not on the database server. Nothing was changed (" + code + ")."
	case transport.DatabaseAccountRefusedUnsupportedEngine:
		return "This server runs MySQL in MariaDB's place; CelikPanel manages database accounts on MariaDB only, so it did not change the database user " + username + ". " +
			"Nothing was changed. The server's administrator makes the change on the server itself (" + code + ")."
	case transport.DatabaseAccountRefusedSystemDatabase:
		return "CelikPanel never changes the database server's own system database. Nothing was changed (" + code + ")."
	case transport.DatabaseAccountRefusedOwnsObjects:
		return "The database user " + username + " owns tables or other objects in this database, so it was not removed. Nothing was changed. " +
			"The server's administrator first hands them to the database's own user on the server (the user guide's Databases chapter gives the command), " +
			"then removes the user again from the panel (" + code + ")."
	}
	return "What the database user " + username + " holds on the database server could not be read, so CelikPanel did not change it. " +
		"Nothing was changed. The server's administrator checks that the database server answers, then the step can be repeated (" + code + ")."
}

// withDatabaseAccountRefusal replaces an answer's error with the panel's
// sentence when the Agent refused the account.
func withDatabaseAccountRefusal(code, username, current string) string {
	if code == "" {
		return current
	}
	return databaseAccountRefusalSentence(code, username)
}
