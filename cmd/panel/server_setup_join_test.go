package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// --- fixtures ---------------------------------------------------------------

func joinTestCertificate(t *testing.T) (tls.Certificate, *x509.Certificate) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: "celikpanel"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}, leaf
}

// joinTestSeams: loopback addresses are allowed, the caller's address is
// mapped, and this panel serves the given certificate.
func joinTestSeams(t *testing.T, cert *x509.Certificate, source string) {
	t.Helper()
	previousAllowed, previousCert, previousSource, previousLimiter := serverSetupPeerAddressAllowed, serverSetupPanelCertificate, serverSetupJoinSourceIP, setupJoinLimiter
	t.Cleanup(func() {
		serverSetupPeerAddressAllowed, serverSetupPanelCertificate, serverSetupJoinSourceIP, setupJoinLimiter = previousAllowed, previousCert, previousSource, previousLimiter
	})
	serverSetupPeerAddressAllowed = func(netip.Addr) bool { return true }
	serverSetupPanelCertificate = func() (*x509.Certificate, error) { return cert, nil }
	setupJoinLimiter = &serverSetupJoinLimiter{}
	if source != "" {
		serverSetupJoinSourceIP = func(*http.Request) (netip.Addr, bool) { return netip.MustParseAddr(source), true }
	}
}

func joinFirstDraft() serverSetupDraft {
	return serverSetupDraft{Purpose: "web", PanelDomain: "panel.example.test", DNSMode: "local", DNSEngine: "bind", DNSRole: "primary", NS1: "ns1.example.test", NS2: "ns2.example.test",
		LocalIP: "203.0.113.10", MainDomain: "example.test", ServerCount: 2, ShortName: "alpha", MainDNSServer: "this"}
}

