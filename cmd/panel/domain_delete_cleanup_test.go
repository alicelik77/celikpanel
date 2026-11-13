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

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.134: a domain deletion keeps the domain's databases (unbound, still
// in the panel) unless the person chose to delete them; deleting them goes
// through the one database deletion of the Databases page
// (deleteDatabaseRecord): the same users' fates, the same audit rows, the
// same partial result, and pressing the deletion again continues. The Agent
// is faked at the same seams the database deletion's own tests use
// (databaseDeleteAgent, databaseAccessNative).

type deletionDatabaseFixture struct {
	domainID       int
	subscriptionID int
	keepOne        int
	keepTwo        int
	elsewhere      int
	ownUser        int
	sharedUser     int
}

// seedDeletionDatabases gives the domain two MariaDB databases on this
// machine's engine: shop_db (its own user shop_user) and blog_db (blog_user,
// who also has rights on elsewhere_db, a database bound to no domain).
func seedDeletionDatabases(t *testing.T, p *Panel, domainID, subscriptionID int) deletionDatabaseFixture {
	t.Helper()
	db := p.db.GetDB()
	var mariadb int
	if err := db.QueryRow(`SELECT id FROM database_server_types WHERE name = 'mariadb'`).Scan(&mariadb); err != nil {
		t.Fatal(err)
	}
	exec := func(query string, args ...any) int {
		t.Helper()
		result, err := db.Exec(query, args...)
		if err != nil {
			t.Fatalf("%s: %v", query, err)
		}
		id, _ := result.LastInsertId()
		return int(id)
	}
	server := exec(`INSERT INTO database_servers (subscription_id, type_id, name, version, host, port, status)
		VALUES (?, ?, 'local', '10.11', 'localhost', 3306, 'active')`, subscriptionID, mariadb)
	f := deletionDatabaseFixture{domainID: domainID, subscriptionID: subscriptionID}
	f.keepOne = exec(`INSERT INTO databases_v2 (server_id, subscription_id, domain_id, name) VALUES (?, ?, ?, 'shop_db')`, server, subscriptionID, domainID)
	f.keepTwo = exec(`INSERT INTO databases_v2 (server_id, subscription_id, domain_id, name) VALUES (?, ?, ?, 'blog_db')`, server, subscriptionID, domainID)
	f.elsewhere = exec(`INSERT INTO databases_v2 (server_id, subscription_id, domain_id, name) VALUES (?, ?, NULL, 'elsewhere_db')`, server, subscriptionID)
	f.ownUser = exec(`INSERT INTO database_users (server_id, subscription_id, username, password) VALUES (?, ?, 'shop_user', 'sealed')`, server, subscriptionID)
	f.sharedUser = exec(`INSERT INTO database_users (server_id, subscription_id, username, password) VALUES (?, ?, 'blog_user', 'sealed')`, server, subscriptionID)
	exec(`INSERT INTO database_user_grants (database_id, user_id) VALUES (?, ?), (?, ?), (?, ?)`,
		f.keepOne, f.ownUser, f.keepTwo, f.sharedUser, f.elsewhere, f.sharedUser)
	return f
}

func deleteDomainWithQuery(t *testing.T, p *Panel, domainID int, query string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/domains/%d%s", domainID, query), nil)
	recorder := httptest.NewRecorder()
	p.handleDeleteDomain(recorder, request)
	return recorder
}

type deletionAnswer struct {
	Status    string             `json:"status"`
	Stage     string             `json:"stage"`
	Reason    string             `json:"reason"`
	ErrorLine string             `json:"error_line"`
	Code      string             `json:"code"`
	Kept      domainDeletionKept `json:"kept"`
}

func decodeDeletionAnswer(t *testing.T, recorder *httptest.ResponseRecorder) deletionAnswer {
	t.Helper()
	var answer deletionAnswer
	if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil {
		t.Fatalf("decode %q: %v", recorder.Body.String(), err)
	}
	return answer
}

func databaseBinding(t *testing.T, p *Panel, id int) (exists bool, domainID *int) {
	t.Helper()
	err := p.db.GetDB().QueryRow(`SELECT domain_id FROM databases_v2 WHERE id = ?`, id).Scan(&domainID)
	if err != nil {
		return false, nil
	}
	return true, domainID
}

