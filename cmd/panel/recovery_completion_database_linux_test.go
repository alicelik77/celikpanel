//go:build linux

package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	paneldb "github.com/alicelik/celikpanel/internal/db"
)

func TestCompletedUpdateDatabaseWALAwareChecksExactLatestContract(t *testing.T) {
	repository, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	latest, err := paneldb.HighestEmbeddedMigrationVersion()
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"complete", "older-schema", "missing-latest-row", "fake-latest", "future-row", "history-gap", "foreign-table", "changed-schema", "changed-filename", "changed-digest", "legacy-null-identities"} {
		t.Run(kind, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "celikpanel.db")
			version := latest
			if kind == "older-schema" || kind == "fake-latest" {
				version--
			}
			seedRecoveryCheckerDatabase(t, repository, path, version)
			database, err := sql.Open("sqlite", sqliteSnapshotURI(path, false))
			if err != nil {
				t.Fatal(err)
			}
			statement := ""
			switch kind {
			case "missing-latest-row":
				statement = fmt.Sprintf("DELETE FROM schema_migrations WHERE version=%d", latest)
			case "fake-latest":
				statement = fmt.Sprintf("INSERT INTO schema_migrations(version,filename,sha256) VALUES(%d,'fake.sql','fake')", latest)
			case "future-row":
				statement = fmt.Sprintf("INSERT INTO schema_migrations(version,filename,sha256) VALUES(%d,'future.sql','future')", latest+1)
			case "history-gap":
				statement = "DELETE FROM schema_migrations WHERE version=20"
			case "foreign-table":
				statement = "CREATE TABLE owner_extra(value TEXT)"
			case "changed-schema":
				statement = "ALTER TABLE users ADD COLUMN unreviewed TEXT"
			case "changed-filename":
				statement = "UPDATE schema_migrations SET filename='foreign.sql' WHERE version=1"
			case "changed-digest":
				statement = "UPDATE schema_migrations SET sha256=lower(hex(zeroblob(32))) WHERE version=1"
			case "legacy-null-identities":
				statement = "UPDATE schema_migrations SET filename=NULL,sha256=NULL WHERE version=1"
			}
			if statement != "" {
				if _, err := database.Exec(statement); err != nil {
					database.Close()
					t.Fatal(err)
				}
			}
			if err := database.Close(); err != nil {
				t.Fatal(err)
			}
			before := captureSQLiteSourceStates(t, path)
			err = checkCompletedUpdateDatabaseWALAware(path)
			if (err == nil) != (kind == "complete") {
				t.Fatalf("completion result: %v", err)
			}
			if after := captureSQLiteSourceStates(t, path); !reflect.DeepEqual(before, after) {
				t.Fatal("completion proof changed database or source sidecars")
			}
			// Older schemas remain compatible with the original queue-only
			// reader. The new completion contract must not change that API.
			if kind == "older-schema" || kind == "missing-latest-row" {
				if err := checkWALAwareServiceOperationsIdle(path); err != nil {
					t.Fatalf("legacy queue check semantics changed: %v", err)
				}
			}
		})
	}
}

func TestCompletedUpdateDatabaseReadsCommittedWALWithoutChangingSource(t *testing.T) {
	for _, kind := range []string{"complete", "active-in-wal", "schema-change-in-wal", "identity-change-in-wal"} {
		t.Run(kind, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "celikpanel.db")
			database := openWALAwareTestDatabase(t, path)
			defer database.Close()
			writeWALAwareIdleMarker(t, database, "completion-wal-proof")
			switch kind {
			case "active-in-wal":
				panel := &Panel{db: database}
				if _, err := panel.createServiceOperation(context.Background(), serviceOperationKindInstall, "certbot", "", serviceOperationActor{}); err != nil {
					t.Fatal(err)
				}
			case "schema-change-in-wal":
				if _, err := database.GetDB().Exec("CREATE TABLE unreviewed_wal_table(value TEXT)"); err != nil {
					t.Fatal(err)
				}
			case "identity-change-in-wal":
				if _, err := database.GetDB().Exec("UPDATE schema_migrations SET sha256='changed' WHERE version=1"); err != nil {
					t.Fatal(err)
				}
			}
			requireNonEmptyWAL(t, path)
			before := captureSQLiteSourceStates(t, path)
			err := checkCompletedUpdateDatabaseWALAware(path)
			if (err == nil) != (kind == "complete") {
				t.Fatalf("WAL completion result: %v", err)
			}
			if after := captureSQLiteSourceStates(t, path); !reflect.DeepEqual(before, after) {
				t.Fatal("completion proof changed source DB/WAL/SHM")
			}
		})
	}
}

