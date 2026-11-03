package transport

// Service apps (alpha.118, docs/AI-APPS.md): AI and automation applications a
// site runs as a long-running, sandboxed systemd unit (celiksvc-<install>).
// The Panel decides who may install what (D-042) and keeps the durable
// operation; the Agent derives every path and account from the identities
// below, never from text the customer typed. Secrets travel only in the
// Prepare and Secrets requests, are written only to the application's own
// environment file and are never echoed back.
//
// Hizmet uygulamaları: sitenin korumalı bir systemd birimi olarak çalıştırdığı
// yapay zekâ ve otomasyon uygulamaları. Gizli değerler yalnız uygulamanın kendi
// ortam dosyasına yazılır ve hiçbir yanıtta geri dönmez.

// ServiceAppAvailability says whether one catalog app can run on this server.
type ServiceAppAvailability struct {
	AppID          string `json:"app_id"`
	Available      bool   `json:"available"`
	Reason         string `json:"reason,omitempty"`
	Detail         string `json:"detail,omitempty"`
	RuntimeVersion string `json:"runtime_version,omitempty"`
	RuntimeSource  string `json:"runtime_source,omitempty"`
}

// ServiceAppProbeResponse lists every catalog app.
type ServiceAppProbeResponse struct {
	Apps  []ServiceAppAvailability `json:"apps"`
	Error string                   `json:"error,omitempty"`
}

// ServiceAppTarget names one install by database identities.
type ServiceAppTarget struct {
	InstallID      int    `json:"install_id"`
	SiteID         int    `json:"site_id"`
	SubscriptionID int    `json:"subscription_id"`
	DomainID       int    `json:"domain_id"`
	Domain         string `json:"domain"`
	AppID          string `json:"app_id"`
}

// ServiceAppLimits are the resource limits written into the unit.
type ServiceAppLimits struct {
	MemoryMB   int `json:"memory_mb"`
	CPUPercent int `json:"cpu_percent"`
	TasksMax   int `json:"tasks_max"`
}

// ServiceAppPrepareRequest prepares an install or an update: folders, the
// verified upstream file, the environment file, then the transient install
// unit (npm or pip as the site's user). It returns as soon as that unit has
// started. Repeating it with the same OperationID changes nothing more.
type ServiceAppPrepareRequest struct {
	ExpectedBuildCommit string            `json:"expected_build_commit"`
	OperationID         string            `json:"operation_id"`
	Target              ServiceAppTarget  `json:"target"`
	Version             string            `json:"version"`
	Port                int               `json:"port"`
	AuxPort             int               `json:"aux_port"`
	Limits              ServiceAppLimits  `json:"limits"`
	OwnerEmail          string            `json:"owner_email,omitempty"`
	OwnerSecret         string            `json:"owner_secret,omitempty"`
	Secrets             map[string]string `json:"secrets,omitempty"`
}

// Outcome classes of a failed step (D-024): a verified failure, an unmet
// prerequisite someone must fix first, or an unknown result.
const (
	ServiceAppVerifiedFailure = "verified_failure"
	ServiceAppPrerequisite    = "prerequisite"
	ServiceAppUnknown         = "unknown"
)

// ServiceAppStepResponse is the common answer of a step.
type ServiceAppStepResponse struct {
	Done bool `json:"done"`
	// Class and Reason say why it is not done (Reason is a code the screen
	// words: svcapps.reason.<code>); Error is the server's own words,
	// already redacted.
	Class  string `json:"class,omitempty"`
	Reason string `json:"reason,omitempty"`
	Error  string `json:"error,omitempty"`
}

// ServiceAppStatusRequest reads an install; it never changes anything.
type ServiceAppStatusRequest struct {
	Target      ServiceAppTarget `json:"target"`
	Version     string           `json:"version"`
	Port        int              `json:"port"`
	OperationID string           `json:"operation_id,omitempty"`
}

