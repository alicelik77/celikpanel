package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/appcatalog"
	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

const catalogTestDomain = "shop.example.test"

func catalogTestApp(t *testing.T, id string) appcatalog.App {
	t.Helper()
	app, ok := appcatalog.Lookup(id)
	if !ok {
		t.Fatalf("no %s in the catalog", id)
	}
	return app
}

// A download that is not the pinned file is deleted and refused as a
// verification failure; nothing else is touched.
func TestFetchVerifiedReleaseRefusesChecksumMismatch(t *testing.T) {
	previous := catalogDownload
	t.Cleanup(func() { catalogDownload = previous })
	body := []byte("a release that was changed on the download server")
	catalogDownload = func(_ context.Context, url, destination string, _ int64) error {
		if !strings.HasPrefix(url, "https://download.nextcloud.com/") {
			t.Fatalf("the pinned address was not used: %s", url)
		}
		return os.WriteFile(destination, body, 0o600)
	}
	dir := t.TempDir()
	app := catalogTestApp(t, "nextcloud")
	_, kind, err := fetchVerifiedRelease(app, dir)
	if err == nil || kind != transport.AppFailVerification || !strings.Contains(err.Error(), "not the file CelikPanel pinned") {
		t.Fatalf("kind=%q err=%v", kind, err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("the refused download was kept: %v", entries)
	}

	sum := sha256.Sum256(body)
	app.Archive.SHA256 = hex.EncodeToString(sum[:])
	path, kind, err := fetchVerifiedRelease(app, dir)
	if err != nil || kind != "" || path == "" {
		t.Fatalf("a matching file was refused: kind=%q err=%v", kind, err)
	}
	if err := verifyPinnedSHA256(path, ""); err == nil {
		t.Fatal("a catalog entry without a digest must never verify")
	}
}

type tarEntry struct {
	name, body, link string
	dir              bool
}

func writeTarGz(t *testing.T, path string, entries []tarEntry) {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		header := &tar.Header{Name: e.name, Mode: 0o644, Size: int64(len(e.body)), Typeflag: tar.TypeReg}
		switch {
		case e.dir:
			header.Typeflag, header.Size, header.Mode = tar.TypeDir, 0, 0o755
		case e.link != "":
			header.Typeflag, header.Size, header.Linkname = tar.TypeSymlink, 0, e.link
		}
		if err := tw.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if header.Typeflag == tar.TypeReg {
			if _, err := tw.Write([]byte(e.body)); err != nil {
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
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}

func nextcloudTestArchive() appcatalog.App {
	app, _ := appcatalog.Lookup("nextcloud")
	app.Archive.Format = appcatalog.FormatTarGz
	return app
}

func TestExtractCatalogArchiveStripsTheTopFolderAndCopiesSiblingLinks(t *testing.T) {
	scratch := t.TempDir()
	archive := filepath.Join(scratch, "release")
	writeTarGz(t, archive, []tarEntry{
		{name: "nextcloud/", dir: true},
		{name: "nextcloud/occ", body: "<?php // occ"},
		{name: "nextcloud/version.php", body: "<?php"},
		{name: "nextcloud/index.php", body: "<?php"},
		{name: "nextcloud/lib/base.php", body: "<?php"},
		{name: "nextcloud/core/x.js", body: "x"},
		{name: "nextcloud/dist/a.js.map.license", link: "a.js.license"},
		{name: "nextcloud/dist/a.js.license", body: "AGPL"},
	})
	destination := t.TempDir()
	if err := extractCatalogArchive(nextcloudTestArchive(), archive, destination, scratch); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(filepath.Join(destination, "dist", "a.js.map.license"))
	if err != nil || !info.Mode().IsRegular() {
		t.Fatalf("the link was not written as a plain copy: %v %v", info, err)
	}
	if content, _ := os.ReadFile(filepath.Join(destination, "dist", "a.js.map.license")); string(content) != "AGPL" {
		t.Fatalf("copy content = %q", content)
	}
}

func TestExtractCatalogArchiveRefusesUnsafeEntries(t *testing.T) {
	base := []tarEntry{
		{name: "nextcloud/occ", body: "x"}, {name: "nextcloud/version.php", body: "x"},
		{name: "nextcloud/index.php", body: "x"}, {name: "nextcloud/lib/a", body: "x"}, {name: "nextcloud/core/a", body: "x"},
	}
	cases := map[string][]tarEntry{
		"escaping link":         append(append([]tarEntry{}, base...), tarEntry{name: "nextcloud/evil", link: "../../etc/passwd"}),
		"absolute link":         append(append([]tarEntry{}, base...), tarEntry{name: "nextcloud/evil", link: "/etc/passwd"}),
		"link to missing file":  append(append([]tarEntry{}, base...), tarEntry{name: "nextcloud/evil", link: "nothing"}),
		"outside the top":       append(append([]tarEntry{}, base...), tarEntry{name: "other/file", body: "x"}),
		"parent path":           append(append([]tarEntry{}, base...), tarEntry{name: "nextcloud/../../x", body: "x"}),
		"duplicate":             append(append([]tarEntry{}, base...), tarEntry{name: "nextcloud/occ", body: "y"}),
		"missing release files": {{name: "nextcloud/index.php", body: "x"}},
	}
	for name, entries := range cases {
		t.Run(name, func(t *testing.T) {
			scratch := t.TempDir()
			archive := filepath.Join(scratch, "release")
			writeTarGz(t, archive, entries)
			destination := t.TempDir()
			if err := extractCatalogArchive(nextcloudTestArchive(), archive, destination, scratch); err == nil {
				t.Fatal("an unsafe archive was accepted")
			}
		})
	}
}

func TestExtractCatalogArchiveOpensPrestaShopsInnerZip(t *testing.T) {
	var inner bytes.Buffer
	zw := zip.NewWriter(&inner)
	for _, name := range []string{"index.php", "install/index_cli.php", "admin/index.php", "app/AppKernel.php", "vendor/autoload.php"} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte("<?php"))
	}
	_ = zw.Close()
	var outer bytes.Buffer
	ow := zip.NewWriter(&outer)
	for name, body := range map[string][]byte{"prestashop.zip": inner.Bytes(), "index.php": []byte("<?php // the web unzipper"), "Install_PrestaShop.html": []byte("<html>")} {
		w, err := ow.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write(body)
	}
	_ = ow.Close()
	scratch := t.TempDir()
	archive := filepath.Join(scratch, "release")
	if err := os.WriteFile(archive, outer.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	destination := t.TempDir()
	if err := extractCatalogArchive(catalogTestApp(t, "prestashop"), archive, destination, scratch); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(destination, "install", "index_cli.php")); err != nil {
		t.Fatal("the inner tree was not unpacked")
	}
	if _, err := os.Stat(filepath.Join(destination, "Install_PrestaShop.html")); err == nil {
		t.Fatal("the outer web unzipper was published")
	}
}

type catalogTestSite struct {
	docRoot, home string
	site          transport.AppSiteIdentity
}

func newCatalogTestSite(t *testing.T, php string) catalogTestSite {
	t.Helper()
	home := filepath.Join(t.TempDir(), "sites", "13")
	docRoot := filepath.Join(home, "public_html")
	if err := os.MkdirAll(docRoot, 0o750); err != nil {
		t.Fatal(err)
	}
	previous, previousRemovals := catalogSitePaths, catalogRemovalStateDir
	catalogSitePaths = func(int, int) (string, string, error) { return docRoot, home, nil }
	catalogRemovalStateDir = filepath.Join(t.TempDir(), "app-removals")
	t.Cleanup(func() { catalogSitePaths, catalogRemovalStateDir = previous, previousRemovals })
	return catalogTestSite{docRoot: docRoot, home: home, site: transport.AppSiteIdentity{
		SiteID: 21, SubscriptionID: 4, DomainID: 13, Domain: catalogTestDomain,
		Username: services.SiteUsername(catalogTestDomain), PHPVersion: php,
	}}
}

func TestAppPrerequisitesNameTheMissingPackagesAndChangeNothing(t *testing.T) {
	site := newCatalogTestSite(t, "8.3.12")
	previousFamily, previousProbe, previousFree := catalogPkgFamily, catalogPHPExtensions, catalogFreeDiskMB
	t.Cleanup(func() {
		catalogPkgFamily, catalogPHPExtensions, catalogFreeDiskMB = previousFamily, previousProbe, previousFree
	})
	catalogPkgFamily = func() string { return "apt" }
	catalogFreeDiskMB = func(string) int64 { return 0 }
	var asked []string
	catalogPHPExtensions = func(_ context.Context, line string, extensions []string) ([]string, error) {
		if line != "8.3" {
			t.Fatalf("the probe asked PHP %q", line)
		}
		asked = extensions
		return []string{"intl", "pdo_mysql"}, nil
	}
	agent := &Agent{}
	var resp transport.AppPrerequisitesResponse
	if err := agent.AppPrerequisitesV1(&transport.AppPrerequisitesRequest{AppID: "prestashop", Site: site.site}, &resp); err != nil || resp.Error != "" {
		t.Fatalf("err=%v resp=%+v", err, resp)
	}
	if len(asked) == 0 || !resp.PHPOK || resp.DocRoot != transport.AppDocRootEmpty {
		t.Fatalf("unexpected answer %+v", resp)
	}
	packages := []string{}
	for _, m := range resp.Missing {
		packages = append(packages, m.Package)
	}
	sort.Strings(packages)
	if strings.Join(packages, " ") != "php8.3-intl php8.3-mysql" {
		t.Fatalf("missing packages = %v", packages)
	}

	// Drupal 11 on PHP 8.2: refused with the bound named; customer content
	// in the document root is reported, never touched.
	site.site.PHPVersion = "8.2"
	if err := os.WriteFile(filepath.Join(site.docRoot, "index.html"), []byte("mine"), 0o640); err != nil {
		t.Fatal(err)
	}
	catalogPHPExtensions = func(context.Context, string, []string) ([]string, error) { return nil, nil }
	resp = transport.AppPrerequisitesResponse{}
	_ = agent.AppPrerequisitesV1(&transport.AppPrerequisitesRequest{AppID: "drupal", Site: site.site}, &resp)
	if resp.PHPOK || !strings.Contains(resp.PHPReason, "8.3 or newer") || resp.DocRoot != transport.AppDocRootContent {
		t.Fatalf("unexpected answer %+v", resp)
	}
	if content, _ := os.ReadFile(filepath.Join(site.docRoot, "index.html")); string(content) != "mine" {
		t.Fatal("the read-only check changed the site")
	}
}

func TestSetupPlansKeepSecretsOffTheCommandLine(t *testing.T) {
	site := newCatalogTestSite(t, "8.3")
	previousPrepend := setupPrependPath
	setupPrependPath = filepath.Join(t.TempDir(), "setup", "prepend.php")
	t.Cleanup(func() { setupPrependPath = previousPrepend })
	const dbPass, adminPass = "DbSecretValue123456789", "AdminSecretValue987654"
	for _, app := range appcatalog.All() {
		if app.Legacy() {
			continue
		}
		resolved, err := resolveCatalogSite(app.ID, site.site)
		if err != nil {
			t.Fatal(err)
		}
		plan, err := buildSetupPlan(resolved, &transport.SetupCatalogAppRequest{
			DBName: "shop_example_test_x", DBUser: "shop_example_test_x", DBPass: dbPass,
			AdminPass: adminPass, AdminEmail: "owner@example.test", HTTPS: true,
		})
		if err != nil {
			t.Fatalf("%s: %v", app.ID, err)
		}
		if len(plan.steps) == 0 || plan.done == nil {
			t.Fatalf("%s: no installer step or no finished-setup check", app.ID)
		}
		carried := false
		for _, step := range append(append([]setupStep{}, plan.steps...), plan.after...) {
			argv, stdin, err := step.command(resolved)
			if err != nil {
				t.Fatalf("%s %s: %v", app.ID, step.name, err)
			}
			line := strings.Join(argv, " ")
			if strings.Contains(line, dbPass) || strings.Contains(line, adminPass) {
				t.Fatalf("%s %s: a secret is on the command line: %s", app.ID, step.name, line)
			}
			if bytes.Contains(stdin, []byte(dbPass)) && bytes.Contains(stdin, []byte(adminPass)) {
				carried = true
			}
		}
		if !carried {
			t.Fatalf("%s: the installer does not receive the secrets on standard input", app.ID)
		}
	}
	if content, err := os.ReadFile(setupPrependPath); err != nil || !strings.Contains(string(content), "@@CELIKPANEL:") {
		t.Fatalf("the prepend helper was not written: %v", err)
	}
}

type recordingIncludes struct {
	published, removed []string
	keep               map[string]bool
}

func (r *recordingIncludes) PublishAppInclude(_, fileName, _ string) error {
	r.published = append(r.published, fileName)
	return nil
}

func (r *recordingIncludes) RemoveAppInclude(_, fileName string) error {
	if r.keep[fileName] {
		return services.ErrAppIncludeKept
	}
	r.removed = append(r.removed, fileName)
	return nil
}

func useRecordingIncludes(t *testing.T) *recordingIncludes {
	t.Helper()
	rec := &recordingIncludes{keep: map[string]bool{}}
	previous := catalogNginx
	catalogNginx = func(*Agent) appIncludeWriter { return rec }
	t.Cleanup(func() { catalogNginx = previous })
	return rec
}

// fakeSiteUser runs the plain commands of removal in-process and records them.
func fakeSiteUser(t *testing.T, calls *[]string) {
	t.Helper()
	previous := catalogRunAsSiteUser
	catalogRunAsSiteUser = func(_ context.Context, username, _ string, argv []string, stdin []byte) (string, error) {
		*calls = append(*calls, strings.Join(argv, " "))
		switch argv[0] {
		case "find":
			if argv[len(argv)-1] != "-delete" {
				return "", nil // the permission reset before the removal
			}
			// find <entry>... -xdev -delete: the recorded entries.
			for _, target := range argv[1 : len(argv)-2] {
				_ = os.RemoveAll(target)
			}
		case "sh":
			if strings.Contains(argv[2], "cat >") {
				if _, err := os.Lstat(argv[len(argv)-1]); err == nil && strings.Contains(argv[2], "set -C") {
					return "", nil // noclobber: an existing file stays
				}
				return "", os.WriteFile(argv[len(argv)-1], stdin, 0o640)
			}
		}
		return "", nil
	}
	t.Cleanup(func() { catalogRunAsSiteUser = previous })
}

func writeMarkers(t *testing.T, root string, app appcatalog.App) {
	t.Helper()
	for _, marker := range app.Archive.Markers {
		path := filepath.Join(root, filepath.FromSlash(strings.TrimSuffix(marker, "/")))
		if strings.HasSuffix(marker, "/") {
			if err := os.MkdirAll(path, 0o750); err != nil {
				t.Fatal(err)
			}
			continue
		}
		_ = os.MkdirAll(filepath.Dir(path), 0o750)
		if err := os.WriteFile(path, []byte("<?php"), 0o640); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRemoveCatalogAppRemovesFilesAndItsNginxRules(t *testing.T) {
	site := newCatalogTestSite(t, "8.3")
	rec := useRecordingIncludes(t)
	var calls []string
	fakeSiteUser(t, &calls)
	writeMarkers(t, site.docRoot, catalogTestApp(t, "drupal"))
	var resp transport.RemoveCatalogAppResponse
	if err := (&Agent{}).RemoveCatalogAppV1(&transport.RemoveCatalogAppRequest{AppID: "drupal", Site: site.site}, &resp); err != nil || resp.Error != "" {
		t.Fatalf("err=%v resp=%+v", err, resp)
	}
	if !resp.FilesRemoved || !resp.RulesRemoved {
		t.Fatalf("unexpected answer %+v", resp)
	}
	sort.Strings(rec.removed)
	if strings.Join(rec.removed, ",") != "app-drupal.conf,app-setup-lock.conf" {
		t.Fatalf("removed includes = %v", rec.removed)
	}
	name, _ := celikPanelSitePlaceholder(catalogTestDomain, "php")
	entries, _ := os.ReadDir(site.docRoot)
	if len(entries) != 1 || entries[0].Name() != name {
		t.Fatalf("the document root does not hold only the placeholder: %v", entries)
	}
}

func TestRemoveCatalogAppRefusesASiteThatNoLongerHoldsIt(t *testing.T) {
	site := newCatalogTestSite(t, "8.3")
	rec := useRecordingIncludes(t)
	var calls []string
	fakeSiteUser(t, &calls)
	if err := os.WriteFile(filepath.Join(site.docRoot, "index.html"), []byte("the owner's new site"), 0o640); err != nil {
		t.Fatal(err)
	}
	var resp transport.RemoveCatalogAppResponse
	_ = (&Agent{}).RemoveCatalogAppV1(&transport.RemoveCatalogAppRequest{AppID: "drupal", Site: site.site}, &resp)
	if !resp.NotThisApp || resp.FilesRemoved || len(calls) != 0 || len(rec.removed) != 0 {
		t.Fatalf("something else in the document root was touched: resp=%+v calls=%v removed=%v", resp, calls, rec.removed)
	}
}

// A setup run again after it finished detects that and only finishes
// CelikPanel's part: the installer does not run twice.
func TestSetupCatalogAppRunAgainDoesNotRepeatTheInstaller(t *testing.T) {
	site := newCatalogTestSite(t, "8.3")
	rec := useRecordingIncludes(t)
	var calls []string
	fakeSiteUser(t, &calls)
	app := catalogTestApp(t, "joomla")
	writeMarkers(t, site.docRoot, app)
	if err := os.WriteFile(filepath.Join(site.docRoot, "configuration.php"), []byte("<?php class JConfig { public $password = 'x'; }"), 0o640); err != nil {
		t.Fatal(err)
	}
	var resp transport.SetupCatalogAppResponse
	err := (&Agent{}).SetupCatalogAppV1(&transport.SetupCatalogAppRequest{
		OperationID: strings.Repeat("ab", 16), AppID: "joomla", Site: site.site,
		DBName: "shop_joomla", DBUser: "shop_joomla", DBPass: "x1234567890", AdminPass: "Admin1234567890", AdminEmail: "o@example.test",
	}, &resp)
	if err != nil || !resp.Configured || !resp.AlreadyConfigured {
		t.Fatalf("err=%v resp=%+v", err, resp)
	}
	for _, call := range calls {
		if strings.Contains(call, "joomla.php") {
			t.Fatalf("the installer ran again: %v", calls)
		}
	}
	if strings.Join(rec.published, ",") != "app-joomla.conf" || strings.Join(rec.removed, ",") != "app-setup-lock.conf" {
		t.Fatalf("published=%v removed=%v", rec.published, rec.removed)
	}
	if resp.AdminPath != "/administrator/" {
		t.Fatalf("admin path = %q", resp.AdminPath)
	}
}

func TestSetupCatalogAppFailureKeepsTheSiteLocked(t *testing.T) {
	site := newCatalogTestSite(t, "8.3")
	rec := useRecordingIncludes(t)
	writeMarkers(t, site.docRoot, catalogTestApp(t, "nextcloud"))
	previous := catalogRunAsSiteUser
	catalogRunAsSiteUser = func(context.Context, string, string, []string, []byte) (string, error) {
		return "Error: DbSecretValue was refused", errors.New("it ended with exit code 1")
	}
	t.Cleanup(func() { catalogRunAsSiteUser = previous })
	previousPrepend := setupPrependPath
	setupPrependPath = filepath.Join(t.TempDir(), "prepend.php")
	t.Cleanup(func() { setupPrependPath = previousPrepend })
	var resp transport.SetupCatalogAppResponse
	_ = (&Agent{}).SetupCatalogAppV1(&transport.SetupCatalogAppRequest{
		OperationID: strings.Repeat("cd", 16), AppID: "nextcloud", Site: site.site,
		DBName: "shop_nc", DBUser: "shop_nc", DBPass: "DbSecretValue", AdminPass: "Admin1234567890", AdminEmail: "o@example.test",
	}, &resp)
	if resp.Configured || resp.ErrorKind != transport.AppFailSetup || !strings.Contains(resp.Error, "being set up") {
		t.Fatalf("unexpected answer %+v", resp)
	}
	if strings.Contains(resp.Output, "DbSecretValue") {
		t.Fatalf("a secret reached the output: %q", resp.Output)
	}
	if len(rec.removed) != 0 || len(rec.published) != 0 {
		t.Fatalf("the site was opened after a failed setup: published=%v removed=%v", rec.published, rec.removed)
	}
}

func TestAppConfigFormatsRewriteOnlyThePassword(t *testing.T) {
	samples := map[string]string{
		"nextcloud":  "<?php\n$CONFIG = array (\n  'dbname' => 'nc_db',\n  'dbuser' => 'nc_user',\n  'dbpassword' => 'old',\n  'installed' => true,\n);\n",
		"drupal":     "<?php\n/**\n *   'database' => 'databasename',\n *   'username' => 'sqlusername',\n *   'password' => 'sqlpassword',\n */\n$databases['default']['default'] = array (\n  'database' => 'nc_db',\n  'username' => 'nc_user',\n  'password' => 'old',\n  'host' => 'localhost',\n);\n",
		"joomla":     "<?php\nclass JConfig {\n\tpublic $db = 'nc_db';\n\tpublic $user = 'nc_user';\n\tpublic $password = 'old';\n}\n",
		"prestashop": "<?php return array (\n  'parameters' => array (\n    'database_name' => 'nc_db',\n    'database_user' => 'nc_user',\n    'database_password' => 'old',\n  ),\n);\n",
		"mediawiki":  "<?php\n$wgDBname = \"nc_db\";\n$wgDBuser = \"nc_user\";\n$wgDBpassword = \"old\";\n",
		"phpbb":      "<?php\n$dbname = 'nc_db';\n$dbuser = 'nc_user';\n$dbpasswd = 'old';\n",
	}
	for kind, sample := range samples {
		format, ok := appConfigFormatFor(kind)
		if !ok {
			t.Fatalf("%s has no configuration format", kind)
		}
		if reason := format.matches([]byte(sample), "nc_db", "nc_user"); reason != "" {
			t.Fatalf("%s: %s", kind, reason)
		}
		if reason := format.matches([]byte(sample), "other_db", "nc_user"); reason == "" {
			t.Fatalf("%s: a file naming another database matched", kind)
		}
		next, err := format.replacePassword([]byte(sample), `n3w'p"a$s\word`)
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		_, _, start, end, err := format.defines(next)
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		if got := format.unquote(string(next[start+1 : end-1])); got != `n3w'p"a$s\word` {
			t.Fatalf("%s: the password reads back as %q", kind, got)
		}
		if strings.Replace(string(next), string(next[start:end]), "'old'", 1) != sample &&
			strings.Replace(string(next), string(next[start:end]), `"old"`, 1) != sample {
			t.Fatalf("%s: more than the password changed:\n%s", kind, next)
		}
	}
	if _, ok := appConfigFormatFor("wordpress"); ok {
		t.Fatal("WordPress keeps its own wp-config.php reader")
	}
	if err := validDatabaseUserApp(&transport.DatabaseUserApp{Kind: "nextcloud", SubscriptionID: 1, DomainID: 2, SiteUsername: "s", DBName: "nc_db"}, "nc_user"); err != nil {
		t.Fatalf("a catalog application was refused for rotation: %v", err)
	}
}
