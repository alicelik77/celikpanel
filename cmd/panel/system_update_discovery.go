package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alicelik/celikpanel/internal/recoveryobs"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Update discovery (owner request 2026-10-17, D-035): the Panel looks for a
// newer signed release by itself, keeps what it found, and the panel says so
// calmly where the owner looks. It never starts an update: only an
// administrator does, from the panel's own update screen.
//
// The scheduled check is the same read the "Check for updates" button runs
// (discoverPanelUpdate): the Agent asks the release server for the signed
// manifest. It changes nothing on the host. It runs about two minutes after
// the Panel starts, then every 12 hours with half an hour of jitter; after a
// failure the next try is in 1 hour, then 3, then back to 12. It is skipped
// (next try in an hour, never queued) while this Panel admitted an update in
// the last hour, while the server reports a change in progress (an update
// holds the host lock), while a server setup run is changing the server, when
// the Agent does not answer, and while the owner has switched it off; it ends
// with the process.
//
// It never writes while an update is admitted or running: a write to the
// Panel's database while the updater's read-only preflight holds its pinned
// copy makes that preflight stop. A skip is kept in memory only. Before a
// result is written the Agent's readiness is read with no lock held; then,
// under the Panel's own mutation lease (TryLock), only the in-process facts
// are read again (no update admitted, no Panel operation active) and the
// setting is written. Otherwise the result is kept in memory and written on
// the next idle pass.
//
// Guncelleme kesfi: Panel imzali yeni surumu kendisi arar, buldugunu saklar
// ve panel bunu sakin bicimde soyler. Guncellemeyi asla baslatmaz. Bir
// guncelleme kabul edilmis ya da suruyorken veritabanina yazmaz: atlama
// yalniz bellekte tutulur, sonuc bosta bir gecisde kaydedilir.

const (
	panelUpdateDiscoveryPath = "/api/v1/panel/update/discovery"

	// panel_settings keys (additive; no schema change).
	panelUpdateDiscoveryKey = "panel_update_discovery"
	panelUpdateAutoCheckKey = "panel_update_auto_check"

	panelUpdateDiscoveryFirstDelay  = 2 * time.Minute
	panelUpdateDiscoveryFirstJitter = 30 * time.Second
	panelUpdateDiscoveryPeriod      = 12 * time.Hour
	panelUpdateDiscoveryJitter      = 30 * time.Minute
	// A skipped check (busy server, setup run, Agent not answering) is tried
	// again after an hour. It is not a failure and does not grow the backoff.
	panelUpdateDiscoverySkipDelay = time.Hour
	// The bound of one check: the Agent's own bound of the release read (30 s,
	// cmd/agent system_update_rpc.go) plus the identity reads before it.
	panelUpdateDiscoveryTimeout = 40 * time.Second
	// How long a start this Panel admitted counts as an update in progress
	// for discovery: an hour covers a slow download before the updater's
	// preflight; the update's host lock covers the updater itself, and a
	// successful update restarts the Panel, which forgets it.
	panelUpdateActiveWindow = time.Hour

	// Opening the update screen (alpha.103, owner observation 2026-10-23):
	// with automatic checks on, a stored result older than this is checked
	// again through the same discovery. Ten minutes keeps a person who opens
	// the screen again within one sitting from asking celikpanel.net each
	// time (at most one release read per ten minutes from screen openings,
	// whoever opens it), while a result from the 12-hour schedule is never
	// what the screen offers when a newer release has since been published.
	panelUpdateDiscoveryRefreshPath = "/api/v1/panel/update/discovery/refresh"
	panelUpdateDiscoveryOpenFresh   = 10 * time.Minute
	// The bound of the whole request: a check already running is waited for
	// (it is itself bounded at panelUpdateDiscoveryTimeout), then this one's
	// check uses what is left, never more than its own bound.
	panelUpdateDiscoveryOpenTimeout = 50 * time.Second

	panelUpdateOutcomeAvailable = "available"
	panelUpdateOutcomeCurrent   = "current"
	panelUpdateOutcomeFailed    = "failed"
)

// The backoff after consecutive failed checks: 1 hour, then 3, then the
// ordinary period.
var panelUpdateDiscoveryBackoff = [...]time.Duration{time.Hour, 3 * time.Hour}

// Reason codes of a failed or skipped check. Closed set; the browser
// translates each and shows nothing for an unknown one.
const (
	// failed
	panelUpdateReasonReleaseUnverified = "release_unverified" // the release server gave no verified answer
	panelUpdateReasonTimeout           = "timeout"            // the check did not finish in time; result not known
	panelUpdateReasonNotSupported      = "not_supported"      // this build pair or system cannot use signed updates
	panelUpdateReasonIdentityMismatch  = "identity_mismatch"  // the Agent reported a different running build
	panelUpdateReasonInvalidAnswer     = "invalid_answer"     // the answer was not a valid signed release
	panelUpdateReasonAgentUnavailable  = "agent_unavailable"  // the Agent did not answer (skipped when automatic)
	// skipped
	panelUpdateReasonUpdateActive    = "update_active"
	panelUpdateReasonServerBusy      = "server_busy"
	panelUpdateReasonSetupRunning    = "setup_running"
	panelUpdateReasonStateUnverified = "state_unverified"
)

var panelUpdateDiscoveryReasons = map[string]bool{
	panelUpdateReasonReleaseUnverified: true, panelUpdateReasonTimeout: true,
	panelUpdateReasonNotSupported: true, panelUpdateReasonIdentityMismatch: true,
	panelUpdateReasonInvalidAnswer: true, panelUpdateReasonAgentUnavailable: true,
	panelUpdateReasonUpdateActive: true, panelUpdateReasonServerBusy: true,
	panelUpdateReasonSetupRunning: true, panelUpdateReasonStateUnverified: true,
}

var errPanelUpdateAgentUnreachable = errors.New("update agent did not answer")

