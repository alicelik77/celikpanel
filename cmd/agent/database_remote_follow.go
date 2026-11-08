package main

import (
	"context"
	"errors"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alicelik/celikpanel/internal/dbremote"
)

// A PostgreSQL cluster that changes port keeps CelikPanel's record
// (alpha.131, docs/DATABASE-REMOTE-ACCESS.md "alpha.131").
//
// The record (dbremote.StatePath) is keyed by port. In alpha.130 a cluster
// moved from 5433 to 5432 with the panel's "Change port" left its record on
// 5433: on 5432 CelikPanel's own listen_addresses and auth_delay looked like
// the owner's, so every rule was refused (OWNER_LISTEN, NO_SLOWDOWN), and the
// filter kept dropping 5433, where nothing listens any more. Now:
//
//   - the port change itself moves the record and the cluster's filter port
//     (databaseRemoteFollowPGPort), and the panel asks for an apply;
//   - every apply first heals a record whose port has lost its cluster
//     (healDatabaseRemotePGPorts): it moves to the port where that cluster
//     runs now, found by its data directory (recorded since alpha.131), or,
//     for a record written by alpha.129/130, by CelikPanel's own settings
//     in that cluster's postgresql.auto.conf, and only when exactly one
//     cluster matches. Anything ambiguous stays as it was: closed.
//
// Port değişen PostgreSQL kümesi CelikPanel kaydını yanında götürür; kayıt
// veri klasörüyle (eski kayıtta CelikPanel'in kendi ayarlarıyla) bulunur.

// databaseRemotePGIdentity answers the data directory of the PostgreSQL on
// port: answered is false when nothing answers there; dir is "" when the
// answer is not a usable path. Swapped by tests.
var databaseRemotePGIdentity = func(port int) (dir string, answered bool) {
	out, err := databaseRemotePGQuery(port, "SELECT current_setting('data_directory');")
	if err != nil {
		return "", false
	}
	dir = strings.TrimSpace(out)
	if !dbremote.ValidDataDir(dir) {
		return "", true
	}
	return filepath.Clean(dir), true
}

// databaseRemotePGClusterPorts answers the ports of the clusters
// pg_lsclusters lists, running or not (Debian, Ubuntu); known is false where
// there is no such list. Swapped by tests.
var databaseRemotePGClusterPorts = func() (ports map[int]bool, known bool) {
	host := pgClusterHostActive()
	if host.family == nil || host.family() != "apt" || host.lsclusters == nil {
		return nil, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), pgReadBound)
	defer cancel()
	out, err := host.lsclusters(ctx)
	if err != nil {
		return nil, false
	}
	ports = map[int]bool{}
	for _, c := range parsePGLsClusters(out) {
		ports[c.Port] = true
	}
	return ports, true
}

// pgPortMove is one record that followed its cluster.
type pgPortMove struct{ from, to int }

// healDatabaseRemotePGPorts moves the record of every PostgreSQL cluster that
// no longer runs on its recorded port to the port it runs on now (see
// above), and records the data directory of every cluster that answers on
// its port. persist: an apply, which also removes CelikPanel's Fail2ban jail
// of the old port; a status read only looks (and treats the moved record as
// having nothing in force, because the filter does not hold the new port
// yet).
func healDatabaseRemotePGPorts(state *dbremote.State, named []dbremote.Target, persist bool) []pgPortMove {
	var keys []string
	for key, ts := range state.Targets {
		if ts.Engine == dbremote.EnginePostgreSQL {
			keys = append(keys, key)
		}
	}
	if len(keys) == 0 {
		return nil
	}
	sort.Strings(keys)
	type identity struct {
		dir      string
		answered bool
	}
	seen := map[int]identity{}
	id := func(port int) identity {
		if known, ok := seen[port]; ok {
			return known
		}
		dir, answered := databaseRemotePGIdentity(port)
		seen[port] = identity{dir, answered}
		return seen[port]
	}
	var listed map[int]bool
	listedKnown, listedRead := false, false
	clusterPorts := func() (map[int]bool, bool) {
		if !listedRead {
			listed, listedKnown = databaseRemotePGClusterPorts()
			listedRead = true
		}
		return listed, listedKnown
	}
	var moves []pgPortMove
	for _, key := range keys {
		ts, ok := state.Targets[key]
		if !ok {
			continue
		}
		here := id(ts.Port)
		if here.answered && (ts.DataDir == "" || here.dir == "" || here.dir == ts.DataDir) {
			if ts.DataDir == "" && here.dir != "" {
				ts.DataDir = here.dir
				state.Targets[key] = ts
			}
			continue
		}
		legacy := ts.DataDir == ""
		if legacy {
			if here.answered {
				continue // some cluster answers there; without a data directory it is taken as the same
			}
			if ports, known := clusterPorts(); known && ports[ts.Port] {
				continue // its cluster is only stopped
			}
		}
		candidates := map[int]bool{}
		for _, t := range named {
			if t.Engine == dbremote.EnginePostgreSQL {
				candidates[t.Port] = true
			}
		}
		if ports, known := clusterPorts(); known {
			for port := range ports {
				candidates[port] = true
			}
		}
		var matches []int
		for port := range candidates {
			if port == ts.Port {
				continue
			}
			if other, taken := state.Targets[dbremote.TargetKey(dbremote.EnginePostgreSQL, port)]; taken && !other.Empty() {
				continue
			}
			c := id(port)
			if !c.answered {
				continue
			}
			if legacy {
				if !pgCarriesOwnSettings(port, ts) {
					continue
				}
			} else if c.dir != ts.DataDir {
				continue
			}
			matches = append(matches, port)
		}
		if len(matches) != 1 {
			if len(matches) > 1 {
				log.Printf("database remote access: the cluster recorded on port %d may run on ports %v; CelikPanel cannot tell which, so its record stays and nothing opens there", ts.Port, matches)
			}
			continue
		}
		from, to := ts.Port, matches[0]
		if !dbremote.MovePostgreSQLTarget(state, from, to) {
			continue
		}
		toKey := dbremote.TargetKey(dbremote.EnginePostgreSQL, to)
		moved := state.Targets[toKey]
		if dir := id(to).dir; dir != "" {
			moved.DataDir = dir
		}
		if persist {
			if moved.PGJail {
				removeOwnPGJail(from)
				moved.PGJail = false
			}
			log.Printf("database remote access: the PostgreSQL cluster recorded on port %d runs on port %d now; CelikPanel's record follows it", from, to)
		} else {
			moved.Active, moved.Addresses = nil, nil
		}
		state.Targets[toKey] = moved
		moves = append(moves, pgPortMove{from, to})
	}
	return moves
}