func deletionCountRows(t *testing.T, p *Panel, query string, args ...any) int {
	t.Helper()
	var n int
	if err := p.db.GetDB().QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func newDatabaseDeletionPanel(t *testing.T, domain string) (*Panel, deletionDatabaseFixture, *domainDeletionRPCAgent) {
	t.Helper()
	p := newDNSPanelForTest(t)
	domainID, subscriptionID := seedDomainDeletionLedger(t, p, domain, "static")
	fixture := seedDeletionDatabases(t, p, domainID, subscriptionID)
	withPanelBuildCommit(t, "domain-delete-test")
	agent := &domainDeletionRPCAgent{commit: "domain-delete-test"}
	attachDomainDeletionRPCAgent(t, p, agent)
	return p, fixture, agent
}

// deletionDatabaseFakes fakes the Agent's database drop and user removal at
// the database deletion's own seams.
func deletionDatabaseFakes(t *testing.T, p *Panel) (*recordedDatabaseDrops, *fakeDatabaseAccessNative) {
	t.Helper()
	drops := recordDatabaseDrops(t)
	native := &fakeDatabaseAccessNative{}
	p.databaseAccessNative = native
	return drops, native
}

func droppedNames(drops *recordedDatabaseDrops) string {
	names := []string{}
	for _, request := range drops.requests {
		names = append(names, request.Name)
	}
	return strings.Join(names, ",")
}

func TestDomainDeletionKeepsDatabasesUnboundByDefault(t *testing.T) {
	p, f, _ := newDatabaseDeletionPanel(t, "keep-databases.example")
	drops, native := deletionDatabaseFakes(t, p)

	recorder := deleteDomainForSagaTest(t, p, f.domainID)
	if recorder.Code != http.StatusOK {
		t.Fatalf("delete = %d %s", recorder.Code, recorder.Body.String())
	}
	answer := decodeDeletionAnswer(t, recorder)
	if answer.Status != "deleted" || answer.Kept.Choice != domainDeletionDatabasesKeep ||
		len(answer.Kept.Databases) != 2 || len(answer.Kept.Deleted) != 0 {
		t.Fatalf("answer = %s", recorder.Body.String())
	}
	if len(drops.requests) != 0 || len(native.removed) != 0 {
		t.Fatalf("a kept database was touched: drops %v users %q", drops.requests, removedUsernames(native))
	}
	for _, id := range []int{f.keepOne, f.keepTwo} {
		exists, bound := databaseBinding(t, p, id)
		if !exists || bound != nil {
			t.Fatalf("database %d: exists=%v bound=%v, want kept and unbound", id, exists, bound)
		}
	}
	if n := deletionCountRows(t, p, `SELECT COUNT(*) FROM database_users WHERE id IN (?, ?)`, f.ownUser, f.sharedUser); n != 2 {
		t.Fatalf("users kept = %d", n)
	}
	if n := deletionCountRows(t, p, `SELECT COUNT(*) FROM database_user_grants WHERE database_id IN (?, ?)`, f.keepOne, f.keepTwo); n != 2 {
		t.Fatalf("grants kept = %d", n)
	}
	if n := deletionCountRows(t, p, `SELECT COUNT(*) FROM panel_settings WHERE key = ?`, domainDeletionPlanKey(f.domainID)); n != 0 {
		t.Fatalf("plan record left after the deletion: %d", n)
	}
}

func TestDomainDeletionDeletesChosenDatabasesThroughTheOneDatabaseDeletion(t *testing.T) {
	p, f, _ := newDatabaseDeletionPanel(t, "delete-databases.example")
	drops, native := deletionDatabaseFakes(t, p)

	recorder := deleteDomainWithQuery(t, p, f.domainID, "?databases=delete")
	if recorder.Code != http.StatusOK {
		t.Fatalf("delete = %d %s", recorder.Code, recorder.Body.String())
	}
	answer := decodeDeletionAnswer(t, recorder)
	if answer.Kept.Choice != domainDeletionDatabasesDelete || len(answer.Kept.Deleted) != 2 || len(answer.Kept.Databases) != 0 {
		t.Fatalf("answer = %s", recorder.Body.String())
	}
	// The databases are dropped without a user (the one deletion never names
	// a user to the drop); then each user: shop_user goes, blog_user stays
	// (elsewhere_db uses it) and only loses its rights on blog_db.
	if got := droppedNames(drops); got != "blog_db,shop_db" {
		t.Fatalf("drops = %s", got)
	}
	for _, request := range drops.requests {
		if request.Type != "mysql" || request.User != "" {
			t.Fatalf("drop request = %+v", request)
		}
	}
	if removed := removedUsernames(native); removed != "blog_user,shop_user" {
		t.Fatalf("user removals = %q", removed)
	}
	for _, request := range native.removed {
		if !request.DatabaseDropped || request.RightsOnly != (request.Username == "blog_user") {
			t.Fatalf("user removal = %+v", request)
		}
	}
	// The answer carries the database deletion's own words per database.
	byName := map[string]domainDeletionDeletedDatabase{}
	for _, item := range answer.Kept.Deleted {
		byName[item.Name] = item
	}
	if strings.Join(byName["shop_db"].UsersRemoved, ",") != "shop_user" || strings.Join(byName["blog_db"].UsersKept, ",") != "blog_user" {
		t.Fatalf("deleted = %+v", answer.Kept.Deleted)
	}
	for _, id := range []int{f.keepOne, f.keepTwo} {
		if exists, _ := databaseBinding(t, p, id); exists {
			t.Fatalf("database %d still recorded", id)
		}
	}
	if exists, bound := databaseBinding(t, p, f.elsewhere); !exists || bound != nil {
		t.Fatalf("an unrelated database changed: exists=%v bound=%v", exists, bound)
	}
	if n := deletionCountRows(t, p, `SELECT COUNT(*) FROM database_users WHERE id = ?`, f.ownUser); n != 0 {
		t.Fatalf("own user record kept")
	}
	if n := deletionCountRows(t, p, `SELECT COUNT(*) FROM database_user_grants WHERE user_id = ? AND database_id = ?`, f.sharedUser, f.elsewhere); n != 1 {
		t.Fatalf("shared user's other grant lost")
	}
	// The same audit rows as the Databases page writes.
	for _, action := range []string{"database.delete:shop_db", "database.delete:blog_db"} {
		if n := deletionCountRows(t, p, `SELECT COUNT(*) FROM audit_logs WHERE action = ?`, action); n != 1 {
			t.Fatalf("audit %s = %d", action, n)
		}
	}
}

// A database whose users are not all done stops the domain deletion at its
// database step with the database deletion's own partial answer; the
// database stays listed and bound; pressing the deletion again continues.
func TestDomainDeletionDatabasePartialResultStopsAndTheRetryContinues(t *testing.T) {
	p, f, agent := newDatabaseDeletionPanel(t, "resume-databases.example")
	drops, native := deletionDatabaseFakes(t, p)
	native.failRemove = true

	first := deleteDomainWithQuery(t, p, f.domainID, "?databases=delete")
	requireDeletionPendingStage(t, first, domainDeletionStageDatabases)
	answer := decodeDeletionAnswer(t, first)
	if answer.Reason != domainDeletionReasonDatabases || !strings.Contains(answer.ErrorLine, "blog_db") {
		t.Fatalf("pending answer = %s", first.Body.String())
	}
	failure := answer.Kept.DatabaseFailure
	var body apiErrorBody
	if failure == nil || failure.Name != "blog_db" || failure.Status != http.StatusConflict || json.Unmarshal(failure.Answer, &body) != nil ||
		body.Code != errCodeDatabaseDeleteUsersLeft || body.Reason != "rights" || body.Vars["name"] != "blog_db" || !body.PartialSuccess {
		t.Fatalf("database failure = %s", first.Body.String())
	}
	if n := deletionCountRows(t, p, `SELECT COUNT(*) FROM audit_logs WHERE action = 'database.delete.partial:blog_db'`); n != 1 {
		t.Fatalf("partial audit rows = %d", n)
	}
	// blog_db (first by name) stays listed and bound until it is finished;
	// shop_db was not reached.
	for _, id := range []int{f.keepOne, f.keepTwo} {
		if exists, bound := databaseBinding(t, p, id); !exists || bound == nil || *bound != f.domainID {
			t.Fatalf("database %d: exists=%v bound=%v", id, exists, bound)
		}
	}
	if got := droppedNames(drops); got != "blog_db" {
		t.Fatalf("drops = %s", got)
	}
	// The site step ran before; the DNS step did not.
	agent.callsMu.Lock()
	sites, syncs := agent.deleteSiteCalls, len(agent.syncRequests)
	agent.callsMu.Unlock()
	if sites != 1 || syncs != 0 {
		t.Fatalf("site calls=%d dns calls=%d", sites, syncs)
	}
	// A status read names the failure and the plan (D-024) and changes nothing.
	status := httptest.NewRecorder()
	p.handleDomainDeletionStatus(status, httptest.NewRequest(http.MethodGet, "/", nil), f.domainID)
	var read domainDeletionStatusResponse
	if err := json.Unmarshal(status.Body.Bytes(), &read); err != nil {
		t.Fatal(err)
	}
	if read.Status != domainDeletionStatusFailed || read.Stage != domainDeletionStageDatabases ||
		read.Reason != domainDeletionReasonDatabases || read.Databases != domainDeletionDatabasesDelete ||
		!strings.Contains(read.Message, "Retry this same deletion") {
		t.Fatalf("status = %+v", read)
	}
	// Retrying with the other choice is refused before anything changes.
	refused := deleteDomainWithQuery(t, p, f.domainID, "?databases=keep")
	if refused.Code != http.StatusConflict || decodeDeletionAnswer(t, refused).Code != errCodeDomainDeletionPlanFixed {
		t.Fatalf("conflicting retry = %d %s", refused.Code, refused.Body.String())
	}
	if exists, bound := databaseBinding(t, p, f.keepOne); !exists || bound == nil {
		t.Fatal("the refused retry changed shop_db")
	}
	// The plain retry continues the accepted plan from blog_db.
	native.failRemove = false
	second := deleteDomainForSagaTest(t, p, f.domainID)
	if second.Code != http.StatusOK {
		t.Fatalf("retry = %d %s", second.Code, second.Body.String())
	}
	if got := droppedNames(drops); got != "blog_db,blog_db,shop_db" {
		t.Fatalf("drops after the retry = %s", got)
	}
	for _, id := range []int{f.keepOne, f.keepTwo} {
		if exists, _ := databaseBinding(t, p, id); exists {
			t.Fatalf("database %d kept after the retry", id)
		}
	}
	if done := decodeDeletionAnswer(t, second); len(done.Kept.Deleted) != 2 || done.Kept.DatabaseFailure != nil {
		t.Fatalf("final answer = %s", second.Body.String())
	}
	if n := deletionCountRows(t, p, `SELECT COUNT(*) FROM panel_settings WHERE key IN (?, ?)`,
		domainDeletionPlanKey(f.domainID), domainDeletionFailureKey(f.domainID)); n != 0 {
		t.Fatalf("records left after the deletion: %d", n)
	}
}

// No answer to the drop: whether the database went is not known. The
// database deletion's own unknown answer is carried, the domain deletion
// keeps no failure (its status reads as not known), and nothing else ran.
func TestDomainDeletionDatabaseDropWithoutAnAnswerIsUnknown(t *testing.T) {
	p, f, agent := newDatabaseDeletionPanel(t, "unknown-databases.example")
	drops, native := deletionDatabaseFakes(t, p)
	drops.answer = func(*transport.DeleteDatabaseResponse) error { return errors.New("read: connection reset mid-call") }

	first := deleteDomainWithQuery(t, p, f.domainID, "?databases=delete")
	requireDeletionPendingStage(t, first, domainDeletionStageDatabases)
	answer := decodeDeletionAnswer(t, first)
	var body apiErrorBody
	if answer.Reason != "" || answer.Kept.DatabaseFailure == nil || answer.Kept.DatabaseFailure.Status != http.StatusBadGateway ||
		json.Unmarshal(answer.Kept.DatabaseFailure.Answer, &body) != nil || body.Code != errCodeDatabaseDeleteUnconfirmed {
		t.Fatalf("answer = %s", first.Body.String())
	}
	if len(native.removed) != 0 || deletionCountRows(t, p, `SELECT COUNT(*) FROM panel_settings WHERE key = ?`, domainDeletionFailureKey(f.domainID)) != 0 {
		t.Fatal("an unknown drop went on, or was kept as a verified failure")
	}
	agent.callsMu.Lock()
	syncs := len(agent.syncRequests)
	agent.callsMu.Unlock()
	if syncs != 0 {
		t.Fatalf("the DNS step ran after an unknown database step")
	}
}

func TestDomainDeletionRefusesAnUnknownDatabaseChoice(t *testing.T) {
	p, f, _ := newDatabaseDeletionPanel(t, "bad-choice.example")
	drops := recordDatabaseDrops(t)
	for _, query := range []string{"?databases=all", "?databases=keep&databases=delete"} {
		recorder := deleteDomainWithQuery(t, p, f.domainID, query)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("%s = %d", query, recorder.Code)
		}
	}
	if n := deletionCountRows(t, p, `SELECT COUNT(*) FROM domain_deletion_operations WHERE domain_id = ?`, f.domainID); n != 0 || len(drops.requests) != 0 {
		t.Fatal("a refused choice started the deletion")
	}
}