// panelUpdateDiscoveryFailure is a check that did not give a result. kind
// selects the HTTP answer the button's handler has always written.
type panelUpdateDiscoveryFailure struct {
	agent  bool // writePanelUpdateAgentFailure; otherwise writePanelUpdateUnavailable
	reason string
	err    error
}

// discoverPanelUpdate is the one discovery path, shared by the button's
// handler and the scheduled check: Agent availability and build pair, the
// host's authority for the eventual start, the Agent's signed release read,
// the identity match and the target validation. Read-only.
func (p *Panel) discoverPanelUpdate(ctx context.Context) (panelUpdateCheckResponse, *panelUpdateDiscoveryFailure) {
	if err := p.requireSystemUpdateAgent(ctx); err != nil {
		reason := panelUpdateReasonNotSupported
		if errors.Is(err, errPanelUpdateAgentUnreachable) {
			reason = panelUpdateReasonAgentUnavailable
			if errors.Is(err, context.DeadlineExceeded) {
				reason = panelUpdateReasonTimeout
			}
		}
		return panelUpdateCheckResponse{}, &panelUpdateDiscoveryFailure{reason: reason, err: err}
	}
	// Discovery must prove that this host could authorize the eventual
	// privileged start. Read-only Check is allowed on every platform, while
	// Start remains deliberately closed on unreviewed package families.
	if err := p.authorizeAgentRPCContext(ctx, "Agent.StartSystemUpdate"); err != nil {
		reason := panelUpdateReasonNotSupported
		if errors.Is(err, context.DeadlineExceeded) {
			reason = panelUpdateReasonTimeout
		}
		return panelUpdateCheckResponse{}, &panelUpdateDiscoveryFailure{reason: reason, err: err}
	}
	var reply transport.SystemUpdateCheckResponse
	if err := p.callAgentContext(ctx, "Agent.CheckSystemUpdate", &transport.Empty{}, &reply); err != nil {
		reason := panelUpdateReasonAgentUnavailable
		if errors.Is(err, context.DeadlineExceeded) {
			reason = panelUpdateReasonTimeout
		}
		return panelUpdateCheckResponse{}, &panelUpdateDiscoveryFailure{agent: true, reason: reason, err: err}
	}
	if !reply.Supported {
		return panelUpdateCheckResponse{}, &panelUpdateDiscoveryFailure{
			reason: panelUpdateReasonNotSupported, err: errors.New("agent reported updater unsupported"),
		}
	}
	if strings.TrimSpace(reply.Error) != "" {
		log.Printf("[panel-update] discovery refused: %s", sanitizePanelUpdateSummary(reply.Error))
		return panelUpdateCheckResponse{}, &panelUpdateDiscoveryFailure{
			reason: panelUpdateReasonReleaseUnverified, err: errors.New("agent reported update discovery failure"),
		}
	}
	if reply.CurrentVersion != buildVersion || reply.CurrentCommit != buildCommit {
		return panelUpdateCheckResponse{}, &panelUpdateDiscoveryFailure{
			reason: panelUpdateReasonIdentityMismatch, err: errors.New("update discovery current identity does not match panel"),
		}
	}
	response := panelUpdateCheckResponse{
		Supported: true, Available: reply.Available,
		CurrentVersion: buildVersion, CurrentCommit: buildCommit,
	}
	if reply.Available {
		target := panelUpdateTargetFromCheck(reply)
		if !validPanelUpdateTarget(target) {
			return panelUpdateCheckResponse{}, &panelUpdateDiscoveryFailure{
				reason: panelUpdateReasonInvalidAnswer, err: errors.New("agent returned an invalid signed update identity"),
			}
		}
		response.Target = &target
		if target.Commit != buildCommit {
			response.PreviousAttempt = p.previousPanelUpdateAttempt(target.Commit)
		}
	}
	return response, nil
}

// panelUpdateDiscoveryRecord is the last check's result, kept in
// panel_settings. Target is the found release; after a failed check the last
// found target is kept with the moment it was found, so a passing outage does
// not hide an update that is still waiting. A skipped check is never a record.
type panelUpdateDiscoveryRecord struct {
	CheckedAt  string             `json:"checked_at"`
	Outcome    string             `json:"outcome"`
	Automatic  bool               `json:"automatic"`
	ReasonCode string             `json:"reason_code,omitempty"`
	Target     *panelUpdateTarget `json:"target,omitempty"`
	FoundAt    string             `json:"found_at,omitempty"`
	// The build that made the check; a record of another build is cleared.
	CurrentVersion string `json:"current_version"`
	CurrentCommit  string `json:"current_commit"`
}

func validPanelUpdateDiscoveryRecord(record panelUpdateDiscoveryRecord) bool {
	if _, err := time.Parse(time.RFC3339, record.CheckedAt); err != nil {
		return false
	}
	switch record.Outcome {
	case panelUpdateOutcomeAvailable:
		if record.Target == nil || record.ReasonCode != "" {
			return false
		}
	case panelUpdateOutcomeCurrent:
		if record.Target != nil || record.ReasonCode != "" {
			return false
		}
	case panelUpdateOutcomeFailed:
		if !panelUpdateDiscoveryReasons[record.ReasonCode] {
			return false
		}
	default:
		return false
	}
	if record.Target != nil {
		if !validPanelUpdateTarget(*record.Target) {
			return false
		}
		if _, err := time.Parse(time.RFC3339, record.FoundAt); err != nil {
			return false
		}
	}
	return validPanelUpdateVersion(record.CurrentVersion) && panelUpdateCommitPattern.MatchString(record.CurrentCommit)
}

// relevantPanelUpdateDiscovery says whether a record still describes this
// build: made by it, and any target not this build and not older than it. A
// rebuild with the same version and another commit is the Agent's to call
// available (its signed sequence decides), so equal versions are kept.
func relevantPanelUpdateDiscovery(record panelUpdateDiscoveryRecord) bool {
	if record.CurrentVersion != buildVersion || record.CurrentCommit != buildCommit {
		return false
	}
	if record.Target != nil {
		if record.Target.Commit == buildCommit {
			return false
		}
		if order, ok := comparePanelUpdateVersions(record.Target.Version, buildVersion); !ok || order < 0 {
			return false
		}
	}
	return true
}

