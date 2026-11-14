package transport

// Remote access to databases (alpha.129, docs/DATABASE-REMOTE-ACCESS.md).
// The panel sends the Agent the complete rule set of every engine instance
// (MariaDB; each PostgreSQL cluster by port) and the Agent makes the native
// configuration match it: the nftables address filter, pg_hba lines or
// MariaDB accounts, TLS and the listen address. Status reads only.
//
// Uzak veritabanı erişimi: panel her motorun tüm kural kümesini gönderir,
// Agent yerel yapılandırmayı ona uydurur. Durum yalnız okur.

// DatabaseRemoteRule lets one database user reach one database from one
// address (canonical prefix, e.g. 203.0.113.7/32).
type DatabaseRemoteRule struct {
	Database string
	User     string
	Address  string
}

// DatabaseRemoteTarget is one engine instance and every rule it should carry.
// Engine is "mysql" or "postgresql"; Port is the instance's port.
type DatabaseRemoteTarget struct {
	Engine string
	Port   int
	Rules  []DatabaseRemoteRule
}

// DatabaseRemoteApplyRequest: the whole desired state. A target the Agent
// changed before and that is not named here is closed (no rules).
type DatabaseRemoteApplyRequest struct {
	ServiceMutationBinding
	Targets []DatabaseRemoteTarget
	// HostName is the certificate's name when the Agent has to make one.
	HostName string
	// MariaDBSkipNameResolve (alpha.130): the administrator agreed that
	// CelikPanel turns MariaDB's host name lookup off while remote rules
	// exist (its option file), so refused sign-ins name the client's
	// address and the Fail2ban jail can ban it. Without it, MariaDB rules
	// wait while MariaDB looks host names up.
	MariaDBSkipNameResolve bool
	// ServerOffered (alpha.131): the administrator's server switch is on.
	// An engine CelikPanel already prepared (listen address, TLS, the slowing
	// of wrong passwords; MariaDB's option file) keeps it when its last rule
	// goes, so a customer's next address needs no restart; the address filter
	// and the engine's own rules still close every address. Only the switch
	// turned off undoes it. An older panel sends false: alpha.130 behaviour.
	ServerOffered bool
	// ExactCopies (alpha.134): a remote copy's grant held in the pattern form
	// (written before alpha.134) may be rewritten to the exact form now - the
	// panel's rewrite of rights to exact names is allowed to run (the update
	// is verified, the server is quiet). False leaves such a grant as it is.
	ExactCopies bool
}

// DatabaseRemoteNarrowRequest (alpha.130, D5) narrows the address filter to
// the addresses of Targets' rules: an address leaves it, nothing is ever
// added. It runs no engine client and no package or service manager, takes
// no service-mutation lease (so it never holds back a release recovery's
// idle check) and also works while the server has not finished starting;
// the engines' own rules follow at the next full apply.
type DatabaseRemoteNarrowRequest struct {
	Targets []DatabaseRemoteTarget
}

// DatabaseRemoteStatusRequest names the instances to describe.
type DatabaseRemoteStatusRequest struct {
	Targets []DatabaseRemoteTarget
}

// Problem codes. Each is one plain sentence on the screen with its fix.
const (
	DatabaseRemoteNFTMissing        = "NFT_MISSING"        // nftables is not installed
	DatabaseRemoteEngineDown        = "ENGINE_DOWN"        // the engine does not answer
	DatabaseRemoteOwnerListen       = "OWNER_LISTEN"       // the owner set the listen address
	DatabaseRemoteOwnerHBA          = "OWNER_HBA"          // pg_hba.conf lets the network in before CelikPanel's block
	DatabaseRemoteOwnerFile         = "OWNER_FILE"         // a CelikPanel file was edited by hand
	DatabaseRemoteRestartNeeded     = "RESTART_NEEDED"     // the engine must restart for the listen address/TLS
	DatabaseRemoteTLSMissing        = "TLS_MISSING"        // TLS could not be turned on
	DatabaseRemoteAlterSystemOff    = "ALTER_SYSTEM_OFF"   // PostgreSQL refuses ALTER SYSTEM
	DatabaseRemotePasswordMD5       = "PASSWORD_MD5"       // the role's password is stored as md5
	DatabaseRemotePrivilegedUser    = "PRIVILEGED_USER"    // superuser / server-wide rights
	DatabaseRemoteNoAccess          = "NO_ACCESS"          // the user holds no access to the database
	DatabaseRemoteUserMissing       = "USER_MISSING"       // the user does not exist on the engine
	DatabaseRemoteAccountConflict   = "ACCOUNT_CONFLICT"   // user@host exists and is not CelikPanel's
	DatabaseRemoteNoSlowDown        = "NO_SLOWDOWN"        // wrong passwords are not slowed down
	DatabaseRemoteFailed            = "FAILED"             // a verified failure; Detail says which
	DatabaseRemoteRecordUnreadable  = "RECORD_UNREADABLE"  // the Agent's record cannot be read
	DatabaseRemoteFirewallUnchanged = "FIREWALL_UNCHANGED" // the address filter could not be written
	// alpha.130: MariaDB looks client host names up, so a refused sign-in
	// names a host, which the jail does not ban; the administrator may let
	// CelikPanel turn the lookup off (MariaDBSkipNameResolve).
	DatabaseRemoteNameResolve = "NAME_RESOLVE"
	// alpha.130: the owner's MariaDB configuration sets the lookup itself
	// (Detail names the file); CelikPanel does not override it.
	DatabaseRemoteOwnerNameResolve = "OWNER_NAME_RESOLVE"
	// alpha.130: accounts with a host name (Detail lists them) would stop
	// working once the lookup is off; they must be changed first.
	DatabaseRemoteNameAccounts = "NAME_ACCOUNTS"
	// alpha.130: an informational note on the extra Fail2ban layer of
	// PostgreSQL (never holds a rule back).
	DatabaseRemotePGFail2ban = "PG_FAIL2BAN"
)

