package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/transport"
)

// A vendor repository is confined to its own component (alpha.107, D-038 §12).
// On Frankfurt the repository that brings PHP 8.5 and 8.6 was a candidate to
// replace the distribution's own PHP 8.4 packages with its builds on the next
// `apt upgrade`: the repository was enabled for one component and could
// change the whole system. CelikPanel now writes one apt preferences file per
// vendor repository it enables:
//
//	Package: *
//	Pin: origin "<the repository's host>"
//	Pin-Priority: 100
//
// Priority 100 is below the distribution's 500, so the repository provides a
// package only where the distribution has none (PHP 8.5 from Sury, PostgreSQL
// 18 from PGDG) and never replaces a distribution package, with the panel or
// with the owner's own `apt upgrade`. Packages that were already installed
// from the repository before it was confined (Kept) are named in a second
// record at 500, so they keep following it: confining never strands a
// package without updates. The file is CelikPanel's: its SHA-256 is recorded,
// and once someone changes it, it is kept as it is and the panel says so. An
// owner's own preferences file that already sets a rule for the repository is
// respected: CelikPanel writes none and names the owner's file.
//
// Bir üretici deposu kendi bileşeniyle sınırlanır: depo yalnız dağıtımın
// sunmadığı paketleri sağlar ve hiçbir dağıtım paketinin yerini almaz.
// Önceden depodan kurulmuş paketler depoyu izlemeye devam eder. Dosya
// CelikPanel'indir; biri değiştirirse olduğu gibi bırakılır. Sahibin kendi
// tercih dosyası varsa CelikPanel dosya yazmaz ve sahibin dosyasını adlandırır.

var (
	aptPreferencesDir         = "/etc/apt/preferences.d"
	aptPreferencesFile        = "/etc/apt/preferences"
	dpkgStatusPath            = "/var/lib/dpkg/status"
	repoRecordPath            = "/var/lib/celikpanel-agent-private/repo-record.json"
	repoConfinementNow        = time.Now
	repoConfinementRecordLock sync.Mutex
)

const (
	repoConfinementConfined   = "confined"
	repoConfinementEdited     = "edited"
	repoConfinementOwnerRule  = "owner_rule"
	repoConfinementNotWritten = "not_written"
	// repoConfinementRemovedByOwner: CelikPanel wrote the file and someone
	// deleted it; it is never written again unless the owner asks.
	repoConfinementRemovedByOwner = "removed_by_owner"
	repoConfinementUnknown        = "unknown"

	repoConfinementReasonListsUnreadable = "lists_unreadable"
	repoConfinementReasonWriteFailed     = "write_failed"
	repoConfinementReasonPlanChanged     = "plan_changed"

	repoConfinedByOwner = "owner"

	repoPinLowPriority  = 100
	repoPinKeepPriority = 500
	repoKeptCap         = 64
	repoListsReadCap    = 256 << 20
	repoPrefReadCap     = 256 << 10
	repoRecordReadCap   = 256 << 10
	repoAptErrorCap     = 8
)

func repoPinPath(id string) string {
	return filepath.Join(aptPreferencesDir, "celikpanel-"+id+".pref")
}

// aptSourceLocation is one "deb" line split into what apt names its lists by.
type aptSourceLocation struct {
	URI        string
	Suite      string
	Components []string
	Host       string
}

// parseAptSourceLine reads `deb [options] URI suite [component…]`.
func parseAptSourceLine(line string) (aptSourceLocation, error) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "deb ") {
		return aptSourceLocation{}, fmt.Errorf("not a deb source line")
	}
	rest := strings.TrimSpace(strings.TrimPrefix(line, "deb "))
	if strings.HasPrefix(rest, "[") {
		end := strings.Index(rest, "]")
		if end < 0 {
			return aptSourceLocation{}, fmt.Errorf("source options are not closed")
		}
		rest = strings.TrimSpace(rest[end+1:])
	}
	fields := strings.Fields(rest)
	if len(fields) < 2 {
		return aptSourceLocation{}, fmt.Errorf("source line has no suite")
	}
	uri := fields[0]
	if !strings.HasPrefix(uri, "https://") {
		return aptSourceLocation{}, fmt.Errorf("source is not https")
	}
	host := strings.TrimPrefix(uri, "https://")
	if i := strings.IndexAny(host, "/:"); i >= 0 {
		host = host[:i]
	}
	if host == "" {
		return aptSourceLocation{}, fmt.Errorf("source has no host")
	}
	return aptSourceLocation{URI: uri, Suite: fields[1], Components: fields[2:], Host: strings.ToLower(host)}, nil
}

