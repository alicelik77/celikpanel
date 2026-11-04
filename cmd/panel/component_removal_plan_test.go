package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/transport"
)

// The removal plan (alpha.123, D-045) against SQLite with the tables each
// kind reads. Built on the dependents fixture: php sites on 8.3/8.4, a node
// site on 24.18.0, a mailbox, a MariaDB database.
func newRemovalPlanTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db := newDependentsTestDB(t)
	if _, err := db.Exec(`
		INSERT INTO domains (id, subscription_id, name) VALUES (107, 10, 'apacheweb.example'), (108, 20, 'n8n.example'), (109, 20, 'webui.example');
		INSERT INTO sites (id, domain_id, project_type, php_version, web_server) VALUES (8, 107, 'php', '8.2', 'apache');
		CREATE TABLE ssl_certificates (id INTEGER PRIMARY KEY, domain_id INTEGER, type TEXT, status TEXT, auto_renew INTEGER);
		INSERT INTO ssl_certificates VALUES (1, 100, 'letsencrypt', 'active', 1), (2, 101, 'custom', 'active', 1);
		CREATE TABLE vpn_peers (id INTEGER PRIMARY KEY, name TEXT, desired_state TEXT);
		CREATE TABLE malware_findings (id INTEGER PRIMARY KEY, domain_id INTEGER, state TEXT);
		CREATE TABLE service_app_installs (id INTEGER PRIMARY KEY, domain_id INTEGER, app_id TEXT);
		INSERT INTO service_app_installs VALUES (12, 108, 'n8n'), (13, 109, 'open-webui');
		CREATE TABLE runtime_apps (id INTEGER PRIMARY KEY, domain_id INTEGER, kind TEXT);
		CREATE TABLE panel_settings (key TEXT PRIMARY KEY, value TEXT, updated_at TEXT);
		CREATE TABLE rights_set_items (set_id INTEGER, item TEXT, PRIMARY KEY (set_id, item));
		INSERT INTO rights_set_items VALUES (1, 'component:php-fpm:8.3'), (1, 'component:node:24'), (2, 'component:php-fpm:8.3'),
			(2, 'component:postgresql:17'), (3, 'component:postgresql'), (3, 'component:php-fpm:8.4');`); err != nil {
		t.Fatal(err)
	}
	return db
}

func planFor(t *testing.T, db *sql.DB, id, version string, installed map[string]bool, facts removalHostFacts) removalPlan {
	t.Helper()
	plan, err := computeRemovalPlan(context.Background(), db, core.GetManagedServiceByID(id), version, installed, facts)
	if err != nil {
		t.Fatalf("%s %s: %v", id, version, err)
	}
	return plan
}

func blockerKinds(plan removalPlan) []string {
	out := []string{}
	for _, blocker := range plan.Blockers {
		kind := blocker.Kind
		if blocker.Component != "" {
			kind += ":" + blocker.Component
		}
		out = append(out, kind)
	}
	return out
}

func itemNames(dependent removalDependent) []string {
	out := []string{}
	for _, item := range dependent.Items {
		out = append(out, item.Name)
	}
	return out
}

