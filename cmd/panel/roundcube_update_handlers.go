package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Webmail (Roundcube) updates (alpha.107, D-038 §9). The operating system
// never updates Roundcube; GET says whether a newer release exists on the
// installed line (read only, the Agent keeps the release list six hours),
// POST updates to it at the administrator's confirmation: in its own
// webmail_update job, with a backup, Roundcube's own database update, an
// atomic swap, proof through webmail's socket and the previous tree put back
// on failure. Admin-only through the /api/v1/service/ prefix; audited with
// from, to, the backup and the outcome.
//
// alpha.109 (Frankfurt): the update ran inside the browser's request, so a
// dropped connection left an unknown result while the lock said "another
// CelikPanel operation is still running", and afterwards the row offered
// "Update available" again without saying what happened. The update is now a
// durable service operation like an install: POST records it and returns
// 202 at once, the work runs on a context detached from the request, every
// phase is recorded, the screen re-attaches to it by its request id after a
// reload (polling only reads), and GET carries the last update's outcome so
// the row says it: updated, failed and put back with the cause, or unknown.
// A panel restart during the update reconciles it read only: the installed
// version decides, and nothing is started again by itself.
//
// Web postası güncellemesi artık kurulumlar gibi kalıcı bir işlemdir: POST
// kaydeder ve hemen döner, iş istekten bağımsız sürer, ekran yeniden
// yüklenince aynı işleme bağlanır; GET son güncellemenin sonucunu taşır.
// Panel yeniden başlarsa sonuç salt okunarak uzlaştırılır; hiçbir şey
// kendiliğinden yeniden başlatılmaz.

const (
	roundcubeUpdatePath      = "/api/v1/service/roundcube/update"
	roundcubeUpdateReadBound = 45 * time.Second

	serviceOperationKindWebmailUpdate = "webmail_update"

	// Outcomes said by the result of a webmail update.
	webmailUpdateOutcomeUpdated        = "updated"
	webmailUpdateOutcomeRolledBack     = "rolled_back"
	webmailUpdateOutcomeNotChanged     = "not_changed"
	webmailUpdateOutcomeRollbackFailed = "rollback_failed"
	webmailUpdateOutcomeUnknown        = "unknown"
	webmailUpdateOutcomeInterrupted    = "interrupted"
)

var roundcubeReleaseText = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)

var readRoundcubeUpdateFromAgent = func(p *Panel, ctx context.Context) (transport.RoundcubeUpdateCheckResponse, error) {
	callCtx, cancel := context.WithTimeout(ctx, roundcubeUpdateReadBound)
	defer cancel()
	var response transport.RoundcubeUpdateCheckResponse
	err := p.callAgentContext(callCtx, "Agent.RoundcubeUpdateCheckV1", &transport.Empty{}, &response)
	return response, err
}

// updateRoundcubeThroughAgent runs inside the durable operation, whose
// context carries the operation's lease binding.
var updateRoundcubeThroughAgent = func(p *Panel, ctx context.Context, version string) (transport.RoundcubeUpdateResponse, error) {
	var response transport.RoundcubeUpdateResponse
	err := p.withStandaloneAgentMutation(ctx, serviceOperationKindWebmailUpdate, "roundcube", version, func(callCtx context.Context, binding agentMutationBinding) error {
		request := transport.RoundcubeUpdateRequest{ServiceMutationBinding: binding, Version: version}
		if err := p.callAgentContext(callCtx, "Agent.UpdateRoundcubeV1", &request, &response); err != nil {
			return err
		}
		if response.Error != "" {
			return errors.New(response.Error)
		}
		return nil
	})
	if err != nil && response.Error == "" {
		return response, err
	}
	return response, nil
}

// boundedWebmailReason keeps the Agent's sentence short for the record; it
// names versions, steps and paths, never a secret.
func boundedWebmailReason(reason string) string {
	reason = strings.TrimSpace(strings.ReplaceAll(reason, "\n", " "))
	if len(reason) > 600 {
		reason = reason[:600] + "…"
	}
	return reason
}

