package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/secrets"
	"github.com/alicelik/celikpanel/internal/transport"
)

type relayFixture struct {
	p       *Panel
	applied []transport.MailRelayApplyRequest
	answer  transport.MailRelayApplyResponse
	status  transport.MailRelayStatus
}

func newRelayFixture(t *testing.T) *relayFixture {
	t.Helper()
	database, err := openTemplatePanelDatabase(filepath.Join(t.TempDir(), "panel.sqlite"))
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(database.Close)
	box, err := secrets.New(bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	f := &relayFixture{p: &Panel{db: database, secrets: box}, answer: transport.MailRelayApplyResponse{Applied: true, Changed: true}}
	savedApply, savedStatus := mailRelayAgentApply, mailRelayAgentStatus
	t.Cleanup(func() { mailRelayAgentApply, mailRelayAgentStatus = savedApply, savedStatus })
	mailRelayAgentApply = func(_ *Panel, _ context.Context, req *transport.MailRelayApplyRequest) (transport.MailRelayApplyResponse, error) {
		f.applied = append(f.applied, *req)
		return f.answer, nil
	}
	mailRelayAgentStatus = func(*Panel, context.Context) (transport.MailRelayStatus, error) { return f.status, nil }
	return f
}

func (f *relayFixture) post(t *testing.T, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, _ := json.Marshal(body)
	recorder := httptest.NewRecorder()
	f.p.handleMailRelay(recorder, httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw)))
	return recorder
}

func (f *relayFixture) row(t *testing.T) mailRelayRow {
	t.Helper()
	row, err := f.p.readMailRelay(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return row
}

func TestMailRelaySavesTheSealedPasswordAndNeverSendsItBack(t *testing.T) {
	f := newRelayFixture(t)
	rec := f.post(t, mailRelayPath, map[string]any{"enabled": true, "host": "Relay.Example.Test.", "port": 587, "username": "sender", "password": "Pa55 word:1"})
	if rec.Code != http.StatusOK {
		t.Fatalf("save = %d %s", rec.Code, rec.Body)
	}
	if len(f.applied) != 1 || f.applied[0].Password != "Pa55 word:1" || f.applied[0].Host != "relay.example.test" || !f.applied[0].Enabled {
		t.Fatalf("agent request = %+v", f.applied)
	}
	row := f.row(t)
	if row.sealed == "" || strings.Contains(row.sealed, "Pa55") || row.apply.State != "applied" {
		t.Fatalf("row = %+v", row)
	}
	get := httptest.NewRecorder()
	f.p.handleMailRelay(get, httptest.NewRequest(http.MethodGet, mailRelayPath, nil))
	if strings.Contains(get.Body.String(), "Pa55") || !strings.Contains(get.Body.String(), `"has_password":true`) {
		t.Fatalf("view = %s", get.Body)
	}
	// Saving again with an empty password keeps and sends the saved one.
	if rec := f.post(t, mailRelayPath, map[string]any{"enabled": true, "host": "relay.example.test", "port": 465, "username": "sender"}); rec.Code != http.StatusOK {
		t.Fatalf("resave = %d %s", rec.Code, rec.Body)
	}
	if f.applied[1].Password != "Pa55 word:1" || f.applied[1].Port != 465 {
		t.Fatalf("resave request = %+v", f.applied[1])
	}
}

func TestMailRelayNeverSendsTheSavedPasswordToAnotherServer(t *testing.T) {
	f := newRelayFixture(t)
	if rec := f.post(t, mailRelayPath, map[string]any{"enabled": true, "host": "relay.example.test", "port": 587, "username": "sender"}); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), errCodeMailRelayPasswordRequired) {
		t.Fatalf("no password = %d %s", rec.Code, rec.Body)
	}
	f.post(t, mailRelayPath, map[string]any{"enabled": true, "host": "relay.example.test", "port": 587, "username": "sender", "password": "secret-1"})
	for _, body := range []map[string]any{
		{"enabled": true, "host": "other.example.test", "port": 587, "username": "sender"},
		{"enabled": true, "host": "relay.example.test", "port": 587, "username": "someone"},
	} {
		if rec := f.post(t, mailRelayPath, body); rec.Code != http.StatusBadRequest {
			t.Fatalf("%v = %d %s", body, rec.Code, rec.Body)
		}
		if rec := f.post(t, mailRelayTestPath, body); rec.Code != http.StatusBadRequest {
			t.Fatalf("test %v = %d %s", body, rec.Code, rec.Body)
		}
	}
	if len(f.applied) != 1 {
		t.Fatalf("the agent was asked %d times", len(f.applied))
	}
}

