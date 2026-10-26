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

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.110: webmail on its own address. The check is read only and says
// what DNS answers against this server's addresses; enabling is one durable
// operation the server checks again; the panel's /webmail/ only redirects.

type webmailAddressTestResolver map[string][]string

func (r webmailAddressTestResolver) LookupHost(_ context.Context, name string) ([]string, error) {
	if answer, ok := r[name]; ok {
		if answer == nil {
			return nil, errors.New("i/o timeout")
		}
		return answer, nil
	}
	return nil, &net.DNSError{Err: "DNS name does not exist", Name: name, IsNotFound: true}
}

func stubWebmailAddressWorld(t *testing.T, answers webmailAddressTestResolver, firewall FirewallStatusResp) {
	t.Helper()
	t.Setenv("CELIKPANEL_SERVER_IP", "203.0.113.10")
	t.Setenv("CELIKPANEL_SERVER_IPV6", "2001:db8::10")
	restoreResolvers, restoreFirewall, restoreStatus := webmailAddressResolvers, webmailAddressFirewallStatus, readWebmailAddressStatusFromAgent
	t.Cleanup(func() {
		webmailAddressResolvers, webmailAddressFirewallStatus, readWebmailAddressStatusFromAgent = restoreResolvers, restoreFirewall, restoreStatus
	})
	webmailAddressResolvers = func() []hostResolver { return []hostResolver{answers, answers} }
	webmailAddressFirewallStatus = func(*Panel) (FirewallStatusResp, error) { return firewall, nil }
	readWebmailAddressStatusFromAgent = func(*Panel, context.Context, string, bool) (transport.WebmailAddressStatusResponse, error) {
		return transport.WebmailAddressStatusResponse{}, nil
	}
}

func checkWebmailAddressForTest(t *testing.T, panel *Panel, userID int, host string) webmailAddressCheck {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := serviceStateAtomicRequest(http.MethodPost, webmailAddressCheckPath, `{"host":"`+host+`"}`, userID)
	request.Host = "panel.example.test:2083"
	panel.handleWebmailAddressCheck(recorder, request)
	var body struct {
		Check webmailAddressCheck `json:"check"`
	}
	if recorder.Code != http.StatusOK || json.Unmarshal(recorder.Body.Bytes(), &body) != nil {
		t.Fatalf("check %s: %d %s", host, recorder.Code, recorder.Body.String())
	}
	return body.Check
}

