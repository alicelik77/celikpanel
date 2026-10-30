package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/transport"
)

// nginxInspect fetches the real, agent-parsed nginx configuration.
// nginxInspect, agent'ın ayrıştırdığı gerçek nginx yapılandırmasını getirir.
func (p *Panel) nginxInspect() (*core.NginxInspectResult, error) {
	var result core.NginxInspectResult
	if err := p.callAgent("Agent.NginxInspect", &transport.Empty{}, &result); err != nil {
		return nil, err
	}
	if result.RateLimits == nil {
		result.RateLimits = []core.NginxRateLimit{}
	}
	return &result, nil
}

// handleNginxGlobalConfig serves the real global directives from `nginx -T`.
// Editing is not implemented yet, so POST is honest about that.
// handleNginxGlobalConfig, `nginx -T`'den gerçek global direktifleri sunar.
// Düzenleme henüz yapılmadı; POST bunu dürüstçe söyler.
func (p *Panel) handleNginxGlobalConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method == http.MethodPost {
		writeClientError(w, http.StatusNotImplemented, "editing nginx config is not supported yet")
		return
	}
	result, err := p.nginxInspect()
	if err != nil {
		writeServerError(w, err)
		return
	}
	json.NewEncoder(w).Encode(result.Global)
}

// handleNginxSSLConfig serves the real SSL directives from `nginx -T`.
// handleNginxSSLConfig, `nginx -T`'den gerçek SSL direktiflerini sunar.
func (p *Panel) handleNginxSSLConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method == http.MethodPost {
		writeClientError(w, http.StatusNotImplemented, "editing nginx config is not supported yet")
		return
	}
	result, err := p.nginxInspect()
	if err != nil {
		writeServerError(w, err)
		return
	}
	json.NewEncoder(w).Encode(result.SSL)
}

// handleNginxRateLimits serves the real rate-limit zones from `nginx -T`.
// handleNginxRateLimits, `nginx -T`'den gerçek rate-limit bölgelerini sunar.
func (p *Panel) handleNginxRateLimits(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	result, err := p.nginxInspect()
	if err != nil {
		writeServerError(w, err)
		return
	}
	json.NewEncoder(w).Encode(result.RateLimits)
}

// nginxDefaultHTTPSPath is CelikPanel's default HTTPS server on the Nginx
// page (alpha.113): GET reads its state (read only; it never starts the
// reconciliation), POST is the administrator's "Try again", which runs the
// reconciliation once (one request identity per press, D-029) and answers
// the state after it.
//
// GET durumu okur ve hiçbir şeyi değiştirmez; POST "Yeniden dene"dir.
const nginxDefaultHTTPSPath = "/api/v1/nginx/default-https"

type nginxDefaultHTTPSAnswer struct {
	Outcome string                 `json:"outcome,omitempty"`
	Status  core.NginxDefaultHTTPS `json:"status"`
}

func (p *Panel) handleNginxDefaultHTTPS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch r.Method {
	case http.MethodGet:
		result, err := p.nginxInspect()
		if err != nil {
			writeServerError(w, err)
			return
		}
		_ = json.NewEncoder(w).Encode(nginxDefaultHTTPSAnswer{Status: result.DefaultHTTPS})
	case http.MethodPost:
		var response transport.ReconcileDefaultHTTPSServerResponse
		request := transport.ReconcileDefaultHTTPSServerRequest{ExpectedBuildCommit: strings.TrimSpace(buildCommit)}
		if err := p.callAgentContext(r.Context(), "Agent.ReconcileDefaultHTTPSServerV1", &request, &response); err != nil {
			writeServerError(w, err)
			return
		}
		if response.Error != "" {
			writeAgentError(w, errors.New("the default HTTPS server attempt did not run"), response.Error)
			return
		}
		_ = json.NewEncoder(w).Encode(nginxDefaultHTTPSAnswer{Outcome: response.Outcome, Status: response.Status})
	default:
		w.Header().Set("Allow", "GET, POST")
		writeClientError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}
