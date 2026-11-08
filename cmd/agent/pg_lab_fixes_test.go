package main

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.130 lab findings (Ubuntu 24.04): a whole PostgreSQL removal left
// rc postgresql-16, postgresql-client-16, postgresql-common,
// postgresql-client-common, empty folders and the PGDG repository; Postfix's
// purge script deleted an owner's systemd drop-in without a word.

const labWholeStatus = `Package: postgresql-16
Status: deinstall ok config-files
Version: 16.10

Package: postgresql-client-16
Status: install ok installed
Version: 16.10

Package: postgresql-18
Status: install ok installed
Version: 18.1-1.pgdg24.04+2

Package: postgresql-client-18
Status: install ok installed
Version: 18.1-1.pgdg24.04+2

Package: postgresql-common
Status: install ok installed
Version: 293.pgdg24.04+1

Package: postgresql-client-common
Status: install ok installed
Version: 293.pgdg24.04+1

Package: libpq5
Status: install ok installed
Version: 18.6-1.pgdg24.04+2

Package: postgresql-contrib
Status: deinstall ok config-files
Version: 16
`

func TestWholePostgreSQLRemovalNamesWhatEarlierRemovalsLeft(t *testing.T) {
	states, err := parseDpkgStates(strings.NewReader(labWholeStatus))
	if err != nil {
		t.Fatal(err)
	}
	packages, prefixes := pgWholeRemovalExtras(states)
	want := []string{"postgresql-16", "postgresql-client-16", "postgresql-client-18", "postgresql-client-common", "postgresql-common", "postgresql-contrib"}
	if !reflect.DeepEqual(packages, want) || !reflect.DeepEqual(prefixes, []string{"postgresql-16"}) {
		t.Fatalf("extras = %v %v", packages, prefixes)
	}
	for _, name := range packages {
		if name == "libpq5" {
			t.Fatal("libpq5 is shared with other programs and is never named")
		}
	}
	svc := core.GetManagedServiceByID("postgresql")
	ops := serviceUninstallOps{
		detectPackageFamily: func() string { return "apt" },
		packageInstalled: func(pkg string) bool {
			return pkg == "postgresql-18" || pkg == "postgresql-client-18" || pkg == "postgresql"
		},
		unitExists:            func(string) bool { return false },
		unitsMatching:         func(string) []string { return nil },
		installedRepoPackages: func(*core.ManagedService) ([]string, error) { return []string{"postgresql-18"}, nil },
		dpkgStates:            func() ([]dpkgPackageState, error) { return states, nil },
	}
	work, refusal := planServiceRemovalWork(svc, "", ops)
	if refusal != "" {
		t.Fatal(refusal)
	}
	got := strings.Join(work.request.Packages, " ")
	for _, name := range append(want, "postgresql", "postgresql-18") {
		if !strings.Contains(" "+got+" ", " "+name+" ") {
			t.Errorf("whole removal misses %s: %s", name, got)
		}
	}
	if work.request.KeepData {
		t.Error("a whole PostgreSQL removal purges")
	}
	if !strings.Contains(strings.Join(work.request.VersionPrefixes, " "), "postgresql-16") {
		t.Errorf("prefixes = %v", work.request.VersionPrefixes)
	}
}

func TestPGRemovalHoldsTreatsLeftoversByTheirFiles(t *testing.T) {
	f := newPGLeftoverFixture(t)
	f.installMajor("18")
	fresh := f.cluster("16", "main", 5432, "down,binaries_missing")
	kept := f.cluster("15", "main", 5434, "down,binaries_missing")
	f.write(filepath.Join(kept.DataDirectory, "base", "16384", "PG_VERSION"), "15")
	host := pgClusterHost{
		family:     func() string { return "apt" },
		lsclusters: func(context.Context) (string, error) { return pgLsLine(fresh) + pgLsLine(kept), nil },
	}
	held, unknown := pgRemovalHolds(context.Background(), host, "")
	if len(unknown) != 0 || len(held) != 1 || !strings.HasPrefix(held[0], "15/main: data kept from removed PostgreSQL 15") {
		t.Fatalf("held=%v unknown=%v", held, unknown)
	}
	host.lsclusters = func(context.Context) (string, error) { return pgLsLine(fresh), nil }
	if held, unknown := pgRemovalHolds(context.Background(), host, ""); len(held)+len(unknown) != 0 {
		t.Fatalf("a fresh leftover goes with the removal: held=%v unknown=%v", held, unknown)
	}
}

