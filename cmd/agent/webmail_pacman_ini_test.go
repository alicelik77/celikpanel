package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Arch's php.ini enables zip already; the Roundcube drop-in must not load it
// twice ("Module zip is already loaded", Arch VM 2026-10-30).
func TestPacmanRoundcubeDropInSkipsModulesEnabledElsewhere(t *testing.T) {
	etc := t.TempDir()
	conf := filepath.Join(etc, "conf.d")
	if err := os.MkdirAll(conf, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		filepath.Join(etc, "php.ini"):                     ";extension=intl\nextension=zip\n;extension=mysqli\n",
		filepath.Join(conf, "celikpanel-phpmyadmin.ini"):  "extension=mysqli\n",
		filepath.Join(conf, pacmanRoundcubePHPConfigFile): "extension=intl\nextension=zip\n",
		filepath.Join(conf, "owner.ini"):                  "extension = \"sqlite3.so\"\n",
	}
	for path, text := range files {
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	enabled := pacmanPHPEnabledElsewhere(etc, conf, pacmanRoundcubePHPConfigFile)
	if !enabled["zip"] || !enabled["mysqli"] || !enabled["sqlite3"] || enabled["intl"] {
		t.Fatalf("enabled=%v", enabled)
	}
	got := pacmanRoundcubePHPConfigWithout(enabled)
	if strings.Contains(got, "extension=zip") || strings.Contains(got, "extension=sqlite3") ||
		!strings.Contains(got, "extension=intl\n") || !strings.Contains(got, "extension=pdo_sqlite\n") ||
		!strings.HasPrefix(got, "# Managed by CelikPanel") {
		t.Fatalf("drop-in=%q", got)
	}
	if pacmanRoundcubePHPConfigWithout(nil) != pacmanRoundcubePHPConfig {
		t.Fatal("nothing enabled elsewhere must keep the full drop-in")
	}
}
