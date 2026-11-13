package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.135: what deletions left is kept 30 days in one list and then
// removed; the tests go through the production handlers and the periodic
// step with the Agent faked at its two calls.

type fakeHoldingAgent struct {
	inventory transport.DeletionHoldingInventoryResponse
	removed   []transport.RemoveHeldItemRequest
	answer    func(transport.RemoveHeldItemRequest) (transport.RemoveHeldItemResponse, error)
}

func fakeHolding(t *testing.T, now time.Time) *fakeHoldingAgent {
	t.Helper()
	fake := &fakeHoldingAgent{answer: func(transport.RemoveHeldItemRequest) (transport.RemoveHeldItemResponse, error) {
		return transport.RemoveHeldItemResponse{Removed: true}, nil
	}}
	savedInventory, savedRemove, savedNow := holdingAgentInventory, holdingAgentRemove, holdingNow
	holdingAgentInventory = func(*Panel, context.Context) (transport.DeletionHoldingInventoryResponse, error) {
		return fake.inventory, nil
	}
	holdingAgentRemove = func(_ *Panel, _ context.Context, req transport.RemoveHeldItemRequest) (transport.RemoveHeldItemResponse, error) {
		fake.removed = append(fake.removed, req)
		return fake.answer(req)
	}
	holdingNow = func() time.Time { return now }
	t.Cleanup(func() { holdingAgentInventory, holdingAgentRemove, holdingNow = savedInventory, savedRemove, savedNow })
	return fake
}

