package sitesandbox

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// The proof before a sandboxed program is published: what systemd actually
// loaded (`systemctl show`) matches the profile, the unit's cgroup is in the
// SMTP block set, and it listens on loopback only. Any failure, and anything
// that cannot be read, is a refusal with a code; there is no "probably".

// Proof codes.
const (
	ProofOK               = "ok"
	ProofUnreadable       = "unreadable"
	ProofProperty         = "property_mismatch"
	ProofCgroup           = "cgroup_unexpected"
	ProofNotInSet         = "not_in_smtp_set"
	ProofEgressMissing    = "egress_rule_missing"
	ProofNotListening     = "not_listening"
	ProofListensElsewhere = "listens_elsewhere"
	ProofPortTaken        = "port_taken"
)

// ProofResult is the outcome of Prove.
type ProofResult struct {
	OK     bool   `json:"ok"`
	Code   string `json:"code"`
	Detail string `json:"detail,omitempty"`
}

func refuse(code, format string, args ...any) ProofResult {
	return ProofResult{Code: code, Detail: fmt.Sprintf(format, args...)}
}

// expectation is one `systemctl show` property and how it is compared.
type expectation struct {
	value    string
	contains bool
}

// ShowProperties are the properties Prove reads (systemctl show -p ...).
func ShowProperties() []string {
	return []string{
		"User", "Group", "ControlGroup", "NoNewPrivileges", "ProtectSystem", "ProtectHome", "PrivateTmp",
		"PrivateDevices", "ProtectKernelTunables", "ProtectKernelModules", "ProtectKernelLogs",
		"ProtectControlGroups", "ProtectClock", "ProtectHostname", "ProtectProc", "ProcSubset",
		"PrivateIPC", "PrivatePIDs", "RestrictSUIDSGID", "RestrictRealtime", "LockPersonality",
		"RemoveIPC", "MemoryDenyWriteExecute", "CapabilityBoundingSet", "AmbientCapabilities",
		"UMask", "MemoryMax", "MemorySwapMax", "TasksMax", "IPAddressDeny", "SocketBindDeny",
		"ExecStartPre", "ExecStopPost", "LogRateLimitBurst",
	}
}

// Expected are the properties a unit rendered from p on a host with c must
// show. Only values whose `systemctl show` form is fixed are compared
// exactly; the rest by a fragment.
func Expected(p Profile, c Caps, unit string) map[string]expectation {
	e := map[string]expectation{
		"User":                  {value: p.User},
		"Group":                 {value: p.Group},
		"ControlGroup":          {value: "/system.slice/" + unit},
		"NoNewPrivileges":       {value: "yes"},
		"ProtectSystem":         {value: "strict"},
		"ProtectHome":           {value: "yes"},
		"PrivateTmp":            {value: "yes"},
		"PrivateDevices":        {value: "yes"},
		"ProtectKernelTunables": {value: "yes"},
		"ProtectKernelModules":  {value: "yes"},
		"ProtectKernelLogs":     {value: "yes"},
		"ProtectControlGroups":  {value: "yes"},
		"ProtectClock":          {value: "yes"},
		"ProtectHostname":       {value: "yes"},
		"RestrictSUIDSGID":      {value: "yes"},
		"RestrictRealtime":      {value: "yes"},
		"LockPersonality":       {value: "yes"},
		"RemoveIPC":             {value: "yes"},
		"CapabilityBoundingSet": {value: ""},
		"AmbientCapabilities":   {value: ""},
		"UMask":                 {value: "0077"},
		"MemoryMax":             {value: strconv.FormatInt(int64(p.Limits.MemoryMB)<<20, 10)},
		"MemorySwapMax":         {value: "0"},
		"TasksMax":              {value: strconv.Itoa(p.Limits.Tasks)},
		"LogRateLimitBurst":     {value: "1000"},
	}
	if c.SystemdVersion >= 247 {
		e["ProtectProc"] = expectation{value: "invisible"}
		if p.Kind != KindServiceApp {
			e["ProcSubset"] = expectation{value: "pid"}
		}
	}
	if c.SystemdVersion >= 248 {
		e["PrivateIPC"] = expectation{value: "yes"}
	}
	if c.SystemdVersion >= 257 {
		e["PrivatePIDs"] = expectation{value: "yes"}
	}
	if p.Kind == KindProgram {
		e["MemoryDenyWriteExecute"] = expectation{value: "yes"}
	}
	if c.bpfEnforced() {
		e["IPAddressDeny"] = expectation{value: "169.254.0.0/16", contains: true}
		if c.SystemdVersion >= 249 {
			e["SocketBindDeny"] = expectation{value: "any", contains: true}
		}
	}
	if c.EgressEnforced() {
		e["ExecStartPre"] = expectation{value: EgressFile, contains: true}
		e["ExecStopPost"] = expectation{value: "delete element inet " + EgressTable, contains: true}
	}
	return e
}