func (p *Panel) storedPanelUpdateDiscovery(ctx context.Context) (panelUpdateDiscoveryRecord, bool) {
	var record panelUpdateDiscoveryRecord
	if p == nil || p.db == nil {
		return record, false
	}
	raw := p.setting(ctx, panelUpdateDiscoveryKey)
	if raw == "" || json.Unmarshal([]byte(raw), &record) != nil ||
		!validPanelUpdateDiscoveryRecord(record) || !relevantPanelUpdateDiscovery(record) {
		return panelUpdateDiscoveryRecord{}, false
	}
	return record, true
}

// loadPanelUpdateDiscovery is the newest known result: one kept in memory
// while the server was busy, else the stored one. The caller holds
// updateDiscoveryMu or accepts a read that may be a moment old.
func (p *Panel) loadPanelUpdateDiscovery(ctx context.Context) (panelUpdateDiscoveryRecord, bool) {
	if p != nil && p.updateDiscoveryPending != nil && relevantPanelUpdateDiscovery(*p.updateDiscoveryPending) {
		return *p.updateDiscoveryPending, true
	}
	return p.storedPanelUpdateDiscovery(ctx)
}

func (p *Panel) currentPanelUpdateDiscovery(ctx context.Context) (panelUpdateDiscoveryRecord, bool) {
	if p == nil {
		return panelUpdateDiscoveryRecord{}, false
	}
	p.updateDiscoveryMu.Lock()
	defer p.updateDiscoveryMu.Unlock()
	return p.loadPanelUpdateDiscovery(ctx)
}

// clearStalePanelUpdateDiscovery removes, at start and before management
// opens, a record made by another build or naming a release this build
// already is or is newer than (after an update). A bookkeeping write only.
func (p *Panel) clearStalePanelUpdateDiscovery(ctx context.Context) {
	if p == nil || p.db == nil {
		return
	}
	p.updateDiscoveryMu.Lock()
	defer p.updateDiscoveryMu.Unlock()
	if p.setting(ctx, panelUpdateDiscoveryKey) == "" {
		return
	}
	if _, ok := p.storedPanelUpdateDiscovery(ctx); ok {
		return
	}
	if _, err := p.db.GetDB().ExecContext(ctx, `DELETE FROM panel_settings WHERE key = ?`, panelUpdateDiscoveryKey); err != nil {
		log.Printf("[panel-update] earlier update check could not be cleared: %v", err)
	}
}

// buildPanelUpdateDiscoveryRecord derives the record of one completed check
// from the earlier one. Pure; nothing is written.
func buildPanelUpdateDiscoveryRecord(previous *panelUpdateDiscoveryRecord, now time.Time, automatic bool, response panelUpdateCheckResponse, failure *panelUpdateDiscoveryFailure) (panelUpdateDiscoveryRecord, bool) {
	record := panelUpdateDiscoveryRecord{
		CheckedAt: now.UTC().Format(time.RFC3339), Automatic: automatic,
		CurrentVersion: buildVersion, CurrentCommit: buildCommit,
	}
	switch {
	case failure != nil:
		record.Outcome, record.ReasonCode = panelUpdateOutcomeFailed, failure.reason
		if previous != nil && previous.Target != nil {
			target := *previous.Target
			record.Target, record.FoundAt = &target, previous.FoundAt
		}
	case response.Available && response.Target != nil:
		target := *response.Target
		record.Outcome, record.Target, record.FoundAt = panelUpdateOutcomeAvailable, &target, record.CheckedAt
		// The same release found again keeps the moment it was first found.
		if previous != nil && previous.Target != nil && samePanelUpdateTarget(*previous.Target, target) {
			record.FoundAt = previous.FoundAt
		}
	default:
		record.Outcome = panelUpdateOutcomeCurrent
	}
	if !validPanelUpdateDiscoveryRecord(record) || !relevantPanelUpdateDiscovery(record) {
		// A target that is this build or older is not an update.
		if record.Outcome == panelUpdateOutcomeAvailable {
			record.Outcome, record.Target, record.FoundAt = panelUpdateOutcomeCurrent, nil, ""
		} else {
			record.Target, record.FoundAt = nil, ""
		}
	}
	return record, validPanelUpdateDiscoveryRecord(record)
}

// panelUpdateDiscoveryHostIdle is the one advisory Agent read before a write:
// the server reports no change in progress (a busy package manager does not
// count). It is made with no lock held, so neither the read of the result nor
// a start request waits for it.
func (p *Panel) panelUpdateDiscoveryHostIdle(ctx context.Context) bool {
	if p == nil || p.agentClient == nil {
		return false
	}
	var readiness transport.HostMutationReadinessResponse
	if err := p.callAgentContext(ctx, "Agent.ServiceMutationReadiness", &transport.Empty{}, &readiness); err != nil {
		return false
	}
	readiness = verifiedAgentMutationReadiness(readiness)
	return readiness.Ready || readiness.Reason == transport.HostMutationReasonPackageManager
}

// writablePanelUpdateDiscoveryLocked takes the Panel's mutation lease with
// TryLock and re-checks only the facts this process holds: that no update was
// admitted and that no Panel operation is active. It makes no Agent call; it
// returns the release of the lease, or nil when the write must wait. The
// lease is held only for this re-check and the settings write. The caller
// holds updateDiscoveryMu.
func (p *Panel) writablePanelUpdateDiscoveryLocked(ctx context.Context, now time.Time) func() {
	if p == nil || p.db == nil || p.panelUpdateRecentlyAdmitted(now) {
		return nil
	}
	if !p.serviceMutationMu.TryLock() {
		return nil
	}
	release := p.serviceMutationMu.Unlock
	if p.panelUpdateRecentlyAdmitted(now) {
		release()
		return nil
	}
	operation, err := p.activeServiceOperation(ctx)
	if err != nil || operation != nil {
		release()
		return nil
	}
	return release
}

