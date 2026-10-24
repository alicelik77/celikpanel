package transport

const defaultWebmailSocketPath = "/run/celikpanel-webmail.sock"

// WebmailSocketPath is the authenticated local boundary between the panel
// proxy and nginx's Roundcube vhost. The production path lives directly below
// root-owned /run: an unprivileged site user cannot create, replace or remove
// its socket node and impersonate the webmail upstream.
//
// This boundary is intentionally not configurable: both the panel and the
// privileged agent must agree on a path directly below root-owned /run. Tests
// that need a temporary socket inject that path into their local helper rather
// than changing the production boundary through process environment.
func WebmailSocketPath() string {
	return defaultWebmailSocketPath
}

// RoundcubeUpdateCheckResponse is the webmail update check (alpha.107, D-038
// §9): the version the installed tree says it is, the newest stable release
// on the same line from Roundcube's own releases, and the newest line there
// is. Roundcube is not installed by the package manager, so the operating
// system never updates it; the panel offers its update and never applies it
// by itself. CheckedAt is when the release list was read (RFC 3339).
// RoundcubeUpdateCheckResponse: web postası güncelleme denetimi.
type RoundcubeUpdateCheckResponse struct {
	Installed       bool   `json:"installed"`
	Version         string `json:"version,omitempty"`
	Line            string `json:"line,omitempty"`
	LatestOnLine    string `json:"latest_on_line,omitempty"`
	LatestOverall   string `json:"latest_overall,omitempty"`
	UpdateAvailable bool   `json:"update_available"`
	CheckedAt       string `json:"checked_at,omitempty"`
	Error           string `json:"error,omitempty"`
}

// RoundcubeUpdateRequest names the release to update to; the Agent checks it
// is the newest stable release on the installed line again.
type RoundcubeUpdateRequest struct {
	ServiceMutationBinding
	Version string `json:"version"`
}

// RoundcubeUpdateResponse: Updated and Proved (webmail answered through its
// socket on the new version), or the step it stopped at (ErrorCode) and
// whether the previous tree was put back byte for byte (RolledBack). Backup
// is the archive of the previous tree, configuration and database.
type RoundcubeUpdateResponse struct {
	Updated    bool   `json:"updated"`
	Proved     bool   `json:"proved"`
	From       string `json:"from,omitempty"`
	To         string `json:"to,omitempty"`
	Backup     string `json:"backup,omitempty"`
	RolledBack bool   `json:"rolled_back,omitempty"`
	Error      string `json:"error,omitempty"`
	ErrorCode  string `json:"error_code,omitempty"`
}
