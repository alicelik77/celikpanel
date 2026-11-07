package main

import (
	"os"
	"path/filepath"
	"testing"
)

// alpha.129 lab (Arch, MariaDB 13): started as "mysql" the client printed a
// deprecation line before every answer, so the ownership proof of a
// site-copy database never matched and a refused copy left the database and
// its user behind. Statements run with the "mariadb" program when it exists.
func TestMySQLStatementsUseTheMariaDBProgramWhenPresent(t *testing.T) {
	dir := t.TempDir()
	mariadb := filepath.Join(dir, "mariadb")
	if err := os.WriteFile(mariadb, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mysql"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	command := newMySQLStatementCommand("SELECT 1;")
	if command.Path != mariadb {
		t.Fatalf("program = %q, want %q", command.Path, mariadb)
	}
	if len(command.Args) != 3 || command.Args[1] != "--batch" || command.Args[2] != "--skip-column-names" {
		t.Fatalf("args = %q", command.Args)
	}
}

func TestMySQLStatementsFallBackToMySQLWithoutMariaDB(t *testing.T) {
	dir := t.TempDir()
	mysql := filepath.Join(dir, "mysql")
	if err := os.WriteFile(mysql, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	command := newMySQLStatementCommand("SELECT 1;")
	if command.Path != mysql {
		t.Fatalf("program = %q, want %q", command.Path, mysql)
	}
}

// The proof itself: a stderr line before the count must not be what is read
// once the client is the mariadb program (stdout carries only "1").
func TestMySQLOwnershipProofReadsTheCountOnly(t *testing.T) {
	previous := mysqlExecStatement
	t.Cleanup(func() { mysqlExecStatement = previous })
	mysqlExecStatement = func(string) ([]byte, error) { return []byte("1\n"), nil }
	if !verifyMySQLOwnership("`db`", testWordPressOperationID, testWordPressCleanupToken) {
		t.Fatal("a clean count of 1 must prove ownership")
	}
	mysqlExecStatement = func(string) ([]byte, error) {
		return []byte("mysql: Deprecated program name. It will be removed in a future release, use '/usr/bin/mariadb' instead\n1\n"), nil
	}
	if verifyMySQLOwnership("`db`", testWordPressOperationID, testWordPressCleanupToken) {
		t.Fatal("the proof must stay exact: anything but the count is not a proof")
	}
}
