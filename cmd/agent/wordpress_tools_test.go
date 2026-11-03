package main

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

func TestCompareWordPressVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"6.6.2", "6.6.1", 1}, {"6.6", "6.6.0", 0}, {"6.10", "6.9.9", 1}, {"6.7-RC1", "6.7", -1},
		{"", "1.0", -1}, {"5.3", "5.3", 0}, {"1.2.3", "1.10", -1},
	}
	for _, c := range cases {
		if got := compareWordPressVersions(c.a, c.b); got != c.want {
			t.Errorf("compare(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

// Updates are asked only from the official API; the answer decides what the
// screen offers and whether CelikPanel can verify it.
func TestCheckWordPressUpdatesReadsTheOfficialAnswers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/core/version-check/1.7/":
			_, _ = w.Write([]byte(`{"offers":[{"response":"upgrade","version":"6.7.1","current":"6.7.1"},{"response":"autoupdate","version":"6.6.3","current":"6.6.3"}]}`))
		case "/plugins/update-check/1.1/":
			_ = r.ParseForm()
			if !strings.Contains(r.PostForm.Get("plugins"), "akismet/akismet.php") {
				t.Errorf("plugins were not sent: %q", r.PostForm.Get("plugins"))
			}
			_, _ = w.Write([]byte(`{"plugins":{"akismet/akismet.php":{"slug":"akismet","new_version":"5.4","package":"x"}},"no_update":[]}`))
		case "/themes/update-check/1.1/":
			_, _ = w.Write([]byte(`{"themes":{"twentytwentyfour":{"new_version":"1.3"}},"no_update":{}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	previous := wordpressAPIBase
	wordpressAPIBase = server.URL
	t.Cleanup(func() { wordpressAPIBase = previous })

	status := &transport.WordPressStatusResponse{CoreVersion: "6.6.2"}
	fillWordPressStatus(status, wordpressInventory{Core: "6.6.2"})
	status.Plugins = []transport.WordPressItem{
		{Kind: "plugin", Slug: "akismet", File: "akismet/akismet.php", Name: "Akismet", Version: "5.3"},
		{Kind: "plugin", Slug: "custom", File: "custom/custom.php", Name: "Custom", Version: "1.0"},
	}
	status.Themes = []transport.WordPressItem{{Kind: "theme", Slug: "twentytwentyfour", Name: "TT4", Version: "1.2"}}
	if err := checkWordPressUpdates(context.Background(), status, false); err != nil {
		t.Fatal(err)
	}
	if status.CoreUpdate != "6.7.1" || status.CoreMinor != "6.6.3" {
		t.Fatalf("core offers = %q / %q", status.CoreUpdate, status.CoreMinor)
	}
	akismet, custom := status.Plugins[0], status.Plugins[1]
	if akismet.UpdateVersion != "5.4" || !akismet.Verifiable || akismet.Source != "wordpress.org" {
		t.Fatalf("akismet = %+v", akismet)
	}
	if custom.UpdateVersion != "" || custom.Verifiable || custom.Source != "other" {
		t.Fatalf("a plugin wordpress.org does not know = %+v", custom)
	}
	if theme := status.Themes[0]; theme.UpdateVersion != "1.3" || theme.Verifiable {
		t.Fatalf("a theme update must never be verifiable: %+v", theme)
	}
}

// A theme update is refused before anything is read or downloaded.
func TestThemeUpdateIsRefusedAsNotVerifiable(t *testing.T) {
	agent := &Agent{}
	var resp transport.WordPressUpdateResponse
	err := agent.WordPressUpdateV1(&transport.WordPressUpdateRequest{
		OperationID: strings.Repeat("a", 32), Kind: "theme", Items: []string{"twentytwentyfour"},
		Site: transport.WordPressSite{SiteID: 1, SubscriptionID: 2, DomainID: 3, Domain: "a.example", Username: "a_example"},
	}, &resp)
	if err != nil || resp.Code != transport.WordPressCodeNotVerifiable || resp.Changed {
		t.Fatalf("resp=%+v err=%v", resp, err)
	}
}

func writePluginZip(t *testing.T, path string, files map[string]string, links ...string) {
	t.Helper()
	out, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(out)
	for name, content := range files {
		w, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(content))
	}
	for _, name := range links {
		header := &zip.FileHeader{Name: name}
		header.SetMode(os.ModeSymlink | 0o777)
		w, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte("/etc/passwd"))
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	_ = out.Close()
}

func checksumsFor(files map[string]string) wordpressChecksums {
	var sums wordpressChecksums
	raw := map[string]map[string]any{}
	for name, content := range files {
		sum := sha256.Sum256([]byte(content))
		raw[strings.TrimPrefix(name, "demo/")] = map[string]any{"sha256": hex.EncodeToString(sum[:])}
	}
	encoded, _ := json.Marshal(map[string]any{"files": raw})
	_ = json.Unmarshal(encoded, &sums)
	return sums
}

func TestPluginPackageIsVerifiedFileByFile(t *testing.T) {
	files := map[string]string{"demo/demo.php": "<?php /* Plugin Name: Demo */", "demo/readme.txt": "readme", "demo/inc/a.php": "<?php"}
	dir := t.TempDir()
	archive := filepath.Join(dir, "demo.zip")
	writePluginZip(t, archive, files)
	dest := filepath.Join(dir, "out")
	if err := os.Mkdir(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := extractWordPressZip(archive, dest, "demo"); err != nil {
		t.Fatal(err)
	}
	sums := checksumsFor(files)
	if err := verifyPluginTree(filepath.Join(dest, "demo"), sums); err != nil {
		t.Fatalf("a matching package was refused: %v", err)
	}
	// readme.txt may carry a list of accepted sums.
	readme := sums.Files["readme.txt"]
	readme.SHA256 = json.RawMessage(`["00","` + strings.Trim(string(readme.SHA256), `"`) + `"]`)
	sums.Files["readme.txt"] = readme
	if err := verifyPluginTree(filepath.Join(dest, "demo"), sums); err != nil {
		t.Fatalf("a listed alternative sum was refused: %v", err)
	}
	// One changed byte, one extra file, one missing file: refused.
	if err := os.WriteFile(filepath.Join(dest, "demo", "inc", "a.php"), []byte("<?php evil"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := verifyPluginTree(filepath.Join(dest, "demo"), sums); err == nil {
		t.Fatal("a changed file passed verification")
	}
	_ = os.WriteFile(filepath.Join(dest, "demo", "inc", "a.php"), []byte("<?php"), 0o644)
	_ = os.WriteFile(filepath.Join(dest, "demo", "extra.php"), []byte("x"), 0o644)
	if err := verifyPluginTree(filepath.Join(dest, "demo"), sums); err == nil {
		t.Fatal("a file outside the checksum list passed verification")
	}
	_ = os.Remove(filepath.Join(dest, "demo", "extra.php"))
	_ = os.Remove(filepath.Join(dest, "demo", "readme.txt"))
	if err := verifyPluginTree(filepath.Join(dest, "demo"), sums); err == nil {
		t.Fatal("a package missing a listed file passed verification")
	}
	if err := verifyPluginTree(filepath.Join(dest, "demo"), wordpressChecksums{}); err == nil {
		t.Fatal("an empty checksum list passed verification")
	}
}

func TestPluginPackageEntriesStayInsideTheirFolder(t *testing.T) {
	for name, build := range map[string]func(string){
		"outside":  func(p string) { writePluginZip(t, p, map[string]string{"other/x.php": "x"}) },
		"traverse": func(p string) { writePluginZip(t, p, map[string]string{"demo/../../x.php": "x"}) },
		"link":     func(p string) { writePluginZip(t, p, map[string]string{"demo/a.php": "x"}, "demo/link") },
	} {
		dir := t.TempDir()
		archive := filepath.Join(dir, "p.zip")
		build(archive)
		if err := extractWordPressZip(archive, dir, "demo"); err == nil {
			t.Errorf("%s: an unsafe package was extracted", name)
		}
	}
}

func TestRewriteWordPressConfigChangesOnlyTheDatabaseAndAddress(t *testing.T) {
	config := strings.Join([]string{
		"<?php",
		"define( 'DB_NAME', 'live_wp' );",
		"define('DB_USER', \"live_wp\");",
		"define( 'DB_PASSWORD', 'o\\'ld' );",
		"define( 'DB_HOST', 'localhost' );",
		"define( 'WP_HOME', 'https://live.example' );",
		"// the owner's own line stays",
		"$table_prefix = 'wp_';",
	}, "\n")
	out, err := rewriteWordPressConfig([]byte(config), "staging_wp", "staging_wp", "n'ew", "live.example", "staging.example.org")
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	for _, want := range []string{"define( 'DB_NAME', 'staging_wp' );", "define('DB_USER', 'staging_wp');",
		`define( 'DB_PASSWORD', 'n\'ew' );`, "'https://staging.example.org'", "// the owner's own line stays", "$table_prefix = 'wp_';"} {
		if !strings.Contains(got, want) {
			t.Errorf("rewritten config lacks %q:\n%s", want, got)
		}
	}
	if _, err := rewriteWordPressConfig([]byte("<?php require 'secrets.php';"), "a", "b", "c", "x.example", "y.example"); err == nil {
		t.Fatal("a config without plain database lines was rewritten")
	}
}

func TestWordPressSiteIdentityMustMatchItsAccount(t *testing.T) {
	site := transport.WordPressSite{SiteID: 1, SubscriptionID: 2, DomainID: 3, Domain: "shop.example", Username: "root"}
	if _, err := validWordPressSite(site); err == nil {
		t.Fatal("a site with another account was accepted")
	}
	site.Username = "shop_example"
	root, err := validWordPressSite(site)
	if err != nil || !strings.HasSuffix(root, "/subscriptions/2/sites/3/public_html") {
		t.Fatalf("root=%q err=%v", root, err)
	}
}

func TestWordPressHealthBroke(t *testing.T) {
	for _, c := range []struct {
		before, after int
		want          bool
	}{{200, 500, true}, {301, 0, true}, {200, 200, false}, {500, 500, false}, {0, 500, false}, {404, 302, false}} {
		if got := wordpressHealthBroke(c.before, c.after); got != c.want {
			t.Errorf("%d -> %d broke=%v, want %v", c.before, c.after, got, c.want)
		}
	}
}
