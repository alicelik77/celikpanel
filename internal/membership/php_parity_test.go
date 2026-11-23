package membership

// Parity with the PHP service, proven against the PHP binary itself. These tests run
// only when CELIKPANEL_MEMBERSHIP_PHP names a PHP 8.3 with pdo_sqlite and sodium (the
// CI job "Membership — Go service" sets it); otherwise they skip, and
// deploy/test-membership-go.sh fails if one skips although PHP was given.
//
//   - TestPHPCompatCorpus: PHP's FILTER_VALIDATE_EMAIL, Service::email(),
//     Service::publicIP(), htmlspecialchars and parse_str against this port, on a
//     fixed corpus plus a seeded fuzz.
//   - TestPHPDifferential: one database made by PHP, copied twice; the same
//     operations on the same clock against the PHP service and the Go service give
//     the same codes, the same receipts (payload bytes, signature, activation
//     token), the same listings and, at the end, the same rows.
//   - TestPHPCrossReading: a database made by Go is served by PHP (Go's password
//     hashes, Go's stored keys, equal receipts) and back; both create the same schema.
//   - TestPHPPagesAreByteEqual: php -S with portal-membership's router and the Go
//     handler, each on a copy of the same database, answer the same requests with
//     the same status and the same page bytes (the per-session CSRF value aside).
//
// The signing key is made here; no request leaves the loopback.

import (
	"bytes"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	mathrand "math/rand/v2"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/payment"
)

func phpCommand(t *testing.T) []string {
	t.Helper()
	php := os.Getenv("CELIKPANEL_MEMBERSHIP_PHP")
	if php == "" {
		// The CI job sets CELIKPANEL_MEMBERSHIP_REQUIRE_PHP=1: there a comparison test that
		// would skip fails instead, whatever its name (review of #361, 10).
		if os.Getenv("CELIKPANEL_MEMBERSHIP_REQUIRE_PHP") == "1" {
			t.Fatal("the PHP comparison is required here but CELIKPANEL_MEMBERSHIP_PHP is not set")
		}
		t.Skip("set CELIKPANEL_MEMBERSHIP_PHP=php (PHP 8.3 with pdo_sqlite and sodium) to compare with the PHP service")
	}
	if _, err := exec.LookPath(php); err != nil {
		t.Fatalf("CELIKPANEL_MEMBERSHIP_PHP names %q, which is not runnable", php)
	}
	command := []string{php}
	for _, extension := range []string{"pdo_sqlite", "sodium"} {
		probe := exec.Command(php, "-r", "exit(extension_loaded('"+extension+"')?0:1);")
		if probe.Run() != nil {
			command = append(command, "-d", "extension="+extension)
		}
	}
	return command
}