// Case 1: a Node.js version's dependents are its sites and the service apps
// whose units run on it (n8n), plus any unit the Panel has no record of.
func TestRemovalPlanNodeVersionNamesSitesAndServiceApps(t *testing.T) {
	db := newRemovalPlanTestDB(t)
	plan := planFor(t, db, "node", "24.18.0", nil, removalHostFacts{RuntimeUnits: []string{"celiksvc-12", "celikrt-99"}, RuntimeUnitsKnown: true})
	if !slices.Equal(blockerKinds(plan), []string{"node_sites", "node_service_apps"}) {
		t.Fatalf("blockers = %v", blockerKinds(plan))
	}
	if names := itemNames(plan.Blockers[1]); !slices.Equal(names, []string{"n8n (n8n.example)", "celikrt-99"}) {
		t.Fatalf("service apps = %v", names)
	}
	if plan.Blockers[0].Items[0].Link != "/domains/nodeapp.example" {
		t.Fatalf("site link = %+v", plan.Blockers[0].Items)
	}
	if !slices.Equal(plan.Withdraws, []string{"component:node:24"}) || plan.WithdrawSets != 1 {
		t.Fatalf("withdraws = %v %d", plan.Withdraws, plan.WithdrawSets)
	}
	// Another version with nothing on it: no blockers. Units unread: a note,
	// the Agent checks again when Remove is pressed.
	plan = planFor(t, db, "node", "22.11.0", nil, removalHostFacts{})
	if len(plan.Blockers) != 0 || !slices.Equal(plan.Notes, []string{removalNoteUnitsUnknown}) {
		t.Fatalf("22: %+v", plan)
	}
	// The whole component: every Node service app, by name.
	plan = planFor(t, db, "node", "", map[string]bool{"node": true, "nginx": true}, removalHostFacts{})
	if !slices.Equal(blockerKinds(plan), []string{"node_sites", "node_service_apps"}) || plan.Blockers[1].Count != 1 {
		t.Fatalf("node whole: %+v", plan.Blockers)
	}
	// Python: Open WebUI.
	plan = planFor(t, db, "python", "", map[string]bool{"python": true}, removalHostFacts{})
	if !slices.Equal(blockerKinds(plan), []string{"python_apps"}) || plan.Blockers[0].Items[0].Name != "Open WebUI (webui.example)" {
		t.Fatalf("python: %+v", plan.Blockers)
	}
}

// Case 2: Certbot is refused while the panel renews a certificate with it.
func TestRemovalPlanCertbotListsCertificates(t *testing.T) {
	db := newRemovalPlanTestDB(t)
	plan := planFor(t, db, "certbot", "", map[string]bool{"certbot": true}, removalHostFacts{PanelCertificate: "panel.example.com", MailHosts: []string{"mail.example.com"}})
	if len(plan.Blockers) != 1 || plan.Blockers[0].Count != 3 ||
		!slices.Equal(itemNames(plan.Blockers[0]), []string{"panel.example.com", "mail.example.com", "php83.example"}) {
		t.Fatalf("certbot: %+v", plan.Blockers)
	}
	if plan.Blockers[0].Items[2].Link != "/domains/php83.example?tab=ssl" {
		t.Fatalf("link: %+v", plan.Blockers[0].Items[2])
	}
}

// Case 3: nftables is required by the panel firewall and by Fail2ban; an
// unread firewall state refuses too.
func TestRemovalPlanFirewallEngine(t *testing.T) {
	db := newRemovalPlanTestDB(t)
	on := &serviceRemovalFirewall{Known: true, Enabled: true}
	off := &serviceRemovalFirewall{Known: true}
	plan := planFor(t, db, "nftables", "", map[string]bool{"nftables": true, "fail2ban": true}, removalHostFacts{Firewall: on})
	if !slices.Equal(blockerKinds(plan), []string{"component:fail2ban", core.UsePanelFirewall}) || len(plan.Together) != 0 {
		t.Fatalf("on + fail2ban: %v %v", blockerKinds(plan), plan.Together)
	}
	plan = planFor(t, db, "nftables", "", map[string]bool{"nftables": true}, removalHostFacts{})
	if !slices.Equal(blockerKinds(plan), []string{removalKindFirewallUnknown}) {
		t.Fatalf("unknown: %v", blockerKinds(plan))
	}
	plan = planFor(t, db, "nftables", "", map[string]bool{"nftables": true}, removalHostFacts{Firewall: off})
	if len(plan.Blockers) != 0 {
		t.Fatalf("off: %v", blockerKinds(plan))
	}
	// Fail2ban itself is a leaf: a confirmation that says ban protection stops.
	plan = planFor(t, db, "fail2ban", "", map[string]bool{"nftables": true, "fail2ban": true}, removalHostFacts{})
	if len(plan.Blockers) != 0 || len(plan.Stops) != 1 || plan.Stops[0].Kind != core.StopBanProtection {
		t.Fatalf("fail2ban: %+v", plan)
	}
}

