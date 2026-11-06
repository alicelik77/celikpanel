// Package backupspec defines the versioned RPC contract shared by the
// unprivileged panel and the privileged agent.
package backupspec

import "time"

const (
	ProtocolVersion = 2
	MaxChunkBytes   = 1 << 20
	MaxJobKeyBytes  = 128
)

const (
	TypeFiles    = "files"
	TypeDatabase = "database"
	TypeFull     = "full"
)

const (
	OriginManual     = "manual"
	OriginScheduled  = "scheduled"
	OriginPreRestore = "pre_restore"
	// OriginPreUpdate (alpha.118): the backup a WordPress update makes
	// first, restored when the update changed files and failed.
	OriginPreUpdate = "pre_update"
)

// DatabaseIdentity is resolved from durable tenant metadata by the panel.
// The agent validates it again before invoking a database client.
type DatabaseIdentity struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
	// ClusterVersion and ClusterPort name the PostgreSQL cluster the database
	// lives on (alpha.127: majors side by side, each on its own port). The
	// panel sends its record's port; the agent resolves the cluster at backup
	// time and records both in the manifest, and a restore goes only to a
	// running cluster of the same major. Empty in a backup made before
	// alpha.127 and for MariaDB. Additive.
	// ClusterVersion/ClusterPort veritabanının PostgreSQL kümesini adlandırır;
	// geri yükleme yalnız aynı ana sürümün çalışan kümesine gider.
	ClusterVersion string `json:"cluster_version,omitempty"`
	ClusterPort    int    `json:"cluster_port,omitempty"`
}

// SameDatabase: the same database (ID, name, engine), whatever cluster each
// side names.
func (d DatabaseIdentity) SameDatabase(other DatabaseIdentity) bool {
	return d.ID == other.ID && d.Name == other.Name && d.Type == other.Type
}

// ClusterRefusal says why a PostgreSQL backup or restore did not run: the
// cluster it needs does not run (Reason "removed", "stopped", "none"), or a
// restore would cross majors ("major_mismatch": the backup's BackupVersion,
// the database's cluster now Version). Nothing ran. The fields match
// transport.DatabaseClusterRefusal.
// ClusterRefusal, PostgreSQL yedeğinin ya da geri yüklemenin neden
// çalışmadığını söyler; hiçbir şey çalışmamıştır.
type ClusterRefusal struct {
	Reason         string `json:"reason"`
	Version        string `json:"version,omitempty"`
	Cluster        string `json:"cluster,omitempty"`
	Port           int    `json:"port,omitempty"`
	DataDirectory  string `json:"data_directory,omitempty"`
	RunningVersion string `json:"running_version,omitempty"`
	RunningPort    int    `json:"running_port,omitempty"`
	BackupVersion  string `json:"backup_version,omitempty"`
}

// RequestScope carries immutable tenant identity. Request structs repeat these
// fields at their top level because net/rpc's gob encoding does not flatten an
// embedded struct for older peers. DomainName is retained only to locate
// backups written by pre-v2 releases; new storage is ID-based.
type RequestScope struct {
	ProtocolVersion int    `json:"protocol_version"`
	SubscriptionID  int    `json:"subscription_id"`
	DomainID        int    `json:"domain_id"`
	DomainName      string `json:"domain_name"`
}

type CreateRequest struct {
	ProtocolVersion int    `json:"protocol_version"`
	SubscriptionID  int    `json:"subscription_id"`
	DomainID        int    `json:"domain_id"`
	DomainName      string `json:"domain_name"`
	Type            string `json:"type"`
	Origin          string `json:"origin"`
	// JobKey makes a logical create operation idempotent across RPC timeouts
	// and retries. Scheduled creates always set it; manual and pre-restore
	// creates leave it empty.
	JobKey    string             `json:"job_key,omitempty"`
	Database  DatabaseIdentity   `json:"database,omitempty"`
	Databases []DatabaseIdentity `json:"databases,omitempty"`

	// DatabaseName and DatabaseType keep the v2 panel wire-compatible with a
	// pre-v2 agent during a stopped-together upgrade or rollback. A v2 agent
	// uses Database instead and never trusts these fields for a v2 request.
	DatabaseName string `json:"database_name,omitempty"`
	DatabaseType string `json:"database_type,omitempty"`

	// SourceDir is sent during the v2 transition so a stopped-together
	// deployment can still be rolled back to the previous agent binary.
	// A v2 agent never trusts or uses it.
	SourceDir string `json:"source_dir,omitempty"`
}

type Info struct {
	// Path is retained for gob compatibility with pre-v2 panel binaries. It is
	// never serialized to the browser and callers must not trust it as input.
	Path       string    `json:"-"`
	Name       string    `json:"name"`
	Size       int64     `json:"size"`
	Type       string    `json:"type"`
	Origin     string    `json:"origin"`
	DatabaseID int       `json:"database_id,omitempty"`
	Legacy     bool      `json:"legacy,omitempty"`
	Restorable bool      `json:"restorable"`
	CreatedAt  time.Time `json:"created_at"`
}

type CreateResponse struct {
	Success bool   `json:"success"`
	Backup  Info   `json:"backup,omitempty"`
	Error   string `json:"error,omitempty"`
	// Refusal (alpha.127): a PostgreSQL cluster prerequisite, nothing ran.
	Refusal *ClusterRefusal `json:"refusal,omitempty"`
}