// TestPHPRequiredMeansNoSkip runs a comparison test in a child process with PHP required
// but not given: it must fail, not skip (the CI job requires PHP).
func TestPHPRequiredMeansNoSkip(t *testing.T) {
	if os.Getenv("CELIKPANEL_MEMBERSHIP_CHILD") == "1" {
		t.Skip("inside the child")
	}
	command := exec.Command(os.Args[0], "-test.run=^TestPHPCompatCorpus$", "-test.v")
	command.Env = []string{"CELIKPANEL_MEMBERSHIP_REQUIRE_PHP=1", "CELIKPANEL_MEMBERSHIP_CHILD=1", "PATH=" + os.Getenv("PATH"),
		"HOME=" + os.Getenv("HOME"), "TMPDIR=" + os.TempDir()}
	out, err := command.CombinedOutput()
	if err == nil || !strings.Contains(string(out), "--- FAIL: TestPHPCompatCorpus") || strings.Contains(string(out), "--- SKIP: TestPHPCompatCorpus") {
		t.Fatalf("a required comparison skipped or passed without PHP:\n%s", out)
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("no source path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func runPHP(t *testing.T, php []string, script string, input any) []byte {
	t.Helper()
	command := phpScript(t, php, script, input)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	out, err := command.Output()
	if err != nil || stderr.Len() > 0 {
		t.Fatalf("php %s: %v\n%s\n%s", script, err, stderr.String(), out)
	}
	return out
}

// phpScript prepares (on the test's goroutine) a PHP run that may then be started anywhere.
func phpScript(t *testing.T, php []string, script string, input any) *exec.Cmd {
	t.Helper()
	root := repositoryRoot(t)
	inputPath := filepath.Join(t.TempDir(), "input.json")
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(inputPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	args := append(append([]string{}, php[1:]...), "-d", "display_errors=stderr",
		filepath.Join(root, "internal", "membership", "testdata", "php", script),
		filepath.Join(root, "portal-membership", "src", "Service.php"), inputPath)
	return exec.Command(php[0], args...)
}

// ---------------------------------------------------------------------------
// The corpus.

var corpusEmails = []string{
	"alice@example.com", "Alice@Example.COM", " alice@example.com\n", "a@b", "a@b.c", "a@b.co", "a.b@c.de", "a..b@c.de",
	".a@b.cd", "a.@b.cd", "\"a b\"@c.de", "\"a\\\"b\"@c.de", "\"a@b\"@c.de", "\"\"@c.de", "a@[1.2.3.4]", "a@[256.2.3.4]",
	"a@[IPv6:::1]", "a@[ipv6:1:2:3:4:5:6:7:8]", "a@[IPv6:1:2:3:4:5:6:1.2.3.4]", "a@[IPv6:1::1.2.3.4]",
	"a@[IPv6:1:2:3:4:5:6:7::]", "a@[IPv6:1:2:3:4:5:6:7:8:9]", "a@[IPv6:1::2::3]", "a@-b.cd", "a@b-.cd", "a@b--c.de",
	"a@xn--bcher-kva.example", "a@b.xn--p1ai", "a@b.c1", "a@b.1c", "a@1.2.3.4", "a+tag@example.com", "a!#$%&'*+-/=?^_`{|}~@example.com",
	"a(b)@example.com", "a,b@example.com", "a;b@example.com", "a<b>@example.com", "a[b]@example.com", "a\\b@example.com",
	"\"a\\\\b\"@example.com", "a@example.com.", "a@.example.com", "a@example..com", "@example.com", "a@", "a", "",
	"x@example.com\r\nBcc: other@example.com", "ü@example.com", "a@exämple.com", "a@ex ample.com", "a b@example.com",
	strings.Repeat("a", 64) + "@example.com", strings.Repeat("a", 65) + "@example.com",
	"a@" + strings.Repeat("b", 63) + ".com", "a@" + strings.Repeat("b", 64) + ".com",
	"a@" + strings.Repeat("b.", 120) + "com", strings.Repeat("a", 60) + "@" + strings.Repeat("b", 60) + "." + strings.Repeat("c", 60) + "." + strings.Repeat("d", 60) + ".com",
	"\"" + strings.Repeat("a", 62) + "\"@example.com", "\"" + strings.Repeat("a", 63) + "\"@example.com",
	strings.Repeat("\\a", 33) + "@example.com", "\"" + strings.Repeat("\\a", 31) + "\"@example.com", "a\x00@example.com",
}

var corpusIPs = []string{
	"8.8.8.8", "9.9.9.9", "1.1.1.1", "203.0.113.9", "198.51.100.1", "192.0.2.1", "100.64.0.1", "192.88.99.1", "224.0.0.1",
	"239.255.255.255", "240.0.0.1", "255.255.255.255", "0.0.0.0", "0.1.2.3", "10.0.0.1", "172.15.255.255", "172.16.0.1",
	"172.31.255.255", "172.32.0.1", "192.168.0.1", "169.254.1.1", "169.253.1.1", "127.0.0.1", "126.255.255.255", "128.0.0.1",
	"::ffff:8.8.4.4", "::FFFF:8.8.4.4", "::ffff:10.1.1.1", "::ffff:127.0.0.1", "::1", "::", "::2", "::1.2.3.4", "::0.0.0.2",
	"::ffff:0:1.2.3.4", "64:ff9b::1.2.3.4", "2a00:1450:4001:81c::200e", "2A00:1450:4001:081C:0000:0000:0000:200E",
	"2001:4860:4860::8888", "2001:0:1::1", "2001:200::1", "2002:c000:0204::1", "fc00::1", "fdff:ffff::1", "fe00::1",
	"fe80::1", "febf::1", "fec0::1", "ff02::1", "100::1", "3ffe::1", "1:0:0:2:0:0:0:3", "1:0:0:2::3", "1::2:0:0:3",
	"0:0:1:0:0:0:0:0", "1:2:3:4:5:6:7:8", "fe80::1%eth0", "1.2.3", "01.2.3.4", "1.2.3.04", "256.1.1.1", "1.2.3.4 ",
	" 1.2.3.4", "", "abc", "::1::", "12345::1", "1:2:3:4:5:6:7:8:9", "8.8.8.8:80", "[::1]", "unknown",
}

// disputedIPs are the IPv6 prefixes PHP 8.3 patch releases answer differently:
// 8.3.6 refuses them with FILTER_FLAG_NO_RES_RANGE, the RFC 6890 tables of later 8.3
// releases (8.3.28) accept them. This port refuses them; whichever PHP the CI runs
// is recorded, not compared.
var disputedIPs = map[string]bool{"2001:db8::1": true, "2001:10::1": true, "2001:1f::1": true, "5f::1": true, "3ff3::1": true}

var corpusHTML = []string{
	"", "plain", `<script>alert("x")&'y'</script>`, "&amp;", "Türkçe ğüşıöç", "’ …", "\x80", "\xc0\xaf", "\xc2", "\xc2\x41",
	"\xe2\x82", "\xe2\x82\x41", "\xe2\x41\x41", "\xe0\x80\x80", "\xed\xa0\x80", "\xef\xbf\xbd", "\xf0\x9f\x98\x80", "\xf0\x9f\x98",
	"\xf0\x9f\x41", "\xf0\x41", "\xf4\x90\x80\x80", "\xf5\x80\x80\x80", "\xff\xfe", "a\x00b", "\xe2\x82\xac\xe2",
}

var corpusQueries = []string{
	"action=home", "action=home&action=login", "action[]=x", "action=x&action[]=y", "action[]=y&action=x", "action[a]=1",
	"act.ion=x", "act ion=x", " action=x", "action%00junk=x", "action[=x", "action[x=y", "a[b.c=1", "a.b[c]=1", "[a]=1",
	"=x", "&&a=1&&", "a", "a=", "a=%41%4a%zz%4", "a=+b+", "%61ction=reveal", "id=%3Cscript%3E", "id=a%26b%3Dc",
	"lang=en;action=x", "a=1&b=2&c[]=3&c[]=4", "a%5B%5D=1", "a]=1", "%20a=1", "a%20b=1", ".=1",
}

func corpusFuzz() (emails, html []string, queries []string) {
	random := mathrand.New(mathrand.NewPCG(20261123, 47))
	alphabet := []string{"a", "b", "Z", "0", "9", ".", ".", "@", "@", "\"", "\\", "[", "]", ":", "-", "_", "+", "!", "'", "(",
		")", ",", ";", "<", ">", " ", "\t", "\x00", "\x7f", "é", "xn--", "IPv6:", "1.2.3.4", "::", "ab", "\xff", "\xe2\x82"}
	for i := 0; i < 2500; i++ {
		var b strings.Builder
		for n := random.IntN(24) + 1; n > 0; n-- {
			b.WriteString(alphabet[random.IntN(len(alphabet))])
		}
		if random.IntN(2) == 0 {
			emails = append(emails, b.String()+"@example.com")
		} else {
			emails = append(emails, b.String())
		}
	}
	for i := 0; i < 800; i++ {
		raw := make([]byte, random.IntN(12))
		for j := range raw {
			switch random.IntN(4) {
			case 0:
				raw[j] = byte(0x80 + random.IntN(0x80))
			case 1:
				raw[j] = "&<>\"'a"[random.IntN(6)]
			default:
				raw[j] = byte(random.IntN(256))
			}
		}
		html = append(html, string(raw))
	}
	pieces := []string{"a", "id", "action", "=", "&", "[", "]", ".", " ", "%00", "%5B", "%2E", "+", "x", "%zz"}
	for i := 0; i < 600; i++ {
		var b strings.Builder
		for n := random.IntN(10) + 1; n > 0; n-- {
			b.WriteString(pieces[random.IntN(len(pieces))])
		}
		queries = append(queries, b.String())
	}
	return
}

func TestPHPCompatCorpus(t *testing.T) {
	php := phpCommand(t)
	fuzzEmails, fuzzHTML, fuzzQueries := corpusFuzz()
	emails := append(append([]string{}, corpusEmails...), fuzzEmails...)
	html := append(append([]string{}, corpusHTML...), fuzzHTML...)
	queries := append(append([]string{}, corpusQueries...), fuzzQueries...)
	ips := append([]string{}, corpusIPs...)
	for ip := range disputedIPs {
		ips = append(ips, ip)
	}
	encode := func(list []string) []string {
		out := make([]string, len(list))
		for i, s := range list {
			out[i] = base64.StdEncoding.EncodeToString([]byte(s))
		}
		return out
	}
	raw := runPHP(t, php, "compat.php", map[string]any{"emails": encode(emails), "ips": ips, "html": encode(html), "queries": encode(queries)})
	var answer struct {
		Filter  []bool              `json:"filter"`
		Email   []string            `json:"email"`
		IP      []string            `json:"ip"`
		HTML    []string            `json:"html"`
		Query   []map[string]string `json:"query"`
		Version string              `json:"version"`
	}
	if err := json.Unmarshal(raw, &answer); err != nil {
		t.Fatal(err)
	}
	t.Logf("PHP %s; %d e-mail, %d address, %d HTML and %d query cases", answer.Version, len(emails), len(ips), len(html), len(queries))
	failures := 0
	byKind := map[string]int{}
	fail := func(format string, args ...any) {
		failures++
		byKind[strings.SplitN(format, "(", 2)[0]]++
		if failures <= 40 {
			t.Errorf(format, args...)
		}
	}
	for i, email := range emails {
		if got := validEmail(email); got != answer.Filter[i] {
			fail("FILTER_VALIDATE_EMAIL(%q): PHP %v, Go %v", email, answer.Filter[i], got)
		}
		want := answer.Email[i]
		normalized, err := normalEmail(email)
		got := "!" + fmt.Sprint(err)
		if err == nil {
			got = base64.StdEncoding.EncodeToString([]byte(normalized))
		}
		if got != want {
			fail("Service::email(%q): PHP %s, Go %s", email, want, got)
		}
	}
	for i, ip := range ips {
		normalized, err := publicIP(ip)
		got := "!" + fmt.Sprint(err)
		if err == nil {
			got = normalized
		}
		if disputedIPs[ip] {
			if got != "!invalid_server_ip" {
				fail("publicIP(%q) must stay refused in Go: %s", ip, got)
			}
			t.Logf("known difference: PHP %s answers %s for %s; Go refuses it", answer.Version, answer.IP[i], ip)
			continue
		}
		if got != answer.IP[i] {
			fail("Service::publicIP(%q): PHP %s, Go %s", ip, answer.IP[i], got)
		}
	}
	for i, s := range html {
		if got := base64.StdEncoding.EncodeToString([]byte(htmlEscape(s))); got != answer.HTML[i] {
			want, _ := base64.StdEncoding.DecodeString(answer.HTML[i])
			fail("htmlspecialchars(%q): PHP %q, Go %q", s, want, htmlEscape(s))
		}
	}
	for i, q := range queries {
		vars := phpVars{}
		parsePHPQuery(q, vars)
		got := map[string]string{}
		for name, v := range vars {
			if v.array {
				got[base64.StdEncoding.EncodeToString([]byte(name))] = "a"
			} else {
				got[base64.StdEncoding.EncodeToString([]byte(name))] = "s" + base64.StdEncoding.EncodeToString([]byte(v.str))
			}
		}
		if !reflect.DeepEqual(got, answer.Query[i]) && !(len(got) == 0 && len(answer.Query[i]) == 0) {
			fail("parse_str(%q): PHP %v, Go %v", q, decodeQueryAnswer(answer.Query[i]), decodeQueryAnswer(got))
		}
	}
	if failures > 0 {
		t.Fatalf("%d differences from PHP, by kind: %v", failures, byKind)
	}
}

func decodeQueryAnswer(m map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range m {
		name, _ := base64.StdEncoding.DecodeString(k)
		value := v
		if strings.HasPrefix(v, "s") {
			raw, _ := base64.StdEncoding.DecodeString(v[1:])
			value = "s:" + string(raw)
		}
		out[string(name)] = value
	}
	return out
}

// ---------------------------------------------------------------------------
// The operation driver, in Go: the twin of testdata/php/driver.php.

type driverOp struct {
	Op   string   `json:"op"`
	Args []string `json:"args,omitempty"`
	Now  int64    `json:"now,omitempty"`
	// OnlyOutcome: the answer holds fresh randomness (a new key); compare ok/code only.
	OnlyOutcome bool `json:"-"`
	// GoDiffers names a deliberate difference from PHP: PHP must answer invalid_activation
	// and Go this code. The one such difference is "database_behind" (second review of #361,
	// 1b): a panel's own token for a state the database does not know gets 503 from Go, so
	// the panel keeps its receipt, where PHP's 400 would lock it.
	GoDiffers string `json:"-"`
}

// tokenFor is the activation token our key makes for a licence state (Service.php
// activationToken), computed here so the test can present a token from "the future".
func tokenFor(secret ed25519.PrivateKey, id, server string, generation int64) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte("activation-v1:" + id + ":" + server + ":" + strconv.FormatInt(generation, 10)))
	return hex.EncodeToString(mac.Sum(nil))
}

type driverResult struct {
	OK     bool   `json:"ok"`
	Code   string `json:"code,omitempty"`
	Result any    `json:"result"`
}

func runPHPOps(t *testing.T, php []string, db string, secret ed25519.PrivateKey, now int64, ops []driverOp) []driverResult {
	t.Helper()
	raw := runPHP(t, php, "driver.php", map[string]any{"db": db, "secret": base64.StdEncoding.EncodeToString(secret), "now": now, "ops": ops})
	var out []driverResult
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&out); err != nil {
		t.Fatalf("driver answer: %v\n%s", err, raw)
	}
	return out
}

