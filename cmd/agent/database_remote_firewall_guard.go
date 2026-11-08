package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/alicelik/celikpanel/internal/firewallpolicy"
)

// The panel firewall after `nft flush ruleset` (alpha.130, D6).
//
// A flush removes every table: the panel firewall's (celikpanel_fw) and the
// database address filter's (celikpanel_dbremote). The filter's own guard
// timer loads it again within 30 seconds; the panel firewall came back only
// at the next boot (its restore unit runs before the network). This guard
// in the running Agent puts the panel firewall back within seconds, from the
// same persisted policy and by the same rules as that boot restore:
//
//   - only when a persisted policy exists (the administrator chose "Save for
//     reboot"); a firewall that was only turned on live has no record to
//     restore from and is left to the administrator, as before;
//   - never against the owner (D-022): when the firewall's boot restore unit
//     is disabled or masked, or nftables.service is masked, the owner turned
//     the firewall off outside CelikPanel; the guard changes nothing and the
//     status says so;
//   - the database filter first: its file is loaded again before the panel
//     table, and the database ports are left out of the restored panel table
//     until that filter is proven loaded, so they never open to every
//     address; the panel then syncs its live policy (PanelFirewallRestores).
//
// Panel güvenlik duvarı `nft flush ruleset` sonrası: kalıcı politika varsa ve
// sahip kapatmadıysa saniyeler içinde geri yüklenir; veritabanı portları süzgeç
// kanıtlanana dek kapalı kalır.

const panelFirewallGuardInterval = 15 * time.Second

var panelFirewallGuard struct {
	mu       sync.Mutex
	restores int
	note     string
	logged   string
}

// panelFirewallGuardReport answers how often the guard restored the table and
// why it last did not (or failed to).
func panelFirewallGuardReport() (int, string) {
	panelFirewallGuard.mu.Lock()
	defer panelFirewallGuard.mu.Unlock()
	return panelFirewallGuard.restores, panelFirewallGuard.note
}

func panelFirewallGuardNote(note string) {
	panelFirewallGuard.mu.Lock()
	defer panelFirewallGuard.mu.Unlock()
	panelFirewallGuard.note = note
	if note != "" && note != panelFirewallGuard.logged {
		log.Printf("panel firewall guard: %s", note)
	}
	panelFirewallGuard.logged = note
}

// startPanelFirewallFlushGuard runs the check every 15 seconds for the life
// of the Agent, after a first pause that leaves the start-up recovery alone.
func startPanelFirewallFlushGuard() {
	go func() {
		time.Sleep(2 * panelFirewallGuardInterval)
		ticker := time.NewTicker(panelFirewallGuardInterval)
		defer ticker.Stop()
		for {
			panelFirewallFlushCheck(hostFirewallCommandRunner{}, fileFirewallStateStore{path: firewallSnapshotPath})
			<-ticker.C
		}
	}()
}

// panelFirewallUnitState reads `systemctl is-enabled unit` ("enabled",
// "disabled", "masked", ...; "" when it cannot be read).
func panelFirewallUnitState(runner firewallCommandRunner, unit string) string {
	out, _ := runner.Output("systemctl", "is-enabled", unit)
	return strings.TrimSpace(firstLine(string(out)))
}

