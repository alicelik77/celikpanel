package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/backupspec"
	"github.com/alicelik/celikpanel/internal/transport"
)

type fakePGExec struct {
	execs  []string
	copies []string
	data   []string
	failOn string
}

func (f *fakePGExec) Exec(_ context.Context, sql string) error {
	f.execs = append(f.execs, sql)
	if f.failOn != "" && strings.Contains(sql, f.failOn) {
		return errors.New("refused by server")
	}
	return nil
}

func (f *fakePGExec) CopyFrom(_ context.Context, sql string, data io.Reader) error {
	f.copies = append(f.copies, sql)
	b, _ := io.ReadAll(data)
	f.data = append(f.data, string(b))
	return nil
}

const pgDumpSample = `--
-- PostgreSQL database dump
--
\restrict abcDEF123

SET statement_timeout = 0;
SELECT pg_catalog.set_config('search_path', '', false);
CREATE FUNCTION public.f() RETURNS trigger
    LANGUAGE plpgsql
    AS $_$
BEGIN
  -- a ; inside a dollar quote
  RAISE NOTICE 'x;y';
  RETURN NEW;
END;
$_$;
ALTER FUNCTION public.f() OWNER TO olduser;
CREATE TABLE public.t (id integer, note text DEFAULT 'it''s; fine', e text DEFAULT E'a\';b');
ALTER TABLE public.t OWNER TO olduser;
COPY public.t (id, note, e) FROM stdin;
1	semi;colon	\N
2	back\\slash	x
\.

/* block ; comment /* nested ; */ still */
CREATE INDEX t_id ON public.t USING btree (id);
BEGIN;
COMMIT;
SET SESSION AUTHORIZATION 'olduser';
CREATE FUNCTION public.g() RETURNS int LANGUAGE sql BEGIN ATOMIC SELECT 1; SELECT 2; END;
\unrestrict abcDEF123
`

func TestLoadPGStatementsSplitsLikePostgreSQL(t *testing.T) {
	exec := &fakePGExec{}
	if err := loadPGStatements(context.Background(), exec, strings.NewReader(pgDumpSample)); err != nil {
		t.Fatalf("load: %v", err)
	}
	all := strings.Join(exec.execs, "\n")
	for _, want := range []string{"RAISE NOTICE 'x;y';", "'it''s; fine'", `E'a\';b'`, "CREATE INDEX t_id", "BEGIN ATOMIC SELECT 1; SELECT 2; END;"} {
		if !strings.Contains(all, want) {
			t.Errorf("executed SQL lacks %q:\n%s", want, all)
		}
	}
	for _, unwanted := range []string{"OWNER TO", "SESSION AUTHORIZATION", "\nBEGIN;", "COMMIT", `\restrict`} {
		if strings.Contains(all, unwanted) {
			t.Errorf("executed SQL contains %q:\n%s", unwanted, all)
		}
	}
	if len(exec.copies) != 1 || !strings.HasPrefix(exec.copies[0], "COPY public.t") {
		t.Fatalf("copies = %q", exec.copies)
	}
	if exec.data[0] != "1\tsemi;colon\t\\N\n2\tback\\\\slash\tx\n" {
		t.Fatalf("copy data = %q", exec.data[0])
	}
}

func TestLoadPGStatementsRefusesPsqlCommands(t *testing.T) {
	for _, file := range []string{
		"SELECT 1;\n\\! id\n",
		"\\copy t FROM '/etc/shadow'\n",
		"CREATE TABLE a (b int);\n  \\c postgres\n",
		"\\i /etc/passwd\n",
	} {
		exec := &fakePGExec{}
		err := loadPGStatements(context.Background(), exec, strings.NewReader(file))
		var refusal *pgLoadRefusal
		if !errors.As(err, &refusal) || refusal.Code != transport.DBImportCodeClientCommand {
			t.Fatalf("%q: err = %v", file, err)
		}
	}
	// A backslash inside a string at a line start is data, not a command.
	exec := &fakePGExec{}
	if err := loadPGStatements(context.Background(), exec, strings.NewReader("INSERT INTO t VALUES ('a\n\\! not a command');\n")); err != nil {
		t.Fatalf("quoted backslash: %v", err)
	}
}

func TestLoadPGStatementsRefusesTransactionSteering(t *testing.T) {
	for _, file := range []string{"CREATE TABLE a (b int);\nROLLBACK;\n", "SAVEPOINT x;\n", "PREPARE TRANSACTION 'x';\n"} {
		err := loadPGStatements(context.Background(), &fakePGExec{}, strings.NewReader(file))
		var refusal *pgLoadRefusal
		if !errors.As(err, &refusal) || refusal.Code != transport.DBImportCodeTransaction {
			t.Fatalf("%q: err = %v", file, err)
		}
	}
}

func TestLoadPGStatementsReportsTheFailingLine(t *testing.T) {
	exec := &fakePGExec{failOn: "broken"}
	err := loadPGStatements(context.Background(), exec, strings.NewReader("SELECT 1;\n\nSELECT broken;\n"))
	var statement *pgLoadStatementError
	if !errors.As(err, &statement) || statement.Line != 1 || statement.EndLine != 3 {
		t.Fatalf("err = %#v", err)
	}
	// The batch is named as a range, not as its first line (alpha.128 lab).
	if got := statement.where(); got != "starting on one of lines 1–3" {
		t.Fatalf("where = %q", got)
	}
	single := &pgLoadStatementError{Line: 4, EndLine: 4}
	if got := single.where(); got != "near line 4" {
		t.Fatalf("single where = %q", got)
	}
}

