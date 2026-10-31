package main

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

// E3 (alpha.114 lab): a deferred DNS transaction that read first failed with
// SQLITE_BUSY at its first write while another connection held the write
// lock, without waiting for busy_timeout. beginDNSWriteTx waits instead.
func TestDNSWriteTxWaitsForAnotherWriterWhereADeferredOneFailsBusy(t *testing.T) {
	p := newDNSPanelForTest(t)
	pool := p.db.GetDB()
	ctx := context.Background()
	if _, err := pool.Exec(`CREATE TABLE zz_counter (v INTEGER NOT NULL); INSERT INTO zz_counter VALUES (0)`); err != nil {
		t.Fatal(err)
	}

	// Another writer (automatic DKIM in the lab) holds the write lock.
	holder, err := beginDNSWriteTx(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := holder.Exec(`UPDATE zz_counter SET v = v + 1`); err != nil {
		t.Fatal(err)
	}

	// The shape that failed: read first, then write.
	deferred, err := pool.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	var v int
	if err := deferred.QueryRow(`SELECT v FROM zz_counter`).Scan(&v); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	_, err = deferred.Exec(`UPDATE zz_counter SET v = v + 1`)
	_ = deferred.Rollback()
	if !sqliteBusy(err) || time.Since(started) > 2*time.Second {
		t.Fatalf("a deferred read-then-write did not fail busy at once (the cause of E3): %v after %s", err, time.Since(started))
	}

	// beginDNSWriteTx waits for the holder and then reads its committed value.
	released := make(chan error, 1)
	go func() {
		time.Sleep(300 * time.Millisecond)
		released <- holder.Commit()
	}()
	started = time.Now()
	tx, err := beginDNSWriteTx(ctx, pool)
	if err != nil {
		t.Fatalf("waiting writer failed: %v", err)
	}
	if waited := time.Since(started); waited < 200*time.Millisecond {
		t.Fatalf("the write lock was not held by the first transaction (waited %s)", waited)
	}
	if err := <-released; err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(`SELECT v FROM zz_counter`).Scan(&v); err != nil || v != 1 {
		t.Fatalf("read after the lock: %d %v", v, err)
	}
	if _, err := tx.Exec(`UPDATE zz_counter SET v = v + 1`); err != nil {
		t.Fatalf("write after the read: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(`SELECT v FROM zz_counter`).Scan(&v); err != nil || v != 2 {
		t.Fatalf("final value %d %v", v, err)
	}
}

type codedSQLiteError int

func (e codedSQLiteError) Error() string { return fmt.Sprintf("sqlite %d", int(e)) }
func (e codedSQLiteError) Code() int     { return int(e) }

func TestSQLiteBusyRecognisesBusyAndLockedOnly(t *testing.T) {
	for err, want := range map[error]bool{
		nil: false,
		errors.New("database is locked (5) (SQLITE_BUSY)"):        true,
		fmt.Errorf("advance zone SOA: %w", codedSQLiteError(5)):   true,
		fmt.Errorf("advance zone SOA: %w", codedSQLiteError(517)): true, // SQLITE_BUSY_SNAPSHOT
		codedSQLiteError(6):                        true,
		codedSQLiteError(19):                       false, // constraint
		errors.New("zone example.test has no SOA"): false,
	} {
		if got := sqliteBusy(err); got != want {
			t.Errorf("%v: %v, want %v", err, got, want)
		}
	}
}
