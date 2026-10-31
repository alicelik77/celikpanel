package main

import "path/filepath"

func dispatchDNSApplicationCompatibility(args []string, uid int, check func(string, string) error, report func(string)) int {
	if uid != 0 {
		return exitNotOwner
	}
	if len(args) != 5 || args[0] != "verify-dns-application" || args[1] != "--bin" || args[3] != "--state-root" ||
		!filepath.IsAbs(args[2]) || filepath.Clean(args[2]) != args[2] || filepath.Base(args[2]) != "bin" ||
		!filepath.IsAbs(args[4]) || filepath.Clean(args[4]) != args[4] {
		return exitUsage
	}
	if err := check(args[2], args[4]); err != nil {
		report(dnsApplicationRefusal(err))
		return exitUnavailable
	}
	return exitOK
}

// The updater keeps one diagnostic line of at most 240 bytes (update.sh
// update_probe_diagnostic) and the Panel bounds it again: the cause comes
// first and the line stays whole. Before alpha.114 a 210-byte preamble cut
// the cause mid-word ("v3 PowerDNS evidence requ", E7).
func dnsApplicationRefusal(err error) string {
	return "DNS state check failed: " + err.Error() + ". Nothing changed; preserve the DNS files."
}
