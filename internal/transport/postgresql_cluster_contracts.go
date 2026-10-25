package transport

// PostgreSQL majors side by side (alpha.108; owner direction of 2026-10-25,
// replacing D-038 §7's "major upgrade flow"): every PostgreSQL major runs as
// its own cluster on its own port (Debian and Ubuntu's postgresql-common);
// another major is installed beside the old one, a cluster's port can be
// changed with proof and rollback, a database is copied to another cluster
// with per-table row counts as proof, and the old major is removed only when
// its cluster holds no database. Nothing is converted in place.
//
// PostgreSQL ana sürümleri yan yana: her ana sürüm kendi portunda kendi
// kümesidir; veri yerinde dönüştürülmez.

// PostgreSQLCluster is one cluster as pg_lsclusters and its configuration
// say it is. Port is pg_lsclusters' answer; ConfiguredPort is the port line
// of its postgresql.conf (0 when the file names none, which means 5432).
// Databases are the user databases (not template0, template1, postgres) the
// cluster answered with; DatabasesKnown is false when it could not be asked
// (the cluster is down or did not answer), and DatabasesError says why.
type PostgreSQLCluster struct {
	Version        string   `json:"version"`
	Name           string   `json:"name"`
	Port           int      `json:"port"`
	ConfiguredPort int      `json:"configured_port"`
	Status         string   `json:"status"`
	Online         bool     `json:"online"`
	Owner          string   `json:"owner"`
	DataDirectory  string   `json:"data_directory"`
	ConfigPath     string   `json:"config_path"`
	Databases      []string `json:"databases"`
	DatabasesKnown bool     `json:"databases_known"`
	DatabasesError string   `json:"databases_error,omitempty"`
}

// PostgreSQLMajorOffer is one PostgreSQL major that is not installed and
// whether the package manager can install it now: Origin "distribution"
// (the operating system's own lists offer postgresql-NN), "pgdg" (only the
// enabled PGDG repository offers it) or "" with NeedsRepository when neither
// does (the PGDG repository card enables it). Port is the port the cluster
// Debian creates would get now (next free from 5432); KeptCluster is true
// when a cluster of this major is still on disk from an earlier removal, in
// which case the install uses it on its own configured port.
type PostgreSQLMajorOffer struct {
	Major           string `json:"major"`
	Package         string `json:"package"`
	Candidate       string `json:"candidate,omitempty"`
	Origin          string `json:"origin,omitempty"`
	NeedsRepository bool   `json:"needs_repository"`
	Port            int    `json:"port"`
	KeptCluster     bool   `json:"kept_cluster,omitempty"`
}

// PostgreSQLClustersResponse answers Agent.PostgreSQLClustersV1 (read only).
// Supported is false on a system without postgresql-common (not Debian or
// Ubuntu, or PostgreSQL not installed), with Reason. CreatesCluster is false
// when the owner's /etc/postgresql-common/createcluster.conf says
// create_main_cluster = false: an install then makes no cluster.
type PostgreSQLClustersResponse struct {
	Supported      bool                   `json:"supported"`
	Reason         string                 `json:"reason,omitempty"`
	Clusters       []PostgreSQLCluster    `json:"clusters"`
	Offers         []PostgreSQLMajorOffer `json:"offers"`
	NextPort       int                    `json:"next_port"`
	UsedPorts      []int                  `json:"used_ports"`
	CreatesCluster bool                   `json:"creates_cluster"`
	RepositoryOn   bool                   `json:"repository_on"`
	Error          string                 `json:"error,omitempty"`
}

// PostgreSQLClusterRef names one cluster: its major version and name
// ("17", "main").
type PostgreSQLClusterRef struct {
	Version string `json:"version"`
	Name    string `json:"name"`
}

// PostgreSQLPortPreviewRequest asks what changing a cluster's port to Port
// would do.
type PostgreSQLPortPreviewRequest struct {
	PostgreSQLClusterRef
	Port int `json:"port"`
}

// PostgreSQLSession is a group of live connections to the cluster, from
// pg_stat_activity (this read's own connection excluded).
type PostgreSQLSession struct {
	Database    string `json:"database"`
	User        string `json:"user"`
	Client      string `json:"client"`
	Application string `json:"application,omitempty"`
	Count       int    `json:"count"`
}

// PostgreSQLPortPreviewResponse is the plan: the configuration file, its
// SHA-256 (the change is written only over exactly this file), the line now
// and the line after, who is connected now, the port phpPgAdmin's own
// configuration names, and a Refusal when the change cannot be made (port
// below 1024, in use, set in another file).
type PostgreSQLPortPreviewResponse struct {
	Cluster     PostgreSQLCluster   `json:"cluster"`
	ConfigPath  string              `json:"config_path"`
	ConfigSHA   string              `json:"config_sha256"`
	LineBefore  string              `json:"line_before"`
	LineAfter   string              `json:"line_after"`
	From        int                 `json:"from"`
	To          int                 `json:"to"`
	Sessions    []PostgreSQLSession `json:"sessions"`
	SessionsOK  bool                `json:"sessions_known"`
	ToolPort    int                 `json:"phppgadmin_port,omitempty"`
	RefusalCode string              `json:"refusal_code,omitempty"`
	Refusal     string              `json:"refusal,omitempty"`
	Error       string              `json:"error,omitempty"`
}

