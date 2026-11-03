package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.118 lab findings in the application catalog: Nextcloud's licence
// links point into other folders and at other links; MediaWiki's and phpBB's
// download servers refuse Go's default User-Agent; the setup helper folder
// was unreadable for the site's user (Joomla exit 255); Drupal's read-only
// folders stopped its removal half way.

func TestExtractCatalogArchiveFollowsLinksThatStayInsideTheRelease(t *testing.T) {
	scratch := t.TempDir()
	archive := filepath.Join(scratch, "release")
	writeTarGz(t, archive, []tarEntry{
		{name: "nextcloud/", dir: true},
		{name: "nextcloud/occ", body: "<?php"},
		{name: "nextcloud/version.php", body: "<?php"},
		{name: "nextcloud/index.php", body: "<?php"},
		{name: "nextcloud/lib/base.php", body: "<?php"},
		{name: "nextcloud/core/x.js", body: "x"},
		{name: "nextcloud/dist/vendor.js.license", body: "AGPL"},
		// Another folder of the tree, and a link that points at a link.
		{name: "nextcloud/apps/related/js/vendor.js.license", link: "../../../dist/vendor.js.license"},
		{name: "nextcloud/apps/related/js/vendor.js.map.license", link: "vendor.js.license"},
	})
	destination := t.TempDir()
	if err := extractCatalogArchive(nextcloudTestArchive(), archive, destination, scratch); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"apps/related/js/vendor.js.license", "apps/related/js/vendor.js.map.license"} {
		path := filepath.Join(destination, filepath.FromSlash(rel))
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() {
			t.Fatalf("%s was not written as a plain copy: %v %v", rel, info, err)
		}
		if content, _ := os.ReadFile(path); string(content) != "AGPL" {
			t.Fatalf("%s content = %q", rel, content)
		}
	}
}

func TestExtractCatalogArchiveRefusesLinksThatLeaveTheRelease(t *testing.T) {
	base := []tarEntry{
		{name: "nextcloud/occ", body: "x"}, {name: "nextcloud/version.php", body: "x"},
		{name: "nextcloud/index.php", body: "x"}, {name: "nextcloud/lib/a", body: "x"}, {name: "nextcloud/core/a", body: "x"},
	}
	cases := map[string][]tarEntry{
		"absolute":                {{name: "nextcloud/evil", link: "/etc/passwd"}},
		"climbs out":              {{name: "nextcloud/lib/evil", link: "../../outside"}},
		"climbs out of the root":  {{name: "nextcloud/evil", link: "../nextcloud/occ"}},
		"loop":                    {{name: "nextcloud/a", link: "b"}, {name: "nextcloud/b", link: "a"}},
		"chain to an escape":      {{name: "nextcloud/a", link: "b"}, {name: "nextcloud/b", link: "../../etc/passwd"}},
		"link to a folder":        {{name: "nextcloud/evil", link: "lib"}},
		"backslash":               {{name: "nextcloud/evil", link: "..\\..\\etc\\passwd"}},
		"through a linked folder": {{name: "nextcloud/d", link: "lib"}, {name: "nextcloud/evil", link: "d/a"}},
	}
	for name, extra := range cases {
		t.Run(name, func(t *testing.T) {
			scratch := t.TempDir()
			archive := filepath.Join(scratch, "release")
			writeTarGz(t, archive, append(append([]tarEntry{}, base...), extra...))
			destination := t.TempDir()
			if err := extractCatalogArchive(nextcloudTestArchive(), archive, destination, scratch); err == nil {
				t.Fatal("an unsafe link was accepted")
			}
		})
	}
}

