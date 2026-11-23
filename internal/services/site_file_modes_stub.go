//go:build !linux

package services

import (
	"errors"
	"os"
)

func openRegularNoFollow(string) (*os.File, error) {
	return nil, errors.New("the mode correction needs Linux")
}

func rootOwned(os.FileInfo) bool { return false }