// joinFirstFixture: a first server whose reviewed plan runs and waits at
// its join step.
func joinFirstFixture(t *testing.T, draft serverSetupDraft) (serviceOperationTestFixture, serverSetupPlan, *serverSetupExecution) {
	t.Helper()
	f, state := setupOperationFixture(t)
	t.Setenv("CELIKPANEL_SERVER_IP", draft.LocalIP)
	saved, err := f.panel.saveServerSetupDraft(context.Background(), state.Revision, draft)
	if err != nil {
		t.Fatal(err)
	}
	plan := saveSetupPlanForTest(t, f, saved)
	execution := &serverSetupExecution{ID: strings.Repeat("7", 32), RequestID: strings.Repeat("7", 32), PlanID: plan.ID, Status: "running"}
	for _, step := range plan.Steps {
		execution.Steps = append(execution.Steps, serverSetupExecutionStep{serverSetupPlanStep: step, Status: "pending", RequestID: serverSetupID(execution.ID, step.ID, "request"), OwnerID: serverSetupID(execution.ID, step.ID, "owner")})
	}
	raw, _ := json.Marshal(execution)
	if _, err := f.database.GetDB().Exec(`INSERT INTO server_setup_executions(id,request_id,plan_id,status,execution_json,created_at,updated_at) VALUES(?,?,?,?,?,'now','now')`, execution.ID, execution.RequestID, plan.ID, execution.Status, string(raw)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.database.GetDB().Exec(`UPDATE server_setup_state SET status='running' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	return f, plan, execution
}

func issueJoinCode(t *testing.T, f serviceOperationTestFixture) serverSetupJoinCodeView {
	t.Helper()
	w := httptest.NewRecorder()
	f.panel.handleServerSetupJoinCodes(w, serviceOperationAdminRequest(t, http.MethodPost, serverSetupJoinCodesPath, "{}", f.userID))
	var view serverSetupJoinCodeView
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &view) != nil || view.Code == "" {
		t.Fatalf("join code: %d %s", w.Code, w.Body.String())
	}
	return view
}

func joinMachineRequest(t *testing.T, f serviceOperationTestFixture, path, bearer string, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	req.TLS = &tls.ConnectionState{}
	req.RemoteAddr = "198.51.100.20:40000"
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	w := httptest.NewRecorder()
	f.panel.requireServerSetupLinkMachine(http.NotFoundHandler()).ServeHTTP(w, req)
	return w
}

func joinRequestFor(t *testing.T, code, requestID string) serverSetupJoinRequest {
	t.Helper()
	_, proof, ok := parseServerSetupJoinCode(code)
	if !ok {
		t.Fatalf("code does not parse: %q", code)
	}
	return serverSetupJoinRequest{Proof: proof, RequestID: requestID, DeclaredIPv4: "198.51.100.20", ShortName: "beta", OS: "debian 13"}
}

// --- tests --------------------------------------------------------------------

func TestServerSetupJoinCodeCarriesPinAndSecret(t *testing.T) {
	pin, secret := bytes.Repeat([]byte{0xab}, 10), []byte{1, 2, 3, 4, 5}
	code := formatServerSetupJoinCode(pin, secret)
	if len(code) != 29 || strings.Count(code, "-") != 5 {
		t.Fatalf("24 characters in groups of four: %q", code)
	}
	for _, typed := range []string{code, strings.ToLower(code), strings.ReplaceAll(code, "-", " "), strings.ReplaceAll(code, "-", "")} {
		gotPin, proof, ok := parseServerSetupJoinCode(typed)
		if !ok || !bytes.Equal(gotPin, pin) || proof != strings.ReplaceAll(code, "-", "")[16:] || !bytes.Equal(mustDecodeServerSetupProof(proof), secret) {
			t.Fatalf("%q: %x %q %v", typed, gotPin, proof, ok)
		}
	}
	for _, bad := range []string{"", code[:28], code + "A", strings.Replace(code, code[0:1], "1", 1)} {
		if _, _, ok := parseServerSetupJoinCode(bad); ok {
			t.Fatalf("accepted %q", bad)
		}
	}
}

func TestServerSetupJoinPlanRunsComponentsFirstAndWaitsForTheSecondServer(t *testing.T) {
	_, cert := joinTestCertificate(t)
	joinTestSeams(t, cert, "")
	f, plan, execution := joinFirstFixture(t, joinFirstDraft())
	joinIndex, dnsIndex := -1, -1
	for index, step := range plan.Steps {
		switch step.Kind {
		case "peer_join":
			joinIndex = index
		case "dns":
			dnsIndex = index
		case "service", "runtime", "mail_profile":
			if joinIndex != -1 {
				t.Fatalf("a component after the join wait: %+v", plan.Steps)
			}
		}
	}
	if joinIndex < 1 || dnsIndex < joinIndex || plan.InfrastructureDNS != nil || !serverSetupPlanHasStep(plan, "infrastructure_dns") {
		t.Fatalf("components, then the join, then DNS: %+v", plan.Steps)
	}
	if plan.Draft.PeerIP != "" {
		t.Fatal("the second server's address is never typed")
	}
	// Run up to the join: it waits with its reason.
	for index := range execution.Steps[:joinIndex] {
		execution.Steps[index].Status = "succeeded"
	}
	if _, err := f.panel.advanceServerSetupExecution(plan, execution); err != nil {
		t.Fatal(err)
	}
	if execution.Status != "waiting" || execution.Phase != "peer_join" || execution.Error == nil || execution.Error.Code != "server_setup_peer_join_waiting" {
		t.Fatalf("join wait: %+v %+v", execution.Status, execution.Error)
	}
}

func TestServerSetupJoinCodeIsHashedSingleUseAndReplaysTheSameJoin(t *testing.T) {
	_, cert := joinTestCertificate(t)
	joinTestSeams(t, cert, "")
	f, _, _ := joinFirstFixture(t, joinFirstDraft())
	view := issueJoinCode(t, f)
	if view.AttemptsLeft != 5 || view.Address != "https://203.0.113.10:"+itoaTest(panelPort()) {
		t.Fatalf("code view: %+v", view)
	}
	var stored string
	f.database.GetDB().QueryRow(`SELECT value FROM panel_settings WHERE key=?`, serverSetupJoinCodeKey).Scan(&stored)
	_, proof, _ := parseServerSetupJoinCode(view.Code)
	if strings.Contains(stored, proof) || strings.Contains(stored, strings.ReplaceAll(view.Code, "-", "")) || !strings.Contains(stored, hex.EncodeToString(serverSetupSPKIPin(cert))) {
		t.Fatal("only a salted hash of the secret is kept")
	}
	var audits int
	f.database.GetDB().QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action='server.setup.join_code' AND COALESCE(resource_type,'')=''`).Scan(&audits)
	if audits != 1 {
		t.Fatalf("the code is audited without its secret: %d", audits)
	}
	request := joinRequestFor(t, view.Code, strings.Repeat("a", 32))
	w := joinMachineRequest(t, f, serverSetupJoinPath, "", request)
	var first serverSetupJoinDocument
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &first) != nil {
		t.Fatalf("join: %d %s", w.Code, w.Body.String())
	}
	if first.MainDomain != "example.test" || first.MainDNSServer != "first" || first.NS1 != "ns1.example.test" || first.First.IPv4 != "203.0.113.10" || first.First.PanelDomain != "panel.example.test" ||
		first.PublisherEndpoint != "https://panel.example.test:"+itoaTest(panelPort()) || !remoteDNSValidSecret(first.EnrollmentCode) || !remoteDNSValidSecret(first.LinkCredential) {
		t.Fatalf("document: %+v", first)
	}
	var enrollments int
	f.database.GetDB().QueryRow(`SELECT COUNT(*) FROM remote_dns_enrollments WHERE code_hash=?`, remoteDNSHash(first.EnrollmentCode)).Scan(&enrollments)
	if enrollments != 1 {
		t.Fatal("the handed-over enrollment code is issued, hashed")
	}
	// The same request after a lost answer: the same join, fresh secrets.
	w = joinMachineRequest(t, f, serverSetupJoinPath, "", request)
	var again serverSetupJoinDocument
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &again) != nil || again.RequestID != first.RequestID || again.LinkCredential == first.LinkCredential {
		t.Fatalf("replay: %d %s", w.Code, w.Body.String())
	}
	f.database.GetDB().QueryRow(`SELECT COUNT(*) FROM remote_dns_enrollments`).Scan(&enrollments)
	if enrollments != 1 {
		t.Fatalf("the lost enrollment is withdrawn, never doubled: %d", enrollments)
	}
	var join serverSetupJoinRecord
	if found, _ := readServerSetupJSONSetting(context.Background(), f.database.GetDB(), serverSetupJoinKey, &join); !found || join.RequestID != request.RequestID || join.LinkCredential != again.LinkCredential {
		t.Fatalf("one join, the latest credential: %+v", join)
	}
	// Another request with the same code: used.
	other := request
	other.RequestID = strings.Repeat("b", 32)
	if w := joinMachineRequest(t, f, serverSetupJoinPath, "", other); w.Code != http.StatusGone || !strings.Contains(w.Body.String(), "server_setup_join_code_used") {
		t.Fatalf("second join: %d %s", w.Code, w.Body.String())
	}
}

