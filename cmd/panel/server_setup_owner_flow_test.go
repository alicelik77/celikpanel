package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

// alpha.114 (owner, 2026-10-31): the main domain becomes a website at setup,
// mail is added from Components through a mail-only extension, and an
// extension shows the DNS software the server runs as fixed.

func setupStepKinds(plan serverSetupPlan) []string {
	kinds := []string{}
	for _, step := range plan.Steps {
		kinds = append(kinds, step.Kind+":"+step.Target)
	}
	return kinds
}

func TestServerSetupMainSiteStepIsPlannedOnTheFirstServerOnly(t *testing.T) {
	base := serverSetupDraft{Purpose: "web", MainDomain: "example.test", DNSMode: "local", DNSRole: "primary", MainSite: serverSetupMainSiteHere}
	if !serverSetupMainSiteStep(base, false) {
		t.Fatal("first server, website here: planned")
	}
	for name, draft := range map[string]serverSetupDraft{
		"elsewhere": {Purpose: "web", MainDomain: "example.test", DNSMode: "local", DNSRole: "primary", MainSite: serverSetupMainSiteElsewhere},
		"older":     {Purpose: "web", MainDomain: "example.test", DNSMode: "local", DNSRole: "primary"},
		"dns only":  {Purpose: "dns", MainDomain: "example.test", DNSMode: "local", DNSRole: "primary", MainSite: serverSetupMainSiteHere},
		"backup":    {Purpose: "web", MainDomain: "example.test", DNSMode: "local", DNSRole: "secondary", MainSite: serverSetupMainSiteHere},
	} {
		if serverSetupMainSiteStep(draft, false) {
			t.Fatalf("%s: no website step", name)
		}
	}
	if serverSetupMainSiteStep(base, true) {
		t.Fatal("a joined server is never asked and creates no main-domain website")
	}
	// An older draft keeps its identity: the answer is omitted when empty.
	raw, _ := json.Marshal(serverSetupDraft{Purpose: "web"})
	if strings.Contains(string(raw), "main_site") {
		t.Fatalf("older draft changed: %s", raw)
	}
	if _, err := canonicalServerSetupDraft(serverSetupDraft{Purpose: "web", DNSMode: "external", MainSite: "maybe"}); err == nil {
		t.Fatal("an unknown answer is refused")
	}
}

func TestServerSetupPlanCreatesTheMainDomainWebsiteBeforeTheFinalCheck(t *testing.T) {
	f, state := setupOperationFixture(t)
	draft := state.Draft
	draft.MainDomain, draft.MainSite = "example.test", serverSetupMainSiteHere
	state, err := f.panel.saveServerSetupDraft(context.Background(), state.Revision, draft)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := f.panel.buildServerSetupPlan(context.Background(), state, serviceOperationActor{UserID: f.userID})
	if err != nil {
		t.Fatal(err)
	}
	n := len(plan.Steps)
	if n < 2 || plan.Steps[n-1].Kind != "verify" || plan.Steps[n-2].Kind != "main_site" || plan.Steps[n-2].Target != "example.test" {
		t.Fatalf("main site step: %v", setupStepKinds(plan))
	}
	draft.MainSite = serverSetupMainSiteElsewhere
	state, err = f.panel.saveServerSetupDraft(context.Background(), state.Revision, draft)
	if err != nil {
		t.Fatal(err)
	}
	plan, err = f.panel.buildServerSetupPlan(context.Background(), state, serviceOperationActor{UserID: f.userID})
	if err != nil || serverSetupPlanHasStep(plan, "main_site") {
		t.Fatalf("website elsewhere: %v %v", setupStepKinds(plan), err)
	}
}

