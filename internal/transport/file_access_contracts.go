package transport

// File access per hosting account (alpha.117): FTP over TLS, SFTP and, with
// the ssh right, a jailed command line. The account is the site's own system
// user, so files uploaded keep the owner PHP-FPM and Node run as. The Agent
// keeps the native configuration (sshd drop-in, vsftpd, PAM, Fail2ban); the
// host is the record of who has access, so it keeps working without the
// Panel (D-022).
//
// Site başına dosya erişimi: FTP/TLS, SFTP ve ssh hakkıyla hapsedilmiş komut
// satırı. Hesap sitenin kendi sistem kullanıcısıdır.

const AgentCapabilityFileAccessV1 = "file_access_v1"

// FileAccessSite names one hosting account by its immutable identity.
type FileAccessSite struct {
	SubscriptionID int    `json:"subscription_id"`
	DomainID       int    `json:"domain_id"`
	Username       string `json:"username"`
}

// FileAccessStateRequest reads one account (read only).
type FileAccessStateRequest struct {
	Site FileAccessSite `json:"site"`
}

// ApplyFileAccessRequest brings one account to the requested setting.
// Password is set only when non-empty; it is never logged or returned.
// Reconcile names the subscription's other accounts: they get the no-login
// shell and the site group unless they have the ssh right themselves.
type ApplyFileAccessRequest struct {
	ExpectedBuildCommit string           `json:"expected_build_commit"`
	Site                FileAccessSite   `json:"site"`
	Enabled             bool             `json:"enabled"`
	SSH                 bool             `json:"ssh"`
	Password            string           `json:"password,omitempty"`
	AddKeys             []string         `json:"add_keys,omitempty"`
	RemoveKeys          []string         `json:"remove_keys,omitempty"`
	Reconcile           []FileAccessSite `json:"reconcile,omitempty"`
	// RevokeCredentials (with Enabled false): the password is replaced by
	// "no password" ("*", which a later unlock cannot bring back) and every
	// SSH key is removed. Used after a subscription transfer.
	RevokeCredentials bool `json:"revoke_credentials,omitempty"`
}

// FileAccessPreviewRequest asks what an apply would change (read only).
type FileAccessPreviewRequest struct {
	Change ApplyFileAccessRequest `json:"change"`
}

// FileAccessKey is one public key (never secret).
type FileAccessKey struct {
	Type        string `json:"type"`
	Fingerprint string `json:"fingerprint"`
	Comment     string `json:"comment"`
}

// Configuration file states.
const (
	FileAccessFileAbsent     = "absent"
	FileAccessFileManaged    = "managed"
	FileAccessFileOwner      = "owner"
	FileAccessFileUnreadable = "unreadable"
)

// FTP certificate sources.
const (
	FileAccessCertMailHost   = "mail_host"
	FileAccessCertSelfSigned = "self_signed"
	FileAccessCertNone       = "none"
)

// FileAccessServer is the server side of file access.
type FileAccessServer struct {
	SSHDPresent bool `json:"sshd_present"`
	// SSHDropIn: absent, managed, owner or unreadable.
	SSHDropIn string `json:"ssh_drop_in"`
	SSHPorts  []int  `json:"ssh_ports,omitempty"`
	// PasswordAuthentication is sshd's effective setting for a hosting
	// account: "yes", "no" or "unknown". With "no" only keys work.
	PasswordAuthentication string `json:"password_authentication"`
	FTPInstalled           bool   `json:"ftp_installed"`
	// FTPConfig: absent, managed, owner or unreadable.
	FTPConfig            string `json:"ftp_config"`
	FTPActive            bool   `json:"ftp_active"`
	FTPCertificate       string `json:"ftp_certificate"`
	FTPCertificateName   string `json:"ftp_certificate_name,omitempty"`
	FTPCertificateSHA256 string `json:"ftp_certificate_sha256,omitempty"`
	FTPPassiveMin        int    `json:"ftp_passive_min"`
	FTPPassiveMax        int    `json:"ftp_passive_max"`
	Fail2banInstalled    bool   `json:"fail2ban_installed"`
	Fail2banSSHD         bool   `json:"fail2ban_sshd"`
	Fail2banFTP          bool   `json:"fail2ban_ftp"`
	// SiteUsersWithShell counts hosting accounts (home under the hosting
	// root) whose login shell gives a command line without the ssh right;
	// SiteUsersOutsideGroup counts those not yet in the site group.
	SiteUsersTotal        int `json:"site_users_total"`
	SiteUsersWithShell    int `json:"site_users_with_shell"`
	SiteUsersOutsideGroup int `json:"site_users_outside_group"`
}

