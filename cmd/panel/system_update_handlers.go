package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/recoveryobs"
	"github.com/alicelik/celikpanel/internal/transport"
)

const (
	panelUpdateCheckPath   = "/api/v1/panel/update/check"
	panelUpdateStartPath   = "/api/v1/panel/update/start"
	panelUpdateAbandonPath = "/api/v1/panel/update/abandon"
	panelUpdateStatusPath  = "/api/v1/panel/update/status"
)

var (
	panelUpdateRequestIDPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)
	panelUpdateCommitPattern    = regexp.MustCompile(`^[a-f0-9]{40}$`)
	panelUpdateSHA256Pattern    = regexp.MustCompile(`^[a-f0-9]{64}$`)
	panelUpdateVersionPattern   = regexp.MustCompile(`^v(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)(?:-(?:[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$`)
)

const panelUpdateMaximumArchiveSize uint64 = 2 * 1024 * 1024 * 1024

type panelUpdateTarget struct {
	Version       string `json:"version"`
	Commit        string `json:"commit"`
	Sequence      string `json:"sequence"`
	OS            string `json:"os"`
	Arch          string `json:"arch"`
	ArchiveSHA256 string `json:"archive_sha256"`
	ArchiveSize   string `json:"archive_size"`
	PublishedAt   string `json:"published_at,omitempty"`
}

type panelUpdateCheckResponse struct {
	Supported      bool               `json:"supported"`
	Available      bool               `json:"available"`
	CurrentVersion string             `json:"current_version"`
	CurrentCommit  string             `json:"current_commit"`
	Target         *panelUpdateTarget `json:"target,omitempty"`
	// PreviousAttempt is additive and optional: the latest recorded attempt on
	// this server to the offered target commit, when it failed or was rolled
	// back. It is owner guidance only; Start stays available and unchanged.
	PreviousAttempt *recoveryobs.Attempt `json:"previous_attempt,omitempty"`
}

// previousPanelUpdateAttempt reads the native recovery observations read-only.
// Anything other than an exact failed/recovered attempt for this commit is
// omitted, so older web clients and unknown evidence keep the previous shape.
func (p *Panel) previousPanelUpdateAttempt(commit string) *recoveryobs.Attempt {
	read := p.lastUpdateAttempt
	if read == nil {
		read = recoveryobs.LastAttemptForTarget
	}
	attempt, ok := read(commit)
	if !ok || !recoveryobs.ValidRequestID(attempt.RequestID) ||
		(attempt.Phase != "failed" && attempt.Phase != "recovered") ||
		(attempt.FailureCode != "" && !recoveryobs.ValidFailureCode(attempt.FailureCode)) {
		return nil
	}
	finished, err := time.Parse(time.RFC3339, attempt.FinishedAt)
	if err != nil || finished.UTC().Format("2006-01-02T15:04:05Z") != attempt.FinishedAt {
		return nil
	}
	return &attempt
}

type panelUpdateStartRequest struct {
	RequestID      string `json:"request_id"`
	Confirmed      bool   `json:"confirmed"`
	CurrentVersion string `json:"current_version"`
	CurrentCommit  string `json:"current_commit"`
	panelUpdateTarget
}

type panelUpdateStartResponse struct {
	Accepted  bool              `json:"accepted"`
	RequestID string            `json:"request_id"`
	Status    string            `json:"status"`
	Target    panelUpdateTarget `json:"target"`
	// Plan is additive (2026-10-16): the steps of the update, declared when
	// it is admitted (system_update_plan.go). Older pages ignore it.
	Plan *panelUpdatePlan `json:"plan,omitempty"`
}

// panelUpdateNow is the server clock of the update plan; tests replace it.
var panelUpdateNow = time.Now

type panelUpdateStatusResponse struct {
	Found     bool               `json:"found"`
	RequestID string             `json:"request_id"`
	Status    string             `json:"status,omitempty"`
	Target    *panelUpdateTarget `json:"target,omitempty"`
	CreatedAt string             `json:"created_at,omitempty"`
	UpdatedAt string             `json:"updated_at,omitempty"`
	Summary   string             `json:"summary,omitempty"`
	// Phase is additive (2026-10-10): "verifying" while the record is still
	// running and the Panel answering is already the target build, so the
	// new version is installed and the updater is finishing its checks.
	// Absent otherwise. It never changes the status.
	// Phase eklemedir: kayit hala calisirken yanit veren Panel hedef surumse
	// "verifying"; yeni surum kurulu, guncelleyici denetimlerini bitiriyor.
	Phase string `json:"phase,omitempty"`
	// Plan is additive (2026-10-16): each step's state, moments and elapsed
	// seconds on the server's clock (system_update_plan.go). Absent for a
	// record that was not found. Older pages ignore it.
	// Plan eklemelidir: adimlarin durumu, anlari ve sunucuda olculen sureleri.
	Plan *panelUpdatePlan `json:"plan,omitempty"`
}

