package mariadbgrant

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Rewriting pattern-form grants to the exact form (alpha.134).
//
// The panel proves from its own records which grants are CelikPanel's (a
// recorded user, a recorded database, localhost or a recorded remote copy)
// and asks for those to be rewritten one at a time; every other pattern row
// is only listed. Scan reads; Convert changes one row: it grants the same
// privileges in the exact form first and revokes the pattern form after it,
// so access is never lost, and it can be run again at any point (an exact
// grant that is already there is not granted again; a pattern row that is
// already gone is not revoked). Values are read with HEX() and compared as
// hexadecimal literals, so the server's sql_mode and the client's output
// escaping change nothing.
//
// Kalıp biçimli hakların tam ada çevrilmesi: önce tam biçim verilir, sonra
// kalıp biçim alınır; erişim hiç kesilmez; her adım yeniden çalıştırılabilir.

// privilegeWords names each privilege column of mysql.db as GRANT writes it.
// Grant_priv is the grant option: CelikPanel never gives it, so a row that
// has it is the owner's.
var privilegeWords = map[string]string{
	"Select_priv": "SELECT", "Insert_priv": "INSERT", "Update_priv": "UPDATE", "Delete_priv": "DELETE",
	"Create_priv": "CREATE", "Drop_priv": "DROP", "References_priv": "REFERENCES", "Index_priv": "INDEX",
	"Alter_priv": "ALTER", "Create_tmp_table_priv": "CREATE TEMPORARY TABLES", "Lock_tables_priv": "LOCK TABLES",
	"Create_view_priv": "CREATE VIEW", "Show_view_priv": "SHOW VIEW", "Create_routine_priv": "CREATE ROUTINE",
	"Alter_routine_priv": "ALTER ROUTINE", "Execute_priv": "EXECUTE", "Event_priv": "EVENT", "Trigger_priv": "TRIGGER",
	"Delete_history_priv": "DELETE HISTORY", "Show_create_routine_priv": "SHOW CREATE ROUTINE",
}

const grantOptionColumn = "Grant_priv"

// columnsStatement lists mysql.db's privilege columns in order (MariaDB
// 10.11, 11.8 and MySQL differ in which they have).
const columnsStatement = "SELECT CONCAT('c=', COLUMN_NAME) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = 'mysql' AND TABLE_NAME = 'db' AND RIGHT(COLUMN_NAME, 5) = '_priv' ORDER BY ORDINAL_POSITION;"

// Row is one mysql.db row as read: its account, the stored database name and
// its privilege flags in the order of the columns.
type Row struct {
	User   string `json:"user"`
	Host   string `json:"host"`
	Stored string `json:"stored"`
	// Flags: one Y or N per privilege column, in column order.
	Flags string `json:"flags"`
	// Privileges: the flags as GRANT words ("ALL PRIVILEGES" when every
	// privilege CelikPanel can name is there).
	Privileges string `json:"privileges"`
	// GrantOption: the row holds the grant option (never CelikPanel's).
	GrantOption bool `json:"grant_option,omitempty"`
	// Unnamed: the row holds a privilege this code cannot name, so it cannot
	// be written again exactly.
	Unnamed bool `json:"unnamed,omitempty"`
	// Reached: the other existing databases the stored name covers now.
	Reached []string `json:"reached,omitempty"`
}

// Scan is what one read of an engine found.
type Scan struct {
	Version string `json:"version"`
	// MariaDB: the version string names MariaDB. Rows of another engine (MySQL)
	// are listed but not rewritten automatically.
	MariaDB bool `json:"mariadb"`
	// FoldCase: lower_case_table_names is not 0.
	FoldCase bool `json:"fold_case,omitempty"`
	// Patterns: every database-level row whose stored name may cover more
	// than one database (an unescaped _ or %).
	Patterns []Row `json:"patterns"`
}

