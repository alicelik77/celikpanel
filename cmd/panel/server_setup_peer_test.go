package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/dnswire"
)

// peerPair: a first server waiting at its join step, served over TLS with
// the key its code names, and a second server reaching it through the link
// transport (the dial goes to the loopback listener).
type peerPair struct {
	first      serviceOperationTestFixture
	plan       serverSetupPlan
	execution  *serverSetupExecution
	second     serviceOperationTestFixture
	dropAnswer atomic.Bool
	joins      atomic.Int32
}

func newPeerPair(t *testing.T, firstDraft serverSetupDraft) *peerPair {
	t.Helper()
	served, leaf := joinTestCertificate(t)
	joinTestSeams(t, leaf, "198.51.100.20")
	pair := &peerPair{}
	pair.first, pair.plan, pair.execution = joinFirstFixture(t, firstDraft)
	machine := pair.first.panel.requireServerSetupLinkMachine(http.NotFoundHandler())
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == serverSetupJoinPath {
			pair.joins.Add(1)
			if pair.dropAnswer.CompareAndSwap(true, false) {
				// The first server handles the join; its answer is lost.
				machine.ServeHTTP(httptest.NewRecorder(), r)
				conn, _, _ := w.(http.Hijacker).Hijack()
				conn.Close()
				return
			}
		}
		machine.ServeHTTP(w, r)
	}))
	server.TLS = &tls.Config{Certificates: []tls.Certificate{served}}
	server.StartTLS()
	t.Cleanup(server.Close)
	previous := serverSetupLinkDeps
	t.Cleanup(func() { serverSetupLinkDeps = previous })
	listener := server.Listener.Addr().String()
	serverSetupLinkDeps = func() serverSetupLinkDependencies {
		dialer := &net.Dialer{Timeout: time.Second}
		return serverSetupLinkDependencies{dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, network, listener)
		}}
	}
	pair.second, _ = setupOperationFixture(t)
	t.Setenv("CELIKPANEL_SERVER_IP", "198.51.100.20")
	return pair
}

func (pair *peerPair) connect(t *testing.T, code string) serverSetupPeerConnectAnswer {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"address": "203.0.113.10:" + itoaTest(panelPort()), "code": code})
	w := httptest.NewRecorder()
	pair.second.panel.handleServerSetupPeerConnect(w, serviceOperationAdminRequest(t, http.MethodPost, serverSetupPeerConnectPath, string(body), pair.second.userID))
	var answer serverSetupPeerConnectAnswer
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &answer) != nil {
		t.Fatalf("connect: %d %s", w.Code, w.Body.String())
	}
	return answer
}

