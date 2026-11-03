package transport

// Application catalog RPCs (alpha.118, docs/APP-CATALOG.md). The Panel names
// an application by its catalog ID only; the Agent takes the pinned address,
// digest and recipe from its own compiled catalog.
//
// Uygulama kataloğu RPC'leri: Panel uygulamayı yalnız katalog kimliğiyle
// adlandırır; adres, özet ve tarif Agent'ın kendi kataloğundandır.

// AppSiteIdentity is the immutable identity of the site an application lives
// on; the Agent derives every path from it.
type AppSiteIdentity struct {
	SiteID         int    `json:"site_id"`
	SubscriptionID int    `json:"subscription_id"`
	DomainID       int    `json:"domain_id"`
	Domain         string `json:"domain"`
	Username       string `json:"username"`
	PHPVersion     string `json:"php_version"`
}

// AppMissingExtension is one PHP extension the site's PHP lacks.
type AppMissingExtension struct {
	Extension string `json:"extension"`
	Package   string `json:"package,omitempty"`
	Enable    bool   `json:"enable,omitempty"`
}

// Document-root states of AppPrerequisitesResponse.
const (
	AppDocRootEmpty       = "empty"
	AppDocRootPlaceholder = "placeholder"
	AppDocRootContent     = "content"
	AppDocRootUnavailable = "unavailable"
)

type AppPrerequisitesRequest struct {
	AppID string          `json:"app_id"`
	Site  AppSiteIdentity `json:"site"`
}

// AppPrerequisitesResponse is read-only: nothing on the server changes.
type AppPrerequisitesResponse struct {
	Family    string                `json:"family"`
	PHPLine   string                `json:"php_line"`
	PHPOK     bool                  `json:"php_ok"`
	PHPReason string                `json:"php_reason,omitempty"`
	Missing   []AppMissingExtension `json:"missing,omitempty"`
	// ExtensionsUnknown: the site's PHP could not be asked; Missing is empty
	// because nothing is known, not because nothing is missing.
	ExtensionsUnknown bool   `json:"extensions_unknown,omitempty"`
	DocRoot           string `json:"doc_root"`
	// FreeDiskMB is the free room on the hosting file system (0 unknown).
	FreeDiskMB int64  `json:"free_disk_mb"`
	Error      string `json:"error,omitempty"`
}

// Failure kinds of the catalog RPCs, for the screen's guidance.
const (
	AppFailVerification = "verification" // the download did not match the pinned digest
	AppFailUnpack       = "unpack"       // the verified release could not be unpacked safely (nothing changed)
	AppFailDownload     = "download"     // the release could not be downloaded
	AppFailPrerequisite = "prerequisite" // something the administrator or owner must change first
	AppFailContent      = "content"      // the document root holds content
	AppFailPublish      = "publish"      // the files could not be put in place
	AppFailSetup        = "setup"        // the application's own installer failed
	AppFailNginx        = "nginx"        // nginx refused or kept a file
	AppFailRemove       = "remove"       // a removal stopped part way; running it again continues it
	AppFailInternal     = "internal"
)

type InstallCatalogAppRequest struct {
	ExpectedBuildCommit string          `json:"expected_build_commit"`
	OperationID         string          `json:"operation_id"`
	AppID               string          `json:"app_id"`
	Version             string          `json:"version"`
	Site                AppSiteIdentity `json:"site"`
}

type InstallCatalogAppResponse struct {
	Installed bool `json:"installed"`
	// CompensationSafe: the document root was not changed, so the Panel may
	// remove the database it created for this operation.
	CompensationSafe bool   `json:"compensation_safe"`
	Detail           string `json:"detail,omitempty"`
	Error            string `json:"error,omitempty"`
	ErrorKind        string `json:"error_kind,omitempty"`
}

type SetupCatalogAppRequest struct {
	ExpectedBuildCommit string          `json:"expected_build_commit"`
	OperationID         string          `json:"operation_id"`
	AppID               string          `json:"app_id"`
	Site                AppSiteIdentity `json:"site"`
	HTTPS               bool            `json:"https"`
	DBName              string          `json:"db_name"`
	DBUser              string          `json:"db_user"`
	DBPass              string          `json:"db_pass"`
	DBHost              string          `json:"db_host"`
	AdminPass           string          `json:"admin_pass"`
	AdminEmail          string          `json:"admin_email"`
}

type SetupCatalogAppResponse struct {
	Configured bool `json:"configured"`
	// AlreadyConfigured: an earlier attempt had finished the application's
	// own setup; this one only finished CelikPanel's part.
	AlreadyConfigured bool   `json:"already_configured,omitempty"`
	AdminPath         string `json:"admin_path,omitempty"`
	// RulesKept: the owner changed the application's nginx file; it was kept.
	RulesKept bool   `json:"rules_kept,omitempty"`
	Detail    string `json:"detail,omitempty"`
	Error     string `json:"error,omitempty"`
	ErrorKind string `json:"error_kind,omitempty"`
	// Output is the end of the installer's output with every secret removed.
	Output string `json:"output,omitempty"`
}

type RemoveCatalogAppRequest struct {
	ExpectedBuildCommit string          `json:"expected_build_commit"`
	AppID               string          `json:"app_id"`
	Site                AppSiteIdentity `json:"site"`
	// RemoveData also removes the application's private data folder
	// (Nextcloud's files); only with the owner's explicit confirmation.
	RemoveData bool `json:"remove_data"`
}

type RemoveCatalogAppResponse struct {
	FilesRemoved bool   `json:"files_removed"`
	RulesRemoved bool   `json:"rules_removed"`
	RulesKept    bool   `json:"rules_kept,omitempty"`
	DataRemoved  bool   `json:"data_removed,omitempty"`
	DataKept     string `json:"data_kept,omitempty"`
	NotThisApp   bool   `json:"not_this_app,omitempty"`
	Detail       string `json:"detail,omitempty"`
	Error        string `json:"error,omitempty"`
	ErrorKind    string `json:"error_kind,omitempty"`
}