func TestWholeRemovalFinishRemovesEmptyFoldersAndKeepsPGDGWhileUsed(t *testing.T) {
	f := newPGLeftoverFixture(t)
	for _, dir := range []string{pgConfRoot, pgDataRoot, pgLogRoot} {
		os.MkdirAll(dir, 0o755)
	}
	f.write(filepath.Join(pgDataRoot, ".psql_history"), "select 1;")
	oldStatus, oldSources := dpkgStatusPath, aptSourcesListDir
	t.Cleanup(func() { dpkgStatusPath, aptSourcesListDir = oldStatus, oldSources })
	dpkgStatusPath = filepath.Join(f.root, "status")
	aptSourcesListDir = filepath.Join(f.root, "sources.list.d")
	f.write(dpkgStatusPath, "Package: libpq5\nStatus: install ok installed\nVersion: 18.6-1.pgdg24.04+2\n\nPackage: php8.3-pgsql\nStatus: install ok installed\nVersion: 8.3\n")
	purged := 0
	pgLeftoverPurge = func(_ context.Context, request packageRemovalRequest) (packageRemovalPlan, error) {
		purged++
		if !reflect.DeepEqual(request.Packages, []string{"libpq5"}) {
			t.Fatalf("request = %v", request.Packages)
		}
		return packageRemovalPlan{}, &packageRemovalRefusal{Code: errCodeRemovalWouldRemoveOthers, Message: "it would also remove php8.3-pgsql"}
	}
	notes := strings.Join(pgWholeRemovalFinish(context.Background()), " | ")
	if purged != 1 || !strings.Contains(notes, "libpq5 from PGDG was kept: it would also remove php8.3-pgsql") ||
		!strings.Contains(notes, "the PGDG repository was kept: libpq5 still come from it") ||
		!strings.Contains(notes, pgDataRoot+" was kept: it still holds .psql_history") {
		t.Fatalf("notes = %s", notes)
	}
	for _, dir := range []string{pgConfRoot, pgLogRoot} {
		if _, err := os.Lstat(dir); !os.IsNotExist(err) {
			t.Fatalf("%s stayed although empty", dir)
		}
	}
}

