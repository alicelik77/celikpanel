package transport

// SSH password sign-in (alpha.130, owner approval of 8 Nov 2026: "add the
// button; the owner presses it"). The Security page shows what sshd itself
// says (`sshd -T`) and which file decides it; the administrator may turn
// password sign-in off with one CelikPanel drop-in that sshd reads before the
// others, and turn it back on by removing only that drop-in. The owner's own
// files are never changed. Without CelikPanel the owner deletes the drop-in
// and reloads ssh.
//
// SSH parola ile giriş: sshd'nin kendi söylediği durum ve karar veren dosya;
// kapatma tek bir CelikPanel ek dosyasıdır, açma yalnız onu kaldırır.

const (
	SSHPasswordLoginStateRead        = "read"
	SSHPasswordLoginStateUnsupported = "unsupported"
	SSHPasswordLoginStateUnreadable  = "unreadable"

	SSHDropInAbsent = "absent"
	SSHDropInPanel  = "panel"
	SSHDropInEdited = "edited"

	SSHPasswordLoginActionOff = "turn_off"
	SSHPasswordLoginActionOn  = "turn_on"

	SSHPasswordLoginTurnedOff = "turned_off"
	SSHPasswordLoginTurnedOn  = "turned_on"
	SSHPasswordLoginUnchanged = "unchanged"
	SSHPasswordLoginRefused   = "refused"
	// SSHPasswordLoginRolledBack: a step failed and CelikPanel's change was
	// taken back; sshd runs with the configuration it had.
	SSHPasswordLoginRolledBack = "rolled_back"
	// SSHPasswordLoginRollbackFailed: a step failed and taking the change
	// back failed too; Detail names what the owner checks.
	SSHPasswordLoginRollbackFailed = "rollback_failed"

	SSHKeyAccountsMax = 32
)

// SSHSettingDecider says which line decides one setting (sshd uses the first
// value it reads), or that sshd's built-in default applies.
type SSHSettingDecider struct {
	Setting string `json:"setting"`
	Value   string `json:"value,omitempty"`
	File    string `json:"file,omitempty"`
	Line    int    `json:"line,omitempty"`
	Default bool   `json:"default,omitempty"`
}

// SSHKeyAccount is an account that can sign in with a key once passwords are
// off: root (when sshd lets root in with a key) or a member of an
// administrators' group (sudo, wheel, admin), with a login shell and at least
// one key sshd would accept. Only names and counts; never key material.
type SSHKeyAccount struct {
	Name string `json:"name"`
	Root bool   `json:"root,omitempty"`
	Keys int    `json:"keys"`
}

type SSHDropIn struct {
	Path   string `json:"path"`
	State  string `json:"state"`
	SHA256 string `json:"sha256,omitempty"`
}

type SSHPasswordLoginStatusResponse struct {
	State string `json:"state"`
	// Reason is a stable code when State is not read, or when turning off
	// is blocked: already_off, drop_in_edited, include_order, match_block,
	// no_key_account, pubkey_off, config_invalid, drop_in_dir_missing.
	Reason string `json:"reason,omitempty"`
	// Detail is a bounded fact for the reason: the file and line that would
	// still decide, or sshd's first line of refusal. Never a secret.
	Detail string `json:"detail,omitempty"`

	Unit       string `json:"unit,omitempty"`
	UnitActive bool   `json:"unit_active"`

	PasswordAuthentication       string `json:"password_authentication"`
	KbdInteractiveAuthentication string `json:"kbd_interactive_authentication"`
	PermitRootLogin              string `json:"permit_root_login"`
	PubkeyAuthentication         string `json:"pubkey_authentication"`

	Deciders    []SSHSettingDecider `json:"deciders"`
	DropIn      SSHDropIn           `json:"drop_in"`
	KeyAccounts []SSHKeyAccount     `json:"key_accounts"`

	CanTurnOff bool `json:"can_turn_off"`
	CanTurnOn  bool `json:"can_turn_on"`
}

type SetSSHPasswordLoginRequest struct {
	ServiceMutationBinding
	Action string `json:"action"`
	// ExpectedDropInSHA256 is the drop-in digest the page showed (empty when
	// absent); a drop-in changed since is never touched.
	ExpectedDropInSHA256 string `json:"expected_drop_in_sha256"`
}

type SetSSHPasswordLoginResponse struct {
	Outcome string `json:"outcome"`
	Code    string `json:"code,omitempty"`
	Error   string `json:"error,omitempty"`
	Detail  string `json:"detail,omitempty"`

	Unit string `json:"unit,omitempty"`
	// Reloaded: sshd was told to read its configuration again (a reload,
	// never a restart). AppliesAtNextStart: sshd was not running (socket
	// activation), so the next connection starts it with the new setting.
	Reloaded           bool `json:"reloaded"`
	AppliesAtNextStart bool `json:"applies_at_next_start,omitempty"`

	PasswordAuthentication       string `json:"password_authentication,omitempty"`
	KbdInteractiveAuthentication string `json:"kbd_interactive_authentication,omitempty"`
	// StillOffBy names the owner's file that keeps passwords off after
	// CelikPanel's drop-in was removed.
	StillOffBy string `json:"still_off_by,omitempty"`
}
