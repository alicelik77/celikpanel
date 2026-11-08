package main

import (
	"context"
	"time"

	"github.com/alicelik/celikpanel/internal/dbremote"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Remote database access at panel start (alpha.130 lab, release recovery).
//
// A forced failing update went to release recovery, whose rollback asks the
// Agent whether service mutations are idle. At that moment the new panel's
// start-up apply held a database_remote lease (the in-memory digest is empty
// after a restart, so it applied again), and a refused apply sent a second
// one (D5): the rollback was refused once and the services stayed down about
// a minute longer. Now:
//
//   - start reads the Agent's status only; when its record carries exactly
//     the wanted set (dbremote.RequestDigest), the panel takes it as applied
//     and sends nothing; with no rule and nothing in the filter, nothing at all;
//   - the loop takes no lease during the first databaseRemoteStartupQuiet
//     after start, nor while the server is busy (a panel operation, an Agent
//     mutation, the host lock, the package manager, an update just admitted);
//   - what must close meanwhile — an ended rule, a withdrawn permission or
//     offer, a suspension — closes at the address filter through the
//     lease-free narrowing, so security does not wait for the window;
//     opening and the engines' own rules (pg_hba lines, MariaDB accounts)
//     follow after it.
//
// Panel açılışında kiralama alınmaz; kapanması gereken süzgeçte kiralamasız
// kapanır, açma ve motor değişiklikleri sessiz bir sunucuyu bekler.

// databaseRemoteStartupQuiet is how long after start the loop takes no
// lease: longer than a release update's health and rollback window.
var databaseRemoteStartupQuiet = 3 * time.Minute

// databaseRemoteHostQuiet reads (advisory, no lease) whether the server is
// quiet; swapped by tests.
var databaseRemoteHostQuiet = func(p *Panel, ctx context.Context) bool {
	if p == nil || p.agentClient == nil || p.panelUpdateRecentlyAdmitted(time.Now()) {
		return false
	}
	return p.readHostMutationReadinessLogged(ctx, func(string, ...any) {}).Ready
}

// databaseRemoteMayMutate: the loop may take a service-mutation lease now.
func (p *Panel) databaseRemoteMayMutate(ctx context.Context, startedAt time.Time) bool {
	if databaseRemoteNow().Sub(startedAt) < databaseRemoteStartupQuiet {
		return false
	}
	return databaseRemoteHostQuiet(p, ctx)
}

// seedDatabaseRemoteFromStatus reads the Agent's status (read only) and takes
// its record as the last apply: the rules in force (only those still wanted,
// so the panel firewall never opens a port for a rule that left), the
// filter's ports and whether it is loaded. When the record's digest is the
// wanted set's, or nothing is wanted and nothing is filtered, the loop has
// nothing to send.
func (p *Panel) seedDatabaseRemoteFromStatus(ctx context.Context) bool {
	if p == nil || p.db == nil {
		return false
	}
	desired, err := p.databaseRemoteDesired(ctx)
	if err != nil {
		return false
	}
	skip, err := p.databaseRemoteMariaDBSkipNameResolve(ctx)
	if err != nil {
		return false
	}
	offered, err := p.databaseRemoteOffered(ctx)
	if err != nil {
		return false
	}
	readCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	status, err := databaseRemoteStatusFromAgent(p, readCtx, transport.DatabaseRemoteStatusRequest{Targets: desired})
	if err != nil || status.Error != "" {
		return false
	}
	wanted := map[string]bool{}
	for _, t := range desired {
		for _, r := range t.Rules {
			wanted[dbremote.TargetKey(t.Engine, t.Port)+"|"+r.Database+"|"+r.User+"|"+r.Address] = true
		}
	}
	for i, t := range status.Targets {
		kept := t.Active[:0:0]
		for _, r := range t.Active {
			if wanted[dbremote.TargetKey(t.Engine, t.Port)+"|"+r.Database+"|"+r.User+"|"+r.Address] {
				kept = append(kept, r)
			}
		}
		status.Targets[i].Active = kept
	}
	targets := make([]dbremote.Target, 0, len(desired))
	for _, t := range desired {
		targets = append(targets, dbremote.Target{Engine: t.Engine, Port: t.Port, Rules: fromTransportRemoteRules(t.Rules)})
	}
	same := (len(desired) == 0 && len(status.FilterPorts) == 0) ||
		(status.AppliedDigest != "" && status.AppliedDigest == dbremote.RequestDigestOffered(targets, skip, offered))
	m := databaseRemoteMemory
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.appliedAt.IsZero() {
		return false // an apply already answered in this process
	}
	m.answer, m.appliedAt, m.lastError = status, databaseRemoteNow(), ""
	if same {
		m.digest = databaseRemoteDigest(desired)
	}
	return same
}

// closeDatabaseRemoteWithoutLease runs while the loop may not take a lease:
// when the wanted set differs from what was last applied, the filter is
// narrowed to it (lease-free; it never opens anything).
func (p *Panel) closeDatabaseRemoteWithoutLease(ctx context.Context) {
	desired, err := p.databaseRemoteDesired(ctx)
	if err != nil {
		return
	}
	m := databaseRemoteMemory
	m.mu.Lock()
	same := m.digest != "" && m.digest == databaseRemoteDigest(desired)
	nothingFiltered := !m.appliedAt.IsZero() && len(m.answer.FilterPorts) == 0
	m.mu.Unlock()
	if same || nothingFiltered {
		return
	}
	p.narrowDatabaseRemote(ctx, desired)
}