func TestMailRelayTurnedOffClearsThePassword(t *testing.T) {
	f := newRelayFixture(t)
	f.post(t, mailRelayPath, map[string]any{"enabled": true, "host": "relay.example.test", "port": 587, "username": "sender", "password": "secret-1"})
	if rec := f.post(t, mailRelayPath, map[string]any{"enabled": false}); rec.Code != http.StatusOK {
		t.Fatalf("off = %d %s", rec.Code, rec.Body)
	}
	last := f.applied[len(f.applied)-1]
	if last.Enabled || last.Password != "" || last.Host != "" {
		t.Fatalf("off request = %+v", last)
	}
	row := f.row(t)
	if row.settings.Enabled || row.sealed != "" || row.settings.Host != "relay.example.test" {
		t.Fatalf("row after off = %+v", row)
	}
}

func TestMailRelayRecordsTheServersRefusal(t *testing.T) {
	f := newRelayFixture(t)
	f.answer = transport.MailRelayApplyResponse{Code: transport.MailRelayOwnerConfig, Error: "main.cf already has the server owner's relay setting"}
	rec := f.post(t, mailRelayPath, map[string]any{"enabled": true, "host": "relay.example.test", "port": 587, "username": "sender", "password": "secret-1"})
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), transport.MailRelayOwnerConfig) {
		t.Fatalf("save = %d %s", rec.Code, rec.Body)
	}
	if row := f.row(t); row.apply.State != "refused" || row.apply.Code != transport.MailRelayOwnerConfig {
		t.Fatalf("row = %+v", row.apply)
	}
	for _, bad := range []map[string]any{
		{"enabled": true, "host": "127.0.0.1", "port": 587, "username": "u", "password": "p"},
		{"enabled": true, "host": "relay.example.test", "port": 26, "username": "u", "password": "p"},
		{"enabled": true, "host": "relay.example.test", "port": 587, "username": "u", "password": "p\nq"},
	} {
		if rec := f.post(t, mailRelayPath, bad); rec.Code != http.StatusBadRequest {
			t.Fatalf("%v = %d", bad, rec.Code)
		}
	}
}

// ---- The sign-in test against an SMTP double ------------------------------------

type fakeRelay struct {
	cert        tls.Certificate
	implicitTLS bool
	noSTARTTLS  bool
	user, pass  string
	mu          sync.Mutex
	commands    []string
}

func newRelayCertificate(t *testing.T, name string) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: name}, DNSNames: []string{name},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ := x509.ParseCertificate(der)
	pool := x509.NewCertPool()
	pool.AddCert(parsed)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, pool
}

