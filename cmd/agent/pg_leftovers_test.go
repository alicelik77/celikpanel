package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.130 (the owner's rule of 8 Nov 2026: "no leftovers, ever"). Boston:
// PostgreSQL 16 was removed with its cluster kept ("16 main 5432
// down,binaries_missing") and PostgreSQL 18 then ran on 5433. Mounted tests:
// the clusters' files live in temporary /etc/postgresql, /var/lib/postgresql,
// /var/log/postgresql and /var/backups/celikpanel; nothing runs on the host.

type pgLeftoverFixture struct {
	t    *testing.T
	root string
}

func newPGLeftoverFixture(t *testing.T) *pgLeftoverFixture {
	t.Helper()
	root := t.TempDir()
	oldConf, oldLib, oldData, oldLog, oldArchive := pgConfRoot, pgLibRoot, pgDataRoot, pgLogRoot, pgArchiveRoot
	oldReset, oldNow := pgLeftoverUnitReset, pgLeftoverNow
	pgConfRoot = filepath.Join(root, "etc", "postgresql")
	pgLibRoot = filepath.Join(root, "usr", "lib", "postgresql")
	pgDataRoot = filepath.Join(root, "var", "lib", "postgresql")
	pgLogRoot = filepath.Join(root, "var", "log", "postgresql")
	pgArchiveRoot = filepath.Join(root, "var", "backups", "celikpanel")
	pgLeftoverUnitReset = func(string, string) {}
	pgLeftoverNow = func() time.Time { return time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC) }
	t.Cleanup(func() {
		pgConfRoot, pgLibRoot, pgDataRoot, pgLogRoot, pgArchiveRoot = oldConf, oldLib, oldData, oldLog, oldArchive
		pgLeftoverUnitReset, pgLeftoverNow = oldReset, oldNow
	})
	return &pgLeftoverFixture{t: t, root: root}
}

func (f *pgLeftoverFixture) write(path, content string) {
	f.t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		f.t.Fatal(err)
	}
}

// installMajor puts a server program for version in pgLibRoot.
func (f *pgLeftoverFixture) installMajor(version string) {
	f.write(filepath.Join(pgLibRoot, version, "bin", "postgres"), "#!/bin/true")
}

type pgRoleRow struct {
	oid  uint32
	name string
}

// pgAuthidPage builds one 8 KiB heap page of pg_authid rows (PostgreSQL 12+
// layout: oid, then rolname as a 64-byte name).
func pgAuthidPage(rows []pgRoleRow) []byte {
	page := make([]byte, pgPageSize)
	upper := pgPageSize
	lower := 24
	for _, row := range rows {
		const hoff = 24
		length := hoff + 4 + 64 + 8
		upper -= length
		upper &^= 7
		tuple := page[upper : upper+length]
		tuple[22] = hoff
		binary.LittleEndian.PutUint32(tuple[hoff:], row.oid)
		copy(tuple[hoff+4:hoff+4+64], row.name)
		id := uint32(upper) | 1<<15 | uint32(length)<<17
		binary.LittleEndian.PutUint32(page[lower:], id)
		lower += 4
	}
	binary.LittleEndian.PutUint16(page[12:], uint16(lower))
	binary.LittleEndian.PutUint16(page[14:], uint16(upper))
	binary.LittleEndian.PutUint16(page[16:], pgPageSize)
	binary.LittleEndian.PutUint16(page[18:], pgPageSize|4)
	return page
}

var pgInitdbRoles = []pgRoleRow{{10, "postgres"}, {6171, "pg_database_owner"}, {3373, "pg_monitor"}, {4200, "pg_signal_backend"}}

