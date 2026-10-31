package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/alicelik/celikpanel/internal/auth"
)

// Two-factor recovery codes (alpha.114).
//
// When two-factor sign-in is turned on, the account gets ten single-use codes
// ("XXXX-XXXX-XXXX-XXXX": 16 characters of a 32-letter
// alphabet, 80 bits each). A code stands in for the authenticator code at
// sign-in, once. Only a SHA-256 digest of each is stored (a code carries 80
// random bits, so a fast digest is enough; there is nothing to guess), the
// plain codes are shown once, when they are made, and a person who has lost
// them makes a new set with their password and a current authenticator code.
// Using one is an audit event. Turning two-factor off deletes the set.
//
// İki faktörlü kurtarma kodları: 2FA açılınca hesaba on adet tek kullanımlık
// kod verilir. Kod, girişte doğrulayıcı kodunun yerini bir kez tutar. Yalnız
// SHA-256 özeti saklanır; düz kodlar yalnız üretildiklerinde bir kez gösterilir;
// kaybolunca parola ve güncel doğrulayıcı koduyla yeni set üretilir. Kullanım
// denetim kaydına yazılır. 2FA kapanınca set silinir.

const (
	recoveryCodeCount    = 10
	recoveryCodeGroups   = 4
	recoveryCodeGroupLen = 4
	recoveryCodeLen      = recoveryCodeGroups * recoveryCodeGroupLen
	// No I and O: they read as 1 and 0.
	recoveryCodeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	recoveryCodePrefix   = "celikpanel-2fa-recovery-v1:"
)

// newRecoveryCode makes one code: 16 characters from a 32-letter alphabet, so
// each random byte is reduced by a mask, never by a modulus.
func newRecoveryCode() (string, error) {
	raw := make([]byte, recoveryCodeLen)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate a recovery code: %w", err)
	}
	var code strings.Builder
	for index, value := range raw {
		if index > 0 && index%recoveryCodeGroupLen == 0 {
			code.WriteByte('-')
		}
		code.WriteByte(recoveryCodeAlphabet[value&31])
	}
	return code.String(), nil
}

// normalizeRecoveryCode reads what a person typed: letters in either case,
// with or without the dashes and spaces. ok is false for anything that is not
// a code of the right length and alphabet.
func normalizeRecoveryCode(raw string) (string, bool) {
	var code strings.Builder
	for _, r := range strings.ToUpper(raw) {
		switch {
		case r == '-' || r == ' ':
			continue
		case strings.ContainsRune(recoveryCodeAlphabet, r):
			code.WriteRune(r)
		default:
			return "", false
		}
	}
	if code.Len() != recoveryCodeLen {
		return "", false
	}
	return code.String(), true
}

func hashRecoveryCode(normalized string) string {
	sum := sha256.Sum256([]byte(recoveryCodePrefix + normalized))
	return hex.EncodeToString(sum[:])
}

// newRecoveryCodeSet returns the plain codes (shown once) and their digests.
func newRecoveryCodeSet() (codes, hashes []string, err error) {
	for len(codes) < recoveryCodeCount {
		code, err := newRecoveryCode()
		if err != nil {
			return nil, nil, err
		}
		normalized, _ := normalizeRecoveryCode(code)
		hash := hashRecoveryCode(normalized)
		duplicate := false
		for _, existing := range hashes {
			if existing == hash {
				duplicate = true
			}
		}
		if duplicate {
			continue
		}
		codes = append(codes, code)
		hashes = append(hashes, hash)
	}
	return codes, hashes, nil
}

// replaceRecoveryCodesTx makes the digests the account's whole set: the codes
// it had, spent or not, are deleted.
func replaceRecoveryCodesTx(ctx context.Context, tx *sql.Tx, userID int, hashes []string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM two_factor_recovery_codes WHERE user_id = ?`, userID); err != nil {
		return fmt.Errorf("delete recovery codes: %w", err)
	}
	for _, hash := range hashes {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO two_factor_recovery_codes (user_id, code_hash) VALUES (?, ?)`, userID, hash); err != nil {
			return fmt.Errorf("store a recovery code: %w", err)
		}
	}
	return nil
}