// A deletion started by an older release (no plan) and finished by
// CelikPanel by itself never cascades a database record away.
func TestDomainDeletionFinalizeUnbindsDatabasesWithoutAPlan(t *testing.T) {
	p, f, _ := newDatabaseDeletionPanel(t, "older-deletion.example")
	if _, err := p.markDomainDeletionPending(context.Background(), f.domainID); err != nil {
		t.Fatal(err)
	}
	if err := p.finalizeDomainDeletion(context.Background(), f.domainID); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int{f.keepOne, f.keepTwo} {
		if exists, bound := databaseBinding(t, p, id); !exists || bound != nil {
			t.Fatalf("database %d: exists=%v bound=%v", id, exists, bound)
		}
	}
	if n := deletionCountRows(t, p, `SELECT COUNT(*) FROM database_user_grants WHERE database_id IN (?, ?)`, f.keepOne, f.keepTwo); n != 2 {
		t.Fatalf("grants after finalize = %d", n)
	}
}

// A first attempt that is put back (its mail certificate step failed before
// anything else ran) leaves no plan behind.
func TestDomainDeletionRestoreRemovesThePlan(t *testing.T) {
	p, f, _ := newDatabaseDeletionPanel(t, "restored.example")
	ctx := context.Background()
	if _, err := p.markDomainDeletionPending(ctx, f.domainID); err != nil {
		t.Fatal(err)
	}
	if _, err := p.acceptDomainDeletionPlan(ctx, f.domainID, "restored.example", domainDeletionDatabasesDelete); err != nil {
		t.Fatal(err)
	}
	if err := p.restoreDomainDeletionStart(ctx, f.domainID); err != nil {
		t.Fatal(err)
	}
	if n := deletionCountRows(t, p, `SELECT COUNT(*) FROM panel_settings WHERE key = ?`, domainDeletionPlanKey(f.domainID)); n != 0 {
		t.Fatalf("plan kept after restoration")
	}
}

