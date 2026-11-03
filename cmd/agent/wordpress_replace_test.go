package main

import (
	"bytes"
	"strconv"
	"strings"
	"testing"
)

func ser(value string) string { return "s:" + strconv.Itoa(len(value)) + `:"` + value + `";` }

// The address change of a WordPress copy must keep PHP serialized values
// valid: every string length is written again, nested values are walked,
// and a value that cannot be parsed is left untouched rather than broken.

func replaceForTest(value string) (string, bool, bool) {
	out, changed, skipped := replaceInValue([]byte(value), wordpressAddressPairs("live.example", "staging.example.org"))
	return string(out), changed, skipped
}

func TestReplaceKeepsSerializedLengthsValid(t *testing.T) {
	in := `a:2:{` + ser("home") + ser("https://live.example/blog") + ser("other") + `i:5;}`
	// The input itself is valid: "https://live.example/blog" is 24 bytes.
	if r := (&serializedReplacer{in: []byte(in), pairs: nil}); r.value() != nil || r.pos != len(in) {
		t.Fatal("fixture is not a valid serialized value")
	}
	out, changed, skipped := replaceForTest(in)
	want := `a:2:{` + ser("home") + ser("https://staging.example.org/blog") + ser("other") + `i:5;}`
	if !changed || skipped || out != want {
		t.Fatalf("got %q changed=%v skipped=%v, want %q", out, changed, skipped, want)
	}
	// The output parses again, completely.
	r := &serializedReplacer{in: []byte(out)}
	if err := r.value(); err != nil || r.pos != len(out) {
		t.Fatalf("output is not a valid serialized value: %v", err)
	}
}

func TestReplaceWalksNestedSerializedStringsAndObjects(t *testing.T) {
	inner := `a:1:{i:0;s:19:"http://live.example";}`
	in := `O:8:"stdClass":2:{s:3:"url";s:20:"//live.example/x.png";s:5:"inner";s:` + strconv.Itoa(len(inner)) + `:"` + inner + `";}`
	out, changed, skipped := replaceForTest(in)
	if !changed || skipped {
		t.Fatalf("changed=%v skipped=%v", changed, skipped)
	}
	newInner := `a:1:{i:0;s:26:"http://staging.example.org";}`
	want := `O:8:"stdClass":2:{s:3:"url";s:27:"//staging.example.org/x.png";s:5:"inner";s:` + strconv.Itoa(len(newInner)) + `:"` + newInner + `";}`
	if out != want {
		t.Fatalf("got  %q\nwant %q", out, want)
	}
}

func TestReplaceLeavesUnparsableSerializedValueUntouched(t *testing.T) {
	// The length is wrong already (a broken value from elsewhere): changing
	// it would make things worse, so it is counted and left as it is.
	in := `a:1:{s:3:"url";s:99:"https://live.example";}`
	out, changed, skipped := replaceForTest(in)
	if changed || !skipped || out != in {
		t.Fatalf("got %q changed=%v skipped=%v", out, changed, skipped)
	}
}

func TestReplaceKeepsCustomSerializedPayloadOpaque(t *testing.T) {
	payload := ser("http://live.example")
	in := `C:11:"ArrayObject":` + strconv.Itoa(len(payload)) + `:{` + payload + `}`
	out, changed, skipped := replaceForTest(in)
	if changed || skipped || out != in {
		t.Fatalf("got %q changed=%v skipped=%v", out, changed, skipped)
	}
}

func TestReplaceRespectsHostBoundariesAndForms(t *testing.T) {
	cases := map[string]string{
		"see https://live.example/ and //live.example": "see https://staging.example.org/ and //staging.example.org",
		"https://www.live.example/a":                   "https://staging.example.org/a",
		`{"u":"https:\/\/live.example\/p"}`:            `{"u":"https:\/\/staging.example.org\/p"}`,
		"https%3A%2F%2Flive.example%2F":                "https%3A%2F%2Fstaging.example.org%2F",
		"https://live.example.com/":                    "https://live.example.com/",
		"https://live.examples/":                       "https://live.examples/",
		"https://live.example-shop/":                   "https://live.example-shop/",
		"admin@live.example":                           "admin@live.example",
		"end //live.example.":                          "end //staging.example.org.",
	}
	for in, want := range cases {
		if got, _, _ := replaceForTest(in); got != want {
			t.Errorf("%q -> %q, want %q", in, got, want)
		}
	}
}

func TestTransformSQLDumpChangesOnlyLiterals(t *testing.T) {
	dump := strings.Join([]string{
		"-- MariaDB dump; it's a comment with an apostrophe",
		"/*!40101 SET NAMES utf8mb4 */;",
		"CREATE TABLE `wp_options` (`option_value` longtext COMMENT 'it''s //live.example');",
		`INSERT INTO ` + "`wp_options`" + ` VALUES (1,'siteurl','https://live.example','yes'),(2,'widget','a:1:{s:3:\"url\";s:19:\"http://live.example\";}','yes'),(3,'note','O\'Brien\\path\nline //live.example','no');`,
		"",
	}, "\n")
	var out bytes.Buffer
	stats, err := transformSQLDump(strings.NewReader(dump), &out, wordpressAddressPairs("live.example", "staging.example.org"))
	if err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{
		"-- MariaDB dump; it's a comment with an apostrophe\n",
		"`wp_options`",
		`'https://staging.example.org'`,
		`'a:1:{s:3:\"url\";s:26:\"http://staging.example.org\";}'`,
		`'O\'Brien\\path\nline //staging.example.org'`,
		`'it\'s //staging.example.org'`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q:\n%s", want, got)
		}
	}
	if stats.Replaced != 4 || stats.Skipped != 0 {
		t.Errorf("stats = %+v, want 4 replaced", stats)
	}
}

func TestTransformSQLDumpRefusesATruncatedDump(t *testing.T) {
	var out bytes.Buffer
	if _, err := transformSQLDump(strings.NewReader("INSERT INTO t VALUES ('unterminated"), &out, nil); err == nil {
		t.Fatal("a dump that ends inside a value was accepted")
	}
}
