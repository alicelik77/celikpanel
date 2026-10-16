package main

import (
	"context"
	"encoding/json"
	"log"
	"math"
	"strconv"
	"time"

	"github.com/alicelik/celikpanel/internal/recoveryobs"
	"github.com/alicelik/celikpanel/internal/transport"
)

// The update's plan, as the update page draws it: one bar per step (owner
// design 2026-10-16, after updating the second installed server from
// alpha.85 to alpha.87). The server declares the steps when the update is
// admitted and reports each one's state with every status answer. It is
// additive JSON on the start and status answers; an older page ignores it.
//
// The steps follow the real phases of an update and nothing else:
//
//   - download: the installer (get.sh) downloads the signed archive. Byte
//     progress is reported when the Agent can match the archive being written
//     (cmd/agent system_update_progress_linux.go); otherwise the step is
//     indeterminate, because the installer reports nothing about it.
//   - verify: the installer checks the archive's size and SHA-256 against the
//     signed manifest and every extracted file against its checksums. It ends
//     when the installer publishes this request's release floor, which it
//     does only after all of that (the Agent's verified_at).
//   - prepare: the updater (update.sh) checks the server once more before it
//     changes anything. The Panel still answers; nothing more fine-grained
//     can be read.
//   - restart: the updater freezes and stops the Panel and the Agent, installs
//     the new release and starts it. The Panel cannot answer during this
//     step, so installing the files and restarting are one step: from outside
//     they cannot be told apart. The page shows it against the expected
//     duration below. It is done when this Panel answering is the target
//     build; its end is when this Panel's management opened.
//   - confirm: the Agent's worker checks the installed build and the final
//     state of the release; the record turns succeeded.
//
// Moments are the server's clock. Each step that has a start also has
// elapsed_seconds, measured on the server's clock when the answer is written,
// so the browser never compares two clocks (D-032).
//
// Guncellemenin plani: adimlar guncellemenin gercek asamalarini izler;
// yeniden baslatma sirasinda Panel yanit veremez, kurulum ve yeniden baslatma
// tek adimdir. Anlar sunucu saatidir; gecen sure de sunucuda olculur.

const (
	panelUpdateStepDownload = "download"
	panelUpdateStepVerify   = "verify"
	panelUpdateStepPrepare  = "prepare"
	panelUpdateStepRestart  = "restart"
	panelUpdateStepConfirm  = "confirm"

	panelUpdateStepPending = "pending"
	panelUpdateStepRunning = "running"
	panelUpdateStepDone    = "done"
	panelUpdateStepFailed  = "failed"
	// retrying (additive, 2026-10-16): the update's own last step did not
	// confirm and the server's automatic recovery owns the request and is
	// repeating it (a forward completion). An older page does not know it and
	// ignores the plan, keeping its previous wording.
	panelUpdateStepRetrying = "retrying"

	// The native recovery timer admits at most three automatic attempts.
	panelUpdateRecoveryMaxAttempts = 3

	// The restart step's expected length when no update was measured on this
	// server: the eighteenth native record (set16) measured the Panel's API
	// unanswered for at most 32 s and whole updates of 48 to 51 s; 90 s is the
	// floor of the expected length either way.
	panelUpdateRestartDefaultSeconds = 90
	// The hard bound of the restart (the page's window: 6 minutes from the
	// update's start). The bar never claims more; the page's overdue wording
	// comes after it.
	panelUpdateRestartBoundSeconds = 360
	// The last measured restart, kept in panel_settings (additive key).
	panelUpdateLastRestartKey = "panel_update_last_restart"
)

var panelUpdateStepOrder = [...]string{
	panelUpdateStepDownload, panelUpdateStepVerify, panelUpdateStepPrepare,
	panelUpdateStepRestart, panelUpdateStepConfirm,
}

type panelUpdateStep struct {
	ID             string   `json:"id"`
	State          string   `json:"state"`
	StartedAt      string   `json:"started_at,omitempty"`
	FinishedAt     string   `json:"finished_at,omitempty"`
	ElapsedSeconds *int64   `json:"elapsed_seconds,omitempty"`
	Progress       *float64 `json:"progress,omitempty"`
	// Attempt and MaxAttempts accompany a retrying step only when the attempt
	// is known from the recovery record (the first automatic attempt).
	Attempt     int `json:"attempt,omitempty"`
	MaxAttempts int `json:"max_attempts,omitempty"`
}

