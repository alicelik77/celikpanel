package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strings"
)

// The changelog and the known problems are records, not page text: one file
// per release under changelog/ and one list of cases in problems.json. Each
// line and each case names the release-notes line that proves it, in both
// languages side by side. The pages are written from them.
//
// Değişiklik günlüğü ve bilinen sorunlar kayıttır: sürüm başına bir dosya
// (changelog/) ve bir vaka listesi (problems.json); her satır kanıtladığı
// sürüm notu satırını adlandırır.

// both is one text in both languages.
type both struct {
	EN string `json:"en"`
	TR string `json:"tr"`
}

func (b both) in(lang string) string {
	if lang == "tr" {
		return b.TR
	}
	return b.EN
}

// A changelog line: what kind of change, one sentence a server owner
// understands, the case it closes or names (if any), and its proof.
type changeLine struct {
	Kind   string `json:"kind"`
	Text   both   `json:"text"`
	Case   string `json:"case,omitempty"`
	Source string `json:"source"`
}

type changelogRelease struct {
	Version   string       `json:"version"`
	Published string       `json:"published"`
	Notes     string       `json:"notes"`
	Source    string       `json:"source"`
	Lines     []changeLine `json:"lines"`
}

// A case: our number, when it was first stated, its state, our own summary,
// what to do meanwhile or why it is not a defect, and its proof. There is
// deliberately no field for a reporter, a server, a domain or the reporter's
// own words.
type problemCase struct {
	ID        string   `json:"id"`
	Opened    string   `json:"opened"`
	State     string   `json:"state"`
	FixedIn   string   `json:"fixed_in,omitempty"`
	Summary   both     `json:"summary"`
	Meanwhile *both    `json:"meanwhile,omitempty"`
	Reason    *both    `json:"reason,omitempty"`
	Sources   []string `json:"sources"`
}

type problemsFile struct {
	About string        `json:"about"`
	Cases []problemCase `json:"cases"`
}

var (
	changeKinds   = []string{"security", "fixed", "new", "limits"}
	caseStates    = []string{"looking", "accepted", "fixed", "not-defect"}
	caseNumber    = regexp.MustCompile(`^CP-(20[0-9]{2})-([1-9][0-9]*)$`)
	releaseName   = regexp.MustCompile(`^v0\.1\.0-alpha\.([0-9]+)$`)
	isoDay        = regexp.MustCompile(`^20[0-9]{2}-[01][0-9]-[0-3][0-9]$`)
	sourcePattern = regexp.MustCompile(`^[A-Za-z0-9_./-]+(:[0-9]+)?$`)
)

func decodeStrict(raw []byte, into any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	return decoder.Decode(into)
}

func alphaNumber(version string) int {
	match := releaseName.FindStringSubmatch(version)
	if match == nil {
		return -1
	}
	n := 0
	fmt.Sscanf(match[1], "%d", &n)
	return n
}

