package main

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

//go:embed templates/*.html text/*.json assets/*
var sources embed.FS

// The languages, in the order the language switch shows them.
var languages = []string{"en", "tr"}

// Every page: its key (template and catalogue prefix) and its output path per
// language. English is the root of the site; Turkish lives under tr/.
// /systems.html keeps its old address; /technical.html becomes a pointer to
// the download page, where its content now lives.
var pages = []struct {
	Key   string
	Paths map[string]string
}{
	{"home", map[string]string{"en": "index.html", "tr": "tr/index.html"}},
	{"pricing", map[string]string{"en": "pricing.html", "tr": "tr/fiyat.html"}},
	{"download", map[string]string{"en": "download.html", "tr": "tr/indir.html"}},
	{"systems", map[string]string{"en": "systems.html", "tr": "tr/sistemler.html"}},
	{"releases", map[string]string{"en": "releases.html", "tr": "tr/surumler.html"}},
}

// Old addresses that are now pointers to a section of a new page.
var moved = []struct {
	Path     string
	Target   string // page key
	Fragment map[string]string
}{
	{"technical.html", "download", map[string]string{"en": "how", "tr": "nasil"}},
}

// Files copied verbatim from assets/ into <out>/assets/.
var assetFiles = []string{"public.css", "public.js", "boot.js"}

// Product screens: assets/shots/<name>-<lang>-<theme>.webp, taken from the
// current panel build with sample data (web/tools/browser-inspect, batch 37).
var shots = map[string][2]int{
	"dashboard": {1440, 900},
	"domains":   {1440, 900},
	"databases": {1440, 900},
}

const manifestName = "site.sha256"

type catalogue map[string]string

func loadCatalogues() (map[string]catalogue, error) {
	result := map[string]catalogue{}
	for _, lang := range languages {
		raw, err := sources.ReadFile("text/" + lang + ".json")
		if err != nil {
			return nil, err
		}
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		var text catalogue
		if err := decoder.Decode(&text); err != nil {
			return nil, fmt.Errorf("text/%s.json: %w", lang, err)
		}
		result[lang] = text
	}
	return result, nil
}

// view is what a page template sees.
type view struct {
	Key     string
	Lang    string
	Path    string
	Preview bool
}

// root is the relative way back to the site root from the page.
func (v view) root() string {
	return strings.Repeat("../", strings.Count(v.Path, "/"))
}

func pagePath(key, lang string) (string, bool) {
	for _, page := range pages {
		if page.Key == key {
			p, ok := page.Paths[lang]
			return p, ok
		}
	}
	return "", false
}

// href is the relative address of another page from this one. Published
// pages name a directory's index by the directory (./, tr/); the disk
// preview names the file, because a browser opens a directory listing from
// disk.
func (v view) href(target string) string {
	from := path.Dir(v.Path)
	dir, file := path.Split(target)
	dir = path.Clean("/" + dir)
	rel := ""
	// Walk up from the page's directory until the target's directory is
	// below it, then down to the target.
	for here := path.Clean("/" + from); ; here = path.Dir(here) {
		if here == "/" || dir == here || strings.HasPrefix(dir, here+"/") {
			down := strings.TrimPrefix(strings.TrimPrefix(dir, here), "/")
			if down != "" {
				rel += down + "/"
			}
			break
		}
		rel += "../"
	}
	if v.Preview || file != "index.html" {
		rel += file
	}
	if rel == "" {
		rel = "./"
	}
	return rel
}

// renderer renders every page and records the catalogue keys it used.
type renderer struct {
	text    map[string]catalogue
	used    map[string]bool
	preview bool
}

var allowedMarkup = regexp.MustCompile(`</?(strong|em|code|br|abbr)( title="[^"<>]*")?>`)

