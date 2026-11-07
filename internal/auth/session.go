package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// SessionDuration is how long a session stays valid after login.
// SessionDuration, bir oturumun girişten sonra ne kadar geçerli kalacağıdır.
const SessionDuration = 24 * time.Hour

// ErrSessionInvalid covers a missing, expired or malformed session.
// ErrSessionInvalid; eksik, süresi dolmuş ya da bozuk bir oturumu kapsar.
var ErrSessionInvalid = errors.New("session invalid or expired")

// ErrAuthStateChanged means an authentication flow was completed against an
// obsolete password, suspension or two-factor state.
var ErrAuthStateChanged = errors.New("authentication state changed")

// SessionStore persists sessions in SQLite. Only the SHA-256 of each token
// is stored, so the raw token exists solely in the user's cookie.
// SessionStore, oturumları SQLite'da saklar. Her jetonun yalnızca SHA-256
// özeti saklanır; ham jeton yalnızca kullanıcının çerezinde bulunur.
type SessionStore struct {
	db *sql.DB
}

func NewSessionStore(db *sql.DB) *SessionStore {
	return &SessionStore{db: db}
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// Create issues a new session for userID and returns the raw token that
// belongs in the cookie.
// Create, userID için yeni bir oturum açar ve çereze konulacak ham jetonu
// döndürür.
func (s *SessionStore) Create(ctx context.Context, userID int) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("failed to generate session token: %w", err)
	}
	token := hex.EncodeToString(raw)
	expiresAt := time.Now().Add(SessionDuration).UTC().Format(time.RFC3339)

	_, err := s.db.ExecContext(ctx,
		"INSERT INTO sessions (token_hash, user_id, expires_at) VALUES (?, ?, ?)",
		hashToken(token), userID, expiresAt,
	)
	if err != nil {
		return "", fmt.Errorf("failed to store session: %w", err)
	}
	return token, nil
}

// CreateForAuthEpoch creates a session only while the user's credential epoch
// and account state still match the authentication flow. The conditional
// INSERT closes the race between the final state check and session creation.
func (s *SessionStore) CreateForAuthEpoch(ctx context.Context, userID int, authEpoch int64, requireTOTP bool) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("failed to generate session token: %w", err)
	}
	token := hex.EncodeToString(raw)
	expiresAt := time.Now().Add(SessionDuration).UTC().Format(time.RFC3339)
	totpRequired := 0
	if requireTOTP {
		totpRequired = 1
	}

	result, err := s.db.ExecContext(ctx, `
		INSERT INTO sessions (token_hash, user_id, expires_at)
		SELECT ?, id, ?
		FROM users
		WHERE id = ?
		  AND auth_epoch = ?
		  AND COALESCE(status, 'active') != 'suspended'
		  AND (? = 0 OR totp_enabled = 1)`,
		hashToken(token), expiresAt, userID, authEpoch, totpRequired)
	if err != nil {
		return "", fmt.Errorf("failed to store session: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return "", fmt.Errorf("failed to confirm session creation: %w", err)
	}
	if affected != 1 {
		return "", ErrAuthStateChanged
	}
	return token, nil
}

// CreateImpersonation issues a session for targetID that records which
// operator opened it ("Log in as"). The authentication middleware refuses
// it once the operator is gone, inactive or may no longer manage the target.
// CreateImpersonation, hedef için hangi operatörün açtığını kaydeden bir
// oturum açar ("Farklı giriş yap").
func (s *SessionStore) CreateImpersonation(ctx context.Context, targetID, operatorID int) (string, error) {
	if operatorID <= 0 || targetID <= 0 || operatorID == targetID {
		return "", fmt.Errorf("invalid impersonation identities")
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("failed to generate session token: %w", err)
	}
	token := hex.EncodeToString(raw)
	expiresAt := time.Now().Add(SessionDuration).UTC().Format(time.RFC3339)
	if _, err := s.db.ExecContext(ctx,
		"INSERT INTO sessions (token_hash, user_id, expires_at, impersonator_id) VALUES (?, ?, ?, ?)",
		hashToken(token), targetID, expiresAt, operatorID,
	); err != nil {
		return "", fmt.Errorf("failed to store session: %w", err)
	}
	return token, nil
}

// SessionIdentity is who a session belongs to and, for a "Log in as"
// session, the operator who opened it (0 otherwise).
// SessionIdentity, oturumun sahibi ve "Farklı giriş yap" oturumunda onu
// açan operatördür (değilse 0).
type SessionIdentity struct {
	UserID         int
	ImpersonatorID int
	// LastSeen is when the session was last used (alpha.128, migration
	// 069); zero for a session not used since this release (never idle).
	LastSeen time.Time
}

