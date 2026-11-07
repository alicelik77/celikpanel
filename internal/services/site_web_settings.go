package services

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Web server settings of one site (alpha.128, PLESK-GAP G3; design
// SITE-SETTINGS-DESIGN §2.4). See transport/site_web_settings.go for the
// model. This file validates the fields, renders the site's two nginx files
// and the one http-level file they need, and plans their writes inside
// ApplyManagedVhosts, so they are checked by the same `nginx -t` as the vhost
// and put back with it.
//
// Where they go: a content-only include point, `include
// <managed dir>/*.site;`, at the end of the server blocks that serve the
// site's content (never the block that only redirects to HTTPS, and never a
// CelikPanel page: suspended, maintenance, traffic limit). A vhost carries
// that line only once the site saved a web server setting, so every other
// site's file stays byte for byte what it was.
//
// Gotchas handled (nginx rules):
//   - a second `index` line appends instead of replacing: index files are a
//     template variable of the vhost, not a line in settings.site;
//   - a `types` block at server level replaces the inherited map: the file
//     includes nginx's own mime.types first, so the map is extended;
//   - plain `expires` would also cache PHP pages: browser caching is
//     `expires $celikpanel_expires_<n>;`, a map over the answer's content
//     type in conf.d/celikpanel-web.conf, so only static files are cached;
//   - an access rule at server level is inherited by the certificate
//     validation location: that location (acme-http-01.conf) says
//     `allow all;` itself;
//   - `add_header` in a location drops the server's headers there: the
//     site's headers and HSTS stay at server level, and a reseller's or
//     customer's directives cannot put add_header inside a block.
//
// Sitenin web sunucusu ayarları: alanlar doğrulanır, sitenin iki nginx
// dosyası ve gereken tek http düzeyi dosyası üretilir ve vhost ile aynı
// nginx -t denetiminden geçer.

const (
	// SiteSettingsPartName and SiteDirectivesPartName are the site's two
	// files in its Panel-owned directory.
	SiteSettingsPartName   = "settings.site"
	SiteDirectivesPartName = "directives.site"
	// WebHTTPConfPartName names the http-level file in SiteParts results.
	WebHTTPConfPartName = "celikpanel-web.conf"

	maxSiteIndexFiles   = 8
	maxSiteMIMETypes    = 20
	maxSiteMIMEExts     = 10
	maxSiteHeaders      = 20
	maxSiteAccessList   = 100
	maxSiteHeaderValue  = 1024
	siteExpiresVariable = "$celikpanel_expires_"
)

// SiteWebSettings is the transport model.
type SiteWebSettings = transport.SiteWebSettings

// BrowserCacheChoices are the offered browser caching periods in days.
var BrowserCacheChoices = []int{0, 1, 7, 30}

var (
	siteIndexFilePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
	siteMIMETypePattern  = regexp.MustCompile(`^[a-z0-9][a-z0-9.+-]{0,62}/[a-z0-9][a-z0-9.+_-]{0,126}$`)
	siteMIMEExtPattern   = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,15}$`)
	siteHeaderPattern    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]{0,63}$`)
)

// Headers a site cannot set: HSTS belongs to the SSL/TLS settings; the rest
// would break the answer or are nginx's own.
var siteHeaderRefused = map[string]string{
	"strict-transport-security": "set it in the SSL/TLS settings",
	"content-length":            "nginx sets it",
	"content-type":              "use the extra MIME types",
	"transfer-encoding":         "nginx sets it",
	"connection":                "nginx sets it",
	"keep-alive":                "nginx sets it",
	"upgrade":                   "nginx sets it",
	"date":                      "nginx sets it",
	"server":                    "nginx sets it",
	"location":                  "use a redirect",
	"set-cookie":                "the application sets cookies",
	"expires":                   "use browser caching",
	"cache-control":             "use browser caching",
}

// SiteWebSettingsRefusal names the field a refusal is about.
type SiteWebSettingsRefusal struct {
	Field   string
	Message string
}

func (e *SiteWebSettingsRefusal) Error() string { return e.Message }

