//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/alicelik/celikpanel/internal/hostingpath"
	"github.com/alicelik/celikpanel/internal/transport"
	"golang.org/x/sys/unix"
)

// wordpressRunAsSite runs one command as the site's own account (no shell,
// fixed environment, own process group, bounded output). Every change a
// WordPress tool makes inside a site goes through it, so a link the site's
// files contain can never lead a root process anywhere.
//
// Komut sitenin kendi hesabıyla, kabuksuz ve sınırlı çıktıyla koşar; sitedeki
// bir bağlantı root işlemini hiçbir yere götüremez.
var wordpressRunAsSite = func(ctx context.Context, site transport.WordPressSite, dir string, stdin []byte, name string, args ...string) ([]byte, error) {
	ownership, err := resolveWordPressOwnership(site.Username)
	if err != nil {
		return nil, err
	}
	account, err := user.Lookup(site.Username)
	if err != nil {
		return nil, err
	}
	if !hostingpath.IsSiteHome(account.HomeDir) {
		return nil, errors.New("the site account is not a hosting account")
	}
	binary := name
	if !filepath.IsAbs(binary) {
		binary, err = exec.LookPath(name)
		if err != nil {
			if strings.HasPrefix(filepath.Base(name), "php") {
				return nil, errWordPressPHPMissing
			}
			return nil, fmt.Errorf("%s is not installed", name)
		}
	} else if _, err := os.Stat(binary); err != nil {
		if strings.HasPrefix(filepath.Base(binary), "php") {
			return nil, errWordPressPHPMissing
		}
		return nil, err
	}
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = dir
	cmd.Env = []string{
		"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LC_ALL=C", "LANG=C",
		"HOME=" + account.HomeDir, "USER=" + site.Username,
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid:    true,
		Credential: &syscall.Credential{Uid: uint32(ownership.uid), Gid: uint32(ownership.gid), Groups: []uint32{}},
		Pdeathsig:  syscall.SIGKILL,
	}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	cmd.WaitDelay = 5 * time.Second
	if stdin != nil {
		cmd.Stdin = strings.NewReader(string(stdin))
	}
	stdout := &limitedBuffer{left: 8 << 20}
	stderr := &limitedBuffer{left: 4 << 10}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.buffer.String())
		if detail == "" {
			detail = strings.TrimSpace(boundedText(stdout.buffer.String(), 300))
		}
		if detail != "" {
			return nil, fmt.Errorf("%s: %w: %s", filepath.Base(binary), err, boundedText(detail, 300))
		}
		return nil, fmt.Errorf("%s: %w", filepath.Base(binary), err)
	}
	return stdout.buffer.Bytes(), nil
}

// copyOwnership gives target the owner and group of source (no link is
// followed on either side).
func copyOwnership(source, target string) error {
	var stat unix.Stat_t
	if err := unix.Lstat(source, &stat); err != nil {
		return err
	}
	return unix.Lchown(target, int(stat.Uid), int(stat.Gid))
}