func TestCatalogDownloadsSendAnHonestUserAgent(t *testing.T) {
	previous := buildVersion
	buildVersion = "v0.1.0-alpha.118"
	t.Cleanup(func() { buildVersion = previous })
	want := "CelikPanel/v0.1.0-alpha.118 (+https://celikpanel.net)"
	var seen []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Header.Get("User-Agent"))
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "/file", http.StatusFound)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()
	request, err := newGetRequest(context.Background(), server.URL+"/file")
	if err != nil {
		t.Fatal(err)
	}
	if got := request.Header.Get("User-Agent"); got != want {
		t.Fatalf("catalog request User-Agent = %q, want %q", got, want)
	}
	// The legacy WordPress download uses the client's Get: the client's
	// transport names CelikPanel too, also after a redirect.
	client := &http.Client{Transport: catalogUserAgentTransport{}}
	response, err := client.Get(server.URL + "/redirect")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if len(seen) != 2 || seen[0] != want || seen[1] != want {
		t.Fatalf("server saw User-Agents %q", seen)
	}
	if _, ok := wpHTTPClient(0).Transport.(catalogUserAgentTransport); !ok {
		t.Fatal("the download client does not name CelikPanel")
	}
	buildVersion = "dev build (local)"
	if got := catalogUserAgent(); got != "CelikPanel/dev (+https://celikpanel.net)" {
		t.Fatalf("an unusable version produced %q", got)
	}
}

func TestSetupHelperIsReadableByTheSiteUser(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX modes")
	}
	previous := setupPrependPath
	dir := filepath.Join(t.TempDir(), "celikpanel-app-setup")
	setupPrependPath = filepath.Join(dir, "prepend.php")
	t.Cleanup(func() { setupPrependPath = previous })
	// The agent's UMask=0027 made the folder 0750 root:root.
	if err := os.Mkdir(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	for round := 0; round < 2; round++ {
		if err := ensureSetupPrepend(); err != nil {
			t.Fatal(err)
		}
		dirInfo, err := os.Stat(dir)
		if err != nil || dirInfo.Mode().Perm() != 0o755 {
			t.Fatalf("round %d: helper folder mode %v (%v), want 0755", round, dirInfo.Mode().Perm(), err)
		}
		fileInfo, err := os.Stat(setupPrependPath)
		if err != nil || fileInfo.Mode().Perm() != 0o644 {
			t.Fatalf("round %d: helper file mode %v (%v), want 0644", round, fileInfo.Mode().Perm(), err)
		}
		_ = os.Chmod(setupPrependPath, 0o600)
		_ = os.Chmod(dir, 0o700)
	}
}

// The permission reset is a real command: it gives the owner rwx back on
// read-only folders and leaves files alone.
func TestRemovalPermissionResetOpensReadOnlyFolders(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("runs find")
	}
	root := t.TempDir()
	locked := filepath.Join(root, "sites", "default")
	if err := os.MkdirAll(locked, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(locked, "settings.php"), []byte("<?php"), 0o440); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0o550); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o750) })
	argv := catalogRemovalPermissionArgv(root)
	if output, err := exec.Command(argv[0], argv[1:]...).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, output)
	}
	info, err := os.Stat(locked)
	if err != nil || info.Mode().Perm()&0o700 != 0o700 {
		t.Fatalf("read-only folder still %v (%v)", info.Mode().Perm(), err)
	}
	if file, _ := os.Stat(filepath.Join(locked, "settings.php")); file.Mode().Perm() != 0o440 {
		t.Fatalf("a file's mode was changed: %v", file.Mode().Perm())
	}
}