func refuseWebField(field, format string, args ...any) error {
	return &SiteWebSettingsRefusal{Field: field, Message: fmt.Sprintf(format, args...)}
}

// SiteWebFieldsApply says which fields a project type can use.
func SiteWebFieldsApply(projectType string) (index, static, node bool) {
	switch projectType {
	case "php", "static":
		return true, true, false
	case "node":
		return false, true, true
	case "proxy", "service_app", "runtime_app":
		return false, true, false
	}
	return false, false, false
}

// PrepareSiteWebSettings validates and normalizes the settings for a site of
// the project type. A field the type cannot use is refused when set.
func PrepareSiteWebSettings(projectType string, s SiteWebSettings) (SiteWebSettings, error) {
	index, any, node := SiteWebFieldsApply(projectType)
	if !any {
		if !siteWebSettingsEmpty(s) {
			return s, refuseWebField("", "a %s site serves no content of its own, so it has no web server settings", projectType)
		}
		return SiteWebSettings{}, nil
	}
	out := SiteWebSettings{}

	if len(s.IndexFiles) > 0 {
		if !index {
			return s, refuseWebField("index_files", "index files apply to PHP and static sites")
		}
		if len(s.IndexFiles) > maxSiteIndexFiles {
			return s, refuseWebField("index_files", "at most %d index files", maxSiteIndexFiles)
		}
		seen := map[string]bool{}
		for _, name := range s.IndexFiles {
			name = strings.TrimSpace(name)
			if !siteIndexFilePattern.MatchString(name) {
				return s, refuseWebField("index_files", "%q is not a file name (letters, digits, dot, dash, underscore; no folder)", name)
			}
			if !seen[name] {
				seen[name] = true
				out.IndexFiles = append(out.IndexFiles, name)
			}
		}
	}

	if len(s.MIMETypes) > maxSiteMIMETypes {
		return s, refuseWebField("mime_types", "at most %d extra MIME types", maxSiteMIMETypes)
	}
	seenExt := map[string]bool{}
	for _, mt := range s.MIMETypes {
		typ := strings.ToLower(strings.TrimSpace(mt.Type))
		if !siteMIMETypePattern.MatchString(typ) {
			return s, refuseWebField("mime_types", "%q is not a MIME type such as application/x-example", mt.Type)
		}
		if len(mt.Extensions) == 0 || len(mt.Extensions) > maxSiteMIMEExts {
			return s, refuseWebField("mime_types", "%s needs 1 to %d file extensions", typ, maxSiteMIMEExts)
		}
		entry := transport.SiteMIMEType{Type: typ}
		for _, ext := range mt.Extensions {
			ext = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(ext), "."))
			if !siteMIMEExtPattern.MatchString(ext) {
				return s, refuseWebField("mime_types", "%q is not a file extension", ext)
			}
			if seenExt[ext] {
				return s, refuseWebField("mime_types", "the extension .%s is listed twice", ext)
			}
			seenExt[ext] = true
			entry.Extensions = append(entry.Extensions, ext)
		}
		out.MIMETypes = append(out.MIMETypes, entry)
	}

	if !validBrowserCacheDays(s.BrowserCacheDays) {
		return s, refuseWebField("browser_cache_days", "browser caching is off, 1, 7 or 30 days")
	}
	out.BrowserCacheDays = s.BrowserCacheDays

	if len(s.Headers) > maxSiteHeaders {
		return s, refuseWebField("headers", "at most %d headers", maxSiteHeaders)
	}
	seenHeader := map[string]bool{}
	for _, h := range s.Headers {
		name := strings.TrimSpace(h.Name)
		if !siteHeaderPattern.MatchString(name) {
			return s, refuseWebField("headers", "%q is not a header name (letters, digits and dashes)", h.Name)
		}
		lower := strings.ToLower(name)
		if why, refused := siteHeaderRefused[lower]; refused {
			return s, refuseWebField("headers", "the %s header cannot be set here: %s", name, why)
		}
		if seenHeader[lower] {
			return s, refuseWebField("headers", "the %s header is listed twice", name)
		}
		seenHeader[lower] = true
		value := strings.TrimSpace(h.Value)
		if value == "" || len(value) > maxSiteHeaderValue {
			return s, refuseWebField("headers", "the value of %s is 1 to %d characters", name, maxSiteHeaderValue)
		}
		for _, r := range value {
			if r < 0x20 || r == 0x7f {
				return s, refuseWebField("headers", "the value of %s has a line break or a control character", name)
			}
		}
		if strings.Contains(value, "$") {
			return s, refuseWebField("headers", "the value of %s cannot contain $", name)
		}
		out.Headers = append(out.Headers, transport.SiteHeader{Name: name, Value: value})
	}

	switch s.AccessMode {
	case "":
		if len(s.AccessList) > 0 {
			return s, refuseWebField("access", "choose whether the list is allowed or denied")
		}
	case "allow", "deny":
		if len(s.AccessList) == 0 {
			return s, refuseWebField("access", "the list of addresses is empty")
		}
		if len(s.AccessList) > maxSiteAccessList {
			return s, refuseWebField("access", "at most %d addresses or networks", maxSiteAccessList)
		}
		seen := map[string]bool{}
		for _, raw := range s.AccessList {
			entry, err := normalizedAccessEntry(raw)
			if err != nil {
				return s, refuseWebField("access", "%s", err.Error())
			}
			if !seen[entry] {
				seen[entry] = true
				out.AccessList = append(out.AccessList, entry)
			}
		}
		out.AccessMode = s.AccessMode
	default:
		return s, refuseWebField("access", "access is everybody, only the listed addresses, or everybody but them")
	}

	if s.NodePublic {
		if !node {
			return s, refuseWebField("node_public", "serving public/ directly applies to Node.js sites")
		}
		out.NodePublic = true
	}

	// G28: the page cache (the Panel lets only the administrator set it).
	if s.PageCache {
		if !PageCacheApplies(projectType) {
			return s, refuseWebField("page_cache", "the page cache applies to PHP sites and sites served by an application")
		}
		out.PageCache = true
	}
	if !validPageCacheMinutes(s.PageCacheMinutes) {
		return s, refuseWebField("page_cache", "a cached page is used for 1 to 1440 minutes")
	}
	if out.PageCache {
		out.PageCacheMinutes = s.PageCacheMinutes
	}

	// G26: the site's own error pages.
	if len(s.ErrorPages) > 0 {
		if !index {
			return s, refuseWebField("error_pages", "custom error pages apply to PHP and static sites; a Node.js application answers its own errors")
		}
		seenCode := map[int]bool{}
		for _, page := range s.ErrorPages {
			if !siteErrorCodes[page.Code] {
				return s, refuseWebField("error_pages", "%d is not an error code a page can be set for", page.Code)
			}
			if seenCode[page.Code] {
				return s, refuseWebField("error_pages", "the error %d is listed twice", page.Code)
			}
			seenCode[page.Code] = true
			p := strings.TrimSpace(page.Path)
			if !siteErrorPagePattern.MatchString(p) || strings.Contains(p, "..") || strings.Contains(p, "//") || strings.Contains(p, "/.") {
				return s, refuseWebField("error_pages", "%q is not a page of this site such as /errors/404.html (an .html file)", page.Path)
			}
			out.ErrorPages = append(out.ErrorPages, transport.SiteErrorPage{Code: page.Code, Path: p})
		}
		sort.Slice(out.ErrorPages, func(i, j int) bool { return out.ErrorPages[i].Code < out.ErrorPages[j].Code })
	}
	return out, nil
}

