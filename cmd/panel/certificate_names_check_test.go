package main

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The request for a free certificate is refused, before anything is touched,
// when public DNS shows a requested name somewhere else; it goes ahead when the
// names point here; and a lookup that failed (unknown) never refuses. The
// resolvers are fakes: no test here asks real DNS or a certificate authority.

const certNamesServerIP = "203.0.113.7"

// byNameResolver answers per name: an address list, "nxdomain", or an error.
type byNameResolver map[string][]string

func (r byNameResolver) LookupNS(context.Context, string) ([]*net.NS, error) {
	return nil, errors.New("not asked")
}

func (r byNameResolver) LookupHost(_ context.Context, name string) ([]string, error) {
	answer, ok := r[name]
	if !ok {
		return nil, errors.New("i/o timeout")
	}
	if len(answer) == 1 && answer[0] == "nxdomain" {
		return nil, &net.DNSError{Err: "no such host", Name: name, IsNotFound: true}
	}
	return answer, nil
}

// withCertificateDNS installs fake resolvers and this server's address for one
// test.
func withCertificateDNS(t *testing.T, serverIPs []string, resolvers ...byNameResolver) {
	t.Helper()
	previousResolvers, previousServer := certificateNameResolvers, certificateServerAddresses
	t.Cleanup(func() { certificateNameResolvers, certificateServerAddresses = previousResolvers, previousServer })
	certificateServerAddresses = func() []string { return serverIPs }
	certificateNameResolvers = func() []namedConnectionResolver {
		out := make([]namedConnectionResolver, 0, len(resolvers))
		for i, r := range resolvers {
			out = append(out, namedConnectionResolver{Name: string(rune('A' + i)), Resolver: r})
		}
		return out
	}
}

func certNameStates(checks []certificateNameCheck) map[string]string {
	out := map[string]string{}
	for _, c := range checks {
		out[c.Name] = c.State
	}
	return out
}

func TestCheckCertificateNamesSeparatesHereElsewhereAndUnknown(t *testing.T) {
	withCertificateDNS(t, []string{certNamesServerIP},
		byNameResolver{
			"example.test":      {certNamesServerIP},
			"www.example.test":  {"198.51.100.9"},
			"mail.example.test": {"nxdomain"},
		},
		byNameResolver{
			"example.test":     {"198.51.100.9"}, // one cache still behind: one view showing here is enough
			"www.example.test": {"198.51.100.9"},
		},
	)
	checks := checkCertificateNames(context.Background(), []string{"example.test", "www.example.test"}, "mail.example.test")
	states := certNameStates(checks)
	if states["example.test"] != certificateNameHere {
		t.Errorf("example.test = %q, want here", states["example.test"])
	}
	if states["www.example.test"] != certificateNameElsewhere {
		t.Errorf("www = %q, want elsewhere", states["www.example.test"])
	}
	if states["mail.example.test"] != certificateNameElsewhere {
		t.Errorf("mail (NXDOMAIN is an answer) = %q, want elsewhere", states["mail.example.test"])
	}
	if checks[2].Kind != certificateNameKindMail || checks[0].Kind != certificateNameKindSite {
		t.Errorf("kinds = %#v", checks)
	}

	// No resolver answered, or this server's address is not known: unknown.
	withCertificateDNS(t, []string{certNamesServerIP}, byNameResolver{})
	if got := certNameStates(checkCertificateNames(context.Background(), []string{"example.test"}, ""))["example.test"]; got != certificateNameUnknown {
		t.Errorf("all lookups failed = %q, want unknown", got)
	}
	withCertificateDNS(t, nil, byNameResolver{"example.test": {"198.51.100.9"}})
	if got := certNameStates(checkCertificateNames(context.Background(), []string{"example.test"}, ""))["example.test"]; got != certificateNameUnknown {
		t.Errorf("no server address = %q, want unknown", got)
	}
}

