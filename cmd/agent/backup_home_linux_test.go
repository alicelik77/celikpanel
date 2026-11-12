//go:build linux

package main

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/backupspec"
)

// docs/SITE-BACKUP-CONTENTS-DESIGN.md: a site backup holds the site-home
// areas (a program's app/, service apps' apps/<app>/), leaves out only what
// its rules name, and says so; restore puts the program back with the site's
// owner and modes; older packages restore as before; an older reader refuses
// a version-3 package.

const testSiteUID = 4321

// writeTree writes files (path -> content) below root with mode 0o640, and
// folders with 0o750.
func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		full := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o640); err != nil {
			t.Fatal(err)
		}
	}
}

// chownTree gives root and everything below it uid:uid, links included.
func chownTree(t *testing.T, root string, uid int) {
	t.Helper()
	err := filepath.Walk(root, func(p string, _ os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		return os.Lchown(p, uid, uid)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// homeEntries lists the entries of a package's home payload: name -> kind
// ("d", "f:<content>", "l:<target>").
func homeEntries(t *testing.T, backupPath string) map[string]string {
	t.Helper()
	dir := t.TempDir()
	manifest, err := unpackBackupPackage(backupPath, dir, testScope())
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Home == nil {
		return nil
	}
	file, err := os.Open(filepath.Join(dir, filepath.FromSlash(manifest.Home.Name)))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(gz)
	out := map[string]string{}
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return out
		}
		if err != nil {
			t.Fatal(err)
		}
		switch header.Typeflag {
		case tar.TypeDir:
			out[header.Name] = "d"
		case tar.TypeReg:
			data, _ := io.ReadAll(tr)
			out[header.Name] = "f:" + string(data)
		case tar.TypeSymlink:
			out[header.Name] = "l:" + header.Linkname
		default:
			out[header.Name] = "?"
		}
	}
}

func leftOutReasons(items []backupspec.LeftOut) map[string]string {
	out := map[string]string{}
	for _, item := range items {
		out[item.Path] = item.Reason
	}
	return out
}

func TestSiteWithoutAreasKeepsTheVersionTwoPackage(t *testing.T) {
	_, docroot := installBackupTestPaths(t)
	writeTree(t, docroot, map[string]string{"index.html": "hello"})
	info, err := (&Agent{}).createBackup(testCreateRequest(backupspec.TypeFiles))
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := readBackupManifest(info.Path)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Version != backupspec.ManifestVersionSite || manifest.Home != nil || info.Format != 2 || len(info.Areas) != 0 {
		t.Fatalf("a site without areas must keep the version-2 package: %#v %#v", manifest, info)
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"home"`, `"areas"`, `"left_out"`} {
		if strings.Contains(string(raw), key) {
			t.Fatalf("version-2 manifest carries %s: %s", key, raw)
		}
	}
}

// The file set per site kind: what is kept, what is left out and why.
func TestSiteBackupFileSetPerSiteKind(t *testing.T) {
	cases := []struct {
		name     string
		files    map[string]string
		links    map[string]string
		areas    []string
		kept     []string
		leftOut  map[string]string
		notKept  []string
		linkKept map[string]string
	}{
		{
			name: "program",
			files: map[string]string{
				"app/bin/server": "ELF", "app/bin/server.v1": "ELF1",
				"app/config/app.toml": "port=1", "app/data/note.txt": "note",
			},
			areas: []string{"app"},
			kept:  []string{"app/bin/server", "app/bin/server.v1", "app/config/app.toml", "app/data/note.txt"},
		},
		{
			name: "python",
			files: map[string]string{
				"app/src/main.py": "app", "app/src/requirements.txt": "flask",
				"app/venv-3/lib/site.py": "x", "app/.pip-cache/http/blob": "12345",
				"app/.build-tmp/t": "1", "app/venv-4.partial/x": "123", "app/data/db": "rows",
			},
			links:    map[string]string{"app/venv-3/bin/python": "/usr/bin/python3"},
			areas:    []string{"app"},
			kept:     []string{"app/src/main.py", "app/venv-3/lib/site.py", "app/data/db"},
			leftOut:  map[string]string{"app/.pip-cache": "cache", "app/.build-tmp": "build_temp", "app/venv-4.partial": "build_temp"},
			notKept:  []string{"app/.pip-cache/http/blob", "app/venv-4.partial/x"},
			linkKept: map[string]string{"app/venv-3/bin/python": "/usr/bin/python3"},
		},
		{
			name: "service app",
			files: map[string]string{
				"apps/n8n/data/database.sqlite": "rows", "apps/n8n/config/c": "c",
				"apps/n8n/dist/n8n.tgz": "tgz", "apps/n8n/app-1.2.3/index.js": "js", "apps/n8n/.cache/x": "cc",
			},
			areas:   []string{"apps/n8n"},
			kept:    []string{"apps/n8n/data/database.sqlite", "apps/n8n/config/c"},
			leftOut: map[string]string{"apps/n8n/dist": "reinstalled", "apps/n8n/app-1.2.3": "reinstalled", "apps/n8n/.cache": "cache"},
			notKept: []string{"apps/n8n/dist/n8n.tgz", "apps/n8n/app-1.2.3/index.js"},
		},
		{
			name:  "program and service app",
			files: map[string]string{"app/bin/server": "ELF", "apps/open-webui/data/webui.db": "w"},
			areas: []string{"app", "apps/open-webui"},
			kept:  []string{"app/bin/server", "apps/open-webui/data/webui.db"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, docroot := installBackupTestPaths(t)
			home := filepath.Dir(docroot)
			writeTree(t, docroot, map[string]string{"index.html": "site"})
			writeTree(t, home, tc.files)
			for name, target := range tc.links {
				full := filepath.Join(home, filepath.FromSlash(name))
				if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, full); err != nil {
					t.Fatal(err)
				}
			}
			info, err := (&Agent{}).createBackup(testCreateRequest(backupspec.TypeFiles))
			if err != nil {
				t.Fatal(err)
			}
			if info.Format != backupspec.ManifestVersionHome {
				t.Fatalf("format = %d, want 3", info.Format)
			}
			var names []string
			for _, area := range info.Areas {
				names = append(names, area.Name)
			}
			if !reflect.DeepEqual(names, tc.areas) {
				t.Fatalf("areas = %v, want %v", names, tc.areas)
			}
			entries := homeEntries(t, info.Path)
			for _, kept := range tc.kept {
				if !strings.HasPrefix(entries[kept], "f:") {
					t.Fatalf("%s is not in the backup: %v", kept, entries)
				}
			}
			for _, gone := range tc.notKept {
				if _, ok := entries[gone]; ok {
					t.Fatalf("%s should be left out", gone)
				}
			}
			for name, target := range tc.linkKept {
				if entries[name] != "l:"+target {
					t.Fatalf("link %s = %q, want the link itself", name, entries[name])
				}
			}
			if got := leftOutReasons(info.LeftOut); len(tc.leftOut) > 0 && !reflect.DeepEqual(got, tc.leftOut) {
				t.Fatalf("left out = %v, want %v", got, tc.leftOut)
			}
			for _, item := range info.LeftOut {
				if item.Reason == backupspec.LeftOutCache && item.Bytes == 0 {
					t.Fatalf("a left-out cache names its size: %#v", item)
				}
			}
		})
	}
}

// Sockets, FIFOs and another account's files are left out and named; the
// site's own hard-linked files are kept.
func TestSpecialAndForeignFilesAreLeftOutAndNamed(t *testing.T) {
	_, docroot := installBackupTestPaths(t)
	home := filepath.Dir(docroot)
	writeTree(t, home, map[string]string{"app/bin/server": "ELF", "app/data/secret": "root-only"})
	if err := syscall.Mkfifo(filepath.Join(home, "app", "data", "pipe"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(filepath.Join(home, "app", "bin", "server"), filepath.Join(home, "app", "bin", "server.hard")); err != nil {
		t.Fatal(err)
	}
	chownTree(t, home, testSiteUID)
	// The server owner put a root file into the program's folder.
	if err := os.Lchown(filepath.Join(home, "app", "data", "secret"), 0, 0); err != nil {
		t.Fatal(err)
	}
	info, err := (&Agent{}).createBackup(testCreateRequest(backupspec.TypeFiles))
	if err != nil {
		t.Fatal(err)
	}
	reasons := leftOutReasons(info.LeftOut)
	if reasons["app/data/pipe"] != backupspec.LeftOutSpecial || reasons["app/data/secret"] != backupspec.LeftOutNotSiteOwned {
		t.Fatalf("left out = %v", reasons)
	}
	entries := homeEntries(t, info.Path)
	if _, ok := entries["app/data/secret"]; ok {
		t.Fatal("another account's file reached the archive")
	}
	if entries["app/bin/server"] != "f:ELF" || entries["app/bin/server.hard"] != "f:ELF" {
		t.Fatalf("the site's own hard-linked files are kept: %v", entries)
	}
}

// The measurement before a backup uses the archive's walk: its numbers are
// the archived ones.
func TestContentsMeasureEqualsTheArchive(t *testing.T) {
	_, docroot := installBackupTestPaths(t)
	home := filepath.Dir(docroot)
	writeTree(t, docroot, map[string]string{"index.html": "0123456789", "a/b.css": "xx"})
	writeTree(t, home, map[string]string{
		"app/bin/server": strings.Repeat("e", 5000), "app/data/d": "data", "app/.pip-cache/x": "123",
		"apps/n8n/data/db": "n8n",
	})
	contents := &backupspec.ContentsResponse{}
	if err := (&Agent{}).BackupContentsV1(&backupspec.ListRequest{ProtocolVersion: 2, SubscriptionID: 7, DomainID: 9, DomainName: "example.test"}, contents); err != nil {
		t.Fatal(err)
	}
	if !contents.Complete || contents.Error != "" || contents.SiteBytes != 12 || contents.SiteFiles != 2 {
		t.Fatalf("contents = %#v", contents)
	}
	if contents.BackupFreeBytes <= 0 {
		t.Fatalf("free space on the backup disk not read: %d", contents.BackupFreeBytes)
	}
	if contents.LeftOutBytes != 3 || len(contents.LeftOut) != 1 {
		t.Fatalf("left out = %#v (%d bytes)", contents.LeftOut, contents.LeftOutBytes)
	}
	info, err := (&Agent{}).createBackup(testCreateRequest(backupspec.TypeFiles))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(contents.Areas, info.Areas) {
		t.Fatalf("measured %#v, archived %#v", contents.Areas, info.Areas)
	}
	if contents.Areas[0].Bytes != 5004 || contents.Areas[0].Files != 2 {
		t.Fatalf("program area = %#v", contents.Areas[0])
	}
}

func TestContentsMeasureStopsAtItsTimeLimit(t *testing.T) {
	_, docroot := installBackupTestPaths(t)
	files := map[string]string{}
	for i := 0; i < 600; i++ {
		files["app/data/f"+strings.Repeat("x", i%7)+string(rune('a'+i%26))+time.Duration(i).String()] = "1"
	}
	writeTree(t, filepath.Dir(docroot), files)
	contents := measureSiteBackup(docroot, filepath.Dir(docroot), -time.Second)
	if contents.Complete || contents.Error != "" {
		t.Fatalf("a measurement past its limit says it is incomplete: %#v", contents)
	}
}

func TestManifestVersionsBothWays(t *testing.T) {
	scope := testScope()
	hash := strings.Repeat("a", 64)
	base := backupManifest{
		Version: 2, Type: backupspec.TypeFiles, Origin: backupspec.OriginManual,
		SubscriptionID: scope.SubscriptionID, DomainID: scope.DomainID, CreatedAt: time.Unix(1, 0),
		Files: manifestPayload{Name: filesPayloadName, Size: 1, SHA256: hash},
	}
	home := &manifestPayload{Name: homePayloadName, Size: 1, SHA256: hash}
	v3 := base
	v3.Version, v3.Home = 3, home
	v3.Areas = []backupspec.Area{{Name: "app", Bytes: 1, Files: 1}, {Name: "apps/n8n"}}
	v3.LeftOut = []backupspec.LeftOut{{Path: "app/.pip-cache", Reason: "cache", Bytes: 9}, {Path: "apps/x", Reason: "not_a_folder"}}
	mutate := func(change func(*backupManifest)) backupManifest {
		m := v3
		m.Areas = append([]backupspec.Area(nil), v3.Areas...)
		m.LeftOut = append([]backupspec.LeftOut(nil), v3.LeftOut...)
		change(&m)
		return m
	}
	cases := []struct {
		name string
		m    backupManifest
		ok   bool
	}{
		{"version 2", base, true},
		{"version 3", v3, true},
		{"version 2 naming a home payload", func() backupManifest { m := base; m.Home = home; return m }(), false},
		{"version 3 without home payload", mutate(func(m *backupManifest) { m.Home = nil }), false},
		{"version 3 database backup", mutate(func(m *backupManifest) {
			m.Type, m.Files = backupspec.TypeDatabase, manifestPayload{}
			m.Databases = []manifestDatabase{{Identity: backupspec.DatabaseIdentity{ID: 1, Name: "db", Type: "mysql"}, Payload: manifestPayload{Name: databasePayloadName(1), Size: 1, SHA256: hash}}}
		}), false},
		{"area escaping the site", mutate(func(m *backupManifest) { m.Areas[1].Name = "apps/../etc" }), false},
		{"unknown area", mutate(func(m *backupManifest) { m.Areas[1].Name = "private" }), false},
		{"areas out of order", mutate(func(m *backupManifest) { m.Areas[0], m.Areas[1] = m.Areas[1], m.Areas[0] }), false},
		{"left out outside the areas", mutate(func(m *backupManifest) { m.LeftOut[0].Path = "public_html/x" }), false},
		{"unknown reason", mutate(func(m *backupManifest) { m.LeftOut[0].Reason = "too_big" }), false},
		{"unknown version", mutate(func(m *backupManifest) { m.Version = 4 }), false},
	}
	for _, tc := range cases {
		err := validateManifest(tc.m, scope)
		if (err == nil) != tc.ok {
			t.Fatalf("%s: err = %v, want ok=%v", tc.name, err, tc.ok)
		}
	}
	// An older release's reader (alpha.132 and before) accepted only the
	// protocol version and refused every payload it did not expect: it
	// refuses a version-3 package before anything is written.
	olderReader := func(m backupManifest) error {
		if m.Version != backupspec.ProtocolVersion {
			return errors.New("manifest version mismatch")
		}
		return nil
	}
	if olderReader(v3) == nil || olderReader(base) != nil {
		t.Fatal("an older reader must refuse version 3 and accept version 2")
	}
}

// restoreRequest is a restore of name on the test scope.
func restoreRequest(name string) *backupspec.RestoreRequest {
	return &backupspec.RestoreRequest{ProtocolVersion: 2, SubscriptionID: 7, DomainID: 9, DomainName: "example.test", BackupName: name}
}

func TestProgramRestoreBringsBackFilesOwnerModesAndLinks(t *testing.T) {
	_, docroot := installBackupTestPaths(t)
	home := filepath.Dir(docroot)
	writeTree(t, docroot, map[string]string{"index.html": "site"})
	writeTree(t, home, map[string]string{"app/bin/server": "ELF", "app/data/note.txt": "note", "app/config/c": "c"})
	if err := os.Chmod(filepath.Join(home, "app", "bin", "server"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("server", filepath.Join(home, "app", "bin", "current")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/etc", filepath.Join(home, "app", "data", "outside")); err != nil {
		t.Fatal(err)
	}
	chownTree(t, home, testSiteUID)
	agent := &Agent{}
	info, err := agent.createBackup(testCreateRequest(backupspec.TypeFiles))
	if err != nil {
		t.Fatal(err)
	}
	// The disaster: the program and its data are gone, something else came.
	for _, gone := range []string{"app/bin/server", "app/data/note.txt"} {
		if err := os.Remove(filepath.Join(home, filepath.FromSlash(gone))); err != nil {
			t.Fatal(err)
		}
	}
	writeTree(t, home, map[string]string{"app/data/stale": "stale"})
	resp := &backupspec.RestoreResponse{}
	if err := agent.restoreBackup(restoreRequest(info.Name), resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Success || !reflect.DeepEqual(resp.Restored, []string{"app"}) || len(resp.NotRestored) != 0 {
		t.Fatalf("restore answer = %#v", resp)
	}
	if data, err := os.ReadFile(filepath.Join(home, "app", "data", "note.txt")); err != nil || string(data) != "note" {
		t.Fatalf("data not restored: %q %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(home, "app", "data", "stale")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the restored folder is the backup's, not a merge: %v", err)
	}
	server, err := os.Lstat(filepath.Join(home, "app", "bin", "server"))
	if err != nil || server.Mode().Perm() != 0o755 {
		t.Fatalf("program mode = %v, %v", server.Mode(), err)
	}
	for _, name := range []string{"app", "app/bin", "app/bin/server", "app/bin/current", "app/data/outside"} {
		st, err := os.Lstat(filepath.Join(home, filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}
		if sys := st.Sys().(*syscall.Stat_t); sys.Uid != testSiteUID || sys.Gid != testSiteUID {
			t.Fatalf("%s owner %d:%d, want the site's account", name, sys.Uid, sys.Gid)
		}
	}
	if target, err := os.Readlink(filepath.Join(home, "app", "data", "outside")); err != nil || target != "/etc" {
		t.Fatalf("a link comes back as the same link: %q %v", target, err)
	}
	// The link was made, never followed: /etc kept its owner.
	if st, err := os.Stat("/etc"); err != nil || st.Sys().(*syscall.Stat_t).Uid != 0 {
		t.Fatalf("a restored link was followed: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(docroot, "index.html")); err != nil || string(data) != "site" {
		t.Fatalf("public_html not restored: %q %v", data, err)
	}
	leftovers, _ := filepath.Glob(filepath.Join(home, ".restore-*"))
	if len(leftovers) != 0 {
		t.Fatalf("staging left behind: %v", leftovers)
	}
}

func TestProgramRestoreMakesAMissingProgramFolder(t *testing.T) {
	_, docroot := installBackupTestPaths(t)
	home := filepath.Dir(docroot)
	writeTree(t, home, map[string]string{"app/bin/server": "ELF"})
	if err := os.Chmod(filepath.Join(home, "app"), 0o751); err != nil {
		t.Fatal(err)
	}
	chownTree(t, home, testSiteUID)
	agent := &Agent{}
	info, err := agent.createBackup(testCreateRequest(backupspec.TypeFiles))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(home, "app")); err != nil {
		t.Fatal(err)
	}
	resp := &backupspec.RestoreResponse{}
	if err := agent.restoreBackup(restoreRequest(info.Name), resp); err != nil {
		t.Fatal(err)
	}
	st, err := os.Lstat(filepath.Join(home, "app"))
	if err != nil || st.Mode().Perm() != 0o751 || st.Sys().(*syscall.Stat_t).Uid != testSiteUID {
		t.Fatalf("made folder: %v %v", st, err)
	}
	if data, err := os.ReadFile(filepath.Join(home, "app", "bin", "server")); err != nil || string(data) != "ELF" {
		t.Fatalf("program not restored: %q %v", data, err)
	}
}

// A backup made before this change (or before the program existed) restores
// exactly as before: public_html only; the program is not touched.
func TestOlderPackageRestoreLeavesTheProgramAlone(t *testing.T) {
	_, docroot := installBackupTestPaths(t)
	home := filepath.Dir(docroot)
	writeTree(t, docroot, map[string]string{"index.html": "old"})
	agent := &Agent{}
	old, err := agent.createBackup(testCreateRequest(backupspec.TypeFiles))
	if err != nil {
		t.Fatal(err)
	}
	if old.Format != backupspec.ManifestVersionSite {
		t.Fatalf("format = %d", old.Format)
	}
	writeTree(t, home, map[string]string{"app/bin/server": "ELF-now"})
	writeTree(t, docroot, map[string]string{"index.html": "new"})
	resp := &backupspec.RestoreResponse{}
	if err := agent.restoreBackup(restoreRequest(old.Name), resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Restored) != 0 || len(resp.NotRestored) != 0 {
		t.Fatalf("an older package names no area: %#v", resp)
	}
	if data, _ := os.ReadFile(filepath.Join(home, "app", "bin", "server")); string(data) != "ELF-now" {
		t.Fatalf("an older package changed the program: %q", data)
	}
	if data, _ := os.ReadFile(filepath.Join(docroot, "index.html")); string(data) != "old" {
		t.Fatalf("public_html not restored: %q", data)
	}
	// The safety copy taken before that restore holds the program.
	if resp.SafetyBackup == nil || resp.SafetyBackup.Format != backupspec.ManifestVersionHome {
		t.Fatalf("safety backup does not hold the program: %#v", resp.SafetyBackup)
	}
}

func TestServiceAppAreaIsBackedUpButNotRestored(t *testing.T) {
	_, docroot := installBackupTestPaths(t)
	home := filepath.Dir(docroot)
	writeTree(t, home, map[string]string{"apps/n8n/data/db": "backed-up", "app/bin/server": "ELF"})
	agent := &Agent{}
	info, err := agent.createBackup(testCreateRequest(backupspec.TypeFiles))
	if err != nil {
		t.Fatal(err)
	}
	writeTree(t, home, map[string]string{"apps/n8n/data/db": "live"})
	resp := &backupspec.RestoreResponse{}
	if err := agent.restoreBackup(restoreRequest(info.Name), resp); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(resp.Restored, []string{"app"}) || !reflect.DeepEqual(resp.NotRestored, []string{"apps/n8n"}) {
		t.Fatalf("answer = %#v", resp)
	}
	if data, _ := os.ReadFile(filepath.Join(home, "apps", "n8n", "data", "db")); string(data) != "live" {
		t.Fatalf("a service app's folder was changed: %q", data)
	}
}

// buildCraftedHomePackage publishes a version-3 package whose home payload
// holds the given tar entries.
func buildCraftedHomePackage(t *testing.T, backupRoot string, entries []tar.Header, contents map[string]string) string {
	t.Helper()
	scope := testScope()
	dir, err := ensureBackupDir(scope)
	if err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	if err := os.MkdirAll(filepath.Join(work, "payload"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(work, "site"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := safeCreateFilesArchive(filepath.Join(work, "site"), filepath.Join(work, filepath.FromSlash(filesPayloadName))); err != nil {
		t.Fatal(err)
	}
	homePath := filepath.Join(work, filepath.FromSlash(homePayloadName))
	out, err := os.Create(homePath)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(out)
	tw := tar.NewWriter(gz)
	for _, header := range entries {
		header := header
		data := contents[header.Name]
		header.Size = int64(len(data))
		if header.Typeflag != tar.TypeReg {
			header.Size = 0
		}
		if err := tw.WriteHeader(&header); err != nil {
			t.Fatal(err)
		}
		if header.Typeflag == tar.TypeReg {
			if _, err := tw.Write([]byte(data)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
	files, err := describePayload(filepath.Join(work, filepath.FromSlash(filesPayloadName)), filesPayloadName)
	if err != nil {
		t.Fatal(err)
	}
	homePayload, err := describePayload(homePath, homePayloadName)
	if err != nil {
		t.Fatal(err)
	}
	manifest := backupManifest{
		Version: 3, Type: backupspec.TypeFiles, Origin: backupspec.OriginManual,
		SubscriptionID: scope.SubscriptionID, DomainID: scope.DomainID, CreatedAt: time.Now().UTC(),
		Files: files, Home: &homePayload, Areas: []backupspec.Area{{Name: "app"}},
	}
	name, err := newBackupName(backupspec.TypeFiles, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := publishBackupPackage(filepath.Join(dir, name), work, manifest); err != nil {
		t.Fatal(err)
	}
	return name
}

// A crafted package cannot write outside the program's folder: a name with
// "..", a write through a link it made, a hard link or a device all stop the
// restore before anything is replaced.
func TestCraftedProgramPackageNeverEscapes(t *testing.T) {
	dirEntry := func(name string) tar.Header { return tar.Header{Name: name, Typeflag: tar.TypeDir, Mode: 0o755} }
	fileEntry := func(name string) tar.Header { return tar.Header{Name: name, Typeflag: tar.TypeReg, Mode: 0o644} }
	cases := map[string][]tar.Header{
		"dot-dot name": {dirEntry("app"), fileEntry("app/../escaped")},
		"write through a link": {dirEntry("app"), {Name: "app/l", Typeflag: tar.TypeSymlink, Linkname: "/tmp"},
			fileEntry("app/l/escaped")},
		"hard link":   {dirEntry("app"), {Name: "app/h", Typeflag: tar.TypeLink, Linkname: "/etc/passwd"}},
		"device":      {dirEntry("app"), {Name: "app/dev", Typeflag: tar.TypeChar, Devmajor: 1, Devminor: 3}},
		"absolute":    {dirEntry("app"), fileEntry("/app/escaped")},
		"duplicate":   {dirEntry("app"), fileEntry("app/x"), fileEntry("app/x")},
		"empty link":  {dirEntry("app"), {Name: "app/l", Typeflag: tar.TypeSymlink, Linkname: ""}},
		"link to dir": {dirEntry("app"), {Name: "app/d", Typeflag: tar.TypeSymlink, Linkname: "../.."}, dirEntry("app/d/x")},
	}
	names := make([]string, 0, len(cases))
	for name := range cases {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			backupRoot, docroot := installBackupTestPaths(t)
			home := filepath.Dir(docroot)
			writeTree(t, home, map[string]string{"app/bin/server": "live"})
			backup := buildCraftedHomePackage(t, backupRoot, cases[name], map[string]string{"app/../escaped": "x", "app/l/escaped": "x", "/app/escaped": "x", "app/x": "x"})
			resp := &backupspec.RestoreResponse{}
			if err := (&Agent{}).restoreBackup(restoreRequest(backup), resp); err == nil {
				t.Fatal("a crafted package was restored")
			}
			if data, _ := os.ReadFile(filepath.Join(home, "app", "bin", "server")); string(data) != "live" {
				t.Fatalf("the program folder was replaced: %q", data)
			}
			for _, escaped := range []string{filepath.Join(home, "escaped"), "/tmp/escaped", filepath.Join(filepath.Dir(home), "x")} {
				if _, err := os.Lstat(escaped); err == nil {
					t.Fatalf("%s was written", escaped)
				}
			}
			leftovers, _ := filepath.Glob(filepath.Join(home, ".restore-*"))
			if len(leftovers) != 0 {
				t.Fatalf("staging left behind: %v", leftovers)
			}
		})
	}
}

// A full restore whose database fails puts the site files and the program
// back as they were.
func TestFullRestoreDatabaseFailureLeavesTheProgramAsItWas(t *testing.T) {
	_, docroot := installBackupTestPaths(t)
	home := filepath.Dir(docroot)
	database := backupspec.DatabaseIdentity{ID: 1, Name: "tenant_one", Type: "mysql"}
	state := map[int]string{1: "target"}
	fail := true
	installDatabaseState(t, state, func(_ backupspec.DatabaseIdentity, value string) error {
		if fail && value == "target" {
			fail = false
			return errors.New("injected database failure")
		}
		return nil
	}, nil)
	writeTree(t, home, map[string]string{"app/data/d": "target"})
	agent := &Agent{}
	req := testCreateRequest(backupspec.TypeFull)
	req.Databases = []backupspec.DatabaseIdentity{database}
	info, err := agent.createBackup(req)
	if err != nil {
		t.Fatal(err)
	}
	state[1] = "live"
	writeTree(t, home, map[string]string{"app/data/d": "live"})
	restore := restoreRequest(info.Name)
	restore.Databases = []backupspec.DatabaseIdentity{database}
	resp := &backupspec.RestoreResponse{}
	err = agent.restoreBackup(restore, resp)
	if err == nil {
		t.Fatal("restore unexpectedly succeeded")
	}
	if code, _ := restoreFailureOf(err); code != backupspec.RestoreCodeLoadReverted {
		t.Fatalf("code = %q (%v)", code, err)
	}
	if data, _ := os.ReadFile(filepath.Join(home, "app", "data", "d")); string(data) != "live" || state[1] != "live" {
		t.Fatalf("program %q, database %q: not as they were", data, state[1])
	}
	if len(resp.Restored) != 0 {
		t.Fatalf("a failed restore names nothing restored: %#v", resp.Restored)
	}
	leftovers, _ := filepath.Glob(filepath.Join(home, ".restore-*"))
	if len(leftovers) != 0 {
		t.Fatalf("staging left behind: %v", leftovers)
	}
}

// The program's folder goes back to the site's account even when someone
// else owns it now (the sandbox refuses a folder of another account).
func TestProgramRestoreGivesTheFolderToTheSiteAccount(t *testing.T) {
	_, docroot := installBackupTestPaths(t)
	home := filepath.Dir(docroot)
	writeTree(t, home, map[string]string{"app/bin/server": "ELF"})
	chownTree(t, home, testSiteUID)
	agent := &Agent{}
	info, err := agent.createBackup(testCreateRequest(backupspec.TypeFiles))
	if err != nil {
		t.Fatal(err)
	}
	chownTree(t, filepath.Join(home, "app"), 0)
	if err := agent.restoreBackup(restoreRequest(info.Name), &backupspec.RestoreResponse{}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"app", "app/bin/server"} {
		st, err := os.Lstat(filepath.Join(home, filepath.FromSlash(name)))
		if err != nil || st.Sys().(*syscall.Stat_t).Uid != testSiteUID {
			t.Fatalf("%s is not the site account's: %v", name, err)
		}
	}
}

// A version-3 package that comes back from a remote copy is verified whole,
// home payload included.
func TestRemoteImportVerifiesTheHomePayload(t *testing.T) {
	_, docroot := installBackupTestPaths(t)
	writeTree(t, filepath.Dir(docroot), map[string]string{"app/bin/server": "ELF"})
	info, err := (&Agent{}).createBackup(testCreateRequest(backupspec.TypeFiles))
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := verifyBackupPackage(info.Path, testScope())
	if err != nil || manifest.Home == nil {
		t.Fatalf("verify = %#v, %v", manifest, err)
	}
}