// A resumed or repeated step reads what exists and creates nothing twice; a
// DNS-only domain the owner made is never replaced. The step never fails setup.
func TestServerSetupMainSiteStepKeepsWhatExists(t *testing.T) {
	f, state := setupOperationFixture(t)
	db := f.database.GetDB()
	if _, err := db.Exec(`
		INSERT INTO subscriptions (id, owner_id, name, max_databases, status) VALUES (8802, ?, 'owner', 10, 'active');
		INSERT INTO domains (id, subscription_id, name, status) VALUES (8803, 8802, 'example.test', 'active');
		INSERT INTO sites (id, domain_id, document_root, project_type, status) VALUES (8804, 8803, '/srv/example', 'static', 'active');
		INSERT INTO ssl_certificates (domain_id,type,cert_path,key_path,expires_at,auto_renew,renewal_status,status)
		VALUES (8803,'letsencrypt','/tmp/a.crt','/tmp/a.key','2099-01-01T00:00:00Z',1,'','active');
		INSERT INTO domains (id, subscription_id, name, status) VALUES (8805, 8802, 'dnsonly.test', 'active');
		INSERT INTO sites (id, domain_id, document_root, project_type, status) VALUES (8806, 8805, '/srv/dns', 'dnsonly', 'active');`, f.userID); err != nil {
		t.Fatal(err)
	}
	plan := serverSetupPlan{Draft: state.Draft, Actor: serviceOperationActor{UserID: f.userID}}
	step := &serverSetupExecutionStep{serverSetupPlanStep: serverSetupPlanStep{ID: "09-main_site", Kind: "main_site", Target: "example.test"}}
	done, err := f.panel.runServerSetupMainSite(context.Background(), plan, step)
	if !done || err != nil || step.MainSite == nil || step.MainSite.Site != "existed" || step.MainSite.Certificate != "active" || step.MainSite.DomainID != 8803 {
		t.Fatalf("existing website: %v %v %+v", done, err, step.MainSite)
	}
	step = &serverSetupExecutionStep{serverSetupPlanStep: serverSetupPlanStep{ID: "09-main_site", Kind: "main_site", Target: "dnsonly.test"}}
	done, err = f.panel.runServerSetupMainSite(context.Background(), plan, step)
	if !done || err != nil || step.MainSite.Site != "failed" || step.MainSite.Code != "server_setup_main_site_dns_only" {
		t.Fatalf("DNS-only domain kept: %v %v %+v", done, err, step.MainSite)
	}
	// An account that is no longer an administrator creates nothing.
	plan.Actor.UserID = 999999
	step = &serverSetupExecutionStep{serverSetupPlanStep: serverSetupPlanStep{ID: "09-main_site", Kind: "main_site", Target: "new.test"}}
	done, err = f.panel.runServerSetupMainSite(context.Background(), plan, step)
	if !done || err != nil || step.MainSite.Site != "failed" || step.MainSite.Code != "server_setup_main_site_account" {
		t.Fatalf("removed account: %v %v %+v", done, err, step.MainSite)
	}
	var count int
	_ = db.QueryRow(`SELECT COUNT(*) FROM domains WHERE name='new.test'`).Scan(&count)
	if count != 0 {
		t.Fatal("a domain was created without an administrator")
	}
}

