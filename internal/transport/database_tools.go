package transport

import "github.com/alicelik/celikpanel/internal/backupspec"

// Database tools per database (alpha.128, G7): size and tables, an export
// download, an import with a safety copy first, a copy into a new database,
// and check/repair. The Panel resolves every identity from its own records
// (the domain's subscription, the database row, the PostgreSQL cluster port)
// and the Agent derives every path from those IDs; nothing here names a path.
//
// Veritabanı başına araçlar: boyut ve tablolar, dışa aktarma, önce güvenlik
// kopyası alınan içe aktarma, yeni veritabanına kopya, denetim/onarım.

// DBToolsTarget is one database of one domain.
type DBToolsTarget struct {
	SubscriptionID int                         `json:"subscription_id"`
	DomainID       int                         `json:"domain_id"`
	Domain         string                      `json:"domain"`
	Database       backupspec.DatabaseIdentity `json:"database"`
}

// DBToolsInfoResponse: what the database holds now. ImportBlocker names a
// server fact that stops an import before anything runs ("" when none).
type DBToolsInfoResponse struct {
	SizeBytes      int64                   `json:"size_bytes"`
	Tables         int                     `json:"tables"`
	Connections    int                     `json:"connections"`
	ClusterVersion string                  `json:"cluster_version,omitempty"`
	ClusterPort    int                     `json:"cluster_port,omitempty"`
	ImportBlocker  string                  `json:"import_blocker,omitempty"`
	Error          string                  `json:"error,omitempty"`
	Refusal        *DatabaseClusterRefusal `json:"refusal,omitempty"`
}

// DBToolsExportRequest makes one export file, named by ExportID (32 hex).
type DBToolsExportRequest struct {
	Target   DBToolsTarget `json:"target"`
	ExportID string        `json:"export_id"`
}

type DBToolsExportResponse struct {
	Size    int64                   `json:"size"`
	Error   string                  `json:"error,omitempty"`
	Refusal *DatabaseClusterRefusal `json:"refusal,omitempty"`
}

// DBToolsReadExportRequest reads one chunk of an export file; the file is
// removed once its last byte was read.
type DBToolsReadExportRequest struct {
	Target   DBToolsTarget `json:"target"`
	ExportID string        `json:"export_id"`
	Offset   int64         `json:"offset"`
	MaxBytes int           `json:"max_bytes"`
}

type DBToolsReadExportResponse struct {
	Data   []byte `json:"data"`
	Offset int64  `json:"offset"`
	Size   int64  `json:"size"`
	EOF    bool   `json:"eof"`
	Error  string `json:"error,omitempty"`
}

// DBToolsUploadChunkRequest appends one chunk of an import file.
type DBToolsUploadChunkRequest struct {
	Target   DBToolsTarget `json:"target"`
	ImportID string        `json:"import_id"`
	Offset   int64         `json:"offset"`
	Data     []byte        `json:"data"`
}

type DBToolsUploadChunkResponse struct {
	Size  int64  `json:"size"`
	Error string `json:"error,omitempty"`
}

// DBToolsSafetyRequest takes the safety copy before an import.
type DBToolsSafetyRequest struct {
	Target   DBToolsTarget `json:"target"`
	ImportID string        `json:"import_id"`
}

type DBToolsSafetyResponse struct {
	SafetyBackup string                  `json:"safety_backup,omitempty"`
	Error        string                  `json:"error,omitempty"`
	Refusal      *DatabaseClusterRefusal `json:"refusal,omitempty"`
}

// DBToolsImportRequest loads an uploaded file into the database as the
// database's own user (never as the engine's administrator): User and
// Password are that user's stored credential.
type DBToolsImportRequest struct {
	Target       DBToolsTarget `json:"target"`
	ImportID     string        `json:"import_id"`
	Size         int64         `json:"size"`
	SafetyBackup string        `json:"safety_backup"`
	EmptyFirst   bool          `json:"empty_first"`
	User         string        `json:"user"`
	Password     string        `json:"password"`
}

// Import refusal codes: the screen owns their words.
const (
	DBImportCodeAuth            = "auth"
	DBImportCodeClientUnsafe    = "client_unsafe"
	DBImportCodeClientCommand   = "client_command"
	DBImportCodeTransaction     = "transaction_control"
	DBImportCodeNotFound        = "upload_missing"
	DBImportCodeNoSiteAccount   = "no_site_account"
	DBImportCodeLoadFailed      = "load_failed"
	DBImportCodeRollbackFailed  = "rollback_failed"
	DBImportCodeSafetyRequired  = "safety_required"
	DBImportCodeUploadIncorrect = "upload_size"
)

type DBToolsImportResponse struct {
	Success bool `json:"success"`
	// RolledBack: the load stopped and the database is as the safety copy
	// was. For PostgreSQL the load is one transaction, so nothing changed.
	RolledBack bool                    `json:"rolled_back,omitempty"`
	Code       string                  `json:"code,omitempty"`
	Error      string                  `json:"error,omitempty"`
	Refusal    *DatabaseClusterRefusal `json:"refusal,omitempty"`
}

// DBToolsCopyRequest copies Source into a database the Panel has just
// created (Target, same engine and cluster).
type DBToolsCopyRequest struct {
	Source DBToolsTarget               `json:"source"`
	Target backupspec.DatabaseIdentity `json:"target"`
}

type DBToolsCopyResponse struct {
	Success bool                    `json:"success"`
	Error   string                  `json:"error,omitempty"`
	Refusal *DatabaseClusterRefusal `json:"refusal,omitempty"`
}

// DBToolsCheckRequest checks (Repair false) or repairs the tables.
type DBToolsCheckRequest struct {
	Target DBToolsTarget `json:"target"`
	Repair bool          `json:"repair"`
}

type DBToolsTableProblem struct {
	Table   string `json:"table"`
	Message string `json:"message"`
}

type DBToolsCheckResponse struct {
	Checked  int                   `json:"checked"`
	Problems []DBToolsTableProblem `json:"problems"`
	// SelfRepairing counts tables whose engine (InnoDB) has no repair step.
	SelfRepairing int                     `json:"self_repairing,omitempty"`
	Error         string                  `json:"error,omitempty"`
	Refusal       *DatabaseClusterRefusal `json:"refusal,omitempty"`
}