func TestCertificateRequestRefusedWhenNamesDoNotPointHere(t *testing.T) {
	p, domainID := newIncludeMailFixture(t, "example.test")
	agent := &siteCertAgent{}
	attachSiteCertAgent(t, p, agent)
	userID := siteCertUserID(t, p)
	ctx := context.Background()
	if err := p.setSetting(ctx, settingNS1, "ns1.celikhost.test"); err != nil {
		t.Fatal(err)
	}
	if err := p.setSetting(ctx, settingNS2, "ns2.celikhost.test"); err != nil {
		t.Fatal(err)
	}
	withCertificateDNS(t, []string{certNamesServerIP}, byNameResolver{
		"example.test":     {"198.51.100.9"},
		"www.example.test": {"198.51.100.9"},
	})

	recorder := httptest.NewRecorder()
	p.handleIssueLetsEncrypt(recorder, siteCertRequest(domainID, userID, siteCertDefaultBody))
	var refusal apiErrorBody
	if err := json.Unmarshal(recorder.Body.Bytes(), &refusal); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusConflict || refusal.Code != errCodeCertificateDomainNotPointed || refusal.Reason != "" {
		t.Fatalf("answer = %d %+v", recorder.Code, refusal)
	}
	if refusal.Vars["names"] != "example.test, www.example.test" || refusal.Vars["ip"] != certNamesServerIP ||
		refusal.Vars["ns1"] != "ns1.celikhost.test" || refusal.Vars["ns2"] != "ns2.celikhost.test" {
		t.Fatalf("vars = %#v", refusal.Vars)
	}
	// D-024: the reason, that nothing changed, who acts and what to do.
	for _, part := range []string{"does not point to this server", "nothing was changed", "set the nameservers to ns1.celikhost.test and ns2.celikhost.test", certNamesServerIP, "again"} {
		if !strings.Contains(refusal.Error, part) {
			t.Errorf("the refusal does not say %q: %q", part, refusal.Error)
		}
	}
	if got := agent.askedEmails(); len(got) != 0 {
		t.Fatalf("certificate agent was asked %d times for a refused request", len(got))
	}
	agent.includeMailRPCAgent.mu.Lock()
	applied := len(agent.includeMailRPCAgent.applyCalls)
	agent.includeMailRPCAgent.mu.Unlock()
	if applied != 0 {
		t.Fatalf("web server configuration was touched %d times for a refused request", applied)
	}
	if enabled, active := siteCertSiteState(t, p, domainID); enabled || active != 0 {
		t.Fatalf("refused request changed the site: ssl_enabled=%v active=%d", enabled, active)
	}
}

func TestCertificateRequestRefusalNamesOnlyTheMailNameWhenOnlyItFails(t *testing.T) {
	p, domainID := newIncludeMailFixture(t, "example.test")
	agent := &siteCertAgent{}
	attachSiteCertAgent(t, p, agent)
	userID := siteCertUserID(t, p)
	withCertificateDNS(t, []string{certNamesServerIP}, byNameResolver{
		"example.test":      {certNamesServerIP},
		"www.example.test":  {certNamesServerIP},
		"mail.example.test": {"198.51.100.9"},
	})

	withMail := httptest.NewRecorder()
	p.handleIssueLetsEncrypt(withMail, siteCertRequest(domainID, userID,
		`{"auto_renew":true,"include_mail":true,"provider":"letsencrypt","reissue":false}`))
	var refusal apiErrorBody
	if err := json.Unmarshal(withMail.Body.Bytes(), &refusal); err != nil {
		t.Fatal(err)
	}
	if withMail.Code != http.StatusConflict || refusal.Code != errCodeCertificateDomainNotPointed ||
		refusal.Reason != certificateNotPointedReasonMail || refusal.Vars["names"] != "mail.example.test" {
		t.Fatalf("answer = %d %+v", withMail.Code, refusal)
	}
	if !strings.Contains(refusal.Error, "without the mail name") {
		t.Errorf("the refusal does not offer the request without the mail name: %q", refusal.Error)
	}
	if got := agent.askedEmails(); len(got) != 0 {
		t.Fatalf("certificate agent was asked %d times", len(got))
	}

	// The same domain without the mail name goes ahead.
	without := httptest.NewRecorder()
	p.handleIssueLetsEncrypt(without, siteCertRequest(domainID, userID, siteCertDefaultBody))
	if without.Code != http.StatusOK {
		t.Fatalf("without the mail name: status = %d, body=%s", without.Code, without.Body.String())
	}
}