type ListRequest struct {
	ProtocolVersion int    `json:"protocol_version"`
	SubscriptionID  int    `json:"subscription_id"`
	DomainID        int    `json:"domain_id"`
	DomainName      string `json:"domain_name"`
}

type ListResponse struct {
	Backups []Info `json:"backups"`
}

type InspectRequest struct {
	ProtocolVersion int    `json:"protocol_version"`
	SubscriptionID  int    `json:"subscription_id"`
	DomainID        int    `json:"domain_id"`
	DomainName      string `json:"domain_name"`
	BackupName      string `json:"backup_name"`
}

type InspectResponse struct {
	Success   bool               `json:"success"`
	Backup    Info               `json:"backup,omitempty"`
	Databases []DatabaseIdentity `json:"databases,omitempty"`
	Error     string             `json:"error,omitempty"`
}

type RestoreRequest struct {
	ProtocolVersion int                `json:"protocol_version"`
	SubscriptionID  int                `json:"subscription_id"`
	DomainID        int                `json:"domain_id"`
	DomainName      string             `json:"domain_name"`
	BackupName      string             `json:"backup_name"`
	Database        DatabaseIdentity   `json:"database,omitempty"`
	Databases       []DatabaseIdentity `json:"databases,omitempty"`

	// TargetDir is a rollback compatibility field. A v2 agent derives the
	// document root from SubscriptionID and DomainID and ignores this value.
	TargetDir string `json:"target_dir,omitempty"`
}

type RestoreResponse struct {
	Success      bool   `json:"success"`
	SafetyBackup *Info  `json:"safety_backup,omitempty"`
	Error        string `json:"error,omitempty"`
	// Refusal (alpha.127): a PostgreSQL cluster prerequisite, refused before
	// the safety backup and before anything was written.
	Refusal *ClusterRefusal `json:"refusal,omitempty"`
}

// RestoreInProgress is the exact RestoreResponse.Error of a restore refused
// because another restore of the same domain is still running on the Agent.
// The refused request changed nothing (D-029).
const RestoreInProgress = "RESTORE_IN_PROGRESS"

type DeleteRequest struct {
	ProtocolVersion int    `json:"protocol_version"`
	SubscriptionID  int    `json:"subscription_id"`
	DomainID        int    `json:"domain_id"`
	DomainName      string `json:"domain_name"`
	BackupName      string `json:"backup_name"`
}

type ReadChunkRequest struct {
	ProtocolVersion int    `json:"protocol_version"`
	SubscriptionID  int    `json:"subscription_id"`
	DomainID        int    `json:"domain_id"`
	DomainName      string `json:"domain_name"`
	BackupName      string `json:"backup_name"`
	Offset          int64  `json:"offset"`
	MaxBytes        int    `json:"max_bytes"`
}

type ReadChunkResponse struct {
	Data   []byte `json:"data"`
	Offset int64  `json:"offset"`
	Size   int64  `json:"size"`
	EOF    bool   `json:"eof"`
}

// ValidJobKey reports whether key is safe to persist, log and compare as an
// opaque idempotency identifier. It deliberately accepts only a small ASCII
// alphabet so every transport and filesystem-facing diagnostic represents the
// same bytes.
func ValidJobKey(key string) bool {
	if len(key) < 1 || len(key) > MaxJobKeyBytes {
		return false
	}
	for i := 0; i < len(key); i++ {
		c := key[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.' || c == ':' {
			continue
		}
		return false
	}
	return true
}

// ImportChunkRequest writes one chunk of a backup package that came back from
// a remote destination (alpha.118). Chunks are appended in order to a private
// staging file of this import; the Final chunk carries the whole package's
// size and SHA-256, and the Agent publishes the package under BackupName only
// after the size, the hash, the manifest's scope and every payload's hash are
// verified. A chunk that is sent again after a lost answer (the same offset
// and length as the file already holds) is accepted without writing twice.
// Additive: existing requests are unchanged.
//
// Uzak hedeften dönen yedek paketi parça parça yazılır; Agent boyutu, özeti,
// kapsamı ve her yükün özetini doğrulamadan yayımlamaz.
type ImportChunkRequest struct {
	ProtocolVersion int    `json:"protocol_version"`
	SubscriptionID  int    `json:"subscription_id"`
	DomainID        int    `json:"domain_id"`
	DomainName      string `json:"domain_name"`
	BackupName      string `json:"backup_name"`
	// ImportID identifies one import (ValidJobKey alphabet).
	ImportID string `json:"import_id"`
	Offset   int64  `json:"offset"`
	Data     []byte `json:"data"`
	Final    bool   `json:"final"`
	Size     int64  `json:"size,omitempty"`
	SHA256   string `json:"sha256,omitempty"`
}

type ImportChunkResponse struct {
	Success bool   `json:"success"`
	Offset  int64  `json:"offset"`
	Backup  Info   `json:"backup,omitempty"`
	Error   string `json:"error,omitempty"`
}

// ImportAbortRequest removes the staging file of an import that will not be
// finished. Removing a missing staging file is not an error.
type ImportAbortRequest struct {
	ProtocolVersion int    `json:"protocol_version"`
	SubscriptionID  int    `json:"subscription_id"`
	DomainID        int    `json:"domain_id"`
	DomainName      string `json:"domain_name"`
	ImportID        string `json:"import_id"`
}
