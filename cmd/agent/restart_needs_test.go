package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// alpha.107 (D-038 §10): which services still run replaced code, and
// whether the server waits for a reboot, are read without changing anything.

func TestNeedrestartsBatchAnswerIsRead(t *testing.T) {
	services, current, expected, state := parseNeedrestartBatch("NEEDRESTART-VER: 3.6\nNEEDRESTART-KCUR: 6.1.0-18-amd64\nNEEDRESTART-KEXP: 6.1.0-20-amd64\nNEEDRESTART-KSTA: 3\nNEEDRESTART-SVC: nginx.service\nNEEDRESTART-SVC: php8.4-fpm.service\n")
	if strings.Join(services, ",") != "nginx.service,php8.4-fpm.service" || current != "6.1.0-18-amd64" || expected != "6.1.0-20-amd64" || state != "3" {
		t.Fatalf("%v %q %q %q", services, current, expected, state)
	}
}

func TestReplacedLibrariesAreReadFromTheProcessesOwnMaps(t *testing.T) {
	maps := strings.Join([]string{
		"7f00-7f01 r-xp 00000000 08:01 1234 /usr/lib/x86_64-linux-gnu/libssl.so.3 (deleted)",
		"7f01-7f02 r-xp 00000000 08:01 1235 /usr/lib/x86_64-linux-gnu/libc.so.6",
		"7f02-7f03 rw-s 00000000 00:01 99 /memfd:jit (deleted)",
		"7f03-7f04 rw-s 00000000 00:01 98 /dev/shm/x (deleted)",
	}, "\n")
	if got := deletedLibrariesInMaps(strings.NewReader(maps)); len(got) != 1 || got[0] != "/usr/lib/x86_64-linux-gnu/libssl.so.3" {
		t.Fatalf("got %v", got)
	}
	if restartInterruption("nginx.service", nil) != "other" || restartInterruption("celikpanel-agent.service", nil) != "panel" || restartInterruption("sshd.service", nil) != "ssh" {
		t.Fatal("interruption classes")
	}
	if item := restartNeedsService("php8.4-fpm.service", nil); item.Component != "php-fpm" || item.Interruption != "php" {
		t.Fatalf("php: %+v", item)
	}
	if item := restartNeedsService("postgresql@17-main.service", nil); item.Component != "postgresql" || item.Interruption != "database" {
		t.Fatalf("postgresql: %+v", item)
	}
}

func TestRestartsAndARebootAreReadFromTheServersOwnTraces(t *testing.T) {
	root := t.TempDir()
	restore := []func(){}
	set := func(target *string, value string) {
		old := *target
		*target = value
		restore = append(restore, func() { *target = old })
	}
	set(&procRoot, filepath.Join(root, "proc"))
	set(&rebootRequiredPath, filepath.Join(root, "run", "reboot-required"))
	set(&rebootRequiredPkgsPath, filepath.Join(root, "run", "reboot-required.pkgs"))
	set(&needrestartPath, filepath.Join(root, "absent-needrestart"))
	set(&needrestartConfigDir, filepath.Join(root, "needrestart"))
	oldModules := kernelModulesRoots
	kernelModulesRoots = []string{filepath.Join(root, "modules")}
	t.Cleanup(func() {
		kernelModulesRoots = oldModules
		for i := len(restore) - 1; i >= 0; i-- {
			restore[i]()
		}
	})
	os.MkdirAll(filepath.Join(root, "proc", "sys", "kernel"), 0o755)
	os.WriteFile(filepath.Join(root, "proc", "sys", "kernel", "osrelease"), []byte("6.12.48+deb13-amd64\n"), 0o644)
	os.MkdirAll(filepath.Join(root, "modules", "6.12.48+deb13-amd64"), 0o755)
	os.MkdirAll(filepath.Join(root, "run"), 0o755)
	os.MkdirAll(filepath.Join(root, "proc", "4242"), 0o755)
	os.WriteFile(filepath.Join(root, "proc", "4242", "maps"), []byte("7f00-7f01 r-xp 00000000 08:01 1 /usr/lib/x86_64-linux-gnu/libssl.so.3 (deleted)\n"), 0o644)
	os.WriteFile(filepath.Join(root, "proc", "4242", "cgroup"), []byte("0::/system.slice/nginx.service\n"), 0o644)

	resp := restartNeeds(context.Background())
	if !resp.ServicesKnown || resp.Source != "maps" || len(resp.Services) != 1 || resp.Services[0].Unit != "nginx.service" || resp.Services[0].Component != "nginx" || resp.Services[0].Interruption != "web" {
		t.Fatalf("services: %+v", resp)
	}
	if !resp.RebootKnown || resp.RebootRequired {
		t.Fatalf("no reboot: %+v", resp)
	}
	os.WriteFile(rebootRequiredPath, []byte("*** System restart required ***\n"), 0o644)
	os.WriteFile(rebootRequiredPkgsPath, []byte("linux-image-6.12.57+deb13-amd64\nlibc6\n"), 0o644)
	resp = restartNeeds(context.Background())
	if !resp.RebootRequired || resp.RebootReason != "reboot_required_file" || strings.Join(resp.RebootPackages, ",") != "linux-image-6.12.57+deb13-amd64,libc6" {
		t.Fatalf("reboot file: %+v", resp)
	}
	os.Remove(rebootRequiredPath)
	os.RemoveAll(filepath.Join(root, "modules", "6.12.48+deb13-amd64"))
	if resp = restartNeeds(context.Background()); !resp.RebootRequired || resp.RebootReason != "kernel_modules_gone" {
		t.Fatalf("modules gone: %+v", resp)
	}
}
