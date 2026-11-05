package main

import (
	"errors"
	"fmt"
	"io"
)

// The Agent's command line and its single running instance (alpha.124).
//
// Frankfurt, 2026-11-05: `/opt/celikpanel/bin/agent --version` run over SSH
// did not print a version. The Agent ignored the unknown argument and started
// a second full agent, which removed and re-created /run/celikpanel/agent.sock
// as root:root 0660 (the systemd-started agent had made it root:celikpanel).
// The panel (user celikpanel) was refused the socket for about 40 minutes
// until the owner killed the second process and restarted the unit.
//
// Now:
//   - --version (also -version, version) prints the version and exits before
//     anything else; any other argument the Agent does not know is a usage
//     error (exit 2) before any manager, socket or background task starts;
//   - the running Agent holds an exclusive lock for its lifetime; a second
//     one exits with a plain message before it touches anything, so it never
//     removes or re-creates the running Agent's socket;
//   - the socket's group is the group of its directory (/run/celikpanel,
//     celikpanel), whether systemd or a person started the Agent.
//
// Invariant: native service ownership and recovery (D-025): one Agent owns
// the socket and the host; the panel's path to it never depends on how a
// process was started.
//
// Agent bilinmeyen bağımsız değişkeni reddeder, --version yalnız sürümü yazar,
// ikinci bir agent çalışan agent'ın soketine dokunmadan çıkar.

// agentUsage is the usage text of the daemon mode.
const agentUsage = "Usage: agent            run the CelikPanel agent (started by systemd: sudo systemctl start celikpanel-agent)\n" +
	"       agent --version  print the version and exit\n" +
	"Other modes are internal and started by CelikPanel itself."

// agentVersionArgument reports whether args ask only for the version.
func agentVersionArgument(args []string) bool {
	return len(args) == 1 && (args[0] == "--version" || args[0] == "-version" || args[0] == "version")
}

// printAgentVersion prints the version line.
func printAgentVersion(out io.Writer) {
	_, _ = fmt.Fprintf(out, "CelikPanel agent %s (commit %s)\n", buildVersion, buildCommit)
}

// validateAgentDaemonArguments: the daemon mode takes no argument at all.
// Every mode that takes arguments has returned before this is asked.
func validateAgentDaemonArguments(args []string) error {
	if len(args) == 0 {
		return nil
	}
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h" || args[0] == "help") {
		return errAgentHelp
	}
	return fmt.Errorf("unknown argument %q: nothing was started or changed", args[0])
}

var errAgentHelp = errors.New("help")

// agentInstanceLockName is the lock file next to the socket.
const agentInstanceLockName = "agent.instance.lock"

// agentAlreadyRunningMessage is what a second Agent says before it exits.
func agentAlreadyRunningMessage(lockPath string) string {
	return "Another CelikPanel agent is already running on this server (it holds " + lockPath + "). " +
		"This one exits without starting anything or touching its connection. " +
		"To see the running agent: sudo systemctl status celikpanel-agent. " +
		"To restart it: sudo systemctl restart celikpanel-agent. " +
		"For the version, run: /opt/celikpanel/bin/agent --version"
}