// siteErrorCodes are the errors a site can give its own page for.
var siteErrorCodes = map[int]bool{400: true, 401: true, 403: true, 404: true, 405: true, 410: true, 429: true,
	500: true, 502: true, 503: true, 504: true}

// SiteErrorCodes lists them in order (for the screen).
func SiteErrorCodes() []int {
	codes := make([]int, 0, len(siteErrorCodes))
	for code := range siteErrorCodes {
		codes = append(codes, code)
	}
	sort.Ints(codes)
	return codes
}

var siteErrorPagePattern = regexp.MustCompile(`^/[A-Za-z0-9._~/-]{0,250}\.html?$`)

// SiteErrorPageFile is the file of an error page in the document root.
func SiteErrorPageFile(documentRoot, page string) string {
	return strings.TrimSuffix(documentRoot, "/") + page
}

// ApplicableSiteWebSettings drops the fields a project type cannot use (a
// render after the site's type changed must never fail on them; the values
// come back if the type does) and validates the rest.
func ApplicableSiteWebSettings(projectType string, s SiteWebSettings) (SiteWebSettings, error) {
	index, any, node := SiteWebFieldsApply(projectType)
	if !any {
		return SiteWebSettings{}, nil
	}
	if !index {
		s.IndexFiles = nil
		s.ErrorPages = nil
	}
	if !node {
		s.NodePublic = false
	}
	if !PageCacheApplies(projectType) {
		s.PageCache, s.PageCacheMinutes = false, 0
	}
	return PrepareSiteWebSettings(projectType, s)
}

