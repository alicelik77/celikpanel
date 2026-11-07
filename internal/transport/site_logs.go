package transport

// Site log retention (alpha.128, PLESK-GAP G27). The site logs stay where they
// are (/var/log/nginx/<domain>-{access,error}.log) and the distribution's own
// logrotate rule keeps rotating them; CelikPanel never adds a second rule for
// them (that would be logrotate's "duplicate log entry" error and stop
// rotation for the whole server). A plan, a subscription or the server may
// keep a site's logs for fewer days than the system does: the Agent then
// removes only that site's ROTATED files older than the limit. The live log
// is never touched. Without CelikPanel the system rule simply keeps working.
//
// Site günlüklerinin saklanması: dağıtımın kuralı döndürmeye devam eder;
// CelikPanel yalnız sitenin döndürülmüş, sınırdan eski dosyalarını siler.

// SiteLogRetentionItem is one site and how many days its rotated logs keep.
type SiteLogRetentionItem struct {
	Domain   string `json:"domain"`
	KeepDays int    `json:"keep_days"`
}

// SiteLogRetentionRequest applies the limits (removes older rotated files).
type SiteLogRetentionRequest struct {
	ExpectedBuildCommit string                 `json:"expected_build_commit"`
	Items               []SiteLogRetentionItem `json:"items"`
}

// SiteLogRetentionResponse says what was removed, per domain.
type SiteLogRetentionResponse struct {
	Removed map[string]int `json:"removed"`
	Error   string         `json:"error,omitempty"`
}

// SiteLogStatusRequest reads a site's log files and the system's rule
// (read only).
type SiteLogStatusRequest struct {
	Domain string `json:"domain"`
}

// SiteLogFile is one log file of the site.
type SiteLogFile struct {
	Name    string `json:"name"`
	Bytes   int64  `json:"bytes"`
	ModTime string `json:"mod_time"`
	Live    bool   `json:"live,omitempty"`
}

// SiteLogSystemRule is what the distribution's logrotate rule for nginx says.
type SiteLogSystemRule struct {
	Path      string `json:"path,omitempty"`
	Frequency string `json:"frequency,omitempty"`
	Rotate    int    `json:"rotate,omitempty"`
	Found     bool   `json:"found"`
}

// SiteLogStatusResponse is the read.
type SiteLogStatusResponse struct {
	Files  []SiteLogFile     `json:"files"`
	System SiteLogSystemRule `json:"system"`
	Error  string            `json:"error,omitempty"`
}
