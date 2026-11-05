//go:build linux

package dnsunitidentity

import (
	"os"

	"github.com/alicelik/celikpanel/internal/hostplatform"
)

// hostIsUbuntu2204 reads /etc/os-release (read-only). Anything it cannot
// parse answers false, which keeps Ubuntu 22.04's forking BIND command
// refused.
func hostIsUbuntu2204() bool {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return false
	}
	release, err := hostplatform.ParseOSRelease(data)
	return err == nil && release.ID == "ubuntu" && release.Version == "22.04"
}
