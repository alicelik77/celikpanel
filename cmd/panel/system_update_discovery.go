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
// manifest. It is read-only: no host change, no lock, no update record. It
// runs about two minutes after the Panel starts, then every 12 hours with
// half an hour of jitter; after a failure the next try is in 1 hour, then 3,
// then back to 12. It never runs while an update was just admitted by this
// Panel or while the server is busy with a change (an update holds the host
// lock), during a server setup run, while the Panel shuts down, or while the
// owner has switched it off; it is skipped, not queued, when the Agent does
// not answer.
//
// Guncelleme kesfi: Panel imzali yeni surumu kendisi arar, buldugunu saklar
// ve panel bunu sakin bicimde soyler. Guncellemeyi asla baslatmaz; yalniz
// yonetici, panelin kendi guncelleme ekranindan baslatir. Zamanli kontrol
// "Guncellemeleri denetle" dugmesinin yaptigi salt-okur okumanin aynisidir.

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
	// How long a start this Panel admitted counts as an active update: the
	// update's restart bound (system_update_plan.go). The update's host lock
	// covers the rest of it.
	panelUpdateActiveWindow = panelUpdateRestartBoundSeconds * time.Second

	panelUpdateOutcomeAvailable = "available"
	panelUpdateOutcomeCurrent   = "current"
	panelUpdateOutcomeFailed    = "failed"
	panelUpdateOutcomeSkipped   = "skipped"
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
// panel_settings. Target is the found release; after a failed or skipped
// check the last found target is kept with the moment it was found, so a
// passing outage does not hide an update that is still waiting.
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
	case panelUpdateOutcomeFailed, panelUpdateOutcomeSkipped:
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

// relevantPanelUpdateDiscovery says whether a stored record still describes
// this build: made by it, and any target newer than it.
func relevantPanelUpdateDiscovery(record panelUpdateDiscoveryRecord) bool {
	if record.CurrentVersion != buildVersion || record.CurrentCommit != buildCommit {
		return false
	}
	if record.Target != nil {
		if record.Target.Commit == buildCommit {
			return false
		}
		if order, ok := comparePanelUpdateVersions(record.Target.Version, buildVersion); !ok || order <= 0 {
			return false
		}
	}
	return true
}

