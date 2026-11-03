package transport

// Runtime apps (alpha.119, docs/RUNTIME-APPS.md): a customer's own program
// (this release: an uploaded executable) run as a sandboxed systemd unit
// celikrt-<app id> behind the site's HTTPS. The Panel decides who may run
// what (D-042) and keeps the durable operation; the Agent derives every path
// and account from the identities below, never from a path the customer
// typed. Secret environment values travel only in RuntimeAppEnv, are written
// only to the root-owned environment file and never come back.
//
// Çalışma ortamı uygulamaları: müşterinin kendi programı, sitenin HTTPS'i
// arkasında korumalı bir systemd birimi olarak çalışır. Gizli değerler yalnız
// root'un ortam dosyasına yazılır ve hiçbir yanıtta geri dönmez.

// RuntimeAppTarget names one app by database identities.
type RuntimeAppTarget struct {
	AppID          int    `json:"app_id"`
	Kind           string `json:"kind"`
	SiteID         int    `json:"site_id"`
	SubscriptionID int    `json:"subscription_id"`
	DomainID       int    `json:"domain_id"`
	Domain         string `json:"domain"`
}

// RuntimeAppLimits are the offering's limits written into the unit.
type RuntimeAppLimits struct {
	MemoryMB   int `json:"memory_mb"`
	CPUPercent int `json:"cpu_percent"`
	TasksMax   int `json:"tasks_max"`
}

// RuntimeAppSpec is what the unit is rendered from.
type RuntimeAppSpec struct {
	Entry  string           `json:"entry"`
	Args   []string         `json:"args"`
	Port   int              `json:"port"`
	Limits RuntimeAppLimits `json:"limits"`
	// Level is the level the administrator accepted for the offering
	// ("full" or "reduced"); AcceptedGaps the named gaps accepted with a
	// reduced level. The Agent refuses a host whose gaps are not all among
	// them, so a reduced sandbox never passes silently.
	Level        string   `json:"level"`
	AcceptedGaps []string `json:"accepted_gaps,omitempty"`
}

// RuntimeAppCheckRequest checks an app before a deploy: the site account,
// the app folder, the entry (read without following any link) and the
// host's sandbox level. It changes nothing.
type RuntimeAppCheckRequest struct {
	Target RuntimeAppTarget `json:"target"`
	Spec   RuntimeAppSpec   `json:"spec"`
}

// RuntimeAppEntryCheck is the outcome of the entry check.
type RuntimeAppEntryCheck struct {
	// Code is "ok" or a refusal code (runtime.reason.<code>); Format
	// "elf" or "script" when ok.
	Code   string `json:"code"`
	Detail string `json:"detail,omitempty"`
	Format string `json:"format,omitempty"`
}

// RuntimeAppCheckResponse answers RuntimeAppCheckV1.
type RuntimeAppCheckResponse struct {
	RuntimeAppStepResponse
	Entry RuntimeAppEntryCheck `json:"entry"`
	Level string               `json:"level,omitempty"`
	Gaps  []string             `json:"gaps,omitempty"`
}

// Outcome classes of a failed step (D-024), the same words as the service
// apps': a verified failure, an unmet prerequisite, or an unknown result.
const (
	RuntimeAppVerifiedFailure = ServiceAppVerifiedFailure
	RuntimeAppPrerequisite    = ServiceAppPrerequisite
	RuntimeAppUnknown         = ServiceAppUnknown
)

// RuntimeAppStepResponse is the common answer of a step.
type RuntimeAppStepResponse struct {
	Done   bool   `json:"done"`
	Class  string `json:"class,omitempty"`
	Reason string `json:"reason,omitempty"`
	Error  string `json:"error,omitempty"`
}

// RuntimeAppRenderRequest prepares the app folder (as the site's account),
// writes the unit under the D-031 rule and reloads systemd. It starts
// nothing. Repeating it changes nothing more.
type RuntimeAppRenderRequest struct {
	ExpectedBuildCommit string           `json:"expected_build_commit"`
	OperationID         string           `json:"operation_id"`
	Target              RuntimeAppTarget `json:"target"`
	Spec                RuntimeAppSpec   `json:"spec"`
}

// RuntimeAppRenderResponse: UnitFile is the D-031 outcome; PendingPath names
// CelikPanel's text beside a unit the owner changed.
type RuntimeAppRenderResponse struct {
	RuntimeAppStepResponse
	UnitFile    string `json:"unit_file,omitempty"`
	PendingPath string `json:"pending_path,omitempty"`
}

// RuntimeAppControlRequest starts (enable + restart), stops (stop +
// disable) or restarts an app's unit.
type RuntimeAppControlRequest struct {
	ExpectedBuildCommit string           `json:"expected_build_commit"`
	Target              RuntimeAppTarget `json:"target"`
	Action              string           `json:"action"`
}

// RuntimeAppStatusRequest reads an app; it never changes anything. Proof
// asks for the sandbox proof of a running unit against Spec; HealthPath for
// one GET on its loopback port; Entry for the entry check.
type RuntimeAppStatusRequest struct {
	Target     RuntimeAppTarget `json:"target"`
	Spec       RuntimeAppSpec   `json:"spec"`
	Proof      bool             `json:"proof,omitempty"`
	HealthPath string           `json:"health_path,omitempty"`
	Entry      bool             `json:"entry,omitempty"`
}

