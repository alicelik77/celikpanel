package transport

// Mailbox recovery (alpha.130; docs/MAILBOX-RECOVERY-REVIEW.md). A reset link
// is bound to the mailbox's password as it was when the link was made: the
// Agent reads a fingerprint of the stored password field (never the field)
// and later replaces the field only while that fingerprint is unchanged.
//
// Posta kutusu kurtarma: bağlantı, yapıldığı andaki parola alanının parmak
// izine bağlıdır; Agent alanı yalnız parmak izi değişmemişse değiştirir.

// MailPasswordFingerprintRequest asks for one mailbox's fingerprint (read).
type MailPasswordFingerprintRequest struct {
	Email string `json:"email"`
}

// MailPasswordFingerprintResponse: Found is false when the passwd-file has
// no line for the mailbox. LoginDisabled is true when the line carries a
// nologin field (suspension or the owner's own). Fingerprint is a SHA-256
// over the stored password field with a fixed prefix; it is empty unless
// Found.
type MailPasswordFingerprintResponse struct {
	Found         bool   `json:"found"`
	LoginDisabled bool   `json:"login_disabled"`
	Fingerprint   string `json:"fingerprint,omitempty"`
	Error         string `json:"error,omitempty"`
}

// SetMailPasswordIfUnchangedRequest carries a password only for the lifetime
// of one privileged mutation. Callers must never log or persist NewPassword.
type SetMailPasswordIfUnchangedRequest struct {
	ExpectedBuildCommit string `json:"expected_build_commit"`
	Email               string `json:"email"`
	NewPassword         string `json:"new_password"`
	ExpectedFingerprint string `json:"expected_fingerprint"`
}

// SetMailPasswordIfUnchangedResponse: Applied means the new hash is on disk.
// Changed means nothing was written because the password field is no longer
// the one the fingerprint names (or the line is gone). LoginDisabled means
// nothing was written because the line refuses login. Error is a fixed text
// without mailbox data.
type SetMailPasswordIfUnchangedResponse struct {
	Applied       bool   `json:"applied"`
	Changed       bool   `json:"changed"`
	LoginDisabled bool   `json:"login_disabled"`
	Error         string `json:"error,omitempty"`
}
