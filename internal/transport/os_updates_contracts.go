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
	// Reason names why an unknown count is unknown (pacman, 2026-10-30):
	// checkupdates_missing (pacman-contrib not installed), checkupdates_failed
	// (it could not reach the mirrors).
	Reason string `json:"reason,omitempty"`
	// SecurityState says how Security was counted where the package manager
	// has no security archive (pacman): known (arch-audit read the Arch
	// security tracker), arch_audit_missing, arch_audit_failed. Empty on apt,
	// where the -security archive is the source.
	SecurityState string `json:"security_state,omitempty"`
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
	// AutoConfig is the owner's automatic/manual choice on apt systems
	// (alpha.114), read from apt's own file.
	AutoConfig *OSUpdatesAutoConfig `json:"auto_config,omitempty"`
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

// OSUpdatesAutoConfig is the file that holds the owner's choice between
// automatic and manual updates on Debian and Ubuntu (alpha.114):
// /etc/apt/apt.conf.d/20auto-upgrades, apt's own file.
type OSUpdatesAutoConfig struct {
	Path string `json:"path"`
	// State: absent, panel (exactly the text CelikPanel writes), stock
	// (exactly the two standard lines a distribution writes), edited (the
	// owner's own text), unknown.
	State string `json:"state"`
	// Mode (auto or manual) is what a panel or stock file says.
	Mode string `json:"mode,omitempty"`
	// CurrentSHA256 is the file's digest, empty when absent; a change must
	// send it back so a file edited in between is never replaced.
	CurrentSHA256 string `json:"current_sha256,omitempty"`
	// Current is the file's text (bounded), so the owner reads it before
	// confirming a change.
	Current    string `json:"current,omitempty"`
	Unreadable bool   `json:"unreadable,omitempty"`
	// LegacyPath names the file the earlier installer wrote; LegacyState is
	// "" (absent), panel (exactly what the installer wrote) or edited.
	LegacyPath  string `json:"legacy_path,omitempty"`
	LegacyState string `json:"legacy_state,omitempty"`
	// Overridden: apt's effective value differs from what the file says,
	// because another file wins.
	Overridden bool `json:"overridden,omitempty"`
	// TimerState: enabled, disabled, masked, unknown (apt's two timers).
	TimerState string `json:"timer_state"`
	// Covers are the origins unattended-upgrades is set to install from, as
	// apt reports them (bounded).
	Covers []string `json:"covers,omitempty"`
	// ProposedAuto and ProposedManual are the exact texts a change would
	// write, shown before the owner confirms.
	ProposedAuto   string `json:"proposed_auto,omitempty"`
	ProposedManual string `json:"proposed_manual,omitempty"`
}

type SetOSUpdatesModeRequest struct {
	ServiceMutationBinding
	// Mode: auto or manual.
	Mode string `json:"mode"`
	// ExpectedCurrentSHA256 is the CurrentSHA256 the owner saw (empty when
	// the file was absent); a different file is never replaced.
	ExpectedCurrentSHA256 string `json:"expected_current_sha256"`
	// ConfirmOwnerEdit: the owner read the text of their own file and agrees
	// to replace it.
	ConfirmOwnerEdit bool `json:"confirm_owner_edit"`
}

type SetOSUpdatesModeResponse struct {
	Written       bool     `json:"written"`
	Path          string   `json:"path,omitempty"`
	Mode          string   `json:"mode,omitempty"`
	PreviousState string   `json:"previous_state,omitempty"`
	LegacyRemoved bool     `json:"legacy_removed,omitempty"`
	TimersEnabled []string `json:"timers_enabled,omitempty"`
	TimerState    string   `json:"timer_state,omitempty"`
	// Effective: apt-config dump, run after the write, shows the choice and
	// (for automatic) apt's timers are not disabled or masked.
	Effective bool `json:"effective"`
	// EffectiveReason when not effective: overridden_by_other_file,
	// timer_masked, timer_disabled, apt_config_unreadable.
	EffectiveReason string `json:"effective_reason,omitempty"`
	Error           string `json:"error,omitempty"`
	ErrorCode       string `json:"error_code,omitempty"`
}
