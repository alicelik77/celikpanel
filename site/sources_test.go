package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The phrase rule itself (sources.go), on a small file.
func TestCitationPhraseRule(t *testing.T) {
	lines := []string{
		"# A document",
		"The panel keeps thirty days of backups.",
		"",
		"Every site has its own account.",
		"Every site has its own account.",
		"Released on 2026-11-23 12:00.",
	}
	for _, tc := range []struct {
		name, cite, says string
		line             int
		fails            string
	}{
		{"on its line", "doc.md:2", "thirty days of backups", 2, ""},
		{"moved once", "doc.md:1", "thirty days of backups", 2, ""},
		{"beyond the end, moved once", "doc.md:40", "thirty days of backups", 2, ""},
		{"gone", "doc.md:2", "sixty days of backups", 0, "no longer says"},
		{"ambiguous", "doc.md:2", "its own account", 0, "ambiguous"},
		{"repeated but on its line", "doc.md:5", "its own account", 5, ""},
		{"too short", "doc.md:2", "thirty days", 0, "too short"},
		{"too short with spaces", "doc.md:2", "  thirty  ", 0, "too short"},
		{"only digits and punctuation", "doc.md:6", "2026-11-23 12:00", 0, "only digits"},
		{"whole file, phrase present", "doc.md", "has its own account", 0, ""},
		{"whole file, phrase gone", "doc.md", "has no account at all", 0, "no longer says"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			line, err := locate(lines, citation{Cite: tc.cite, Says: tc.says})
			switch {
			case tc.fails == "" && err != nil:
				t.Fatalf("fails: %v", err)
			case tc.fails != "" && (err == nil || !strings.Contains(err.Error(), tc.fails)):
				t.Fatalf("error %v, want one saying %q", err, tc.fails)
			case tc.fails == "" && line != tc.line:
				t.Fatalf("line %d, want %d", line, tc.line)
			}
		})
	}
}

// The fixer rewrites only the line numbers of moved citations: every other
// byte of the catalogue, a citation on its line, one that fails and one of a
// whole file stay as they are; a second run changes nothing.
func TestFixSourcesRewritesOnlyLineNumbers(t *testing.T) {
	root := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("docs/a.md", "# A\n\nA new paragraph pushed everything down.\n\nThe panel keeps thirty days of backups.\nMail is signed with the sender’s domain.\n")
	write("site/sources.json", `{
  "about": "test",
  "sources": {
    "k": [
      {
        "cite": "docs/a.md:3",
        "says": "thirty days of backups"
      },
      {
        "cite": "docs/a.md:6",
        "says": "signed with the sender’s domain"
      },
      {
        "cite": "docs/a.md:3",
        "says": "a sentence that was rewritten"
      },
      {
        "cite": "docs/a.md"
      }
    ]
  }
}
`)
	write("site/problems.json", "{\"cases\": [{\"sources\": [\"docs/a.md:1\", {\"cite\": \"docs/a.md:4\", \"says\": \"thirty days of backups\"}]}]}\n")
	write("site/changelog/v0.1.0-alpha.1.json", "{\"source\": {\"cite\":\"docs/a.md:6\",\"says\":\"Mail is signed with the\"}}\n")

	before := map[string]string{}
	for _, name := range []string{"site/sources.json", "site/problems.json", "site/changelog/v0.1.0-alpha.1.json"} {
		raw, _ := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		before[name] = string(raw)
	}
	moved, err := fixSources(root, true)
	if err == nil || !strings.Contains(err.Error(), "a sentence that was rewritten") || strings.Count(err.Error(), "\n") != 0 {
		t.Fatalf("the rewritten sentence must be the one failure, got %v", err)
	}
	if len(moved) != 2 {
		t.Fatalf("moved %q, want the two moved citations", moved)
	}
	want := map[string]string{
		"site/sources.json": strings.Replace(before["site/sources.json"], `"cite": "docs/a.md:3",
        "says": "thirty days of backups"`, `"cite": "docs/a.md:5",
        "says": "thirty days of backups"`, 1),
		"site/problems.json":                 strings.Replace(before["site/problems.json"], "docs/a.md:4", "docs/a.md:5", 1),
		"site/changelog/v0.1.0-alpha.1.json": before["site/changelog/v0.1.0-alpha.1.json"],
	}
	for name, body := range want {
		raw, _ := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if string(raw) != body {
			t.Errorf("%s after the fix:\n%s\nwant:\n%s", name, raw, body)
		}
	}
	if moved, _ := fixSources(root, true); len(moved) != 0 {
		t.Errorf("a second run moved %q", moved)
	}

	// A citation written with its phrase first is one the fixer cannot see.
	write("site/changelog/v0.1.0-alpha.1.json", "{\"source\": {\"says\": \"Mail is signed with the\", \"cite\": \"docs/a.md:6\"}}\n")
	if _, err := fixSources(root, false); err == nil || !strings.Contains(err.Error(), "in that order") {
		t.Errorf("a phrase before its cite: %v", err)
	}
}

// Every citation of the repository's catalogues is one the fixer can see and
// holds; a moved one is only reported (the release procedure runs
// `go run ./site -fix-sources`).
func TestCataloguesFixable(t *testing.T) {
	moved, err := fixSources("..", false)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range moved {
		t.Logf("moved: %s", m)
	}
}
