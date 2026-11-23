package membership

// The panel API contract of http.php, exercised in process against the Go handler:
// methods, sizes, content type, closed request fields, the bound address taken only
// from the web server, the signing-key refusal, and receipts the panel's own
// licensing.Verify accepts. Keys are made here; nothing leaves the loopback.

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/licensing"
)

// writeFixture prepares a private directory like portal-membership/tests/prepare.php:
// a throwaway signing key, the loopback origin and the test mail folder, and the
// empty database an initialisation makes (the service itself never creates it).
func writeFixture(t testing.TB, dir, origin string) ed25519.PublicKey {
	t.Helper()
	public := writeConfigOnly(t, dir, origin)
	config, err := LoadConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	s, err := Open(filepath.Join(dir, "members.sqlite"), config.SigningKey, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	return public
}

// writeConfigOnly is writeFixture without the database.
func writeConfigOnly(t testing.TB, dir, origin string) ed25519.PublicKey {
	t.Helper()
	public, secret, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{dir, filepath.Join(dir, "sessions"), filepath.Join(dir, "mail")} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	config, _ := json.Marshal(map[string]string{"origin": origin, "sender": "noreply@celikpanel.net",
		"signing_key": base64.StdEncoding.EncodeToString(secret), "test_mail_dir": filepath.Join(dir, "mail")})
	if err := os.WriteFile(filepath.Join(dir, "config.json"), config, 0o600); err != nil {
		t.Fatal(err)
	}
	return public
}

// TestPrepareHTTPFixture writes the private directory deploy/test-membership-go.sh
// serves to portal-membership/tests/http_flow_test.py (unchanged), and the
// throwaway public key the server is told to accept. It does nothing otherwise.
func TestPrepareHTTPFixture(t *testing.T) {
	dir := os.Getenv("CELIKPANEL_MEMBERSHIP_PREPARE")
	if dir == "" {
		t.Skip("used by deploy/test-membership-go.sh")
	}
	public := writeFixture(t, dir, "http://127.0.0.1:8379")
	if err := os.WriteFile(dir+".public-key", []byte(hex.EncodeToString(public)), 0o600); err != nil {
		t.Fatal(err)
	}
}

type apiServer struct {
	t      *testing.T
	dir    string
	url    string
	public ed25519.PublicKey
	peer   string
}

func newAPIServer(t *testing.T) *apiServer {
	t.Helper()
	a := &apiServer{t: t, dir: t.TempDir(), peer: "127.0.0.1"}
	handler := &Handler{Private: a.dir, RemoteAddr: func(*http.Request) string { return a.peer }}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	a.url = server.URL + "/account/"
	a.public = writeFixture(t, a.dir, server.URL)
	handler.AllowTestKey = a.public
	return a
}

func (a *apiServer) call(method, action, contentType string, body []byte) (int, http.Header, string) {
	a.t.Helper()
	request, err := http.NewRequest(method, a.url+"?action="+action, bytes.NewReader(body))
	if err != nil {
		a.t.Fatal(err)
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		a.t.Fatal(err)
	}
	defer response.Body.Close()
	raw, _ := io.ReadAll(response.Body)
	return response.StatusCode, response.Header, string(raw)
}

// issueKey makes a verified member with one licence directly in the database.
func (a *apiServer) issueKey() (Issued, *Service) {
	a.t.Helper()
	config, err := LoadConfig(a.dir)
	if err != nil {
		a.t.Fatal(err)
	}
	var token string
	s, err := Open(filepath.Join(a.dir, "members.sqlite"), config.SigningKey, func(_, _, tk string) error { token = tk; return nil }, nil)
	if err != nil {
		a.t.Fatal(err)
	}
	a.t.Cleanup(func() { s.Close() })
	if err := s.Register("api@example.com"); err != nil {
		a.t.Fatal(err)
	}
	if err := s.ConsumeToken(token, "verify", "api-test-password-123"); err != nil {
		a.t.Fatal(err)
	}
	m := must(s.Login("api@example.com", "api-test-password-123"))
	return must(s.Issue(m.ID)), s
}

func TestPanelAPIContract(t *testing.T) {
	a := newAPIServer(t)
	issued, s := a.issueKey()
	server := strings.Repeat("a", 64)
	activate := func(fields map[string]string) (int, string) {
		body, _ := json.Marshal(fields)
		status, _, text := a.call(http.MethodPost, "activate", "application/json", body)
		return status, text
	}

	status, header, body := a.call(http.MethodGet, "activate", "", nil)
	if status != 405 || header.Get("Allow") != "POST" || body != `{"error":"method_not_allowed"}` || header.Get("Content-Type") != "application/json" {
		t.Fatalf("GET: %d %q %q", status, header, body)
	}
	for _, name := range []string{"Cache-Control", "X-Content-Type-Options", "Referrer-Policy", "Content-Security-Policy"} {
		if header.Get(name) == "" {
			t.Fatalf("missing %s", name)
		}
	}
	if status, _, body = a.call(http.MethodPost, "activate", "text/plain", []byte(`{}`)); status != 400 || body != `{"error":"invalid_request"}` {
		t.Fatalf("content type: %d %s", status, body)
	}
	if status, body = activate(map[string]string{"key": issued.Key, "server_id": server, "hostname": "h.example.com", "server_ip": "9.9.9.9"}); status != 400 || body != `{"error":"invalid_request"}` {
		t.Fatalf("client cannot supply binding IP: %d %s", status, body)
	}
	if status, _, body = a.call(http.MethodPost, "activate", "application/json", bytes.Repeat([]byte(" "), 8193)); status != 413 || body != "" {
		t.Fatalf("over 8192 bytes: %d %q", status, body)
	}
	for raw, want := range map[string]string{
		`[]`:                  `{"error":"invalid_activation"}`,
		`["x"]`:               `{"error":"invalid_request"}`,
		`{"key":1}`:           `{"error":"invalid_request"}`,
		`{"key":"\ud800"}`:    `{"error":"invalid_request"}`,
		"{\"key\":\"\xff\"}":  `{"error":"invalid_request"}`,
		`{} {}`:               `{"error":"invalid_request"}`,
		`null`:                `{"error":"invalid_request"}`,
		``:                    `{"error":"invalid_request"}`,
		`{"key":"CPK-x"}`:     `{"error":"invalid_activation"}`,
		`{"license_id":"ab"}`: `{"error":"invalid_request"}`,
	} {
		if status, _, body = a.call(http.MethodPost, "activate", "Application/JSON ; charset=utf-8", []byte(raw)); body != want {
			t.Errorf("%q: %d %s, want %s", raw, status, body, want)
		}
	}
	// The address is the web server's; loopback becomes 8.8.8.8 only in the
	// loopback test configuration (http.php's test hook).
	status, body = activate(map[string]string{"key": issued.Key, "server_id": server, "hostname": "h.example.com"})
	if status != 200 || strings.Contains(body, `"payload":"`) == false {
		t.Fatalf("activation: %d %s", status, body)
	}
	if strings.Contains(body, "/") && !strings.Contains(body, `\/`) {
		t.Fatalf("slashes must be escaped like PHP's json_encode: %s", body)
	}
	var envelope licensing.Envelope
	if err := json.Unmarshal([]byte(body), &envelope); err != nil {
		t.Fatal(err)
	}
	claims, err := licensing.Verify(envelope, a.public, server, time.Now())
	if err != nil || claims.LicenseID != issued.ID {
		t.Fatalf("the panel's Verify refused the Go receipt: %v", err)
	}
	var bound string
	if err := s.DB().QueryRow("SELECT server_ip FROM licenses WHERE id=?", issued.ID).Scan(&bound); err != nil || bound != "8.8.8.8" {
		t.Fatalf("bound %q %v", bound, err)
	}
	refresh, _ := json.Marshal(map[string]string{"license_id": issued.ID, "server_id": server, "activation_token": envelope.ActivationToken})
	if status, _, body = a.call(http.MethodPost, "refresh", "application/json", refresh); status != 200 {
		t.Fatalf("refresh: %d %s", status, body)
	}
	a.peer = "10.0.0.1"
	if status, _, body = a.call(http.MethodPost, "refresh", "application/json", refresh); status != 400 || body != `{"error":"invalid_server_ip"}` {
		t.Fatalf("private address: %d %s", status, body)
	}
	a.peer = "9.9.9.9"
	if status, _, body = a.call(http.MethodPost, "refresh", "application/json", refresh); status != 400 || body != `{"error":"license_in_use"}` {
		t.Fatalf("other address: %d %s", status, body)
	}
	a.peer = "203.0.113.9"
	for i := 0; i < 60; i++ {
		a.call(http.MethodPost, "refresh", "application/json", refresh)
	}
	if status, _, body = a.call(http.MethodPost, "refresh", "application/json", refresh); status != 400 || body != `{"error":"rate_limited"}` {
		t.Fatalf("61st request a minute: %d %s", status, body)
	}
}

func TestUnpinnedKeyIsRefused(t *testing.T) {
	dir := t.TempDir()
	server := httptest.NewServer(&Handler{Private: dir, RemoteAddr: PeerAddress})
	defer server.Close()
	writeConfigOnly(t, dir, server.URL) // a throwaway key nobody named
	for _, action := range []string{"activate", "home"} {
		response, err := http.Post(server.URL+"/account/?action="+action, "application/json", strings.NewReader(`{}`))
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		if response.StatusCode != 503 {
			t.Fatalf("%s with an unpinned key: %d %s", action, response.StatusCode, body)
		}
		if action == "activate" && string(body) != `{"error":"service_unavailable"}` {
			t.Fatalf("API body %s", body)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "members.sqlite")); err == nil {
		t.Fatal("the database was opened with an unpinned key")
	}
	if err := CheckSigningKey(ed25519.NewKeyFromSeed(make([]byte, 32)), nil); err != ErrUnpinnedKey {
		t.Fatalf("CheckSigningKey: %v", err)
	}
	// A secret whose halves disagree is refused even if its second half is pinned.
	forged := append(append([]byte{}, make([]byte, 32)...), ProductionPublicKey()...)
	if err := CheckSigningKey(forged, nil); err == nil || err == ErrUnpinnedKey {
		t.Fatalf("mismatched halves: %v", err)
	}
}

func TestPagesHeadersAndRefusals(t *testing.T) {
	a := newAPIServer(t)
	status, header, body := a.call(http.MethodGet, "register", "", nil)
	if status != 200 || !strings.Contains(body, "<form") || !strings.Contains(header.Get("Cache-Control"), "no-store") ||
		!strings.Contains(header.Get("Content-Security-Policy"), "form-action 'self'") || header.Get("Content-Type") != "text/html; charset=UTF-8" {
		t.Fatalf("register page: %d %v", status, header)
	}
	cookie := header.Get("Set-Cookie")
	if !strings.HasPrefix(cookie, "celikpanel_member_dev=") || !strings.Contains(cookie, "HttpOnly") || !strings.Contains(cookie, "SameSite=Lax") {
		t.Fatalf("session cookie %q", cookie)
	}
	if status, _, _ = a.call(http.MethodPut, "home", "", nil); status != 405 {
		t.Fatalf("PUT: %d", status)
	}
	request, _ := http.NewRequest(http.MethodGet, a.url+"?action[]=home", nil)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	text, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != 503 || string(text) != unavailableText {
		t.Fatalf("an array action: %d %q", response.StatusCode, text)
	}
}