func validBrowserCacheDays(days int) bool {
	for _, choice := range BrowserCacheChoices {
		if days == choice {
			return true
		}
	}
	return false
}

func siteWebSettingsEmpty(s SiteWebSettings) bool {
	return len(s.IndexFiles) == 0 && len(s.MIMETypes) == 0 && s.BrowserCacheDays == 0 && len(s.Headers) == 0 &&
		s.AccessMode == "" && len(s.AccessList) == 0 && !s.NodePublic && len(s.ErrorPages) == 0 && !s.PageCache
}

// normalizedAccessEntry is one IP address or network in nginx's form.
func normalizedAccessEntry(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if strings.Contains(value, "/") {
		ip, network, err := net.ParseCIDR(value)
		if err != nil {
			return "", fmt.Errorf("%q is not an IP address or a network such as 203.0.113.0/24", raw)
		}
		ones, _ := network.Mask.Size()
		if ones == 0 {
			return "", fmt.Errorf("%q covers every address; leave the list empty instead", raw)
		}
		_ = ip
		return network.String(), nil
	}
	ip := net.ParseIP(value)
	if ip == nil {
		return "", fmt.Errorf("%q is not an IP address or a network such as 203.0.113.0/24", raw)
	}
	return ip.String(), nil
}

// DefaultIndexFiles is the template's index line of a project type.
func DefaultIndexFiles(projectType string) string {
	if projectType == "static" {
		return "index.html index.htm"
	}
	return "index.php index.html index.htm"
}

// SiteIncludeLine is the content-only include line of a site's vhost.
func SiteIncludeLine(domain string) string {
	return "include " + PanelManagedDir(domain) + "/*.site;"
}

// SitePartPath is one of the site's files in its Panel-owned directory.
func SitePartPath(domain, name string) string {
	return PanelManagedDir(domain) + "/" + name
}

// WebHTTPConfPath is the http-level file the site files' variables need.
func WebHTTPConfPath() string {
	return filepath.Join(nginxBase(), "conf.d", WebHTTPConfPartName)
}

// siteStaticContentTypes are the answers browser caching applies to: files a
// browser can keep. text/html (every PHP page) is never among them.
var siteStaticContentTypes = []string{
	`~^image/`, `~^font/`, `~^video/`, `~^audio/`, `~^text/css`, `~^text/javascript`,
	`~^application/javascript`, `~^application/x-javascript`, `~^application/pdf`,
	`~^application/wasm`, `~^application/(x-)?font`, `~^application/vnd\.ms-fontobject`,
}

