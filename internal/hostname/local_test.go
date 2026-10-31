package hostname

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalFQDN(t *testing.T) {
	// Hostinger's Ubuntu image: stale image lines first, the server's own last.
	hostinger := "# Your system has configured 'manage_etc_hosts' as True.\n" +
		"127.0.0.1       localhost\n::1     localhost       ip6-localhost   ip6-loopback\n\n" +
		"127.0.1.1       ubuntu-24.localhost     ubuntu-24\n" +
		"127.0.1.1       srv1811058.hstgr.cloud  srv1811058\n" +
		"127.0.1.1       boston.celikhost.com    boston\n"
	cases := []struct {
		name, kernel, hosts, want string
	}{
		{"hostinger short kernel name", "boston", hostinger, "boston.celikhost.com"},
		{"full kernel name is kept", "frankfurt.celikhost.com", hostinger, "frankfurt.celikhost.com"},
		{"no line lists the name", "paris", hostinger, "paris"},
		{"canonical is not an extension", "boston", "127.0.1.1 other.example.com boston\n", "boston"},
		{"first matching line wins", "boston", "127.0.1.1 boston\n127.0.1.1 boston.example.com boston\n", "boston"},
		{"trailing comment and case", "Boston", "127.0.1.1 Boston.Example.COM. boston # cloud\n", "boston.example.com"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "hosts")
			if err := os.WriteFile(path, []byte(c.hosts), 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := localFQDN(func() (string, error) { return c.kernel, nil }, path)
			if err != nil || got != c.want {
				t.Fatalf("localFQDN = %q, %v; want %q", got, err, c.want)
			}
		})
	}
	t.Run("missing hosts file keeps the kernel name", func(t *testing.T) {
		got, err := localFQDN(func() (string, error) { return "boston", nil }, filepath.Join(t.TempDir(), "none"))
		if err != nil || got != "boston" {
			t.Fatalf("localFQDN = %q, %v", got, err)
		}
	})
	t.Run("kernel error is returned", func(t *testing.T) {
		want := errors.New("no name")
		if _, err := localFQDN(func() (string, error) { return "", want }, "/etc/hosts"); !errors.Is(err, want) {
			t.Fatalf("err = %v", err)
		}
	})
}