// cluster writes a cluster as initdb leaves it (and its configuration and
// log), and returns it as pg_lsclusters names it.
func (f *pgLeftoverFixture) cluster(version, name string, port int, status string) transport.PostgreSQLCluster {
	data := filepath.Join(pgDataRoot, version, name)
	f.write(filepath.Join(pgConfRoot, version, name, "postgresql.conf"), "port = "+strconv.Itoa(port)+"\n")
	f.write(filepath.Join(pgConfRoot, version, name, "pg_hba.conf"), "local all postgres peer\n")
	f.write(filepath.Join(pgLogRoot, "postgresql-"+version+"-"+name+".log"), "log\n")
	f.write(filepath.Join(pgLogRoot, "postgresql-"+version+"-"+name+".log.1.gz"), "old log\n")
	f.write(filepath.Join(data, "PG_VERSION"), version+"\n")
	for _, db := range []string{"1", "4", "5"} {
		for _, file := range []string{"1247", "1249", "1259", "1259_fsm", "1259_vm", "2608", "13450", "13450.1", "PG_VERSION", "pg_filenode.map"} {
			f.write(filepath.Join(data, "base", db, file), "x")
		}
	}
	for _, file := range []string{"1213", "1262", "pg_control", "pg_filenode.map"} {
		f.write(filepath.Join(data, "global", file), "x")
	}
	f.write(filepath.Join(data, "global", "1260"), string(pgAuthidPage(pgInitdbRoles)))
	for _, dir := range []string{"pg_tblspc", "pg_wal", "pg_xact"} {
		if err := os.MkdirAll(filepath.Join(data, dir), 0o700); err != nil {
			f.t.Fatal(err)
		}
	}
	return transport.PostgreSQLCluster{Version: version, Name: name, Port: port, Status: status, Online: strings.HasPrefix(status, "online"), Owner: "postgres", DataDirectory: data, ConfigPath: filepath.Join(pgConfRoot, version, name, "postgresql.conf")}
}

func pgLsLine(c transport.PostgreSQLCluster) string {
	return c.Version + " " + c.Name + " " + strconv.Itoa(c.Port) + " " + c.Status + " postgres " + c.DataDirectory + " " + filepath.Join(pgLogRoot, "postgresql-"+c.Version+"-"+c.Name+".log") + "\n"
}

func TestPGLeftoverClassifierFreshClusterOnlyHoldsWhatInitdbMade(t *testing.T) {
	f := newPGLeftoverFixture(t)
	f.installMajor("18")
	dead := f.cluster("16", "main", 5432, "down,binaries_missing")
	if !pgClusterIsLeftover(dead) {
		t.Fatal("16/main (program gone) is a leftover")
	}
	got := classifyPGLeftover(dead)
	if !got.Fresh || got.Reason != "" || got.DataBytes == 0 {
		t.Fatalf("fresh initdb cluster = %+v", got)
	}
	live := f.cluster("18", "main", 5433, "online")
	if pgClusterIsLeftover(live) {
		t.Fatal("a running cluster is never a leftover")
	}
	stopped := live
	stopped.Status, stopped.Online = "down", false
	if pgClusterIsLeftover(stopped) {
		t.Fatal("a stopped cluster whose program is installed is not a leftover")
	}
	// pg_lsclusters says binaries_missing, but the program is there: trust
	// the disk, not the word.
	f.installMajor("16")
	if pgClusterIsLeftover(dead) {
		t.Fatal("a major whose program is on disk is not a leftover")
	}
}

