package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/alicelik/celikpanel/internal/transport"
)

func featureGenerator(t *testing.T) *NginxGenerator {
	t.Helper()
	ng, err := NewNginxGenerator()
	if err != nil {
		t.Fatal(err)
	}
	ng.nginxFeatures = func() NginxFeatures { return NginxFeatures{} }
	return ng
}

func featureVhost(projectType string) VhostData {
	data := VhostData{
		SiteID: 7, Domain: "example.test", DocumentRoot: "/srv/example.test/public_html",
		ACMEChallengeRoot: "/var/lib/celikpanel/acme/example.test",
		ServerNames:       []string{"example.test", "www.example.test"},
		SSLType:           "none", ProjectType: projectType,
	}
	switch projectType {
	case "php":
		data.PHPSocket = "/run/php/php8.3-fpm-site7.sock"
	case "proxy":
		data.ForwardTo = "http://127.0.0.1:3000"
	case "forwarding":
		data.ForwardTo = "https://destination.example/"
	}
	return data
}

func TestSiteWithoutFeaturesRendersNothingOfThem(t *testing.T) {
	ng := featureGenerator(t)
	for _, projectType := range []string{"php", "static", "proxy", "forwarding"} {
		text, err := ng.Render(featureVhost(projectType))
		if err != nil {
			t.Fatal(err)
		}
		for _, banned := range []string{"auth_basic", "celikpanel_rest", "Redirect /", "Password-protected"} {
			if strings.Contains(text, banned) {
				t.Fatalf("%s vhost without features contains %q", projectType, banned)
			}
		}
	}
}

func TestProtectedFolderIsRenderedWithItsOwnPHPHandler(t *testing.T) {
	ng := featureGenerator(t)
	data := featureVhost("php")
	data.ProtectedDirs = []SiteProtectedDir{{ID: 3, Path: "/private/", Realm: "Staff only"}}
	text, err := ng.Render(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"location = /private {\n        return 301 /private/$is_args$args;",
		"location ^~ /private/ {",
		`auth_basic "Staff only";`,
		"auth_basic_user_file " + SiteAuthFilePath("example.test", 3) + ";",
		"location ~ \\.php$ {",
		"fastcgi_pass unix:/run/php/php8.3-fpm-site7.sock;",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in:\n%s", want, text)
		}
	}
	// The protected block comes with its own dotfile rule; the PHP handler is
	// inside the protected location, not only at the server level.
	protected := text[strings.Index(text, "location ^~ /private/ {"):]
	protected = protected[:strings.Index(protected, "\n    root ")]
	if !strings.Contains(protected, "fastcgi_pass") || !strings.Contains(protected, "deny all") {
		t.Fatalf("protected folder lacks its PHP handler or dotfile rule:\n%s", protected)
	}
	if strings.Count(text, "auth_basic_user_file") != 1 {
		t.Fatalf("one password file expected:\n%s", text)
	}
}

func TestProtectedFolderOnStaticSiteDeniesPHPSourceInside(t *testing.T) {
	ng := featureGenerator(t)
	data := featureVhost("static")
	data.ProtectedDirs = []SiteProtectedDir{{ID: 1, Path: "/docs"}}
	text, err := ng.Render(data)
	if err != nil {
		t.Fatal(err)
	}
	protected := text[strings.Index(text, "location ^~ /docs/ {"):]
	if !strings.Contains(protected, `auth_basic "Restricted area";`) ||
		!strings.Contains(protected, `location ~ \.(php|phtml|phps|php[0-9])$ {`) {
		t.Fatalf("static protected folder:\n%s", protected)
	}
}

func TestProtectedFoldersAreOnlyInTheServingBlocks(t *testing.T) {
	ng := featureGenerator(t)
	data := featureVhost("php")
	data.SSLType, data.SSLCert, data.SSLKey = "custom", "/etc/ssl/c/fullchain.pem", "/etc/ssl/c/privkey.pem"
	data.ForceHTTPS = true
	data.ProtectedDirs = []SiteProtectedDir{{ID: 1, Path: "/private"}}
	data.Redirects = []SiteRedirect{{Match: RedirectMatchExact, Source: "/old", Target: "/new", Code: 301}}
	text, err := ng.Render(data)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(text, "auth_basic ") != 1 || strings.Count(text, "location = /old") != 1 {
		t.Fatalf("a force-HTTPS site serves the folders in its HTTPS block only:\n%s", text)
	}
	httpBlock := text[:strings.Index(text, "# HTTPS server")]
	if strings.Contains(httpBlock, "auth_basic") || strings.Contains(httpBlock, "/old") {
		t.Fatalf("HTTP block of a force-HTTPS site carries a folder or redirect:\n%s", httpBlock)
	}
}

