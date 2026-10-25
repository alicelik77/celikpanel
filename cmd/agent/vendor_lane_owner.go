package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alicelik/celikpanel/internal/core"
)

// An owner's own source for a vendor repository (alpha.108 review). The owner
// may have added Sury or PGDG by hand (Frankfurt's php.list, the base image's
// php.list). CelikPanel then never adds a second source for the same address:
// two sources with different Signed-By make apt refuse every update
// ("Conflicting values set for option Signed-By"). The lane offers only the
// confinement and the automatic updates for the owner's source, and a first
// enable validates the staged source together with every existing one.
//
// Sahibin kendi depo kaynağı: CelikPanel aynı adrese ikinci kaynak eklemez;
// yol yalnız sınırlamayı ve otomatik güncellemeleri sunar.

// aptMainSourcesList is apt's main sources file (a variable for tests).
var aptMainSourcesList = "/etc/apt/sources.list"

// ownerRepoSource is one source of the owner's for the repository's address.
type ownerRepoSource struct {
	Path string
	Loc  aptSourceLocation
}

const ownerSourceReadCap = 1 << 20

// repoAddressKey is a source address as apt compares it: no scheme, no
// trailing slash, lower-case host.
func repoAddressKey(uri string) string {
	u := strings.TrimSpace(uri)
	u = strings.TrimPrefix(strings.TrimPrefix(u, "https://"), "http://")
	u = strings.TrimSuffix(u, "/")
	host, path, _ := strings.Cut(u, "/")
	return strings.ToLower(host) + "/" + path
}

// parseOwnerSourceLine reads a one-line `deb` source of the owner's,
// http or https.
func parseOwnerSourceLine(line string) (aptSourceLocation, bool) {
	normalized := line
	if i := strings.Index(normalized, "http://"); i >= 0 {
		normalized = normalized[:i] + "https://" + normalized[i+len("http://"):]
	}
	loc, err := parseAptSourceLine(normalized)
	if err != nil {
		return aptSourceLocation{}, false
	}
	return loc, true
}

// deb822SourceLocations reads a .sources file's enabled `deb` stanzas.
func deb822SourceLocations(r io.Reader) []aptSourceLocation {
	var out []aptSourceLocation
	flush := func(fields map[string]string) {
		if len(fields) == 0 || strings.EqualFold(strings.TrimSpace(fields["enabled"]), "no") {
			return
		}
		if !strings.Contains(" "+fields["types"]+" ", " deb ") {
			return
		}
		for _, uri := range strings.Fields(fields["uris"]) {
			for _, suite := range strings.Fields(fields["suites"]) {
				if loc, ok := parseOwnerSourceLine("deb " + uri + " " + suite + " " + fields["components"]); ok {
					out = append(out, loc)
				}
			}
		}
	}
	fields := map[string]string{}
	last := ""
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64<<10), ownerSourceReadCap)
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "":
			flush(fields)
			fields, last = map[string]string{}, ""
		case strings.HasPrefix(trimmed, "#"):
		case line[0] == ' ' || line[0] == '\t':
			if last != "" {
				fields[last] += " " + trimmed
			}
		default:
			key, value, ok := strings.Cut(trimmed, ":")
			if ok {
				last = strings.ToLower(strings.TrimSpace(key))
				fields[last] = strings.TrimSpace(value)
			}
		}
	}
	flush(fields)
	return out
}

// ownerSourcesFor lists the owner's sources (not CelikPanel's own file) that
// point at the repository's address.
func ownerSourcesFor(repo *core.ManagedRepo, want aptSourceLocation) []ownerRepoSource {
	key := repoAddressKey(want.URI)
	own := repoSourcePath(repo.ID)
	files := []string{aptMainSourcesList}
	for _, pattern := range []string{"*.list", "*.sources"} {
		matches, _ := filepath.Glob(filepath.Join(aptSourcesListDir, pattern))
		files = append(files, matches...)
	}
	sort.Strings(files[1:])
	var out []ownerRepoSource
	for _, path := range files {
		if path == own || strings.HasPrefix(filepath.Base(path), ".") {
			continue
		}
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Size() > ownerSourceReadCap {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var locs []aptSourceLocation
		if strings.HasSuffix(path, ".sources") {
			locs = deb822SourceLocations(bytes.NewReader(data))
		} else {
			for _, line := range strings.Split(string(data), "\n") {
				line = strings.TrimSpace(line)
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				if loc, ok := parseOwnerSourceLine(line); ok {
					locs = append(locs, loc)
				}
			}
		}
		for _, loc := range locs {
			if repoAddressKey(loc.URI) == key {
				out = append(out, ownerRepoSource{Path: path, Loc: loc})
			}
		}
	}
	return out
}

func ownerSourcePaths(sources []ownerRepoSource) string {
	seen := map[string]bool{}
	var paths []string
	for _, source := range sources {
		if !seen[source.Path] {
			seen[source.Path] = true
			paths = append(paths, source.Path)
		}
	}
	return strings.Join(paths, ", ")
}

// stagedSourcePartsDir is a copy of apt's sources.list.d with the staged
// source added as CelikPanel's file, so the staged `apt-get update` reads it
// together with every existing source (sources.list stays apt's own) and a
// conflict between them is found before anything is written.
func stagedSourcePartsDir(stagedSource, repoID string) (string, func(), error) {
	if err := os.MkdirAll(filepath.Dir(repoRecordPath), 0o700); err != nil {
		return "", func() {}, err
	}
	dir, err := os.MkdirTemp(filepath.Dir(repoRecordPath), ".lane-sources-*")
	if err != nil {
		return "", func() {}, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	for _, pattern := range []string{"*.list", "*.sources"} {
		matches, _ := filepath.Glob(filepath.Join(aptSourcesListDir, pattern))
		for _, path := range matches {
			name := filepath.Base(path)
			if strings.HasPrefix(name, ".") || path == repoSourcePath(repoID) {
				continue
			}
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > ownerSourceReadCap {
				continue
			}
			data, err := os.ReadFile(path)
			if err != nil {
				cleanup()
				return "", func() {}, fmt.Errorf("read %s: %w", path, err)
			}
			if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
				cleanup()
				return "", func() {}, err
			}
		}
	}
	staged, err := os.ReadFile(stagedSource)
	if err != nil {
		cleanup()
		return "", func() {}, err
	}
	if err := os.WriteFile(filepath.Join(dir, filepath.Base(repoSourcePath(repoID))), staged, 0o644); err != nil {
		cleanup()
		return "", func() {}, err
	}
	// apt drops to _apt for downloads but reads the source parts as root;
	// the directory is still made readable like apt's own.
	_ = os.Chmod(dir, 0o755)
	return dir, cleanup, nil
}
