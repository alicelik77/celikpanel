package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Second review of alpha.143 (22 Nov 2026), the out-of-office rule after the
// account's names change (B2), a rewrite that cannot be made (B3) and the
// start-time reconcile against a customer's save (B9).

func autoresponderAddressesOf(t *testing.T, panel *Panel, accountID int) (string, int) {
	t.Helper()
	var addresses string
	var enabled int
	if err := panel.db.GetDB().QueryRow(`SELECT addresses, enabled FROM email_autoresponders WHERE account_id = ?`, accountID).Scan(&addresses, &enabled); err != nil {
		t.Fatal(err)
	}
	return addresses, enabled
}

// B2: a subscription transfer moves a domain to another account; a reply of
// the old account that names an address of it is rewritten by the transfer
// request itself, before it answers, not at the next start.
func TestMailAutoresponderTransferRewritesTheReplyInTheSameRequest(t *testing.T) {
	f := newTransferFixture(t)
	agent := &autoresponderPanelAgent{answer: transport.MailAutoresponderResponse{Applied: true, Script: transport.AutoresponderScriptManaged, SieveAvailable: true}}
	f.panel.agentClient = autoresponderAgentClient(t, agent)
	var moved string
	if err := f.panel.db.GetDB().QueryRow(`SELECT name FROM domains WHERE id = 9902`).Scan(&moved); err != nil {
		t.Fatal(err)
	}
	f.exec(t, `INSERT INTO subscriptions (id, owner_id, name, status) VALUES (9890, 9703, 'stays subscription', 'active')`)
	f.exec(t, `INSERT INTO domains (id, subscription_id, name, status) VALUES (9990, 9890, 'stays.matrix.test', 'active')`)
	f.exec(t, `INSERT INTO email_accounts (id, domain_id, address, password_hash, quota_mb) VALUES (9991, 9990, 'me@stays.matrix.test', 'x', 100)`)
	f.exec(t, `INSERT INTO email_forwardings (domain_id, source, destination) VALUES (9902, 'boss@`+moved+`', 'me@stays.matrix.test')`)
	f.exec(t, `INSERT INTO email_autoresponders (account_id, enabled, subject, message, interval_days, addresses)
		VALUES (9991, 1, 'Away', 'Pay here', 1, '["boss@`+moved+`"]')`)

	wantStatus(t, f.do(t, authzMatrixAdminID, http.MethodPost, "/api/v1/subscriptions/9802/transfer",
		`{"to_user_id":9708,"from_user_id":9703,"request_id":"move-reply-0001","confirm":true}`), http.StatusOK, "")
	if f.owner(t, 9802) != 9708 {
		t.Fatal("the transfer did not happen")
	}
	calls := agent.calls()
	if len(calls) != 1 || calls[0].Email != "me@stays.matrix.test" || len(calls[0].Autoresponder.Addresses) != 0 || !calls[0].Autoresponder.Enabled {
		t.Fatalf("the transfer did not rewrite the reply: %+v", calls)
	}
	if addresses, enabled := autoresponderAddressesOf(t, f.panel, 9991); addresses != `[]` || enabled != 1 {
		t.Fatalf("stored %s enabled %d", addresses, enabled)
	}
	if f.count(t, `SELECT COUNT(*) FROM audit_logs WHERE action = 'mail.autoresponder.addresses_removed' AND resource_id = 9991 AND details LIKE '%subscription_transfer%'`) != 1 {
		t.Fatal("the rewrite after the transfer was not audited")
	}
}

// B2: removing the forwarder that made an additional address the mailbox's
// own rewrites the reply in the same request.
func TestMailAutoresponderForwarderRemovalRewritesTheReplyInTheSameRequest(t *testing.T) {
	agent := &autoresponderPanelAgent{answer: transport.MailAutoresponderResponse{Applied: true, Script: transport.AutoresponderScriptManaged, SieveAvailable: true}}
	panel, _ := newAutoresponderPanel(t, agent)
	result, err := panel.db.GetDB().Exec(`INSERT INTO email_forwardings (domain_id, source, destination) VALUES (6104, 'sales@other.test', 'me@example.com')`)
	if err != nil {
		t.Fatal(err)
	}
	forwardingID, _ := result.LastInsertId()
	if _, err := panel.db.GetDB().Exec(`INSERT INTO email_autoresponders (account_id, enabled, subject, message, interval_days, addresses)
		VALUES (6201, 1, 'Away', 'x', 1, '["sales@other.test","info@example.com"]')`); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodDelete, "/api/v1/domains/6104/mail/forwardings?id="+strconv.FormatInt(forwardingID, 10), nil)
	request = request.WithContext(context.WithValue(request.Context(), callerKey, &Caller{ID: 6101, Role: roleAdmin, AccountType: core.AccountTypeAccount}))
	response := httptest.NewRecorder()
	panel.handleDeleteEmailForwarding(response, request, 6104)
	if response.Code != http.StatusOK {
		t.Fatalf("delete forwarding: %d %s", response.Code, response.Body.String())
	}
	calls := agent.calls()
	if len(calls) != 1 || strings.Join(calls[0].Autoresponder.Addresses, ",") != "info@example.com" {
		t.Fatalf("agent calls: %+v", calls)
	}
	if addresses, _ := autoresponderAddressesOf(t, panel, 6201); addresses != `["info@example.com"]` {
		t.Fatalf("stored %s", addresses)
	}
}

