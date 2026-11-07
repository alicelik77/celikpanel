package services

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"
	"text/template"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.128 (PLESK-GAP G3, G19): web server settings of a site.

const alpha127TemplatePath = "testdata/vhost-v0.1.0-alpha.127.conf.tmpl"

// A site that never saved a web server setting renders byte for byte what
// alpha.127 wrote, for every project type and certificate shape: the
// template change rewrites no existing vhost.
func TestWebSettingsTemplateChangeKeepsEveryExistingVhostByteForByte(t *testing.T) {
	source, err := os.ReadFile(alpha127TemplatePath)
	if err != nil {
		t.Fatal(err)
	}
	old, err := template.New("alpha127").Parse(string(source))
	if err != nil {
		t.Fatal(err)
	}
	ng, err := NewNginxGenerator()
	if err != nil {
		t.Fatal(err)
	}
	cert, key := "/etc/celikpanel/ssl/ornek.com/fullchain.pem", "/etc/celikpanel/ssl/ornek.com/privkey.pem"
	shapes := []VhostData{
		{ProjectType: "php", PHPSocket: "/run/php/php8.3-fpm-site2.sock"},
		{ProjectType: "static"},
		{ProjectType: "node", AppPort: 3000},
		{ProjectType: "proxy", ForwardTo: "http://127.0.0.1:8080"},
		{ProjectType: "forwarding", ForwardTo: "https://example.org", ForwardCode: 302},
		{ProjectType: "php", PHPSocket: "/run/php/php8.3-fpm-site2.sock", Suspended: true},
	}
	for _, shape := range shapes {
		for _, tls := range []int{0, 1, 2} {
			data := shape
			data.SiteID, data.Domain, data.DocumentRoot = 2, "ornek.com", "/var/www/celikpanel/subscriptions/1/sites/2/public_html"
			data.ACMEChallengeRoot = testACMEChallengeRoot
			data.SSLType = "none"
			if tls > 0 {
				data.SSLType, data.SSLCert, data.SSLKey = "custom", cert, key
				data.ForceHTTPS = tls == 2
			}
			now, err := ng.Render(data)
			if err != nil {
				t.Fatalf("%+v: %v", data, err)
			}
			prepared, err := prepareVhostData(data)
			if err != nil {
				t.Fatal(err)
			}
			prepared.HTTP2 = ng.features().HTTP2Form()
			var before bytes.Buffer
			if err := old.Execute(&before, prepared); err != nil {
				t.Fatal(err)
			}
			if now != before.String() {
				t.Fatalf("%s tls=%d: the render changed for a site without web settings", shape.ProjectType, tls)
			}
		}
	}
}

func webTestData(projectType string) VhostData {
	data := VhostData{
		SiteID: 2, Domain: "ornek.com", DocumentRoot: "/var/www/celikpanel/subscriptions/1/sites/2/public_html",
		ProjectType: projectType, SSLType: "custom", SSLCert: "/etc/celikpanel/ssl/ornek.com/fullchain.pem",
		SSLKey: "/etc/celikpanel/ssl/ornek.com/privkey.pem",
	}
	switch projectType {
	case "php":
		data.PHPSocket = "/run/php/php8.3-fpm-site2.sock"
	case "node":
		data.AppPort = 3000
	}
	return data
}

// The content-only include point is in the blocks that serve the content,
// never in the block that only redirects to HTTPS; index files replace the
// fixed line instead of adding a second one.
func TestWebSettingsIncludeIsContentOnly(t *testing.T) {
	line := SiteIncludeLine("ornek.com")
	for _, forced := range []bool{false, true} {
		data := webTestData("php")
		data.ForceHTTPS = forced
		data.SiteInclude = true
		data.IndexFiles = "index.html index.php"
		out := renderTestVhost(t, data)
		want := 2
		if forced {
			want = 1
		}
		if got := strings.Count(out, line); got != want {
			t.Fatalf("forced=%v: include lines %d, want %d\n%s", forced, got, want, out)
		}
		if strings.Contains(out, "index index.php index.html index.htm;") || strings.Count(out, "index index.html index.php;") != want {
			t.Fatalf("index line not replaced:\n%s", out)
		}
	}
	suspended := webTestData("php")
	suspended.SiteInclude, suspended.Suspended = true, true
	if strings.Contains(renderTestVhost(t, suspended), line) {
		t.Fatal("a suspended site's page reads the site's settings")
	}
}

