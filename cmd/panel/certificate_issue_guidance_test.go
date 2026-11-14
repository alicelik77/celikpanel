package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/rpc"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.134 lab: a certificate the authority issued whose chain this server
// does not trust answered a bare 500 "internal error". It is now the coded
// refusal with its check, the previous certificate keeps serving, and nothing
// of the new one is installed.

type issuedCheckAgent struct {
	*coverMailAgent
	answer MailTLSInspectRPCResponse
	check  bool
}

func (a *issuedCheckAgent) InspectInstalledCertificate(req *MailTLSInspectRPCRequest, resp *MailTLSInspectRPCResponse) error {
	if !a.check {
		return a.coverMailAgent.InspectInstalledCertificate(req, resp)
	}
	a.issueMu.Lock()
	defer a.issueMu.Unlock()
	*resp = a.answer
	if resp.DNSNames == nil {
		resp.DNSNames = append([]string(nil), a.issued...)
	}
	return nil
}

func attachIssuedCheckAgent(t *testing.T, p *Panel, agent *issuedCheckAgent) {
	t.Helper()
	server := rpc.NewServer()
	if err := server.RegisterName("Agent", agent); err != nil {
		t.Fatal(err)
	}
	connector := func(ctx context.Context) (*rpc.Client, error) {
		serverConn, clientConn := net.Pipe()
		go server.ServeConn(serverConn)
		return rpc.NewClient(clientConn), nil
	}
	client, err := connector(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	p.agentClient = transport.NewReconnectingClientWithContextConnector(client, connector)
	t.Cleanup(func() { _ = client.Close() })
}

func TestIssuedCertificateThatFailsTheCheckIsACodedRefusalAndKeepsThePreviousOne(t *testing.T) {
	for _, tc := range []struct {
		name   string
		answer MailTLSInspectRPCResponse
		check  string
	}{
		{"chain not trusted", MailTLSInspectRPCResponse{Valid: true, TrustChecked: true, TrustError: "x509: certificate signed by unknown authority"}, issuedCheckUntrusted},
		{"trust not checked", MailTLSInspectRPCResponse{Valid: true, Error: "trust store unreadable"}, issuedCheckUnusable},
		{"other names", MailTLSInspectRPCResponse{Valid: true, Trusted: true, TrustChecked: true, DNSNames: []string{"example.test"}}, issuedCheckNames},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, domainID, inner := newCoverMailFixture(t, certificateNameHere)
			agent := &issuedCheckAgent{coverMailAgent: inner, answer: tc.answer}
			attachIssuedCheckAgent(t, p, agent)
			if code, answer := issueCertificateForTest(t, p, domainID, `{"email":"admin@example.test","auto_renew":true,"exclude_mail":true}`); code != http.StatusOK {
				t.Fatalf("first issue = %d %v", code, answer)
			}
			var before string
			if err := p.db.GetDB().QueryRow(`SELECT id || cert_path || secure_mail FROM ssl_certificates WHERE domain_id = ? AND status = 'active'`, domainID).Scan(&before); err != nil {
				t.Fatal(err)
			}
			agent.check = true
			request := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/domains/%d/ssl/letsencrypt", domainID),
				strings.NewReader(`{"email":"admin@example.test","auto_renew":true,"reissue":true}`))
			request = request.WithContext(context.WithValue(request.Context(), callerKey, &Caller{ID: 1, Role: roleAdmin}))
			recorder := httptest.NewRecorder()
			p.handleIssueLetsEncrypt(recorder, request)
			var body apiErrorBody
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatalf("%d %s", recorder.Code, recorder.Body.String())
			}
			if recorder.Code != http.StatusConflict || body.Code != errCodeCertificateIssueFailed ||
				body.Reason != transport.CertificateFailureIssuedCheck || body.Vars["check"] != tc.check ||
				body.Vars["kept"] != "previous" || body.Vars["detail"] == "" {
				t.Fatalf("answer = %d %+v", recorder.Code, body)
			}
			for _, want := range []string{"did not install it and removed it again", "already had is still in place", "Nothing asks again automatically"} {
				if !strings.Contains(body.Error, want) {
					t.Fatalf("sentence lacks %q: %s", want, body.Error)
				}
			}
			var after string
			if err := p.db.GetDB().QueryRow(`SELECT id || cert_path || secure_mail FROM ssl_certificates WHERE domain_id = ? AND status = 'active'`, domainID).Scan(&after); err != nil {
				t.Fatal(err)
			}
			if after != before {
				t.Fatalf("the refused certificate replaced the served one: %q -> %q", before, after)
			}
		})
	}
}

func TestIssuedCertificateCheckFailureTellsACustomerNoServerDetail(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeIssuedCertificateCheckFailure(recorder, &Caller{ID: 2, Role: roleCustomer}, "shop.example", false, issuedCheckUntrusted, "x509: certificate signed by unknown authority")
	var body apiErrorBody
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Vars["detail"] != "" || body.Vars["kept"] != "none" || body.Vars["check"] != issuedCheckUntrusted ||
		!strings.Contains(body.Error, "ca-certificates") || !strings.Contains(body.Error, "has no certificate from this request") {
		t.Fatalf("body = %+v", body)
	}
	// An unknown check is said as the general one, never dropped.
	recorder = httptest.NewRecorder()
	writeIssuedCertificateCheckFailure(recorder, nil, "shop.example", false, "something-new", "")
	body = apiErrorBody{}
	_ = json.Unmarshal(recorder.Body.Bytes(), &body)
	if body.Vars["check"] != issuedCheckUnusable || body.Error == "" {
		t.Fatalf("body = %+v", body)
	}
}
