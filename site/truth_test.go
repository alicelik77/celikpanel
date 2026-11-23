package main

// The site's truth rules, checked mechanically (owner, 19–23 November 2026):
// every number has a source in the repository; no other control panel's
// product name appears anywhere in the site; titles and headings say "Linux
// servers", never two distributions; an optional feature is always called
// optional; and nothing of the rejected decorative design comes back.

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var digit = regexp.MustCompile(`[0-9]`)

// Every catalogue text that holds a number, in either language, names where
// the repository proves it (site/sources.json). Each citation of a line also
// quotes a phrase from that line, and the phrase is the proof (sources.go):
// on the cited line, or exactly once elsewhere in the file (the line moved;
// `go run ./site -fix-sources` brings the number up to date). A phrase the
// file no longer has, or one that occurs several times and not on the cited
// line, fails here. A placeholder the portal builder fills is not a number of
// ours.
func TestEveryNumberHasASource(t *testing.T) {
	raw, err := os.ReadFile("sources.json")
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		About   string                `json:"about"`
		Sources map[string][]citation `json:"sources"`
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&file); err != nil {
		t.Fatalf("sources.json: %v", err)
	}
	text, err := loadCatalogues()
	if err != nil {
		t.Fatal(err)
	}
	for key := range text["en"] {
		numbered := false
		for _, lang := range languages {
			if digit.MatchString(placeholder.ReplaceAllString(text[lang][key], "")) {
				numbered = true
			}
		}
		if numbered && len(file.Sources[key]) == 0 {
			t.Errorf("%q holds a number but sources.json gives no source for it", key)
		}
	}
	files := newRepoFiles("..")
	for key, refs := range file.Sources {
		if _, ok := text["en"][key]; !ok {
			t.Errorf("sources.json names %q, which no catalogue has", key)
		}
		for _, ref := range refs {
			_, line := ref.split()
			if line > 0 && strings.TrimSpace(ref.Says) == "" {
				t.Errorf("%s: source %s quotes no phrase from its line", key, ref.Cite)
				continue
			}
			at, err := files.check(ref)
			switch {
			case err != nil:
				t.Errorf("%s: source %s: %v", key, ref.Cite, err)
			case at != line:
				t.Logf("%s: source %s: the phrase is now on line %d; `go run ./site -fix-sources` updates the number", key, ref.Cite, at)
			}
		}
	}
}

// Other control panels' product names, matched as whole words, any case.
var competitors = regexp.MustCompile(`(?i)\b(cpanel|plesk|directadmin|aapanel|cyberpanel|hestia(cp)?|vestacp|webmin|virtualmin|cloudpanel|cloudron|fastpanel|ispconfig|ispmanager|webuzo|1panel|cloudlinux|coolify|dokploy|easypanel|tinycp|kusanagi|serverkit|froxlor|ajenti|interworx|runcloud|ploi|forge)\b`)

