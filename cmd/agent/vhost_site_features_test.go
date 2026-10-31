package main

import (
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

func siteFeatureTestHash(t *testing.T) string {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("secret-secret"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	return "$2y$" + strings.TrimPrefix(string(hash), "$2a$")
}

func TestValidatedVhostDataCarriesFoldersAndRedirectsAndRendersThem(t *testing.T) {
	request := validApplyVhostRequest(t)
	request.ProtectedDirs = []transport.SiteProtectedDirRequest{{
		ID: 5, Path: "/private", Realm: "Staff only",
		Users: []transport.SiteProtectedUserRecord{{Username: "alice", PasswordHash: siteFeatureTestHash(t)}},
	}}
	request.Redirects = []transport.SiteRedirectRequest{
		{Match: "exact", Source: "/old", Target: "/new", Code: 307},
		{Match: "prefix", Source: "/blog", Target: "https://news.example.org", Code: 301, KeepQuery: true},
	}
	data, err := validatedVhostData(&request)
	if err != nil {
		t.Fatal(err)
	}
	if len(data.ProtectedDirs) != 1 || len(data.Redirects) != 2 {
		t.Fatalf("data = %#v", data)
	}
	nginxGenerator, err := services.NewNginxGenerator()
	if err != nil {
		t.Fatal(err)
	}
	text, err := nginxGenerator.Render(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"location ^~ /private/ {", `auth_basic "Staff only";`, "location = /old {", "return 307 /new;", "location ^~ /blog/ {"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in:\n%s", want, text)
		}
	}
	files := siteAuthFilesFor(&request)
	if len(files) != 1 || files[0].ID != 5 || len(files[0].Users) != 1 || files[0].Users[0].Username != "alice" {
		t.Fatalf("password files = %#v", files)
	}
}

func TestValidatedVhostDataRefusesUnsafeFolderUsersAndRedirects(t *testing.T) {
	good := siteFeatureTestHash(t)
	dir := func(path string, users ...transport.SiteProtectedUserRecord) transport.SiteProtectedDirRequest {
		return transport.SiteProtectedDirRequest{ID: 1, Path: path, Users: users}
	}
	user := func(name, hash string) transport.SiteProtectedUserRecord {
		return transport.SiteProtectedUserRecord{Username: name, PasswordHash: hash}
	}
	nginxGenerator, err := services.NewNginxGenerator()
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]func(*ApplyVhostRequest){
		"a folder without users": func(r *ApplyVhostRequest) { r.ProtectedDirs = []transport.SiteProtectedDirRequest{dir("/a")} },
		"a plain password instead of a hash": func(r *ApplyVhostRequest) {
			r.ProtectedDirs = []transport.SiteProtectedDirRequest{dir("/a", user("alice", "hunter2hunter2"))}
		},
		"a newline in the hash": func(r *ApplyVhostRequest) {
			r.ProtectedDirs = []transport.SiteProtectedDirRequest{dir("/a", user("alice", good[:20]+"\nbob:"+good[26:]))}
		},
		"a colon in the user name": func(r *ApplyVhostRequest) {
			r.ProtectedDirs = []transport.SiteProtectedDirRequest{dir("/a", user("al:ice", good))}
		},
		"too many users": func(r *ApplyVhostRequest) {
			users := make([]transport.SiteProtectedUserRecord, 0, services.MaxSiteProtectedUsers+1)
			for i := 0; i <= services.MaxSiteProtectedUsers; i++ {
				users = append(users, user("user"+strings.Repeat("a", i/26)+string(rune('a'+i%26)), good))
			}
			r.ProtectedDirs = []transport.SiteProtectedDirRequest{dir("/a", users...)}
		},
		"a folder with an injected path": func(r *ApplyVhostRequest) {
			r.ProtectedDirs = []transport.SiteProtectedDirRequest{dir("/a{ return 200; }", user("alice", good))}
		},
		"a redirect with an injected source": func(r *ApplyVhostRequest) {
			r.Redirects = []transport.SiteRedirectRequest{{Match: "exact", Source: "/a;\nreturn 200", Target: "/b", Code: 301}}
		},
		"a redirect with an injected target": func(r *ApplyVhostRequest) {
			r.Redirects = []transport.SiteRedirectRequest{{Match: "exact", Source: "/a", Target: "/b;\nreturn 200", Code: 301}}
		},
		"a redirect with a status nginx would not send as a redirect": func(r *ApplyVhostRequest) {
			r.Redirects = []transport.SiteRedirectRequest{{Match: "exact", Source: "/a", Target: "/b", Code: 200}}
		},
		"a folder on a site that cannot have one": func(r *ApplyVhostRequest) {
			r.ProjectType, r.ForwardTo, r.PHPSocket = "proxy", "http://127.0.0.1:3000", ""
			r.ProtectedDirs = []transport.SiteProtectedDirRequest{dir("/a", user("alice", good))}
		},
		"a redirect on a forwarding site": func(r *ApplyVhostRequest) {
			r.ProjectType, r.ForwardTo, r.PHPSocket = "forwarding", "https://destination.example/", ""
			r.Redirects = []transport.SiteRedirectRequest{{Match: "exact", Source: "/a", Target: "/b", Code: 301}}
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			request := validApplyVhostRequest(t)
			mutate(&request)
			data, err := validatedVhostData(&request)
			if err == nil {
				// The generator is the second line: it must refuse what the
				// first let through.
				_, err = nginxGenerator.Render(data)
			}
			if err == nil {
				t.Fatalf("%s was accepted", name)
			}
		})
	}
}
