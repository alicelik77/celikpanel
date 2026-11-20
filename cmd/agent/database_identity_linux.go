//go:build linux

package main

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/unix"
)

// fsIocGetVersion is FS_IOC_GETVERSION, _IOR('v', 1, long): the inode's
// generation (x/sys does not name it).
var fsIocGetVersion = uint(2)<<30 | uint(unsafe.Sizeof(uintptr(0)))<<16 | uint('v')<<8 | 1

// databaseDirectoryIdentity is the identity of one MariaDB database
// directory: its device, inode number, inode generation and birth time.
// MariaDB tells a client no identifier of a database; the directory it makes
// for CREATE DATABASE has one. A directory made again after a DROP can get
// the freed inode number back, but not its generation (ext4, XFS and btrfs
// give a reused inode a new one) and not its earlier birth time. "" when the
// directory cannot be opened, is not a directory (a link is not followed), or
// the filesystem records no birth time: the identity is then not known. The
// generation is "-" where the filesystem does not tell it. Read only.
//
// Veritabanı dizininin aygıtı, düğüm numarası, kuşağı ve doğum zamanı;
// bilinemezse "".
func databaseDirectoryIdentity(path string) string {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return ""
	}
	defer unix.Close(fd)
	var stat unix.Statx_t
	if err := unix.Statx(fd, "", unix.AT_EMPTY_PATH, unix.STATX_TYPE|unix.STATX_INO|unix.STATX_BTIME, &stat); err != nil {
		return ""
	}
	if stat.Mask&unix.STATX_BTIME == 0 || stat.Mask&unix.STATX_INO == 0 || stat.Mode&unix.S_IFMT != unix.S_IFDIR ||
		(stat.Btime.Sec == 0 && stat.Btime.Nsec == 0) {
		return ""
	}
	generation := "-"
	if value, err := unix.IoctlGetUint32(fd, fsIocGetVersion); err == nil {
		generation = fmt.Sprint(value)
	}
	return fmt.Sprintf("mariadb-dir:%d:%d:%d:%s:%d.%09d", stat.Dev_major, stat.Dev_minor, stat.Ino, generation, stat.Btime.Sec, stat.Btime.Nsec)
}
