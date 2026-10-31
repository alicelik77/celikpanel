package services

import (
	"regexp"
	"strings"
	"testing"
)

// nginxFolderRedirect answers a request the way nginx runs the rendered
// folder redirect: `location = <source>` for the folder itself, otherwise the
// `location ^~ <source>/` block: its `if ($request_uri ~ ...)` regex, then
// the fallback return. Only the directives the template writes are known.
func nginxFolderRedirect(t *testing.T, text, source, requestURI string) string {
	t.Helper()
	path, args, hasArgs := strings.Cut(requestURI, "?")
	isArgs := ""
	if hasArgs {
		isArgs = "?"
	}
	expand := func(value string, rest string) string {
		value = strings.ReplaceAll(value, "$celikpanel_rest", rest)
		value = strings.ReplaceAll(value, "$is_args", isArgs)
		return strings.ReplaceAll(value, "$args", args)
	}
	block := func(header string) []string {
		start := strings.Index(text, header)
		if start < 0 {
			t.Fatalf("missing %q in:\n%s", header, text)
		}
		body := text[start+len(header):]
		return strings.Split(body[:strings.Index(body, "\n    }")], "\n")
	}
	returnTarget := func(line string) string {
		fields := strings.Fields(strings.TrimSuffix(strings.TrimSpace(line), ";"))
		if len(fields) != 3 || fields[0] != "return" {
			t.Fatalf("not a return: %q", line)
		}
		return fields[1] + " " + fields[2]
	}
	if path == source {
		lines := block("location = " + source + " {")
		return expand(returnTarget(lines[1]), "")
	}
	lines := block("location ^~ " + source + "/ {")
	condition := strings.TrimSpace(lines[1])
	if !strings.HasPrefix(condition, "if ($request_uri ~ ") || !strings.HasSuffix(condition, ") {") {
		t.Fatalf("unexpected condition %q", condition)
	}
	pattern := regexp.MustCompile(strings.TrimSuffix(strings.TrimPrefix(condition, "if ($request_uri ~ "), ") {"))
	if match := pattern.FindStringSubmatch(requestURI); match != nil {
		return expand(returnTarget(lines[2]), match[pattern.SubexpIndex("celikpanel_rest")])
	}
	return expand(returnTarget(lines[4]), "")
}

// E5 (alpha.114 lab): without "keep query", /blog/post?q=1 went to the bare
// target, although the page promises the rest of the path is carried over.
func TestFolderRedirectCarriesTheRestOfThePathWithAndWithoutAQuery(t *testing.T) {
	ng := featureGenerator(t)
	render := func(keepQuery bool) string {
		data := featureVhost("php")
		data.Redirects = []SiteRedirect{{Match: RedirectMatchPrefix, Source: "/blog", Target: "https://example.org/archive", Code: 302, KeepQuery: keepQuery}}
		text, err := ng.Render(data)
		if err != nil {
			t.Fatal(err)
		}
		return text
	}
	plain, keep := render(false), render(true)
	if !strings.Contains(plain, `if ($request_uri ~ ^/blog/(?<celikpanel_rest>[^?]*)) {`) {
		t.Fatalf("without keep query:\n%s", plain)
	}
	if !strings.Contains(keep, `if ($request_uri ~ ^/blog/(?<celikpanel_rest>.*)$) {`) {
		t.Fatalf("with keep query:\n%s", keep)
	}
	for _, c := range []struct {
		keep      bool
		uri, want string
	}{
		{false, "/blog/post", "302 https://example.org/archive/post"},
		{false, "/blog/post?q=1", "302 https://example.org/archive/post"},
		{false, "/blog/a/b.html?x=1&y=2", "302 https://example.org/archive/a/b.html"},
		{false, "/blog/", "302 https://example.org/archive/"},
		{false, "/blog/?q=1", "302 https://example.org/archive/"},
		{false, "/blog", "302 https://example.org/archive"},
		{false, "/blog?q=1", "302 https://example.org/archive"},
		{true, "/blog/post", "302 https://example.org/archive/post"},
		{true, "/blog/post?q=1", "302 https://example.org/archive/post?q=1"},
		{true, "/blog/a/b.html?x=1&y=2", "302 https://example.org/archive/a/b.html?x=1&y=2"},
		{true, "/blog/?q=1", "302 https://example.org/archive/?q=1"},
		{true, "/blog", "302 https://example.org/archive"},
		{true, "/blog?q=1", "302 https://example.org/archive?q=1"},
		// The rest comes from the raw $request_uri: an encoded line break
		// stays encoded and never splits the Location header.
		{false, "/blog/%0D%0ASet-Cookie:%20a=b?x=%0A", "302 https://example.org/archive/%0D%0ASet-Cookie:%20a=b"},
		{true, "/blog/%0D%0ASet-Cookie:%20a=b?x=%0A", "302 https://example.org/archive/%0D%0ASet-Cookie:%20a=b?x=%0A"},
	} {
		text := plain
		if c.keep {
			text = keep
		}
		if got := nginxFolderRedirect(t, text, "/blog", c.uri); got != c.want {
			t.Errorf("keep=%v %s: %s, want %s", c.keep, c.uri, got, c.want)
		}
	}
}

// The source of a folder redirect reaches the regex only with its dots
// escaped, so /a.b matches neither /aXb nor another folder.
func TestFolderRedirectSourceIsALiteralInTheRegex(t *testing.T) {
	ng := featureGenerator(t)
	data := featureVhost("php")
	data.Redirects = []SiteRedirect{{Match: RedirectMatchPrefix, Source: "/v1.0", Target: "/v2", Code: 301}}
	text, err := ng.Render(data)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, `^/v1\.0/(?<celikpanel_rest>[^?]*)) {`) {
		t.Fatalf("source not escaped:\n%s", text)
	}
	if got := nginxFolderRedirect(t, text, "/v1.0", "/v1.0/x?y=1"); got != "301 /v2/x" {
		t.Fatalf("got %s", got)
	}
}