func TestServerSetupMailScopeExtensionPlansTheMailStepsOnly(t *testing.T) {
	f, _ := setupOperationFixture(t)
	ready := markServerSetupReadyForTest(t, f)
	full, err := f.panel.buildServerSetupPlan(context.Background(), ready, serviceOperationActor{UserID: f.userID})
	if err != nil {
		t.Fatal(err)
	}
	w := postSetupExtendForTest(t, f, map[string]any{"revision": ready.Revision, "action": "begin", "scope": "mail"})
	if w.Code != http.StatusBadRequest || setupErrorCode(t, w) != errCodeServerSetupExtendNoMail {
		t.Fatalf("no mail to repair: %d %s", w.Code, w.Body.String())
	}
	w = postSetupExtendForTest(t, f, map[string]any{"revision": ready.Revision, "action": "begin", "scope": "mail", "mail_profile": "core-mail", "mail_hostname": "mail.example.test"})
	if w.Code != http.StatusOK {
		t.Fatalf("begin: %d %s", w.Code, w.Body.String())
	}
	state, err := f.panel.loadServerSetup(context.Background())
	if err != nil || state.ExtensionScope != serverSetupExtensionScopeMail {
		t.Fatalf("scope: %+v %v", state, err)
	}
	plan, err := f.panel.buildServerSetupPlan(context.Background(), state, serviceOperationActor{UserID: f.userID})
	if err != nil {
		t.Fatal(err)
	}
	kinds := setupStepKinds(plan)
	if plan.Scope != serverSetupExtensionScopeMail || !slices.Contains(kinds, "mail_profile:core-mail") || !slices.Contains(kinds, "access_dns:mail.example.test") || !slices.Contains(kinds, "mail_certificate:mail.example.test") || kinds[len(kinds)-1] != "verify:"+plan.Draft.Purpose {
		t.Fatalf("mail steps: %v", kinds)
	}
	for _, step := range plan.Steps {
		if stringIn(step.Kind, "dns", "dns_readiness", "dns_publisher", "infrastructure_dns", "panel_certificate", "peer_join", "peer_names", "main_site", "runtime") || (step.Kind == "access_dns" && step.Target == plan.Draft.PanelDomain) {
			t.Fatalf("a mail-only extension plans %s: %v", step.Kind, kinds)
		}
	}
	// Names and DNS software stay as completed; the plan identity differs from a full extension.
	if plan.Draft.PanelDomain != ready.Draft.PanelDomain || plan.Draft.DNSMode != ready.Draft.DNSMode || plan.Draft.DNSEngine != ready.Draft.DNSEngine || len(plan.Steps) >= len(full.Steps)+4 {
		t.Fatalf("other answers changed: %+v", plan.Draft)
	}
	// An edit in the wizard that changes anything else returns to the full review.
	edited := state.Draft
	edited.PanelDomain = "admin.example.test"
	state, err = f.panel.saveServerSetupDraft(context.Background(), state.Revision, edited)
	if err != nil || state.ExtensionScope != "" {
		t.Fatalf("scope kept after another change: %q %v", state.ExtensionScope, err)
	}
	// Stopping the extension removes the scope.
	w = postSetupExtendForTest(t, f, map[string]any{"revision": state.Revision, "action": "cancel"})
	if w.Code != http.StatusOK {
		t.Fatalf("cancel: %d %s", w.Code, w.Body.String())
	}
	var leftover int
	_ = f.database.GetDB().QueryRow(`SELECT COUNT(*) FROM panel_settings WHERE key=?`, settingServerSetupExtensionScope).Scan(&leftover)
	if leftover != 0 {
		t.Fatal("the scope outlived the extension")
	}
}

func TestServerSetupMailOnlyChange(t *testing.T) {
	base, err := canonicalServerSetupDraft(serverSetupDraft{Purpose: "web", PanelDomain: "panel.example.test", DNSMode: "external"})
	if err != nil {
		t.Fatal(err)
	}
	mail, err := serverSetupDraftWithMailProfile(base, "protected-mail")
	if err != nil {
		t.Fatal(err)
	}
	mail.MailHostname = "mail.example.test"
	if !serverSetupMailOnlyChange(base, mail) || !serverSetupMailOnlyChange(mail, mail) {
		t.Fatal("adding or repairing mail is a mail-only change")
	}
	other := mail
	other.DNSEngine = "pdns"
	if serverSetupMailOnlyChange(base, other) {
		t.Fatal("a DNS software change is not mail-only")
	}
	removed := base
	removed.Customization = &serverSetupCustomization{Components: []string{"postfix", "dovecot"}}
	if serverSetupMailOnlyChange(base, removed) {
		t.Fatal("removing the web server is not mail-only")
	}
}