func (r *renderer) funcs(v view) template.FuncMap {
	lookup := func(key string) (string, error) {
		value, ok := r.text[v.Lang][key]
		if !ok {
			return "", fmt.Errorf("catalogue %s has no key %q (page %s)", v.Lang, key, v.Path)
		}
		r.used[key] = true
		return value, nil
	}
	return template.FuncMap{
		"t": lookup,
		// th returns reviewed catalogue markup; only keys ending in "_html"
		// may carry it, and only the few inline elements allowedMarkup names.
		"th": func(key string) (template.HTML, error) {
			if !strings.HasSuffix(key, "_html") {
				return "", fmt.Errorf("th used on %q, which is not an _html key", key)
			}
			value, err := lookup(key)
			return template.HTML(value), err
		},
		"link": func(key string, fragment ...string) (string, error) {
			target, ok := pagePath(key, v.Lang)
			if !ok {
				return "", fmt.Errorf("no page %q in %s", key, v.Lang)
			}
			result := v.href(target)
			if len(fragment) > 0 && fragment[0] != "" {
				result += "#" + fragment[0]
			}
			return result, nil
		},
		// in is the same page in another language.
		"in": func(lang string) (string, error) {
			target, ok := pagePath(v.Key, lang)
			if !ok {
				return "", fmt.Errorf("page %q has no %s version", v.Key, lang)
			}
			return v.href(target), nil
		},
		"asset": func(name string) string { return v.root() + "assets/" + name },
		"guide": func(chapter string) string {
			if chapter == "" {
				if v.Preview {
					return v.root() + "guide/" + v.Lang + "/index.html"
				}
				return v.root() + "guide/" + v.Lang + "/"
			}
			return v.root() + "guide/" + v.Lang + "/" + chapter + ".html"
		},
		// account names the existing membership pages (portal-membership,
		// unchanged by this site).
		"account": func(action string) string {
			return v.root() + "account/?action=" + action + "&lang=" + v.Lang
		},
		"feedback": func(kind string) (string, error) {
			const base = "https://github.com/alicelik77/celikpanel-feedback"
			switch kind {
			case "bug", "feature":
				return base + "/issues/new?template=" + kind + "_" + v.Lang + ".yml", nil
			case "security":
				return base + "/security/advisories/new", nil
			}
			return "", fmt.Errorf("unknown feedback kind %q", kind)
		},
		"repo": func(file string) string {
			return "https://github.com/alicelik77/celikpanel/blob/main/" + file
		},
		"shot": func(name string) (map[string]any, error) {
			size, ok := shots[name]
			if !ok {
				return nil, fmt.Errorf("no product screen %q", name)
			}
			prefix := v.root() + "assets/shots/" + name + "-" + v.Lang
			return map[string]any{
				"Light": prefix + "-light.webp", "Dark": prefix + "-dark.webp",
				"Width": size[0], "Height": size[1],
			}, nil
		},
		"icon": func(name string) template.HTML {
			return template.HTML(`<svg class="i i-` + name + `" aria-hidden="true" focusable="false"><use href="#i-` + name + `"></use></svg>`)
		},
		"current": func(key string) bool { return key == v.Key },
		"root":    v.root,
		"dict": func(pairs ...any) (map[string]any, error) {
			if len(pairs)%2 != 0 {
				return nil, errors.New("dict needs key and value pairs")
			}
			result := map[string]any{}
			for i := 0; i < len(pairs); i += 2 {
				key, ok := pairs[i].(string)
				if !ok {
					return nil, fmt.Errorf("dict key %v is not a string", pairs[i])
				}
				result[key] = pairs[i+1]
			}
			return result, nil
		},
		"list": func(items ...string) []string { return items },
		// dots sets the separating middle dots of a short line in their own
		// span: Overpass draws the dot against the following word.
		"dots": func(text string) template.HTML {
			return template.HTML(strings.ReplaceAll(template.HTMLEscapeString(text), " · ", ` <span class="dot">·</span> `))
		},
		// marker writes a signed-block marker (html/template drops comments
		// written in a template).
		"marker": func(which string) (template.HTML, error) {
			switch which {
			case "start":
				return template.HTML(signedStart), nil
			case "end":
				return template.HTML(signedEnd), nil
			}
			return "", fmt.Errorf("unknown marker %q", which)
		},
		"other": func() string { return map[string]string{"en": "tr", "tr": "en"}[v.Lang] },
		"langs": func() []string { return languages },
		"canonical": func() string {
			p, _ := pagePath(v.Key, v.Lang)
			return "https://celikpanel.net/" + strings.TrimSuffix(p, "index.html")
		},
		"absolute": func(lang string) string {
			p, _ := pagePath(v.Key, lang)
			return "https://celikpanel.net/" + strings.TrimSuffix(p, "index.html")
		},
	}
}

