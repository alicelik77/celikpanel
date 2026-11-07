package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Mail delivery lookup for customers (alpha.128, gap G9).
//
//	GET /api/v1/domains/{id}/mail/delivery?address=…&days=N
//
// "Did my mail arrive?": what happened to the mail of this domain in the last
// days, from Postfix's own log. The domain comes from the route (ownership
// and the mail right are checked before this runs); the Agent returns only
// messages from or to this domain, and of an incoming message only its
// recipients here, so no other customer's mail is shown. Reads only.
//
// "Postam ulaştı mı?": bu alan adının postası, yalnız ona ait olanlar.

// mailDeliveryAgent is replaced by tests.
var mailDeliveryAgent = func(p *Panel, ctx context.Context, req transport.MailDeliveryLookupRequest) (transport.MailDeliveryLookupResponse, error) {
	var resp transport.MailDeliveryLookupResponse
	err := p.callAgentContext(ctx, "Agent.MailDeliveryLookupV1", &req, &resp)
	return resp, err
}

func (p *Panel) handleMailDelivery(w http.ResponseWriter, r *http.Request, domainID int) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		writeClientError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	domain, err := p.domainNameByIDStrict(r.Context(), domainID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	address := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("address")))
	if address != "" {
		if _, err := transport.CanonicalMailAddress(address); err != nil {
			writeClientError(w, http.StatusBadRequest, "the address is not a mail address")
			return
		}
	}
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	if days < 1 || days > transport.MailDeliveryMaxDays {
		days = transport.MailDeliveryMaxDays
	}
	resp, err := mailDeliveryAgent(p, r.Context(), transport.MailDeliveryLookupRequest{Domain: domain, Address: address, Days: days})
	if err != nil {
		writeServerError(w, err)
		return
	}
	if !resp.Read {
		writeCodedError(w, http.StatusServiceUnavailable, "MAIL_LOG_UNREADABLE",
			boundedAgentDiagnostic(firstNonEmptyPanel(resp.Error, "Postfix's log could not be read")), "")
		return
	}
	// The Agent filtered by domain already; this keeps that promise even if
	// an answer ever carried another domain's line.
	kept := make([]transport.MailDeliveryEntry, 0, len(resp.Entries))
	for _, e := range resp.Entries {
		if deliveryDomainOfPanel(e.From) == domain || deliveryDomainOfPanel(e.To) == domain {
			kept = append(kept, e)
		}
	}
	resp.Entries = kept
	_ = json.NewEncoder(w).Encode(map[string]any{"domain": domain, "days": days, "result": resp})
}

func deliveryDomainOfPanel(address string) string {
	at := strings.LastIndexByte(address, '@')
	if at < 0 {
		return ""
	}
	return strings.ToLower(address[at+1:])
}
