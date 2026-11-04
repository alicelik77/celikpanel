//go:build !linux

package sandboxbind

import (
	"errors"
	"fmt"
	"io"
)

// CheckAppFolder needs Linux (openat2).
func CheckAppFolder(appDir string, uid uint32) error {
	if _, err := PlanFor(appDir, uid); err != nil {
		return err
	}
	return errors.New("the app folder check runs only on Linux")
}

// Main needs Linux.
func Main(_ []string, stderr io.Writer) int {
	fmt.Fprintln(stderr, "celikpanel-sandbox: the helper runs only on Linux")
	return 1
}