// B3: a rewrite the Agent cannot make takes the reply off and the window says
// so; when even that cannot be applied, the administrator gets one Problems
// now line naming the mailbox and the step, the window says it, and the next
// round finishes it.
func TestMailAutoresponderRewriteThatCannotBeMadeIsNotSilent(t *testing.T) {
	agent := &autoresponderPanelAgent{}
	panel, domainID := newAutoresponderPanel(t, agent)
	seed := func() {
		if _, err := panel.db.GetDB().Exec(`DELETE FROM email_autoresponders; DELETE FROM panel_settings;
			INSERT INTO email_autoresponders (account_id, enabled, subject, message, interval_days, addresses)
			VALUES (6201, 1, 'Away', 'Pay here', 1, '["info@example.com","ceo@b-customer.test"]')`); err != nil {
			t.Fatal(err)
		}
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

	// Dovecot stopped: the rewrite is refused, turning the reply off is not.
	seed()
	agent.mu.Lock()
	agent.respond = func(request *transport.SetMailAutoresponderRequest) (transport.MailAutoresponderResponse, error) {
		if request.Autoresponder.Enabled {
			return transport.MailAutoresponderResponse{Refusal: transport.AutoresponderRefusedDovecotStopped, Detail: "Dovecot is stopped"}, nil
		}
		return transport.MailAutoresponderResponse{Applied: true, Script: transport.AutoresponderScriptNone}, nil
	}
	agent.mu.Unlock()
	panel.reconcileAutoresponderAddresses(context.Background())
	if addresses, enabled := autoresponderAddressesOf(t, panel, 6201); addresses != `["info@example.com"]` || enabled != 0 {
		t.Fatalf("taken off: stored %s enabled %d", addresses, enabled)
	}
	if got := view(); strings.Join(got.RemovedAddresses, ",") != "ceo@b-customer.test" || !got.RemovedTurnedOff || len(got.ForeignPending) != 0 {
		t.Fatalf("view after taking off: %+v", got)
	}
	if problems := autoresponderForeignProblems(context.Background(), panel.db.GetDB()); len(problems) != 0 {
		t.Fatalf("a reply that was taken off is listed: %+v", problems)
	}

	// The Agent is another build: it refuses both; the foreign address stays
	// active and the administrator is told, with the step.
	seed()
	agent.mu.Lock()
	agent.setCalls = nil
	agent.respond = func(*transport.SetMailAutoresponderRequest) (transport.MailAutoresponderResponse, error) {
		return transport.MailAutoresponderResponse{}, errors.New("panel/agent build mismatch (panel new, agent old); finish the paired upgrade before setting an out-of-office reply")
	}
	agent.mu.Unlock()
	panel.reconcileAutoresponderAddresses(context.Background())
	if calls := agent.calls(); len(calls) != 2 || !calls[0].Autoresponder.Enabled || calls[1].Autoresponder.Enabled {
		t.Fatalf("expected a rewrite then a turn-off: %+v", calls)
	}
	if addresses, enabled := autoresponderAddressesOf(t, panel, 6201); addresses != `["info@example.com","ceo@b-customer.test"]` || enabled != 1 {
		t.Fatalf("nothing applied, so nothing may be stored: %s %d", addresses, enabled)
	}
	problems := autoresponderForeignProblems(context.Background(), panel.db.GetDB())
	if len(problems) != 1 || problems[0].Kind != "mail_autoresponder_foreign" || problems[0].Name != "me@example.com" ||
		problems[0].Subject != autoresponderStepAgentBuild || problems[0].Actor != "administrator" ||
		problems[0].Key != "mail_autoresponder_foreign:6201" || problems[0].Action.To != "/domains/example.com?tab=mail" || problems[0].Since == "" {
		t.Fatalf("problems %+v", problems)
	}
	report := panel.collectMonitoringProblems(context.Background(), time.Now())
	listed := false
	for _, problem := range report.Problems {
		listed = listed || problem.Kind == "mail_autoresponder_foreign"
	}
	if !listed {
		t.Fatalf("not in Problems now: %+v", report.Problems)
	}
	if got := view(); strings.Join(got.ForeignPending, ",") != "ceo@b-customer.test" {
		t.Fatalf("the window does not say it: %+v", got)
	}

	// No answer at all: the step says so.
	agent.mu.Lock()
	agent.respond = nil
	agent.mu.Unlock()
	panel.agentClient = nil
	panel.reconcileAutoresponderAddresses(context.Background())
	if problems := autoresponderForeignProblems(context.Background(), panel.db.GetDB()); len(problems) != 1 || problems[0].Subject != autoresponderStepAgentUnreachable {
		t.Fatalf("unreachable: %+v", problems)
	}

	// The next round, with the Agent back: done, and the line goes.
	panel.agentClient = autoresponderAgentClient(t, agent)
	agent.mu.Lock()
	agent.answer = transport.MailAutoresponderResponse{Applied: true, Script: transport.AutoresponderScriptManaged, SieveAvailable: true}
	agent.mu.Unlock()
	panel.reconcileAutoresponderAddressesAfter(context.Background(), "hourly")
	if addresses, enabled := autoresponderAddressesOf(t, panel, 6201); addresses != `["info@example.com"]` || enabled != 1 {
		t.Fatalf("after the round: %s %d", addresses, enabled)
	}
	if problems := autoresponderForeignProblems(context.Background(), panel.db.GetDB()); len(problems) != 0 {
		t.Fatalf("the line stayed: %+v", problems)
	}
	if got := view(); len(got.ForeignPending) != 0 || got.RemovedTurnedOff {
		t.Fatalf("view after the round: %+v", got)
	}
}

// B4: a main.cf filter list the Agent cannot read is a problem with its own
// state (two of them), never the words of a hand-changed port-25 entry; the
// state in which nothing is signed has its own alert words. B3's kind has its
// place in the order and its alert words.
func TestMailSigningVariableListStatesAndForeignReplyAreWorded(t *testing.T) {
	for _, state := range []string{"port25_variable", "port25_variable_own"} {
		raw, _ := json.Marshal(transport.MailSigningState{State: state, Detail: "x", Subject: "smtp/inet"})
		problems := mailSigningProblems(string(raw))
		if len(problems) != 1 || problems[0].Name != state || problems[0].Subject != "smtp/inet" {
			t.Errorf("%s: %+v", state, problems)
		}
	}
	signs, _ := alertLine(pendingAlertMail{kind: "mail_signing", name: "port25_kept"})
	quiet, _ := alertLine(pendingAlertMail{kind: "mail_signing", name: "port25_variable_own"})
	if quiet == signs || strings.Contains(quiet, "sign mail for a hosted domain") {
		t.Fatalf("port25_variable_own alert: %q", quiet)
	}
	if _, ordered := problemKindOrder["mail_autoresponder_foreign"]; !ordered {
		t.Fatal("mail_autoresponder_foreign has no place in the list's order")
	}
	if en, tr := alertLine(pendingAlertMail{kind: "mail_autoresponder_foreign", name: "me@example.com"}); !strings.Contains(en, "me@example.com") || !strings.Contains(tr, "me@example.com") {
		t.Fatalf("alert words: %q / %q", en, tr)
	}
}

// B9: the reconcile reads each reply again under the mail lock, so a save the
// customer made while the reconcile waited is neither overwritten nor sent to
// the Agent in its old form.
func TestMailAutoresponderReconcileDoesNotOverwriteASaveMadeMeanwhile(t *testing.T) {
	agent := &autoresponderPanelAgent{answer: transport.MailAutoresponderResponse{Applied: true, Script: transport.AutoresponderScriptManaged, SieveAvailable: true}}
	panel, _ := newAutoresponderPanel(t, agent)
	if _, err := panel.db.GetDB().Exec(`INSERT INTO email_autoresponders (account_id, enabled, subject, message, interval_days, addresses)
		VALUES (6201, 1, 'Away', 'Old text', 1, '["info@example.com","ceo@b-customer.test"]')`); err != nil {
		t.Fatal(err)
	}
	// The customer's save holds the lock (as handlePutMailAutoresponder does).
	panel.mailMutationMu.Lock()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		panel.reconcileAutoresponderAddresses(context.Background())
	}()
	time.Sleep(300 * time.Millisecond)
	if _, err := panel.db.GetDB().Exec(`UPDATE email_autoresponders SET subject = 'Back', message = 'New text', addresses = '["info@example.com"]' WHERE account_id = 6201`); err != nil {
		t.Fatal(err)
	}
	panel.mailMutationMu.Unlock()
	wg.Wait()
	if calls := agent.calls(); len(calls) != 0 {
		t.Fatalf("the reconcile sent a reply read before the save: %+v", calls)
	}
	var subject, message, addresses string
	if err := panel.db.GetDB().QueryRow(`SELECT subject, message, addresses FROM email_autoresponders WHERE account_id = 6201`).Scan(&subject, &message, &addresses); err != nil {
		t.Fatal(err)
	}
	if subject != "Back" || message != "New text" || addresses != `["info@example.com"]` {
		t.Fatalf("the save was overwritten: %s %s %s", subject, message, addresses)
	}
}
