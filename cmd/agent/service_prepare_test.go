package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClamAVSignaturesReady(t *testing.T) {
	dir := t.TempDir()
	if clamAVSignaturesReady(dir) {
		t.Fatal("empty directory must not be ready")
	}
	if err := os.Mkdir(filepath.Join(dir, "daily.cvd"), 0o755); err != nil {
		t.Fatal(err)
	}
	if clamAVSignaturesReady(dir) {
		t.Fatal("a directory named daily.cvd is not a signature database")
	}
	if err := os.Remove(filepath.Join(dir, "daily.cvd")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "daily.cld"), []byte("signature"), 0o644); err != nil {
		t.Fatal(err)
	}
	// alpha.118 lab round 2: the daemon's units require main AND daily;
	// daily alone let systemd skip the daemon and its socket for good.
	if clamAVSignaturesReady(dir) {
		t.Fatal("daily.cld without main must not be ready")
	}
	if err := os.Mkdir(filepath.Join(dir, "main.inc"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !clamAVSignaturesReady(dir) {
		t.Fatal("daily.cld with the main.inc folder must be ready")
	}
	if err := os.Remove(filepath.Join(dir, "daily.cld")); err != nil {
		t.Fatal(err)
	}
	if clamAVSignaturesReady(dir) {
		t.Fatal("main without daily must not be ready")
	}
}

func TestServiceStartsAfterPanelSetup(t *testing.T) {
	for _, id := range []string{"pdns", "bind", "postfix", "dovecot", "wireguard"} {
		if !serviceStartsAfterPanelSetup(id) {
			t.Errorf("%s must wait for panel configuration before start", id)
		}
	}
	for _, id := range []string{"nginx", "postgresql", "mariadb", "clamav"} {
		if serviceStartsAfterPanelSetup(id) {
			t.Errorf("%s must be started and verified by the install RPC", id)
		}
	}
}

func TestBINDPackageAutoStartGuardDoesNotAffectOtherServices(t *testing.T) {
	if !serviceUsesBINDPackageInstallGuard("bind") {
		t.Fatal("BIND must use the package-maintainer auto-start guard")
	}
	for _, id := range []string{"pdns", "nginx", "postfix", "dovecot", "wireguard"} {
		if serviceUsesBINDPackageInstallGuard(id) {
			t.Errorf("%s unexpectedly uses the BIND package install guard", id)
		}
	}
}
