package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/alicelik/celikpanel/internal/appcatalog"
	"github.com/alicelik/celikpanel/internal/transport"
)

// A fake agent for the catalog: every privileged step is recorded.
type catalogAgent struct {
	wordpressInstallAgent

	cmu         sync.Mutex
	prereq      transport.AppPrerequisitesResponse
	files       transport.InstallCatalogAppResponse
	setup       transport.SetupCatalogAppResponse
	remove      transport.RemoveCatalogAppResponse
	prereqCalls int
	filesCalls  []transport.InstallCatalogAppRequest
	setupCalls  []transport.SetupCatalogAppRequest
	removeCalls []transport.RemoveCatalogAppRequest
}

func (a *catalogAgent) AppPrerequisitesV1(_ *transport.AppPrerequisitesRequest, resp *transport.AppPrerequisitesResponse) error {
	a.cmu.Lock()
	defer a.cmu.Unlock()
	a.prereqCalls++
	*resp = a.prereq
	return nil
}

func (a *catalogAgent) InstallCatalogAppV1(req *transport.InstallCatalogAppRequest, resp *transport.InstallCatalogAppResponse) error {
	a.cmu.Lock()
	defer a.cmu.Unlock()
	a.filesCalls = append(a.filesCalls, *req)
	*resp = a.files
	return nil
}

func (a *catalogAgent) SetupCatalogAppV1(req *transport.SetupCatalogAppRequest, resp *transport.SetupCatalogAppResponse) error {
	a.cmu.Lock()
	defer a.cmu.Unlock()
	a.setupCalls = append(a.setupCalls, *req)
	*resp = a.setup
	return nil
}

func (a *catalogAgent) RemoveCatalogAppV1(req *transport.RemoveCatalogAppRequest, resp *transport.RemoveCatalogAppResponse) error {
	a.cmu.Lock()
	defer a.cmu.Unlock()
	a.removeCalls = append(a.removeCalls, *req)
	*resp = a.remove
	return nil
}

func newCatalogAgent() *catalogAgent {
	agent := &catalogAgent{
		prereq: transport.AppPrerequisitesResponse{Family: "apt", PHPLine: "8.3", PHPOK: true, DocRoot: transport.AppDocRootPlaceholder},
		files:  transport.InstallCatalogAppResponse{Installed: true, Detail: "files in place"},
		setup:  transport.SetupCatalogAppResponse{Configured: true, AdminPath: "/user/login", Detail: "set up"},
		remove: transport.RemoveCatalogAppResponse{FilesRemoved: true, RulesRemoved: true},
	}
	agent.createResponse = transport.CreateDatabaseResponse{Success: true, OwnedByOperation: true}
	agent.deleteResponse = transport.DeleteDatabaseResponse{Success: true}
	return agent
}

func newCatalogFixture(t *testing.T) (wordpressInstallFixture, *catalogAgent) {
	t.Helper()
	fixture := newWordPressInstallFixture(t)
	if _, err := fixture.panel.db.GetDB().Exec(`UPDATE sites SET php_version = '8.3' WHERE id = ?`, wordpressSite); err != nil {
		t.Fatal(err)
	}
	agent := newCatalogAgent()
	attachWordPressInstallAgent(t, fixture.panel, agent)
	offered := map[string]bool{}
	for _, id := range []string{"wordpress", "prestashop", "nextcloud", "drupal", "joomla", "mediawiki", "phpbb"} {
		offered[id] = true
	}
	if err := fixture.panel.saveOfferedApps(context.Background(), offered); err != nil {
		t.Fatal(err)
	}
	return fixture, agent
}

func catalogRequest(method, path, body string) *http.Request {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	return req.WithContext(context.WithValue(req.Context(), callerKey, &Caller{ID: wordpressAdminID, Role: roleAdmin}))
}

func installCatalogApp(t *testing.T, f wordpressInstallFixture, app string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	f.panel.handleAppInstall(recorder, catalogRequest(http.MethodPost, "/api/v1/domains/7803/apps/install", `{"app":"`+app+`"}`), wordpressDomain)
	return recorder
}