// RenderWebHTTPConf is the body of WebHTTPConfPath (http context).
func RenderWebHTTPConf() string {
	var b strings.Builder
	b.WriteString("# CelikPanel's web server settings: the maps the sites' settings.site files\n")
	b.WriteString("# use. Plain nginx configuration. Browser caching applies to static files\n")
	b.WriteString("# only (images, fonts, styles, scripts, media); a PHP page is never cached by\n")
	b.WriteString("# the browser. CelikPanel writes this file only while it is CelikPanel's\n")
	b.WriteString("# unchanged text.\n")
	b.WriteString("# CelikPanel web sunucusu ayarları: sitelerin settings.site dosyalarının\n")
	b.WriteString("# kullandığı eşlemeler. Tarayıcı önbelleği yalnız statik dosyalara uygulanır.\n")
	for _, days := range BrowserCacheChoices {
		if days == 0 {
			continue
		}
		fmt.Fprintf(&b, "map $sent_http_content_type %s%dd {\n    default off;\n", siteExpiresVariable, days)
		for _, typ := range siteStaticContentTypes {
			fmt.Fprintf(&b, "    %s %dd;\n", typ, days)
		}
		b.WriteString("}\n")
	}
	// G28: the page cache's bypass.
	b.WriteString(pageCacheMaps())
	return b.String()
}

// siteHeaderValue quotes a header value for nginx.
func siteHeaderValue(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return `"` + value + `"`
}

// RenderSiteSettingsPart is the body of settings.site for prepared settings.
// withHTTP=false: the http-level file is the owner's, so nothing that needs
// its maps is written.
func RenderSiteSettingsPart(domain string, s SiteWebSettings, withHTTP bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Web server settings of %s, written by CelikPanel (Hosting → Web server).\n", domain)
	fmt.Fprintf(&b, "# This directory is CelikPanel's; put your own additions in %s/*.conf.\n", OwnerIncludeDir(domain))
	b.WriteString("# Web sunucusu ayarları; bu dizin CelikPanel'indir.\n")
	if len(s.MIMETypes) > 0 {
		// The map of nginx's package first: a types block here replaces the
		// inherited one, so the server's own types must be read again.
		b.WriteString("include mime.types;\ntypes {\n")
		for _, mt := range s.MIMETypes {
			fmt.Fprintf(&b, "    %s %s;\n", mt.Type, strings.Join(mt.Extensions, " "))
		}
		b.WriteString("}\n")
	}
	if s.BrowserCacheDays > 0 {
		if withHTTP {
			fmt.Fprintf(&b, "expires %s%dd;\n", siteExpiresVariable, s.BrowserCacheDays)
		} else {
			fmt.Fprintf(&b, "# Browser caching (%d days) is not written: %s is not CelikPanel's file.\n", s.BrowserCacheDays, WebHTTPConfPath())
		}
	}
	for _, h := range s.Headers {
		fmt.Fprintf(&b, "add_header %s %s always;\n", h.Name, siteHeaderValue(h.Value))
	}
	// G26: nginx's own errors answered with the site's pages (the files
	// were checked to exist when the settings were saved).
	for _, page := range s.ErrorPages {
		fmt.Fprintf(&b, "error_page %d %s;\n", page.Code, page.Path)
	}
	switch s.AccessMode {
	case "allow":
		for _, entry := range s.AccessList {
			fmt.Fprintf(&b, "allow %s;\n", entry)
		}
		b.WriteString("deny all;\n")
	case "deny":
		for _, entry := range s.AccessList {
			fmt.Fprintf(&b, "deny %s;\n", entry)
		}
	}
	return b.String()
}

// RenderSiteDirectivesPart is the body of directives.site ("" for no text:
// the file is then removed when it is CelikPanel's unchanged text).
func RenderSiteDirectivesPart(domain, text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	if strings.TrimSpace(text) == "" {
		return ""
	}
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	return fmt.Sprintf("# Additional nginx directives of %s, saved in CelikPanel (Hosting → Web server).\n", domain) +
		"# Server context, read only where the site's content is served. An edit here is\n" +
		"# kept: CelikPanel then shows it read-only and does not overwrite it.\n" +
		"# Ek nginx yönergeleri; burada yapılan değişiklik korunur.\n" + text
}

