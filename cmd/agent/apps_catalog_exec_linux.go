//go:build linux

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// runAsSiteUser runs argv as the site's own user (its uid and primary group,
// no supplementary groups, umask 027) in dir, with a minimal environment and
// the given standard input. The whole process group is killed when ctx ends.
// It returns the combined output (its last 64 KiB).
//
// runAsSiteUser komutu sitenin kendi kullanıcısıyla, en küçük ortamla çalıştırır.
func runAsSiteUser(ctx context.Context, username, dir string, argv []string, stdin []byte) (string, error) {
	if len(argv) == 0 {
		return "", errors.New("empty command")
	}
	account, err := user.Lookup(username)
	if err != nil {
		return "", fmt.Errorf("the site user %s was not found", username)
	}
	uid, err := strconv.Atoi(account.Uid)
	if err != nil || uid < 1000 {
		return "", errors.New("the site user is not a hosting account")
	}
	gid, err := strconv.Atoi(account.Gid)
	if err != nil || gid < 1 {
		return "", errors.New("the site user's group is not valid")
	}
	cmd := exec.CommandContext(ctx, "/bin/sh", append([]string{"-c", `umask 027; exec "$@"`, "sh"}, argv...)...)
	cmd.Dir = dir
	cmd.Env = []string{
		"PATH=/usr/local/bin:/usr/bin:/bin", "HOME=" + account.HomeDir, "USER=" + username,
		"LOGNAME=" + username, "LANG=C.UTF-8", "LC_ALL=C.UTF-8",
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid:    true,
		Credential: &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid), Groups: []uint32{}},
		Pdeathsig:  syscall.SIGKILL,
	}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		err := unix.Kill(-cmd.Process.Pid, unix.SIGKILL)
		if errors.Is(err, unix.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	cmd.WaitDelay = 5 * time.Second
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	out := &tailBuffer{limit: 64 << 10}
	cmd.Stdout, cmd.Stderr = out, out
	err = cmd.Run()
	text := strings.TrimSpace(out.String())
	if ctx.Err() != nil {
		return text, fmt.Errorf("it did not finish in time")
	}
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return text, fmt.Errorf("it ended with exit code %d", exit.ExitCode())
		}
		return text, err
	}
	return text, nil
}

// freeDiskMB is the free room for unprivileged writes on dir's file system
// (0 when it cannot be read).
func freeDiskMB(dir string) int64 {
	var stat unix.Statfs_t
	if err := unix.Statfs(dir, &stat); err != nil {
		return 0
	}
	return int64(stat.Bavail) * int64(stat.Bsize) / (1 << 20)
}
