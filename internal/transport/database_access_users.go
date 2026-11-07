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
}

// DatabaseAccessUserResponse says what happened.
type DatabaseAccessUserResponse struct {
	Applied bool `json:"applied"`
	// Exists: creation found the account name already on the engine; nothing
	// was changed.
	Exists bool `json:"exists,omitempty"`
	// Absent: removal found no such account; nothing was left to remove.
	Absent bool `json:"absent,omitempty"`
	// CleanupIncomplete: a failed creation could not take back the account
	// it had just made.
	CleanupIncomplete bool                    `json:"cleanup_incomplete,omitempty"`
	Refusal           *DatabaseClusterRefusal `json:"refusal,omitempty"`
	Error             string                  `json:"error,omitempty"`
}
