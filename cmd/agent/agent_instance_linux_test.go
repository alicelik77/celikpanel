//go:build linux

package main

import (
	"bytes"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// Frankfurt, 2026-11-05: `agent --version` started a second full agent.
func TestAgentCommandLineAnswersVersionAndRefusesUnknownArguments(t *testing.T) {
	for _, args := range [][]string{{"--version"}, {"-version"}, {"version"}} {
		if !agentVersionArgument(args) {
			t.Fatalf("%v is not the version", args)
		}
	}
	var out bytes.Buffer
	printAgentVersion(&out)
	if !strings.HasPrefix(out.String(), "CelikPanel agent ") {
		t.Fatalf("version line %q", out.String())
	}
	if err := validateAgentDaemonArguments(nil); err != nil {
		t.Fatalf("the daemon refused no arguments: %v", err)
	}
	for _, args := range [][]string{{"--verison"}, {"status"}, {"--version", "x"}, {"-v"}} {
		err := validateAgentDaemonArguments(args)
		if err == nil || errors.Is(err, errAgentHelp) || !strings.Contains(err.Error(), "nothing was started") {
			t.Fatalf("%v: %v", args, err)
		}
	}
	if !errors.Is(validateAgentDaemonArguments([]string{"--help"}), errAgentHelp) {
		t.Fatal("--help is not the usage")
	}
}

// A second Agent never gets past the instance lock, so it never removes or
// re-creates the running Agent's socket; once the first ends, a new one starts.
func TestAgentInstanceLockAdmitsOneAgent(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "run")
	first, path, err := acquireAgentInstanceLock(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := acquireAgentInstanceLock(dir); !errors.Is(err, errAgentAlreadyRunning) {
		t.Fatalf("a second agent was admitted: %v", err)
	}
	if message := agentAlreadyRunningMessage(path); !strings.Contains(message, "sudo systemctl restart celikpanel-agent") || !strings.Contains(message, path) {
		t.Fatalf("message %q", message)
	}
	first.Close()
	again, _, err := acquireAgentInstanceLock(dir)
	if err != nil {
		t.Fatalf("the lock outlived its agent: %v", err)
	}
	again.Close()
}

// The socket's group is its directory's group, however the Agent started.
func TestAgentSocketTakesItsDirectoryGroup(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("needs root to change a group")
	}
	dir := filepath.Join(t.TempDir(), "run")
	if err := os.Mkdir(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	const group = 54321
	if err := os.Chown(dir, 0, group); err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(dir, "agent.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if err := alignAgentSocketGroup(socket); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(socket)
	if err != nil {
		t.Fatal(err)
	}
	if gid := info.Sys().(*syscall.Stat_t).Gid; gid != group {
		t.Fatalf("socket group %d, want %d", gid, group)
	}
}