// panelUpdateStatusPhase reads the one phase the status can show beside
// running: this Panel already is the exact target build. The Agent's record
// stays running until its updater has exited and its final proof has passed
// (cmd/agent system_update_worker_linux.go), after the new Panel has started.
// Kayit, yeni Panel basladiktan sonra guncelleyici bitip son kanit gecene
// kadar running kalir; bu Panel hedef surumse asama "verifying"dir.
func panelUpdateStatusPhase(status string, target panelUpdateTarget) string {
	if status == "running" && target.Version == buildVersion && target.Commit == buildCommit {
		return "verifying"
	}
	return ""
}

func requirePanelUpdateAdmin(w http.ResponseWriter, r *http.Request) bool {
	if caller := currentCaller(r); caller == nil || caller.Role != roleAdmin {
		writeCodedError(w, http.StatusForbidden, "ADMIN_ONLY", "administrator access is required", "")
		return false
	}
	return true
}

// requireSystemUpdateAgent fails closed unless the running panel and agent are
// the exact release pair that understands the signed updater protocol.
func (p *Panel) requireSystemUpdateAgent(ctx context.Context) error {
	panelVersion := strings.TrimSpace(buildVersion)
	panelCommit := strings.TrimSpace(buildCommit)
	if panelVersion == "" || panelVersion == "dev" || panelCommit == "" || panelCommit == "unknown" {
		return errors.New("panel build identity is unavailable")
	}
	var agent transport.AgentVersionResponse
	if err := p.callAgentContext(ctx, "Agent.Version", &transport.Empty{}, &agent); err != nil {
		return fmt.Errorf("verify update agent identity: %w: %w", errPanelUpdateAgentUnreachable, err)
	}
	if strings.TrimSpace(agent.Version) != panelVersion || strings.TrimSpace(agent.Commit) != panelCommit {
		return errors.New("panel and agent build identities do not match")
	}
	if err := requireKnownAgentCapabilities(
		agent.Capabilities,
		transport.AgentCapabilitySystemUpdateV1,
		transport.AgentCapabilitySystemUpdateAbandonV1,
	); err != nil {
		return fmt.Errorf("verify system updater capability: %w", err)
	}
	return nil
}

func validPanelUpdateDecimal(raw string, max uint64) bool {
	if raw == "" || raw[0] == '0' {
		return false
	}
	for _, r := range raw {
		if r < '0' || r > '9' {
			return false
		}
	}
	value, err := strconv.ParseUint(raw, 10, 64)
	return err == nil && value > 0 && value <= max && strconv.FormatUint(value, 10) == raw
}

func validPanelUpdateTarget(target panelUpdateTarget) bool {
	if !validPanelUpdateVersion(target.Version) {
		return false
	}
	if !panelUpdateCommitPattern.MatchString(target.Commit) || !panelUpdateSHA256Pattern.MatchString(target.ArchiveSHA256) {
		return false
	}
	if !validPanelUpdateDecimal(target.Sequence, math.MaxInt64) ||
		!validPanelUpdateDecimal(target.ArchiveSize, panelUpdateMaximumArchiveSize) {
		return false
	}
	if target.OS != "linux" || (target.Arch != "amd64" && target.Arch != "arm64") {
		return false
	}
	if target.PublishedAt != "" {
		published, err := time.Parse(time.RFC3339, target.PublishedAt)
		if err != nil || published.Format(time.RFC3339) != target.PublishedAt {
			return false
		}
	}
	return true
}

func validPanelUpdateVersion(version string) bool {
	if len(version) > 80 || version != strings.TrimSpace(version) || !panelUpdateVersionPattern.MatchString(version) {
		return false
	}
	separator := strings.IndexByte(version, '-')
	if separator < 0 {
		return true
	}
	for _, identifier := range strings.Split(version[separator+1:], ".") {
		allDigits := true
		for _, r := range identifier {
			if r < '0' || r > '9' {
				allDigits = false
				break
			}
		}
		if allDigits && len(identifier) > 1 && identifier[0] == '0' {
			return false
		}
	}
	return true
}

