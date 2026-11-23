package membership

// Second review of PR #361, finding 1: a database that exists but is wrong must not
// reject panels. (a) A file without a single licence answers the panel API 503.
// (b) A panel's own activation token (made by our key) for a state the database does not
// know yet answers 503, not 400 -- while every deliberate revocation (remove, rotate,
// release, a reinstall at the same address) keeps its rejection exactly as today.

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestRefreshWhenTheDatabaseIsBehind(t *testing.T) {
	h := newHarness(t)
	s := h.s
	password := "behind-password-1234"
	if err := s.Register("behind@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := s.ConsumeToken(h.lastMail().Token, "verify", password); err != nil {
		t.Fatal(err)
	}
	m := must(s.Login("behind@example.com", password))
	a, b := strings.Repeat("a", 64), strings.Repeat("b", 64)

	// A stale copy taken while the licence was unbound; the live service then activated it
	// on server a (generation 1, no raise on a first activation).
	unbound := must(s.Issue(m.ID))
	staleToken := s.activationToken(unbound.ID, a, 1)
	if _, err := s.Refresh(unbound.ID, a, staleToken, "8.8.8.8"); !errors.Is(err, ErrDatabaseBehind) {
		t.Fatalf("unbound row, our token for its generation: %v", err)
	}

	// A licence the copy does not have at all.
	missing := strings.Repeat("e", 32)
	if _, err := s.Refresh(missing, a, s.activationToken(missing, a, 1), "8.8.8.8"); !errors.Is(err, ErrDatabaseBehind) {
		t.Fatalf("missing row, our token: %v", err)
	}
	if _, err := s.Refresh(missing, a, strings.Repeat("0", 64), "8.8.8.8"); !errors.Is(err, ErrInvalidActivation) {
		t.Fatalf("missing row, a token that is not ours: %v", err)
	}

	// A licence moved after the copy: released (generation 2) and activated on server b.
	moved := must(s.Issue(m.ID))
	must(s.Activate(moved.Key, a, "a.example.com", "8.8.8.8"))
	if _, err := s.Refresh(moved.ID, b, s.activationToken(moved.ID, b, 2), "9.9.9.9"); !errors.Is(err, ErrDatabaseBehind) {
		t.Fatalf("a generation the copy has not reached: %v", err)
	}

	// Every deliberate change keeps its rejection: its old token is for a lower generation.
	for name, change := range map[string]func(Issued) error{
		"rotated":  func(l Issued) error { _, err := s.Rotate(m.ID, l.ID, password); return err },
		"released": func(l Issued) error { return s.Release(m.ID, l.ID, password) },
		"removed":  func(l Issued) error { return s.Remove(m.ID, l.ID, password) },
		"reinstalled elsewhere at the same address": func(l Issued) error {
			_, err := s.Activate(l.Key, b, "b.example.com", "8.8.8.8")
			return err
		},
	} {
		l := must(s.Issue(m.ID))
		envelope := must(s.Activate(l.Key, a, "a.example.com", "8.8.8.8"))
		if err := change(l); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if _, err := s.Refresh(l.ID, a, envelope.ActivationToken, "8.8.8.8"); !errors.Is(err, ErrInvalidActivation) {
			t.Fatalf("%s: the old installation must stay rejected: %v", name, err)
		}
	}
}

func TestPanelAPIWhenTheDatabaseIsWrong(t *testing.T) {
	dir := t.TempDir()
	var logged []string
	var mu sync.Mutex
	handler := &Handler{Private: dir, RemoteAddr: func(*http.Request) string { return "8.8.8.8" },
		Log: func(line string) { mu.Lock(); logged = append(logged, line); mu.Unlock() }}
	server := httptest.NewServer(handler)
	defer server.Close()
	handler.AllowTestKey = writeFixture(t, dir, server.URL)
	refresh := func(id, token string) (int, string) {
		return answer(t, server, http.MethodPost, "refresh", "application/json",
			`{"license_id":"`+id+`","server_id":"`+strings.Repeat("a", 64)+`","activation_token":"`+token+`"}`)
	}
	// (a) No licence in the file: the API answers 503, the pages stay open.
	if status, body := refresh(strings.Repeat("e", 32), strings.Repeat("0", 64)); status != 503 || body != `{"error":"service_unavailable"}` {
		t.Fatalf("a file without licences: %d %s", status, body)
	}
	if status, _ := answer(t, server, http.MethodGet, "register", "", ""); status != 200 {
		t.Fatalf("the pages of a fresh service: %d", status)
	}
	// (b) With licences, a panel's own token for a licence the file does not know: 503 and
	// one log line; a token that is not ours: the rejection as before.
	config, err := LoadConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	s, err := Open(filepath.Join(dir, "members.sqlite"), config.SigningKey, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB().Exec("INSERT INTO members(email,password,created) VALUES('x@example.com','x',1)"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB().Exec("INSERT INTO licenses(id,member_id,key_hash,key_suffix,created,expires) VALUES(?,1,'h','s',1,4102444800)", strings.Repeat("c", 32)); err != nil {
		t.Fatal(err)
	}
	missing := strings.Repeat("e", 32)
	token := s.activationToken(missing, strings.Repeat("a", 64), 3)
	s.Close()
	mu.Lock()
	logged = nil
	mu.Unlock()
	if status, body := refresh(missing, token); status != 503 || body != `{"error":"service_unavailable"}` {
		t.Fatalf("our token, a licence the file does not know: %d %s", status, body)
	}
	mu.Lock()
	if len(logged) != 1 || !strings.Contains(logged[0], "database behind the signing key") || strings.Contains(logged[0], missing) {
		t.Fatalf("log: %q", logged)
	}
	mu.Unlock()
	if status, body := refresh(missing, strings.Repeat("0", 64)); status != 400 || body != `{"error":"invalid_activation"}` {
		t.Fatalf("a token that is not ours: %d %s", status, body)
	}
}
