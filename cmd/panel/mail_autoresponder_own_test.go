package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Review of alpha.143, finding 1 (21 Nov 2026): an additional address of
// another account's domain, even one that account forwards to the mailbox, or
// the owner's own, is refused before the Agent is asked; the neutral refusal
// names the address and never whose it is. The mailbox's own alias is taken
// and the account's domains go to the Agent for its own check.
func TestMailAutoresponderRefusesAnotherAccountsAddress(t *testing.T) {
	agent := &autoresponderPanelAgent{answer: transport.MailAutoresponderResponse{Applied: true, Script: transport.AutoresponderScriptManaged, SieveAvailable: true}}
	panel, domainID := newAutoresponderPanel(t, agent)
	for _, address := range []string{"ceo@b-customer.test", "no-reply@owner-main.test", "boss@example.com"} {
		response := putAutoresponder(panel, domainID, `{"id":6201,"enabled":true,"subject":"Away","message":"Pay here","addresses":["info@example.com","`+address+`"]}`)
		var body apiErrorBody
		if response.Code != http.StatusConflict || json.Unmarshal(response.Body.Bytes(), &body) != nil ||
			body.Code != errCodeMailAutoresponderRefused || body.Reason != transport.AutoresponderRefusedForeignAddress ||
			!strings.Contains(body.Error, address) || strings.Contains(body.Error, "another-customer") || strings.Contains(body.Error, "info@example.com") {
			t.Fatalf("%s: %d %s", address, response.Code, response.Body.String())
		}
	}
	if calls := agent.calls(); len(calls) != 0 {
		t.Fatalf("the Agent was asked: %+v", calls)
	}
	var rows int
	if err := panel.db.GetDB().QueryRow(`SELECT COUNT(*) FROM email_autoresponders`).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("rows=%d %v", rows, err)
	}
	// The mailbox's own alias and a domain of the same account.
	if _, err := panel.db.GetDB().Exec(`INSERT INTO email_forwardings (domain_id, source, destination) VALUES (6104, 'sales@other.test', 'me@example.com')`); err != nil {
		t.Fatal(err)
	}
	response := putAutoresponder(panel, domainID, `{"id":6201,"enabled":true,"subject":"Away","message":"x","addresses":["info@example.com","sales@other.test"]}`)
	if response.Code != http.StatusOK {
		t.Fatalf("own addresses: %d %s", response.Code, response.Body.String())
	}
	calls := agent.calls()
	if len(calls) != 1 || strings.Join(calls[0].AccountDomains, ",") != "example.com,other.test" {
		t.Fatalf("agent calls: %+v", calls)
	}
	// Turning the reply off is never refused for its addresses.
	response = putAutoresponder(panel, domainID, `{"id":6201,"enabled":false,"subject":"Away","message":"x","addresses":["ceo@b-customer.test"]}`)
	if response.Code != http.StatusOK {
		t.Fatalf("turning off: %d %s", response.Code, response.Body.String())
	}
}

// A reply saved before the rule names another account's address: at the next
// start it is rendered again without it, the stored row follows, the reply
// window says once which address was removed (until the reply is saved
// again), and it is audited.
func TestMailAutoresponderStoredForeignAddressIsRemovedAtStart(t *testing.T) {
	agent := &autoresponderPanelAgent{answer: transport.MailAutoresponderResponse{Applied: true, Script: transport.AutoresponderScriptManaged, SieveAvailable: true}}
	panel, domainID := newAutoresponderPanel(t, agent)
	if _, err := panel.db.GetDB().Exec(`
		INSERT INTO email_autoresponders (account_id, enabled, subject, message, interval_days, addresses)
		VALUES (6201, 1, 'Away', 'Pay here', 1, '["info@example.com","ceo@b-customer.test"]'),
		       (6202, 1, 'Away', 'x', 1, '["you@other.test"]')`); err != nil {
		t.Fatal(err)
	}
	if _, err := panel.db.GetDB().Exec(`INSERT INTO panel_settings (key, value) VALUES (?, '{"addresses":["x@y.test"]}')`,
		autoresponderRemovedSettingPrefix+"999999"); err != nil {
		t.Fatal(err)
	}
	panel.reconcileAutoresponderAddresses(context.Background())

	calls := agent.calls()
	if len(calls) != 1 || calls[0].Email != "me@example.com" || strings.Join(calls[0].Autoresponder.Addresses, ",") != "info@example.com" ||
		!calls[0].Autoresponder.Enabled || calls[0].Autoresponder.Message != "Pay here" {
		t.Fatalf("agent calls: %+v", calls)
	}
	var addresses string
	if err := panel.db.GetDB().QueryRow(`SELECT addresses FROM email_autoresponders WHERE account_id = 6201`).Scan(&addresses); err != nil || addresses != `["info@example.com"]` {
		t.Fatalf("stored %s %v", addresses, err)
	}
	var audited int
	if err := panel.db.GetDB().QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action = 'mail.autoresponder.addresses_removed' AND resource_id = 6201`).Scan(&audited); err != nil || audited != 1 {
		t.Fatalf("audit rows %d %v", audited, err)
	}
	if panel.setting(context.Background(), autoresponderRemovedSettingPrefix+"999999") != "" {
		t.Fatal("a notice of a mailbox that is gone was left behind")
	}
	view := func() mailAutoresponderView {
		request := httptest.NewRequest(http.MethodGet, "/api/v1/domains/6103/mail/autoresponder?id=6201", nil)
		got := httptest.NewRecorder()
		panel.handleMailAutoresponder(got, request, domainID)
		var view mailAutoresponderView
		if err := json.Unmarshal(got.Body.Bytes(), &view); err != nil {
			t.Fatal(err)
		}
		return view
	}
	if got := view(); strings.Join(got.RemovedAddresses, ",") != "ceo@b-customer.test" {
		t.Fatalf("view %+v", got)
	}
	// A second start changes nothing.
	panel.reconcileAutoresponderAddresses(context.Background())
	if calls := agent.calls(); len(calls) != 1 {
		t.Fatalf("second start asked the Agent again: %+v", calls)
	}
	// Saved again: the notice is gone.
	if response := putAutoresponder(panel, domainID, `{"id":6201,"enabled":true,"subject":"Away","message":"Back","addresses":["info@example.com"]}`); response.Code != http.StatusOK {
		t.Fatalf("save: %d %s", response.Code, response.Body.String())
	}
	if got := view(); len(got.RemovedAddresses) != 0 {
		t.Fatalf("the notice stayed after a save: %+v", got)
	}
}
