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

// SiteSliceCounters are the slice cgroup's counters since its cgroup was
// made (sitesandbox.SliceCounters: which counter proves the site's own limit
// was measured on Debian 13 and Ubuntu 24.04 kernels).
type SiteSliceCounters struct {
	MemoryLimitHits  int64  `json:"memory_limit_hits"`
	MemoryOOMAll     int64  `json:"memory_oom_all"`
	OOMKills         int64  `json:"oom_kills"`
	MemoryMaxHits    int64  `json:"memory_max_hits"`
	TasksLimitHits   int64  `json:"tasks_limit_hits"`
	TasksOtherHits   int64  `json:"tasks_other_hits"`
	TasksRule        string `json:"tasks_rule,omitempty"`
	CPUThrottled     int64  `json:"cpu_throttled"`
	CPUThrottledUSec int64  `json:"cpu_throttled_usec"`
	CPUPeriods       int64  `json:"cpu_periods"`
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
	// DropInSlice is the slice an owner's drop-in of the unit names ("":
	// none). Such a unit is the owner's to place: the move does not restart
	// it and it is not counted as outside.
	DropInSlice string `json:"drop_in_slice,omitempty"`
}

// SiteSliceMoveRecovery is what the Agent did with a move of the site that an
// Agent stop interrupted or whose put-back did not complete
// (site_slices_move.go): per unit "moved" (it had come up in the slice and
// stays), "put_back" (its pre-move bytes are back and it runs where it ran),
// "rewritten" (stopped; its file names the slice), "not_tried", "changed"
// (its file was changed meanwhile and is left as it is) and, while Kept,
// "failed" (its pre-move bytes are back but it did not start again: it is
// stopped), "unknown" (where it runs could not be read, so it was not
// restarted), "starting" (its start was still running at its start wait) or
// "waiting" (not restarted yet: the Agent's start does not wait for it).
type SiteSliceMoveRecovery struct {
	At    string            `json:"at"`
	Units map[string]string `json:"units"`
	// Kept (S4 R2a third fix round, review N2): the put-back did not
	// complete, so the move's record stays and no site is moved until it is
	// settled (MoveSiteSliceRequest.Recover tries again).
	Kept bool `json:"kept,omitempty"`
}

// Recovery outcomes of a unit whose put-back did not complete (the record
// is kept).
const (
	// SiteMoveUnknown: where the unit runs could not be read; it was not
	// restarted.
	SiteMoveUnknown = "unknown"
	// SiteMoveStarting: its start was still running at its start wait.
	SiteMoveStarting = "starting"
	// SiteMoveWaiting (alpha.148 fix round): its pre-move bytes are back, it
	// ran before and does not run in the system slice now, and the Agent's
	// start did not restart it (the start's recovery is bounded so that it
	// never holds up the Agent); the next start, a move's attempt or Try
	// again restarts it.
	SiteMoveWaiting = "waiting"
)

// Kernel states of a running slice (SiteSliceStatus.KernelState).
const (
	// SiteSliceInForce: the slice's cgroup holds the values systemd applies
	// (memory.max, pids.max, cpu.max).
	SiteSliceInForce = "in_force"
	// SiteSlicePending: the values are written but the kernel (or systemd,
	// which has not read the file) does not hold them yet.
	SiteSlicePending = "pending"
	// SiteSliceKernelUnknown: the slice runs but its cgroup files could not
	// be read: never "in force".
	SiteSliceKernelUnknown = "unknown"
)

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
	// MoveInterrupted: an Agent stop interrupted the last move of this site
	// and the Agent finished or put it back at its start.
	MoveInterrupted *SiteSliceMoveRecovery `json:"move_interrupted,omitempty"`
	// Named (S4 R2a second fix round, review B2): a unit file names the
	// site's slice or a unit runs in it. An absent file is then a file that
	// was there and is gone (the next change of limits writes it again);
	// without it, the site's programs were never moved into their limits.
	Named bool `json:"named,omitempty"`
	// UnitsUnread (review B9): units of this site the read did not take
	// (above the request's bound); the rest were read.
	UnitsUnread int `json:"units_unread,omitempty"`
	// Kernel is what the slice's cgroup enforces (memory.max, pids.max,
	// cpu.max; 0: no limit) and KernelState compares it with Live (S4 R2a
	// third fix round, review N3): in_force, pending, unknown, or "" when the
	// slice does not run (systemd applies the file when a program starts).
	Kernel      SiteSliceLimitsRead `json:"kernel"`
	KernelState string              `json:"kernel_state,omitempty"`
}

