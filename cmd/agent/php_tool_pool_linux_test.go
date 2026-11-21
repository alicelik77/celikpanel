//go:build linux

package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// S4 R1: webmail's tree under its own account. The private parts (the
// configuration with des_key, the SQLite database with sessions, temp with
// attachments, logs) are closed to the account before the published code is
// opened for reading, so nothing private is readable to others on the way.
func TestRoundcubeOwnAccountClosesPrivatePartsBeforeOpeningTheCode(t *testing.T) {
	base := filepath.Join(t.TempDir(), "celikpanel-webmail")
	for _, dir := range []string{"config", "db", "temp", "logs", "public_html", "program", "skins"} {
		if err := os.MkdirAll(filepath.Join(base, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	var commands []string
	run := func(_ context.Context, name string, args ...string) ([]byte, error) {
		commands = append(commands, name+" "+strings.Join(args, " "))
		return nil, nil
	}
	saved := setRoundcubeTreeRoot
	var root string
	setRoundcubeTreeRoot = func(path string, mode os.FileMode, uid, gid int) error {
		root = path + " " + mode.String()
		return nil
	}
	t.Cleanup(func() { setRoundcubeTreeRoot = saved })
	if err := applyRoundcubeOwnAccountPermissions(context.Background(), base, 990, 991, run); err != nil {
		t.Fatal(err)
	}
	firstOpen := -1
	lastClose := -1
	for i, command := range commands {
		if strings.Contains(command, "go=rX") {
			if firstOpen < 0 {
				firstOpen = i
			}
		} else if strings.HasPrefix(command, "chmod") {
			lastClose = i
		}
	}
	if firstOpen < 0 || lastClose < 0 || lastClose > firstOpen {
		t.Fatalf("the code was opened before the private parts were closed:\n%s", strings.Join(commands, "\n"))
	}
	for _, want := range []string{
		"chown -R 990:991 -- " + filepath.Join(base, "db"),
		"chmod -R u=rwX,go= -- " + filepath.Join(base, "db"),
		"chown -R 990:991 -- " + filepath.Join(base, "temp"),
		"chown -R 990:991 -- " + filepath.Join(base, "logs"),
		"chown -R 0:991 -- " + filepath.Join(base, "config"),
		"chmod -R u=rwX,g=rX,o= -- " + filepath.Join(base, "config"),
		"chown -R 0:0 -- " + filepath.Join(base, "public_html"),
		"chmod -R u=rwX,go=rX -- " + filepath.Join(base, "program"),
	} {
		found := false
		for _, command := range commands {
			found = found || command == want
		}
		if !found {
			t.Fatalf("missing %q in:\n%s", want, strings.Join(commands, "\n"))
		}
	}
	for _, command := range commands {
		if strings.Contains(command, "www-data") {
			t.Fatalf("the web server's group is still given something: %s", command)
		}
	}
	if root != base+" -rwxr-xr-x" {
		t.Fatalf("tree root = %q", root)
	}
}