func joinedReadyForMailScope(t *testing.T, pair *peerPair) serverSetupState {
	t.Helper()
	pair.secondReview(t, nil)
	db := pair.second.database.GetDB()
	if _, err := db.Exec(`UPDATE server_setup_state SET status='ready', completed_at='2026-10-31T09:00:00Z' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	if err := pair.second.panel.revokeServerSetupLinkSide(context.Background(), serverSetupLinkRoleSecond); err != nil {
		t.Fatal(err)
	}
	state, err := pair.second.panel.loadServerSetup(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func beginJoinedMailScope(t *testing.T, pair *peerPair, state serverSetupState) serverSetupPlan {
	t.Helper()
	raw, _ := json.Marshal(map[string]any{"revision": state.Revision, "action": "begin", "scope": "mail", "mail_profile": "protected-mail", "mail_hostname": "mail.beta.example.test"})
	w := httptest.NewRecorder()
	pair.second.panel.handleServerSetupExtend(w, serviceOperationAdminRequest(t, http.MethodPost, "/api/v1/setup/extend", string(raw), pair.second.userID))
	if w.Code != http.StatusOK {
		t.Fatalf("begin: %d %s", w.Code, w.Body.String())
	}
	state, err := pair.second.panel.loadServerSetup(context.Background())
	if err != nil || state.ExtensionScope != serverSetupExtensionScopeMail {
		t.Fatalf("scope: %q %v", state.ExtensionScope, err)
	}
	plan, err := pair.second.panel.buildServerSetupPlan(context.Background(), state, serviceOperationActor{UserID: pair.second.userID})
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

// On a joined server the main domain's mail records stay with the first
// server: a mail-only extension plans no mail step for the main domain, no
// zone and no names step, whether or not the first server has mail.
func TestServerSetupJoinedMailScopeNeverTouchesTheMainDomainMail(t *testing.T) {
	for name, pair := range map[string]*peerPair{"first has mail": joinedMailPair(t), "first has no mail": newJoinedPairWithoutMail(t)} {
		state := joinedReadyForMailScope(t, pair)
		plan := beginJoinedMailScope(t, pair, state)
		kinds := setupStepKinds(plan)
		if plan.InfrastructureDNS != nil || !slices.Contains(kinds, "access_dns:mail.beta.example.test") || !slices.Contains(kinds, "mail_certificate:mail.beta.example.test") {
			t.Fatalf("%s: %v", name, kinds)
		}
		for _, step := range plan.Steps {
			if stringIn(step.Kind, "mail_dns", "infrastructure_dns", "peer_names", "dns", "main_site") {
				t.Fatalf("%s: the joined server's mail-only extension plans %s: %v", name, step.Kind, kinds)
			}
		}
	}
}

func newJoinedPairWithoutMail(t *testing.T) *peerPair {
	t.Helper()
	pair := newPeerPair(t, joinFirstDraft())
	if answer := pair.connect(t, issueJoinCode(t, pair.first).Code); answer.State != "ok" {
		t.Fatalf("join: %+v", answer)
	}
	link, err := pair.second.panel.readServerSetupLink(context.Background())
	if err != nil || link == nil || link.Document.First.MainDomainMail {
		t.Fatalf("the first server has no mail: %+v %v", link, err)
	}
	return pair
}

// The DNS software the server runs is marked active: the wizard shows it as
// one fixed line instead of a choice (owner report, Boston, 2026-10-31).
func TestServerSetupDNSEngineChoicesMarkTheRunningSoftware(t *testing.T) {
	choices := serverSetupDNSEngineChoices(serverSetupCatalogInputs{InventoryReady: true, InstalledIDs: []string{"bind"}, DNSState: &dnsEngineDBState{ActiveEngine: "bind"}, OwnDNSEngine: true})
	for _, choice := range choices {
		if choice.Active != (choice.ID == "bind") {
			t.Fatalf("active: %+v", choice)
		}
		if choice.ID == "pdns" && choice.Offered {
			t.Fatalf("PowerDNS is not offered while BIND runs: %+v", choice)
		}
	}
	for _, choice := range serverSetupDNSEngineChoices(serverSetupCatalogInputs{InventoryReady: true, DNSState: &dnsEngineDBState{}}) {
		if choice.Active {
			t.Fatalf("no DNS software runs: %+v", choice)
		}
	}
}