func TestServerSetupJoinRefusesWrongCodesSourcesAndFloods(t *testing.T) {
	_, cert := joinTestCertificate(t)
	joinTestSeams(t, cert, "")
	f, _, _ := joinFirstFixture(t, joinFirstDraft())
	view := issueJoinCode(t, f)
	request := joinRequestFor(t, view.Code, strings.Repeat("c", 32))
	wrong := request
	wrong.Proof = "AAAAAAAA"
	if wrong.Proof == request.Proof {
		wrong.Proof = "BBBBBBBB"
	}
	// The caller's address must be the one it declares.
	spoofed := request
	spoofed.DeclaredIPv4 = "198.51.100.99"
	if w := joinMachineRequest(t, f, serverSetupJoinPath, "", spoofed); w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "server_setup_join_source_mismatch") {
		t.Fatalf("source: %d %s", w.Code, w.Body.String())
	}
	for attempt := 2; attempt <= 4; attempt++ {
		w := joinMachineRequest(t, f, serverSetupJoinPath, "", wrong)
		var refusal serverSetupJoinRefusal
		if w.Code != http.StatusUnauthorized || json.Unmarshal(w.Body.Bytes(), &refusal) != nil || refusal.Code != "server_setup_join_code_wrong" || refusal.AttemptsLeft == nil || *refusal.AttemptsLeft != 5-attempt {
			t.Fatalf("attempt %d: %d %s", attempt, w.Code, w.Body.String())
		}
	}
	// The rate limit (5 a minute from one address) answers before the code.
	if w := joinMachineRequest(t, f, serverSetupJoinPath, "", request); w.Code != http.StatusOK {
		t.Fatalf("the right code within the attempts: %d %s", w.Code, w.Body.String())
	}
	if w := joinMachineRequest(t, f, serverSetupJoinPath, "", request); w.Code != http.StatusTooManyRequests {
		t.Fatalf("flood: %d %s", w.Code, w.Body.String())
	}
	// A fresh code exhausted by five wrong tries refuses even the right one.
	setupJoinLimiter = &serverSetupJoinLimiter{}
	if _, err := f.database.GetDB().Exec(`DELETE FROM panel_settings WHERE key IN (?,?)`, serverSetupJoinKey, serverSetupJoinCodeKey); err != nil {
		t.Fatal(err)
	}
	view = issueJoinCode(t, f)
	request = joinRequestFor(t, view.Code, strings.Repeat("d", 32))
	wrong.Proof = "AAAAAAAA"
	if wrong.Proof == request.Proof {
		wrong.Proof = "BBBBBBBB"
	}
	for attempt := 0; attempt < 5; attempt++ {
		setupJoinLimiter = &serverSetupJoinLimiter{}
		joinMachineRequest(t, f, serverSetupJoinPath, "", wrong)
	}
	setupJoinLimiter = &serverSetupJoinLimiter{}
	if w := joinMachineRequest(t, f, serverSetupJoinPath, "", request); w.Code != http.StatusGone || !strings.Contains(w.Body.String(), "server_setup_join_code_exhausted") {
		t.Fatalf("exhausted: %d %s", w.Code, w.Body.String())
	}
	// The link endpoints accept no browser request and no unknown credential.
	if w := joinMachineRequest(t, f, serverSetupPeerNamesPath, strings.Repeat("e", 32)+"."+strings.Repeat("f", 64), serverSetupPeerNames{}); w.Code != http.StatusUnauthorized {
		t.Fatalf("unknown link: %d", w.Code)
	}
}

