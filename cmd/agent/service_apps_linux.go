//go:build linux

package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// runServiceAppAsUser runs a fixed shell text as the site's user, with no
// supplementary groups and a minimal environment. Values arrive as
// positional arguments; stdin (when given) is the content to write.
//
// runServiceAppAsUser, sabit bir kabuk metnini sitenin kullanıcısıyla, ek
// grup olmadan ve en küçük ortamla çalıştırır.
func runServiceAppAsUser(account serviceAppAccount, stdin io.Reader, script string, args ...string) ([]byte, error) {
	if account.UID < 1000 || account.GID < 1000 {
		return nil, errors.New("refusing to run as a system account")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/bin/sh", append([]string{"-c", script, "sh"}, args...)...)
	cmd.Dir = "/"
	cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + account.Home, "LC_ALL=C"}
	cmd.Stdin = stdin
	cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{
		Uid: uint32(account.UID), Gid: uint32(account.GID), Groups: []uint32{},
	}}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
	}
	return out, nil
}

// unitCgroupPIDs lists every process in a unit's control group (cgroup v2),
// including child processes such as n8n's task runner.
func unitCgroupPIDs(unit string) (map[int]bool, error) {
	out, err := appUnitSystemctl(systemctlArgs("show", unit, "--property=ControlGroup", "--value")...)
	if err != nil {
		return nil, fmt.Errorf("systemctl show: %v", err)
	}
	group := strings.TrimSpace(string(out))
	pids := map[int]bool{}
	if group == "" {
		return pids, nil
	}
	root := filepath.Join("/sys/fs/cgroup", filepath.Clean("/"+group))
	walkErr := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || entry.Name() != "cgroup.procs" {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			if pid, err := strconv.Atoi(strings.TrimSpace(scanner.Text())); err == nil {
				pids[pid] = true
			}
		}
		return scanner.Err()
	})
	if walkErr != nil && !errors.Is(walkErr, fs.ErrNotExist) {
		return nil, walkErr
	}
	return pids, nil
}
