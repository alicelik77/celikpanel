package mariadbgrant

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// droppedFake answers the sweep's statements. rows are mysql.db rows
// ("<hex user>/<hex host>/<hex db>"), others the table/column/routine
// grantees, schemas the database names; left answers the counts (a, b, c)
// per call; flushFails makes the FLUSH inside the run fail.
type droppedFake struct {
	exists     string
	rows       []string
	others     []string
	schemas    []string
	left       []string
	flushFails int
	log        []string
}

func (f *droppedFake) run(statement string) ([]byte, error) {
	f.log = append(f.log, statement)
	switch {
	case strings.HasPrefix(statement, "SELECT CONCAT('n=', COUNT(*)) FROM information_schema.SCHEMATA"):
		return []byte("n=" + f.exists + "\n"), nil
	case strings.HasPrefix(statement, "SELECT CONCAT('l='"):
		var b strings.Builder
		b.WriteString("l=0\n")
		for _, s := range f.schemas {
			b.WriteString("s=" + hexOf(s) + "\n")
		}
		for _, r := range f.rows {
			b.WriteString("d=" + r + "\n")
		}
		for _, g := range f.others {
			b.WriteString("g=" + g + "\n")
		}
		return []byte(b.String()), nil
	case statement == "FLUSH PRIVILEGES;":
		if f.flushFails > 0 {
			f.flushFails--
			return nil, errors.New("flush refused")
		}
		return nil, nil
	case strings.HasPrefix(statement, "SELECT CONCAT('a='"):
		answer := "a=0\nb=0\nc=0\n"
		if len(f.left) > 0 {
			answer, f.left = f.left[0], f.left[1:]
		}
		return []byte(answer), nil
	case strings.HasPrefix(statement, "DELETE"):
		return nil, nil
	}
	return nil, errors.New("unexpected " + statement)
}

func dRow(user, host, db string) string { return hexOf(user) + "/" + hexOf(host) + "/" + hexOf(db) }

func TestRemoveDroppedRightsTakesEveryGranteeAndNamesThem(t *testing.T) {
	f := &droppedFake{exists: "0", rows: []string{
		dRow("hand", "localhost", "a_b"), dRow("reporting", "", `a\_b`), dRow("", "%", `a\_b`),
	}, others: []string{hexOf("hand") + "/" + hexOf("localhost")}}
	result, err := RemoveDroppedRights(f.run, "a_b")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, g := range result.Removed {
		names = append(names, g.String())
	}
	// The anonymous account is ''@host, never a role named after its host.
	if strings.Join(names, ",") != "''@%,hand@localhost,reporting" || len(result.Kept) != 0 {
		t.Fatalf("removed %v kept %v", names, result.Kept)
	}
	all := strings.Join(f.log, "\n")
	if !strings.Contains(all, "DELETE FROM mysql.db WHERE User = X'68616E64' AND Host = X'6C6F63616C686F7374' AND Db = X'615F62'"+absentGuard("a_b")) ||
		strings.Count(all, absentGuard("a_b")) != 6 {
		t.Fatalf("statements:\n%s", all)
	}
}

// A raw-form row that also reaches another existing database is kept and
// reported, with what it still reaches; the exact row of the same name goes.
func TestRemoveDroppedRightsKeepsARawRowThatReachesAnotherDatabase(t *testing.T) {
	f := &droppedFake{exists: "0", schemas: []string{"a1b", "other"}, rows: []string{
		dRow("hand", "%", "a_b"), dRow("cust", "localhost", `a\_b`),
	}, left: []string{"a=0\nb=1\nc=0\n"}}
	result, err := RemoveDroppedRights(f.run, "a_b")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Kept) != 1 || result.Kept[0].User != "hand" || strings.Join(result.Kept[0].Reaches, ",") != "a1b" {
		t.Fatalf("kept %+v", result.Kept)
	}
	if len(result.Removed) != 1 || result.Removed[0].String() != "cust@localhost" {
		t.Fatalf("removed %+v", result.Removed)
	}
	for _, statement := range f.log {
		if strings.Contains(statement, "User = X'68616E64'") && strings.HasPrefix(statement, "DELETE") {
			t.Fatalf("the kept row was deleted: %s", statement)
		}
	}
	var left string
	for _, statement := range f.log {
		if strings.HasPrefix(statement, "SELECT CONCAT('a='") {
			left = statement
		}
	}
	if !strings.Contains(left, "GRANTEE NOT IN") || !strings.Contains(left, "FROM mysql.columns_priv") {
		t.Fatalf("left check: %s", left)
	}
}

