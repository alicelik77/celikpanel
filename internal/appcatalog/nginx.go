package appcatalog

import (
	"fmt"
	"strings"
)

// The nginx rules an application needs, written to the site's CelikPanel
// include directory (/etc/nginx/celikpanel-managed.d/<domain>/app-<id>.conf,
// D-031). The vhost includes that directory in EVERY server block of the site:
// the one that serves the site, the plain-HTTP block that only redirects to
// HTTPS, the validation-only block of mail.<domain>, and the suspended and
// maintenance blocks. So the rules here never serve content on their own:
//
//   - no root, no fastcgi_pass, no proxy: a request is denied (404), redirected
//     inside the site, or handed back to the vhost's own PHP location by an
//     internal rewrite. In a block that serves nothing (suspended, maintenance,
//     redirect-only) the rewritten request reaches that block's own answer
//     (the suspended page, the redirect), never a PHP file;
//   - only regular-expression and exact locations: a prefix location could
//     collide with the owner's protected folders ("duplicate location").
//
// Regular-expression locations are tried in the order they appear and the
// include comes before the vhost's own, so a deny here wins over the vhost's
// "\.php$" location; the ACME challenge (a ^~ prefix) is never touched.
//
// "Path info" requests (/remote.php/dav/..., /index.php/apps/...,
// /admin123/index.php/sell/...) are rewritten to the script itself; the
// applications in the catalog read the rest from REQUEST_URI, which keeps
// the original address.
//
// Uygulamanın nginx kuralları sitenin CelikPanel ekleme dizinine yazılır. Bu
// dizin sitenin HER sunucu bloğunda okunur (askı, bakım ve yalnız yönlendiren
// bloklar dahil); bu yüzden kurallar kendi başına içerik sunmaz: kök yok,
// fastcgi_pass yok; istek reddedilir, site içinde yönlendirilir ya da iç
// yeniden yazımla vhost'un kendi PHP konumuna geri verilir.

// IncludeFileName is the file name of an application's rules in the site's
// CelikPanel include directory.
func IncludeFileName(id string) string { return "app-" + id + ".conf" }

const pathInfoRule = `
# "Path info" addresses (script.php/rest) run the script itself; the
# application reads the rest from the original address.
# Yol bilgili adresler betiğin kendisine verilir.
location ~ ^(?<celikpanel_app_script>/.+?\.php)/ {
    rewrite ^ $celikpanel_app_script last;
}
`

var nginxRules = map[string]string{
	"nextcloud": `
# Calendar and contact clients find the server here (Nextcloud's own rules).
location = /.well-known/carddav { return 301 /remote.php/dav/; }
location = /.well-known/caldav { return 301 /remote.php/dav/; }
location = /.well-known/webfinger { return 301 /index.php$uri; }
location = /.well-known/nodeinfo { return 301 /index.php$uri; }

# Never served: code, configuration and (if moved here) data.
# Asla sunulmaz: kod, yapılandırma ve veri.
location ~ ^/(?:build|tests|config|lib|3rdparty|templates|data)(?:$|/) { return 404; }
location ~ ^/(?:\.|autotest|occ|issue|indie|db_|console) { return 404; }

# Browsers need JavaScript modules served as JavaScript.
location ~ \.mjs$ {
    types { text/javascript mjs; }
    try_files $uri =404;
}
`,
	"drupal": `
# Never served: PHP under the uploaded files, private files, vendor code,
# configuration and templates (Drupal's own rules).
# Asla sunulmaz: yüklenen dosyalar altındaki PHP, özel dosyalar, kod ve şablonlar.
location ~ ^/sites/[^/]+/files/.*\.php$ { return 404; }
location ~ ^/sites/.*/private/ { return 404; }
location ~ /vendor/.*\.php$ { return 404; }
location ~ \..*/.*\.php$ { return 403; }
location ~* \.(?:engine|inc|install|make|module|profile|po|sh|sql|theme|twig|tpl|xtmpl|yml)$ { return 404; }
location ~* (?:^|/)(?:composer\.(?:json|lock)|web\.config)$ { return 404; }
`,
	"joomla": `
# Never run scripts from upload, cache and log folders; never serve logs.
# Yükleme, önbellek ve günlük klasörlerinde betik çalışmaz.
location ~* ^/(?:images|cache|media|logs|tmp)/.*\.(?:php|pl|py|jsp|asp|sh|cgi)$ { return 403; }
location ~ ^/(?:administrator/)?(?:logs|tmp)/ { return 404; }
location ~* (?:^|/)(?:composer\.(?:json|lock)|package(?:-lock)?\.json|web\.config\.txt|htaccess\.txt)$ { return 404; }
`,
	"prestashop": `
# Never served: application code, configuration, caches and templates
# (PrestaShop's own rules).
# Asla sunulmaz: uygulama kodu, yapılandırma, önbellek ve şablonlar.
location ~ ^/(?:app|bin|cache|classes|config|controllers|docs|localization|mails|override|pdf|src|tests|tools|translations|var|vendor)/ { return 404; }
location ~ ^/modules/.*/(?:vendor|config)/ { return 404; }
location ~ ^/(?:img|upload|download)/.*\.php$ { return 404; }
location ~* \.(?:tpl|twig|yml|yaml|log|sql|md|dist|lock)$ { return 404; }
location ~* (?:^|/)composer\.json$ { return 404; }
`,
	"mediawiki": `
# Never served: code, caches, maintenance scripts and the web installer
# (setup ran on the server; LocalSettings.php is the configuration).
# Asla sunulmaz: kod, önbellek, bakım betikleri ve web kurucusu.
location ~ ^/(?:cache|includes|maintenance|languages|serialized|tests|vendor|docs|mw-config|images/deleted)(?:$|/) { return 404; }
location ~ ^/images/.*\.(?:php|phtml|php[0-9])$ { return 404; }
location ~ ^/(?:LocalSettings\.php|composer\.(?:json|lock))$ { return 404; }
`,
	"phpbb": `
# Never served: configuration, code, caches, uploads and stored files
# (phpBB's own rules).
# Asla sunulmaz: yapılandırma, kod, önbellek ve saklanan dosyalar.
location ~ ^/(?:config\.php|common\.php|cache|files|images/avatars/upload|includes|phpbb|store|vendor|install)(?:$|/) { return 404; }
`,
}

// HasNginxRules reports whether the application has rules of its own.
func (a App) HasNginxRules() bool {
	_, ok := nginxRules[a.Nginx]
	return ok && a.Nginx != ""
}

// RenderNginx is the body of the application's include file for one site
// (the caller adds CelikPanel's render header).
func (a App) RenderNginx(domain string) string {
	rules, ok := nginxRules[a.Nginx]
	if !ok {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# %s %s on %s, written by CelikPanel's application installer.\n", a.Name, a.Version, domain)
	b.WriteString("# CelikPanel removes this file when the application is removed. Put your own\n")
	b.WriteString("# additions in the site's own include directory (celikpanel-sites.d).\n")
	fmt.Fprintf(&b, "# %s %s, CelikPanel uygulama kurucusu yazdı; uygulama kaldırılınca silinir.\n", a.Name, a.Version)
	if a.UploadMB > 0 {
		fmt.Fprintf(&b, "\n# Uploads up to %d MB / %d MB'a kadar yükleme\nclient_max_body_size %dm;\n", a.UploadMB, a.UploadMB, a.UploadMB)
	}
	b.WriteString(rules)
	b.WriteString(pathInfoRule)
	return b.String()
}