func panelUpdateTargetFromCheck(reply transport.SystemUpdateCheckResponse) panelUpdateTarget {
	return panelUpdateTarget{
		Version:       reply.TargetVersion,
		Commit:        reply.TargetCommit,
		Sequence:      reply.TargetSequence,
		OS:            reply.TargetOS,
		Arch:          reply.TargetArch,
		ArchiveSHA256: reply.TargetArchiveSHA256,
		ArchiveSize:   reply.TargetArchiveSize,
		PublishedAt:   reply.PublishedAt,
	}
}

func panelUpdateTargetFromStatus(reply transport.SystemUpdateStatusResponse) panelUpdateTarget {
	return panelUpdateTarget{
		Version:       reply.TargetVersion,
		Commit:        reply.TargetCommit,
		Sequence:      reply.TargetSequence,
		OS:            reply.TargetOS,
		Arch:          reply.TargetArch,
		ArchiveSHA256: reply.TargetArchiveSHA256,
		ArchiveSize:   reply.TargetArchiveSize,
	}
}

func samePanelUpdateTarget(left, right panelUpdateTarget) bool {
	return left.Version == right.Version && left.Commit == right.Commit &&
		left.Sequence == right.Sequence && left.OS == right.OS && left.Arch == right.Arch &&
		left.ArchiveSHA256 == right.ArchiveSHA256 && left.ArchiveSize == right.ArchiveSize
}

func validPanelUpdateStatus(status string) bool {
	switch status {
	case "queued", "running", "succeeded", "failed":
		return true
	default:
		return false
	}
}

// sanitizePanelUpdateSummary treats agent text as untrusted. Paths, URLs,
// controls and oversized detail remain in the agent journal, never the API.
// A reviewed updater failure line that is too long or carries a path is not
// dropped: the Panel rebuilds a bounded form from its closed tokens (code,
// state, the preflight step and reason class) and keeps a free reason only
// when it is itself short and plain. Since alpha.97 a preflight stop also
// keeps its diagnostic line (the recovery tool's after the step, the
// checker's as the detail), made safe by panelUpdateDiagnostic: before that
// it was dropped and the owner's dialog had no reason to show. The agent
// journal keeps the full line ("System update worker failed: …").
// Güncelleyicinin uzun ya da yol içeren hata satırı düşürülmez; Panel kapalı
// belirteçlerden sınırlı bir biçim kurar. Tam satır agent günlüğünde kalır.
func sanitizePanelUpdateSummary(raw string) string {
	value := strings.TrimSpace(raw)
	if plainPanelUpdateSummary(value, 240) {
		return value
	}
	return boundedPanelUpdateFailure(value)
}