// savePanelUpdateDiscovery keeps the result of one completed check, manual
// or automatic: written when the server is idle, otherwise kept in memory
// until the next idle pass. It returns the record and whether it was written.
// The Agent read comes first, with no lock held.
func (p *Panel) savePanelUpdateDiscovery(ctx context.Context, now time.Time, automatic bool, response panelUpdateCheckResponse, failure *panelUpdateDiscoveryFailure) (panelUpdateDiscoveryRecord, bool) {
	if p == nil || p.db == nil {
		record, _ := buildPanelUpdateDiscoveryRecord(nil, now, automatic, response, failure)
		return record, false
	}
	hostIdle := p.panelUpdateDiscoveryHostIdle(ctx)
	p.updateDiscoveryMu.Lock()
	defer p.updateDiscoveryMu.Unlock()
	var previous *panelUpdateDiscoveryRecord
	if earlier, ok := p.loadPanelUpdateDiscovery(ctx); ok {
		previous = &earlier
	}
	record, ok := buildPanelUpdateDiscoveryRecord(previous, now, automatic, response, failure)
	if !ok {
		return record, false
	}
	p.updateDiscoveryPending = &record
	return record, p.flushPanelUpdateDiscoveryLocked(ctx, now, hostIdle)
}

// flushPanelUpdateDiscovery writes a result kept in memory, when the server
// is idle now. The Agent read is made only when something is kept, and with
// no lock held.
func (p *Panel) flushPanelUpdateDiscovery(ctx context.Context, now time.Time) bool {
	if p == nil || p.db == nil {
		return false
	}
	p.updateDiscoveryMu.Lock()
	kept := p.updateDiscoveryPending != nil
	p.updateDiscoveryMu.Unlock()
	if !kept {
		return false
	}
	hostIdle := p.panelUpdateDiscoveryHostIdle(ctx)
	p.updateDiscoveryMu.Lock()
	defer p.updateDiscoveryMu.Unlock()
	return p.flushPanelUpdateDiscoveryLocked(ctx, now, hostIdle)
}

// flushPanelUpdateDiscoveryLocked writes the kept result when the Agent read
// made just before said the host is idle and the in-process re-check under
// the lease passes. The caller holds updateDiscoveryMu.
func (p *Panel) flushPanelUpdateDiscoveryLocked(ctx context.Context, now time.Time, hostIdle bool) bool {
	if p.updateDiscoveryPending == nil {
		return false
	}
	record := *p.updateDiscoveryPending
	if !relevantPanelUpdateDiscovery(record) {
		p.updateDiscoveryPending = nil
		return false
	}
	if !hostIdle {
		return false
	}
	release := p.writablePanelUpdateDiscoveryLocked(ctx, now)
	if release == nil {
		return false
	}
	defer release()
	raw, err := json.Marshal(record)
	if err == nil {
		err = p.setSetting(ctx, panelUpdateDiscoveryKey, string(raw))
	}
	if err != nil {
		log.Printf("[panel-update] update check result not recorded: %v", err)
		return false
	}
	p.updateDiscoveryPending = nil
	return true
}

func (p *Panel) panelUpdateAutoCheck(ctx context.Context) bool {
	if p == nil || p.db == nil {
		return false
	}
	return p.setting(ctx, panelUpdateAutoCheckKey) != "off"
}

// markPanelUpdateAdmitted notes that this Panel admitted an update start.
func (p *Panel) markPanelUpdateAdmitted(now time.Time) {
	p.updateAdmittedAt.Store(now.UnixNano())
}

func (p *Panel) panelUpdateRecentlyAdmitted(now time.Time) bool {
	admitted := p.updateAdmittedAt.Load()
	return admitted > 0 && now.Sub(time.Unix(0, admitted)) < panelUpdateActiveWindow
}

// panelUpdateDiscoverySkipReason names why a scheduled check must not run
// now, or "" when it may. Reads only; its readiness read, like the readiness
// endpoint, takes the Panel's mutation lease with TryLock only for the
// in-process check of an active operation and releases it before its Agent
// read.
func (p *Panel) panelUpdateDiscoverySkipReason(ctx context.Context, now time.Time) string {
	if p.panelUpdateRecentlyAdmitted(now) {
		return panelUpdateReasonUpdateActive
	}
	if p.agentClient == nil {
		return panelUpdateReasonAgentUnavailable
	}
	if p.db != nil {
		// Only a setup run that is changing the server holds discovery; one
		// waiting for a DNS record does not (serverSetupExecutionMutating).
		mutating, err := p.serverSetupExecutionMutating(ctx)
		if err != nil {
			return panelUpdateReasonStateUnverified
		}
		if mutating {
			return panelUpdateReasonSetupRunning
		}
	}
	readiness := p.readHostMutationReadinessLogged(ctx, func(string, ...any) {})
	if readiness.Ready {
		return ""
	}
	switch readiness.Reason {
	case transport.HostMutationReasonPackageManager:
		// The package manager's own work does not touch the release read.
		return ""
	case transport.HostMutationReasonPanelOperation, transport.HostMutationReasonAgentMutation,
		transport.HostMutationReasonHostLock:
		return panelUpdateReasonServerBusy
	default:
		return panelUpdateReasonStateUnverified
	}
}

// panelUpdateDiscoverySkip is the last skipped scheduled check, kept in memory.
type panelUpdateDiscoverySkip struct {
	at     time.Time
	reason string
}

