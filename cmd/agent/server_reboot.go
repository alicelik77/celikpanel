package main

import (
	"errors"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Server → Maintenance → Restart the server (alpha.128, G29). The
// administrator asked for it in the panel, after the panel's own checks; the
// Agent decides again here, authoritatively:
//
//  1. the panel and the Agent are the same build;
//  2. the host mutation lock is taken and the privileged-change ledger and
//     the package manager are proved idle under it (the same proof a panel
//     update's start makes), and no CelikPanel update is queued or running;
//  3. the answer is returned, and a few seconds later `systemctl reboot`
//     runs with fixed arguments. The lock is kept until the machine goes
//     down, so no change can start in between; if the reboot does not come
//     within serverRebootLockHold, the lock is let go and the journal says so.
//
// Nothing is ever powered off. One reboot per Agent run: a second request
// is answered "already scheduled" and schedules nothing.
//
// Sunucuyu yeniden başlat: Agent kendi denetimini yineler, ana kilidi alır,
// yanıtı verir ve birkaç saniye sonra sabit argümanlı `systemctl reboot`
// çalıştırır. Kapatma asla yapılmaz.

const (
	serverRebootDelay    = 3 * time.Second
	serverRebootLockHold = 5 * time.Minute
)

// serverRebootHost is the platform's part, replaced by tests.
type serverRebootHost interface {
	// acquireIdle takes the host mutation lock and proves the host idle; the
	// returned release lets the lock go.
	acquireIdle() (release func(), outcome string, detail string)
	// reboot runs `systemctl reboot`.
	reboot() error
}

var (
	serverRebootPlatform serverRebootHost = platformServerReboot{}
	serverRebootSleep                     = time.Sleep
	serverRebootMu       sync.Mutex
	serverRebootPending  bool
)

// ServerRebootV1 schedules the operating system's reboot. See above.
func (a *Agent) ServerRebootV1(req *transport.ServerRebootRequest, resp *transport.ServerRebootResponse) error {
	if req == nil || resp == nil {
		return errors.New("server reboot request and response are required")
	}
	*resp = transport.ServerRebootResponse{}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "restarting the server"); err != nil {
		resp.Outcome = transport.ServerRebootUnavailable
		resp.Error = err.Error()
		return nil
	}
	serverRebootMu.Lock()
	defer serverRebootMu.Unlock()
	if serverRebootPending {
		resp.Outcome = transport.ServerRebootAlreadyScheduled
		return nil
	}
	release, outcome, detail := serverRebootPlatform.acquireIdle()
	if outcome != transport.ServerRebootScheduled {
		resp.Outcome, resp.Detail = outcome, detail
		return nil
	}
	serverRebootPending = true
	requestID := sanitizedRebootRequestID(req.RequestID)
	log.Printf("server reboot: requested by the panel (request %s); systemctl reboot in %s", requestID, serverRebootDelay)
	go runScheduledServerReboot(release, requestID)
	resp.Outcome = transport.ServerRebootScheduled
	resp.DelaySeconds = int(serverRebootDelay / time.Second)
	return nil
}

func runScheduledServerReboot(release func(), requestID string) {
	serverRebootSleep(serverRebootDelay)
	if err := serverRebootPlatform.reboot(); err != nil {
		log.Printf("server reboot: systemctl reboot failed (request %s): %v; nothing else was changed", requestID, err)
		release()
		serverRebootMu.Lock()
		serverRebootPending = false
		serverRebootMu.Unlock()
		return
	}
	// The machine is going down; keep the lock until it does. Should it not
	// (a stuck unit stop), let changes start again after a while.
	serverRebootSleep(serverRebootLockHold)
	log.Printf("server reboot: the server has not gone down %s after systemctl reboot (request %s); the change lock is released", serverRebootLockHold, requestID)
	release()
	serverRebootMu.Lock()
	serverRebootPending = false
	serverRebootMu.Unlock()
}

// sanitizedRebootRequestID keeps the journal line plain.
func sanitizedRebootRequestID(id string) string {
	id = strings.TrimSpace(id)
	if len(id) > 64 {
		id = id[:64]
	}
	for _, c := range id {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
			return "unnamed"
		}
	}
	if id == "" {
		return "unnamed"
	}
	return id
}