// loadRecords reads and checks both records: known kinds and states, both
// languages filled, a source everywhere, cases that exist, a "fixed" case
// that names a release of the changelog. Releases come newest first, cases
// newest first.
func loadRecords() ([]changelogRelease, []problemCase, error) {
	names, err := fs.Glob(sources, "changelog/*.json")
	if err != nil {
		return nil, nil, err
	}
	var releases []changelogRelease
	versions := map[string]bool{}
	for _, name := range names {
		raw, err := sources.ReadFile(name)
		if err != nil {
			return nil, nil, err
		}
		var release changelogRelease
		if err := decodeStrict(raw, &release); err != nil {
			return nil, nil, fmt.Errorf("%s: %w", name, err)
		}
		if alphaNumber(release.Version) < 0 || name != "changelog/"+release.Version+".json" {
			return nil, nil, fmt.Errorf("%s: the file must be named after its version, v0.1.0-alpha.N", name)
		}
		if !isoDay.MatchString(release.Published) || !sourcePattern.MatchString(release.Source) ||
			release.Notes != "docs/RELEASE-NOTES-"+release.Version+".md" || len(release.Lines) == 0 {
			return nil, nil, fmt.Errorf("%s: needs its published day, its notes file and the line that records the publication", name)
		}
		for i, line := range release.Lines {
			if !contains(changeKinds, line.Kind) || strings.TrimSpace(line.Text.EN) == "" || strings.TrimSpace(line.Text.TR) == "" || !sourcePattern.MatchString(line.Source) {
				return nil, nil, fmt.Errorf("%s: line %d needs a known kind, both languages and a source", name, i+1)
			}
		}
		versions[release.Version] = true
		releases = append(releases, release)
	}
	sort.Slice(releases, func(i, j int) bool { return alphaNumber(releases[i].Version) > alphaNumber(releases[j].Version) })

	raw, err := sources.ReadFile("problems.json")
	if err != nil {
		return nil, nil, err
	}
	var file problemsFile
	if err := decodeStrict(raw, &file); err != nil {
		return nil, nil, fmt.Errorf("problems.json: %w", err)
	}
	ids := map[string]bool{}
	for _, c := range file.Cases {
		if !caseNumber.MatchString(c.ID) || ids[c.ID] {
			return nil, nil, fmt.Errorf("problems.json: %q is not a new CP-<year>-<n> number", c.ID)
		}
		ids[c.ID] = true
		switch {
		case !contains(caseStates, c.State):
			return nil, nil, fmt.Errorf("problems.json %s: unknown state %q", c.ID, c.State)
		case !isoDay.MatchString(c.Opened):
			return nil, nil, fmt.Errorf("problems.json %s: opened must be a day, YYYY-MM-DD", c.ID)
		case strings.TrimSpace(c.Summary.EN) == "" || strings.TrimSpace(c.Summary.TR) == "":
			return nil, nil, fmt.Errorf("problems.json %s: the summary needs both languages", c.ID)
		case len(c.Sources) == 0:
			return nil, nil, fmt.Errorf("problems.json %s: no source", c.ID)
		case (c.State == "fixed") != (c.FixedIn != ""):
			return nil, nil, fmt.Errorf("problems.json %s: a fixed case, and only a fixed case, names the release that fixed it", c.ID)
		case c.State == "fixed" && !versions[c.FixedIn]:
			return nil, nil, fmt.Errorf("problems.json %s: fixed in %s, which the changelog does not have", c.ID, c.FixedIn)
		case (c.State == "not-defect") != (c.Reason != nil):
			return nil, nil, fmt.Errorf("problems.json %s: a case that is not a defect, and only such a case, gives the reason", c.ID)
		}
		for _, s := range c.Sources {
			if !sourcePattern.MatchString(s) {
				return nil, nil, fmt.Errorf("problems.json %s: source %q", c.ID, s)
			}
		}
	}
	for _, release := range releases {
		for _, line := range release.Lines {
			if line.Case != "" && !ids[line.Case] {
				return nil, nil, fmt.Errorf("changelog %s names case %s, which problems.json does not have", release.Version, line.Case)
			}
		}
	}
	cases := file.Cases
	sort.SliceStable(cases, func(i, j int) bool { return caseOrder(cases[i].ID) > caseOrder(cases[j].ID) })
	return releases, cases, nil
}

func caseOrder(id string) int {
	match := caseNumber.FindStringSubmatch(id)
	year, n := 0, 0
	fmt.Sscanf(match[1], "%d", &year)
	fmt.Sscanf(match[2], "%d", &n)
	return year*100000 + n
}

func contains(list []string, item string) bool {
	for _, value := range list {
		if value == item {
			return true
		}
	}
	return false
}

// The template funcs: the records in the page's language, with the links
// between them made relative to the page.
func (r *renderer) recordFuncs(v view, lookup func(string) (string, error)) map[string]any {
	link := func(key, fragment string) string {
		target, _ := pagePath(key, v.Lang)
		return v.href(target) + "#" + fragment
	}
	notes := func(version string) string {
		file := "docs/RELEASE-NOTES-" + version + ".md"
		if v.Lang == "tr" {
			file = "docs/RELEASE-NOTES-" + version + ".tr.md"
		}
		return "https://github.com/alicelik77/celikpanel/blob/main/" + file
	}
	return map[string]any{
		"changelog": func() ([]map[string]any, error) {
			var out []map[string]any
			for _, release := range r.releases {
				var lines []map[string]any
				for _, kind := range changeKinds {
					for _, line := range release.Lines {
						if line.Kind != kind {
							continue
						}
						label, err := lookup("changelog.kind." + kind)
						if err != nil {
							return nil, err
						}
						entry := map[string]any{"Kind": label, "Text": line.Text.in(v.Lang), "Case": line.Case}
						if line.Case != "" {
							entry["CaseHref"] = link("problems", line.Case)
						}
						lines = append(lines, entry)
					}
				}
				out = append(out, map[string]any{
					"Version": release.Version, "Published": release.Published,
					"Notes": notes(release.Version), "Lines": lines,
				})
			}
			return out, nil
		},
		"cases": func() ([]map[string]any, error) {
			var out []map[string]any
			for _, c := range r.cases {
				state, err := lookup("problems.state." + c.State)
				if err != nil {
					return nil, err
				}
				entry := map[string]any{"ID": c.ID, "Opened": c.Opened, "Summary": c.Summary.in(v.Lang), "State": state, "Class": c.State}
				if c.FixedIn != "" {
					entry["FixedIn"] = c.FixedIn
					entry["FixedHref"] = link("changelog", c.FixedIn)
				}
				if c.Meanwhile != nil {
					entry["Meanwhile"] = c.Meanwhile.in(v.Lang)
				}
				if c.Reason != nil {
					entry["Reason"] = c.Reason.in(v.Lang)
				}
				out = append(out, entry)
			}
			return out, nil
		},
	}
}