func TestPGLeftoverClassifierRefusesAnythingThatMayBeData(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(f *pgLeftoverFixture, c *transport.PostgreSQLCluster)
		reason string
	}{
		{"user database", func(f *pgLeftoverFixture, c *transport.PostgreSQLCluster) {
			f.write(filepath.Join(c.DataDirectory, "base", "16384", "PG_VERSION"), "16")
		}, "database made after initdb"},
		{"table in postgres", func(f *pgLeftoverFixture, c *transport.PostgreSQLCluster) {
			f.write(filepath.Join(c.DataDirectory, "base", "5", "16390"), "rows")
		}, "objects made after initdb"},
		{"table segment", func(f *pgLeftoverFixture, c *transport.PostgreSQLCluster) {
			f.write(filepath.Join(c.DataDirectory, "base", "1", "24576.2"), "rows")
		}, "objects made after initdb"},
		{"tablespace", func(f *pgLeftoverFixture, c *transport.PostgreSQLCluster) {
			f.write(filepath.Join(c.DataDirectory, "pg_tblspc", "16400"), "")
		}, "tablespaces"},
		{"user role", func(f *pgLeftoverFixture, c *transport.PostgreSQLCluster) {
			f.write(filepath.Join(c.DataDirectory, "global", "1260"), string(pgAuthidPage(append(append([]pgRoleRow{}, pgInitdbRoles...), pgRoleRow{16385, "shop"}))))
		}, "roles made after initdb (shop)"},
		{"unreadable roles", func(f *pgLeftoverFixture, c *transport.PostgreSQLCluster) {
			f.write(filepath.Join(c.DataDirectory, "global", "1260"), "not a heap page")
		}, "roles could not be read"},
		{"running", func(f *pgLeftoverFixture, c *transport.PostgreSQLCluster) {
			f.write(filepath.Join(c.DataDirectory, "postmaster.pid"), "123\n")
		}, "postmaster.pid"},
		{"other directory", func(f *pgLeftoverFixture, c *transport.PostgreSQLCluster) {
			c.DataDirectory = filepath.Join(f.root, "srv", "pg16")
		}, "not the standard"},
		{"link inside", func(f *pgLeftoverFixture, c *transport.PostgreSQLCluster) {
			os.RemoveAll(filepath.Join(c.DataDirectory, "pg_wal"))
			if err := os.Symlink(f.root, filepath.Join(c.DataDirectory, "pg_wal")); err != nil {
				t.Skip("symlinks unavailable")
			}
		}, "is a link"},
		{"unknown file", func(f *pgLeftoverFixture, c *transport.PostgreSQLCluster) {
			f.write(filepath.Join(c.DataDirectory, "base", "5", "dump.sql"), "COPY")
		}, "unexpected file"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newPGLeftoverFixture(t)
			c := f.cluster("16", "main", 5432, "down,binaries_missing")
			tc.mutate(f, &c)
			got := classifyPGLeftover(c)
			if got.Fresh || !strings.Contains(got.Reason, tc.reason) {
				t.Fatalf("classifier = %+v, want not fresh with %q", got, tc.reason)
			}
		})
	}
}

func TestPGLeftoverClassifierAllowsCelikPanelsOwnRole(t *testing.T) {
	f := newPGLeftoverFixture(t)
	c := f.cluster("16", "main", 5432, "down,binaries_missing")
	f.write(filepath.Join(c.DataDirectory, "global", "1260"), string(pgAuthidPage(append(append([]pgRoleRow{}, pgInitdbRoles...), pgRoleRow{16384, databaseAdminAccountName}))))
	if got := classifyPGLeftover(c); !got.Fresh {
		t.Fatalf("only CelikPanel's own account = %+v", got)
	}
	roles, ok := pgAuthidUserRoles(append(pgAuthidPage([]pgRoleRow{{16390, "b"}, {16391, "a"}}), make([]byte, pgPageSize)...))
	if !ok || !reflect.DeepEqual(roles, []string{"a", "b"}) {
		t.Fatalf("two pages, the second new: %v %v", roles, ok)
	}
	if _, ok := pgAuthidUserRoles(make([]byte, 100)); ok {
		t.Fatal("a short file is not heap pages")
	}
}