func TestRedirectsAreRenderedAsLocations(t *testing.T) {
	ng := featureGenerator(t)
	data := featureVhost("php")
	data.Redirects = []SiteRedirect{
		{Match: RedirectMatchExact, Source: "/old.html", Target: "https://Example.ORG/new", Code: 308},
		{Match: RedirectMatchExact, Source: "/search", Target: "/find", Code: 302, KeepQuery: true},
		{Match: RedirectMatchPrefix, Source: "/blog", Target: "https://news.example.org/archive/", Code: 301, KeepQuery: true},
	}
	text, err := ng.Render(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"location = /old.html {\n        return 308 https://example.org/new;",
		"location = /search {\n        return 302 /find$is_args$args;",
		"location = /blog {\n        return 301 https://news.example.org/archive/$is_args$args;",
		"location ^~ /blog/ {",
		`if ($request_uri ~ ^/blog/(?<celikpanel_rest>.*)$) {`,
		"return 301 https://news.example.org/archive/$celikpanel_rest;",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in:\n%s", want, text)
		}
	}
	// The same input always renders the same text.
	again, _ := ng.Render(data)
	if again != text {
		t.Fatal("two renders of the same input differ")
	}
}

func TestRedirectsAreRenderedForProxySitesAndRefusedForForwardingOnes(t *testing.T) {
	ng := featureGenerator(t)
	redirect := []SiteRedirect{{Match: RedirectMatchExact, Source: "/old", Target: "/new", Code: 301}}
	proxy := featureVhost("proxy")
	proxy.Redirects = redirect
	if text, err := ng.Render(proxy); err != nil || !strings.Contains(text, "location = /old {") {
		t.Fatalf("proxy site: %v\n%s", err, text)
	}
	forwarding := featureVhost("forwarding")
	forwarding.Redirects = redirect
	if _, err := ng.Render(forwarding); err == nil {
		t.Fatal("a forwarding site accepted redirects")
	}
	proxy.ProtectedDirs = []SiteProtectedDir{{ID: 1, Path: "/private"}}
	if _, err := ng.Render(proxy); err == nil {
		t.Fatal("a proxy site accepted a protected folder")
	}
}

// Every value that reaches the template is checked, not escaped: none of these
// can become another directive.
func TestSiteFeatureInputCannotInjectNginxSyntax(t *testing.T) {
	ng := featureGenerator(t)
	evil := []string{
		"/a\nlocation / { return 200; }",
		"/a\r\nreturn 200;",
		"/a;return 200",
		"/a{",
		"/a}",
		"/a b",
		"/a\tb",
		"/a\"b",
		"/a'b",
		"/a$uri",
		"/a#b",
		"/a\\b",
		"/a`id`",
		"/../etc",
		"/a/../b",
		"/a/./b",
		"//a",
		"/a//b",
		"/.hidden",
		"/a/.git",
		"/.well-known/acme-challenge",
		"/%2e%2e/a",
		"/ü",
		"",
		"a",
	}
	for _, source := range evil {
		for _, match := range []string{RedirectMatchExact, RedirectMatchPrefix} {
			data := featureVhost("php")
			data.Redirects = []SiteRedirect{{Match: match, Source: source, Target: "/new", Code: 301}}
			if text, err := ng.Render(data); err == nil {
				t.Errorf("redirect source %q (%s) was accepted:\n%s", source, match, text)
			}
		}
		data := featureVhost("php")
		data.ProtectedDirs = []SiteProtectedDir{{ID: 1, Path: source}}
		if text, err := ng.Render(data); err == nil {
			t.Errorf("protected path %q was accepted:\n%s", source, text)
		}
	}
	evilTargets := []string{
		"https://example.org/\nreturn 200",
		"https://example.org/a;b",
		"https://example.org/{a}",
		"https://example.org/a b",
		"https://example.org/\"a",
		"https://example.org/$host",
		"https://example.org/a#frag",
		"https://user:pw@example.org/",
		"https://example.org\\@evil.test/",
		"//evil.test/x",
		"javascript:alert(1)",
		"ftp://example.org/",
		"https:///nohost",
		"https://exa mple.org/",
		"https://example.org:99999/",
		"https://-bad-.example.org/",
		"relative/path",
		"",
	}
	for _, target := range evilTargets {
		data := featureVhost("php")
		data.Redirects = []SiteRedirect{{Match: RedirectMatchExact, Source: "/old", Target: target, Code: 301}}
		if text, err := ng.Render(data); err == nil {
			t.Errorf("redirect target %q was accepted:\n%s", target, text)
		}
	}
	for _, realm := range []string{"a\"b", "a\nb", "a;b", "a{b", "a$b", strings.Repeat("a", 65), "-a", "ü"} {
		data := featureVhost("php")
		data.ProtectedDirs = []SiteProtectedDir{{ID: 1, Path: "/private", Realm: realm}}
		if text, err := ng.Render(data); err == nil {
			t.Errorf("sign-in title %q was accepted:\n%s", realm, text)
		}
	}
	for _, code := range []int{0, 200, 300, 303, 304, 400, 500, 30101} {
		data := featureVhost("php")
		data.Redirects = []SiteRedirect{{Match: RedirectMatchExact, Source: "/old", Target: "/new", Code: code}}
		if _, err := ng.Render(data); err == nil {
			t.Errorf("status code %d was accepted", code)
		}
	}
	data := featureVhost("php")
	data.Redirects = []SiteRedirect{{Match: "regex", Source: "/old", Target: "/new", Code: 301}}
	if _, err := ng.Render(data); err == nil {
		t.Error("an unknown match type was accepted")
	}
}