type panelUpdatePlan struct {
	Steps []panelUpdateStep `json:"steps"`
	// The length the page gives the restart bar: the last measured restart on
	// this server times 1.5, at least 90 s, at most the 6-minute bound.
	ExpectedRestartSeconds int `json:"expected_restart_seconds"`
	RestartBoundSeconds    int `json:"restart_bound_seconds"`
}

// panelUpdatePlanInput is everything the plan is derived from. Zero times are
// unknown.
type panelUpdatePlanInput struct {
	Status        string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	ArchiveSize   uint64
	Downloaded    uint64
	HaveBytes     bool
	DownloadedAt  time.Time
	VerifiedAt    time.Time
	RunningTarget bool
	PanelOpenedAt time.Time
	Now           time.Time
	Expected      int
	// From the native recovery record of a failed update record (zero when
	// none was read): recovery owns the request and repeats its last step, or
	// it verified the update.
	RecoveryOwns     bool
	RecoveryAttempt  int
	RecoveryVerified bool
	RecoveryAt       time.Time
}

func panelUpdateExpectedRestartSeconds(measured int) int {
	expected := panelUpdateRestartDefaultSeconds
	if measured > 0 {
		if scaled := int(math.Ceil(float64(measured) * 1.5)); scaled > expected {
			expected = scaled
		}
	}
	if expected > panelUpdateRestartBoundSeconds {
		expected = panelUpdateRestartBoundSeconds
	}
	return expected
}

// buildPanelUpdatePlan names as finished only what the evidence shows:
//
//	nothing             -> download running
//	downloaded_at       -> download done, verify running
//	verified_at         -> download and verify done, prepare running
//	this Panel = target -> prepare and restart done, confirm running
//	succeeded           -> every step done
//
// A failed record marks a step failed only where the evidence places the
// failure: on this Panel being the target build, the confirmation. Otherwise
// the failure is not placed on a step (the page shows the failure's own
// wording) and the steps not proven stay pending. While the server's own
// recovery owns that failed request and repeats the last step, the
// confirmation is retrying, not failed; a recovery that verified the update
// makes every step done (Frankfurt 2026-10-16).
func buildPanelUpdatePlan(in panelUpdatePlanInput) panelUpdatePlan {
	steps := make([]panelUpdateStep, len(panelUpdateStepOrder))
	for index, id := range panelUpdateStepOrder {
		steps[index] = panelUpdateStep{ID: id, State: panelUpdateStepPending}
	}
	start := map[string]time.Time{panelUpdateStepDownload: in.CreatedAt}
	finish := map[string]time.Time{}
	reached := 0 // the index of the first step not proven finished
	if !in.DownloadedAt.IsZero() {
		reached = 1
		finish[panelUpdateStepDownload] = in.DownloadedAt
		start[panelUpdateStepVerify] = in.DownloadedAt
	}
	if !in.VerifiedAt.IsZero() {
		reached = 2
		finish[panelUpdateStepVerify] = in.VerifiedAt
		start[panelUpdateStepPrepare] = in.VerifiedAt
	}
	if in.RunningTarget {
		reached = 4
		if !in.PanelOpenedAt.IsZero() && !in.PanelOpenedAt.Before(in.CreatedAt) {
			finish[panelUpdateStepRestart] = in.PanelOpenedAt
			start[panelUpdateStepConfirm] = in.PanelOpenedAt
		}
	}
	switch in.Status {
	case "succeeded":
		reached = len(steps)
		if !in.UpdatedAt.IsZero() {
			finish[panelUpdateStepConfirm] = in.UpdatedAt
		}
	case "failed":
		switch {
		case in.RunningTarget && in.RecoveryVerified:
			reached = len(steps)
			if !in.RecoveryAt.IsZero() {
				finish[panelUpdateStepConfirm] = in.RecoveryAt
			}
		case in.RunningTarget && in.RecoveryOwns:
			steps[4].State = panelUpdateStepRetrying
			if in.RecoveryAttempt > 0 {
				steps[4].Attempt, steps[4].MaxAttempts = in.RecoveryAttempt, panelUpdateRecoveryMaxAttempts
			}
		case in.RunningTarget:
			steps[4].State = panelUpdateStepFailed
		}
	}
	for index := range steps {
		step := &steps[index]
		started, hasStart := start[step.ID]
		finished, hasFinish := finish[step.ID]
		hasStart = hasStart && !started.IsZero()
		hasFinish = hasFinish && !finished.IsZero()
		switch {
		case index < reached:
			step.State = panelUpdateStepDone
		case index == reached && (in.Status == "queued" || in.Status == "running"):
			step.State = panelUpdateStepRunning
		}
		if step.State == panelUpdateStepPending {
			continue
		}
		if hasStart {
			step.StartedAt = started.UTC().Format(time.RFC3339)
		}
		if step.State == panelUpdateStepDone && hasFinish {
			step.FinishedAt = finished.UTC().Format(time.RFC3339)
		}
		var end time.Time
		switch {
		case step.State == panelUpdateStepRunning || step.State == panelUpdateStepRetrying:
			end = in.Now
		case hasFinish:
			end = finished
		}
		if hasStart && !end.IsZero() {
			elapsed := int64(end.Sub(started) / time.Second)
			if elapsed < 0 {
				elapsed = 0
			}
			step.ElapsedSeconds = &elapsed
		}
		if step.ID == panelUpdateStepDownload && step.State == panelUpdateStepRunning &&
			in.HaveBytes && in.ArchiveSize > 0 {
			progress := float64(in.Downloaded) / float64(in.ArchiveSize)
			if progress > 1 {
				progress = 1
			}
			step.Progress = &progress
		}
	}
	expected := in.Expected
	if expected <= 0 {
		expected = panelUpdateExpectedRestartSeconds(0)
	}
	return panelUpdatePlan{Steps: steps, ExpectedRestartSeconds: expected, RestartBoundSeconds: panelUpdateRestartBoundSeconds}
}