func TestCertificateRequestGoesAheadWhenNamesPointHereOrAreUnknown(t *testing.T) {
	for name, resolvers := range map[string][]byNameResolver{
		"pointing here": {{
			"example.test":     {certNamesServerIP},
			"www.example.test": {certNamesServerIP},
		}},
		// Every lookup failed: not evidence that the domain is elsewhere.
		"lookup failed": {{}},
		// One name known to be here, the other not answered: no known negative.
		"partly unknown": {{"example.test": {certNamesServerIP}}},
	} {
		t.Run(name, func(t *testing.T) {
			p, domainID := newIncludeMailFixture(t, "example.test")
			agent := &siteCertAgent{}
			attachSiteCertAgent(t, p, agent)
			userID := siteCertUserID(t, p)
			withCertificateDNS(t, []string{certNamesServerIP}, resolvers...)

			recorder := httptest.NewRecorder()
			p.handleIssueLetsEncrypt(recorder, siteCertRequest(domainID, userID, siteCertDefaultBody))
			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, body=%s", recorder.Code, recorder.Body.String())
			}
			if got := agent.askedEmails(); len(got) != 1 {
				t.Fatalf("certificate agent was asked %d times, want 1", len(got))
			}
		})
	}
}

func TestDomainConnectionAnswersTheCertificateNamesOnlyWhenAsked(t *testing.T) {
	p, domainID := newIncludeMailFixture(t, "example.test")
	withCertificateDNS(t, []string{certNamesServerIP}, byNameResolver{
		"example.test":     {certNamesServerIP},
		"www.example.test": {"198.51.100.9"},
	})
	// The Overview's own lookups and the nameserver check ask nobody either:
	// the public resolvers are fakes, and nameservers under the domain are glue.
	previousPublic := publicConnectionResolvers
	t.Cleanup(func() { publicConnectionResolvers = previousPublic })
	publicConnectionResolvers = []namedConnectionResolver{{Name: "fake", Resolver: fakeConnectionDNSResolver{
		nameservers: nsRecords("ns1.example.test"), addresses: []string{certNamesServerIP},
	}}}
	for key, value := range map[string]string{settingNS1: "ns1.example.test", settingNS2: "ns2.example.test"} {
		if err := p.setSetting(context.Background(), key, value); err != nil {
			t.Fatal(err)
		}
	}
	read := func(query string) connectionCheck {
		recorder := httptest.NewRecorder()
		p.handleDomainConnection(recorder, httptest.NewRequest(http.MethodGet,
			"/api/v1/domains/1/connection"+query, nil), domainID)
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, body=%s", recorder.Code, recorder.Body.String())
		}
		var out connectionCheck
		if err := json.Unmarshal(recorder.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	if plain := read(""); len(plain.CertificateNames) != 0 {
		t.Fatalf("the Overview's read carries certificate names: %#v", plain.CertificateNames)
	}
	asked := read("?names=certificate")
	states := certNameStates(asked.CertificateNames)
	if len(asked.CertificateNames) != 3 || states["example.test"] != certificateNameHere ||
		states["www.example.test"] != certificateNameElsewhere || states["mail.example.test"] != certificateNameUnknown {
		t.Fatalf("certificate names = %#v", asked.CertificateNames)
	}
}