func (l aptSourceLocation) flat() bool { return strings.HasSuffix(l.Suite, "/") }

// aptURIFileName is apt's own naming of a downloaded index (URItoFileName):
// the scheme dropped and every "/" turned into "_".
func aptURIFileName(uri string) string {
	u := strings.TrimPrefix(strings.TrimPrefix(uri, "https://"), "http://")
	u = strings.TrimSuffix(u, "/")
	return strings.ReplaceAll(u, "/", "_")
}

// listsPrefix is the common start of every list file apt keeps for this
// source: "<host>_<path>_dists_<suite>", or "<host>_<path>_<dir>" for a flat
// repository ("trixie/").
func (l aptSourceLocation) listsPrefix() string {
	if l.flat() {
		return aptURIFileName(strings.TrimSuffix(l.URI, "/") + "/" + strings.TrimSuffix(l.Suite, "/"))
	}
	return aptURIFileName(l.URI) + "_dists_" + strings.ReplaceAll(l.Suite, "/", "_")
}

func (l aptSourceLocation) packagesFiles() []string {
	prefix := filepath.Join(aptListsDir, l.listsPrefix())
	var files []string
	if l.flat() {
		files = append(files, prefix+"_Packages")
	} else {
		matches, _ := filepath.Glob(prefix + "_*_Packages")
		files = append(files, matches...)
	}
	out := files[:0]
	for _, file := range files {
		if info, err := os.Lstat(file); err == nil && info.Mode().IsRegular() {
			out = append(out, file)
		}
	}
	sort.Strings(out)
	return out
}

// vendorIndexCache keeps one parsed package index per list file, by its
// size and modification time: the install check reads them on every install.
var vendorIndexCache = struct {
	sync.Mutex
	entries map[string]vendorIndexEntry
}{entries: map[string]vendorIndexEntry{}}

type vendorIndexEntry struct {
	size     int64
	modTime  time.Time
	versions map[string]map[string]bool
}

// readVendorPackageIndex reads every (package, version) the repository's
// downloaded lists offer. An empty answer with no error means no list file.
func readVendorPackageIndex(loc aptSourceLocation) (map[string]map[string]bool, error) {
	files := loc.packagesFiles()
	if len(files) == 0 {
		return nil, fmt.Errorf("no package list from %s has been downloaded (apt-get update)", loc.Host)
	}
	out := map[string]map[string]bool{}
	for _, file := range files {
		info, err := os.Stat(file)
		if err != nil {
			return nil, err
		}
		vendorIndexCache.Lock()
		cached, ok := vendorIndexCache.entries[file]
		vendorIndexCache.Unlock()
		var versions map[string]map[string]bool
		if ok && cached.size == info.Size() && cached.modTime.Equal(info.ModTime()) {
			versions = cached.versions
		} else {
			versions, err = parseAptPackagesFile(file)
			if err != nil {
				return nil, err
			}
			vendorIndexCache.Lock()
			vendorIndexCache.entries[file] = vendorIndexEntry{size: info.Size(), modTime: info.ModTime(), versions: versions}
			vendorIndexCache.Unlock()
		}
		for name, set := range versions {
			if out[name] == nil {
				out[name] = map[string]bool{}
			}
			for version := range set {
				out[name][version] = true
			}
		}
	}
	return out, nil
}

func parseAptPackagesFile(path string) (map[string]map[string]bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return parseAptPackagesStanzas(io.LimitReader(file, repoListsReadCap))
}

// parseAptPackagesStanzas reads "Package:" and "Version:" of every stanza.
func parseAptPackagesStanzas(r io.Reader) (map[string]map[string]bool, error) {
	out := map[string]map[string]bool{}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64<<10), 4<<20)
	name := ""
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case line == "":
			name = ""
		case strings.HasPrefix(line, "Package: "):
			name = strings.TrimSpace(strings.TrimPrefix(line, "Package: "))
		case strings.HasPrefix(line, "Version: ") && name != "":
			version := strings.TrimSpace(strings.TrimPrefix(line, "Version: "))
			if out[name] == nil {
				out[name] = map[string]bool{}
			}
			out[name][version] = true
		}
	}
	return out, scanner.Err()
}