// SiteSlicesStatusResponse answers SiteSlicesStatusV1. Unsupported says why
// this host cannot hold a site's limits at all ("": it can); no slice is
// written then. Partial says the request was larger than one read takes
// and only its first part was read.
type SiteSlicesStatusResponse struct {
	Sites       []SiteSliceStatus `json:"sites"`
	Unsupported string            `json:"unsupported,omitempty"`
	Partial     string            `json:"partial,omitempty"`
	Error       string            `json:"error,omitempty"`
}

// ApplySiteSlicesRequest writes changed limits into the slices (a plan, a
// reseller's limits or the server's changed), at most 100 sites per request
// (the Panel sends a large change in batches). A site without a slice file
// gets one when one of its units names the slice or runs in it, when
// CelikPanel wrote the file before, or when its first unit goes into it.
type ApplySiteSlicesRequest struct {
	ExpectedBuildCommit string      `json:"expected_build_commit"`
	Sites               []SiteSlice `json:"sites"`
}

// SiteSliceApplied is what happened to one slice: written, unchanged,
// absent (nothing uses it yet), kept (an owner's edit), failed, refused.
type SiteSliceApplied struct {
	SiteID      int    `json:"site_id"`
	Outcome     string `json:"outcome"`
	PendingPath string `json:"pending_path,omitempty"`
	Detail      string `json:"detail,omitempty"`
}

// ApplySiteSlicesResponse answers ApplySiteSlicesV1 (Unsupported: as in
// SiteSlicesStatusResponse; nothing was written).
type ApplySiteSlicesResponse struct {
	Sites       []SiteSliceApplied `json:"sites"`
	Unsupported string             `json:"unsupported,omitempty"`
	Error       string             `json:"error,omitempty"`
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
	// Recover (S4 R2a third fix round, review N2): the administrator's "Try
	// again" for a move whose put-back did not complete. The Agent runs the
	// put-back of the kept record now and moves nothing new; only RequestID
	// and ExpectedBuildCommit are read.
	Recover bool `json:"recover,omitempty"`
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
	// SiteMoveNotTried: an earlier unit of the site failed, or the move's
	// time ran out, so this one was not touched.
	SiteMoveNotTried = "not_tried"
	// SiteMoveChanged: its file was changed while the move ran (a customer's
	// change, a deletion): that change is kept and nothing is put over it.
	SiteMoveChanged = "changed"
	// SiteMoveOwnerSlice: an owner's drop-in places it in a slice; it is the
	// owner's choice, not moved and not restarted.
	SiteMoveOwnerSlice = "owner_slice"
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
	Done       bool `json:"done"`
	RolledBack bool `json:"rolled_back,omitempty"`
	// WaitSeconds is how long the move waited at most for each program to
	// run and answer again (the unit's own start timeout, at least 20 and
	// at most 300 seconds; systemd's default of 90 for these units).
	WaitSeconds int `json:"wait_seconds,omitempty"`
	// TimeUp (review B8): the move's time ran out; the units it moved stay
	// moved ("moved", "rewritten") and the ones it did not start are
	// "not_tried", for the next move to continue with.
	TimeUp    bool             `json:"time_up,omitempty"`
	SliceFile string           `json:"slice_file,omitempty"`
	Units     []SiteSliceMoved `json:"units"`
	Error     string           `json:"error,omitempty"`
	// Kept (review N2): the move's record stays after this call (a put-back
	// did not complete); no site is moved until it is settled. Recovery is
	// the outcome of a Recover request (or of the kept record).
	Kept     bool                   `json:"kept,omitempty"`
	Recovery *SiteSliceMoveRecovery `json:"recovery,omitempty"`
}
