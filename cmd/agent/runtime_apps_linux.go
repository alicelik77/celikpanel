//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"github.com/alicelik/celikpanel/internal/siteruntime"
)

// readRuntimeAppEntryFacts reads the app folder and the entry without
// following a link anywhere below the site home: the folder is opened
// beneath the home and the entry beneath the folder with openat2
// RESOLVE_BENEATH | RESOLVE_NO_SYMLINKS | RESOLVE_NO_MAGICLINKS, O_NOFOLLOW
// and O_NONBLOCK (a FIFO planted as the entry cannot stall the Agent). The
// entry is only read, never executed. A link anywhere is ELOOP and reported
// as a link; a missing file is os.ErrNotExist.
func readRuntimeAppEntryFacts(appRoot, relative string) (siteruntime.FileFacts, error) {
	var facts siteruntime.FileFacts
	home := filepath.Dir(appRoot)
	homeFD, err := unix.Open(home, unix.O_PATH|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		if errors.Is(err, unix.ENOENT) {
			return facts, os.ErrNotExist
		}
		return facts, fmt.Errorf("open the site home: %w", err)
	}
	defer unix.Close(homeFD)
	resolve := uint64(unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_MAGICLINKS | unix.RESOLVE_NO_XDEV)
	appFD, err := unix.Openat2(homeFD, filepath.Base(appRoot), &unix.OpenHow{
		Flags: unix.O_PATH | unix.O_DIRECTORY | unix.O_NOFOLLOW | unix.O_CLOEXEC, Resolve: resolve})
	switch {
	case errors.Is(err, unix.ENOENT):
		return facts, os.ErrNotExist
	case errors.Is(err, unix.ELOOP) || errors.Is(err, unix.ENOTDIR):
		facts.FolderLink = true
		return facts, nil
	case errors.Is(err, unix.ENOSYS):
		return facts, errors.New("this kernel has no openat2, which the entry check needs")
	case err != nil:
		return facts, fmt.Errorf("open the app folder: %w", err)
	}
	defer unix.Close(appFD)
	var folder unix.Stat_t
	if err := unix.Fstat(appFD, &folder); err != nil {
		return facts, err
	}
	facts.FolderUID = int(folder.Uid)
	fd, err := unix.Openat2(appFD, relative, &unix.OpenHow{
		Flags: unix.O_RDONLY | unix.O_NOFOLLOW | unix.O_NONBLOCK | unix.O_NOCTTY | unix.O_CLOEXEC, Resolve: resolve})
	switch {
	case errors.Is(err, unix.ENOENT) || errors.Is(err, unix.ENOTDIR):
		return facts, os.ErrNotExist
	case errors.Is(err, unix.ELOOP) || errors.Is(err, unix.EXDEV):
		facts.Link = true
		return facts, nil
	case err != nil:
		return facts, fmt.Errorf("open the entry: %w", err)
	}
	defer unix.Close(fd)
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return facts, err
	}
	facts.UID = int(st.Uid)
	facts.Mode = st.Mode & 0o7777
	facts.Regular = st.Mode&unix.S_IFMT == unix.S_IFREG
	if facts.Regular {
		buf := make([]byte, 256)
		n, _ := unix.Read(fd, buf)
		if n > 0 {
			facts.Header = buf[:n]
		}
	}
	return facts, nil
}

// runRuntimeAppAsUser executes one program (mkdir, rm) directly as the site's
// account: no shell, no supplementary groups, a minimal environment, "/" as
// the working folder. It refuses a system account.
func runRuntimeAppAsUser(account serviceAppAccount, program string, args ...string) ([]byte, error) {
	if account.UID < 1000 || account.GID < 1000 {
		return nil, errors.New("refusing to run as a system account")
	}
	if program != "/bin/mkdir" && program != "/bin/rm" {
		return nil, errors.New("only mkdir and rm run as the site's account here")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, program, args...)
	cmd.Dir = "/"
	cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + account.Home, "LC_ALL=C"}
	cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{
		Uid: uint32(account.UID), Gid: uint32(account.GID), Groups: []uint32{},
	}}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
	}
	return out, nil
}
