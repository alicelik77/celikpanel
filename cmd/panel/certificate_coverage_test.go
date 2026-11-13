package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/core"
)

// The coverage list as a table: names × certificates × DNS answers → each
// line's state, certificate, reason and action (docs/CERTIFICATE-COVERAGE.md).
func TestCertificateCoverageTable(t *testing.T) {
	until := time.Date(2027, 1, 12, 0, 0, 0, 0, time.UTC)
	serverUntil := time.Date(2027, 2, 1, 0, 0, 0, 0, time.UTC)
	here := func(name string) certificateNameCheck {
		return certificateNameCheck{Name: name, State: certificateNameHere, IPs: []string{"192.0.2.10"}}
	}
	elsewhere := func(name string, ips ...string) certificateNameCheck {
		return certificateNameCheck{Name: name, State: certificateNameElsewhere, IPs: ips}
	}
	unknown := func(name string) certificateNameCheck {
		return certificateNameCheck{Name: name, State: certificateNameUnknown}
	}
	dns := func(checks ...certificateNameCheck) map[string]certificateNameCheck {
		out := map[string]certificateNameCheck{}
		for _, check := range checks {
			out[check.Name] = check
		}
		return out
	}
	site := []string{"example.com", "www.example.com"}
	cert := func(serves bool, names ...string) *coverageCertificate {
		return &coverageCertificate{Names: names, ExpiresAt: until, WebUsable: true, Trusted: true, ServesMail: serves}
	}
	mailBase := func(in coverageInput) coverageInput {
		in.MailHere = true
		in.ServerMailHost = "mail.server.example"
		if in.ToldHost == "" {
			in.ToldHost = "mail.server.example"
		}
		if in.ServerMail.State == "" {
			in.ServerMail = coverageServerMail{State: "ready", ExpiresAt: serverUntil}
		}
		return in
	}
	type want struct{ kind, name, state, by, reason, action string }
	for _, tc := range []struct {
		name string
		in   coverageInput
		want []want
	}{
		{
			name: "no certificate, no mail here",
			in:   coverageInput{Domain: "example.com", SiteNames: site, CertificateKnown: true},
			want: []want{
				{"site", "example.com", "not_protected", "", "no_certificate", ""},
				{"site", "www.example.com", "not_protected", "", "no_certificate", ""},
			},
		},
		{
			name: "the owner's case: site covered, mail name points here but is not in the certificate",
			in: mailBase(coverageInput{Domain: "example.com", SiteNames: site, CertificateKnown: true,
				Certificate: cert(false, "example.com", "www.example.com", "autoconfig.example.com", "autodiscover.example.com"),
				SetupNames:  []string{"autoconfig.example.com", "autodiscover.example.com"},
				DNS:         dns(here("mail.example.com"))}),
			want: []want{
				{"site", "example.com", "protected", "domain_certificate", "", ""},
				{"site", "www.example.com", "protected", "domain_certificate", "", ""},
				{"mail_programs", "mail.server.example", "protected", "server_mail_certificate", "", ""},
				{"mail_name", "mail.example.com", "not_protected", "", "not_in_certificate", "cover_mail"},
				{"mail_setup", "autoconfig.example.com", "protected", "domain_certificate", "", ""},
				{"mail_setup", "autodiscover.example.com", "protected", "domain_certificate", "", ""},
			},
		},
		{
			name: "mail name in the certificate and served",
			in: mailBase(coverageInput{Domain: "example.com", SiteNames: site, CertificateKnown: true,
				Certificate: cert(true, "example.com", "www.example.com", "mail.example.com"),
				DNS:         dns(here("mail.example.com"))}),
			want: []want{
				{"site", "example.com", "protected", "domain_certificate", "", ""},
				{"site", "www.example.com", "protected", "domain_certificate", "", ""},
				{"mail_programs", "mail.server.example", "protected", "server_mail_certificate", "", ""},
				{"mail_name", "mail.example.com", "protected", "domain_certificate", "", ""},
			},
		},
		{
			name: "mail name in the certificate, mail does not use it yet",
			in: mailBase(coverageInput{Domain: "example.com", SiteNames: site[:1], CertificateKnown: true,
				Certificate: cert(false, "example.com", "mail.example.com"), DNS: dns(here("mail.example.com"))}),
			want: []want{
				{"site", "example.com", "protected", "domain_certificate", "", ""},
				{"mail_programs", "mail.server.example", "protected", "server_mail_certificate", "", ""},
				{"mail_name", "mail.example.com", "not_protected", "", "mail_not_using", "use_for_mail"},
			},
		},
		{
			name: "mail name points elsewhere, DNS here: its addresses and the DNS action",
			in: mailBase(coverageInput{Domain: "example.com", SiteNames: site[:1], CertificateKnown: true, DNSHere: true,
				Certificate: cert(false, "example.com"), DNS: dns(elsewhere("mail.example.com", "198.51.100.7"))}),
			want: []want{
				{"site", "example.com", "protected", "domain_certificate", "", ""},
				{"mail_programs", "mail.server.example", "protected", "server_mail_certificate", "", ""},
				{"mail_name", "mail.example.com", "not_protected", "", "points_elsewhere", "dns"},
			},
		},
		{
			name: "mail name points elsewhere, DNS run elsewhere: no action here",
			in: mailBase(coverageInput{Domain: "example.com", SiteNames: site[:1], CertificateKnown: true,
				Certificate: cert(false, "example.com"), DNS: dns(elsewhere("mail.example.com", "198.51.100.7"))}),
			want: []want{
				{"site", "example.com", "protected", "domain_certificate", "", ""},
				{"mail_programs", "mail.server.example", "protected", "server_mail_certificate", "", ""},
				{"mail_name", "mail.example.com", "not_protected", "", "points_elsewhere", "recheck"},
			},
		},
		{
			name: "a domain without the mail name has no line for it",
			in: mailBase(coverageInput{Domain: "example.com", SiteNames: site[:1], CertificateKnown: true,
				Certificate: cert(false, "example.com"), DNS: dns(elsewhere("mail.example.com"))}),
			want: []want{
				{"site", "example.com", "protected", "domain_certificate", "", ""},
				{"mail_programs", "mail.server.example", "protected", "server_mail_certificate", "", ""},
			},
		},
		{
			name: "mail name not checked: said, never guessed",
			in: mailBase(coverageInput{Domain: "example.com", SiteNames: site[:1], CertificateKnown: true,
				Certificate: cert(false, "example.com"), DNS: dns(unknown("mail.example.com"))}),
			want: []want{
				{"site", "example.com", "protected", "domain_certificate", "", ""},
				{"mail_programs", "mail.server.example", "protected", "server_mail_certificate", "", ""},
				{"mail_name", "mail.example.com", "unknown", "", "not_checked", ""},
			},
		},
		{
			name: "no certificate, mail here, mail name points here",
			in: mailBase(coverageInput{Domain: "example.com", SiteNames: site[:1], CertificateKnown: true,
				DNS: dns(here("mail.example.com"))}),
			want: []want{
				{"site", "example.com", "not_protected", "", "no_certificate", ""},
				{"mail_programs", "mail.server.example", "protected", "server_mail_certificate", "", ""},
				{"mail_name", "mail.example.com", "not_protected", "", "no_certificate", ""},
			},
		},
		{
			name: "an alias not in the certificate: points here → request again; elsewhere → DNS",
			in: coverageInput{Domain: "example.com", SiteNames: []string{"example.com", "shop.example.net", "old.example.net"},
				CertificateKnown: true, DNSHere: true, Certificate: cert(false, "example.com"),
				DNS: dns(here("shop.example.net"), elsewhere("old.example.net", "203.0.113.5"))},
			want: []want{
				{"site", "example.com", "protected", "domain_certificate", "", ""},
				{"site", "shop.example.net", "not_protected", "", "not_in_certificate", "reissue"},
				{"site", "old.example.net", "not_protected", "", "points_elsewhere", "dns"},
			},
		},
		{
			name: "an uploaded certificate is replaced by upload, not requested",
			in: mailBase(coverageInput{Domain: "example.com", SiteNames: site, CertificateKnown: true,
				Certificate: &coverageCertificate{Names: []string{"example.com"}, ExpiresAt: until, WebUsable: true, Trusted: true, Uploaded: true},
				DNS:         dns(here("www.example.com"), here("mail.example.com"))}),
			want: []want{
				{"site", "example.com", "protected", "domain_certificate", "", ""},
				{"site", "www.example.com", "not_protected", "", "not_in_certificate", "upload"},
				{"mail_programs", "mail.server.example", "protected", "server_mail_certificate", "", ""},
				{"mail_name", "mail.example.com", "not_protected", "", "not_in_certificate", "upload"},
			},
		},
		{
			name: "an expired certificate protects nothing",
			in: mailBase(coverageInput{Domain: "example.com", SiteNames: site[:1], CertificateKnown: true,
				Certificate: &coverageCertificate{Names: []string{"example.com", "mail.example.com"}, ServesMail: true},
				DNS:         dns(here("mail.example.com"))}),
			want: []want{
				{"site", "example.com", "not_protected", "", "certificate_not_usable", ""},
				{"mail_programs", "mail.server.example", "protected", "server_mail_certificate", "", ""},
				{"mail_name", "mail.example.com", "not_protected", "", "certificate_not_usable", ""},
			},
		},
		{
			name: "a domain that chose its own mail name: programs are told it and it is the one line",
			in: mailBase(coverageInput{Domain: "example.com", SiteNames: site[:1], CertificateKnown: true, ToldHost: "mail.example.com",
				Certificate: cert(true, "example.com", "mail.example.com"), DNS: dns(here("mail.example.com"))}),
			want: []want{
				{"site", "example.com", "protected", "domain_certificate", "", ""},
				{"mail_programs", "mail.example.com", "protected", "domain_certificate", "", ""},
			},
		},
		{
			name: "server mail certificate not ready; web mail answers without one",
			in: mailBase(coverageInput{Domain: "example.com", CertificateKnown: true,
				ServerMail: coverageServerMail{State: "pending"},
				Webmail:    &coverageWebmail{Host: "webmail.server.example", Secure: "no"},
				DNS:        dns(elsewhere("mail.example.com"))}),
			want: []want{
				{"webmail", "webmail.server.example", "not_protected", "", "webmail_no_certificate", ""},
				{"mail_programs", "mail.server.example", "not_protected", "", "server_certificate_not_ready", ""},
			},
		},
		{
			name: "web mail with its own certificate; the server's mail name unknown",
			in: coverageInput{Domain: "example.com", CertificateKnown: true, MailHere: true,
				Webmail: &coverageWebmail{Host: "webmail.server.example", Secure: "yes", ExpiresAt: serverUntil},
				DNS:     dns(elsewhere("mail.example.com"))},
			want: []want{
				{"webmail", "webmail.server.example", "protected", "webmail_certificate", "", ""},
				{"mail_programs", "", "not_protected", "", "mail_name_unknown", ""},
			},
		},
		{
			name: "auto-setup names: one has no address (the mail screen adds it), one points elsewhere",
			in: mailBase(coverageInput{Domain: "example.com", CertificateKnown: true, DNSHere: true,
				Certificate: cert(false, "example.com"),
				SetupNames:  []string{"autoconfig.example.com", "autodiscover.example.com"},
				DNS:         dns(elsewhere("mail.example.com"), elsewhere("autoconfig.example.com"), elsewhere("autodiscover.example.com", "198.51.100.9"))}),
			want: []want{
				{"mail_programs", "mail.server.example", "protected", "server_mail_certificate", "", ""},
				{"mail_setup", "autoconfig.example.com", "not_protected", "", "no_address", "mail_setup"},
				{"mail_setup", "autodiscover.example.com", "not_protected", "", "points_elsewhere", "dns"},
			},
		},
		{
			// An unmet prerequisite, not a failure: with the zone run
			// elsewhere the one action looks at the public DNS again; no
			// request is offered for a name that does not point here.
			name: "names that do not point here, DNS run elsewhere: look again, never request",
			in: coverageInput{Domain: "example.com", SiteNames: []string{"example.com", "new.example.com", "old.example.com"},
				CertificateKnown: true, Certificate: cert(false, "example.com"),
				DNS: dns(elsewhere("new.example.com"), elsewhere("old.example.com", "203.0.113.5"))},
			want: []want{
				{"site", "example.com", "protected", "domain_certificate", "", ""},
				{"site", "new.example.com", "not_protected", "", "no_address", "recheck"},
				{"site", "old.example.com", "not_protected", "", "points_elsewhere", "recheck"},
			},
		},
		{
			name: "the certificate could not be read: not known, never 'not protected'",
			in: mailBase(coverageInput{Domain: "example.com", SiteNames: site[:1], CertificateKnown: false,
				ServerMail: coverageServerMail{State: "unknown"}, DNS: dns(here("mail.example.com"))}),
			want: []want{
				{"site", "example.com", "unknown", "", "not_checked", ""},
				{"mail_programs", "mail.server.example", "unknown", "", "not_checked", ""},
				{"mail_name", "mail.example.com", "unknown", "", "not_checked", ""},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lines := certificateCoverage(tc.in)
			if len(lines) != len(tc.want) {
				t.Fatalf("lines = %+v, want %d", lines, len(tc.want))
			}
			for index, line := range lines {
				w := tc.want[index]
				got := want{line.Kind, line.Name, line.State, line.By, line.Reason, line.Action}
				if got != w {
					t.Fatalf("line %d = %+v, want %+v", index, got, w)
				}
				switch {
				case line.State == coverageProtected && line.Until == "":
					t.Fatalf("line %d is protected without an end: %+v", index, line)
				case line.State != coverageProtected && (line.Until != "" || line.By != ""):
					t.Fatalf("line %d names a certificate it is not protected by: %+v", index, line)
				case line.State != coverageProtected && line.Reason == "":
					t.Fatalf("line %d has no reason: %+v", index, line)
				}
				if line.Reason == coverageReasonPointsElsewhere && len(line.IPs) == 0 {
					t.Fatalf("line %d does not say where the name points: %+v", index, line)
				}
			}
		})
	}
}