// secondReview saves the second server's own answers and reviews its plan.
func (pair *peerPair) secondReview(t *testing.T, change func(*serverSetupDraft)) (serverSetupState, serverSetupPlan) {
	t.Helper()
	state, err := pair.second.panel.loadServerSetup(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	draft := state.Draft
	draft.Purpose, draft.PanelDomain, draft.LocalIP, draft.DNSEngine, draft.ShortName = "web", "beta.example.test", "198.51.100.20", "bind", "beta"
	if change != nil {
		change(&draft)
	}
	state, err = pair.second.panel.saveServerSetupDraft(context.Background(), state.Revision, draft)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := pair.second.panel.buildServerSetupPlan(context.Background(), state, serviceOperationActor{UserID: pair.second.userID})
	if err != nil {
		t.Fatal(err)
	}
	return state, plan
}

func TestServerSetupPeerJoinsByCodeAndTakesTheFirstServersAnswers(t *testing.T) {
	pair := newPeerPair(t, joinFirstDraft())
	view := issueJoinCode(t, pair.first)
	// The first answer is lost: unknown, the request kept for the next try.
	pair.dropAnswer.Store(true)
	if answer := pair.connect(t, view.Code); answer.State != "unknown" || answer.Code != "server_setup_join_unknown" {
		t.Fatalf("lost answer: %+v", answer)
	}
	var pending serverSetupLinkPending
	if found, _ := readServerSetupJSONSetting(context.Background(), pair.second.database.GetDB(), serverSetupLinkPendingKey, &pending); !found {
		t.Fatal("the unanswered join is kept for the same retry")
	}
	answer := pair.connect(t, view.Code)
	if answer.State != "ok" || answer.Setup == nil {
		t.Fatalf("retry: %+v", answer)
	}
	var join serverSetupJoinRecord
	if found, _ := readServerSetupJSONSetting(context.Background(), pair.first.database.GetDB(), serverSetupJoinKey, &join); !found || join.RequestID != pending.RequestID || pair.joins.Load() != 2 {
		t.Fatalf("the retry is the same join, never a second: %+v %d", join, pair.joins.Load())
	}
	d := answer.Setup.Draft
	if d.MainDomain != "example.test" || d.ServerCount != 2 || d.DNSMode != "local" || d.DNSRole != "secondary" || d.MainDNSServer != "peer" || d.PeerIP != "203.0.113.10" ||
		d.NS1 != "ns1.example.test" || d.NS2 != "ns2.example.test" || d.PeerNS != "ns1.example.test" || d.DNSPublisherEndpoint != "https://panel.example.test:"+itoaTest(panelPort()) {
		t.Fatalf("the second server's draft comes from the join: %+v", d)
	}
	// No secret leaves through the administrator API.
	w := httptest.NewRecorder()
	pair.second.panel.handleServerSetupPeer(w, serviceOperationAdminRequest(t, http.MethodGet, serverSetupPeerPath, "", pair.second.userID))
	link, _ := pair.second.panel.readServerSetupLink(context.Background())
	if w.Code != http.StatusOK || link == nil || strings.Contains(w.Body.String(), link.LinkCredential) || strings.Contains(w.Body.String(), link.EnrollmentCode) || !strings.Contains(w.Body.String(), `"role":"second"`) {
		t.Fatalf("peer view: %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	pair.first.panel.handleServerSetupPeer(w, serviceOperationAdminRequest(t, http.MethodGet, serverSetupPeerPath, "", pair.first.userID))
	if strings.Contains(w.Body.String(), join.LinkCredential) || !strings.Contains(w.Body.String(), `"ipv4":"198.51.100.20"`) {
		t.Fatalf("first server's view: %s", w.Body.String())
	}
	// The second server's plan starts by sending its names; its shared
	// answers stay the first server's.
	_, plan := pair.secondReview(t, nil)
	if !plan.CanStart || plan.Steps[0].Kind != "peer_names" || plan.Steps[0].Target != link.Address {
		t.Fatalf("second plan: %v %+v", plan.Blockers, plan.Steps)
	}
	_, changed := pair.secondReview(t, func(d *serverSetupDraft) { d.NS1 = "dns1.example.test" })
	if changed.CanStart || !strings.Contains(strings.Join(changed.Blockers, ","), "server_setup_peer_join_answers_changed") {
		t.Fatalf("changed shared answers: %v", changed.Blockers)
	}
}

func TestServerSetupPeerNamesBindTheSecondServerOnceAcrossRestarts(t *testing.T) {
	pair := newPeerPair(t, joinFirstDraft())
	view := issueJoinCode(t, pair.first)
	if answer := pair.connect(t, view.Code); answer.State != "ok" {
		t.Fatalf("connect: %+v", answer)
	}
	// The first server runs up to its join step and waits for the names.
	joinIndex := -1
	for index, step := range pair.execution.Steps {
		if step.Kind == "peer_join" {
			joinIndex = index
			break
		}
		pair.execution.Steps[index].Status = "succeeded"
	}
	if _, err := pair.first.panel.advanceServerSetupExecution(pair.plan, pair.execution); err != nil {
		t.Fatal(err)
	}
	if pair.execution.Phase != "peer_join" || pair.execution.Error == nil || pair.execution.Error.Code != "server_setup_peer_names_waiting" {
		t.Fatalf("names wait: %+v", pair.execution.Error)
	}
	_, plan := pair.secondReview(t, nil)
	step := serverSetupExecutionStep{serverSetupPlanStep: plan.Steps[0], RequestID: strings.Repeat("5", 32), OwnerID: strings.Repeat("6", 32)}
	for attempt := 0; attempt < 2; attempt++ {
		// The same names twice (a retry after a lost answer) are accepted.
		if done, err := pair.second.panel.runServerSetupPeerNames(context.Background(), plan, step); !done || err != nil {
			t.Fatalf("names %d: %v %v", attempt, done, err)
		}
	}
	// A runner restarted mid-wait reads the execution again and binds once.
	for pass := 0; pass < 2; pass++ {
		var raw string
		if err := pair.first.database.GetDB().QueryRow(`SELECT execution_json FROM server_setup_executions WHERE id=?`, pair.execution.ID).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		reloaded := &serverSetupExecution{}
		if err := json.Unmarshal([]byte(raw), reloaded); err != nil {
			t.Fatal(err)
		}
		if _, err := pair.first.panel.advanceServerSetupExecution(pair.plan, reloaded); err != nil {
			t.Fatal(err)
		}
		if reloaded.Steps[joinIndex].Status != "succeeded" {
			t.Fatalf("pass %d: the join step binds: %+v %+v", pass, reloaded.Steps[joinIndex], reloaded.Error)
		}
	}
	binding, err := pair.first.panel.readServerSetupPeerBinding(context.Background(), pair.plan, pair.execution.ID)
	if err != nil || binding == nil || binding.PeerIPv4 != "198.51.100.20" || binding.PeerPanelDomain != "beta.example.test" || binding.InfrastructureDNS == nil {
		t.Fatalf("binding: %+v %v", binding, err)
	}
	found := false
	for _, record := range binding.InfrastructureDNS.Records {
		found = found || (record.Name == "beta.example.test" && record.Type == "A" && record.Content == "198.51.100.20")
	}
	if !found {
		t.Fatalf("the second server's name is written on the main DNS server: %+v", binding.InfrastructureDNS.Records)
	}
	effective, err := pair.first.panel.effectiveServerSetupPlan(context.Background(), pair.plan, pair.execution.ID)
	if err != nil || effective.Draft.PeerIP != "198.51.100.20" || effective.Draft.PeerNS != "ns2.example.test" || effective.ID != pair.plan.ID || !effective.peerBound {
		t.Fatalf("bound plan: %+v %v", effective.Draft, err)
	}
	if _, _, err := setupDNSIdentity(effective.Draft); err != nil {
		t.Fatalf("the bound draft is a full DNS pair: %v", err)
	}
	// The registrar card names the bound second server.
	previousOptions := setupDelegationOptions
	t.Cleanup(func() { setupDelegationOptions = previousOptions })
	setupDelegationOptions = func() dnswire.DelegationOptions {
		return dnswire.DelegationOptions{Resolvers: []string{"127.0.0.1:1"}, Address: func(string) string { return "127.0.0.1:1" }}
	}
	w := httptest.NewRecorder()
	pair.first.panel.handleServerSetupDelegation(w, serviceOperationAdminRequest(t, http.MethodGet, serverSetupDelegationPath+"?zone=example.test", "", pair.first.userID))
	var card serverSetupDelegation
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &card) != nil || card.ExpectedNS[1].IPv4 != "198.51.100.20" {
		t.Fatalf("registrar card after binding: %d %s", w.Code, w.Body.String())
	}
	var bindings int
	pair.first.database.GetDB().QueryRow(`SELECT COUNT(*) FROM panel_settings WHERE key LIKE 'server_setup_peer_binding:%'`).Scan(&bindings)
	if bindings != 1 {
		t.Fatalf("one binding: %d", bindings)
	}
	// Other names once bound are refused with the reason.
	changed := plan
	changed.Draft.PanelDomain = "gamma.example.test"
	if done, err := pair.second.panel.runServerSetupPeerNames(context.Background(), changed, step); done || err == nil || !strings.Contains(err.Error(), "server_setup_peer_names_bound") {
		t.Fatalf("names after binding: %v %v", done, err)
	}
	// A new code is refused once the second server is bound (the setup has
	// moved past its join step).
	w = httptest.NewRecorder()
	pair.first.panel.handleServerSetupJoinCodes(w, serviceOperationAdminRequest(t, http.MethodPost, serverSetupJoinCodesPath, "{}", pair.first.userID))
	if w.Code != http.StatusConflict || !(strings.Contains(w.Body.String(), "server_setup_join_bound") || strings.Contains(w.Body.String(), "server_setup_join_not_waiting")) {
		t.Fatalf("code after binding: %d %s", w.Code, w.Body.String())
	}
}

func TestServerSetupPeerNamesClashesAreRefusedBeforeAnythingIsWritten(t *testing.T) {
	d, err := canonicalServerSetupDraft(joinFirstDraft())
	if err != nil {
		t.Fatal(err)
	}
	join := serverSetupJoinRecord{RequestID: strings.Repeat("a", 32), Peer: serverSetupJoinPeer{IPv4: "198.51.100.20"}}
	base := serverSetupPeerNames{RequestID: strings.Repeat("b", 32), PanelDomain: "beta.example.test", DNSEngine: "bind", Address: "https://198.51.100.20:2083", Pin: strings.Repeat("ab", 32)}
	if code, message := canonicalServerSetupPeerNames(&base, d, join); code != "" {
		t.Fatalf("valid names refused: %s %s", code, message)
	}
	for label, test := range map[string]struct {
		change func(*serverSetupPeerNames)
		code   string
	}{
		"the first server's panel name": {func(n *serverSetupPeerNames) { n.PanelDomain = "panel.example.test" }, "server_setup_peer_names_clash"},
		"a nameserver name":             {func(n *serverSetupPeerNames) { n.PanelDomain = "ns1.example.test" }, "server_setup_peer_names_clash"},
		"outside the main domain":       {func(n *serverSetupPeerNames) { n.PanelDomain = "beta.other.test" }, "server_setup_peer_names_outside_domain"},
		"another address":               {func(n *serverSetupPeerNames) { n.Address = "https://198.51.100.99:2083" }, "server_setup_peer_names_invalid"},
		"publisher on a backup":         {func(n *serverSetupPeerNames) { n.PublisherEndpoint = "https://beta.example.test:2083" }, "server_setup_peer_names_invalid"},
	} {
		names := base
		test.change(&names)
		if code, _ := canonicalServerSetupPeerNames(&names, d, join); code != test.code {
			t.Fatalf("%s: %q, want %q", label, code, test.code)
		}
	}
	// Both servers cannot take the main domain's mail.
	mail := d
	mail.Customization = &serverSetupCustomization{Components: []string{"postfix"}}
	mail.Purpose, mail.MailHostname = "custom", "mail.example.test"
	names := base
	names.MailHostname, names.Mail = "mail2.example.test", &serverSetupPeerMail{Hostname: "mail2.example.test", MX: "now"}
	if code, _ := canonicalServerSetupPeerNames(&names, mail, join); code != "server_setup_peer_mail_both" {
		t.Fatalf("mail on both: %q", code)
	}
}

// A draft that could not be saved after the join is applied by sending the
// form again; the join is never repeated, and a second press after success
// is the same answer, not "code used".
func TestServerSetupPeerConnectRecoversAFailedDraftSave(t *testing.T) {
	pair := newPeerPair(t, joinFirstDraft())
	view := issueJoinCode(t, pair.first)
	previous := serverSetupPeerSaveDraft
	t.Cleanup(func() { serverSetupPeerSaveDraft = previous })
	failed := false
	serverSetupPeerSaveDraft = func(p *Panel, ctx context.Context, revision int, d serverSetupDraft) (serverSetupState, error) {
		if !failed {
			failed = true
			return serverSetupState{}, errServerSetupConflict
		}
		return previous(p, ctx, revision, d)
	}
	if answer := pair.connect(t, view.Code); answer.State != "ok" || answer.Code != "server_setup_peer_draft_retry" || answer.Setup != nil {
		t.Fatalf("a failed save is said: %+v", answer)
	}
	for attempt := 0; attempt < 2; attempt++ {
		answer := pair.connect(t, view.Code)
		if answer.State != "ok" || answer.Code != "" || answer.Setup == nil || answer.Setup.Draft.MainDomain != "example.test" || answer.Setup.Draft.PeerIP != "203.0.113.10" {
			t.Fatalf("attempt %d recovers: %+v", attempt, answer)
		}
	}
	if pair.joins.Load() != 1 {
		t.Fatalf("the join is not repeated: %d", pair.joins.Load())
	}
}

// The join document's full key fingerprint must be the key that answered;
// another one is a wrong server and nothing is kept.
func TestServerSetupPeerDocumentFingerprintMustBeTheAnsweringKey(t *testing.T) {
	pair := newPeerPair(t, joinFirstDraft())
	view := issueJoinCode(t, pair.first)
	_, other := joinTestCertificate(t)
	serverSetupPanelCertificate = func() (*x509.Certificate, error) { return other, nil }
	if answer := pair.connect(t, view.Code); answer.State != "wrong_server" {
		t.Fatalf("a document naming another key: %+v", answer)
	}
	if link, _ := pair.second.panel.readServerSetupLink(context.Background()); link != nil {
		t.Fatal("nothing of it is kept")
	}
}

// The pin is the identity the panel serves on its IP address: the first
// self-signed certificate while it is valid, across domain certificates.
func TestServerSetupPanelIdentityIsTheIPAddressIdentity(t *testing.T) {
	dir := t.TempDir()
	served, leaf := joinTestCertificate(t)
	path := filepath.Join(dir, "panel.crt")
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: served.Certificate[0]}), 0o600); err != nil {
		t.Fatal(err)
	}
	if !serverSetupBootstrapIdentityValid(path) || len(leaf.IPAddresses) == 0 {
		t.Fatal("a valid self-signed IP identity is the pinned one")
	}
	if serverSetupBootstrapIdentityValid(filepath.Join(dir, "missing.crt")) {
		t.Fatal("a missing identity is not")
	}
}