// A Node.js site's public/ folder is served by nginx; the application folder
// never is, and every other address goes to the application.
func TestNodePublicFolderIsServedBeforeTheApplication(t *testing.T) {
	data := webTestData("node")
	data.NodePublicRoot = data.DocumentRoot + "/public"
	out := renderTestVhost(t, data)
	for _, want := range []string{
		"root " + data.DocumentRoot + "/public;",
		"try_files $uri @celikpanel_app;",
		"location @celikpanel_app {",
		"proxy_pass http://127.0.0.1:3000;",
		"location ~ /\\.(?!well-known).* {",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "root "+data.DocumentRoot+";") {
		t.Fatal("the application folder is served")
	}
	data.NodePublicRoot = data.DocumentRoot
	ng, _ := NewNginxGenerator()
	data.ACMEChallengeRoot = testACMEChallengeRoot
	if _, err := ng.Render(data); err == nil {
		t.Fatal("a public root other than public/ was accepted")
	}
}

func TestPrepareSiteWebSettingsRefusesUnsafeValues(t *testing.T) {
	bad := []struct {
		name string
		typ  string
		s    SiteWebSettings
	}{
		{"index path", "php", SiteWebSettings{IndexFiles: []string{"../x.php"}}},
		{"index on node", "node", SiteWebSettings{IndexFiles: []string{"index.html"}}},
		{"mime syntax", "php", SiteWebSettings{MIMETypes: []transport.SiteMIMEType{{Type: "text/html; x", Extensions: []string{"x"}}}}},
		{"mime twice", "php", SiteWebSettings{MIMETypes: []transport.SiteMIMEType{{Type: "a/b", Extensions: []string{"x"}}, {Type: "a/c", Extensions: []string{"x"}}}}},
		{"cache days", "php", SiteWebSettings{BrowserCacheDays: 3}},
		{"hsts header", "php", SiteWebSettings{Headers: []transport.SiteHeader{{Name: "Strict-Transport-Security", Value: "max-age=1"}}}},
		{"header newline", "php", SiteWebSettings{Headers: []transport.SiteHeader{{Name: "X-A", Value: "a\nb"}}}},
		{"header variable", "php", SiteWebSettings{Headers: []transport.SiteHeader{{Name: "X-A", Value: "$http_cookie"}}}},
		{"access empty", "php", SiteWebSettings{AccessMode: "allow"}},
		{"access everything", "php", SiteWebSettings{AccessMode: "deny", AccessList: []string{"0.0.0.0/0"}}},
		{"access junk", "php", SiteWebSettings{AccessMode: "allow", AccessList: []string{"all"}}},
		{"public on php", "php", SiteWebSettings{NodePublic: true}},
		{"forwarding", "forwarding", SiteWebSettings{BrowserCacheDays: 1}},
	}
	for _, c := range bad {
		if _, err := PrepareSiteWebSettings(c.typ, c.s); err == nil {
			t.Errorf("%s: accepted", c.name)
		}
	}
	got, err := PrepareSiteWebSettings("php", SiteWebSettings{
		IndexFiles: []string{"index.html", "index.html", "app.php"},
		MIMETypes:  []transport.SiteMIMEType{{Type: "Application/X-Example", Extensions: []string{".EXA"}}},
		Headers:    []transport.SiteHeader{{Name: "X-Frame-Options", Value: `SAMEORIGIN "x"`}},
		AccessMode: "allow", AccessList: []string{"203.0.113.7", "2001:db8::/32", "198.51.100.9/24"},
		BrowserCacheDays: 7,
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got.IndexFiles, " ") != "index.html app.php" || got.MIMETypes[0].Type != "application/x-example" ||
		got.MIMETypes[0].Extensions[0] != "exa" || got.AccessList[2] != "198.51.100.0/24" {
		t.Fatalf("normalized: %+v", got)
	}
	body := RenderSiteSettingsPart("ornek.com", got, true)
	for _, want := range []string{
		"include mime.types;\ntypes {\n    application/x-example exa;\n}\n",
		"expires $celikpanel_expires_7d;\n",
		`add_header X-Frame-Options "SAMEORIGIN \"x\"" always;`,
		"allow 203.0.113.7;\nallow 2001:db8::/32;\nallow 198.51.100.0/24;\ndeny all;\n",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(RenderSiteSettingsPart("ornek.com", got, false), "expires ") {
		t.Fatal("the fallback body names the owner's maps")
	}
	// A type change never makes a render fail on a field that no longer applies.
	if s, err := ApplicableSiteWebSettings("node", got); err != nil || len(s.IndexFiles) != 0 {
		t.Fatalf("applicable: %+v %v", s, err)
	}
	if s, err := ApplicableSiteWebSettings("forwarding", got); err != nil || !siteWebSettingsEmpty(s) {
		t.Fatalf("forwarding: %+v %v", s, err)
	}
}

func TestBrowserCachingNeverCachesPHPPages(t *testing.T) {
	conf := RenderWebHTTPConf()
	if strings.Contains(conf, "text/html") || !strings.Contains(conf, "map $sent_http_content_type $celikpanel_expires_30d {\n    default off;") {
		t.Fatalf("http conf:\n%s", conf)
	}
}

func TestSiteDirectivesForbiddenListAndPathRule(t *testing.T) {
	home := "/var/www/celikpanel/subscriptions/1/sites/2"
	refused := map[string]string{
		"include /etc/passwd;":                           "forbidden",
		"location /x { proxy_pass http://10.0.0.1; }":    "forbidden",
		"location /x { fastcgi_pass unix:/run/x.sock; }": "forbidden",
		"root /etc;":                                                 "forbidden",
		"location /a { alias /etc/; }":                               "forbidden",
		"ssl_certificate /x;":                                        "forbidden",
		"access_log /tmp/x.log;":                                     "forbidden",
		"client_body_temp_path /tmp;":                                "forbidden",
		"set_real_ip_from 0.0.0.0/0;":                                "forbidden",
		"location /d { dav_methods PUT; }":                           "forbidden",
		"location /s { proxy_store on; }":                            "forbidden",
		"js_content x.y;":                                            "forbidden",
		"content_by_lua_block { }":                                   "forbidden",
		"index x.php;":                                               "field",
		"expires 1d;":                                                "field",
		"add_header Strict-Transport-Security max-age=0;":            "field",
		"location /a { add_header X-A b; }":                          "header",
		"location /p { auth_basic_user_file /etc/shadow; }":          "path",
		"location /p { auth_basic_user_file " + home + "/../../x; }": "path",
		"location /x { return 200;":                                  "syntax",
		"return 200 \"unterminated;":                                 "syntax",
		"}":                                                          "syntax",
	}
	for text, kind := range refused {
		err := ValidateSiteDirectives(text, false, home)
		var refusal *SiteDirectiveRefusal
		if !errors.As(err, &refusal) || refusal.Kind != kind {
			t.Errorf("%q: %v, want %s", text, err, kind)
		}
	}
	allowed := []string{
		"",
		"# just a comment\n",
		"location = /old { return 301 /new; }",
		"location ~* \\.(?:ico|css)$ { expires 30d; access_log off; }",
		"rewrite ^/blog/(.*)$ /news/$1 permanent;",
		"add_header X-Robots-Tag \"noindex\" always;",
		"location /p { auth_basic \"x\"; auth_basic_user_file " + home + "/.htpasswd; }",
		"if ($http_user_agent ~* bot) { return 403; }",
		"location /x { set ${a} 1; return 200 \"${a};\"; }",
	}
	for _, text := range allowed {
		if err := ValidateSiteDirectives(text, false, home); err != nil {
			t.Errorf("%q: %v", text, err)
		}
	}
	// The administrator's text: only syntax and field ownership.
	if err := ValidateSiteDirectives("location /x { proxy_pass http://10.0.0.1; }\naccess_log /var/log/x.log;", true, ""); err != nil {
		t.Fatalf("administrator: %v", err)
	}
	if err := ValidateSiteDirectives(strings.Repeat("#", MaxSiteDirectivesBytes+1), true, ""); err == nil {
		t.Fatal("an oversized text was accepted")
	}
}

// The site's files are written with the vhost, kept when the owner changed
// them, removed when the text is cleared, and put back when nginx refuses.
func TestSitePartsFollowTheVhostAndAreKeptWhenEdited(t *testing.T) {
	ng, reloads := trafficTestGenerator(t)
	domain := "web.example"
	body := "server {\n    include " + PanelManagedDir(domain) + "/*.conf;\n    " + SiteIncludeLine(domain) + "\n}\n"
	settings := SiteWebSettings{BrowserCacheDays: 7, Headers: []transport.SiteHeader{{Name: "X-A", Value: "b"}}}
	item := ManagedVhostItem{Domain: domain, Body: body, Trigger: transport.SiteFileTriggerCreate,
		SiteParts: SitePartsFor(domain, settings, "location = /x { return 204; }")}
	results, err := ng.ApplyManagedVhosts([]ManagedVhostItem{item})
	if err != nil {
		t.Fatal(err)
	}
	parts := results[0].SiteParts
	if parts[SiteSettingsPartName] != transport.SiteFileChallengeWritten || parts[SiteDirectivesPartName] != transport.SiteFileChallengeWritten {
		t.Fatalf("parts: %+v", parts)
	}
	if state, text := SitePartState(domain, SiteSettingsPartName); state != "unchanged" || !strings.Contains(text, "expires $celikpanel_expires_7d;") {
		t.Fatalf("settings: %s %q", state, text)
	}
	if _, err := os.Stat(WebHTTPConfPath()); err != nil {
		t.Fatalf("http conf: %v", err)
	}
	before := *reloads

	// The same render again writes nothing and reloads nothing.
	item.Trigger = transport.SiteFileTriggerChange
	results, _ = ng.ApplyManagedVhosts([]ManagedVhostItem{item})
	if results[0].SiteParts[SiteSettingsPartName] != transport.SiteFileChallengeUnchanged || *reloads != before {
		t.Fatalf("second render: %+v reloads %d", results[0].SiteParts, *reloads-before)
	}

	// nginx refuses a new text: the previous file is back.
	ng.validateNginx = func() error { return errors.New("nginx: [emerg] unknown directive \"bogus\"") }
	item.SiteParts = SitePartsFor(domain, settings, "bogus;")
	if _, err := ng.ApplyManagedVhosts([]ManagedVhostItem{item}); err == nil {
		t.Fatal("a refused text was reported applied")
	}
	if _, text := SitePartState(domain, SiteDirectivesPartName); !strings.Contains(text, "return 204;") {
		t.Fatalf("the previous directives are not back:\n%s", text)
	}
	ng.validateNginx = func() error { return nil }

	// The owner edits the directives file: it is kept, never overwritten.
	path := SitePartPath(domain, SiteDirectivesPartName)
	if err := os.WriteFile(path, []byte("# mine\nlocation = /y { return 204; }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	item.SiteParts = SitePartsFor(domain, settings, "location = /z { return 204; }")
	results, _ = ng.ApplyManagedVhosts([]ManagedVhostItem{item})
	if results[0].SiteParts[SiteDirectivesPartName] != transport.SiteFileChallengeKept {
		t.Fatalf("owner's file: %+v", results[0].SiteParts)
	}
	if content, _ := os.ReadFile(path); !strings.Contains(string(content), "/y") {
		t.Fatal("the owner's file was overwritten")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}

	// Clearing the text removes CelikPanel's file.
	item.SiteParts = SitePartsFor(domain, settings, "location = /z { return 204; }")
	_, _ = ng.ApplyManagedVhosts([]ManagedVhostItem{item})
	item.SiteParts = SitePartsFor(domain, settings, "")
	results, _ = ng.ApplyManagedVhosts([]ManagedVhostItem{item})
	if results[0].SiteParts[SiteDirectivesPartName] != transport.SitePartRemoved {
		t.Fatalf("cleared: %+v", results[0].SiteParts)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("the cleared file is still there")
	}

	// A kept vhost without the include line reads none of the files.
	vhost := SiteVhostPath(domain)
	if err := os.WriteFile(vhost, []byte("server { listen 80; }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	results, _ = ng.ApplyManagedVhosts([]ManagedVhostItem{item})
	if results[0].SiteParts[SiteSettingsPartName] != transport.SitePartNotRead {
		t.Fatalf("kept vhost: %+v", results[0].SiteParts)
	}
}

func TestACMEChallengeAllowsEverybodyOnlyWithAnAccessRule(t *testing.T) {
	plain := RenderACMEChallengeFile("ornek.com", testACMEChallengeRoot)
	if plain != RenderACMEChallengeFileFor("ornek.com", testACMEChallengeRoot, false) || strings.Contains(plain, "allow all;") {
		t.Fatal("the challenge file changed for a site without an access rule")
	}
	if !strings.Contains(RenderACMEChallengeFileFor("ornek.com", testACMEChallengeRoot, true), "    allow all;\n") {
		t.Fatal("the challenge location does not allow everybody")
	}
}