func latestCatalogOperation(t *testing.T, f wordpressInstallFixture) (string, string) {
	t.Helper()
	var id, status string
	if err := f.panel.db.GetDB().QueryRow(`SELECT operation_id, status FROM app_catalog_operations ORDER BY rowid DESC LIMIT 1`).Scan(&id, &status); err != nil {
		t.Fatal(err)
	}
	return id, status
}

func appOperation(t *testing.T, f wordpressInstallFixture, method, op, action, body string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	f.panel.handleAppOperation(recorder, catalogRequest(method, "/api/v1/domains/7803/apps/"+op, body), wordpressDomain, op, action)
	return recorder
}

func TestCatalogInstallRunsEachStepOnceAndShowsThePasswordOnce(t *testing.T) {
	f, agent := newCatalogFixture(t)
	recorder := installCatalogApp(t, f, "drupal")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var answer map[string]any
	_ = json.Unmarshal(recorder.Body.Bytes(), &answer)
	password, _ := answer["admin_password"].(string)
	if answer["success"] != true || len(password) < 20 || !strings.HasSuffix(answer["admin_url"].(string), "/user/login") {
		t.Fatalf("answer %v", answer)
	}
	creates, _, _ := agent.snapshot()
	if agent.prereqCalls != 1 || len(creates) != 1 || len(agent.filesCalls) != 1 || len(agent.setupCalls) != 1 {
		t.Fatalf("steps: prereq=%d create=%d files=%d setup=%d", agent.prereqCalls, len(creates), len(agent.filesCalls), len(agent.setupCalls))
	}
	setup := agent.setupCalls[0]
	if setup.DBPass != creates[0].Password || setup.AdminPass != password || setup.DBName != creates[0].Name ||
		agent.filesCalls[0].Version != "11.4.11" || agent.filesCalls[0].Site.PHPVersion != "8.3" {
		t.Fatalf("the steps disagree: create=%+v files=%+v setup=%+v", creates[0], agent.filesCalls[0], setup)
	}
	op, status := latestCatalogOperation(t, f)
	if status != appOpApplied {
		t.Fatalf("status %s", status)
	}
	databases, users, grants := f.ledgerCounts(t)
	if databases != 1 || users != 1 || grants != 1 {
		t.Fatalf("the database is not counted: %d %d %d", databases, users, grants)
	}
	// The sealed copy is shown once, then forgotten.
	first := appOperation(t, f, http.MethodPost, op, "credentials", "{}")
	if first.Code != http.StatusOK || !strings.Contains(first.Body.String(), password) {
		t.Fatalf("reveal: %d %s", first.Code, first.Body.String())
	}
	if second := appOperation(t, f, http.MethodPost, op, "credentials", "{}"); second.Code != http.StatusGone || strings.Contains(second.Body.String(), password) {
		t.Fatalf("the password was shown twice: %d %s", second.Code, second.Body.String())
	}
}

func TestCatalogInstallIsNeverStartedTwiceOnADomain(t *testing.T) {
	f, agent := newCatalogFixture(t)
	if recorder := installCatalogApp(t, f, "drupal"); recorder.Code != http.StatusOK {
		t.Fatalf("first install: %d %s", recorder.Code, recorder.Body.String())
	}
	for _, app := range []string{"drupal", "joomla", "wordpress"} {
		recorder := installCatalogApp(t, f, app)
		if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), errCodeAppAlreadyInstalled) {
			t.Fatalf("%s: second install %d %s", app, recorder.Code, recorder.Body.String())
		}
	}
	creates, _, installs := agent.snapshot()
	if len(creates) != 1 || len(agent.filesCalls) != 1 || len(agent.setupCalls) != 1 || len(installs) != 0 {
		t.Fatalf("a second mutation started: create=%d files=%d setup=%d wordpress=%d", len(creates), len(agent.filesCalls), len(agent.setupCalls), len(installs))
	}
	// The database refuses a second live operation on the domain too.
	_, err := f.panel.db.GetDB().Exec(`INSERT INTO app_catalog_operations (operation_id, app_id, app_version, domain_id, subscription_id, site_id,
		database_server_id, database_name, database_user, cleanup_token_encrypted, status)
		VALUES (?, 'joomla', '5.4.10', 7803, 7802, 7804, 7805, 'x', 'x', 'x', 'reserved')`, strings.Repeat("e", 32))
	if err == nil {
		t.Fatal("a second live application row was accepted on the same domain")
	}
}

