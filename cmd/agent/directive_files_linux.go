//go:build linux

package main

import (
	"errors"
	"strings"

	"golang.org/x/sys/unix"
)

// directiveFileIsReal proves that path names a regular file reached from "/"
// without following any link (openat2 RESOLVE_NO_SYMLINKS on every
// component, O_NOFOLLOW on the last). The lexical "inside the site's home"
// check alone let a link inside the home point anywhere (20 Nov 2026).
//
// Yolun "/"den hiçbir bağlantı izlenmeden ulaşılan düz bir dosya olduğunu
// kanıtlar.
var directiveFileIsReal = func(path string) error {
	if !strings.HasPrefix(path, "/") {
		return errors.New("not an absolute path")
	}
	fd, err := unix.Openat2(unix.AT_FDCWD, path, &unix.OpenHow{
		Flags:   unix.O_PATH | unix.O_CLOEXEC | unix.O_NOFOLLOW,
		Resolve: unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_MAGICLINKS,
	})
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return err
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFREG {
		return errors.New("not a regular file")
	}
	return nil
}
