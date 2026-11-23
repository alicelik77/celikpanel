package membership

// Third look at PR #361. The "database behind" answer rests on one invariant: a licence row
// is never deleted, its generation never falls, and every revocation raises the generation
// in the same statement. These tests hold the service to it and keep the answer narrow.

import (
	"database/sql"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// licenceState is what the invariant compares, per licence id.
type licenceState struct {
	server     sql.NullString
	keyHash    string
	removed    sql.NullInt64
	revoked    sql.NullInt64
	generation int64
}

func licenceStates(t *testing.T, db *sql.DB) map[string]licenceState {
	t.Helper()
	columns, err := tableColumns(t.Context(), db, "licenses")
	if err != nil {
		t.Fatal(err)
	}
	revoked := "NULL"
	if columns["revoked"] {
		revoked = "revoked"
	}
	rows, err := db.Query("SELECT id,server_id,key_hash,removed," + revoked + ",generation FROM licenses")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]licenceState{}
	for rows.Next() {
		var id string
		var s licenceState
		if err := rows.Scan(&id, &s.server, &s.keyHash, &s.removed, &s.revoked, &s.generation); err != nil {
			t.Fatal(err)
		}
		out[id] = s
	}
	return out
}

// checkGenerationInvariant fails when, between two snapshots, a licence row disappeared, a
// generation fell, or a binding was cleared or changed, a row was marked removed, or its
// key was replaced, without the generation rising. A first binding (no server before) is
// the one change allowed at the same generation.
func checkGenerationInvariant(t *testing.T, operation string, before, after map[string]licenceState) {
	t.Helper()
	for _, problem := range invariantProblems(before, after) {
		t.Errorf("%s: %s", operation, problem)
	}
}