// No other control panel's name appears in the generated site or in the
// site's own sources (texts, templates, styles, scripts, the generator).
func TestNoCompetitorNames(t *testing.T) {
	files, _ := rendered(t)
	for name, body := range files {
		if found := competitors.FindString(string(body)); found != "" {
			t.Errorf("%s names %q", name, found)
		}
	}
	err := filepath.WalkDir(".", func(p string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || strings.HasSuffix(p, "_test.go") {
			return err
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if found := competitors.FindString(string(raw)); found != "" {
			t.Errorf("site/%s names %q", filepath.ToSlash(p), found)
		}
		if found := competitors.FindString(p); found != "" {
			t.Errorf("site/%s: the file name names %q", filepath.ToSlash(p), found)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

var (
	twoDistributions = regexp.MustCompile(`(?i)debian\s*(and|or|&|/|,|ve|ya da|veya|ile)\s*ubuntu`)
	headline         = regexp.MustCompile(`(?s)<(title|h1|h2|h3|caption|figcaption)\b[^>]*>(.*?)</(title|h1|h2|h3|caption|figcaption)>|<meta name="description" content="([^"]*)">`)
)

// Titles, descriptions, headings and captions never tie the product to two
// distributions: CelikPanel is a panel for Linux servers (owner, 23 November
// 2026). The home page says so in its title and first heading, in both
// languages.
func TestHeadlinesSayLinuxServers(t *testing.T) {
	files, _ := rendered(t)
	for _, name := range pageNames(files) {
		for _, match := range headline.FindAllStringSubmatch(string(files[name]), -1) {
			if twoDistributions.MatchString(match[0]) {
				t.Errorf("%s: %q ties the product to two distributions", name, match[0])
			}
		}
	}
	text, err := loadCatalogues()
	if err != nil {
		t.Fatal(err)
	}
	for _, lang := range languages {
		for key, value := range text[lang] {
			product := strings.HasSuffix(key, ".title") || strings.HasSuffix(key, ".description") ||
				strings.HasSuffix(key, ".h1") || strings.HasSuffix(key, ".def") || key == "footer.tagline"
			if product && twoDistributions.MatchString(value) {
				t.Errorf("%s.json %q ties the product to two distributions: %q", lang, key, value)
			}
		}
	}
	for page, wants := range map[string][]string{
		"index.html":    {"<title>CelikPanel · Hosting control panel for Linux servers</title>", "<h1>A hosting control panel for Linux servers</h1>"},
		"tr/index.html": {"<title>CelikPanel · Linux sunucular için hosting kontrol paneli</title>", "<h1>Linux sunucular için hosting kontrol paneli</h1>"},
	} {
		for _, want := range wants {
			if !strings.Contains(string(files[page]), want) {
				t.Errorf("%s lacks %s", page, want)
			}
		}
	}
}

var (
	optionalFeature = regexp.MustCompile(`(?i)second server|two servers|backup dns server|wireguard|\bvpn\b|remote (copy|backup|destination)|outgoing mail limits|ikinci sunucu|iki sunucu|yedek dns sunucu|uzak (kopya|yedek|hedef)|giden posta sınır`)
	optionalWord    = regexp.MustCompile(`(?i)optional|isteğe bağlı`)
	textBlock       = regexp.MustCompile(`(?s)<(tr|li|dd|p|h1|h2|h3|figcaption)\b[^>]*>(.*?)</(tr|li|dd|p|h1|h2|h3|figcaption)>`)
	anyTag          = regexp.MustCompile(`<[^>]+>`)
)

// A feature that a server does not need (a second server, the VPN, remote
// backups, outgoing mail limits) is called optional wherever it is named: in
// the same table row, list item, definition or paragraph.
func TestOptionalFeaturesSayOptional(t *testing.T) {
	files, _ := rendered(t)
	for _, name := range pageNames(files) {
		for _, match := range textBlock.FindAllStringSubmatch(string(files[name]), -1) {
			plain := anyTag.ReplaceAllString(match[2], "")
			if optionalFeature.MatchString(plain) && !optionalWord.MatchString(plain) {
				t.Errorf("%s: %q names an optional feature without saying it is optional", name, strings.TrimSpace(plain))
			}
		}
	}
}

// Nothing of the rejected design comes back: no gradients, glows, blur,
// shadows, transforms or animation in the styles, and no emoji in the texts.
func TestNoDecoration(t *testing.T) {
	files, _ := rendered(t)
	forbidden := regexp.MustCompile(`(?i)gradient|backdrop-filter|box-shadow|text-shadow|[{;]filter:|@keyframes|animation|[{;]transform:|transition`)
	for _, name := range assetFiles {
		if !strings.HasSuffix(name, ".css") {
			continue
		}
		if found := forbidden.FindString(string(files["assets/"+name])); found != "" {
			t.Errorf("assets/%s uses %q", name, found)
		}
	}
	emoji := regexp.MustCompile(`[\x{1F300}-\x{1FAFF}\x{2600}-\x{27BF}]`)
	text, err := loadCatalogues()
	if err != nil {
		t.Fatal(err)
	}
	for _, lang := range languages {
		for key, value := range text[lang] {
			if emoji.MatchString(value) {
				t.Errorf("%s.json %q carries an emoji", lang, key)
			}
		}
	}
}

// What the portal's own contract checks (deploy/test-download-portal-contract.sh
// and deploy/test-download-command-contract.sh) is on the pages, so a text
// change is caught here before the release build refuses it.
func TestPagesCarryWhatTheDeployContractChecks(t *testing.T) {
	files, _ := rendered(t)
	want := map[string][]string{
		"index.html":        {"A hosting control panel for Linux servers", `href="systems.html"`, `href="guide/en/"`, "root terminal", `<code id="cmd-home">`},
		"tr/index.html":     {"Linux sunucular için hosting kontrol paneli", `href="sistemler.html"`, `href="../guide/tr/"`, "root terminalinde", `<code id="cmd-home">`},
		"download.html":     {`href="systems.html"`, `href="guide/en/"`, "root terminal", `<code id="cmd-download">`, `id="how"`},
		"tr/indir.html":     {"root terminalinde", `<code id="cmd-download">`, `id="nasil"`},
		"pricing.html":      {"from version 1.0", `href="systems.html"`, `href="guide/en/"`},
		"tr/fiyat.html":     {"1.0 sürümünden itibaren"},
		"releases.html":     {`href="systems.html"`, `href="guide/en/"`},
		"systems.html":      {`Debian 13 <span class="code-name">trixie`, `Debian 12 <span class="code-name">bookworm`, `Ubuntu 24.04 LTS <span class="code-name">noble`, `Ubuntu 22.04 LTS <span class="code-name">jammy`, "Arch Linux", "sudo pacman -Syu", "archlinux.org/news", "phpPgAdmin", "SpamAssassin", "Valkey", "unattended-upgrades", "Supported operating systems", "How updates work", "is a separate program and not an update", `href="guide/en/"`},
		"tr/sistemler.html": {"Desteklenen işletim sistemleri", "Güncellemeler nasıl işler", "ayrı bir programdır, güncelleme değildir", "sudo pacman -Syu"},
	}
	for page, needles := range want {
		for _, needle := range needles {
			if !strings.Contains(string(files[page]), needle) {
				t.Errorf("%s lacks %q, which the deploy contract checks", page, needle)
			}
		}
	}
	for _, page := range []string{"index.html", "tr/index.html"} {
		lang := map[string]string{"index.html": "en", "tr/index.html": "tr"}[page]
		for _, needle := range []string{
			"alicelik77/celikpanel-feedback/issues/new?template=bug_" + lang + ".yml",
			"alicelik77/celikpanel-feedback/issues/new?template=feature_" + lang + ".yml",
			"alicelik77/celikpanel-feedback/security/advisories/new",
			"account/?action=register&amp;lang=" + lang,
			"account/?action=login&amp;lang=" + lang,
		} {
			if !strings.Contains(string(files[page]), needle) {
				t.Errorf("%s lacks %q", page, needle)
			}
		}
	}
	purchase := regexp.MustCompile(`(?i)<(a|button)[^>]*>[^<]*(buy|satın al|start a (30-day )?trial|denemeyi başlat|deneme başlat)`)
	for _, page := range []string{"index.html", "pricing.html", "tr/index.html", "tr/fiyat.html"} {
		if purchase.MatchString(string(files[page])) {
			t.Errorf("%s offers a purchase or a trial that does not exist yet", page)
		}
	}
	command := regexp.MustCompile(`(?s)<code id="(cmd-[a-z]+)">(.*?)</code>`)
	var seen string
	for _, page := range []string{"index.html", "download.html", "tr/index.html", "tr/indir.html"} {
		found := command.FindAllStringSubmatch(string(files[page]), -1)
		if len(found) == 0 {
			t.Errorf("%s shows no install command", page)
		}
		for _, match := range found {
			if strings.Count(string(files[page]), `data-copy="`+match[1]+`"`) != 1 {
				t.Errorf("%s: the command %s has no single Copy button", page, match[1])
			}
			body := anyTag.ReplaceAllString(match[2], "")
			if seen == "" {
				seen = body
			} else if body != seen {
				t.Errorf("%s shows a different install command", page)
			}
			if lines := strings.Split(body, "\n"); len(lines) != 3 || lines[0] != "timedatectl set-ntp true" {
				t.Errorf("%s: the install command is not the three lines starting with the clock line", page)
			}
		}
	}
}
