package mariadbgrant

import (
	"errors"
	"fmt"
	"slices"
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
// One exception (review of alpha.134): a row in the name's raw form (as
// written, a pattern) that ALSO reaches another existing database is kept
// and reported: removing it would take the account's access to that other
// database away. It never applies to the accounts on record for the deleted
// database (owners; second review): their rows naming it go in both forms,
// from every host, as on alpha.133 - CelikPanel gave them, and a pattern it
// gave that reaches another customer's database must not outlive the
// deletion.
//
// Silinen veritabanını adlandıran haklar her hesaptan ve rolden kaldırılır;
// hesaplar ve roller kalır; yalnız eşleşen kalıplara dokunulmaz. Başka bir
// veritabanına da erişen kalıp biçimli satır bırakılır ve bildirilir.

// Exec runs statements on the engine and answers the client's output.
type Exec func(statement string) ([]byte, error)

// ErrDatabaseExists: a database of the name exists (again); nothing was
// changed.
var ErrDatabaseExists = errors.New("a database of that name exists on the database server; nothing was changed")

// RightsLeftError: accounts or roles still hold a right naming the database
// after the removal (Left: how many, each counted once).
type RightsLeftError struct{ Left int }

func (e *RightsLeftError) Error() string {
	return fmt.Sprintf("%d account(s) or role(s) still hold a right naming the database on the database server", e.Left)
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

// droppedReadStatement reads what the removal needs: the name case rule, the
// database names, the database-level rows of the name's two forms
// ("d=<user>/<host>/<db>") and the grantees of the table, column and routine
// rows ("g=<user>/<host>"), every value in hexadecimal. "/" never occurs in
// hexadecimal, so an empty user or host stays a field.
func droppedReadStatement(name string) string {
	forms, literal := HexList(Forms(name)), Hex(name)
	part := func(table string) string {
		return "SELECT CONCAT('g=', HEX(User), '/', HEX(Host)) FROM mysql." + table + " WHERE Db = " + literal
	}
	return "SELECT CONCAT('l=', @@lower_case_table_names); " +
		"SELECT CONCAT('s=', HEX(SCHEMA_NAME)) FROM information_schema.SCHEMATA; " +
		"SELECT CONCAT('d=', HEX(User), '/', HEX(Host), '/', HEX(Db)) FROM mysql.db WHERE Db IN (" + forms + "); " +
		part("tables_priv") + " UNION " + part("columns_priv") + " UNION " + part("procs_priv") + ";"
}

// droppedDeleteStatements removes the given database-level rows one by one
// and every table, column and routine row of the name; never while a
// database of that name exists. FLUSH PRIVILEGES runs on its own after it.
func droppedDeleteStatements(name string, rows []DroppedRow) string {
	literal, absent := Hex(name), absentGuard(name)
	var out []string
	for _, row := range rows {
		out = append(out, "DELETE FROM mysql.db WHERE User = "+Hex(row.User)+" AND Host = "+Hex(row.Host)+" AND Db = "+Hex(row.Stored)+absent+";")
	}
	out = append(out,
		"DELETE FROM mysql.tables_priv WHERE Db = "+literal+absent+";",
		"DELETE FROM mysql.columns_priv WHERE Db = "+literal+absent+";",
		"DELETE FROM mysql.procs_priv WHERE Db = "+literal+absent+";",
	)
	return strings.Join(out, "\n")
}

// droppedLeftStatement counts the accounts and roles that still hold a right
// naming the database (second review: one thing, counted once each). "a=" is
// what the server has LOADED (information_schema; the kept rows' accounts
// left out for the raw form), "b=" what the grant tables STORE at every level
// (information_schema does not show every role's rows; the kept rows left
// out). The two are counted apart because a FLUSH that failed leaves rows
// loaded that are gone from the tables.
func droppedLeftStatement(name string, kept []DroppedRow) string {
	exact, raw, literal := Hex(Exact(name)), Hex(name), Hex(name)
	notKept, notKeptStored := "", ""
	if len(kept) > 0 {
		var grantees, pairs []string
		for _, row := range kept {
			grantees = append(grantees, "'"+row.User+"'@'"+row.Host+"'", "'"+row.User+"'")
			pairs = append(pairs, "(User = "+Hex(row.User)+" AND Host = "+Hex(row.Host)+")")
		}
		notKept = " AND GRANTEE NOT IN (" + HexList(grantees) + ")"
		notKeptStored = " AND NOT (Db = " + raw + " AND (" + strings.Join(pairs, " OR ") + "))"
	}
	loaded := "SELECT GRANTEE FROM information_schema.SCHEMA_PRIVILEGES WHERE BINARY TABLE_SCHEMA = " + exact
	if Exact(name) != name {
		loaded += " UNION SELECT GRANTEE FROM information_schema.SCHEMA_PRIVILEGES WHERE BINARY TABLE_SCHEMA = " + raw + notKept
	}
	loaded += " UNION SELECT GRANTEE FROM information_schema.TABLE_PRIVILEGES WHERE BINARY TABLE_SCHEMA = " + literal +
		" UNION SELECT GRANTEE FROM information_schema.COLUMN_PRIVILEGES WHERE BINARY TABLE_SCHEMA = " + literal
	stored := "SELECT User, Host FROM mysql.db WHERE Db IN (" + HexList(Forms(name)) + ")" + notKeptStored +
		" UNION SELECT User, Host FROM mysql.tables_priv WHERE Db = " + literal +
		" UNION SELECT User, Host FROM mysql.columns_priv WHERE Db = " + literal +
		" UNION SELECT User, Host FROM mysql.procs_priv WHERE Db = " + literal
	return "SELECT CONCAT('a=', COUNT(*)) FROM (" + loaded + ") loaded; " +
		"SELECT CONCAT('b=', COUNT(*)) FROM (" + stored + ") stored;"
}

// Grantee is one account or role that held a right.
type Grantee struct{ User, Host string }

// String is user@host; a role (an empty host) is its name alone; the
// anonymous account (an empty user) is ”@host.
func (g Grantee) String() string {
	switch {
	case g.User == "":
		return "''@" + g.Host
	case g.Host == "":
		return g.User
	}
	return g.User + "@" + g.Host
}

// DroppedRow is one database-level row naming the dropped database.
type DroppedRow struct {
	User, Host, Stored string
	// Reaches: the other existing databases a kept row still covers.
	Reaches []string
}

// DroppedResult says whose rights went and which rows were kept.
type DroppedResult struct {
	Removed []Grantee
	// Kept: rows in the raw form that also reach another existing database
	// (left for their owner; see the package comment).
	Kept []DroppedRow
}

// RemoveDroppedRights removes every right naming the dropped database from
// every grantee (but the kept rows) and answers whose rights went.
// ErrDatabaseExists: a database of that name exists, nothing was changed.
// *RightsLeftError: something is still loaded (removing again continues).
// Other errors: the engine could not be asked or did not do it; nothing is
// claimed.
//
// owners are the account names on record for the deleted database: their
// rows are always removed, never kept.
func RemoveDroppedRights(run Exec, name string, owners ...string) (DroppedResult, error) {
	if !ValidName(name) {
		return DroppedResult{}, ErrName
	}
	exists, err := countMarker(run, DatabaseExistsStatement(name), "n", "whether the database still exists")
	if err != nil {
		return DroppedResult{}, err
	}
	if exists != 0 {
		return DroppedResult{}, ErrDatabaseExists
	}
	out, err := run(droppedReadStatement(name))
	if err != nil {
		return DroppedResult{}, fmt.Errorf("the database server could not list the rights that name the database: %w", err)
	}
	foldCase := false
	if rule, ok := Marker(out, "l"); ok && rule != "0" {
		foldCase = true
	}
	var schemas []string
	for _, value := range Markers(out, "s") {
		schema, err := FromHex(value)
		if err != nil {
			return DroppedResult{}, err
		}
		schemas = append(schemas, schema)
	}
	var result DroppedResult
	var remove []DroppedRow
	seen := map[Grantee]bool{}
	add := func(g Grantee) {
		if !seen[g] {
			seen[g] = true
			result.Removed = append(result.Removed, g)
		}
	}
	for _, value := range Markers(out, "d") {
		fields, err := hexFields(value, 3)
		if err != nil {
			return DroppedResult{}, err
		}
		row := DroppedRow{User: fields[0], Host: fields[1], Stored: fields[2]}
		if row.Stored == name && Exact(name) != name && !(row.Host != "" && slices.Contains(owners, row.User)) {
			if reaches := reached(row.Stored, schemas, foldCase); len(reaches) > 0 {
				row.Reaches = reaches
				result.Kept = append(result.Kept, row)
				continue
			}
		}
		remove = append(remove, row)
		add(Grantee{User: row.User, Host: row.Host})
	}
	others := Markers(out, "g")
	for _, value := range others {
		fields, err := hexFields(value, 2)
		if err != nil {
			return DroppedResult{}, err
		}
		add(Grantee{User: fields[0], Host: fields[1]})
	}
	sort.Slice(result.Removed, func(i, j int) bool { return result.Removed[i].String() < result.Removed[j].String() })
	if len(remove) > 0 || len(others) > 0 {
		if _, err := run(droppedDeleteStatements(name, remove)); err != nil {
			return DroppedResult{}, fmt.Errorf("the database server did not take away the rights that name the database: %w", err)
		}
		if _, err := run("FLUSH PRIVILEGES;"); err != nil {
			return DroppedResult{}, fmt.Errorf("the database server did not reload its rights: %w", err)
		}
	}
	left, err := droppedLeft(run, name, result.Kept)
	if err != nil {
		return DroppedResult{}, err
	}
	if left != 0 {
		// The rows may be gone from the tables while the server still holds
		// them loaded (a FLUSH that failed after the DELETEs, here or on an
		// earlier attempt): reload on its own and count again.
		if _, err := run("FLUSH PRIVILEGES;"); err != nil {
			return DroppedResult{}, fmt.Errorf("the database server did not reload its rights: %w", err)
		}
		if left, err = droppedLeft(run, name, result.Kept); err != nil {
			return DroppedResult{}, err
		}
	}
	if left != 0 {
		// A database made again between the first check and the DELETE
		// (whose guard then skipped every row) is not "rights left".
		if again, err := countMarker(run, DatabaseExistsStatement(name), "n", "whether the database exists again"); err == nil && again != 0 {
			return DroppedResult{}, ErrDatabaseExists
		}
		return DroppedResult{}, &RightsLeftError{Left: left}
	}
	return result, nil
}

// droppedLeft answers how many accounts and roles still hold a right naming
// the database, loaded or stored (the larger of the two counts), the kept
// rows aside.
func droppedLeft(run Exec, name string, kept []DroppedRow) (int, error) {
	out, err := run(droppedLeftStatement(name, kept))
	if err != nil {
		return 0, fmt.Errorf("the database server could not be asked whether rights naming the database are left: %w", err)
	}
	left := 0
	for _, marker := range []string{"a", "b"} {
		value, ok := Marker(out, marker)
		n, convErr := strconv.Atoi(value)
		if !ok || convErr != nil || n < 0 {
			return 0, errors.New("the database server's answer about rights left on the database could not be read")
		}
		left = max(left, n)
	}
	return left, nil
}

// hexFields splits "<hex>/<hex>[/<hex>]" into its decoded fields.
func hexFields(value string, want int) ([]string, error) {
	parts := strings.Split(value, "/")
	if len(parts) != want {
		return nil, errors.New("a grant row of the database server could not be read")
	}
	out := make([]string, 0, want)
	for _, part := range parts {
		if part == "" {
			out = append(out, "")
			continue
		}
		decoded, err := FromHex(part)
		if err != nil {
			return nil, err
		}
		out = append(out, decoded)
	}
	return out, nil
}

// countMarker runs a statement answering one "<marker>=<count>" line.
func countMarker(run Exec, statement, marker, what string) (int, error) {
	out, err := run(statement)
	if err != nil {
		return 0, fmt.Errorf("the database server could not be asked %s: %w", what, err)
	}
	value, ok := Marker(out, marker)
	if !ok {
		return 0, fmt.Errorf("the database server's answer about %s could not be read", what)
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("the database server's answer about %s could not be read", what)
	}
	return n, nil
}