// pgCarriesOwnSettings: the cluster on port carries, in postgresql.auto.conf,
// exactly the settings ts says CelikPanel set (a record of alpha.129/130,
// which has no data directory). At least one of them must be recorded.
func pgCarriesOwnSettings(port int, ts dbremote.TargetState) bool {
	if !ts.Listen && !ts.AuthDelay {
		return false
	}
	s, err := pgRemoteInstance{}.read(port)
	if err != nil {
		return false
	}
	if ts.Listen && !(strings.HasSuffix(s.listenFile, "postgresql.auto.conf") && strings.Trim(strings.TrimSpace(s.listenFileValue), "'\"") == "*") {
		return false
	}
	if ts.AuthDelay && strings.Trim(strings.TrimSpace(s.preloadFile), "'\"") != "auth_delay" {
		return false
	}
	return true
}

// removeOwnPGJail removes CelikPanel's Fail2ban jail of a port no cluster
// runs on any more (no leftovers); an edited jail is the owner's and stays.
func removeOwnPGJail(port int) {
	path := dbremote.PGJailPath(port)
	current, err := databaseRemoteFiles.read(path)
	if err != nil || !pgOwnJailText(port, string(current)) {
		return
	}
	if databaseRemoteFiles.remove(path) == nil && fail2banInstalled() {
		_ = databaseRemoteFail2ban.reload()
	}
}

// databaseRemoteFollowPGPort runs after a proved port change of a cluster
// (SetPostgreSQLClusterPortV1): CelikPanel's record and the cluster's
// address filter move from the old port to the new one at once, so neither
// the old port keeps a rule nor the new one is unfiltered until the panel's
// apply (which it asks for right after) makes everything else follow.
func databaseRemoteFollowPGPort(from, to int) {
	databaseRemoteMu.Lock()
	defer databaseRemoteMu.Unlock()
	state, err := loadDatabaseRemoteState()
	if err != nil {
		log.Printf("database remote access: the record could not be read after the port change %d->%d: %v", from, to, err)
		return
	}
	if !dbremote.MovePostgreSQLTarget(&state, from, to) {
		return
	}
	toKey := dbremote.TargetKey(dbremote.EnginePostgreSQL, to)
	moved := state.Targets[toKey]
	if dir, answered := databaseRemotePGIdentity(to); answered && dir != "" {
		moved.DataDir = dir
	}
	if moved.PGJail {
		removeOwnPGJail(from)
		moved.PGJail = false
	}
	if err := followDatabaseRemoteFilterPort(state, from, to, moved); err != nil {
		// The apply the panel asks for writes the filter; until then the
		// record claims nothing in force on the new port.
		log.Printf("database remote access: the address filter could not follow the port change %d->%d: %v", from, to, err)
		moved.Active, moved.Addresses = nil, nil
	}
	state.Targets[toKey] = moved
	if err := saveDatabaseRemoteState(state); err != nil {
		log.Printf("database remote access: the record could not be saved after the port change %d->%d: %v", from, to, err)
	}
}

// followDatabaseRemoteFilterPort rewrites CelikPanel's filter file with the
// cluster's addresses on its new port and without its old port.
func followDatabaseRemoteFilterPort(state dbremote.State, from, to int, moved dbremote.TargetState) error {
	nftPath, err := databaseRemoteLookPath("nft")
	if err != nil {
		return nil // no filter without nft
	}
	current, err := os.ReadFile(databaseRemoteFilterFiles.nftPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil // nothing was filtered
	}
	if err != nil {
		return err
	}
	byPort := map[int]dbremote.TargetState{}
	for _, ts := range state.Targets {
		byPort[ts.Port] = ts
	}
	byPort[to] = moved
	entry := func(port int, addresses []string) dbremote.NFTPort {
		e := dbremote.NFTPort{Port: port}
		for _, p := range listedPrefixes(addresses) {
			if p.Addr().Is4() {
				e.V4 = append(e.V4, p)
			} else {
				e.V6 = append(e.V6, p)
			}
		}
		return e
	}
	var ports []dbremote.NFTPort
	for _, port := range nftFilePorts(string(current)) {
		if port == from || port == to {
			continue
		}
		ports = append(ports, entry(port, byPort[port].Addresses))
	}
	if len(moved.Addresses) > 0 || moved.Listen || moved.ListenPending {
		ports = append(ports, entry(to, moved.Addresses))
	}
	if len(ports) == 0 {
		return nil // the apply removes the table
	}
	text, err := dbremote.NFTFile(ports)
	if err != nil {
		return err
	}
	return loadNarrowedDatabaseRemoteFilter(nftPath, text)
}
