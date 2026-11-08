package main

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/mutationpayload"
)

// Mailbox recovery address and reset link (alpha.130, ROADMAP alpha.130 Mail
// item 4). The security review came first and sets every rule here:
// docs/MAILBOX-RECOVERY-REVIEW.md.
//
//   - A panel user who may change the mailbox's password may set or remove
//     its recovery address. The address works only after the link mailed to
//     it was opened (24 h, once). It is bound to the domain's subscription and
//     that subscription's owner of the moment; when the address's own domain
//     is hosted here, to that domain's row at confirmation.
//   - "Forgot the mailbox password" always answers the same. A reset link
//     goes only to a confirmed, still-bound address, lives 30 minutes, works
//     once and carries the Agent's fingerprint of the password field; the
//     Agent sets the new password only while that fingerprint is unchanged.
//   - After a reset the mailbox's IMAP/POP3 sessions are ended, the mailbox
//     is told, a security pause ends and a manual pause stays.
//
// Posta kutusu kurtarma adresi ve sıfırlama bağlantısı. Kurallar güvenlik
// incelemesindedir: adres onaylanmadan kullanılmaz, abonelik ve sahibine
// bağlıdır; sıfırlama bağlantısı 30 dakika, tek kullanımlık ve parola alanının
// parmak izine bağlıdır.

const (
	mailboxRecoveryPurposeConfirm = "confirm"
	mailboxRecoveryPurposeReset   = "reset"

	mailboxRecoveryConfirmTTL = 24 * time.Hour
	mailboxRecoveryResetTTL   = 30 * time.Minute
	mailboxRecoveryRetention  = 30 * 24 * time.Hour

	mailboxRecoveryConfirmPage = "/account-link/mailbox-confirm"
	mailboxRecoveryResetPage   = "/account-link/mailbox-reset"
	mailboxRecoveryRequestPage = "/account-link/mailbox-recovery"
)

// The state a manager sees. "void" carries why it no longer works.
const (
	mailboxRecoveryStateNone      = "none"
	mailboxRecoveryStatePending   = "pending"
	mailboxRecoveryStateConfirmed = "confirmed"
	mailboxRecoveryStateVoid      = "void"

	mailboxRecoveryVoidTransferred = "transferred"
	mailboxRecoveryVoidLocalDomain = "local_domain_changed"
)

var errMailboxRecoveryLinkInvalid = errors.New("mailbox recovery link is not valid")

// mailboxRecoveryRow is a mailbox with its recovery address as it is now.
type mailboxRecoveryRow struct {
	accountID      int
	mailbox        string
	domainID       int
	subscriptionID int
	ownerID        int
	address        string
	lang           string
	setSubID       int
	setOwnerID     int
	localDomainID  sql.NullInt64
	setAt          string
	confirmedAt    sql.NullString
	present        bool
}

type mailboxRecoveryQueryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

const mailboxRecoverySelect = `
	SELECT a.id, lower(a.address), d.id, d.subscription_id, COALESCE(s.owner_id, 0),
	       r.address, COALESCE(r.lang, 'en'), r.subscription_id, r.owner_user_id,
	       r.local_domain_id, r.set_at, r.confirmed_at
	FROM email_accounts a
	JOIN domains d ON d.id = a.domain_id
	LEFT JOIN subscriptions s ON s.id = d.subscription_id
	LEFT JOIN mailbox_recovery_addresses r ON r.email_account_id = a.id`

func scanMailboxRecoveryRow(row *sql.Row) (mailboxRecoveryRow, error) {
	var out mailboxRecoveryRow
	var address, lang, setAt sql.NullString
	var setSub, setOwner sql.NullInt64
	if err := row.Scan(&out.accountID, &out.mailbox, &out.domainID, &out.subscriptionID, &out.ownerID,
		&address, &lang, &setSub, &setOwner, &out.localDomainID, &setAt, &out.confirmedAt); err != nil {
		return out, err
	}
	out.present = address.Valid
	out.address, out.lang, out.setAt = address.String, lang.String, setAt.String
	out.setSubID, out.setOwnerID = int(setSub.Int64), int(setOwner.Int64)
	return out, nil
}

