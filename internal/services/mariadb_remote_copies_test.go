package services

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// alpha.129 (docs/DATABASE-REMOTE-ACCESS.md): the driver's user changes
// reach the user's remote copies (an address or IPv4 network host) and
// never an account in any other form (a wildcard or a name).
func TestMariaDBDriverFollowsRemoteCopies(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake mysql client is a shell script")
	}
	dir := t.TempDir()
	log := filepath.Join(dir, "statements.log")
	script := "#!/bin/sh\ninput=$(cat)\nprintf '%s\n----\n' \"$input\" >> " + log + "\n" +
		"case \"$input\" in *'SELECT Host FROM mysql.user'*) printf 'Host\n203.0.113.7\n198.51.100.0/255.255.255.0\n%%\nexample.com\n' ;; esac\n"
	if err := os.WriteFile(filepath.Join(dir, "mysql"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	driver := NewMariaDBDriver(DriverConfig{Host: "localhost", Port: 3306, Username: "root", Password: "x"})

	for _, step := range []func() error{
		func() error { return driver.ChangePassword("shop_u", "New-pass-12345") },
		func() error { return driver.RevokePrivileges("shop", "shop_u") },
		func() error { return driver.SetUserConnectionLimit("shop_u", 15) },
		func() error { return driver.DeleteUser("shop_u") },
	} {
		if err := step(); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{
		"ALTER USER 'shop_u'@'203.0.113.7' IDENTIFIED BY 'New-pass-12345';",
		"REVOKE ALL PRIVILEGES, GRANT OPTION FROM 'shop_u'@'198.51.100.0/255.255.255.0';",
		"ALTER USER 'shop_u'@'203.0.113.7' WITH MAX_USER_CONNECTIONS 15;",
		"DROP USER IF EXISTS 'shop_u'@'198.51.100.0/255.255.255.0';",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in\n%s", want, text)
		}
	}
	if strings.Contains(text, "'shop_u'@'%'") || strings.Contains(text, "'shop_u'@'example.com'") {
		t.Fatalf("an account CelikPanel does not write was touched:\n%s", text)
	}
}