func runGoOps(t *testing.T, db string, secret ed25519.PrivateKey, now int64, ops []driverOp) []driverResult {
	t.Helper()
	var mail []map[string]string
	s, err := Open(db, secret, func(email, purpose, token string) error {
		mail = append(mail, map[string]string{"email": email, "purpose": purpose, "token": token})
		return nil
	}, func() int64 { return now })
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var results []driverResult
	reference := regexp.MustCompile(`^@(\d+)\.([a-z_]+)$`)
	for _, op := range ops {
		if op.Now != 0 {
			now = op.Now
		}
		args := make([]string, len(op.Args))
		for i, arg := range op.Args {
			if m := reference.FindStringSubmatch(arg); m != nil {
				n, _ := strconv.Atoi(m[1])
				fields, _ := toPlain(results[n].Result).(map[string]any)
				arg = fmt.Sprint(fields[m[2]])
			}
			args[i] = arg
		}
		id := func(i int) int64 { n, _ := strconv.ParseInt(args[i], 10, 64); return n }
		var result any
		switch op.Op {
		case "register":
			err = s.Register(args[0])
		case "request":
			err = s.RequestToken(args[0], args[1])
		case "consume":
			err = s.ConsumeToken(args[0], args[1], args[2])
		case "login":
			var m Member
			if m, err = s.Login(args[0], args[1]); err == nil {
				result = map[string]any{"id": m.ID, "email": m.Email, "epoch": m.Epoch}
			}
		case "member":
			var m *Member
			if m, err = s.MemberByEpoch(id(0), id(1)); err == nil && m != nil {
				result = map[string]any{"id": m.ID, "email": m.Email, "epoch": m.Epoch}
			}
		case "licenses":
			var list []License
			if list, err = s.Licenses(id(0)); err == nil {
				rows := []any{}
				for _, l := range list {
					rows = append(rows, map[string]any{"id": l.ID, "key_suffix": l.KeySuffix, "created": l.Created, "activated": nullInt(l.Activated.Valid, l.Activated.Int64),
						"expires": nullInt(l.Expires.Valid, l.Expires.Int64), "hostname": nullString(l.Hostname.Valid, l.Hostname.String),
						"server_id": nullString(l.ServerID.Valid, l.ServerID.String), "server_ip": nullString(l.ServerIP.Valid, l.ServerIP.String), "key_available": l.KeyAvailable})
				}
				result = rows
			}
		case "issue", "reveal", "rotate":
			var issued Issued
			switch op.Op {
			case "issue":
				issued, err = s.Issue(id(0))
			case "reveal":
				issued, err = s.Reveal(id(0), args[1], args[2])
			default:
				issued, err = s.Rotate(id(0), args[1], args[2])
			}
			if err == nil {
				result = map[string]any{"id": issued.ID, "key": issued.Key}
			}
		case "release":
			err = s.Release(id(0), args[1], args[2])
		case "remove":
			err = s.Remove(id(0), args[1], args[2])
		case "renew":
			err = s.Renew(id(0), args[1], args[2])
		case "activate", "refresh":
			var e Envelope
			if op.Op == "activate" {
				e, err = s.Activate(args[0], args[1], args[2], args[3])
			} else {
				e, err = s.Refresh(args[0], args[1], args[2], args[3])
			}
			if err == nil {
				result = map[string]any{"payload": e.Payload, "signature": e.Signature, "activation_token": e.ActivationToken}
			}
		case "limit":
			err = s.Limit(args[0], id(1), id(2))
		case "mail":
			result, mail = mail, nil
			if result == nil {
				result = []any{}
			}
		case "sql":
			_, err = s.DB().Exec(args[0])
		case "dump":
			result = goDump(t, s)
		case "schema":
			result = queryMaps(t, s, "SELECT type,name,tbl_name,sql FROM sqlite_master ORDER BY name")
		default:
			t.Fatalf("unknown op %s", op.Op)
		}
		var problem Problem
		switch {
		case err == nil:
			results = append(results, driverResult{OK: true, Result: result})
		case errors.As(err, &problem):
			results = append(results, driverResult{Code: string(problem)})
			err = nil
		case errors.Is(err, ErrDatabaseBehind):
			results = append(results, driverResult{Code: "database_behind"})
			err = nil
		default:
			t.Fatalf("op %s: %v", op.Op, err)
		}
	}
	return results
}

func nullInt(valid bool, v int64) any {
	if !valid {
		return nil
	}
	return v
}

func nullString(valid bool, v string) any {
	if !valid {
		return nil
	}
	return v
}

func queryMaps(t *testing.T, s *Service, query string) []any {
	t.Helper()
	rows, err := s.DB().Query(query)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	columns, _ := rows.Columns()
	out := []any{}
	for rows.Next() {
		values := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for i := range values {
			pointers[i] = &values[i]
		}
		if err := rows.Scan(pointers...); err != nil {
			t.Fatal(err)
		}
		row := map[string]any{}
		for i, c := range columns {
			if b, ok := values[i].([]byte); ok {
				values[i] = string(b)
			}
			row[c] = values[i]
		}
		out = append(out, row)
	}
	return out
}