// loadMailboxRecovery reads one mailbox of one domain (the manager's view).
func loadMailboxRecovery(ctx context.Context, q mailboxRecoveryQueryer, domainID, accountID int) (mailboxRecoveryRow, error) {
	return scanMailboxRecoveryRow(q.QueryRowContext(ctx, mailboxRecoverySelect+`
	WHERE a.id = ? AND a.domain_id = ?`, accountID, domainID))
}

// loadMailboxRecoveryByAddress reads the mailbox a public request names.
func loadMailboxRecoveryByAddress(ctx context.Context, q mailboxRecoveryQueryer, mailbox string) (mailboxRecoveryRow, error) {
	return scanMailboxRecoveryRow(q.QueryRowContext(ctx, mailboxRecoverySelect+`
	WHERE lower(a.address) = lower(?)`, mailbox))
}

func loadMailboxRecoveryByID(ctx context.Context, q mailboxRecoveryQueryer, accountID int) (mailboxRecoveryRow, error) {
	return scanMailboxRecoveryRow(q.QueryRowContext(ctx, mailboxRecoverySelect+`
	WHERE a.id = ?`, accountID))
}

// mailboxRecoveryDomainOf is the hosted domain row of an address's domain,
// or 0 when that domain is not hosted on this panel.
func mailboxRecoveryDomainOf(ctx context.Context, q mailboxRecoveryQueryer, address string) (int, error) {
	_, domain, ok := strings.Cut(address, "@")
	if !ok {
		return 0, nil
	}
	var id int
	err := q.QueryRowContext(ctx, `SELECT id FROM domains WHERE lower(name) = lower(?)`, domain).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return id, err
}

// state is what the address can do now: none, pending (not confirmed yet),
// confirmed, or void with its reason (a transfer, or the hosted domain of the
// address was deleted and added again).
func (row mailboxRecoveryRow) state(ctx context.Context, q mailboxRecoveryQueryer) (string, string, error) {
	if !row.present {
		return mailboxRecoveryStateNone, "", nil
	}
	if row.setSubID != row.subscriptionID || row.setOwnerID != row.ownerID {
		return mailboxRecoveryStateVoid, mailboxRecoveryVoidTransferred, nil
	}
	if !row.confirmedAt.Valid {
		return mailboxRecoveryStatePending, "", nil
	}
	if row.localDomainID.Valid {
		current, err := mailboxRecoveryDomainOf(ctx, q, row.address)
		if err != nil {
			return "", "", err
		}
		if int64(current) != row.localDomainID.Int64 {
			return mailboxRecoveryStateVoid, mailboxRecoveryVoidLocalDomain, nil
		}
	}
	return mailboxRecoveryStateConfirmed, "", nil
}

// canonicalRecoveryAddress validates the address a manager typed.
func canonicalRecoveryAddress(raw string) (string, bool) {
	address, err := mutationpayload.CanonicalContactEmail(strings.TrimSpace(raw))
	if err != nil || len(address) > 254 {
		return "", false
	}
	return strings.ToLower(address), true
}

// maskedRecoveryAddress shows enough of the address for its owner to know
// it: the first character of the local part and the domain.
func maskedRecoveryAddress(address string) string {
	local, domain, ok := strings.Cut(address, "@")
	if !ok || local == "" {
		return "***"
	}
	first := []rune(local)[0]
	return string(first) + "***@" + domain
}

