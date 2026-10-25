package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/transport"
)

// The vendor lane (alpha.108, D-038 amendment "two lanes"): an
// administrator's informed choice, per component. The plan is shown first —
// the source line and its key, CelikPanel's preferences text and the
// automatic-updates drop-in, each with its SHA-256 — and only the confirmed
// plan is written, as one operation in its own vendor_lane job. Customers and
// team members never reach these routes (/api/v1/repo is an administrator
// prefix, and the handlers check again).
//
// Satıcı yolu: plan önce gösterilir, yalnız onaylanan plan tek işlemle
// yazılır. Müşteri ve ekip üyesi bu yollara ulaşamaz.

const (
	vendorLanePath      = "/api/v1/repo/lane"
	vendorLaneReadBound = 100 * time.Second
	vendorLaneCallBound = 12 * time.Minute
	vendorLaneAudit     = "repo.lane"
)

var readVendorLanePlan = func(p *Panel, ctx context.Context, repoID string) (transport.VendorLanePlan, error) {
	var plan transport.VendorLanePlan
	callCtx, cancel := context.WithTimeout(ctx, vendorLaneReadBound)
	defer cancel()
	err := p.callAgentContext(callCtx, "Agent.VendorLanePlanV1", &enableRepoReq{RepoID: repoID}, &plan)
	return plan, err
}

var enableVendorLaneThroughAgent = func(p *Panel, ctx context.Context, repoID, expected string) (transport.VendorLaneResponse, error) {
	var response transport.VendorLaneResponse
	callCtx, cancel := context.WithTimeout(ctx, vendorLaneCallBound)
	defer cancel()
	err := p.withStandaloneAgentMutation(callCtx, "vendor_lane", repoID, "", func(callCtx context.Context, binding agentMutationBinding) error {
		request := &enableRepoReq{ServiceMutationBinding: binding, RepoID: repoID, ExpectedSHA256: expected}
		if err := p.callAgentContext(callCtx, "Agent.EnableVendorLaneV1", request, &response); err != nil {
			return err
		}
		if response.ErrorCode != "" {
			return errors.New(response.Error)
		}
		return nil
	})
	return response, err
}

// laneServiceRepo is the component's repository when it is a vendor lane.
func laneServiceRepo(serviceID string) *core.ManagedRepo {
	svc := core.GetManagedServiceByID(serviceID)
	if svc == nil || svc.Repo == nil || strings.TrimSpace(svc.Repo.ReleaseOrigin) == "" {
		return nil
	}
	return svc.Repo
}

func (p *Panel) handleVendorLane(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if c := currentCaller(r); c == nil || c.Role != roleAdmin {
		writeCodedError(w, http.StatusForbidden, "ADMIN_ONLY", "administrator access is required", "")
		return
	}
	switch r.Method {
	case http.MethodGet:
		repo := laneServiceRepo(r.URL.Query().Get("service_id"))
		if repo == nil || p.packageFamily() != "apt" {
			json.NewEncoder(w).Encode(map[string]any{"state": "none"})
			return
		}
		plan, err := readVendorLanePlan(p, r.Context(), repo.ID)
		if err != nil {
			log.Printf("[repo][%s][lane-plan] %v", repo.ID, err)
			json.NewEncoder(w).Encode(map[string]any{"state": "unknown"})
			return
		}
		if plan.Error != "" {
			json.NewEncoder(w).Encode(map[string]any{"state": "failed", "error": cleanAptLines([]string{plan.Error})})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"state": "read", "plan": plan})
	case http.MethodPost:
		var req struct {
			ServiceID      string `json:"service_id"`
			ExpectedSHA256 string `json:"expected_sha256"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || !sha256Text.MatchString(strings.TrimSpace(req.ExpectedSHA256)) {
			writeClientError(w, http.StatusBadRequest, "the confirmed plan is required")
			return
		}
		repo := laneServiceRepo(req.ServiceID)
		if repo == nil {
			writeClientError(w, http.StatusBadRequest, "this component has no vendor repository lane")
			return
		}
		release, busy := p.beginServiceMutation(w, r)
		if busy {
			return
		}
		defer release()
		response, err := enableVendorLaneThroughAgent(p, r.Context(), repo.ID, strings.TrimSpace(req.ExpectedSHA256))
		details := map[string]any{
			"repository": repo.ID, "origin": repo.ReleaseOrigin, "confirmed_sha256": strings.TrimSpace(req.ExpectedSHA256),
			"completed": response.Completed, "effective": response.Effective, "code": response.ErrorCode,
		}
		if err != nil && response.ErrorCode == "" {
			log.Printf("[repo][%s][lane][transport] %v", repo.ID, err)
			p.auditDetailed(r, vendorLaneAudit+".unknown:"+repo.ID+" — "+auditReason(err.Error()), "repo", 0, details)
			// The answer was lost: whether anything was written is not
			// known. The card reads the lane again; nothing is resent.
			writeCodedError(w, http.StatusBadGateway, "LANE_RESULT_UNKNOWN", "the vendor repository's answer was lost; read the card again", "/services")
			return
		}
		if response.ErrorCode != "" {
			p.auditDetailed(r, vendorLaneAudit+".failed:"+repo.ID+" — "+response.ErrorCode, "repo", 0, details)
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]any{
				"error": cleanAptLines([]string{response.Error}), "code": response.ErrorCode,
				"completed": response.Completed, "steps": response.Steps,
			})
			return
		}
		p.auditDetailed(r, vendorLaneAudit+":"+repo.ID+" — "+strings.Join(response.Completed, ","), "repo", 0, details)
		json.NewEncoder(w).Encode(map[string]any{
			"written": true, "completed": response.Completed, "effective": response.Effective,
			"steps": response.Steps, "repo": p.repoInfoContext(r.Context(), repo),
		})
	default:
		writeClientError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}
