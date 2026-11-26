package transport

// PostgreSQL role limits through the Agent (alpha.149 fix round B). The panel
// connects to PostgreSQL as celikpanel_admin (CREATEROLE, not a superuser):
// since PostgreSQL 16 it may alter only the roles it created itself, and
// temp_file_limit needs a superuser in any case. So a site's role limits go
// through this one narrow call, which the Agent runs as the postgres
// superuser. The panel sends a role name and integer values only; the Agent
// checks the role itself and builds every statement (cmd/agent
// database_role_limits_rpc.go). The panel's own account gains no right.
//
// PostgreSQL rol sınırları Agent üzerinden; panel yalnız rol adı ve tam sayı
// değerler gönderir, ifadeleri Agent kurar.

// The keys a PostgreSQL role-limit write may name.
const (
	PGRoleLimitConnection               = "connection_limit"
	PGRoleLimitStatementSeconds         = "statement_seconds"
	PGRoleLimitIdleInTransactionSeconds = "idle_in_transaction_seconds"
	PGRoleLimitTempFileMB               = "temp_file_mb"
)

// The Agent's refusals of a role (PostgreSQLRoleLimitsResponse.Refused).
const (
	PGRoleLimitRefusedInvalid     = "invalid"
	PGRoleLimitRefusedReserved    = "reserved"
	PGRoleLimitRefusedNotRecorded = "not_recorded"
	PGRoleLimitRefusedAbsent      = DatabaseAccountRefusedAbsent
	PGRoleLimitRefusedPrivileged  = DatabaseAccountRefusedPrivileged
	// PGRoleLimitRefusedMember (alpha.149): the role is a member of another
	// role, which no site role CelikPanel makes is.
	PGRoleLimitRefusedMember = DatabaseAccountRefusedMember
)

// PostgreSQLRoleLimitWrite is one setting: the connection limit (0: none,
// written as -1) or one role setting (0: RESET).
type PostgreSQLRoleLimitWrite struct {
	Key   string `json:"key"`
	Value int    `json:"value"`
}

// PostgreSQLRoleLimitsRequest reads, and with Writes changes, the limits of
// one site role on this machine's cluster at Port.
type PostgreSQLRoleLimitsRequest struct {
	ExpectedBuildCommit string                     `json:"expected_build_commit"`
	Port                int                        `json:"port"`
	Role                string                     `json:"role"`
	Writes              []PostgreSQLRoleLimitWrite `json:"writes,omitempty"`
}

// PostgreSQLRoleLimitsResponse is the role's state as the Agent read it after
// any writes. Exactly one of Applied (with the values), Refused, Refusal,
// Prerequisite, Unknown or a plain Error describes the outcome.
type PostgreSQLRoleLimitsResponse struct {
	// Applied: the writes (if any) committed and the values below were read.
	Applied bool `json:"applied"`
	// ConnectionLimit is pg_roles.rolconnlimit (-1: none).
	ConnectionLimit int `json:"connection_limit"`
	// Settings are the role's own values per role key ("" when not set).
	Settings map[string]string `json:"settings,omitempty"`
	// Refused names why the Agent would not touch the role; nothing ran.
	Refused string `json:"refused,omitempty"`
	// Refusal: the role's cluster does not run; nothing ran.
	Refusal *DatabaseClusterRefusal `json:"refusal,omitempty"`
	// Prerequisite: the database server does not run (the Agent read its
	// service state or its socket); nothing ran.
	Prerequisite bool `json:"prerequisite,omitempty"`
	// Connection (alpha.149): the database server runs but refused the
	// connection (authentication, too many connections, starting up);
	// nothing ran. Additive.
	Connection bool `json:"connection,omitempty"`
	// Unknown: the statements were sent and their end could not be read.
	Unknown bool   `json:"unknown,omitempty"`
	Error   string `json:"error,omitempty"`
}