func (r *renderer) base() (*template.Template, error) {
	return template.New("site").Funcs(r.funcs(view{})).ParseFS(sources, "templates/layout.html", "templates/parts.html")
}

func (r *renderer) page(key, lang, out string) ([]byte, error) {
	base, err := r.base()
	if err != nil {
		return nil, err
	}
	v := view{Key: key, Lang: lang, Path: out, Preview: r.preview}
	set, err := base.Clone()
	if err != nil {
		return nil, err
	}
	set = set.Funcs(r.funcs(v))
	if key != "moved" {
		if set, err = set.ParseFS(sources, "templates/"+key+".html"); err != nil {
			return nil, err
		}
	}
	var buffer bytes.Buffer
	name := "layout"
	if key == "moved" {
		name = "moved"
	}
	if err := set.ExecuteTemplate(&buffer, name, v); err != nil {
		return nil, err
	}
	return tidy(buffer.Bytes()), nil
}

// tidy removes the blank lines and trailing spaces template actions leave.
func tidy(page []byte) []byte {
	lines := strings.Split(string(page), "\n")
	kept := lines[:0]
	for _, line := range lines {
		line = strings.TrimRight(line, " \t\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		kept = append(kept, line)
	}
	return []byte(strings.Join(kept, "\n") + "\n")
}

// render returns every generated file by its path below the site root.
func render(preview bool) (map[string][]byte, map[string]bool, error) {
	text, err := loadCatalogues()
	if err != nil {
		return nil, nil, err
	}
	r := &renderer{text: text, used: map[string]bool{}, preview: preview}
	files := map[string][]byte{}
	for _, page := range pages {
		for _, lang := range languages {
			out, ok := page.Paths[lang]
			if !ok {
				return nil, nil, fmt.Errorf("page %s has no %s path", page.Key, lang)
			}
			body, err := r.page(page.Key, lang, out)
			if err != nil {
				return nil, nil, fmt.Errorf("%s: %w", out, err)
			}
			files[out] = body
		}
	}
	for _, item := range moved {
		body, err := r.movedPage(item.Path, item.Target, item.Fragment)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", item.Path, err)
		}
		files[item.Path] = body
	}
	for _, name := range assetFiles {
		raw, err := sources.ReadFile("assets/" + name)
		if err != nil {
			return nil, nil, err
		}
		// A Windows checkout may hand the sources over with CRLF; the
		// published bytes are always LF, so their digests match everywhere.
		raw = bytes.ReplaceAll(raw, []byte("\r\n"), []byte("\n"))
		if strings.HasSuffix(name, ".css") {
			raw = minifyCSS(raw)
		}
		files["assets/"+name] = raw
	}
	return files, r.used, nil
}

var (
	cssComment    = regexp.MustCompile(`(?s)/\*.*?\*/`)
	cssSpace      = regexp.MustCompile(`\s+`)
	cssPunctSpace = regexp.MustCompile(`\s*([{};,>])\s*`)
)

// minifyCSS drops comments and the whitespace that carries no meaning: runs
// collapse to one space, and none is kept around braces, semicolons, commas
// and child combinators. A space before a colon is kept, because in a
// selector it is a descendant combinator (".gantry :focus-visible"). The
// readable source stays in site/assets/public.css.
func minifyCSS(source []byte) []byte {
	out := cssComment.ReplaceAll(source, nil)
	out = cssSpace.ReplaceAll(out, []byte(" "))
	out = cssPunctSpace.ReplaceAll(out, []byte("$1"))
	return append(bytes.TrimSpace(out), '\n')
}