// DatabaseRemoteProblem is one problem of an instance or of one rule (User,
// Database and Address set).
type DatabaseRemoteProblem struct {
	Code     string
	Detail   string
	User     string
	Database string
	Address  string
}

// Instance states.
const (
	DatabaseRemoteStateOff          = "off"          // no rule; the engine listens locally
	DatabaseRemoteStateOpen         = "open"         // rules in force, listening, TLS on
	DatabaseRemoteStateWaitRestart  = "wait_restart" // rules written, the engine must restart
	DatabaseRemoteStateWaiting      = "waiting"      // rules wait for a prerequisite (slowing, TLS); see the problems
	DatabaseRemoteStateClosing      = "closing"      // no rule; the port stays filtered until the restart
	DatabaseRemoteStateOwnerManaged = "owner_managed"
	DatabaseRemoteStateDown         = "down"
	DatabaseRemoteStateFailed       = "failed"
)

// DatabaseRemoteTargetResult describes one instance after the change (or as
// it is, for a status read).
type DatabaseRemoteTargetResult struct {
	Engine string
	Port   int
	State  string
	// Unit is the systemd unit whose restart applies the waiting change.
	Unit string
	// ListenRemote: the running engine listens beyond the local address.
	ListenRemote bool
	// TLS: "on", "pending_restart" or "off".
	TLS string
	// SlowDown: "auth_delay", "auth_delay_pending_restart", "fail2ban" or
	// "none".
	SlowDown string
	// Active: the rules in force at the engine (refused rules left out).
	Active   []DatabaseRemoteRule
	Problems []DatabaseRemoteProblem
	// Fail2ban (alpha.130): PostgreSQL's extra layer: "on" (its jail runs),
	// "off" (no rule, or not wanted), "unavailable" (Fail2ban missing or
	// stopped, or the log cannot be read: Fail2banDetail says why). MariaDB:
	// "on" or "unavailable" (its jail is required).
	Fail2ban       string
	Fail2banDetail string
	// NameResolve (alpha.130, MariaDB): "off" (addresses are logged), "on",
	// "pending_restart" (CelikPanel's file turns it off at the next start).
	NameResolve string
	// Readiness (alpha.130, status only): what is still missing before a
	// first rule could be in force here, read without changing anything.
	Readiness []DatabaseRemoteProblem
}

// DatabaseRemoteResponse answers both apply and status.
type DatabaseRemoteResponse struct {
	Targets      []DatabaseRemoteTargetResult
	NFTAvailable bool
	// FilterPorts: the ports the address filter holds now.
	FilterPorts []int
	// FilterLoaded: nft lists the filter's chain with its drop rules now
	// (review L2); the panel opens a port only while it is true.
	FilterLoaded bool
	ErrorCode    string
	Error        string
	// PanelFirewallRestores (alpha.130, D6): how often this Agent put the
	// panel firewall's table back after it vanished (nft flush ruleset);
	// the panel syncs its live policy when the number changes.
	PanelFirewallRestores int
	// PanelFirewallNote: why the vanished table was not put back (the
	// owner turned the firewall's boot restore or nftables off), or the
	// last restore failure.
	PanelFirewallNote string
	// AppliedDigest (alpha.130, status): dbremote.RequestDigest of the last
	// clean apply, "" when none; Targets' Active then are the rules in
	// force by the Agent's record.
	AppliedDigest string
}
