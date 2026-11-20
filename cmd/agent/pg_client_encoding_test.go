package main

import (
	"context"
	"io"
	"strings"
	"testing"
)

// 20 Nov 2026: a dump of a SQL_ASCII database begins with
// SET client_encoding = 'SQL_ASCII', and PostgreSQL does not check COPY data
// sent under that client encoding, so the panel's import and its copy put a
// Latin-1 byte into a UTF8 database (found on a real PostgreSQL 17 while
// testing the way out of a SQL_ASCII database). The statement is now one
// that leaves SQL_ASCII as it was everywhere but in a UTF8 database, where it
// makes the client encoding UTF8 so the server checks every byte.

func TestPGSQLASCIIClientEncodingBecomesTheCheckedOne(t *testing.T) {
	for statement, rewritten := range map[string]bool{
		"SET client_encoding = 'SQL_ASCII'":            true,
		"set client_encoding to 'sql_ascii'":           true,
		"SET client_encoding=SQL_ASCII":                true,
		"SET SESSION client_encoding = 'SQL_ASCII'":    true,
		"SET client_encoding = 'UTF8'":                 false,
		"SET client_encoding = 'LATIN1'":               false,
		"SET standard_conforming_strings = on":         false,
		"SELECT 'SET client_encoding = ''SQL_ASCII'''": false,
	} {
		got, ok := pgCheckedClientEncoding(statement)
		if ok != rewritten || (ok && got != pgCheckedClientEncodingStatement) || (!ok && got != statement) {
			t.Errorf("%q -> %q %v", statement, got, ok)
		}
	}
	if !strings.Contains(pgCheckedClientEncodingStatement, "current_setting('server_encoding') = 'UTF8' THEN 'UTF8' ELSE 'SQL_ASCII'") {
		t.Fatalf("statement = %s", pgCheckedClientEncodingStatement)
	}
}

// The import sends the checked statement in place of the dump's own.
func TestPGImportSendsTheCheckedClientEncoding(t *testing.T) {
	dump := "--\n-- PostgreSQL database dump\n--\n\nSET statement_timeout = 0;\nSET client_encoding = 'SQL_ASCII';\nSET standard_conforming_strings = on;\n" +
		"CREATE TABLE public.notes (body text);\nCOPY public.notes (body) FROM stdin;\ncaf\xe9\n\\.\n"
	exec := &recordingPGExecutor{}
	if err := loadPGStatements(context.Background(), exec, strings.NewReader(dump)); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(exec.execs, "\n")
	if strings.Contains(joined, "'SQL_ASCII';") || !strings.Contains(joined, pgCheckedClientEncodingStatement) {
		t.Fatalf("sent %q", exec.execs)
	}
}

type recordingPGExecutor struct{ execs []string }

func (r *recordingPGExecutor) Exec(_ context.Context, sql string) error {
	r.execs = append(r.execs, sql)
	return nil
}

func (r *recordingPGExecutor) CopyFrom(_ context.Context, sql string, data io.Reader) error {
	_, err := io.Copy(io.Discard, data)
	r.execs = append(r.execs, sql)
	return err
}