// Cases 4 and 5: VPN peers and quarantined files are customer data; the
// malware scan schedule is a setting turned off together.
func TestRemovalPlanWireGuardAndClamAV(t *testing.T) {
	db := newRemovalPlanTestDB(t)
	if _, err := db.Exec(`INSERT INTO vpn_peers VALUES (1, 'laptop', 'active'), (2, 'old', 'revoked');
		INSERT INTO malware_findings VALUES (1, 100, 'quarantined'), (2, 100, 'quarantined'), (3, 101, 'restored');
		INSERT INTO panel_settings VALUES ('malware_scan_schedule', '{"enabled":true}', '');`); err != nil {
		t.Fatal(err)
	}
	plan := planFor(t, db, "wireguard", "", map[string]bool{"wireguard": true}, removalHostFacts{})
	if len(plan.Blockers) != 1 || !slices.Equal(itemNames(plan.Blockers[0]), []string{"laptop"}) {
		t.Fatalf("wireguard: %+v", plan.Blockers)
	}
	plan = planFor(t, db, "clamav", "", map[string]bool{"clamav": true}, removalHostFacts{})
	if len(plan.Blockers) != 1 || plan.Blockers[0].Count != 2 || !slices.Equal(itemNames(plan.Blockers[0]), []string{"php83.example (2)"}) {
		t.Fatalf("clamav blockers: %+v", plan.Blockers)
	}
	if !slices.Equal(plan.Together, []string{core.TogetherMalwareSchedule}) || plan.Stops[0].Kind != core.StopMalwareScans {
		t.Fatalf("clamav together/stops: %v %v", plan.Together, plan.Stops)
	}
	if _, err := db.Exec(`UPDATE malware_findings SET state = 'deleted'`); err != nil {
		t.Fatal(err)
	}
	if plan = planFor(t, db, "clamav", "", map[string]bool{"clamav": true}, removalHostFacts{}); len(plan.Blockers) != 0 || len(plan.Together) != 1 {
		t.Fatalf("clamav emptied: %+v", plan)
	}
}

// Case 6: components that require the target are blockers unless they are
// leaves nothing else uses, which go together; runtimes never do.
func TestRemovalPlanRequiredComponents(t *testing.T) {
	db := newRemovalPlanTestDB(t)
	if _, err := db.Exec(`DELETE FROM sites; DELETE FROM email_accounts; DELETE FROM databases_v2; DELETE FROM databases`); err != nil {
		t.Fatal(err)
	}
	installed := map[string]bool{"nginx": true, "php-fpm": true, "mariadb": true, "phpmyadmin": true, "node": true,
		"postfix": true, "dovecot": true, "roundcube": true, "rspamd": true}
	plan := planFor(t, db, "nginx", "", installed, removalHostFacts{})
	if !slices.Equal(blockerKinds(plan), []string{"component:node"}) || !slices.Equal(plan.Together, []string{"phpmyadmin", "roundcube"}) {
		t.Fatalf("nginx: %v together %v", blockerKinds(plan), plan.Together)
	}
	plan = planFor(t, db, "postfix", "", installed, removalHostFacts{})
	if len(plan.Blockers) != 0 || !slices.Equal(plan.Together, []string{"rspamd", "roundcube"}) {
		t.Fatalf("postfix: %v together %v", blockerKinds(plan), plan.Together)
	}
	stops := []string{}
	for _, stop := range plan.Stops {
		stops = append(stops, stop.Kind)
	}
	if !slices.Equal(stops, []string{core.StopSpamFiltering, core.StopWebmail}) {
		t.Fatalf("postfix stops: %v", stops)
	}
	plan = planFor(t, db, "mariadb", "", installed, removalHostFacts{})
	if len(plan.Blockers) != 0 || !slices.Equal(plan.Together, []string{"phpmyadmin"}) {
		t.Fatalf("mariadb: %+v", plan)
	}
	// Exim follows Postfix's rules: a mailbox refuses it.
	if _, err := db.Exec(`INSERT INTO email_accounts VALUES (2, 100, 'a@php83.example')`); err != nil {
		t.Fatal(err)
	}
	plan = planFor(t, db, "exim", "", map[string]bool{"exim": true}, removalHostFacts{})
	if !slices.Equal(blockerKinds(plan), []string{core.UseMailAddresses}) || plan.Blockers[0].Items[0].Link != "/domains/php83.example?tab=mail" {
		t.Fatalf("exim: %+v", plan.Blockers)
	}
}

