package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Server → Maintenance → Restarts and reboot (alpha.107, D-038 §10): which
// services still run code an update replaced, and whether the server waits
// for a reboot, read by the Agent without changing anything. A restart is the
// component's own restart, asked for by the administrator; nothing restarts
// by itself. A failed read is "unknown", never "nothing to restart".
// Bakım → Yeniden başlatmalar: salt okunur; yeniden başlatmayı yönetici ister.

const (
	restartNeedsPath      = "/api/v1/maintenance/restarts"
	restartNeedsReadBound = 90 * time.Second
)

var readRestartNeedsFromAgent = func(p *Panel, ctx context.Context) (transport.RestartNeedsResponse, error) {
	callCtx, cancel := context.WithTimeout(ctx, restartNeedsReadBound)
	defer cancel()
	var response transport.RestartNeedsResponse
	err := p.callAgentContext(callCtx, "Agent.RestartNeedsV1", &transport.Empty{}, &response)
	return response, err
}

func (p *Panel) handleRestartNeeds(w http.ResponseWriter, r *http.Request) {
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
	response, err := readRestartNeedsFromAgent(p, r.Context())
	if err != nil {
		log.Printf("restarts and reboot: could not be read: %v", err)
		json.NewEncoder(w).Encode(map[string]any{"state": "unknown"})
		return
	}
	if response.Services == nil {
		response.Services = []transport.RestartNeedsService{}
	}
	json.NewEncoder(w).Encode(map[string]any{"state": "read", "needs": response})
}
