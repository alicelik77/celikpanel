package membership

// Review of PR #361, finding 1: a missing or wrong database must never become an
// empty one, because an empty database answers "invalid" to every panel that checks
// in. The service answers 503 instead (panels keep their receipts and retry) and
// creates nothing. Finding 12: a private path with "%", "?" or "#" cannot change the
// database address.

import (
	"database/sql"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func answer(t *testing.T, server *httptest.Server, method, action, contentType, body string) (int, string) {
	t.Helper()
	request, _ := http.NewRequest(method, server.URL+"/account/?action="+action, strings.NewReader(body))
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	raw, _ := io.ReadAll(response.Body)
	return response.StatusCode, string(raw)
}

func TestMissingDatabaseAnswers503AndCreatesNothing(t *testing.T) {
	for _, shape := range []string{"missing file", "empty file", "a database without the licences table"} {
		t.Run(shape, func(t *testing.T) {
			dir := t.TempDir()
			handler := &Handler{Private: dir, RemoteAddr: func(*http.Request) string { return "8.8.8.8" }}
			server := httptest.NewServer(handler)
			defer server.Close()
			handler.AllowTestKey = writeConfigOnly(t, dir, server.URL)
			path := filepath.Join(dir, "members.sqlite")
			switch shape {
			case "empty file":
				if err := os.WriteFile(path, nil, 0o600); err != nil {
					t.Fatal(err)
				}
			case "a database without the licences table":
				db, err := sql.Open("sqlite", "file:"+path)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := db.Exec("CREATE TABLE unrelated (x)"); err != nil {
					t.Fatal(err)
				}
				db.Close()
			}
			refresh := `{"license_id":"` + strings.Repeat("a", 32) + `","server_id":"` + strings.Repeat("b", 64) + `","activation_token":"` + strings.Repeat("c", 64) + `"}`
			if status, body := answer(t, server, http.MethodPost, "refresh", "application/json", refresh); status != 503 || body != `{"error":"service_unavailable"}` {
				t.Fatalf("a panel's refresh: %d %s (a 400 here would lock every panel)", status, body)
			}
			if status, _ := answer(t, server, http.MethodGet, "login", "", ""); status != 503 {
				t.Fatalf("the sign-in page: %d", status)
			}
			switch shape {
			case "missing file":
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatalf("a database file was created: %v", err)
				}
			case "empty file":
				if info, err := os.Stat(path); err != nil || info.Size() != 0 {
					t.Fatalf("the empty file was written: %v", err)
				}
			default:
				db, _ := sql.Open("sqlite", "file:"+path+"?mode=ro")
				defer db.Close()
				var tables int
				if err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table'").Scan(&tables); err != nil || tables != 1 {
					t.Fatalf("tables were created: %d %v", tables, err)
				}
			}
		})
	}
	// An unset or relative private directory is answered the same way, never read
	// relative to wherever the program runs.
	for _, private := range []string{"", "membership-private"} {
		server := httptest.NewServer(&Handler{Private: private, RemoteAddr: func(*http.Request) string { return "8.8.8.8" }})
		status, body := answer(t, server, http.MethodPost, "refresh", "application/json", `{}`)
		server.Close()
		if status != 503 || body != `{"error":"service_unavailable"}` {
			t.Fatalf("private %q: %d %s", private, status, body)
		}
	}
}

func TestCheckDBWarnsOnAnEmptyDatabase(t *testing.T) {
	h := newHarness(t)
	report, err := CheckDB(filepath.Join(h.dir, "db.sqlite"), h.secret, h.now)
	if err != nil || report.OK() || report.Live != 0 {
		t.Fatalf("an empty database must not read as fine: %+v %v", report, err)
	}
	var out strings.Builder
	report.Print(&out)
	if !strings.Contains(out.String(), "no live licence in this file") || !strings.Contains(out.String(), "NOT every live licence") ||
		!strings.Contains(out.String(), "to compare two copies: events 0, newest event at 0, licence generations in total 0") {
		t.Fatalf("report: %s", out.String())
	}
	if _, err := CheckDB(filepath.Join(h.dir, "absent.sqlite"), h.secret, h.now); err == nil {
		t.Fatal("check-db opened a missing file")
	}
	if _, err := os.Stat(filepath.Join(h.dir, "absent.sqlite")); !os.IsNotExist(err) {
		t.Fatal("check-db created a file")
	}
}

func TestDatabasePathWithURICharacters(t *testing.T) {
	name := "odd%41#x"
	if runtime.GOOS != "windows" {
		name = "odd%41?mode=memory#x"
	}
	dir := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	h := newHarness(t)
	path := filepath.Join(dir, "members.sqlite")
	s, err := Open(path, h.secret, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Register("uri@example.com"); err != nil {
		t.Fatal(err)
	}
	s.Close()
	if info, err := os.Stat(path); err != nil || info.Size() == 0 {
		t.Fatalf("the database is not at the path as written: %v", err)
	}
	again, err := OpenExisting(path, h.secret, nil, nil)
	if err != nil {
		t.Fatalf("reopening: %v", err)
	}
	defer again.Close()
	var members int
	if err := again.DB().QueryRow("SELECT COUNT(*) FROM members").Scan(&members); err != nil || members != 1 {
		t.Fatalf("the same file was not read back: %d %v", members, err)
	}
}