func (r *fakeRelay) serve(conn net.Conn) {
	defer conn.Close()
	if r.implicitTLS {
		conn = tls.Server(conn, &tls.Config{Certificates: []tls.Certificate{r.cert}})
	}
	reader := bufio.NewReader(conn)
	write := func(s string) { _, _ = conn.Write([]byte(s + "\r\n")) }
	write("220 relay.example.test ESMTP")
	secure := r.implicitTLS
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		r.mu.Lock()
		r.commands = append(r.commands, line)
		r.mu.Unlock()
		upper := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(upper, "EHLO"):
			if !secure && !r.noSTARTTLS {
				write("250-relay.example.test")
				write("250 STARTTLS")
			} else if secure {
				write("250-relay.example.test")
				write("250 AUTH LOGIN PLAIN")
			} else {
				write("250 relay.example.test")
			}
		case upper == "STARTTLS":
			write("220 go ahead")
			conn = tls.Server(conn, &tls.Config{Certificates: []tls.Certificate{r.cert}})
			reader = bufio.NewReader(conn)
			secure = true
		case strings.HasPrefix(upper, "AUTH PLAIN "):
			raw, _ := base64.StdEncoding.DecodeString(strings.TrimSpace(line[len("AUTH PLAIN "):]))
			if string(raw) == "\x00"+r.user+"\x00"+r.pass {
				write("235 ok")
			} else {
				write("535 authentication failed")
			}
		case upper == "QUIT":
			write("221 bye")
			return
		default:
			write("502 not here")
		}
	}
}

func (r *fakeRelay) use(t *testing.T) {
	t.Helper()
	saved, savedRoots := mailRelayDial, mailRelayTestRoots
	t.Cleanup(func() { mailRelayDial, mailRelayTestRoots = saved, savedRoots })
	mailRelayDial = func(context.Context, string) (net.Conn, error) {
		client, server := net.Pipe()
		go r.serve(server)
		return client, nil
	}
}

func TestMailRelaySignInTestSendsNoMessage(t *testing.T) {
	cert, pool := newRelayCertificate(t, "relay.example.test")
	for name, tc := range map[string]struct {
		relay     *fakeRelay
		port      int
		password  string
		wantOK    bool
		wantStage string
	}{
		"starttls":       {relay: &fakeRelay{cert: cert, user: "sender", pass: "right"}, port: 587, password: "right", wantOK: true, wantStage: "done"},
		"implicit tls":   {relay: &fakeRelay{cert: cert, implicitTLS: true, user: "sender", pass: "right"}, port: 465, password: "right", wantOK: true, wantStage: "done"},
		"wrong password": {relay: &fakeRelay{cert: cert, user: "sender", pass: "right"}, port: 587, password: "wrong", wantStage: "auth"},
		"no encryption":  {relay: &fakeRelay{cert: cert, noSTARTTLS: true, user: "sender", pass: "right"}, port: 587, password: "right", wantStage: "tls"},
	} {
		t.Run(name, func(t *testing.T) {
			f := newRelayFixture(t)
			relay := tc.relay
			relay.use(t)
			mailRelayTestRoots = pool
			rec := f.post(t, mailRelayTestPath, map[string]any{"host": "relay.example.test", "port": tc.port, "username": "sender", "password": tc.password})
			if rec.Code != http.StatusOK {
				t.Fatalf("test = %d %s", rec.Code, rec.Body)
			}
			var answer mailRelayTestAnswer
			if err := json.Unmarshal(rec.Body.Bytes(), &answer); err != nil {
				t.Fatal(err)
			}
			if answer.OK != tc.wantOK || answer.Stage != tc.wantStage {
				t.Fatalf("answer = %+v", answer)
			}
			if tc.wantOK && !answer.Verified {
				t.Fatalf("the certificate was not verified: %+v", answer)
			}
			if strings.Contains(rec.Body.String(), tc.password) && tc.password != "" {
				t.Fatalf("the password reached the answer: %s", rec.Body)
			}
			relay.mu.Lock()
			defer relay.mu.Unlock()
			for _, command := range relay.commands {
				upper := strings.ToUpper(command)
				if strings.HasPrefix(upper, "MAIL") || strings.HasPrefix(upper, "RCPT") || strings.HasPrefix(upper, "DATA") {
					t.Fatalf("the test sent a message command: %q", command)
				}
			}
			if len(f.applied) != 0 {
				t.Fatal("the test changed the server")
			}
		})
	}
}
