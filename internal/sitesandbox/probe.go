package sitesandbox

import (
	"context"
	"os"
	"os/exec"
	"strings"
)

// Runner runs the probe's read-only commands. The Agent and the escape-test
// kit use HostRunner; tests replace it.
type Runner interface {
	// Run runs name with args, stdin as its input, and returns its
	// combined output.
	Run(ctx context.Context, stdin, name string, args ...string) ([]byte, error)
	// Exists reports whether a path exists.
	Exists(path string) bool
}

// HostRunner runs commands on this host.
type HostRunner struct{}

func (HostRunner) Run(ctx context.Context, stdin, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	return cmd.CombinedOutput()
}

func (HostRunner) Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// CgroupControllers exists only on the unified (v2) hierarchy.
const CgroupControllers = "/sys/fs/cgroup/cgroup.controllers"

// Probe reads what this host can enforce. It changes nothing: systemd is
// asked for its version, the cgroup mount is looked at, and nftables checks
// the egress table with --check, which applies nothing.
func Probe(ctx context.Context, r Runner) Caps {
	var c Caps
	if out, err := r.Run(ctx, "", "systemctl", "--version"); err == nil {
		c.SystemdVersion, c.BPFFramework = ParseSystemctlVersion(string(out))
	}
	c.CgroupV2 = r.Exists(CgroupControllers)
	if r.Exists(NFT) {
		if _, err := r.Run(ctx, EgressRuleset(), NFT, "--check", "-f", "-"); err == nil {
			c.NFTSocketCgroup = true
		}
	}
	return c
}