// movedPage is a small bilingual pointer for an old address. Without
// JavaScript it moves to the English section after a moment; boot.js moves a
// visitor who chose Turkish to the Turkish one at once.
func (r *renderer) movedPage(out, target string, fragment map[string]string) ([]byte, error) {
	base, err := r.base()
	if err != nil {
		return nil, err
	}
	v := view{Key: "moved", Lang: "en", Path: out, Preview: r.preview}
	links := map[string]string{}
	for _, lang := range languages {
		p, ok := pagePath(target, lang)
		if !ok {
			return nil, fmt.Errorf("no %s page %q", lang, target)
		}
		links[lang] = v.href(p) + "#" + fragment[lang]
	}
	set, err := base.Clone()
	if err != nil {
		return nil, err
	}
	en, tr := r.text["en"], r.text["tr"]
	for _, key := range []string{"moved.title", "moved.text", "moved.link"} {
		if _, ok := en[key]; !ok {
			return nil, fmt.Errorf("catalogue en has no key %q", key)
		}
		if _, ok := tr[key]; !ok {
			return nil, fmt.Errorf("catalogue tr has no key %q", key)
		}
		r.used[key] = true
	}
	data := map[string]any{
		"View": v, "EN": links["en"], "TR": links["tr"],
		"TitleEN": en["moved.title"], "TitleTR": tr["moved.title"],
		"TextEN": en["moved.text"], "TextTR": tr["moved.text"],
		"LinkEN": en["moved.link"], "LinkTR": tr["moved.link"],
		"CSS": v.root() + "assets/public.css", "Boot": v.root() + "assets/boot.js",
		"Icon": v.root() + "assets/favicon-v2.svg",
	}
	var buffer bytes.Buffer
	if err := set.ExecuteTemplate(&buffer, "moved", data); err != nil {
		return nil, err
	}
	return tidy(buffer.Bytes()), nil
}

func manifest(files map[string][]byte) []byte {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	var buffer bytes.Buffer
	for _, name := range names {
		sum := sha256.Sum256(files[name])
		fmt.Fprintf(&buffer, "%s  %s\n", hex.EncodeToString(sum[:]), name)
	}
	return buffer.Bytes()
}

// writeSite writes the generated files and their manifest into out, and
// removes a file the previous manifest listed that is no longer generated.
func writeSite(out string) error {
	files, _, err := render(false)
	if err != nil {
		return err
	}
	if previous, err := os.ReadFile(filepath.Join(out, manifestName)); err == nil {
		for _, line := range strings.Split(strings.TrimSpace(string(previous)), "\n") {
			fields := strings.Fields(line)
			if len(fields) != 2 {
				continue
			}
			if _, still := files[fields[1]]; !still {
				if err := os.Remove(filepath.Join(out, filepath.FromSlash(fields[1]))); err != nil && !errors.Is(err, fs.ErrNotExist) {
					return err
				}
			}
		}
	}
	for name, body := range files {
		target := filepath.Join(out, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, body, 0o644); err != nil {
			return err
		}
	}
	return os.WriteFile(filepath.Join(out, manifestName), manifest(files), 0o644)
}

// writePreview writes a copy of the site that opens from disk: the pages
// with file links, the portal's assets and guide beside them, and the
// release placeholders filled with the release download-portal/get.sh pins.
func writePreview(portal, dir string) error {
	files, _, err := render(true)
	if err != nil {
		return err
	}
	release, err := currentRelease(portal)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for name, body := range files {
		if strings.HasSuffix(name, ".html") {
			body = fillRelease(body, release)
		}
		target := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, body, 0o644); err != nil {
			return err
		}
	}
	for _, sub := range []string{"assets/fonts", "assets/shots", "guide"} {
		if err := copyTree(filepath.Join(portal, filepath.FromSlash(sub)), filepath.Join(dir, filepath.FromSlash(sub))); err != nil {
			return err
		}
	}
	for _, name := range []string{"assets/favicon-v2.svg", "assets/site.css", "assets/guide.css"} {
		if err := copyFile(filepath.Join(portal, filepath.FromSlash(name)), filepath.Join(dir, filepath.FromSlash(name))); err != nil {
			return err
		}
	}
	return nil
}

func copyTree(from, to string) error {
	return filepath.WalkDir(from, func(p string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(from, p)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.MkdirAll(filepath.Join(to, rel), 0o755)
		}
		if path.Ext(entry.Name()) == ".sha256" {
			return nil
		}
		return copyFile(p, filepath.Join(to, rel))
	})
}

func copyFile(from, to string) error {
	raw, err := os.ReadFile(from)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}
	return os.WriteFile(to, raw, 0o644)
}
