package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Every claim of the site names where the repository proves it: the texts in
// site/sources.json, the changelog lines and the known problems in their own
// records. A citation is "path" or "path:line"; a citation of a line quotes a
// phrase of that line (says). The phrase is the proof and the line number
// only says where to look first (owner's site, 23 November 2026):
//
//   - the phrase is on the cited line: the citation holds;
//   - it is not, but occurs exactly once elsewhere in the file: it holds, the
//     line has only moved; `go run ./site -fix-sources` rewrites the number;
//   - it occurs nowhere in the file: it fails, the document no longer says it;
//   - it occurs more than once and not on the cited line: it fails as
//     ambiguous, and the phrase must be lengthened.
//
// A phrase has at least minPhrase characters and a letter, so a date or a
// number alone cannot hold a citation in an unrelated place.
//
// Kaynaklar: alıntılanan ifade kanıttır, satır numarası yalnız yol gösterir.
// İfade dosyada bir kez başka satırdaysa kaynak geçerlidir (satır kaymıştır,
// `go run ./site -fix-sources` numarayı düzeltir); hiç yoksa ya da birden çok
// yerde olup belirtilen satırda yoksa test başarısız olur.

// minPhrase is the shortest phrase, in characters, a citation may quote.
const minPhrase = 12

// citation is one source: written as {"cite": …, "says": …}, or in the
// records as a bare "path:line" string without a phrase.
type citation struct {
	Cite string `json:"cite"`
	Says string `json:"says,omitempty"`
}

func (c *citation) UnmarshalJSON(raw []byte) error {
	if len(raw) > 0 && raw[0] == '"' {
		*c = citation{}
		return json.Unmarshal(raw, &c.Cite)
	}
	type plain citation
	var p plain
	if err := decodeStrict(raw, &p); err != nil {
		return err
	}
	*c = citation(p)
	return nil
}

// split returns the cited file and line; the line is 0 for a whole file.
func (c citation) split() (string, int) {
	if i := strings.LastIndex(c.Cite, ":"); i > 0 {
		if n, err := strconv.Atoi(c.Cite[i+1:]); err == nil {
			return c.Cite[:i], n
		}
	}
	return c.Cite, 0
}

// phraseProblem says why a phrase cannot prove a citation, or returns nil.
func phraseProblem(says string) error {
	if utf8.RuneCountInString(strings.TrimSpace(says)) < minPhrase {
		return fmt.Errorf("the phrase %q is too short: quote at least %d characters of the line", says, minPhrase)
	}
	if strings.IndexFunc(says, unicode.IsLetter) < 0 {
		return fmt.Errorf("the phrase %q is only digits and punctuation: quote words of the line", says)
	}
	return nil
}

// locate applies the phrase rule to the lines of the cited file and returns
// the line that holds the phrase: the cited line when it is there, otherwise
// the phrase's only occurrence. A citation of a whole file only needs the
// phrase to occur.
func locate(lines []string, c citation) (int, error) {
	_, line := c.split()
	if err := phraseProblem(c.Says); err != nil {
		return 0, err
	}
	if line > 0 && line <= len(lines) && strings.Contains(lines[line-1], c.Says) {
		return line, nil
	}
	found, count := 0, 0
	for i, text := range lines {
		if n := strings.Count(text, c.Says); n > 0 {
			found, count = i+1, count+n
		}
	}
	switch {
	case count == 0:
		return 0, fmt.Errorf("the file no longer says %q: correct the claim, or cite where the document proves it now", c.Says)
	case line == 0:
		return 0, nil
	case count > 1:
		return 0, fmt.Errorf("%q is not on line %d and occurs %d times in the file: ambiguous — lengthen the phrase", c.Says, line, count)
	}
	return found, nil
}

// repoFiles reads repository files as lines, once each.
type repoFiles struct {
	root  string
	lines map[string][]string
}

func newRepoFiles(root string) *repoFiles {
	return &repoFiles{root: root, lines: map[string][]string{}}
}

func (r *repoFiles) read(name string) ([]string, error) {
	if lines, ok := r.lines[name]; ok {
		return lines, nil
	}
	raw, err := os.ReadFile(filepath.Join(r.root, filepath.FromSlash(name)))
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
	r.lines[name] = lines
	return lines, nil
}

