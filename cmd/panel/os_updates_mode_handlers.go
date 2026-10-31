package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.114. Two things next to the security updates card:
//
//   - POST /api/v1/security/os-updates/mode: the owner's choice between
//     automatic and manual operating-system updates on Debian and Ubuntu.
//     The Agent writes apt's own file (/etc/apt/apt.conf.d/20auto-upgrades);
//     apt's timers keep working without the panel. The request carries the
//     digest of the file the owner saw, and an explicit confirmation when
//     that file is the owner's own text; otherwise nothing is written.
//   - GET /api/v1/security/os-updates/summary: the compact count for the
//     Dashboard. It never starts a package operation by itself being asked:
//     it answers from the last reading and refreshes it in the background,
//     at most one reading at a time and not more often than the interval
//     below. Asking again and again cannot start a second reading or any
//     change.
//
// alpha.114: sahibin otomatik/elle güncelleme seçimi (apt'ın kendi dosyası)
// ve Gösterge Paneli için kısa özet; özet sorgusu hiçbir değişiklik başlatmaz.

const (
	osUpdatesModePath    = "/api/v1/security/os-updates/mode"
	osUpdatesSummaryPath = "/api/v1/security/os-updates/summary"

	// A reading of waiting updates is reused this long.
	osUpdatesSummaryFresh = 10 * time.Minute
	// After a failed reading the next attempt waits this long.
	osUpdatesSummaryRetry = 2 * time.Minute
)

type osUpdatesSummaryView struct {
	// State: ready (a reading exists; Stale says whether it is older than the
	// interval), checking (the first reading is under way), unknown (the
	// reading failed and there is none to show).
	State         string                        `json:"state"`
	Stale         bool                          `json:"stale,omitempty"`
	CheckedAt     string                        `json:"checked_at,omitempty"`
	PackageFamily string                        `json:"package_family,omitempty"`
	Pending       *transport.OSUpdatesPending   `json:"pending,omitempty"`
	Automatic     *transport.OSUpdatesAutomatic `json:"automatic,omitempty"`
}

type osUpdatesSummaryCache struct {
	mu         sync.Mutex
	reading    *transport.OSUpdatesStatusResponse
	at         time.Time
	refreshing bool
	failedAt   time.Time
}

var osUpdatesSummary osUpdatesSummaryCache

// startOSUpdatesRefresh runs a background reading; a variable for tests.
var startOSUpdatesRefresh = func(run func()) { go run() }

func (c *osUpdatesSummaryCache) remember(response transport.OSUpdatesStatusResponse) {
	c.mu.Lock()
	defer c.mu.Unlock()
	copied := response
	c.reading, c.at, c.failedAt = &copied, time.Now(), time.Time{}
}

func (c *osUpdatesSummaryCache) forget() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.reading, c.at = nil, time.Time{}
}

func (c *osUpdatesSummaryCache) view(p *Panel) osUpdatesSummaryView {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	fresh := c.reading != nil && now.Sub(c.at) < osUpdatesSummaryFresh
	waiting := !c.failedAt.IsZero() && now.Sub(c.failedAt) < osUpdatesSummaryRetry
	if !fresh && !c.refreshing && !waiting {
		c.refreshing = true
		startOSUpdatesRefresh(func() {
			response, err := readOSUpdatesFromAgent(p, context.Background())
			c.mu.Lock()
			c.refreshing = false
			if err != nil {
				log.Printf("security updates summary: could not be read: %v", err)
				c.failedAt = time.Now()
				c.mu.Unlock()
				return
			}
			copied := response
			c.reading, c.at, c.failedAt = &copied, time.Now(), time.Time{}
			c.mu.Unlock()
		})
	}
	if c.reading == nil {
		if waiting && !c.refreshing {
			return osUpdatesSummaryView{State: "unknown"}
		}
		return osUpdatesSummaryView{State: "checking"}
	}
	reading := *c.reading
	reading.Pending.SecurityNames = nil // the Dashboard shows counts only
	return osUpdatesSummaryView{
		State: "ready", Stale: !fresh, CheckedAt: c.at.UTC().Format(time.RFC3339),
		PackageFamily: reading.PackageFamily, Pending: &reading.Pending, Automatic: &reading.Automatic,
	}
}

