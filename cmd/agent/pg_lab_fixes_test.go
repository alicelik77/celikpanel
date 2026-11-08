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

// ownerFilesFixture redirects every path the owner-file keeping reads.
func ownerFilesFixture(t *testing.T) (root string, write func(path, content string)) {
	t.Helper()
	root = t.TempDir()
	oldInfo, oldUcf, oldKept, oldSystemd, oldEtc, oldNow := dpkgInfoDir, ucfRegistryPath, keptFilesRoot, systemdEtcDir, etcDir, pgLeftoverNow
	t.Cleanup(func() {
		dpkgInfoDir, ucfRegistryPath, keptFilesRoot, systemdEtcDir, etcDir, pgLeftoverNow = oldInfo, oldUcf, oldKept, oldSystemd, oldEtc, oldNow
	})
	dpkgInfoDir = filepath.Join(root, "dpkg", "info")
	ucfRegistryPath = filepath.Join(root, "ucf", "registry")
	keptFilesRoot = filepath.Join(root, "kept")
	systemdEtcDir = filepath.Join(root, "etc", "systemd", "system")
	etcDir = filepath.Join(root, "etc")
	write = func(path, content string) {
		os.MkdirAll(filepath.Dir(path), 0o755)
		if err := os.WriteFile(path, []byte(content), 0o640); err != nil {
			t.Fatal(err)
		}
	}
	return root, write
}

// alpha.130 re-verification: Postfix's purge left main.cf, master.cf,
// dynamicmaps.cf, the *.proto copies and CelikPanel's maps, and the first
// fix put them all back as "your own files". Now /etc/postfix is emptied
// and removed; only the owner's systemd drop-in goes back in place; a file
// that may be the owner's is copied to the kept folder, named.
func TestPostfixPurgeEmptiesItsFolderAndKeepsOnlyTheOwnersDropIn(t *testing.T) {
	_, write := ownerFilesFixture(t)
	dir := filepath.Join(etcDir, "postfix")
	script := filepath.Join(dir, "postfix-script")
	initScript := filepath.Join(etcDir, "init.d", "postfix")
	postinst := []string{"main.cf", "master.cf", "dynamicmaps.cf", "main.cf.proto", "master.cf.proto", "sasl/smtpd.conf",
		"virtual", "virtual.db", "vmailbox", "vmailbox.db", "vmailbox_domains", "vmailbox_domains.db", "celikpanel_sni"}
	for _, name := range postinst {
		write(filepath.Join(dir, name), name+"\n")
	}
	write(script, "#!/bin/sh\n")
	write(initScript, "#!/bin/sh\n")
	ownerMap := filepath.Join(dir, "my_access")
	write(ownerMap, "spam@example.com REJECT\n")
	dropIn := filepath.Join(systemdEtcDir, "postfix.service.d", "owner.conf")
	write(dropIn, "[Service]\nNice=5\n")
	write(filepath.Join(dpkgInfoDir, "postfix.list"), "/etc\n"+dir+"\n"+script+"\n"+initScript+"\n")
	shipped, _ := fileMD5(initScript)
	status := "Package: postfix\nStatus: install ok installed\nConffiles:\n " + initScript + " " + shipped + "\nVersion: 3\n"
	keep, err := keepOwnerFiles("postfix", []string{"postfix"}, []string{"postfix", "postfix@-"}, strings.NewReader(status))
	if err != nil {
		t.Fatal(err)
	}
	// The purge: the package's own files and the drop-in folder go; the
	// maintainer-script and CelikPanel files stay (as on the lab VM).
	os.Remove(script)
	os.Remove(initScript)
	os.RemoveAll(filepath.Join(systemdEtcDir, "postfix.service.d"))
	out := keep.finish(true)
	if !reflect.DeepEqual(out.Restored, []string{dropIn}) || !reflect.DeepEqual(out.Copied, []string{ownerMap}) ||
		len(out.Changed)+len(out.Left)+len(out.Notes) != 0 || !reflect.DeepEqual(out.Cleared, []string{dir}) {
		t.Fatalf("outcome = %+v", out)
	}
	if _, err := os.Lstat(dir); !os.IsNotExist(err) {
		t.Fatalf("%s stayed", dir)
	}
	if data, err := os.ReadFile(dropIn); err != nil || string(data) != "[Service]\nNice=5\n" {
		t.Fatalf("drop-in back = %q %v", data, err)
	}
	if info, _ := os.Stat(dropIn); info.Mode().Perm() != 0o640 {
		t.Fatalf("mode = %v", info.Mode().Perm())
	}
	copied := filepath.Join(out.CopyDir, strings.TrimPrefix(ownerMap, "/"))
	if data, err := os.ReadFile(copied); err != nil || string(data) != "spam@example.com REJECT\n" {
		t.Fatalf("copy = %q %v", data, err)
	}
	if info, _ := os.Stat(copied); info.Mode().Perm() != 0o600 {
		t.Fatalf("the copy is root-only: %v", info.Mode().Perm())
	}
	if detail := out.detail(); !strings.Contains(detail, "a copy of 1 file(s) you may have added is in "+out.CopyDir) || strings.Contains(detail, "main.cf") {
		t.Fatalf("detail = %q", detail)
	}
}

