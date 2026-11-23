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
	"strconv"
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
	// For comparing two copies of one database (a move or a switch): every recorded
	// action adds an event, and every deliberate licence change raises a generation,
	// so a copy that is behind shows lower numbers here.
	Events, NewestEvent, GenerationSum int64
	SchemaTables                       []string
	// DatabaseIsAuthority is the private configuration's setting (the caller fills it).
	DatabaseIsAuthority bool
	// The store (S2): its version (0: not set up) and counts; Charging and Payment are the
	// private configuration's (the caller fills them; never a value, only set / not set).
	StoreVersion                                int
	Orders, OrdersPaid, OrdersMismatch, Revoked int
	// StoreIssued: licences the store issued for an account that pays (a trial, or one awaiting
	// its first year); the PHP service's free renewal would give each a free year.
	StoreIssued              int
	Charging                 ChargingMode
	Payment, PaymentProvider string
	UnknownColumns           []string
	Problems                 []string
}

// CheckDB reads path with the configured key; now is the clock the receipts are
// checked at (the panel's Verify).
func CheckDB(path string, signingKey ed25519.PrivateKey, now int64) (DBReport, error) {
	var r DBReport
	db, err := sql.Open("sqlite", fileURI(path)+"?mode=ro&_pragma=query_only(1)&_pragma=busy_timeout(5000)")
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
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*), COALESCE(MAX(created),0) FROM events").Scan(&r.Events, &r.NewestEvent); err != nil {
		return r, err
	}
	if err := db.QueryRowContext(ctx, "SELECT COALESCE(SUM(generation),0) FROM licenses").Scan(&r.GenerationSum); err != nil {
		return r, err
	}
	var version string
	if db.QueryRowContext(ctx, "SELECT value FROM store_meta WHERE key='version'").Scan(&version) == nil {
		r.StoreVersion, _ = strconv.Atoi(version)
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*), COALESCE(SUM(state='paid'),0), COALESCE(SUM(state='mismatch'),0) FROM store_orders").Scan(&r.Orders, &r.OrdersPaid, &r.OrdersMismatch); err != nil {
			return r, err
		}
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM licenses WHERE revoked IS NOT NULL").Scan(&r.Revoked); err != nil {
			return r, err
		}
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM store_audit WHERE action IN ('trial_started','licence_awaiting_payment')").Scan(&r.StoreIssued); err != nil {
			return r, err
		}
	}
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
		if !known[name] && name != "revoked" && name != "revoked_reason" { // the store's own (MigrateStore)
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
	if r.Live == 0 {
		// An empty or stale copy answers "invalid" to every panel; never call it fine.
		problems["no live licence in this file: is it the live database?"]++
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
	fmt.Fprintf(w, "to compare two copies: events %d, newest event at %d, licence generations in total %d\n", r.Events, r.NewestEvent, r.GenerationSum)
	if r.DatabaseIsAuthority {
		fmt.Fprintln(w, "database-behind check: OFF (database_is_authority is set): every unknown or older state is rejected, as the PHP service does")
	} else {
		fmt.Fprintln(w, "database-behind check: on (the default): a panel's own token for a state this file does not know is answered 503, receipt kept")
	}
	if r.StoreVersion == 0 {
		fmt.Fprintln(w, "store: not set up (membership migrate-store creates it); charging is off")
	} else {
		fmt.Fprintf(w, "store: version %d; orders %d (paid %d, for the operator %d); licences revoked by a refund %d; licences issued to be paid for %d\n", r.StoreVersion, r.Orders, r.OrdersPaid, r.OrdersMismatch, r.Revoked, r.StoreIssued)
		if r.PHPMustNotServe() {
			fmt.Fprintln(w, "note: the PHP service must not serve this file again: it does not read the store's \"revoked\" mark, and its free renewal would give a year to a licence that must be paid for")
		}
	}
	switch r.Charging {
	case ChargingTest:
		fmt.Fprintln(w, "charging: test (the provider's test keys only; test accounts only)")
	case ChargingLive:
		fmt.Fprintln(w, "charging: live is set, but refused in this release (no order can be made)")
	default:
		fmt.Fprintln(w, "charging: off (the default)")
	}
	if r.PaymentProvider == "" {
		fmt.Fprintln(w, "payment provider: none set")
	} else {
		fmt.Fprintf(w, "payment provider: %s, keys %s\n", r.PaymentProvider, r.Payment)
	}
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

// PHPMustNotServe says whether the store has been used on this file: an order, a revocation or a
// licence issued to be paid for. The PHP service would then renew for free and activate what
// must be paid for or has ended (S2 review, 9; the move procedure's way back).
func (r DBReport) PHPMustNotServe() bool { return r.Orders > 0 || r.Revoked > 0 || r.StoreIssued > 0 }

// OK says whether nothing was found wrong.
func (r DBReport) OK() bool { return len(r.Problems) == 0 }