// Every mutating operation of Service, run against a database with licences in every
// state, keeps the invariant. A new revocation path (refund, operator revoke, account
// closing) must be added to this table; the static check below catches one that is not.
func TestEveryLicenceChangeRaisesTheGeneration(t *testing.T) {
	h := newHarness(t)
	s := h.s
	password := "invariant-password-1234"
	if err := s.Register("invariant@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := s.ConsumeToken(h.lastMail().Token, "verify", password); err != nil {
		t.Fatal(err)
	}
	m := must(s.Login("invariant@example.com", password))
	a, b := strings.Repeat("a", 64), strings.Repeat("b", 64)
	bound := func() Issued {
		l := must(s.Issue(m.ID))
		must(s.Activate(l.Key, a, "a.example.com", "8.8.8.8"))
		return l
	}
	unbound := func() Issued { return must(s.Issue(m.ID)) }
	// Each operation's licence is made before the snapshot, so the change it makes is the
	// only one the checker sees.
	operations := []struct {
		name    string
		prepare func() Issued
		run     func(Issued) error
	}{
		{"issue", nil, func(Issued) error { _, err := s.Issue(m.ID); return err }},
		{"first activation", unbound, func(l Issued) error { _, err := s.Activate(l.Key, a, "a.example.com", "8.8.8.8"); return err }},
		{"the same server again", bound, func(l Issued) error { _, err := s.Activate(l.Key, a, "a.example.com", "8.8.8.8"); return err }},
		{"another server at the same address", bound, func(l Issued) error { _, err := s.Activate(l.Key, b, "b.example.com", "8.8.8.8"); return err }},
		{"refresh (address bound)", bound, func(l Issued) error {
			_, err := s.Refresh(l.ID, a, s.activationToken(l.ID, a, 1), "8.8.8.8")
			return err
		}},
		{"release", bound, func(l Issued) error { return s.Release(m.ID, l.ID, password) }},
		{"remove", bound, func(l Issued) error { return s.Remove(m.ID, l.ID, password) }},
		{"remove an unbound licence", unbound, func(l Issued) error { return s.Remove(m.ID, l.ID, password) }},
		{"rotate", bound, func(l Issued) error { _, err := s.Rotate(m.ID, l.ID, password); return err }},
		{"reveal", bound, func(l Issued) error { _, err := s.Reveal(m.ID, l.ID, password); return err }},
		{"renew (not due)", bound, func(l Issued) error {
			if err := s.Renew(m.ID, l.ID, password); !errors.Is(err, ErrRenewalNotDue) {
				return err
			}
			return nil
		}},
	}
	for _, op := range operations {
		// The issue limit (10 a day per member) is not what this test is about.
		if _, err := s.DB().Exec("DELETE FROM rate_limits"); err != nil {
			t.Fatal(err)
		}
		var l Issued
		if op.prepare != nil {
			l = op.prepare()
		}
		before := licenceStates(t, s.DB())
		if err := op.run(l); err != nil {
			t.Fatalf("%s: %v", op.name, err)
		}
		after := licenceStates(t, s.DB())
		if op.prepare != nil && after[l.ID] == before[l.ID] && op.name != "the same server again" && op.name != "reveal" && op.name != "renew (not due)" && op.name != "refresh (address bound)" {
			t.Fatalf("%s: the operation changed nothing the checker compares", op.name)
		}
		checkGenerationInvariant(t, op.name, before, after)
	}
	// The checker itself catches a revocation without a raise and a deletion.
	if _, err := s.DB().Exec("DELETE FROM rate_limits"); err != nil {
		t.Fatal(err)
	}
	l, gone := bound(), must(s.Issue(m.ID))
	before := licenceStates(t, s.DB())
	if _, err := s.DB().Exec("UPDATE licenses SET server_id=NULL WHERE id=?", l.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB().Exec("DELETE FROM licenses WHERE id=?", gone.ID); err != nil {
		t.Fatal(err)
	}
	if problems := invariantProblems(before, licenceStates(t, s.DB())); len(problems) != 2 {
		t.Fatalf("the checker must see the unbinding without a raise and the deletion: %q", problems)
	}
}

func invariantProblems(before, after map[string]licenceState) []string {
	var problems []string
	for id, b := range before {
		a, ok := after[id]
		if !ok {
			problems = append(problems, "licence "+id+" was deleted")
			continue
		}
		if a.generation < b.generation {
			problems = append(problems, "licence "+id+"'s generation fell")
		}
		unbound := b.server.Valid && b.server.String != "" && (!a.server.Valid || a.server.String != b.server.String)
		revoked := (!b.removed.Valid && a.removed.Valid) || (!b.revoked.Valid && a.revoked.Valid)
		if (unbound || revoked || a.keyHash != b.keyHash) && a.generation <= b.generation {
			problems = append(problems, "licence "+id+" was unbound, removed or re-keyed without a generation raise")
		}
	}
	return problems
}

// The static half: no Go file of this module (outside tests) deletes a licence row, and
// every UPDATE of licenses that clears or changes a binding, marks a row removed or revoked,
// or replaces its key also sets the generation in the same statement. It reads string
// literals only, so it does not see SQL built from pieces at run time, and it cannot tell
// whether an explicit "generation=?" value is higher: the table above checks that for the
// operations it runs.
func TestNoSQLRevokesWithoutRaisingTheGeneration(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("module root not found at %s", root)
	}
	update := regexp.MustCompile(`(?is)\bUPDATE\s+licenses\s+SET\s+(.*?)(\bWHERE\b|$)`)
	touches := regexp.MustCompile(`(?i)\b(server_id|removed|revoked|key_hash)\s*=`)
	setsGeneration := regexp.MustCompile(`(?i)\bgeneration\s*=`)
	deletes := regexp.MustCompile(`(?i)\bDELETE\s+FROM\s+licenses\b`)
	checked := 0
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if path != root && (strings.HasPrefix(name, ".") || name == "node_modules" || name == "testdata" || name == "vendor") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
		if err != nil {
			return nil // not this test's business
		}
		ast.Inspect(file, func(n ast.Node) bool {
			literal, ok := n.(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}
			text, err := strconv.Unquote(literal.Value)
			if err != nil {
				return true
			}
			rel, _ := filepath.Rel(root, path)
			if deletes.MatchString(text) {
				t.Errorf("%s: a licence row is deleted: %q", rel, text)
			}
			for _, m := range update.FindAllStringSubmatch(text, -1) {
				checked++
				if touches.MatchString(m[1]) && !setsGeneration.MatchString(m[1]) {
					t.Errorf("%s: an UPDATE of licenses changes a binding, removal or key without setting the generation: %q", rel, text)
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if checked < 5 {
		t.Fatalf("only %d UPDATE statements found: the scan is not reading the service", checked)
	}
	// The pattern itself: these must be caught.
	for _, bad := range []string{"UPDATE licenses SET server_id=NULL WHERE id=?", "UPDATE licenses SET revoked=?, server_id=NULL WHERE id=?"} {
		m := update.FindStringSubmatch(bad)
		if m == nil || !touches.MatchString(m[1]) || setsGeneration.MatchString(m[1]) {
			t.Fatalf("the pattern misses %q", bad)
		}
	}
}

// Item 1: a row marked removed is never "behind", whatever its generation -- even a removal
// that (wrongly) did not raise it.
func TestARemovedLicenceIsNeverBehind(t *testing.T) {
	h := newHarness(t)
	s := h.s
	if err := s.Register("removed@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := s.ConsumeToken(h.lastMail().Token, "verify", "removed-password-1234"); err != nil {
		t.Fatal(err)
	}
	m := must(s.Login("removed@example.com", "removed-password-1234"))
	a := strings.Repeat("a", 64)
	l := must(s.Issue(m.ID))
	envelope := must(s.Activate(l.Key, a, "a.example.com", "8.8.8.8"))
	// A revocation written the way the review warns against: unbound and marked, no raise.
	if _, err := s.DB().Exec("UPDATE licenses SET removed=1,server_id=NULL WHERE id=?", l.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Refresh(l.ID, a, envelope.ActivationToken, "8.8.8.8"); !errors.Is(err, ErrInvalidActivation) {
		t.Fatalf("a removed licence must be rejected, never 'behind': %v", err)
	}
	// And a token for a later generation of that removed row: still rejected.
	if _, err := s.Refresh(l.ID, a, s.activationToken(l.ID, a, 5), "8.8.8.8"); !errors.Is(err, ErrInvalidActivation) {
		t.Fatalf("a removed licence, a later generation: %v", err)
	}
}

// Item 2: "bound elsewhere at the same generation" is a fork or a deliberate restore, not a
// stale copy: the ordinary rejection. The three behind cases stay behind.
func TestBoundElsewhereAtTheSameGenerationIsNotBehind(t *testing.T) {
	h := newHarness(t)
	s := h.s
	if err := s.Register("fork@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := s.ConsumeToken(h.lastMail().Token, "verify", "fork-password-12345"); err != nil {
		t.Fatal(err)
	}
	m := must(s.Login("fork@example.com", "fork-password-12345"))
	a, x := strings.Repeat("a", 64), strings.Repeat("f", 64)
	l := must(s.Issue(m.ID))
	must(s.Activate(l.Key, a, "a.example.com", "8.8.8.8")) // generation 1, bound to a
	// A token for server x at generation 1: only a forked timeline could have issued it.
	if _, err := s.Refresh(l.ID, x, s.activationToken(l.ID, x, 1), "9.9.9.9"); !errors.Is(err, ErrInvalidActivation) {
		t.Fatalf("bound elsewhere at the same generation: %v", err)
	}
	// Above the row's generation it is still behind.
	if _, err := s.Refresh(l.ID, x, s.activationToken(l.ID, x, 2), "9.9.9.9"); !errors.Is(err, ErrDatabaseBehind) {
		t.Fatalf("a later generation: %v", err)
	}
}

// Item 2: the operator's "this database is the authority" switches the behind answer off,
// so every unknown or older state is rejected as the PHP service does.
func TestDatabaseIsAuthorityRejectsLikePHP(t *testing.T) {
	h := newHarness(t)
	s := h.s
	if err := s.Register("authority@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := s.ConsumeToken(h.lastMail().Token, "verify", "authority-password-1"); err != nil {
		t.Fatal(err)
	}
	m := must(s.Login("authority@example.com", "authority-password-1"))
	a := strings.Repeat("a", 64)
	unbound := must(s.Issue(m.ID))
	missing := strings.Repeat("e", 32)
	cases := map[string][3]string{
		"unbound":          {unbound.ID, a, s.activationToken(unbound.ID, a, 1)},
		"missing":          {missing, a, s.activationToken(missing, a, 1)},
		"later generation": {unbound.ID, a, s.activationToken(unbound.ID, a, 4)},
	}
	for name, c := range cases {
		if _, err := s.Refresh(c[0], c[1], c[2], "8.8.8.8"); !errors.Is(err, ErrDatabaseBehind) {
			t.Fatalf("%s, default: %v", name, err)
		}
	}
	s.SetDatabaseIsAuthority(true)
	for name, c := range cases {
		if _, err := s.Refresh(c[0], c[1], c[2], "8.8.8.8"); !errors.Is(err, ErrInvalidActivation) {
			t.Fatalf("%s, database is the authority: %v", name, err)
		}
	}
	// The setting is read from the private configuration: only a JSON true turns it on.
	for raw, want := range map[string]bool{`true`: true, `false`: false, `"true"`: false, `1`: false} {
		config, err := ParseConfig([]byte(`{"origin":"https://a.example","signing_key":"` + strings.Repeat("A", 86) + `==","database_is_authority":` + raw + `}`))
		if err != nil || config.DatabaseIsAuthority != want {
			t.Fatalf("database_is_authority %s: %v %v", raw, config.DatabaseIsAuthority, err)
		}
	}
	if config, err := ParseConfig([]byte(`{"origin":"https://a.example","signing_key":"` + strings.Repeat("A", 86) + `=="}`)); err != nil || config.DatabaseIsAuthority {
		t.Fatalf("absent: %v %v", config.DatabaseIsAuthority, err)
	}
}

// Item 2, end to end: the handler applies the private configuration's setting.
func TestDatabaseIsAuthorityThroughTheHandler(t *testing.T) {
	dir := t.TempDir()
	handler := &Handler{Private: dir, RemoteAddr: func(*http.Request) string { return "8.8.8.8" }, Log: func(string) {}}
	server := httptest.NewServer(handler)
	defer server.Close()
	handler.AllowTestKey = writeFixture(t, dir, server.URL)
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
	missing, server64 := strings.Repeat("e", 32), strings.Repeat("a", 64)
	body := `{"license_id":"` + missing + `","server_id":"` + server64 + `","activation_token":"` + s.activationToken(missing, server64, 1) + `"}`
	s.Close()
	if status, _ := answer(t, server, http.MethodPost, "refresh", "application/json", body); status != 503 {
		t.Fatalf("default: %d", status)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	authority := strings.TrimSuffix(strings.TrimSpace(string(raw)), "}") + `,"database_is_authority":true}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(authority), 0o600); err != nil {
		t.Fatal(err)
	}
	if status, answerBody := answer(t, server, http.MethodPost, "refresh", "application/json", body); status != 400 || answerBody != `{"error":"invalid_activation"}` {
		t.Fatalf("database is the authority: %d %s", status, answerBody)
	}
}

// Item 3: on a file without licences a wrong method still answers 405 and logs nothing; the
// "no licence" line comes only after the per-address limit, so it is bounded; a read error
// is logged as what it is.
func TestEmptyDatabaseGateRunsAfterTheMethodAndTheLimit(t *testing.T) {
	dir := t.TempDir()
	var logged []string
	var mu sync.Mutex
	handler := &Handler{Private: dir, RemoteAddr: func(*http.Request) string { return "8.8.8.8" },
		Log: func(line string) { mu.Lock(); logged = append(logged, line); mu.Unlock() }}
	server := httptest.NewServer(handler)
	defer server.Close()
	handler.AllowTestKey = writeFixture(t, dir, server.URL)
	if status, body := answer(t, server, http.MethodGet, "refresh", "", ""); status != 405 || body != `{"error":"method_not_allowed"}` {
		t.Fatalf("a GET on a file without licences: %d %s", status, body)
	}
	mu.Lock()
	if len(logged) != 0 {
		t.Fatalf("a wrong method was logged: %q", logged)
	}
	mu.Unlock()
	refresh := `{"license_id":"` + strings.Repeat("e", 32) + `","server_id":"` + strings.Repeat("a", 64) + `","activation_token":"` + strings.Repeat("0", 64) + `"}`
	for i := 0; i < 61; i++ {
		status, body := answer(t, server, http.MethodPost, "refresh", "application/json", refresh)
		if i < 60 && status != 503 || i == 60 && (status != 400 || body != `{"error":"rate_limited"}`) {
			t.Fatalf("request %d: %d %s", i+1, status, body)
		}
	}
	mu.Lock()
	gate := 0
	for _, line := range logged {
		if strings.Contains(line, "no licence in the database") {
			gate++
		}
	}
	mu.Unlock()
	if gate != 60 {
		t.Fatalf("the gate's line was logged %d times for 61 requests; the limit allows 60", gate)
	}
	// A read error is not "no licence".
	recorder := httptest.NewRecorder()
	var line string
	x := &exchange{h: &Handler{Log: func(l string) { line = l }}, w: recorder, api: true}
	x.fail(errDatabaseUnreadable)
	if recorder.Code != 503 || !strings.Contains(line, "could not be read") || strings.Contains(line, "no licence") {
		t.Fatalf("a read error: %d %q", recorder.Code, line)
	}
}

// Item 5: a full password queue says the service is busy, in the pages' words, never "too
// many attempts" (which blames the member).
func TestAFullPasswordQueueSaysTheServiceIsBusy(t *testing.T) {
	wait, limit := passwordWait, passwordQueueLimit
	defer func() { passwordWait, passwordQueueLimit = wait, limit }()
	passwordWait, passwordQueueLimit = 50*time.Millisecond, 0
	a := newAPIServer(t)
	a.issueKey()
	for lang, want := range map[string]string{"tr": "Hizmet şu anda yoğun. Lütfen biraz sonra tekrar deneyin.", "en": "The service is busy. Please try again in a moment."} {
		jar, _ := cookiejar.New(nil)
		s := &signedIn{a: a, client: &http.Client{Jar: jar, Timeout: 30 * time.Second}}
		s.page(t, http.MethodGet, "action=login&lang="+lang, nil)
		free := fillSlots(t)
		_, body := s.page(t, http.MethodPost, "action=login&lang="+lang, url.Values{"csrf": {s.csrf}, "email": {"api@example.com"},
			"password": {"api-test-password-123"}})
		free()
		if !strings.Contains(body, want) || strings.Contains(body, "Too many attempts") || strings.Contains(body, "Çok fazla deneme") {
			t.Fatalf("%s: the busy answer is not shown:\n%s", lang, body)
		}
	}
}
