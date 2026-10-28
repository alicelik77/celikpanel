package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
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
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/secrets"
	"github.com/alicelik/celikpanel/internal/transport"
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

// joinTestSeams, on one panel before it serves: loopback addresses are
// allowed, the caller's address is mapped, and the panel serves the given
// certificate.
func joinTestSeams(p *Panel, cert *x509.Certificate, source string) {
	p.setupLink.addressAllowed = func(netip.Addr) bool { return true }
	p.setupLink.certificate = func() (*x509.Certificate, error) { return cert, nil }
	if source != "" {
		p.setupLink.sourceIP = func(*http.Request) (netip.Addr, bool) { return netip.MustParseAddr(source), true }
	}
}

// joinTestDial sends every setup link dial of this panel to one listener.
func joinTestDial(p *Panel, listener func() string) {
	p.setupLink.dial = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: time.Second}).DialContext(ctx, network, listener())
	}
}

func (l *serverSetupJoinLimiter) reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.window, l.total, l.byIP = time.Time{}, 0, nil
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
	// An Agent that can issue the mail host certificate (a first server with
	// the main domain's mail).
	caps := []string{transport.AgentCapabilityMailHostCertificateV1, transport.AgentCapabilityMailTLSSyncV2}
	f.agent.versionCapabilities = &caps
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
	return serverSetupJoinRequest{Protocol: serverSetupLinkProtocol, Proof: proof, RequestID: requestID, DeclaredIPv4: "198.51.100.20", ShortName: "beta", OS: "debian 13"}
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
	// Digits that look like letters are read as those letters.
	lookAlike := strings.NewReplacer("O", "0", "I", "1", "B", "8").Replace(code)
	if gotPin, _, ok := parseServerSetupJoinCode(lookAlike); !ok || !bytes.Equal(gotPin, pin) {
		t.Fatalf("look-alikes: %q", lookAlike)
	}
	for _, bad := range []string{"", code[:28], code + "A", strings.Replace(code, code[0:1], "9", 1)} {
		if _, _, ok := parseServerSetupJoinCode(bad); ok {
			t.Fatalf("accepted %q", bad)
		}
	}
}