// ServiceAppStatusResponse is what systemd and the folder show.
type ServiceAppStatusResponse struct {
	// Install is the transient install unit: none, running, succeeded or
	// failed; InstallOperation is the operation that started it.
	Install          string `json:"install"`
	InstallOperation string `json:"install_operation,omitempty"`
	// Installed: the application of Version is present in its folder.
	Installed bool `json:"installed"`
	// UnitFile: absent, ours, edited (the owner changed it) or foreign.
	UnitFile    string       `json:"unit_file"`
	Unit        AppUnitState `json:"unit"`
	ActiveState string       `json:"active_state,omitempty"`
	Result      string       `json:"result,omitempty"`
	MemoryMB    int64        `json:"memory_mb,omitempty"`
	Serving     bool         `json:"serving"`
	Exposed     []string     `json:"exposed,omitempty"`
	Squatted    bool         `json:"squatted,omitempty"`
	Error       string       `json:"error,omitempty"`
}

// ServiceAppActivateRequest writes the long-running unit for Version, starts
// it (unless KeepStopped) and proves it listens on 127.0.0.1:Port only.
type ServiceAppActivateRequest struct {
	ExpectedBuildCommit string           `json:"expected_build_commit"`
	OperationID         string           `json:"operation_id"`
	Target              ServiceAppTarget `json:"target"`
	Version             string           `json:"version"`
	Port                int              `json:"port"`
	AuxPort             int              `json:"aux_port"`
	Limits              ServiceAppLimits `json:"limits"`
	KeepStopped         bool             `json:"keep_stopped,omitempty"`
}

// ServiceAppControlRequest starts, stops or restarts an install.
type ServiceAppControlRequest struct {
	ExpectedBuildCommit string           `json:"expected_build_commit"`
	Target              ServiceAppTarget `json:"target"`
	Action              string           `json:"action"`
}

// ServiceAppLogsRequest reads the last Lines journal lines (redacted) of the
// unit, or of the install unit when Install is set.
type ServiceAppLogsRequest struct {
	Target  ServiceAppTarget `json:"target"`
	Lines   int              `json:"lines"`
	Install bool             `json:"install,omitempty"`
}

// ServiceAppLogsResponse carries redacted lines.
type ServiceAppLogsResponse struct {
	Lines []string `json:"lines"`
	Error string   `json:"error,omitempty"`
}

// ServiceAppSecretsRequest replaces the customer's values (and, for the
// owner password, the owner's secret) in the environment file and restarts
// the app if it runs. Clear removes keys.
type ServiceAppSecretsRequest struct {
	ExpectedBuildCommit string            `json:"expected_build_commit"`
	Target              ServiceAppTarget  `json:"target"`
	Port                int               `json:"port"`
	AuxPort             int               `json:"aux_port"`
	OwnerEmail          string            `json:"owner_email,omitempty"`
	OwnerSecret         string            `json:"owner_secret,omitempty"`
	Secrets             map[string]string `json:"secrets,omitempty"`
	Clear               []string          `json:"clear,omitempty"`
}

// ServiceAppRemoveRequest removes the unit and the application's files. The
// data folder is removed only with RemoveData.
type ServiceAppRemoveRequest struct {
	ExpectedBuildCommit string           `json:"expected_build_commit"`
	OperationID         string           `json:"operation_id"`
	Target              ServiceAppTarget `json:"target"`
	RemoveData          bool             `json:"remove_data,omitempty"`
}

// ServiceAppRemoveResponse: KeptCopy names the copy of a unit file the owner
// had changed (it is kept beside the removed one); KeptEnvCopy the same for
// the environment file (root's, 0600, in the environment folder).
type ServiceAppRemoveResponse struct {
	ServiceAppStepResponse
	KeptCopy    string `json:"kept_copy,omitempty"`
	KeptEnvCopy string `json:"kept_env_copy,omitempty"`
}

// ServiceAppUnitStateRequest reads one install's unit state (suspension).
type ServiceAppUnitStateRequest struct {
	InstallID int `json:"install_id"`
}

// ServiceAppUnitStateResponse is the unit's state.
type ServiceAppUnitStateResponse struct {
	State AppUnitState `json:"state"`
	Error string       `json:"error,omitempty"`
}

// SetServiceAppSuspensionRequest stops and disables an install's unit, or
// restores the state recorded in Before (compare-and-set, D-022).
type SetServiceAppSuspensionRequest struct {
	ExpectedBuildCommit string       `json:"expected_build_commit"`
	InstallID           int          `json:"install_id"`
	Suspend             bool         `json:"suspend"`
	Before              AppUnitState `json:"before"`
}
