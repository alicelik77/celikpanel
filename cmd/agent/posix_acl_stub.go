//go:build !linux

package main

import "fmt"

var readPathACL = func(path string) ([]byte, error) {
	return nil, fmt.Errorf("%s: %w", path, errACLUnsupported)
}

var writePathACL = func(path string, _ []byte) error {
	return fmt.Errorf("%s: %w", path, errACLUnsupported)
}