func TestServerSetupJoinPlanRunsComponentsFirstAndWaitsForTheSecondServer(t *testing.T) {
	_, cert := joinTestCertificate(t)
	f, plan, execution := joinFirstFixture(t, joinFirstDraft())
	joinTestSeams(f.panel, cert, "")
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
	f, _, _ := joinFirstFixture(t, joinFirstDraft())
	joinTestSeams(f.panel, cert, "")
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
	if found, _ := readServerSetupJSONSetting(context.Background(), f.database.GetDB(), serverSetupJoinKey, &join); !found || join.RequestID != request.RequestID || join.LinkCredentialHash != f.panel.serverSetupMAC("link", again.LinkCredential) {
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
	f, _, _ := joinFirstFixture(t, joinFirstDraft())
	joinTestSeams(f.panel, cert, "")
	view := issueJoinCode(t, f)
	request := joinRequestFor(t, view.Code, strings.Repeat("c", 32))
	wrong := request
	wrong.Proof = "AAAAAAAA"
	if wrong.Proof == request.Proof {
		wrong.Proof = "BBBBBBBB"
	}
	// The caller's address must be the one it declares; a correct code from
	// a wrong address uses up nothing.
	spoofed := request
	spoofed.DeclaredIPv4 = "198.51.100.99"
	for attempt := 0; attempt < 3; attempt++ {
		w := joinMachineRequest(t, f, serverSetupJoinPath, "", spoofed)
		var refusal serverSetupJoinRefusal
		if w.Code != http.StatusForbidden || json.Unmarshal(w.Body.Bytes(), &refusal) != nil || refusal.Code != "server_setup_join_source_mismatch" || *refusal.AttemptsLeft != 5 {
			t.Fatalf("source: %d %s", w.Code, w.Body.String())
		}
	}
	for attempt := 1; attempt <= 4; attempt++ {
		w := joinMachineRequest(t, f, serverSetupJoinPath, "", wrong)
		var refusal serverSetupJoinRefusal
		if w.Code != http.StatusUnauthorized || json.Unmarshal(w.Body.Bytes(), &refusal) != nil || refusal.Code != "server_setup_join_code_wrong" || refusal.AttemptsLeft == nil || *refusal.AttemptsLeft != 5-attempt {
			t.Fatalf("attempt %d: %d %s", attempt, w.Code, w.Body.String())
		}
	}
	// Five wrong codes from one source end that source, not the code: the
	// right code from another address still joins.
	if w := joinMachineRequest(t, f, serverSetupJoinPath, "", wrong); w.Code != http.StatusUnauthorized {
		t.Fatalf("fifth: %d", w.Code)
	}
	if w := joinMachineRequest(t, f, serverSetupJoinPath, "", request); w.Code != http.StatusGone || !strings.Contains(w.Body.String(), "server_setup_join_code_exhausted") {
		t.Fatalf("the source is exhausted: %d %s", w.Code, w.Body.String())
	}
	f.panel.setupLink.sourceIP = func(*http.Request) (netip.Addr, bool) { return netip.MustParseAddr("198.51.100.30"), true }
	other := request
	other.DeclaredIPv4 = "198.51.100.30"
	if w := joinMachineRequest(t, f, serverSetupJoinPath, "", other); w.Code != http.StatusOK {
		t.Fatalf("another source with the right code: %d %s", w.Code, w.Body.String())
	}
	// The rate limit is per source with a large ceiling: one flooding caller
	// does not hold another out.
	limiter := &serverSetupJoinLimiter{}
	now := time.Now()
	for i := 0; i < serverSetupJoinRatePerSource; i++ {
		if !limiter.allow("198.51.100.40", now) {
			t.Fatalf("request %d refused", i)
		}
	}
	if limiter.allow("198.51.100.40", now) || !limiter.allow("198.51.100.41", now) {
		t.Fatal("per-source limit")
	}
	if serverSetupJoinSourceKey(netip.MustParseAddr("2001:db8:1:2:aaaa::1")) != serverSetupJoinSourceKey(netip.MustParseAddr("2001:db8:1:2:bbbb::2")) {
		t.Fatal("an IPv6 /64 is one source")
	}
	// A code with 50 wrong tries in all is dead for everyone.
	if _, err := f.database.GetDB().Exec(`DELETE FROM panel_settings WHERE key IN (?,?)`, serverSetupJoinKey, serverSetupJoinCodeKey); err != nil {
		t.Fatal(err)
	}
	f.panel.setupLink.sourceIP = nil
	view = issueJoinCode(t, f)
	var record serverSetupJoinCodeRecord
	readServerSetupJSONSetting(context.Background(), f.database.GetDB(), serverSetupJoinCodeKey, &record)
	record.Attempts = serverSetupJoinCodeFailures
	writeServerSetupJSONSetting(context.Background(), f.database.GetDB(), serverSetupJoinCodeKey, record)
	if w := joinMachineRequest(t, f, serverSetupJoinPath, "", joinRequestFor(t, view.Code, strings.Repeat("d", 32))); w.Code != http.StatusGone || !strings.Contains(w.Body.String(), "server_setup_join_code_exhausted") {
		t.Fatalf("dead code: %d %s", w.Code, w.Body.String())
	}
	// The link endpoints accept no browser request and no unknown credential.
	if w := joinMachineRequest(t, f, serverSetupPeerNamesPath, strings.Repeat("e", 32)+"."+strings.Repeat("f", 64), serverSetupPeerNames{}); w.Code != http.StatusUnauthorized {
		t.Fatalf("unknown link: %d", w.Code)
	}
}

func TestServerSetupJoinPinnedTLSSendsNothingToAnotherKey(t *testing.T) {
	served, leaf := joinTestCertificate(t)
	_, other := joinTestCertificate(t)
	caller := &Panel{}
	joinTestSeams(caller, leaf, "")
	var received, bearers atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		if r.Header.Get("Authorization") != "" {
			bearers.Add(1)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"accepted":true}`))
	}))
	server.TLS = &tls.Config{Certificates: []tls.Certificate{served}}
	server.StartTLS()
	t.Cleanup(server.Close)
	var listenerMu sync.Mutex
	listener := server.Listener.Addr().String()
	joinTestDial(caller, func() string { listenerMu.Lock(); defer listenerMu.Unlock(); return listener })
	target := serverSetupLinkTarget{Address: "https://203.0.113.10:2083", Pin: serverSetupSPKIPin(leaf)}
	var answer struct {
		Accepted bool `json:"accepted"`
	}
	if err := caller.serverSetupLinkJSON(context.Background(), target, serverSetupPeerNamesPath, "x.y", map[string]string{}, &answer); err != nil || !answer.Accepted || received.Load() != 1 {
		t.Fatalf("pinned exchange: %v %v %d", err, answer, received.Load())
	}
	// After the join the full fingerprint is pinned, and it is observed.
	var observed []byte
	target = serverSetupLinkTarget{Address: target.Address, Pin: serverSetupSPKI(leaf), Observed: &observed}
	if err := caller.serverSetupLinkJSON(context.Background(), target, serverSetupPeerNamesPath, "x.y", map[string]string{}, &answer); err != nil || !bytes.Equal(observed, serverSetupSPKI(leaf)) || bearers.Load() != 2 {
		t.Fatalf("full pin: %v %x %d", err, observed, bearers.Load())
	}
	// Another key gets no bearer and there is no fallback of any kind.
	for _, pin := range [][]byte{serverSetupSPKIPin(other), serverSetupSPKI(other), append(serverSetupSPKIPin(leaf)[:9:9], 0)} {
		target = serverSetupLinkTarget{Address: target.Address, Pin: pin}
		if err := caller.serverSetupLinkJSON(context.Background(), target, serverSetupPeerNamesPath, "x.y", map[string]string{}, &answer); !errors.Is(err, errServerSetupLinkPinMismatch) || received.Load() != 2 || bearers.Load() != 2 {
			t.Fatalf("another key must get nothing: %v %d %d", err, received.Load(), bearers.Load())
		}
	}
	target = serverSetupLinkTarget{Address: target.Address, Pin: serverSetupSPKIPin(leaf)}
	server.Close()
	target.Pin = serverSetupSPKIPin(leaf)
	if err := caller.serverSetupLinkJSON(context.Background(), target, serverSetupPeerNamesPath, "x.y", map[string]string{}, &answer); !errors.Is(err, errServerSetupLinkUnreachable) {
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
	listenerMu.Lock()
	listener = lost.Listener.Addr().String()
	listenerMu.Unlock()
	if err := caller.serverSetupLinkJSON(context.Background(), target, serverSetupPeerNamesPath, "x.y", map[string]string{}, &answer); !errors.Is(err, errServerSetupLinkUnknown) {
		t.Fatalf("a lost answer is unknown: %v", err)
	}
}

// The source check is the declared IPv4 address only (alpha.114 review):
// an IPv6 caller declaring any IPv4 address is refused, the declared IPv6
// address must be public and never authorizes, and the first server is
// dialed at its IPv4 address.
func TestServerSetupJoinSourceIsTheDeclaredIPv4Only(t *testing.T) {
	_, cert := joinTestCertificate(t)
	f, _, _ := joinFirstFixture(t, joinFirstDraft())
	joinTestSeams(f.panel, cert, "")
	view := issueJoinCode(t, f)
	send := func(remote string, request serverSetupJoinRequest) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(request)
		req := httptest.NewRequest(http.MethodPost, serverSetupJoinPath, bytes.NewReader(raw))
		req.TLS, req.RemoteAddr = &tls.ConnectionState{}, remote
		w := httptest.NewRecorder()
		f.panel.requireServerSetupLinkMachine(http.NotFoundHandler()).ServeHTTP(w, req)
		return w
	}
	request := joinRequestFor(t, view.Code, strings.Repeat("a", 32))
	request.DeclaredIPv6 = "2001:db8::5"
	if w := send("[2001:db8::5]:40000", request); w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "server_setup_join_source_mismatch") {
		t.Fatalf("an IPv6 caller declaring an IPv4 address: %d %s", w.Code, w.Body.String())
	}
	for _, v6 := range []string{"fe80::1", "::1", "fd00::1", "not-an-address"} {
		bad := request
		bad.DeclaredIPv6 = v6
		if w := send("198.51.100.20:40000", bad); w.Code != http.StatusBadRequest {
			t.Fatalf("declared IPv6 %q: %d %s", v6, w.Code, w.Body.String())
		}
	}
	if w := send("198.51.100.20:40000", request); w.Code != http.StatusOK {
		t.Fatalf("the declared IPv4 caller joins: %d %s", w.Code, w.Body.String())
	}
	for _, address := range []string{"[2001:db8::1]:2083", "https://[2001:db8::1]:2083"} {
		if _, ok := canonicalServerSetupPeerAddress(address); ok {
			t.Fatalf("the first server is dialed at its IPv4 address only: %q", address)
		}
	}
}

// A new join withdraws what the earlier join handed over: its enrollment
// code and, once used, the publishing client it made.
func TestServerSetupJoinReplacedWithdrawsTheEarlierEnrollment(t *testing.T) {
	_, cert := joinTestCertificate(t)
	f, _, _ := joinFirstFixture(t, joinFirstDraft())
	joinTestSeams(f.panel, cert, "")
	view := issueJoinCode(t, f)
	w := joinMachineRequest(t, f, serverSetupJoinPath, "", joinRequestFor(t, view.Code, strings.Repeat("a", 32)))
	var first serverSetupJoinDocument
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &first) != nil {
		t.Fatalf("first join: %d %s", w.Code, w.Body.String())
	}
	// The second server enrolled with it.
	client := strings.Repeat("9", 32)
	db := f.database.GetDB()
	if _, err := db.Exec(`INSERT INTO remote_dns_clients(id,credential_hash,label,created_at,revoked) VALUES(?,?,?,?,0)`, client, remoteDNSHash(strings.Repeat("1", 64)), "old", "now"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE remote_dns_enrollments SET client_id=? WHERE code_hash=?`, client, remoteDNSHash(first.EnrollmentCode)); err != nil {
		t.Fatal(err)
	}
	f.panel.setupLink.limiter.reset()
	view = issueJoinCode(t, f)
	if w := joinMachineRequest(t, f, serverSetupJoinPath, "", joinRequestFor(t, view.Code, strings.Repeat("b", 32))); w.Code != http.StatusOK {
		t.Fatalf("new join: %d %s", w.Code, w.Body.String())
	}
	var old, revoked, replaced int
	db.QueryRow(`SELECT COUNT(*) FROM remote_dns_enrollments WHERE code_hash=?`, remoteDNSHash(first.EnrollmentCode)).Scan(&old)
	db.QueryRow(`SELECT revoked FROM remote_dns_clients WHERE id=?`, client).Scan(&revoked)
	db.QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action='server.setup.join_replaced'`).Scan(&replaced)
	if old != 0 || revoked != 1 || replaced != 1 {
		t.Fatalf("the old enrollment and its client are withdrawn: enrollment=%d revoked=%d audit=%d", old, revoked, replaced)
	}
}

// Mixed versions: a request of another link protocol is a verified refusal;
// a newer peer's extra fields are tolerated; a panel without the endpoint is
// an older version, never an unknown answer.
func TestServerSetupJoinOtherVersionsAreAKnownState(t *testing.T) {
	_, cert := joinTestCertificate(t)
	f, _, _ := joinFirstFixture(t, joinFirstDraft())
	joinTestSeams(f.panel, cert, "")
	view := issueJoinCode(t, f)
	request := joinRequestFor(t, view.Code, strings.Repeat("a", 32))
	older := request
	older.Protocol = 0
	if w := joinMachineRequest(t, f, serverSetupJoinPath, "", older); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "server_setup_peer_version_incompatible") {
		t.Fatalf("another protocol: %d %s", w.Code, w.Body.String())
	}
	newer := map[string]any{"protocol": serverSetupLinkProtocol, "proof": request.Proof, "request_id": request.RequestID, "declared_ipv4": request.DeclaredIPv4, "a_field_from_a_newer_panel": true}
	if w := joinMachineRequest(t, f, serverSetupJoinPath, "", newer); w.Code != http.StatusOK {
		t.Fatalf("extra fields are tolerated: %d %s", w.Code, w.Body.String())
	}
	served, leaf := joinTestCertificate(t)
	for _, test := range []struct {
		status int
		body   string
		want   string
	}{
		{http.StatusNotFound, "404 page not found", "server_setup_peer_version_incompatible"},
		{http.StatusOK, `{"protocol":1,"accepted":true,"later":"field"}`, ""},
	} {
		server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if test.status == http.StatusOK {
				w.Header().Set("Content-Type", "application/json")
			}
			w.WriteHeader(test.status)
			_, _ = w.Write([]byte(test.body))
		}))
		server.TLS = &tls.Config{Certificates: []tls.Certificate{served}}
		server.StartTLS()
		listener := server.Listener.Addr().String()
		caller := &Panel{}
		joinTestSeams(caller, leaf, "")
		joinTestDial(caller, func() string { return listener })
		var answer struct {
			Protocol int  `json:"protocol"`
			Accepted bool `json:"accepted"`
		}
		err := caller.serverSetupLinkJSON(context.Background(), serverSetupLinkTarget{Address: "https://203.0.113.10:2083", Pin: serverSetupSPKI(leaf)}, serverSetupPeerNamesPath, "x.y", map[string]string{}, &answer)
		var refusal *serverSetupLinkRefusal
		if test.want != "" && (!errors.As(err, &refusal) || refusal.Code != test.want) {
			t.Fatalf("%d: %v", test.status, err)
		}
		if test.want == "" && (err != nil || !answer.Accepted || answer.Protocol != 1) {
			t.Fatalf("tolerant decoding: %v %+v", err, answer)
		}
		server.Close()
	}
}

// Secrets at rest: the first server keeps only keyed hashes (under the key
// file, outside the database) of the code's secret and of the second
// server's credential; the second server keeps the code only while the
// join's outcome is unknown, sealed.
func TestServerSetupJoinKeepsOnlyKeyedHashes(t *testing.T) {
	pair := newPeerPair(t, joinFirstDraft())
	box, err := secrets.LoadOrCreate(filepath.Join(t.TempDir(), "secret.key"))
	if err != nil {
		t.Fatal(err)
	}
	pair.first.panel.secrets, pair.second.panel.secrets = box, box
	view := issueJoinCode(t, pair.first)
	_, proof, _ := parseServerSetupJoinCode(view.Code)
	var record serverSetupJoinCodeRecord
	readServerSetupJSONSetting(context.Background(), pair.first.database.GetDB(), serverSetupJoinCodeKey, &record)
	plain := sha256.Sum256([]byte(record.Salt + ":" + proof))
	if record.Hash != box.MAC("server-setup/join-proof", record.Salt+":"+proof) || record.Hash == hex.EncodeToString(plain[:]) {
		t.Fatal("the code's secret is kept as a keyed hash")
	}
	pair.dropAnswer.Store(true)
	pair.connect(t, view.Code)
	var raw string
	pair.second.database.GetDB().QueryRow(`SELECT value FROM panel_settings WHERE key=?`, serverSetupLinkPendingKey).Scan(&raw)
	if raw == "" || strings.Contains(raw, strings.ReplaceAll(view.Code, "-", "")) || strings.Contains(raw, view.Code) {
		t.Fatalf("the unknown join's code is kept sealed: %q", raw)
	}
	if answer := pair.connect(t, view.Code); answer.State != "ok" {
		t.Fatalf("retry: %+v", answer)
	}
	var left int
	pair.second.database.GetDB().QueryRow(`SELECT COUNT(*) FROM panel_settings WHERE key=?`, serverSetupLinkPendingKey).Scan(&left)
	link, _ := pair.second.panel.readServerSetupLink(context.Background())
	var join serverSetupJoinRecord
	readServerSetupJSONSetting(context.Background(), pair.first.database.GetDB(), serverSetupJoinKey, &join)
	if left != 0 || join.LinkCredentialHash != box.MAC("server-setup/link", link.LinkCredential) {
		t.Fatalf("pending gone, credential hashed: %d %+v", left, join)
	}
	stored, _ := json.Marshal(join)
	if strings.Contains(string(stored), link.LinkCredential) {
		t.Fatal("the first server keeps no plain credential of the second")
	}
}

// Two joins at the same moment with the same code: one joins, the other is
// told the code is used; there is never a second join.
func TestServerSetupJoinConcurrentJoinsWithOneCode(t *testing.T) {
	_, cert := joinTestCertificate(t)
	f, _, _ := joinFirstFixture(t, joinFirstDraft())
	joinTestSeams(f.panel, cert, "")
	view := issueJoinCode(t, f)
	codes := make(chan int, 2)
	for _, id := range []string{strings.Repeat("a", 32), strings.Repeat("b", 32)} {
		go func(id string) {
			codes <- joinMachineRequest(t, f, serverSetupJoinPath, "", joinRequestFor(t, view.Code, id)).Code
		}(id)
	}
	got := []int{<-codes, <-codes}
	slices.Sort(got)
	if got[0] != http.StatusOK || got[1] != http.StatusGone {
		t.Fatalf("one join, one refusal: %v", got)
	}
}

// A replayed join whose enrollment the second server already used is still
// a valid document (no new code, said so).
func TestServerSetupJoinReplayAfterAUsedEnrollmentIsValid(t *testing.T) {
	_, cert := joinTestCertificate(t)
	f, _, _ := joinFirstFixture(t, joinFirstDraft())
	joinTestSeams(f.panel, cert, "")
	view := issueJoinCode(t, f)
	request := joinRequestFor(t, view.Code, strings.Repeat("a", 32))
	var first serverSetupJoinDocument
	json.Unmarshal(joinMachineRequest(t, f, serverSetupJoinPath, "", request).Body.Bytes(), &first)
	if _, err := f.database.GetDB().Exec(`UPDATE remote_dns_enrollments SET client_id=? WHERE code_hash=?`, strings.Repeat("9", 32), remoteDNSHash(first.EnrollmentCode)); err != nil {
		t.Fatal(err)
	}
	var again serverSetupJoinDocument
	if w := joinMachineRequest(t, f, serverSetupJoinPath, "", request); w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &again) != nil || again.EnrollmentCode != "" || !again.EnrollmentUsed {
		t.Fatalf("replay: %d %s", w.Code, w.Body.String())
	}
	if !validServerSetupJoinDocument(again, "https://203.0.113.10:2083", request.RequestID) {
		t.Fatalf("the replayed document is valid: %+v", again)
	}
}

// SHOULD-2 (alpha.114 review): an ended join is final. A replay of the same
// request after the setup completed (link revoked, code removed), after the
// setup stopped, or once the code expired hands out no credential and no
// enrollment.
func TestServerSetupJoinReplayAfterTheJoinEndedGetsNothing(t *testing.T) {
	for _, end := range []string{"completed", "stopped", "expired"} {
		t.Run(end, func(t *testing.T) {
			_, cert := joinTestCertificate(t)
			f, _, execution := joinFirstFixture(t, joinFirstDraft())
			joinTestSeams(f.panel, cert, "")
			view := issueJoinCode(t, f)
			request := joinRequestFor(t, view.Code, strings.Repeat("a", 32))
			if w := joinMachineRequest(t, f, serverSetupJoinPath, "", request); w.Code != http.StatusOK {
				t.Fatalf("join: %d %s", w.Code, w.Body.String())
			}
			db := f.database.GetDB()
			var before serverSetupJoinRecord
			readServerSetupJSONSetting(context.Background(), db, serverSetupJoinKey, &before)
			want := "server_setup_join_code_used"
			switch end {
			case "completed":
				if err := f.panel.revokeServerSetupLink(context.Background()); err != nil {
					t.Fatal(err)
				}
				readServerSetupJSONSetting(context.Background(), db, serverSetupJoinKey, &before)
				want = "server_setup_join_code_unavailable"
			case "stopped":
				if _, err := db.Exec(`UPDATE server_setup_executions SET status='failed',execution_json=replace(execution_json,'"status":"running"','"status":"failed"') WHERE id=?`, execution.ID); err != nil {
					t.Fatal(err)
				}
			case "expired":
				var record serverSetupJoinCodeRecord
				readServerSetupJSONSetting(context.Background(), db, serverSetupJoinCodeKey, &record)
				record.ExpiresAt = time.Now().Add(-time.Minute).Unix()
				writeServerSetupJSONSetting(context.Background(), db, serverSetupJoinCodeKey, record)
				want = "server_setup_join_code_expired"
			}
			var enrollments int
			db.QueryRow(`SELECT COUNT(*) FROM remote_dns_enrollments`).Scan(&enrollments)
			w := joinMachineRequest(t, f, serverSetupJoinPath, "", request)
			if w.Code != http.StatusGone || !strings.Contains(w.Body.String(), want) || strings.Contains(w.Body.String(), "link_credential") || strings.Contains(w.Body.String(), "enrollment_code") {
				t.Fatalf("replay: %d %s", w.Code, w.Body.String())
			}
			var after serverSetupJoinRecord
			readServerSetupJSONSetting(context.Background(), db, serverSetupJoinKey, &after)
			var enrollmentsAfter int
			db.QueryRow(`SELECT COUNT(*) FROM remote_dns_enrollments`).Scan(&enrollmentsAfter)
			if after.LinkCredentialHash != before.LinkCredentialHash || after.LinkID != before.LinkID || enrollmentsAfter != enrollments {
				t.Fatalf("nothing new is issued: %+v %d %d", after, enrollments, enrollmentsAfter)
			}
		})
	}
}