func plainPanelUpdateSummary(value string, limit int) bool {
	if value == "" || len(value) > limit || strings.ContainsAny(value, "/\\\r\n\t") || strings.Contains(value, "://") {
		return false
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

// Updater failure codes whose meaning the product defines (update.sh).
var panelUpdateFailureCodes = map[string]bool{
	"update_failed": true, "update_preflight_refused": true, "recovery_runtime_preflight_failed": true,
	"package_manager_busy": true, "candidate_panel_startup_check_failed": true, "panel_start_unverified": true,
	"unit_start_limit_hit": true, "recovery_runtime_preparation_unconfirmed": true,
	"firewall_runtime_preparation_unconfirmed": true, "mail_runtime_preparation_unconfirmed": true,
	"update_finishing_scheduler": true,
}

var (
	panelUpdateFailureLine = regexp.MustCompile(`(?:^|: )!! CELIKPANEL_UPDATE_FAILURE code=([a-z_]{1,64}) state=(unchanged|recovery_required) reason=`)
	// The step and reason class of a refused update preflight, and the step of
	// a recovery runtime preflight stop: closed lowercase tokens only.
	panelUpdatePreflightReason = regexp.MustCompile(`^update preflight step=([a-z_]{1,40}) class=([a-z_]{1,40})(?:[: ]|$)`)
	panelUpdateRuntimeReason   = regexp.MustCompile(`^recovery runtime preflight step=([a-z_]{1,40})(?:[: ]|$)`)
)

const panelUpdateFailureMarker = "!! CELIKPANEL_UPDATE_FAILURE"

func boundedPanelUpdateFailure(value string) string {
	match := panelUpdateFailureLine.FindStringSubmatchIndex(value)
	if match == nil {
		return ""
	}
	code, state := value[match[2]:match[3]], value[match[4]:match[5]]
	if !panelUpdateFailureCodes[code] {
		return ""
	}
	reason := value[match[1]:]
	detail := ""
	if end := strings.LastIndex(reason, " detail="); end >= 0 {
		detail = reason[end+len(" detail="):]
		reason = reason[:end]
	}
	reason = strings.TrimSpace(reason)
	original := reason
	switch {
	case code == "update_preflight_refused":
		// The checker's line is the detail; the browser shows it beside
		// the step's own words (alpha.97).
		if parts := panelUpdatePreflightReason.FindStringSubmatch(reason); parts != nil {
			reason = "update preflight step=" + parts[1] + " class=" + parts[2]
			return panelUpdateFailureMarker + " code=" + code + " state=" + state + " reason=" + reason +
				" detail=" + panelUpdateDiagnostic(detail)
		}
		reason = ""
	case code == "recovery_runtime_preflight_failed":
		// The recovery tool's first diagnostic line follows the step
		// (alpha.97: Frankfurt's stop of 2026-10-17 reached the owner with
		// no reason because this line was dropped here).
		if parts := panelUpdateRuntimeReason.FindStringSubmatch(reason); parts != nil {
			reason = "recovery runtime preflight step=" + parts[1]
			if diagnostic := panelUpdateDiagnostic(strings.TrimPrefix(original[len(parts[0]):], ":")); diagnostic != "" {
				reason += ": " + diagnostic
			}
		} else {
			reason = ""
		}
	case !plainPanelUpdateSummary(reason, 120):
		reason = ""
	}
	return panelUpdateFailureMarker + " code=" + code + " state=" + state + " reason=" + reason + " detail="
}

// panelUpdateDiagnosticLimit bounds the diagnostic line the browser receives.
const panelUpdateDiagnosticLimit = 240

var (
	panelUpdateDiagnosticURL = regexp.MustCompile(`[A-Za-z][A-Za-z0-9+.-]*://\S*`)
	// A path starts a word: at the start, after a space or after one of
	// = ' " ( [ < : , ; ` @ (a date such as 2026/10/17 is not one).
	panelUpdateDiagnosticPath = regexp.MustCompile("(^|[\\s='\"(\\[<:,;`@])(/[^\\s:;,'\"()\\[\\]<>`@]*)")
	// NAME=value as an environment or argument assignment; the value is hidden.
	panelUpdateDiagnosticAssignment = regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_]*)=("[^"]*"|'[^']*'|\S*)`)
	// A long run of key-, token- or hash-like characters, "/" included so a
	// base64 value split by slashes is one run: 32 characters or more with no
	// space. A run that starts with "/" is a path (a product path is kept).
	panelUpdateDiagnosticSecret = regexp.MustCompile(`[A-Za-z0-9+/_-]{32,}={0,2}`)
	// A file mode after "mode=" is readable information, not a secret.
	panelUpdateDiagnosticMode  = regexp.MustCompile(`^0?[0-7]{3,4}$`)
	panelUpdateDiagnosticSpace = regexp.MustCompile(`\s+`)
)

// panelUpdateProductPaths are the product's own locations; a path outside
// them is replaced by "[path]".
var panelUpdateProductPaths = []string{
	"/usr/libexec/celikpanel", "/var/lib/celikpanel", "/etc/celikpanel", "/opt/celikpanel",
	"/run/celikpanel", "/var/log/celikpanel", "/var/backups/celikpanel", "/usr/local/bin/celikpanel",
}