func TestOwnerFilesNextToAPurgedPackageAreKept(t *testing.T) {
	root := t.TempDir()
	oldInfo, oldKept, oldSystemd, oldEtc, oldNow := dpkgInfoDir, keptFilesRoot, systemdEtcDir, etcDir, pgLeftoverNow
	t.Cleanup(func() {
		dpkgInfoDir, keptFilesRoot, systemdEtcDir, etcDir, pgLeftoverNow = oldInfo, oldKept, oldSystemd, oldEtc, oldNow
	})
	dpkgInfoDir = filepath.Join(root, "dpkg", "info")
	keptFilesRoot = filepath.Join(root, "kept")
	systemdEtcDir = filepath.Join(root, "etc", "systemd", "system")
	etcDir = filepath.Join(root, "etc")
	write := func(path, content string) {
		os.MkdirAll(filepath.Dir(path), 0o755)
		if err := os.WriteFile(path, []byte(content), 0o640); err != nil {
			t.Fatal(err)
		}
	}
	mainCF := filepath.Join(etcDir, "postfix", "main.cf")
	masterCF := filepath.Join(etcDir, "postfix", "master.cf")
	ownerMap := filepath.Join(etcDir, "postfix", "my_access")
	ourMap := filepath.Join(etcDir, "postfix", "celikpanel_sni")
	dropIn := filepath.Join(systemdEtcDir, "postfix.service.d", "override.conf")
	write(mainCF, "edited by the owner\n")
	write(masterCF, "as shipped\n")
	write(ownerMap, "spam@example.com REJECT\n")
	write(ourMap, "sni\n")
	write(dropIn, "[Service]\nNice=5\n")
	write(filepath.Join(dpkgInfoDir, "postfix.list"), "/etc\n"+mainCF+"\n"+masterCF+"\n")
	shipped, _ := fileMD5(masterCF)
	status := "Package: postfix\nStatus: install ok installed\nConffiles:\n " + mainCF + " 00000000000000000000000000000000\n " + masterCF + " " + shipped + "\nVersion: 3\n"
	keep, err := keepOwnerFiles("postfix", []string{"postfix"}, []string{"postfix", "postfix@-"}, strings.NewReader(status))
	if err != nil {
		t.Fatal(err)
	}
	// The purge script deletes the drop-in folder and /etc/postfix.
	os.RemoveAll(filepath.Join(systemdEtcDir, "postfix.service.d"))
	os.RemoveAll(filepath.Join(etcDir, "postfix"))
	restored, inPlace, copies, notes := keep.restore()
	if !reflect.DeepEqual(restored, []string{ownerMap, dropIn}) || len(inPlace) != 0 || !reflect.DeepEqual(copies, []string{mainCF}) || len(notes) != 0 {
		t.Fatalf("restored=%v inPlace=%v copies=%v notes=%v", restored, inPlace, copies, notes)
	}
	if data, err := os.ReadFile(dropIn); err != nil || string(data) != "[Service]\nNice=5\n" {
		t.Fatalf("drop-in back = %q %v", data, err)
	}
	if info, _ := os.Stat(dropIn); info.Mode().Perm() != 0o640 {
		t.Fatalf("mode = %v", info.Mode().Perm())
	}
	if _, err := os.Stat(ourMap); !os.IsNotExist(err) {
		t.Fatal("CelikPanel's own map is the component cleanup's, never put back")
	}
	if _, err := os.Stat(masterCF); !os.IsNotExist(err) {
		t.Fatal("an unchanged package file is not kept")
	}
	copyOfMain := filepath.Join(keep.root, strings.TrimPrefix(mainCF, "/"))
	if data, err := os.ReadFile(copyOfMain); err != nil || string(data) != "edited by the owner\n" {
		t.Fatalf("the owner's edited main.cf copy = %q %v", data, err)
	}
	if detail := keptOwnerFilesDetail(keep.root, restored, inPlace, copies, notes); !strings.Contains(detail, dropIn) || !strings.Contains(detail, keep.root) {
		t.Fatalf("detail = %q", detail)
	}
	// A refused removal discards every copy.
	keep2, _ := keepOwnerFiles("postfix", []string{"postfix"}, []string{"postfix"}, strings.NewReader(status))
	keep2.discard()
	if _, err := os.Stat(keep2.root); !os.IsNotExist(err) {
		t.Fatal("copies stayed after a refusal")
	}
}

func TestUninstallKeepsOwnerFilesAroundThePurge(t *testing.T) {
	var events []string
	keep := &ownerFilesKeep{}
	ops := serviceUninstallOps{
		detectPackageFamily: func() string { return "apt" },
		packageInstalled:    func(string) bool { return true },
		unitExists:          func(unit string) bool { return unit == "postfix" },
		unitsMatching:       func(string) []string { return nil },
		disableUnit:         func(unit string) error { events = append(events, "stop "+unit); return nil },
		removePackages: func(_ string, req packageRemovalRequest, hooks removalHooks) (packageRemovalPlan, string, error) {
			if err := hooks.runStop(); err != nil {
				return packageRemovalPlan{}, "", err
			}
			events = append(events, "purge")
			return packageRemovalPlan{Remove: req.Packages}, "", nil
		},
		keepOwnerFiles: func(component string, packages, units []string) (*ownerFilesKeep, error) {
			events = append(events, "keep "+component)
			if !strings.Contains(strings.Join(units, " "), "postfix") {
				t.Fatalf("units = %v", units)
			}
			return keep, nil
		},
	}
	var response UninstallServiceResponse
	if err := (&Agent{}).uninstallServiceWithOps(&InstallServiceRequest{ID: "postfix"}, &response, ops); err != nil {
		t.Fatal(err)
	}
	if !response.Removed || !reflect.DeepEqual(events, []string{"stop postfix", "keep postfix", "purge"}) {
		t.Fatalf("response=%+v events=%v", response, events)
	}
	_ = transport.UninstallServiceResponse{}
}