// check checks one citation and returns the line that proves it (0 for a
// whole file). The file must exist; a phrase follows the phrase rule; a line
// cited without a phrase (allowed in the records only) must exist and not be
// empty.
func (r *repoFiles) check(c citation) (int, error) {
	name, line := c.split()
	lines, err := r.read(name)
	if err != nil {
		return 0, err
	}
	if c.Says != "" {
		return locate(lines, c)
	}
	if line > 0 && (line > len(lines) || strings.TrimSpace(lines[line-1]) == "") {
		return 0, fmt.Errorf("%s has no line %d, or it is empty", name, line)
	}
	return line, nil
}

// citeObject matches a citation written as {"cite": …, "says": …}, in that
// order: the form every catalogue uses, and the only one the fixer rewrites.
var citeObject = regexp.MustCompile(`"cite"\s*:\s*("(?:[^"\\]|\\.)*")\s*,\s*"says"\s*:\s*("(?:[^"\\]|\\.)*")`)

// fixCitations rewrites, in one catalogue file, the line number of every
// citation whose phrase has moved to exactly one other line, and changes no
// other byte. It returns the new content, the citations it moved and those
// that do not hold.
func fixCitations(raw []byte, files *repoFiles) ([]byte, []string, []error) {
	var (
		out    []byte
		moved  []string
		failed []error
		last   int
	)
	for _, m := range citeObject.FindAllSubmatchIndex(raw, -1) {
		var c citation
		if json.Unmarshal(raw[m[2]:m[3]], &c.Cite) != nil || json.Unmarshal(raw[m[4]:m[5]], &c.Says) != nil {
			failed = append(failed, fmt.Errorf("unreadable citation %s", raw[m[0]:m[1]]))
			continue
		}
		name, line := c.split()
		at, err := files.check(c)
		if err != nil {
			failed = append(failed, fmt.Errorf("%s: %w", c.Cite, err))
			continue
		}
		if line == 0 || at == line {
			continue
		}
		cite := name + ":" + strconv.Itoa(at)
		quoted, err := json.Marshal(cite)
		if err != nil {
			failed = append(failed, err)
			continue
		}
		out = append(append(out, raw[last:m[2]]...), quoted...)
		last = m[3]
		moved = append(moved, c.Cite+" → "+cite)
	}
	return append(out, raw[last:]...), moved, failed
}

// phrasesIn counts the citations with a phrase in a decoded catalogue.
func phrasesIn(value any) int {
	count := 0
	switch v := value.(type) {
	case map[string]any:
		_, says := v["says"].(string)
		_, cite := v["cite"].(string)
		if says && cite {
			count++
		}
		for _, item := range v {
			count += phrasesIn(item)
		}
	case []any:
		for _, item := range v {
			count += phrasesIn(item)
		}
	}
	return count
}

// catalogueFiles lists, relative to the repository root, the files whose
// citations the fixer keeps current.
func catalogueFiles(root string) ([]string, error) {
	changelog, err := filepath.Glob(filepath.Join(root, "site", "changelog", "*.json"))
	if err != nil {
		return nil, err
	}
	names := []string{"site/sources.json", "site/problems.json"}
	for _, name := range changelog {
		rel, err := filepath.Rel(root, name)
		if err != nil {
			return nil, err
		}
		names = append(names, filepath.ToSlash(rel))
	}
	return names, nil
}

// fixSources brings the line numbers of every catalogue's citations up to
// date (`go run ./site -fix-sources`, run from the repository root): a phrase
// that moved to exactly one other line gets that line's number. With write
// false it only reports. It returns what moved; the error lists every
// citation that does not hold, which a person must correct.
func fixSources(root string, write bool) ([]string, error) {
	names, err := catalogueFiles(root)
	if err != nil {
		return nil, err
	}
	files := newRepoFiles(root)
	var moved []string
	var failed []error
	for _, name := range names {
		path := filepath.Join(root, filepath.FromSlash(name))
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var decoded any
		if err := json.Unmarshal(raw, &decoded); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		fixed, movedHere, failedHere := fixCitations(raw, files)
		if want, seen := phrasesIn(decoded), len(citeObject.FindAll(raw, -1)); want != seen {
			failed = append(failed, fmt.Errorf("%s: %d citations quote a phrase but only %d are written as {\"cite\": …, \"says\": …} in that order", name, want, seen))
		}
		for _, m := range movedHere {
			moved = append(moved, name+": "+m)
		}
		for _, f := range failedHere {
			failed = append(failed, fmt.Errorf("%s: %w", name, f))
		}
		if write && len(movedHere) > 0 {
			info, err := os.Stat(path)
			if err != nil {
				return nil, err
			}
			if err := os.WriteFile(path, fixed, info.Mode().Perm()); err != nil {
				return nil, err
			}
		}
	}
	return moved, errors.Join(failed...)
}
