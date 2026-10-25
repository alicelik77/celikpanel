package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.108 (D-038 amendment "two lanes"): a vendor repository is enabled
// only as one confirmed operation with its confinement and its automatic
// updates; an enabled one shows its missing steps and completes them.

// laneHost is confinementHost plus CelikPanel's sources directory, the
// automatic-updates drop-in and a fake apt-config.
func laneHost(t *testing.T, automatic string) string {
	t.Helper()
	root := confinementHost(t)
	sources := filepath.Join(root, "sources")
	os.MkdirAll(sources, 0o755)
	oldSources, oldDropIn, oldConfig := aptSourcesListDir, vendorOriginsDropInPath, vendorLaneAptConfig
	aptSourcesListDir = sources
	vendorOriginsDropInPath = filepath.Join(root, "52celikpanel-vendor-origins")
	vendorLaneAptConfig = func(context.Context) (map[string]string, map[string][]string, error) {
		lists := map[string][]string{"Unattended-Upgrade::Origins-Pattern": {"origin=Debian,codename=trixie,label=Debian-Security"}}
		if _, origins := readVendorOriginsDropIn(); len(origins) > 0 {
			for _, origin := range origins {
				lists["Unattended-Upgrade::Origins-Pattern"] = append(lists["Unattended-Upgrade::Origins-Pattern"], "origin="+origin)
			}
		}
		return map[string]string{"APT::Periodic::Unattended-Upgrade": automatic}, lists, nil
	}
	t.Cleanup(func() {
		aptSourcesListDir, vendorOriginsDropInPath, vendorLaneAptConfig = oldSources, oldDropIn, oldConfig
	})
	return root
}

func laneSuryLists(t *testing.T, root string, status string) {
	t.Helper()
	_, loc := suryForTest(t)
	lists := filepath.Join(root, "lists", loc.listsPrefix())
	writeTestFile(t, lists+"_InRelease", "-----BEGIN PGP SIGNED MESSAGE-----\nHash: SHA512\n\nOrigin: deb.sury.org\nLabel: PHP\nSuite: trixie\nSHA512:\n")
	writeTestFile(t, lists+"_main_binary-amd64_Packages", "Package: php8.4-fpm\nVersion: 8.4.13-1+sury\n\nPackage: php8.5-fpm\nVersion: 8.5.1-1\n")
	writeTestFile(t, filepath.Join(root, "status"), status)
}