func TestServerSetupJoinPinnedTLSSendsNothingToAnotherKey(t *testing.T) {
	served, leaf := joinTestCertificate(t)
	_, other := joinTestCertificate(t)
	joinTestSeams(t, leaf, "")
	var received atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"accepted":true}`))
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
	target := serverSetupLinkTarget{Address: "https://203.0.113.10:2083", Pin: serverSetupSPKIPin(leaf)}
	var answer struct {
		Accepted bool `json:"accepted"`
	}
	if err := serverSetupLinkJSON(context.Background(), target, serverSetupPeerNamesPath, "x.y", map[string]string{}, &answer); err != nil || !answer.Accepted || received.Load() != 1 {
		t.Fatalf("pinned exchange: %v %v %d", err, answer, received.Load())
	}
	target.Pin = serverSetupSPKIPin(other)
	if err := serverSetupLinkJSON(context.Background(), target, serverSetupPeerNamesPath, "x.y", map[string]string{}, &answer); !errors.Is(err, errServerSetupLinkPinMismatch) || received.Load() != 1 {
		t.Fatalf("another key must get nothing: %v %d", err, received.Load())
	}
	server.Close()
	target.Pin = serverSetupSPKIPin(leaf)
	if err := serverSetupLinkJSON(context.Background(), target, serverSetupPeerNamesPath, "x.y", map[string]string{}, &answer); !errors.Is(err, errServerSetupLinkUnreachable) {
		t.Fatalf("a closed server is unreachable, nothing sent: %v", err)
	}
	// An answer lost after the request was written is unknown.
	lost := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		conn, _, _ := w.(http.Hijacker).Hijack()
		conn.Close()
	}))
	lost.TLS = &tls.Config{Certificates: []tls.Certificate{served}}
	lost.StartTLS()
	t.Cleanup(lost.Close)
	listener = lost.Listener.Addr().String()
	if err := serverSetupLinkJSON(context.Background(), target, serverSetupPeerNamesPath, "x.y", map[string]string{}, &answer); !errors.Is(err, errServerSetupLinkUnknown) {
		t.Fatalf("a lost answer is unknown: %v", err)
	}
}
