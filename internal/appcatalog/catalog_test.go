package appcatalog

import (
	"regexp"
	"strings"
	"testing"
)

var sha256Hex = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Every pinned entry is complete: an https address, a SHA-256, a known
// format, markers, a configuration file for rotation and nginx rules.
func TestCatalogEntriesArePinnedAndComplete(t *testing.T) {
	seen := map[string]bool{}
	for _, app := range All() {
		if !ValidID(app.ID) || seen[app.ID] || app.ID == "setup-lock" {
			t.Fatalf("invalid or duplicate id %q", app.ID)
		}
		seen[app.ID] = true
		if id, ok := IDFromOffering(OfferingID(app.ID)); !ok || id != app.ID {
			t.Fatalf("%s: offering id does not round-trip", app.ID)
		}
		if app.Kind != KindPHPSite || app.Name == "" || app.Version == "" || app.License == "" || app.Risk == "" || app.ConfigFile == "" {
			t.Fatalf("%s: incomplete entry %+v", app.ID, app)
		}
		if !strings.HasPrefix(app.Archive.URL, "https://") {
			t.Fatalf("%s: archive address is not https", app.ID)
		}
		if app.Legacy() {
			continue
		}
		if !sha256Hex.MatchString(app.Archive.SHA256) {
			t.Fatalf("%s: no pinned SHA-256", app.ID)
		}
		if strings.Contains(app.Archive.URL, "latest") {
			t.Fatalf("%s: a pinned release must not follow a moving address", app.ID)
		}
		switch app.Archive.Format {
		case FormatTarGz, FormatTarBz2, FormatZip:
		default:
			t.Fatalf("%s: unknown format %q", app.ID, app.Archive.Format)
		}
		if len(app.Archive.Markers) == 0 || app.Archive.MaxDownloadBytes <= 0 || app.Archive.MaxExtractedBytes <= 0 || app.Archive.MaxEntries <= 0 {
			t.Fatalf("%s: markers and limits are required", app.ID)
		}
		if app.Setup == "" || !app.HasNginxRules() || len(app.Databases) == 0 || app.PHPMin == "" || app.Archive.Upstream == "" {
			t.Fatalf("%s: setup, nginx rules, database, PHP floor and the upstream check are required", app.ID)
		}
	}
	for _, id := range []string{"wordpress", "prestashop", "nextcloud", "drupal", "joomla"} {
		if !seen[id] {
			t.Fatalf("the catalog lacks %s", id)
		}
	}
	if len(DefaultOffered) != 1 || DefaultOffered[0] != "wordpress" {
		t.Fatalf("only WordPress is offered before the administrator chose: %v", DefaultOffered)
	}
}

func TestPHPSupportedNamesTheBound(t *testing.T) {
	nextcloud, _ := Lookup("nextcloud")
	if ok, reason := nextcloud.PHPSupported("8.2.29"); ok || !strings.Contains(reason, "8.3 or newer") || !strings.Contains(reason, "8.2") {
		t.Fatalf("PHP 8.2: ok=%v reason=%q", ok, reason)
	}
	if ok, _ := nextcloud.PHPSupported("8.4"); !ok {
		t.Fatal("PHP 8.4 must be accepted")
	}
	if ok, reason := nextcloud.PHPSupported("8.6"); ok || !strings.Contains(reason, "up to 8.5") {
		t.Fatalf("PHP 8.6: ok=%v reason=%q", ok, reason)
	}
	if ok, _ := nextcloud.PHPSupported(""); ok {
		t.Fatal("an unknown PHP version must not pass")
	}
}

func TestExtensionSourceNamesTheDistributionPackage(t *testing.T) {
	cases := []struct {
		family, line, ext string
		want              MissingExtension
	}{
		{"apt", "8.3", "intl", MissingExtension{Extension: "intl", Package: "php8.3-intl"}},
		{"apt", "8.4", "pdo_mysql", MissingExtension{Extension: "pdo_mysql", Package: "php8.4-mysql"}},
		{"apt", "8.2", "dom", MissingExtension{Extension: "dom", Package: "php8.2-xml"}},
		{"apt", "8.3", "fileinfo", MissingExtension{Extension: "fileinfo", Package: "php8.3-common"}},
		{"apt", "8.3", "json", MissingExtension{Extension: "json", Enable: true}},
		{"pacman", "8.4", "intl", MissingExtension{Extension: "intl", Package: "php-intl"}},
		{"pacman", "8.4", "mysqli", MissingExtension{Extension: "mysqli", Enable: true}},
	}
	for _, c := range cases {
		if got := ExtensionSource(c.family, c.line, c.ext); got != c.want {
			t.Fatalf("%s %s %s: got %+v want %+v", c.family, c.line, c.ext, got, c.want)
		}
	}
}

var locationLine = regexp.MustCompile(`(?m)^\s*location\s+(\S+)`)

// The rules land in every server block of the site (the suspended,
// maintenance and redirect-only blocks too), so they never serve content on
// their own and never use a prefix location an owner's protected folder could
// duplicate.
func TestNginxRulesNeverServeContentOnTheirOwn(t *testing.T) {
	wants := map[string][]string{
		"nextcloud":  {"location = /.well-known/carddav { return 301 /remote.php/dav/; }", "(?:build|tests|config|lib|3rdparty|templates|data)", "client_max_body_size 512m;"},
		"drupal":     {`^/sites/[^/]+/files/.*\.php$`, "/vendor/.*"},
		"joomla":     {`(?:images|cache|media|logs|tmp)`},
		"prestashop": {"(?:app|bin|cache|classes|config", "client_max_body_size 64m;"},
		"mediawiki":  {"mw-config", "LocalSettings"},
		"phpbb":      {`config\.php`, "install"},
	}
	for id, needles := range wants {
		app, ok := Lookup(id)
		if !ok {
			t.Fatalf("no %s", id)
		}
		full := app.RenderNginx("shop.example.test")
		directives := []string{}
		for _, line := range strings.Split(full, "\n") {
			if trimmed := strings.TrimSpace(line); trimmed != "" && !strings.HasPrefix(trimmed, "#") {
				directives = append(directives, line)
			}
		}
		body := strings.Join(directives, "\n")
		for _, forbidden := range []string{"\nroot ", " root ", "fastcgi_pass", "proxy_pass", "alias ", "include "} {
			if strings.Contains(body, forbidden) {
				t.Fatalf("%s: rules contain %q:\n%s", id, forbidden, body)
			}
		}
		for _, m := range locationLine.FindAllStringSubmatch(body, -1) {
			if m[1] != "~" && m[1] != "~*" && m[1] != "=" {
				t.Fatalf("%s: a %q location could collide with the site's own locations", id, m[1])
			}
		}
		if !strings.Contains(body, `rewrite ^ $celikpanel_app_script last;`) {
			t.Fatalf("%s: the path-info rule is missing", id)
		}
		for _, needle := range needles {
			if !strings.Contains(body, needle) {
				t.Fatalf("%s: missing %q:\n%s", id, needle, body)
			}
		}
		if strings.Count(body, "{") != strings.Count(body, "}") {
			t.Fatalf("%s: unbalanced braces", id)
		}
	}
	if wp, _ := Lookup("wordpress"); wp.HasNginxRules() || wp.RenderNginx("x.test") != "" {
		t.Fatal("WordPress keeps the vhost's own rules")
	}
}
