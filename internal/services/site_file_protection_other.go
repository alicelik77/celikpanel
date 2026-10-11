//go:build !linux

package services

// measureSiteFileProtection is measured on Linux only; elsewhere nothing is
// known.
func measureSiteFileProtection(string) (string, string) { return "", "" }

// siteFileErrnoName is measured on Linux only.
func siteFileErrnoName(error) string { return "" }