// Case 7: only sites recorded on Apache block Apache. Cases 8 and 9: vsftpd
// and the caches are confirmations that say what stops.
func TestRemovalPlanApacheFTPAndCaches(t *testing.T) {
	db := newRemovalPlanTestDB(t)
	plan := planFor(t, db, "apache", "", map[string]bool{"apache": true, "nginx": true}, removalHostFacts{})
	if len(plan.Blockers) != 1 || !slices.Equal(itemNames(plan.Blockers[0]), []string{"apacheweb.example (ali)"}) {
		t.Fatalf("apache: %+v", plan.Blockers)
	}
	plan = planFor(t, db, "vsftpd", "", map[string]bool{"vsftpd": true}, removalHostFacts{})
	if len(plan.Blockers) != 0 || len(plan.Stops) != 1 || plan.Stops[0].Kind != core.StopFTP || plan.Stops[0].Count != 7 {
		t.Fatalf("vsftpd: %+v", plan)
	}
	plan = planFor(t, db, "redis", "", map[string]bool{"redis": true}, removalHostFacts{})
	if len(plan.Blockers) != 0 || plan.Stops[0].Kind != core.StopUnknownUse {
		t.Fatalf("redis: %+v", plan)
	}
}

// Offerings of a removed component or version are withdrawn: the plan names
// them, the removal deletes exactly those entries.
func TestRemovalPlanWithdrawsOfferings(t *testing.T) {
	db := newRemovalPlanTestDB(t)
	plan := planFor(t, db, "php-fpm", "8.3", nil, removalHostFacts{})
	if !slices.Equal(blockerKinds(plan), []string{core.UsePHPSites}) || !slices.Equal(plan.Withdraws, []string{"component:php-fpm:8.3"}) || plan.WithdrawSets != 2 {
		t.Fatalf("php 8.3: %+v", plan)
	}
	plan = planFor(t, db, "postgresql", "", map[string]bool{"postgresql": true}, removalHostFacts{})
	if !slices.Equal(plan.Withdraws, []string{"component:postgresql", "component:postgresql:17"}) || plan.WithdrawSets != 2 {
		t.Fatalf("postgresql: %+v", plan)
	}
	n, err := withdrawRemovedOfferings(context.Background(), db, "postgresql", "")
	if err != nil || n != 2 {
		t.Fatalf("withdraw = %d, %v", n, err)
	}
	if n, err = withdrawRemovedOfferings(context.Background(), db, "php-fpm", "8.3"); err != nil || n != 2 {
		t.Fatalf("withdraw php = %d, %v", n, err)
	}
	var left []string
	rows, _ := db.Query(`SELECT item FROM rights_set_items ORDER BY item`)
	for rows.Next() {
		var item string
		_ = rows.Scan(&item)
		left = append(left, item)
	}
	rows.Close()
	if !slices.Equal(left, []string{"component:node:24", "component:php-fpm:8.4"}) {
		t.Fatalf("left = %v", left)
	}
	// A Node.js line stays offered while another version of it is installed.
	view := []ManagedServiceResponse{{ID: "node", Versions: []string{"24.18.0", "24.11.0", "22.1.0"}}}
	if !nodeLineStays(view, "24.11.0") || nodeLineStays(view, "22.1.0") {
		t.Fatal("node line rule wrong")
	}
}

// --- the server-side remove-together operation --------------------------

func (a *serviceOperationTestAgent) UninstallService(req *transport.InstallServiceRequest, resp *transport.UninstallServiceResponse) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.mutationEvents = append(a.mutationEvents, "uninstall:"+req.ID)
	a.installed[req.ID] = false
	for unit := range a.active {
		if svc := core.ServiceForUnit(unit); (svc != nil && svc.ID == req.ID) || unit == req.ID {
			a.active[unit] = false
		}
	}
	resp.Removed = true
	return nil
}

