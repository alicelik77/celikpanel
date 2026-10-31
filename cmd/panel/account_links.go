package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

// Account links (v0.1.0-alpha.114, P6): single-use, short-lived links for a
// password reset, an invitation and an address confirmation.
//
//   - The token is 32 random bytes (256 bits), URL-safe base64. Only its
//     SHA-256 is stored (migration 047, account_links.token_hash). A token of
//     this entropy needs no slow hash; a deterministic one is what lets the
//     panel find the row without keeping the token.
//   - A link belongs to one user and one purpose; using it anywhere else
//     reads as "not valid".
//   - A reset or invitation link is void once the user's credential epoch
//     moved (password, suspension or two-factor change) or the account's
//     address differs from the one the link was made for. A new link of the
//     same purpose revokes the older ones.
//   - Consumption is a compare-and-set in the same transaction as the change
//     it allows, so two simultaneous uses change the account once.
//
// Hesap bağlantıları: tek kullanımlık, kısa ömürlü. Jeton 256 bit rastgeledir;
// yalnız SHA-256 özeti saklanır. Bağlantı tek kullanıcıya ve tek amaca aittir.
// Parola, askı ya da iki adımlı doğrulama değişince veya adres değişince
// sıfırlama ve davet bağlantıları geçersizdir. Kullanım, izin verdiği
// değişiklikle aynı işlemde koşullu yazılır.

const (
	accountLinkPasswordReset = "password_reset"
	accountLinkInvitation    = "invitation"
	accountLinkEmailVerify   = "email_verify"

	accountLinkDeliveryEmail = "email"
	accountLinkDeliveryLink  = "link"

	accountLinkResetTTL      = 30 * time.Minute
	accountLinkInvitationTTL = 7 * 24 * time.Hour
	accountLinkVerifyTTL     = 24 * time.Hour

	// Finished, revoked and expired rows are kept this long for the
	// inviter's list and then removed by the hourly sweep.
	accountLinkRetention = 30 * 24 * time.Hour

	accountLinkTimeLayout = "2006-01-02T15:04:05Z"
)

// accountLinkPaths are the panel pages a link opens (the SPA renders them
// without a session). The token travels in the fragment, so it never reaches
// a server log or a Referer header.
var accountLinkPaths = map[string]string{
	accountLinkPasswordReset: "/account-link/reset",
	accountLinkInvitation:    "/account-link/invitation",
	accountLinkEmailVerify:   "/account-link/verify",
}

var (
	errAccountLinkInvalid = errors.New("account link is not valid")
	accountLinkNow        = func() time.Time { return time.Now().UTC() }
)

func accountLinkTime(t time.Time) string { return t.UTC().Format(accountLinkTimeLayout) }

func hashAccountLinkToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func newAccountLinkToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// wellFormedAccountLinkToken refuses anything that is not a token this panel
// could have made, before any lookup.
func wellFormedAccountLinkToken(token string) bool {
	if len(token) != 43 {
		return false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	return err == nil && len(decoded) == 32
}

func accountLinkURL(baseURL, purpose, token string) string {
	return baseURL + accountLinkPaths[purpose] + "#t=" + token
}

// issueAccountLink stores a new link and revokes the user's older open links
// of the same purpose. It returns the raw token (never stored).
func issueAccountLink(ctx context.Context, db *sql.DB, purpose string, userID int, email string,
	authEpoch int64, delivery string, createdBy int, ttl time.Duration) (string, int64, time.Time, error) {
	token, err := newAccountLinkToken()
	if err != nil {
		return "", 0, time.Time{}, err
	}
	now := accountLinkNow()
	expires := now.Add(ttl)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return "", 0, time.Time{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `
		UPDATE account_links SET revoked_at = ?
		WHERE user_id = ? AND purpose = ? AND consumed_at IS NULL AND revoked_at IS NULL`,
		accountLinkTime(now), userID, purpose); err != nil {
		return "", 0, time.Time{}, err
	}
	var creator any
	if createdBy > 0 {
		creator = createdBy
	}
	result, err := tx.ExecContext(ctx, `
		INSERT INTO account_links (purpose, user_id, token_hash, email, auth_epoch, delivery, created_by, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		purpose, userID, hashAccountLinkToken(token), email, authEpoch, delivery, creator,
		accountLinkTime(now), accountLinkTime(expires))
	if err != nil {
		return "", 0, time.Time{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return "", 0, time.Time{}, err
	}
	if err := tx.Commit(); err != nil {
		return "", 0, time.Time{}, err
	}
	return token, id, expires, nil
}

// accountLinkRow is one open link joined with its account as it is now.
type accountLinkRow struct {
	id          int64
	purpose     string
	userID      int
	email       string
	authEpoch   int64
	delivery    string
	username    string
	userEmail   string
	userEpoch   int64
	userStatus  string
	accountType string
	parentID    sql.NullInt64
}

// queryer is satisfied by *sql.DB and *sql.Tx.
type accountLinkQueryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// lookupAccountLink finds an open, unexpired link of exactly this purpose and
// checks that the account still matches it. Every failure is the same
// errAccountLinkInvalid: a link holder learns nothing more than "not valid".
func lookupAccountLink(ctx context.Context, q accountLinkQueryer, purpose, token string) (accountLinkRow, error) {
	var row accountLinkRow
	if !wellFormedAccountLinkToken(token) {
		return row, errAccountLinkInvalid
	}
	err := q.QueryRowContext(ctx, `
		SELECT l.id, l.purpose, l.user_id, l.email, l.auth_epoch, l.delivery,
		       u.username, u.email, u.auth_epoch, COALESCE(NULLIF(u.status, ''), 'active'),
		       COALESCE(NULLIF(u.account_type, ''), 'account'), u.parent_id
		FROM account_links l JOIN users u ON u.id = l.user_id
		WHERE l.token_hash = ? AND l.purpose = ?
		  AND l.consumed_at IS NULL AND l.revoked_at IS NULL AND l.expires_at > ?`,
		hashAccountLinkToken(token), purpose, accountLinkTime(accountLinkNow())).
		Scan(&row.id, &row.purpose, &row.userID, &row.email, &row.authEpoch, &row.delivery,
			&row.username, &row.userEmail, &row.userEpoch, &row.userStatus, &row.accountType, &row.parentID)
	if errors.Is(err, sql.ErrNoRows) {
		return row, errAccountLinkInvalid
	}
	if err != nil {
		return row, err
	}
	if !strings.EqualFold(strings.TrimSpace(row.email), strings.TrimSpace(row.userEmail)) {
		return row, errAccountLinkInvalid
	}
	if purpose != accountLinkEmailVerify && row.authEpoch != row.userEpoch {
		return row, errAccountLinkInvalid
	}
	if row.userStatus != "active" {
		return row, errAccountLinkInvalid
	}
	return row, nil
}

// consumeAccountLink marks the link used inside tx; false when another use
// got there first.
func consumeAccountLink(ctx context.Context, tx *sql.Tx, id int64) (bool, error) {
	result, err := tx.ExecContext(ctx, `
		UPDATE account_links SET consumed_at = ?
		WHERE id = ? AND consumed_at IS NULL AND revoked_at IS NULL`,
		accountLinkTime(accountLinkNow()), id)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	return affected == 1, err
}

// sweepAccountLinks removes rows that can no longer be used and are older
// than the retention, so the table never grows without bound.
func sweepAccountLinks(ctx context.Context, db *sql.DB) error {
	cutoff := accountLinkTime(accountLinkNow().Add(-accountLinkRetention))
	_, err := db.ExecContext(ctx, `
		DELETE FROM account_links
		WHERE expires_at < ? OR consumed_at < ? OR revoked_at < ?`, cutoff, cutoff, cutoff)
	return err
}
