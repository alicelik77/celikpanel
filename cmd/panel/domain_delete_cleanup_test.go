package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.134: a domain deletion keeps the domain's databases (unbound, still
// in the panel) unless the person chose to delete them; deleting them drops
// each database and its own users as a real, resumable step.

type deletionDatabaseFixture struct {
	domainID       int
	subscriptionID int
	keepOne        int
	keepTwo        int
	elsewhere      int
	ownUser        int
	sharedUser     int
}

// seedDeletionDatabases gives the domain two MariaDB databases: shop_db
// (its own user shop_user) and blog_db (blog_user, who also has rights on
// elsewhere_db, a database bound to no domain).
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

type recordedDatabaseDrops struct {
	sync.Mutex
	requests []transport.DeleteDatabaseRequest
	// failName answers Success=false for this database while failures > 0.
	failName string
	failures int
}

func fakeDomainDatabaseDrops(t *testing.T) *recordedDatabaseDrops {
	t.Helper()
	recorded := &recordedDatabaseDrops{}
	saved := domainDeletionDropDatabase
	domainDeletionDropDatabase = func(_ *Panel, _ context.Context, request transport.DeleteDatabaseRequest) (transport.DeleteDatabaseResponse, error) {
		recorded.Lock()
		defer recorded.Unlock()
		recorded.requests = append(recorded.requests, request)
		if request.Name == recorded.failName && recorded.failures > 0 {
			recorded.failures--
			return transport.DeleteDatabaseResponse{Error: "failed to drop database"}, nil
		}
		return transport.DeleteDatabaseResponse{Success: true}, nil
	}
	t.Cleanup(func() { domainDeletionDropDatabase = saved })
	return recorded
}

func (r *recordedDatabaseDrops) dropped() []string {
	r.Lock()
	defer r.Unlock()
	var out []string
	for _, request := range r.requests {
		out = append(out, request.Name+"/"+request.User)
	}
	sort.Strings(out)
	return out
}

func deleteDomainWithQuery(t *testing.T, p *Panel, domainID int, query string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/domains/%d%s", domainID, query), nil)
	recorder := httptest.NewRecorder()
	p.handleDeleteDomain(recorder, request)
	return recorder
}

