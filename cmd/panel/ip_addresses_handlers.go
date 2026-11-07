package main

import (
	"encoding/json"
	"net/http"

	"github.com/alicelik/celikpanel/internal/transport"
)

// The server's IP addresses (alpha.128, G14), administrators only (the
// /api/v1/network/ prefix is an administrator prefix). Read only: every site
// answers on every address (the vhosts listen on all of them), so each
// address is shared by all hosted sites. Assigning a dedicated address is
// not offered; see docs/A128-DATA-ACCESS-DESIGN.md.
//
// Sunucunun IP adresleri; yalnız yönetici, salt okunur.

const serverIPAddressesPath = "/api/v1/network/ip-addresses"

func (p *Panel) handleServerIPAddresses(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodGet {
		rejectRouteMethod(w, []string{http.MethodGet})
		return
	}
	var resp transport.ServerIPAddressesResponse
	if err := p.callAgentContext(r.Context(), "Agent.ServerIPAddressesV1", transport.Empty{}, &resp); err != nil {
		writeServerError(w, err)
		return
	}
	if resp.Error != "" {
		writeCodedError(w, http.StatusBadGateway, "IP_ADDRESSES_UNREAD",
			"The server's addresses could not be read just now. Nothing was changed. Try again; on the server, ip addr show lists them.", "")
		return
	}
	var sites int
	if err := p.db.GetDB().QueryRowContext(r.Context(), "SELECT COUNT(*) FROM sites").Scan(&sites); err != nil {
		writeServerError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"addresses": resp.Addresses, "sites": sites, "shared": true})
}
