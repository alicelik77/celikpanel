//go:build linux

package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"

	"github.com/alicelik/celikpanel/internal/siteruntime"
)

// The Python kind's reads of the customer's folder, all beneath the app
// folder and without following any link (openat2 RESOLVE_BENEATH |
// RESOLVE_NO_SYMLINKS | RESOLVE_NO_MAGICLINKS | RESOLVE_NO_XDEV), as the
// entry check does. Root only reads here; it never executes a file.

const runtimeResolve = uint64(unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_MAGICLINKS | unix.RESOLVE_NO_XDEV)

// openRuntimeAppRoot opens the app folder beneath the site home.
func openRuntimeAppRoot(appRoot string, flags uint64) (int, error) {
	homeFD, err := unix.Open(filepath.Dir(appRoot), unix.O_PATH|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		if errors.Is(err, unix.ENOENT) {
			return -1, os.ErrNotExist
		}
		return -1, fmt.Errorf("open the site home: %w", err)
	}
	defer unix.Close(homeFD)
	fd, err := unix.Openat2(homeFD, filepath.Base(appRoot), &unix.OpenHow{Flags: flags | unix.O_DIRECTORY | unix.O_NOFOLLOW | unix.O_CLOEXEC, Resolve: runtimeResolve})
	switch {
	case errors.Is(err, unix.ENOENT):
		return -1, os.ErrNotExist
	case errors.Is(err, unix.ELOOP) || errors.Is(err, unix.ENOTDIR) || errors.Is(err, unix.EXDEV):
		return -1, errSourceLink
	case err != nil:
		return -1, fmt.Errorf("open the app folder: %w", err)
	}
	return fd, nil
}

// readRuntimeAppSourceFile reads a regular file beneath the app folder
// (rel such as "src/requirements.txt"), at most max bytes.
func readRuntimeAppSourceFile(appRoot, rel string, max int64) ([]byte, error) {
	appFD, err := openRuntimeAppRoot(appRoot, unix.O_PATH)
	if err != nil {
		return nil, err
	}
	defer unix.Close(appFD)
	fd, err := unix.Openat2(appFD, rel, &unix.OpenHow{
		Flags: unix.O_RDONLY | unix.O_NOFOLLOW | unix.O_NONBLOCK | unix.O_NOCTTY | unix.O_CLOEXEC, Resolve: runtimeResolve})
	switch {
	case errors.Is(err, unix.ENOENT) || errors.Is(err, unix.ENOTDIR):
		return nil, os.ErrNotExist
	case errors.Is(err, unix.ELOOP) || errors.Is(err, unix.EXDEV):
		return nil, errSourceLink
	case err != nil:
		return nil, fmt.Errorf("open %s: %w", rel, err)
	}
	f := os.NewFile(uintptr(fd), rel)
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", rel)
	}
	if st.Size() > max {
		return nil, &siteruntime.ReqRefusal{File: filepath.Base(rel), Code: siteruntime.ReqTooLarge, Text: fmt.Sprintf("larger than %d KiB", max>>10)}
	}
	return io.ReadAll(io.LimitReader(f, max+1))
}

// listRuntimeAppDirNames lists the real folders directly in the app folder
// (a link is never listed: getdents reports its own type).
func listRuntimeAppDirNames(appRoot string) ([]string, error) {
	fd, err := openRuntimeAppRoot(appRoot, unix.O_RDONLY)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), appRoot)
	defer f.Close()
	entries, err := f.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	out := []string{}
	for _, e := range entries {
		if e.Type().IsDir() {
			out = append(out, e.Name())
		}
	}
	return out, nil
}

// runtimeAppVenvCompleteOnDisk: <app>/<name>/pyvenv.cfg is a regular file
// reached without a link (the environment was finished by "python -m venv").
func runtimeAppVenvCompleteOnDisk(appRoot, name string) bool {
	appFD, err := openRuntimeAppRoot(appRoot, unix.O_PATH)
	if err != nil {
		return false
	}
	defer unix.Close(appFD)
	fd, err := unix.Openat2(appFD, name+"/pyvenv.cfg", &unix.OpenHow{
		Flags: unix.O_PATH | unix.O_NOFOLLOW | unix.O_CLOEXEC, Resolve: runtimeResolve})
	if err != nil {
		return false
	}
	defer unix.Close(fd)
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return false
	}
	return st.Mode&unix.S_IFMT == unix.S_IFREG
}