// readDpkgStatusVersions reads dpkg's status: every installed package
// and its version(s) (one per architecture).
func readDpkgStatusVersions(path string) (map[string][]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return parseDpkgStatus(io.LimitReader(file, repoListsReadCap))
}

func parseDpkgStatus(r io.Reader) (map[string][]string, error) {
	out := map[string][]string{}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64<<10), 4<<20)
	var name, version, status string
	flush := func() {
		if name != "" && version != "" && strings.HasSuffix(status, "install ok installed") {
			out[name] = append(out[name], version)
		}
		name, version, status = "", "", ""
	}
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case line == "":
			flush()
		case strings.HasPrefix(line, "Package: "):
			name = strings.TrimSpace(strings.TrimPrefix(line, "Package: "))
		case strings.HasPrefix(line, "Version: "):
			version = strings.TrimSpace(strings.TrimPrefix(line, "Version: "))
		case strings.HasPrefix(line, "Status: "):
			status = strings.TrimSpace(strings.TrimPrefix(line, "Status: "))
		}
	}
	flush()
	return out, scanner.Err()
}

// repoKeptPackages are the installed packages whose installed version is one
// the repository offers: they came from it.
func repoKeptPackages(installed map[string][]string, vendor map[string]map[string]bool) []string {
	var kept []string
	for name, versions := range installed {
		for _, version := range versions {
			if vendor[name][version] {
				kept = append(kept, name)
				break
			}
		}
	}
	sort.Strings(kept)
	return kept
}

// aptReleaseInfo is the head of a repository's Release file.
type aptReleaseInfo struct {
	Origin, Label, Suite, Codename, Date, ValidUntil string
}

func readRepoReleaseInfo(loc aptSourceLocation) (aptReleaseInfo, bool) {
	prefix := filepath.Join(aptListsDir, loc.listsPrefix())
	for _, name := range []string{prefix + "_InRelease", prefix + "_Release"} {
		file, err := os.Open(name)
		if err != nil {
			continue
		}
		info := parseAptReleaseHead(io.LimitReader(file, 1<<20))
		file.Close()
		return info, true
	}
	return aptReleaseInfo{}, false
}

func parseAptReleaseHead(r io.Reader) aptReleaseInfo {
	var info aptReleaseInfo
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64<<10), 1<<20)
	started := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "-----BEGIN PGP") || strings.HasPrefix(line, "Hash:") {
			continue
		}
		if strings.TrimSpace(line) == "" {
			if started {
				break
			}
			continue
		}
		if strings.HasPrefix(line, " ") {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		started = true
		value = strings.TrimSpace(value)
		switch key {
		case "Origin":
			info.Origin = value
		case "Label":
			info.Label = value
		case "Suite":
			info.Suite = value
		case "Codename":
			info.Codename = value
		case "Date":
			info.Date = value
		case "Valid-Until":
			info.ValidUntil = value
		case "MD5Sum", "SHA1", "SHA256", "SHA512":
			return info
		}
	}
	return info
}

// renderRepoPin is the exact text CelikPanel writes for one repository.
func renderRepoPin(repo *core.ManagedRepo, host string, kept []string) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "Explanation: Written by CelikPanel (D-038). The %s repository was enabled for one component.\n", repo.Name)
	b.WriteString("Explanation: It provides a package only where the distribution has none and never replaces a distribution package.\n")
	b.WriteString("Explanation: CelikPanel does not rewrite this file once it is changed; the panel shows the change.\n")
	b.WriteString("Package: *\n")
	fmt.Fprintf(&b, "Pin: origin \"%s\"\n", host)
	fmt.Fprintf(&b, "Pin-Priority: %d\n", repoPinLowPriority)
	if len(kept) > 0 {
		b.WriteString("\n")
		fmt.Fprintf(&b, "Explanation: These packages are installed from %s; they keep following it.\n", repo.Name)
		fmt.Fprintf(&b, "Package: %s\n", strings.Join(kept, " "))
		fmt.Fprintf(&b, "Pin: origin \"%s\"\n", host)
		fmt.Fprintf(&b, "Pin-Priority: %d\n", repoPinKeepPriority)
	}
	return []byte(b.String())
}

