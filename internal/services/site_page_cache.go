package services

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/alicelik/celikpanel/internal/transport"
)

// nginx page cache of one site (alpha.128, PLESK-GAP G28; design §2.4
// "nginx caching"). Off by default; only the administrator turns it on, per
// site, because a wrong cache key would serve one visitor's private page to
// another (security before freedom).
//
// Native form:
//   - conf.d/celikpanel-pagecache-<domain>.conf (http context): the site's own
//     cache zone, fastcgi_cache_path (PHP) or proxy_cache_path (Node.js and
//     proxy sites) under /var/cache/celikpanel/pagecache/<site id>;
//   - settings.site: fastcgi_cache / proxy_cache with that zone, a 10-minute
//     default validity, stale answers while the application is down;
//   - conf.d/celikpanel-web.conf: the bypass map $celikpanel_cache_skip.
//
// Never cached (bypass and no-store): any method but GET and HEAD; any
// Authorization header; ?nocache; any cookie except analytics cookies only
// (_ga, _gid, _gat*, _gcl_*, _fbp, _hj*) — a session or login cookie of any
// name means the visitor may see a private page. nginx itself never caches an
// answer that sets a cookie or says private/no-store. "Clear cache" empties
// the site's cache folder through the Agent; nginx tolerates that at any time.
//
// Sitenin nginx sayfa önbelleği: varsayılan kapalı, yalnız yönetici açar.
// Çerezli, yetkili ya da GET/HEAD olmayan istekler önbelleğe girmez.

// pageCacheRootDir is the parent of the sites' cache folders (a seam: tests
// and the development mode use a directory under nginxDir).
var pageCacheRootDir = "/var/cache/celikpanel/pagecache"

// PageCacheRoot is the parent of the sites' cache folders.
func PageCacheRoot() string {
	if nginxDevMode() {
		return filepath.Join(nginxDir, "celikpanel-pagecache")
	}
	return pageCacheRootDir
}

// PageCacheDir is one site's cache folder (made by nginx's master, owned by
// its worker account).
func PageCacheDir(siteID int) string {
	return filepath.Join(PageCacheRoot(), strconv.Itoa(siteID))
}

// PageCacheZone is the site's cache zone name.
func PageCacheZone(siteID int) string { return "celikpanel_pc_" + strconv.Itoa(siteID) }

// PageCacheHTTPConfName is the site's http-level file name in SiteParts.
func PageCacheHTTPConfName(domain string) string { return "celikpanel-pagecache-" + domain + ".conf" }

// PageCacheHTTPConfPath is the site's http-level cache zone file.
func PageCacheHTTPConfPath(domain string) string {
	return filepath.Join(nginxBase(), "conf.d", PageCacheHTTPConfName(domain))
}

// pageCacheDirective is "fastcgi" for PHP and "proxy" for the proxied types.
func pageCacheDirective(projectType string) string {
	if projectType == "php" {
		return "fastcgi"
	}
	return "proxy"
}

// PageCacheApplies: the project type has an upstream whose answers can be cached.
func PageCacheApplies(projectType string) bool {
	switch projectType {
	case "php", "node", "proxy", "service_app", "runtime_app":
		return true
	}
	return false
}

// RenderPageCacheHTTPConf is the body of PageCacheHTTPConfPath.
func RenderPageCacheHTTPConf(domain string, siteID int, projectType string) string {
	return fmt.Sprintf("# The page cache zone of %s, written by CelikPanel (Hosting → Web server).\n", domain) +
		"# Clear it with \"Clear cache\" in CelikPanel, or empty the folder below; nginx\n" +
		"# keeps working either way. Sayfa önbelleği bölgesi.\n" +
		fmt.Sprintf("%s_cache_path %s levels=1:2 keys_zone=%s:10m max_size=512m inactive=60m use_temp_path=off;\n",
			pageCacheDirective(projectType), PageCacheDir(siteID), PageCacheZone(siteID))
}

