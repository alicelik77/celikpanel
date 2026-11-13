package transport

// More users for one database (alpha.129): a user that only reads it, or
// one that reads and writes it, made and removed on the local engine by the
// Agent. Creation is exclusive (an account name that already exists on the
// engine changes nothing); removal takes away the user and its rights and
// never the database or its data.
//
// Bir veritabanına ek kullanıcı: yalnız okuma ya da okuma ve yazma.

const (
	DatabaseAccessRead  = "read"
	DatabaseAccessWrite = "write"
)

// DatabaseAccessUserRequest names one user of one database.
type DatabaseAccessUserRequest struct {
	ExpectedBuildCommit string `json:"expected_build_commit"`
	// Engine is mysql (MariaDB) or postgresql.
	Engine string `json:"engine"`
	// Port is the PostgreSQL cluster of the database's record.
	Port     int    `json:"port,omitempty"`
	Database string `json:"database"`
	Username string `json:"username"`
	// Password and Access: creation only.
	Password string `json:"password,omitempty"`
	Access   string `json:"access,omitempty"`
	// Readers and Writers: the database's other added users (creation only,
	// PostgreSQL), so a reader sees what every writer makes and a new writer
	// shares what it makes with them (alpha.129).
	Readers []string `json:"readers,omitempty"`
	Writers []string `json:"writers,omitempty"`
}

// DatabaseAccessUserResponse says what happened.
// DatabaseAccessKeptOtherRights is the KeptReason of an account kept for
// rights CelikPanel did not give.
const DatabaseAccessKeptOtherRights = "other_rights"

// DatabaseAccessKeptUnchecked is the KeptReason of an account kept because
// what else it has could not be checked (fail closed).
const DatabaseAccessKeptUnchecked = "unchecked"

type DatabaseAccessUserResponse struct {
	Applied bool `json:"applied"`
	// Exists: creation found the account name already on the engine; nothing
	// was changed.
	Exists bool `json:"exists,omitempty"`
	// Absent: removal found no such account; nothing was left to remove.
	Absent bool `json:"absent,omitempty"`
	// CleanupIncomplete: a failed creation could not take back the account
	// it had just made.
	CleanupIncomplete bool `json:"cleanup_incomplete,omitempty"`
	// Kept (alpha.133 round 5): removal found rights on the account that
	// CelikPanel did not give - on another database, globally, or on a host
	// CelikPanel did not create - so the account stays; only what CelikPanel
	// gave on this database was taken away. KeptReason: "other_rights".
	// Additive. Kept: hesabın CelikPanel'in vermediği başka hakları var; hesap
	// kalır, yalnız bu veritabanındaki hakları alınır.
	Kept       bool                    `json:"kept,omitempty"`
	KeptReason string                  `json:"kept_reason,omitempty"`
	Refusal    *DatabaseClusterRefusal `json:"refusal,omitempty"`
	Error      string                  `json:"error,omitempty"`
}
