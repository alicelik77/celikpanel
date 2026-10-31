package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// DNS write transactions take SQLite's write lock with their first statement
// (alpha.114 lab, E3).
//
// database/sql begins SQLite transactions as DEFERRED. The DNS publication
// reads first (engine authority, sync state, the SOA) and writes after (the
// next SOA serial, the lease). In WAL mode that read-to-write upgrade does not
// wait: while another connection holds the write lock, or has committed since
// this transaction's first read, SQLite answers SQLITE_BUSY at once, and the
// pool's busy_timeout(5000) does not apply (waiting could deadlock two
// upgrading readers). Automatic DKIM wrote its record into the same zone
// between those two steps, and the customer's saved record answered HTTP 500
// "advance zone SOA: database is locked".
//
// The first statement of a DNS write transaction is therefore a write that
// changes nothing. It runs before any read, when the transaction has no
// snapshot yet, so SQLite waits for the write lock up to busy_timeout exactly
// like BEGIN IMMEDIATE, and the reads that follow see the latest data.
//
// DNS yazma işlemleri SQLite yazma kilidini ilk ifadeleriyle alır: önce okuyup
// sonra yazan ertelenmiş işlem WAL kipinde beklemeden SQLITE_BUSY döndürüyordu.

const dnsWriteLockStatement = `UPDATE dns_zone_sync_state SET zone_name = zone_name WHERE 0`

// beginDNSWriteTx begins a transaction that already holds the write lock.
func beginDNSWriteTx(ctx context.Context, database *sql.DB) (*sql.Tx, error) {
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, dnsWriteLockStatement); err != nil {
		_ = tx.Rollback()
		return nil, fmt.Errorf("wait for the panel database's write lock: %w", err)
	}
	return tx, nil
}

// sqliteBusy reports whether err is SQLite's "database is locked" or "table
// is locked" (SQLITE_BUSY / SQLITE_LOCKED, any extended code): another writer
// held the database longer than the busy timeout. Nothing of the failed
// transaction was stored; the same work can run again.
func sqliteBusy(err error) bool {
	if err == nil {
		return false
	}
	var coded interface{ Code() int }
	if errors.As(err, &coded) {
		switch coded.Code() & 0xff {
		case 5, 6: // SQLITE_BUSY, SQLITE_LOCKED
			return true
		}
	}
	text := err.Error()
	return strings.Contains(text, "SQLITE_BUSY") || strings.Contains(text, "SQLITE_LOCKED")
}
