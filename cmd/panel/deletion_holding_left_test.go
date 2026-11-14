package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/rpc"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.135: mail an earlier deletion left at an address goes into the
// 30-day holding list before a mailbox is made there, and the folders still
// left in live domain folders are listed on Maintenance and held when the
// administrator asks. Through the production handlers, with the Agent faked
// at its calls.

const leftTestName = "left-6103-0123456789abcdef"

func callerRequest(method, target, body string, caller *Caller) *http.Request {
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	if caller == nil {
		return request
	}
	return request.WithContext(context.WithValue(request.Context(), callerKey, caller))
}

var (
	leftAdmin    = &Caller{ID: 6101, Role: roleAdmin, AccountType: core.AccountTypeAccount}
	leftCustomer = &Caller{ID: 6104, Role: roleCustomer, AccountType: core.AccountTypeAccount}
)

// Making a mailbox over a folder left at its address: the Agent is asked to
// put the folder aside first (with the domain's record), the list keeps it 30
// days as left by an earlier deletion, the administrator's answer says so in
// one line, and nobody else is told.
func TestMailboxMadeOverLeftMailPutsItInTheListFirst(t *testing.T) {
	now := time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC)
	fakeHolding(t, now)
	for _, caller := range []*Caller{leftAdmin, leftCustomer} {
		agent := &mailMutationPanelAgent{addApplied: true, addHeldName: leftTestName, addHeldBytes: 4096}
		panel, domainID := newMailMutationPanel(t, agent)
		if _, err := panel.db.GetDB().Exec(`INSERT INTO users (id, username, password_hash, email, role) VALUES (6104, 'mail-customer', 'x', 'mail-customer@example.test', 'customer')`); err != nil {
			t.Fatal(err)
		}
		recorder := httptest.NewRecorder()
		panel.handleAddEmailAccount(recorder, callerRequest(http.MethodPost, "/mail/accounts",
			`{"address":"info","password":"Long-enough-1","quota_mb":100}`, caller), domainID)
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s: create = %d %s", caller.Role, recorder.Code, recorder.Body.String())
		}
		if len(agent.addRequests) != 1 || agent.addRequests[0].LeftDomainID != domainID || agent.addRequests[0].Email != "info@example.com" {
			t.Fatalf("agent request = %+v", agent.addRequests)
		}
		var answer struct {
			Success  bool             `json:"success"`
			LeftHeld *holdingKeptView `json:"left_held"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil || !answer.Success {
			t.Fatalf("answer = %s", recorder.Body.String())
		}
		until := now.Add(holdingKeep).Format(time.RFC3339)
		if caller == leftAdmin && (answer.LeftHeld == nil || answer.LeftHeld.RemoveAfter != until || answer.LeftHeld.Bytes != 4096) {
			t.Fatalf("the administrator was not told: %s", recorder.Body.String())
		}
		if caller != leftAdmin && answer.LeftHeld != nil {
			t.Fatalf("a customer was told about mail that is not theirs: %s", recorder.Body.String())
		}
		rec, ok := panel.readHoldingRecord(context.Background(), holdingMailID(leftTestName))
		if !ok || rec.Mailbox != "info@example.com" || rec.Domain != "example.com" || rec.DomainID != domainID || rec.RemoveAfter != until || rec.Bytes != 4096 {
			t.Fatalf("record = %+v, %v", rec, ok)
		}
		if holdingCount(t, panel, "deletion_hold.left:"+rec.ID) != 1 {
			t.Fatal("no audit row")
		}
		// The list says what it is: left by an earlier deletion, and kept.
		saved := holdingAgentLeft
		holdingAgentLeft = func(*Panel, context.Context) (transport.LeftMailFoldersResponse, error) {
			return transport.LeftMailFoldersResponse{Folders: []transport.LeftMailFolder{}}, nil
		}
		list := httptest.NewRecorder()
		panel.handleDeletionHolding(list, holdingRequestAs(t, http.MethodGet, holdingPath, roleAdmin))
		holdingAgentLeft = saved
		if !strings.Contains(list.Body.String(), `"left":true`) || !strings.Contains(list.Body.String(), `"state":"kept"`) {
			t.Fatalf("list = %s", list.Body.String())
		}
	}
}

// A mailbox made where nothing was left holds nothing and says nothing.
func TestMailboxMadeWithoutLeftMailSaysNothingMore(t *testing.T) {
	fakeHolding(t, time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC))
	agent := &mailMutationPanelAgent{addApplied: true}
	panel, domainID := newMailMutationPanel(t, agent)
	recorder := httptest.NewRecorder()
	panel.handleAddEmailAccount(recorder, callerRequest(http.MethodPost, "/mail/accounts",
		`{"address":"info","password":"Long-enough-1","quota_mb":100}`, leftAdmin), domainID)
	if recorder.Code != http.StatusOK || strings.Contains(recorder.Body.String(), "left_held") {
		t.Fatalf("create = %d %s", recorder.Code, recorder.Body.String())
	}
	if records, _ := panel.readHoldingRecords(context.Background()); len(records) != 0 {
		t.Fatalf("records = %+v", records)
	}
}

type fakeLeftAgent struct {
	folders transport.LeftMailFoldersResponse
	listErr error
	asked   []transport.HoldLeftMailFolderRequest
	answer  func(transport.HoldLeftMailFolderRequest) (transport.HoldLeftMailFolderResponse, error)
}

func fakeLeft(t *testing.T) *fakeLeftAgent {
	t.Helper()
	fake := &fakeLeftAgent{answer: func(transport.HoldLeftMailFolderRequest) (transport.HoldLeftMailFolderResponse, error) {
		return transport.HoldLeftMailFolderResponse{Held: true, HeldName: leftTestName, HeldBytes: 3}, nil
	}}
	savedList, savedHold := holdingAgentLeft, holdingAgentHoldLeft
	holdingAgentLeft = func(*Panel, context.Context) (transport.LeftMailFoldersResponse, error) {
		return fake.folders, fake.listErr
	}
	holdingAgentHoldLeft = func(_ *Panel, _ context.Context, req transport.HoldLeftMailFolderRequest) (transport.HoldLeftMailFolderResponse, error) {
		fake.asked = append(fake.asked, req)
		return fake.answer(req)
	}
	t.Cleanup(func() { holdingAgentLeft, holdingAgentHoldLeft = savedList, savedHold })
	return fake
}

func holdLeftAs(t *testing.T, p *Panel, role, address string) *httptest.ResponseRecorder {
	t.Helper()
	request := holdingRequestAs(t, http.MethodPost, holdingPath+"/"+holdingLeftPath, role)
	request.Body = io.NopCloser(strings.NewReader(`{"address":"` + address + `"}`))
	recorder := httptest.NewRecorder()
	p.handleDeletionHoldingItem(recorder, request)
	return recorder
}

// The list shows only folders of this panel's live domains that have no
// mailbox; a list the Agent could not give is said as not known; nothing is
// moved by reading it.
func TestLeftMailIsListedForTheAdministratorOnly(t *testing.T) {
	now := time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC)
	fakeHolding(t, now)
	fake := fakeLeft(t)
	panel, _ := newMailMutationPanel(t, &mailMutationPanelAgent{})
	if _, err := panel.db.GetDB().Exec(`INSERT INTO email_accounts (domain_id, address, password_hash, quota_mb) VALUES (6103, 'info@example.com', 'x', 10)`); err != nil {
		t.Fatal(err)
	}
	fake.folders = transport.LeftMailFoldersResponse{Folders: []transport.LeftMailFolder{
		{Address: "old@example.com", Bytes: 3, Path: "/var/mail/vhosts/example.com/old"},
		{Address: "info@example.com", Bytes: 9},
		{Address: "x@not-here.example", Bytes: 1},
	}}
	list := httptest.NewRecorder()
	panel.handleDeletionHolding(list, holdingRequestAs(t, http.MethodGet, holdingPath, roleAdmin))
	var body struct {
		Left        []leftMailView `json:"left"`
		LeftUnknown bool           `json:"left_unknown"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &body); err != nil || len(body.Left) != 1 || body.LeftUnknown ||
		body.Left[0].Address != "old@example.com" || body.Left[0].DomainID != 6103 || body.Left[0].Domain != "example.com" || body.Left[0].Bytes != 3 {
		t.Fatalf("list = %s", list.Body.String())
	}
	if len(fake.asked) != 0 {
		t.Fatal("reading the list moved something")
	}
	fake.listErr = errors.New("connection reset")
	list = httptest.NewRecorder()
	panel.handleDeletionHolding(list, holdingRequestAs(t, http.MethodGet, holdingPath, roleAdmin))
	if !strings.Contains(list.Body.String(), `"left_unknown":true`) || !strings.Contains(list.Body.String(), `"left":[]`) {
		t.Fatalf("unknown list = %s", list.Body.String())
	}
	for _, role := range []string{roleReseller, roleCustomer, "team"} {
		if recorder := holdLeftAs(t, panel, role, "old@example.com"); recorder.Code != http.StatusForbidden {
			t.Fatalf("%s hold = %d", role, recorder.Code)
		}
	}
	if len(fake.asked) != 0 {
		t.Fatal("a refused caller moved something")
	}
}

