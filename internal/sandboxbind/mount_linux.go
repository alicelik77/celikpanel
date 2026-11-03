//go:build linux

package sandboxbind

import (
	"errors"
	"fmt"

	"golang.org/x/sys/unix"
)

// ErrNotMounted is Unmount's answer when nothing is mounted at the target.
var ErrNotMounted = errors.New("nothing is mounted there")

// Mounter binds an opened folder onto a mount point and removes it again.
type Mounter interface {
	// Bind binds the folder open at fd onto target, not recursively, nosuid
	// and nodev, and proves that target now shows that very folder.
	Bind(fd int, target string) error
	// Unmount detaches one mount at target; ErrNotMounted when there is none.
	Unmount(target string) error
}

// KernelMounter is the real Mounter. It uses the new mount API (open_tree,
// mount_setattr, move_mount: Linux 5.2 and 5.12) and falls back to
// mount(2) of /proc/self/fd/<fd> where a call is missing. Either way the
// source is the descriptor the walk opened, never a name.
type KernelMounter struct{}

const bindAttrs = unix.MOUNT_ATTR_NOSUID | unix.MOUNT_ATTR_NODEV

// Bind implements Mounter.
func (KernelMounter) Bind(fd int, target string) error {
	tree, err := unix.OpenTree(fd, "", unix.OPEN_TREE_CLONE|unix.OPEN_TREE_CLOEXEC|unix.AT_EMPTY_PATH)
	switch {
	case errors.Is(err, unix.ENOSYS):
		if err := legacyBind(fd, target); err != nil {
			return err
		}
	case err != nil:
		return fmt.Errorf("open_tree: %w", err)
	default:
		attrsSet := true
		if err := unix.MountSetattr(tree, "", unix.AT_EMPTY_PATH, &unix.MountAttr{Attr_set: bindAttrs}); err != nil {
			if !errors.Is(err, unix.ENOSYS) {
				unix.Close(tree)
				return fmt.Errorf("mount_setattr: %w", err)
			}
			attrsSet = false
		}
		err := unix.MoveMount(tree, "", unix.AT_FDCWD, target, unix.MOVE_MOUNT_F_EMPTY_PATH)
		unix.Close(tree)
		if err != nil {
			return fmt.Errorf("move_mount: %w", err)
		}
		if !attrsSet {
			if err := unix.Mount("", target, "", unix.MS_REMOUNT|unix.MS_BIND|unix.MS_NOSUID|unix.MS_NODEV, ""); err != nil {
				_ = unix.Unmount(target, unix.MNT_DETACH)
				return fmt.Errorf("remount nosuid,nodev: %w", err)
			}
		}
	}
	if err := sameFolder(fd, target); err != nil {
		_ = unix.Unmount(target, unix.MNT_DETACH)
		return err
	}
	return nil
}

func legacyBind(fd int, target string) error {
	source := fmt.Sprintf("/proc/self/fd/%d", fd)
	if err := unix.Mount(source, target, "", unix.MS_BIND, ""); err != nil {
		return fmt.Errorf("bind mount: %w", err)
	}
	if err := unix.Mount("", target, "", unix.MS_REMOUNT|unix.MS_BIND|unix.MS_NOSUID|unix.MS_NODEV, ""); err != nil {
		_ = unix.Unmount(target, unix.MNT_DETACH)
		return fmt.Errorf("remount nosuid,nodev: %w", err)
	}
	return nil
}

// sameFolder proves that target shows the folder open at fd.
func sameFolder(fd int, target string) error {
	var want, got unix.Stat_t
	if err := unix.Fstat(fd, &want); err != nil {
		return fmt.Errorf("read the app folder: %w", err)
	}
	if err := unix.Lstat(target, &got); err != nil {
		return fmt.Errorf("read the mount point: %w", err)
	}
	if want.Dev != got.Dev || want.Ino != got.Ino {
		return errors.New("the mount point does not show the checked app folder")
	}
	return nil
}

// Unmount implements Mounter.
func (KernelMounter) Unmount(target string) error {
	err := unix.Unmount(target, unix.MNT_DETACH|unix.UMOUNT_NOFOLLOW)
	if errors.Is(err, unix.EINVAL) || errors.Is(err, unix.ENOENT) {
		return ErrNotMounted
	}
	return err
}