// renderPageCacheLines are settings.site's cache lines.
func renderPageCacheLines(siteID int, projectType string, minutes int) string {
	d := pageCacheDirective(projectType)
	if minutes <= 0 {
		minutes = DefaultPageCacheMinutes
	}
	var b strings.Builder
	b.WriteString("# Page cache: GET/HEAD without login, session or authorization only.\n")
	fmt.Fprintf(&b, "%s_cache %s;\n", d, PageCacheZone(siteID))
	fmt.Fprintf(&b, "%s_cache_key \"$scheme$request_method$host$request_uri\";\n", d)
	fmt.Fprintf(&b, "%s_cache_valid 200 301 302 %dm;\n", d, minutes)
	fmt.Fprintf(&b, "%s_cache_use_stale error timeout updating http_500 http_503;\n", d)
	fmt.Fprintf(&b, "%s_cache_lock on;\n", d)
	fmt.Fprintf(&b, "%s_cache_bypass $celikpanel_cache_skip;\n", d)
	fmt.Fprintf(&b, "%s_no_cache $celikpanel_cache_skip;\n", d)
	b.WriteString("add_header X-Cache-Status $upstream_cache_status always;\n")
	return b.String()
}

// pageCacheMaps are the bypass maps in conf.d/celikpanel-web.conf.
func pageCacheMaps() string {
	return "# Page cache bypass: 1 = never cached for this request.\n" +
		"map $request_method $celikpanel_cache_method {\n    GET 0;\n    HEAD 0;\n    default 1;\n}\n" +
		"map $http_authorization $celikpanel_cache_auth {\n    \"\" 0;\n    default 1;\n}\n" +
		"map $arg_nocache $celikpanel_cache_arg {\n    \"\" 0;\n    default 1;\n}\n" +
		"map $http_cookie $celikpanel_cache_cookie {\n    \"\" 0;\n" +
		"    \"~^\\s*(?:(?:_ga[A-Za-z0-9_-]*|_gid|_gat[A-Za-z0-9_-]*|_gcl_[A-Za-z0-9_-]+|_fbp|_hj[A-Za-z0-9_-]+)=[^;]*(?:;\\s*|$))+$\" 0;\n" +
		"    default 1;\n}\n" +
		"map \"$celikpanel_cache_method$celikpanel_cache_auth$celikpanel_cache_arg$celikpanel_cache_cookie\" $celikpanel_cache_skip {\n" +
		"    \"0000\" 0;\n    default 1;\n}\n"
}

// DefaultPageCacheMinutes is how long a cached page is used.
const DefaultPageCacheMinutes = 10

// validPageCacheMinutes bounds the validity (1 minute to 1 day).
func validPageCacheMinutes(minutes int) bool {
	return minutes == 0 || (minutes >= 1 && minutes <= 1440)
}

// pageCacheHTTPPart is the site's http-level part ("" body: removed).
func pageCacheHTTPPart(domain string, siteID int, projectType string, on bool) SitePart {
	part := SitePart{Name: PageCacheHTTPConfName(domain), Path: PageCacheHTTPConfPath(domain)}
	if on {
		part.Body = RenderPageCacheHTTPConf(domain, siteID, projectType)
		part.EnsureDir = PageCacheRoot()
	}
	return part
}

func init() {
	previous := removeSiteHTTPParts
	removeSiteHTTPParts = func(domain string) {
		previous(domain)
		path := PageCacheHTTPConfPath(domain)
		if inspected := inspectSiteFile(path, nil, ""); inspected.state == transport.SiteFileManagedUnchanged {
			_ = os.Remove(path)
		}
	}
}

// ClearPageCache empties one site's cache folder (its files only; the folder
// stays, nginx keeps writing into it). A missing folder is already empty.
func ClearPageCache(siteID int) (int, error) {
	if siteID <= 0 {
		return 0, errors.New("a site is required")
	}
	dir := PageCacheDir(siteID)
	info, err := os.Lstat(dir)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if !info.IsDir() {
		return 0, fmt.Errorf("%s is not a folder; it was left alone", dir)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}
	removed := 0
	var errs []error
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(dir, entry.Name())); err != nil {
			errs = append(errs, err)
			continue
		}
		removed++
	}
	return removed, errors.Join(errs...)
}
