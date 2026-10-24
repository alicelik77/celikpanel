package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.107 (D-038 §12): a vendor repository is confined to its own
// component, and an install that would replace a distribution package with
// a vendor build, or upgrade packages it did not ask for, is refused by name.

func TestAptSourceLinesNameTheirListFilesAsAptDoes(t *testing.T) {
	cases := []struct {
		line, host, prefix string
	}{
		{"deb [signed-by=/usr/share/keyrings/celikpanel-sury.gpg] https://packages.sury.org/php/ trixie main", "packages.sury.org", "packages.sury.org_php_dists_trixie"},
		{"deb https://apt.postgresql.org/pub/repos/apt trixie-pgdg main", "apt.postgresql.org", "apt.postgresql.org_pub_repos_apt_dists_trixie-pgdg"},
		{"deb https://repository.netdata.cloud/repos/stable/debian/ trixie/", "repository.netdata.cloud", "repository.netdata.cloud_repos_stable_debian_trixie"},
	}
	for _, c := range cases {
		loc, err := parseAptSourceLine(c.line)
		if err != nil || loc.Host != c.host || loc.listsPrefix() != c.prefix {
			t.Fatalf("%q: %+v %q %v", c.line, loc, loc.listsPrefix(), err)
		}
	}
	if _, err := parseAptSourceLine("deb http://example.org/ x main"); err == nil {
		t.Fatal("a plain-http source was accepted")
	}
}

func TestTheSimulationNamesUpgradesWithTheirVersions(t *testing.T) {
	sim := parseAptSimulation([]byte(strings.Join([]string{
		"Inst libpq5 [17.6-0+deb13u1] (18.0-1.pgdg13+1 PostgreSQL for Debian/Ubuntu repository:trixie-pgdg [amd64])",
		"Inst php8.5-fpm (8.5.1-1+0~20251121 deb.sury.org:trixie [amd64])",
		"Conf libpq5 (18.0-1.pgdg13+1 PostgreSQL for Debian/Ubuntu repository:trixie-pgdg [amd64])",
	}, "\n")))
	if len(sim.Installed) != 2 || sim.Upgrades["libpq5"] != (aptUpgrade{From: "17.6-0+deb13u1", To: "18.0-1.pgdg13+1"}) {
		t.Fatalf("sim = %+v", sim)
	}
	if _, ok := sim.Upgrades["php8.5-fpm"]; ok {
		t.Fatal("a new install was read as an upgrade")
	}
}

func vendorLookup(repo string, versions map[string]string) func(string, string) string {
	return func(name, version string) string {
		if versions[name] == version {
			return repo
		}
		return ""
	}
}

func TestAVendorReplacementIsRefusedWhateverWasConfirmed(t *testing.T) {
	withPHPVersionsPresent(t, "8.4")
	c := newInstallClassification("apt", []string{"php8.5-fpm"}, []string{"php-fpm"}, nil, nil)
	c.vendorOf = vendorLookup("sury", map[string]string{"php8.4-common": "8.4.13-1+sury", "php8.5-fpm": "8.5.1-1"})
	sim := aptSimulation{
		Installed: []string{"php8.5-fpm", "php8.4-common", "libssl3"},
		Upgrades: map[string]aptUpgrade{
			"php8.4-common": {From: "8.4.11-1", To: "8.4.13-1+sury"},
			"libssl3":       {From: "3.5.1-1", To: "3.5.1-1+deb13u1"},
		},
	}
	notable := c.classifySimulation(sim)
	if got := notableNames(notable); strings.Join(got, ",") != "vendor_replaces_distribution:php8.4-common,upgrade:libssl3" {
		t.Fatalf("notable = %v", got)
	}
	if notable[0].Component != "sury" || notable[0].From != "8.4.11-1" || notable[0].To != "8.4.13-1+sury" {
		t.Fatalf("vendor item = %+v", notable[0])
	}
	err := notableInstallGuard(c, []string{"php8.5-fpm"}, []string{"php8.4-common", "libssl3"})(sim)
	var vendor *packageInstallVendorRefusal
	if !errors.As(err, &vendor) || !strings.Contains(err.Error(), "php8.4-common") || !strings.Contains(err.Error(), "nothing was installed") {
		t.Fatalf("err = %v", err)
	}
	// Without the replacement, a confirmed upgrade goes ahead and an
	// unconfirmed one is refused by name.
	delete(sim.Upgrades, "php8.4-common")
	sim.Installed = []string{"php8.5-fpm", "libssl3"}
	err = notableInstallGuard(c, []string{"php8.5-fpm"}, nil)(sim)
	var refusal *packageInstallNotableRefusal
	if !errors.As(err, &refusal) || refusal.Notable[0].Reason != notableReasonUpgrade || refusal.Notable[0].Package != "libssl3" {
		t.Fatalf("err = %v", err)
	}
	if err := notableInstallGuard(c, []string{"php8.5-fpm"}, []string{"libssl3"})(sim); err != nil {
		t.Fatal(err)
	}
}

