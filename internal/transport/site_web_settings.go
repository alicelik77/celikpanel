package transport

// Web server settings of one site (alpha.128, PLESK-GAP G3, G19, G26, G28;
// SITE-SETTINGS-DESIGN §2.4). The Panel keeps the site's choices and sends
// them with every render of the site's vhost (ApplyVhostRequest.WebSettings),
// so no flow that re-renders (startup, certificates, settings) can drop them.
// The Agent validates them again (services.PrepareSiteWebSettings) and writes
// them as plain nginx files in the site's Panel-owned directory, read only by
// the server blocks that serve the site's content:
//
//	/etc/nginx/celikpanel-managed.d/<domain>/settings.site    the fields
//	/etc/nginx/celikpanel-managed.d/<domain>/directives.site  additional directives
//
// Both are CelikPanel's files with the render header (D-031): an edited file
// is kept and named, never overwritten. Without the Panel nginx keeps reading
// them; the site keeps working exactly as configured (D-022).
//
// Bir sitenin web sunucusu ayarları: Panel seçimleri saklar ve her vhost
// üretiminde gönderir; Agent yeniden doğrular ve sitenin Panel dizinine düz
// nginx dosyaları olarak yazar. Düzenlenmiş dosya korunur.

// SiteMIMEType is one extra content type and its file extensions.
type SiteMIMEType struct {
	Type       string   `json:"type"`
	Extensions []string `json:"extensions"`
}

// SiteHeader is one response header the site adds.
type SiteHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// SiteWebSettings is everything a site chose. A zero value is "default" for
// every field and renders nothing but the file's comment.
type SiteWebSettings struct {
	// IndexFiles replaces the template's fixed index line (php: index.php
	// index.html index.htm; static: index.html index.htm). Empty: default.
	IndexFiles []string `json:"index_files,omitempty"`
	// MIMETypes are added to nginx's own map (rendered after `include
	// mime.types;`, so the map is extended, never replaced).
	MIMETypes []SiteMIMEType `json:"mime_types,omitempty"`
	// BrowserCacheDays: 0 (off), 1, 7 or 30. Static files only, through a
	// content-type map: a PHP page is never cached by the browser.
	BrowserCacheDays int `json:"browser_cache_days,omitempty"`
	// Headers are added to every answer (add_header ... always).
	Headers []SiteHeader `json:"headers,omitempty"`
	// AccessMode: "" (everybody), "allow" (only AccessList) or "deny"
	// (everybody but AccessList). Certificate validation stays reachable.
	AccessMode string   `json:"access_mode,omitempty"`
	AccessList []string `json:"access_list,omitempty"`
	// NodePublic (Node.js sites, G19): nginx serves the files of the
	// application's public/ folder itself and hands every other address to
	// the application. The application folder itself is never served.
	NodePublic bool `json:"node_public,omitempty"`
}

// SiteWebSettingsRequest travels with a vhost render. Settings nil: the site
// has never saved a web server setting, and its vhost renders exactly as
// before this release (no content-only include line, nothing written).
type SiteWebSettingsRequest struct {
	Settings SiteWebSettings `json:"settings"`
	// Directives is the additional nginx directives text ("" for none).
	Directives string `json:"directives,omitempty"`
	// DirectivesTrusted: the text was saved by the administrator, so only
	// the parse, size and nginx -t checks apply (never the forbidden list).
	DirectivesTrusted bool `json:"directives_trusted,omitempty"`
	// PrecheckNginx: run `nginx -t` before writing, so a configuration that
	// already fails is reported as the server's prerequisite and nothing is
	// written (set by the save of the settings themselves).
	PrecheckNginx bool `json:"precheck_nginx,omitempty"`
}

// Site part outcomes in SiteFileResult.SiteParts besides the challenge words
// (written, unchanged, kept, failed).
const (
	// SitePartRemoved: CelikPanel's unchanged file was removed (no text).
	SitePartRemoved = "removed"
	// SitePartNotRead: the site's vhost after this operation does not read
	// the content-only include point (the owner kept a file without the
	// line), so nothing was written.
	SitePartNotRead = "not_read"
	// SitePartHTTPKept: the http-level file the field needs is the owner's,
	// so the field was left out of the site's file.
	SitePartHTTPKept = "http_kept"
	// SitePartNginxFailingBefore: `nginx -t` already failed before the change;
	// nothing was written.
	SitePartNginxFailingBefore = "nginx_failing_before"
)