// The plan the start answer declares: the download has begun, nothing else.
func (p *Panel) admittedPanelUpdatePlan(ctx context.Context, now time.Time) *panelUpdatePlan {
	plan := buildPanelUpdatePlan(panelUpdatePlanInput{
		Status: "queued", CreatedAt: now, Now: now,
		Expected: panelUpdateExpectedRestartSeconds(p.lastPanelUpdateRestart(ctx).Seconds),
	})
	return &plan
}

// panelUpdatePlanFromStatus derives the plan of a status answer, and records
// the restart duration once this Panel, the new build, can measure it.
func (p *Panel) panelUpdatePlanFromStatus(ctx context.Context, reply transport.SystemUpdateStatusResponse, target panelUpdateTarget, now time.Time) *panelUpdatePlan {
	in := panelUpdatePlanInput{
		Status:        reply.Status,
		CreatedAt:     parsePanelUpdateTime(reply.CreatedAt),
		UpdatedAt:     parsePanelUpdateTime(reply.UpdatedAt),
		DownloadedAt:  parsePanelUpdateTime(reply.DownloadedAt),
		VerifiedAt:    parsePanelUpdateTime(reply.VerifiedAt),
		RunningTarget: target.Version == buildVersion && target.Commit == buildCommit,
		PanelOpenedAt: p.panelOpenedAt(),
		Now:           now,
	}
	if size, err := strconv.ParseUint(target.ArchiveSize, 10, 64); err == nil {
		in.ArchiveSize = size
	}
	if reply.DownloadedBytes != "" {
		if downloaded, err := strconv.ParseUint(reply.DownloadedBytes, 10, 64); err == nil {
			in.Downloaded, in.HaveBytes = downloaded, true
		}
	}
	if reply.Status == "failed" && in.RunningTarget {
		applyPanelUpdateRecovery(&in, p.readPanelUpdateRecovery(reply.RequestID))
	}
	last := p.lastPanelUpdateRestart(ctx)
	if in.RunningTarget && (in.Status == "running" || in.Status == "succeeded") {
		last = p.recordPanelUpdateRestart(ctx, reply.RequestID, in, last)
	}
	in.Expected = panelUpdateExpectedRestartSeconds(last.Seconds)
	plan := buildPanelUpdatePlan(in)
	return &plan
}

