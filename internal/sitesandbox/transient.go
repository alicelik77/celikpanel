package sitesandbox

import (
	"strconv"
	"strings"
)

// TransientPropertyMinimums is every property name CelikPanel passes to
// systemd-run --property, with the oldest systemd whose systemd-run parses
// it (src/shared/bus-unit-util.c). A unit file and systemd-run do not parse
// the same way: a value a unit file takes can be refused by systemd-run,
// which then starts nothing (alpha.119 lab round 2: every Python build
// failed with "Failed to parse ProtectControlGroups: strict" on systemd
// 257). 249 stands for "older than every supported system". The whole
// rendered lists of the Python build and the service-app install were run
// with systemd-run --wait on systemd 257 (Debian 13) and 262 (Arch); the
// other minimums follow systemd's NEWS. A name missing here fails the tests:
// add it only after systemd-run accepted it on the systems CelikPanel
// supports.
//
// systemd-run --property ile verilen her ad ve onu ayrıştıran en eski
// systemd; birim dosyası ile systemd-run aynı değerleri kabul etmez.
var TransientPropertyMinimums = map[string]int{
	// The unit and its lifecycle.
	"Type": 249, "RemainAfterExit": 249, "TimeoutStartSec": 249, "RuntimeMaxSec": 249,
	"Requires": 249, "After": 249, "Slice": 249,
	"ExecStartPre": 249, "ExecStartPost": 249, "ExecStopPost": 249,
	// The sandbox (Directives).
	"NoNewPrivileges": 249, "ProtectSystem": 249, "ProtectHome": 249, "PrivateTmp": 249,
	"PrivateDevices": 249, "ProtectKernelTunables": 249, "ProtectKernelModules": 249,
	"ProtectKernelLogs": 249, "ProtectClock": 249, "ProtectHostname": 249,
	"ProtectProc": 249, "ProcSubset": 249, "PrivateIPC": 249,
	"RestrictSUIDSGID": 249, "RestrictRealtime": 249, "RestrictNamespaces": 249,
	"LockPersonality": 249, "RemoveIPC": 249, "CapabilityBoundingSet": 249,
	"AmbientCapabilities": 249, "SystemCallArchitectures": 249, "SystemCallFilter": 249,
	"SystemCallErrorNumber": 249, "RestrictAddressFamilies": 249, "UMask": 249,
	"TemporaryFileSystem": 249, "BindPaths": 249, "InaccessiblePaths": 249, "ReadOnlyPaths": 249,
	"IPAddressDeny": 249, "LogRateLimitIntervalSec": 249, "LogRateLimitBurst": 249,
	"MemoryMax": 249, "MemorySwapMax": 249, "CPUQuota": 249, "TasksMax": 249,
	"LimitNOFILE": 249, "LimitCORE": 249,
	"SocketBindAllow": 249, "SocketBindDeny": 249,
	"MemoryDenyWriteExecute": 249, "NoExecPaths": 249, "ExecPaths": 249,
	"ProtectControlGroups": 249, "PrivatePIDs": 257, "ProtectControlGroupsEx": 257,
}

// TransientValueMinimums are assignments whose value systemd-run refuses on
// older systems although the name parses: systemd 257's systemd-run reads
// ProtectControlGroups= as a boolean. 262 is the oldest seen to take them;
// CelikPanel renders ProtectControlGroupsEx= instead.
var TransientValueMinimums = map[string]int{
	"ProtectControlGroups=strict":  262,
	"ProtectControlGroups=private": 262,
}

// TransientPropertyRefusal names why systemd-run of systemd version would
// refuse one --property assignment, or "" when it parses there.
func TransientPropertyRefusal(assignment string, version int) string {
	name, _, ok := strings.Cut(assignment, "=")
	if !ok || name == "" {
		return "not a property assignment"
	}
	min, known := TransientPropertyMinimums[name]
	if !known {
		return name + " is not known to parse as a systemd-run property"
	}
	if version < min {
		return name + " needs systemd " + strconv.Itoa(min) + " for systemd-run"
	}
	if vmin, limited := TransientValueMinimums[assignment]; limited && version < vmin {
		return assignment + " is refused by systemd-run before systemd " + strconv.Itoa(vmin)
	}
	return ""
}
