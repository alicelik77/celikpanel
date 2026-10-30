//go:build !linux

package pacmanlocal

import (
	"errors"
	"os"
)

var errUnsupported = errors.New("the pacman local database is read only on Linux")

func openNoFollow(string) (*os.File, error) { return nil, errUnsupported }

// ReadSecureFile is unavailable off Linux.
func ReadSecureFile(string, int64) ([]byte, error) { return nil, errUnsupported }

func secureDirectory(string) error { return errUnsupported }