func laneStates(plan transport.VendorLanePlan) string {
	parts := []string{}
	for _, step := range plan.Steps {
		part := step.Step + "=" + step.State
		if step.Reason != "" {
			part += "(" + step.Reason + ")"
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, " ")
}

const laneStatusWithUnattended = "Package: unattended-upgrades\nStatus: install ok installed\nVersion: 2.12\n\n"

func TestAnEnabledVendorRepositoryShowsItsMissingStepsAndCompletesThemAsConfirmed(t *testing.T) {
	root := laneHost(t, "1")
	repo, _ := suryForTest(t)
	laneSuryLists(t, root, laneStatusWithUnattended+"Package: php8.4-fpm\nStatus: install ok installed\nVersion: 8.4.13-1+sury\n\n")
	writeTestFile(t, repoSourcePath("sury"), "deb https://packages.sury.org/php/ trixie main\n")

	plan := vendorLanePlan(context.Background(), repo)
	if got := laneStates(plan); got != "repository=done confinement=todo updates=todo" || !plan.Ready || plan.SHA256 == "" {
		t.Fatalf("plan = %s %+v", got, plan)
	}
	if strings.Join(plan.Kept, ",") != "php8.4-fpm" || !strings.Contains(laneStepOf(plan, laneStepUpdates).Text, "\"origin=deb.sury.org\";") {
		t.Fatalf("plan text = %+v", plan)
	}
	// Anything but the confirmed plan writes nothing.
	var refused transport.VendorLaneResponse
	enableVendorLaneLocked(context.Background(), repo, strings.Repeat("0", 64), &refused)
	if refused.ErrorCode != "LANE_PLAN_CHANGED" || len(refused.Completed) != 0 {
		t.Fatalf("refused = %+v", refused)
	}
	if _, err := os.Stat(repoPinPath("sury")); !os.IsNotExist(err) {
		t.Fatal("an unconfirmed lane wrote the confinement")
	}
	var done transport.VendorLaneResponse
	enableVendorLaneLocked(context.Background(), repo, plan.SHA256, &done)
	if done.ErrorCode != "" || strings.Join(done.Completed, ",") != "confinement,updates" {
		t.Fatalf("done = %+v", done)
	}
	pin, _ := os.ReadFile(repoPinPath("sury"))
	if !strings.Contains(string(pin), "Package: php8.4-fpm\nPin: origin \"packages.sury.org\"\nPin-Priority: 500") {
		t.Fatalf("pin = %s", pin)
	}
	if state, origins := readVendorOriginsDropIn(); state != "present" || strings.Join(origins, ",") != "deb.sury.org" {
		t.Fatalf("drop-in = %s %v", state, origins)
	}
	after := vendorLanePlan(context.Background(), repo)
	if got := laneStates(after); got != "repository=done confinement=done updates=done" || after.Ready || after.SHA256 != "" {
		t.Fatalf("after = %s", got)
	}
}

func TestTheLaneNeverRewritesWhatTheOwnerWrote(t *testing.T) {
	root := laneHost(t, "1")
	repo, _ := suryForTest(t)
	laneSuryLists(t, root, laneStatusWithUnattended)
	writeTestFile(t, repoSourcePath("sury"), "deb https://packages.sury.org/php/ trixie main\n")
	// The owner's own preferences rule and an edited drop-in.
	writeTestFile(t, filepath.Join(root, "prefs", "sury.pref"), "Package: *\nPin: origin \"packages.sury.org\"\nPin-Priority: 50\n")
	writeTestFile(t, vendorOriginsDropInPath, "// mine\nUnattended-Upgrade::Origins-Pattern { \"origin=Mine\"; };\n")
	plan := vendorLanePlan(context.Background(), repo)
	if got := laneStates(plan); got != "repository=done confinement=owner(owner_rule) updates=blocked(drop_in_edited)" || plan.Ready {
		t.Fatalf("plan = %s", got)
	}
	if step := laneStepOf(plan, laneStepUpdates); step.Detail != `"origin=deb.sury.org";` {
		t.Fatalf("the owner is not given the line to add: %+v", step)
	}
}

func TestAutomaticUpdatesOffBlocksAFreshVendorRepository(t *testing.T) {
	if osCodename() == "" {
		t.Skip("the host has no os-release codename")
	}
	root := laneHost(t, "0")
	repo, _ := suryForTest(t)
	writeTestFile(t, filepath.Join(root, "status"), laneStatusWithUnattended)
	plan := vendorLanePlan(context.Background(), repo)
	if plan.Error != "" {
		t.Skipf("this host cannot render the source: %s", plan.Error)
	}
	if got := laneStates(plan); got != "repository=todo confinement=todo updates=blocked(automatic_off)" || plan.Ready {
		t.Fatalf("plan = %s", got)
	}
	var refused transport.VendorLaneResponse
	enableVendorLaneLocked(context.Background(), repo, plan.SHA256, &refused)
	if refused.ErrorCode != "LANE_BLOCKED" {
		t.Fatalf("refused = %+v", refused)
	}
	if _, err := os.Stat(repoSourcePath("sury")); !os.IsNotExist(err) {
		t.Fatal("a blocked lane enabled the repository")
	}
	// Without unattended-upgrades at all, the same.
	writeTestFile(t, filepath.Join(root, "status"), "")
	if step := laneStepOf(vendorLanePlan(context.Background(), repo), laneStepUpdates); step.Reason != "automatic_not_installed" {
		t.Fatalf("step = %+v", step)
	}
}

func TestAFreshVendorRepositoryPlanNamesTheSourceTheKeyAndAnEmptyConfinement(t *testing.T) {
	if osCodename() == "" {
		t.Skip("the host has no os-release codename")
	}
	root := laneHost(t, "1")
	repo, _ := suryForTest(t)
	writeTestFile(t, filepath.Join(root, "status"), laneStatusWithUnattended)
	plan := vendorLanePlan(context.Background(), repo)
	if plan.Error != "" {
		t.Skipf("this host cannot render the source: %s", plan.Error)
	}
	if got := laneStates(plan); got != "repository=todo confinement=todo updates=todo" || !plan.Ready {
		t.Fatalf("plan = %s", got)
	}
	repoStep := laneStepOf(plan, laneStepRepository)
	if !strings.HasPrefix(repoStep.Text, "deb https://packages.sury.org/php/ ") || !strings.Contains(repoStep.Text, repo.KeyFingerprint) {
		t.Fatalf("repository step = %+v", repoStep)
	}
	if conf := laneStepOf(plan, laneStepConfinement); strings.Contains(conf.Text, "Pin-Priority: 500") || !strings.Contains(conf.Text, "Pin-Priority: 100") {
		t.Fatalf("a fresh confinement keeps something: %s", conf.Text)
	}
}

func TestAFailedLaneTakesBackWhatItWrote(t *testing.T) {
	root := laneHost(t, "1")
	repo, _ := suryForTest(t)
	writeTestFile(t, filepath.Join(root, "status"), laneStatusWithUnattended)
	writeTestFile(t, vendorOriginsDropInPath, renderVendorOriginsDropIn([]string{"apt.postgresql.org"}))
	undo := &laneUndo{}
	if err := writeLanePin(repo, renderRepoPin(repo, "packages.sury.org", nil), nil); err != nil {
		t.Fatal(err)
	}
	undo.pin = repoPinPath(repo.ID)
	if err := writeLaneDropIn(renderVendorOriginsDropIn([]string{"apt.postgresql.org", "deb.sury.org"}), undo); err != nil {
		t.Fatal(err)
	}
	undo.run(repo.ID)
	if _, err := os.Stat(repoPinPath("sury")); !os.IsNotExist(err) {
		t.Fatal("the confinement stayed")
	}
	if _, origins := readVendorOriginsDropIn(); strings.Join(origins, ",") != "apt.postgresql.org" {
		t.Fatalf("the drop-in was not put back: %v", origins)
	}
	if entry := readRepoRecord().Repos["sury"]; entry.PinSHA256 != "" {
		t.Fatalf("record = %+v", entry)
	}
}

func TestADisabledRepositorysPackagesReadNoLongerUpdated(t *testing.T) {
	root := laneHost(t, "1")
	writeTestFile(t, filepath.Join(root, "status"), "Package: php8.4-fpm\nStatus: install ok installed\nVersion: 8.4.13-1+sury\n\n")
	if err := updateRepoRecord("sury", func(entry *repoRecordEntry, _ bool) bool {
		entry.NoLongerUpdated, entry.DisabledAt = []string{"php8.4-fpm", "php8.5-fpm"}, "2026-10-25T10:00:00Z"
		return true
	}); err != nil {
		t.Fatal(err)
	}
	names, at := laneNoLongerUpdated("sury")
	if strings.Join(names, ",") != "php8.4-fpm" || at != "2026-10-25T10:00:00Z" {
		t.Fatalf("no longer updated = %v %s", names, at)
	}
}

func TestOnlyCatalogueVendorLanesHaveALane(t *testing.T) {
	if laneRepo("sury") == nil || laneRepo("pgdg") == nil {
		t.Fatal("Sury and PGDG are vendor lanes")
	}
	if laneRepo("netdata") != nil || laneRepo("../x") != nil {
		t.Fatal("a repository without a known origin was given a lane")
	}
}

// alpha.108 review: an owner's own source for the vendor address. The lane
// never adds a second source; it offers only the confinement and the
// automatic updates for the owner's source. With CelikPanel's source as
// well, the duplicate is named and blocks.
func TestAnOwnersOwnVendorSourceGetsNoSecondSourceOnlyItsConfinementAndUpdates(t *testing.T) {
	if osCodename() == "" {
		t.Skip("the host has no os-release codename")
	}
	root := laneHost(t, "1")
	repo, _ := suryForTest(t)
	oldMain := aptMainSourcesList
	aptMainSourcesList = filepath.Join(root, "sources.list")
	t.Cleanup(func() { aptMainSourcesList = oldMain })
	writeTestFile(t, aptMainSourcesList, "deb http://deb.debian.org/debian "+osCodename()+" main\n")
	laneSuryLists(t, root, laneStatusWithUnattended)
	// The base image's and Frankfurt's shape: php.list with the owner's key.
	writeTestFile(t, filepath.Join(aptSourcesListDir, "php.list"), "# mine\ndeb [signed-by=/usr/share/keyrings/deb.sury.org-php.gpg] https://packages.sury.org/php/ "+osCodename()+" main\n")
	plan := vendorLanePlan(context.Background(), repo)
	if plan.Error != "" {
		t.Skipf("this host cannot render the source: %s", plan.Error)
	}
	if got := laneStates(plan); got != "repository=owner(owner_source) confinement=todo updates=todo" || !plan.Ready {
		t.Fatalf("plan = %s %+v", got, plan)
	}
	var resp transport.VendorLaneResponse
	enableVendorLaneLocked(context.Background(), repo, plan.SHA256, &resp)
	if resp.ErrorCode != "" || strings.Join(resp.Completed, ",") != "confinement,updates" {
		t.Fatalf("resp = %+v", resp)
	}
	if _, err := os.Stat(repoSourcePath("sury")); !os.IsNotExist(err) {
		t.Fatal("a second source was added beside the owner's")
	}
	if pin, _ := os.ReadFile(repoPinPath("sury")); !strings.Contains(string(pin), `Pin: origin "packages.sury.org"`) {
		t.Fatalf("pin = %s", pin)
	}
	// CelikPanel's source beside the owner's: the duplicate is named.
	writeTestFile(t, repoSourcePath("sury"), "deb https://packages.sury.org/php/ "+osCodename()+" main\n")
	plan = vendorLanePlan(context.Background(), repo)
	if step := laneStepOf(plan, laneStepRepository); step.State != laneStateBlocked || step.Reason != "duplicate_source" || !strings.Contains(step.Detail, "php.list") {
		t.Fatalf("duplicate = %+v", step)
	}
}

func TestAnOwnersDeb822SourceIsFoundAndDisabledStanzasAreNot(t *testing.T) {
	root := laneHost(t, "1")
	oldMain := aptMainSourcesList
	aptMainSourcesList = filepath.Join(root, "sources.list")
	t.Cleanup(func() { aptMainSourcesList = oldMain })
	repo := repoFromCatalogue("pgdg")
	want, err := parseAptSourceLine("deb https://apt.postgresql.org/pub/repos/apt trixie-pgdg main")
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(aptSourcesListDir, "pgdg.sources"), "Types: deb\nURIs: http://apt.postgresql.org/pub/repos/apt/\nSuites: trixie-pgdg\nComponents: main\nSigned-By: /usr/share/keyrings/pgdg.asc\n\nTypes: deb\nURIs: https://apt.postgresql.org/pub/repos/apt\nSuites: trixie-pgdg-testing\nComponents: main\nEnabled: no\n")
	owners := ownerSourcesFor(repo, want)
	if len(owners) != 1 || !strings.HasSuffix(owners[0].Path, "pgdg.sources") || owners[0].Loc.Suite != "trixie-pgdg" {
		t.Fatalf("owners = %+v", owners)
	}
	if got := ownerSourcesFor(repoFromCatalogue("sury"), aptSourceLocation{URI: "https://packages.sury.org/php/"}); len(got) != 0 {
		t.Fatalf("another address matched: %+v", got)
	}
}

func TestTheStagedSourceIsValidatedWithEveryExistingSource(t *testing.T) {
	root := laneHost(t, "1")
	writeTestFile(t, filepath.Join(aptSourcesListDir, "debian.sources"), "Types: deb\nURIs: https://deb.debian.org/debian\nSuites: trixie\nComponents: main\n")
	writeTestFile(t, filepath.Join(aptSourcesListDir, "php.list"), "deb https://packages.sury.org/php/ trixie main\n")
	writeTestFile(t, filepath.Join(aptSourcesListDir, ".celikpanel-sury-validate-1.list"), "deb [signed-by=/x] https://packages.sury.org/php/ trixie main\n")
	staged := filepath.Join(root, "staged.list")
	writeTestFile(t, staged, "deb [signed-by=/x] https://packages.sury.org/php/ trixie main\n")
	dir, cleanup, err := stagedSourcePartsDir(staged, "sury")
	if err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	if strings.Join(names, ",") != "celikpanel-sury.list,debian.sources,php.list" {
		t.Fatalf("parts = %v", names)
	}
	cleanup()
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("the parts directory stayed")
	}
}