func holdingCount(t *testing.T, p *Panel, action string) int {
	t.Helper()
	var n int
	if err := p.db.GetDB().QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action = ?`, action).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func mailItemName(domainID int) string { return fmt.Sprintf("domain-%d-0123456789abcdef", domainID) }

// A finished domain deletion puts its stored mail and its backups in the
// list for 30 days; the answer says until when, with no folder for anybody
// but the administrator; a deletion pressed again keeps the first date.
func TestHoldingDomainDeletionKeepsMailAndBackups30Days(t *testing.T) {
	now := time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC)
	fakeHolding(t, now)
	p, f, agent := newDatabaseDeletionPanel(t, "held.example")
	recordDatabaseDrops(t)
	if _, err := p.db.GetDB().Exec(`INSERT INTO email_accounts (domain_id, address, password_hash, quota_mb) VALUES (?, 'info@held.example', 'x', 100)`, f.domainID); err != nil {
		t.Fatal(err)
	}
	agent.mailQuarantine = "/var/mail/vhosts/.celikpanel-quarantine/" + mailItemName(f.domainID)
	agent.keptBackups = &transport.KeptBackups{Path: "/var/backups/celikpanel/subscriptions/1/domains/1", Files: 3, Bytes: 4096}

	recorder := deleteDomainForSagaTest(t, p, f.domainID)
	if recorder.Code != http.StatusOK {
		t.Fatalf("delete = %d %s", recorder.Code, recorder.Body.String())
	}
	answer := decodeDeletionAnswer(t, recorder)
	until := now.Add(holdingKeep).Format(time.RFC3339)
	if answer.Kept.Mail == nil || answer.Kept.Mail.RemoveAfter != until || answer.Kept.Mail.Path != "" ||
		answer.Kept.Backups == nil || answer.Kept.Backups.RemoveAfter != until || answer.Kept.Backups.Files != 3 {
		t.Fatalf("answer = %s", recorder.Body.String())
	}
	records, err := p.readHoldingRecords(context.Background())
	if err != nil || len(records) != 2 {
		t.Fatalf("records = %+v, %v", records, err)
	}
	for _, rec := range records {
		if rec.Domain != "held.example" || rec.DomainID != f.domainID || rec.RemoveAfter != until || rec.Adopted {
			t.Fatalf("record = %+v", rec)
		}
	}
	// Held again a day later (a retried deletion): the first date stays.
	holdingNow = func() time.Time { return now.Add(24 * time.Hour) }
	again, err := p.holdItem(context.Background(), holdingRecord{ID: holdingMailID(mailItemName(f.domainID)), Kind: transport.HeldItemMail, Name: mailItemName(f.domainID), DomainID: f.domainID})
	if err != nil || again.RemoveAfter != until {
		t.Fatalf("held again = %+v, %v", again, err)
	}
}

func seedHoldingRecord(t *testing.T, p *Panel, rec holdingRecord, removeAfter time.Time) {
	t.Helper()
	rec.RemoveAfter = removeAfter.UTC().Format(time.RFC3339)
	rec.DeletedAt = removeAfter.Add(-holdingKeep).UTC().Format(time.RFC3339)
	if _, err := p.holdItem(context.Background(), rec); err != nil {
		t.Fatal(err)
	}
}

// The periodic step removes a due item whose domain is gone, writes an audit
// row, keeps an item whose date has not come and one whose domain still
// exists (its deletion has not finished).
func TestHoldingStepRemovesOnlyDueItemsOfDeletedDomains(t *testing.T) {
	now := time.Date(2026, 12, 20, 10, 0, 0, 0, time.UTC)
	fake := fakeHolding(t, now)
	p := newDNSPanelForTest(t)
	liveID, _ := seedDomainDeletionLedger(t, p, "still-here.example", "static")
	due := holdingRecord{ID: holdingMailID(mailItemName(9001)), Kind: transport.HeldItemMail, Name: mailItemName(9001), DomainID: 9001, Domain: "gone.example"}
	notYet := holdingRecord{ID: holdingBackupsID(4, 9002), Kind: transport.HeldItemBackups, SubscriptionID: 4, DomainID: 9002}
	live := holdingRecord{ID: holdingBackupsID(4, liveID), Kind: transport.HeldItemBackups, SubscriptionID: 4, DomainID: liveID}
	seedHoldingRecord(t, p, due, now.Add(-time.Minute))
	seedHoldingRecord(t, p, notYet, now.Add(time.Hour))
	seedHoldingRecord(t, p, live, now.Add(-time.Hour))
	fake.inventory = transport.DeletionHoldingInventoryResponse{Items: []transport.HeldItemInfo{
		{Kind: transport.HeldItemMail, Name: mailItemName(9001), DomainID: 9001, Bytes: 10},
		{Kind: transport.HeldItemBackups, SubscriptionID: 4, DomainID: 9002, Bytes: 20},
		{Kind: transport.HeldItemBackups, SubscriptionID: 4, DomainID: liveID, Bytes: 30},
	}}

	p.runHoldingStep(context.Background())
	if len(fake.removed) != 1 || fake.removed[0].Kind != transport.HeldItemMail || fake.removed[0].Name != mailItemName(9001) ||
		fake.removed[0].DomainID != 0 || fake.removed[0].ExpectedBuildCommit != strings.TrimSpace(buildCommit) {
		t.Fatalf("removed = %+v", fake.removed)
	}
	if _, ok := p.readHoldingRecord(context.Background(), due.ID); ok {
		t.Fatal("the removed item's record stayed")
	}
	for _, id := range []string{notYet.ID, live.ID} {
		if _, ok := p.readHoldingRecord(context.Background(), id); !ok {
			t.Fatalf("%s was dropped", id)
		}
	}
	if holdingCount(t, p, "deletion_hold.remove:"+due.ID) != 1 {
		t.Fatal("no audit row for the removal")
	}
	// Run again: nothing more (repeating is safe).
	p.runHoldingStep(context.Background())
	if len(fake.removed) != 1 {
		t.Fatalf("a second step removed again: %+v", fake.removed)
	}
}

// A removal the Agent refuses is kept as failed with its reason and tried
// again after an hour; no answer is kept as not known. A status read never
// removes anything.
func TestHoldingFailureIsKeptAndTriedAgainLater(t *testing.T) {
	now := time.Date(2026, 12, 20, 10, 0, 0, 0, time.UTC)
	fake := fakeHolding(t, now)
	p := newDNSPanelForTest(t)
	rec := holdingRecord{ID: holdingBackupsID(4, 9002), Kind: transport.HeldItemBackups, SubscriptionID: 4, DomainID: 9002}
	seedHoldingRecord(t, p, rec, now.Add(-time.Minute))
	fake.inventory = transport.DeletionHoldingInventoryResponse{Items: []transport.HeldItemInfo{{Kind: transport.HeldItemBackups, SubscriptionID: 4, DomainID: 9002, Bytes: 20}}}
	fake.answer = func(transport.RemoveHeldItemRequest) (transport.RemoveHeldItemResponse, error) {
		return transport.RemoveHeldItemResponse{Error: "/var/backups/celikpanel/subscriptions/4/domains/9002 is not a folder; it was left as it is"}, nil
	}
	p.runHoldingStep(context.Background())
	got, ok := p.readHoldingRecord(context.Background(), rec.ID)
	if !ok || got.Failure == nil || got.Failure.Kind != holdingFailed || !strings.Contains(got.Failure.Line, "not a folder") ||
		got.Attempts != 1 || got.NextTryAt != now.Add(time.Hour).Format(time.RFC3339) {
		t.Fatalf("record = %+v", got)
	}
	if holdingCount(t, p, "deletion_hold.remove.failed:"+rec.ID) != 1 {
		t.Fatal("no audit row for the failure")
	}
	// The list says it; reading it removes nothing.
	list := httptest.NewRecorder()
	p.handleDeletionHolding(list, holdingRequestAs(t, http.MethodGet, holdingPath, roleAdmin))
	if !strings.Contains(list.Body.String(), `"state":"failed"`) || len(fake.removed) != 1 {
		t.Fatalf("list = %s, removals %d", list.Body.String(), len(fake.removed))
	}
	// Before the next try nothing is sent; after it, no answer is "unknown".
	p.runHoldingStep(context.Background())
	if len(fake.removed) != 1 {
		t.Fatal("tried again before its time")
	}
	holdingNow = func() time.Time { return now.Add(61 * time.Minute) }
	fake.answer = func(transport.RemoveHeldItemRequest) (transport.RemoveHeldItemResponse, error) {
		return transport.RemoveHeldItemResponse{}, errors.New("read: connection reset")
	}
	p.runHoldingStep(context.Background())
	got, _ = p.readHoldingRecord(context.Background(), rec.ID)
	if len(fake.removed) != 2 || got.Failure == nil || got.Failure.Kind != holdingUnknown || got.Attempts != 2 {
		t.Fatalf("second try: removals %d, record %+v", len(fake.removed), got)
	}
}

// What earlier releases left is taken into the list at the step, its 30
// days starting then; a live domain's backups and an existing mailbox's
// folder are not; a record whose item went is dropped.
func TestHoldingAdoptsWhatEarlierReleasesLeft(t *testing.T) {
	now := time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC)
	fake := fakeHolding(t, now)
	p := newDNSPanelForTest(t)
	liveID, _ := seedDomainDeletionLedger(t, p, "live.example", "static")
	var accountID int
	if err := p.db.GetDB().QueryRow(`INSERT INTO email_accounts (domain_id, address, password_hash, quota_mb) VALUES (?, 'a@live.example', 'x', 1) RETURNING id`, liveID).Scan(&accountID); err != nil {
		t.Fatal(err)
	}
	p.recordAudit(auditRecord{Action: "domain.delete:celikbros.com", ResourceType: "domain", ResourceID: 2})
	gone := holdingRecord{ID: holdingMailID(mailItemName(9300)), Kind: transport.HeldItemMail, Name: mailItemName(9300), DomainID: 9300}
	seedHoldingRecord(t, p, gone, now.Add(time.Hour))
	fake.inventory = transport.DeletionHoldingInventoryResponse{Items: []transport.HeldItemInfo{
		{Kind: transport.HeldItemMail, Name: "domain-2-6e7ceffb7e3cb3a1", DomainID: 2, Bytes: 12288, Path: "/var/mail/vhosts/.celikpanel-quarantine/domain-2-6e7ceffb7e3cb3a1"},
		{Kind: transport.HeldItemMail, Name: fmt.Sprintf("mailbox-%d-0123456789abcdef", accountID), AccountID: accountID, Bytes: 5},
		{Kind: transport.HeldItemBackups, SubscriptionID: 1, DomainID: liveID, Bytes: 100, Files: 1},
		{Kind: transport.HeldItemBackups, SubscriptionID: 2, DomainID: 2, Bytes: 200, Files: 2},
	}}
	if err := p.adoptHeldItems(context.Background()); err != nil {
		t.Fatal(err)
	}
	records, _ := p.readHoldingRecords(context.Background())
	ids := []string{}
	for _, rec := range records {
		ids = append(ids, rec.ID)
		if !rec.Adopted || rec.RemoveAfter != now.Add(holdingKeep).Format(time.RFC3339) || rec.Domain != "celikbros.com" {
			t.Fatalf("adopted = %+v", rec)
		}
	}
	if strings.Join(ids, ",") != "backups.s2-d2,mail.domain-2-6e7ceffb7e3cb3a1" {
		t.Fatalf("adopted ids = %v", ids)
	}
	if len(fake.removed) != 0 {
		t.Fatal("adoption removed something")
	}
	if holdingCount(t, p, "deletion_hold.gone:"+gone.ID) != 1 {
		t.Fatal("the record of an item no longer there was not dropped")
	}
	// A place that could not be read keeps its records.
	seedHoldingRecord(t, p, gone, now.Add(time.Hour))
	fake.inventory.MailError = "permission denied"
	if err := p.adoptHeldItems(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, ok := p.readHoldingRecord(context.Background(), gone.ID); !ok {
		t.Fatal("an unreadable place dropped its records")
	}
}

func holdingRequestAs(t *testing.T, method, path, role string) *http.Request {
	t.Helper()
	request := httptest.NewRequest(method, path, nil)
	caller := &Caller{ID: 1, Role: role}
	if role == "team" {
		caller = &Caller{ID: 3, Role: core.EffectiveRoleAdditionalUser, AccountType: core.AccountTypeAdditionalUser, CustomerID: 2}
	}
	return request.WithContext(context.WithValue(request.Context(), callerKey, caller))
}

// Only the administrator reads the list or deletes from it; "Delete now"
// removes the item at once and says a refusal with its reason.
func TestHoldingListAndDeleteNowAreTheAdministratorsOnly(t *testing.T) {
	now := time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC)
	fake := fakeHolding(t, now)
	p := newDNSPanelForTest(t)
	rec := holdingRecord{ID: holdingMailID(mailItemName(9400)), Kind: transport.HeldItemMail, Name: mailItemName(9400), DomainID: 9400, Domain: "old.example", Bytes: 2048, Path: "/var/mail/vhosts/.celikpanel-quarantine/x"}
	seedHoldingRecord(t, p, rec, now.Add(10*24*time.Hour))
	for _, role := range []string{roleReseller, roleCustomer, "team"} {
		for _, call := range []struct {
			method, path string
			handler      func(http.ResponseWriter, *http.Request)
		}{
			{http.MethodGet, holdingPath, p.handleDeletionHolding},
			{http.MethodDelete, holdingPath + "/" + rec.ID, p.handleDeletionHoldingItem},
			{http.MethodGet, holdingPath + "/backups.s1-d1/download?name=x", p.handleDeletionHoldingItem},
		} {
			recorder := httptest.NewRecorder()
			call.handler(recorder, holdingRequestAs(t, call.method, call.path, role))
			if recorder.Code != http.StatusForbidden {
				t.Fatalf("%s %s %s = %d", role, call.method, call.path, recorder.Code)
			}
		}
	}
	if len(fake.removed) != 0 {
		t.Fatal("a refused caller removed something")
	}
	list := httptest.NewRecorder()
	p.handleDeletionHolding(list, holdingRequestAs(t, http.MethodGet, holdingPath, roleAdmin))
	var body struct {
		Items   []holdingItemView `json:"items"`
		Summary holdingSummary    `json:"summary"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &body); err != nil || len(body.Items) != 1 || body.Summary.Count != 1 ||
		body.Summary.Bytes != 2048 || body.Items[0].State != "kept" || body.Items[0].Path == "" {
		t.Fatalf("list = %s", list.Body.String())
	}
	// A refusal is said, the item stays.
	fake.answer = func(transport.RemoveHeldItemRequest) (transport.RemoveHeldItemResponse, error) {
		return transport.RemoveHeldItemResponse{Error: "remove x: permission denied"}, nil
	}
	refused := httptest.NewRecorder()
	p.handleDeletionHoldingItem(refused, holdingRequestAs(t, http.MethodDelete, holdingPath+"/"+rec.ID, roleAdmin))
	if refused.Code != http.StatusConflict || !strings.Contains(refused.Body.String(), "HOLDING_REMOVE_FAILED") {
		t.Fatalf("refused = %d %s", refused.Code, refused.Body.String())
	}
	fake.answer = func(transport.RemoveHeldItemRequest) (transport.RemoveHeldItemResponse, error) {
		return transport.RemoveHeldItemResponse{Removed: true}, nil
	}
	done := httptest.NewRecorder()
	p.handleDeletionHoldingItem(done, holdingRequestAs(t, http.MethodDelete, holdingPath+"/"+rec.ID, roleAdmin))
	if done.Code != http.StatusOK {
		t.Fatalf("delete now = %d %s", done.Code, done.Body.String())
	}
	if _, ok := p.readHoldingRecord(context.Background(), rec.ID); ok {
		t.Fatal("the record stayed after Delete now")
	}
	// Pressed again: already gone, nothing sent.
	again := httptest.NewRecorder()
	p.handleDeletionHoldingItem(again, holdingRequestAs(t, http.MethodDelete, holdingPath+"/"+rec.ID, roleAdmin))
	if again.Code != http.StatusOK || len(fake.removed) != 2 {
		t.Fatalf("again = %d, removals %d", again.Code, len(fake.removed))
	}
}

