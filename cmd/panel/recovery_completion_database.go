package main

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	paneldb "github.com/alicelik/celikpanel/internal/db"
)

// checkCompletedUpdateDatabaseWALAware inspects one pinned private DB/WAL copy.
// Queue idleness alone cannot prove that the target migrations ran. Completion
// additionally requires the latest embedded schema and every migration identity.
// Neither the source nor its sidecars are opened through SQLite or modified.
func checkCompletedUpdateDatabaseWALAware(databasePath string) error {
	return checkWALAwareServiceOperationsIdleWith(databasePath, checkCompletedUpdateDatabase)
}

// The late (post-start) database proofs read the running panel's database.
// Panel start-up writes (pending audit records, the mail and certificate
// outbox, sessions) and SQLite checkpoints legitimately change the database or
// its -wal/-shm while the private copy is taken; that copy is then discarded as
// torn (alpha.124, Boston 123 -> 124: the first pass failed seven seconds
// after the new panel started, and recovery verified the same database with
// the services stopped). Only that typed concurrent-write refusal is read
// again, each time as a complete new proof; every other refusal is final.
// Geç kanıtlar çalışan paneli okur; yalnız eşzamanlı yazım yeniden okunur.
var (
	runningPanelDatabaseReadAttempts = 20
	runningPanelDatabaseReadDelay    = 500 * time.Millisecond
	runningPanelDatabaseReadSleep    = time.Sleep
)

func settleRunningPanelDatabaseReads(check func() error) error {
	var err error
	for attempt := 1; attempt <= runningPanelDatabaseReadAttempts; attempt++ {
		if err = check(); err == nil || !isConcurrentPanelDatabaseWrite(err) {
			return err
		}
		if attempt < runningPanelDatabaseReadAttempts {
			runningPanelDatabaseReadSleep(runningPanelDatabaseReadDelay)
		}
	}
	return fmt.Errorf(
		"the running panel kept writing its database during %d read-only reads over about %s, so its completed state could not be read consistently yet: %w",
		runningPanelDatabaseReadAttempts,
		time.Duration(runningPanelDatabaseReadAttempts-1)*runningPanelDatabaseReadDelay,
		err,
	)
}

func checkCompletedUpdateDatabase(databasePath string) error {
	if err := checkServiceOperationsIdle(databasePath); err != nil {
		return err
	}
	latest, err := paneldb.HighestEmbeddedMigrationVersion()
	if err != nil {
		return fmt.Errorf("read completed database contract: %w", err)
	}
	if err := validateServiceOperationSnapshotSchema(databasePath, latest, true); err != nil {
		return fmt.Errorf("completed update database schema is not verified: %w", err)
	}
	database, err := sql.Open("sqlite", sqliteSnapshotURI(databasePath, true))
	if err != nil {
		return fmt.Errorf("open completed database identity read-only: %w", err)
	}
	defer database.Close()
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := paneldb.VerifyCompletedMigrationHistory(ctx, database); err != nil {
		return fmt.Errorf("completed update migration history is not verified: %w", err)
	}
	return nil
}