func TestPGLeftoverCleanupRemovesOnlyFreshLeftoversAndTheirOwnFiles(t *testing.T) {
	f := newPGLeftoverFixture(t)
	f.installMajor("18")
	fresh := f.cluster("16", "main", 5432, "down,binaries_missing")
	held := f.cluster("15", "main", 5434, "down,binaries_missing")
	f.write(filepath.Join(held.DataDirectory, "base", "16384", "PG_VERSION"), "15")
	live := f.cluster("18", "main", 5433, "online")
	f.write(filepath.Join(pgLogRoot, "postgresql-16-main2.log"), "another cluster's log")
	var resets []string
	pgLeftoverUnitReset = func(v, n string) { resets = append(resets, v+"/"+n) }
	host := pgClusterHost{
		family:     func() string { return "apt" },
		lsclusters: func(context.Context) (string, error) { return pgLsLine(held) + pgLsLine(fresh) + pgLsLine(live), nil },
	}
	var resp transport.PostgreSQLLeftoverCleanupResponse
	cleanPGLeftovers(context.Background(), host, &resp)
	if resp.Error != "" || len(resp.Removed) != 1 || len(resp.Kept) != 1 || resp.Kept[0].Version != "15" {
		t.Fatalf("cleanup = %+v", resp)
	}
	removal := resp.Removed[0]
	want := []string{
		filepath.Join(pgConfRoot, "16", "main"), filepath.Join(pgDataRoot, "16", "main"),
		filepath.Join(pgLogRoot, "postgresql-16-main.log"), filepath.Join(pgLogRoot, "postgresql-16-main.log.1.gz"),
		filepath.Join(pgConfRoot, "16"), filepath.Join(pgDataRoot, "16"),
	}
	if !reflect.DeepEqual(removal.Removed, want) || removal.Archive != "" || removal.Port != 5432 {
		t.Fatalf("removal = %+v, want %v", removal, want)
	}
	for _, gone := range want {
		if _, err := os.Lstat(gone); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s is still there", gone)
		}
	}
	for _, kept := range []string{held.DataDirectory, held.ConfigPath, live.DataDirectory, live.ConfigPath, filepath.Join(pgLogRoot, "postgresql-16-main2.log")} {
		if _, err := os.Lstat(kept); err != nil {
			t.Fatalf("%s was touched: %v", kept, err)
		}
	}
	if !reflect.DeepEqual(resets, []string{"16/main"}) {
		t.Fatalf("unit resets = %v", resets)
	}
	// The next run finds nothing more to do.
	host.lsclusters = func(context.Context) (string, error) { return pgLsLine(held) + pgLsLine(live), nil }
	resp = transport.PostgreSQLLeftoverCleanupResponse{}
	cleanPGLeftovers(context.Background(), host, &resp)
	if len(resp.Removed) != 0 || len(resp.Kept) != 1 {
		t.Fatalf("second run = %+v", resp)
	}
}

func TestPGLeftoverArchiveAndRemove(t *testing.T) {
	f := newPGLeftoverFixture(t)
	f.installMajor("18")
	held := f.cluster("16", "main", 5432, "down,binaries_missing")
	f.write(filepath.Join(held.DataDirectory, "base", "16384", "16385"), "shop rows")
	host := pgClusterHost{
		family:     func() string { return "apt" },
		lsclusters: func(context.Context) (string, error) { return pgLsLine(held), nil },
		freeBytes:  func(string) (int64, error) { return 10 << 30, nil },
	}
	var resp transport.PostgreSQLLeftoverRemoveResponse
	removePGLeftover(context.Background(), host, &transport.PostgreSQLLeftoverRemoveRequest{PostgreSQLClusterRef: transport.PostgreSQLClusterRef{Version: "16", Name: "main"}}, &resp)
	wantArchive := filepath.Join(pgArchiveRoot, "postgresql-16-main-20261108T120000Z.tar.gz")
	if !resp.Removed || resp.Removal.Archive != wantArchive || resp.Error != "" {
		t.Fatalf("archive and remove = %+v", resp)
	}
	info, err := os.Stat(wantArchive)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("archive %v %v", info, err)
	}
	names := tarNames(t, wantArchive)
	for _, want := range []string{"base/16384/16385", "PG_VERSION", "postgresql.conf", "postgresql-16-main.log"} {
		if !containsSuffix(names, want) {
			t.Fatalf("archive misses %s: %v", want, names)
		}
	}
	if _, err := os.Lstat(held.DataDirectory); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("the data directory is still there")
	}
	// Free space too small: nothing removed.
	f2 := newPGLeftoverFixture(t)
	other := f2.cluster("16", "main", 5432, "down,binaries_missing")
	host.lsclusters = func(context.Context) (string, error) { return pgLsLine(other), nil }
	host.freeBytes = func(string) (int64, error) { return 1 << 20, nil }
	resp = transport.PostgreSQLLeftoverRemoveResponse{}
	removePGLeftover(context.Background(), host, &transport.PostgreSQLLeftoverRemoveRequest{PostgreSQLClusterRef: transport.PostgreSQLClusterRef{Version: "16", Name: "main"}}, &resp)
	if resp.Removed || resp.Changed || resp.ErrorCode != "ARCHIVE_FAILED" {
		t.Fatalf("no room = %+v", resp)
	}
	if _, err := os.Lstat(other.DataDirectory); err != nil {
		t.Fatal("a refused archive removed data")
	}
	if entries, _ := os.ReadDir(pgArchiveRoot); len(entries) != 0 {
		t.Fatalf("a refused archive left files: %v", entries)
	}
	// Not a leftover (it runs): refused.
	f2.installMajor("16")
	resp = transport.PostgreSQLLeftoverRemoveResponse{}
	removePGLeftover(context.Background(), host, &transport.PostgreSQLLeftoverRemoveRequest{PostgreSQLClusterRef: transport.PostgreSQLClusterRef{Version: "16", Name: "main"}}, &resp)
	if resp.Removed || resp.ErrorCode != "NOT_A_LEFTOVER" {
		t.Fatalf("installed major = %+v", resp)
	}
}

