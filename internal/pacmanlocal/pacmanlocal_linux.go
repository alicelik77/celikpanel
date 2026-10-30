//go:build linux

package pacmanlocal

import (
	"errors"
	"fmt"
	"io"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

func openNoFollow(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
}

// ReadSecureFile reads one bounded, root-owned regular file that only root
// can write, without following links.
func ReadSecureFile(path string, limit int64) ([]byte, error) {
	file, err := openNoFollow(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !info.Mode().IsRegular() || !ok || stat.Uid != 0 || info.Mode().Perm()&0o022 != 0 {
		return nil, fmt.Errorf("pacman database file %s is not a root-owned regular file", path)
	}
	raw, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > limit {
		return nil, fmt.Errorf("pacman database file %s exceeds its bound", path)
	}
	return raw, nil
}

func secureDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("pacman local database: %w", err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !info.IsDir() || !ok || stat.Uid != 0 || info.Mode().Perm()&0o022 != 0 {
		return errors.New("pacman local database directory is not root-owned")
	}
	return nil
}
