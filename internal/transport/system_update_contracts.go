package transport

const (
	AgentCapabilitySystemUpdateV1        = "system_update_v1"
	AgentCapabilitySystemUpdateAbandonV1 = "system_update_abandon_v1"
)

// The base V1 capability is exposed only through Agent.CheckSystemUpdate,
// Agent.StartSystemUpdate, and Agent.SystemUpdateStatus. The independently
// negotiated abandon capability adds Agent.AbandonSystemUpdate. There is
// intentionally no generic download or command RPC.
//
// Security-sensitive integers stay canonical decimal strings so browser
// clients never round them through IEEE-754 numbers.
type SystemUpdateCheckResponse struct {
	Supported           bool   `json:"supported"`
	Available           bool   `json:"available"`
	CurrentVersion      string `json:"current_version"`
	CurrentCommit       string `json:"current_commit"`
	TargetVersion       string `json:"target_version,omitempty"`
	TargetCommit        string `json:"target_commit,omitempty"`
	TargetSequence      string `json:"target_sequence,omitempty"`
	TargetOS            string `json:"target_os,omitempty"`
	TargetArch          string `json:"target_arch,omitempty"`
	TargetArchiveSHA256 string `json:"target_archive_sha256,omitempty"`
	TargetArchiveSize   string `json:"target_archive_size,omitempty"`
	PublishedAt         string `json:"published_at,omitempty"`
	Error               string `json:"error,omitempty"`
}

// The request intentionally has no URL, path, command, argument, or
// environment field. Every value must match a freshly verified manifest.
type SystemUpdateStartRequest struct {
	RequestID              string `json:"request_id"`
	TargetVersion          string `json:"target_version"`
	TargetCommit           string `json:"target_commit"`
	TargetSequence         string `json:"target_sequence"`
	TargetOS               string `json:"target_os"`
	TargetArch             string `json:"target_arch"`
	TargetArchiveSHA256    string `json:"target_archive_sha256"`
	TargetArchiveSize      string `json:"target_archive_size"`
	ExpectedCurrentVersion string `json:"expected_current_version"`
	ExpectedCurrentCommit  string `json:"expected_current_commit"`
}

type SystemUpdateStartResponse struct {
	Accepted bool   `json:"accepted"`
	Status   string `json:"status,omitempty"`
	Error    string `json:"error,omitempty"`
	// Reason names a refused start's host cause with one of the stable
	// HostMutationReason* codes when the agent knows it (for example
	// package_manager_active), as ServiceMutationResponse.Reason does. Empty
	// for every other refusal and from an agent that predates it. Additive.
	// Reason, reddedilen başlatmanın bilinen ana makine nedenidir; eklemelidir.
	Reason string `json:"reason,omitempty"`
}

// Abandon carries the exact same immutable identity as Start. The agent uses
// it to publish a durable negative receipt before the browser may conclude
// that an authorized start was never accepted.
type SystemUpdateAbandonRequest = SystemUpdateStartRequest

type SystemUpdateStatusRequest struct {
	RequestID string `json:"request_id"`
}

type SystemUpdateStatusResponse struct {
	Found               bool   `json:"found"`
	RequestID           string `json:"request_id,omitempty"`
	Status              string `json:"status,omitempty"`
	TargetVersion       string `json:"target_version,omitempty"`
	TargetCommit        string `json:"target_commit,omitempty"`
	TargetSequence      string `json:"target_sequence,omitempty"`
	TargetOS            string `json:"target_os,omitempty"`
	TargetArch          string `json:"target_arch,omitempty"`
	TargetArchiveSHA256 string `json:"target_archive_sha256,omitempty"`
	TargetArchiveSize   string `json:"target_archive_size,omitempty"`
	CreatedAt           string `json:"created_at,omitempty"`
	UpdatedAt           string `json:"updated_at,omitempty"`
	Error               string `json:"error,omitempty"`
	// Additive (2026-10-16): where the reviewed updater is, read by the Agent
	// from what the updater already writes (cmd/agent
	// system_update_progress_linux.go). Best effort and never an input to
	// admission, recovery or the status above; empty when it was not read.
	// DownloadedBytes is the size of the archive the installer is writing,
	// DownloadedAt the moment it reached the signed size, and VerifiedAt the
	// moment the installer published this request's release floor, which it
	// does only after the signature, the archive's size and SHA-256 and the
	// extracted files' checksums were verified. Times are RFC 3339 UTC on the
	// server's clock.
	// Eklemeli: guncelleyicinin yazdiklarindan okunan ilerleme; karar girdisi degildir.
	DownloadedBytes string `json:"downloaded_bytes,omitempty"`
	DownloadedAt    string `json:"downloaded_at,omitempty"`
	VerifiedAt      string `json:"verified_at,omitempty"`
}