// runWebmailUpdate is the durable operation's work. The outcome is also
// handed to the audit writer through outcome.
func (p *Panel) runWebmailUpdate(ctx context.Context, version string, advance func(string) error, outcome *transport.RoundcubeUpdateResponse) (serviceOperationResult, *serviceOperationFailure) {
	result := serviceOperationResult{"success": false, "to": version}
	response, err := updateRoundcubeThroughAgent(p, ctx, version)
	*outcome = response
	if response.From != "" {
		result["from"] = response.From
	}
	if response.Backup != "" {
		result["backup"] = response.Backup
	}
	if err != nil {
		// The Agent's answer did not arrive: whether it changed anything is
		// not known here; the row reads the installed version again.
		result["outcome"] = webmailUpdateOutcomeUnknown
		if failure := platformServiceOperationFailure(err); failure != nil {
			return result, failure
		}
		return result, operationFailure("webmail_update_unknown",
			"The webmail update's result did not arrive from the server agent, so whether Roundcube changed is not known.", err)
	}
	if !response.Updated {
		result["code"] = response.ErrorCode
		result["reason"] = boundedWebmailReason(response.Error)
		result["rolled_back"] = response.RolledBack
		switch {
		case response.ErrorCode == "ROLLBACK_FAILED":
			result["outcome"] = webmailUpdateOutcomeRollbackFailed
		case response.RolledBack:
			result["outcome"] = webmailUpdateOutcomeRolledBack
			result["previous_answers"] = response.PreviousAnswers
		default:
			result["outcome"] = webmailUpdateOutcomeNotChanged
		}
		return result, operationFailure("webmail_update_failed", "Webmail was not updated.", errors.New(response.Error))
	}
	result["success"], result["outcome"], result["proved"] = true, webmailUpdateOutcomeUpdated, response.Proved
	if err := advance("scanning"); err != nil {
		return result, operationAdvanceFailure(err)
	}
	if _, err := p.scanManagedServices(ctx); err != nil {
		// The update is proved; the Components list refreshes on its next scan.
		log.Printf("webmail update %s: post-update scan: %v", version, err)
	}
	return result, nil
}

// readWebmailProofFromAgent proves webmail read only through the Agent-side
// webmail server: the PHP check, the login page and its stylesheet.
var readWebmailProofFromAgent = func(p *Panel, ctx context.Context) (transport.WebmailProofResponse, error) {
	callCtx, cancel := context.WithTimeout(ctx, roundcubeUpdateReadBound)
	defer cancel()
	var response transport.WebmailProofResponse
	err := p.callAgentContext(callCtx, "Agent.WebmailProofV1", &transport.Empty{}, &response)
	return response, err
}

// reconcileInterruptedWebmailUpdate decides, read only, what an update
// interrupted by a panel restart left. "Updated" is said only when the
// installed release is the target and webmail proves itself there (check,
// page, stylesheet); a target release that does not prove, a Roundcube that
// is not found and a read that fails are all "not known". Nothing is started
// again by itself.
// Yarıda kalan güncelleme salt okunarak uzlaştırılır; "güncellendi" yalnız
// hedef sürüm kuruluyken ve web postası kanıtlanınca söylenir.
func (p *Panel) reconcileInterruptedWebmailUpdate(ctx context.Context, version string) (serviceOperationResult, *serviceOperationFailure) {
	result := serviceOperationResult{"success": false, "to": version, "recovered": true}
	unknown := func(message string, cause error) (serviceOperationResult, *serviceOperationFailure) {
		result["outcome"] = webmailUpdateOutcomeUnknown
		return result, operationFailure("webmail_update_unknown", message, cause)
	}
	check, err := readRoundcubeUpdateFromAgent(p, ctx)
	if err != nil {
		return unknown("The panel restarted during the webmail update and the installed version could not be read, so its result is not known.", err)
	}
	if !check.Installed {
		return unknown("The panel restarted during the webmail update and Roundcube was not found where CelikPanel installs it, so its result is not known.", nil)
	}
	result["from"] = check.Version
	if check.Version != version {
		result["outcome"] = webmailUpdateOutcomeInterrupted
		return result, operationFailure("webmail_update_interrupted",
			"The panel restarted during the webmail update; Roundcube was not updated.", nil)
	}
	proof, err := readWebmailProofFromAgent(p, ctx)
	if err != nil || !proof.Proved || proof.Version != version {
		if err == nil {
			err = errors.New(proof.Error)
		}
		result["reason"] = boundedWebmailReason(err.Error())
		return unknown("Roundcube "+version+" is installed but webmail did not prove itself after the panel restarted; press Update again.", err)
	}
	result["success"], result["outcome"], result["proved"] = true, webmailUpdateOutcomeUpdated, true
	return result, nil
}