// FileAccessAccount is one account as the host has it.
type FileAccessAccount struct {
	Username    string `json:"username"`
	Exists      bool   `json:"exists"`
	HomeMatches bool   `json:"home_matches"`
	Enabled     bool   `json:"enabled"`
	SSH         bool   `json:"ssh"`
	ShellUsable bool   `json:"shell_usable"`
	// Password: none, set, locked or empty (the hash is never read out).
	Password    string          `json:"password"`
	InSiteGroup bool            `json:"in_site_group"`
	Keys        []FileAccessKey `json:"keys"`
	// KeysFile: absent, managed, owner or unreadable.
	KeysFile  string `json:"keys_file"`
	ChrootDir string `json:"chroot_dir"`
	StartDir  string `json:"start_dir"`
	// JailMissing names jail programs the host does not have.
	JailMissing []string `json:"jail_missing,omitempty"`
}

// FileAccessStateResponse is the State answer and part of every apply answer.
type FileAccessStateResponse struct {
	Account FileAccessAccount `json:"account"`
	Server  FileAccessServer  `json:"server"`
	Error   string            `json:"error,omitempty"`
}

// One planned host change, for the preview.
type FileAccessChange struct {
	// Kind: file, account, service, jail.
	Kind   string `json:"kind"`
	Target string `json:"target"`
	// Action: create, update, remove, keep, reload, restart, set.
	Action string `json:"action"`
	Detail string `json:"detail,omitempty"`
}

// Why the Agent refused an apply. Each has its own guidance on the page.
const (
	FileAccessRefusedInvalid         = "invalid"
	FileAccessRefusedNoAccount       = "no_account"
	FileAccessRefusedAccountMismatch = "account_mismatch"
	FileAccessRefusedNoSSHD          = "sshd_missing"
	FileAccessRefusedSSHDOwner       = "sshd_owner_config"
	FileAccessRefusedSSHDInclude     = "sshd_include_missing"
	FileAccessRefusedSSHDRejected    = "sshd_rejected"
	FileAccessRefusedChrootUnsafe    = "chroot_unsafe"
	FileAccessRefusedKeysOwner       = "keys_owner_file"
	FileAccessRefusedJail            = "jail_failed"
	FileAccessRefusedAccountChange   = "account_change_failed"
)

// Warnings: the access works, one part did not (FTP, Fail2ban, ...).
const (
	FileAccessWarnFTPNotInstalled = "ftp_not_installed"
	FileAccessWarnFTPOwnerConfig  = "ftp_owner_config"
	FileAccessWarnFTPRejected     = "ftp_rejected"
	FileAccessWarnFTPSelfSigned   = "ftp_self_signed"
	FileAccessWarnFail2banMissing = "fail2ban_missing"
	FileAccessWarnFail2banFailed  = "fail2ban_failed"
	FileAccessWarnFail2banOwner   = "fail2ban_owner_config"
	FileAccessWarnPasswordAuthOff = "ssh_password_auth_off"
	FileAccessWarnJailNoDevices   = "jail_no_devices"
	FileAccessWarnJailMissing     = "jail_programs_missing"
)

// ApplyFileAccessResponse answers both Apply and Preview. Applied is true
// only when the account's access is in place as requested (or, for a
// preview, never). Changes lists what was (or would be) changed.
type ApplyFileAccessResponse struct {
	Applied  bool                    `json:"applied"`
	Changes  []FileAccessChange      `json:"changes"`
	Refusal  string                  `json:"refusal,omitempty"`
	Detail   string                  `json:"detail,omitempty"`
	Warnings []string                `json:"warnings,omitempty"`
	State    FileAccessStateResponse `json:"state"`
}
