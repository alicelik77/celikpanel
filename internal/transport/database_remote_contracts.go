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
}