// aptPreferencesFileName is apt's rule for which preferences.d entries it
// reads: letters, digits, "-", "_", "." with no extension or ".pref".
var aptPreferencesFileName = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

func aptReadsPreferencesFile(name string) bool {
	if !aptPreferencesFileName.MatchString(name) {
		return false
	}
	ext := filepath.Ext(name)
	return ext == "" || ext == ".pref"
}

// repoIdentity is what an apt preferences rule can name a repository by.
type repoIdentity struct {
	Host, Origin, Label, Codename, Suite string
	Components                           []string
}

func repoIdentityOf(loc aptSourceLocation) repoIdentity {
	release, _ := readRepoReleaseInfo(loc)
	return repoIdentity{Host: loc.Host, Origin: release.Origin, Label: release.Label, Codename: release.Codename, Suite: release.Suite, Components: loc.Components}
}

// ownerAptPreferencesFor names the owner's own preferences files that set a
// rule matching this repository. CelikPanel's own files are not the owner's.
func ownerAptPreferencesFor(id repoIdentity) []string {
	var files []string
	candidates := []string{aptPreferencesFile}
	if entries, err := os.ReadDir(aptPreferencesDir); err == nil {
		for _, entry := range entries {
			name := entry.Name()
			if strings.HasPrefix(name, "celikpanel-") || !aptReadsPreferencesFile(name) {
				continue
			}
			candidates = append(candidates, filepath.Join(aptPreferencesDir, name))
		}
	}
	for _, path := range candidates {
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if len(data) > repoPrefReadCap {
			data = data[:repoPrefReadCap]
		}
		if preferencesNameRepository(string(data), id) {
			files = append(files, path)
		}
	}
	return files
}

// preferencesNameRepository: some "Pin:" line matches the repository, by
// "origin <host>" or by "release" fields (o=, l=, n=, a=, c=, and their long
// forms); every field the line names must match.
func preferencesNameRepository(text string, id repoIdentity) bool {
	lower := func(value string) string { return strings.ToLower(strings.Trim(strings.TrimSpace(value), `"`)) }
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(strings.ToLower(trimmed), "pin:") {
			continue
		}
		value := strings.TrimSpace(trimmed[len("pin:"):])
		lowerValue := strings.ToLower(value)
		if strings.HasPrefix(lowerValue, "origin") {
			if host := lower(value[len("origin"):]); host != "" && host == lower(id.Host) {
				return true
			}
			continue
		}
		if !strings.HasPrefix(lowerValue, "release") {
			continue
		}
		fields := strings.TrimSpace(value[len("release"):])
		if fields == "" || !strings.Contains(fields, "=") {
			continue
		}
		matched, named := true, 0
		for _, part := range strings.Split(fields, ",") {
			key, val, ok := strings.Cut(strings.TrimSpace(part), "=")
			if !ok {
				continue
			}
			val = lower(val)
			var have []string
			switch lower(key) {
			case "o", "origin":
				have = []string{id.Origin}
			case "l", "label":
				have = []string{id.Label}
			case "n", "codename":
				have = []string{id.Codename}
			case "a", "archive", "suite":
				have = []string{id.Suite}
			case "c", "component":
				have = id.Components
			default:
				continue
			}
			named++
			found := false
			for _, h := range have {
				if h != "" && lower(h) == val {
					found = true
				}
			}
			if !found {
				matched = false
			}
		}
		if named > 0 && matched {
			return true
		}
	}
	return false
}

// ---- the record ----

type repoRecord struct {
	Version int                        `json:"version"`
	Repos   map[string]repoRecordEntry `json:"repos"`
}

type repoRecordEntry struct {
	PinSHA256   string   `json:"pin_sha256,omitempty"`
	ConfinedAt  string   `json:"confined_at,omitempty"`
	ConfinedBy  string   `json:"confined_by,omitempty"`
	Kept        []string `json:"kept,omitempty"`
	KeptTotal   int      `json:"kept_total,omitempty"`
	EnabledAt   string   `json:"enabled_at,omitempty"`
	AptErrors   []string `json:"apt_errors,omitempty"`
	AptErrorsAt string   `json:"apt_errors_at,omitempty"`
}

