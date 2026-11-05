//go:build !linux

package dnsunitidentity

// Only Linux hosts run vendor DNS units.
func hostIsUbuntu2204() bool {
	return false
}