// The endpoint reads only: a domain with an uploaded certificate the Agent
// cannot read now answers every certificate line "not known", and nothing is
// changed. Team members see it with the SSL view right.
func TestCertificateCoverageEndpointReadsOnly(t *testing.T) {
	database, err := openTemplatePanelDatabase(filepath.Join(t.TempDir(), "panel.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Close)
	saved, savedResolvers, savedAddresses := mailAutoconfigMailHere, certificateNameResolvers, certificateServerAddresses
	t.Cleanup(func() {
		mailAutoconfigMailHere, certificateNameResolvers, certificateServerAddresses = saved, savedResolvers, savedAddresses
	})
	mailAutoconfigMailHere = func(*Panel, context.Context) bool { return false }
	certificateNameResolvers = func() []namedConnectionResolver { return nil }
	certificateServerAddresses = func() []string { return []string{"2001:db8::10", "192.0.2.10"} }
	p := &Panel{db: database}
	for _, stmt := range []string{
		`INSERT INTO users (id, username, password_hash, email, role) VALUES (9602, 'cust1', 'x', 'cust1@example.net', 'customer')`,
		`INSERT INTO subscriptions (id, owner_id, name) VALUES (9701, 9602, 'cust1-sub')`,
		`INSERT INTO domains (id, subscription_id, name) VALUES (9801, 9701, 'example.com')`,
		`INSERT INTO sites (domain_id, document_root, project_type) VALUES (9801, '/srv/a', 'php')`,
	} {
		if _, err := database.GetDB().Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	recorder := httptest.NewRecorder()
	p.handleCertificateCoverage(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/domains/9801/ssl/coverage", nil), 9801)
	body := recorder.Body.String()
	if recorder.Code != http.StatusOK || !strings.Contains(body, `"name":"example.com","state":"not_protected","reason":"no_certificate"`) ||
		!strings.Contains(body, `"mail_name":"mail.example.com"`) || strings.Contains(body, "mail_programs") ||
		!strings.Contains(body, `"server_ip":"192.0.2.10"`) {
		t.Fatalf("coverage = %d %s", recorder.Code, body)
	}
	recorder = httptest.NewRecorder()
	p.handleCertificateCoverage(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/domains/9801/ssl/coverage", nil), 9801)
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST = %d", recorder.Code)
	}
	if rule, ok := teamMemberDomainRequirements[teamMemberRouteKey{kind: "ssl-coverage", method: http.MethodGet}]; !ok ||
		rule.mode != core.TeamPermissionView || rule.capability != core.TeamCapabilitySSL {
		t.Fatalf("team member rule = %+v %v", rule, ok)
	}
}