func TestCatalogInstallRefusesMissingExtensionsBeforeChangingAnything(t *testing.T) {
	f, agent := newCatalogFixture(t)
	agent.prereq.Missing = []transport.AppMissingExtension{{Extension: "intl", Package: "php8.3-intl"}, {Extension: "pdo_mysql", Package: "php8.3-mysql"}}
	recorder := installCatalogApp(t, f, "prestashop")
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), errCodeAppExtensionsMissing) ||
		!strings.Contains(recorder.Body.String(), "php8.3-intl php8.3-mysql") {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	creates, _, _ := agent.snapshot()
	databases, _, _ := f.ledgerCounts(t)
	var operations int
	_ = f.panel.db.GetDB().QueryRow(`SELECT COUNT(*) FROM app_catalog_operations`).Scan(&operations)
	if len(creates) != 0 || len(agent.filesCalls) != 0 || databases != 0 || operations != 0 {
		t.Fatalf("something changed: creates=%d files=%d databases=%d operations=%d", len(creates), len(agent.filesCalls), databases, operations)
	}

	// A PHP the application does not support is refused before the server is asked.
	agent.prereq.Missing = nil
	if _, err := f.panel.db.GetDB().Exec(`UPDATE sites SET php_version = '8.2' WHERE id = ?`, wordpressSite); err != nil {
		t.Fatal(err)
	}
	before := agent.prereqCalls
	recorder = installCatalogApp(t, f, "nextcloud")
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), errCodeAppPHPUnsupported) || agent.prereqCalls != before {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

// alpha.118 lab round 2: a prerequisite check without an answer says why.
// A refusal by the agent and an answer that did not come in time (other work
// on the server) each have their own reason; nothing is created.
func TestCatalogPrerequisitesWithoutAnswerSayWhy(t *testing.T) {
	f, agent := newCatalogFixture(t)
	agent.prereq = transport.AppPrerequisitesResponse{Error: `"wordpress" is not an application this agent can install`}
	recorder := installCatalogApp(t, f, "prestashop")
	var body apiErrorBody
	_ = json.Unmarshal(recorder.Body.Bytes(), &body)
	if recorder.Code != http.StatusServiceUnavailable || body.Code != errCodeAppPrereqUnknown || body.Reason != appPrereqRefused ||
		!strings.Contains(body.Error, "is not an application") || body.Vars["app"] != "PrestaShop" || !strings.Contains(body.Vars["cause"], "is not an application") {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var operations int
	_ = f.panel.db.GetDB().QueryRow(`SELECT COUNT(*) FROM app_catalog_operations`).Scan(&operations)
	if creates, _, _ := agent.snapshot(); len(creates) != 0 || len(agent.filesCalls) != 0 || operations != 0 {
		t.Fatalf("something changed: creates=%d files=%d operations=%d", len(creates), len(agent.filesCalls), operations)
	}

	app, _ := appcatalog.Lookup("wordpress")
	reason, message := appPrereqFailure(app, context.DeadlineExceeded, transport.AppPrerequisitesResponse{})
	if reason != appPrereqBusy || !strings.Contains(message, "Nobody needs to fix anything") || !strings.Contains(message, "install again") {
		t.Fatalf("busy: %q %q", reason, message)
	}
	reason, message = appPrereqFailure(app, errAgentRPCClientUnavailable, transport.AppPrerequisitesResponse{})
	if reason != "" || !strings.Contains(message, "agent is running") {
		t.Fatalf("unreachable: %q %q", reason, message)
	}
}

func TestCatalogInstallVerificationFailureLeavesNothingBehind(t *testing.T) {
	f, agent := newCatalogFixture(t)
	agent.files = transport.InstallCatalogAppResponse{CompensationSafe: true, ErrorKind: transport.AppFailVerification,
		Error: "the downloaded Joomla 5.4.10 release is not the file CelikPanel pinned"}
	recorder := installCatalogApp(t, f, "joomla")
	if recorder.Code != http.StatusBadGateway || !strings.Contains(recorder.Body.String(), errCodeAppVerification) {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	_, deletes, _ := agent.snapshot()
	if len(deletes) != 1 || !deletes[0].RequireOwnershipProof || len(agent.setupCalls) != 0 {
		t.Fatalf("the database was not removed by proof, or setup ran: deletes=%+v setup=%d", deletes, len(agent.setupCalls))
	}
	databases, users, grants := f.ledgerCounts(t)
	if _, status := latestCatalogOperation(t, f); status != appOpFailed || databases+users+grants != 0 {
		t.Fatalf("status=%s ledger=%d/%d/%d", status, databases, users, grants)
	}
}

func TestCatalogSetupFailureRetriesOnlyTheSetup(t *testing.T) {
	f, agent := newCatalogFixture(t)
	agent.setup = transport.SetupCatalogAppResponse{ErrorKind: transport.AppFailSetup, Error: "Nextcloud's own installer did not finish", Output: "Error"}
	recorder := installCatalogApp(t, f, "nextcloud")
	if recorder.Code != http.StatusBadGateway || !strings.Contains(recorder.Body.String(), errCodeAppSetupFailed) {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	op, status := latestCatalogOperation(t, f)
	if status != appOpSetupFailed {
		t.Fatalf("status %s", status)
	}
	// A retry before anything changed: the same setup, the same secrets.
	agent.setup = transport.SetupCatalogAppResponse{Configured: true, AdminPath: "/", Detail: "set up"}
	retry := appOperation(t, f, http.MethodPost, op, "retry", "{}")
	if retry.Code != http.StatusOK {
		t.Fatalf("retry: %d %s", retry.Code, retry.Body.String())
	}
	creates, _, _ := agent.snapshot()
	if len(creates) != 1 || len(agent.filesCalls) != 1 || len(agent.setupCalls) != 2 {
		t.Fatalf("retry repeated an earlier step: create=%d files=%d setup=%d", len(creates), len(agent.filesCalls), len(agent.setupCalls))
	}
	if agent.setupCalls[0].DBPass != agent.setupCalls[1].DBPass || agent.setupCalls[0].AdminPass != agent.setupCalls[1].AdminPass {
		t.Fatal("the retry did not use the operation's own secrets")
	}
	if _, status := latestCatalogOperation(t, f); status != appOpApplied {
		t.Fatalf("status after retry %s", status)
	}
	if again := appOperation(t, f, http.MethodPost, op, "retry", "{}"); again.Code != http.StatusConflict || len(agent.setupCalls) != 2 {
		t.Fatalf("a finished setup was retried: %d", again.Code)
	}
}

func TestCatalogRemoveKeepsTheDatabaseUnlessConfirmed(t *testing.T) {
	f, agent := newCatalogFixture(t)
	if recorder := installCatalogApp(t, f, "phpbb"); recorder.Code != http.StatusOK {
		t.Fatalf("install: %d %s", recorder.Code, recorder.Body.String())
	}
	op, _ := latestCatalogOperation(t, f)
	if wrong := appOperation(t, f, http.MethodDelete, op, "remove", `{"confirm":"other.example"}`); wrong.Code != http.StatusBadRequest || len(agent.removeCalls) != 0 {
		t.Fatalf("a removal without the domain's name went on: %d", wrong.Code)
	}
	recorder := appOperation(t, f, http.MethodDelete, op, "remove", `{"confirm":"wordpress.example","remove_database":false}`)
	if recorder.Code != http.StatusOK || len(agent.removeCalls) != 1 {
		t.Fatalf("remove: %d %s", recorder.Code, recorder.Body.String())
	}
	_, deletes, _ := agent.snapshot()
	databases, _, _ := f.ledgerCounts(t)
	if len(deletes) != 0 || databases != 1 {
		t.Fatalf("the database was removed without confirmation: deletes=%d databases=%d", len(deletes), databases)
	}
	if _, status := latestCatalogOperation(t, f); status != appOpRemoved {
		t.Fatalf("status %s", status)
	}
	// The domain is free again.
	if recorder := installCatalogApp(t, f, "mediawiki"); recorder.Code != http.StatusOK {
		t.Fatalf("install after removal: %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestAppOfferIsOptInAndNarrowedByEveryLayer(t *testing.T) {
	f, agent := newCatalogFixture(t)
	ctx := context.Background()
	if _, err := f.panel.db.GetDB().Exec(`DELETE FROM panel_settings WHERE key = ?`, settingOfferedApps); err != nil {
		t.Fatal(err)
	}
	layer, err := f.panel.loadAdminOfferLayer(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !layer.offers("app:wordpress") || layer.offers("app:nextcloud") {
		t.Fatal("before any choice only WordPress is offered")
	}
	// Not offered: refused for everyone, the administrator too, before
	// anything is asked of the server.
	if recorder := installCatalogApp(t, f, "nextcloud"); recorder.Code != http.StatusConflict ||
		!strings.Contains(recorder.Body.String(), errCodeNotOffered) || agent.prereqCalls != 0 {
		t.Fatalf("an application nobody offered was installed: %d %s", recorder.Code, recorder.Body.String())
	}
	recorder := httptest.NewRecorder()
	f.panel.handleOfferedVersions(recorder, catalogRequest(http.MethodPut, "/api/v1/offered-versions", `{"offering_id":"app:nextcloud","offered":true}`))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"nextcloud"`) {
		t.Fatalf("offer: %d %s", recorder.Code, recorder.Body.String())
	}
	layer, _ = f.panel.loadAdminOfferLayer(ctx)
	if !layer.offers("app:nextcloud") || !layer.offers("app:wordpress") || layer.offers("app:drupal") {
		t.Fatal("the offer was not recorded as chosen")
	}
	// A reseller without the item narrows it; so does a Custom subscription.
	e := &effectiveRights{admin: layer, reseller: &rightsState{Set: newRightsSet(true, false, "component:mariadb")}, Own: rightsState{Set: allOfferedRightsSet()}}
	if got := e.offeringRefusal("app:nextcloud"); got != rightsLayerReseller {
		t.Fatalf("reseller layer: %q", got)
	}
	e.reseller = &rightsState{Set: allOfferedRightsSet()}
	e.Own = rightsState{Set: newRightsSet(true, false, "app:wordpress")}
	if got := e.offeringRefusal("app:nextcloud"); got != rightsLayerSubscription {
		t.Fatalf("subscription layer: %q", got)
	}
	if got := e.offeringRefusal("app:wordpress"); got != "" {
		t.Fatalf("WordPress: %q", got)
	}
	if got := e.offeringRefusal("app:drupal"); got != rightsLayerAdministrator {
		t.Fatalf("an application the administrator does not offer: %q", got)
	}
	if !validRightsItem("app:nextcloud") || validRightsItem("app:../x") {
		t.Fatal("app offering ids are not validated")
	}
}

func TestCatalogRecoveryAfterRestartNeverGuesses(t *testing.T) {
	f, _ := newCatalogFixture(t)
	db := f.panel.db.GetDB()
	insert := func(id, domainStatus string) {
		t.Helper()
		if _, err := db.Exec(`INSERT INTO app_catalog_operations (operation_id, app_id, app_version, domain_id, subscription_id, site_id,
			database_server_id, database_name, database_user, cleanup_token_encrypted, status) VALUES (?, 'drupal', '11.4.11', 7803, 7802, 7804, 7805, ?, ?, 'x', ?)`,
			id, id[:6], id[:6], domainStatus); err != nil {
			t.Fatal(err)
		}
	}
	insert(strings.Repeat("a", 32), appOpSettingUp)
	if _, err := f.panel.recoverInterruptedCatalogAppOperations(context.Background()); err != nil {
		t.Fatal(err)
	}
	var status string
	_ = db.QueryRow(`SELECT status FROM app_catalog_operations WHERE operation_id = ?`, strings.Repeat("a", 32)).Scan(&status)
	if status != appOpSetupFailed {
		t.Fatalf("an interrupted setup is %s, want setup_failed (Retry is safe)", status)
	}
	if _, err := db.Exec(`UPDATE app_catalog_operations SET status = 'files_installing' WHERE operation_id = ?`, strings.Repeat("a", 32)); err != nil {
		t.Fatal(err)
	}
	_, _ = f.panel.recoverInterruptedCatalogAppOperations(context.Background())
	_ = db.QueryRow(`SELECT status FROM app_catalog_operations WHERE operation_id = ?`, strings.Repeat("a", 32)).Scan(&status)
	if status != appOpNeedsReview {
		t.Fatalf("interrupted file publication is %s, want needs_review", status)
	}
}
