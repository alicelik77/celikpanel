//go:build !linux

package dnsunitidentity

// Only Linux hosts run vendor DNS units; elsewhere the /lib name is refused.
func hostLibIsUsrLibAlias() bool {
	return false
}