func TestAnUpgradeOfARequestedPackageIsNotNotable(t *testing.T) {
	c := newInstallClassification("apt", []string{"php8.4-fpm"}, []string{"php-fpm"}, nil, nil)
	sim := aptSimulation{Installed: []string{"php8.4-fpm"}, Upgrades: map[string]aptUpgrade{"php8.4-fpm": {From: "1", To: "2"}}}
	if notable := c.classifySimulation(sim); len(notable) != 0 {
		t.Fatalf("notable = %+v", notable)
	}
}

// confinementHost points every path the confinement reads at a temporary
// tree: preferences, the record, dpkg's status and apt's lists.
func confinementHost(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, dir := range []string{"prefs", "lists", "private"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	restore := []func(){}
	set := func(target *string, value string) {
		old := *target
		*target = value
		restore = append(restore, func() { *target = old })
	}
	set(&aptPreferencesDir, filepath.Join(root, "prefs"))
	set(&aptPreferencesFile, filepath.Join(root, "preferences"))
	set(&aptListsDir, filepath.Join(root, "lists"))
	set(&dpkgStatusPath, filepath.Join(root, "status"))
	set(&repoRecordPath, filepath.Join(root, "private", "repo-record.json"))
	t.Cleanup(func() {
		for i := len(restore) - 1; i >= 0; i-- {
			restore[i]()
		}
	})
	return root
}

func writeTestFile(t *testing.T, path, text string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
}

func suryForTest(t *testing.T) (*core.ManagedRepo, aptSourceLocation) {
	t.Helper()
	repo := repoFromCatalogue("sury")
	if repo == nil {
		t.Fatal("no sury in the catalogue")
	}
	loc, err := parseAptSourceLine("deb https://packages.sury.org/php/ trixie main")
	if err != nil {
		t.Fatal(err)
	}
	return repo, loc
}

func TestARepositoryIsConfinedOnlyAsConfirmedAndWhatCameFromItKeepsFollowingIt(t *testing.T) {
	root := confinementHost(t)
	repo, loc := suryForTest(t)
	lists := filepath.Join(root, "lists", loc.listsPrefix())
	writeTestFile(t, lists+"_InRelease", "-----BEGIN PGP SIGNED MESSAGE-----\nHash: SHA512\n\nOrigin: deb.sury.org\nLabel: PHP\nSuite: trixie\nDate: Fri, 23 Oct 2026 10:00:00 UTC\nSHA512:\n")
	writeTestFile(t, lists+"_main_binary-amd64_Packages", "Package: php8.4-fpm\nVersion: 8.4.13-1+sury\n\nPackage: php8.5-fpm\nVersion: 8.5.1-1\n\nPackage: libpcre2-8-0\nVersion: 10.46-1+sury\n\nPackage: php-common\nVersion: 2:102+sury\n")
	writeTestFile(t, filepath.Join(root, "lists", "deb.debian.org_debian_dists_trixie_main_binary-amd64_Packages"), "Package: php8.4-cli\nVersion: 8.4.11-1\n\nPackage: php-common\nVersion: 2:96\n\nPackage: libpcre2-8-0\nVersion: 10.46-1\n")
	writeTestFile(t, filepath.Join(root, "status"),
		"Package: php8.4-fpm\nStatus: install ok installed\nVersion: 8.4.13-1+sury\n\n"+
			"Package: php8.4-cli\nStatus: install ok installed\nVersion: 8.4.11-1\n\n"+
			// Sury has moved on to 2:102; the installed 2:101 is not Debian's.
			"Package: php-common\nStatus: install ok installed\nVersion: 2:101+sury\n\n"+
			"Package: libpcre2-8-0\nStatus: install ok installed\nVersion: 10.46-1\n\n"+
			"Package: php-fpm\nStatus: install ok installed\nVersion: 2:96\nDepends: php8.4-fpm\n\n")
	sourcesDir := filepath.Join(root, "sources")
	os.MkdirAll(sourcesDir, 0o755)
	restoreSources := aptSourcesListDir
	aptSourcesListDir = sourcesDir
	t.Cleanup(func() { aptSourcesListDir = restoreSources })
	writeTestFile(t, repoSourcePath("sury"), "deb https://packages.sury.org/php/ trixie main\n")

	if state := repoConfinementOf(repo, loc); state.State != repoConfinementNotWritten {
		t.Fatalf("before = %+v", state)
	}
	text, kept, err := repoPinPlan(repo, loc)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(kept, ",") != "php-common,php8.4-fpm" {
		t.Fatalf("kept = %v", kept)
	}
	// Anything but the confirmed text is refused, and nothing is written.
	if refused := confineRepoLocked(repo, loc, strings.Repeat("0", 64)); refused.State != repoConfinementNotWritten || refused.Reason != repoConfinementReasonPlanChanged {
		t.Fatalf("unconfirmed = %+v", refused)
	}
	if _, err := os.Stat(repoPinPath("sury")); !os.IsNotExist(err) {
		t.Fatal("an unconfirmed plan was written")
	}
	state := confineRepoLocked(repo, loc, sha256Hex(text))
	if state.State != repoConfinementConfined || state.KeptTotal != 2 || state.WrittenBy != repoConfinedByOwner {
		t.Fatalf("confined = %+v", state)
	}
	pin, err := os.ReadFile(repoPinPath("sury"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Package: *\nPin: origin \"packages.sury.org\"\nPin-Priority: 100", "Package: php-common php8.4-fpm\nPin: origin \"packages.sury.org\"\nPin-Priority: 500"} {
		if !strings.Contains(string(pin), want) {
			t.Fatalf("pin lacks %q:\n%s", want, pin)
		}
	}
	// Sticky: a kept name stays while it is installed, even if nothing says
	// it came from the vendor any more.
	writeTestFile(t, lists+"_main_binary-amd64_Packages", "Package: php8.5-fpm\nVersion: 8.5.1-1\n")
	if _, again, _ := repoPinPlan(repo, loc); strings.Join(again, ",") != "php-common,php8.4-fpm" {
		t.Fatalf("sticky = %v", again)
	}
	// Someone edits it: kept as it is, said edited, never rewritten.
	writeTestFile(t, repoPinPath("sury"), string(pin)+"\n# mine\n")
	if edited := confineRepoLocked(repo, loc, sha256Hex(text)); edited.State != repoConfinementEdited {
		t.Fatalf("edited = %+v", edited)
	}
	// The owner deletes it: said, and never written again by itself.
	os.Remove(repoPinPath("sury"))
	if removed := repoConfinementOf(repo, loc); removed.State != repoConfinementRemovedByOwner {
		t.Fatalf("removed = %+v", removed)
	}
	refreshRepoConfinement(repo)
	if _, err := os.Stat(repoPinPath("sury")); !os.IsNotExist(err) {
		t.Fatal("a file the owner removed was written again")
	}
	// The moving default-version package is reported with what it brings.
	moving := repoMovingPackages(repo)
	if len(moving) != 1 || moving[0] != (transport.RepoMovingPackage{Package: "php-fpm", Exact: "php8.4-fpm"}) {
		t.Fatalf("moving = %+v", moving)
	}
}

func TestAnInstalledVendorPackageBehindTheIndexIsKept(t *testing.T) {
	// PGDG's postgresql-17 installed at a version its index no longer lists.
	installed := map[string][]string{"postgresql-17": {"17.5-1.pgdg13+1"}, "libpq5": {"17.6-0+deb13u1"}, "nginx": {"1.26.3-3"}}
	vendor := map[string]map[string]bool{"postgresql-17": {"17.6-1.pgdg13+1": true}, "libpq5": {"18.0-1.pgdg13+1": true}}
	distro := map[string]map[string]bool{"postgresql-17": {"17.6-0+deb13u1": true}, "libpq5": {"17.6-0+deb13u1": true}}
	if got := repoKeepList(installed, vendor, distro, nil); strings.Join(got, ",") != "postgresql-17" {
		t.Fatalf("kept = %v", got)
	}
	if got := repoKeepList(installed, vendor, distro, []string{"libpq5", "gone"}); strings.Join(got, ",") != "libpq5,postgresql-17" {
		t.Fatalf("sticky = %v", got)
	}
}

func TestConfiningSaysWhatAptUpgradeWouldDoDifferently(t *testing.T) {
	before := aptSimulation{Upgrades: map[string]aptUpgrade{
		"php-common": {From: "2:96", To: "2:102+sury"},
		"php8.4-fpm": {From: "8.4.13-1+sury", To: "8.4.14-1+sury"},
		"perl-base":  {From: "5.40.1-6", To: "5.40.1-6+deb13u1"},
	}}
	after := aptSimulation{Upgrades: map[string]aptUpgrade{
		"php8.4-fpm": {From: "8.4.13-1+sury", To: "8.4.14-1+sury"},
		"perl-base":  {From: "5.40.1-6", To: "5.40.1-6+deb13u1"},
	}}
	vendorOf := func(name, version string) string {
		if strings.Contains(version, "sury") {
			return "sury"
		}
		return ""
	}
	changes, still := confineChanges(before, after, vendorOf)
	if len(changes) != 1 || changes[0].Package != "php-common" || changes[0].Before != "2:102+sury" || !changes[0].BeforeVendor || changes[0].After != "" {
		t.Fatalf("changes = %+v", changes)
	}
	if strings.Join(still, ",") != "php8.4-fpm" {
		t.Fatalf("still from the vendor = %v", still)
	}
}

func TestTheOwnersOwnRuleIsRespected(t *testing.T) {
	root := confinementHost(t)
	repo, loc := suryForTest(t)
	writeTestFile(t, filepath.Join(root, "lists", loc.listsPrefix()+"_InRelease"), "Origin: deb.sury.org\nSuite: trixie\n")
	writeTestFile(t, filepath.Join(root, "prefs", "php-from-sury.pref"), "Package: php*\nPin: release o=deb.sury.org\nPin-Priority: 990\n")
	state := confineRepoLocked(repo, loc, strings.Repeat("0", 64))
	if state.State != repoConfinementOwnerRule || len(state.OwnerFiles) != 1 || !strings.HasSuffix(state.OwnerFiles[0], "php-from-sury.pref") {
		t.Fatalf("state = %+v", state)
	}
	if _, err := os.Stat(repoPinPath("sury")); !os.IsNotExist(err) {
		t.Fatal("a preferences file was written beside the owner's rule")
	}
	// A file apt does not read is not a rule.
	os.Remove(filepath.Join(root, "prefs", "php-from-sury.pref"))
	writeTestFile(t, filepath.Join(root, "prefs", "php-from-sury.pref.disabled"), "Pin: origin \"packages.sury.org\"\n")
	if state := repoConfinementOf(repo, loc); state.State != repoConfinementNotWritten {
		t.Fatalf("state = %+v", state)
	}
}

func TestWithoutItsListsARepositoryIsNotConfinedBlindly(t *testing.T) {
	confinementHost(t)
	repo, loc := suryForTest(t)
	state := confineRepoLocked(repo, loc, strings.Repeat("0", 64))
	if state.State != repoConfinementNotWritten || state.Reason != repoConfinementReasonListsUnreadable {
		t.Fatalf("state = %+v", state)
	}
	if _, err := os.Stat(repoPinPath("sury")); !os.IsNotExist(err) {
		t.Fatal("a preferences file was written without knowing what came from the repository")
	}
}

func TestAptErrorsThatNameARepositoryAreKept(t *testing.T) {
	root := confinementHost(t)
	old := aptSourcesListDir
	aptSourcesListDir = filepath.Join(root, "sources")
	t.Cleanup(func() { aptSourcesListDir = old })
	if err := os.MkdirAll(aptSourcesListDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, repoSourcePath("sury"), "deb [signed-by=/usr/share/keyrings/celikpanel-sury.gpg] https://packages.sury.org/php/ trixie main\n")
	recordRepoAptErrors("Hit:1 http://deb.debian.org/debian trixie InRelease\nE: The repository 'https://packages.sury.org/php trixie InRelease' is not signed.\nW: something else entirely\n")
	got := readRepoRecord().Repos["sury"]
	if len(got.AptErrors) != 1 || !strings.Contains(got.AptErrors[0], "not signed") || got.AptErrorsAt == "" {
		t.Fatalf("record = %+v", got)
	}
	// A clean run clears them.
	recordRepoAptErrors("Hit:1 https://packages.sury.org/php trixie InRelease\n")
	if got := readRepoRecord().Repos["sury"]; len(got.AptErrors) != 0 || got.AptErrorsAt != "" {
		t.Fatalf("record = %+v", got)
	}
}

func mustParseTime(t *testing.T, text string) time.Time {
	t.Helper()
	value, err := time.Parse(time.RFC3339, text)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestAKeyThatExpiresSoonIsSaid(t *testing.T) {
	now := mustParseTime(t, "2026-10-24T00:00:00Z")
	if repoKeyState(mustParseTime(t, "2026-12-01T00:00:00Z"), now) != "expiring" ||
		repoKeyState(mustParseTime(t, "2026-10-01T00:00:00Z"), now) != "expired" ||
		repoKeyState(mustParseTime(t, "2028-01-01T00:00:00Z"), now) != "valid" {
		t.Fatal("key states")
	}
}

func TestPreferencesRulesAreMatchedByAnyFieldThatNamesTheRepository(t *testing.T) {
	id := repoIdentity{Host: "packages.sury.org", Origin: "deb.sury.org", Label: "PHP-FPM", Codename: "trixie", Suite: "trixie", Components: []string{"main"}}
	for text, want := range map[string]bool{
		"Pin: origin packages.sury.org":         true,
		"Pin: origin \"packages.sury.org\"":     true,
		"Pin: release o=deb.sury.org,n=trixie":  true,
		"Pin: release l=PHP-FPM":                true,
		"Pin: release origin=deb.sury.org":      true,
		"Pin: release n=trixie, c=main":         true,
		"Pin: release a=trixie":                 true,
		"Pin: release o=Debian":                 false,
		"Pin: release o=deb.sury.org,c=contrib": false,
		"Pin: release v=13":                     false,
		"Explanation: origin packages.sury.org": false,
	} {
		if got := preferencesNameRepository(text, id); got != want {
			t.Fatalf("%q = %v", text, got)
		}
	}
}

func TestMovingPackagesNameWhatTheyBringToday(t *testing.T) {
	status := "Package: postgresql\nStatus: install ok installed\nDepends: postgresql-17\n\nPackage: postgresql-client\nStatus: deinstall ok config-files\nDepends: postgresql-client-17\n\n"
	got := movingPackagesFromStatus(strings.NewReader(status), regexp.MustCompile(`^postgresql(-client|-contrib)?$`), `^postgresql-[0-9]+$`)
	if len(got) != 1 || got[0].Exact != "postgresql-17" {
		t.Fatalf("got = %+v", got)
	}
}

func TestAPreviewNamesAVendorReplacement(t *testing.T) {
	host := &fakePreviewHost{
		installed: map[string]bool{},
		sim: aptSimulation{
			Installed: []string{"postgresql-18", "libpq5"},
			Upgrades:  map[string]aptUpgrade{"libpq5": {From: "17.6-0+deb13u1", To: "18.0-1.pgdg13+1"}},
		},
	}
	ops := host.ops()
	ops.vendorOf = vendorLookup("pgdg", map[string]string{"libpq5": "18.0-1.pgdg13+1"})
	var resp transport.ServiceInstallPreviewResponse
	serviceInstallPreviewWithOps(context.Background(), &transport.ServiceInstallPreviewRequest{Steps: []transport.ServiceInstallPreviewStep{{ID: "postgresql", Package: "postgresql-18"}}}, &resp, ops)
	if resp.State != "known" || len(resp.Notable) == 0 || resp.Notable[0].Reason != notableReasonVendorReplacesDist || resp.Notable[0].Component != "pgdg" {
		t.Fatalf("resp = %+v", resp)
	}
}