// panelFirewallFlushCheck restores the panel table once when it vanished.
// It reports whether it restored it.
func panelFirewallFlushCheck(runner firewallCommandRunner, store firewallStateStore) bool {
	firewallMu.Lock()
	defer firewallMu.Unlock()
	if _, err := runner.LookPath("nft"); err != nil {
		return false
	}
	snapshot, exists, err := store.Load()
	if err != nil || !exists {
		// No persisted policy: nothing CelikPanel may put back by itself.
		panelFirewallGuardNote("")
		return false
	}
	if _, _, err := firewallpolicy.Decode(snapshot); err != nil {
		return false
	}
	tables, err := runner.Output("nft", "list", "tables")
	if err != nil || firewallTablePresent(tables) {
		if err == nil {
			panelFirewallGuardNote("")
		}
		return false
	}
	// The table is gone. Did the owner turn the firewall off outside
	// CelikPanel?
	switch state := panelFirewallUnitState(runner, firewallRestoreUnitName); state {
	case "enabled":
	default:
		panelFirewallGuardNote("the panel firewall's table is gone and its boot restore (" + firewallRestoreUnitName + ") is " +
			unitStateText(state) + " outside CelikPanel, so it is left off; turn the firewall on again in the panel to use it")
		return false
	}
	if panelFirewallUnitState(runner, "nftables.service") == "masked" {
		panelFirewallGuardNote("the panel firewall's table is gone and nftables.service is masked outside CelikPanel, so it is left off; unmask nftables and turn the firewall on again in the panel to use it")
		return false
	}
	lock, err := runner.AcquireFirewallLock()
	if err != nil {
		return false
	}
	defer lock.Close()

	// The database filter first; its ports stay out until it is proven.
	exclude := map[int]bool{}
	if !restoreDatabaseRemoteFilterForGuard() {
		for _, port := range databaseRemoteRecordedPorts() {
			exclude[port] = true
		}
	}
	batch, _, err := prepareFirewallRestoreBatch(runner, store)
	if err == nil && len(exclude) > 0 {
		batch, err = rulesetWithoutTCPPorts(batch, exclude)
	}
	if err == nil {
		if out, runErr := runner.CombinedOutput("nft", []string{"-f", "-"}, batch); runErr != nil {
			err = errors.New(commandFailureDetail("nft restore failed", out, runErr))
		}
	}
	if err != nil {
		panelFirewallGuardNote("the panel firewall's table is gone (nft flush ruleset?) and could not be put back: " + err.Error())
		return false
	}
	panelFirewallGuard.mu.Lock()
	panelFirewallGuard.restores++
	panelFirewallGuard.mu.Unlock()
	panelFirewallGuardNote("")
	log.Printf("panel firewall guard: the panel firewall's table was gone (nft flush ruleset?); it was put back from the saved policy")
	return true
}

func unitStateText(s string) string {
	if s == "" {
		return "not readable"
	}
	return s
}

// rulesetWithoutTCPPorts renders the restore ruleset again without the
// given TCP ports (firewallpolicy.Ruleset's fixed form).
func rulesetWithoutTCPPorts(batch string, exclude map[int]bool) (string, error) {
	var tcp, udp []int
	replace := false
	for _, raw := range strings.Split(batch, "\n") {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "delete table inet "+fwTable) {
			replace = true
		}
		if ports, ok := parsePortLine(line, "tcp dport"); ok {
			tcp = ports
		}
		if ports, ok := parsePortLine(line, "udp dport"); ok {
			udp = ports
		}
	}
	kept := tcp[:0:0]
	for _, port := range tcp {
		if !exclude[port] {
			kept = append(kept, port)
		}
	}
	return firewallpolicy.Ruleset(replace, kept, udp), nil
}

// databaseRemoteRecordedPorts: every port the remote access record names.
func databaseRemoteRecordedPorts() []int {
	databaseRemoteMu.Lock()
	defer databaseRemoteMu.Unlock()
	state, err := loadDatabaseRemoteState()
	if err != nil {
		// Unreadable: no database port is trusted.
		return []int{3306, 5432}
	}
	var ports []int
	for _, ts := range state.Targets {
		ports = append(ports, ts.Port)
	}
	return ports
}

// restoreDatabaseRemoteFilterForGuard loads the database filter's file again
// when its chain is missing (the guard script's step, done here first) and
// answers whether the filter is proven loaded. No file: nothing to prove
// (no database port is open through the panel firewall then either).
func restoreDatabaseRemoteFilterForGuard() bool {
	nftPath, err := databaseRemoteLookPath("nft")
	if err != nil {
		return false
	}
	if _, err := os.Stat(databaseRemoteFilterFiles.nftPath); errors.Is(err, os.ErrNotExist) {
		return true
	}
	if databaseRemoteFilterLoaded(nftPath) {
		return true
	}
	unlock, err := databaseRemoteLockFilter()
	if err != nil {
		return false
	}
	defer unlock()
	if out, err := databaseRemoteRun(nftPath, []string{"-f", databaseRemoteFilterFiles.nftPath}, ""); err != nil {
		log.Printf("panel firewall guard: %v", fmt.Errorf("the database filter could not be loaded again: %s", firstLine(string(out))))
		return false
	}
	return databaseRemoteFilterLoaded(nftPath)
}