// PostgreSQLSetPortRequest changes the port of one cluster, only over the
// configuration file the administrator saw (ExpectedSHA256).
type PostgreSQLSetPortRequest struct {
	ServiceMutationBinding
	PostgreSQLClusterRef
	Port           int    `json:"port"`
	ExpectedSHA256 string `json:"expected_sha256"`
}

// PostgreSQLSetPortResponse: Changed and Proved when the cluster answers on
// the new port; otherwise RolledBack says the previous file was written back
// and the cluster restarted, Restored that it answers on the old port again.
type PostgreSQLSetPortResponse struct {
	Changed    bool   `json:"changed"`
	Proved     bool   `json:"proved"`
	From       int    `json:"from"`
	To         int    `json:"to"`
	RolledBack bool   `json:"rolled_back,omitempty"`
	Restored   bool   `json:"restored,omitempty"`
	ErrorCode  string `json:"error_code,omitempty"`
	Error      string `json:"error,omitempty"`
}

// PostgreSQLCopyRequest copies one database from the cluster on SourcePort
// to the cluster on TargetPort. The source is never changed.
type PostgreSQLCopyRequest struct {
	ServiceMutationBinding
	Database   string `json:"database"`
	SourcePort int    `json:"source_port"`
	TargetPort int    `json:"target_port"`
}

// PostgreSQLTableCount is one table's rows on both sides.
type PostgreSQLTableCount struct {
	Schema string `json:"schema"`
	Table  string `json:"table"`
	Source int64  `json:"source"`
	Target int64  `json:"target"`
}

// PostgreSQLCopyProof is the proof of a copy: every ordinary table of the
// database, its rows on the source (read again after the copy) and on the
// target. Equal is true only when the two table lists are the same and every
// count is equal. Tables is capped (TablesTotal is the real number).
type PostgreSQLCopyProof struct {
	Equal         bool                   `json:"equal"`
	TablesTotal   int                    `json:"tables_total"`
	Tables        []PostgreSQLTableCount `json:"tables"`
	Mismatches    int                    `json:"mismatches"`
	SourceVersion string                 `json:"source_version,omitempty"`
	TargetVersion string                 `json:"target_version,omitempty"`
	RolesCreated  []string               `json:"roles_created"`
	RolesKept     []string               `json:"roles_kept"`
	Owner         string                 `json:"owner,omitempty"`
	SizeBytes     int64                  `json:"size_bytes,omitempty"`
	Seconds       int                    `json:"seconds,omitempty"`
	// SourceWrites (alpha.108 review) is the source database's write
	// counters when the copy was proved. The copy proves rows at copy time
	// only; a switch and the drop of the source read them again and refuse
	// when the source was written after the copy.
	SourceWrites *PostgreSQLWriteMark `json:"source_writes,omitempty"`
}

// PostgreSQLWriteMark is one database's cumulative write counters from
// pg_stat_database (rows inserted, updated, deleted) and when they were last
// reset; a different reset means the counters cannot prove anything.
type PostgreSQLWriteMark struct {
	Inserted   int64  `json:"inserted"`
	Updated    int64  `json:"updated"`
	Deleted    int64  `json:"deleted"`
	StatsReset string `json:"stats_reset,omitempty"`
}

// PostgreSQLSourceExpectation is what the source looked like at the copy:
// its write counters and its tables' rows (the stored list may be shortened;
// TablesTotal is the real number).
type PostgreSQLSourceExpectation struct {
	Writes      PostgreSQLWriteMark    `json:"writes"`
	Tables      []PostgreSQLTableCount `json:"tables"`
	TablesTotal int                    `json:"tables_total"`
}

// PostgreSQLSourceCheckRequest asks, read only, whether a database is still
// as it was at the copy.
type PostgreSQLSourceCheckRequest struct {
	Database string                      `json:"database"`
	Port     int                         `json:"port"`
	Expect   PostgreSQLSourceExpectation `json:"expect"`
}

// PostgreSQLSourceCheckResponse: Unchanged, or Code SOURCE_CHANGED (written
// after the copy) / SOURCE_UNKNOWN (it could not be read) with Error.
type PostgreSQLSourceCheckResponse struct {
	Unchanged bool   `json:"unchanged"`
	Code      string `json:"code,omitempty"`
	Error     string `json:"error,omitempty"`
}

// PostgreSQLCopyResponse: Copied and Proof.Equal when the copy is complete
// and proved. A copy that failed or did not prove is removed from the target
// (Cleaned) and the roles it created are dropped again.
type PostgreSQLCopyResponse struct {
	Copied    bool                `json:"copied"`
	Proof     PostgreSQLCopyProof `json:"proof"`
	Cleaned   bool                `json:"cleaned,omitempty"`
	ErrorCode string              `json:"error_code,omitempty"`
	Error     string              `json:"error,omitempty"`
}

// PostgreSQLDropRequest drops one database on the cluster on Port, only
// while a copy of it exists on the cluster on KeepPort. Roles are kept.
type PostgreSQLDropRequest struct {
	ServiceMutationBinding
	Database string `json:"database"`
	Port     int    `json:"port"`
	KeepPort int    `json:"keep_port"`
	// ExpectSource (alpha.108 review): dropping the source of a move is
	// refused unless it is still as it was at the copy.
	ExpectSource *PostgreSQLSourceExpectation `json:"expect_source,omitempty"`
}

// PostgreSQLDropResponse says whether the database is gone.
type PostgreSQLDropResponse struct {
	Dropped   bool   `json:"dropped"`
	ErrorCode string `json:"error_code,omitempty"`
	Error     string `json:"error,omitempty"`
}
