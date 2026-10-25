package transport

// The vendor lane (alpha.108, D-038 amendment "two lanes"). By default a
// server uses its operating system's own packages only. A vendor repository
// (Sury for PHP, PGDG for PostgreSQL) is the administrator's informed choice,
// per component, and it is enabled only as ONE shown-and-confirmed operation
// that adds the repository, writes its confinement and adds its origin to the
// operating system's automatic updates, so a vendor repository is never
// unconfined or unpatched. On a server where the repository is already
// enabled, the same plan shows only the missing steps.
//
// Satıcı yolu: dış depo yalnız gösterilen ve onaylanan tek işlemle açılır;
// depo, sınırlama ve otomatik güncelleme birlikte. Açık bir depoda eksik
// adımlar gösterilir.

// VendorLaneStep is one part of the lane. Step: "repository" (the source
// line and its pinned key), "confinement" (CelikPanel's preferences file) or
// "updates" (the origin in CelikPanel's automatic-updates drop-in). State:
// "done" (in place as CelikPanel wrote it), "owner" (the owner's own
// configuration covers it; Reason names how), "todo" (Text is what is
// written, SHA256 its digest) or "blocked" (Reason says why; nothing of this
// step can be written until it is resolved).
type VendorLaneStep struct {
	Step   string `json:"step"`
	State  string `json:"state"`
	Reason string `json:"reason,omitempty"`
	Path   string `json:"path,omitempty"`
	Text   string `json:"text,omitempty"`
	SHA256 string `json:"sha256,omitempty"`
	Detail string `json:"detail,omitempty"`
}

// VendorLanePlan is what the confirmation shows. SHA256 covers every step
// that would be written; the owner confirms it and the Agent refuses when
// the plan it computes again is different. Ready: at least one step is to be
// written and nothing blocks the steps that would be written (a fresh
// repository needs all three). Kept are the installed packages that keep
// following the vendor under the confinement.
type VendorLanePlan struct {
	RepoID  string           `json:"repo_id"`
	Name    string           `json:"name,omitempty"`
	Origin  string           `json:"origin,omitempty"`
	Enabled bool             `json:"enabled"`
	Steps   []VendorLaneStep `json:"steps"`
	SHA256  string           `json:"sha256,omitempty"`
	Ready   bool             `json:"ready"`
	Kept    []string         `json:"kept,omitempty"`
	Error   string           `json:"error,omitempty"`
}

// VendorLaneResponse is the outcome. Completed names the steps written now;
// Steps is the lane as read afterwards. ErrorCode: LANE_PLAN_CHANGED (nothing
// written), LANE_BLOCKED, LANE_KEY_UNTRUSTED, LANE_ENABLE_FAILED (nothing of
// a fresh repository stays), LANE_PARTIAL (the steps in Completed stay; the
// rest is said), LANE_INVALID, LANE_UNSUPPORTED. Effective: apt-config shows
// the origin among the automatic updates' origins after the write.
type VendorLaneResponse struct {
	Completed []string         `json:"completed,omitempty"`
	Steps     []VendorLaneStep `json:"steps,omitempty"`
	Enabled   bool             `json:"enabled"`
	Effective bool             `json:"effective"`
	Error     string           `json:"error,omitempty"`
	ErrorCode string           `json:"error_code,omitempty"`
}
