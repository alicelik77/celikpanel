package main

import (
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alicelik/celikpanel/internal/backupspec"
	"github.com/alicelik/celikpanel/internal/hostingpath"
	"github.com/alicelik/celikpanel/internal/hostname"
	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// WordPress tools (alpha.118): status, verified updates and copies of a
// WordPress site the panel installed or found.
//
// Decision (recorded in docs/WORDPRESS-TOOLS.md): WP-CLI is not used. It is
// in Arch's official "extra" repository but not in Debian 12/13 or Ubuntu
// 22.04/24.04 (packages.debian.org and packages.ubuntu.com, checked
// 2026-11-03), and CelikPanel installs only official distribution packages.
// The few operations needed are done here instead:
//   - Reading versions: a small panel-shipped PHP script, run with the site's
//     own PHP as the site's own account, reads version headers. It runs no
//     WordPress or plugin code.
//   - Asking for updates: only wordpress.org's official API.
//   - Updating: the package is downloaded from wordpress.org and verified
//     before it is used (core: the release's published SHA-1; plugin: the
//     per-file SHA-256 list wordpress.org publishes for every plugin
//     version). Files are copied by the site's own account; the core's
//     database step is WordPress's own wp_upgrade(), run as the site account
//     with plugins and themes not loaded (WP_INSTALLING).
//   - Themes: wordpress.org publishes no checksum for theme packages, so a
//     theme update is not done by CelikPanel (security before freedom); the
//     screen says to update it from WordPress's own Updates page.
//   - Copying: files with the backup engine's safe archive, the database
//     with the backup engine's dump, the address changed with a
//     serialization-safe replace (wordpress_replace.go).
//
// WordPress araçları: WP-CLI kullanılmaz (Debian/Ubuntu resmi depolarında
// yok). Sürümler sitenin kendi hesabıyla küçük bir PHP betiğiyle okunur;
// güncellemeler wordpress.org'dan indirilip doğrulanır; tema paketleri
// doğrulanamadığı için temalar WordPress'in kendi ekranından güncellenir.

var (
	wordpressAPIBase      = "https://api.wordpress.org"
	wordpressDownloadBase = "https://downloads.wordpress.org"
	wordpressReleaseBase  = "https://wordpress.org"
	// wordpressToolsDir holds the panel-shipped scripts and verified update
	// packages. Root-owned, readable by site accounts, writable by root only.
	wordpressToolsDir = "/var/lib/celikpanel-wordpress-tools"
	// wordpressHealthURL is where a site's front page is asked locally.
	wordpressHealthURL  = "http://127.0.0.1/"
	wordpressHTTPClient = func(timeout time.Duration) *http.Client { return wpHTTPClient(timeout) }
	wordpressNow        = time.Now
)

const (
	wordpressMaxPlugins      = 500
	wordpressMaxPackageBytes = int64(64 << 20)
	wordpressAPICacheTTL     = 30 * time.Minute
)

var wordpressVersionText = regexp.MustCompile(`^[0-9]{1,3}(\.[0-9]{1,3}){1,3}([-.+][0-9A-Za-z.]{1,20})?$`)
var wordpressSlugText = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,99}$`)

// wordpressInventoryScript reads versions only. It is run as the site's
// account with the site's PHP; it includes no file and runs no site code.
const wordpressInventoryScript = `<?php
// CelikPanel WordPress inventory (alpha.118). Reads version headers only and
// runs no WordPress, plugin or theme code. Run as the site's own account.
error_reporting(0);
$root = isset($argv[1]) ? $argv[1] : '';
$out = array('core' => '', 'plugins' => array(), 'themes' => array(), 'db_name' => '', 'table_prefix' => '');
function cp_read($file, $limit) {
    if (!is_file($file) || is_link($file)) { return false; }
    return @file_get_contents($file, false, null, 0, $limit);
}
function cp_header($file, $labels) {
    $data = cp_read($file, 8192);
    if ($data === false) { return array(); }
    $data = str_replace("\r", "\n", $data);
    $found = array();
    foreach ($labels as $key => $label) {
        if (preg_match('/^[ \t\/*#@]*' . preg_quote($label, '/') . ':(.*)$/mi', $data, $m)) {
            $found[$key] = trim(preg_replace('/\s*(?:\*\/|\?>).*/', '', $m[1]));
        }
    }
    return $found;
}
$version = cp_read($root . '/wp-includes/version.php', 65536);
if ($version !== false && preg_match('/\$wp_version\s*=\s*[\'"]([^\'"]+)[\'"]/', $version, $m)) { $out['core'] = $m[1]; }
$config = cp_read($root . '/wp-config.php', 262144);
if ($config !== false) {
    if (preg_match('/define\s*\(\s*[\'"]DB_NAME[\'"]\s*,\s*[\'"]([^\'"]*)[\'"]/', $config, $m)) { $out['db_name'] = $m[1]; }
    if (preg_match('/\$table_prefix\s*=\s*[\'"]([A-Za-z0-9_]*)[\'"]/', $config, $m)) { $out['table_prefix'] = $m[1]; }
}
$dir = $root . '/wp-content/plugins';
if (is_dir($dir) && !is_link($dir)) {
    foreach (scandir($dir) as $entry) {
        if ($entry === '' || $entry[0] === '.') { continue; }
        $path = $dir . '/' . $entry;
        if (is_link($path)) { continue; }
        $candidates = array();
        if (is_dir($path)) {
            foreach (scandir($path) as $file) {
                if (substr($file, -4) === '.php') { $candidates[] = $entry . '/' . $file; }
            }
        } elseif (substr($entry, -4) === '.php') {
            $candidates[] = $entry;
        }
        foreach ($candidates as $rel) {
            $h = cp_header($dir . '/' . $rel, array('name' => 'Plugin Name', 'version' => 'Version'));
            if (!empty($h['name'])) {
                $out['plugins'][] = array('file' => $rel, 'name' => $h['name'], 'version' => isset($h['version']) ? $h['version'] : '');
                break;
            }
        }
        if (count($out['plugins']) >= 500) { break; }
    }
}
$dir = $root . '/wp-content/themes';
if (is_dir($dir) && !is_link($dir)) {
    foreach (scandir($dir) as $entry) {
        if ($entry === '' || $entry[0] === '.' || is_link($dir . '/' . $entry) || !is_dir($dir . '/' . $entry)) { continue; }
        $h = cp_header($dir . '/' . $entry . '/style.css', array('name' => 'Theme Name', 'version' => 'Version'));
        if (!empty($h['name'])) {
            $out['themes'][] = array('slug' => $entry, 'name' => $h['name'], 'version' => isset($h['version']) ? $h['version'] : '');
        }
        if (count($out['themes']) >= 200) { break; }
    }
}
echo json_encode($out);
`

// wordpressUpgradeScript runs WordPress's own database upgrade after a core
// update, with plugins and themes not loaded (WP_INSTALLING).
const wordpressUpgradeScript = `<?php
// CelikPanel core database step (alpha.118): WordPress's own wp_upgrade(),
// with plugins and themes not loaded. Run as the site's own account.
$root = isset($argv[1]) ? rtrim($argv[1], '/') : '';
$host = isset($argv[2]) ? $argv[2] : 'localhost';
$_SERVER['HTTP_HOST'] = $host;
$_SERVER['SERVER_NAME'] = $host;
$_SERVER['REQUEST_URI'] = '/wp-admin/upgrade.php';
$_SERVER['REQUEST_METHOD'] = 'GET';
define('WP_INSTALLING', true);
require $root . '/wp-load.php';
require_once ABSPATH . 'wp-admin/includes/upgrade.php';
wp_upgrade();
echo json_encode(array('ok' => true, 'db_version' => (int) get_option('db_version')));
`

type wordpressInventory struct {
	Core    string `json:"core"`
	Plugins []struct {
		File    string `json:"file"`
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"plugins"`
	Themes []struct {
		Slug    string `json:"slug"`
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"themes"`
	DBName      string `json:"db_name"`
	TablePrefix string `json:"table_prefix"`
}

// validWordPressSite checks a site identity and derives its web folder. The
// site account must be the one CelikPanel names for this domain.
func validWordPressSite(site transport.WordPressSite) (string, error) {
	canonical, err := hostname.CanonicalFQDN(site.Domain)
	if err != nil || canonical != site.Domain || site.SiteID <= 0 || site.SubscriptionID <= 0 || site.DomainID <= 0 {
		return "", errors.New("site, subscription and domain identities are required")
	}
	if site.Username == "" || site.Username != services.SiteUsername(site.Domain) {
		return "", errors.New("the site account does not belong to this domain")
	}
	if site.PHPVersion != "" && !phpVersionText.MatchString(site.PHPVersion) {
		return "", errors.New("invalid PHP version")
	}
	return hostingpath.DocumentRoot(site.SubscriptionID, site.DomainID)
}

// ensureWordPressToolScript writes one panel-shipped script, root-owned and
// readable by everyone (it holds no secret), and returns its path.
func ensureWordPressToolScript(name, content string) (string, error) {
	if err := os.MkdirAll(wordpressToolsDir, 0o711); err != nil {
		return "", err
	}
	if err := os.Chmod(wordpressToolsDir, 0o711); err != nil {
		return "", err
	}
	target := filepath.Join(wordpressToolsDir, name)
	if current, err := os.ReadFile(target); err == nil && string(current) == content {
		return target, nil
	}
	temp, err := os.CreateTemp(wordpressToolsDir, "."+name+"-")
	if err != nil {
		return "", err
	}
	defer os.Remove(temp.Name())
	if _, err := temp.WriteString(content); err != nil {
		temp.Close()
		return "", err
	}
	if err := temp.Chmod(0o644); err != nil {
		temp.Close()
		return "", err
	}
	if err := temp.Close(); err != nil {
		return "", err
	}
	return target, os.Rename(temp.Name(), target)
}

// readWordPressInventory runs the inventory script as the site account.
func readWordPressInventory(ctx context.Context, site transport.WordPressSite, docRoot string) (wordpressInventory, error) {
	script, err := ensureWordPressToolScript("inventory.php", wordpressInventoryScript)
	if err != nil {
		return wordpressInventory{}, fmt.Errorf("prepare the inventory script: %w", err)
	}
	php := phpCLIForVersion(site.PHPVersion)
	out, err := wordpressRunAsSite(ctx, site, docRoot, nil, php, "-d", "display_errors=0", script, docRoot)
	if err != nil {
		return wordpressInventory{}, err
	}
	var inventory wordpressInventory
	if err := json.Unmarshal(out, &inventory); err != nil {
		return wordpressInventory{}, errors.New("the inventory script gave no readable answer")
	}
	return inventory, nil
}

// WordPressStatusV1 reads a site's WordPress version, plugins and themes and,
// when asked, which updates wordpress.org offers. Read only.
func (a *Agent) WordPressStatusV1(req *transport.WordPressStatusRequest, resp *transport.WordPressStatusResponse) error {
	if req == nil {
		resp.Error = "request is required"
		return nil
	}
	docRoot, err := validWordPressSite(req.Site)
	if err != nil {
		resp.Error = err.Error()
		return nil
	}
	if !wordpressCoreFilesPresent(docRoot) {
		resp.Code = transport.WordPressCodeNotDetected
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	inventory, err := readWordPressInventory(ctx, req.Site, docRoot)
	if err != nil {
		resp.Code, resp.Error = classifyWordPressRunError(err)
		return nil
	}
	fillWordPressStatus(resp, inventory)
	if req.CheckUpdates {
		if err := checkWordPressUpdates(ctx, resp, true); err != nil {
			resp.UpdatesError = boundedText(err.Error(), 300)
		} else {
			resp.UpdatesChecked = true
		}
	}
	return nil
}

func wordpressCoreFilesPresent(docRoot string) bool {
	info, err := os.Lstat(filepath.Join(docRoot, "wp-includes", "version.php"))
	return err == nil && info.Mode().IsRegular()
}

func classifyWordPressRunError(err error) (string, string) {
	if errors.Is(err, errWordPressPHPMissing) {
		return transport.WordPressCodePHPMissing, err.Error()
	}
	return "", boundedText(err.Error(), 400)
}

var errWordPressPHPMissing = errors.New("the PHP command line for this site is not installed")

func fillWordPressStatus(resp *transport.WordPressStatusResponse, inventory wordpressInventory) {
	resp.Detected = inventory.Core != ""
	if !resp.Detected {
		resp.Code = transport.WordPressCodeNotDetected
	}
	resp.CoreVersion = inventory.Core
	resp.DatabaseName = inventory.DBName
	resp.TablePrefix = inventory.TablePrefix
	resp.Plugins = []transport.WordPressItem{}
	resp.Themes = []transport.WordPressItem{}
	for _, plugin := range inventory.Plugins {
		if len(resp.Plugins) >= wordpressMaxPlugins {
			break
		}
		slug := plugin.File
		if dir, _, found := strings.Cut(plugin.File, "/"); found {
			slug = dir
		} else {
			slug = strings.TrimSuffix(slug, ".php")
		}
		resp.Plugins = append(resp.Plugins, transport.WordPressItem{
			Kind: "plugin", Slug: slug, File: plugin.File,
			Name: boundedText(plugin.Name, 120), Version: boundedText(plugin.Version, 40),
		})
	}
	for _, theme := range inventory.Themes {
		resp.Themes = append(resp.Themes, transport.WordPressItem{
			Kind: "theme", Slug: theme.Slug,
			Name: boundedText(theme.Name, 120), Version: boundedText(theme.Version, 40),
		})
	}
}

// --- wordpress.org API -------------------------------------------------

type wordpressAPICacheEntry struct {
	at   time.Time
	body []byte
}

var (
	wordpressAPICacheMu sync.Mutex
	wordpressAPICache   = map[string]wordpressAPICacheEntry{}
)

// wordpressAPI asks wordpress.org's official API. Only the API host is ever
// called; a cached answer is reused for a short time when allowed.
func wordpressAPI(ctx context.Context, method, endpoint string, form url.Values, cached bool) ([]byte, error) {
	key := method + " " + endpoint + "?" + form.Encode()
	if cached {
		wordpressAPICacheMu.Lock()
		entry, ok := wordpressAPICache[key]
		wordpressAPICacheMu.Unlock()
		if ok && wordpressNow().Sub(entry.at) < wordpressAPICacheTTL {
			return entry.body, nil
		}
	}
	target := wordpressAPIBase + endpoint
	var request *http.Request
	var err error
	if method == http.MethodPost {
		request, err = http.NewRequestWithContext(ctx, method, target, strings.NewReader(form.Encode()))
		if err == nil {
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
	} else {
		if len(form) > 0 {
			target += "?" + form.Encode()
		}
		request, err = http.NewRequestWithContext(ctx, method, target, nil)
	}
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "CelikPanel")
	response, err := wordpressHTTPClient(30 * time.Second).Do(request)
	if err != nil {
		return nil, fmt.Errorf("wordpress.org could not be reached: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("wordpress.org answered HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	wordpressAPICacheMu.Lock()
	wordpressAPICache[key] = wordpressAPICacheEntry{at: wordpressNow(), body: body}
	wordpressAPICacheMu.Unlock()
	return body, nil
}

type wordpressPackageOffer struct {
	Slug       string `json:"slug"`
	NewVersion string `json:"new_version"`
	Package    string `json:"package"`
}

// checkWordPressUpdates fills the update fields of a status answer.
func checkWordPressUpdates(ctx context.Context, resp *transport.WordPressStatusResponse, cached bool) error {
	if resp.CoreVersion != "" {
		latest, minor, err := wordpressCoreOffers(ctx, resp.CoreVersion, cached)
		if err != nil {
			return err
		}
		if compareWordPressVersions(latest, resp.CoreVersion) > 0 {
			resp.CoreUpdate = latest
		}
		if compareWordPressVersions(minor, resp.CoreVersion) > 0 {
			resp.CoreMinor = minor
		}
	}
	if len(resp.Plugins) > 0 {
		offers, known, err := wordpressPluginOffers(ctx, resp.Plugins, cached)
		if err != nil {
			return err
		}
		for i := range resp.Plugins {
			item := &resp.Plugins[i]
			if offer, ok := offers[item.File]; ok && compareWordPressVersions(offer.NewVersion, item.Version) > 0 {
				item.UpdateVersion = offer.NewVersion
			}
			if known[item.File] {
				item.Source = "wordpress.org"
				item.Verifiable = strings.Contains(item.File, "/")
			} else {
				item.Source = "other"
			}
		}
	}
	if len(resp.Themes) > 0 {
		offers, known, err := wordpressThemeOffers(ctx, resp.Themes, cached)
		if err != nil {
			return err
		}
		for i := range resp.Themes {
			item := &resp.Themes[i]
			if offer, ok := offers[item.Slug]; ok && compareWordPressVersions(offer.NewVersion, item.Version) > 0 {
				item.UpdateVersion = offer.NewVersion
			}
			if known[item.Slug] {
				item.Source = "wordpress.org"
			} else {
				item.Source = "other"
			}
			// No checksum is published for theme packages.
			item.Verifiable = false
		}
	}
	return nil
}

// wordpressCoreOffers: the newest release and the newest release of the
// installed branch (major.minor).
func wordpressCoreOffers(ctx context.Context, current string, cached bool) (string, string, error) {
	body, err := wordpressAPI(ctx, http.MethodGet, "/core/version-check/1.7/", url.Values{"version": {current}}, cached)
	if err != nil {
		return "", "", err
	}
	var answer struct {
		Offers []struct {
			Response string `json:"response"`
			Version  string `json:"version"`
			Current  string `json:"current"`
		} `json:"offers"`
	}
	if err := json.Unmarshal(body, &answer); err != nil {
		return "", "", errors.New("wordpress.org gave an unreadable core answer")
	}
	branch := wordpressBranch(current)
	latest, minor := "", ""
	for _, offer := range answer.Offers {
		for _, version := range []string{offer.Version, offer.Current} {
			if !wordpressVersionText.MatchString(version) {
				continue
			}
			if compareWordPressVersions(version, latest) > 0 {
				latest = version
			}
			if wordpressBranch(version) == branch && compareWordPressVersions(version, minor) > 0 {
				minor = version
			}
		}
	}
	return latest, minor, nil
}

func wordpressBranch(version string) string {
	parts := strings.SplitN(version, ".", 3)
	if len(parts) < 2 {
		return version
	}
	return parts[0] + "." + parts[1]
}

func wordpressPluginOffers(ctx context.Context, plugins []transport.WordPressItem, cached bool) (map[string]wordpressPackageOffer, map[string]bool, error) {
	list := map[string]map[string]string{}
	for _, plugin := range plugins {
		list[plugin.File] = map[string]string{"Name": plugin.Name, "Version": plugin.Version}
	}
	encoded, _ := json.Marshal(map[string]any{"plugins": list, "active": []string{}})
	body, err := wordpressAPI(ctx, http.MethodPost, "/plugins/update-check/1.1/",
		url.Values{"plugins": {string(encoded)}, "translations": {"[]"}, "locale": {"[]"}, "all": {"true"}}, cached)
	if err != nil {
		return nil, nil, err
	}
	return decodeWordPressOffers(body, "plugins")
}

func wordpressThemeOffers(ctx context.Context, themes []transport.WordPressItem, cached bool) (map[string]wordpressPackageOffer, map[string]bool, error) {
	list := map[string]map[string]string{}
	for _, theme := range themes {
		list[theme.Slug] = map[string]string{"Version": theme.Version}
	}
	encoded, _ := json.Marshal(map[string]any{"active": "", "themes": list})
	body, err := wordpressAPI(ctx, http.MethodPost, "/themes/update-check/1.1/",
		url.Values{"themes": {string(encoded)}, "translations": {"[]"}, "locale": {"[]"}}, cached)
	if err != nil {
		return nil, nil, err
	}
	return decodeWordPressOffers(body, "themes")
}

// decodeWordPressOffers reads {"<field>": {key: offer}, "no_update": {key: …}}.
// wordpress.org writes an empty object as [] — both are accepted.
func decodeWordPressOffers(body []byte, field string) (map[string]wordpressPackageOffer, map[string]bool, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, nil, errors.New("wordpress.org gave an unreadable update answer")
	}
	decode := func(name string) map[string]wordpressPackageOffer {
		out := map[string]wordpressPackageOffer{}
		_ = json.Unmarshal(raw[name], &out)
		return out
	}
	offers := decode(field)
	known := map[string]bool{}
	for key := range offers {
		known[key] = true
	}
	for key := range decode("no_update") {
		known[key] = true
	}
	return offers, known, nil
}

// compareWordPressVersions compares dotted versions numerically ("" is the
// smallest). A suffix (-RC1) sorts below the same version without one.
func compareWordPressVersions(a, b string) int {
	if a == b {
		return 0
	}
	if a == "" {
		return -1
	}
	if b == "" {
		return 1
	}
	split := func(v string) ([]int, string) {
		main, suffix := v, ""
		if i := strings.IndexAny(v, "-+ "); i >= 0 {
			main, suffix = v[:i], v[i:]
		}
		parts := strings.Split(main, ".")
		nums := make([]int, len(parts))
		for i, part := range parts {
			n, _ := strconv.Atoi(part)
			nums[i] = n
		}
		return nums, suffix
	}
	an, as := split(a)
	bn, bs := split(b)
	for i := 0; i < len(an) || i < len(bn); i++ {
		x, y := 0, 0
		if i < len(an) {
			x = an[i]
		}
		if i < len(bn) {
			y = bn[i]
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	switch {
	case as == bs:
		return 0
	case as == "":
		return 1
	case bs == "":
		return -1
	case as < bs:
		return -1
	}
	return 1
}

// --- verified packages ---------------------------------------------------

// downloadWordPressPackage fetches one package from a wordpress.org download
// host into a private file.
func downloadWordPressPackage(ctx context.Context, packageURL, destination string, allowedBase string) error {
	if !strings.HasPrefix(packageURL, allowedBase+"/") {
		return fmt.Errorf("the package address is not on %s", allowedBase)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, packageURL, nil)
	if err != nil {
		return err
	}
	request.Header.Set("User-Agent", "CelikPanel")
	response, err := wordpressHTTPClient(5 * time.Minute).Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("download answered HTTP %d", response.StatusCode)
	}
	out, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	written, copyErr := io.Copy(out, io.LimitReader(response.Body, wordpressMaxPackageBytes+1))
	closeErr := out.Close()
	if copyErr == nil && written > wordpressMaxPackageBytes {
		copyErr = errors.New("the package is larger than allowed")
	}
	if copyErr != nil {
		_ = os.Remove(destination)
		return copyErr
	}
	return closeErr
}

// extractWordPressZip extracts a plugin or theme package whose every entry
// is under "<slug>/" into destination/<slug>. Only regular files and
// folders; no links; bounded.
func extractWordPressZip(archive, destination, slug string) error {
	reader, err := zip.OpenReader(archive)
	if err != nil {
		return fmt.Errorf("the package is not a readable zip: %w", err)
	}
	defer reader.Close()
	if len(reader.File) > wpMaxArchiveEntries {
		return errors.New("the package has too many entries")
	}
	var total int64
	root := filepath.Join(destination, slug)
	if err := os.Mkdir(root, 0o755); err != nil {
		return err
	}
	for _, file := range reader.File {
		name := file.Name
		if name == "" || len(name) > wpMaxArchivePathBytes || strings.ContainsAny(name, "\x00\\") {
			return errors.New("the package has an invalid path")
		}
		clean := path.Clean(name)
		if path.IsAbs(clean) || clean == slug && !file.FileInfo().IsDir() || !strings.HasPrefix(clean, slug+"/") && clean != slug {
			return fmt.Errorf("the package entry %q is outside %s/", name, slug)
		}
		if strings.Contains("/"+clean+"/", "/../") {
			return errors.New("the package has an unsafe path")
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(clean, slug), "/")
		mode := file.Mode()
		target := filepath.Join(root, filepath.FromSlash(rel))
		switch {
		case mode.IsDir():
			if rel != "" {
				if err := os.MkdirAll(target, 0o755); err != nil {
					return err
				}
			}
		case mode.IsRegular():
			size := int64(file.UncompressedSize64)
			if size < 0 || size > wpMaxExtractedFile || total > wpMaxExtractedBytes-size {
				return errors.New("the package exceeds the extraction limits")
			}
			total += size
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			if err := writeZipEntry(file, target, size); err != nil {
				return err
			}
		default:
			return fmt.Errorf("the package entry %q is not a plain file or folder", name)
		}
	}
	return nil
}

func writeZipEntry(file *zip.File, target string, size int64) error {
	in, err := file.Open()
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	written, copyErr := io.Copy(out, io.LimitReader(in, size+1))
	closeErr := out.Close()
	if copyErr == nil && written != size {
		copyErr = errors.New("a package entry has a wrong size")
	}
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

// wordpressChecksums is wordpress.org's per-file list for one plugin version.
// A value is a string, or a list of accepted strings (readme.txt).
type wordpressChecksums struct {
	Files map[string]struct {
		SHA256 json.RawMessage `json:"sha256"`
	} `json:"files"`
}

func acceptedChecksums(raw json.RawMessage) []string {
	var one string
	if json.Unmarshal(raw, &one) == nil {
		return []string{strings.ToLower(one)}
	}
	var many []string
	if json.Unmarshal(raw, &many) == nil {
		for i := range many {
			many[i] = strings.ToLower(many[i])
		}
		return many
	}
	return nil
}

// verifyPluginTree checks every extracted file against wordpress.org's
// SHA-256 list for that plugin version, and that no listed file is missing.
func verifyPluginTree(root string, sums wordpressChecksums) error {
	if len(sums.Files) == 0 {
		return errors.New("wordpress.org published no checksums for this version")
	}
	seen := map[string]bool{}
	err := filepath.WalkDir(root, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("%s is not a plain file", entry.Name())
		}
		rel, err := filepath.Rel(root, current)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		expected, ok := sums.Files[rel]
		if !ok {
			return fmt.Errorf("%s is not in wordpress.org's checksum list", rel)
		}
		actual, err := fileSHA256(current)
		if err != nil {
			return err
		}
		matched := false
		for _, candidate := range acceptedChecksums(expected.SHA256) {
			matched = matched || candidate == actual
		}
		if !matched {
			return fmt.Errorf("%s does not match wordpress.org's checksum", rel)
		}
		seen[rel] = true
		return nil
	})
	if err != nil {
		return err
	}
	for name := range sums.Files {
		if !seen[name] {
			return fmt.Errorf("%s is missing from the package", name)
		}
	}
	return nil
}

func fileSHA256(name string) (string, error) {
	file, err := os.Open(name)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func fetchPluginChecksums(ctx context.Context, slug, version string) (wordpressChecksums, error) {
	target := wordpressDownloadBase + "/plugin-checksums/" + url.PathEscape(slug) + "/" + url.PathEscape(version) + ".json"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return wordpressChecksums{}, err
	}
	response, err := wordpressHTTPClient(30 * time.Second).Do(request)
	if err != nil {
		return wordpressChecksums{}, err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return wordpressChecksums{}, errWordPressNotVerifiable
	}
	if response.StatusCode != http.StatusOK {
		return wordpressChecksums{}, fmt.Errorf("checksums answered HTTP %d", response.StatusCode)
	}
	var sums wordpressChecksums
	if err := json.NewDecoder(io.LimitReader(response.Body, 16<<20)).Decode(&sums); err != nil {
		return wordpressChecksums{}, errors.New("checksums could not be read")
	}
	return sums, nil
}

var errWordPressNotVerifiable = errors.New("wordpress.org publishes no checksums for this package")

// --- update ---------------------------------------------------------------

// wordpressPlannedPlugin is one verified plugin package ready to be copied.
type wordpressPlannedPlugin struct {
	file, slug, version string
	staged              string // the verified folder
}

// WordPressUpdateV1 updates the core or plugins of one site. Every package
// is downloaded and verified before the site is touched; the Panel has made
// a backup first and restores it when the answer says the files changed and
// the update did not succeed.
func (a *Agent) WordPressUpdateV1(req *transport.WordPressUpdateRequest, resp *transport.WordPressUpdateResponse) error {
	if req == nil {
		resp.Error = "request is required"
		return nil
	}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "WordPress update"); err != nil {
		resp.Error = err.Error()
		return nil
	}
	if !validWordPressOperationID(req.OperationID) {
		resp.Error = "operation identity is required"
		return nil
	}
	docRoot, err := validWordPressSite(req.Site)
	if err != nil {
		resp.Error = err.Error()
		return nil
	}
	switch req.Kind {
	case "core":
		if !wordpressVersionText.MatchString(req.CoreVersion) {
			resp.Error = "the core version to install is required"
			return nil
		}
	case "plugin":
		if len(req.Items) == 0 || len(req.Items) > 50 {
			resp.Error = "choose between 1 and 50 plugins"
			return nil
		}
	case "theme":
		resp.Code = transport.WordPressCodeNotVerifiable
		resp.Error = "wordpress.org publishes no checksum for theme packages, so CelikPanel does not install them; update the theme from WordPress's own Dashboard > Updates page"
		return nil
	default:
		resp.Error = "kind must be core, plugin or theme"
		return nil
	}

	lock := siteMutationMutex(req.Site.SiteID)
	lock.Lock()
	defer lock.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()
	inventory, err := readWordPressInventory(ctx, req.Site, docRoot)
	if err != nil {
		resp.Code, resp.Error = classifyWordPressRunError(err)
		return nil
	}
	status := &transport.WordPressStatusResponse{}
	fillWordPressStatus(status, inventory)
	if !status.Detected {
		resp.Code = transport.WordPressCodeNotDetected
		resp.Error = "WordPress was not found in this site's web folder"
		return nil
	}
	if err := checkWordPressUpdates(ctx, status, false); err != nil {
		resp.Code = transport.WordPressCodeUpdateSource
		resp.Error = boundedText(err.Error(), 300)
		return nil
	}

	work, err := wordpressWorkDir(req.OperationID)
	if err != nil {
		resp.Error = fmt.Sprintf("prepare the update folder: %v", err)
		return nil
	}
	defer os.RemoveAll(work)

	switch req.Kind {
	case "core":
		a.updateWordPressCore(ctx, req, resp, status, docRoot, work)
	case "plugin":
		a.updateWordPressPlugins(ctx, req, resp, status, docRoot, work)
	}
	return nil
}

// wordpressWorkDir: a folder site accounts can read (the verified package is
// public software) but not change.
func wordpressWorkDir(operationID string) (string, error) {
	if err := os.MkdirAll(wordpressToolsDir, 0o711); err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp(wordpressToolsDir, "update-"+operationID[:8]+"-")
	if err != nil {
		return "", err
	}
	return dir, os.Chmod(dir, 0o755)
}

func (a *Agent) updateWordPressCore(ctx context.Context, req *transport.WordPressUpdateRequest, resp *transport.WordPressUpdateResponse,
	status *transport.WordPressStatusResponse, docRoot, work string) {
	if req.CoreVersion != status.CoreUpdate && req.CoreVersion != status.CoreMinor {
		resp.Code = transport.WordPressCodeNoUpdate
		resp.Error = fmt.Sprintf("WordPress %s is not an update wordpress.org offers for this site now; read the status again", req.CoreVersion)
		return
	}
	if req.MinorOnly && wordpressBranch(req.CoreVersion) != wordpressBranch(status.CoreVersion) {
		resp.Code = transport.WordPressCodeNoUpdate
		resp.Error = "only minor and security releases of the installed branch are installed automatically"
		return
	}
	tarball := filepath.Join(work, "wordpress.tar.gz")
	releaseURL := wordpressReleaseBase + "/wordpress-" + req.CoreVersion + ".tar.gz"
	if err := downloadWordPressPackage(ctx, releaseURL, tarball, wordpressReleaseBase); err != nil {
		resp.Code = transport.WordPressCodeUpdateSource
		resp.Error = "download WordPress " + req.CoreVersion + ": " + boundedText(err.Error(), 200)
		return
	}
	if err := verifySHA1(tarball, releaseURL+".sha1"); err != nil {
		resp.Code = transport.WordPressCodeVerifyFailed
		resp.Error = "WordPress " + req.CoreVersion + " did not pass its integrity check: " + boundedText(err.Error(), 200)
		return
	}
	stage := filepath.Join(work, "core")
	if err := os.Mkdir(stage, 0o755); err != nil {
		resp.Error = err.Error()
		return
	}
	if err := extractWordPressArchive(tarball, stage); err != nil {
		resp.Code = transport.WordPressCodeVerifyFailed
		resp.Error = "extract WordPress: " + boundedText(err.Error(), 200)
		return
	}
	if err := makeTreeReadable(stage); err != nil {
		resp.Error = err.Error()
		return
	}
	entries, err := os.ReadDir(stage)
	if err != nil {
		resp.Error = err.Error()
		return
	}
	before := wordpressHealth(req.Site.Domain)

	// From here the site's files change.
	resp.Changed = true
	if err := wordpressMaintenance(ctx, req.Site, docRoot, work, true); err != nil {
		resp.Changed = false
		resp.Error = "could not put the site in maintenance mode: " + boundedText(err.Error(), 200)
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		// The site's own content and configuration are never replaced.
		if name == "wp-content" || name == "wp-config.php" {
			continue
		}
		if _, err := wordpressRunAsSite(ctx, req.Site, docRoot, nil, "cp", "-R", "-T", "--", filepath.Join(stage, name), filepath.Join(docRoot, name)); err != nil {
			resp.Error = "copy " + name + ": " + boundedText(err.Error(), 200)
			_ = wordpressMaintenance(ctx, req.Site, docRoot, work, false)
			return
		}
	}
	script, err := ensureWordPressToolScript("upgrade.php", wordpressUpgradeScript)
	if err == nil {
		_, err = wordpressRunAsSite(ctx, req.Site, docRoot, nil, phpCLIForVersion(req.Site.PHPVersion), "-d", "display_errors=0", script, docRoot, req.Site.Domain)
	}
	_ = wordpressMaintenance(ctx, req.Site, docRoot, work, false)
	if err != nil {
		resp.Error = "WordPress's database step did not finish: " + boundedText(err.Error(), 200)
		return
	}
	if after := wordpressHealth(req.Site.Domain); wordpressHealthBroke(before, after) {
		resp.Code = transport.WordPressCodeHealthFailed
		resp.Error = fmt.Sprintf("the site answers HTTP %d after the update (HTTP %d before)", after, before)
		return
	}
	resp.Updated = []string{"core " + req.CoreVersion}
	resp.Detail = "WordPress updated to " + req.CoreVersion
}

func (a *Agent) updateWordPressPlugins(ctx context.Context, req *transport.WordPressUpdateRequest, resp *transport.WordPressUpdateResponse,
	status *transport.WordPressStatusResponse, docRoot, work string) {
	byFile := map[string]transport.WordPressItem{}
	for _, plugin := range status.Plugins {
		byFile[plugin.File] = plugin
	}
	var planned []wordpressPlannedPlugin
	for _, file := range req.Items {
		plugin, ok := byFile[file]
		if !ok {
			resp.Code = transport.WordPressCodeNoUpdate
			resp.Error = fmt.Sprintf("the plugin %s is not on this site any more; read the status again", file)
			return
		}
		if plugin.UpdateVersion == "" {
			resp.Code = transport.WordPressCodeNoUpdate
			resp.Error = fmt.Sprintf("%s has no update now; read the status again", plugin.Name)
			return
		}
		if !plugin.Verifiable || !wordpressSlugText.MatchString(plugin.Slug) || !wordpressVersionText.MatchString(plugin.UpdateVersion) {
			resp.Code = transport.WordPressCodeNotVerifiable
			resp.Error = fmt.Sprintf("%s cannot be verified by CelikPanel; update it from WordPress's own Dashboard > Updates page", plugin.Name)
			return
		}
		planned = append(planned, wordpressPlannedPlugin{file: file, slug: plugin.Slug, version: plugin.UpdateVersion})
	}
	// Download and verify everything before touching the site.
	for i := range planned {
		item := &planned[i]
		zipPath := filepath.Join(work, item.slug+".zip")
		packageURL := wordpressDownloadBase + "/plugin/" + item.slug + "." + item.version + ".zip"
		if err := downloadWordPressPackage(ctx, packageURL, zipPath, wordpressDownloadBase); err != nil {
			resp.Code = transport.WordPressCodeUpdateSource
			resp.Error = "download " + item.slug + ": " + boundedText(err.Error(), 200)
			return
		}
		dest := filepath.Join(work, "plugin-"+strconv.Itoa(i))
		if err := os.Mkdir(dest, 0o755); err != nil {
			resp.Error = err.Error()
			return
		}
		if err := extractWordPressZip(zipPath, dest, item.slug); err != nil {
			resp.Code = transport.WordPressCodeVerifyFailed
			resp.Error = item.slug + ": " + boundedText(err.Error(), 200)
			return
		}
		sums, err := fetchPluginChecksums(ctx, item.slug, item.version)
		if errors.Is(err, errWordPressNotVerifiable) {
			resp.Code = transport.WordPressCodeNotVerifiable
			resp.Error = item.slug + ": " + err.Error() + "; update it from WordPress's own Dashboard > Updates page"
			return
		}
		if err == nil {
			err = verifyPluginTree(filepath.Join(dest, item.slug), sums)
		}
		if err != nil {
			resp.Code = transport.WordPressCodeVerifyFailed
			resp.Error = item.slug + " did not pass verification: " + boundedText(err.Error(), 200)
			return
		}
		item.staged = filepath.Join(dest, item.slug)
	}
	if err := makeTreeReadable(work); err != nil {
		resp.Error = err.Error()
		return
	}

	before := wordpressHealth(req.Site.Domain)
	plugins := filepath.Join(docRoot, "wp-content", "plugins")
	type swapped struct{ live, old string }
	var done []swapped
	putBack := func() bool {
		ok := true
		for i := len(done) - 1; i >= 0; i-- {
			if _, err := wordpressRunAsSite(ctx, req.Site, plugins, nil, "rm", "-rf", "--", done[i].live); err != nil {
				ok = false
				continue
			}
			if _, err := wordpressRunAsSite(ctx, req.Site, plugins, nil, "mv", "-T", "--", done[i].old, done[i].live); err != nil {
				ok = false
			}
		}
		return ok
	}
	resp.Changed = true
	if err := wordpressMaintenance(ctx, req.Site, docRoot, work, true); err != nil {
		resp.Changed = false
		resp.Error = "could not put the site in maintenance mode: " + boundedText(err.Error(), 200)
		return
	}
	defer func() { _ = wordpressMaintenance(ctx, req.Site, docRoot, work, false) }()
	suffix := ".celikpanel-old-" + req.OperationID[:8]
	for _, item := range planned {
		live := filepath.Join(plugins, item.slug)
		old := filepath.Join(plugins, "."+item.slug+suffix)
		if _, err := wordpressRunAsSite(ctx, req.Site, plugins, nil, "mv", "-T", "--", live, old); err != nil {
			resp.Error = "set aside " + item.slug + ": " + boundedText(err.Error(), 200)
			resp.RolledBack = putBack()
			return
		}
		done = append(done, swapped{live: live, old: old})
		if _, err := wordpressRunAsSite(ctx, req.Site, plugins, nil, "cp", "-R", "-T", "--", item.staged, live); err != nil {
			resp.Error = "copy " + item.slug + ": " + boundedText(err.Error(), 200)
			resp.RolledBack = putBack()
			return
		}
	}
	_ = wordpressMaintenance(ctx, req.Site, docRoot, work, false)
	if after := wordpressHealth(req.Site.Domain); wordpressHealthBroke(before, after) {
		resp.Code = transport.WordPressCodeHealthFailed
		resp.Error = fmt.Sprintf("the site answers HTTP %d after the update (HTTP %d before)", after, before)
		resp.RolledBack = putBack()
		return
	}
	for _, item := range done {
		_, _ = wordpressRunAsSite(ctx, req.Site, plugins, nil, "rm", "-rf", "--", item.old)
	}
	for _, item := range planned {
		resp.Updated = append(resp.Updated, item.slug+" "+item.version)
	}
	sort.Strings(resp.Updated)
	resp.Detail = fmt.Sprintf("%d plugin(s) updated", len(planned))
}

// wordpressMaintenance switches WordPress's own maintenance file on or off,
// as the site account.
func wordpressMaintenance(ctx context.Context, site transport.WordPressSite, docRoot, work string, on bool) error {
	target := filepath.Join(docRoot, ".maintenance")
	if !on {
		_, err := wordpressRunAsSite(ctx, site, docRoot, nil, "rm", "-f", "--", target)
		return err
	}
	source := filepath.Join(work, "maintenance.php")
	content := fmt.Sprintf("<?php $upgrading = %d; ?>\n", wordpressNow().Unix())
	if err := os.WriteFile(source, []byte(content), 0o644); err != nil {
		return err
	}
	if err := os.Chmod(source, 0o644); err != nil {
		return err
	}
	_, err := wordpressRunAsSite(ctx, site, docRoot, nil, "cp", "-T", "--", source, target)
	return err
}

func makeTreeReadable(root string) error {
	return filepath.WalkDir(root, func(current string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("unexpected link %s", current)
		}
		if entry.IsDir() {
			return os.Chmod(current, 0o755)
		}
		return os.Chmod(current, 0o644)
	})
}

// wordpressHealth asks the site's front page locally (no redirect is
// followed); 0 when there is no answer.
func wordpressHealth(domain string) int {
	request, err := http.NewRequest(http.MethodGet, wordpressHealthURL, nil)
	if err != nil {
		return 0
	}
	request.Host = domain
	client := &http.Client{
		Timeout:       20 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	response, err := client.Do(request)
	if err != nil {
		return 0
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
	response.Body.Close()
	return response.StatusCode
}

// wordpressHealthBroke: the site answered before and fails after.
func wordpressHealthBroke(before, after int) bool {
	return before > 0 && before < 500 && (after == 0 || after >= 500)
}

// --- copy (staging) -------------------------------------------------------

var wpConfigDefine = regexp.MustCompile(`(define\s*\(\s*['"](DB_NAME|DB_USER|DB_PASSWORD|DB_HOST)['"]\s*,\s*)(['"])((?:\\.|[^\\])*?)(['"])(\s*\))`)

// rewriteWordPressConfig points a copied wp-config.php at the copy's
// database and address. Only the four database lines and the address are
// changed; everything else the owner wrote stays.
func rewriteWordPressConfig(content []byte, database, user, password, sourceDomain, targetDomain string) ([]byte, error) {
	values := map[string]string{"DB_NAME": database, "DB_USER": user, "DB_PASSWORD": password, "DB_HOST": "localhost"}
	found := map[string]bool{}
	out := wpConfigDefine.ReplaceAllFunc(content, func(match []byte) []byte {
		parts := wpConfigDefine.FindSubmatch(match)
		name := string(parts[2])
		found[name] = true
		return []byte(string(parts[1]) + phpQuote(values[name]) + string(parts[6]))
	})
	for _, name := range []string{"DB_NAME", "DB_USER", "DB_PASSWORD"} {
		if !found[name] {
			return nil, fmt.Errorf("wp-config.php has no plain %s line CelikPanel can change", name)
		}
	}
	out, _ = replaceAddress(out, wordpressAddressPairs(sourceDomain, targetDomain))
	return out, nil
}

// WordPressCloneV1 copies a WordPress site to another empty site of the same
// subscription: files, database, and the address inside both.
func (a *Agent) WordPressCloneV1(req *transport.WordPressCloneRequest, resp *transport.WordPressCloneResponse) error {
	if req == nil {
		resp.Error = "request is required"
		return nil
	}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "WordPress copy"); err != nil {
		resp.Error = err.Error()
		return nil
	}
	resp.CompensationSafe = true
	if !validWordPressOperationID(req.OperationID) {
		resp.Error = "operation identity is required"
		return nil
	}
	sourceRoot, err := validWordPressSite(req.Source)
	if err != nil {
		resp.Error = "source: " + err.Error()
		return nil
	}
	targetRoot, err := validWordPressSite(req.Target)
	if err != nil {
		resp.Error = "target: " + err.Error()
		return nil
	}
	if req.Source.SubscriptionID != req.Target.SubscriptionID || req.Source.DomainID == req.Target.DomainID {
		resp.Error = "a copy goes to another site of the same subscription"
		return nil
	}
	for _, name := range []string{req.SourceDatabase, req.TargetDatabase, req.TargetDBUser} {
		if err := services.ValidateSQLIdentifier(name); err != nil {
			resp.Error = "invalid database identity"
			return nil
		}
	}
	if req.TargetDBPass == "" {
		resp.Error = "the copy's database password is required"
		return nil
	}
	first, second := siteMutationMutex(req.Source.SiteID), siteMutationMutex(req.Target.SiteID)
	first.Lock()
	defer first.Unlock()
	if second != first {
		second.Lock()
		defer second.Unlock()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()
	inventory, err := readWordPressInventory(ctx, req.Source, sourceRoot)
	if err != nil {
		resp.Code, resp.Error = classifyWordPressRunError(err)
		return nil
	}
	if inventory.Core == "" {
		resp.Code = transport.WordPressCodeNotDetected
		resp.Error = "WordPress was not found in the source site's web folder"
		return nil
	}
	if inventory.DBName != req.SourceDatabase {
		resp.Code = transport.WordPressCodeDatabaseMismatch
		resp.Error = "the source site's wp-config.php names another database than the one recorded for it"
		return nil
	}
	if _, err := inspectInstallableDocumentRoot(targetRoot, req.Target.Domain); err != nil {
		resp.Code = transport.WordPressCodeTargetNotEmpty
		resp.Error = "the target site's web folder already has content; nothing was changed"
		return nil
	}

	if err := secureMkdirAll(backupBaseDir, 0o700); err != nil {
		resp.Error = "prepare the copy folder: " + err.Error()
		return nil
	}
	work, err := os.MkdirTemp(backupBaseDir, ".wordpress-copy-")
	if err != nil {
		resp.Error = "prepare the copy folder: " + err.Error()
		return nil
	}
	defer os.RemoveAll(work)
	if err := os.Chmod(work, 0o700); err != nil {
		resp.Error = err.Error()
		return nil
	}

	filesArchive := filepath.Join(work, "files.tar.gz")
	if err := safeCreateFilesArchive(sourceRoot, filesArchive); err != nil {
		resp.Error = "copy the source files: " + boundedText(err.Error(), 300)
		return nil
	}
	sourceDump := filepath.Join(work, "source.sql.gz")
	if err := dumpDatabaseToFile(copyDatabaseIdentity(req.SourceDatabase), sourceDump); err != nil {
		resp.Error = "copy the source database: " + boundedText(err.Error(), 300)
		return nil
	}
	targetDump := filepath.Join(work, "target.sql.gz")
	stats, err := transformDumpFile(sourceDump, targetDump, wordpressAddressPairs(req.Source.Domain, req.Target.Domain))
	if err != nil {
		resp.Error = "change the address in the database copy: " + boundedText(err.Error(), 300)
		return nil
	}
	resp.ReplacedValues, resp.SkippedValues = stats.Replaced, stats.Skipped

	// wp-config.php is rewritten inside the staging folder while it is still
	// root's and 0700, through its descriptor (never by path).
	var editErr error
	staged, err := prepareFilesRestoreEdited(filesArchive, targetRoot, func(stageFD int) error {
		editErr = rewriteStagedWordPressConfig(stageFD, req)
		return editErr
	})
	if editErr != nil {
		resp.Error = editErr.Error()
		return nil
	}
	if err != nil {
		resp.Error = "prepare the copied files: " + boundedText(err.Error(), 300)
		return nil
	}
	defer staged.Cleanup()
	if err := restoreDatabaseFromFile(copyDatabaseIdentity(req.TargetDatabase), targetDump); err != nil {
		resp.Error = "load the database copy: " + boundedText(err.Error(), 300)
		return nil
	}
	// The target is checked again right before it is replaced: content a
	// person put there meanwhile is never overwritten.
	if _, err := inspectInstallableDocumentRoot(targetRoot, req.Target.Domain); err != nil {
		resp.Code = transport.WordPressCodeTargetNotEmpty
		resp.Error = "the target site's web folder got content while the copy was prepared; nothing was published"
		return nil
	}
	if err := staged.Publish(); err != nil {
		resp.Error = "publish the copied files: " + boundedText(err.Error(), 300)
		return nil
	}
	resp.CompensationSafe = false
	if err := staged.Commit(); err != nil {
		resp.Detail = "the copy is published; cleaning up the old placeholder did not finish: " + boundedText(err.Error(), 200)
	}
	resp.Cloned = true
	if resp.Detail == "" {
		resp.Detail = fmt.Sprintf("copied; the address was changed in %d values", stats.Replaced)
	}
	return nil
}

// transformDumpFile changes the address in a gzip'd dump into a new one.
func transformDumpFile(source, destination string, pairs []addressPair) (replaceStats, error) {
	in, _, err := secureOpenRegular(source)
	if err != nil {
		return replaceStats{}, err
	}
	defer in.Close()
	out, err := openPrivateExclusive(destination)
	if err != nil {
		return replaceStats{}, err
	}
	stats, err := gzipTransform(in, out, pairs)
	closeErr := out.Close()
	if err == nil {
		err = closeErr
	}
	return stats, err
}

// copyDatabaseIdentity names a MariaDB database for the backup engine's dump
// and load helpers (the id is only their bookkeeping).
func copyDatabaseIdentity(name string) backupspec.DatabaseIdentity {
	return backupspec.DatabaseIdentity{ID: 1, Name: name, Type: "mysql"}
}

func gzipTransform(in io.Reader, out io.Writer, pairs []addressPair) (replaceStats, error) {
	reader, err := gzip.NewReader(in)
	if err != nil {
		return replaceStats{}, err
	}
	defer reader.Close()
	writer := gzip.NewWriter(out)
	stats, err := transformSQLDump(reader, writer, pairs)
	if closeErr := writer.Close(); err == nil {
		err = closeErr
	}
	return stats, err
}
