package transport

// The site's limits (S4 R2a, docs/SITE-SANDBOX.md section 8): every program
// CelikPanel starts for one site as a systemd unit runs in that site's slice
// celiksite_<site id>.slice, whose memory, process and processor limits
// bound them together. The Panel computes the limits from the rights chain
// (site.memory_mb, site.tasks, site.cpu_percent; docs/SITE-LIMITS.md) and
// sends them with every operation that writes or starts such a unit; the
// Agent writes them into the native slice file under the D-031 rule.
//
// Sitenin sınırları: CelikPanel'in bir site için başlattığı her program o
// sitenin diliminde çalışır; dilimin sınırları hepsini birlikte sınırlar.

// SiteSlice is one site's slice and its limits. A request without it (an
// older Panel) leaves every unit where it runs.
type SiteSlice struct {
	SiteID int `json:"site_id"`
	// Domain names the site in the slice file's comment.
	Domain     string `json:"domain,omitempty"`
	MemoryMB   int    `json:"memory_mb"`
	Tasks      int    `json:"tasks"`
	CPUPercent int    `json:"cpu_percent"`
}

// Placements of one unit as the status read finds it.
const (
	// SiteUnitInSlice: the unit runs in its site's slice.
	SiteUnitInSlice = "in_slice"
	// SiteUnitOutside: the unit runs in the system slice (not moved yet).
	SiteUnitOutside = "outside"
	// SiteUnitStopped: the unit does not run; its file names the slice it
	// will start in (Slice, "" when unknown).
	SiteUnitStopped = "stopped"
	// SiteUnitElsewhere: the unit runs in another slice (an owner's drop-in).
	SiteUnitElsewhere = "elsewhere"
	// SiteUnitAbsent: no unit of that name on this server.
	SiteUnitAbsent = "absent"
)

// SiteSliceQuery asks for one site's slice and units.
type SiteSliceQuery struct {
	SiteID int      `json:"site_id"`
	Units  []string `json:"units,omitempty"`
}

// SiteSlicesStatusRequest reads the slices of these sites (read-only: it
// never writes, reloads or starts anything).
type SiteSlicesStatusRequest struct {
	Sites []SiteSliceQuery `json:"sites"`
}

// SiteSliceLimitsRead is what systemd applies to a slice or unit (0: no
// limit, "infinity").
type SiteSliceLimitsRead struct {
	MemoryMB   int `json:"memory_mb"`
	Tasks      int `json:"tasks"`
	CPUPercent int `json:"cpu_percent"`
}

// SiteSliceCounters are the slice cgroup's own counters since its cgroup
// was made (sitesandbox.SliceCounters).
type SiteSliceCounters struct {
	OOMKills         int64 `json:"oom_kills"`
	MemoryMaxHits    int64 `json:"memory_max_hits"`
	TasksMaxHits     int64 `json:"tasks_max_hits"`
	CPUThrottled     int64 `json:"cpu_throttled"`
	CPUThrottledUSec int64 `json:"cpu_throttled_usec"`
	CPUPeriods       int64 `json:"cpu_periods"`
}

// SiteSliceUnit is one unit of a site.
type SiteSliceUnit struct {
	Unit      string `json:"unit"`
	Placement string `json:"placement"`
	// Slice is the slice the unit runs in or its file names.
	Slice string `json:"slice,omitempty"`
	// File: ours, edited (an owner changed it), older (an earlier
	// protection), absent, unreadable.
	File string `json:"file,omitempty"`
	// Own is the unit's own limits (systemd's MemoryMax=, TasksMax=,
	// CPUQuota= of the unit itself).
	Own SiteSliceLimitsRead `json:"own"`
}

// SiteSliceStatus is one site's slice.
type SiteSliceStatus struct {
	SiteID int    `json:"site_id"`
	Slice  string `json:"slice"`
	// File is the slice file's state: absent, ours, edited, foreign,
	// unknown_origin, unreadable.
	File string `json:"file"`
	// DropIns are the owner's drop-in files of the slice.
	DropIns []string `json:"drop_ins,omitempty"`
	// Active: the slice runs (at least one unit of the site runs in it).
	Active bool `json:"active"`
	// Live is what systemd applies to the slice now.
	Live            SiteSliceLimitsRead `json:"live"`
	MemoryCurrentMB int64               `json:"memory_current_mb,omitempty"`
	TasksCurrent    int64               `json:"tasks_current,omitempty"`
	// CountersRead: the slice's cgroup exists and its counters were read.
	CountersRead bool              `json:"counters_read"`
	Counters     SiteSliceCounters `json:"counters"`
	Units        []SiteSliceUnit   `json:"units"`
}

