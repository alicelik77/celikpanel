package transport

// EnableRepoRequest identifies a repository operation while carrying the
// durable mutation identity used to make privileged changes idempotent.
// Repository URLs and signing keys deliberately do not cross the RPC boundary;
// the agent resolves those values from its compiled catalogue.
type EnableRepoRequest struct {
	ServiceMutationBinding
	RepoID string `json:"repo_id"`
	// ExpectedSHA256 (alpha.107, confine only): the preferences text the
	// owner confirmed; the Agent recomputes it and refuses when it differs.
	ExpectedSHA256 string `json:"expected_sha256,omitempty"`
}

// RepoConfinePreview is what confining a repository would do, shown before
// the owner confirms it (alpha.107): the exact preferences text and its
// SHA-256, the installed packages that keep following the vendor, and how
// `apt-get upgrade` changes (Changes: before and after, each version's
// vendor flag; StillFromVendor: vendor packages still upgraded). Read only.
type RepoConfinePreview struct {
	State            string              `json:"state"`
	PinPath          string              `json:"pin_path,omitempty"`
	OwnerFiles       []string            `json:"owner_files,omitempty"`
	Text             string              `json:"text,omitempty"`
	SHA256           string              `json:"sha256,omitempty"`
	Kept             []string            `json:"kept,omitempty"`
	Changes          []RepoConfineChange `json:"changes,omitempty"`
	StillFromVendor  []string            `json:"still_from_vendor,omitempty"`
	UpgradesNow      int                 `json:"upgrades_now"`
	UpgradesConfined int                 `json:"upgrades_confined"`
	Error            string              `json:"error,omitempty"`
}

// RepoConfineChange is one package `apt-get upgrade` treats differently with
// the confinement: Before/After are the versions it would install ("" for
// none, the package stays at From).
type RepoConfineChange struct {
	Package      string `json:"package"`
	From         string `json:"from,omitempty"`
	Before       string `json:"before,omitempty"`
	BeforeVendor bool   `json:"before_vendor,omitempty"`
	After        string `json:"after,omitempty"`
	AfterVendor  bool   `json:"after_vendor,omitempty"`
}

type RepoStatusResponse struct {
	Enabled         bool   `json:"enabled"`
	Repairable      bool   `json:"repairable,omitempty"`
	PartialSuccess  bool   `json:"partial_success,omitempty"`
	MutationApplied bool   `json:"mutation_applied,omitempty"`
	Source          string `json:"source,omitempty"`
	Error           string `json:"error,omitempty"`
	ErrorCode       string `json:"error_code,omitempty"`
	// alpha.107 (D-038 §12, additive; an older Agent sets none of them):
	// the confinement, the moving default-version packages installed, the
	// signing key, what apt last downloaded from the repository, when it was
	// enabled (the Agent's own record) and apt's last error lines that name
	// it.
	Confinement    *RepoConfinement    `json:"confinement,omitempty"`
	MovingPackages []RepoMovingPackage `json:"moving_packages,omitempty"`
	Key            *RepoKeyInfo        `json:"key,omitempty"`
	Lists          *RepoListsInfo      `json:"lists,omitempty"`
	EnabledAt      string              `json:"enabled_at,omitempty"`
	AptErrors      []string            `json:"apt_errors,omitempty"`
	AptErrorsAt    string              `json:"apt_errors_at,omitempty"`
}

type RepoPackagesRequest struct {
	RepoID string `json:"repo_id"`
}

type RepoPackagesResponse struct {
	Packages  []string `json:"packages"`
	ErrorCode string   `json:"error_code,omitempty"`
	Error     string   `json:"error,omitempty"`
}

// RepoConfinement says whether a vendor repository is confined to its own
// component (alpha.107, D-038 §12): an apt preferences file that lets the
// repository provide a package only where the distribution has none, so it
// never replaces a distribution package. State:
//
//   - "confined": CelikPanel's preferences file is in place, as written;
//   - "edited": CelikPanel's file was changed by someone; it is kept as it is;
//   - "owner_rule": the owner's own preferences file (OwnerFiles) already sets
//     a rule for this repository, so CelikPanel writes none;
//   - "not_written": the repository is enabled but not confined; the owner
//     confines it from the repository card (Reason says why when an attempt
//     failed: lists_unreadable, write_failed, plan_changed);
//   - "removed_by_owner": CelikPanel wrote the file and someone deleted it;
//     it is never written again unless the owner asks;
//   - "unknown": it could not be read.
//
// Kept are packages that were already installed from the repository before it
// was confined: they keep following it, so nothing that was updated from it
// stops being updated (KeptTotal counts them all; Kept is capped). Additive.
// RepoConfinement: bir üretici deposunun kendi bileşeniyle sınırlanıp
// sınırlanmadığı. Eklemelidir.
type RepoConfinement struct {
	State      string   `json:"state"`
	PinPath    string   `json:"pin_path,omitempty"`
	OwnerFiles []string `json:"owner_files,omitempty"`
	Kept       []string `json:"kept,omitempty"`
	KeptTotal  int      `json:"kept_total,omitempty"`
	WrittenAt  string   `json:"written_at,omitempty"`
	WrittenBy  string   `json:"written_by,omitempty"`
	Reason     string   `json:"reason,omitempty"`
	Detail     string   `json:"detail,omitempty"`
}

// RepoMovingPackage is an installed distribution "default version" package
// (php-fpm, postgresql) on a server with the repository enabled (alpha.107,
// D-038 §1/§7): it follows whatever version is the default, and Exact names
// the versioned package it brings today. Reported, never removed.
// RepoMovingPackage: depo açıkken kurulu "varsayılan sürüm" paketi.
type RepoMovingPackage struct {
	Package string `json:"package"`
	Exact   string `json:"exact,omitempty"`
}

// RepoKeyInfo is the pinned signing key as it is on disk (alpha.107, D-038
// §12): its fingerprint, when it was made and when it expires (empty: it does
// not expire). State: valid, expiring (60 days or less), expired, unknown.
// RepoKeyInfo: diskteki sabitlenmiş imza anahtarı.
type RepoKeyInfo struct {
	Fingerprint string `json:"fingerprint"`
	CreatedAt   string `json:"created_at,omitempty"`
	ExpiresAt   string `json:"expires_at,omitempty"`
	State       string `json:"state"`
}

// RepoListsInfo is what apt last downloaded from the repository (alpha.107):
// State fresh, missing (no package list from it), expired (its Valid-Until
// has passed) or unknown; Date and ValidUntil from its Release file, Origin
// its Origin field (what unattended-upgrades matches).
// RepoListsInfo: apt'ın depodan en son indirdiği.
type RepoListsInfo struct {
	State      string `json:"state"`
	Date       string `json:"date,omitempty"`
	ValidUntil string `json:"valid_until,omitempty"`
	Origin     string `json:"origin,omitempty"`
	Suite      string `json:"suite,omitempty"`
}