// alpha.124 Boston (123 -> 124): the late database proof ran seven seconds
// after the new panel started and failed with an empty detail; recovery then
// verified the same database with the services stopped. A panel commit while
// the private copy is taken is read again as a whole new proof; any other
// refusal stays final on the first read.
func TestRunningPanelDatabaseProofRereadsOnlyAConcurrentWrite(t *testing.T) {
	oldAttempts, oldDelay, oldSleep := runningPanelDatabaseReadAttempts, runningPanelDatabaseReadDelay, runningPanelDatabaseReadSleep
	t.Cleanup(func() {
		runningPanelDatabaseReadAttempts, runningPanelDatabaseReadDelay, runningPanelDatabaseReadSleep = oldAttempts, oldDelay, oldSleep
	})
	runningPanelDatabaseReadAttempts, runningPanelDatabaseReadDelay = 4, time.Millisecond
	sleeps := 0
	runningPanelDatabaseReadSleep = func(time.Duration) { sleeps++ }
	concurrent := &panelDatabaseChangedError{detail: "SQLite sidecar -wal changed after pinning"}
	final := fmt.Errorf("%w: operation x is running", errServiceOperationsNotIdle)
	for _, test := range []struct {
		name       string
		results    []error
		wantCalls  int
		wantSleeps int
		wantErr    bool
	}{
		{"settles-after-two-writes", []error{concurrent, concurrent, nil}, 3, 2, false},
		{"other-refusal-is-final", []error{final, nil}, 1, 0, true},
		{"write-then-real-refusal", []error{concurrent, final, nil}, 2, 1, true},
		{"never-settles", []error{concurrent, concurrent, concurrent, concurrent, nil}, 4, 3, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			sleeps = 0
			err := settleRunningPanelDatabaseReads(func() error {
				calls++
				return test.results[calls-1]
			})
			if (err != nil) != test.wantErr || calls != test.wantCalls || sleeps != test.wantSleeps {
				t.Fatalf("err=%v calls=%d sleeps=%d", err, calls, sleeps)
			}
			if test.name == "never-settles" && (!errors.Is(err, errServiceOperationsNotIdle) || !strings.Contains(err.Error(), "4 read-only reads")) {
				t.Fatalf("the final refusal does not name the repeated writes: %v", err)
			}
			if test.name != "never-settles" && err != nil && !errors.Is(err, final) {
				t.Fatalf("a final refusal was replaced: %v", err)
			}
		})
	}

	// The real WAL-aware completion proof on a database a panel is writing.
	path := filepath.Join(t.TempDir(), "celikpanel.db")
	database := openWALAwareTestDatabase(t, path)
	defer database.Close()
	writeWALAwareIdleMarker(t, database, "completed-update")
	writes := 0
	proof := func() error {
		return checkWALAwareServiceOperationsIdleWith(path, func(copy string) error {
			if writes < 2 {
				writes++
				writeWALAwareIdleMarker(t, database, fmt.Sprintf("panel-startup-write-%d", writes))
			}
			return checkCompletedUpdateDatabase(copy)
		})
	}
	if err := proof(); !isConcurrentPanelDatabaseWrite(err) {
		t.Fatalf("a panel commit during the copy was not the concurrent-write refusal: %v", err)
	}
	sleeps = 0
	if err := settleRunningPanelDatabaseReads(proof); err != nil || sleeps != 1 {
		t.Fatalf("the running database was not verified after the panel's writes: %v (sleeps %d)", err, sleeps)
	}
}

// A WAL frame being appended (or a checkpoint) while the source is copied
// leaves a torn tail; with the pinned files seen to change that is the
// concurrent-write refusal. Unchanged files keep the original failure.
func TestTornLiveSQLiteReadIsAConcurrentWriteOnlyWhenTheFilesChanged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "celikpanel.db")
	for _, suffix := range []string{"", "-wal"} {
		if err := os.WriteFile(path+suffix, []byte("pinned"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	pinned, err := pinWALAwarePanelDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	defer pinned.close()
	torn := fmt.Errorf("%w: recover pinned SQLite WAL: SQLite WAL has an incomplete 24-byte tail", errServiceOperationsNotIdle)
	if got := explainTornLiveSQLiteRead(pinned, torn); got != torn {
		t.Fatalf("an unchanged torn WAL was reclassified: %v", got)
	}
	time.Sleep(10 * time.Millisecond)
	file, err := os.OpenFile(path+"-wal", os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("+frame"); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if got := explainTornLiveSQLiteRead(pinned, torn); !isConcurrentPanelDatabaseWrite(got) {
		t.Fatalf("a torn read of a changing WAL is not the concurrent-write refusal: %v", got)
	}
}