func (p *Panel) handleOSUpdatesSummary(w http.ResponseWriter, r *http.Request) {
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
	json.NewEncoder(w).Encode(osUpdatesSummary.view(p))
}

var setOSUpdatesModeThroughAgent = func(p *Panel, ctx context.Context, mode, expectedSHA string, confirmEdit bool) (transport.SetOSUpdatesModeResponse, error) {
	var response transport.SetOSUpdatesModeResponse
	err := p.withStandaloneAgentMutation(ctx, "os_updates_mode", "unattended-upgrades", "", func(callCtx context.Context, binding agentMutationBinding) error {
		request := transport.SetOSUpdatesModeRequest{ServiceMutationBinding: binding, Mode: mode, ExpectedCurrentSHA256: expectedSHA, ConfirmOwnerEdit: confirmEdit}
		if err := p.callAgentContext(callCtx, "Agent.SetOSUpdatesModeV1", &request, &response); err != nil {
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

func (p *Panel) handleOSUpdatesMode(w http.ResponseWriter, r *http.Request) {
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
		Mode                  string `json:"mode"`
		ExpectedCurrentSHA256 string `json:"expected_current_sha256"`
		ConfirmOwnerEdit      bool   `json:"confirm_owner_edit"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&req); err != nil ||
		(req.Mode != "auto" && req.Mode != "manual") ||
		(req.ExpectedCurrentSHA256 != "" && !sha256Text.MatchString(req.ExpectedCurrentSHA256)) {
		writeClientError(w, http.StatusBadRequest, "the choice (auto or manual) and the digest of the file that was shown are required")
		return
	}
	release, busy := p.beginServiceMutation(w, r)
	if busy {
		return
	}
	defer release()
	response, err := setOSUpdatesModeThroughAgent(p, r.Context(), req.Mode, req.ExpectedCurrentSHA256, req.ConfirmOwnerEdit)
	if err != nil {
		p.auditDetailed(r, "security.os_updates_mode.failed — "+auditReason(err.Error()), "security", 0, map[string]any{"mode": req.Mode})
		writeAgentError(w, err, "operating-system update choice")
		return
	}
	if !response.Written {
		p.auditDetailed(r, "security.os_updates_mode.failed — "+auditReason(response.Error), "security", 0, map[string]any{"mode": req.Mode, "code": response.ErrorCode})
		code := response.ErrorCode
		if code == "" {
			code = "FAILED"
		}
		writeCodedError(w, http.StatusConflict, "OS_UPDATES_MODE_"+strings.TrimPrefix(code, "OS_UPDATES_MODE_"), response.Error, "/security")
		return
	}
	osUpdatesSummary.forget()
	p.auditDetailed(r, "security.os_updates_mode:"+req.Mode, "security", 0, map[string]any{
		"mode": req.Mode, "path": response.Path, "previous_state": response.PreviousState, "effective": response.Effective,
		"effective_reason": response.EffectiveReason, "legacy_removed": response.LegacyRemoved, "timers_enabled": response.TimersEnabled,
		"owner_edit_confirmed": req.ConfirmOwnerEdit, "seen_sha256": req.ExpectedCurrentSHA256,
	})
	json.NewEncoder(w).Encode(map[string]any{
		"written": true, "mode": response.Mode, "path": response.Path, "previous_state": response.PreviousState,
		"effective": response.Effective, "effective_reason": response.EffectiveReason, "timer_state": response.TimerState,
		"timers_enabled": response.TimersEnabled, "legacy_removed": response.LegacyRemoved,
	})
}