// SitePart is one file written with a site's vhost.
type SitePart struct {
	Name string
	Path string
	// Body "" removes CelikPanel's unchanged file (an owner's is kept).
	Body string
	// NeedsWebHTTP: the body uses the maps of WebHTTPConfPath; when that
	// file is the owner's, FallbackBody is written instead.
	NeedsWebHTTP bool
	FallbackBody string
	// EnsureDir, when set, is made (0755) before the part is written: the
	// page cache's parent folder, which nginx -t needs to exist.
	EnsureDir string
}

// SitePartsFor renders the site's files for prepared settings (no page
// cache: SitePartsForSite adds it).
func SitePartsFor(domain string, s SiteWebSettings, directives string) []SitePart {
	s.PageCache = false
	return SitePartsForSite(domain, 0, "", s, directives)
}

// SitePartsForSite renders the site's files, with the page cache of the
// site's identity and project type (G28) when it is on.
func SitePartsForSite(domain string, siteID int, projectType string, s SiteWebSettings, directives string) []SitePart {
	cache := s.PageCache && siteID > 0 && PageCacheApplies(projectType)
	body := RenderSiteSettingsPart(domain, s, true)
	settings := SitePart{Name: SiteSettingsPartName, Path: SitePartPath(domain, SiteSettingsPartName), Body: body}
	if cache {
		settings.Body += renderPageCacheLines(siteID, projectType, s.PageCacheMinutes)
	}
	if s.BrowserCacheDays > 0 || cache {
		// The fallback (the maps file is the owner's) has neither browser
		// caching nor the page cache: both need its maps.
		settings.NeedsWebHTTP = true
		settings.FallbackBody = RenderSiteSettingsPart(domain, s, false)
	}
	parts := []SitePart{settings, {
		Name: SiteDirectivesPartName, Path: SitePartPath(domain, SiteDirectivesPartName),
		Body: RenderSiteDirectivesPart(domain, directives),
	}}
	if siteID > 0 && PageCacheApplies(projectType) {
		parts = append(parts, pageCacheHTTPPart(domain, siteID, projectType, cache))
	}
	return parts
}

// SitePartState is a site file's state, read only: "absent", "unchanged"
// (CelikPanel's text), or "kept" (anything else), and the file's text
// without the header when it is CelikPanel's or the raw text when kept.
func SitePartState(domain, name string) (string, string) {
	path := SitePartPath(domain, name)
	inspected := inspectSiteFile(path, nil, "")
	switch inspected.state {
	case transport.SiteFileAbsent:
		return "absent", ""
	case transport.SiteFileManagedUnchanged:
		return "unchanged", string(stripManagedHeader(inspected.content))
	}
	return "kept", string(inspected.content)
}

// planSitePart writes or removes one part; nil plan when nothing changed.
func planSitePart(domain string, part SitePart, body string) (*plannedTrafficFile, string, error) {
	inspected := inspectSiteFile(part.Path, nil, "")
	if body == "" {
		switch inspected.state {
		case transport.SiteFileAbsent:
			return nil, transport.SiteFileChallengeUnchanged, nil
		case transport.SiteFileManagedUnchanged:
			if err := os.Remove(part.Path); err != nil {
				return nil, transport.SiteFileChallengeFailed, err
			}
			return &plannedTrafficFile{path: part.Path, inspected: inspected}, transport.SitePartRemoved, nil
		}
		return nil, transport.SiteFileChallengeKept, nil
	}
	sealed := []byte(SealManagedText(body))
	inspected, state := managedPartState(part.Path, sealed)
	switch state {
	case transport.SiteFileChallengeUnchanged, transport.SiteFileChallengeKept:
		return nil, state, nil
	}
	if part.EnsureDir != "" {
		if err := os.MkdirAll(part.EnsureDir, 0o755); err != nil {
			return nil, transport.SiteFileChallengeFailed, err
		}
	}
	if err := os.MkdirAll(filepath.Dir(part.Path), 0o755); err != nil {
		return nil, transport.SiteFileChallengeFailed, err
	}
	if strings.HasPrefix(part.Path, PanelManagedDir(domain)+"/") {
		if err := ensurePanelManagedDir(domain); err != nil {
			return nil, transport.SiteFileChallengeFailed, err
		}
	}
	if err := writeManagedSiteFile(part.Path, sealed, inspected); err != nil {
		return nil, transport.SiteFileChallengeFailed, err
	}
	return &plannedTrafficFile{path: part.Path, sealed: sealed, inspected: inspected}, transport.SiteFileChallengeWritten, nil
}

