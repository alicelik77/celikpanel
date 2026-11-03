package fileaccess

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestJailPackagesCoverEveryCommandOnEveryFamily(t *testing.T) {
	for family, table := range JailCommandPackages {
		for _, command := range JailCommands {
			if table[command] == "" {
				t.Fatalf("%s: no package for %s", family, command)
			}
		}
		for _, pkg := range JailInstallPackages[family] {
			if !IsJailPackage(family, pkg) {
				t.Fatalf("%s: install.sh package %s provides no jail command", family, pkg)
			}
		}
	}
}

func TestJailPackagesPerFamily(t *testing.T) {
	cases := []struct {
		family  string
		missing []string
		want    []string
	}{
		{"apt", []string{"zip", "unzip", "vi", "gunzip", "zcat"}, []string{"gzip", "unzip", "vim-tiny", "zip"}},
		{"apt", []string{"awk", "sh", "clear", "scp", "less", "rsync", "nano", "diff"}, []string{"dash", "diffutils", "less", "mawk", "nano", "ncurses-bin", "openssh-client", "rsync"}},
		{"pacman", []string{"vi", "awk", "sh", "clear", "scp"}, []string{"bash", "ex-vi-compat", "gawk", "ncurses", "openssh"}},
		{"pacman", []string{"zip", "unzip", "less", "rsync", "diff", "realpath"}, []string{"coreutils", "diffutils", "less", "rsync", "unzip", "zip"}},
	}
	for _, c := range cases {
		got, unknown := JailPackagesFor(c.family, c.missing)
		if !reflect.DeepEqual(got, c.want) || len(unknown) != 0 {
			t.Fatalf("%s %v: %v unknown %v, want %v", c.family, c.missing, got, unknown, c.want)
		}
	}
	if got, unknown := JailPackagesFor("dnf", []string{"zip"}); len(got) != 0 || !reflect.DeepEqual(unknown, []string{"zip"}) {
		t.Fatalf("unknown family: %v %v", got, unknown)
	}
	if q := JailToolsQualifier([]string{"zip", "unzip"}); q != "unzip,zip" {
		t.Fatalf("qualifier %q", q)
	}
}

// install.sh's small prerequisites bring the packages a minimal image lacks.
func TestInstallScriptBringsTheJailPackages(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "install.sh"))
	if err != nil {
		t.Fatal(err)
	}
	lines := map[string]string{}
	for _, line := range strings.Split(string(body), "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, `"$APT_GET_BIN" install -y -qq tar xz-utils`):
			lines["apt"] = trimmed
		case strings.HasPrefix(trimmed, `"$PACMAN_BIN" -Syu --noconfirm --needed tar xz`):
			lines["pacman"] = trimmed
		}
	}
	for family, packages := range JailInstallPackages {
		fields := strings.Fields(lines[family])
		for _, pkg := range packages {
			found := false
			for _, field := range fields {
				if field == pkg {
					found = true
				}
			}
			if !found {
				t.Fatalf("install.sh (%s) does not install %s: %q", family, pkg, lines[family])
			}
		}
	}
}
