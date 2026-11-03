package transport

// Site tools (alpha.118): WordPress tools and malware scanning of hosted
// sites. Every request carries immutable row identities; the Agent derives
// paths from them and never trusts a path from the Panel.
//
// Site araçları: WordPress araçları ve barındırılan sitelerin zararlı yazılım
// taraması. İstekler değişmez kimlik taşır; yolları Agent kendisi türetir.

// WordPressSite is the identity of one WordPress site. Username is the
// site's own system account; PHPVersion is the site's PHP line ("8.3"), used
// to pick the PHP command the panel-shipped scripts run with.
type WordPressSite struct {
	SiteID         int    `json:"site_id"`
	SubscriptionID int    `json:"subscription_id"`
	DomainID       int    `json:"domain_id"`
	Domain         string `json:"domain"`
	Username       string `json:"username"`
	PHPVersion     string `json:"php_version"`
}

// WordPress tools answer codes (the Panel maps them to guidance).
const (
	WordPressCodeNotDetected      = "WORDPRESS_NOT_DETECTED"
	WordPressCodePHPMissing       = "WORDPRESS_PHP_CLI_MISSING"
	WordPressCodeUpdateSource     = "WORDPRESS_UPDATE_SOURCE_UNAVAILABLE"
	WordPressCodeNotVerifiable    = "WORDPRESS_UPDATE_NOT_VERIFIABLE"
	WordPressCodeVerifyFailed     = "WORDPRESS_UPDATE_VERIFY_FAILED"
	WordPressCodeNoUpdate         = "WORDPRESS_NO_UPDATE"
	WordPressCodeHealthFailed     = "WORDPRESS_HEALTH_FAILED"
	WordPressCodeTargetNotEmpty   = "WORDPRESS_CLONE_TARGET_NOT_EMPTY"
	WordPressCodeDatabaseMismatch = "WORDPRESS_DATABASE_MISMATCH"
)

type WordPressStatusRequest struct {
	Site WordPressSite `json:"site"`
	// CheckUpdates asks wordpress.org's official API which updates exist.
	CheckUpdates bool `json:"check_updates"`
}

// WordPressItem is one plugin or theme as the site's files describe it.
type WordPressItem struct {
	Kind    string `json:"kind"` // plugin | theme
	Slug    string `json:"slug"` // directory name (or single-file plugin basename)
	File    string `json:"file"` // plugin main file relative to the plugins folder
	Name    string `json:"name"`
	Version string `json:"version"`
	// UpdateVersion is the newer version wordpress.org offers ("" = none).
	UpdateVersion string `json:"update_version,omitempty"`
	// Source: "wordpress.org" when wordpress.org knows the item, else "other".
	Source string `json:"source,omitempty"`
	// Verifiable: CelikPanel can verify the update package before using it.
	Verifiable bool `json:"verifiable"`
}

type WordPressStatusResponse struct {
	Detected    bool   `json:"detected"`
	CoreVersion string `json:"core_version,omitempty"`
	// CoreUpdate is the newest release; CoreMinor the newest release of the
	// installed branch (a minor or security release), "" when none.
	CoreUpdate string          `json:"core_update,omitempty"`
	CoreMinor  string          `json:"core_minor,omitempty"`
	Plugins    []WordPressItem `json:"plugins"`
	Themes     []WordPressItem `json:"themes"`
	// DatabaseName and TablePrefix come from wp-config.php (read only).
	DatabaseName string `json:"database_name,omitempty"`
	TablePrefix  string `json:"table_prefix,omitempty"`
	// UpdatesChecked is false when wordpress.org could not be asked.
	UpdatesChecked bool   `json:"updates_checked"`
	UpdatesError   string `json:"updates_error,omitempty"`
	Code           string `json:"code,omitempty"`
	Error          string `json:"error,omitempty"`
}

type WordPressUpdateRequest struct {
	ExpectedBuildCommit string        `json:"expected_build_commit"`
	OperationID         string        `json:"operation_id"`
	Site                WordPressSite `json:"site"`
	Kind                string        `json:"kind"` // core | plugin | theme
	// Items: plugin files ("akismet/akismet.php") or theme slugs.
	Items []string `json:"items,omitempty"`
	// CoreVersion pins the core release the person saw; MinorOnly refuses
	// anything outside the installed branch (the scheduled switch).
	CoreVersion string `json:"core_version,omitempty"`
	MinorOnly   bool   `json:"minor_only,omitempty"`
}

