// Source: bioexamine, backend/internal/payment/fake_server_test.go, read at commit 181fccf6
// (last changed there in 602dbeeb, 2026-11-04); copied into CelikPanel on 2026-11-23 under
// the shared catalogue's entry A (same language: copy with a source note). CelikPanel's
// changes after the copy are marked "CelikPanel:" and are in this repository's history.

package payment

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// A fake provider: an httptest server that answers each documented path with a handler the test
// writes, and records what it was sent. No test in this package reaches a real provider.

type recordedRequest struct {
	Method string
	Path   string
	Query  string
	Header http.Header
	Body   []byte
}

type fakeServer struct {
	t        *testing.T
	server   *httptest.Server
	mu       sync.Mutex
	routes   map[string]func(recordedRequest) (int, string)
	received []recordedRequest
}

func newFakeServer(t *testing.T) *fakeServer {
	t.Helper()
	f := &fakeServer{t: t, routes: map[string]func(recordedRequest) (int, string){}}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		request := recordedRequest{Method: r.Method, Path: r.URL.Path, Query: r.URL.RawQuery, Header: r.Header.Clone(), Body: body}
		f.mu.Lock()
		f.received = append(f.received, request)
		handler := f.routes[r.Method+" "+r.URL.Path]
		f.mu.Unlock()
		if handler == nil {
			http.Error(w, `{"error":"no such route in the fake"}`, http.StatusNotFound)
			return
		}
		status, answer := handler(request)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(answer))
	}))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeServer) on(method, path string, handler func(recordedRequest) (int, string)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.routes[method+" "+path] = handler
}

func (f *fakeServer) last() recordedRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.received) == 0 {
		f.t.Fatal("the fake server received nothing")
	}
	return f.received[len(f.received)-1]
}

func (f *fakeServer) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.received)
}

func credentialsOf(t *testing.T, values map[string]string) Credentials {
	t.Helper()
	encoded, err := json.Marshal(values)
	if err != nil {
		t.Fatal(err)
	}
	return Credentials(encoded)
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

// noSecretIn fails when an error text carries any of the secrets.
func noSecretIn(t *testing.T, err error, secrets ...string) {
	t.Helper()
	if err == nil {
		return
	}
	for _, secret := range secrets {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("an error carries a secret: %v", err)
		}
	}
}

// CelikPanel: licenseID is a licence id's shape (32 hex) for the tests, in place of
// bioexamine's membership UUID.
const licenseID = "0123456789abcdef0123456789abcdef"

// mustReference is a fresh order reference for licenseID.
func mustReference(t *testing.T, license string, automaticRenewal bool) string {
	t.Helper()
	reference, err := NewOrderReference(license, automaticRenewal)
	if err != nil {
		t.Fatal(err)
	}
	return reference
}
