package main

// The entry points as a web server and the owner run them: the test binary re-executes
// itself as the membership program (CELIKPANEL_MEMBERSHIP_RUN_MAIN), so CGI gets its
// request from the environment and standard input exactly as under a web server. Keys
// are throwaway keys made here.

import (
	"bufio"
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/membership"
)

func TestMain(m *testing.M) {
	if os.Getenv("CELIKPANEL_MEMBERSHIP_RUN_MAIN") == "1" {
		os.Args = append([]string{"membership"}, strings.Fields(os.Getenv("CELIKPANEL_MEMBERSHIP_ARGS"))...)
		main()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

type fixture struct {
	dir    string
	public ed25519.PublicKey
	secret ed25519.PrivateKey
	key    string
}

// newFixture writes a private directory with a throwaway key and one verified member
// with one licence.
func newFixture(t *testing.T, origin string) fixture {
	t.Helper()
	public, secret, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	f := fixture{dir: t.TempDir(), public: public, secret: secret}
	for _, p := range []string{filepath.Join(f.dir, "sessions"), filepath.Join(f.dir, "mail")} {
		if err := os.MkdirAll(p, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	config, _ := json.Marshal(map[string]string{"origin": origin, "sender": "noreply@celikpanel.net",
		"signing_key": base64.StdEncoding.EncodeToString(secret)})
	if err := os.WriteFile(filepath.Join(f.dir, "config.json"), config, 0o600); err != nil {
		t.Fatal(err)
	}
	var token string
	s, err := membership.Open(filepath.Join(f.dir, "members.sqlite"), secret, func(_, _, tk string) error { token = tk; return nil }, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Register("owner@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := s.ConsumeToken(token, "verify", "entry-point-password-123"); err != nil {
		t.Fatal(err)
	}
	m, err := s.Login("owner@example.com", "entry-point-password-123")
	if err != nil {
		t.Fatal(err)
	}
	issued, err := s.Issue(m.ID)
	if err != nil {
		t.Fatal(err)
	}
	f.key = issued.Key
	if _, err := s.Activate(issued.Key, strings.Repeat("a", 64), "bound.example.com", "8.8.8.8"); err != nil {
		t.Fatal(err)
	}
	return f
}

func run(t *testing.T, env []string, args string, stdin []byte) (int, string, string) {
	t.Helper()
	command := exec.Command(os.Args[0], "-test.run=^$")
	command.Env = append([]string{"CELIKPANEL_MEMBERSHIP_RUN_MAIN=1", "CELIKPANEL_MEMBERSHIP_ARGS=" + args, "PATH=" + os.Getenv("PATH")}, env...)
	command.Stdin = bytes.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	code := 0
	if exit, ok := err.(*exec.ExitError); ok {
		code = exit.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return code, stdout.String(), stderr.String()
}

func cgiEnv(f fixture, method, query, remote string, body []byte) []string {
	return []string{
		"CELIKPANEL_MEMBERSHIP_PRIVATE=" + f.dir, "GATEWAY_INTERFACE=CGI/1.1", "SERVER_PROTOCOL=HTTP/1.1",
		"REQUEST_METHOD=" + method, "QUERY_STRING=" + query, "REQUEST_URI=/account/?" + query, "SCRIPT_NAME=/account/",
		"SERVER_NAME=account.example", "SERVER_PORT=443", "HTTPS=on", "REMOTE_ADDR=" + remote, "REMOTE_PORT=40000",
		"CONTENT_TYPE=application/json", "CONTENT_LENGTH=" + strconv.Itoa(len(body)),
	}
}

func parseCGI(t *testing.T, out string) (int, string) {
	t.Helper()
	reader := bufio.NewReader(strings.NewReader(out))
	status := 200
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("no CGI header end in %q", out)
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if value, ok := strings.CutPrefix(line, "Status: "); ok {
			status, _ = strconv.Atoi(strings.Fields(value)[0])
		}
	}
	body, _ := io.ReadAll(reader)
	return status, string(body)
}

func TestCGIActivationUsesTheWebServersAddress(t *testing.T) {
	f := newFixture(t, "https://account.example")
	testKey := "CELIKPANEL_MEMBERSHIP_TEST_SIGNING_PUBLIC_KEY=" + hex.EncodeToString(f.public)
	body := []byte(`{"key":"` + f.key + `","server_id":"` + strings.Repeat("a", 64) + `","hostname":"bound.example.com"}`)

	_, out, _ := run(t, append(cgiEnv(f, http.MethodPost, "action=activate", "8.8.8.8", body), testKey), "", body)
	status, answer := parseCGI(t, out)
	if status != 200 || !strings.Contains(answer, `"activation_token"`) {
		t.Fatalf("activation from the bound address: %d %s", status, answer)
	}
	_, out, _ = run(t, append(cgiEnv(f, http.MethodPost, "action=activate", "10.0.0.1", body), testKey), "", body)
	if status, answer = parseCGI(t, out); status != 400 || answer != `{"error":"invalid_server_ip"}` {
		t.Fatalf("a private REMOTE_ADDR: %d %s", status, answer)
	}
	_, out, _ = run(t, append(cgiEnv(f, http.MethodPost, "action=activate", "9.9.9.9", body), testKey), "", body)
	if status, answer = parseCGI(t, out); status != 400 || answer != `{"error":"license_in_use"}` {
		t.Fatalf("another address: %d %s", status, answer)
	}
}

func TestCGIIgnoresTheCommandLineAndRefusesAnUnpinnedKey(t *testing.T) {
	f := newFixture(t, "https://account.example")
	body := []byte(`{}`)
	// A web server passes an isindex query ("?-test-signing-public-key+<hex>") as argv. The
	// program must not read it: without the environment variable the key stays refused.
	hostile := "-test-signing-public-key " + hex.EncodeToString(f.public)
	_, out, stderr := run(t, cgiEnv(f, http.MethodPost, "action=activate", "8.8.8.8", body), hostile, body)
	status, answer := parseCGI(t, out)
	if status != 503 || answer != `{"error":"service_unavailable"}` {
		t.Fatalf("an unpinned key under CGI: %d %s %s", status, answer, stderr)
	}
	if strings.Contains(stderr, f.key) || strings.Contains(stderr, base64.StdEncoding.EncodeToString(f.secret)) {
		t.Fatal("the log carries a secret")
	}
	// Naming the production key as a "test" key is refused outright.
	production := "CELIKPANEL_MEMBERSHIP_TEST_SIGNING_PUBLIC_KEY=" + hex.EncodeToString(membership.ProductionPublicKey())
	if code, _, _ := run(t, append(cgiEnv(f, http.MethodPost, "action=activate", "8.8.8.8", body), production), "", body); code == 0 {
		t.Fatal("the production key was accepted as a test key")
	}
}

// Review finding 1: the private directory is named explicitly and a missing database
// is never created: CGI answers 503 (panels keep their receipts), serve refuses to
// start.
func TestNoDatabaseIsNeverCreated(t *testing.T) {
	f := newFixture(t, "https://account.example")
	testKey := "CELIKPANEL_MEMBERSHIP_TEST_SIGNING_PUBLIC_KEY=" + hex.EncodeToString(f.public)
	if err := os.Remove(filepath.Join(f.dir, "members.sqlite")); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		_ = os.Remove(filepath.Join(f.dir, "members.sqlite"+suffix))
	}
	body := []byte(`{"key":"` + f.key + `","server_id":"` + strings.Repeat("a", 64) + `","hostname":"bound.example.com"}`)
	_, out, _ := run(t, append(cgiEnv(f, http.MethodPost, "action=activate", "8.8.8.8", body), testKey), "", body)
	if status, answer := parseCGI(t, out); status != 503 || answer != `{"error":"service_unavailable"}` {
		t.Fatalf("CGI without the database: %d %s", status, answer)
	}
	env := []string{"CELIKPANEL_MEMBERSHIP_PRIVATE=" + f.dir}
	if code, _, stderr := run(t, env, "serve -listen 127.0.0.1:0 -test-signing-public-key "+hex.EncodeToString(f.public), nil); code != 1 ||
		!strings.Contains(stderr, "nothing was created") {
		t.Fatalf("serve without the database: %d %s", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(f.dir, "members.sqlite")); !os.IsNotExist(err) {
		t.Fatal("a database file was created")
	}
	// No private directory named: CGI answers 503, the other commands refuse; nothing
	// is guessed from the program's own location.
	noDir := cgiEnv(f, http.MethodPost, "action=activate", "8.8.8.8", body)[1:]
	_, out, _ = run(t, append(noDir, testKey), "", body)
	if status, answer := parseCGI(t, out); status != 503 || answer != `{"error":"service_unavailable"}` {
		t.Fatalf("CGI without a private directory: %d %s", status, answer)
	}
	for _, args := range []string{"public-key", "check-db", "serve -listen 127.0.0.1:0"} {
		if code, _, stderr := run(t, []string{"CELIKPANEL_MEMBERSHIP_PRIVATE=relative/dir"}, args, nil); code == 0 ||
			!strings.Contains(stderr, "CELIKPANEL_MEMBERSHIP_PRIVATE") {
			t.Fatalf("%s with a relative directory: %d %s", args, code, stderr)
		}
	}
}

func TestServeRefusesBeforeListening(t *testing.T) {
	f := newFixture(t, "http://127.0.0.1:8379")
	env := []string{"CELIKPANEL_MEMBERSHIP_PRIVATE=" + f.dir}
	if code, _, stderr := run(t, env, "serve -listen 127.0.0.1:0", nil); code != 1 || !strings.Contains(stderr, "pinned production key") {
		t.Fatalf("serve with an unpinned key: %d %s", code, stderr)
	}
	if code, _, _ := run(t, env, "serve -listen 0.0.0.0:8379 -test-signing-public-key "+hex.EncodeToString(f.public), nil); code != 2 {
		t.Fatalf("serve on a public address: %d", code)
	}
	if code, _, _ := run(t, env, "serve -test-signing-public-key "+hex.EncodeToString(membership.ProductionPublicKey()), nil); code != 2 {
		t.Fatalf("the production key named as a test key: %d", code)
	}
}

func TestPublicKeyAndReadOnlyCheckDB(t *testing.T) {
	f := newFixture(t, "https://account.example")
	env := []string{"CELIKPANEL_MEMBERSHIP_PRIVATE=" + f.dir}
	code, out, _ := run(t, env, "public-key", nil)
	if code != 1 || strings.TrimSpace(out) != "license_public_key="+hex.EncodeToString(f.public) {
		t.Fatalf("public-key of an unpinned key: %d %q", code, out)
	}
	if code, _, _ = run(t, env, "public-key -test-signing-public-key "+hex.EncodeToString(f.public), nil); code != 0 {
		t.Fatalf("public-key of the named test key: %d", code)
	}
	if code, _, _ = run(t, env, "check-db", nil); code != 1 {
		t.Fatalf("check-db with an unpinned key must refuse: %d", code)
	}
	digest := func() string {
		h := sha256.New()
		for _, name := range []string{"members.sqlite", "members.sqlite-wal"} {
			raw, _ := os.ReadFile(filepath.Join(f.dir, name))
			h.Write(raw)
		}
		return hex.EncodeToString(h.Sum(nil))
	}
	before := digest()
	code, out, stderr := run(t, env, "check-db -test-signing-public-key "+hex.EncodeToString(f.public), nil)
	if code != 0 || !strings.Contains(out, "result: every live licence can be served") || !strings.Contains(out, "receipts made and verified with the configured key: 1 of 1") ||
		!strings.Contains(out, "to compare two copies: events ") {
		t.Fatalf("check-db: %d\n%s\n%s", code, out, stderr)
	}
	if digest() != before {
		t.Fatal("check-db changed the database")
	}
	if strings.Contains(out, "owner@example.com") || strings.Contains(out, f.key) || strings.Contains(out, "8.8.8.8") {
		t.Fatalf("check-db printed a value: %s", out)
	}
	// A stored key that no longer decrypts (another signing key) is reported, by count only.
	_, other, _ := ed25519.GenerateKey(rand.Reader)
	config, _ := json.Marshal(map[string]string{"origin": "https://account.example", "signing_key": base64.StdEncoding.EncodeToString(other)})
	if err := os.WriteFile(filepath.Join(f.dir, "config.json"), config, 0o600); err != nil {
		t.Fatal(err)
	}
	code, out, _ = run(t, env, "check-db -test-signing-public-key "+hex.EncodeToString(other.Public().(ed25519.PublicKey)), nil)
	if code != 1 || !strings.Contains(out, "1 × a stored key does not decrypt to its hash") || !strings.Contains(out, "NOT every live licence") {
		t.Fatalf("check-db with the wrong key: %d\n%s", code, out)
	}
}
