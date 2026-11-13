package transport

import "github.com/alicelik/celikpanel/internal/mariadbgrant"

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
	// DatabaseDropped (alpha.133 lab round): removal as part of deleting the
	// database, which is already dropped. An account that is kept then loses
	// every right that names this database on every host of the name - the
	// owner's own hosts too - because MariaDB keeps such rights after DROP
	// DATABASE and a later database of the same name would inherit them.
	// Additive. Silinen veritabanını adlandıran her hak, hesap kalsa da
	// bütün host'lardan kaldırılır.
	DatabaseDropped bool `json:"database_dropped,omitempty"`
	// RightsOnly (with DatabaseDropped): the user stays (another database uses
	// it); only the rights that name the dropped database go - no DROP USER,
	// no check of other rights. Additive.
	// RightsOnly: kullanıcı kalır, yalnız silinen veritabanını adlandıran
	// hakları gider.
	RightsOnly bool `json:"rights_only,omitempty"`
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
	// RightsLeft (alpha.133 lab round): the account is kept, but a right that
	// names the dropped database could not be removed from it (Applied is
	// false; removing again continues). Additive.
	RightsLeft bool `json:"rights_left,omitempty"`
	// DatabaseExists (with DatabaseDropped): the engine still has a database of
	// that name, so nothing was removed. Additive.
	DatabaseExists bool `json:"database_exists,omitempty"`
}

// DroppedDatabaseRightsRequest (alpha.134): after a database is deleted on
// the machine's own MariaDB, every right that names it goes from every
// grantee - accounts made by hand, accounts under any name and roles - while
// the accounts and roles stay. Only while no database of that name exists.
// Silinen veritabanını adlandıran haklar her hesaptan ve rolden kaldırılır.
type DroppedDatabaseRightsRequest struct {
	ExpectedBuildCommit string `json:"expected_build_commit"`
	Database            string `json:"database"`
}

// DroppedDatabaseRightsResponse says whose rights went.
type DroppedDatabaseRightsResponse struct {
	Applied bool `json:"applied"`
	// Grantees: user@host of each account (a role's name alone) that held a
	// right naming the database and lost it. Names only.
	Grantees []string `json:"grantees,omitempty"`
	// DatabaseExists: a database of that name exists (again); nothing was
	// changed.
	DatabaseExists bool `json:"database_exists,omitempty"`
	// RightsLeft: something naming the database is still loaded; removing
	// again continues.
	RightsLeft bool   `json:"rights_left,omitempty"`
	Error      string `json:"error,omitempty"`
}

// DatabaseGrantFormsScanRequest (alpha.134) reads the machine's own MariaDB
// for the rewrite of pattern-form grants to the exact form. Read only.
// Kalıp biçimli hakların okunması; salt okur.
type DatabaseGrantFormsScanRequest struct {
	ExpectedBuildCommit string `json:"expected_build_commit"`
}

// DatabaseGrantFormsScanResponse is what the read found. RemoteHosts: for
// each account name of a pattern row, the hosts of its remote copies the
// Agent's remote access record lists (CelikPanel's own besides localhost).
type DatabaseGrantFormsScanResponse struct {
	Scan          mariadbgrant.Scan   `json:"scan"`
	RemoteHosts   map[string][]string `json:"remote_hosts,omitempty"`
	BuildMismatch bool                `json:"build_mismatch,omitempty"`
	Error         string              `json:"error,omitempty"`
}

// DatabaseGrantFormConvertRequest rewrites one captured pattern-form row to
// the exact form (mariadbgrant.Convert).
type DatabaseGrantFormConvertRequest struct {
	ExpectedBuildCommit string `json:"expected_build_commit"`
	mariadbgrant.ConvertRequest
}

// DatabaseGrantFormConvertResponse: Outcome is one of mariadbgrant's
// Converted, AlreadyExact, Gone, Changed, OwnerRow; Error when it stopped.
type DatabaseGrantFormConvertResponse struct {
	Outcome       string `json:"outcome,omitempty"`
	BuildMismatch bool   `json:"build_mismatch,omitempty"`
	Error         string `json:"error,omitempty"`
}