func readRepoRecord() repoRecord {
	record := repoRecord{Version: 1, Repos: map[string]repoRecordEntry{}}
	data, err := os.ReadFile(repoRecordPath)
	if err != nil || len(data) > repoRecordReadCap {
		return record
	}
	var read repoRecord
	if json.Unmarshal(data, &read) != nil || read.Version != 1 {
		return record
	}
	for id, entry := range read.Repos {
		if validRepoID.MatchString(id) {
			record.Repos[id] = entry
		}
	}
	return record
}

func updateRepoRecord(id string, change func(*repoRecordEntry, bool) bool) error {
	repoConfinementRecordLock.Lock()
	defer repoConfinementRecordLock.Unlock()
	record := readRepoRecord()
	entry, ok := record.Repos[id]
	if !change(&entry, ok) {
		delete(record.Repos, id)
	} else {
		record.Repos[id] = entry
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(repoRecordPath), 0o700); err != nil {
		return err
	}
	return writePrivateFileAtomic(repoRecordPath, data, ".repo-record-*")
}

// ---- the state and the write ----

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// repoConfinementOf reads, without changing anything, whether the repository
// is confined.
func repoConfinementOf(repo *core.ManagedRepo, loc aptSourceLocation) transport.RepoConfinement {
	entry := readRepoRecord().Repos[repo.ID]
	path := repoPinPath(repo.ID)
	state := transport.RepoConfinement{PinPath: path}
	data, err := readSecureRepoFile(path, repoManagedFileMode)
	switch {
	case err == nil:
		state.WrittenAt, state.WrittenBy = entry.ConfinedAt, entry.ConfinedBy
		state.Kept, state.KeptTotal = entry.Kept, entry.KeptTotal
		if entry.PinSHA256 != "" && entry.PinSHA256 == sha256Hex(data) {
			state.State = repoConfinementConfined
		} else {
			state.State = repoConfinementEdited
		}
		return state
	case !errors.Is(err, os.ErrNotExist):
		// Present but not a plain root-owned 0644 file: someone else's.
		state.State = repoConfinementEdited
		state.Detail = err.Error()
		return state
	}
	if owners := ownerAptPreferencesFor(repoIdentityOf(loc)); len(owners) > 0 {
		state.State = repoConfinementOwnerRule
		state.OwnerFiles = owners
		return state
	}
	if entry.PinSHA256 != "" {
		state.State = repoConfinementRemovedByOwner
		state.WrittenAt, state.WrittenBy = entry.ConfinedAt, entry.ConfinedBy
		return state
	}
	state.State = repoConfinementNotWritten
	return state
}

// confineRepoLocked writes the preferences file the owner confirmed (its
// SHA-256) when the repository is not confined, or CelikPanel's file was
// removed by the owner and the owner asks again, and nothing of the owner's
// sets a rule for it. The caller holds the package operation lock. It never
// rewrites an existing file.
func confineRepoLocked(repo *core.ManagedRepo, loc aptSourceLocation, expectedSHA256 string) transport.RepoConfinement {
	state := repoConfinementOf(repo, loc)
	if state.State != repoConfinementNotWritten && state.State != repoConfinementRemovedByOwner {
		return state
	}
	content, kept, err := repoPinPlan(repo, loc)
	if err != nil {
		state.Reason, state.Detail = repoConfinementReasonListsUnreadable, err.Error()
		return state
	}
	if sha256Hex(content) != expectedSHA256 {
		state.Reason, state.Detail = repoConfinementReasonPlanChanged, "what would be written is not what was confirmed; nothing was written"
		return state
	}
	if err := publishRepoPin(repoPinPath(repo.ID), content); err != nil {
		state.Reason, state.Detail = repoConfinementReasonWriteFailed, err.Error()
		return state
	}
	now := repoConfinementNow().UTC().Format(time.RFC3339)
	shown := kept
	if len(shown) > repoKeptCap {
		shown = shown[:repoKeptCap]
	}
	if err := updateRepoRecord(repo.ID, func(entry *repoRecordEntry, _ bool) bool {
		entry.PinSHA256 = sha256Hex(content)
		entry.ConfinedAt, entry.ConfinedBy = now, repoConfinedByOwner
		entry.Kept, entry.KeptTotal = shown, len(kept)
		return true
	}); err != nil {
		// Without the record the file would read as edited and never be
		// rewritten again; take it back and say so.
		_ = removeRepoFileVerified(repoPinPath(repo.ID))
		state.Reason, state.Detail = repoConfinementReasonWriteFailed, "record the confinement: "+err.Error()
		return state
	}
	return transport.RepoConfinement{
		State: repoConfinementConfined, PinPath: repoPinPath(repo.ID),
		Kept: shown, KeptTotal: len(kept), WrittenAt: now, WrittenBy: repoConfinedByOwner,
	}
}

