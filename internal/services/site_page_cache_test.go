package services

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.128 (PLESK-GAP G28): the page cache of a site.

func TestPageCacheBypassesEverythingPrivate(t *testing.T) {
	conf := RenderWebHTTPConf()
	for _, want := range []string{
		"map $request_method $celikpanel_cache_method {\n    GET 0;\n    HEAD 0;\n    default 1;\n}",
		"map $http_authorization $celikpanel_cache_auth {\n    \"\" 0;\n    default 1;\n}",
		"map $arg_nocache $celikpanel_cache_arg {",
		"\"0000\" 0;\n    default 1;",
	} {
		if !strings.Contains(conf, want) {
			t.Fatalf("missing %q in:\n%s", want, conf)
		}
	}
	// The cookie rule: only a header made of analytics cookies is cacheable.
	line := ""
	for _, l := range strings.Split(conf, "\n") {
		if strings.Contains(l, "_fbp") {
			line = l
		}
	}
	start, end := strings.Index(line, `"~`), strings.LastIndex(line, `"`)
	pattern := regexp.MustCompile(line[start+2 : end])
	for cookie, cacheable := range map[string]bool{
		"_ga=GA1.2.3; _gid=GA1.2.4":          true,
		"_ga=1; _fbp=fb.1":                   true,
		"PHPSESSID=abc":                      false,
		"_ga=1; wordpress_logged_in_x=admin": false,
		"laravel_session=x; _ga=1":           false,
		"_gat_UA-1=1;_hjSession_1=x":         true,
		"_ga=1; custom_login=1":              false,
	} {
		if pattern.MatchString(cookie) != cacheable {
			t.Errorf("cookie %q: cacheable=%v", cookie, !cacheable)
		}
	}
}

func TestPageCachePartsFollowTheSwitch(t *testing.T) {
	ng, _ := trafficTestGenerator(t)
	domain := "cache.example"
	body := "server {\n    include " + PanelManagedDir(domain) + "/*.conf;\n    " + SiteIncludeLine(domain) + "\n}\n"
	on := SiteWebSettings{PageCache: true, PageCacheMinutes: 5}
	item := ManagedVhostItem{Domain: domain, Body: body, Trigger: transport.SiteFileTriggerCreate,
		SiteParts: SitePartsForSite(domain, 9, "php", on, "")}
	results, err := ng.ApplyManagedVhosts([]ManagedVhostItem{item})
	if err != nil {
		t.Fatal(err)
	}
	if results[0].SiteParts[PageCacheHTTPConfName(domain)] != transport.SiteFileChallengeWritten {
		t.Fatalf("parts: %+v", results[0].SiteParts)
	}
	zone, _ := os.ReadFile(PageCacheHTTPConfPath(domain))
	if !strings.Contains(string(zone), "fastcgi_cache_path "+PageCacheDir(9)+" levels=1:2 keys_zone=celikpanel_pc_9:10m") {
		t.Fatalf("zone:\n%s", zone)
	}
	if info, err := os.Stat(PageCacheRoot()); err != nil || !info.IsDir() {
		t.Fatalf("cache root: %v", err)
	}
	_, settings := SitePartState(domain, SiteSettingsPartName)
	for _, want := range []string{"fastcgi_cache celikpanel_pc_9;", "fastcgi_cache_valid 200 301 302 5m;", "fastcgi_cache_bypass $celikpanel_cache_skip;", "fastcgi_no_cache $celikpanel_cache_skip;"} {
		if !strings.Contains(settings, want) {
			t.Fatalf("missing %q:\n%s", want, settings)
		}
	}
	// A Node.js site uses proxy_cache.
	if node := SitePartsForSite("n.example", 3, "node", on, ""); !strings.Contains(node[0].Body, "proxy_cache celikpanel_pc_3;") ||
		!strings.Contains(node[2].Body, "proxy_cache_path") {
		t.Fatalf("node parts: %+v", node)
	}
	// Off: the zone file goes, the site's file has no cache line.
	item.Trigger = transport.SiteFileTriggerChange
	item.SiteParts = SitePartsForSite(domain, 9, "php", SiteWebSettings{}, "")
	results, _ = ng.ApplyManagedVhosts([]ManagedVhostItem{item})
	if results[0].SiteParts[PageCacheHTTPConfName(domain)] != transport.SitePartRemoved {
		t.Fatalf("off: %+v", results[0].SiteParts)
	}
	if _, settings = SitePartState(domain, SiteSettingsPartName); strings.Contains(settings, "fastcgi_cache") {
		t.Fatal("cache lines left after the switch went off")
	}
	// A static site has no upstream to cache.
	if _, err := PrepareSiteWebSettings("static", on); err == nil {
		t.Fatal("a page cache was accepted for a static site")
	}
	if _, err := PrepareSiteWebSettings("php", SiteWebSettings{PageCache: true, PageCacheMinutes: 5000}); err == nil {
		t.Fatal("5000 minutes accepted")
	}
}

func TestClearPageCacheEmptiesOnlyTheSitesFolder(t *testing.T) {
	previous := nginxDir
	nginxDir = t.TempDir()
	t.Cleanup(func() { nginxDir = previous })
	if n, err := ClearPageCache(4); err != nil || n != 0 {
		t.Fatalf("missing folder: %d %v", n, err)
	}
	dir := PageCacheDir(4)
	other := PageCacheDir(5)
	for _, d := range []string{filepath.Join(dir, "a", "b"), other} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	_ = os.WriteFile(filepath.Join(dir, "a", "b", "entry"), []byte("x"), 0o600)
	_ = os.WriteFile(filepath.Join(other, "keep"), []byte("x"), 0o600)
	if n, err := ClearPageCache(4); err != nil || n != 1 {
		t.Fatalf("clear: %d %v", n, err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatal("the folder was not emptied")
	}
	if _, err := os.Stat(filepath.Join(other, "keep")); err != nil {
		t.Fatal("another site's cache was touched")
	}
}
