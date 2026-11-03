//go:build linux

package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
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
//
// The process takes the account's own primary group, never the web server's:
// every site home is <user>:<web group> 0750, so the web group would let the
// site's PHP (a plugin run by upgrade.php) read every other site. New files
// still get the web group from the document root's setgid bit.
var wordpressRunAsSite = func(ctx context.Context, site transport.WordPressSite, dir string, stdin []byte, name string, args ...string) ([]byte, error) {
	account, err := user.Lookup(site.Username)
	if err != nil {
		return nil, err
	}
	if !hostingpath.IsSiteHome(account.HomeDir) {
		return nil, errors.New("the site account is not a hosting account")
	}
	uid, err := strconv.Atoi(account.Uid)
	if err != nil || uid < 1000 {
		return nil, errors.New("the site account is a system account")
	}
	gid, err := strconv.Atoi(account.Gid)
	if err != nil || gid < 1000 {
		return nil, errors.New("the site account's group is a system group")
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
		Credential: &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid), Groups: []uint32{}},
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

// rewriteStagedWordPressConfig points the staged copy's wp-config.php at the
// copy's database. It runs on the staging folder's descriptor while the tree
// is still root's and 0700 (prepareFilesRestoreEdited): no path, no link.
func rewriteStagedWordPressConfig(stageFD int, req *transport.WordPressCloneRequest) error {
	outside := errors.New("the source site keeps wp-config.php outside its web folder; CelikPanel copies only sites that keep it inside")
	fd, err := unix.Openat(stageFD, "wp-config.php", unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return outside
	}
	file := os.NewFile(uintptr(fd), "wp-config.php")
	if file == nil {
		_ = unix.Close(fd)
		return outside
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		file.Close()
		return outside
	}
	content, err := io.ReadAll(io.LimitReader(file, 1<<20))
	file.Close()
	if err != nil {
		return err
	}
	rewritten, err := rewriteWordPressConfig(content, req.TargetDatabase, req.TargetDBUser, req.TargetDBPass, req.Source.Domain, req.Target.Domain)
	if err != nil {
		return err
	}
	random := make([]byte, 8)
	if _, err := rand.Read(random); err != nil {
		return err
	}
	temp := ".wp-config-" + hex.EncodeToString(random)
	tempFD, err := unix.Openat(stageFD, temp, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return err
	}
	out := os.NewFile(uintptr(tempFD), temp)
	if out == nil {
		_ = unix.Close(tempFD)
		_ = unix.Unlinkat(stageFD, temp, 0)
		return errors.New("could not wrap file descriptor")
	}
	_, werr := out.Write(rewritten)
	if werr == nil {
		werr = out.Sync()
	}
	if werr == nil {
		werr = out.Chmod(0o600)
	}
	if cerr := out.Close(); werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = unix.Renameat(stageFD, temp, stageFD, "wp-config.php")
	}
	if werr != nil {
		_ = unix.Unlinkat(stageFD, temp, 0)
	}
	return werr
}
