package transport

// Additional FTP/SFTP logins of a site (alpha.128, gap G16,
// docs/FTP-LOGINS.md). Each login is a local account with the site account's
// uid and group, closed into a root-owned jail onto which the sandbox helper
// binds one folder of the site. The Agent keeps the native configuration
// (the sshd drop-in's login records, vsftpd, one systemd unit per login);
// the host is the record, so logins keep working without the Panel (D-022).
//
// Sitenin ek FTP/SFTP girişleri: sitenin uid'iyle, yalnız seçilen klasöre
// kapalı hesaplar. Kayıt sunucudadır.

const AgentCapabilityFTPLoginsV1 = "ftp_logins_v1"

// FTP login actions.
const (
	FTPLoginAdd    = "add"
	FTPLoginUpdate = "update" // a new password and/or a new folder
	FTPLoginRemove = "remove"
)

// Refusal codes of ApplyFTPLoginV1 (the screens word them, D-024).
const (
	FTPLoginRefusedInvalid      = "invalid"
	FTPLoginRefusedNameTaken    = "name_taken"
	FTPLoginRefusedNoLogin      = "no_such_login"
	FTPLoginRefusedNoSSHD       = "no_sshd"
	FTPLoginRefusedSSHDOwner    = "sshd_owner_config"
	FTPLoginRefusedSSHDRejected = "sshd_rejected"
	FTPLoginRefusedSSHDInclude  = "sshd_include_missing"
	FTPLoginRefusedUnitOwner    = "unit_owner_config"
	FTPLoginRefusedSiteAccount  = "site_account"
	FTPLoginRefusedFolder       = "folder"
	FTPLoginRefusedJail         = "jail"
	FTPLoginRefusedAccount      = "account_change"
	FTPLoginRefusedFTP          = "ftp_rejected"
	FTPLoginRefusedLimit        = "server_limit"
)

// Warnings of ApplyFTPLoginV1 and FTPLoginsStateV1.
const (
	// The server's sshd refuses passwords for this login: SFTP does not
	// work with a password, FTP over TLS does.
	FTPLoginWarnPasswordAuthOff = "sftp_password_off"
	// vsftpd is not installed, or its configuration is the owner's: SFTP
	// works, FTP does not.
	FTPLoginWarnFTPNotInstalled = "ftp_not_installed"
	FTPLoginWarnFTPOwnerConfig  = "ftp_owner_config"
	FTPLoginWarnFTPRejected     = "ftp_rejected"
)

// FTPLoginSite names the site whose logins are meant.
type FTPLoginSite = FileAccessSite

// ApplyFTPLoginRequest adds, changes or removes one login. Password is set
// only when non-empty; it is never logged or returned. ReadOnly follows the
// subscription (a full disk, D-044).
type ApplyFTPLoginRequest struct {
	ExpectedBuildCommit string       `json:"expected_build_commit"`
	Site                FTPLoginSite `json:"site"`
	Action              string       `json:"action"`
	Login               string       `json:"login"`
	// Folder is the folder below the site home ("/" is the home itself);
	// empty in an update keeps the folder.
	Folder   string `json:"folder,omitempty"`
	Password string `json:"password,omitempty"`
	ReadOnly bool   `json:"read_only,omitempty"`
}

// FTPLoginStep is one step the change ran, in order (OperationProgress).
type FTPLoginStep struct {
	Step   string `json:"step"`   // check, account, jail, sshd, ftp, password, verify, cleanup
	Status string `json:"status"` // done, failed, skipped, rolled_back
	Detail string `json:"detail,omitempty"`
}

// ApplyFTPLoginResponse: Applied when the server matches the request. A
// Refusal is a verified "nothing changed" (or "put back"); Detail says why
// and what to do.
type ApplyFTPLoginResponse struct {
	Applied  bool                   `json:"applied"`
	Refusal  string                 `json:"refusal,omitempty"`
	Detail   string                 `json:"detail,omitempty"`
	Steps    []FTPLoginStep         `json:"steps,omitempty"`
	Warnings []string               `json:"warnings,omitempty"`
	State    FTPLoginsStateResponse `json:"state"`
}

// FTPLoginsStateRequest reads the logins of one site (read only).
type FTPLoginsStateRequest struct {
	Site FTPLoginSite `json:"site"`
}

// FTPLoginState is one login as the server has it.
type FTPLoginState struct {
	Login    string `json:"login"`
	Folder   string `json:"folder"`
	ReadOnly bool   `json:"read_only"`
	// AccountOK: the account exists with the site's uid and group, its jail
	// home and no shell. Password: none, set, locked, disabled.
	AccountOK bool   `json:"account_ok"`
	Password  string `json:"password"`
	// Suspended: the account has an expiry in the past (the site is
	// suspended, D-025).
	Suspended bool `json:"suspended,omitempty"`
	// JailActive: the unit is active and the folder is bound onto the jail.
	JailActive bool `json:"jail_active"`
	// JailProblem is the reason the jail is not there (the helper's words);
	// "unfinished" logins (an account without a record) say so here.
	JailProblem string `json:"jail_problem,omitempty"`
	// Unfinished: the account exists but the record does not (the Agent
	// stopped half way); the only action is removal.
	Unfinished bool `json:"unfinished,omitempty"`
	// UnitFile: managed, owner, absent.
	UnitFile string `json:"unit_file"`
}

// FTPLoginsStateResponse is the site's logins and the server side.
type FTPLoginsStateResponse struct {
	Logins []FTPLoginState `json:"logins"`
	// ServerLogins counts every login on the server (MaxLoginsPerServer).
	ServerLogins int `json:"server_logins"`
	// SSHDropIn: managed, owner, absent, unreadable. An owner's file stops
	// every change of logins (nothing is overwritten).
	SSHDropIn    string   `json:"sshd_drop_in"`
	SSHDPresent  bool     `json:"sshd_present"`
	FTPInstalled bool     `json:"ftp_installed"`
	FTPConfig    string   `json:"ftp_config"`
	SSHPorts     []int    `json:"ssh_ports,omitempty"`
	Warnings     []string `json:"warnings,omitempty"`
	Error        string   `json:"error,omitempty"`
}