func TestPGLeftoverWithDataOutsideTheStandardPlaceIsNotRemoved(t *testing.T) {
	f := newPGLeftoverFixture(t)
	c := f.cluster("16", "main", 5432, "down,binaries_missing")
	c.DataDirectory = filepath.Join(f.root, "srv", "pg16")
	host := pgClusterHost{
		family:     func() string { return "apt" },
		lsclusters: func(context.Context) (string, error) { return pgLsLine(c), nil },
		freeBytes:  func(string) (int64, error) { return 10 << 30, nil },
	}
	var resp transport.PostgreSQLLeftoverRemoveResponse
	removePGLeftover(context.Background(), host, &transport.PostgreSQLLeftoverRemoveRequest{PostgreSQLClusterRef: transport.PostgreSQLClusterRef{Version: "16", Name: "main"}}, &resp)
	if resp.Removed || resp.Changed || resp.ErrorCode != "DATA_OUTSIDE_STANDARD" {
		t.Fatalf("custom data directory = %+v", resp)
	}
	if _, err := os.Lstat(c.ConfigPath); err != nil {
		t.Fatal("its configuration was removed")
	}
}

func tarNames(t *testing.T, path string) []string {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	zipped, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	reader := tar.NewReader(zipped)
	var names []string
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, header.Name)
	}
	sort.Strings(names)
	return names
}

func containsSuffix(list []string, suffix string) bool {
	for _, item := range list {
		if strings.HasSuffix(item, suffix) {
			return true
		}
	}
	return false
}