// SiteSlicesStatusResponse answers SiteSlicesStatusV1.
type SiteSlicesStatusResponse struct {
	Sites []SiteSliceStatus `json:"sites"`
	Error string            `json:"error,omitempty"`
}

// ApplySiteSlicesRequest writes changed limits into the slices that exist
// (a plan, a reseller's limits or the server's changed). A site without a
// slice file gets one when its first unit goes into it.
type ApplySiteSlicesRequest struct {
	ExpectedBuildCommit string      `json:"expected_build_commit"`
	Sites               []SiteSlice `json:"sites"`
}

// SiteSliceApplied is what happened to one slice: written, unchanged,
// absent (nothing to change yet), kept (an owner's edit), failed.
type SiteSliceApplied struct {
	SiteID      int    `json:"site_id"`
	Outcome     string `json:"outcome"`
	PendingPath string `json:"pending_path,omitempty"`
	Detail      string `json:"detail,omitempty"`
}

// ApplySiteSlicesResponse answers ApplySiteSlicesV1.
type ApplySiteSlicesResponse struct {
	Sites []SiteSliceApplied `json:"sites"`
	Error string             `json:"error,omitempty"`
}

// MoveSiteSliceRequest moves a site's running units into its slice: the
// administrator's one action per site. Units are the site's long-running
// sandboxed units (celikapp-, celikrt-, celiksvc-); the Agent checks that
// each one's app folder is in the site's home (SubscriptionID, DomainID).
type MoveSiteSliceRequest struct {
	ExpectedBuildCommit string    `json:"expected_build_commit"`
	RequestID           string    `json:"request_id"`
	Site                SiteSlice `json:"site"`
	SubscriptionID      int       `json:"subscription_id"`
	DomainID            int       `json:"domain_id"`
	Units               []string  `json:"units"`
}

// Outcomes of one unit in a move.
const (
	// SiteMoveMoved: it was running, restarted once in the slice and runs
	// (and answers on its port, when it did before).
	SiteMoveMoved = "moved"
	// SiteMoveRewritten: it was stopped; its file names the slice and it
	// stays stopped.
	SiteMoveRewritten = "rewritten"
	// SiteMoveAlready: it already runs in (or, stopped, names) the slice.
	SiteMoveAlready = "already"
	// SiteMoveKept: an owner changed its file; kept, not moved.
	SiteMoveKept = "kept"
	// SiteMoveOlder: it runs in an earlier protection; the sandbox move on
	// its own page comes first.
	SiteMoveOlder = "older"
	// SiteMoveAbsent: no such unit on this server.
	SiteMoveAbsent = "absent"
	// SiteMoveRefused: the unit is not one of this site's.
	SiteMoveRefused = "refused"
	// SiteMoveFailed: the move of this unit failed; it was put back and
	// runs as before (PutBack false: putting it back failed too, Detail
	// says what is left).
	SiteMoveFailed = "failed"
	// SiteMovePutBack: it had moved, a later unit of the site failed, so
	// it was put back where it ran before.
	SiteMovePutBack = "put_back"
	// SiteMoveNotTried: an earlier unit of the site failed, so this one was
	// not touched.
	SiteMoveNotTried = "not_tried"
)

// SiteSliceMoved is one unit's outcome.
type SiteSliceMoved struct {
	Unit    string `json:"unit"`
	Outcome string `json:"outcome"`
	// Step names where a failure happened: write, restart, cgroup, answer.
	Step    string `json:"step,omitempty"`
	PutBack bool   `json:"put_back,omitempty"`
	Detail  string `json:"detail,omitempty"`
}

// MoveSiteSliceResponse answers MoveSiteSliceV1. Done: every unit of the
// site that could move is in its slice; RolledBack: a failure put the site
// back as it was before.
type MoveSiteSliceResponse struct {
	Done       bool             `json:"done"`
	RolledBack bool             `json:"rolled_back,omitempty"`
	SliceFile  string           `json:"slice_file,omitempty"`
	Units      []SiteSliceMoved `json:"units"`
	Error      string           `json:"error,omitempty"`
}
