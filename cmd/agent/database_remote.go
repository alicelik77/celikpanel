package main

import (
	"errors"
	"fmt"
	"log"
	"net/netip"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"

	"github.com/alicelik/celikpanel/internal/dbremote"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Remote access to databases (alpha.129, docs/DATABASE-REMOTE-ACCESS.md).
//
// The panel sends the complete rule set of every engine instance; this file
// makes the native configuration match it, in an order that never opens more
// than the rules allow:
//
//  1. read every instance (nothing changes yet) and decide which ones
//     CelikPanel may manage: an engine whose listen address the owner set
//     is left alone (D-022);
//  2. the nftables address filter (table celikpanel_dbremote, its file and
//     boot unit): listed addresses only, before anything can listen;
//  3. per instance: the engine's own rules (pg_hba block; MariaDB accounts),
//     TLS, then the listen address (which waits for a restart, the
//     administrator's step), and the slowing of wrong passwords;
//  4. the filter again when an instance finished closing.
//
// Closing runs the same way: the filter loses the addresses first, then the
// engine's rules go, and the port stays dropped until the engine runs on the
// local address again. The record of what CelikPanel changed
// (dbremote.StatePath) is saved after every instance, so only CelikPanel's own
// changes are ever undone.
//
// Uzak veritabanı erişimi: panel her motorun tüm kurallarını gönderir; bu dosya
// yerel yapılandırmayı, kuralların izin verdiğinden fazlasını hiç açmayan bir
// sırayla ona uydurur.

var databaseRemoteMu sync.Mutex

// Swapped by tests.
var (
	databaseRemoteStatePath = dbremote.StatePath
	databaseRemoteLookPath  = exec.LookPath
)

func loadDatabaseRemoteState() (dbremote.State, error) {
	data, err := os.ReadFile(databaseRemoteStatePath)
	if errors.Is(err, os.ErrNotExist) {
		return dbremote.DecodeState(nil)
	}
	if err != nil {
		return dbremote.State{}, err
	}
	return dbremote.DecodeState(data)
}

func saveDatabaseRemoteState(state dbremote.State) error {
	data, err := dbremote.EncodeState(state)
	if err != nil {
		return err
	}
	if len(state.Targets) == 0 {
		if err := os.Remove(databaseRemoteStatePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(dirOf(databaseRemoteStatePath), 0o755); err != nil {
		return err
	}
	return secureWriteConfig(databaseRemoteStatePath, data, 0o600)
}

func dirOf(path string) string {
	if i := strings.LastIndex(path, "/"); i > 0 {
		return path[:i]
	}
	return "/"
}

func toDBRemoteTarget(t transport.DatabaseRemoteTarget) dbremote.Target {
	out := dbremote.Target{Engine: t.Engine, Port: t.Port}
	for _, rule := range t.Rules {
		out.Rules = append(out.Rules, dbremote.Rule{Database: rule.Database, User: rule.User, Address: rule.Address})
	}
	return out
}

func toTransportRules(rules []dbremote.Rule) []transport.DatabaseRemoteRule {
	out := make([]transport.DatabaseRemoteRule, 0, len(rules))
	for _, rule := range rules {
		out = append(out, transport.DatabaseRemoteRule{Database: rule.Database, User: rule.User, Address: rule.Address})
	}
	return out
}

func ruleProblem(code string, rule dbremote.Rule, detail string) transport.DatabaseRemoteProblem {
	return transport.DatabaseRemoteProblem{Code: code, Detail: detail, User: rule.User, Database: rule.Database, Address: rule.Address}
}

// dbRemoteInstance is what one engine instance does with its rules.
type dbRemoteInstance interface {
	// inspect reads the instance; it changes nothing.
	inspect(target dbremote.Target, state dbremote.TargetState) dbRemoteFacts
	// apply makes the engine's rules, TLS, listen address and slowing match
	// target, recording what it changed in state.
	apply(target dbremote.Target, facts dbRemoteFacts, state *dbremote.TargetState, hostName string) transport.DatabaseRemoteTargetResult
	// readiness adds to result what is still missing before a first rule
	// could be in force (alpha.130); it changes nothing.
	readiness(target dbremote.Target, state dbremote.TargetState, result *transport.DatabaseRemoteTargetResult)
}

// dbRemoteFacts is one instance as it was read.
type dbRemoteFacts struct {
	running      bool
	ownerManaged bool
	ownerDetail  string
	listenRemote bool
	readError    string
}

func dbRemoteInstanceFor(engine string) dbRemoteInstance {
	if engine == dbremote.EnginePostgreSQL {
		return pgRemoteInstance{}
	}
	return mariadbRemoteInstance{}
}

// DatabaseRemoteStatusV1 describes the named instances. It reads only: no
// file, account or filter changes, and nothing is retried (D-024).
func (a *Agent) DatabaseRemoteStatusV1(req *transport.DatabaseRemoteStatusRequest, resp *transport.DatabaseRemoteResponse) error {
	*resp = transport.DatabaseRemoteResponse{}
	if req == nil {
		resp.ErrorCode, resp.Error = transport.DatabaseRemoteFailed, "a status request is required"
		return nil
	}
	databaseRemoteMu.Lock()
	defer databaseRemoteMu.Unlock()
	nftPath, nftErr := databaseRemoteLookPath("nft")
	resp.NFTAvailable = nftErr == nil
	state, err := loadDatabaseRemoteState()
	if err != nil {
		resp.ErrorCode, resp.Error = transport.DatabaseRemoteRecordUnreadable, err.Error()
		return nil
	}
	resp.FilterLoaded = nftErr == nil && databaseRemoteFilterLoaded(nftPath)
	for _, raw := range req.Targets {
		target := dbremote.Target{Engine: raw.Engine, Port: raw.Port}
		if _, err := target.Normalize(); err != nil {
			continue
		}
		ts := state.Targets[target.Key()]
		instance := dbRemoteInstanceFor(target.Engine)
		facts := instance.inspect(target, ts)
		result := describeDBRemoteTarget(target, facts, ts)
		if facts.running {
			instance.readiness(target, ts, &result)
		}
		if nftErr != nil {
			result.Readiness = append(result.Readiness, readinessProblem(transport.DatabaseRemoteNFTMissing, "nftables is not installed"))
		}
		resp.Targets = append(resp.Targets, result)
	}
	resp.PanelFirewallRestores, resp.PanelFirewallNote = panelFirewallGuardReport()
	for _, ts := range state.Targets {
		if len(ts.Addresses) > 0 || ts.Listen || ts.ListenPending {
			resp.FilterPorts = append(resp.FilterPorts, ts.Port)
		}
	}
	sort.Ints(resp.FilterPorts)
	return nil
}

// describeDBRemoteTarget is the status answer of one instance from what was
// read and recorded.
func describeDBRemoteTarget(target dbremote.Target, facts dbRemoteFacts, ts dbremote.TargetState) transport.DatabaseRemoteTargetResult {
	result := transport.DatabaseRemoteTargetResult{Engine: target.Engine, Port: target.Port, Unit: dbRemoteUnit(target), ListenRemote: facts.listenRemote, TLS: "off", SlowDown: "none"}
	switch {
	case !facts.running:
		result.State = transport.DatabaseRemoteStateDown
		result.Problems = append(result.Problems, transport.DatabaseRemoteProblem{Code: transport.DatabaseRemoteEngineDown, Detail: facts.readError})
	case facts.ownerManaged:
		result.State = transport.DatabaseRemoteStateOwnerManaged
		result.Problems = append(result.Problems, transport.DatabaseRemoteProblem{Code: transport.DatabaseRemoteOwnerListen, Detail: facts.ownerDetail})
	case ts.Listen && !facts.listenRemote:
		result.State = transport.DatabaseRemoteStateWaitRestart
	case ts.Listen:
		result.State = transport.DatabaseRemoteStateOpen
	case facts.listenRemote && ts.ListenPending:
		result.State = transport.DatabaseRemoteStateClosing
	default:
		result.State = transport.DatabaseRemoteStateOff
	}
	return result
}

func dbRemoteUnit(target dbremote.Target) string {
	if target.Engine == dbremote.EngineMySQL {
		return "mariadb"
	}
	if cluster, ok := pgClusterForPort(target.Port); ok {
		return "postgresql@" + cluster
	}
	return "postgresql"
}

// ApplyDatabaseRemoteV1 makes every instance match the panel's rules.
func (a *Agent) ApplyDatabaseRemoteV1(req *transport.DatabaseRemoteApplyRequest, resp *transport.DatabaseRemoteResponse) error {
	*resp = transport.DatabaseRemoteResponse{}
	if req == nil {
		resp.ErrorCode, resp.Error = transport.DatabaseRemoteFailed, "a remote access request is required"
		return nil
	}
	targets, err := normalizeDBRemoteRequest(req.Targets)
	if err != nil {
		resp.ErrorCode, resp.Error = transport.DatabaseRemoteFailed, err.Error()+"; nothing was changed"
		return nil
	}
	_, finishStep, err := a.requiredServiceMutationStep(
		req.ServiceMutationBinding,
		newServiceMutationStepClaim(serviceMutationStepApplyDatabaseRemote, databaseRemoteMutationTarget, "", "apply"),
	)
	if err != nil {
		resp.ErrorCode, resp.Error = transport.DatabaseRemoteFailed, "remote access change is blocked: "+err.Error()
		return nil
	}
	defer finishStep()
	a.configMu.Lock()
	defer a.configMu.Unlock()
	databaseRemoteMu.Lock()
	defer databaseRemoteMu.Unlock()
	databaseRemoteMariaDBSkipNameResolve = req.MariaDBSkipNameResolve
	applyDatabaseRemote(targets, req.HostName, a.databaseRemoteReload(), resp)
	return nil
}

// databaseRemoteMutationTarget is the one job target of every remote access
// change; the panel serialises them.
const databaseRemoteMutationTarget = "database-remote"

// normalizeDBRemoteRequest checks every target and refuses a repeated one.
func normalizeDBRemoteRequest(raw []transport.DatabaseRemoteTarget) ([]dbremote.Target, error) {
	seen := map[string]bool{}
	out := make([]dbremote.Target, 0, len(raw))
	for _, t := range raw {
		norm, err := toDBRemoteTarget(t).Normalize()
		if err != nil {
			return nil, err
		}
		if seen[norm.Key()] {
			return nil, fmt.Errorf("%s is named twice", norm.Key())
		}
		seen[norm.Key()] = true
		out = append(out, norm)
	}
	return out, nil
}

// databaseRemoteReload is the PostgreSQL unit reload the validated pg_hba
// writer uses (the same as the configuration editor's).
func (a *Agent) databaseRemoteReload() func(string) error {
	if a.configReload != nil {
		return a.configReload
	}
	if a.systemdMgr == nil {
		return func(string) error { return errors.New("systemd manager unavailable") }
	}
	return a.systemdMgr.Reload
}

var databaseRemoteHBAReload func(string) error

func applyDatabaseRemote(targets []dbremote.Target, hostName string, reload func(string) error, resp *transport.DatabaseRemoteResponse) {
	databaseRemoteHBAReload = reload
	if err := healDatabaseRemoteCertRoot(); err != nil {
		log.Printf("database remote access: the certificate folder %s could not be made readable for the engines: %v", databaseRemoteCertRoot, err)
	}
	nftPath, nftErr := databaseRemoteLookPath("nft")
	resp.NFTAvailable = nftErr == nil
	state, err := loadDatabaseRemoteState()
	if err != nil {
		resp.ErrorCode, resp.Error = transport.DatabaseRemoteRecordUnreadable, err.Error()+"; nothing was changed"
		return
	}
	// Targets CelikPanel changed before and the panel did not name are
	// closed.
	named := map[string]bool{}
	for _, t := range targets {
		named[t.Key()] = true
	}
	for key, ts := range state.Targets {
		if !named[key] {
			targets = append(targets, dbremote.Target{Engine: ts.Engine, Port: ts.Port})
		}
	}
	sort.Slice(targets, func(i, j int) bool { return targets[i].Key() < targets[j].Key() })
	anyRules := false
	for _, t := range targets {
		anyRules = anyRules || len(t.Rules) > 0
	}
	if anyRules && nftErr != nil {
		resp.ErrorCode = transport.DatabaseRemoteNFTMissing
		resp.Error = "nftables is not installed, so the database port could not be limited to the listed addresses; nothing was changed. Install the firewall (nftables) in Components first"
		// Closing never needs the filter: targets without rules still close.
		closing := targets[:0:0]
		for _, t := range targets {
			if len(t.Rules) == 0 {
				closing = append(closing, t)
			}
		}
		targets = closing
	}

	// 1. Read every instance.
	facts := make([]dbRemoteFacts, len(targets))
	for i, t := range targets {
		facts[i] = dbRemoteInstanceFor(t.Engine).inspect(t, state.Targets[t.Key()])
	}

	// 2. The filter: listed addresses of every instance CelikPanel manages,
	// plus a drop for an instance that may still listen beyond local.
	filter := func(active bool) error {
		if nftErr != nil {
			return nil
		}
		ports := dbRemoteFilterPorts(targets, facts, state, active)
		if err := writeDatabaseRemoteFilter(nftPath, ports); err != nil {
			return err
		}
		// Only ports the filter really holds are named (review H1): the
		// panel opens nothing else.
		resp.FilterPorts = resp.FilterPorts[:0]
		for _, p := range ports {
			resp.FilterPorts = append(resp.FilterPorts, p.Port)
		}
		resp.FilterLoaded = len(ports) > 0 // writeDatabaseRemoteFilter verified it
		return nil
	}
	if err := filter(false); err != nil {
		resp.ErrorCode = transport.DatabaseRemoteFirewallUnchanged
		resp.Error = "the address filter of the database ports could not be written (" + err.Error() + "); nothing else was changed"
		return
	}

	// 3. Each instance.
	for i, t := range targets {
		ts := state.Targets[t.Key()]
		ts.Engine, ts.Port = t.Engine, t.Port
		var result transport.DatabaseRemoteTargetResult
		switch {
		case !facts[i].running:
			result = describeDBRemoteTarget(t, facts[i], ts)
			if len(t.Rules) > 0 {
				result.State = transport.DatabaseRemoteStateDown
			}
		case facts[i].ownerManaged && !ts.Listen:
			result = describeDBRemoteTarget(t, facts[i], ts)
		default:
			result = dbRemoteInstanceFor(t.Engine).apply(t, facts[i], &ts, hostName)
		}
		// The filter holds exactly the addresses of the rules in force.
		ts.Addresses = nil
		if len(t.Rules) > 0 && !facts[i].ownerManaged {
			v4, v6 := dbremote.Target{Engine: t.Engine, Port: t.Port, Rules: fromTransportRules(result.Active)}.Addresses()
			for _, p := range append(v4, v6...) {
				ts.Addresses = append(ts.Addresses, p.String())
			}
		}
		state.Targets[t.Key()] = ts
		if err := saveDatabaseRemoteState(state); err != nil {
			log.Printf("database remote access: the record could not be saved: %v", err)
			result.Problems = append(result.Problems, transport.DatabaseRemoteProblem{Code: transport.DatabaseRemoteFailed,
				Detail: "the change was made but CelikPanel's record of it could not be saved (" + err.Error() + ")"})
		}
		resp.Targets = append(resp.Targets, result)
	}

	// 4. The filter again: rules refused at the engine and instances that
	// finished closing leave it.
	if err := filter(true); err != nil {
		resp.ErrorCode = transport.DatabaseRemoteFirewallUnchanged
		resp.Error = "the engines were changed but the address filter could not be updated afterwards (" + err.Error() + "); it still allows only addresses that were listed"
	}
}

func fromTransportRules(rules []transport.DatabaseRemoteRule) []dbremote.Rule {
	out := make([]dbremote.Rule, 0, len(rules))
	for _, r := range rules {
		out = append(out, dbremote.Rule{Database: r.Database, User: r.User, Address: r.Address})
	}
	return out
}

// dbRemoteFilterPorts: for every instance CelikPanel manages, the addresses
// the filter allows. Before the instances' own steps (active false) these are
// the requested addresses: an address in the filter whose rule is not yet at
// the engine reaches nothing, because the engine refuses it. Afterwards
// (active true) they are the addresses of the rules in force, so a rule the
// engine refused leaves the filter too. An instance that may listen beyond
// the local address without rules keeps a drop for its port. An instance
// whose listen address the owner manages is not touched.
func dbRemoteFilterPorts(targets []dbremote.Target, facts []dbRemoteFacts, state dbremote.State, active bool) []dbremote.NFTPort {
	var ports []dbremote.NFTPort
	for i, t := range targets {
		ts := state.Targets[t.Key()]
		if facts[i].ownerManaged && !ts.Listen {
			continue
		}
		var v4, v6 []netip.Prefix
		if active {
			for _, p := range listedPrefixes(ts.Addresses) {
				if p.Addr().Is4() {
					v4 = append(v4, p)
				} else {
					v6 = append(v6, p)
				}
			}
		} else {
			v4, v6 = t.Addresses()
		}
		mayListen := ts.Listen || ts.ListenPending || facts[i].listenRemote
		if len(v4)+len(v6) == 0 && !mayListen {
			continue
		}
		ports = append(ports, dbremote.NFTPort{Port: t.Port, V4: v4, V6: v6})
	}
	return ports
}

// listedPrefixes parses canonical addresses; a bad one is skipped.
func listedPrefixes(list []string) []netip.Prefix {
	out := make([]netip.Prefix, 0, len(list))
	for _, s := range list {
		if p, err := netip.ParsePrefix(s); err == nil {
			out = append(out, p)
		}
	}
	return out
}