// The removal of a PostgreSQL major purges (no dpkg "rc" state), names the
// major's own client, archives a cluster that holds more than initdb made
// after its units stopped, and removes the cluster's files after the
// packages went.
func TestPostgreSQLMajorRemovalPurgesAndTakesItsCluster(t *testing.T) {
	f := newPGLeftoverFixture(t)
	f.installMajor("16")
	cluster := f.cluster("16", "main", 5432, "online")
	f.write(filepath.Join(cluster.DataDirectory, "global", "1260"), string(pgAuthidPage(append(append([]pgRoleRow{}, pgInitdbRoles...), pgRoleRow{16384, "app"}))))
	var events []string
	var request packageRemovalRequest
	ops := serviceUninstallOps{
		detectPackageFamily: func() string { return "apt" },
		packageInstalled: func(pkg string) bool {
			return pkg == "postgresql-16" || pkg == "postgresql-client-16"
		},
		unitExists:    func(string) bool { return true },
		unitsMatching: func(string) []string { return nil },
		disableUnit: func(unit string) error {
			events = append(events, "stop "+unit)
			return nil
		},
		removePackages: func(_ string, req packageRemovalRequest, hooks removalHooks) (packageRemovalPlan, string, error) {
			request = req
			if err := hooks.runStop(); err != nil {
				return packageRemovalPlan{}, "", err
			}
			events = append(events, aptFinalRemovalArgsFor(req.Packages, req.KeepData)[0]+" "+strings.Join(req.Packages, " "))
			return packageRemovalPlan{Remove: req.Packages}, "", nil
		},
		postgresqlHolds: func(string) ([]string, []string) { return nil, nil },
		postgresqlClusters: func(string) ([]transport.PostgreSQLCluster, error) {
			return []transport.PostgreSQLCluster{cluster}, nil
		},
		preparePGClusterRemoval: func(c transport.PostgreSQLCluster) (string, error) {
			events = append(events, "prepare "+c.Version+"/"+c.Name)
			return preparePGClusterRemoval(c, func(string) (int64, error) { return 10 << 30, nil })
		},
		removePGClusterFiles: func(c transport.PostgreSQLCluster) ([]string, error) {
			events = append(events, "files "+c.Version+"/"+c.Name)
			return removePGLeftoverFiles(c)
		},
		discardPGArchive: func(path string) { _ = os.Remove(path) },
	}
	var response UninstallServiceResponse
	if err := (&Agent{}).uninstallServiceWithOps(&InstallServiceRequest{ID: "postgresql", Package: "postgresql-16"}, &response, ops); err != nil {
		t.Fatal(err)
	}
	if !response.Removed || response.Error != "" {
		t.Fatalf("response = %+v", response)
	}
	if request.KeepData || !reflect.DeepEqual(request.Packages, []string{"postgresql-16", "postgresql-client-16"}) {
		t.Fatalf("request = %+v", request)
	}
	wantEvents := []string{"stop postgresql@16-main", "prepare 16/main", "purge postgresql-16 postgresql-client-16", "files 16/main"}
	if !reflect.DeepEqual(events, wantEvents) {
		t.Fatalf("events = %v, want %v", events, wantEvents)
	}
	if !reflect.DeepEqual(response.PostgreSQLClustersRemoved, []string{"16/main"}) || len(response.PostgreSQLArchives) != 1 {
		t.Fatalf("clusters/archives = %v %v", response.PostgreSQLClustersRemoved, response.PostgreSQLArchives)
	}
	if _, err := os.Stat(response.PostgreSQLArchives[0]); err != nil {
		t.Fatalf("the role-holding cluster's archive: %v", err)
	}
	for _, gone := range []string{filepath.Join(pgConfRoot, "16"), filepath.Join(pgDataRoot, "16")} {
		if _, err := os.Lstat(gone); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s stayed", gone)
		}
	}
}

func TestPostgreSQLRemovalRefusedAfterStopDiscardsTheArchive(t *testing.T) {
	f := newPGLeftoverFixture(t)
	f.installMajor("16")
	cluster := f.cluster("16", "main", 5432, "online")
	f.write(filepath.Join(cluster.DataDirectory, "base", "5", "16400"), "a table")
	restored := 0
	ops := serviceUninstallOps{
		detectPackageFamily: func() string { return "apt" },
		packageInstalled:    func(pkg string) bool { return pkg == "postgresql-16" },
		unitExists:          func(string) bool { return true },
		unitsMatching:       func(string) []string { return nil },
		disableUnit:         func(string) error { return nil },
		unitState:           func(unit string) (unitRuntimeState, error) { return unitRuntimeState{ID: unit, Active: true}, nil },
		restoreUnit:         func(unitRuntimeState) error { restored++; return nil },
		removePackages: func(_ string, req packageRemovalRequest, hooks removalHooks) (packageRemovalPlan, string, error) {
			if err := hooks.runStop(); err != nil {
				return packageRemovalPlan{}, "", err
			}
			// The answer changed after the stop.
			_ = hooks.runRestart()
			return packageRemovalPlan{}, "", &packageRemovalRefusal{Code: errCodeRemovalPlanChanged, Message: "changed", AfterStop: true}
		},
		postgresqlHolds: func(string) ([]string, []string) { return nil, nil },
		postgresqlClusters: func(string) ([]transport.PostgreSQLCluster, error) {
			return []transport.PostgreSQLCluster{cluster}, nil
		},
		preparePGClusterRemoval: func(c transport.PostgreSQLCluster) (string, error) {
			return preparePGClusterRemoval(c, func(string) (int64, error) { return 10 << 30, nil })
		},
		removePGClusterFiles: func(transport.PostgreSQLCluster) ([]string, error) {
			t.Fatal("files removed after a refusal")
			return nil, nil
		},
		discardPGArchive: func(path string) { _ = os.Remove(path) },
	}
	var response UninstallServiceResponse
	if err := (&Agent{}).uninstallServiceWithOps(&InstallServiceRequest{ID: "postgresql", Package: "postgresql-16"}, &response, ops); err != nil {
		t.Fatal(err)
	}
	if response.Removed || response.ErrorCode != errCodeRemovalPlanChanged || restored != 1 {
		t.Fatalf("response = %+v restored=%d", response, restored)
	}
	if entries, _ := os.ReadDir(pgArchiveRoot); len(entries) != 0 {
		t.Fatalf("archive kept after a refusal: %v", entries)
	}
	if _, err := os.Lstat(cluster.DataDirectory); err != nil {
		t.Fatal("a refused removal touched the cluster")
	}
}