// panelUpdateDiagnostic makes the updater's diagnostic safe to show: its
// first line, printable ASCII only, URLs and paths outside the product's own
// replaced, assignments' values and long token-like runs hidden, the
// updater's own markers removed, at most 240 characters. The recovery tool
// already promises never to print environment, arguments or file contents;
// this is the Panel's own bound on top of that promise.
// Güncelleyicinin tanı satırını güvenle gösterilir kılar: ilk satır, yalnız
// yazdırılabilir ASCII, ürün dışı yollar ve adresler gizli, en çok 240 karakter.
func panelUpdateDiagnostic(raw string) string {
	line := raw
	if end := strings.IndexAny(line, "\r\n"); end >= 0 {
		line = line[:end]
	}
	var printable strings.Builder
	for _, r := range line {
		switch {
		case r == '\t':
			printable.WriteByte(' ')
		case r >= 0x20 && r < 0x7f:
			printable.WriteRune(r)
		}
	}
	text := printable.String()
	text = strings.ReplaceAll(text, panelUpdateFailureMarker, " ")
	text = strings.ReplaceAll(text, "CELIKPANEL_UPDATE_FAILURE", " ")
	text = panelUpdateDiagnosticURL.ReplaceAllString(text, "[address]")
	text = panelUpdateDiagnosticPath.ReplaceAllStringFunc(text, func(match string) string {
		lead, path := "", match
		if match[0] != '/' {
			lead, path = match[:1], match[1:]
		}
		if len(path) == 1 {
			return match
		}
		for _, prefix := range panelUpdateProductPaths {
			if path == prefix || strings.HasPrefix(path, prefix+"/") || strings.HasPrefix(path, prefix+"-") || strings.HasPrefix(path, prefix+".") {
				if !strings.Contains(path, "/../") && !strings.HasSuffix(path, "/..") {
					return match
				}
			}
		}
		return lead + "[path]"
	})
	// Token runs first, so a value with "=" padding is one run, not an
	// assignment.
	text = panelUpdateDiagnosticSecret.ReplaceAllStringFunc(text, func(run string) string {
		if strings.HasPrefix(run, "/") {
			return run
		}
		return "[hidden]"
	})
	text = panelUpdateDiagnosticAssignment.ReplaceAllStringFunc(text, func(assignment string) string {
		name := assignment[:strings.IndexByte(assignment, '=')]
		value := assignment[len(name)+1:]
		// Sentence punctuation after the value is not part of it.
		trimmed := strings.TrimRight(value, ",;:.)")
		tail := value[len(trimmed):]
		switch strings.ToLower(name) {
		case "step", "class":
			// The updater's own closed tokens repeated in its line stay.
			if panelUpdateClosedToken.MatchString(trimmed) {
				return assignment
			}
		case "mode":
			if panelUpdateDiagnosticMode.MatchString(trimmed) {
				return assignment
			}
		}
		return name + "=[hidden]" + tail
	})
	text = strings.TrimSpace(panelUpdateDiagnosticSpace.ReplaceAllString(text, " "))
	if len(text) > panelUpdateDiagnosticLimit {
		text = strings.TrimSpace(text[:panelUpdateDiagnosticLimit-3]) + "..."
	}
	return text
}

var panelUpdateClosedToken = regexp.MustCompile(`^[a-z_]{1,40}$`)

func writePanelUpdateUnavailable(w http.ResponseWriter, err error) {
	if err != nil {
		log.Printf("[panel-update] unavailable: %v", err)
	}
	writeCodedError(w, http.StatusServiceUnavailable, "PANEL_UPDATE_UNAVAILABLE", "secure panel updates are unavailable for this build pair", "")
}

func writePanelUpdateAgentFailure(w http.ResponseWriter, err error) {
	if err != nil {
		log.Printf("[panel-update] agent call failed: %v", err)
	}
	status := http.StatusBadGateway
	code := "PANEL_UPDATE_AGENT_UNAVAILABLE"
	message := "the update service is temporarily unavailable"
	if errors.Is(err, context.DeadlineExceeded) {
		status = http.StatusGatewayTimeout
		code = "PANEL_UPDATE_TIMEOUT"
		message = "the update service did not respond in time"
	}
	writeCodedError(w, status, code, message, "")
}

// writePanelUpdateStartHostBusy answers a start the agent refused because the
// host package manager is active with the same refusal every other server
// change gets for it: HOST_MUTATION_BUSY, reason package_manager_active and that
// reason's sentence (the web catalogue has both languages for it). Only the
// agent's typed reason selects it; any other refusal keeps
// PANEL_UPDATE_START_REFUSED. No update record exists for either (upd9 F2).
// Paket yöneticisi etkin diye reddedilen başlatma, diğer değişikliklerle aynı
// kodu, nedeni ve cümleyi alır; yalnız agent'ın tipli nedeni bunu seçer.
func writePanelUpdateStartHostBusy(w http.ResponseWriter, reply transport.SystemUpdateStartResponse) bool {
	if reply.Accepted || reply.Reason != transport.HostMutationReasonPackageManager {
		return false
	}
	classification, ok := classifyHostMutationError(&hostMutationBusyError{reason: reply.Reason})
	if !ok {
		return false
	}
	w.WriteHeader(classification.Status)
	_ = json.NewEncoder(w).Encode(apiErrorBody{
		Error:  classification.Message,
		Code:   classification.Code,
		Reason: classification.Reason,
	})
	return true
}