// postgresql-common: createcluster.conf is ucf's, root.crt its postinst's;
// they go with the folder, never back; an owner's note is copied.
func TestPostgreSQLCommonFolderGoesWithTheWholeRemoval(t *testing.T) {
	_, write := ownerFilesFixture(t)
	dir := filepath.Join(etcDir, "postgresql-common")
	owned := filepath.Join(dir, "pg_upgradecluster.d", "analyze")
	write(owned, "x")
	write(filepath.Join(dir, "createcluster.conf"), "create_main_cluster = true\n")
	write(filepath.Join(dir, "root.crt"), "")
	note := filepath.Join(dir, "my-notes.txt")
	write(note, "remember 5433\n")
	write(filepath.Join(dpkgInfoDir, "postgresql-common.list"), dir+"\n"+owned+"\n")
	write(ucfRegistryPath, "0123456789abcdef0123456789abcdef  postgresql-common  "+filepath.Join(dir, "createcluster.conf")+"\n")
	keep, err := keepOwnerFiles("postgresql", []string{"postgresql-common", "postgresql-18"}, []string{"postgresql"}, strings.NewReader(""))
	if err != nil {
		t.Fatal(err)
	}
	os.Remove(owned) // the purge
	out := keep.finish(true)
	if !reflect.DeepEqual(out.Copied, []string{note}) || !reflect.DeepEqual(out.Cleared, []string{dir}) || len(out.Restored) != 0 {
		t.Fatalf("outcome = %+v", out)
	}
	if _, err := os.Lstat(dir); !os.IsNotExist(err) {
		t.Fatal("/etc/postgresql-common stayed")
	}
	// One version's removal does not reach the shared folder.
	write(filepath.Join(dir, "createcluster.conf"), "x")
	write(filepath.Join(dpkgInfoDir, "postgresql-16.list"), "/usr/lib/postgresql/16/bin/postgres\n")
	keep, _ = keepOwnerFiles("postgresql", []string{"postgresql-16"}, nil, strings.NewReader(""))
	out = keep.finish(true)
	if len(out.Cleared) != 0 {
		t.Fatalf("a version's removal cleared %v", out.Cleared)
	}
	if _, err := os.Stat(filepath.Join(dir, "createcluster.conf")); err != nil {
		t.Fatal("a version's removal touched postgresql-common's folder")
	}
}

// A purge that failed empties nothing: the package may still be there.
func TestFailedPurgeEmptiesNothing(t *testing.T) {
	_, write := ownerFilesFixture(t)
	dir := filepath.Join(etcDir, "dovecot")
	write(filepath.Join(dir, "dovecot.conf"), "x")
	users := filepath.Join(dir, "users")
	write(users, "a@example.com:{CRYPT}x\n")
	write(filepath.Join(dpkgInfoDir, "dovecot-core.list"), dir+"\n"+filepath.Join(dir, "dovecot.conf")+"\n")
	keep, err := keepOwnerFiles("dovecot", []string{"dovecot-core"}, []string{"dovecot"}, strings.NewReader(""))
	if err != nil {
		t.Fatal(err)
	}
	out := keep.finish(false)
	if len(out.Cleared) != 0 {
		t.Fatalf("outcome = %+v", out)
	}
	if _, err := os.Stat(users); err != nil {
		t.Fatal("a failed purge removed files")
	}
	// After a purge that worked, CelikPanel's users file goes with the
	// folder, without a copy.
	keep, _ = keepOwnerFiles("dovecot", []string{"dovecot-core"}, []string{"dovecot"}, strings.NewReader(""))
	out = keep.finish(true)
	if len(out.Copied) != 0 || !reflect.DeepEqual(out.Cleared, []string{dir}) {
		t.Fatalf("outcome = %+v", out)
	}
	// A refused removal discards every copy.
	write(filepath.Join(dir, "mine.conf"), "x")
	write(filepath.Join(dir, "dovecot.conf"), "x")
	write(filepath.Join(dpkgInfoDir, "dovecot-core.list"), dir+"\n"+filepath.Join(dir, "dovecot.conf")+"\n")
	keep, _ = keepOwnerFiles("dovecot", []string{"dovecot-core"}, nil, strings.NewReader(""))
	if len(keep.files) != 1 {
		t.Fatalf("kept = %+v", keep.files)
	}
	keep.discard()
	if _, err := os.Stat(keep.root); !os.IsNotExist(err) {
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
