package sitesandbox

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/alicelik/celikpanel/internal/hostingpath"
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
	// ProofSitePath: systemd would read a path inside the hosting tree on
	// the host, as root (a bind source or an environment file): a link
	// there would follow anywhere.
	ProofSitePath = "site_path_on_host"
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

// Expectation is one `systemctl show` property and how it is compared.
type Expectation struct {
	Value    string
	Contains bool
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
		"BindPaths", "BindReadOnlyPaths", "EnvironmentFiles", "Requires", "After",
	}
}

// Expected are the properties a unit rendered from p on a host with c must
// show. Only values whose `systemctl show` form is fixed are compared
// exactly; the rest by a fragment.
func Expected(p Profile, c Caps, unit string) map[string]Expectation {
	e := map[string]Expectation{
		"User":                  {Value: p.User},
		"Group":                 {Value: p.Group},
		"ControlGroup":          {Value: "/system.slice/" + unit},
		"NoNewPrivileges":       {Value: "yes"},
		"ProtectSystem":         {Value: "strict"},
		"ProtectHome":           {Value: "yes"},
		"PrivateTmp":            {Value: "yes"},
		"PrivateDevices":        {Value: "yes"},
		"ProtectKernelTunables": {Value: "yes"},
		"ProtectKernelModules":  {Value: "yes"},
		"ProtectKernelLogs":     {Value: "yes"},
		"ProtectControlGroups":  {Value: "yes"},
		"ProtectClock":          {Value: "yes"},
		"ProtectHostname":       {Value: "yes"},
		"RestrictSUIDSGID":      {Value: "yes"},
		"RestrictRealtime":      {Value: "yes"},
		"LockPersonality":       {Value: "yes"},
		"RemoveIPC":             {Value: "yes"},
		"CapabilityBoundingSet": {Value: ""},
		"AmbientCapabilities":   {Value: ""},
		"UMask":                 {Value: "0077"},
		"MemoryMax":             {Value: strconv.FormatInt(int64(p.Limits.MemoryMB)<<20, 10)},
		"MemorySwapMax":         {Value: "0"},
		"TasksMax":              {Value: strconv.Itoa(p.Limits.Tasks)},
		"LogRateLimitBurst":     {Value: "1000"},
	}
	if c.SystemdVersion >= 247 {
		e["ProtectProc"] = Expectation{Value: "invisible"}
		if ProcSubsetPID(p.Kind) {
			e["ProcSubset"] = Expectation{Value: "pid"}
		}
	}
	if c.SystemdVersion >= 248 {
		e["PrivateIPC"] = Expectation{Value: "yes"}
	}
	if c.SystemdVersion >= 257 {
		e["PrivatePIDs"] = Expectation{Value: "yes"}
	}
	if p.Kind == KindProgram {
		e["MemoryDenyWriteExecute"] = Expectation{Value: "yes"}
	}
	if c.bpfEnforced() {
		e["IPAddressDeny"] = Expectation{Value: "169.254.0.0/16", Contains: true}
		if c.SystemdVersion >= 249 {
			e["SocketBindDeny"] = Expectation{Value: "any", Contains: true}
		}
	}
	// The bind unit binds the folder before every start (its source is the
	// root-owned mount point); where the host holds it, the unit's helper
	// lines load the SMTP block.
	e["Requires"] = Expectation{Value: BindUnit(p.Unit), Contains: true}
	e["After"] = Expectation{Value: BindUnit(p.Unit), Contains: true}
	if c.EgressEnforced() {
		e["ExecStartPre"] = Expectation{Value: "argv[]=" + HelperCommand(EgressAddArgs(p.Unit)), Contains: true}
		e["ExecStopPost"] = Expectation{Value: "argv[]=" + HelperCommand(EgressRemoveArgs(p.Unit)), Contains: true}
	}
	e["BindPaths"] = Expectation{Value: BindSource(p.Unit) + ":" + p.AppDir, Contains: true}
	if p.RuntimeDir != "" {
		e["BindReadOnlyPaths"] = Expectation{Value: p.RuntimeDir, Contains: true}
	}
	return e
}

// sitePathsOnHost lists the paths systemd reads on the host as root that lie
// in the hosting tree: bind sources and environment files.
func sitePathsOnHost(props map[string]string) []string {
	base := hostingpath.HostingBase()
	inTree := func(p string) bool { return p == base || strings.HasPrefix(p, base+"/") }
	var out []string
	for _, name := range []string{"BindPaths", "BindReadOnlyPaths"} {
		for _, field := range strings.Fields(props[name]) {
			source, _, _ := strings.Cut(strings.TrimPrefix(field, "-"), ":")
			if inTree(source) {
				out = append(out, name+" "+source)
			}
		}
	}
	for _, line := range strings.Split(props["EnvironmentFiles"], "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 && inTree(strings.TrimPrefix(fields[0], "-")) {
			out = append(out, "EnvironmentFiles "+fields[0])
		}
	}
	return out
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
	if paths := sitePathsOnHost(props); len(paths) > 0 {
		return refuse(ProofSitePath, "%s: systemd reads %s on the host; a link there would be followed as root", in.Unit, strings.Join(paths, ", "))
	}
	if got, ok := props["ControlGroup"]; !ok || got != expected["ControlGroup"].Value {
		return refuse(ProofCgroup, "%s runs in %q, not %s", in.Unit, got, expected["ControlGroup"].Value)
	}
	for _, name := range names {
		want := expected[name]
		got, ok := props[name]
		if !ok {
			return refuse(ProofProperty, "%s: systemd did not show %s", in.Unit, name)
		}
		if want.Contains {
			if !strings.Contains(got, want.Value) {
				return refuse(ProofProperty, "%s: %s does not include %s", in.Unit, name, want.Value)
			}
			continue
		}
		if got != want.Value {
			return refuse(ProofProperty, "%s: %s is %q, expected %q", in.Unit, name, got, want.Value)
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