func (p *Panel) handlePanelUpdateCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		rejectRouteMethod(w, []string{http.MethodGet})
		return
	}
	if !requirePanelUpdateAdmin(w, r) {
		return
	}
	// The same discovery the scheduled check runs (system_update_discovery.go);
	// its result is kept as the last check, made by hand.
	response, failure := p.discoverPanelUpdate(r.Context())
	// It is written only while no update is admitted or running; otherwise it
	// is kept in memory until the server is idle (system_update_discovery.go).
	if r.Context().Err() == nil {
		now := panelUpdateNow()
		p.savePanelUpdateDiscovery(r.Context(), now, false, response, failure)
		p.updateDiscovery.NoteChecked(now)
	}
	if failure != nil {
		if failure.agent {
			writePanelUpdateAgentFailure(w, failure.err)
		} else {
			writePanelUpdateUnavailable(w, failure.err)
		}
		return
	}
	_ = json.NewEncoder(w).Encode(response)
}

func (p *Panel) handlePanelUpdateStart(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost {
		rejectRouteMethod(w, []string{http.MethodPost})
		return
	}
	if !requirePanelUpdateAdmin(w, r) {
		return
	}
	var request panelUpdateStartRequest
	if err := decodeServiceOperationJSON(w, r, &request); err != nil {
		writeCodedError(w, http.StatusBadRequest, "PANEL_UPDATE_INVALID_REQUEST", "invalid update confirmation", "")
		return
	}
	if !request.Confirmed || !panelUpdateRequestIDPattern.MatchString(request.RequestID) ||
		!validPanelUpdateVersion(request.CurrentVersion) ||
		!panelUpdateCommitPattern.MatchString(request.CurrentCommit) ||
		!validPanelUpdateTarget(request.panelUpdateTarget) {
		writeCodedError(w, http.StatusBadRequest, "PANEL_UPDATE_INVALID_CONFIRMATION", "the exact discovered update must be confirmed", "")
		return
	}
	if err := p.requireSystemUpdateAgent(r.Context()); err != nil {
		writePanelUpdateUnavailable(w, err)
		return
	}
	var existing transport.SystemUpdateStatusResponse
	if err := p.callAgentContext(
		r.Context(), "Agent.SystemUpdateStatus",
		&transport.SystemUpdateStatusRequest{RequestID: request.RequestID}, &existing,
	); err != nil {
		writePanelUpdateAgentFailure(w, err)
		return
	}
	if strings.TrimSpace(existing.Error) != "" && (!existing.Found || existing.Status != "failed") {
		log.Printf("[panel-update] idempotency lookup failed: %s", sanitizePanelUpdateSummary(existing.Error))
		writeCodedError(w, http.StatusServiceUnavailable, "PANEL_UPDATE_STATUS_UNAVAILABLE", "the existing update request could not be verified", "")
		return
	}
	if existing.Found {
		target := panelUpdateTargetFromStatus(existing)
		if existing.RequestID != request.RequestID || !validPanelUpdateStatus(existing.Status) ||
			!validPanelUpdateTarget(target) || !samePanelUpdateTarget(request.panelUpdateTarget, target) {
			writeCodedError(w, http.StatusConflict, "PANEL_UPDATE_REQUEST_CONFLICT", "request_id belongs to a different update", "")
			return
		}
		if existing.Status == "failed" {
			writeCodedError(w, http.StatusConflict, "PANEL_UPDATE_START_REFUSED", "the update service did not accept this request", "")
			return
		}
		status := http.StatusOK
		if existing.Status == "queued" || existing.Status == "running" {
			status = http.StatusAccepted
		}
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(panelUpdateStartResponse{
			Accepted: true, RequestID: request.RequestID, Status: existing.Status, Target: target,
			Plan: p.panelUpdatePlanFromStatus(r.Context(), existing, target, panelUpdateNow()),
		})
		return
	}
	if request.CurrentVersion != buildVersion || request.CurrentCommit != buildCommit {
		writeCodedError(w, http.StatusBadRequest, "PANEL_UPDATE_INVALID_CONFIRMATION", "the exact discovered update must be confirmed", "")
		return
	}
	if !p.serviceMutationMu.TryLock() {
		writeCodedError(w, http.StatusConflict, "PANEL_UPDATE_BUSY", "another server operation is active", "")
		return
	}
	defer p.serviceMutationMu.Unlock()
	active, err := p.activeServiceOperation(r.Context())
	if err != nil {
		writePanelUpdateAgentFailure(w, fmt.Errorf("check active service operation: %w", err))
		return
	}
	if active != nil {
		writeCodedError(w, http.StatusConflict, "PANEL_UPDATE_BUSY", "another server operation is active", "")
		return
	}
	var discovered transport.SystemUpdateCheckResponse
	if err := p.callAgentContext(r.Context(), "Agent.CheckSystemUpdate", &transport.Empty{}, &discovered); err != nil {
		writePanelUpdateAgentFailure(w, err)
		return
	}
	if strings.TrimSpace(discovered.Error) != "" {
		log.Printf("[panel-update] confirmation discovery refused: %s", sanitizePanelUpdateSummary(discovered.Error))
		writeCodedError(w, http.StatusConflict, "PANEL_UPDATE_TARGET_CHANGED", "the discovered update changed; check again before confirming", "")
		return
	}
	if !discovered.Supported || !discovered.Available || discovered.CurrentVersion != buildVersion ||
		discovered.CurrentCommit != buildCommit || !samePanelUpdateTarget(request.panelUpdateTarget, panelUpdateTargetFromCheck(discovered)) {
		writeCodedError(w, http.StatusConflict, "PANEL_UPDATE_TARGET_CHANGED", "the discovered update changed; check again before confirming", "")
		return
	}
	agentRequest := transport.SystemUpdateStartRequest{
		RequestID:     request.RequestID,
		TargetVersion: request.Version, TargetCommit: request.Commit,
		TargetSequence: request.Sequence, TargetOS: request.OS, TargetArch: request.Arch,
		TargetArchiveSHA256: request.ArchiveSHA256, TargetArchiveSize: request.ArchiveSize,
		ExpectedCurrentVersion: request.CurrentVersion, ExpectedCurrentCommit: request.CurrentCommit,
	}
	var reply transport.SystemUpdateStartResponse
	if err := p.callAgentContext(r.Context(), "Agent.StartSystemUpdate", &agentRequest, &reply); err != nil {
		writePanelUpdateAgentFailure(w, err)
		return
	}
	if !reply.Accepted || strings.TrimSpace(reply.Error) != "" ||
		(reply.Status != "queued" && reply.Status != "running") {
		log.Printf("[panel-update] agent refused start: %s", sanitizePanelUpdateSummary(reply.Error))
		if writePanelUpdateStartHostBusy(w, reply) {
			return
		}
		writeCodedError(w, http.StatusConflict, "PANEL_UPDATE_START_REFUSED", "the update service did not accept this request", "")
		return
	}
	// The scheduled update check stays away from an admitted update.
	p.markPanelUpdateAdmitted(panelUpdateNow())
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(panelUpdateStartResponse{
		Accepted: true, RequestID: request.RequestID, Status: reply.Status, Target: request.panelUpdateTarget,
		Plan: p.admittedPanelUpdatePlan(r.Context(), panelUpdateNow()),
	})
}

