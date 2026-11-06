package main

import (
	"encoding/json"
	"net/http"

	"github.com/alicelik/celikpanel/internal/transport"
)

// The DNS check tool on a backup DNS server (6 Nov 2026; Agent side
// cmd/agent/dns_peer_inspector_tool_linux.go). The Agent puts the running
// release's copy in place by itself when it starts after the owner's update,
// over a CelikPanel-shipped or missing file only. The one case with an owner
// step is a file the owner changed: this read names it and the one command.
//
//	GET /api/v1/dns/peer-inspector-tool   administrator: the state (read only)
//
// Yedek DNS sunucusundaki denetim aracının durumu; yalnız okur.

const dnsPeerInspectorToolPath = "/api/v1/dns/peer-inspector-tool"

func (p *Panel) handleDNSPeerInspectorTool(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if !requireServerSetupAdmin(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		rejectRouteMethod(w, []string{http.MethodGet})
		return
	}
	var status transport.DNSPeerInspectorToolStatusResponse
	if err := p.callAgentContext(r.Context(), "Agent.DNSPeerInspectorToolStatusV1", &transport.DNSPeerInspectorToolStatusRequest{}, &status); err != nil {
		writeServerError(w, err)
		return
	}
	if status.Tools == nil {
		status.Tools = []transport.DNSPeerInspectorTool{}
	}
	_ = json.NewEncoder(w).Encode(status)
}