// ScanStatement reads the version, the name case rule, the privilege
// columns, every database-level row and every database name.
func scanStatements(columns []string) string {
	return "SELECT CONCAT('v=', VERSION()); SELECT CONCAT('l=', @@lower_case_table_names); " +
		"SELECT CONCAT('r=', HEX(User), ' ', HEX(Host), ' ', HEX(Db), ' ', " + flagsExpression(columns) + ") FROM mysql.db; " +
		"SELECT CONCAT('s=', HEX(SCHEMA_NAME)) FROM information_schema.SCHEMATA;"
}

// flagsExpression concatenates the privilege columns (validated names).
func flagsExpression(columns []string) string {
	parts := make([]string, 0, len(columns))
	for _, column := range columns {
		parts = append(parts, "`"+column+"`")
	}
	return "CONCAT(" + strings.Join(parts, ", ") + ")"
}

// readColumns reads the privilege columns and refuses a name that is not a
// plain column name.
func readColumns(run Exec) ([]string, error) {
	out, err := run(columnsStatement)
	if err != nil {
		return nil, fmt.Errorf("the database server's grant table could not be read: %w", err)
	}
	columns := Markers(out, "c")
	if len(columns) == 0 {
		return nil, errors.New("the database server's grant table could not be read")
	}
	for _, column := range columns {
		for _, c := range column {
			if !(c >= 'a' && c <= 'z') && !(c >= 'A' && c <= 'Z') && c != '_' {
				return nil, errors.New("the database server's grant table has a column CelikPanel does not read")
			}
		}
	}
	return columns, nil
}

// describe turns a row's flags into GRANT words.
func describe(columns []string, row *Row) {
	var words []string
	named, all := 0, true
	for i, column := range columns {
		if i >= len(row.Flags) {
			break
		}
		set := row.Flags[i] == 'Y'
		if column == grantOptionColumn {
			row.GrantOption = set
			continue
		}
		word, known := privilegeWords[column]
		switch {
		case !known && set:
			row.Unnamed = true
		case !known:
			// A privilege this code cannot name exists on this server: ALL
			// PRIVILEGES would also give it, so the words are listed.
			all = false
		case set:
			words = append(words, word)
			named++
		default:
			all = false
		}
	}
	switch {
	case named == 0:
		row.Privileges = ""
	case all && !row.Unnamed:
		// Every privilege column of this server is set and named.
		row.Privileges = "ALL PRIVILEGES"
	default:
		row.Privileges = strings.Join(words, ", ")
	}
}

// ScanEngine reads an engine for the rewrite. It changes nothing.
func ScanEngine(run Exec) (Scan, error) {
	columns, err := readColumns(run)
	if err != nil {
		return Scan{}, err
	}
	out, err := run(scanStatements(columns))
	if err != nil {
		return Scan{}, fmt.Errorf("the database server's grants could not be read: %w", err)
	}
	version, ok := Marker(out, "v")
	if !ok {
		return Scan{}, errors.New("the database server's version could not be read")
	}
	scan := Scan{Version: version, MariaDB: strings.Contains(strings.ToLower(version), "mariadb")}
	if rule, ok := Marker(out, "l"); ok && rule != "0" {
		scan.FoldCase = true
	}
	var schemas []string
	for _, value := range Markers(out, "s") {
		name, err := FromHex(value)
		if err != nil {
			return Scan{}, err
		}
		schemas = append(schemas, name)
	}
	for _, value := range Markers(out, "r") {
		row, err := parseRow(value, columns)
		if err != nil {
			return Scan{}, err
		}
		if !HasWildcard(row.Stored) {
			continue
		}
		row.Reached = reached(row.Stored, schemas, scan.FoldCase)
		scan.Patterns = append(scan.Patterns, row)
	}
	sort.Slice(scan.Patterns, func(i, j int) bool {
		a, b := scan.Patterns[i], scan.Patterns[j]
		if a.Stored != b.Stored {
			return a.Stored < b.Stored
		}
		if a.User != b.User {
			return a.User < b.User
		}
		return a.Host < b.Host
	})
	return scan, nil
}