// "Keep 30 days…": the Agent is asked with the address and the domain's
// record; the list keeps it; a mailbox at the address, a domain being
// deleted, a refusal and a lost answer are each said as what they are.
func TestHoldingLeftMailOnAsk(t *testing.T) {
	now := time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC)
	fakeHolding(t, now)
	fake := fakeLeft(t)
	panel, domainID := newMailMutationPanel(t, &mailMutationPanelAgent{})

	done := holdLeftAs(t, panel, roleAdmin, "old@example.com")
	if done.Code != http.StatusOK || !strings.Contains(done.Body.String(), `"status":"held"`) ||
		!strings.Contains(done.Body.String(), now.Add(holdingKeep).Format(time.RFC3339)) {
		t.Fatalf("hold = %d %s", done.Code, done.Body.String())
	}
	if len(fake.asked) != 1 || fake.asked[0].Email != "old@example.com" || fake.asked[0].DomainID != domainID ||
		fake.asked[0].ExpectedBuildCommit != strings.TrimSpace(buildCommit) {
		t.Fatalf("asked = %+v", fake.asked)
	}
	if rec, ok := panel.readHoldingRecord(context.Background(), holdingMailID(leftTestName)); !ok || rec.Mailbox != "old@example.com" || !holdingLeftItem(rec) {
		t.Fatalf("record = %+v %v", rec, ok)
	}
	// Pressed again after it went: absent, nothing new.
	fake.answer = func(transport.HoldLeftMailFolderRequest) (transport.HoldLeftMailFolderResponse, error) {
		return transport.HoldLeftMailFolderResponse{Absent: true}, nil
	}
	if again := holdLeftAs(t, panel, roleAdmin, "old@example.com"); again.Code != http.StatusOK || !strings.Contains(again.Body.String(), `"status":"absent"`) {
		t.Fatalf("again = %d %s", again.Code, again.Body.String())
	}
	// A refusal keeps the folder and says the reason.
	fake.answer = func(transport.HoldLeftMailFolderRequest) (transport.HoldLeftMailFolderResponse, error) {
		return transport.HoldLeftMailFolderResponse{Error: "inspect the mailbox folder: too many levels of symbolic links"}, nil
	}
	if refused := holdLeftAs(t, panel, roleAdmin, "link@example.com"); refused.Code != http.StatusConflict || !strings.Contains(refused.Body.String(), "HOLDING_LEFT_FAILED") {
		t.Fatalf("refused = %d %s", refused.Code, refused.Body.String())
	}
	// No answer: not known, said so.
	fake.answer = func(transport.HoldLeftMailFolderRequest) (transport.HoldLeftMailFolderResponse, error) {
		return transport.HoldLeftMailFolderResponse{}, errors.New("read: connection reset")
	}
	if lost := holdLeftAs(t, panel, roleAdmin, "lost@example.com"); lost.Code != http.StatusBadGateway || !strings.Contains(lost.Body.String(), "HOLDING_LEFT_UNKNOWN") {
		t.Fatalf("lost = %d %s", lost.Code, lost.Body.String())
	}
	// The Agent's own refusal (a server error) is a failure, not unknown.
	fake.answer = func(transport.HoldLeftMailFolderRequest) (transport.HoldLeftMailFolderResponse, error) {
		return transport.HoldLeftMailFolderResponse{}, rpc.ServerError("panel/agent build mismatch")
	}
	if refused := holdLeftAs(t, panel, roleAdmin, "gate@example.com"); refused.Code != http.StatusConflict {
		t.Fatalf("server error = %d %s", refused.Code, refused.Body.String())
	}
	asked := len(fake.asked)
	// An address that has a mailbox again: refused before the Agent.
	if _, err := panel.db.GetDB().Exec(`INSERT INTO email_accounts (domain_id, address, password_hash, quota_mb) VALUES (?, 'info@example.com', 'x', 10)`, domainID); err != nil {
		t.Fatal(err)
	}
	if inUse := holdLeftAs(t, panel, roleAdmin, "info@example.com"); inUse.Code != http.StatusConflict || !strings.Contains(inUse.Body.String(), "HOLDING_LEFT_IN_USE") {
		t.Fatalf("in use = %d %s", inUse.Code, inUse.Body.String())
	}
	// Not a domain here, or a domain being deleted: refused before the Agent.
	if missing := holdLeftAs(t, panel, roleAdmin, "a@not-here.example"); missing.Code != http.StatusNotFound {
		t.Fatalf("not a domain = %d", missing.Code)
	}
	if _, err := panel.db.GetDB().Exec(`INSERT INTO domain_deletion_operations (domain_id, previous_status) VALUES (?, 'active')`, domainID); err != nil {
		t.Fatal(err)
	}
	if pending := holdLeftAs(t, panel, roleAdmin, "old2@example.com"); pending.Code != http.StatusConflict || !strings.Contains(pending.Body.String(), errCodeDomainDeletionPending) {
		t.Fatalf("pending = %d %s", pending.Code, pending.Body.String())
	}
	if len(fake.asked) != asked {
		t.Fatal("a refused hold reached the Agent")
	}
}

