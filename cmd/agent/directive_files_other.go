//go:build !linux

package main

import "errors"

// directiveFileIsReal cannot be proved without openat2: refused.
var directiveFileIsReal = func(path string) error {
	return errors.New("the link check needs Linux")
}
