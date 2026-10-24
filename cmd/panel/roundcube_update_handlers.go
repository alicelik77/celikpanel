package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"regexp"
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
// Web postası güncellemeleri: GET hattaki yeni sürümü söyler, POST yöneticinin
// onayıyla günceller; yedek, kanıt ve geri alma Agent'tadır.

const (
	roundcubeUpdatePath      = "/api/v1/service/roundcube/update"
	roundcubeUpdateReadBound = 45 * time.Second
)

var roundcubeReleaseText = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)

var readRoundcubeUpdateFromAgent = func(p *Panel, ctx context.Context) (transport.RoundcubeUpdateCheckResponse, error) {
	callCtx, cancel := context.WithTimeout(ctx, roundcubeUpdateReadBound)
	defer cancel()
	var response transport.RoundcubeUpdateCheckResponse
	err := p.callAgentContext(callCtx, "Agent.RoundcubeUpdateCheckV1", &transport.Empty{}, &response)
	return response, err
}

var updateRoundcubeThroughAgent = func(p *Panel, ctx context.Context, version string) (transport.RoundcubeUpdateResponse, error) {
	var response transport.RoundcubeUpdateResponse
	err := p.withStandaloneAgentMutation(ctx, "webmail_update", "roundcube", version, func(callCtx context.Context, binding agentMutationBinding) error {
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

func (p *Panel) handleRoundcubeUpdate(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	switch r.Method {
	case http.MethodGet:
		response, err := readRoundcubeUpdateFromAgent(p, r.Context())
		if err != nil {
			log.Printf("webmail update check: %v", err)
			json.NewEncoder(w).Encode(map[string]any{"state": "unknown"})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"state": "read", "check": response})
	case http.MethodPost:
		var req struct {
			Version string `json:"version"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || !roundcubeReleaseText.MatchString(req.Version) {
			writeClientError(w, http.StatusBadRequest, "a release version is required")
			return
		}
		release, busy := p.beginServiceMutation(w, r)
		if busy {
			return
		}
		defer release()
		response, err := updateRoundcubeThroughAgent(p, r.Context(), req.Version)
		details := map[string]any{"component": "roundcube", "from": response.From, "to": req.Version, "backup": response.Backup, "rolled_back": response.RolledBack, "code": response.ErrorCode}
		if err != nil {
			p.auditDetailed(r, "webmail.update.failed:"+req.Version+" — "+auditReason(err.Error()), "service", 0, details)
			writeAgentError(w, err, "webmail update")
			return
		}
		if !response.Updated {
			p.auditDetailed(r, "webmail.update.failed:"+req.Version+" — "+auditReason(response.Error), "service", 0, details)
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]any{
				"error": response.Error, "code": "WEBMAIL_UPDATE_" + response.ErrorCode, "rolled_back": response.RolledBack,
				"from": response.From, "to": req.Version, "backup": response.Backup,
			})
			return
		}
		p.auditDetailed(r, "webmail.update:"+response.From+"->"+req.Version, "service", 0, details)
		json.NewEncoder(w).Encode(map[string]any{"updated": true, "proved": response.Proved, "from": response.From, "to": req.Version, "backup": response.Backup})
	default:
		writeClientError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}