func TestPostgreSQLRemovalWithoutClusterStepsKeepsData(t *testing.T) {
	var request packageRemovalRequest
	ops := serviceUninstallOps{
		detectPackageFamily: func() string { return "apt" },
		packageInstalled:    func(pkg string) bool { return pkg == "postgresql-16" },
		unitExists:          func(string) bool { return false },
		unitsMatching:       func(string) []string { return nil },
		disableUnit:         func(string) error { return nil },
		removePackages: func(_ string, req packageRemovalRequest, _ removalHooks) (packageRemovalPlan, string, error) {
			request = req
			return packageRemovalPlan{Remove: req.Packages}, "", nil
		},
		postgresqlHolds: func(string) ([]string, []string) { return nil, nil },
	}
	var response UninstallServiceResponse
	if err := (&Agent{}).uninstallServiceWithOps(&InstallServiceRequest{ID: "postgresql", Package: "postgresql-16"}, &response, ops); err != nil {
		t.Fatal(err)
	}
	if !request.KeepData {
		t.Fatal("without the clusters' own steps the data must stay")
	}
}

// Command plans: PostgreSQL on Debian/Ubuntu purges; MariaDB still keeps its
// data; pacman always removes with -Rns.
func TestRemovalCommandPlansPurge(t *testing.T) {
	pg, maria := core.GetManagedServiceByID("postgresql"), core.GetManagedServiceByID("mariadb")
	if got := aptFinalRemovalArgsFor([]string{"postgresql-16"}, removalKeepsDataOn(pg, "apt")); got[0] != "purge" {
		t.Fatalf("postgresql on apt = %v", got)
	}
	if got := aptFinalRemovalArgsFor([]string{"mariadb-server"}, removalKeepsDataOn(maria, "apt")); got[0] != "remove" {
		t.Fatalf("mariadb on apt = %v", got)
	}
	if got := strings.Join(pacmanRemovalArgs([]string{"postgresql"}), " "); !strings.HasPrefix(got, "-Rns") {
		t.Fatalf("pacman = %v", got)
	}
	installed := func(name string) bool {
		return map[string]bool{"postgresql-client-16": true, "postgresql-client": true}[name]
	}
	if got := pgVersionClientPackages(pg, []string{"postgresql-16", "postgresql-18"}, installed); !reflect.DeepEqual(got, []string{"postgresql-client-16"}) {
		t.Fatalf("clients = %v", got)
	}
	if got := pgMovingPackages(pg, installed); !reflect.DeepEqual(got, []string{"postgresql-client"}) {
		t.Fatalf("moving = %v", got)
	}
	if got := pgVersionClientPackages(maria, []string{"postgresql-16"}, installed); got != nil {
		t.Fatalf("mariadb = %v", got)
	}
}
