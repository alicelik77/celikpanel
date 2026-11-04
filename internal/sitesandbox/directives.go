package sitesandbox

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/alicelik/celikpanel/internal/hostingpath"
)

// InaccessiblePaths are hidden from every sandboxed program: the Agent's
// socket, every PHP-FPM socket (other sites' pools), CelikPanel's
// configuration, state and logs, root's home, the administrator's
// toolchains and the mail spool (with it the sendmail drop). The "-" prefix
// skips a path that does not exist on this system.
var InaccessiblePaths = []string{
	"/run/celikpanel",
	"/run/php",
	"/run/php-fpm",
	"/etc/celikpanel",
	"/var/lib/celikpanel",
	"/var/lib/celikpanel-agent",
	"/var/lib/celikpanel-agent-private",
	"/var/log/celikpanel",
	"/root",
	"/opt/celikpanel/toolchains",
	"/var/spool/postfix",
}

// DeniedAddresses are never reachable from a sandboxed program: link-local
// (the cloud metadata service at 169.254.169.254) and the metadata
// addresses some providers put elsewhere.
var DeniedAddresses = []string{
	"169.254.0.0/16",
	"fe80::/10",
	"fd00:ec2::254/128",
	"100.100.100.200/32",
}

// SyscallDeny is subtracted from @system-service: the plan's groups and the
// kernel keyring (escape matrix item 15; @system-service includes @keyring)
// and reading or writing another process's memory (item 5).
const SyscallDeny = "~@debug @mount @raw-io @reboot @swap @module @cpu-emulation @obsolete @keyring process_vm_readv process_vm_writev"

// directive is one line and the conditions it needs.
type directive struct {
	line string
	// min is the oldest systemd that knows the directive (or this value).
	min int
	// bpf: enforced only with systemd's BPF framework; left out without it
	// (it would be accepted and silently not enforced).
	bpf bool
}

// DirectiveMinimums is the table the gating follows, for the documentation
// and the tests: directive name -> the oldest systemd that has it. A
// directive missing here is older than every supported system (249).
var DirectiveMinimums = map[string]int{
	"ProtectProc":                 247,
	"ProcSubset":                  247,
	"PrivateIPC":                  248,
	"ExecPaths":                   248,
	"NoExecPaths":                 248,
	"SocketBindAllow":             249,
	"SocketBindDeny":              249,
	"PrivatePIDs":                 257,
	"ProtectControlGroups=strict": 257,
}

func (p Profile) ports() []int {
	if p.Port == 0 {
		return nil
	}
	out := append([]int{p.Port}, p.ExtraPorts...)
	sort.Ints(out)
	return out
}

func withOptional(paths []string) string {
	parts := make([]string, len(paths))
	for i, p := range paths {
		parts[i] = "-" + p
	}
	return strings.Join(parts, " ")
}