// RuntimeAppHealth is one health read.
type RuntimeAppHealth struct {
	Checked bool   `json:"checked"`
	Status  int    `json:"status,omitempty"`
	OK      bool   `json:"ok"`
	Error   string `json:"error,omitempty"`
}

// RuntimeAppStatusResponse is what systemd, the kernel and the folder show.
type RuntimeAppStatusResponse struct {
	// UnitFile: absent, managed_unchanged, owner_edited, foreign,
	// unknown_origin or unreadable (D-031 states).
	UnitFile string `json:"unit_file"`
	// DropIns lists the owner's drop-in files (names only).
	DropIns     []string     `json:"drop_ins,omitempty"`
	Unit        AppUnitState `json:"unit"`
	ActiveState string       `json:"active_state,omitempty"`
	SubState    string       `json:"sub_state,omitempty"`
	Result      string       `json:"result,omitempty"`
	Restarts    int          `json:"restarts,omitempty"`
	// StartLimitHit: systemd gave up restarting (restart_loop).
	StartLimitHit bool  `json:"start_limit_hit,omitempty"`
	MemoryMB      int64 `json:"memory_mb,omitempty"`
	// OOMKills and PidsMax are the cgroup's memory.events oom_kill and
	// pids.events max counts (0 when the cgroup is gone).
	OOMKills   int64                 `json:"oom_kills,omitempty"`
	PidsMax    int64                 `json:"pids_max,omitempty"`
	Serving    bool                  `json:"serving"`
	Exposed    []string              `json:"exposed,omitempty"`
	Squatted   bool                  `json:"squatted,omitempty"`
	Health     RuntimeAppHealth      `json:"health"`
	Entry      *RuntimeAppEntryCheck `json:"entry_check,omitempty"`
	Level      string                `json:"level,omitempty"`
	Gaps       []string              `json:"gaps,omitempty"`
	Proof      *SandboxProof         `json:"sandbox_proof,omitempty"`
	EnvKeys    []string              `json:"env_keys,omitempty"`
	Error      string                `json:"error,omitempty"`
	Generation int                   `json:"sandbox_generation,omitempty"`
}

// RuntimeAppEnvRequest replaces the environment file (root's, 0600) and
// restarts a running app so it reads it. Entries without a value keep the
// secret value already on the server; keys not listed are removed.
type RuntimeAppEnvRequest struct {
	ExpectedBuildCommit string               `json:"expected_build_commit"`
	Target              RuntimeAppTarget     `json:"target"`
	Entries             []RuntimeAppEnvEntry `json:"entries"`
	Restart             bool                 `json:"restart,omitempty"`
	// PreviousKeys are the keys CelikPanel wrote last time; a key in the
	// file that is neither there nor in Entries was added by the owner and
	// is not dropped silently (D-022).
	PreviousKeys []string `json:"previous_keys,omitempty"`
}

// RuntimeAppEnvEntry is one environment value (a secret may come without
// one: the server's value is kept).
type RuntimeAppEnvEntry struct {
	Key    string `json:"key"`
	Value  string `json:"value,omitempty"`
	Secret bool   `json:"secret"`
}

// RuntimeAppLogsRequest reads the last Lines journal lines, redacted on the
// server with the secret values of the environment file.
type RuntimeAppLogsRequest struct {
	Target     RuntimeAppTarget `json:"target"`
	Lines      int              `json:"lines"`
	SecretKeys []string         `json:"secret_keys,omitempty"`
}

// RuntimeAppLogsResponse carries redacted lines.
type RuntimeAppLogsResponse struct {
	Lines []string `json:"lines"`
	Error string   `json:"error,omitempty"`
}

// RuntimeAppRemoveRequest removes the unit and the environment file. The app
// folder (the customer's upload and data) is removed only with RemoveFiles.
type RuntimeAppRemoveRequest struct {
	ExpectedBuildCommit string           `json:"expected_build_commit"`
	OperationID         string           `json:"operation_id"`
	Target              RuntimeAppTarget `json:"target"`
	RemoveFiles         bool             `json:"remove_files,omitempty"`
}

// RuntimeAppRemoveResponse: KeptCopy names the copy of a unit the owner had
// changed.
type RuntimeAppRemoveResponse struct {
	RuntimeAppStepResponse
	KeptCopy string `json:"kept_copy,omitempty"`
}

// RuntimeAppUnitStateRequest reads one app's unit state (suspension).
type RuntimeAppUnitStateRequest struct {
	AppID int `json:"app_id"`
}

// SetRuntimeAppSuspensionRequest stops and disables an app's unit, or
// restores the state recorded in Before (compare-and-set, D-022).
type SetRuntimeAppSuspensionRequest struct {
	ExpectedBuildCommit string       `json:"expected_build_commit"`
	AppID               int          `json:"app_id"`
	Suspend             bool         `json:"suspend"`
	Before              AppUnitState `json:"before"`
}