func TestParseMariaDBCheck(t *testing.T) {
	out := "shop.orders                                        OK\n" +
		"shop.items\n" +
		"note     : The storage engine for the table doesn't support repair\n" +
		"shop.broken\n" +
		"error    : Table upgrade required\n" +
		"status   : Operation failed\n"
	got := parseMariaDBCheck("shop", out)
	if got.Checked != 3 || got.SelfRepairing != 1 || len(got.Problems) != 2 || got.Problems[0].Table != "broken" {
		t.Fatalf("parsed = %+v", got)
	}
}

func TestDatabaseExportAndUploadFilesStayPrivateAndGo(t *testing.T) {
	old := backupBaseDir
	backupBaseDir = t.TempDir()
	t.Cleanup(func() { backupBaseDir = old })
	oldDump := dumpDatabaseToFile
	dumpDatabaseToFile = func(_ backupspec.DatabaseIdentity, destination string) error {
		return os.WriteFile(destination, []byte("dump-bytes"), 0o600)
	}
	t.Cleanup(func() { dumpDatabaseToFile = oldDump })
	target := transport.DBToolsTarget{SubscriptionID: 3, DomainID: 7, Domain: "example.com",
		Database: backupspec.DatabaseIdentity{ID: 9, Name: "example_com_shop", Type: "mysql"}}
	a := &Agent{}
	id := strings.Repeat("ab", 16)

	var exported transport.DBToolsExportResponse
	if err := a.DatabaseExportV1(transport.DBToolsExportRequest{Target: target, ExportID: id}, &exported); err != nil || exported.Error != "" || exported.Size != 10 {
		t.Fatalf("export = %+v %v", exported, err)
	}
	var chunk transport.DBToolsReadExportResponse
	_ = a.DatabaseExportReadV1(transport.DBToolsReadExportRequest{Target: target, ExportID: id, MaxBytes: 4}, &chunk)
	if string(chunk.Data) != "dump" || chunk.EOF {
		t.Fatalf("chunk = %+v", chunk)
	}
	_ = a.DatabaseExportReadV1(transport.DBToolsReadExportRequest{Target: target, ExportID: id, Offset: 4}, &chunk)
	if string(chunk.Data) != "-bytes" || !chunk.EOF {
		t.Fatalf("chunk = %+v", chunk)
	}
	if path, _ := dbToolsFile(dbToolsScope(target), "export", id); fileExistsAgent(path) {
		t.Fatal("export file kept after its last byte")
	}
	// Another tenant's IDs never reach this file.
	if err := a.DatabaseExportReadV1(transport.DBToolsReadExportRequest{Target: target, ExportID: "../../x"}, &chunk); err != nil || chunk.Error == "" {
		t.Fatalf("bad id accepted: %+v", chunk)
	}

	var up transport.DBToolsUploadChunkResponse
	_ = a.DatabaseImportUploadV1(transport.DBToolsUploadChunkRequest{Target: target, ImportID: id, Data: []byte("CREATE ")}, &up)
	_ = a.DatabaseImportUploadV1(transport.DBToolsUploadChunkRequest{Target: target, ImportID: id, Offset: 7, Data: []byte("TABLE")}, &up)
	// The same chunk again after a lost answer is not written twice.
	_ = a.DatabaseImportUploadV1(transport.DBToolsUploadChunkRequest{Target: target, ImportID: id, Offset: 7, Data: []byte("TABLE")}, &up)
	if up.Size != 12 || up.Error != "" {
		t.Fatalf("upload = %+v", up)
	}
	_ = a.DatabaseImportUploadV1(transport.DBToolsUploadChunkRequest{Target: target, ImportID: id, Offset: 3, Data: []byte("x")}, &up)
	if up.Error == "" || up.Size != 12 {
		t.Fatalf("out-of-order chunk accepted: %+v", up)
	}
	path, _ := dbToolsFile(dbToolsScope(target), "import", id)
	if filepath.Dir(path) != filepath.Join(backupBaseDir, "subscriptions", "3", "domains", "7") {
		t.Fatalf("upload outside the domain's private folder: %s", path)
	}

	// No safety copy named: nothing is loaded.
	var imported transport.DBToolsImportResponse
	_ = a.DatabaseImportV1(transport.DBToolsImportRequest{Target: target, ImportID: id, User: "u", Password: "p"}, &imported)
	if imported.Code != transport.DBImportCodeSafetyRequired || imported.Success {
		t.Fatalf("import without safety copy = %+v", imported)
	}
}

// A stopped restore is typed with the engine's own reason (alpha.127 lab: a
// refused PostgreSQL restore answered a generic error).
func TestRestoreFailureIsTypedWithTheEngineReason(t *testing.T) {
	engine := errors.New("sudo failed: exit status 3: psql:<stdin>:12: ERROR:  relation \"t\" already exists\n")
	code, detail := restoreFailureOf(restoreRollbackError(engine, nil))
	if code != backupspec.RestoreCodeLoadReverted || detail != `ERROR: relation "t" already exists` {
		t.Fatalf("code %q detail %q", code, detail)
	}
	code, _ = restoreFailureOf(restoreRollbackError(engine, errors.New("rollback broke")))
	if code != backupspec.RestoreCodeRollbackFailed {
		t.Fatalf("rollback failure code %q", code)
	}
	if code, _ := restoreFailureOf(errors.New("plain")); code != "" {
		t.Fatalf("untyped error typed as %q", code)
	}
}
