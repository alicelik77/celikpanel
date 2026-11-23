package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// holds says whether a record's citation holds (sources.go): the file
// exists; with a phrase, the phrase rule; with a line and no phrase, the line
// exists and is not empty. A failure is reported, naming the record.
func holds(t *testing.T, files *repoFiles, what string, ref citation) bool {
	t.Helper()
	_, line := ref.split()
	at, err := files.check(ref)
	switch {
	case err != nil:
		t.Errorf("%s: source %s: %v", what, ref.Cite, err)
		return false
	case at != line:
		t.Logf("%s: source %s: the phrase is now on line %d; `go run ./site -fix-sources` updates the number", what, ref.Cite, at)
	}
	return true
}

var publishedOn = regexp.MustCompile("(?m)^> \\*\\*Published\\*\\* on ([0-9]{4}-[0-9]{2}-[0-9]{2}) \\(UTC\\)")

// The changelog and the known problems: every line and every case names a
// release-notes line that exists; a release's day is its notes' Published
// day, and only the release being built (the version get.sh pins, its notes
// not yet published) may leave it empty for the portal builder to fill; every
// published release from alpha.141 on has its changelog file (a
// release adds one when its publication is recorded); a fixed case names a
// published release; a case carries no field about who reported it.
func TestProblemsAndChangelog(t *testing.T) {
	releases, cases, err := loadRecords()
	if err != nil {
		t.Fatal(err)
	}
	bootstrap, err := os.ReadFile(filepath.Join(portal, "get.sh"))
	if err != nil {
		t.Fatal(err)
	}
	pin := pinnedVersion.FindSubmatch(bootstrap)
	if pin == nil {
		t.Fatal("download-portal/get.sh pins no release version")
	}
	pinned := string(pin[1])
	repo := newRepoFiles("..")
	have := map[string]changelogRelease{}
	for _, release := range releases {
		have[release.Version] = release
		notes, err := os.ReadFile(filepath.Join("..", filepath.FromSlash(release.Notes)))
		if err != nil {
			t.Errorf("%s: %v", release.Version, err)
			continue
		}
		match := publishedOn.FindSubmatch(notes)
		switch {
		case release.Published == "" && match != nil:
			t.Errorf("%s: its notes record the publication on %s; give the changelog file that day", release.Version, match[1])
		case release.Published == "" && release.Version != pinned:
			// Only the release being built (the one download-portal/get.sh
			// pins) may have a block before it is published.
			t.Errorf("%s has no published day and is not the release being built (%s)", release.Version, pinned)
		case release.Published != "" && (match == nil || string(match[1]) != release.Published):
			t.Errorf("%s: published %s, but its notes record no such publication", release.Version, release.Published)
		}
		holds(t, repo, release.Version, release.Source)
		if !strings.HasPrefix(release.Source.Cite, release.Notes+":") {
			t.Errorf("%s: source %s is not a line of its notes", release.Version, release.Source.Cite)
		}
		if len(release.Lines) > 8 {
			t.Errorf("%s: %d lines; keep a release to a handful", release.Version, len(release.Lines))
		}
		for _, line := range release.Lines {
			holds(t, repo, release.Version+" "+strconv.Quote(line.Text.EN), line.Source)
			if !strings.HasPrefix(line.Source.Cite, release.Notes+":") {
				t.Errorf("%s: %q is proved from another release's notes (%s)", release.Version, line.Text.EN, line.Source.Cite)
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
	allowed := map[string]bool{"id": true, "opened": true, "state": true, "fixed_in": true, "summary": true, "meanwhile": true, "note": true, "reason": true, "sources": true}
	for _, c := range loose.Cases {
		for field := range c {
			if !allowed[field] {
				t.Errorf("problems.json: a case carries the field %q; cases hold our own summary only, never reporter data", field)
			}
		}
	}
	for _, c := range cases {
		for _, s := range c.Sources {
			holds(t, repo, c.ID, s)
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