// The home page's line and the disk warning name the list.
func TestHoldingIsOnTheHomePageAndInTheDiskWarning(t *testing.T) {
	now := time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC)
	fakeHolding(t, now)
	p := newDNSPanelForTest(t)
	seedHoldingRecord(t, p, holdingRecord{ID: holdingBackupsID(2, 2), Kind: transport.HeldItemBackups, SubscriptionID: 2, DomainID: 2, Bytes: 5 << 20}, now.Add(48*time.Hour))
	if got := p.holdingForDashboard(context.Background()); got == nil || got.Count != 1 || got.Bytes != 5<<20 {
		t.Fatalf("dashboard = %+v", got)
	}
	saved := monitoringReadDisk
	monitoringReadDisk = func(string) (uint64, uint64) { return 90, 100 }
	t.Cleanup(func() { monitoringReadDisk = saved })
	report := p.collectMonitoringProblems(context.Background(), now)
	for _, problem := range report.Problems {
		if problem.Kind == "disk_full" {
			if problem.HoldingCount != 1 || problem.HoldingBytes != 5<<20 || problem.Action.To != "/maintenance#deletion-holding" {
				t.Fatalf("disk problem = %+v", problem)
			}
			return
		}
	}
	t.Fatal("no disk problem at 90%")
}
