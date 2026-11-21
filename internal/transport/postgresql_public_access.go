package transport

// PostgreSQL databases made before alpha.141 kept PostgreSQL's default
// database rights, under which every role of the server may connect to a
// database and make temporary tables in it (security lane S1b, 20 November
// 2026). The Panel closes them to other accounts: by itself only where the
// database's rights are still that untouched default and nothing inside it
// belongs to a role outside CelikPanel's records for it; everything else is
// listed for the administrator, who closes it after reading what changes.
// Each close keeps what it changed, so it can be undone exactly.
//
// alpha.141'den önce açılan PostgreSQL veritabanları başka hesaplara kapatılır;
// kendiliğinden yalnız dokunulmamış varsayılan haklarda, diğerleri yöneticiye
// listelenir.

// PostgreSQLPublicAccessDatabase is one database the Panel records on a
// cluster of this machine, with the roles it records for it.
type PostgreSQLPublicAccessDatabase struct {
	Name     string   `json:"name"`
	Recorded []string `json:"recorded"`
}

// PostgreSQLPublicAccessScanRequest reads, without changing anything, the
// rights of the recorded databases of one cluster.
type PostgreSQLPublicAccessScanRequest struct {
	ExpectedBuildCommit string                           `json:"expected_build_commit"`
	Port                int                              `json:"port"`
	Databases           []PostgreSQLPublicAccessDatabase `json:"databases"`
}

// PostgreSQLPublicAccessFacts is what one database shows.
type PostgreSQLPublicAccessFacts struct {
	Name   string `json:"name"`
	Exists bool   `json:"exists"`
	Owner  string `json:"owner,omitempty"`
	// ACL is the database's own rights as PostgreSQL stores them; "" is the
	// untouched default (no rights written).
	ACL string `json:"acl"`
	// PublicConnect and PublicTemporary: what every role of the server may
	// do in it now.
	PublicConnect   bool `json:"public_connect"`
	PublicTemporary bool `json:"public_temporary"`
	// Roles: every role (not an administrator, not one of PostgreSQL's own)
	// that owns something in it or holds a right in it. Sessions: the roles
	// (not an administrator) connected to it now.
	Roles    []string `json:"roles"`
	Sessions []string `json:"sessions"`
	// Missing (alpha.143 fix round): recorded users of this database that
	// have no role on the cluster.
	Missing []string `json:"missing,omitempty"`
	Error   string   `json:"error,omitempty"`
}

type PostgreSQLPublicAccessScanResponse struct {
	BuildMismatch bool                    `json:"build_mismatch,omitempty"`
	Error         string                  `json:"error,omitempty"`
	Refusal       *DatabaseClusterRefusal `json:"refusal,omitempty"`
	// Monitoring: login roles of the cluster (not an administrator) that are
	// members of PostgreSQL's monitoring roles; they may sign in to any
	// database through the default rights.
	Monitoring []string `json:"monitoring"`
	// Replication (alpha.143 fix round): roles of the cluster (not an
	// administrator) with the REPLICATION attribute; a logical replication
	// connection of such a role reads a database through its CONNECT right.
	Replication []string                      `json:"replication,omitempty"`
	Databases   []PostgreSQLPublicAccessFacts `json:"databases"`
}

// PostgreSQLPublicAccessGrant gives one recorded role CONNECT (and
// TEMPORARY for a read-and-write user) before PUBLIC loses them.
type PostgreSQLPublicAccessGrant struct {
	Role      string `json:"role"`
	Temporary bool   `json:"temporary,omitempty"`
}

// PostgreSQLPublicAccessCloseRequest closes one database to other accounts
// in one transaction: it changes nothing unless the database's rights are
// still ExpectACL and the roles inside are still ExpectRoles (and, for the
// Panel's own close, no other role is connected); it gives Grants, takes
// CONNECT and TEMPORARY from PUBLIC and reads back that PUBLIC has neither
// and every granted role and the owner may connect.
type PostgreSQLPublicAccessCloseRequest struct {
	ExpectedBuildCommit string                        `json:"expected_build_commit"`
	Port                int                           `json:"port"`
	Name                string                        `json:"name"`
	ExpectACL           string                        `json:"expect_acl"`
	ExpectRoles         []string                      `json:"expect_roles"`
	Grants              []PostgreSQLPublicAccessGrant `json:"grants"`
	// NoOtherSessions: refuse while a role outside ExpectRoles' recorded
	// part is connected (the Panel's own close; an administrator's close
	// has read the sessions).
	NoOtherSessions bool     `json:"no_other_sessions,omitempty"`
	Recorded        []string `json:"recorded"`
}

// PostgreSQLPublicAccessChange is what a close or an undo changed.
type PostgreSQLPublicAccessChange struct {
	BeforeACL string `json:"before_acl"`
	AfterACL  string `json:"after_acl"`
	// Added: the rights the close gave that the role did not hold before
	// ("role:CONNECT", "role:TEMPORARY"); an undo takes exactly these away.
	Added []string `json:"added"`
}

type PostgreSQLPublicAccessCloseResponse struct {
	BuildMismatch bool `json:"build_mismatch,omitempty"`
	// Changed: the database is no longer as it was read; nothing was done.
	Changed bool `json:"changed,omitempty"`
	// Unknown (alpha.143 fix round): the script got past its BEGIN and then
	// failed or gave no read-back, so whether the change was committed is
	// not known; the Panel reads the database's state before anything else.
	// Without Changed, Closed or Unknown, Error is a verified failure:
	// nothing was changed.
	Unknown bool                          `json:"unknown,omitempty"`
	Closed  bool                          `json:"closed,omitempty"`
	Change  *PostgreSQLPublicAccessChange `json:"change,omitempty"`
	Error   string                        `json:"error,omitempty"`
	Refusal *DatabaseClusterRefusal       `json:"refusal,omitempty"`
}

// PostgreSQLPublicAccessUndoRequest undoes one close in one transaction:
// only while the database's rights are still ExpectACL (what the close
// left), it gives PUBLIC back what it had and takes away Added.
type PostgreSQLPublicAccessUndoRequest struct {
	ExpectedBuildCommit string   `json:"expected_build_commit"`
	Port                int      `json:"port"`
	Name                string   `json:"name"`
	ExpectACL           string   `json:"expect_acl"`
	PublicConnect       bool     `json:"public_connect"`
	PublicTemporary     bool     `json:"public_temporary"`
	Added               []string `json:"added"`
}

type PostgreSQLPublicAccessUndoResponse struct {
	BuildMismatch bool                    `json:"build_mismatch,omitempty"`
	Changed       bool                    `json:"changed,omitempty"`
	Unknown       bool                    `json:"unknown,omitempty"`
	Undone        bool                    `json:"undone,omitempty"`
	AfterACL      string                  `json:"after_acl,omitempty"`
	Error         string                  `json:"error,omitempty"`
	Refusal       *DatabaseClusterRefusal `json:"refusal,omitempty"`
}