// A FLUSH that failed after the DELETEs (here, or on an earlier press whose
// rows are gone from the tables already) is repaired: whenever something is
// still counted, FLUSH runs on its own and the count is read again.
func TestRemoveDroppedRightsFlushesAgainWhenRightsAreStillLoaded(t *testing.T) {
	f := &droppedFake{exists: "0", left: []string{"a=3\nb=0\nc=0\n", "a=0\nb=0\nc=0\n"}}
	if _, err := RemoveDroppedRights(f.run, "a_b"); err != nil {
		t.Fatalf("a second press after a failed flush: %v", err)
	}
	flushes := 0
	for _, statement := range f.log {
		if statement == "FLUSH PRIVILEGES;" {
			flushes++
		}
	}
	if flushes != 1 {
		t.Fatalf("flushes %d: %v", flushes, f.log)
	}
	f = &droppedFake{exists: "0", rows: []string{dRow("u", "localhost", `a\_b`)}, flushFails: 1}
	if _, err := RemoveDroppedRights(f.run, "a_b"); err == nil {
		t.Fatal("a failed flush was not reported")
	}
	f = &droppedFake{exists: "0", left: []string{"a=2\nb=0\nc=0\n", "a=2\nb=0\nc=0\n"}}
	var left *RightsLeftError
	if _, err := RemoveDroppedRights(f.run, "a_b"); !errors.As(err, &left) || left.Left != 2 {
		t.Fatalf("still loaded after the flush: %v", err)
	}
	f = &droppedFake{exists: "1"}
	if _, err := RemoveDroppedRights(f.run, "a_b"); !errors.Is(err, ErrDatabaseExists) || len(f.log) != 1 {
		t.Fatalf("exists: %v %v", err, f.log)
	}
}

// Measured on MariaDB 11.8.6: the exact form of a 64-character name with
// several `_` is longer than mysql.db.Db and is stored cut short - a grant
// on another name. Such a name is never written in the exact form, and a
// GRANT is read back.
func TestLongNamesAreNeverGrantedCutShort(t *testing.T) {
	long := "abcd_efgh_ijkl_mnop_qrst_uvwx_yzab_cdef_ghij_klmn_opqr_stuv_wxyz"
	if len(long) != 64 || ExactFits(long) {
		t.Fatal("fixture")
	}
	if _, err := Target(long); !errors.Is(err, ErrExactTooLong) {
		t.Fatalf("Target(%q) = %v", long, err)
	}
	if !ExactFits("s5__x") {
		t.Fatal("a short name does not fit")
	}
	if statement, kind := NarrowStatements(Row{User: "u", Host: "localhost", Stored: long, Privileges: "SELECT"}); kind != StatementRemove || !strings.HasPrefix(statement, "REVOKE") {
		t.Fatalf("a too long name is offered %q (%s)", statement, kind)
	}
	run := func(answer string) Exec {
		return func(string) ([]byte, error) { return []byte(answer), nil }
	}
	if err := VerifyStored(run("n=1\n"), "u", "localhost", "a_b"); err != nil {
		t.Fatal(err)
	}
	if err := VerifyStored(run("n=0\n"), "u", "localhost", "a_b"); !errors.Is(err, ErrNotStored) {
		t.Fatalf("not stored: %v", err)
	}
	if !strings.Contains(StoredStatement("u", "localhost", Exact("a_b")), "Db = X'615C5F62'") {
		t.Fatal(StoredStatement("u", "localhost", Exact("a_b")))
	}
	// Follow-up: an account that may not read mysql.db is a verified answer
	// (the write stands on the length rule); any other failure stays unknown.
	denied := func(string) ([]byte, error) { return nil, fmt.Errorf("MariaDB command failed: %w", ErrReadDenied) }
	if err := VerifyStored(denied, "u", "localhost", "a_b"); !errors.Is(err, ErrUnverifiable) {
		t.Fatalf("read denied: %v", err)
	}
	down := func(string) ([]byte, error) { return nil, errors.New("connection lost") }
	if err := VerifyStored(down, "u", "localhost", "a_b"); err == nil || errors.Is(err, ErrUnverifiable) || errors.Is(err, ErrNotStored) {
		t.Fatalf("unknown taken for an answer: %v", err)
	}
	if !ReadDenied("ERROR 1142 (42000) at line 1: SELECT command denied to user 'p'@'localhost' for table `mysql`.`db`") || ReadDenied("ERROR 1045 (28000): Access denied for user") {
		t.Fatal("ReadDenied")
	}
	// An existing database whose exact form does not fit keeps its pattern form.
	if target, stored, err := GrantTarget(long); err != nil || target != "`"+long+"`" || stored != long {
		t.Fatalf("GrantTarget(long) = %q %q %v", target, stored, err)
	}
	if target, stored, err := GrantTarget("a_b"); err != nil || target != "`a\\_b`" || stored != "a\\_b" {
		t.Fatalf("GrantTarget = %q %q %v", target, stored, err)
	}
	// A generated base is shortened until base+suffix fits.
	base := strings.Repeat("a_", 20)
	if fitted := FitBase(base, "_copy_abcd"); !ExactFits(fitted+"_copy_abcd") || !strings.HasPrefix(base, fitted) || len(fitted) >= len(base) {
		t.Fatalf("FitBase = %q", fitted)
	}
	if FitBase("shop_example_com", "_wp") != "shop_example_com" {
		t.Fatal("a base that fits was changed")
	}
	if OverLimit(long) != len(Exact(long))-64 || OverLimit("a_b") != 0 {
		t.Fatal("OverLimit")
	}
}