// A held folder that was left in place has no owner to wait for: the step
// removes it when its date has come, though its domain lives on; one the
// Agent finds without a record is adopted with its live domain's name.
func TestLeftMailIsRemovedWhenDueAndAdoptedWithItsDomain(t *testing.T) {
	now := time.Date(2026, 12, 20, 10, 0, 0, 0, time.UTC)
	fake := fakeHolding(t, now)
	panel, domainID := newMailMutationPanel(t, &mailMutationPanelAgent{})
	due := holdingRecord{ID: holdingMailID(leftTestName), Kind: transport.HeldItemMail, Name: leftTestName, DomainID: domainID, Domain: "example.com", Mailbox: "old@example.com"}
	seedHoldingRecord(t, panel, due, now.Add(-time.Minute))
	other := "left-6103-fedcba9876543210"
	fake.inventory = transport.DeletionHoldingInventoryResponse{Items: []transport.HeldItemInfo{
		{Kind: transport.HeldItemMail, Name: leftTestName, DomainID: domainID, Bytes: 3},
		{Kind: transport.HeldItemMail, Name: other, DomainID: domainID, Bytes: 7},
	}}
	panel.runHoldingStep(context.Background())
	if len(fake.removed) != 1 || fake.removed[0].Name != leftTestName {
		t.Fatalf("removed = %+v", fake.removed)
	}
	adopted, ok := panel.readHoldingRecord(context.Background(), holdingMailID(other))
	if !ok || !adopted.Adopted || adopted.Domain != "example.com" || adopted.RemoveAfter != now.Add(holdingKeep).Format(time.RFC3339) {
		t.Fatalf("adopted = %+v %v", adopted, ok)
	}
}
