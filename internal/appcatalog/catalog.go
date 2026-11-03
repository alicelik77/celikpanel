// Package appcatalog is the one reviewed list of applications CelikPanel can
// put on a site (alpha.118, docs/APP-CATALOG.md). Every entry pins one
// upstream release: its official archive address and the SHA-256 of that
// exact file. Changing a version is one reviewed change of this file: the
// new address, the new digest (checked against the upstream's own published
// checksum where the upstream publishes one), and nothing else.
//
// The Panel reads the catalog to show and gate applications; the Agent reads
// the same compiled list to download and verify, so the Panel can never send
// it an address or a digest. An application is an offering of D-017/D-042
// ("app:<id>"): the administrator offers it before a reseller or a customer
// sees it.
//
// Uygulama kataloğu: her giriş tek bir resmi sürümü, arşiv adresi ve o
// dosyanın SHA-256 özetiyle sabitler. Sürüm değiştirmek bu dosyada tek bir
// incelenmiş değişikliktir. Panel ve Agent aynı derlenmiş listeyi okur; Panel
// Agent'a adres ya da özet gönderemez.
package appcatalog

import (
	"regexp"
	"strconv"
	"strings"
)

// Kinds of catalog applications. A "php-site" lives in a PHP site's document
// root and is served by the site's PHP-FPM pool. A later kind ("service", a
// long-running Node/Python application) shares the same ID and offering
// space without reshaping this model.
const (
	KindPHPSite = "php-site"
)

// Archive formats the Agent can read with the Go standard library.
const (
	FormatTarGz  = "tar.gz"
	FormatTarBz2 = "tar.bz2"
	FormatZip    = "zip"
)

// Setup recipes the Agent knows. Each runs the application's own supported
// non-interactive installer as the site's user with the site's PHP.
const (
	SetupNextcloud  = "nextcloud-occ"
	SetupDrupal     = "drupal-install-api"
	SetupJoomla     = "joomla-cli"
	SetupPrestaShop = "prestashop-cli"
	SetupMediaWiki  = "mediawiki-cli"
	SetupPhpBB      = "phpbb-cli"
	// SetupWordPressLegacy: WordPress keeps its own installer
	// (Agent.InstallWordPress) and its own setup page.
	SetupWordPressLegacy = "wordpress-legacy"
)

// Archive is one pinned upstream release file.
type Archive struct {
	URL    string
	SHA256 string
	Format string
	// StripPrefix is the archive's top directory ("drupal-11.4.11"); every
	// entry must be inside it. Empty: the files are at the archive's root.
	StripPrefix string
	// Inner names a zip inside the archive that holds the application
	// (PrestaShop ships prestashop.zip beside its own installer page).
	Inner string
	// Limits keep a hostile or corrupted archive from filling the disk.
	MaxDownloadBytes  int64
	MaxExtractedBytes int64
	MaxEntries        int
	// Markers must exist after extraction (relative to the application root);
	// a directory ends in "/".
	Markers []string
	// Upstream says where the digest was checked against the upstream's own
	// published value, or that the upstream publishes none.
	Upstream string
}

// App is one catalog entry.
type App struct {
	// Withheld, when set, says why this release does not offer the
	// application: it is not listed, offered or installed; its code stays.
	Withheld string
	ID       string
	Kind     string
	Name     string
	Version  string
	License  string
	Homepage string
	Summary  string
	Archive  Archive
	// PHPMin and PHPMax are PHP version lines ("8.3"); PHPMax "" has no
	// upper bound the upstream declares.
	PHPMin string
	PHPMax string
	// Extensions are PHP extension names as extension_loaded() knows them.
	Extensions []string
	// Databases are the engines the application can use, by offering base
	// ("mariadb"). The first one is what CelikPanel creates.
	Databases []string
	// MinDiskMB is the free room the files need (the extracted size with a
	// margin), checked against the subscription's disk limit.
	MinDiskMB int64
	// Risk is a short note the administrator reads before offering it.
	Risk string
	// Setup is the recipe that configures the application without a browser.
	Setup string
	// AdminPath is where the administrator signs in ("/administrator").
	// PrestaShop's admin folder gets a random name at setup; the Agent
	// reports it.
	AdminPath string
	// AdminUser is the administrator account the setup creates.
	AdminUser string
	// DataDir is a folder in the site's home, outside the document root, for
	// the application's private data ("" none).
	DataDir string
	// ConfigFile is the file, relative to the document root, that holds the
	// database password (transfer rotation rewrites it).
	ConfigFile string
	// Nginx is the rule set written to the site's CelikPanel include
	// directory ("" none).
	Nginx string
	// UploadMB is the request body size the application needs (nginx
	// client_max_body_size), 0 for nginx's default.
	UploadMB int
}

var idPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{1,40}$`)

// ValidID reports whether id has the catalog's form.
func ValidID(id string) bool { return idPattern.MatchString(id) }

// OfferingPrefix starts every application's offering id.
const OfferingPrefix = "app:"

// OfferingID is the application's offering id ("app:nextcloud").
func OfferingID(id string) string { return OfferingPrefix + id }

// IDFromOffering returns the application id of an offering id.
func IDFromOffering(offering string) (string, bool) {
	id, ok := strings.CutPrefix(offering, OfferingPrefix)
	if !ok || !ValidID(id) {
		return "", false
	}
	return id, true
}

// DefaultOffered lists the applications an existing installation offers
// before the administrator chose anything: WordPress, so nothing that worked
// before this release stops working. Every other application waits for the
// administrator's offer.
var DefaultOffered = []string{"wordpress"}

// All returns the catalog in display order.
func All() []App {
	out := make([]App, len(catalog))
	copy(out, catalog)
	for i := range out {
		out[i].Extensions = append([]string(nil), out[i].Extensions...)
		out[i].Databases = append([]string(nil), out[i].Databases...)
		out[i].Archive.Markers = append([]string(nil), out[i].Archive.Markers...)
	}
	return out
}

// Offerable reports whether this release offers the application at all.
func (a App) Offerable() bool { return a.Withheld == "" }

// Lookup finds one application.
func Lookup(id string) (App, bool) {
	for _, app := range All() {
		if app.ID == id {
			return app, true
		}
	}
	return App{}, false
}

// Legacy reports whether the application keeps its own older installer.
func (a App) Legacy() bool { return a.Setup == SetupWordPressLegacy }

// PHPLine is the "major.minor" line of a PHP version ("8.3.12" -> "8.3").
func PHPLine(version string) string {
	parts := strings.Split(strings.TrimSpace(version), ".")
	if len(parts) < 2 {
		return ""
	}
	if _, err := strconv.Atoi(parts[0]); err != nil {
		return ""
	}
	if _, err := strconv.Atoi(parts[1]); err != nil {
		return ""
	}
	return parts[0] + "." + parts[1]
}

func compareLines(a, b string) int {
	ap := strings.SplitN(a, ".", 2)
	bp := strings.SplitN(b, ".", 2)
	for i := 0; i < 2; i++ {
		x, _ := strconv.Atoi(ap[i])
		y, _ := strconv.Atoi(bp[i])
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

// PHPSupported says whether the application runs on a PHP version; reason
// names the bound when it does not.
func (a App) PHPSupported(version string) (bool, string) {
	line := PHPLine(version)
	if line == "" {
		return false, "the site's PHP version is not known"
	}
	if a.PHPMin != "" && compareLines(line, a.PHPMin) < 0 {
		return false, a.Name + " " + a.Version + " needs PHP " + a.PHPMin + " or newer; the site runs PHP " + line
	}
	if a.PHPMax != "" && compareLines(line, a.PHPMax) > 0 {
		return false, a.Name + " " + a.Version + " supports PHP up to " + a.PHPMax + "; the site runs PHP " + line
	}
	return true, ""
}

// PHPRange is the supported range in words ("8.3 to 8.5", "8.1 or newer").
func (a App) PHPRange() string {
	switch {
	case a.PHPMin != "" && a.PHPMax != "":
		return a.PHPMin + " – " + a.PHPMax
	case a.PHPMin != "":
		return a.PHPMin + "+"
	}
	return ""
}