// parseRow reads "<hex user> <hex host> <hex db> <flags>".
func parseRow(value string, columns []string) (Row, error) {
	fields := strings.Split(value, " ")
	if len(fields) != 4 {
		return Row{}, errors.New("a grant row of the database server could not be read")
	}
	var row Row
	var err error
	if row.User, err = FromHex(fields[0]); err != nil {
		return Row{}, err
	}
	if row.Host, err = FromHex(fields[1]); err != nil {
		return Row{}, err
	}
	if row.Stored, err = FromHex(fields[2]); err != nil {
		return Row{}, err
	}
	row.Flags = fields[3]
	if len(row.Flags) != len(columns) || strings.Trim(row.Flags, "YN") != "" {
		return Row{}, errors.New("a grant row of the database server could not be read")
	}
	describe(columns, &row)
	return row, nil
}

// reached lists the existing databases a stored name covers, other than the
// one database it names (when it names one).
func reached(stored string, schemas []string, foldCase bool) []string {
	own, _, _ := Decode(stored)
	var out []string
	for _, schema := range schemas {
		if schema == own || (foldCase && strings.EqualFold(schema, own)) {
			continue
		}
		if Matches(stored, schema, foldCase) {
			out = append(out, schema)
		}
	}
	sort.Strings(out)
	return out
}

// Outcome of one Convert.
const (
	// Converted: the exact form was written (or was there) and the pattern
	// form is gone.
	Converted = "converted"
	// AlreadyExact: the pattern form was already gone and the exact form is
	// there.
	AlreadyExact = "already_exact"
	// Gone: neither form is there any more (the owner removed it): nothing
	// to do.
	Gone = "gone"
	// Changed: the pattern row is not what was captured; nothing was changed
	// (it is captured again first).
	Changed = "changed"
	// OwnerRow: the row holds the grant option or a privilege that cannot be
	// named; nothing was changed.
	OwnerRow = "owner_row"
	// NameTooLong: the exact form does not fit a grant row (ErrExactTooLong);
	// nothing was changed, the pattern form stays and is listed.
	NameTooLong = "name_too_long"
)

// ConvertRequest names one row to rewrite and the flags captured for it.
type ConvertRequest struct {
	User     string `json:"user"`
	Host     string `json:"host"`
	Database string `json:"database"`
	// Flags: the pattern row's privilege flags as captured before (the
	// before-image); a row that differs is not touched.
	Flags string `json:"flags"`
}

// Fault is a test seam: called with the step about to run ("grant",
// "revoke"); an error stops the conversion there.
type Fault func(step string) error