// consumeRecoveryCode spends a code. It is single-use by construction: the
// UPDATE only touches an unspent row, so two simultaneous uses of one code
// cannot both succeed.
func (p *Panel) consumeRecoveryCode(ctx context.Context, userID int, raw string) (bool, error) {
	normalized, ok := normalizeRecoveryCode(raw)
	if !ok {
		return false, nil
	}
	result, err := p.db.GetDB().ExecContext(ctx, `
		UPDATE two_factor_recovery_codes
		SET used_at = datetime('now')
		WHERE user_id = ? AND code_hash = ? AND used_at IS NULL`, userID, hashRecoveryCode(normalized))
	if err != nil {
		return false, fmt.Errorf("spend a recovery code: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("confirm a spent recovery code: %w", err)
	}
	return affected == 1, nil
}

// recoveryCodeCounts is how many codes of the current set are unspent and how
// many the set had.
func (p *Panel) recoveryCodeCounts(ctx context.Context, userID int) (remaining, total int, err error) {
	err = p.db.GetDB().QueryRowContext(ctx, `
		SELECT COALESCE(SUM(CASE WHEN used_at IS NULL THEN 1 ELSE 0 END), 0), COUNT(*)
		FROM two_factor_recovery_codes WHERE user_id = ?`, userID).Scan(&remaining, &total)
	return remaining, total, err
}

// handle2FARecoveryRegenerate makes a new set of recovery codes. The password
// and a current authenticator code are required, like turning 2FA off: a
// hijacked session alone must not be able to read a way around the second
// factor. The old set stops working at once.
func (p *Panel) handle2FARecoveryRegenerate(w http.ResponseWriter, r *http.Request, userID int) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeClientError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	state, err := p.userAuthState(r.Context(), userID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if !state.totpEnabled {
		writeClientError(w, http.StatusConflict, "two-factor authentication is not enabled")
		return
	}
	if !p.allowSensitiveAuthAttempt(r, userID, "2fa-recovery") {
		writeClientError(w, http.StatusTooManyRequests, "too many attempts, try again later")
		return
	}
	ok, err := auth.VerifyPassword(req.Password, state.passwordHash)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if !ok || !auth.ValidateTOTP(state.totpSecret, req.Code) {
		writeClientError(w, http.StatusUnauthorized, "password or code incorrect")
		return
	}
	codes, hashes, err := newRecoveryCodeSet()
	if err != nil {
		writeServerError(w, err)
		return
	}
	tx, err := p.db.GetDB().BeginTx(r.Context(), nil)
	if err != nil {
		writeServerError(w, err)
		return
	}
	defer tx.Rollback()
	// The set is replaced only while 2FA is still on for the state that was
	// checked; a concurrent disable leaves the account with no codes.
	var epoch int64
	if err := tx.QueryRowContext(r.Context(),
		`SELECT auth_epoch FROM users WHERE id = ? AND totp_enabled = 1`, userID).Scan(&epoch); err != nil || epoch != state.authEpoch {
		if err == nil || errors.Is(err, sql.ErrNoRows) {
			writeClientError(w, http.StatusConflict, "two-factor state changed; reload and try again")
			return
		}
		writeServerError(w, err)
		return
	}
	if err := replaceRecoveryCodesTx(r.Context(), tx, userID, hashes); err != nil {
		writeServerError(w, err)
		return
	}
	if err := tx.Commit(); err != nil {
		writeServerError(w, fmt.Errorf("commit recovery codes: %w", err))
		return
	}
	p.audit(r, "2fa.recovery.regenerate", "user", userID)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"recovery_codes":     codes,
		"recovery_remaining": len(codes),
		"recovery_total":     len(codes),
	})
}
