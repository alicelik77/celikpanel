//go:build linux

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// alpha.113: Arch's nginx said "Server: nginx/1.30.6". nginx readiness adds
// `server_tokens off;` to an http block that has no server_tokens line of its
// own, keeps an owner's line (on or off), and never lets this hardening step
// fail nginx's readiness.

func writeNginxTokensTestConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "nginx.conf")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func readNginxTokensTestConfig(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func TestNginxReadinessServerTokensByLayout(t *testing.T) {
	cases := []struct {
		name      string
		conf      string
		wantAdded bool
	}{
		// Debian 13: includes present and server_tokens off already active.
		{"debian", "events {}\nhttp {\n\tserver_tokens off; # Recommended practice is to turn this off\n\tinclude /etc/nginx/conf.d/*.conf;\n\tinclude /etc/nginx/sites-enabled/*;\n}\n", false},
		// Ubuntu: the line is there, commented out.
		{"ubuntu", "events {}\nhttp {\n\t# server_tokens off;\n\tinclude /etc/nginx/conf.d/*.conf;\n\tinclude /etc/nginx/sites-enabled/*;\n}\n", true},
		// An owner who wants the version shown keeps it.
		{"owner-on", "events {}\nhttp {\n    server_tokens on;\n    include /etc/nginx/conf.d/*.conf;\n    include /etc/nginx/sites-enabled/*;\n}\n", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := writeNginxTokensTestConfig(t, c.conf)
			commands := 0
			changed, err := ensureNginxMainConfig(context.Background(), path,
				func(context.Context, string, ...string) ([]byte, error) { commands++; return nil, nil })
			if err != nil {
				t.Fatal(err)
			}
			got := readNginxTokensTestConfig(t, path)
			if c.wantAdded {
				if !changed || commands != 2 || strings.Count(got, "\n    server_tokens off;\n") != 1 ||
					!strings.Contains(got, "\t# server_tokens off;") {
					t.Fatalf("changed=%v commands=%d config:\n%s", changed, commands, got)
				}
				return
			}
			if changed || commands != 0 || got != c.conf {
				t.Fatalf("an existing server_tokens line was not kept: changed=%v commands=%d config:\n%s", changed, commands, got)
			}
		})
	}
}

func TestNginxReadinessServerTokensRefusalKeepsReadiness(t *testing.T) {
	original := "events {}\nhttp {\n\tinclude /etc/nginx/conf.d/*.conf;\n\tinclude /etc/nginx/sites-enabled/*;\n}\n"
	path := writeNginxTokensTestConfig(t, original)
	checks := 0
	changed, err := ensureNginxMainConfig(context.Background(), path,
		func(_ context.Context, name string, _ ...string) ([]byte, error) {
			if name == "nginx" {
				checks++
				if checks == 1 {
					return []byte(`nginx: [emerg] "server_tokens" directive is duplicate in /etc/nginx/conf.d/owner.conf:1`), errors.New("exit 1")
				}
			}
			return nil, nil
		})
	if err != nil || changed {
		t.Fatalf("a refused hardening step failed readiness: changed=%v err=%v", changed, err)
	}
	if got := readNginxTokensTestConfig(t, path); got != original {
		t.Fatalf("nginx.conf was not put back:\n%s", got)
	}
}