// The server's sender-limit lists and the per-domain mail options map leave
// a domain as soon as its deletion is marked.
func TestMailListsSkipADomainBeingDeleted(t *testing.T) {
	p, f, _ := newDatabaseDeletionPanel(t, "lists.example")
	db := p.db.GetDB()
	if _, err := db.Exec(`INSERT INTO email_accounts (domain_id, address, password_hash, quota_mb) VALUES (?, 'info@lists.example', 'x', 100)`, f.domainID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO domain_mail_options (domain_id, incoming_off) VALUES (?, 1)`, f.domainID); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	mailboxes, err := loadOutgoingMailboxes(ctx, db, 0)
	if err != nil || len(mailboxes) != 1 {
		t.Fatalf("before: %d mailboxes, %v", len(mailboxes), err)
	}
	entries, _, err := domainOptionsEntries(ctx, db)
	if err != nil || len(entries) == 0 {
		t.Fatalf("before: %d option entries, %v", len(entries), err)
	}
	if _, err := p.markDomainDeletionPending(ctx, f.domainID); err != nil {
		t.Fatal(err)
	}
	mailboxes, err = loadOutgoingMailboxes(ctx, db, 0)
	if err != nil || len(mailboxes) != 0 {
		t.Fatalf("after: %d mailboxes, %v", len(mailboxes), err)
	}
	entries, _, err = domainOptionsEntries(ctx, db)
	if err != nil || len(entries) != 0 {
		t.Fatalf("after: %d option entries, %v", len(entries), err)
	}
	// The domain's own mail page still sees its mailbox.
	if own, err := loadOutgoingMailboxes(ctx, db, f.domainID); err != nil || len(own) != 1 {
		t.Fatalf("own view: %d, %v", len(own), err)
	}
}

// Who may choose to delete the databases: exactly the accounts that may
// delete the domain. Strangers are told the domain does not exist and
// nothing is dropped; the domain's administrator, reseller and customer
// reach the deletion itself.
func TestDomainDeletionDatabaseChoiceRoleMatrix(t *testing.T) {
	f := newAccessUsersFixture(t)
	drops := recordDatabaseDrops(t)
	path := fmt.Sprintf("/api/v1/domains/%d?databases=delete", authzMatrixCustomerDomainID)
	for _, stranger := range []int{9708, 9707, authzMatrixOutsiderID} {
		if got := f.call(t, stranger, http.MethodDelete, path, ""); got.Code != http.StatusNotFound {
			t.Fatalf("stranger %d: %d %s", stranger, got.Code, got.Body.String())
		}
	}
	if got := f.call(t, authzMatrixAdditionalUserID, http.MethodDelete, path, ""); got.Code < 400 {
		t.Fatalf("team member deleted: %d", got.Code)
	}
	for _, allowed := range []int{authzMatrixAdminID, authzMatrixResellerID, authzMatrixCustomerID} {
		got := f.call(t, allowed, http.MethodDelete, path, "")
		if got.Code == http.StatusNotFound || got.Code == http.StatusForbidden || got.Code == http.StatusBadRequest {
			t.Fatalf("allowed %d refused: %d %s", allowed, got.Code, got.Body.String())
		}
	}
	if got := drops.requests; len(got) != 0 {
		t.Fatalf("dropped without a deletion: %v", got)
	}
	if n := deletionCountRows(t, f.panel, `SELECT COUNT(*) FROM databases_v2 WHERE domain_id = ?`, authzMatrixCustomerDomainID); n != 2 {
		t.Fatalf("databases of the domain = %d", n)
	}
}

// A site step the Agent answers as incomplete is a named, kept failure; the
// answer already says what stays; the retry finishes and says it again.
func TestDomainDeletionSiteFailureIsNamedAndTheAnswerSaysWhatStays(t *testing.T) {
	p, f, agent := newDatabaseDeletionPanel(t, "site-failure.example")
	recordDatabaseDrops(t)
	if _, err := p.db.GetDB().Exec(`INSERT INTO email_accounts (domain_id, address, password_hash, quota_mb)
		VALUES (?, 'info@site-failure.example', 'x', 100)`, f.domainID); err != nil {
		t.Fatal(err)
	}
	agent.siteFailuresRemaining, agent.siteFailure = 1, "site cleanup incomplete: scheduled tasks"
	agent.keptBackups = &transport.KeptBackups{Path: "/var/backups/celikpanel/subscriptions/1/domains/1", Files: 2, Bytes: 4096}
	agent.mailQuarantine = fmt.Sprintf("/var/mail/vhosts/.celikpanel-quarantine/domain-%d-0123456789abcdef", f.domainID)

	first := deleteDomainForSagaTest(t, p, f.domainID)
	requireDeletionPendingStage(t, first, domainDeletionStageSite)
	var pendingAnswer struct {
		Reason    string             `json:"reason"`
		ErrorLine string             `json:"error_line"`
		Kept      domainDeletionKept `json:"kept"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &pendingAnswer); err != nil {
		t.Fatal(err)
	}
	if pendingAnswer.Reason != domainDeletionReasonSite || !strings.Contains(pendingAnswer.ErrorLine, "scheduled tasks") ||
		pendingAnswer.Kept.Mail == nil || pendingAnswer.Kept.Mail.RemoveAfter == "" || pendingAnswer.Kept.Mail.Path != "" ||
		pendingAnswer.Kept.Backups == nil || pendingAnswer.Kept.Backups.Files != 2 || len(pendingAnswer.Kept.Databases) != 2 {
		t.Fatalf("pending answer = %s", first.Body.String())
	}
	// The databases are not touched before the site is gone.
	if exists, bound := databaseBinding(t, p, f.keepOne); !exists || bound == nil {
		t.Fatal("databases changed before the site step finished")
	}
	status := httptest.NewRecorder()
	p.handleDomainDeletionStatus(status, httptest.NewRequest(http.MethodGet, "/", nil), f.domainID)
	var read domainDeletionStatusResponse
	if err := json.Unmarshal(status.Body.Bytes(), &read); err != nil {
		t.Fatal(err)
	}
	if read.Status != domainDeletionStatusFailed || read.Reason != domainDeletionReasonSite || !strings.Contains(read.ErrorLine, "scheduled tasks") {
		t.Fatalf("status = %+v", read)
	}

	second := deleteDomainForSagaTest(t, p, f.domainID)
	if second.Code != http.StatusOK {
		t.Fatalf("retry = %d %s", second.Code, second.Body.String())
	}
	var done struct {
		Status string             `json:"status"`
		Kept   domainDeletionKept `json:"kept"`
	}
	if err := json.Unmarshal(second.Body.Bytes(), &done); err != nil {
		t.Fatal(err)
	}
	if done.Status != "deleted" || done.Kept.Mail == nil || done.Kept.Backups == nil || done.Kept.Backups.Bytes != 4096 {
		t.Fatalf("final answer = %s", second.Body.String())
	}
}