func (p *Panel) handlePanelUpdateAbandon(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost {
		rejectRouteMethod(w, []string{http.MethodPost})
		return
	}
	if !requirePanelUpdateAdmin(w, r) {
		return
	}
	var request panelUpdateStartRequest
	if err := decodeServiceOperationJSON(w, r, &request); err != nil {
		writeCodedError(w, http.StatusBadRequest, "PANEL_UPDATE_INVALID_REQUEST", "invalid update abandonment", "")
		return
	}
	if !request.Confirmed || !panelUpdateRequestIDPattern.MatchString(request.RequestID) ||
		!validPanelUpdateVersion(request.CurrentVersion) ||
		!panelUpdateCommitPattern.MatchString(request.CurrentCommit) ||
		!validPanelUpdateTarget(request.panelUpdateTarget) {
		writeCodedError(w, http.StatusBadRequest, "PANEL_UPDATE_INVALID_CONFIRMATION", "the exact discovered update must be confirmed", "")
		return
	}
	if err := p.requireSystemUpdateAgent(r.Context()); err != nil {
		writePanelUpdateUnavailable(w, err)
		return
	}
	if !p.serviceMutationMu.TryLock() {
		writeCodedError(w, http.StatusConflict, "PANEL_UPDATE_BUSY", "another server operation is active", "")
		return
	}
	defer p.serviceMutationMu.Unlock()
	agentRequest := transport.SystemUpdateAbandonRequest{
		RequestID:     request.RequestID,
		TargetVersion: request.Version, TargetCommit: request.Commit,
		TargetSequence: request.Sequence, TargetOS: request.OS, TargetArch: request.Arch,
		TargetArchiveSHA256: request.ArchiveSHA256, TargetArchiveSize: request.ArchiveSize,
		ExpectedCurrentVersion: request.CurrentVersion, ExpectedCurrentCommit: request.CurrentCommit,
	}
	var reply transport.SystemUpdateStatusResponse
	if err := p.callAgentContext(r.Context(), "Agent.AbandonSystemUpdate", &agentRequest, &reply); err != nil {
		writePanelUpdateAgentFailure(w, err)
		return
	}
	if strings.TrimSpace(reply.Error) != "" && (!reply.Found || reply.Status != "failed") {
		log.Printf("[panel-update] abandon failed: %s", sanitizePanelUpdateSummary(reply.Error))
		writeCodedError(w, http.StatusServiceUnavailable, "PANEL_UPDATE_STATUS_UNAVAILABLE", "the update abandonment could not be verified", "")
		return
	}
	target := panelUpdateTargetFromStatus(reply)
	if !reply.Found || reply.RequestID != request.RequestID || !validPanelUpdateStatus(reply.Status) ||
		!validPanelUpdateTarget(target) || !samePanelUpdateTarget(request.panelUpdateTarget, target) {
		writePanelUpdateAgentFailure(w, errors.New("agent returned invalid update abandonment receipt"))
		return
	}
	_ = json.NewEncoder(w).Encode(panelUpdateStatusResponse{
		Found: true, RequestID: request.RequestID, Status: reply.Status, Target: &target,
		CreatedAt: reply.CreatedAt, UpdatedAt: reply.UpdatedAt,
		Summary: sanitizePanelUpdateSummary(reply.Error),
		Phase:   panelUpdateStatusPhase(reply.Status, target),
	})
}