func postServiceUninstall(t *testing.T, f serviceOperationTestFixture, body string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	f.panel.handleServiceUninstall(recorder, serviceOperationAdminRequest(t, http.MethodPost, "/api/v1/service/uninstall", body, f.userID))
	return recorder
}

func uninstallEvents(events []string) []string {
	out := []string{}
	for _, event := range events {
		if strings.HasPrefix(event, "uninstall:") || strings.HasPrefix(event, "begin:") {
			out = append(out, event)
		}
	}
	return out
}

func TestRemoveTogetherRunsOnTheServerDependentsFirst(t *testing.T) {
	f := newServiceOperationTestFixture(t)
	f.agent.installed["postgresql"] = true
	f.agent.installed["phppgadmin"] = true
	f.agent.installed["nginx"] = true
	f.agent.installed["php-fpm"] = true

	// Without the confirmation of what goes together: refused, nothing runs.
	recorder := postServiceUninstall(t, f, `{"service_id":"postgresql"}`)
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), errCodeRemovalNeedsTogether) ||
		!strings.Contains(recorder.Body.String(), "phpPgAdmin") {
		t.Fatalf("unconfirmed: %d %s", recorder.Code, recorder.Body.String())
	}
	if events := uninstallEvents(f.agent.capturedMutationEvents()); len(events) != 0 {
		t.Fatalf("unconfirmed removal ran: %v", events)
	}

	requestID := mustServiceOperationRequestID(t)
	recorder = postServiceUninstall(t, f, `{"service_id":"postgresql","remove_together":["phppgadmin"],"request_id":"`+requestID+`"}`)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("confirmed: %d %s", recorder.Code, recorder.Body.String())
	}
	op := decodeServiceOperationEnvelope(t, recorder)
	if op == nil || op.Kind != serviceOperationKindRemoveTogether || op.PackageName != "phppgadmin" {
		t.Fatalf("operation = %+v", op)
	}
	done, body := waitForServiceOperation(t, f.panel, f.userID, op.ID, serviceOperationSucceeded)
	var result struct {
		Removed []string `json:"removed"`
	}
	if err := json.Unmarshal(done.Result, &result); err != nil || !slices.Equal(result.Removed, []string{"phppgadmin", "postgresql"}) {
		t.Fatalf("result = %s", body)
	}
	if events := uninstallEvents(f.agent.capturedMutationEvents()); !slices.Equal(events,
		[]string{"begin:service_remove_together", "uninstall:phppgadmin", "uninstall:postgresql"}) {
		t.Fatalf("events = %v", events)
	}
}

// Resume: the same run again skips what is already gone; a use that appeared
// meanwhile refuses the rest.
func TestRemoveTogetherResumesAndChecksThePlanAgain(t *testing.T) {
	f := newServiceOperationTestFixture(t)
	f.agent.installed["postgresql"] = true
	ctx := serviceOperationBoundContext()
	advance := func(string) error { return nil }

	result, failure := f.panel.runRemoveTogether(ctx, "postgresql", []string{"phppgadmin"}, nil, advance)
	if failure != nil || !slices.Equal(result["removed"].([]string), []string{"phppgadmin", "postgresql"}) {
		t.Fatalf("resume: %v %+v", result, failure)
	}
	if events := uninstallEvents(f.agent.capturedMutationEvents()); !slices.Equal(events, []string{"uninstall:postgresql"}) {
		t.Fatalf("resume events = %v", events)
	}

	f.agent.installed["wireguard"] = true
	if _, err := f.database.GetDB().Exec(`INSERT INTO vpn_peers (name, public_key, preshared_key, ip) VALUES ('shop-laptop', 'pk', 'psk', '10.8.0.2')`); err != nil {
		t.Fatal(err)
	}
	_, failure = f.panel.runRemoveTogether(ctx, "wireguard", nil, nil, advance)
	if failure == nil || failure.Code != errCodeRemovalBlockedOnResume || !strings.Contains(failure.Message, "shop-laptop") {
		t.Fatalf("blocked resume: %+v", failure)
	}
}
