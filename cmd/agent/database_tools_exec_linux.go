//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// runDatabaseClientAsSiteUser runs a database client as the site's own
// account (its uid and primary group, no supplementary groups), with a
// minimal environment plus env, reading stdin. The client never runs as
// root: a file it reads belongs to the customer (alpha.128, G7). It returns
// the first 64 KiB of the combined output (the error comes first).
//
// Veritabanı istemcisini sitenin kendi hesabıyla çalıştırır; asla root değil.
var runDatabaseClientAsSiteUser = func(ctx context.Context, username string, argv []string, env []string, stdin io.Reader) (string, error) {
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
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = "/"
	cmd.Env = append([]string{
		"PATH=/usr/local/bin:/usr/bin:/bin", "HOME=" + account.HomeDir, "USER=" + username,
		"LOGNAME=" + username, "LANG=C.UTF-8", "LC_ALL=C.UTF-8",
	}, env...)
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
	cmd.Stdin = stdin
	out := &limitedBuffer{left: 64 << 10}
	cmd.Stdout, cmd.Stderr = out, out
	err = cmd.Run()
	text := strings.TrimSpace(out.buffer.String())
	if ctx.Err() != nil {
		return text, errors.New("it did not finish in time")
	}
	return text, err
}
