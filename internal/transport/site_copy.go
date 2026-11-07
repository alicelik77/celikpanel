package transport

// Website copying (alpha.128, PLESK-GAP G8): one site's files, and optionally
// one of its MariaDB databases, copied into another site the same person may
// manage. The Panel checks who may copy where, the target's limits and makes
// the target's safety backup and new database first; the Agent copies.
//
// Site kopyalama: dosyalar ve isteğe bağlı bir MariaDB veritabanı.

// SiteCopyRequest copies Source into Target. Overwrite: the target's web
// folder may have content (the Panel made a backup of it first); without it
// a target with content is refused before anything is written.
// SourceDatabase/TargetDatabase: both empty, or the source database and the
// target's new, empty database.
type SiteCopyRequest struct {
	ExpectedBuildCommit string        `json:"expected_build_commit"`
	OperationID         string        `json:"operation_id"`
	Source              WordPressSite `json:"source"`
	Target              WordPressSite `json:"target"`
	Overwrite           bool          `json:"overwrite,omitempty"`
	SourceDatabase      string        `json:"source_database,omitempty"`
	TargetDatabase      string        `json:"target_database,omitempty"`
}

// SiteCopyResponse is the outcome. Code SiteCopyTargetNotEmpty: nothing was
// written. CompensationSafe: nothing was published, so the Panel may remove
// the new database again.
type SiteCopyResponse struct {
	Copied           bool   `json:"copied"`
	Code             string `json:"code,omitempty"`
	Error            string `json:"error,omitempty"`
	Detail           string `json:"detail,omitempty"`
	CompensationSafe bool   `json:"compensation_safe"`
}

// SiteCopyTargetNotEmpty: the target's web folder has content and no
// overwrite was asked for.
const SiteCopyTargetNotEmpty = "SITE_COPY_TARGET_NOT_EMPTY"
