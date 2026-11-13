package mariadbgrant

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// fakeEngine is mysql.db as the statements of this package see it: the
// MariaDB 11.8 privilege columns, rows keyed by account and stored name, and
// the database names. It answers exactly the statement shapes the package
// writes (the real-engine checks are the lab's: docs/DATABASE-USERS.md).
type fakeEngine struct {
	version string
	columns []string
	rows    map[[3]string]string // user, host, stored -> flags
	schemas []string
	log     []string
}

var mariadb118Columns = []string{"Select_priv", "Insert_priv", "Update_priv", "Delete_priv", "Create_priv", "Drop_priv",
	"Grant_priv", "References_priv", "Index_priv", "Alter_priv", "Create_tmp_table_priv", "Lock_tables_priv",
	"Create_view_priv", "Show_view_priv", "Create_routine_priv", "Alter_routine_priv", "Execute_priv", "Event_priv",
	"Trigger_priv", "Delete_history_priv", "Show_create_routine_priv"}

func newFakeEngine() *fakeEngine {
	return &fakeEngine{version: "11.8.6-MariaDB-0+deb13u1", columns: mariadb118Columns, rows: map[[3]string]string{}}
}

func (f *fakeEngine) flags(words string) string {
	set := map[string]bool{}
	for _, w := range strings.Split(words, ", ") {
		set[w] = true
	}
	var b strings.Builder
	for _, column := range f.columns {
		word := privilegeWords[column]
		if (set["ALL PRIVILEGES"] && column != grantOptionColumn) || (word != "" && set[word]) {
			b.WriteByte('Y')
		} else {
			b.WriteByte('N')
		}
	}
	return b.String()
}

var (
	fakeGrant  = regexp.MustCompile("^GRANT (.+) ON `([^`]+)`\\.\\* TO '([^']*)'@'([^']*)';$")
	fakeRevoke = regexp.MustCompile("^REVOKE ALL PRIVILEGES ON `([^`]+)`\\.\\* FROM '([^']*)'@'([^']*)';$")
	fakeWhere  = regexp.MustCompile(`WHERE User = (\S+) AND Host = (\S+) AND Db IN \(([^)]*)\)`)
)

func unhex(t string) string {
	t = strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(t), "X'"), "'")
	if t == "" {
		return ""
	}
	s, err := FromHex(t)
	if err != nil {
		panic(err)
	}
	return s
}

func hexOf(s string) string { return strings.TrimSuffix(strings.TrimPrefix(Hex(s), "X'"), "'") }

func (f *fakeEngine) run(statement string) ([]byte, error) {
	f.log = append(f.log, statement)
	var out strings.Builder
	row := func(k [3]string, flags string) {
		fmt.Fprintf(&out, "r=%s %s %s %s\n", hexOf(k[0]), hexOf(k[1]), hexOf(k[2]), flags)
	}
	switch {
	case statement == columnsStatement:
		for _, c := range f.columns {
			out.WriteString("c=" + c + "\n")
		}
	case strings.HasPrefix(statement, "SELECT CONCAT('v=', VERSION())"):
		out.WriteString("v=" + f.version + "\nl=0\n")
		keys := make([][3]string, 0, len(f.rows))
		for k := range f.rows {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool { return strings.Join(keys[i][:], "|") < strings.Join(keys[j][:], "|") })
		for _, k := range keys {
			row(k, f.rows[k])
		}
		for _, s := range f.schemas {
			out.WriteString("s=" + hexOf(s) + "\n")
		}
	case strings.HasPrefix(statement, "SELECT CONCAT('r='"):
		m := fakeWhere.FindStringSubmatch(statement)
		if m == nil {
			return nil, errors.New("unexpected read " + statement)
		}
		user, host := unhex(m[1]), unhex(m[2])
		for _, db := range strings.Split(m[3], ", ") {
			k := [3]string{user, host, unhex(db)}
			if flags, ok := f.rows[k]; ok {
				row(k, flags)
			}
		}
	case fakeGrant.MatchString(statement):
		m := fakeGrant.FindStringSubmatch(statement)
		k := [3]string{m[3], m[4], m[2]}
		have, want := f.rows[k], f.flags(m[1])
		if have == "" {
			have = strings.Repeat("N", len(want))
		}
		merged := []byte(have)
		for i := range want {
			if want[i] == 'Y' {
				merged[i] = 'Y'
			}
		}
		f.rows[k] = string(merged)
	case fakeRevoke.MatchString(statement):
		m := fakeRevoke.FindStringSubmatch(statement)
		k := [3]string{m[2], m[3], m[1]}
		if _, ok := f.rows[k]; !ok {
			return []byte("ERROR 1141 (42000): There is no such grant defined"), errors.New("exit status 1")
		}
		delete(f.rows, k)
	default:
		return nil, errors.New("unexpected statement " + statement)
	}
	return []byte(out.String()), nil
}

// reaches: whether the account's rows give it any right on the database.
func (f *fakeEngine) reaches(user, host, database string) bool {
	for k, flags := range f.rows {
		if k[0] == user && k[1] == host && strings.ContainsRune(flags, 'Y') && Matches(k[2], database, false) {
			return true
		}
	}
	return false
}

