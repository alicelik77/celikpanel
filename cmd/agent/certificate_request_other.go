//go:build !linux

package main

import "os"

// Only Linux runs the Agent; elsewhere the owner is not checked.
func certificateRequestOwnedBySelf(os.FileInfo) bool { return true }

func openCertificateRequestKeyNoFollow(path string) (*os.File, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, os.ErrPermission
	}
	return os.Open(path)
}
