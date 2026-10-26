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
	// PreviousAnswers: after a put-back, the previous tree answered the
	// proof again (alpha.109).
	PreviousAnswers bool   `json:"previous_answers,omitempty"`
	Error           string `json:"error,omitempty"`
	ErrorCode       string `json:"error_code,omitempty"`
}

// WebmailProofResponse is the read-only proof of webmail through the
// Agent-side webmail server (its socket): CelikPanel's PHP check, the login
// page and its stylesheet as text/css (alpha.109). Nothing is changed.
type WebmailProofResponse struct {
	Proved  bool   `json:"proved"`
	Version string `json:"version,omitempty"`
	Error   string `json:"error,omitempty"`
}

// Webmail on its own address (alpha.110, D-038 amendment). Roundcube is no
// longer served on the panel's origin: the server's own nginx serves it on a
// name the owner chooses (for example mail.example.com) with its own
// certificate, so a script running in Roundcube can never reach the panel's
// API with the administrator's session, and webmail keeps answering when the
// panel is stopped (D-022).
// Web postası kendi adresinde: sunucunun nginx'i sunar, panel aracı değildir.

// WebmailAddressEnableRequest enables webmail on Host. Email is the address
// certbot registers the certificate with (empty: certbot's existing account).
// Replace names the address enabled before, removed only after Host proves
// itself (changing the address: enable the new one, then remove the old).
type WebmailAddressEnableRequest struct {
	ServiceMutationBinding
	Host    string `json:"host"`
	Email   string `json:"email,omitempty"`
	Replace string `json:"replace,omitempty"`
}

// WebmailAddressResponse is the outcome of enabling an address. Proved means
// the login page, its stylesheet (text/css) and CelikPanel's PHP check
// answered over HTTPS on the name with the certificate issued for it.
// RolledBack: the nginx file was put back as it was (the certificate stays).
type WebmailAddressResponse struct {
	Enabled            bool   `json:"enabled"`
	Proved             bool   `json:"proved"`
	Host               string `json:"host,omitempty"`
	CertificateIssued  bool   `json:"certificate_issued,omitempty"`
	CertificateExpires string `json:"certificate_expires,omitempty"`
	Lineage            string `json:"lineage,omitempty"`
	RolledBack         bool   `json:"rolled_back,omitempty"`
	// RoundcubeSettings: "applied" (CelikPanel's lines for the address are
	// in Roundcube's configuration), "owner_edited" (the configuration is not
	// in CelikPanel's form, so it was left as it is) or "unchanged".
	RoundcubeSettings string `json:"roundcube_settings,omitempty"`
	Warning           string `json:"warning,omitempty"`
	Error             string `json:"error,omitempty"`
	ErrorCode         string `json:"error_code,omitempty"`
}

// WebmailAddressRemoveRequest removes the nginx file of Host. The
// certificate is kept unless RemoveCertificate.
type WebmailAddressRemoveRequest struct {
	ServiceMutationBinding
	Host              string `json:"host"`
	RemoveCertificate bool   `json:"remove_certificate,omitempty"`
}

type WebmailAddressRemoveResponse struct {
	Removed            bool   `json:"removed"`
	CertificateRemoved bool   `json:"certificate_removed,omitempty"`
	Warning            string `json:"warning,omitempty"`
	Error              string `json:"error,omitempty"`
	ErrorCode          string `json:"error_code,omitempty"`
}

// WebmailAddressStatusRequest reads one address (read only). Prove also
// asks webmail over HTTPS on the name, locally.
type WebmailAddressStatusRequest struct {
	Host  string `json:"host"`
	Prove bool   `json:"prove,omitempty"`
}

// WebmailAddressStatusResponse: Present (CelikPanel's nginx file for the
// name exists), Managed (in CelikPanel's own form), the certificate's expiry,
// Proved / ProofError when asked, and Hosts: every address file present.
type WebmailAddressStatusResponse struct {
	Present            bool     `json:"present"`
	Managed            bool     `json:"managed"`
	CertificateExpires string   `json:"certificate_expires,omitempty"`
	Proved             bool     `json:"proved,omitempty"`
	ProofError         string   `json:"proof_error,omitempty"`
	Hosts              []string `json:"hosts,omitempty"`
	Error              string   `json:"error,omitempty"`
}