func TestWebmailAddressCheckSaysWhatDNSAnswersAgainstThisServer(t *testing.T) {
	panel, _, database, userID := newServiceStateAtomicPanel(t)
	stubWebmailAddressWorld(t, webmailAddressTestResolver{
		"mail.celikhost.com":     {"203.0.113.10", "2001:db8::10"},
		"webmail.elsewhere.test": {"198.51.100.7"},
		"mail.peer.test":         {"198.51.100.20"},
		"slow.example.test":      nil,
	}, FirewallStatusResp{Enabled: true, EngineAvailable: true, TCPPorts: []int{22, 80, 443, 2083}})
	if _, err := database.GetDB().Exec(`INSERT INTO pdns_domains (name, type) VALUES ('celikhost.com', 'NATIVE')`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.GetDB().Exec(`INSERT INTO panel_settings (key, value) VALUES ('server_setup_infrastructure_zone:celikhost.com', '{"zone":"celikhost.com","zone_id":1}')`); err != nil {
		t.Fatal(err)
	}
	if err := panel.setSetting(context.Background(), settingDNSPeerIP, "198.51.100.20"); err != nil {
		t.Fatal(err)
	}
	if err := panel.setSetting(context.Background(), settingDNSPeerNS, "ns2.peer.test"); err != nil {
		t.Fatal(err)
	}

	ready := checkWebmailAddressForTest(t, panel, userID, "Mail.CelikHost.com.")
	if !ready.Ready || ready.State != webmailAddressReady || ready.Host != "mail.celikhost.com" || strings.Join(ready.Expected, ",") != "203.0.113.10,2001:db8::10" || ready.Firewall.State != "open" {
		t.Fatalf("ready: %+v", ready)
	}

	missing := checkWebmailAddressForTest(t, panel, userID, "webmail.celikhost.com")
	if missing.Ready || missing.State != webmailAddressNotFound || missing.Zone == nil || missing.Owner == nil ||
		missing.Zone.RecordsURL != "/api/v1/dns/infrastructure-zones/celikhost.com/records" ||
		len(missing.Records) != 2 || missing.Records[0].Name != "webmail" || missing.Records[0].Type != "A" || missing.Records[0].Content != "203.0.113.10" ||
		missing.Records[0].Text != "webmail.celikhost.com. 3600 IN A 203.0.113.10" {
		t.Fatalf("not found: %+v", missing)
	}
	elsewhere := checkWebmailAddressForTest(t, panel, userID, "webmail.elsewhere.test")
	if elsewhere.Ready || elsewhere.State != webmailAddressElsewhere || elsewhere.Zone != nil || !strings.Contains(elsewhere.Message, "198.51.100.7") || !strings.Contains(elsewhere.Message, "203.0.113.10") {
		t.Fatalf("elsewhere: %+v", elsewhere)
	}
	peer := checkWebmailAddressForTest(t, panel, userID, "mail.peer.test")
	if peer.Ready || peer.Code != "points_to_peer" || peer.Peer == nil || peer.Peer.Name != "ns2.peer.test" || !strings.Contains(peer.Message, "ns2.peer.test (198.51.100.20)") {
		t.Fatalf("peer: %+v", peer)
	}
	// One public resolver does not see the record yet: not ready.
	webmailAddressResolvers = func() []hostResolver {
		return []hostResolver{webmailAddressTestResolver{"mail.celikhost.com": {"203.0.113.10"}}, webmailAddressTestResolver{}}
	}
	if partial := checkWebmailAddressForTest(t, panel, userID, "mail.celikhost.com"); partial.Ready || partial.State != webmailAddressNotFound {
		t.Fatalf("partial propagation: %+v", partial)
	}
	webmailAddressResolvers = func() []hostResolver {
		r := webmailAddressTestResolver{"mail.celikhost.com": {"203.0.113.10", "2001:db8::10"}, "slow.example.test": nil}
		return []hostResolver{r, r}
	}
	unknown := checkWebmailAddressForTest(t, panel, userID, "slow.example.test")
	if unknown.Ready || unknown.State != webmailAddressUnknown || unknown.Owner == nil {
		t.Fatalf("unknown: %+v", unknown)
	}
	own := checkWebmailAddressForTest(t, panel, userID, "panel.example.test")
	if own.Ready || own.Code != "panel_address" {
		t.Fatalf("panel's own name: %+v", own)
	}
	if invalid := checkWebmailAddressForTest(t, panel, userID, "https://mail.celikhost.com/"); invalid.Ready || invalid.Code != "invalid" {
		t.Fatalf("invalid: %+v", invalid)
	}

	// Closed ports are said with the next action; CelikPanel never opens them.
	webmailAddressFirewallStatus = func(*Panel) (FirewallStatusResp, error) {
		return FirewallStatusResp{Enabled: true, EngineAvailable: true, TCPPorts: []int{22, 2083}}, nil
	}
	closed := checkWebmailAddressForTest(t, panel, userID, "mail.celikhost.com")
	if closed.Ready || closed.Code != "firewall_closed" || len(closed.Firewall.Closed) != 2 || !strings.Contains(closed.Owner.Next, "never opens them by itself") {
		t.Fatalf("closed: %+v", closed)
	}
}

func TestWebmailAddressEnableIsOneCheckedDurableOperation(t *testing.T) {
	panel, _, database, userID := newServiceStateAtomicPanel(t)
	stubWebmailAddressWorld(t, webmailAddressTestResolver{
		"mail.celikhost.com":   {"203.0.113.10"},
		"mail2.celikhost.com":  {"203.0.113.10"},
		"webmail.nowhere.test": {"198.51.100.7"},
	}, FirewallStatusResp{})
	restoreEnable, restoreRemove := enableWebmailAddressThroughAgent, removeWebmailAddressThroughAgent
	t.Cleanup(func() {
		enableWebmailAddressThroughAgent, removeWebmailAddressThroughAgent = restoreEnable, restoreRemove
	})
	var requests []transport.WebmailAddressEnableRequest
	enableWebmailAddressThroughAgent = func(_ *Panel, ctx context.Context, request transport.WebmailAddressEnableRequest) (transport.WebmailAddressResponse, error) {
		if _, err := panelMutationBinding(ctx); err != nil {
			t.Errorf("enable runs without the operation's lease: %v", err)
		}
		requests = append(requests, request)
		if request.Host == "mail2.celikhost.com" {
			return transport.WebmailAddressResponse{Host: request.Host, RolledBack: true, ErrorCode: "PROOF_FAILED", Error: "webmail did not answer as itself on https://mail2.celikhost.com/ (webmail's stylesheet answered HTTP 404)"}, nil
		}
		return transport.WebmailAddressResponse{Enabled: true, Proved: true, Host: request.Host, CertificateIssued: true, CertificateExpires: "2027-01-24T00:00:00Z", RoundcubeSettings: "applied"}, nil
	}
	post := func(path, body string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		request := serviceStateAtomicRequest(http.MethodPost, path, body, userID)
		request.Host = "panel.example.test:2083"
		switch path {
		case webmailAddressPath:
			panel.handleWebmailAddress(recorder, request)
		default:
			panel.handleWebmailAddressRemove(recorder, request)
		}
		return recorder
	}
	// DNS that points elsewhere is refused before anything starts.
	refused := post(webmailAddressPath, `{"host":"webmail.nowhere.test","request_id":"`+mustServiceOperationRequestID(t)+`"}`)
	if refused.Code != http.StatusConflict || !strings.Contains(refused.Body.String(), "WEBMAIL_ADDRESS_NOT_READY") || len(requests) != 0 {
		t.Fatalf("refused: %d %s", refused.Code, refused.Body.String())
	}
	requestID := mustServiceOperationRequestID(t)
	body := `{"host":"mail.celikhost.com","request_id":"` + requestID + `"}`
	accepted := post(webmailAddressPath, body)
	if accepted.Code != http.StatusAccepted {
		t.Fatalf("accepted: %d %s", accepted.Code, accepted.Body.String())
	}
	op := decodeServiceOperationEnvelope(t, accepted)
	done, _ := waitForServiceOperation(t, panel, userID, op.ID, serviceOperationSucceeded)
	if !strings.Contains(string(done.Result), `"url":"https://mail.celikhost.com/"`) || requests[0].Email != "state@example.test" || requests[0].Replace != "" {
		t.Fatalf("enabled: %s %+v", done.Result, requests)
	}
	if again := post(webmailAddressPath, body); decodeServiceOperationEnvelope(t, again).ID != op.ID || len(requests) != 1 {
		t.Fatalf("replay started a second operation: %d", len(requests))
	}
	address, ok := panel.webmailAddress(context.Background())
	if !ok || address.Host != "mail.celikhost.com" {
		t.Fatalf("address = %+v %v", address, ok)
	}
	var action string
	if err := database.GetDB().QueryRow(`SELECT action FROM audit_logs ORDER BY id DESC LIMIT 1`).Scan(&action); err != nil || action != "webmail.address:mail.celikhost.com" {
		t.Fatalf("audit = %q %v", action, err)
	}

	// Changing the address: the new one is enabled with the old one to
	// remove after it proves; a failed proof keeps the old address.
	failed := post(webmailAddressPath, `{"host":"mail2.celikhost.com","request_id":"`+mustServiceOperationRequestID(t)+`"}`)
	failedOp, _ := waitForServiceOperation(t, panel, userID, decodeServiceOperationEnvelope(t, failed).ID, serviceOperationFailed)
	if requests[1].Replace != "mail.celikhost.com" || failedOp.Error == nil || failedOp.Error.Code != "webmail_address_failed" || !strings.Contains(string(failedOp.Result), "HTTP 404") {
		t.Fatalf("change: %+v %+v %s", requests, failedOp.Error, failedOp.Result)
	}
	if address, _ := panel.webmailAddress(context.Background()); address.Host != "mail.celikhost.com" {
		t.Fatalf("a failed change replaced the address: %+v", address)
	}

	// Removal keeps the certificate unless asked, and the panel forgets it.
	removeWebmailAddressThroughAgent = func(_ *Panel, _ context.Context, request transport.WebmailAddressRemoveRequest) (transport.WebmailAddressRemoveResponse, error) {
		if request.Host != "mail.celikhost.com" || request.RemoveCertificate {
			t.Errorf("remove request = %+v", request)
		}
		return transport.WebmailAddressRemoveResponse{Removed: true}, nil
	}
	removed := post(webmailAddressRemovePath, `{"host":"mail.celikhost.com","request_id":"`+mustServiceOperationRequestID(t)+`"}`)
	waitForServiceOperation(t, panel, userID, decodeServiceOperationEnvelope(t, removed).ID, serviceOperationSucceeded)
	if _, ok := panel.webmailAddress(context.Background()); ok {
		t.Fatal("the removed address is still recorded")
	}
}

func TestPanelWebmailPathOnlyRedirectsToItsOwnAddress(t *testing.T) {
	panel, _, _, _ := newServiceStateAtomicPanel(t)
	handler := securityHeaders(true, http.HandlerFunc(panel.handleWebmailEntry))
	get := func(path string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "https://panel.example.test:2083"+path, nil)
		request.RemoteAddr = "192.0.2.47:1234"
		handler.ServeHTTP(recorder, request)
		return recorder
	}
	// Without an address: a short page under the panel's strict policy,
	// with no script, saying where the address is set.
	page := get("/webmail/")
	csp := page.Result().Header.Get("Content-Security-Policy")
	if page.Code != http.StatusOK || !strings.Contains(csp, "script-src 'self'") || strings.Contains(page.Body.String(), "<script") ||
		!strings.Contains(page.Body.String(), "Webmail address") || !strings.Contains(page.Body.String(), "Web postası adresi") {
		t.Fatalf("no address: %d %q %s", page.Code, csp, page.Body.String())
	}
	if err := panel.saveWebmailAddress(context.Background(), "mail.celikhost.com"); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/webmail/", "/webmail/?_task=mail&_mbox=INBOX", "/webmail/skins/elastic/styles/styles.min.css"} {
		redirect := get(path)
		if redirect.Code != http.StatusFound || redirect.Header().Get("Location") != "https://mail.celikhost.com/" || strings.Contains(redirect.Body.String(), "rcmail") {
			t.Fatalf("%s: %d %q", path, redirect.Code, redirect.Header().Get("Location"))
		}
	}
	post := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/webmail/", strings.NewReader("_user=a"))
	panel.handleWebmailEntry(post, request)
	if post.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST = %d", post.Code)
	}
}

