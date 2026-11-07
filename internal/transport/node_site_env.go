package transport

// NodeSiteEnvRequest replaces a Node.js site's environment file (alpha.128,
// PLESK-GAP G19): root's and 0600 in the runtime apps' root folder, read by
// systemd before the site sandbox starts (nodesite.EnvPath). A secret entry
// without a value keeps the value already on the server; the Panel never
// stores or shows a secret value. No entry at all removes the file.
//
// Node.js sitesinin ortam dosyası: root'a ait, 0600; gizli değer Panel'de
// saklanmaz ve gösterilmez.
type NodeSiteEnvRequest struct {
	ExpectedBuildCommit string               `json:"expected_build_commit"`
	SiteID              int                  `json:"site_id"`
	Entries             []RuntimeAppEnvEntry `json:"entries"`
	// PreviousKeys are the keys CelikPanel wrote last time; a key in the
	// file that is neither there nor in Entries was added by the owner and
	// is not dropped silently (D-022).
	PreviousKeys []string `json:"previous_keys,omitempty"`
}