// publishRepoPin writes the preferences file through a temporary file and
// one rename, root-owned 0644 as apt reads it.
func publishRepoPin(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	staged, err := stageRepoFile(path, "confine", content)
	if err != nil {
		return fmt.Errorf("stage the preferences file: %w", err)
	}
	defer func() { _ = os.Remove(staged) }()
	if err := renameRepoFile(staged, path); err != nil {
		return fmt.Errorf("publish the preferences file: %w", err)
	}
	return syncRepoDirectory(filepath.Dir(path))
}

// removeOwnRepoPin removes CelikPanel's file when the repository is disabled,
// only when it is exactly what CelikPanel wrote; an edited file stays.
func removeOwnRepoPin(repo *core.ManagedRepo, loc aptSourceLocation) (kept bool) {
	state := repoConfinementOf(repo, loc)
	if state.State == repoConfinementConfined {
		if err := removeRepoFileVerified(repoPinPath(repo.ID)); err != nil {
			return true
		}
		_ = updateRepoRecord(repo.ID, func(*repoRecordEntry, bool) bool { return false })
		return false
	}
	_ = updateRepoRecord(repo.ID, func(entry *repoRecordEntry, ok bool) bool {
		entry.EnabledAt, entry.AptErrors, entry.AptErrorsAt = "", nil, ""
		return ok && entry.PinSHA256 != ""
	})
	return state.State == repoConfinementEdited
}

// enabledRepoLocations are the catalogue repositories whose CelikPanel source
// file is present, with the line it holds.
func enabledRepoLocations() map[string]aptSourceLocation {
	out := map[string]aptSourceLocation{}
	for i := range core.ManagedServices {
		repo := core.ManagedServices[i].Repo
		if repo == nil {
			continue
		}
		data, err := readSecureRepoFile(repoSourcePath(repo.ID), repoManagedFileMode)
		if err != nil {
			continue
		}
		loc, err := parseAptSourceLine(firstLine(string(data)))
		if err == nil {
			out[repo.ID] = loc
		}
	}
	return out
}

// hostVendorVersionLookup answers "which enabled repository offers this
// version of this package?" ("" when none does).
// The indexes are read on the first question, after any list refresh the
// install made.
func hostVendorVersionLookup() func(name, version string) string {
	indexes := map[string]map[string]map[string]bool{}
	var once sync.Once
	return func(name, version string) string {
		once.Do(func() {
			for id, loc := range enabledRepoLocations() {
				if index, err := readVendorPackageIndex(loc); err == nil {
					indexes[id] = index
				}
			}
		})
		ids := make([]string, 0, len(indexes))
		for id := range indexes {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			if indexes[id][name][version] {
				return id
			}
		}
		return ""
	}
}

// recordRepoAptErrors keeps apt's error and warning lines that name an
// enabled repository's host, from an apt-get update the Agent ran, so the
// repository card shows them instead of their being swallowed (D-038 §12).
// A run that names none clears the repository's previous lines.
func recordRepoAptErrors(output string) {
	locations := enabledRepoLocations()
	if len(locations) == 0 {
		return
	}
	now := repoConfinementNow().UTC().Format(time.RFC3339)
	for id, loc := range locations {
		var lines []string
		for _, line := range strings.Split(output, "\n") {
			line = strings.TrimSpace(line)
			if (strings.HasPrefix(line, "E:") || strings.HasPrefix(line, "W:") || strings.HasPrefix(line, "Err:")) &&
				strings.Contains(strings.ToLower(line), loc.Host) {
				if len(line) > 300 {
					line = line[:300]
				}
				lines = append(lines, line)
				if len(lines) == repoAptErrorCap {
					break
				}
			}
		}
		_ = updateRepoRecord(id, func(entry *repoRecordEntry, _ bool) bool {
			entry.AptErrors = lines
			entry.AptErrorsAt = now
			if len(lines) == 0 {
				entry.AptErrorsAt = ""
			}
			return true
		})
	}
}