func (p *Panel) loadPanelUpdateDiscovery(ctx context.Context) (panelUpdateDiscoveryRecord, bool) {
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

// clearStalePanelUpdateDiscovery removes, at start, a record made by another
// build or naming a release this build already is or is newer than (after
// an update). A bookkeeping write only.
func (p *Panel) clearStalePanelUpdateDiscovery(ctx context.Context) {
	if p == nil || p.db == nil || p.setting(ctx, panelUpdateDiscoveryKey) == "" {
		return
	}
	if _, ok := p.loadPanelUpdateDiscovery(ctx); ok {
		return
	}
	if _, err := p.db.GetDB().ExecContext(ctx, `DELETE FROM panel_settings WHERE key = ?`, panelUpdateDiscoveryKey); err != nil {
		log.Printf("[panel-update] earlier update check could not be cleared: %v", err)
	}
}

// recordPanelUpdateDiscovery writes the result of one check, manual or
// automatic. A failed or skipped check keeps the last found target.
func (p *Panel) recordPanelUpdateDiscovery(ctx context.Context, now time.Time, automatic bool, response panelUpdateCheckResponse, failure *panelUpdateDiscoveryFailure, skipped string) panelUpdateDiscoveryRecord {
	record := panelUpdateDiscoveryRecord{
		CheckedAt: now.UTC().Format(time.RFC3339), Automatic: automatic,
		CurrentVersion: buildVersion, CurrentCommit: buildCommit,
	}
	if p == nil || p.db == nil {
		return record
	}
	p.updateDiscoveryMu.Lock()
	defer p.updateDiscoveryMu.Unlock()
	previous, havePrevious := p.loadPanelUpdateDiscovery(ctx)
	switch {
	case skipped != "":
		record.Outcome, record.ReasonCode = panelUpdateOutcomeSkipped, skipped
	case failure != nil:
		record.Outcome, record.ReasonCode = panelUpdateOutcomeFailed, failure.reason
	case response.Available && response.Target != nil:
		target := *response.Target
		record.Outcome, record.Target, record.FoundAt = panelUpdateOutcomeAvailable, &target, record.CheckedAt
		// The same release found again keeps the moment it was first found.
		if havePrevious && previous.Target != nil && samePanelUpdateTarget(*previous.Target, target) {
			record.FoundAt = previous.FoundAt
		}
	default:
		record.Outcome = panelUpdateOutcomeCurrent
	}
	if record.Outcome == panelUpdateOutcomeFailed || record.Outcome == panelUpdateOutcomeSkipped {
		if havePrevious && previous.Target != nil {
			target := *previous.Target
			record.Target, record.FoundAt = &target, previous.FoundAt
		}
	}
	if !validPanelUpdateDiscoveryRecord(record) || !relevantPanelUpdateDiscovery(record) {
		// A target that is not newer than this build is not an update.
		if record.Outcome == panelUpdateOutcomeAvailable {
			record.Outcome, record.Target, record.FoundAt = panelUpdateOutcomeCurrent, nil, ""
		} else {
			record.Target, record.FoundAt = nil, ""
		}
		if !validPanelUpdateDiscoveryRecord(record) {
			return record
		}
	}
	raw, err := json.Marshal(record)
	if err == nil {
		err = p.setSetting(ctx, panelUpdateDiscoveryKey, string(raw))
	}
	if err != nil {
		log.Printf("[panel-update] update check result not recorded: %v", err)
	}
	return record
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
// now, or "" when it may. Reads only.
func (p *Panel) panelUpdateDiscoverySkipReason(ctx context.Context, now time.Time) string {
	if p.panelUpdateRecentlyAdmitted(now) {
		return panelUpdateReasonUpdateActive
	}
	if p.agentClient == nil {
		return panelUpdateReasonAgentUnavailable
	}
	if p.db != nil {
		active, err := p.serverSetupExecutionActive(ctx)
		if err != nil {
			return panelUpdateReasonStateUnverified
		}
		if active {
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

// panelUpdateDiscoveryScheduler runs the scheduled check. Its clock, jitter,
// timer and dependencies are fields so tests drive it without waiting.
type panelUpdateDiscoveryScheduler struct {
	now      func() time.Time
	jitter   func(max time.Duration) time.Duration // uniform in [-max, max]
	newTimer func(time.Duration) (<-chan time.Time, func())
	enabled  func(context.Context) bool
	skip     func(context.Context, time.Time) string
	discover func(context.Context) (panelUpdateCheckResponse, *panelUpdateDiscoveryFailure)
	record   func(context.Context, time.Time, panelUpdateCheckResponse, *panelUpdateDiscoveryFailure, string)
	logf     func(string, ...any)

	wake chan struct{}

	mu       sync.Mutex
	next     time.Time // zero while switched off or not scheduled
	failures int
	logged   string
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
		record: func(ctx context.Context, now time.Time, response panelUpdateCheckResponse, failure *panelUpdateDiscoveryFailure, skipped string) {
			p.recordPanelUpdateDiscovery(ctx, now, true, response, failure, skipped)
		},
		logf: log.Printf,
		wake: make(chan struct{}, 1),
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

// Next is when the next scheduled check runs; zero while switched off.
func (s *panelUpdateDiscoveryScheduler) Next() time.Time {
	if s == nil {
		return time.Time{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.next
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

// runOnce runs one due check and returns the wait before the next one.
func (s *panelUpdateDiscoveryScheduler) runOnce(ctx context.Context) time.Duration {
	if !s.enabled(ctx) {
		return s.periodDelay()
	}
	now := s.now()
	if reason := s.skip(ctx, now); reason != "" {
		if ctx.Err() != nil {
			return s.periodDelay()
		}
		s.record(ctx, now, panelUpdateCheckResponse{}, nil, reason)
		s.logState("skipped:"+reason, "[panel-update] automatic update check skipped (%s); next try in 1 hour", reason)
		return panelUpdateDiscoverySkipDelay
	}
	checkCtx, cancel := context.WithTimeout(ctx, panelUpdateDiscoveryTimeout)
	response, failure := s.discover(checkCtx)
	cancel()
	if ctx.Err() != nil {
		// Shutting down: nothing is recorded for an interrupted check.
		return s.periodDelay()
	}
	if failure != nil && failure.reason == panelUpdateReasonAgentUnavailable {
		s.record(ctx, now, panelUpdateCheckResponse{}, nil, panelUpdateReasonAgentUnavailable)
		s.logState("skipped:"+panelUpdateReasonAgentUnavailable, "[panel-update] automatic update check skipped: the Agent did not answer; next try in 1 hour")
		return panelUpdateDiscoverySkipDelay
	}
	if failure != nil {
		s.mu.Lock()
		s.failures++
		failures := s.failures
		s.mu.Unlock()
		s.record(ctx, now, panelUpdateCheckResponse{}, failure, "")
		delay := s.failureDelay(failures)
		s.logState("failed:"+failure.reason, "[panel-update] automatic update check failed (%s): %v; next try in %s", failure.reason, failure.err, delay.Round(time.Minute))
		return delay
	}
	s.mu.Lock()
	s.failures = 0
	s.mu.Unlock()
	s.record(ctx, now, response, nil, "")
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
				delay = s.firstDelay()
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
				delay = s.firstDelay()
			}
		case <-fired:
			delay = s.runOnce(ctx)
		}
	}
}

// startPanelUpdateDiscovery clears a stale record and starts the loop.
func (p *Panel) startPanelUpdateDiscovery(ctx context.Context) {
	p.clearStalePanelUpdateDiscovery(ctx)
	scheduler := newPanelUpdateDiscoveryScheduler(p)
	p.updateDiscovery = scheduler
	go scheduler.loop(ctx)
}

// panelUpdateDiscoveryResponse is the stored result, read without any Agent
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
	if record, ok := p.loadPanelUpdateDiscovery(ctx); ok {
		response.Last = &record
		response.CheckedAgeSeconds = panelUpdateAgeSeconds(now, record.CheckedAt)
		if record.Target != nil {
			response.Available = true
			response.FoundAgeSeconds = panelUpdateAgeSeconds(now, record.FoundAt)
			response.PreviousAttempt = p.previousPanelUpdateAttempt(record.Target.Commit)
		}
	}
	return response
}

type panelUpdateDiscoverySettingRequest struct {
	AutoCheck *bool `json:"auto_check"`
}

// handlePanelUpdateDiscovery answers GET with the stored result (no Agent
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
