package sitesandbox

import (
	"regexp"
	"strconv"
	"strings"
)

// Caps is what the host's systemd, kernel and nftables can enforce. The
// Agent's read-only probe (SandboxCapsV1) fills it; nothing is changed to
// learn it.
type Caps struct {
	// SystemdVersion is the major version of the running systemd (0:
	// unknown).
	SystemdVersion int `json:"systemd_version"`
	// BPFFramework: systemd was built with +BPF_FRAMEWORK. Without it
	// SocketBind* and IPAddress* are accepted and silently not enforced.
	BPFFramework bool `json:"bpf_framework"`
	// CgroupV2: the unified cgroup hierarchy is mounted.
	CgroupV2 bool `json:"cgroup_v2"`
	// NFTSocketCgroup: nftables accepts the egress table (a set of type
	// "socket cgroupv2 level 2"); checked with nft --check, which applies
	// nothing.
	NFTSocketCgroup bool `json:"nft_socket_cgroupv2"`
}

// Gap names one protection this host does not enforce.
type Gap string

const (
	// GapSystemdOld: systemd is older than 249 (SocketBind and the
	// directives after it are missing).
	GapSystemdOld Gap = "systemd_old"
	// GapSystemdUnknown: the systemd version could not be read.
	GapSystemdUnknown Gap = "systemd_unknown"
	// GapBPF: SocketBind and IPAddressDeny are not enforced (no BPF).
	GapBPF Gap = "bpf_missing"
	// GapCgroupV2: no unified cgroup hierarchy; neither BPF filters nor the
	// SMTP block can attach.
	GapCgroupV2 Gap = "cgroup_v2_missing"
	// GapNFTCgroup: the SMTP egress block cannot be loaded.
	GapNFTCgroup Gap = "nft_cgroup_missing"
)

// Extra names a protection only newer systemd adds; its absence does not
// reduce the level (the base set holds without it).
type Extra string

const (
	ExtraPrivatePIDs         Extra = "private_pids"
	ExtraStrictControlGroups Extra = "strict_control_groups"
)

// MinSystemd is the oldest systemd of a supported system (Ubuntu 22.04).
const MinSystemd = 249

// Assessment is the level of a host and why.
type Assessment struct {
	Level  Level   `json:"level"`
	Gaps   []Gap   `json:"gaps"`
	Extras []Extra `json:"extras"`
}

// Assess derives the level from the capabilities.
func Assess(c Caps) Assessment {
	a := Assessment{Level: LevelFull, Gaps: []Gap{}, Extras: []Extra{}}
	switch {
	case c.SystemdVersion == 0:
		a.Gaps = append(a.Gaps, GapSystemdUnknown)
	case c.SystemdVersion < MinSystemd:
		a.Gaps = append(a.Gaps, GapSystemdOld)
	}
	if !c.CgroupV2 {
		a.Gaps = append(a.Gaps, GapCgroupV2)
	}
	if !c.BPFFramework {
		a.Gaps = append(a.Gaps, GapBPF)
	}
	if !c.NFTSocketCgroup || !c.CgroupV2 {
		a.Gaps = append(a.Gaps, GapNFTCgroup)
	}
	if c.SystemdVersion >= 257 {
		a.Extras = append(a.Extras, ExtraPrivatePIDs, ExtraStrictControlGroups)
	}
	if len(a.Gaps) > 0 {
		a.Level = LevelReduced
	}
	return a
}

// bpfEnforced: SocketBind and IPAddressDeny take effect only with systemd's
// BPF framework on the unified hierarchy.
func (c Caps) bpfEnforced() bool { return c.BPFFramework && c.CgroupV2 }

// EgressEnforced: the SMTP block can be loaded on this host.
func (c Caps) EgressEnforced() bool { return c.NFTSocketCgroup && c.CgroupV2 }

var systemdVersionLine = regexp.MustCompile(`^systemd ([0-9]{2,4})\b`)

// ParseSystemctlVersion reads `systemctl --version`: the major version from
// its first line and whether the feature line names +BPF_FRAMEWORK. A line
// it does not recognise gives version 0 (unknown), never a guess.
func ParseSystemctlVersion(output string) (version int, bpf bool) {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) == 0 {
		return 0, false
	}
	if m := systemdVersionLine.FindStringSubmatch(strings.TrimSpace(lines[0])); m != nil {
		version, _ = strconv.Atoi(m[1])
	}
	for _, line := range lines[1:] {
		for _, field := range strings.Fields(line) {
			if field == "+BPF_FRAMEWORK" {
				bpf = true
			}
		}
	}
	return version, bpf
}