// planWebHTTPConf writes the http-level file when absent or CelikPanel's
// other text. ok=false: it is the owner's.
func planWebHTTPConf() (*plannedTrafficFile, bool, error) {
	path := WebHTTPConfPath()
	sealed := []byte(SealManagedText(RenderWebHTTPConf()))
	inspected, state := managedPartState(path, sealed)
	switch state {
	case transport.SiteFileChallengeUnchanged:
		return nil, true, nil
	case transport.SiteFileChallengeKept:
		return nil, false, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, false, err
	}
	if err := writeManagedSiteFile(path, sealed, inspected); err != nil {
		return nil, false, err
	}
	return &plannedTrafficFile{path: path, sealed: sealed, inspected: inspected}, true, nil
}

// planSitePartsOf plans one item's parts (the vhost after the operation
// reads the content-only include point). It returns the planned writes and
// fills outcomes.
func planSitePartsOf(item ManagedVhostItem, outcomes map[string]string, webHTTP *webHTTPPlan) ([]*plannedTrafficFile, error) {
	var planned []*plannedTrafficFile
	var errs []error
	for _, part := range item.SiteParts {
		body := part.Body
		if part.NeedsWebHTTP {
			ok, err := webHTTP.ensure()
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", WebHTTPConfPartName, err))
			}
			if !ok {
				body = part.FallbackBody
			}
		}
		file, outcome, err := planSitePart(item.Domain, part, body)
		if part.NeedsWebHTTP && !webHTTP.ok && outcome != transport.SiteFileChallengeFailed && outcome != transport.SiteFileChallengeKept {
			outcome = transport.SitePartHTTPKept
		}
		outcomes[part.Name] = outcome
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", part.Name, err))
			continue
		}
		if file != nil {
			planned = append(planned, file)
		}
	}
	return planned, errors.Join(errs...)
}

// webHTTPPlan plans the http-level file once per batch.
type webHTTPPlan struct {
	checked bool
	ok      bool
	file    *plannedTrafficFile
	err     error
}

func (w *webHTTPPlan) ensure() (bool, error) {
	if !w.checked {
		w.checked = true
		w.file, w.ok, w.err = planWebHTTPConf()
		if w.err != nil {
			w.ok = false
		}
	}
	return w.ok, w.err
}

// removeSiteParts removes a deleted site's files when they are still
// CelikPanel's unchanged text (called before the directory itself).
func removeSiteParts(domain string) {
	for _, name := range []string{SiteSettingsPartName, SiteDirectivesPartName} {
		path := SitePartPath(domain, name)
		if inspected := inspectSiteFile(path, nil, ""); inspected.state == transport.SiteFileManagedUnchanged {
			_ = os.Remove(path)
		}
	}
	removeSiteHTTPParts(domain)
}

// removeSiteHTTPParts is extended by the page cache (its http-level file).
var removeSiteHTTPParts = func(domain string) {}

// SiteHomeOf is a site's home: the parent of its document root.
func SiteHomeOf(documentRoot string) string {
	return filepath.ToSlash(filepath.Dir(strings.TrimSuffix(documentRoot, "/")))
}

// sortedOutcomeNames lists outcome names in a stable order (for messages).
func sortedOutcomeNames(outcomes map[string]string) []string {
	names := make([]string, 0, len(outcomes))
	for name := range outcomes {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// BrowserCacheLabel is "7 days" (for logs and audit details).
func BrowserCacheLabel(days int) string {
	if days == 0 {
		return "off"
	}
	return strconv.Itoa(days) + " days"
}