// The cross-customer pair on the conversion path: s5__x's user reaches
// s51_x before, keeps s5__x at every point of an interrupted conversion, and
// reaches only s5__x after it.
func TestConvertNeverDropsAccessAndEndsTheCrossAccess(t *testing.T) {
	for _, stop := range []string{"grant", "revoke"} {
		t.Run("interrupted before "+stop, func(t *testing.T) {
			f := newFakeEngine()
			f.schemas = []string{"s5__x", "s51_x"}
			f.rows[[3]string{"s5__x_user", "localhost", "s5__x"}] = f.flags("ALL PRIVILEGES")
			if !f.reaches("s5__x_user", "localhost", "s51_x") {
				t.Fatal("the fixture does not show the cross access")
			}
			scan, err := ScanEngine(f.run)
			if err != nil || len(scan.Patterns) != 1 || strings.Join(scan.Patterns[0].Reached, ",") != "s51_x" || !scan.MariaDB {
				t.Fatalf("scan = %+v, %v", scan, err)
			}
			req := ConvertRequest{User: "s5__x_user", Host: "localhost", Database: "s5__x", Flags: scan.Patterns[0].Flags}
			_, err = Convert(f.run, req, func(step string) error {
				if step == stop {
					return errors.New("interrupted")
				}
				return nil
			})
			if err == nil {
				t.Fatal("the interruption was not reported")
			}
			if !f.reaches("s5__x_user", "localhost", "s5__x") {
				t.Fatalf("access lost when interrupted before %s", stop)
			}
			// Run again: it continues.
			outcome, err := Convert(f.run, req, nil)
			if err != nil || outcome != Converted {
				t.Fatalf("continue: %s %v", outcome, err)
			}
			if !f.reaches("s5__x_user", "localhost", "s5__x") || f.reaches("s5__x_user", "localhost", "s51_x") {
				t.Fatalf("after: rows %v", f.rows)
			}
			if _, ok := f.rows[[3]string{"s5__x_user", "localhost", `s5\_\_x`}]; !ok {
				t.Fatalf("the exact row is missing: %v", f.rows)
			}
			// And again: nothing more to do.
			if outcome, err := Convert(f.run, req, nil); err != nil || outcome != AlreadyExact {
				t.Fatalf("third run: %s %v", outcome, err)
			}
			if scan, _ := ScanEngine(f.run); len(scan.Patterns) != 0 {
				t.Fatalf("pattern rows left: %+v", scan.Patterns)
			}
		})
	}
}

func TestConvertTouchesNothingThatIsNotTheCapturedRow(t *testing.T) {
	f := newFakeEngine()
	f.rows[[3]string{"u", "localhost", "a_b"}] = f.flags("SELECT")
	captured := f.flags("SELECT, INSERT")
	if outcome, err := Convert(f.run, ConvertRequest{User: "u", Host: "localhost", Database: "a_b", Flags: captured}, nil); err != nil || outcome != Changed {
		t.Fatalf("changed row: %s %v", outcome, err)
	}
	withOption := []byte(f.flags("SELECT"))
	withOption[6] = 'Y' // Grant_priv
	f.rows[[3]string{"u", "localhost", "a_b"}] = string(withOption)
	if outcome, err := Convert(f.run, ConvertRequest{User: "u", Host: "localhost", Database: "a_b", Flags: string(withOption)}, nil); err != nil || outcome != OwnerRow {
		t.Fatalf("grant option: %s %v", outcome, err)
	}
	for _, statement := range f.log {
		if strings.HasPrefix(statement, "GRANT") || strings.HasPrefix(statement, "REVOKE") {
			t.Fatalf("a row that was not the captured one was changed: %s", statement)
		}
	}
	delete(f.rows, [3]string{"u", "localhost", "a_b"})
	if outcome, _ := Convert(f.run, ConvertRequest{User: "u", Host: "localhost", Database: "a_b", Flags: captured}, nil); outcome != Gone {
		t.Fatalf("gone: %s", outcome)
	}
	if _, err := Convert(f.run, ConvertRequest{User: "u'x", Host: "localhost", Database: "a_b"}, nil); err == nil {
		t.Fatal("an account CelikPanel does not write was accepted")
	}
}

// A row whose privilege set differs from CelikPanel's record is converted
// as it is (here SELECT and INSERT), never widened or narrowed.
func TestConvertKeepsThePrivilegesAsTheyAre(t *testing.T) {
	f := newFakeEngine()
	key := [3]string{"u", "10.0.0.0/255.255.255.0", "shop_a"}
	f.rows[key] = f.flags("SELECT, INSERT")
	outcome, err := Convert(f.run, ConvertRequest{User: key[0], Host: key[1], Database: "shop_a", Flags: f.rows[key]}, nil)
	if err != nil || outcome != Converted {
		t.Fatalf("%s %v", outcome, err)
	}
	if got := f.rows[[3]string{key[0], key[1], `shop\_a`}]; got != f.flags("SELECT, INSERT") {
		t.Fatalf("exact flags %q", got)
	}
	var grant string
	for _, statement := range f.log {
		if strings.HasPrefix(statement, "GRANT") {
			grant = statement
		}
	}
	if grant != "GRANT SELECT, INSERT ON `shop\\_a`.* TO 'u'@'10.0.0.0/255.255.255.0';" {
		t.Fatalf("grant %q", grant)
	}
}

// What an administrator is shown for a row CelikPanel cannot prove its own.
func TestNarrowStatements(t *testing.T) {
	f := newFakeEngine()
	row := Row{User: "hand", Host: "%", Stored: "shop_a", Flags: f.flags("SELECT")}
	describe(f.columns, &row)
	if got := NarrowStatements(row); got != "GRANT SELECT ON `shop\\_a`.* TO 'hand'@'%';\nREVOKE ALL PRIVILEGES ON `shop_a`.* FROM 'hand'@'%';" {
		t.Fatalf("narrow = %q", got)
	}
	wild := Row{User: "hand", Host: "%", Stored: `shop\_%`, Flags: f.flags("SELECT")}
	describe(f.columns, &wild)
	if got := NarrowStatements(wild); got != "REVOKE ALL PRIVILEGES ON `shop\\_%`.* FROM 'hand'@'%';" {
		t.Fatalf("wildcard = %q", got)
	}
}