func (p *Panel) webmailUpdateAuditWriter(version string, outcome *transport.RoundcubeUpdateResponse) serviceOperationAuditWriter {
	return func(ctx context.Context, actor serviceOperationActor, action string) {
		details := map[string]any{"component": "roundcube", "operation": serviceOperationKindWebmailUpdate, "from": outcome.From, "to": version,
			"backup": outcome.Backup, "rolled_back": outcome.RolledBack, "code": outcome.ErrorCode}
		switch {
		case outcome.Updated:
			action = "webmail.update:" + outcome.From + "->" + version
		case outcome.Error != "":
			action = "webmail.update.failed:" + version + " — " + auditReason(outcome.Error)
		}
		p.auditServiceOperation(withAuditDetails(ctx, details), actor, action)
	}
}

// lastWebmailUpdate is the newest webmail update operation, or nil.
func (p *Panel) lastWebmailUpdate(ctx context.Context) (*serviceOperation, error) {
	op, err := scanServiceOperation(p.db.GetDB().QueryRowContext(ctx,
		serviceOperationSelect+` WHERE kind=? ORDER BY started_at DESC LIMIT 1`, serviceOperationKindWebmailUpdate))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &op, nil
}

func (p *Panel) handleRoundcubeUpdate(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	switch r.Method {
	case http.MethodGet:
		last, lastErr := p.lastWebmailUpdate(r.Context())
		if lastErr != nil {
			log.Printf("webmail update: last operation: %v", lastErr)
		}
		// webmail_ready (alpha.109): webmail answers at the panel's
		// /webmail/ address, so the screens offer the link to open it.
		ready := p.webmailAvailable(r.Context())
		response, err := readRoundcubeUpdateFromAgent(p, r.Context())
		if err != nil {
			log.Printf("webmail update check: %v", err)
			json.NewEncoder(w).Encode(map[string]any{"state": "unknown", "last_update": last, "webmail_ready": ready})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"state": "read", "check": response, "last_update": last, "webmail_ready": ready})
	case http.MethodPost:
		if !p.requireSubsystemOperational(w, degradedSubsystemServiceOperations) {
			return
		}
		var req struct {
			Version   string `json:"version"`
			RequestID string `json:"request_id"`
		}
		if err := decodeServiceOperationJSON(w, r, &req); err != nil || !roundcubeReleaseText.MatchString(req.Version) {
			writeClientError(w, http.StatusBadRequest, "a release version is required")
			return
		}
		if !validServiceOperationID(req.RequestID) {
			writeClientError(w, http.StatusBadRequest, "invalid request_id")
			return
		}
		// The same request again (a reload, a lost answer) gets the same
		// operation; it never starts a second update.
		existing, found, err := p.idempotentServiceOperation(r.Context(), req.RequestID, serviceOperationKindWebmailUpdate, "roundcube", req.Version)
		if err != nil {
			if errors.Is(err, errServiceOperationRequestConflict) {
				writeServiceOperationRequestConflict(w)
				return
			}
			writeServerError(w, err)
			return
		}
		if found {
			writeAcceptedServiceOperation(w, existing)
			return
		}
		release, busy := p.beginServiceMutation(w, r)
		if busy {
			return
		}
		releaseInHandler := true
		defer func() {
			if releaseInHandler {
				release()
			}
		}()
		actor := captureServiceOperationActor(r)
		op, err := p.createServiceOperationRequest(r.Context(), serviceOperationKindWebmailUpdate, "roundcube", req.Version, req.RequestID, actor)
		switch {
		case errors.Is(err, errServiceOperationBusy):
			writeServiceOperationBusy(w)
			return
		case errors.Is(err, errServiceOperationReplay):
			writeAcceptedServiceOperation(w, op)
			return
		case errors.Is(err, errServiceOperationRequestConflict):
			writeServiceOperationRequestConflict(w)
			return
		case err != nil:
			writeServerError(w, err)
			return
		}
		version := req.Version
		outcome := &transport.RoundcubeUpdateResponse{}
		p.launchServiceOperationWithAudit(
			op, actor, "updating",
			"webmail.update:"+version, "webmail.update.failed:"+version,
			release,
			func(ctx context.Context, advance func(string) error) (serviceOperationResult, *serviceOperationFailure) {
				return p.runWebmailUpdate(ctx, version, advance, outcome)
			},
			p.webmailUpdateAuditWriter(version, outcome),
		)
		releaseInHandler = false
		writeAcceptedServiceOperation(w, op)
	default:
		writeClientError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}