// Directives are the [Service] lines of a sandboxed unit (or the
// --property values of a transient one) for this profile on a host with
// these capabilities. A directive the host's systemd does not have is never
// written; a BPF directive is left out where it would not be enforced (the
// Assessment names that gap). The profile is validated first: an invalid
// profile renders nothing and returns the reason.
func Directives(p Profile, c Caps) ([]string, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	nofile := p.Limits.NOFILE
	if nofile == 0 {
		nofile = 8192
	}
	list := []directive{
		{line: "Slice=system.slice"},
		{line: "NoNewPrivileges=yes"},
		{line: "ProtectSystem=strict"},
		{line: "ProtectHome=yes"},
		{line: "PrivateTmp=yes"},
		{line: "PrivateDevices=yes"},
		{line: "ProtectKernelTunables=yes"},
		{line: "ProtectKernelModules=yes"},
		{line: "ProtectKernelLogs=yes"},
		{line: "ProtectClock=yes"},
		{line: "ProtectHostname=yes"},
		{line: "ProtectProc=invisible", min: 247},
		{line: "PrivateIPC=yes", min: 248},
		{line: "RestrictSUIDSGID=yes"},
		{line: "RestrictRealtime=yes"},
		{line: "RestrictNamespaces=yes"},
		{line: "LockPersonality=yes"},
		{line: "RemoveIPC=yes"},
		{line: "CapabilityBoundingSet="},
		{line: "AmbientCapabilities="},
		{line: "SystemCallArchitectures=native"},
		{line: "SystemCallFilter=@system-service"},
		{line: "SystemCallFilter=" + SyscallDeny},
		{line: "SystemCallErrorNumber=EPERM"},
		{line: "RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX AF_NETLINK"},
		{line: "UMask=0077"},
		// Every other site and subscription disappears: an empty read-only
		// hosting tree with only this program's folder bound into it. The
		// source is the helper's root-owned mount point, never the folder's
		// name in the site home: systemd resolves a source as root and
		// follows links (helper.go). The destination is created by systemd
		// inside the empty tmpfs, so nothing of the customer's lies on its
		// way. A BindPaths= mount is writable; no ReadWritePaths= is needed
		// (systemd drops one at the same path as a duplicate of the bind).
		{line: "TemporaryFileSystem=" + hostingpath.HostingBase() + ":ro"},
		{line: "BindPaths=" + BindSource(p.Unit) + ":" + p.AppDir},
		{line: "InaccessiblePaths=" + withOptional(InaccessiblePaths)},
		{line: "IPAddressDeny=" + strings.Join(DeniedAddresses, " "), bpf: true},
		{line: "LogRateLimitIntervalSec=30s"},
		{line: "LogRateLimitBurst=1000"},
		{line: "MemoryMax=" + strconv.Itoa(p.Limits.MemoryMB) + "M"},
		{line: "MemorySwapMax=0"},
		{line: "CPUQuota=" + strconv.Itoa(p.Limits.CPUPercent) + "%"},
		{line: "TasksMax=" + strconv.Itoa(p.Limits.Tasks)},
		{line: "LimitNOFILE=" + strconv.Itoa(nofile)},
		{line: "LimitCORE=0"},
	}
	if len(p.ReadOnlyPaths) > 0 {
		list = append(list, directive{line: "ReadOnlyPaths=" + strings.Join(p.ReadOnlyPaths, " ")})
	}
	// /proc shows only processes: the service apps (Node.js reads
	// /proc/cpuinfo and /proc/meminfo for os.cpus()) keep alpha.118's view
	// until their own measurement; every other kind gets the subset.
	if p.Kind != KindServiceApp {
		list = append(list, directive{line: "ProcSubset=pid", min: 247})
	}
	// Binding: only the program's own loopback ports; port 0, any other
	// port and every UDP port are refused by the kernel.
	for _, port := range p.ports() {
		list = append(list, directive{line: "SocketBindAllow=tcp:" + strconv.Itoa(port), min: 249, bpf: true})
	}
	list = append(list, directive{line: "SocketBindDeny=any", min: 249, bpf: true})
	if p.Kind == KindProgram {
		// Defence in depth, not the boundary: an uploaded program cannot
		// map writable memory executable, and executes only from its own
		// folder and the shared libraries (so /bin/sh, python3 and
		// "ld.so /usr/bin/python3" fail).
		list = append(list,
			directive{line: "MemoryDenyWriteExecute=yes"},
			directive{line: "NoExecPaths=/", min: 248},
			directive{line: "ExecPaths=" + p.AppDir + " " + withOptional([]string{"/usr/lib", "/usr/lib64", "/lib", "/lib64"}), min: 248},
		)
	}
	if c.SystemdVersion >= 257 {
		list = append(list,
			directive{line: "PrivatePIDs=yes", min: 257},
			directive{line: "ProtectControlGroups=strict", min: 257},
		)
	} else {
		list = append(list, directive{line: "ProtectControlGroups=yes"})
	}
	out := make([]string, 0, len(list))
	for _, d := range list {
		if d.min > 0 && c.SystemdVersion < d.min {
			continue
		}
		if d.bpf && !c.bpfEnforced() {
			continue
		}
		if err := checkLine(d.line); err != nil {
			return nil, err
		}
		out = append(out, d.line)
	}
	return out, nil
}

// checkLine refuses a rendered line systemd would expand: the only "%" is
// CPUQuota's own unit and there is never a "$".
func checkLine(line string) error {
	name, value, ok := strings.Cut(line, "=")
	if !ok || name == "" {
		return fmt.Errorf("malformed sandbox directive %q", line)
	}
	if strings.ContainsAny(line, "$\\\"'\n\r\t") {
		return fmt.Errorf("the sandbox directive %s has characters systemd would expand", name)
	}
	if strings.Contains(value, "%") && !(name == "CPUQuota" && strings.Count(value, "%") == 1 && strings.HasSuffix(value, "%")) {
		return fmt.Errorf("the sandbox directive %s has a specifier character", name)
	}
	return nil
}

// UnitDirectives are the [Unit] lines: a program that keeps crashing stops
// being restarted after five starts in five minutes.
func UnitDirectives() []string {
	return []string{"StartLimitIntervalSec=300", "StartLimitBurst=5"}
}

// Permit refuses a profile the host cannot hold at its accepted level: a
// unit accepted at "full" is not written on a host that assesses reduced.
func Permit(p Profile, c Caps) error {
	a := Assess(c)
	if a.Level == LevelReduced && p.Level != LevelReduced {
		gaps := make([]string, len(a.Gaps))
		for i, g := range a.Gaps {
			gaps[i] = string(g)
		}
		return fmt.Errorf("sandbox_reduced: this server does not enforce the full sandbox (%s)", strings.Join(gaps, ", "))
	}
	if c.SystemdVersion == 0 {
		return errors.New("sandbox_unknown: the systemd version of this server could not be read")
	}
	return nil
}