// Convert rewrites one pattern-form row to the exact form: GRANT the same
// privileges on the exact name, check, REVOKE the pattern form, check. Run
// again it continues where it stopped.
func Convert(run Exec, req ConvertRequest, fault Fault) (string, error) {
	if !ValidName(req.Database) {
		return "", ErrName
	}
	account, err := AccountLiteral(req.User, req.Host)
	if err != nil {
		return "", err
	}
	columns, err := readColumns(run)
	if err != nil {
		return "", err
	}
	exact, pattern := Exact(req.Database), req.Database
	if exact == pattern {
		return AlreadyExact, nil // no _ in the name: there is no pattern form
	}
	if !ExactFits(req.Database) {
		return NameTooLong, nil
	}
	read := func() (map[string]Row, error) {
		out, err := run("SELECT CONCAT('r=', HEX(User), ' ', HEX(Host), ' ', HEX(Db), ' ', " + flagsExpression(columns) +
			") FROM mysql.db WHERE User = " + Hex(req.User) + " AND Host = " + Hex(req.Host) +
			" AND Db IN (" + HexList([]string{pattern, exact}) + ");")
		if err != nil {
			return nil, fmt.Errorf("the database server's grants of %s could not be read: %w", account, err)
		}
		rows := map[string]Row{}
		for _, value := range Markers(out, "r") {
			row, err := parseRow(value, columns)
			if err != nil {
				return nil, err
			}
			rows[row.Stored] = row
		}
		return rows, nil
	}
	rows, err := read()
	if err != nil {
		return "", err
	}
	patternRow, hasPattern := rows[pattern]
	exactRow, hasExact := rows[exact]
	switch {
	case !hasPattern && hasExact:
		return AlreadyExact, nil
	case !hasPattern:
		return Gone, nil
	case patternRow.Flags != req.Flags:
		return Changed, nil
	case patternRow.GrantOption || patternRow.Unnamed || patternRow.Privileges == "":
		return OwnerRow, nil
	}
	target, _ := Target(req.Database)
	if !hasExact || !covers(exactRow.Flags, patternRow.Flags) {
		if fault != nil {
			if err := fault("grant"); err != nil {
				return "", err
			}
		}
		if _, err := run(NoAutoCreateUser + "\nGRANT " + patternRow.Privileges + " ON " + target + ".* TO " + account + ";"); err != nil {
			return "", fmt.Errorf("the database server did not write the exact right of %s: %w", account, err)
		}
		rows, err = read()
		if err != nil {
			return "", err
		}
		if exactRow, hasExact = rows[exact]; !hasExact || !covers(exactRow.Flags, patternRow.Flags) {
			return "", fmt.Errorf("the exact right of %s is not what was written; the pattern form was kept", account)
		}
	}
	if fault != nil {
		if err := fault("revoke"); err != nil {
			return "", err
		}
	}
	patternTarget, _ := PatternTarget(req.Database)
	if _, err := run("REVOKE ALL PRIVILEGES ON " + patternTarget + ".* FROM " + account + ";"); err != nil {
		return "", fmt.Errorf("the database server did not take away the pattern-form right of %s: %w", account, err)
	}
	rows, err = read()
	if err != nil {
		return "", err
	}
	if _, still := rows[pattern]; still {
		return "", fmt.Errorf("the pattern-form right of %s is still there", account)
	}
	if exactRow, hasExact = rows[exact]; !hasExact || !covers(exactRow.Flags, patternRow.Flags) {
		return "", fmt.Errorf("the exact right of %s changed while it was rewritten", account)
	}
	return Converted, nil
}

// covers: every flag set in want is set in have (same columns).
func covers(have, want string) bool {
	if len(have) != len(want) {
		return false
	}
	for i := range want {
		if want[i] == 'Y' && have[i] != 'Y' {
			return false
		}
	}
	return true
}

// What a statement shown for a listed row does.
const (
	// StatementNarrow: grants the same privileges by the one database's
	// exact name, then takes the pattern form away; access to that database
	// stays.
	StatementNarrow = "narrow"
	// StatementRemove: takes the right away entirely (a wildcard on purpose,
	// or a name whose exact form does not fit): the account loses this access.
	StatementRemove = "remove"
)

// NarrowStatements is what an administrator runs by hand for a listed row,
// and what it does: narrow it to one database's exact name when the row
// names one database, otherwise remove it. Shown, never run by CelikPanel for
// such rows.
func NarrowStatements(row Row) (statement, kind string) {
	grantee, err := GranteeLiteral(row.User, row.Host)
	if err != nil || strings.ContainsRune(row.Stored, '`') {
		return "", ""
	}
	revoke := "REVOKE ALL PRIVILEGES ON `" + row.Stored + "`.* FROM " + grantee + ";"
	if row.GrantOption {
		revoke += "\nREVOKE GRANT OPTION ON `" + row.Stored + "`.* FROM " + grantee + ";"
	}
	if name, pattern, ok := Decode(row.Stored); ok && pattern && row.Privileges != "" && !row.Unnamed && ExactFits(name) {
		target, _ := Target(name)
		withOption := ""
		if row.GrantOption {
			withOption = " WITH GRANT OPTION"
		}
		return "GRANT " + row.Privileges + " ON " + target + ".* TO " + grantee + withOption + ";\n" + revoke, StatementNarrow
	}
	return revoke, StatementRemove
}
