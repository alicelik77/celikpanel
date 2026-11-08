package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Mailbox recovery (alpha.130; docs/MAILBOX-RECOVERY-REVIEW.md T7). A reset
// link records the fingerprint of the mailbox's password field when it is
// made; the reset replaces the field only while the fingerprint is the same,
// so a link is void after any other change (the panel, a transfer rotation or
// the owner editing /etc/dovecot/users by hand). A line that refuses login
// (suspension, or the owner's own nologin) is never changed by a link.
//
// Posta kutusu kurtarma: bağlantı parola alanının parmak izini taşır; alan
// yalnız parmak izi aynıysa değiştirilir. Oturum açmayı reddeden satıra
// dokunulmaz.

const mailPasswordFingerprintPrefix = "celikpanel-mailbox-password-v1\x00"

// mailPasswordFingerprint is a SHA-256 of the stored field (itself a salted
// hash) with a fixed prefix; it helps no offline guess and is safe to keep.
func mailPasswordFingerprint(field string) string {
	sum := sha256.Sum256([]byte(mailPasswordFingerprintPrefix + field))
	return "sha256:" + hex.EncodeToString(sum[:])
}

var errMailboxLineMissing = errors.New("mail account not found")

// dovecotMailboxPasswordState finds the mailbox's one line with the same
// matching rules as replaceDovecotPasswordHash and returns its password field
// and whether the line refuses login.
func dovecotMailboxPasswordState(content []byte, email string) (string, bool, error) {
	field, disabled, found := "", false, 0
	for _, line := range strings.Split(string(content), "\n") {
		colon := strings.IndexByte(line, ':')
		if colon < 0 {
			fields := strings.Fields(line)
			if len(fields) > 0 && strings.EqualFold(fields[0], email) {
				return "", false, errors.New("mail account has a malformed dovecot row")
			}
			continue
		}
		key := line[:colon]
		canonicalKey, canonicalErr := transport.CanonicalMailAddress(key)
		targets := canonicalErr == nil && canonicalKey == email
		if !targets && strings.EqualFold(strings.TrimSpace(key), email) {
			targets = true
		}
		if !targets {
			continue
		}
		if key != email || strings.ContainsRune(line, '\r') {
			return "", false, errors.New("mail account has a malformed dovecot row")
		}
		found++
		if found > 1 {
			return "", false, errors.New("mail account has duplicate dovecot rows")
		}
		rest := line[colon+1:]
		value, _, _ := strings.Cut(rest, ":")
		if err := validateStoredDovecotHash(value); err != nil {
			return "", false, err
		}
		field, disabled = value, dovecotUsersLineRefusesLogin(line)
	}
	if found == 0 {
		return "", false, errMailboxLineMissing
	}
	return field, disabled, nil
}

// MailPasswordFingerprintV1 reads one mailbox's fingerprint. It changes
// nothing and never returns the field.
func (a *Agent) MailPasswordFingerprintV1(req *transport.MailPasswordFingerprintRequest, resp *transport.MailPasswordFingerprintResponse) error {
	if req == nil || resp == nil {
		return errors.New("mailbox fingerprint request and response are required")
	}
	*resp = transport.MailPasswordFingerprintResponse{}
	email, _, _, err := canonicalAgentMailbox(req.Email)
	if err != nil {
		resp.Error = "invalid mailbox address"
		return nil
	}
	mailMutex.Lock()
	defer mailMutex.Unlock()
	users, err := readMailFile(dovecotUsersPath)
	if err != nil {
		resp.Error = "the mail account file could not be read"
		return nil
	}
	field, disabled, err := dovecotMailboxPasswordState(users, email)
	if errors.Is(err, errMailboxLineMissing) {
		return nil
	}
	if err != nil {
		resp.Error = "the mailbox's line in the mail account file is not one CelikPanel can read"
		return nil
	}
	resp.Found, resp.LoginDisabled, resp.Fingerprint = true, disabled, mailPasswordFingerprint(field)
	return nil
}

