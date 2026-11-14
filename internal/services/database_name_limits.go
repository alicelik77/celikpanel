package services

import "github.com/alicelik/celikpanel/internal/mariadbgrant"

// The length rule of the names one create makes (alpha.134 fix round): the
// one place that says whether a database and the users made with it fit the
// engine, used by the panel before anything is sent and by the Agent before
// anything runs. Every name goes through ValidateSQLIdentifier (63
// characters, on both engines: PostgreSQL's limit, and the validator's own on
// MariaDB); on MariaDB the database name must also fit a right by its exact
// name (mariadbgrant: each `_` counts twice, 64 in all). The browser half is
// web/src/lib/databaseNameLimits.ts; both halves read
// testdata/database-name-limits.json, so they cannot drift.
//
// Bir oluşturmanın yaptığı adların uzunluk kuralı tek yerdedir: panel hiçbir
// şey göndermeden, Agent hiçbir şey çalıştırmadan önce buna bakar.

// NameOver says by how much the names of one create are too long.
type NameOver struct {
	// By is how many characters the name the person chose must lose at least
	// (0: every name fits). Every generated name grows with the chosen one,
	// so the largest excess is the one to remove.
	By int
	// Underscores: MariaDB's exact-name limit is the one exceeded most, so a
	// name with fewer underscores helps more (each counts twice there).
	Underscores bool
}

// DatabaseNamesOver answers whether the database (empty: none is made) and
// the users a create makes on the engine ("mysql" or "mariadb" for MariaDB,
// "postgresql") fit.
func DatabaseNamesOver(engine, database string, users ...string) NameOver {
	identifier := 0
	for _, name := range append([]string{database}, users...) {
		if over := len(name) - maxIdentifierLen; over > identifier {
			identifier = over
		}
	}
	exact := 0
	if database != "" && engine != "postgresql" {
		exact = mariadbgrant.OverLimit(database)
	}
	if exact > identifier {
		return NameOver{By: exact, Underscores: true}
	}
	return NameOver{By: identifier}
}