// ParseShow reads `systemctl show` output into properties.
func ParseShow(output string) map[string]string {
	props := map[string]string{}
	for _, line := range strings.Split(output, "\n") {
		if key, value, ok := strings.Cut(line, "="); ok {
			key = strings.TrimSpace(key)
			if prev, seen := props[key]; seen {
				// ExecStartPre and the like repeat: keep every line.
				props[key] = prev + "\n" + strings.TrimSpace(value)
				continue
			}
			props[key] = strings.TrimSpace(value)
		}
	}
	return props
}

// ProofInput is what the Agent read for one running unit.
type ProofInput struct {
	Unit    string
	Profile Profile
	Caps    Caps
	// Show is `systemctl show <unit> -p <ShowProperties>` output.
	Show string
	// SetElements are ParseSetElements of the SMTP set (nil: unreadable).
	SetElements []string
	// OutputChain is `nft list chain inet celikpanel_egress output`.
	OutputChain string
	// Listen is the listen proof of the unit's processes (nil: the unit
	// listens on nothing, an install step).
	Listen *ListenProof
}

// Prove compares a running unit with its profile. It fails closed: the
// first mismatch, and anything missing, is the refusal.
func Prove(in ProofInput) ProofResult {
	if !ValidUnitName(in.Unit) {
		return refuse(ProofUnreadable, "not a sandboxed unit name")
	}
	if strings.TrimSpace(in.Show) == "" {
		return refuse(ProofUnreadable, "systemd's view of %s could not be read", in.Unit)
	}
	props := ParseShow(in.Show)
	expected := Expected(in.Profile, in.Caps, in.Unit)
	names := make([]string, 0, len(expected))
	for name := range expected {
		names = append(names, name)
	}
	sort.Strings(names)
	if got, ok := props["ControlGroup"]; !ok || got != expected["ControlGroup"].value {
		return refuse(ProofCgroup, "%s runs in %q, not %s", in.Unit, got, expected["ControlGroup"].value)
	}
	for _, name := range names {
		want := expected[name]
		got, ok := props[name]
		if !ok {
			return refuse(ProofProperty, "%s: systemd did not show %s", in.Unit, name)
		}
		if want.contains {
			if !strings.Contains(got, want.value) {
				return refuse(ProofProperty, "%s: %s does not include %s", in.Unit, name, want.value)
			}
			continue
		}
		if got != want.value {
			return refuse(ProofProperty, "%s: %s is %q, expected %q", in.Unit, name, got, want.value)
		}
	}
	if in.Caps.EgressEnforced() {
		if !strings.Contains(in.OutputChain, "@"+EgressSet) || !strings.Contains(in.OutputChain, "reject") {
			return refuse(ProofEgressMissing, "the SMTP block rule of inet %s is not loaded", EgressTable)
		}
		if in.SetElements == nil {
			return refuse(ProofUnreadable, "the SMTP block set could not be read")
		}
		if !InSet(in.SetElements, in.Unit) {
			return refuse(ProofNotInSet, "%s is not in the SMTP block set", in.Unit)
		}
	}
	if in.Listen != nil {
		switch {
		case len(in.Listen.Exposed) > 0:
			return refuse(ProofListensElsewhere, "%s listens on %s", in.Unit, strings.Join(in.Listen.Exposed, ", "))
		case in.Listen.Squatted:
			return refuse(ProofPortTaken, "another program holds the port of %s", in.Unit)
		case !in.Listen.Serving:
			return refuse(ProofNotListening, "%s does not listen on its loopback port", in.Unit)
		}
	}
	return ProofResult{OK: true, Code: ProofOK}
}
