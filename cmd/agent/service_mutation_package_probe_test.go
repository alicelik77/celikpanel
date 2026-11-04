package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestMain isolates ledger and supervisor tests from package-manager locks on
// the machine running the test binary. Production keeps the real host probe.
// TestMain, ledger ve supervisor testlerini test ikilisini çalıştıran makinenin
// paket yöneticisi kilitlerinden yalıtır. Üretim gerçek host probunu korur.
// Native mail acceptance retains the real package-manager and readiness probes.
// Its test entrypoint separately refuses non-QEMU or installed-panel hosts.
func TestMain(m *testing.M) {
	nativeV3 := runtime.GOOS == "linux" && os.Getenv("CELIKPANEL_V3_NATIVE_TRIAL") == "pdns-switch__intent__after-write__paired-primary__peer-reachable"
	if !nativeV3 && os.Getenv("CELIKPANEL_DISPOSABLE_MAIL_VM") != "debian13-20260911" {
		packageManagerMutationBusyProbe = func() (bool, error) { return false, nil }
	}
	// The same isolation, for the other thing these tests read off the machine:
	// whether the host has finished starting. Startup recovery asks, and when
	// the answer is "still starting" it correctly defers the decision to a
	// goroutine that waits for the host. A test that reloads a manager and
	// asserts the decision was made then returns before the goroutine runs,
	// t.TempDir() removes the state directory underneath it, and the run fails
	// with a missing lock directory - at random, because the answer depends on
	// the machine running the tests rather than on anything the test set up.
	//
	// Ten test files reload a manager that way. Pinning them one at a time
	// would be the same mistake this codebase keeps finding: fixing where it
	// was found and leaving the sibling. So the default is here, once, beside
	// the probe that was already pinned for exactly this reason. The one file
	// that is ABOUT readiness - host_boot_recovery_test.go - overrides it per
	// test to drive all three answers, and always did.
	//
	// Ayni yalitim, bu testlerin makineden okudugu diger sey icin: makinenin
	// acilisinin bitip bitmedigi. On test dosyasi yoneticiyi bu sekilde yeniden
	// yukluyor; onlari tek tek sabitlemek, bu kod tabaninin surekli buldugu
	// hatanin ayni olurdu. Varsayilan burada, bir kez.
	if !nativeV3 && os.Getenv("CELIKPANEL_DISPOSABLE_MAIL_VM") != "debian13-20260911" {
		hostRecoveryProbe = func() (hostRecoveryReadiness, error) { return hostRecoveryDecideNow, nil }
	}
	// Unit tests exercise the mutation coordinator on portability builds, where
	// production deliberately refuses every host mutation.
	if runtime.GOOS != "linux" {
		verifyServiceMutationSecurityPolicy = func() error { return nil }
	}
	// Outside the disposable VM trials no test may reach a real CA.
	if !nativeV3 && os.Getenv("CELIKPANEL_DISPOSABLE_MAIL_VM") != "debian13-20260911" {
		blockRealCertificateAuthority()
	}
	// The site sandbox's native files (the helper copy, the egress table)
	// go to a throwaway folder, never /usr/lib/celikpanel-sandbox, and the
	// app-folder walk, which reads the real hosting tree, passes unless a
	// test sets its own (sandbox_helper_test.go).
	sandboxDir, err := os.MkdirTemp("", "celikpanel-sandbox-test-")
	if err != nil {
		panic(err)
	}
	sandboxEgressDir = sandboxDir
	sandboxLedgerPath = filepath.Join(sandboxDir, "native-files.json")
	sandboxFolderRefusal = func(string, int) (string, string) { return "", "" }
	sandboxHelperSource = filepath.Join(sandboxDir, "agent-under-test")
	if err := os.WriteFile(sandboxHelperSource, []byte("#!/bin/false\n"), 0o755); err != nil {
		panic(err)
	}
	sandboxEgressDir = filepath.Join(sandboxDir, "celikpanel-sandbox")
	sandboxUnits := filepath.Join(sandboxDir, "systemd")
	if err := os.MkdirAll(sandboxUnits, 0o755); err != nil {
		panic(err)
	}
	sandboxUnitDir = func() (string, error) { return sandboxUnits, nil }
	sandboxDaemonReload = func() error { return nil }
	code := m.Run()
	_ = os.RemoveAll(sandboxDir)
	os.Exit(code)
}