func TestSiteFeatureConflictsAreRefused(t *testing.T) {
	ng := featureGenerator(t)
	render := func(mutate func(*VhostData)) error {
		data := featureVhost("php")
		mutate(&data)
		_, err := ng.Render(data)
		return err
	}
	cases := map[string]func(*VhostData){
		"the same redirect twice": func(d *VhostData) {
			r := SiteRedirect{Match: RedirectMatchExact, Source: "/a", Target: "/b", Code: 301}
			d.Redirects = []SiteRedirect{r, r}
		},
		"exact and folder on one path": func(d *VhostData) {
			d.Redirects = []SiteRedirect{
				{Match: RedirectMatchExact, Source: "/a", Target: "/b", Code: 301},
				{Match: RedirectMatchPrefix, Source: "/a", Target: "/c", Code: 301},
			}
		},
		"redirect on a protected path": func(d *VhostData) {
			d.ProtectedDirs = []SiteProtectedDir{{ID: 1, Path: "/a"}}
			d.Redirects = []SiteRedirect{{Match: RedirectMatchExact, Source: "/a", Target: "/b", Code: 301}}
		},
		"the same protected folder twice": func(d *VhostData) {
			d.ProtectedDirs = []SiteProtectedDir{{ID: 1, Path: "/a"}, {ID: 2, Path: "/a/"}}
		},
		"a loop on the same path": func(d *VhostData) {
			d.Redirects = []SiteRedirect{{Match: RedirectMatchExact, Source: "/a", Target: "/a", Code: 301}}
		},
		"a loop through the site's own name": func(d *VhostData) {
			d.Redirects = []SiteRedirect{{Match: RedirectMatchExact, Source: "/a", Target: "https://www.example.test/a", Code: 301}}
		},
		"a folder redirected into itself": func(d *VhostData) {
			d.Redirects = []SiteRedirect{{Match: RedirectMatchPrefix, Source: "/a", Target: "/a/new", Code: 301}}
		},
		"a folder redirected to the root": func(d *VhostData) {
			d.Redirects = []SiteRedirect{{Match: RedirectMatchPrefix, Source: "/a", Target: "/", Code: 301}}
		},
		"a folder redirect with a query in its target": func(d *VhostData) {
			d.Redirects = []SiteRedirect{{Match: RedirectMatchPrefix, Source: "/a", Target: "https://x.example/b?z=1", Code: 301}}
		},
		"keeping the query and giving one": func(d *VhostData) {
			d.Redirects = []SiteRedirect{{Match: RedirectMatchExact, Source: "/a", Target: "/b?z=1", Code: 301, KeepQuery: true}}
		},
		"a protected folder without identity": func(d *VhostData) {
			d.ProtectedDirs = []SiteProtectedDir{{Path: "/a"}}
		},
		"the folder / alone": func(d *VhostData) {
			d.ProtectedDirs = []SiteProtectedDir{{ID: 1, Path: "/"}}
		},
		"too many redirects": func(d *VhostData) {
			for i := 0; i <= MaxSiteRedirects; i++ {
				d.Redirects = append(d.Redirects, SiteRedirect{Match: RedirectMatchExact, Source: "/r" + strings.Repeat("a", i%5) + string(rune('a'+i%26)) + strings.Repeat("b", i/26), Target: "/x", Code: 301})
			}
		},
	}
	for name, mutate := range cases {
		if err := render(mutate); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
	// Redirecting the root address exactly is allowed; a legitimate folder
	// redirect to another host is too.
	if err := render(func(d *VhostData) {
		d.Redirects = []SiteRedirect{
			{Match: RedirectMatchExact, Source: "/", Target: "/home", Code: 302},
			{Match: RedirectMatchPrefix, Source: "/blog", Target: "https://blog.example.org", Code: 301},
		}
	}); err != nil {
		t.Fatalf("legitimate redirects refused: %v", err)
	}
}

// ---------------------------------------------------------------------------
// password files

func authTestHash(t *testing.T, password string) string {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	return "$2y$" + strings.TrimPrefix(string(hash), "$2a$")
}

func withNginxDevDir(t *testing.T) string {
	t.Helper()
	saved := nginxDir
	nginxDir = t.TempDir()
	t.Cleanup(func() { nginxDir = saved })
	return nginxDir
}

func TestSiteAuthFileIsWrittenPrivatelyAndKeptInStep(t *testing.T) {
	withNginxDevDir(t)
	file := SiteAuthFile{ID: 4, Path: "/private", Users: []SiteAuthUser{
		{Username: "zoe", Hash: authTestHash(t, "zoe-password")},
		{Username: "adam", Hash: authTestHash(t, "adam-password")},
	}}
	change, err := PrepareSiteAuthFiles("example.test", []SiteAuthFile{file})
	if err != nil {
		t.Fatal(err)
	}
	path := SiteAuthFilePath("example.test", 4)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("password file mode = %v, want 0640", info.Mode().Perm())
	}
	if dir, _ := os.Stat(filepath.Dir(path)); dir.Mode().Perm() != 0o750 {
		t.Fatalf("password directory mode = %v, want 0750", dir.Mode().Perm())
	}
	content, _ := os.ReadFile(path)
	text := string(content)
	if !strings.HasPrefix(text, managedRenderHeaderPrefix) {
		t.Fatalf("the file has no render header:\n%s", text)
	}
	if strings.Index(text, "adam:") > strings.Index(text, "zoe:") || strings.Contains(text, "adam-password") {
		t.Fatalf("users are not sorted, or a password is in the file:\n%s", text)
	}
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "adam:") {
			hash := strings.TrimPrefix(line, "adam:")
			if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte("adam-password")); err != nil {
				t.Fatalf("the stored hash does not match: %v", err)
			}
		}
	}
	change.Commit()

	// The same input writes nothing more.
	before, _ := os.Stat(path)
	if again, err := PrepareSiteAuthFiles("example.test", []SiteAuthFile{file}); err != nil {
		t.Fatal(err)
	} else if len(again.touched) != 0 {
		t.Fatal("an unchanged password file was rewritten")
	}
	after, _ := os.Stat(path)
	if !after.ModTime().Equal(before.ModTime()) {
		t.Fatal("an unchanged password file was replaced")
	}
}

