package transport

// The operating system's own security updates (alpha.107, D-038 §4 and §9).
// Agent.OSUpdatesStatusV1 reads, without changing anything and bounded,
// whether the operating system's automatic updates are on (the OS's own
// configuration: unattended-upgrades on apt, dnf-automatic on dnf; Arch has
// none by design), when they last ran, how many updates and security updates
// are waiting (the package manager's simulation against its current lists),
// which installed packages come from vendor repositories the automatic
// updates do not cover, and the components the package manager does not
// install at all. Unknown stays unknown.
//
// Agent.WriteVendorUpdateOriginsV1 writes one separate drop-in
// (/etc/apt/apt.conf.d/52celikpanel-vendor-origins) that adds the confined
// vendor repositories to unattended-upgrades' origins, at the owner's
// confirmation of its exact text; it never edits the owner's own files and
// never switches automatic updates off.
//
// İşletim sisteminin kendi güvenlik güncellemeleri: salt okunur durum ve
// sahibin onayıyla yazılan tek ayrı ek dosya.

type OSUpdatesAutomatic struct {
	// State: on, off, not_installed, not_applicable (the system has no
	// automatic updates by design), unknown.
	State string `json:"state"`
	// Tool: unattended-upgrades, dnf-automatic, or empty.
	Tool string `json:"tool,omitempty"`
	// Reason names why it is off: config_off, timer_disabled.
	Reason string `json:"reason,omitempty"`
}

type OSUpdatesPending struct {
	// State: known, busy (another package operation holds the lock),
	// unknown.
	State         string   `json:"state"`
	Total         int      `json:"total"`
	Security      int      `json:"security"`
	SecurityNames []string `json:"security_names,omitempty"`
	// ListsAt is when the package lists the count is based on were last
	// refreshed (RFC 3339), empty when not known.
	ListsAt string `json:"lists_at,omitempty"`
}

// OSUpdatesVendorOrigin is one enabled vendor repository and whether the
// automatic updates install from it.
type OSUpdatesVendorOrigin struct {
	RepoID   string   `json:"repo_id"`
	Name     string   `json:"name"`
	Origin   string   `json:"origin,omitempty"`
	Covered  bool     `json:"covered"`
	Confined bool     `json:"confined"`
	Packages int      `json:"packages"`
	Names    []string `json:"names,omitempty"`
}

// OSUpdatesOutside is a component the package manager did not install, so
// the operating system never updates it (D-038 §9).
type OSUpdatesOutside struct {
	ID       string   `json:"id"`
	Versions []string `json:"versions,omitempty"`
}

// OSUpdatesDropIn is CelikPanel's own drop-in: whether it is there as
// written (State present, edited, absent), the origins it adds, and the
// exact text a confirmation would write now (Proposed, ProposedSHA256;
// empty when nothing would be added).
type OSUpdatesDropIn struct {
	Path           string   `json:"path"`
	State          string   `json:"state"`
	Origins        []string `json:"origins,omitempty"`
	Proposed       string   `json:"proposed,omitempty"`
	ProposedSHA256 string   `json:"proposed_sha256,omitempty"`
}

type OSUpdatesStatusResponse struct {
	PackageFamily string                  `json:"package_family"`
	Automatic     OSUpdatesAutomatic      `json:"automatic"`
	LastRun       string                  `json:"last_run,omitempty"`
	LastRunSource string                  `json:"last_run_source,omitempty"`
	Pending       OSUpdatesPending        `json:"pending"`
	Vendors       []OSUpdatesVendorOrigin `json:"vendors"`
	Outside       []OSUpdatesOutside      `json:"outside"`
	DropIn        *OSUpdatesDropIn        `json:"drop_in,omitempty"`
	// NeedrestartAutoRestart (Ubuntu): needrestart is configured to restart
	// services by itself after updates (what the OS does, not the panel).
	NeedrestartAutoRestart bool   `json:"needrestart_auto_restart,omitempty"`
	Error                  string `json:"error,omitempty"`
}

type WriteVendorUpdateOriginsRequest struct {
	ServiceMutationBinding
	// ExpectedSHA256 is the ProposedSHA256 the owner confirmed; the Agent
	// recomputes the text and refuses when it differs.
	ExpectedSHA256 string `json:"expected_sha256"`
}

type WriteVendorUpdateOriginsResponse struct {
	Written bool     `json:"written"`
	Path    string   `json:"path,omitempty"`
	Origins []string `json:"origins,omitempty"`
	// Effective: apt-config dump, run after the write, shows the origins.
	Effective bool   `json:"effective"`
	Error     string `json:"error,omitempty"`
	ErrorCode string `json:"error_code,omitempty"`
}
