package transport

import (
	"errors"
	"strings"
)

// Mail server settings one at a time (alpha.130; docs/MAIL-SERVER-SETTINGS-
// DESIGN.md): subaddressing and the server's sender allow and block list.
// Both are native Postfix settings CelikPanel writes in its own named form and
// removes exactly; an owner's own value is detected and never overwritten.
//
// Alt adresleme ve sunucunun gönderen izin/engel listesi: Postfix'in kendi
// ayarları; sahibin değeri korunur, kapatınca yalnız yazılan kalkar.

// Setting states (shared by both settings).
const (
	MailSettingOff     = "off"
	MailSettingOn      = "on"
	MailSettingOwner   = "owner"
	MailSettingEdited  = "edited"
	MailSettingPartial = "partial"
)

// Refusal codes (stable; the screen words them).
const (
	MailSettingNoPostfix      = "MAIL_SETTING_NO_POSTFIX"
	MailSettingOwnerConfig    = "MAIL_SETTING_OWNER_CONFIG"
	MailSettingEditedConfig   = "MAIL_SETTING_EDITED_CONFIG"
	MailSettingDovecot        = "MAIL_SETTING_DOVECOT"
	MailSettingInvalid        = "MAIL_SETTING_INVALID"
	MailSettingPostfixFailed  = "MAIL_SETTING_POSTFIX_FAILED"
	MailSettingRollbackFailed = "MAIL_SETTING_ROLLBACK_FAILED"
	MailSettingBusy           = "MAIL_SETTING_BUSY"
	MailSettingUnreadable     = "MAIL_SETTING_UNREADABLE"
)

// Sender list actions.
const (
	MailSenderAllow = "allow"
	MailSenderBlock = "block"
	// MailSenderMaxRules bounds the list (a bigger list belongs in a spam
	// filter, not in a Postfix access map kept by hand).
	MailSenderMaxRules = 2000
)

// MailSubaddressingRequest turns `user+tag@domain` delivery on or off.
type MailSubaddressingRequest struct {
	ExpectedBuildCommit string `json:"expected_build_commit"`
	Enabled             bool   `json:"enabled"`
}

// MailSenderRule is one line of the server's sender list: an address
// (user@example.com) or a whole domain (example.com, which also covers its
// subdomains), allowed or blocked.
type MailSenderRule struct {
	Value  string `json:"value"`
	Action string `json:"action"`
}

// MailSenderListRequest is the whole wanted list (empty: none).
type MailSenderListRequest struct {
	ExpectedBuildCommit string           `json:"expected_build_commit"`
	Rules               []MailSenderRule `json:"rules"`
}

// MailSettingApplyResponse answers one apply of either setting.
type MailSettingApplyResponse struct {
	Applied bool     `json:"applied"`
	Changed bool     `json:"changed"`
	State   string   `json:"state,omitempty"`
	Code    string   `json:"code,omitempty"`
	Error   string   `json:"error,omitempty"`
	Steps   []string `json:"steps,omitempty"`
}

// MailServerSettingsStatus is what the server holds now.
type MailServerSettingsStatus struct {
	Postfix bool `json:"postfix"`
	// Subaddressing: its state, Postfix's recipient_delimiter and Dovecot's.
	Subaddressing       string `json:"subaddressing,omitempty"`
	RecipientDelimiter  string `json:"recipient_delimiter,omitempty"`
	DovecotDelimiter    string `json:"dovecot_delimiter,omitempty"`
	SubaddressingDetail string `json:"subaddressing_detail,omitempty"`
	SenderList          string `json:"sender_list,omitempty"`
	SenderListDetail    string `json:"sender_list_detail,omitempty"`
	Error               string `json:"error,omitempty"`
}

// CanonicalMailSenderValue validates an address or a domain of the sender
// list and returns it in canonical (lower-case) form.
func CanonicalMailSenderValue(raw string) (string, error) {
	value := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(raw), "."))
	if value == "" {
		return "", errors.New("enter an address or a domain")
	}
	if strings.HasPrefix(value, "@") {
		value = strings.TrimPrefix(value, "@")
	}
	if strings.Contains(value, "@") {
		address, err := CanonicalMailAddress(value)
		if err != nil {
			return "", errors.New("the address is not valid")
		}
		return address, nil
	}
	if _, err := CanonicalMailAddress("postmaster@" + value); err != nil || !strings.Contains(value, ".") {
		return "", errors.New("the domain is not valid")
	}
	return value, nil
}
