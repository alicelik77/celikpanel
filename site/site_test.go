package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The portal tree the generator writes into, as the repository holds it.
const portal = "../download-portal"

func rendered(t *testing.T) (map[string][]byte, map[string]bool) {
	t.Helper()
	files, used, err := render(false)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	return files, used
}

func pageNames(files map[string][]byte) []string {
	var names []string
	for name := range files {
		if strings.HasSuffix(name, ".html") {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// The committed pages are exactly what the generator writes now. When this
// fails, run `go run ./site` from the repository root and commit the result.
func TestCommittedSiteIsCurrent(t *testing.T) {
	files, _ := rendered(t)
	for name, want := range files {
		got, err := os.ReadFile(filepath.Join(portal, filepath.FromSlash(name)))
		if err != nil {
			t.Errorf("%s is not committed: %v (run `go run ./site`)", name, err)
			continue
		}
		if !bytes.Equal(got, want) {
			t.Errorf("download-portal/%s differs from what site/ generates (run `go run ./site`)", name)
		}
	}
	got, err := os.ReadFile(filepath.Join(portal, manifestName))
	if err != nil || !bytes.Equal(got, manifest(files)) {
		t.Errorf("download-portal/%s is not the manifest of the generated files (run `go run ./site`)", manifestName)
	}
	// No other page at the top level or in tr/: the builder ships only listed pages.
	for _, dir := range []string{"", "tr"} {
		entries, err := os.ReadDir(filepath.Join(portal, dir))
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			name := path.Join(dir, entry.Name())
			if strings.HasSuffix(name, ".html") {
				if _, ok := files[name]; !ok {
					t.Errorf("download-portal/%s is a page the generator does not write", name)
				}
			}
		}
	}
}

// Every page renders in both languages from the same catalogue keys, and
// every key is used: a text cannot exist in one language only, or linger.
func TestCataloguesMatchAndAreUsed(t *testing.T) {
	_, used := rendered(t)
	text, err := loadCatalogues()
	if err != nil {
		t.Fatal(err)
	}
	for _, lang := range languages {
		for key, value := range text[lang] {
			if strings.TrimSpace(value) == "" {
				t.Errorf("%s.json: %q is empty", lang, key)
			}
			if !used[key] {
				t.Errorf("%s.json: %q is not used by any page", lang, key)
			}
			for _, other := range languages {
				if _, ok := text[other][key]; !ok {
					t.Errorf("%q is in %s.json but not in %s.json", key, lang, other)
				}
			}
			if strings.HasSuffix(key, "_html") {
				rest := allowedMarkup.ReplaceAllString(value, "")
				if strings.ContainsAny(rest, "<>") {
					t.Errorf("%s.json: %q carries markup outside the allowed inline elements", lang, key)
				}
			} else if strings.ContainsAny(value, "<>") {
				t.Errorf("%s.json: %q carries markup but is not an _html key", lang, key)
			}
		}
	}
	for _, page := range pages {
		for _, lang := range languages {
			if _, ok := page.Paths[lang]; !ok {
				t.Errorf("page %s has no %s version", page.Key, lang)
			}
		}
	}
}

var (
	reference = regexp.MustCompile(`(href|src|srcset)="([^"]*)"`)
	idAttr    = regexp.MustCompile(` id="([^"]+)"`)
	tagToken  = regexp.MustCompile(`<(/?)([a-zA-Z][a-zA-Z0-9]*)((?:\s+[a-zA-Z_:][-a-zA-Z0-9_:.]*(?:="[^"]*")?)*)\s*(/?)>`)
)

var voidElements = map[string]bool{"area": true, "base": true, "br": true, "col": true, "embed": true, "hr": true, "img": true, "input": true, "link": true, "meta": true, "source": true, "track": true, "wbr": true}

// The SVG elements the sprite writes self-closed.
var selfClosing = map[string]bool{"path": true, "circle": true, "line": true, "polyline": true, "polygon": true, "rect": true, "ellipse": true}

// Structure: every element closes in order, attributes are quoted, ids are
// unique, every image has a text alternative and its size, and the document
// declares its language.
func TestPagesAreWellFormed(t *testing.T) {
	files, _ := rendered(t)
	for _, name := range pageNames(files) {
		page := string(files[name])
		if !strings.HasPrefix(page, "<!doctype html>\n<html lang=\"") {
			t.Errorf("%s: no doctype or document language", name)
		}
		var stack []string
		for _, match := range tagToken.FindAllStringSubmatchIndex(page, -1) {
			closing, tag, selfClose := page[match[2]:match[3]] == "/", strings.ToLower(page[match[4]:match[5]]), page[match[8]:match[9]] == "/"
			switch {
			case closing:
				if len(stack) == 0 || stack[len(stack)-1] != tag {
					t.Errorf("%s: </%s> closes %v", name, tag, stack)
					stack = nil
					continue
				}
				stack = stack[:len(stack)-1]
			case voidElements[tag], selfClose && selfClosing[tag]:
			case selfClose:
				t.Errorf("%s: <%s/> is not a void element", name, tag)
			default:
				stack = append(stack, tag)
			}
		}
		if len(stack) != 0 {
			t.Errorf("%s: unclosed elements %v", name, stack)
		}
		if strings.Count(page, "<") != strings.Count(page, ">") {
			t.Errorf("%s: an angle bracket in text is not escaped", name)
		}
		seen := map[string]bool{}
		for _, id := range idAttr.FindAllStringSubmatch(page, -1) {
			if seen[id[1]] {
				t.Errorf("%s: duplicate id %q", name, id[1])
			}
			seen[id[1]] = true
		}
		for _, img := range regexp.MustCompile(`<img [^>]*>`).FindAllString(page, -1) {
			for _, attribute := range []string{` alt="`, ` width="`, ` height="`} {
				if !strings.Contains(img, attribute) {
					t.Errorf("%s: an image without%s: %s", name, attribute, img)
				}
			}
		}
	}
}

// The Content-Security-Policy the portal sends (download-portal/.htaccess)
// allows only the site's own scripts, styles, images and fonts: no inline
// script, no style attribute, no event handler, nothing from outside.
func TestPagesKeepToTheContentSecurityPolicy(t *testing.T) {
	htaccess, err := os.ReadFile(filepath.Join(portal, ".htaccess"))
	if err != nil {
		t.Fatal(err)
	}
	for _, rule := range []string{"script-src 'self'", "style-src 'self'", "img-src 'self'", "font-src 'self'", "default-src 'none'"} {
		if !bytes.Contains(htaccess, []byte(rule)) {
			t.Fatalf(".htaccess no longer sends %q; review this test", rule)
		}
	}
	files, _ := rendered(t)
	inlineScript := regexp.MustCompile(`<script(?:\s[^>]*)?>[^<]`)
	handler := regexp.MustCompile(`\son[a-z]+="`)
	for _, name := range pageNames(files) {
		page := string(files[name])
		switch {
		case inlineScript.MatchString(page):
			t.Errorf("%s: inline script", name)
		case strings.Contains(page, " style=\"") || strings.Contains(page, "<style"):
			t.Errorf("%s: inline style", name)
		case handler.MatchString(page):
			t.Errorf("%s: inline event handler", name)
		}
		for _, script := range regexp.MustCompile(`<script[^>]*>`).FindAllString(page, -1) {
			if !strings.Contains(script, ` src="`) {
				t.Errorf("%s: script without src: %s", name, script)
			}
		}
		for _, ref := range references(page) {
			target, err := url.Parse(ref[1])
			if err != nil {
				t.Errorf("%s: unparsable address %q", name, ref[1])
				continue
			}
			if target.Scheme != "" && regexp.MustCompile(`\.(css|js|woff2|webp|png|svg|jpg)$`).MatchString(target.Path) {
				t.Errorf("%s: loads %s from outside the site", name, ref[1])
			}
		}
	}
	for _, name := range assetFiles {
		if strings.HasSuffix(name, ".css") && bytes.Contains(files["assets/"+name], []byte("@import")) {
			t.Errorf("assets/%s imports another stylesheet", name)
		}
	}
}

// Every relative address resolves to a generated file, to a file the portal
// ships, or to the membership pages; every in-page anchor exists.
func TestLinksResolve(t *testing.T) {
	files, _ := rendered(t)
	ids := map[string]map[string]bool{}
	for _, name := range pageNames(files) {
		ids[name] = map[string]bool{}
		for _, id := range idAttr.FindAllStringSubmatch(string(files[name]), -1) {
			ids[name][id[1]] = true
		}
	}
	exists := func(target string) bool {
		if _, ok := files[target]; ok {
			return true
		}
		if target == "release-signing-ed25519.pem" {
			// The builder publishes the tracked key at the site root.
			_, err := os.Stat("../deploy/release-signing-ed25519.pem")
			return err == nil
		}
		info, err := os.Stat(filepath.Join(portal, filepath.FromSlash(target)))
		return err == nil && !info.IsDir()
	}
	for _, name := range pageNames(files) {
		page := string(files[name])
		for _, ref := range references(page) {
			raw := ref[1]
			if strings.Contains(raw, "@@CP_") {
				if !strings.HasPrefix(raw, "/releases/") && !strings.HasPrefix(raw, "@@CP_") && !strings.HasPrefix(raw, "https://github.com/alicelik77/celikpanel/blob/main/docs/RELEASE-NOTES-") {
					t.Errorf("%s: unexpected placeholder address %q", name, raw)
				}
				continue
			}
			target, err := url.Parse(strings.ReplaceAll(raw, "&amp;", "&"))
			if err != nil {
				t.Errorf("%s: %q: %v", name, raw, err)
				continue
			}
			if target.Scheme == "https" {
				if target.Host != "celikpanel.net" && target.Host != "github.com" {
					t.Errorf("%s: links an unexpected host %q", name, target.Host)
				}
				if target.Host == "github.com" && strings.HasPrefix(target.Path, "/alicelik77/celikpanel/blob/main/") {
					file := strings.TrimPrefix(target.Path, "/alicelik77/celikpanel/blob/main/")
					if _, err := os.Stat(filepath.Join("..", filepath.FromSlash(file))); err != nil {
						t.Errorf("%s: links a repository file that does not exist: %s", name, file)
					}
				}
				continue
			}
			if target.Scheme != "" {
				t.Errorf("%s: unexpected address %q", name, raw)
				continue
			}
			resolved := name
			if target.Path != "" {
				resolved = path.Join(path.Dir(name), target.Path)
				if strings.HasPrefix(resolved, "..") {
					t.Errorf("%s: %q leaves the site", name, raw)
					continue
				}
				if strings.HasSuffix(target.Path, "/") || target.Path == "." || target.Path == "./" {
					resolved = path.Join(resolved, "index.html")
				}
			}
			switch {
			case resolved == "account/index.html":
				if _, err := os.Stat(filepath.Join(portal, "account", "index.php")); err != nil {
					t.Errorf("%s: the membership pages are missing", name)
				}
				if action := target.Query().Get("action"); action != "login" && action != "register" && action != "terms" {
					t.Errorf("%s: unknown account action %q", name, action)
				}
			case !exists(resolved):
				t.Errorf("%s: %q points to %s, which does not exist", name, raw, resolved)
			case target.Fragment != "" && strings.HasSuffix(resolved, ".html"):
				if target, ok := ids[resolved]; ok && !target[strings.SplitN(raw, "#", 2)[1]] {
					t.Errorf("%s: %q names an anchor that %s does not have", name, raw, resolved)
				}
			}
		}
	}
}

// The release placeholders are the ones the builder fills, the signed block
// is well formed, and the pages that show the release carry it.
func TestReleasePlaceholders(t *testing.T) {
	files, _ := rendered(t)
	for _, name := range pageNames(files) {
		page := files[name]
		for _, found := range placeholdersIn(page) {
			if !knownPlaceholders[found] {
				t.Errorf("%s: unknown placeholder %s", name, found)
			}
		}
		if err := signedBlocksWellFormed(page); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	builder, err := os.ReadFile("../deploy/build-download-portal.sh")
	if err != nil {
		t.Fatal(err)
	}
	for found := range knownPlaceholders {
		if !bytes.Contains(builder, []byte(found)) {
			t.Errorf("deploy/build-download-portal.sh does not fill %s", found)
		}
	}
	for _, name := range []string{"releases.html", "tr/surumler.html"} {
		page := string(files[name])
		for _, want := range []string{phVersion, phSHA256, phCommit, phArchiveURL, signedStart} {
			if !strings.Contains(page, want) {
				t.Errorf("%s does not show %s", name, want)
			}
		}
	}
	filled := string(fillRelease(files["releases.html"], release{Version: "v1.2.3", Commit: "c", CommitShort: "c", SHA256: "s"}))
	if placeholder.MatchString(filled) || strings.Contains(filled, "cp:") {
		t.Error("the preview's substitution leaves a placeholder or marker")
	}
}

// The files the pages show are present: the product screens in both
// languages and themes at the size the pages declare, and the typefaces.
func TestRequiredFilesArePresent(t *testing.T) {
	for name, size := range shots {
		for _, lang := range languages {
			for _, theme := range []string{"light", "dark"} {
				for suffix, want := range map[string][2]int{"": size, "-2x": {size[0] * 2, size[1] * 2}, "-phone": phoneShot} {
					file := filepath.Join(portal, "assets", "shots", fmt.Sprintf("%s-%s-%s%s.webp", name, lang, theme, suffix))
					raw, err := os.ReadFile(file)
					if err != nil {
						t.Errorf("product screen missing: %v", err)
						continue
					}
					width, height, err := webpSize(raw)
					if err != nil || width != want[0] || height != want[1] {
						t.Errorf("%s: %dx%d (%v), the pages declare %dx%d", file, width, height, err, want[0], want[1])
					}
					if len(raw) > 120*1024 {
						t.Errorf("%s weighs %d bytes; keep a screen under 120 KiB", file, len(raw))
					}
				}
			}
		}
	}
	for _, font := range []string{"overpass-latin.woff2", "overpass-latin-ext.woff2", "overpass-mono-latin.woff2", "overpass-mono-latin-ext.woff2", "OFL-Overpass.txt", "OFL-OverpassMono.txt"} {
		if _, err := os.Stat(filepath.Join(portal, "assets", "fonts", font)); err != nil {
			t.Errorf("typeface file missing: %v", err)
		}
	}
	files, _ := rendered(t)
	if size := len(files["assets/public.css"]); size > 40*1024 {
		t.Errorf("assets/public.css is %d bytes; the budget is 40 KiB", size)
	}
}

// The old addresses keep working: /systems.html is the English systems page
// and /technical.html points to the section that took its content.
func TestOldAddressesAreKept(t *testing.T) {
	files, _ := rendered(t)
	if !bytes.Contains(files["systems.html"], []byte(`<html lang="en" data-page="systems">`)) {
		t.Error("systems.html is no longer the supported-systems page")
	}
	moved := string(files["technical.html"])
	for _, want := range []string{`href="download.html#how"`, `href="tr/indir.html#nasil"`, `http-equiv="refresh"`} {
		if !strings.Contains(moved, want) {
			t.Errorf("technical.html lacks %s", want)
		}
	}
}

// webpSize reads the canvas size of a lossy, lossless or extended WebP.
func webpSize(raw []byte) (int, int, error) {
	if len(raw) < 30 || string(raw[0:4]) != "RIFF" || string(raw[8:12]) != "WEBP" {
		return 0, 0, fmt.Errorf("not a WebP file")
	}
	switch string(raw[12:16]) {
	case "VP8 ":
		return int(binary.LittleEndian.Uint16(raw[26:28]) & 0x3fff), int(binary.LittleEndian.Uint16(raw[28:30]) & 0x3fff), nil
	case "VP8L":
		bits := binary.LittleEndian.Uint32(raw[21:25])
		return int(bits&0x3fff) + 1, int((bits>>14)&0x3fff) + 1, nil
	case "VP8X":
		width := int(raw[24]) | int(raw[25])<<8 | int(raw[26])<<16
		height := int(raw[27]) | int(raw[28])<<8 | int(raw[29])<<16
		return width + 1, height + 1, nil
	}
	return 0, 0, fmt.Errorf("unknown WebP chunk %q", raw[12:16])
}

// references lists every address a page names, as [attribute value, address]
// pairs; each candidate of a srcset is its own address.
func references(page string) [][]string {
	var result [][]string
	for _, match := range reference.FindAllStringSubmatch(page, -1) {
		if match[1] != "srcset" {
			result = append(result, []string{match[2], match[2]})
			continue
		}
		for _, candidate := range strings.Split(match[2], ",") {
			fields := strings.Fields(candidate)
			if len(fields) > 0 {
				result = append(result, []string{fields[0], fields[0]})
			}
		}
	}
	return result
}