type deletionAnswer struct {
	Status    string `json:"status"`
	Stage     string `json:"stage"`
	Reason    string `json:"reason"`
	ErrorLine string `json:"error_line"`
	Code      string `json:"code"`
	Kept      struct {
		Databases []domainDeletionPlanDatabase `json:"databases"`
		Deleted   []domainDeletionPlanDatabase `json:"deleted_databases"`
	} `json:"kept"`
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

func TestDomainDeletionKeepsDatabasesUnboundByDefault(t *testing.T) {
	p, f, _ := newDatabaseDeletionPanel(t, "keep-databases.example")
	drops := fakeDomainDatabaseDrops(t)

	recorder := deleteDomainForSagaTest(t, p, f.domainID)
	if recorder.Code != http.StatusOK {
		t.Fatalf("delete = %d %s", recorder.Code, recorder.Body.String())
	}
	answer := decodeDeletionAnswer(t, recorder)
	if answer.Status != "deleted" || len(answer.Kept.Databases) != 2 || len(answer.Kept.Deleted) != 0 {
		t.Fatalf("answer = %+v", answer)
	}
	if got := drops.dropped(); len(got) != 0 {
		t.Fatalf("a kept database was dropped: %v", got)
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

func TestDomainDeletionDeletesChosenDatabasesAndTheirOwnUsers(t *testing.T) {
	p, f, _ := newDatabaseDeletionPanel(t, "delete-databases.example")
	drops := fakeDomainDatabaseDrops(t)

	recorder := deleteDomainWithQuery(t, p, f.domainID, "?databases=delete")
	if recorder.Code != http.StatusOK {
		t.Fatalf("delete = %d %s", recorder.Code, recorder.Body.String())
	}
	answer := decodeDeletionAnswer(t, recorder)
	if len(answer.Kept.Deleted) != 2 || len(answer.Kept.Databases) != 0 {
		t.Fatalf("answer = %+v", answer)
	}
	// shop_db goes with shop_user; blog_db goes, but blog_user keeps its
	// rights on elsewhere_db and is not dropped.
	if got, want := strings.Join(drops.dropped(), ","), "blog_db/,shop_db/shop_user"; got != want {
		t.Fatalf("drops = %s, want %s", got, want)
	}
	for _, request := range drops.requests {
		if request.Type != "mysql" || !request.RequireUserCleanup {
			t.Fatalf("drop request = %+v", request)
		}
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
}

func TestDomainDeletionDatabaseFailureIsNamedAndResumesTheSamePlan(t *testing.T) {
	p, f, agent := newDatabaseDeletionPanel(t, "resume-databases.example")
	drops := fakeDomainDatabaseDrops(t)
	drops.failName, drops.failures = "shop_db", 1

	first := deleteDomainWithQuery(t, p, f.domainID, "?databases=delete")
	requireDeletionPendingStage(t, first, domainDeletionStageDatabases)
	answer := decodeDeletionAnswer(t, first)
	if answer.Reason != domainDeletionReasonDatabases || !strings.Contains(answer.ErrorLine, "shop_db") {
		t.Fatalf("pending answer = %+v", answer)
	}
	// blog_db (first by name) is gone; shop_db is still there, bound.
	if exists, _ := databaseBinding(t, p, f.keepTwo); exists {
		t.Fatal("blog_db kept after it was dropped")
	}
	if exists, bound := databaseBinding(t, p, f.keepOne); !exists || bound == nil || *bound != f.domainID {
		t.Fatalf("shop_db: exists=%v bound=%v", exists, bound)
	}
	// The site step ran before; the DNS step did not.
	agent.callsMu.Lock()
	sites, syncs := agent.deleteSiteCalls, len(agent.syncRequests)
	agent.callsMu.Unlock()
	if sites != 1 || syncs != 0 {
		t.Fatalf("site calls=%d dns calls=%d", sites, syncs)
	}
	// A status read names the failure (D-024) and changes nothing.
	status := httptest.NewRecorder()
	p.handleDomainDeletionStatus(status, httptest.NewRequest(http.MethodGet, "/", nil), f.domainID)
	var read domainDeletionStatusResponse
	if err := json.Unmarshal(status.Body.Bytes(), &read); err != nil {
		t.Fatal(err)
	}
	if read.Status != domainDeletionStatusFailed || read.Stage != domainDeletionStageDatabases ||
		read.Reason != domainDeletionReasonDatabases || !strings.Contains(read.Message, "database engine") {
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
	// The plain retry continues the accepted plan.
	second := deleteDomainForSagaTest(t, p, f.domainID)
	if second.Code != http.StatusOK {
		t.Fatalf("retry = %d %s", second.Code, second.Body.String())
	}
	if exists, _ := databaseBinding(t, p, f.keepOne); exists {
		t.Fatal("shop_db kept after the retry")
	}
	if n := deletionCountRows(t, p, `SELECT COUNT(*) FROM panel_settings WHERE key IN (?, ?)`,
		domainDeletionPlanKey(f.domainID), domainDeletionFailureKey(f.domainID)); n != 0 {
		t.Fatalf("records left after the deletion: %d", n)
	}
}

func TestDomainDeletionRefusesAnUnknownDatabaseChoice(t *testing.T) {
	p, f, _ := newDatabaseDeletionPanel(t, "bad-choice.example")
	drops := fakeDomainDatabaseDrops(t)
	for _, query := range []string{"?databases=all", "?databases=keep&databases=delete"} {
		recorder := deleteDomainWithQuery(t, p, f.domainID, query)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("%s = %d", query, recorder.Code)
		}
	}
	if n := deletionCountRows(t, p, `SELECT COUNT(*) FROM domain_deletion_operations WHERE domain_id = ?`, f.domainID); n != 0 || len(drops.dropped()) != 0 {
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
	drops := fakeDomainDatabaseDrops(t)
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
	if got := drops.dropped(); len(got) != 0 {
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
	fakeDomainDatabaseDrops(t)
	if _, err := p.db.GetDB().Exec(`INSERT INTO email_accounts (domain_id, address, password_hash, quota_mb)
		VALUES (?, 'info@site-failure.example', 'x', 100)`, f.domainID); err != nil {
		t.Fatal(err)
	}
	agent.siteFailuresRemaining, agent.siteFailure = 1, "site cleanup incomplete: scheduled tasks"
	agent.keptBackups = &transport.KeptBackups{Path: "/var/backups/celikpanel/subscriptions/1/domains/1", Files: 2, Bytes: 4096}
	agent.mailQuarantine = "/var/mail/vhosts/.celikpanel-quarantine/domain-1-abc"

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
		pendingAnswer.Kept.Mail == nil || pendingAnswer.Kept.Mail.Path != agent.mailQuarantine ||
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