func TestWebmailIsOfferedOnlyOnItsOwnAddress(t *testing.T) {
	panel, _, _, _ := newServiceStateAtomicPanel(t)
	panel.webmailReadinessProbe = func(context.Context) bool { return true }
	recorder := httptest.NewRecorder()
	panel.handleMailClientSetup(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/domains/1/mail/setup", nil), "example.test")
	if strings.Contains(recorder.Body.String(), `"webmail_available":true`) || strings.Contains(recorder.Body.String(), "/webmail/") {
		t.Fatalf("webmail offered on the panel's origin: %s", recorder.Body.String())
	}
	if err := panel.saveWebmailAddress(context.Background(), "mail.example.test"); err != nil {
		t.Fatal(err)
	}
	recorder = httptest.NewRecorder()
	panel.handleMailClientSetup(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/domains/1/mail/setup", nil), "example.test")
	if !strings.Contains(recorder.Body.String(), `"webmail_available":true`) || !strings.Contains(recorder.Body.String(), `"webmail_url":"https://mail.example.test/"`) {
		t.Fatalf("address not offered: %s", recorder.Body.String())
	}
}

func TestAnInterruptedWebmailAddressOperationIsReconciledReadOnly(t *testing.T) {
	panel, _, _, userID := newServiceStateAtomicPanel(t)
	restoreStatus, restoreEnable := readWebmailAddressStatusFromAgent, enableWebmailAddressThroughAgent
	t.Cleanup(func() {
		readWebmailAddressStatusFromAgent, enableWebmailAddressThroughAgent = restoreStatus, restoreEnable
	})
	enableWebmailAddressThroughAgent = func(*Panel, context.Context, transport.WebmailAddressEnableRequest) (transport.WebmailAddressResponse, error) {
		t.Error("recovery started an enable")
		return transport.WebmailAddressResponse{}, errors.New("no")
	}
	for _, tc := range []struct {
		name   string
		status transport.WebmailAddressStatusResponse
		want   string
	}{
		{"proved", transport.WebmailAddressStatusResponse{Present: true, Managed: true, Proved: true}, serviceOperationSucceeded},
		{"not proved", transport.WebmailAddressStatusResponse{Present: true, Managed: true, ProofError: "no answer"}, serviceOperationFailed},
	} {
		readWebmailAddressStatusFromAgent = func(*Panel, context.Context, string, bool) (transport.WebmailAddressStatusResponse, error) {
			return tc.status, nil
		}
		op, err := panel.createServiceOperationRequest(context.Background(), serviceOperationKindWebmailAddress, "roundcube", "mail.example.test", mustServiceOperationRequestID(t), serviceOperationActor{UserID: userID})
		if err != nil {
			t.Fatal(err)
		}
		if err := panel.markServiceOperationRunning(context.Background(), op.ID, "enabling"); err != nil {
			t.Fatal(err)
		}
		op.Status = serviceOperationRunning
		panel.serviceMutationMu.Lock()
		if err := panel.resumeInterruptedServiceOperationLocked(op); err != nil {
			panel.serviceMutationMu.Unlock()
			t.Fatalf("%s: %v", tc.name, err)
		}
		waitForServiceOperation(t, panel, userID, op.ID, tc.want)
		panel.serviceMutationMu.Lock()
		panel.serviceMutationMu.Unlock()
	}
	if address, ok := panel.webmailAddress(context.Background()); !ok || address.Host != "mail.example.test" {
		t.Fatalf("a proved interrupted enable is recorded: %+v %v", address, ok)
	}
}
