//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

type platformServerReboot struct{}

// acquireIdle takes the host mutation lock with the ledger and package
// manager proved idle under it (acquireSystemUpdateServiceMutationIdleLock,
// the proof a panel update's start makes), then refuses while a CelikPanel
// update is queued or running: its worker lets the lock go while the
// installer runs.
func (platformServerReboot) acquireIdle() (func(), string, string) {
	lock, err := acquireSystemUpdateServiceMutationIdleLock()
	if err != nil {
		outcome, detail := classifyServerRebootRefusal(err)
		return nil, outcome, detail
	}
	release := func() { _ = lock.Close() }
	active, err := serverRebootUpdateActive()
	if err != nil {
		release()
		return nil, transport.ServerRebootUnavailable, "the CelikPanel update state could not be read"
	}
	if active {
		release()
		return nil, transport.ServerRebootBusyUpdate, "a CelikPanel update is queued or running"
	}
	return release, transport.ServerRebootScheduled, ""
}

func classifyServerRebootRefusal(err error) (string, string) {
	if reason, ok := serviceMutationReadinessReason(err); ok {
		switch reason {
		case transport.HostMutationReasonPackageManager:
			return transport.ServerRebootBusyPackageManager, "the package manager is running"
		case transport.HostMutationReasonAgentMutation, transport.HostMutationReasonHostLock:
			return transport.ServerRebootBusyMutation, "a server change is running"
		}
	}
	if errors.Is(err, errServiceMutationHostBusy) {
		return transport.ServerRebootBusyMutation, "a server change is running"
	}
	text := err.Error()
	if strings.Contains(text, "is active") || strings.Contains(text, "is not idle") {
		return transport.ServerRebootBusyMutation, "a server change is running"
	}
	return transport.ServerRebootUnavailable, "the server's change state could not be proved idle"
}

// serverRebootUpdateActive reads the update attempts' records (read only:
// no lock file or directory is made). A record that cannot be read counts
// as not proved idle.
func serverRebootUpdateActive() (bool, error) {
	service, err := agentSystemUpdateService()
	if err != nil || service == nil {
		return false, fmt.Errorf("system update service: %v", err)
	}
	backend, ok := service.backend.(*linuxSystemUpdateBackend)
	if !ok || backend == nil {
		return false, errors.New("system update backend is not the Linux one")
	}
	exists, err := systemUpdateRootExistsForReconcile(backend.stateRoot)
	if err != nil {
		return false, err
	}
	if !exists {
		return false, nil
	}
	entries, err := os.ReadDir(backend.stateRoot)
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") || !strings.HasSuffix(name, ".json") {
			continue
		}
		state, err := readSystemUpdateState(backend.stateRoot, strings.TrimSuffix(name, ".json"))
		if err != nil {
			return false, err
		}
		if state.active() {
			return true, nil
		}
	}
	return false, nil
}

// reboot runs the verified systemctl with fixed arguments. systemctl only
// queues the reboot job and returns.
func (platformServerReboot) reboot() error {
	systemctl, err := resolveVerifiedSystemctlExecutable()
	if err != nil {
		return fmt.Errorf("resolve verified systemctl: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, systemctl, "reboot").CombinedOutput()
	if err != nil {
		text := strings.TrimSpace(string(out))
		if len(text) > 300 {
			text = text[:300]
		}
		return fmt.Errorf("%v: %s", err, text)
	}
	return nil
}