func TestRemoveAgainAfterAPartialRemovalKeepsANewFileTheOwnerAdded(t *testing.T) {
	site := newCatalogTestSite(t, "8.3")
	rec := useRecordingIncludes(t)
	app := catalogTestApp(t, "drupal")
	writeMarkers(t, site.docRoot, app)
	if err := os.MkdirAll(filepath.Join(site.docRoot, "sites", "default"), 0o750); err != nil {
		t.Fatal(err)
	}
	var calls, deleted []string
	failDelete := true
	previous := catalogRunAsSiteUser
	t.Cleanup(func() { catalogRunAsSiteUser = previous })
	catalogRunAsSiteUser = func(_ context.Context, _ string, _ string, argv []string, stdin []byte) (string, error) {
		calls = append(calls, strings.Join(argv, " "))
		switch {
		case argv[0] == "find" && argv[len(argv)-1] == "-delete":
			deleted = append(deleted, argv[1:len(argv)-2]...)
			for _, target := range argv[1 : len(argv)-2] {
				if failDelete && filepath.Base(target) == "sites" {
					continue // the read-only folder that stays
				}
				_ = os.RemoveAll(target)
			}
			if failDelete {
				return "find: cannot delete " + site.docRoot + "/sites/default: Permission denied", errors.New("it ended with exit code 1")
			}
		case argv[0] == "sh" && strings.Contains(argv[2], "cat >"):
			target := argv[len(argv)-1]
			if strings.Contains(argv[2], "set -C") {
				if _, err := os.Lstat(target); err == nil {
					if strings.Contains(argv[2], "[ -e") {
						return "", nil
					}
					return "", errors.New("file exists")
				}
			}
			return "", os.WriteFile(target, stdin, 0o640)
		}
		return "", nil
	}

	var resp transport.RemoveCatalogAppResponse
	if err := (&Agent{}).RemoveCatalogAppV1(&transport.RemoveCatalogAppRequest{AppID: "drupal", Site: site.site}, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.ErrorKind != transport.AppFailRemove || resp.FilesRemoved || !strings.Contains(resp.Error, "Press Remove again") ||
		!strings.Contains(resp.Error, "Permission denied") {
		t.Fatalf("partial removal answer %+v", resp)
	}
	resetAt, deleteAt := -1, -1
	for i, call := range calls {
		if strings.Contains(call, "chmod u+rwx") && resetAt < 0 {
			resetAt = i
		}
		if strings.HasSuffix(call, "-delete") && deleteAt < 0 {
			deleteAt = i
		}
	}
	if resetAt < 0 || deleteAt < 0 || resetAt > deleteAt {
		t.Fatalf("the owner's write permission was not restored before deleting: %v", calls)
	}
	name, _ := celikPanelSitePlaceholder(catalogTestDomain, "php")
	if _, err := os.Stat(filepath.Join(site.docRoot, name)); err != nil {
		t.Fatal("the placeholder page was not put back beside what remains")
	}
	if len(rec.removed) != 0 {
		t.Fatalf("nginx rules were removed although the files remain: %v", rec.removed)
	}

	// The owner puts a file of their own into the folder meanwhile.
	ownerFile := filepath.Join(site.docRoot, "owner-upload.html")
	if err := os.WriteFile(ownerFile, []byte("mine"), 0o640); err != nil {
		t.Fatal(err)
	}

	// Drupal's own files are gone; the recorded removal continues anyway,
	// and deletes only the entries recorded when it started.
	failDelete = false
	deleted = nil
	resp = transport.RemoveCatalogAppResponse{}
	if err := (&Agent{}).RemoveCatalogAppV1(&transport.RemoveCatalogAppRequest{AppID: "drupal", Site: site.site}, &resp); err != nil || resp.Error != "" {
		t.Fatalf("the second removal: err=%v resp=%+v", err, resp)
	}
	if !resp.FilesRemoved || resp.NotThisApp {
		t.Fatalf("the second removal did not continue: %+v", resp)
	}
	for _, path := range deleted {
		if base := filepath.Base(path); base != "sites" {
			t.Fatalf("Remove again deleted %s, which was not left from the application: %v", base, deleted)
		}
	}
	if content, err := os.ReadFile(ownerFile); err != nil || string(content) != "mine" {
		t.Fatalf("the owner's new file did not survive Remove again: %q %v", content, err)
	}
	if len(resp.KeptEntries) != 1 || resp.KeptEntries[0] != "owner-upload.html" || !strings.Contains(resp.Detail, "owner-upload.html") {
		t.Fatalf("the kept item is not named: %+v", resp)
	}
	if _, err := os.Stat(filepath.Join(site.docRoot, name)); err != nil {
		t.Fatal("the placeholder page is gone")
	}
	if _, err := os.Stat(filepath.Join(site.docRoot, "sites")); err == nil {
		t.Fatal("the application's remaining folder was not removed")
	}
	if entries, _ := os.ReadDir(catalogRemovalStateDir); len(entries) != 0 {
		t.Fatalf("the removal record was kept: %v", entries)
	}
	if len(rec.removed) != 2 {
		t.Fatalf("the application's nginx rules were not removed: %v", rec.removed)
	}
	// Without a record, a document root without Drupal's files stays refused.
	resp = transport.RemoveCatalogAppResponse{}
	_ = (&Agent{}).RemoveCatalogAppV1(&transport.RemoveCatalogAppRequest{AppID: "drupal", Site: site.site}, &resp)
	if !resp.NotThisApp {
		t.Fatalf("the owner's content was not protected: %+v", resp)
	}
}
