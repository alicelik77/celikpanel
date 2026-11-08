package main

import (
	"errors"
	"fmt"
	"net/netip"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/alicelik/celikpanel/internal/dbremote"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Closing at the filter while the full apply cannot run (alpha.130, D5).
//
// The panel sends a host-changing request only once it knows the host's
// identity, which the Agent reads only after systemd finished starting. When
// a unit hangs at start-up (the alpha.129 lab: the engine's own unit stuck
// in "starting"), every apply is refused, so a removed address or a
// withdrawn permission stayed open. This request only takes addresses out
// of the address filter (table celikpanel_dbremote): it never adds an
// address or a port, never removes a port's drop rule, runs no engine
// client and no package or service manager — only nft on CelikPanel's own
// file — so it is safe without the host identity. The engines' own rules
// (pg_hba lines, MariaDB accounts) follow at the next full apply; until then
// they are unreachable from the removed address, because the filter drops
// its packets before any engine sees them.
//
// Sunucu açılışı bitmeden tam uygulama reddedilince süzgeçten yalnız adres
// çıkarılır; hiçbir şey eklenmez.

// NarrowDatabaseRemoteFilterV1 narrows the filter to the addresses of the
// request's rules.
func (a *Agent) NarrowDatabaseRemoteFilterV1(req *transport.DatabaseRemoteNarrowRequest, resp *transport.DatabaseRemoteResponse) error {
	*resp = transport.DatabaseRemoteResponse{}
	if req == nil {
		resp.ErrorCode, resp.Error = transport.DatabaseRemoteFailed, "a narrowing request is required"
		return nil
	}
	targets, err := normalizeDBRemoteRequest(req.Targets)
	if err != nil {
		resp.ErrorCode, resp.Error = transport.DatabaseRemoteFailed, err.Error()+"; nothing was changed"
		return nil
	}
	_, finishStep, err := a.requiredServiceMutationStep(
		req.ServiceMutationBinding,
		newServiceMutationStepClaim(serviceMutationStepNarrowDatabaseRemote, databaseRemoteMutationTarget, "", "narrow"),
	)
	if err != nil {
		resp.ErrorCode, resp.Error = transport.DatabaseRemoteFailed, "closing at the filter is blocked: "+err.Error()
		return nil
	}
	defer finishStep()
	databaseRemoteMu.Lock()
	defer databaseRemoteMu.Unlock()
	narrowDatabaseRemoteFilter(targets, resp)
	return nil
}

// nftFilePorts reads the ports of CelikPanel's filter file (each has one
// drop rule).
func nftFilePorts(text string) []int {
	var ports []int
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 4 && fields[0] == "tcp" && fields[1] == "dport" && fields[3] == "drop" {
			if port, err := strconv.Atoi(fields[2]); err == nil && port > 0 && port < 65536 {
				ports = append(ports, port)
			}
		}
	}
	sort.Ints(ports)
	return ports
}

func narrowDatabaseRemoteFilter(targets []dbremote.Target, resp *transport.DatabaseRemoteResponse) {
	nftPath, nftErr := databaseRemoteLookPath("nft")
	resp.NFTAvailable = nftErr == nil
	if nftErr != nil {
		return // no filter can be loaded without nft: nothing to narrow
	}
	state, err := loadDatabaseRemoteState()
	if err != nil {
		resp.ErrorCode, resp.Error = transport.DatabaseRemoteRecordUnreadable, err.Error()+"; nothing was changed"
		return
	}
	current, err := os.ReadFile(databaseRemoteFilterFiles.nftPath)
	if errors.Is(err, os.ErrNotExist) {
		resp.FilterLoaded = databaseRemoteFilterLoaded(nftPath)
		return
	}
	if err != nil {
		resp.ErrorCode, resp.Error = transport.DatabaseRemoteFirewallUnchanged, "the filter file could not be read: "+err.Error()
		return
	}
	allowed := map[string]map[netip.Prefix]bool{}
	for _, t := range targets {
		set := map[netip.Prefix]bool{}
		v4, v6 := t.Addresses()
		for _, p := range append(v4, v6...) {
			set[p] = true
		}
		allowed[t.Key()] = set
	}
	byPort := map[int]string{}
	for key, ts := range state.Targets {
		byPort[ts.Port] = key
	}
	var ports []dbremote.NFTPort
	changed := false
	for _, port := range nftFilePorts(string(current)) {
		entry := dbremote.NFTPort{Port: port}
		key, known := byPort[port]
		if known {
			ts := state.Targets[key]
			var kept []string
			narrowed := false
			for _, p := range listedPrefixes(ts.Addresses) {
				if !allowed[key][p] {
					changed, narrowed = true, true
					continue
				}
				kept = append(kept, p.String())
				if p.Addr().Is4() {
					entry.V4 = append(entry.V4, p)
				} else {
					entry.V6 = append(entry.V6, p)
				}
			}
			ts.Addresses = kept
			state.Targets[key] = ts
			if narrowed {
				// Named in the answer: closed at the filter, the engine's
				// own rules follow at the next full apply.
				resp.Targets = append(resp.Targets, transport.DatabaseRemoteTargetResult{Engine: ts.Engine, Port: ts.Port,
					State: transport.DatabaseRemoteStateClosing, Unit: dbRemoteUnit(dbremote.Target{Engine: ts.Engine, Port: ts.Port})})
			}
		}
		ports = append(ports, entry)
	}
	for _, p := range ports {
		resp.FilterPorts = append(resp.FilterPorts, p.Port)
	}
	if !changed {
		resp.FilterLoaded = databaseRemoteFilterLoaded(nftPath)
		return
	}
	text, err := dbremote.NFTFile(ports)
	if err == nil && text == "" {
		err = errors.New("the narrowed filter would be empty")
	}
	if err == nil {
		err = loadNarrowedDatabaseRemoteFilter(nftPath, text)
	}
	if err != nil {
		resp.Targets = nil
		resp.ErrorCode, resp.Error = transport.DatabaseRemoteFirewallUnchanged, "the address filter could not be narrowed ("+err.Error()+"); it still allows the earlier addresses"
		resp.FilterLoaded = databaseRemoteFilterLoaded(nftPath)
		return
	}
	resp.FilterLoaded = databaseRemoteFilterLoaded(nftPath)
	if err := saveDatabaseRemoteState(state); err != nil {
		resp.ErrorCode = transport.DatabaseRemoteFailed
		resp.Error = "the filter was narrowed but CelikPanel's record of it could not be saved (" + err.Error() + ")"
	}
}

// loadNarrowedDatabaseRemoteFilter checks, writes and loads the filter file
// under the guard's lock. Units and timer stay as they are.
func loadNarrowedDatabaseRemoteFilter(nftPath, text string) error {
	unlock, err := databaseRemoteLockFilter()
	if err != nil {
		return fmt.Errorf("the filter's lock could not be taken: %w", err)
	}
	defer unlock()
	if out, err := databaseRemoteRun(nftPath, []string{"--check", "-f", "-"}, text); err != nil {
		return fmt.Errorf("nft refused the filter: %s", firstLine(string(out)))
	}
	if err := secureWriteConfig(databaseRemoteFilterFiles.nftPath, []byte(text), 0o600); err != nil {
		return err
	}
	if out, err := databaseRemoteRun(nftPath, []string{"-f", databaseRemoteFilterFiles.nftPath}, ""); err != nil {
		return fmt.Errorf("nft did not load the filter: %s", firstLine(string(out)))
	}
	return nil
}