// readPanelUpdateRecovery reads the request's native recovery record
// read-only; it never starts, retries or unlocks anything.
func (p *Panel) readPanelUpdateRecovery(requestID string) recoveryobs.Status {
	read := recoveryobs.Read
	if p != nil && p.recoveryStatus != nil {
		read = p.recoveryStatus
	}
	if !recoveryobs.ValidRequestID(requestID) {
		return recoveryobs.Status{}
	}
	return read(requestID)
}

// applyPanelUpdateRecovery derives the plan's view of a failed update from its
// recovery record. Recovery owns the request while the record is recovering or
// failed without a terminal proof (a read-only preflight stop excepted: it is
// final), or while it waits between automatic attempts. The attempt is known
// only for the first one (recovering right after the update's own failure).
func applyPanelUpdateRecovery(in *panelUpdatePlanInput, status recoveryobs.Status) {
	if status.Observation != "known" {
		return
	}
	if status.Phase == "succeeded" && status.TerminalProof == "update_verified" {
		in.RecoveryVerified = true
		in.RecoveryAt = parsePanelUpdateTime(status.ObservedAt)
		return
	}
	if status.TerminalProof != "none" {
		return
	}
	preflight := status.PreviousFailure == "update_failed" &&
		(status.FailureCode == "recovery_runtime_preflight_failed" || status.FailureCode == "update_preflight_refused")
	switch {
	case preflight:
	case status.Phase == "recovering" || status.Phase == "failed":
		in.RecoveryOwns = true
		if status.Phase == "recovering" && status.PreviousFailure == "update_failed" {
			in.RecoveryAttempt = 1
		}
	case status.Phase == "recovery_required" &&
		(status.AutomaticRecovery == "retry_scheduled" || status.AutomaticRecovery == "pause_pending"):
		in.RecoveryOwns = true
	}
}

func parsePanelUpdateTime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}
	}
	return parsed.UTC()
}

type panelUpdateRestartRecord struct {
	RequestID string `json:"request_id"`
	Seconds   int    `json:"seconds"`
}

func (p *Panel) lastPanelUpdateRestart(ctx context.Context) panelUpdateRestartRecord {
	var record panelUpdateRestartRecord
	if p == nil || p.db == nil {
		return record
	}
	raw := p.setting(ctx, panelUpdateLastRestartKey)
	if raw == "" || json.Unmarshal([]byte(raw), &record) != nil ||
		!panelUpdateRequestIDPattern.MatchString(record.RequestID) || record.Seconds <= 0 || record.Seconds > 3600 {
		return panelUpdateRestartRecord{}
	}
	return record
}

// recordPanelUpdateRestart keeps the measured restart of this update: from
// the moment the installer finished verifying the package (verified_at) to
// the moment this Panel's management opened. That span also holds the
// updater's own checks before it stops the Panel, so it is at least the time
// the page cannot reach the Panel. It is written once per update (the request
// ID is kept with it) and is a bookkeeping value only: it starts nothing and
// is never an input to admission or recovery.
// Olculen yeniden baslama suresi guncelleme basina bir kez yazilir; yalniz
// cubugun beklenen uzunlugu icindir.
func (p *Panel) recordPanelUpdateRestart(ctx context.Context, requestID string, in panelUpdatePlanInput, last panelUpdateRestartRecord) panelUpdateRestartRecord {
	if p == nil || p.db == nil || last.RequestID == requestID || in.VerifiedAt.IsZero() || in.PanelOpenedAt.IsZero() ||
		!panelUpdateRequestIDPattern.MatchString(requestID) {
		return last
	}
	seconds := int(math.Ceil(in.PanelOpenedAt.Sub(in.VerifiedAt).Seconds()))
	if seconds <= 0 || seconds > 3600 {
		return last
	}
	record := panelUpdateRestartRecord{RequestID: requestID, Seconds: seconds}
	raw, err := json.Marshal(record)
	if err != nil {
		return last
	}
	if err := p.setSetting(ctx, panelUpdateLastRestartKey, string(raw)); err != nil {
		log.Printf("[panel-update] restart duration not recorded: %v", err)
		return last
	}
	return record
}

// When this Panel's management opened (the startup gate), zero when unknown.
func (p *Panel) panelOpenedAt() time.Time {
	if p == nil || p.startupGate == nil {
		return time.Time{}
	}
	return p.startupGate.OpenedAt()
}
