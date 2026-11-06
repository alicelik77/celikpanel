package transport

// The DNS check tool on a backup DNS server (6 Nov 2026). The paired main
// server checks a deletion with a small read-only program installed on the
// backup (/usr/local/libexec/celikpanel-<engine>-peer-inspect). It is
// CelikPanel's own helper: the Agent puts the running release's copy in
// place when it starts after the owner's update, but only over a file
// CelikPanel shipped or a missing one, never over an owner's own file. The
// status names that one case and the command the owner would run.
//
// Yedek DNS sunucusundaki denetim aracı: güncellemeden sonra Agent, yalnız
// CelikPanel'in dağıttığı bir sürümün ya da eksik dosyanın yerine bu sürümün
// kopyasını koyar; sahibin dosyasına dokunulmaz.

const (
	// The channel for this engine is not set up on this server.
	DNSPeerInspectorToolNotEnrolled = "not_enrolled"
	// The installed program is the one this release ships.
	DNSPeerInspectorToolCurrent = "current"
	// The installed program is an earlier CelikPanel release's.
	DNSPeerInspectorToolOutdated = "outdated"
	// The channel is set up but the program file is missing.
	DNSPeerInspectorToolMissing = "missing"
	// The installed file is no program CelikPanel shipped: changed by hand.
	DNSPeerInspectorToolOwnerChanged = "owner_changed"
	// This release's copy of the program cannot be found on this server.
	DNSPeerInspectorToolReleaseCopyUnavailable = "release_copy_unavailable"
	// The state could not be read safely.
	DNSPeerInspectorToolUnknown = "unknown"
)

// DNSPeerInspectorTool is one engine's check tool on this server.
type DNSPeerInspectorTool struct {
	Engine          string `json:"engine"`
	State           string `json:"state"`
	InstalledPath   string `json:"installed_path"`
	InstalledSHA256 string `json:"installed_sha256,omitempty"`
	ReleasePath     string `json:"release_path,omitempty"`
	ReleaseSHA256   string `json:"release_sha256,omitempty"`
	// Command is the one line an owner who changed the file runs to use
	// this release's program instead (owner_changed only).
	Command string `json:"command,omitempty"`
}

type DNSPeerInspectorToolStatusRequest struct{}

type DNSPeerInspectorToolStatusResponse struct {
	Tools []DNSPeerInspectorTool `json:"tools"`
}
