package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

type autoconfigFixture struct {
	p *Panel
}

func newAutoconfigFixture(t *testing.T, mailHere bool) *autoconfigFixture {
	t.Helper()
	database, err := openTemplatePanelDatabase(filepath.Join(t.TempDir(), "panel.sqlite"))
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(database.Close)
	saved := mailAutoconfigMailHere
	t.Cleanup(func() { mailAutoconfigMailHere = saved })
	mailAutoconfigMailHere = func(*Panel, context.Context) bool { return mailHere }
	f := &autoconfigFixture{p: &Panel{db: database}}
	for _, stmt := range []string{
		`INSERT INTO users (id, username, password_hash, email, role) VALUES (9602, 'cust1', 'x', 'cust1@example.net', 'customer')`,
		`INSERT INTO subscriptions (id, owner_id, name) VALUES (9701, 9602, 'cust1-sub')`,
		`INSERT INTO domains (id, subscription_id, name) VALUES (9801, 9701, 'example.com')`,
		`INSERT INTO domains (id, subscription_id, name, parent_domain_id) VALUES (9802, 9701, 'shop.example.com', 9801)`,
		`INSERT INTO domains (id, subscription_id, name) VALUES (9803, 9701, 'nosite.example')`,
		`INSERT INTO sites (domain_id, document_root, project_type) VALUES (9801, '/srv/a', 'php')`,
		`INSERT INTO sites (domain_id, document_root, project_type) VALUES (9802, '/srv/b', 'php')`,
		`INSERT INTO panel_settings (key, value) VALUES ('mail_hostname', 'mail.server.example')`,
	} {
		if _, err := database.GetDB().Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	return f
}

func TestMailAutoconfigOnlyWhereTheDomainsMailIsHere(t *testing.T) {
	f := newAutoconfigFixture(t, true)
	ctx := context.Background()
	for id, want := range map[int]string{9802: mailAutoconfigReasonSubdomain, 9803: mailAutoconfigReasonNoSite} {
		plan, err := f.p.mailAutoconfigFor(ctx, id)
		if err != nil || plan.available || plan.reason != want {
			t.Fatalf("domain %d: %+v %v", id, plan, err)
		}
	}
	plan, err := f.p.mailAutoconfigFor(ctx, 9801)
	if err != nil || !plan.available || plan.mailHost != "mail.server.example" || strings.Join(plan.names, ",") != "autoconfig.example.com,autodiscover.example.com" {
		t.Fatalf("plan = %+v %v", plan, err)
	}
	// A certificate that carries mail.<domain> for mail makes it the name.
	if _, err := f.p.db.GetDB().Exec(`INSERT INTO ssl_certificates (domain_id, type, cert_path, key_path, expires_at, status, secure_mail) VALUES (9801, 'letsencrypt', '/c', '/k', '2099-01-01T00:00:00Z', 'active', 1)`); err != nil {
		t.Fatal(err)
	}
	if plan, _ := f.p.mailAutoconfigFor(ctx, 9801); plan.mailHost != "mail.example.com" {
		t.Fatalf("secure mail host = %q", plan.mailHost)
	}
	// A name another domain or alias holds is never taken over.
	if _, err := f.p.db.GetDB().Exec(`INSERT INTO domain_aliases (domain_id, alias) VALUES (9803, 'autodiscover.example.com')`); err != nil {
		t.Fatal(err)
	}
	if plan, _ := f.p.mailAutoconfigFor(ctx, 9801); plan.available || plan.reason != mailAutoconfigReasonNameTaken {
		t.Fatalf("taken = %+v", plan)
	}
	if req := f.p.mailAutoconfigRequest(ctx, 9801, nil); req != nil {
		t.Fatalf("request for a taken name = %+v", req)
	}
}

func TestMailAutoconfigIsNotOfferedWithoutMailHere(t *testing.T) {
	f := newAutoconfigFixture(t, false)
	plan, err := f.p.mailAutoconfigFor(context.Background(), 9801)
	if err != nil || plan.available || plan.reason != mailAutoconfigReasonMailNotHere {
		t.Fatalf("plan = %+v %v", plan, err)
	}
	if req := f.p.mailAutoconfigRequest(context.Background(), 9801, []string{"example.com"}); req != nil {
		t.Fatal("a site of a domain whose mail is elsewhere answers mail set-up")
	}
}

func TestMailAutoconfigRequestUsesHTTPSOnlyWhenTheCertificateCoversBothNames(t *testing.T) {
	f := newAutoconfigFixture(t, true)
	ctx := context.Background()
	for _, tc := range []struct {
		names []string
		tls   bool
	}{
		{nil, false},
		{[]string{"example.com", "www.example.com"}, false},
		{[]string{"example.com", "autoconfig.example.com"}, false},
		{[]string{"example.com", "autoconfig.example.com", "autodiscover.example.com"}, true},
		{[]string{"*.example.com"}, true},
	} {
		req := f.p.mailAutoconfigRequest(ctx, 9801, tc.names)
		if req == nil || req.TLS != tc.tls || req.MailHost != "mail.server.example" {
			t.Fatalf("%v: %+v", tc.names, req)
		}
	}
}

func TestMailAutoconfigRecordsOfANewZone(t *testing.T) {
	records := mailAutoconfigRecords("example.com", "mail.example.com")
	got := []string{}
	for _, r := range records {
		got = append(got, r.Name+" "+r.Type+" "+r.Content)
	}
	want := "autoconfig.example.com CNAME mail.example.com|autodiscover.example.com CNAME mail.example.com|_autodiscover._tcp.example.com SRV 0 443 autodiscover.example.com"
	if strings.Join(got, "|") != want {
		t.Fatalf("records = %v", got)
	}
}

func TestMailAutoconfigZoneStateNeverReplacesAnotherRecord(t *testing.T) {
	f := newAutoconfigFixture(t, true)
	db := f.p.db.GetDB()
	result, err := db.Exec(`INSERT INTO pdns_domains (name, type) VALUES ('example.com', 'NATIVE')`)
	if err != nil {
		t.Fatal(err)
	}
	zoneID, _ := result.LastInsertId()
	for _, stmt := range []string{
		`INSERT INTO pdns_records (domain_id, name, type, content, ttl) VALUES (?, 'mail.example.com', 'A', '192.0.2.10', 3600)`,
		`INSERT INTO pdns_records (domain_id, name, type, content, ttl) VALUES (?, 'autoconfig.example.com', 'CNAME', 'mail.example.com', 3600)`,
		`INSERT INTO pdns_records (domain_id, name, type, content, ttl) VALUES (?, 'autodiscover.example.com', 'A', '198.51.100.7', 3600)`,
	} {
		if _, err := db.Exec(stmt, zoneID); err != nil {
			t.Fatal(err)
		}
	}
	ctx := context.Background()
	target := f.p.mailAutoconfigTarget(ctx, int(zoneID), "example.com", "mail.server.example")
	if target != "mail.example.com" {
		t.Fatalf("target = %q", target)
	}
	records, err := f.p.mailAutoconfigZoneState(ctx, int(zoneID), mailAutoconfigRecords("example.com", target))
	if err != nil {
		t.Fatal(err)
	}
	states := []string{}
	for _, r := range records {
		states = append(states, r.State)
	}
	if strings.Join(states, ",") != "present,different,missing" {
		t.Fatalf("states = %v", states)
	}
}

func TestMailSetupAnswerCarriesTheAutoconfigStateAndTheSameMailHost(t *testing.T) {
	f := newAutoconfigFixture(t, true)
	savedHTTP, savedTLS := mailAutoconfigProbeHTTP, mailAutoconfigProbeTLS
	t.Cleanup(func() { mailAutoconfigProbeHTTP, mailAutoconfigProbeTLS = savedHTTP, savedTLS })
	mailAutoconfigProbeHTTP = func(context.Context, string) (bool, error) { return true, nil }
	mailAutoconfigProbeTLS = func(_ context.Context, host string) (bool, error) { return host == "autoconfig.example.com", nil }
	for _, stmt := range []string{
		`INSERT INTO domains (id, subscription_id, name, dns_management) VALUES (9804, 9701, 'ext.example', 'external')`,
		`INSERT INTO sites (domain_id, document_root, project_type) VALUES (9804, '/srv/e', 'php')`,
	} {
		if _, err := f.p.db.GetDB().Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	mailAutoconfigProbeTLS = func(_ context.Context, host string) (bool, error) { return host == "autoconfig.ext.example", nil }
	view := f.p.mailAutoconfigView(context.Background(), 9804, "ext.example")
	if view.State != mailAutoconfigPartial || view.Served != "yes" || view.Secure != "no" || view.DNS != "external" || len(view.Records) != 3 {
		t.Fatalf("view = %+v", view)
	}
	recorder := httptest.NewRecorder()
	f.p.handleMailClientSetupWith(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/domains/9804/mail/setup", nil), "ext.example", &view)
	var answer map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil {
		t.Fatal(err)
	}
	if answer["mail_host"] != "mail.server.example" || answer["autoconfig"] == nil {
		t.Fatalf("answer = %s", recorder.Body)
	}
	mailAutoconfigProbeTLS = func(context.Context, string) (bool, error) { return true, nil }
	if view := f.p.mailAutoconfigView(context.Background(), 9804, "ext.example"); view.State != mailAutoconfigOn {
		t.Fatalf("all served = %+v", view)
	}
}

// Lab, alpha.130: "Set up now" answered 404 because the dispatcher's list of
// domain sub-routes did not name mail/autoconfig.
func TestMailAutoconfigRouteReachesTheHandler(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/domains/7/mail/autoconfig", nil)
	match, ok := matchDomainSubroute(request)
	if !ok || match.kind != "mail" || match.domainID != 7 || !routeAllows(http.MethodPost, match.methods) {
		t.Fatalf("POST mail/autoconfig matched %+v, %v", match, ok)
	}
	if routeAllows(http.MethodGet, match.methods) {
		t.Fatalf("GET mail/autoconfig is allowed: %+v", match.methods)
	}
	if _, guarded := requestIdentityRouteFor(request); !guarded {
		t.Fatal("POST mail/autoconfig is not a guarded operation route")
	}
}