// SetMailPasswordIfUnchangedV1 replaces one mailbox's password field while it
// still has the expected fingerprint and does not refuse login. It is the
// same file transaction as UpdateMailPassword.
func (a *Agent) SetMailPasswordIfUnchangedV1(req *transport.SetMailPasswordIfUnchangedRequest, resp *transport.SetMailPasswordIfUnchangedResponse) error {
	if req == nil || resp == nil {
		return errors.New("mailbox password request and response are required")
	}
	*resp = transport.SetMailPasswordIfUnchangedResponse{}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "resetting a mailbox password"); err != nil {
		resp.Error = err.Error()
		return nil
	}
	email, _, _, err := canonicalAgentMailbox(req.Email)
	if err != nil {
		resp.Error = "invalid mailbox address"
		return nil
	}
	if !strings.HasPrefix(req.ExpectedFingerprint, "sha256:") || len(req.ExpectedFingerprint) != len("sha256:")+64 {
		resp.Error = "invalid password fingerprint"
		return nil
	}
	if err := transport.ValidateMailboxPassword(req.NewPassword); err != nil {
		resp.Error = "invalid mailbox password"
		return nil
	}
	if !mailMutex.TryLock() {
		resp.Error = transport.MailConfigurationBusy
		return nil
	}
	defer mailMutex.Unlock()
	if err := validateDovecotUsersFileMetadata(dovecotUsersPath, true); err != nil {
		resp.Error = "the mail account file is not as CelikPanel expects"
		return nil
	}
	ctx, cancel := newMailMutationContext()
	defer cancel()
	// The cheap check first: a stale link costs no hash.
	users, err := readMailFile(dovecotUsersPath)
	if err != nil {
		resp.Error = "the mail account file could not be read"
		return nil
	}
	field, disabled, err := dovecotMailboxPasswordState(users, email)
	if errors.Is(err, errMailboxLineMissing) {
		resp.Changed = true
		return nil
	}
	if err != nil {
		resp.Error = "the mailbox's line in the mail account file is not one CelikPanel can read"
		return nil
	}
	if disabled {
		resp.LoginDisabled = true
		return nil
	}
	if mailPasswordFingerprint(field) != req.ExpectedFingerprint {
		resp.Changed = true
		return nil
	}
	hash, err := generateMailboxPasswordHash(ctx, req.NewPassword)
	if err != nil {
		resp.Error = "the new password could not be hashed"
		return nil
	}
	// Read again under the same lock: the file is the one the check saw
	// unless something outside the Agent wrote it meanwhile.
	users, err = readMailFile(dovecotUsersPath)
	if err != nil {
		resp.Error = "the mail account file could not be read"
		return nil
	}
	field, disabled, err = dovecotMailboxPasswordState(users, email)
	if err != nil || disabled || mailPasswordFingerprint(field) != req.ExpectedFingerprint {
		resp.Changed = err == nil || errors.Is(err, errMailboxLineMissing)
		resp.LoginDisabled = err == nil && disabled
		if err != nil && !errors.Is(err, errMailboxLineMissing) {
			resp.Error = "the mailbox's line in the mail account file is not one CelikPanel can read"
		}
		return nil
	}
	updated, err := replaceDovecotPasswordHash(users, email, hash)
	if err != nil {
		resp.Error = "the mailbox's password field could not be replaced"
		return nil
	}
	if err := ctx.Err(); err != nil {
		resp.Error = fmt.Sprintf("the mailbox password reset expired before it was written: %v", err)
		return nil
	}
	if err := applyMailFileMutation(ctx, []mailFileWrite{{
		path: dovecotUsersPath, content: updated, mode: 0o640,
	}}, nil, nil); err != nil {
		resp.Error = "the mail account file could not be written"
		return nil
	}
	resp.Applied = true
	return nil
}
