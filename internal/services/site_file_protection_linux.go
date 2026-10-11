//go:build linux

package services

import (
	"errors"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/unix"

	"github.com/alicelik/celikpanel/internal/transport"
)

// The inode attribute flags of linux/fs.h (golang.org/x/sys/unix names the
// ioctl, not these): `chattr +i` and `chattr +a`.
const (
	inodeFlagImmutable  = 0x00000010 // FS_IMMUTABLE_FL
	inodeFlagAppendOnly = 0x00000020 // FS_APPEND_FL
)

// measureSiteFileProtection looks, without writing anything, at what would refuse
// replacing path by a rename in its directory: a read-only file system
// (EROFS), the file's immutable or append-only attribute (`chattr +i`/`+a`,
// EPERM), or the same attribute on its directory (EPERM). It returns the
// protection (transport.SiteFileProtection*) and the system's error name a
// replacement would get; ("", "") when nothing could be measured. set10
// scenario g measured the immutable file's rename refused with EPERM on
// Debian 13, Ubuntu 24.04 and Arch.
//
// measureSiteFileProtection, dosyanın değiştirilmesini neyin reddedeceğine
// hiçbir şey yazmadan bakar: salt okunur dosya sistemi, dosyanın ya da
// dizininin değiştirilemez (chattr +i) ya da yalnız eklenebilir (+a) özniteliği.
func measureSiteFileProtection(path string) (string, string) {
	directory := filepath.Dir(path)
	measured := false
	var mount unix.Statfs_t
	if err := unix.Statfs(directory, &mount); err == nil {
		measured = true
		if mount.Flags&unix.ST_RDONLY != 0 {
			return transport.SiteFileProtectionReadOnlyFilesystem, "EROFS"
		}
	}
	if flags, ok := inodeAttributeFlags(path, false); ok {
		measured = true
		switch {
		case flags&inodeFlagImmutable != 0:
			return transport.SiteFileProtectionImmutable, "EPERM"
		case flags&inodeFlagAppendOnly != 0:
			return transport.SiteFileProtectionAppendOnly, "EPERM"
		}
	}
	if flags, ok := inodeAttributeFlags(directory, true); ok {
		measured = true
		if flags&(inodeFlagImmutable|inodeFlagAppendOnly) != 0 {
			return transport.SiteFileProtectionDirectoryImmutable, "EPERM"
		}
	}
	if !measured {
		return "", ""
	}
	return transport.SiteFileProtectionNone, ""
}

// inodeAttributeFlags reads a file's or directory's attribute flags
// (FS_IOC_GETFLAGS, what lsattr shows). It opens read-only, never follows a
// link and never blocks; a file system without attributes answers false.
func inodeAttributeFlags(path string, directory bool) (uint32, bool) {
	mode := unix.O_RDONLY | unix.O_NOFOLLOW | unix.O_NONBLOCK | unix.O_CLOEXEC
	if directory {
		mode |= unix.O_DIRECTORY
	}
	fd, err := unix.Open(path, mode, 0)
	if err != nil {
		return 0, false
	}
	defer unix.Close(fd)
	flags, err := unix.IoctlGetUint32(fd, unix.FS_IOC_GETFLAGS)
	if err != nil {
		return 0, false
	}
	return flags, true
}

// siteFileErrnoName is the system's error name in err ("EPERM"), or "".
func siteFileErrnoName(err error) string {
	var errno syscall.Errno
	if !errors.As(err, &errno) {
		return ""
	}
	return unix.ErrnoName(errno)
}