func goDump(t *testing.T, s *Service) map[string]any {
	t.Helper()
	out := map[string]any{}
	for _, table := range []string{"members", "tokens", "licenses", "events", "rate_limits"} {
		columns, err := tableColumnsOrdered(s, table)
		if err != nil {
			t.Fatal(err)
		}
		selects := make([]string, len(columns))
		for i, c := range columns {
			selects[i] = "quote(" + c + ") AS " + c
		}
		out[table] = queryMaps(t, s, "SELECT "+strings.Join(selects, ",")+" FROM "+table+" ORDER BY rowid")
	}
	return out
}

func tableColumnsOrdered(s *Service, table string) ([]string, error) {
	rows, err := s.DB().Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var cid, notNull, pk int
		var name, kind string
		var dflt any
		if err := rows.Scan(&cid, &name, &kind, &notNull, &dflt, &pk); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

// toPlain makes both sides comparable: JSON round trip, every number and string as
// its decimal or text, so PHP's int and Go's int64 meet.
func toPlain(v any) any {
	raw, _ := json.Marshal(v)
	var out any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	_ = decoder.Decode(&out)
	return plain(out)
}

func plain(v any) any {
	switch x := v.(type) {
	case json.Number:
		return x.String()
	case map[string]any:
		for k, item := range x {
			x[k] = plain(item)
		}
		return x
	case []any:
		for i, item := range x {
			x[i] = plain(item)
		}
		return x
	}
	return v
}

// dropRandom removes the values both services make fresh each time (password hashes
// of new registrations, token hashes, the nonce-bearing stored keys).
func dropRandom(dump any) any {
	tables, _ := dump.(map[string]any)
	drop := map[string][]string{"members": {"password"}, "tokens": {"hash"}, "licenses": {"key_encrypted"}}
	for table, columns := range drop {
		rows, _ := tables[table].([]any)
		for _, row := range rows {
			m, _ := row.(map[string]any)
			for _, c := range columns {
				if m[c] != "NULL" {
					m[c] = "<fresh>"
				}
			}
		}
	}
	return dump
}

func compareResults(t *testing.T, label string, ops []driverOp, php, goResults []driverResult) {
	t.Helper()
	if len(php) != len(goResults) {
		t.Fatalf("%s: PHP gave %d answers, Go %d", label, len(php), len(goResults))
	}
	for i := range ops {
		p, g := php[i], goResults[i]
		if ops[i].GoDiffers != "" {
			if p.OK || p.Code != "invalid_activation" || g.OK || g.Code != ops[i].GoDiffers {
				t.Errorf("%s #%d %s: the named difference %q: PHP ok=%v %q, Go ok=%v %q", label, i, ops[i].Op, ops[i].GoDiffers, p.OK, p.Code, g.OK, g.Code)
			} else {
				t.Logf("%s #%d: deliberate difference %q: PHP %s (400), Go %s (503)", label, i, ops[i].GoDiffers, p.Code, g.Code)
			}
			continue
		}
		if p.OK != g.OK || p.Code != g.Code {
			t.Errorf("%s #%d %s %q: PHP ok=%v %q, Go ok=%v %q", label, i, ops[i].Op, ops[i].Args, p.OK, p.Code, g.OK, g.Code)
			continue
		}
		if ops[i].OnlyOutcome {
			continue
		}
		pp, gp := toPlain(p.Result), toPlain(g.Result)
		if ops[i].Op == "dump" {
			pp, gp = dropRandom(pp), dropRandom(gp)
		}
		if !reflect.DeepEqual(pp, gp) {
			pj, _ := json.Marshal(pp)
			gj, _ := json.Marshal(gp)
			t.Errorf("%s #%d %s %q:\nPHP %s\nGo  %s", label, i, ops[i].Op, ops[i].Args, pj, gj)
		}
	}
}

// ---------------------------------------------------------------------------
// A database made by PHP.

type phpBase struct {
	php       []string
	dir       string
	db        string
	secret    ed25519.PrivateKey
	public    ed25519.PublicKey
	t0        int64
	member    string
	password  string
	ids, keys [4]string
}

const parityPassword = "parity-password-never-production"

type opRunner func(t *testing.T, db string, secret ed25519.PrivateKey, now int64, ops []driverOp) []driverResult

func buildPHPBase(t *testing.T) *phpBase {
	t.Helper()
	php := phpCommand(t)
	return buildBase(t, func(t *testing.T, db string, secret ed25519.PrivateKey, now int64, ops []driverOp) []driverResult {
		return runPHPOps(t, php, db, secret, now, ops)
	}, php)
}

// buildBase makes the shared starting database with one service (PHP in the
// comparison tests): two members (one verified), four licences, three of them bound.
func buildBase(t *testing.T, run opRunner, php []string) *phpBase {
	t.Helper()
	b := &phpBase{php: php, dir: t.TempDir(), t0: time.Now().Unix(), password: parityPassword}
	var err error
	b.public, b.secret, err = ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	b.db = filepath.Join(b.dir, "base.sqlite")
	first := run(t, b.db, b.secret, b.t0, []driverOp{
		{Op: "register", Args: []string{"alice@example.com"}}, {Op: "register", Args: []string{"bob@example.com"}}, {Op: "mail"},
	})
	mails, _ := toPlain(first[2].Result).([]any)
	token := ""
	for _, m := range mails {
		if mm := m.(map[string]any); mm["email"] == "alice@example.com" {
			token = mm["token"].(string)
		}
	}
	ops := []driverOp{
		{Op: "consume", Args: []string{token, "verify", b.password}},
		{Op: "login", Args: []string{"alice@example.com", b.password}},
		{Op: "issue", Args: []string{"@1.id"}}, {Op: "issue", Args: []string{"@1.id"}},
		{Op: "issue", Args: []string{"@1.id"}}, {Op: "issue", Args: []string{"@1.id"}},
		{Op: "activate", Args: []string{"@2.key", strings.Repeat("a", 64), "frankfurt.example.com", "8.8.8.8"}},
		{Op: "activate", Args: []string{"@3.key", strings.Repeat("b", 64), "boston.example.com", "::ffff:9.9.9.9"}},
		{Op: "activate", Args: []string{"@4.key", strings.Repeat("c", 64), "v6.example.com", "2a00:1450:4001:81c::200e"}},
	}
	second := run(t, b.db, b.secret, b.t0, ops)
	for i, r := range second {
		if !r.OK {
			t.Fatalf("building the PHP database, step %d %s: %s", i, ops[i].Op, r.Code)
		}
	}
	login := toPlain(second[1].Result).(map[string]any)
	b.member = login["id"].(string)
	for i := 0; i < 4; i++ {
		issued := toPlain(second[2+i].Result).(map[string]any)
		b.ids[i], b.keys[i] = issued["id"].(string), issued["key"].(string)
	}
	return b
}

func copyDB(t *testing.T, from, to string) {
	t.Helper()
	for _, suffix := range []string{"", "-wal", "-shm"} {
		raw, err := os.ReadFile(from + suffix)
		if errors.Is(err, os.ErrNotExist) && suffix != "" {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(to+suffix, raw, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPHPDifferential(t *testing.T) {
	b := buildPHPBase(t)
	t1 := b.t0 + 120
	ops := differentialOps(b, t1)
	phpDB, goDB := filepath.Join(b.dir, "php.sqlite"), filepath.Join(b.dir, "go.sqlite")
	copyDB(t, b.db, phpDB)
	copyDB(t, b.db, goDB)
	phpResults := runPHPOps(t, b.php, phpDB, b.secret, t1, ops)
	goResults := runGoOps(t, goDB, b.secret, t1, ops)
	compareResults(t, "PHP-made database", ops, phpResults, goResults)
	// The answers that matter most, said once more by name.
	for _, i := range []int{11, 12, 13, 16, 17, 18, 19} {
		if !goResults[i].OK || !reflect.DeepEqual(toPlain(phpResults[i].Result), toPlain(goResults[i].Result)) {
			t.Errorf("receipt #%d differs", i)
		}
	}
	if !goResults[8].OK {
		t.Error("Go could not decrypt a key PHP stored")
	}
	if !goResults[0].OK {
		t.Error("Go refused a password PHP hashed")
	}
}

// The store's migration adds tables and two licence columns to the same file; the PHP
// service must keep working on it exactly as before (S2: "prove it with the existing
// cross tests"). The whole comparison script runs on store-migrated copies, and PHP then
// refuses a licence the Go store revoked.
func TestPHPDifferentialOnAStoreFile(t *testing.T) {
	b := buildPHPBase(t)
	t1 := b.t0 + 120
	ops := differentialOps(b, t1)
	phpDB, goDB := filepath.Join(b.dir, "php-store.sqlite"), filepath.Join(b.dir, "go-store.sqlite")
	for _, db := range []string{phpDB, goDB} {
		copyDB(t, b.db, db)
		s, err := OpenExisting(db, b.secret, nil, func() int64 { return t1 })
		if err != nil {
			t.Fatal(err)
		}
		if err := s.MigrateStore("operator:test"); err != nil {
			t.Fatal(err)
		}
		s.Close()
	}
	compareResults(t, "store-migrated database", ops, runPHPOps(t, b.php, phpDB, b.secret, t1, ops), runGoOps(t, goDB, b.secret, t1, ops))

	// A first year bought and refunded through the Go store, then the PHP service asked.
	db := filepath.Join(b.dir, "revoked.sqlite")
	copyDB(t, b.db, db)
	s, err := OpenExisting(db, b.secret, nil, func() int64 { return t1 })
	if err != nil {
		t.Fatal(err)
	}
	member, _ := strconv.ParseInt(b.member, 10, 64)
	if err := s.MigrateStore("operator:test"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetTestAccount("operator:test", "cross test", member, true); err != nil {
		t.Fatal(err)
	}
	o, err := s.CreateOrder(member, b.ids[0], ChargingTest)
	if err != nil {
		t.Fatal(err)
	}
	fact := payment.Fact{Outcome: payment.OutcomePaid, PaymentID: "pi_x", Kind: payment.OrderOneTime, Order: o.ID, LicenseID: o.LicenseID, PaidMinor: o.AmountMinor, Currency: "USD"}
	if _, err := s.ApplyFact(o.ID, "stripe", "test", fact); err != nil {
		t.Fatal(err)
	}
	fact.Outcome = payment.OutcomeRefunded
	if _, err := s.ApplyFact(o.ID, "stripe", "test", fact); err != nil {
		t.Fatal(err)
	}
	s.Close()
	a := strings.Repeat("a", 64)
	byPHP := runPHPOps(t, b.php, db, b.secret, t1+60, []driverOp{
		{Op: "refresh", Args: []string{b.ids[0], a, tokenFor(b.secret, b.ids[0], a, 1), "8.8.8.8"}},
		{Op: "activate", Args: []string{b.keys[0], a, "frankfurt.example.com", "8.8.8.8"}},
		{Op: "licenses", Args: []string{b.member}},
	})
	if byPHP[0].OK || byPHP[0].Code != "invalid_activation" || byPHP[1].OK || byPHP[1].Code != "license_expired" || !byPHP[2].OK {
		t.Fatalf("PHP on a store-revoked licence: %+v", byPHP)
	}
}

// differentialOps is the comparison script: sign-in, listings, key display,
// activation and refresh (retry, reinstall, move, address and host-name refusals),
// release, renewal across a year, removal, limits, mail requests, tokens,
// registration, then every row, then a key rotation.
func differentialOps(b *phpBase, t1 int64) []driverOp {
	a, bb, c, d := strings.Repeat("a", 64), strings.Repeat("b", 64), strings.Repeat("c", 64), strings.Repeat("d", 64)
	m, pw := b.member, b.password
	ops := []driverOp{
		{Op: "login", Args: []string{"alice@example.com", pw}},                                            // 0
		{Op: "login", Args: []string{"Alice@Example.com ", "wrong-password-123"}},                         // 1
		{Op: "login", Args: []string{"nobody@example.com", pw}},                                           // 2
		{Op: "login", Args: []string{"not-an-email", pw}},                                                 // 3
		{Op: "login", Args: []string{"bob@example.com", pw}},                                              // 4
		{Op: "member", Args: []string{m, "@0.epoch"}},                                                     // 5
		{Op: "member", Args: []string{m, "99"}},                                                           // 6
		{Op: "licenses", Args: []string{m}},                                                               // 7
		{Op: "reveal", Args: []string{m, b.ids[0], pw}},                                                   // 8: Go decrypts PHP's stored key
		{Op: "reveal", Args: []string{m, b.ids[0], "wrong-password-123"}},                                 // 9
		{Op: "reveal", Args: []string{m, strings.Repeat("f", 32), pw}},                                    // 10
		{Op: "activate", Args: []string{b.keys[0], a, "frankfurt.example.com", "8.8.8.8"}},                // 11: retry
		{Op: "activate", Args: []string{b.keys[0], d, "frankfurt.example.com", "8.8.8.8"}},                // 12: reinstall, same address
		{Op: "refresh", Args: []string{b.ids[0], d, "@12.activation_token", "8.8.8.8"}},                   // 13
		{Op: "refresh", Args: []string{b.ids[0], d, "@12.activation_token", "9.9.9.9"}},                   // 14
		{Op: "refresh", Args: []string{b.ids[0], a, "@11.activation_token", "8.8.8.8"}},                   // 15
		{Op: "activate", Args: []string{b.keys[1], bb, "boston.example.com", "9.9.9.9"}},                  // 16
		{Op: "activate", Args: []string{b.keys[2], c, "v6.example.com", "2a00:1450:4001:81c:0:0:0:200e"}}, // 17
		{Op: "activate", Args: []string{b.keys[3], d, "compat.example.com", "::1.2.3.4"}},                 // 18: glibc's text is stored
		{Op: "refresh", Args: []string{b.ids[3], d, "@18.activation_token", "::1.2.3.4"}},                 // 19
	}
	for _, ip := range []string{"127.0.0.1", "10.1.1.1", "172.16.0.1", "192.168.1.1", "169.254.1.1", "0.1.2.3", "240.0.0.1",
		"::1", "::", "fe80::1", "fc00::1", "::ffff:10.0.0.1", "abc", "", "01.2.3.4", "fe80::1%eth0", "1.1.1.1", "2a00:1450::1"} {
		ops = append(ops, driverOp{Op: "activate", Args: []string{"CPK-x", a, "h.example.com", ip}})
	}
	for _, host := range []string{"-bad", strings.Repeat("h", 253), strings.Repeat("h", 254), "host\n", "a.b-c", "a_b", ""} {
		ops = append(ops, driverOp{Op: "activate", Args: []string{"CPK-" + strings.Repeat("0", 64), a, host, "1.1.1.1"}})
	}
	ops = append(ops,
		driverOp{Op: "activate", Args: []string{b.keys[0], a, "x.example.com", "1.1.1.1"}},
		driverOp{Op: "release", Args: []string{m, b.ids[1], "wrong-password-123"}},
		driverOp{Op: "release", Args: []string{m, b.ids[1], pw}},
		driverOp{Op: "refresh", Args: []string{b.ids[1], bb, "@16.activation_token", "9.9.9.9"}},
		driverOp{Op: "activate", Args: []string{b.keys[1], a, "moved.example.com", "1.1.1.1"}},
		driverOp{Op: "renew", Args: []string{m, b.ids[0], pw}},
		driverOp{Op: "renew", Args: []string{m, b.ids[0], pw}, Now: t1 + 340*86400},
		driverOp{Op: "renew", Args: []string{m, b.ids[0], pw}},
		driverOp{Op: "refresh", Args: []string{b.ids[0], d, "@12.activation_token", "8.8.8.8"}},
		driverOp{Op: "licenses", Args: []string{m}},
		driverOp{Op: "remove", Args: []string{m, b.ids[3], pw}},
		driverOp{Op: "activate", Args: []string{b.keys[3], d, "compat.example.com", "::1.2.3.4"}},
		driverOp{Op: "refresh", Args: []string{b.ids[3], d, "@18.activation_token", "::1.2.3.4"}},
		driverOp{Op: "remove", Args: []string{m, b.ids[3], pw}},
		driverOp{Op: "limit", Args: []string{"parity", "1", "60"}},
		driverOp{Op: "limit", Args: []string{"parity", "1", "60"}},
		driverOp{Op: "request", Args: []string{"alice@example.com", "reset"}},
		driverOp{Op: "request", Args: []string{"bob@example.com", "verify"}},
		driverOp{Op: "request", Args: []string{"alice@example.com", "verify"}},
		driverOp{Op: "request", Args: []string{"alice@example.com", "other"}},
		driverOp{Op: "consume", Args: []string{"zz", "reset", pw}},
		driverOp{Op: "consume", Args: []string{strings.Repeat("a", 64), "reset", "short"}},
		driverOp{Op: "consume", Args: []string{strings.Repeat("a", 64), "reset", pw}},
		driverOp{Op: "register", Args: []string{" New@Example.com "}},
		driverOp{Op: "register", Args: []string{"bad address@example.com"}},
		// The one deliberate difference: our own token for a state this database does not
		// know (a generation ahead of the row; a licence it does not have).
		driverOp{Op: "refresh", Args: []string{b.ids[0], d, tokenFor(b.secret, b.ids[0], d, 3), "8.8.8.8"}, GoDiffers: "database_behind"},
		driverOp{Op: "refresh", Args: []string{strings.Repeat("e", 32), d, tokenFor(b.secret, strings.Repeat("e", 32), d, 1), "8.8.8.8"}, GoDiffers: "database_behind"},
		driverOp{Op: "dump"},
		driverOp{Op: "rotate", Args: []string{m, b.ids[2], pw}, OnlyOutcome: true},
		driverOp{Op: "activate", Args: []string{b.keys[2], c, "v6.example.com", "2a00:1450:4001:81c::200e"}},
	)
	return ops
}

func TestPHPCrossReading(t *testing.T) {
	php := phpCommand(t)
	dir := t.TempDir()
	_, secret, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().Unix()
	// Both services create the same schema in an empty file.
	phpSchema := runPHPOps(t, php, filepath.Join(dir, "php-empty.sqlite"), secret, now, []driverOp{{Op: "schema"}})
	goSchema := runGoOps(t, filepath.Join(dir, "go-empty.sqlite"), secret, now, []driverOp{{Op: "schema"}})
	compareResults(t, "empty schema", []driverOp{{Op: "schema"}}, phpSchema, goSchema)

	// A database made by Go, read and served by PHP.
	db := filepath.Join(dir, "go-made.sqlite")
	made := runGoOps(t, db, secret, now, []driverOp{{Op: "register", Args: []string{"carol@example.com"}}, {Op: "mail"}})
	token := toPlain(made[1].Result).([]any)[0].(map[string]any)["token"].(string)
	server := strings.Repeat("e", 64)
	goOps := []driverOp{
		{Op: "consume", Args: []string{token, "verify", parityPassword}},
		{Op: "login", Args: []string{"carol@example.com", parityPassword}},
		{Op: "issue", Args: []string{"@1.id"}},
		{Op: "activate", Args: []string{"@2.key", server, "cross.example.com", "8.8.4.4"}},
	}
	goMade := runGoOps(t, db, secret, now, goOps)
	issued := toPlain(goMade[2].Result).(map[string]any)
	member := toPlain(goMade[1].Result).(map[string]any)["id"].(string)
	envelope := toPlain(goMade[3].Result).(map[string]any)
	later := now + 60
	readOps := []driverOp{
		{Op: "login", Args: []string{"carol@example.com", parityPassword}},
		{Op: "reveal", Args: []string{member, issued["id"].(string), parityPassword}},
		{Op: "refresh", Args: []string{issued["id"].(string), server, envelope["activation_token"].(string), "8.8.4.4"}},
		{Op: "activate", Args: []string{issued["key"].(string), server, "cross.example.com", "8.8.4.4"}},
		{Op: "licenses", Args: []string{member}},
	}
	byPHP := runPHPOps(t, php, db, secret, later, readOps)
	for i, r := range byPHP {
		if !r.OK {
			t.Fatalf("PHP reading a Go-made database, %s: %s", readOps[i].Op, r.Code)
		}
	}
	if toPlain(byPHP[1].Result).(map[string]any)["key"] != issued["key"] {
		t.Fatal("PHP decrypted a different key from Go's stored copy")
	}
	// The same refresh by Go on the same clock is the same receipt, byte for byte;
	// and Go reads the key PHP just stored again with a fresh nonce.
	copyDB(t, db, filepath.Join(dir, "after-php.sqlite"))
	byGo := runGoOps(t, filepath.Join(dir, "after-php.sqlite"), secret, later, []driverOp{readOps[2], readOps[1], readOps[0]})
	if !reflect.DeepEqual(toPlain(byGo[0].Result), toPlain(byPHP[2].Result)) {
		t.Fatalf("receipts differ:\nPHP %v\nGo  %v", byPHP[2].Result, byGo[0].Result)
	}
	if !byGo[1].OK || toPlain(byGo[1].Result).(map[string]any)["key"] != issued["key"] || !byGo[2].OK {
		t.Fatalf("Go reading what PHP wrote: %+v", byGo)
	}
}

// ---------------------------------------------------------------------------
// The pages, over HTTP.

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func writePrivate(t *testing.T, dir, origin string, secret ed25519.PrivateKey, db string) {
	t.Helper()
	for _, p := range []string{dir, filepath.Join(dir, "sessions"), filepath.Join(dir, "mail")} {
		if err := os.MkdirAll(p, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	config, _ := json.Marshal(map[string]string{"origin": origin, "sender": "noreply@celikpanel.net",
		"signing_key": base64.StdEncoding.EncodeToString(secret), "test_mail_dir": filepath.Join(dir, "mail")})
	if err := os.WriteFile(filepath.Join(dir, "config.json"), config, 0o600); err != nil {
		t.Fatal(err)
	}
	copyDB(t, db, filepath.Join(dir, "members.sqlite"))
}

type pageClient struct {
	name   string
	base   string
	client *http.Client
	body   string
}

// newPageClient keeps cookies but never follows a redirect by itself: the 303 and its
// Location are compared, then followed as a step of their own.
func newPageClient(name, base string, transport http.RoundTripper) *pageClient {
	jar, _ := cookiejar.New(nil)
	return &pageClient{name: name, base: base, client: &http.Client{Jar: jar, Timeout: 30 * time.Second, Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

var csrfValue = regexp.MustCompile(`name="csrf" value="[a-f0-9]{64}"`)

func (c *pageClient) csrf() string {
	m := regexp.MustCompile(`name="csrf" value="([a-f0-9]{64})"`).FindStringSubmatch(c.body)
	if m == nil {
		return ""
	}
	return m[1]
}

type pageStep struct {
	method, query, contentType, origin string
	form                               url.Values
	raw                                string
	keepCSRF                           bool // send the form's csrf as given
	// expect must be in both answers (after a redirect is followed), so a failure that both
	// sides share cannot pass; articles is the number of licences the page must list (-1: any).
	expect   string
	articles int
}

// comparedHeaders are the headers a member's browser acts on.
var comparedHeaders = []string{"Content-Type", "Cache-Control", "Expires", "Pragma", "Referrer-Policy",
	"X-Content-Type-Options", "Content-Security-Policy", "Location", "Allow"}

type pageAnswer struct {
	status  int
	headers map[string]string
	// cookie is the last Set-Cookie's name and attributes (the value is a session id and
	// differs), "" when none was sent.
	cookie string
	body   string
}

// cookieShape is a Set-Cookie line without its value: the name and the lower-cased,
// sorted attributes, as a browser reads them.
func cookieShape(line string) string {
	parts := strings.Split(line, ";")
	name, _, _ := strings.Cut(strings.TrimSpace(parts[0]), "=")
	var attributes []string
	for _, part := range parts[1:] {
		if part = strings.ToLower(strings.TrimSpace(part)); part != "" {
			attributes = append(attributes, part)
		}
	}
	sort.Strings(attributes)
	return name + "; " + strings.Join(attributes, "; ")
}

func (c *pageClient) send(t *testing.T, method, target, contentType, origin string, body io.Reader) pageAnswer {
	t.Helper()
	request, err := http.NewRequest(method, target, body)
	if err != nil {
		t.Fatal(err)
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	if origin != "" {
		request.Header.Set("Origin", origin)
	}
	response, err := c.client.Do(request)
	if err != nil {
		t.Fatalf("%s %s %s: %v", c.name, method, target, err)
	}
	defer response.Body.Close()
	raw, _ := io.ReadAll(response.Body)
	// Keep the last page that holds a form: an empty answer (a 405, a 303) must not wipe
	// the CSRF value the next form step is posted with (second review of #361, 2).
	if csrfValue.Match(raw) {
		c.body = string(raw)
	}
	answer := pageAnswer{status: response.StatusCode, headers: map[string]string{},
		body: csrfValue.ReplaceAllString(string(raw), `name="csrf" value="<csrf>"`)}
	for _, name := range comparedHeaders {
		answer.headers[name] = strings.Join(response.Header.Values(name), " | ")
	}
	if cookies := response.Header.Values("Set-Cookie"); len(cookies) > 0 {
		answer.cookie = cookieShape(cookies[len(cookies)-1])
	}
	return answer
}

func (c *pageClient) do(t *testing.T, step pageStep) pageAnswer {
	t.Helper()
	var body io.Reader
	contentType := step.contentType
	if step.form != nil {
		form := url.Values{}
		for k, v := range step.form {
			form[k] = v
		}
		if !step.keepCSRF {
			form.Set("csrf", c.csrf())
		}
		body = strings.NewReader(form.Encode())
		contentType = "application/x-www-form-urlencoded"
	} else if step.raw != "" {
		body = strings.NewReader(step.raw)
	}
	return c.send(t, step.method, c.base+"/account/?"+step.query, contentType, step.origin, body)
}

func compareAnswers(t *testing.T, label string, p, g pageAnswer) {
	t.Helper()
	if p.status != g.status || p.body != g.body {
		t.Errorf("%s: PHP %d, Go %d\n%s", label, p.status, g.status, firstDifference(p.body, g.body))
	}
	for _, name := range comparedHeaders {
		if p.headers[name] != g.headers[name] {
			t.Errorf("%s: header %s: PHP %q, Go %q", label, name, p.headers[name], g.headers[name])
		}
	}
	if (p.cookie == "") != (g.cookie == "") || (p.cookie != "" && p.cookie != g.cookie) {
		t.Errorf("%s: Set-Cookie: PHP %q, Go %q", label, p.cookie, g.cookie)
	}
}

// startPHPServer runs php -S with portal-membership's own router on a private directory.
func startPHPServer(t *testing.T, php []string, port int, private string) {
	t.Helper()
	root := repositoryRoot(t)
	args := append(append([]string{}, php[1:]...), "-S", "127.0.0.1:"+strconv.Itoa(port), "-t", filepath.Join(root, "download-portal"),
		filepath.Join(root, "portal-membership", "tests", "router.php"))
	server := exec.Command(php[0], args...)
	server.Env = append(os.Environ(), "CELIKPANEL_MEMBERSHIP_PRIVATE="+private)
	var serverLog bytes.Buffer
	server.Stdout, server.Stderr = &serverLog, &serverLog
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = server.Process.Kill()
		_ = server.Wait()
		if t.Failed() {
			t.Logf("php -S log:\n%s", serverLog.String())
		}
	})
	for i := 0; i < 100; i++ {
		if conn, err := net.Dial("tcp", "127.0.0.1:"+strconv.Itoa(port)); err == nil {
			conn.Close()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("php -S did not start:\n%s", serverLog.String())
}

func TestPHPPagesAreByteEqual(t *testing.T) {
	b := buildPHPBase(t)
	phpPort := freePort(t)
	phpOrigin := "http://127.0.0.1:" + strconv.Itoa(phpPort)
	phpDir := filepath.Join(b.dir, "php-private")
	writePrivate(t, phpDir, phpOrigin, b.secret, b.db)
	startPHPServer(t, b.php, phpPort, phpDir)

	goDir := filepath.Join(b.dir, "go-private")
	goServer := httptest.NewUnstartedServer(&Handler{Private: goDir, AllowTestKey: b.public, RemoteAddr: PeerAddress})
	goServer.Start()
	defer goServer.Close()
	writePrivate(t, goDir, goServer.URL, b.secret, b.db)

	phpClient, goClient := newPageClient("PHP", phpOrigin, nil), newPageClient("Go", goServer.URL, nil)
	form := func(pairs ...string) url.Values {
		v := url.Values{}
		for i := 0; i+1 < len(pairs); i += 2 {
			v.Set(pairs[i], pairs[i+1])
		}
		return v
	}
	get := func(query, expect string) pageStep {
		return pageStep{method: http.MethodGet, query: query, expect: expect, articles: -1}
	}
	post := func(query string, values url.Values, expect string) pageStep {
		return pageStep{method: http.MethodPost, query: query, form: values, expect: expect, articles: -1}
	}
	l1, l2, l4 := b.ids[0], b.ids[1], b.ids[3]
	home := func(query, expect string) pageStep {
		return pageStep{method: http.MethodGet, query: query, expect: expect, articles: 4}
	}
	steps := []pageStep{
		get("action=register", "Hesap oluştur"), get("action=login", "Hesabınıza giriş yapın"),
		get("action=terms", "Geçerli metin: 30 Ekim 2026."), get("action=terms&lang=en", "Effective text: 30 October 2026."),
		get("action=forgot", "Reset your password"), get("action=resend", "Request a verification email"),
		get("action=verify", "Code from your email"), get("action=reset", "Choose your new password"),
		get("action=nonexistent", "Sign in to your account"), get("", "Sign in to your account"), get("action=home", "Sign in to your account"),
		get("action=release&id="+l1, "Change server / IP"), get("action=register&lang=en", "Create your CelikPanel account"),
		get("action=login", "Sign in to your account"),
		post("action=login", form("email", "alice@example.com", "password", "wrong-password-123"), "The email or password is incorrect."),
		post("action=login", form("email", `"<x>'&@example.com`, "password", "wrong-password-123"), "&quot;&lt;x&gt;&#039;&amp;@example.com"),
		post("action=login", form("email", "\xff\xfe@x\xe2\x82", "password", "wrong-password-123"), "Enter a valid email address."),
		post("action=register", form("email", "new@example.com"), "Accept the terms to continue."),
		{method: http.MethodPost, query: "action=login", form: form("email", "alice@example.com", "csrf", "wrong"), keepCSRF: true,
			expect: "Your session changed.", articles: -1},
		{method: http.MethodPost, query: "action=login", form: form("email", "alice@example.com", "password", parityPassword),
			origin: "https://attacker.example", expect: "The request could not be completed.", articles: -1},
		post("action=issue", form(), "Sign in to continue."),
		post("action=nonexistent", form(), "The request could not be completed."),
		post("action=login", form("email", "alice@example.com", "password", parityPassword), ""), // 303, followed below
		home("action=home&lang=en", "alice@example.com"), home("action=home", "Your licenses"),
		get("action=remove&id="+l1+"&lang=en", "This license is used by frankfurt.example.com (8.8.8.8)."),
		get("action=remove&id="+l4+"&lang=tr", "Bu lisans bir sunucuya bağlı değil."), get("action=remove&id="+l4+"&lang=en", "This license is not bound to a server."),
		get("action=release&id="+l2+"&lang=tr", "Sunucu eşlemesi kaldırılacak."), get("action=reveal&id="+l1, "Anahtarınızı görüntülemek"),
		get("action=rotate&id="+l1, "Mevcut anahtar iptal edilip"), get("action=renew&id="+l1, "Şu an yenileme ücretsizdir."),
		get("action=renew&id="+l1+"&lang=en", "Renewal is currently free."), get("action=remove&id=%22%3E%3Cscript%3E", "&quot;&gt;&lt;script&gt;"),
		get("action=remove&id[]=1", `value="Array"`), get("action=terms&id=x%27y", "&amp;id=x&#039;y"),
		get("action=nonexistent&lang=en", "Your licenses"),
		post("action=reveal", form("id", l1, "password", "wrong-password-123"), "The email or password is incorrect."),
		post("action=reveal", form("id", l1, "password", parityPassword), ""), // 303 to the key, followed below
		home("action=home", "Your licenses"),
		post("action=renew", form("id", l1, "password", parityPassword), "Renew during the last 30 days"),
		post("action=release", form("id", strings.Repeat("f", 32), "password", parityPassword), "License not found."),
		{method: http.MethodPost, query: "action=release&lang=en", form: form("id", l1, "password", parityPassword, "extra[]", "x"),
			expect: "The request could not be completed.", articles: -1},
		get("action=activate", `{"error":"method_not_allowed"}`),
		{method: http.MethodPost, query: "action=activate", contentType: "application/json",
			raw:    `{"key":"CPK-` + strings.Repeat("0", 64) + `","server_id":"` + strings.Repeat("a", 64) + `","hostname":"h.example.com"}`,
			expect: `{"error":"invalid_license"}`, articles: -1},
		{method: http.MethodPost, query: "action=refresh", contentType: "application/json", raw: `[]`, expect: `{"error":"invalid_activation"}`, articles: -1},
		{method: http.MethodPost, query: "action=activate", contentType: "text/plain", raw: `{}`, expect: `{"error":"invalid_request"}`, articles: -1},
		{method: http.MethodPut, query: "action=home", articles: -1},
		get("action[]=home", "Membership is temporarily unavailable."),
		post("action=logout", form(), ""), // 303, followed below
		get("action=home", "Sign in to your account"),
	}
	redirects := 0
	for i, step := range steps {
		label := fmt.Sprintf("step %d %s %s", i, step.method, step.query)
		p, g := phpClient.do(t, step), goClient.do(t, step)
		compareAnswers(t, label, p, g)
		if p.status == http.StatusSeeOther && g.status == http.StatusSeeOther && p.headers["Location"] == g.headers["Location"] {
			redirects++
			target := p.headers["Location"]
			p = phpClient.send(t, http.MethodGet, phpOrigin+target, "", "", nil)
			g = goClient.send(t, http.MethodGet, goServer.URL+target, "", "", nil)
			compareAnswers(t, label+" -> "+target, p, g)
		}
		for name, a := range map[string]pageAnswer{"PHP": p, "Go": g} {
			if step.expect != "" && !strings.Contains(a.body, step.expect) {
				t.Errorf("%s: %s's answer lacks %q", label, name, step.expect)
			}
			if step.articles >= 0 && strings.Count(a.body, `<article class="member-license">`) != step.articles {
				t.Errorf("%s: %s lists %d licences, want %d", label, name, strings.Count(a.body, `<article class="member-license">`), step.articles)
			}
		}
	}
	if redirects != 3 {
		t.Errorf("%d redirects were compared, want 3 (sign-in, key, sign-out)", redirects)
	}
}

// TestPHPSecureCookieShapes compares the cookie both services set for an https origin:
// the __Host- name with Secure, HttpOnly, Path=/ and SameSite=Lax, and the cache headers.
// PHP's built-in server speaks only plain HTTP, so its header is read there; the Go side
// is served over TLS, where a browser keeps and returns such a cookie
// (TestSecureCookieWorksOverTLS shows the round trip).
func TestPHPSecureCookieShapes(t *testing.T) {
	b := buildPHPBase(t)
	phpPort := freePort(t)
	phpDir := filepath.Join(b.dir, "php-https")
	writePrivate(t, phpDir, "https://account.example", b.secret, b.db)
	startPHPServer(t, b.php, phpPort, phpDir)
	goDir := filepath.Join(b.dir, "go-https")
	goServer := httptest.NewUnstartedServer(&Handler{Private: goDir, AllowTestKey: b.public, RemoteAddr: PeerAddress})
	goServer.StartTLS()
	defer goServer.Close()
	writePrivate(t, goDir, "https://account.example", b.secret, b.db)
	p := newPageClient("PHP", "http://127.0.0.1:"+strconv.Itoa(phpPort), nil).do(t, pageStep{method: http.MethodGet, query: "action=register"})
	g := newPageClient("Go", goServer.URL, goServer.Client().Transport).do(t, pageStep{method: http.MethodGet, query: "action=register"})
	compareAnswers(t, "register over an https origin", p, g)
	if g.cookie != "__Host-celikpanel_member; httponly; path=/; samesite=lax; secure" {
		t.Fatalf("Go's secure cookie: %q", g.cookie)
	}
}

// TestPHPAndGoShareTheFile: while Go holds a write transaction on the live file (an
// activation in progress), PHP's activation of the same licence waits on the SQLite lock and
// then sees Go's binding: one winner across the two processes.
func TestPHPAndGoShareTheFile(t *testing.T) {
	b := buildPHPBase(t)
	db, err := sql.Open("sqlite", dsn(b.db, "rw"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	tx, err := db.Begin() // BEGIN IMMEDIATE (_txlock=immediate): the write lock is held from here
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec("UPDATE licenses SET activated=?,server_id=?,hostname=?,server_ip=? WHERE id=?",
		b.t0, strings.Repeat("d", 64), "go.example.com", "8.8.8.8", b.ids[3]); err != nil {
		t.Fatal(err)
	}
	command := phpScript(t, b.php, "driver.php", map[string]any{"db": b.db, "secret": base64.StdEncoding.EncodeToString(b.secret),
		"now": b.t0 + 60, "ops": []driverOp{{Op: "activate", Args: []string{b.keys[3], strings.Repeat("e", 64), "php.example.com", "9.9.9.9"}}}})
	type ran struct {
		out []byte
		err error
	}
	done := make(chan ran, 1)
	go func() {
		out, err := command.Output()
		done <- ran{out, err}
	}()
	select {
	case r := <-done:
		tx.Rollback()
		t.Fatalf("PHP did not wait for Go's write lock: %s %v", r.out, r.err)
	case <-time.After(1500 * time.Millisecond):
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	select {
	case r := <-done:
		var results []driverResult
		if r.err != nil || json.Unmarshal(r.out, &results) != nil || len(results) != 1 || results[0].OK || results[0].Code != "license_in_use" {
			t.Fatalf("PHP after Go's binding: %s %v", r.out, r.err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("PHP never finished")
	}
	var ip string
	if err := db.QueryRow("SELECT server_ip FROM licenses WHERE id=?", b.ids[3]).Scan(&ip); err != nil || ip != "8.8.8.8" {
		t.Fatalf("the binding after both: %q %v", ip, err)
	}
}

// firstDifference shows where two pages part.
func firstDifference(a, b string) string {
	i := 0
	for i < len(a) && i < len(b) && a[i] == b[i] {
		i++
	}
	from := i - 80
	if from < 0 {
		from = 0
	}
	cut := func(s string) string {
		end := i + 120
		if end > len(s) {
			end = len(s)
		}
		if from > len(s) {
			return ""
		}
		return s[from:end]
	}
	return fmt.Sprintf("at byte %d:\nPHP …%q\nGo  …%q", i, cut(a), cut(b))
}