// panelUpdateDiscoveryScheduler runs the scheduled check. Its clock, jitter,
// timer and dependencies are fields so tests drive it without waiting.
type panelUpdateDiscoveryScheduler struct {
	now      func() time.Time
	jitter   func(max time.Duration) time.Duration // uniform in [-max, max]
	newTimer func(time.Duration) (<-chan time.Time, func())
	enabled  func(context.Context) bool
	skip     func(context.Context, time.Time) string
	discover func(context.Context) (panelUpdateCheckResponse, *panelUpdateDiscoveryFailure)
	save     func(context.Context, time.Time, panelUpdateCheckResponse, *panelUpdateDiscoveryFailure)
	flush    func(context.Context, time.Time)
	logf     func(string, ...any)
	// gate waits until no other release check runs (the button's or the one
	// made when the update screen opens); nil in tests that fake discovery.
	gate func(context.Context) (func(), error)

	wake chan struct{}

	mu        sync.Mutex
	next      time.Time // zero while switched off or not scheduled
	failures  int
	logged    string
	lastCheck time.Time // the last completed check, manual or automatic
	lastSkip  panelUpdateDiscoverySkip
}

func newPanelUpdateDiscoveryScheduler(p *Panel) *panelUpdateDiscoveryScheduler {
	source := rand.New(rand.NewSource(time.Now().UnixNano()))
	var sourceMu sync.Mutex
	return &panelUpdateDiscoveryScheduler{
		now: time.Now,
		jitter: func(max time.Duration) time.Duration {
			if max <= 0 {
				return 0
			}
			sourceMu.Lock()
			defer sourceMu.Unlock()
			return time.Duration(source.Int63n(int64(2*max)+1)) - max
		},
		newTimer: func(delay time.Duration) (<-chan time.Time, func()) {
			timer := time.NewTimer(delay)
			return timer.C, func() { timer.Stop() }
		},
		enabled:  p.panelUpdateAutoCheck,
		skip:     p.panelUpdateDiscoverySkipReason,
		discover: p.discoverPanelUpdate,
		save: func(ctx context.Context, now time.Time, response panelUpdateCheckResponse, failure *panelUpdateDiscoveryFailure) {
			p.savePanelUpdateDiscovery(ctx, now, true, response, failure)
		},
		flush: func(ctx context.Context, now time.Time) { p.flushPanelUpdateDiscovery(ctx, now) },
		logf:  log.Printf,
		gate:  p.updateCheckGate.acquire,
		wake:  make(chan struct{}, 1),
	}
}

func (s *panelUpdateDiscoveryScheduler) firstDelay() time.Duration {
	return panelUpdateDiscoveryFirstDelay + s.jitter(panelUpdateDiscoveryFirstJitter)
}

func (s *panelUpdateDiscoveryScheduler) periodDelay() time.Duration {
	return panelUpdateDiscoveryPeriod + s.jitter(panelUpdateDiscoveryJitter)
}

// failureDelay is the wait after the n-th consecutive failed check (n >= 1).
func (s *panelUpdateDiscoveryScheduler) failureDelay(failures int) time.Duration {
	if failures >= 1 && failures <= len(panelUpdateDiscoveryBackoff) {
		return panelUpdateDiscoveryBackoff[failures-1]
	}
	return s.periodDelay()
}

// resumeDelay is the wait when the switch is turned on: the period from the
// last completed check when that is still ahead, otherwise the first delay.
func (s *panelUpdateDiscoveryScheduler) resumeDelay() time.Duration {
	s.mu.Lock()
	last := s.lastCheck
	s.mu.Unlock()
	if !last.IsZero() {
		if remaining := last.Add(s.periodDelay()).Sub(s.now()); remaining > 0 {
			return remaining
		}
	}
	return s.firstDelay()
}

