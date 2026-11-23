package membership

// check-db: a read-only look at a live members.sqlite that prints counts only, so the
// owner can show on his own host, before any switch, that the Go service can serve
// every licence there. Nothing is written: the file is opened read-only with
// query_only on, no schema is created and no migration runs. No address, key, hash,
// id or e-mail ever leaves this function; only numbers do.

import (
	"context"
	"crypto/ed25519"
	"database/sql"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/licensing"
)

// DBReport is what check-db prints.
type DBReport struct {
	Members, Verified          int
	HashArgon2id, HashOther    int
	Licenses, Removed          int
	Live, Bound, NoExpiry      int
	StoredKeys, StoredKeysOK   int
	Receipts, ReceiptsVerified int
	Expired                    int
	SchemaTables               []string
	UnknownColumns             []string
	Problems                   []string
}

// CheckDB reads path with the configured key; now is the clock the receipts are
// checked at (the panel's Verify).
func CheckDB(path string, signingKey ed25519.PrivateKey, now int64) (DBReport, error) {
	var r DBReport
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro&_pragma=query_only(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		return r, err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	ctx := context.Background()
	s := &Service{db: db, signingKey: signingKey, clock: func() int64 { return now }}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*), COALESCE(SUM(verified=1),0), COALESCE(SUM(password LIKE '$argon2id$%'),0) FROM members").
		Scan(&r.Members, &r.Verified, &r.HashArgon2id); err != nil {
		return r, err
	}
	r.HashOther = r.Members - r.HashArgon2id
	tables, err := db.QueryContext(ctx, "SELECT name FROM sqlite_master WHERE type='table' ORDER BY name")
	if err != nil {
		return r, err
	}
	for tables.Next() {
		var name string
		if err := tables.Scan(&name); err != nil {
			tables.Close()
			return r, err
		}
		r.SchemaTables = append(r.SchemaTables, name)
	}
	tables.Close()
	known := map[string]bool{"id": true, "member_id": true, "key_hash": true, "key_suffix": true, "created": true, "activated": true,
		"expires": true, "server_id": true, "hostname": true, "generation": true, "key_encrypted": true, "server_ip": true, "removed": true}
	columns, err := tableColumns(ctx, db, "licenses")
	if err != nil {
		return r, err
	}
	for name := range columns {
		if !known[name] {
			r.UnknownColumns = append(r.UnknownColumns, name)
		}
	}
	for name := range known {
		if !columns[name] {
			return r, fmt.Errorf("check-db: the licenses table has no %s column: the PHP service has not migrated this file yet", name)
		}
	}
	sort.Strings(r.UnknownColumns)
	rows, err := db.QueryContext(ctx, "SELECT "+rowColumns+" FROM licenses")
	if err != nil {
		return r, err
	}
	var all []row
	for rows.Next() {
		var l row
		if err := rows.Scan(&l.ID, &l.Member, &l.KeyHash, &l.KeySuffix, &l.Created, &l.Activated, &l.Expires, &l.ServerID, &l.Hostname,
			&l.Generation, &l.KeyEncrypted, &l.ServerIP, &l.Removed); err != nil {
			rows.Close()
			return r, err
		}
		all = append(all, l)
	}
	rows.Close()
	public, err := PublicHalf(signingKey)
	if err != nil {
		return r, err
	}
	problems := map[string]int{}
	for _, l := range all {
		r.Licenses++
		if l.Removed.Valid {
			r.Removed++
			continue
		}
		r.Live++
		if !l.Expires.Valid {
			r.NoExpiry++
		} else if l.Expires.Int64 <= now {
			r.Expired++
		}
		if l.KeyEncrypted.Valid && phpTruthyString(l.KeyEncrypted.String) {
			r.StoredKeys++
			if _, err := s.decryptKey(l.ID, l.KeyEncrypted.String, l.KeyHash); err == nil {
				r.StoredKeysOK++
			} else {
				problems["a stored key does not decrypt to its hash"]++
			}
		}
		if !l.ServerID.Valid || !l.Activated.Valid || !l.Expires.Valid || l.Expires.Int64 <= now {
			continue
		}
		r.Bound++
		r.Receipts++
		envelope := s.entitlement(l.ID, l.ServerID.String, l.Activated.Int64, l.Expires.Int64, l.Generation)
		claims, err := licensing.Verify(licensing.Envelope{Payload: envelope.Payload, Signature: envelope.Signature,
			ActivationToken: envelope.ActivationToken}, public, l.ServerID.String, unixTime(now))
		if err != nil || claims.LicenseID != l.ID || claims.ExpiresAt != l.Expires.Int64 {
			problems["a bound licence's receipt does not verify"]++
			continue
		}
		r.ReceiptsVerified++
	}
	for reason, n := range problems {
		r.Problems = append(r.Problems, fmt.Sprintf("%d × %s", n, reason))
	}
	sort.Strings(r.Problems)
	return r, nil
}

// Print writes the report as plain lines of counts.
func (r DBReport) Print(w io.Writer) {
	fmt.Fprintf(w, "members: %d (verified %d; password hashes argon2id %d, other %d)\n", r.Members, r.Verified, r.HashArgon2id, r.HashOther)
	fmt.Fprintf(w, "licences: %d (removed %d, live %d, live without an end date %d, live and ended %d)\n", r.Licenses, r.Removed, r.Live, r.NoExpiry, r.Expired)
	fmt.Fprintf(w, "stored keys: %d, of which decrypt to their hash: %d\n", r.StoredKeys, r.StoredKeysOK)
	fmt.Fprintf(w, "bound and running licences: %d, receipts made and verified with the configured key: %d of %d\n", r.Bound, r.ReceiptsVerified, r.Receipts)
	fmt.Fprintf(w, "tables: %s\n", strings.Join(r.SchemaTables, ", "))
	if len(r.UnknownColumns) > 0 {
		fmt.Fprintf(w, "licence columns this version does not know (left untouched): %s\n", strings.Join(r.UnknownColumns, ", "))
	}
	if len(r.Problems) == 0 {
		fmt.Fprintln(w, "result: every live licence can be served")
		return
	}
	for _, p := range r.Problems {
		fmt.Fprintln(w, "problem: "+p)
	}
	fmt.Fprintln(w, "result: NOT every live licence can be served")
}

func unixTime(n int64) time.Time { return time.Unix(n, 0) }

// OK says whether nothing was found wrong.
func (r DBReport) OK() bool { return len(r.Problems) == 0 }
