package transport

// OSReleaseFactsResponse is what the Agent reads, without changing anything,
// for the operating-system release check (alpha.107, D-038 §17): the release
// and the facts an OS release upgrade changes underneath CelikPanel.
//
// İşletim sistemi sürüm denetimi için salt okunur bilgiler.
type OSReleaseFactsResponse struct {
	ID         string `json:"id"`
	VersionID  string `json:"version_id,omitempty"`
	Codename   string `json:"codename,omitempty"`
	PrettyName string `json:"pretty_name,omitempty"`
	// RepoSuites: each enabled CelikPanel vendor repository and the suite
	// its source line names ("trixie").
	RepoSuites map[string]string `json:"repo_suites,omitempty"`
	// PostgreSQLClusters: "17/main", from /etc/postgresql.
	PostgreSQLClusters []string `json:"postgresql_clusters,omitempty"`
	// PHPVersions: the PHP-FPM lines installed ("8.4").
	PHPVersions []string `json:"php_versions,omitempty"`
	// MariaDBVersion: the installed server's major.minor ("11.8").
	MariaDBVersion string `json:"mariadb_version,omitempty"`
	Error          string `json:"error,omitempty"`
}