// Next is when the next scheduled check runs; zero while switched off.
func (s *panelUpdateDiscoveryScheduler) Next() time.Time {
	if s == nil {
		return time.Time{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.next
}

// LastSkip is the last skipped scheduled check, if the last attempt was one.
func (s *panelUpdateDiscoveryScheduler) LastSkip() (time.Time, string) {
	if s == nil {
		return time.Time{}, ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastSkip.at, s.lastSkip.reason
}

// NoteChecked records a completed check (the button's too): it is the base
// of the next period when the switch is turned on, and it ends a skip.
func (s *panelUpdateDiscoveryScheduler) NoteChecked(at time.Time) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.lastCheck = at
	s.lastSkip = panelUpdateDiscoverySkip{}
	s.mu.Unlock()
}

func (s *panelUpdateDiscoveryScheduler) noteSkip(at time.Time, reason string) {
	s.mu.Lock()
	s.lastSkip = panelUpdateDiscoverySkip{at: at, reason: reason}
	s.mu.Unlock()
}

func (s *panelUpdateDiscoveryScheduler) setNext(next time.Time) {
	s.mu.Lock()
	s.next = next
	s.mu.Unlock()
}

// Wake tells the loop that the owner's switch changed.
func (s *panelUpdateDiscoveryScheduler) Wake() {
	if s == nil {
		return
	}
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// logState writes one line per change of state, not one per attempt.
func (s *panelUpdateDiscoveryScheduler) logState(state, format string, args ...any) {
	s.mu.Lock()
	changed := s.logged != state
	s.logged = state
	s.mu.Unlock()
	if changed && s.logf != nil {
		s.logf(format, args...)
	}
}

// runOnce runs one due check and returns the wait before the next one. A
// skip is kept in memory only; it never writes.
func (s *panelUpdateDiscoveryScheduler) runOnce(ctx context.Context) time.Duration {
	if !s.enabled(ctx) {
		return s.periodDelay()
	}
	now := s.now()
	if reason := s.skip(ctx, now); reason != "" {
		if ctx.Err() != nil {
			return s.periodDelay()
		}
		s.noteSkip(now, reason)
		s.logState("skipped:"+reason, "[panel-update] automatic update check skipped (%s); next try in 1 hour", reason)
		return panelUpdateDiscoverySkipDelay
	}
	// The server is idle: a result kept in memory while it was busy is
	// written now, before this check.
	s.flush(ctx, now)
	// One release check at a time: a check the button or the update screen
	// started is waited for (it is bounded); this one then runs after it.
	if s.gate != nil {
		leave, err := s.gate(ctx)
		if err != nil {
			return s.periodDelay()
		}
		defer leave()
	}
	checkCtx, cancel := context.WithTimeout(ctx, panelUpdateDiscoveryTimeout)
	response, failure := s.discover(checkCtx)
	cancel()
	if ctx.Err() != nil {
		// Shutting down: nothing is kept for an interrupted check.
		return s.periodDelay()
	}
	if failure != nil && failure.reason == panelUpdateReasonAgentUnavailable {
		s.noteSkip(now, panelUpdateReasonAgentUnavailable)
		s.logState("skipped:"+panelUpdateReasonAgentUnavailable, "[panel-update] automatic update check skipped: the Agent did not answer; next try in 1 hour")
		return panelUpdateDiscoverySkipDelay
	}
	s.NoteChecked(now)
	if failure != nil {
		s.mu.Lock()
		s.failures++
		failures := s.failures
		s.mu.Unlock()
		// save reads admission and readiness again right before the write.
		s.save(ctx, s.now(), panelUpdateCheckResponse{}, failure)
		delay := s.failureDelay(failures)
		s.logState("failed:"+failure.reason, "[panel-update] automatic update check failed (%s): %v; next try in %s", failure.reason, failure.err, delay.Round(time.Minute))
		return delay
	}
	s.mu.Lock()
	s.failures = 0
	s.mu.Unlock()
	s.save(ctx, s.now(), response, nil)
	if response.Available && response.Target != nil {
		s.logState("available:"+response.Target.Version, "[panel-update] a newer signed release is available: %s (nothing is installed by itself)", response.Target.Version)
	} else {
		s.logState("current", "[panel-update] automatic update check: this panel runs the latest signed release")
	}
	return s.periodDelay()
}

// loop runs until ctx ends. While switched off it makes no network read and
// waits for the switch.
func (s *panelUpdateDiscoveryScheduler) loop(ctx context.Context) {
	delay := s.firstDelay()
	for {
		if ctx.Err() != nil {
			s.setNext(time.Time{})
			return
		}
		if !s.enabled(ctx) {
			s.setNext(time.Time{})
			s.logState("off", "[panel-update] automatic update checks are switched off")
			select {
			case <-ctx.Done():
				return
			case <-s.wake:
				delay = s.resumeDelay()
				continue
			}
		}
		next := s.now().Add(delay)
		s.setNext(next)
		fired, stop := s.newTimer(delay)
		select {
		case <-ctx.Done():
			stop()
			s.setNext(time.Time{})
			return
		case <-s.wake:
			stop()
			if remaining := next.Sub(s.now()); remaining > 0 {
				delay = remaining
			} else {
				delay = s.resumeDelay()
			}
		case <-fired:
			delay = s.runOnce(ctx)
		}
	}
}

// preparePanelUpdateDiscovery clears a stale record and creates the
// scheduler. It runs before management opens, so no request reads the
// scheduler while it is assigned.
func (p *Panel) preparePanelUpdateDiscovery(ctx context.Context) {
	p.clearStalePanelUpdateDiscovery(ctx)
	p.updateDiscovery = newPanelUpdateDiscoveryScheduler(p)
}

// startPanelUpdateDiscovery starts the prepared loop.
func (p *Panel) startPanelUpdateDiscovery(ctx context.Context) {
	if p.updateDiscovery == nil {
		p.preparePanelUpdateDiscovery(ctx)
	}
	go p.updateDiscovery.loop(ctx)
}

// panelUpdateCheckGate lets one release check run at a time (alpha.103). A
// caller that finds a check running waits for it to end, bounded by its own
// context; it never runs a second check beside it. The zero value is ready.
// Ayni anda tek surum denetimi: suren denetim beklenir, yanina ikincisi acilmaz.
type panelUpdateCheckGate struct {
	mu      sync.Mutex
	running chan struct{} // closed when the running check ends; nil when idle
}

// acquire returns the release of the gate once no other check runs, or the
// context's error when it ended first.
func (g *panelUpdateCheckGate) acquire(ctx context.Context) (func(), error) {
	for {
		g.mu.Lock()
		if g.running == nil {
			done := make(chan struct{})
			g.running = done
			g.mu.Unlock()
			var once sync.Once
			return func() {
				once.Do(func() {
					g.mu.Lock()
					g.running = nil
					g.mu.Unlock()
					close(done)
				})
			}, nil
		}
		wait := g.running
		g.mu.Unlock()
		select {
		case <-wait:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// Outcomes of the check made when the update screen opens. Closed set.
const (
	panelUpdateRefreshChecked = "checked" // a check ran; its result is the view's last result
	panelUpdateRefreshFresh   = "fresh"   // the known result is younger than the window; no read
	panelUpdateRefreshOff     = "off"     // automatic checks are off; celikpanel.net is not contacted
	panelUpdateRefreshSkipped = "skipped" // a condition was not met (reason_code); nothing kept
	panelUpdateRefreshUnknown = "unknown" // the request ended before a result; nothing kept
)

// panelUpdateDiscoveryRefreshView says what opening the update screen did. It
// is memory only: a skipped or unknown attempt is never stored.
type panelUpdateDiscoveryRefreshView struct {
	Outcome    string `json:"outcome"`
	ReasonCode string `json:"reason_code,omitempty"`
}

// panelUpdateDiscoveryFresh says whether the newest known result is younger
// than the window on the server's clock. A result dated more than a minute in
// the future (a clock set back) is not fresh, so it cannot hold the check off.
func (p *Panel) panelUpdateDiscoveryFresh(ctx context.Context, now time.Time) bool {
	record, ok := p.currentPanelUpdateDiscovery(ctx)
	if !ok {
		return false
	}
	checked, err := time.Parse(time.RFC3339, record.CheckedAt)
	if err != nil {
		return false
	}
	age := now.Sub(checked)
	return age >= -time.Minute && age < panelUpdateDiscoveryOpenFresh
}

// refreshPanelUpdateDiscoveryOnOpen is the check made when an administrator
// opens the update screen (owner observation 2026-10-23: the screen offered
// alpha.101 from the 12-hour schedule while alpha.102 was published). With
// automatic checks on and the known result older than the window, it runs the
// same discovery as the button and the schedule, one at a time with them,
// under the schedule's skip rules (no check while an update was admitted in
// the last hour, while the server reports a change in progress, while setup
// changes the server, or when the Agent does not answer) and its write rules.
// It never installs or starts anything.
// Guncelleme ekrani acildiginda: otomatik denetim aciksa ve bilinen sonuc
// pencereden eskiyse ayni kesif, ayni kurallarla bir kez calisir; hicbir sey
// kurulmaz ya da baslatilmaz.
func (p *Panel) refreshPanelUpdateDiscoveryOnOpen(ctx context.Context) panelUpdateDiscoveryRefreshView {
	if !p.panelUpdateAutoCheck(ctx) {
		return panelUpdateDiscoveryRefreshView{Outcome: panelUpdateRefreshOff}
	}
	if p.panelUpdateDiscoveryFresh(ctx, panelUpdateNow()) {
		return panelUpdateDiscoveryRefreshView{Outcome: panelUpdateRefreshFresh}
	}
	if p.panelUpdateRecentlyAdmitted(panelUpdateNow()) {
		return panelUpdateDiscoveryRefreshView{Outcome: panelUpdateRefreshSkipped, ReasonCode: panelUpdateReasonUpdateActive}
	}
	leave, err := p.updateCheckGate.acquire(ctx)
	if err != nil {
		return panelUpdateDiscoveryRefreshView{Outcome: panelUpdateRefreshUnknown, ReasonCode: panelUpdateReasonTimeout}
	}
	defer leave()
	// A check that ended while this one waited is the fresh result.
	if p.panelUpdateDiscoveryFresh(ctx, panelUpdateNow()) {
		return panelUpdateDiscoveryRefreshView{Outcome: panelUpdateRefreshFresh}
	}
	if reason := p.panelUpdateDiscoverySkipReason(ctx, panelUpdateNow()); reason != "" {
		if ctx.Err() != nil {
			return panelUpdateDiscoveryRefreshView{Outcome: panelUpdateRefreshUnknown, ReasonCode: panelUpdateReasonTimeout}
		}
		return panelUpdateDiscoveryRefreshView{Outcome: panelUpdateRefreshSkipped, ReasonCode: reason}
	}
	checkCtx, cancel := context.WithTimeout(ctx, panelUpdateDiscoveryTimeout)
	response, failure := p.discoverPanelUpdate(checkCtx)
	cancel()
	if ctx.Err() != nil {
		// The request ended: nothing is kept for an interrupted check.
		return panelUpdateDiscoveryRefreshView{Outcome: panelUpdateRefreshUnknown, ReasonCode: panelUpdateReasonTimeout}
	}
	if failure != nil && failure.reason == panelUpdateReasonAgentUnavailable {
		// As on the schedule: an Agent that does not answer is a skip.
		return panelUpdateDiscoveryRefreshView{Outcome: panelUpdateRefreshSkipped, ReasonCode: panelUpdateReasonAgentUnavailable}
	}
	now := panelUpdateNow()
	p.savePanelUpdateDiscovery(ctx, now, true, response, failure)
	p.updateDiscovery.NoteChecked(now)
	if failure != nil {
		log.Printf("[panel-update] update check on opening the update screen failed (%s): %v", failure.reason, failure.err)
	}
	return panelUpdateDiscoveryRefreshView{Outcome: panelUpdateRefreshChecked}
}

// handlePanelUpdateDiscoveryRefresh answers GET with the known result after
// the check made when the update screen opens (administrators only). The
// answer has the discovery read's shape plus what the opening did.
func (p *Panel) handlePanelUpdateDiscoveryRefresh(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		rejectRouteMethod(w, []string{http.MethodGet})
		return
	}
	if !requirePanelUpdateAdmin(w, r) {
		return
	}
	if p == nil || p.db == nil {
		writeCodedError(w, http.StatusServiceUnavailable, "PANEL_UPDATE_DISCOVERY_UNAVAILABLE", "the stored update check could not be read", "")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), panelUpdateDiscoveryOpenTimeout)
	refresh := p.refreshPanelUpdateDiscoveryOnOpen(ctx)
	cancel()
	if r.Context().Err() != nil {
		return
	}
	view := p.panelUpdateDiscoveryView(r.Context(), panelUpdateNow())
	view.Refresh = &refresh
	_ = json.NewEncoder(w).Encode(view)
}

// panelUpdateDiscoveryResponse is the known result, read without any Agent
// call. Ages and the wait to the next check are measured on the server's
// clock when the answer is written, so the browser never compares two clocks
// (D-032).
type panelUpdateDiscoveryResponse struct {
	AutoCheck              bool                        `json:"auto_check"`
	IntervalHours          int                         `json:"interval_hours"`
	CurrentVersion         string                      `json:"current_version"`
	CurrentCommit          string                      `json:"current_commit"`
	NextCheckInSeconds     *int64                      `json:"next_check_in_seconds,omitempty"`
	Last                   *panelUpdateDiscoveryRecord `json:"last,omitempty"`
	CheckedAgeSeconds      *int64                      `json:"checked_age_seconds,omitempty"`
	FoundAgeSeconds        *int64                      `json:"found_age_seconds,omitempty"`
	Available              bool                        `json:"available"`
	PreviousAttempt        *recoveryobs.Attempt        `json:"previous_attempt,omitempty"`
	ExpectedRestartSeconds int                         `json:"expected_restart_seconds"`
	// The last scheduled attempt, when it was skipped after the result above
	// (memory only; never stored).
	LastSkip *panelUpdateDiscoverySkipView `json:"last_skip,omitempty"`
	// What opening the update screen did (alpha.103); only on the refresh
	// path's answer.
	Refresh *panelUpdateDiscoveryRefreshView `json:"refresh,omitempty"`
}

type panelUpdateDiscoverySkipView struct {
	ReasonCode string `json:"reason_code"`
	AgeSeconds int64  `json:"age_seconds"`
}

func panelUpdateAgeSeconds(now time.Time, value string) *int64 {
	moment, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil
	}
	age := int64(now.Sub(moment) / time.Second)
	if age < 0 {
		age = 0
	}
	return &age
}

func (p *Panel) panelUpdateDiscoveryView(ctx context.Context, now time.Time) panelUpdateDiscoveryResponse {
	response := panelUpdateDiscoveryResponse{
		AutoCheck:      p.panelUpdateAutoCheck(ctx),
		IntervalHours:  int(panelUpdateDiscoveryPeriod / time.Hour),
		CurrentVersion: buildVersion, CurrentCommit: buildCommit,
		ExpectedRestartSeconds: panelUpdateExpectedRestartSeconds(p.lastPanelUpdateRestart(ctx).Seconds),
	}
	if response.AutoCheck {
		if next := p.updateDiscovery.Next(); !next.IsZero() {
			wait := int64(next.Sub(now) / time.Second)
			if wait < 0 {
				wait = 0
			}
			response.NextCheckInSeconds = &wait
		}
	}
	record, ok := p.currentPanelUpdateDiscovery(ctx)
	if ok {
		response.Last = &record
		response.CheckedAgeSeconds = panelUpdateAgeSeconds(now, record.CheckedAt)
		if record.Target != nil {
			response.Available = true
			response.FoundAgeSeconds = panelUpdateAgeSeconds(now, record.FoundAt)
			response.PreviousAttempt = p.previousPanelUpdateAttempt(record.Target.Commit)
		}
	}
	if at, reason := p.updateDiscovery.LastSkip(); reason != "" {
		checked, err := time.Parse(time.RFC3339, record.CheckedAt)
		if !ok || err != nil || at.After(checked) {
			age := int64(now.Sub(at) / time.Second)
			if age < 0 {
				age = 0
			}
			response.LastSkip = &panelUpdateDiscoverySkipView{ReasonCode: reason, AgeSeconds: age}
		}
	}
	return response
}

type panelUpdateDiscoverySettingRequest struct {
	AutoCheck *bool `json:"auto_check"`
}

// handlePanelUpdateDiscovery answers GET with the known result (no Agent
// call, no network read) and PUT with the owner's switch.
func (p *Panel) handlePanelUpdateDiscovery(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet && r.Method != http.MethodPut {
		rejectRouteMethod(w, []string{http.MethodGet, http.MethodPut})
		return
	}
	if !requirePanelUpdateAdmin(w, r) {
		return
	}
	if p == nil || p.db == nil {
		writeCodedError(w, http.StatusServiceUnavailable, "PANEL_UPDATE_DISCOVERY_UNAVAILABLE", "the stored update check could not be read", "")
		return
	}
	if r.Method == http.MethodPut {
		var request panelUpdateDiscoverySettingRequest
		if err := decodeServiceOperationJSON(w, r, &request); err != nil || request.AutoCheck == nil {
			writeCodedError(w, http.StatusBadRequest, "PANEL_UPDATE_INVALID_REQUEST", "auto_check must be true or false", "")
			return
		}
		value := "on"
		if !*request.AutoCheck {
			value = "off"
		}
		if err := p.setSetting(r.Context(), panelUpdateAutoCheckKey, value); err != nil {
			log.Printf("[panel-update] automatic check setting not saved: %v", err)
			writeCodedError(w, http.StatusInternalServerError, "PANEL_UPDATE_SETTING_FAILED", "the setting could not be saved", "")
			return
		}
		p.updateDiscovery.Wake()
		// The loop sets its next time after the wake; answer with the
		// switch as saved. The page reads the next time again.
	}
	_ = json.NewEncoder(w).Encode(p.panelUpdateDiscoveryView(r.Context(), panelUpdateNow()))
}

// comparePanelUpdateVersions orders two release versions by semantic
// version precedence: -1, 0 or 1, and false when either is not a release
// version.
func comparePanelUpdateVersions(left, right string) (int, bool) {
	if !validPanelUpdateVersion(left) || !validPanelUpdateVersion(right) {
		return 0, false
	}
	split := func(version string) ([3]uint64, []string, bool) {
		var core [3]uint64
		body, pre, hasPre := strings.Cut(strings.TrimPrefix(version, "v"), "-")
		parts := strings.Split(body, ".")
		if len(parts) != 3 {
			return core, nil, false
		}
		for index, part := range parts {
			value, err := strconv.ParseUint(part, 10, 64)
			if err != nil {
				return core, nil, false
			}
			core[index] = value
		}
		if !hasPre {
			return core, nil, true
		}
		return core, strings.Split(pre, "."), true
	}
	leftCore, leftPre, okLeft := split(left)
	rightCore, rightPre, okRight := split(right)
	if !okLeft || !okRight {
		return 0, false
	}
	for index := range leftCore {
		if leftCore[index] != rightCore[index] {
			if leftCore[index] < rightCore[index] {
				return -1, true
			}
			return 1, true
		}
	}
	switch {
	case len(leftPre) == 0 && len(rightPre) == 0:
		return 0, true
	case len(leftPre) == 0:
		return 1, true
	case len(rightPre) == 0:
		return -1, true
	}
	for index := 0; index < len(leftPre) && index < len(rightPre); index++ {
		a, b := leftPre[index], rightPre[index]
		if a == b {
			continue
		}
		an, aErr := strconv.ParseUint(a, 10, 64)
		bn, bErr := strconv.ParseUint(b, 10, 64)
		switch {
		case aErr == nil && bErr == nil:
			if an < bn {
				return -1, true
			}
			return 1, true
		case aErr == nil:
			return -1, true
		case bErr == nil:
			return 1, true
		case a < b:
			return -1, true
		default:
			return 1, true
		}
	}
	switch {
	case len(leftPre) < len(rightPre):
		return -1, true
	case len(leftPre) > len(rightPre):
		return 1, true
	}
	return 0, true
}