// issueMailboxRecoveryLink stores a new link and revokes the mailbox's open
// links of the same purpose. It returns the raw token (never stored).
func issueMailboxRecoveryLink(ctx context.Context, db *sql.DB, purpose string, accountID int,
	address, fingerprint string, ttl time.Duration) (string, error) {
	token, err := newAccountLinkToken()
	if err != nil {
		return "", err
	}
	now := accountLinkNow()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `
		UPDATE mailbox_recovery_links SET revoked_at = ?
		WHERE email_account_id = ? AND purpose = ? AND consumed_at IS NULL AND revoked_at IS NULL`,
		accountLinkTime(now), accountID, purpose); err != nil {
		return "", err
	}
	var stored any
	if fingerprint != "" {
		stored = fingerprint
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO mailbox_recovery_links (purpose, email_account_id, address, token_hash, password_fingerprint, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		purpose, accountID, address, hashAccountLinkToken(token), stored,
		accountLinkTime(now), accountLinkTime(now.Add(ttl))); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return token, nil
}

// revokeMailboxRecoveryLinks revokes every open link of a mailbox.
func revokeMailboxRecoveryLinks(ctx context.Context, q interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}, accountID int) error {
	_, err := q.ExecContext(ctx, `
		UPDATE mailbox_recovery_links SET revoked_at = ?
		WHERE email_account_id = ? AND consumed_at IS NULL AND revoked_at IS NULL`,
		accountLinkTime(accountLinkNow()), accountID)
	return err
}

// mailboxRecoveryLinkRow is one open link with its mailbox as it is now.
type mailboxRecoveryLinkRow struct {
	id          int64
	accountID   int
	address     string
	fingerprint string
	mailbox     mailboxRecoveryRow
}

// lookupMailboxRecoveryLink finds an open, unexpired link of this purpose
// whose address is still the mailbox's recovery address. Every failure is the
// same errMailboxRecoveryLinkInvalid.
func lookupMailboxRecoveryLink(ctx context.Context, q mailboxRecoveryQueryer, purpose, token string) (mailboxRecoveryLinkRow, error) {
	var link mailboxRecoveryLinkRow
	if !wellFormedAccountLinkToken(token) {
		return link, errMailboxRecoveryLinkInvalid
	}
	var fingerprint sql.NullString
	err := q.QueryRowContext(ctx, `
		SELECT id, email_account_id, address, password_fingerprint
		FROM mailbox_recovery_links
		WHERE token_hash = ? AND purpose = ? AND consumed_at IS NULL AND revoked_at IS NULL AND expires_at > ?`,
		hashAccountLinkToken(token), purpose, accountLinkTime(accountLinkNow())).
		Scan(&link.id, &link.accountID, &link.address, &fingerprint)
	if errors.Is(err, sql.ErrNoRows) {
		return link, errMailboxRecoveryLinkInvalid
	}
	if err != nil {
		return link, err
	}
	link.fingerprint = fingerprint.String
	link.mailbox, err = loadMailboxRecoveryByID(ctx, q, link.accountID)
	if errors.Is(err, sql.ErrNoRows) {
		return link, errMailboxRecoveryLinkInvalid
	}
	if err != nil {
		return link, err
	}
	if !link.mailbox.present || link.mailbox.address != link.address {
		return link, errMailboxRecoveryLinkInvalid
	}
	state, _, err := link.mailbox.state(ctx, q)
	if err != nil {
		return link, err
	}
	switch purpose {
	case mailboxRecoveryPurposeConfirm:
		if state != mailboxRecoveryStatePending {
			return link, errMailboxRecoveryLinkInvalid
		}
	case mailboxRecoveryPurposeReset:
		if state != mailboxRecoveryStateConfirmed || !strings.HasPrefix(link.fingerprint, "sha256:") {
			return link, errMailboxRecoveryLinkInvalid
		}
	default:
		return link, errMailboxRecoveryLinkInvalid
	}
	return link, nil
}

// sweepMailboxRecoveryLinks removes rows that can no longer be used and are
// older than the retention.
func sweepMailboxRecoveryLinks(ctx context.Context, db *sql.DB) error {
	cutoff := accountLinkTime(accountLinkNow().Add(-mailboxRecoveryRetention))
	_, err := db.ExecContext(ctx, `
		DELETE FROM mailbox_recovery_links
		WHERE expires_at < ? OR consumed_at < ? OR revoked_at < ?`, cutoff, cutoff, cutoff)
	return err
}
