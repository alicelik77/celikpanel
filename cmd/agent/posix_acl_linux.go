//go:build linux

package main

import (
	"errors"
	"fmt"

	"golang.org/x/sys/unix"
)

// One POSIX access-control-list entry on Roundcube's tree (S4 R1, second fix
// round, B2): the tree's root stays closed to others (root:<web group> 0750,
// as the install leaves it) and webmail's own account alone gets through it
// by an entry "user:celikpanel-webmail:--x".
//
// It is set through the system call (fsetxattr of system.posix_acl_access on
// a descriptor opened without following any link), not with setfacl: the acl
// package (setfacl, getfacl) is not installed on every Debian 13 or Ubuntu
// 24.04 system, the Agent must not depend on a tool to keep a tenant
// boundary, and the descriptor means a link swapped in cannot redirect the
// change. The format is the kernel's (include/uapi/linux/posix_acl_xattr.h):
// a little-endian version 2 header, then entries of tag, permission and id,
// sorted by tag and id. The owner reads and removes the entry with the
// standard tools: getfacl /var/lib/celikpanel-webmail, setfacl -x
// u:celikpanel-webmail /var/lib/celikpanel-webmail.

const posixACLXattr = "system.posix_acl_access"

// openACLTarget opens a directory or file for its access-control list
// without following any symbolic link on the way (RESOLVE_NO_SYMLINKS from
// "/").
func openACLTarget(path string) (int, error) {
	relative, err := secureConfigRelativePath(path)
	if err != nil {
		return -1, err
	}
	rootFD, err := openSecureConfigRoot()
	if err != nil {
		return -1, err
	}
	defer unix.Close(rootFD)
	fd, err := unix.Openat2(rootFD, relative, &unix.OpenHow{
		Flags:   uint64(unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW),
		Resolve: secureConfigResolve,
	})
	if err != nil {
		return -1, secureConfigOpenError("open for its access-control list", path, err)
	}
	return fd, nil
}

// readPathACL returns the path's access-control list as stored (nil when it
// has none beyond its mode). A file system without them is errACLUnsupported.
var readPathACL = func(path string) ([]byte, error) {
	fd, err := openACLTarget(path)
	if err != nil {
		return nil, err
	}
	defer unix.Close(fd)
	buf := make([]byte, 4096)
	n, err := unix.Fgetxattr(fd, posixACLXattr, buf)
	switch {
	case errors.Is(err, unix.ENODATA):
		return nil, nil
	case errors.Is(err, unix.EOPNOTSUPP), errors.Is(err, unix.ENOTSUP):
		return nil, fmt.Errorf("%s: %w", path, errACLUnsupported)
	case err != nil:
		return nil, fmt.Errorf("read the access-control list of %s: %w", path, err)
	}
	return append([]byte(nil), buf[:n]...), nil
}

// writePathACL sets the path's access-control list (nil removes it, leaving
// the mode alone).
var writePathACL = func(path string, acl []byte) error {
	fd, err := openACLTarget(path)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	if len(acl) == 0 {
		if err := unix.Fremovexattr(fd, posixACLXattr); err != nil && !errors.Is(err, unix.ENODATA) {
			if errors.Is(err, unix.EOPNOTSUPP) || errors.Is(err, unix.ENOTSUP) {
				return fmt.Errorf("%s: %w", path, errACLUnsupported)
			}
			return fmt.Errorf("remove the access-control list of %s: %w", path, err)
		}
		return nil
	}
	if err := unix.Fsetxattr(fd, posixACLXattr, acl, 0); err != nil {
		if errors.Is(err, unix.EOPNOTSUPP) || errors.Is(err, unix.ENOTSUP) {
			return fmt.Errorf("%s: %w", path, errACLUnsupported)
		}
		return fmt.Errorf("set the access-control list of %s: %w", path, err)
	}
	return nil
}
