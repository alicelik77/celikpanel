package main

// Confinement is the owner's action, shown before it is done (alpha.107
// review, D-038 §3 and §12, D-022). The repository card asks the Agent for
// the plan: the exact preferences text and its SHA-256, which installed
// packages keep following the vendor, and what `apt-get upgrade` would do
// now and with the file in place. Only that text, confirmed by its SHA-256,
// is written; nothing confines a repository by itself, and a file the owner
// deleted is never written again unless the owner asks.
//
// Sınırlama sahibin eylemidir: önce tam metin, SHA-256 ve apt upgrade'in
// önce/sonra farkı gösterilir; yalnız onaylanan metin yazılır.

import (
	"bufio"
	"context"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/transport"
)

// readDistributionVersions reads, from every downloaded package list that is
// not one of CelikPanel's vendor repositories, the versions of the named
// packages.
func readDistributionVersions(names map[string]bool) (map[string]map[string]bool, error) {
	files, err := filepath.Glob(filepath.Join(aptListsDir, "*_Packages"))
	if err != nil {
		return nil, err
	}
	var vendorPrefixes []string
	for _, loc := range enabledRepoLocations() {
		vendorPrefixes = append(vendorPrefixes, loc.listsPrefix())
	}
	out := map[string]map[string]bool{}
	for _, file := range files {
		base := filepath.Base(file)
		vendor := false
		for _, prefix := range vendorPrefixes {
			if strings.HasPrefix(base, prefix) {
				vendor = true
			}
		}
		if vendor {
			continue
		}
		f, err := os.Open(file)
		if err != nil {
			return nil, err
		}
		err = scanPackagesFor(io.LimitReader(f, repoListsReadCap), names, out)
		f.Close()
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func scanPackagesFor(r io.Reader, names map[string]bool, out map[string]map[string]bool) error {
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
			if !names[name] {
				name = ""
			}
		case strings.HasPrefix(line, "Version: ") && name != "":
			if out[name] == nil {
				out[name] = map[string]bool{}
			}
			out[name][strings.TrimSpace(strings.TrimPrefix(line, "Version: "))] = true
		}
	}
	return scanner.Err()
}

// repoKeepList is every installed package whose installed version came from
// the vendor: the vendor offers the name, and the installed version is the
// vendor's or not one the distribution's lists offer (the vendor's index has
// moved on). The list is sticky: a name kept before stays while it is
// installed, so nothing installed from the vendor is left at the low
// priority without its updates.
func repoKeepList(installed map[string][]string, vendor, distro map[string]map[string]bool, previous []string) []string {
	keep := map[string]bool{}
	for name, versions := range installed {
		if len(vendor[name]) == 0 {
			continue
		}
		for _, version := range versions {
			if vendor[name][version] || !distro[name][version] {
				keep[name] = true
			}
		}
	}
	for _, name := range previous {
		if len(installed[name]) > 0 {
			keep[name] = true
		}
	}
	out := make([]string, 0, len(keep))
	for name := range keep {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// keptInPin reads the names a CelikPanel preferences file keeps at the
// higher priority.
func keptInPin(text string) []string {
	var names []string
	block := []string{}
	flush := func() {
		pkg, keep := "", false
		for _, line := range block {
			if value, ok := strings.CutPrefix(line, "Package: "); ok {
				pkg = value
			}
			if strings.HasPrefix(line, "Pin-Priority: ") && strings.TrimSpace(strings.TrimPrefix(line, "Pin-Priority: ")) != "100" {
				keep = true
			}
		}
		if keep && pkg != "*" {
			names = append(names, strings.Fields(pkg)...)
		}
		block = block[:0]
	}
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) == "" {
			flush()
			continue
		}
		block = append(block, strings.TrimSpace(line))
	}
	flush()
	return names
}

// repoPinPlan is the exact text CelikPanel would write for the repository
// now, and the packages it keeps following the vendor.
func repoPinPlan(repo *core.ManagedRepo, loc aptSourceLocation) ([]byte, []string, error) {
	vendor, err := readVendorPackageIndex(loc)
	if err != nil {
		return nil, nil, err
	}
	installed, err := readDpkgStatusVersions(dpkgStatusPath)
	if err != nil {
		return nil, nil, err
	}
	wanted := map[string]bool{}
	for name := range installed {
		if len(vendor[name]) > 0 {
			wanted[name] = true
		}
	}
	distro, err := readDistributionVersions(wanted)
	if err != nil {
		return nil, nil, err
	}
	previous := append([]string{}, readRepoRecord().Repos[repo.ID].Kept...)
	if data, err := readSecureRepoFile(repoPinPath(repo.ID), repoManagedFileMode); err == nil {
		previous = append(previous, keptInPin(string(data))...)
	}
	kept := repoKeepList(installed, vendor, distro, previous)
	return renderRepoPin(repo, loc.Host, kept), kept, nil
}

