package transport

type DeleteSiteRequest struct {
	ExpectedBuildCommit string `json:"expected_build_commit"`
	SiteID              int    `json:"site_id"`
	SubscriptionID      int    `json:"subscription_id"`
	DomainID            int    `json:"domain_id"`
	Domain              string `json:"domain"`
	Username            string `json:"username"`
	PHPVersion          string `json:"php_version"`
	// SiteHome is retained for rolling RPC compatibility, but the agent
	// derives the authoritative value from SubscriptionID and DomainID and
	// rejects a non-empty mismatch.
	SiteHome string `json:"site_home"`
}

type DeleteSiteResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
	// KeptBackups (alpha.134, additive): the deleted site's backups, which a
	// deletion keeps on purpose: where, how many files, how many bytes. Nil
	// when there are none.
	KeptBackups *KeptBackups `json:"kept_backups,omitempty"`
}

// KeptBackups describes backup files a deletion left in place.
type KeptBackups struct {
	Path  string `json:"path"`
	Files int    `json:"files"`
	Bytes int64  `json:"bytes"`
	// Unknown: the folder exists but could not be read to the end.
	Unknown bool `json:"unknown,omitempty"`
}
