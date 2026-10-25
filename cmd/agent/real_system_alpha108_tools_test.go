//go:build linux && realsystem

package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// Real-system evidence for the alpha.108 PHP tool findings (Boston, Ubuntu
// 24.04), run only inside a disposable Debian 13 virtual machine booted from
// the base image (build tag realsystem, plus CELIKPANEL_REAL_SYSTEM_A108=1).
// Never run it on an installed server.
//
// The cause was not the distribution: the installed Agent runs with the
// unit's UMask=0027 and CelikPanel's PHP check script was created 0750/0640,
// so PHP-FPM's pool user could not open it. Earlier real-system runs started
// the Agent's code with umask 0022. This run sets the unit's umask, installs
// PHP-FPM (the distribution's default), PostgreSQL and phpPgAdmin through the
// Agent's own install code, and proves:
//  1. configured and proved under UMask=0027, the check's modes are 0755/0644;
//  2. Boston's state (0750/0640) makes the check answer 404, said as
//     check_unreadable; a proof (Check again) repairs it and passes;
//  3. "Use PHP <the same version>" only proves: nothing rewritten or reloaded.
//
// alpha.108 gerçek sistem kanıtı; yalnız atılabilir Debian 13 sanal makinesinde.

func requireRealSystemA108Tools(t *testing.T) {
	t.Helper()
	if os.Getenv("CELIKPANEL_REAL_SYSTEM_A108") != "1" {
		t.Skip("real-system evidence runs only in the disposable alpha.108 VM")
	}
}

func a108ToolsMode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode().Perm()
}

func TestRealSystemA108PHPToolProof(t *testing.T) {
	requireRealSystemA108Tools(t)
	a := &Agent{}
	// The distribution's PHP only (the base image's vendor PHP source out).
	if err := os.Remove("/etc/apt/sources.list.d/php.list"); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	a105Run(t, "apt-get", "update", "-q")
	for _, id := range []string{"php-fpm", "postgresql", "phppgadmin"} {
		if resp := a107Install(t, a, id, ""); resp.Error != "" {
			t.Fatalf("%s: %s", id, resp.Error)
		}
	}
	version := ""
	for _, candidate := range installedPHPFPMVersions() {
		version = candidate
		break
	}
	if version == "" {
		t.Fatal("no PHP-FPM version installed")
	}
	t.Logf("PHP-FPM %s; check script %s", version, phpToolCheckScriptPath)

	// The Agent unit's umask (deploy/systemd/celikpanel-agent.service).
	previous := syscall.Umask(0o027)
	t.Cleanup(func() { syscall.Umask(previous) })
	if err := os.RemoveAll(filepath.Dir(phpToolCheckScriptPath)); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// 1. Configured and proved under the unit's umask.
	var conf ConfigureDBToolsResponse
	configureDBToolsWith(ctx, &conf)
	logJSON(t, "configure", conf)
	if conf.Error != "" || !conf.Configured {
		t.Fatalf("configure = %+v", conf)
	}
	if dir, file := a108ToolsMode(t, filepath.Dir(phpToolCheckScriptPath)), a108ToolsMode(t, phpToolCheckScriptPath); dir != 0o755 || file != 0o644 {
		t.Fatalf("check modes = %v %v under UMask=0027", dir, file)
	}
	proof := provePHPToolWith(ctx, hostPHPToolProveOps(), phpToolPHPPgAdmin, false)
	logJSON(t, "proof under UMask=0027", proof)
	if !proof.Proved || proof.Version != version {
		t.Fatalf("proof = %+v", proof)
	}

	// 2. Boston's state: the check unreadable for the pool user.
	if err := os.Chmod(filepath.Dir(phpToolCheckScriptPath), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(phpToolCheckScriptPath, 0o640); err != nil {
		t.Fatal(err)
	}
	err := probePHPToolHTTP(ctx, phpToolPHPPgAdmin, version)
	var typed *phpToolProbeError
	if !errors.As(err, &typed) || typed.Kind != "check_unreadable" {
		t.Fatalf("Boston's state: err = %v (%+v)", err, typed)
	}
	t.Logf("Boston's state reproduced: %v", err)
	// The tool's own page still answers, as on Boston.
	if out, cerr := exec.Command("curl", "-s", "-o", "/dev/null", "-w", "%{http_code}", "http://"+dbToolsAddr+"/dbtool/phppgadmin/").Output(); cerr != nil || string(out) != "200" {
		t.Fatalf("phpPgAdmin page = %q %v", out, cerr)
	}
	proof = provePHPToolWith(ctx, hostPHPToolProveOps(), phpToolPHPPgAdmin, false)
	logJSON(t, "Check again", proof)
	if !proof.Proved || a108ToolsMode(t, phpToolCheckScriptPath) != 0o644 || a108ToolsMode(t, filepath.Dir(phpToolCheckScriptPath)) != 0o755 {
		t.Fatalf("Check again did not repair and prove: %+v", proof)
	}
	if failures := readPHPToolProofFailures(); len(failures) != 0 {
		t.Fatalf("a passing proof left a recorded failure: %+v", failures)
	}

	// 3. The same version: proof only.
	before, err := os.ReadFile(dbToolsConfPath)
	if err != nil {
		t.Fatal(err)
	}
	beforeInfo, _ := os.Stat(dbToolsConfPath)
	result, err := bindPHPToolWith(ctx, hostPHPToolBindOps(), phpToolPHPPgAdmin, version)
	logJSON(t, "same-version bind", result)
	if err != nil || !result.ProofOnly || result.MutationApplied {
		t.Fatalf("same-version bind = %+v err = %v", result, err)
	}
	after, _ := os.ReadFile(dbToolsConfPath)
	afterInfo, _ := os.Stat(dbToolsConfPath)
	if string(after) != string(before) || !afterInfo.ModTime().Equal(beforeInfo.ModTime()) {
		t.Fatal("a same-version bind rewrote the configuration")
	}
}