// ValidateIdentity is Validate plus the session's impersonator.
// ValidateIdentity, Validate artı oturumun taklitçisidir.
func (s *SessionStore) ValidateIdentity(ctx context.Context, token string) (SessionIdentity, error) {
	if token == "" {
		return SessionIdentity{}, ErrSessionInvalid
	}
	var identity SessionIdentity
	var impersonator sql.NullInt64
	var expiresAt string
	var lastSeen sql.NullString
	err := s.db.QueryRowContext(ctx,
		"SELECT user_id, impersonator_id, expires_at, last_seen_at FROM sessions WHERE token_hash = ?",
		hashToken(token),
	).Scan(&identity.UserID, &impersonator, &expiresAt, &lastSeen)
	if errors.Is(err, sql.ErrNoRows) {
		return SessionIdentity{}, ErrSessionInvalid
	}
	if err != nil {
		return SessionIdentity{}, fmt.Errorf("failed to load session: %w", err)
	}
	exp, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil || time.Now().After(exp) {
		_ = s.Delete(ctx, token)
		return SessionIdentity{}, ErrSessionInvalid
	}
	if impersonator.Valid {
		identity.ImpersonatorID = int(impersonator.Int64)
	}
	if lastSeen.Valid {
		if at, parseErr := time.Parse(time.RFC3339, lastSeen.String); parseErr == nil {
			identity.LastSeen = at
		}
	}
	return identity, nil
}

// SessionHandle is the public name of a session: the first 16 hex digits
// of its token's SHA-256. It identifies the row for "End this session" and
// cannot be turned back into the token (alpha.128).
// SessionHandle, oturumun herkese açık adıdır: jeton özetinin ilk 16 hanesi.
func SessionHandle(token string) string {
	return hashToken(token)[:16]
}

// Touch records that the session was used at `at` and, when given, from
// which address and browser (alpha.128). A missing row is not an error.
// Touch, oturumun kullanıldığını (ve verilirse adres ile tarayıcıyı) yazar.
func (s *SessionStore) Touch(ctx context.Context, token string, at time.Time, clientIP, userAgent string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE sessions SET last_seen_at = ?,
		       client_ip = COALESCE(NULLIF(?, ''), client_ip),
		       user_agent = COALESCE(NULLIF(?, ''), user_agent)
		WHERE token_hash = ?`,
		at.UTC().Format(time.RFC3339), boundedText(clientIP, 64), boundedText(userAgent, 200), hashToken(token))
	return err
}

// boundedText keeps printable characters only and cuts to max bytes on a
// rune boundary.
func boundedText(value string, max int) string {
	out := make([]rune, 0, len(value))
	size := 0
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			continue
		}
		n := len(string(r))
		if size+n > max {
			break
		}
		out = append(out, r)
		size += n
	}
	return string(out)
}

// Validate resolves a raw token to its user ID, rejecting expired ones.
// Expired rows are cleaned up opportunistically.
// Validate, ham bir jetonu kullanıcı kimliğine çözer ve süresi dolmuş
// olanları reddeder. Süresi dolmuş satırlar fırsat buldukça temizlenir.
func (s *SessionStore) Validate(ctx context.Context, token string) (int, error) {
	if token == "" {
		return 0, ErrSessionInvalid
	}

	var userID int
	var expiresAt string
	err := s.db.QueryRowContext(ctx,
		"SELECT user_id, expires_at FROM sessions WHERE token_hash = ?",
		hashToken(token),
	).Scan(&userID, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrSessionInvalid
	}
	if err != nil {
		return 0, fmt.Errorf("failed to load session: %w", err)
	}

	exp, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil || time.Now().After(exp) {
		_ = s.Delete(ctx, token)
		return 0, ErrSessionInvalid
	}
	return userID, nil
}

// Delete removes a single session (logout).
// Delete, tek bir oturumu kaldırır (çıkış).
func (s *SessionStore) Delete(ctx context.Context, token string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM sessions WHERE token_hash = ?", hashToken(token))
	return err
}

// DeleteExpired purges all sessions past their expiry. Meant to run
// periodically.
// DeleteExpired, süresi geçmiş tüm oturumları siler. Periyodik çalışması
// amaçlanmıştır.
func (s *SessionStore) DeleteExpired(ctx context.Context) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx, "DELETE FROM sessions WHERE expires_at < ?", now)
	return err
}
