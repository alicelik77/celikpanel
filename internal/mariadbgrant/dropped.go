package mariadbgrant

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Rights that name a dropped database (alpha.134). MariaDB keeps every grant
// row that names a database after DROP DATABASE ("user privileges … are not
// automatically dropped", MariaDB KB DROP DATABASE), and a database created
// later under the same name hands its tables to whoever holds them. When
// CelikPanel deletes a database these rows go from EVERY grantee - accounts
// made by hand, accounts under any name, and roles (a role's rows have the
// empty host): database level by the name's two forms, table, column and
// routine level by the literal name. The accounts and roles themselves stay.
// A pattern that merely matches the name (`shop\_%`, `sh_p`) is another
// text and is never touched. Every statement repeats that no database of
// that name exists, so a database made again in between is not touched.
//
// Silinen veritabanını adlandıran haklar her hesaptan ve rolden kaldırılır;
// hesaplar ve roller kalır; yalnız eşleşen kalıplara dokunulmaz.

// Exec runs statements on the engine and answers the client's output.
type Exec func(statement string) ([]byte, error)

// ErrDatabaseExists: a database of the name exists (again); nothing was
// changed.
var ErrDatabaseExists = errors.New("a database of that name exists on the database server; nothing was changed")

// RightsLeftError: rows naming the database are still loaded after the
// removal.
type RightsLeftError struct{ Left int }

func (e *RightsLeftError) Error() string {
	return fmt.Sprintf("%d right(s) naming the database are still loaded on the database server", e.Left)
}

// absentGuard is the condition every removal repeats.
func absentGuard(name string) string {
	return " AND NOT EXISTS (SELECT 1 FROM information_schema.SCHEMATA WHERE BINARY SCHEMA_NAME = " + Hex(name) + ")"
}

// DatabaseExistsStatement answers "n=<count>" of databases with exactly
// this name.
func DatabaseExistsStatement(name string) string {
	return "SELECT CONCAT('n=', COUNT(*)) FROM information_schema.SCHEMATA WHERE BINARY SCHEMA_NAME = " + Hex(name) + ";"
}

// DroppedGranteesStatement lists every grantee with a row naming the
// database, as "g=<hex user> <hex host>".
func DroppedGranteesStatement(name string) string {
	forms, literal := HexList(Forms(name)), Hex(name)
	part := func(table, where string) string {
		return "SELECT CONCAT('g=', HEX(User), ' ', HEX(Host)) FROM mysql." + table + " WHERE " + where
	}
	return part("db", "Db IN ("+forms+")") +
		" UNION " + part("tables_priv", "Db = "+literal) +
		" UNION " + part("columns_priv", "Db = "+literal) +
		" UNION " + part("procs_priv", "Db = "+literal) + ";"
}

// DroppedDeleteStatements removes those rows and reloads the grant tables.
func DroppedDeleteStatements(name string) string {
	forms, literal, absent := HexList(Forms(name)), Hex(name), absentGuard(name)
	return strings.Join([]string{
		"DELETE FROM mysql.db WHERE Db IN (" + forms + ")" + absent + ";",
		"DELETE FROM mysql.tables_priv WHERE Db = " + literal + absent + ";",
		"DELETE FROM mysql.columns_priv WHERE Db = " + literal + absent + ";",
		"DELETE FROM mysql.procs_priv WHERE Db = " + literal + absent + ";",
		"FLUSH PRIVILEGES;",
	}, "\n")
}

// DroppedLeftStatement counts what is still there after the flush: what the
// server has LOADED (information_schema, every grantee) and the rows of the
// tables information_schema does not show for roles.
func DroppedLeftStatement(name string) string {
	forms, literal := HexList(Forms(name)), Hex(name)
	return "SELECT CONCAT('n=', " +
		"(SELECT COUNT(*) FROM information_schema.SCHEMA_PRIVILEGES WHERE BINARY TABLE_SCHEMA IN (" + forms + "))" +
		" + (SELECT COUNT(*) FROM information_schema.TABLE_PRIVILEGES WHERE BINARY TABLE_SCHEMA = " + literal + ")" +
		" + (SELECT COUNT(*) FROM information_schema.COLUMN_PRIVILEGES WHERE BINARY TABLE_SCHEMA = " + literal + ")" +
		" + (SELECT COUNT(*) FROM mysql.db WHERE Db IN (" + forms + "))" +
		" + (SELECT COUNT(*) FROM mysql.tables_priv WHERE Db = " + literal + ")" +
		" + (SELECT COUNT(*) FROM mysql.procs_priv WHERE Db = " + literal + "));"
}

// Grantee is one account or role that held a right.
type Grantee struct{ User, Host string }

// String is user@host, or the role's name alone (empty host).
func (g Grantee) String() string {
	if g.Host == "" {
		return g.User
	}
	return g.User + "@" + g.Host
}

// RemoveDroppedRights removes every right naming the dropped database from
// every grantee and answers whose rights went. ErrDatabaseExists: a database
// of that name exists, nothing was changed. *RightsLeftError: something is
// still loaded (removing again continues). Other errors: the engine could
// not be asked or did not do it; nothing is claimed.
func RemoveDroppedRights(run Exec, name string) ([]Grantee, error) {
	if !ValidName(name) {
		return nil, ErrName
	}
	exists, err := countMarker(run, DatabaseExistsStatement(name), "whether the database still exists")
	if err != nil {
		return nil, err
	}
	if exists != 0 {
		return nil, ErrDatabaseExists
	}
	out, err := run(DroppedGranteesStatement(name))
	if err != nil {
		return nil, fmt.Errorf("the database server could not list the rights that name the database: %w", err)
	}
	var grantees []Grantee
	for _, value := range Markers(out, "g") {
		parts := strings.Fields(value)
		user, host := "", ""
		if len(parts) >= 1 {
			if user, err = FromHex(parts[0]); err != nil {
				return nil, err
			}
		}
		if len(parts) == 2 {
			if host, err = FromHex(parts[1]); err != nil {
				return nil, err
			}
		}
		grantees = append(grantees, Grantee{User: user, Host: host})
	}
	sort.Slice(grantees, func(i, j int) bool { return grantees[i].String() < grantees[j].String() })
	if len(grantees) > 0 {
		if _, err := run(DroppedDeleteStatements(name)); err != nil {
			return nil, fmt.Errorf("the database server did not take away the rights that name the database: %w", err)
		}
	}
	left, err := countMarker(run, DroppedLeftStatement(name), "whether rights naming the database are left")
	if err != nil {
		return nil, err
	}
	if left != 0 {
		// A database made again between the first check and the DELETE
		// (whose guard then skipped every row) is not "rights left".
		if again, err := countMarker(run, DatabaseExistsStatement(name), "whether the database exists again"); err == nil && again != 0 {
			return nil, ErrDatabaseExists
		}
		return nil, &RightsLeftError{Left: left}
	}
	return grantees, nil
}

// countMarker runs a statement answering one "n=<count>" line.
func countMarker(run Exec, statement, what string) (int, error) {
	out, err := run(statement)
	if err != nil {
		return 0, fmt.Errorf("the database server could not be asked %s: %w", what, err)
	}
	value, ok := Marker(out, "n")
	if !ok {
		return 0, fmt.Errorf("the database server's answer about %s could not be read", what)
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("the database server's answer about %s could not be read", what)
	}
	return n, nil
}
