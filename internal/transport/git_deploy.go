package transport

// Git deploy of a site (alpha.128, PLESK-GAP G32; nodesite/gitdeploy.go).

// GitDeployKeyRequest makes the site's deploy key (ed25519) on the server.
// Replace: a key exists and a new one replaces it (the old one stops working
// wherever it was added).
type GitDeployKeyRequest struct {
	ExpectedBuildCommit string `json:"expected_build_commit"`
	SiteID              int    `json:"site_id"`
	Domain              string `json:"domain"`
	Replace             bool   `json:"replace,omitempty"`
}

// GitDeployKeyResponse carries the PUBLIC key only (the private key never
// leaves the server) and its fingerprint.
type GitDeployKeyResponse struct {
	PublicKey   string `json:"public_key,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
	Reason      string `json:"reason,omitempty"`
	Error       string `json:"error,omitempty"`
}

// GitDeployRequest starts one deploy (idempotent by RequestID).
type GitDeployRequest struct {
	ExpectedBuildCommit string `json:"expected_build_commit"`
	RequestID           string `json:"request_id"`
	SiteID              int    `json:"site_id"`
	Domain              string `json:"domain"`
	WorkDir             string `json:"work_dir"`
	RunAsUser           string `json:"run_as_user"`
	RepoURL             string `json:"repo_url"`
	Branch              string `json:"branch"`
	// Build: a Node.js site installs its packages and runs its build script
	// after the files are in place (NodeVersion is the site's).
	Build       bool   `json:"build,omitempty"`
	NodeVersion string `json:"node_version,omitempty"`
}

// GitDeployStatusRequest reads a site's last deploy and its key (read only).
type GitDeployStatusRequest struct {
	SiteID int `json:"site_id"`
}

// GitDeployResponse is a deploy's state (the install step's phases) and the
// deploy key's fingerprint ("" without one).
type GitDeployResponse struct {
	RequestID      string   `json:"request_id,omitempty"`
	Phase          string   `json:"phase"`
	StartedAt      string   `json:"started_at,omitempty"`
	EndedAt        string   `json:"ended_at,omitempty"`
	Lines          []string `json:"lines,omitempty"`
	Class          string   `json:"class,omitempty"`
	Reason         string   `json:"reason,omitempty"`
	Error          string   `json:"error,omitempty"`
	KeyFingerprint string   `json:"key_fingerprint,omitempty"`
}
