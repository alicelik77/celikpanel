// Package backupspec defines the versioned RPC contract shared by the
// unprivileged panel and the privileged agent.
package backupspec

import "time"

const (
	ProtocolVersion = 2
	MaxChunkBytes   = 1 << 20
	MaxJobKeyBytes  = 128
)

// Backup package (manifest) versions. ManifestVersionSite is the package
// every release since v2 writes and reads: public_html and databases.
// ManifestVersionHome (docs/SITE-BACKUP-CONTENTS-DESIGN.md) adds the site-home
// areas (a program's or Python app's "app" folder, service apps' "apps/<app>"
// folders) in payload/home.tar.gz. It is written only when such an area
// exists, so a site without one keeps the version-2 package byte for byte. An
// older release refuses a version-3 manifest (fail closed): it neither lists,
// restores nor imports it. The RPC protocol stays ProtocolVersion.
//
// Yedek paketi sürümleri: 3 yalnız site ev dizininde program/uygulama alanı
// varsa yazılır; eski sürüm 3'ü reddeder (kapalı kalır).
const (
	ManifestVersionSite = 2
	ManifestVersionHome = 3
)

// Site-home areas and the reasons an item was left out of one.
const (
	// AreaApp is a program's or Python app's folder, <site home>/app.
	AreaApp = "app"
	// AreaAppsPrefix starts a service app's area, "apps/<app>".
	AreaAppsPrefix = "apps/"

	LeftOutCache        = "cache"
	LeftOutBuildTemp    = "build_temp"
	LeftOutReinstalled  = "reinstalled"
	LeftOutSpecial      = "special"
	LeftOutNotSiteOwned = "not_site_owned"
	LeftOutNotAFolder   = "not_a_folder"
	// MaxLeftOutNamed bounds the left-out items named one by one; the rest
	// are counted (LeftOutMore).
	MaxLeftOutNamed = 100
)

// Area is one site-home area a backup holds (or would hold): its name and the
// bytes and number of files kept in it, uncompressed.
type Area struct {
	Name  string `json:"name"`
	Bytes int64  `json:"bytes"`
	Files int64  `json:"files"`
}

// LeftOut is one item of an area that a backup does not hold, and why.
// Bytes is the size of the files under it (0 for a special file).
type LeftOut struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
	Bytes  int64  `json:"bytes,omitempty"`
}

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
	// Format is the package's manifest version (0 for a pre-v2 archive).
	// Areas and LeftOut are what a version-3 package holds besides
	// public_html and what it does not; LeftOutMore counts the left-out
	// items beyond the named ones. Additive.
	Format      int       `json:"format,omitempty"`
	Areas       []Area    `json:"areas,omitempty"`
	LeftOut     []LeftOut `json:"left_out,omitempty"`
	LeftOutMore int       `json:"left_out_more,omitempty"`
}

// ContentsResponse is what a files backup of the site would hold now,
// measured with the walk and rules that make the archive (so the sizes shown
// are the sizes archived): public_html's files, each site-home area, what an
// area leaves out, and the free space on the backup disk (-1 when it could not
// be read). Complete is false when the measurement stopped at its time limit:
// the sizes are then "at least". Error is a measurement that failed.
// ContentsResponse, sitenin bir dosya yedeğinin şu an neleri içereceğidir.
type ContentsResponse struct {
	SiteBytes       int64     `json:"site_bytes"`
	SiteFiles       int64     `json:"site_files"`
	Areas           []Area    `json:"areas"`
	LeftOut         []LeftOut `json:"left_out"`
	LeftOutMore     int       `json:"left_out_more,omitempty"`
	LeftOutBytes    int64     `json:"left_out_bytes"`
	BackupFreeBytes int64     `json:"backup_free_bytes"`
	Complete        bool      `json:"complete"`
	Error           string    `json:"error,omitempty"`
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
	// Code (alpha.134, additive): a refusal the screen can name
	// (RestoreCodeNotFound, RestoreCodeOtherSite, RestoreCodeInvalid); empty
	// for an error of the server itself.
	Code string `json:"code,omitempty"`
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
	// Code and Detail (alpha.128): a restore that stopped says how
	// (RestoreCode*) and the engine's own reason, bounded, so the screen
	// can name what happened and what to do instead of a generic error.
	Code   string `json:"code,omitempty"`
	Detail string `json:"detail,omitempty"`
	// Restored names the site-home areas this restore replaced, and
	// NotRestored the areas the backup holds that this release does not put
	// back (service apps: left as they are on the server). Additive.
	Restored    []string `json:"restored,omitempty"`
	NotRestored []string `json:"not_restored,omitempty"`
}

// Restore failure codes (alpha.128). Additive.
const (
	// RestoreCodeSafetyFailed: the safety backup could not be made; nothing
	// was written.
	RestoreCodeSafetyFailed = "safety_failed"
	// RestoreCodeLoadReverted: loading stopped and the safety backup was
	// put back; the site is as it was.
	RestoreCodeLoadReverted = "load_reverted"
	// RestoreCodeRollbackFailed: loading stopped and putting the safety
	// backup back did not finish either; the safety backup is kept.
	RestoreCodeRollbackFailed = "rollback_failed"
	// RestoreCodeInvalid: the backup failed its checks before anything was
	// written.
	RestoreCodeInvalid = "invalid_backup"
	// RestoreCodeFinishFailed: the restore was published but its final
	// cleanup or durability proof did not finish.
	RestoreCodeFinishFailed = "finish_failed"
	// alpha.134 lab findings (D-024: a refusal says what happened, not
	// "internal error"); also InspectResponse.Code. Nothing was replaced.
	// RestoreCodeNotFound: the backup is not (or no longer) among this
	// site's backups on the server.
	RestoreCodeNotFound = "not_found"
	// RestoreCodeOtherSite: the package was made of another site.
	RestoreCodeOtherSite = "other_site"
	// RestoreCodeNoSpace: the server's disk had no room to unpack or stage
	// the backup.
	RestoreCodeNoSpace = "no_space"
	// RestoreCodeSiteAccount: the site's own system account, which the
	// program's files are given to, could not be found or proved.
	RestoreCodeSiteAccount = "site_account"
)

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
