package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Security → Security updates (alpha.107, D-038 §4 and §9). GET reads the
// operating system's own automatic updates through the Agent, read only and
// bounded; a failed read is "unknown", never "off". POST adds the confined
// vendor repositories to the automatic updates' origins by writing the one
// separate drop-in the administrator saw and confirmed (its SHA-256 is
// checked again by the Agent); nothing else is changed and automatic updates
// are never switched off.
//
// Güvenlik → Güvenlik güncellemeleri: işletim sisteminin otomatik
// güncellemeleri salt okunur okunur; onaylanan tek ek dosya yazılır.

const (
	osUpdatesPath        = "/api/v1/security/os-updates"
	osUpdatesOriginsPath = "/api/v1/security/os-updates/origins"
	osUpdatesReadBound   = 100 * time.Second
)

var sha256Text = regexp.MustCompile(`^[0-9a-f]{64}$`)

type osUpdatesView struct {
	// State: read (the Agent answered) or unknown.
	State string `json:"state"`
	transport.OSUpdatesStatusResponse
}

var readOSUpdatesFromAgent = func(p *Panel, ctx context.Context) (transport.OSUpdatesStatusResponse, error) {
	callCtx, cancel := context.WithTimeout(ctx, osUpdatesReadBound)
	defer cancel()
	var response transport.OSUpdatesStatusResponse
	if err := p.callAgentContext(callCtx, "Agent.OSUpdatesStatusV1", &transport.Empty{}, &response); err != nil {
		return response, err
	}
	if response.Error != "" {
		return response, errors.New(response.Error)
	}
	return response, nil
}

func (p *Panel) handleOSUpdates(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if caller := currentCaller(r); caller == nil || !caller.hasAccountRole(roleAdmin) {
		writeCodedError(w, http.StatusForbidden, "ADMIN_ONLY", "administrator access is required", "")
		return
	}
	if r.Method != http.MethodGet {
		writeClientError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	response, err := readOSUpdatesFromAgent(p, r.Context())
	if err != nil {
		log.Printf("security updates: could not be read: %v", err)
		json.NewEncoder(w).Encode(osUpdatesView{State: "unknown", OSUpdatesStatusResponse: transport.OSUpdatesStatusResponse{
			Vendors: []transport.OSUpdatesVendorOrigin{}, Outside: []transport.OSUpdatesOutside{},
		}})
		return
	}
	osUpdatesSummary.remember(response)
	if response.Vendors == nil {
		response.Vendors = []transport.OSUpdatesVendorOrigin{}
	}
	if response.Outside == nil {
		response.Outside = []transport.OSUpdatesOutside{}
	}
	json.NewEncoder(w).Encode(osUpdatesView{State: "read", OSUpdatesStatusResponse: response})
}

var writeVendorOriginsThroughAgent = func(p *Panel, ctx context.Context, expected string) (transport.WriteVendorUpdateOriginsResponse, error) {
	var response transport.WriteVendorUpdateOriginsResponse
	err := p.withStandaloneAgentMutation(ctx, "os_updates_origins", "unattended-upgrades", "", func(callCtx context.Context, binding agentMutationBinding) error {
		request := transport.WriteVendorUpdateOriginsRequest{ServiceMutationBinding: binding, ExpectedSHA256: expected}
		if err := p.callAgentContext(callCtx, "Agent.WriteVendorUpdateOriginsV1", &request, &response); err != nil {
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

func (p *Panel) handleOSUpdatesOrigins(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if caller := currentCaller(r); caller == nil || !caller.hasAccountRole(roleAdmin) {
		writeCodedError(w, http.StatusForbidden, "ADMIN_ONLY", "administrator access is required", "")
		return
	}
	if r.Method != http.MethodPost {
		writeClientError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		ExpectedSHA256 string `json:"expected_sha256"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || !sha256Text.MatchString(req.ExpectedSHA256) {
		writeClientError(w, http.StatusBadRequest, "the confirmed text is required")
		return
	}
	release, busy := p.beginServiceMutation(w, r)
	if busy {
		return
	}
	defer release()
	response, err := writeVendorOriginsThroughAgent(p, r.Context(), req.ExpectedSHA256)
	if err != nil {
		p.auditDetailed(r, "security.update_origins.failed — "+auditReason(err.Error()), "security", 0, nil)
		writeAgentError(w, err, "automatic update origins")
		return
	}
	if !response.Written {
		p.auditDetailed(r, "security.update_origins.failed — "+auditReason(response.Error), "security", 0, map[string]any{"code": response.ErrorCode})
		code := response.ErrorCode
		if code == "" {
			code = "UPDATE_ORIGINS_FAILED"
		}
		writeCodedError(w, http.StatusConflict, "UPDATE_ORIGINS_"+strings.TrimPrefix(code, "UPDATE_ORIGINS_"), response.Error, "/security")
		return
	}
	p.auditDetailed(r, "security.update_origins:"+strings.Join(response.Origins, ","), "security", 0, map[string]any{
		"path": response.Path, "origins": response.Origins, "effective": response.Effective, "confirmed_sha256": req.ExpectedSHA256,
	})
	json.NewEncoder(w).Encode(map[string]any{"written": true, "path": response.Path, "origins": response.Origins, "effective": response.Effective})
}