type WordPressUpdateResponse struct {
	// Changed: the site's files were touched; the Panel then restores the
	// backup taken first when the update did not succeed.
	Changed bool     `json:"changed"`
	Updated []string `json:"updated,omitempty"`
	// RolledBack: the Agent itself put the previous files back.
	RolledBack bool   `json:"rolled_back,omitempty"`
	Code       string `json:"code,omitempty"`
	Detail     string `json:"detail,omitempty"`
	Error      string `json:"error,omitempty"`
}

type WordPressCloneRequest struct {
	ExpectedBuildCommit string        `json:"expected_build_commit"`
	OperationID         string        `json:"operation_id"`
	Source              WordPressSite `json:"source"`
	Target              WordPressSite `json:"target"`
	SourceDatabase      string        `json:"source_database"`
	TargetDatabase      string        `json:"target_database"`
	TargetDBUser        string        `json:"target_db_user"`
	TargetDBPass        string        `json:"target_db_pass"`
}

type WordPressCloneResponse struct {
	Cloned bool `json:"cloned"`
	// CompensationSafe: nothing was published on the target; the Panel may
	// remove the database it created for this clone.
	CompensationSafe bool   `json:"compensation_safe"`
	ReplacedValues   int    `json:"replaced_values"`
	SkippedValues    int    `json:"skipped_values"`
	Code             string `json:"code,omitempty"`
	Detail           string `json:"detail,omitempty"`
	Error            string `json:"error,omitempty"`
}

// Malware scan states of the scanner itself (prerequisites).
const (
	MalwareScannerReady          = "ready"
	MalwareScannerNotInstalled   = "not_installed"
	MalwareScannerNoSignatures   = "no_signatures"
	MalwareScannerSignaturesOld  = "signatures_old"
	MalwareCodeQuarantineExists  = "MALWARE_RESTORE_PATH_TAKEN"
	MalwareCodeFileChanged       = "MALWARE_FILE_CHANGED"
	MalwareCodeQuarantineMissing = "MALWARE_QUARANTINE_MISSING"
)

type MalwareScannerStatusRequest struct{}

type MalwareScannerStatusResponse struct {
	State string `json:"state"`
	// SignaturesAgeHours: how old the newest daily signature database is.
	SignaturesAgeHours int    `json:"signatures_age_hours,omitempty"`
	Scanner            string `json:"scanner,omitempty"` // clamdscan | clamscan
	Error              string `json:"error,omitempty"`
}

type MalwareScanSiteRequest struct {
	SubscriptionID int `json:"subscription_id"`
	DomainID       int `json:"domain_id"`
}

type MalwareFinding struct {
	Path      string `json:"path"` // relative to the site's web folder
	Signature string `json:"signature"`
}

type MalwareScanSiteResponse struct {
	State        string           `json:"state"` // scanner state; "ready" when the scan ran
	FilesScanned int              `json:"files_scanned"`
	Findings     []MalwareFinding `json:"findings"`
	Truncated    bool             `json:"truncated,omitempty"`
	Error        string           `json:"error,omitempty"`
}

type MalwareQuarantineRequest struct {
	ExpectedBuildCommit string `json:"expected_build_commit"`
	SubscriptionID      int    `json:"subscription_id"`
	DomainID            int    `json:"domain_id"`
	Path                string `json:"path"`
	Signature           string `json:"signature"`
}

type MalwareQuarantineResponse struct {
	QuarantineID string `json:"quarantine_id,omitempty"`
	Code         string `json:"code,omitempty"`
	Error        string `json:"error,omitempty"`
}

// MalwareQuarantineItemRequest names one quarantined file (restore or
// delete). DomainID is checked against the item's own record.
type MalwareQuarantineItemRequest struct {
	ExpectedBuildCommit string `json:"expected_build_commit"`
	SubscriptionID      int    `json:"subscription_id"`
	DomainID            int    `json:"domain_id"`
	QuarantineID        string `json:"quarantine_id"`
}

type MalwareQuarantineItemResponse struct {
	Done  bool   `json:"done"`
	Path  string `json:"path,omitempty"`
	Code  string `json:"code,omitempty"`
	Error string `json:"error,omitempty"`
}