func (p *Panel) handlePanelUpdateStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		rejectRouteMethod(w, []string{http.MethodGet})
		return
	}
	if !requirePanelUpdateAdmin(w, r) {
		return
	}
	requestID := r.URL.Query().Get("request_id")
	if !panelUpdateRequestIDPattern.MatchString(requestID) {
		writeCodedError(w, http.StatusBadRequest, "PANEL_UPDATE_INVALID_REQUEST_ID", "invalid update request_id", "")
		return
	}
	if err := p.requireSystemUpdateAgent(r.Context()); err != nil {
		log.Printf("[panel-update] status unavailable during build-pair transition: %v", err)
		writeCodedError(w, http.StatusServiceUnavailable, "PANEL_UPDATE_RESTARTING", "the panel and update service are restarting", "")
		return
	}
	var reply transport.SystemUpdateStatusResponse
	if err := p.callAgentContext(r.Context(), "Agent.SystemUpdateStatus", &transport.SystemUpdateStatusRequest{RequestID: requestID}, &reply); err != nil {
		writePanelUpdateAgentFailure(w, err)
		return
	}
	if strings.TrimSpace(reply.Error) != "" && (!reply.Found || reply.Status != "failed") {
		log.Printf("[panel-update] status lookup failed: %s", sanitizePanelUpdateSummary(reply.Error))
		writeCodedError(w, http.StatusServiceUnavailable, "PANEL_UPDATE_STATUS_UNAVAILABLE", "the update status is temporarily unavailable", "")
		return
	}
	if !reply.Found {
		_ = json.NewEncoder(w).Encode(panelUpdateStatusResponse{Found: false, RequestID: requestID})
		return
	}
	target := panelUpdateTargetFromStatus(reply)
	if reply.RequestID != requestID || !validPanelUpdateStatus(reply.Status) || !validPanelUpdateTarget(target) {
		writePanelUpdateAgentFailure(w, errors.New("agent returned invalid update status identity"))
		return
	}
	_ = json.NewEncoder(w).Encode(panelUpdateStatusResponse{
		Found: true, RequestID: requestID, Status: reply.Status, Target: &target,
		CreatedAt: reply.CreatedAt, UpdatedAt: reply.UpdatedAt,
		Summary: sanitizePanelUpdateSummary(reply.Error),
		Phase:   panelUpdateStatusPhase(reply.Status, target),
		Plan:    p.panelUpdatePlanFromStatus(r.Context(), reply, target, panelUpdateNow()),
	})
}