func TestSiteAuthChangeRollsBackAndCommitRemovesWhatIsNotAskedFor(t *testing.T) {
	withNginxDevDir(t)
	user := func(name string) []SiteAuthUser {
		return []SiteAuthUser{{Username: name, Hash: authTestHash(t, name+"-secret")}}
	}
	first, err := PrepareSiteAuthFiles("example.test", []SiteAuthFile{
		{ID: 1, Path: "/a", Users: user("one")}, {ID: 2, Path: "/b", Users: user("two")},
	})
	if err != nil {
		t.Fatal(err)
	}
	first.Commit()
	original, _ := os.ReadFile(SiteAuthFilePath("example.test", 1))

	// A change that is not applied puts the old bytes back and removes the new file.
	change, err := PrepareSiteAuthFiles("example.test", []SiteAuthFile{
		{ID: 1, Path: "/a", Users: user("changed")}, {ID: 3, Path: "/c", Users: user("three")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if now, _ := os.ReadFile(SiteAuthFilePath("example.test", 1)); string(now) == string(original) {
		t.Fatal("the change did not write the new password file")
	}
	if err := change.Rollback(); err != nil {
		t.Fatal(err)
	}
	if now, _ := os.ReadFile(SiteAuthFilePath("example.test", 1)); string(now) != string(original) {
		t.Fatal("rollback did not put the old password file back")
	}
	if _, err := os.Stat(SiteAuthFilePath("example.test", 3)); !os.IsNotExist(err) {
		t.Fatalf("rollback left the new file behind: %v", err)
	}

	// A change that is applied drops the file nothing asks for any more.
	applied, err := PrepareSiteAuthFiles("example.test", []SiteAuthFile{{ID: 1, Path: "/a", Users: user("one")}})
	if err != nil {
		t.Fatal(err)
	}
	applied.Commit()
	if _, err := os.Stat(SiteAuthFilePath("example.test", 2)); !os.IsNotExist(err) {
		t.Fatalf("a password file nothing asks for was kept: %v", err)
	}
	// No folder left: the directory goes too.
	last, err := PrepareSiteAuthFiles("example.test", nil)
	if err != nil {
		t.Fatal(err)
	}
	last.Commit()
	if _, err := os.Stat(SiteAuthDir("example.test")); !os.IsNotExist(err) {
		t.Fatalf("an empty password directory was kept: %v", err)
	}
}

func TestSiteAuthFileTheOwnerChangedIsKeptAndRefusesTheChange(t *testing.T) {
	withNginxDevDir(t)
	user := []SiteAuthUser{{Username: "one", Hash: authTestHash(t, "one-secret")}}
	change, err := PrepareSiteAuthFiles("example.test", []SiteAuthFile{{ID: 1, Path: "/a", Users: user}})
	if err != nil {
		t.Fatal(err)
	}
	change.Commit()
	path := SiteAuthFilePath("example.test", 1)
	original, _ := os.ReadFile(path)
	edited := append(append([]byte(nil), original...), []byte("owner:$apr1$abc$def\n")...)
	if err := os.WriteFile(path, edited, 0o640); err != nil {
		t.Fatal(err)
	}
	_, err = PrepareSiteAuthFiles("example.test", []SiteAuthFile{
		{ID: 1, Path: "/a", Users: append(user, SiteAuthUser{Username: "two", Hash: authTestHash(t, "two-secret")})},
		{ID: 2, Path: "/b", Users: user},
	})
	kept, ok := err.(*SiteAuthFileKeptError)
	if !ok || kept.Path != path || kept.State != transport.SiteFileOwnerEdited {
		t.Fatalf("error = %#v, want the kept owner-edited file", err)
	}
	if now, _ := os.ReadFile(path); string(now) != string(edited) {
		t.Fatal("the owner's password file was overwritten")
	}
	// Nothing else of the refused change was written.
	if _, err := os.Stat(SiteAuthFilePath("example.test", 2)); !os.IsNotExist(err) {
		t.Fatalf("a file of the refused change was written: %v", err)
	}
	// Commit never removes a file the owner changed.
	RemoveSiteAuthFiles("example.test")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the owner's file was removed: %v", err)
	}
}

func TestSiteAuthFileRefusesAnythingButABcryptHash(t *testing.T) {
	good := authTestHash(t, "secret-secret")
	for name, user := range map[string]SiteAuthUser{
		"plain password":      {Username: "a", Hash: "secret-secret"},
		"apr1":                {Username: "a", Hash: "$apr1$salt$abcdefghijklmnopqrstuv"},
		"newline in the hash": {Username: "a", Hash: good[:30] + "\n" + good[31:]},
		"colon in the hash":   {Username: "a", Hash: good[:30] + ":" + good[31:]},
		"short hash":          {Username: "a", Hash: good[:50]},
		"colon in the user":   {Username: "a:b", Hash: good},
		"newline in the user": {Username: "a\nb:" + good, Hash: good},
		"space in the user":   {Username: "a b", Hash: good},
		"empty user":          {Username: "", Hash: good},
		"comment-like user":   {Username: "#a", Hash: good},
		"unicode user":        {Username: "ü", Hash: good},
		"overlong user":       {Username: strings.Repeat("a", 65), Hash: good},
	} {
		if _, err := RenderSiteAuthFile("example.test", SiteAuthFile{ID: 1, Path: "/a", Users: []SiteAuthUser{user}}); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
	if _, err := RenderSiteAuthFile("example.test", SiteAuthFile{ID: 1, Path: "/a", Users: []SiteAuthUser{
		{Username: "a", Hash: good}, {Username: "a", Hash: good},
	}}); err == nil {
		t.Error("a user listed twice was accepted")
	}
}