// previewPreferencesDir is a copy of the preferences files apt reads now,
// with this repository's file replaced by text (nil: without it).
func previewPreferencesDir(repo *core.ManagedRepo, text []byte) (string, error) {
	dir, err := os.MkdirTemp(filepath.Dir(repoRecordPath), ".repo-preview-*")
	if err != nil {
		return "", err
	}
	entries, _ := os.ReadDir(aptPreferencesDir)
	for _, entry := range entries {
		name := entry.Name()
		if !aptReadsPreferencesFile(name) || name == filepath.Base(repoPinPath(repo.ID)) {
			continue
		}
		path := filepath.Join(aptPreferencesDir, name)
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			os.RemoveAll(dir)
			return "", err
		}
	}
	if text != nil {
		if err := os.WriteFile(filepath.Join(dir, filepath.Base(repoPinPath(repo.ID))), text, 0o644); err != nil {
			os.RemoveAll(dir)
			return "", err
		}
	}
	return dir, nil
}

// repoUpgradeSimulation is `apt-get -s upgrade` with the given preferences
// directory.
var repoUpgradeSimulation = func(ctx context.Context, prefsDir string) (aptSimulation, error) {
	profile, err := verifiedHostProfile("apt")
	if err != nil {
		return aptSimulation{}, err
	}
	aptGet, err := executableForProfile(profile, "apt", "apt-get")
	if err != nil {
		return aptSimulation{}, err
	}
	return runAptSimulation(ctx, runPackageSimulationCommand, aptGet, []string{"-o", "Dir::Etc::PreferencesParts=" + prefsDir, "upgrade"})
}

// confineChanges compares `apt-get upgrade` now and with the file.
func confineChanges(before, after aptSimulation, vendorOf func(name, version string) string) (changes []transport.RepoConfineChange, stillVendor []string) {
	names := map[string]bool{}
	for name := range before.Upgrades {
		names[name] = true
	}
	for name := range after.Upgrades {
		names[name] = true
	}
	sorted := make([]string, 0, len(names))
	for name := range names {
		sorted = append(sorted, name)
	}
	sort.Strings(sorted)
	for _, name := range sorted {
		b, inBefore := before.Upgrades[name]
		a, inAfter := after.Upgrades[name]
		pkg := packageBaseName(name)
		if inAfter && vendorOf(pkg, a.To) != "" {
			stillVendor = append(stillVendor, pkg)
		}
		if inBefore && inAfter && b.To == a.To {
			continue
		}
		change := transport.RepoConfineChange{Package: pkg}
		if inBefore {
			change.From, change.Before, change.BeforeVendor = b.From, b.To, vendorOf(pkg, b.To) != ""
		}
		if inAfter {
			change.From, change.After, change.AfterVendor = a.From, a.To, vendorOf(pkg, a.To) != ""
		}
		changes = append(changes, change)
	}
	return changes, stillVendor
}

// RepoConfinePreviewV1 is the plan the repository card shows before the
// owner confines a repository. Read only: the simulations run with a
// temporary copy of the preferences files.
func (a *Agent) RepoConfinePreviewV1(req *EnableRepoRequest, resp *transport.RepoConfinePreview) error {
	*resp = transport.RepoConfinePreview{}
	if req == nil || !validRepoID.MatchString(req.RepoID) {
		resp.Error = "invalid repo id"
		return nil
	}
	repo := repoFromCatalogue(req.RepoID)
	if repo == nil {
		resp.Error = "unknown repository"
		return nil
	}
	loc, ok := enabledRepoLocations()[repo.ID]
	if !ok {
		resp.Error = "the repository is not enabled"
		return nil
	}
	state := repoConfinementOf(repo, loc)
	resp.State, resp.PinPath, resp.OwnerFiles = state.State, repoPinPath(repo.ID), state.OwnerFiles
	text, kept, err := repoPinPlan(repo, loc)
	if err != nil {
		resp.Error = "what came from the repository could not be read: " + err.Error()
		return nil
	}
	resp.Text, resp.SHA256, resp.Kept = string(text), sha256Hex(text), kept
	ctx, cancel := context.WithTimeout(context.Background(), packageRemovalSimulationBound*2)
	defer cancel()
	current, _ := readSecureRepoFile(repoPinPath(repo.ID), repoManagedFileMode)
	beforeDir, err := previewPreferencesDir(repo, current)
	if err != nil {
		resp.Error = err.Error()
		return nil
	}
	defer os.RemoveAll(beforeDir)
	afterDir, err := previewPreferencesDir(repo, text)
	if err != nil {
		resp.Error = err.Error()
		return nil
	}
	defer os.RemoveAll(afterDir)
	before, err := repoUpgradeSimulation(ctx, beforeDir)
	if err != nil {
		resp.Error = err.Error()
		return nil
	}
	after, err := repoUpgradeSimulation(ctx, afterDir)
	if err != nil {
		resp.Error = err.Error()
		return nil
	}
	resp.Changes, resp.StillFromVendor = confineChanges(before, after, hostVendorVersionLookup())
	resp.UpgradesNow, resp.UpgradesConfined = len(before.Upgrades), len(after.Upgrades)
	return nil
}
