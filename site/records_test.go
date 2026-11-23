package main

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// sourceExists says whether "path" or "path:line" names a repository file
// and, with a line, a line that exists and is not empty.
func sourceExists(t *testing.T, ref string) bool {
	t.Helper()
	name, line := ref, 0
	if i := strings.LastIndex(ref, ":"); i > 0 {
		if n, err := strconv.Atoi(ref[i+1:]); err == nil {
			name, line = ref[:i], n
		}
	}
	handle, err := os.Open(filepath.Join("..", filepath.FromSlash(name)))
	if err != nil {
		return false
	}
	defer handle.Close()
	if line == 0 {
		return true
	}
	scanner := bufio.NewScanner(handle)
	scanner.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	for n := 1; scanner.Scan(); n++ {
		if n == line {
			return strings.TrimSpace(scanner.Text()) != ""
		}
	}
	return false
}

var publishedOn = regexp.MustCompile("(?m)^> \\*\\*Published\\*\\* on ([0-9]{4}-[0-9]{2}-[0-9]{2}) \\(UTC\\)")

// The changelog and the known problems: every line and every case names a
// release-notes line that exists; a release's day is its notes' Published
// day; every published release from alpha.141 on has its changelog file (a
// release adds one when its publication is recorded); a fixed case names a
// published release; a case carries no field about who reported it.
func TestProblemsAndChangelog(t *testing.T) {
	releases, cases, err := loadRecords()
	if err != nil {
		t.Fatal(err)
	}
	have := map[string]changelogRelease{}
	for _, release := range releases {
		have[release.Version] = release
		notes, err := os.ReadFile(filepath.Join("..", filepath.FromSlash(release.Notes)))
		if err != nil {
			t.Errorf("%s: %v", release.Version, err)
			continue
		}
		match := publishedOn.FindSubmatch(notes)
		if match == nil || string(match[1]) != release.Published {
			t.Errorf("%s: published %s, but its notes record no such publication", release.Version, release.Published)
		}
		if !sourceExists(t, release.Source) || !strings.HasPrefix(release.Source, release.Notes+":") {
			t.Errorf("%s: source %s is not a line of its notes", release.Version, release.Source)
		}
		if len(release.Lines) > 8 {
			t.Errorf("%s: %d lines; keep a release to a handful", release.Version, len(release.Lines))
		}
		for _, line := range release.Lines {
			if !sourceExists(t, line.Source) || !strings.HasPrefix(line.Source, "docs/RELEASE-NOTES-") {
				t.Errorf("%s: %q: source %s is not a release-notes line that exists", release.Version, line.Text.EN, line.Source)
			}
			if !strings.HasPrefix(line.Source, release.Notes+":") {
				t.Errorf("%s: %q is proved from another release's notes (%s)", release.Version, line.Text.EN, line.Source)
			}
		}
	}

	// Every published release from the first of the changelog on has its file.
	notes, err := filepath.Glob(filepath.Join("..", "docs", "RELEASE-NOTES-v0.1.0-alpha.*.md"))
	if err != nil {
		t.Fatal(err)
	}
	first := 141
	var missing []string
	for _, file := range notes {
		base := filepath.Base(file)
		if strings.HasSuffix(base, ".tr.md") {
			continue
		}
		version := strings.TrimSuffix(strings.TrimPrefix(base, "RELEASE-NOTES-"), ".md")
		if alphaNumber(version) < first {
			continue
		}
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if publishedOn.Match(raw) {
			if _, ok := have[version]; !ok {
				missing = append(missing, version)
			}
		}
	}
	sort.Strings(missing)
	for _, version := range missing {
		t.Errorf("%s is published but site/changelog/%s.json is missing: add its lines (see site/records.go) and run `go run ./site`", version, version)
	}

	raw, err := os.ReadFile("problems.json")
	if err != nil {
		t.Fatal(err)
	}
	var loose struct {
		Cases []map[string]json.RawMessage `json:"cases"`
	}
	if err := json.Unmarshal(raw, &loose); err != nil {
		t.Fatal(err)
	}
	allowed := map[string]bool{"id": true, "opened": true, "state": true, "fixed_in": true, "summary": true, "meanwhile": true, "reason": true, "sources": true}
	for _, c := range loose.Cases {
		for field := range c {
			if !allowed[field] {
				t.Errorf("problems.json: a case carries the field %q; cases hold our own summary only, never reporter data", field)
			}
		}
	}
	for _, c := range cases {
		for _, s := range c.Sources {
			if !sourceExists(t, s) {
				t.Errorf("%s: source %s does not exist", c.ID, s)
			}
		}
		if c.State == "fixed" {
			if _, ok := have[c.FixedIn]; !ok {
				t.Errorf("%s: fixed in %s, which has no changelog block", c.ID, c.FixedIn)
			}
		}
	}

	// The pages show every release and every case, with working anchors.
	files, _ := rendered(t)
	for _, page := range []string{"changelog.html", "tr/degisiklikler.html"} {
		for _, release := range releases {
			if !strings.Contains(string(files[page]), `id="`+release.Version+`"`) {
				t.Errorf("%s has no block for %s", page, release.Version)
			}
		}
	}
	for _, page := range []string{"problems.html", "tr/sorunlar.html"} {
		for _, c := range cases {
			if !strings.Contains(string(files[page]), `id="`+c.ID+`"`) {
				t.Errorf("%s does not list %s", page, c.ID)
			}
		}
	}
}
