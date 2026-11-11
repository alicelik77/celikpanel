package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/alicelik/celikpanel/internal/dbremote"
	"github.com/alicelik/celikpanel/internal/hostname"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Remote access to databases (alpha.129, docs/DATABASE-REMOTE-ACCESS.md):
// the panel side. The rows of database_remote_rules are the wish; the Agent
// makes the server match the eligible ones (ApplyDatabaseRemoteV1). A rule
// is eligible when the server offer (admin_offered_permissions) is on, the
// database lives on this server's engine, its subscription is active and
// holds the permission, and its user still has access to its database. The
// whole eligible set is sent every time, after every change, at start and
// every five minutes when it differs from what was last applied, so a plan
// change, a suspension or a revoked grant closes the rule within minutes.
//
// Veritabanlarına dışarıdan erişimin panel tarafı: satırlar istektir; Agent
// sunucuyu uygun olanlara uydurur. Uygun küme her değişiklikten sonra,
// açılışta ve beş dakikada bir (farklıysa) gönderilir.

// The loop probes the filter every minute (read only) and applies the rule
// set every fifth minute when it changed (or waits for a prerequisite).
const (
	databaseRemoteProbeInterval = time.Minute
	databaseRemoteApplyEvery    = 5
)

// databaseRemoteState is the panel's memory of the last apply: what was sent
// and what the Agent answered, for the screens. Losing it (a restart) only
// means the next read says "not applied yet" until the start-up apply ran.
type databaseRemoteState struct {
	mu        sync.Mutex
	applyMu   sync.Mutex
	digest    string
	answer    transport.DatabaseRemoteResponse
	appliedAt time.Time
	lastError string
	kick      chan struct{}
	// closePending (alpha.130, D5): the full apply did not run, but the
	// address filter was narrowed to the wanted addresses at closedAt; the
	// engines' own rules follow at the next full apply.
	closePending bool
	closedAt     time.Time
	// callFailed: the last full apply did not reach the Agent or was
	// refused before it ran (D5: tried again every minute).
	callFailed bool
	// narrowDigest: the wanted set the filter was last narrowed to; the
	// same set is not sent again while the full apply keeps failing.
	narrowDigest string
	// firewallStale: a narrowing changed the ports with rules in force; the
	// next full apply syncs the panel firewall.
	firewallStale bool
	// firewallRestores is the Agent's count of panel firewall restores
	// after a flush (D6), firewallNote why it was not restored.
	firewallRestores      int
	firewallRestoresKnown bool
	firewallNote          string
}

var databaseRemoteMemory = &databaseRemoteState{kick: make(chan struct{}, 1)}

// databaseRemoteNarrowThroughAgent closes at the address filter (D5). It
// takes no durable job and no service-mutation lease (alpha.130 lab: a lease
// held at panel start made a release recovery refuse its rollback): the Agent
// only takes addresses out of CelikPanel's own nft table (swapped by tests).
var databaseRemoteNarrowThroughAgent = func(p *Panel, ctx context.Context, req transport.DatabaseRemoteNarrowRequest) (transport.DatabaseRemoteResponse, error) {
	var response transport.DatabaseRemoteResponse
	err := p.callAgentContext(ctx, "Agent.NarrowDatabaseRemoteFilterV1", &req, &response)
	return response, err
}

// databaseRemoteApplyThroughAgent sends the rule set in its own durable job
// (swapped by tests).
var databaseRemoteApplyThroughAgent = func(p *Panel, ctx context.Context, req transport.DatabaseRemoteApplyRequest) (transport.DatabaseRemoteResponse, error) {
	var response transport.DatabaseRemoteResponse
	err := p.withStandaloneAgentMutation(ctx, "database_remote", "database-remote", "", func(callCtx context.Context, binding agentMutationBinding) error {
		req.ServiceMutationBinding = binding
		return p.callAgentContext(callCtx, "Agent.ApplyDatabaseRemoteV1", &req, &response)
	})
	return response, err
}

// databaseRemoteStatusFromAgent reads instances (swapped by tests).
var databaseRemoteStatusFromAgent = func(p *Panel, ctx context.Context, req transport.DatabaseRemoteStatusRequest) (transport.DatabaseRemoteResponse, error) {
	callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var response transport.DatabaseRemoteResponse
	err := p.callAgentContext(callCtx, "Agent.DatabaseRemoteStatusV1", &req, &response)
	return response, err
}

// databaseRemoteOffered: the administrator's server switch.
func (p *Panel) databaseRemoteOffered(ctx context.Context) (bool, error) {
	offered, err := p.loadOfferedOptInPermissions(ctx)
	if err != nil {
		return false, err
	}
	return offered[rightDatabaseRemote], nil
}

// databaseRemoteRow is one rule with what decides its eligibility.
type databaseRemoteRow struct {
	ID             int
	DatabaseID     int
	Database       string
	UserID         int
	User           string
	Address        string
	Engine         string
	Port           int
	Host           string
	SubscriptionID int
	Status         string
	Granted        bool
	// ExpiresAt (alpha.130): UTC "YYYY-MM-DD HH:MM:SS", "" = no end.
	ExpiresAt string
}

const databaseRemoteRowsSQL = `
	SELECT r.id, d.id, d.name, u.id, u.username, r.address, dst.name, ds.port, COALESCE(ds.host, ''),
	       d.subscription_id, COALESCE(s.status, ''),
	       EXISTS (SELECT 1 FROM database_user_grants g WHERE g.database_id = d.id AND g.user_id = u.id),
	       COALESCE(r.expires_at, '')
	FROM database_remote_rules r
	JOIN databases_v2 d ON d.id = r.database_id
	JOIN database_users u ON u.id = r.user_id
	JOIN database_servers ds ON ds.id = d.server_id
	JOIN database_server_types dst ON dst.id = ds.type_id
	JOIN subscriptions s ON s.id = d.subscription_id`

func (p *Panel) databaseRemoteRows(ctx context.Context, where string, args ...any) ([]databaseRemoteRow, error) {
	rows, err := p.db.GetDB().QueryContext(ctx, databaseRemoteRowsSQL+" "+where+" ORDER BY r.id", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []databaseRemoteRow
	for rows.Next() {
		var row databaseRemoteRow
		var serverType string
		if err := rows.Scan(&row.ID, &row.DatabaseID, &row.Database, &row.UserID, &row.User, &row.Address, &serverType,
			&row.Port, &row.Host, &row.SubscriptionID, &row.Status, &row.Granted, &row.ExpiresAt); err != nil {
			return nil, err
		}
		row.Engine = apiTypeNameFor(serverType)
		out = append(out, row)
	}
	return out, rows.Err()
}

// databaseRemoteIneligible says why a rule is not applied ("" = applied).
// Reasons are stable codes for the screen.
func (p *Panel) databaseRemoteIneligible(ctx context.Context, row databaseRemoteRow, offered bool, rights map[int]*effectiveRights) (string, error) {
	switch {
	case !offered:
		return "not_offered", nil
	case !localDatabaseHost(row.Host):
		return "remote_server", nil
	case row.Engine != dbremote.EngineMySQL && row.Engine != dbremote.EnginePostgreSQL:
		return "engine", nil
	case row.Status != "active":
		return "subscription_" + row.Status, nil
	case !row.Granted:
		return "no_access", nil
	case databaseRemoteExpired(row.ExpiresAt, databaseRemoteNow()):
		// alpha.130: never applied past its end, even before the sweep
		// removes it.
		return "expired", nil
	}
	e, ok := rights[row.SubscriptionID]
	if !ok {
		var err error
		if e, err = p.effectiveSubscriptionRights(ctx, row.SubscriptionID); err != nil {
			return "", err
		}
		rights[row.SubscriptionID] = e
	}
	if e.permissionRefusal(rightDatabaseRemote) != "" {
		return "not_in_plan", nil
	}
	if _, err := dbremote.CanonicalAddress(row.Engine, row.Address); err != nil {
		return "address", nil
	}
	return "", nil
}

// databaseRemoteDesired is the eligible rule set, grouped by instance.
func (p *Panel) databaseRemoteDesired(ctx context.Context) ([]transport.DatabaseRemoteTarget, error) {
	offered, err := p.databaseRemoteOffered(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := p.databaseRemoteRows(ctx, "")
	if err != nil {
		return nil, err
	}
	rights := map[int]*effectiveRights{}
	byKey := map[string]*transport.DatabaseRemoteTarget{}
	for _, row := range rows {
		reason, err := p.databaseRemoteIneligible(ctx, row, offered, rights)
		if err != nil {
			return nil, err
		}
		if reason != "" {
			continue
		}
		key := dbremote.TargetKey(row.Engine, row.Port)
		target := byKey[key]
		if target == nil {
			target = &transport.DatabaseRemoteTarget{Engine: row.Engine, Port: row.Port}
			byKey[key] = target
		}
		target.Rules = append(target.Rules, transport.DatabaseRemoteRule{Database: row.Database, User: row.User, Address: row.Address})
	}
	out := make([]transport.DatabaseRemoteTarget, 0, len(byKey))
	for _, target := range byKey {
		// Each user's rules are checked on their own (review L3): a user over
		// the address limit (rows written around the panel) loses only its
		// own rules, never every other customer's on the same engine.
		byUser := map[string][]dbremote.Rule{}
		for _, r := range fromTransportRemoteRules(target.Rules) {
			byUser[r.User] = append(byUser[r.User], r)
		}
		var kept []dbremote.Rule
		for user, rules := range byUser {
			if _, err := (dbremote.Target{Engine: target.Engine, Port: target.Port, Rules: rules}).Normalize(); err != nil {
				log.Printf("database remote access: the rules of %s on %s:%d are left out: %v", user, target.Engine, target.Port, err)
				continue
			}
			kept = append(kept, rules...)
		}
		norm, err := (dbremote.Target{Engine: target.Engine, Port: target.Port, Rules: kept}).Normalize()
		if err != nil {
			log.Printf("database remote access: %s:%d left out: %v", target.Engine, target.Port, err)
			continue
		}
		if len(norm.Rules) == 0 {
			continue
		}
		target.Rules = toTransportRemoteRules(norm.Rules)
		out = append(out, *target)
	}
	sort.Slice(out, func(i, j int) bool {
		return dbremote.TargetKey(out[i].Engine, out[i].Port) < dbremote.TargetKey(out[j].Engine, out[j].Port)
	})
	return out, nil
}

func fromTransportRemoteRules(rules []transport.DatabaseRemoteRule) []dbremote.Rule {
	out := make([]dbremote.Rule, 0, len(rules))
	for _, r := range rules {
		out = append(out, dbremote.Rule{Database: r.Database, User: r.User, Address: r.Address})
	}
	return out
}

func toTransportRemoteRules(rules []dbremote.Rule) []transport.DatabaseRemoteRule {
	out := make([]transport.DatabaseRemoteRule, 0, len(rules))
	for _, r := range rules {
		out = append(out, transport.DatabaseRemoteRule{Database: r.Database, User: r.User, Address: r.Address})
	}
	return out
}

func databaseRemoteDigest(targets []transport.DatabaseRemoteTarget) string {
	var b strings.Builder
	for _, t := range targets {
		b.WriteString(dbremote.TargetKey(t.Engine, t.Port))
		for _, r := range t.Rules {
			b.WriteString("|" + r.Database + "," + r.User + "," + r.Address)
		}
		b.WriteString("\n")
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

// databaseRemoteFirewallPorts are the ports the panel firewall opens (review
// H1): an engine port only while it has eligible rules AND the Agent's last
// answer says rules are in force there AND its address filter holds that
// port. An instance the Agent left alone (an owner-set listen address, a
// lost record), a refused or waiting rule, a filter that could not be
// written, or no answer yet (a restart) keeps the port closed: the panel
// firewall is then the only door, and it stays shut.
func (p *Panel) databaseRemoteFirewallPorts() []int {
	if p == nil || p.db == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	targets, err := p.databaseRemoteDesired(ctx)
	if err != nil {
		log.Printf("database remote access: firewall ports not known: %v", err)
		return nil
	}
	proven := map[int]bool{}
	for _, port := range databaseRemoteFilteredOpenPorts() {
		proven[port] = true
	}
	ports := []int{}
	for _, t := range targets {
		if len(t.Rules) > 0 && proven[t.Port] {
			ports = append(ports, t.Port)
		}
	}
	return ports
}

// databaseRemoteFilteredOpenPorts: ports the last answer has rules in force
// on and the filter holding them.
func databaseRemoteFilteredOpenPorts() []int {
	m := databaseRemoteMemory
	m.mu.Lock()
	defer m.mu.Unlock()
	// Review L2: and only while the filter is proven loaded.
	if m.appliedAt.IsZero() || !m.answer.FilterLoaded {
		return nil
	}
	filtered := map[int]bool{}
	for _, port := range m.answer.FilterPorts {
		filtered[port] = true
	}
	var ports []int
	for _, t := range m.answer.Targets {
		if len(t.Active) > 0 && filtered[t.Port] {
			ports = append(ports, t.Port)
		}
	}
	return ports
}

// applyDatabaseRemote sends the eligible set when it differs from what was
// last applied (always when force). It reports its steps for the progress
// window and syncs the panel firewall when the open ports changed.
func (p *Panel) applyDatabaseRemote(ctx context.Context, force bool) (transport.DatabaseRemoteResponse, bool, error) {
	m := databaseRemoteMemory
	m.applyMu.Lock()
	defer m.applyMu.Unlock()
	targets, err := p.databaseRemoteDesired(ctx)
	if err != nil {
		return transport.DatabaseRemoteResponse{}, false, err
	}
	digest := databaseRemoteDigest(targets)
	m.mu.Lock()
	same := digest == m.digest && m.lastError == ""
	previous := m.answer
	m.mu.Unlock()
	if same && !force {
		return previous, false, nil
	}
	reportOperationStep(ctx, "apply")
	skipNameResolve, err := p.databaseRemoteMariaDBSkipNameResolve(ctx)
	if err != nil {
		return transport.DatabaseRemoteResponse{}, false, err
	}
	offered, err := p.databaseRemoteOffered(ctx)
	if err != nil {
		return transport.DatabaseRemoteResponse{}, false, err
	}
	req := transport.DatabaseRemoteApplyRequest{Targets: targets, HostName: p.mailHostnameIdentity(ctx).Hostname, MariaDBSkipNameResolve: skipNameResolve,
		ServerOffered: offered}
	answer, callErr := databaseRemoteApplyThroughAgent(p, ctx, req)
	m.mu.Lock()
	m.callFailed = callErr != nil
	if callErr != nil {
		m.lastError = callErr.Error()
		m.digest = ""
	} else {
		m.answer, m.appliedAt = answer, time.Now()
		m.lastError = answer.Error
		m.digest = digest
		m.closePending = false
		m.narrowDigest = ""
		if answer.Error != "" {
			m.digest = ""
		}
	}
	m.mu.Unlock()
	if callErr != nil {
		// D5 (alpha.130): the full apply did not run (the server has not
		// finished starting, or the Agent refused it): what is no longer
		// wanted is still closed at the address filter.
		p.narrowDatabaseRemote(ctx, targets)
		return transport.DatabaseRemoteResponse{}, true, callErr
	}
	// The panel firewall opens or closes the engines' ports.
	reportOperationStep(ctx, "firewall")
	// Only when the open ports changed (re-review): a forced apply alone
	// never re-syncs the firewall.
	m.mu.Lock()
	stale := m.firewallStale
	m.firewallStale = false
	m.mu.Unlock()
	if stale || !portSetsEqual(filterOpenPorts(previous), filterOpenPorts(answer)) {
		if err := p.syncFirewall(ctx); err != nil {
			log.Printf("database remote access: firewall sync: %v", err)
			answer.Error = strings.TrimSpace(answer.Error + " The panel firewall could not be updated (" + err.Error() + "); the database port may stay closed until it is.")
		}
	}
	return answer, true, nil
}

// filterOpenPorts: instances with rules in force whose port the filter holds.
func filterOpenPorts(answer transport.DatabaseRemoteResponse) []int {
	filtered := map[int]bool{}
	for _, port := range answer.FilterPorts {
		filtered[port] = answer.FilterLoaded
	}
	var ports []int
	for _, t := range answer.Targets {
		if len(t.Active) > 0 && filtered[t.Port] {
			ports = append(ports, t.Port)
		}
	}
	sort.Ints(ports)
	return ports
}

func portSetsEqual(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// kickDatabaseRemoteSync asks the background loop to apply soon.
func (p *Panel) kickDatabaseRemoteSync() { kickDatabaseRemoteSync() }

// kickDatabaseRemoteSync is called by every change that can change which
// rules are eligible or what a MariaDB copy must carry (review M1): a
// suspension or its end, a plan or rights change, a grant or revoke, a
// database or user deleted. The loop waits a moment (the caller's
// transaction commits first) and then applies the whole set again, forced,
// so a copy also follows a change that leaves the rule set the same.
func kickDatabaseRemoteSync() {
	select {
	case databaseRemoteMemory.kick <- struct{}{}:
	default:
	}
}

// Kicks are coalesced (re-review): the loop waits databaseRemoteKickDelay
// after the first kick, folds every kick that arrives meanwhile into one
// forced apply, and runs forced applies at most once per
// databaseRemoteForcedGap. A burst of changes (or of requests) therefore
// costs one durable job, and the Agent's apply changes nothing that is
// already right.
var (
	databaseRemoteKickDelay = 5 * time.Second
	databaseRemoteForcedGap = 30 * time.Second
)

// databaseRemoteKickWait is how long the loop waits after a kick.
func databaseRemoteKickWait(now, lastForced time.Time) time.Duration {
	wait := databaseRemoteKickDelay
	if !lastForced.IsZero() {
		if gap := lastForced.Add(databaseRemoteForcedGap).Sub(now); gap > wait {
			wait = gap
		}
	}
	return wait
}

// startDatabaseRemoteSync runs the apply at start, on a kick and every five
// minutes; each run sends nothing when the eligible set did not change. The
// returned channel closes when the loop has stopped (after ctx ends).
func (p *Panel) startDatabaseRemoteSync(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(databaseRemoteProbeInterval)
		defer ticker.Stop()
		ticks := 0
		var lastForced time.Time
		startedAt := databaseRemoteNow()
		run := func(force bool) {
			runCtx, cancel := context.WithTimeout(ctx, agentRPCMutationTimeout+time.Minute)
			defer cancel()
			// alpha.130 lab: at start (and while the server is busy) the
			// loop takes no service-mutation lease; a release recovery's idle
			// check must never wait on it. What must close (an ended rule, a
			// withdrawn permission) closes at the address filter, which takes
			// no lease; opening and the engines' own changes follow once the
			// start-up window is over and the server is quiet.
			if !p.databaseRemoteMayMutate(runCtx, startedAt) {
				p.closeDatabaseRemoteWithoutLease(runCtx)
				return
			}
			rows := 0
			_ = p.db.GetDB().QueryRowContext(runCtx, `SELECT COUNT(*) FROM database_remote_rules`).Scan(&rows)
			m := databaseRemoteMemory
			m.mu.Lock()
			never := m.appliedAt.IsZero()
			m.mu.Unlock()
			if rows == 0 && never {
				// No rule here and none applied by this process: ask the
				// Agent (read only) whether its record still holds anything
				// CelikPanel opened, for example after a removal whose answer
				// was lost before a restart. Only then is a change sent.
				status, err := databaseRemoteStatusFromAgent(p, runCtx, transport.DatabaseRemoteStatusRequest{})
				if err != nil || (status.Error == "" && len(status.FilterPorts) == 0) {
					return
				}
			}
			// Rules that wait (a restart, Fail2ban, auth_delay, TLS) are
			// tried again on every tick, so they open by themselves once
			// the administrator did the step (review M3).
			force = force || databaseRemoteWaiting()
			if _, _, err := p.applyDatabaseRemote(runCtx, force); err != nil && !errors.Is(err, context.Canceled) {
				log.Printf("database remote access: apply: %v", err)
			}
		}
		p.sweepExpiredDatabaseRemoteRules(ctx)
		// Read only: a server that already carries the wanted set is
		// recognised, so nothing is sent at start.
		p.seedDatabaseRemoteFromStatus(ctx)
		run(false)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				ticks++
				p.probeDatabaseRemoteFilter(ctx)
				// alpha.130: an address past its end is removed within a
				// minute (and applied at once); a failed apply is tried
				// again every minute, not every fifth (D5).
				expired := p.sweepExpiredDatabaseRemoteRules(ctx) > 0
				if expired || ticks%databaseRemoteApplyEvery == 0 || databaseRemoteFailing() {
					run(expired)
				}
			case <-databaseRemoteMemory.kick:
				timer := time.NewTimer(databaseRemoteKickWait(time.Now(), lastForced))
			coalesce:
				for {
					select {
					case <-ctx.Done():
						timer.Stop()
						return
					case <-databaseRemoteMemory.kick: // folded into this run
					case <-timer.C:
						break coalesce
					}
				}
				lastForced = time.Now()
				run(true)
			}
		}
	}()
	return done
}

// databaseRemoteLast is the last apply's answer for one instance.
func databaseRemoteLast(engine string, port int) (transport.DatabaseRemoteTargetResult, bool, time.Time, string) {
	m := databaseRemoteMemory
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range m.answer.Targets {
		if t.Engine == engine && t.Port == port {
			return t, true, m.appliedAt, m.lastError
		}
	}
	return transport.DatabaseRemoteTargetResult{}, false, m.appliedAt, m.lastError
}

// databaseRemoteRuleStatus is one rule's state on the screen: active,
// refused (with the Agent's problem), waiting (not applied yet) or one of the
// ineligibility reasons.
func databaseRemoteRuleStatus(row databaseRemoteRow, reason string) (string, *transport.DatabaseRemoteProblem) {
	if reason != "" {
		return reason, nil
	}
	canonical, _ := dbremote.CanonicalAddress(row.Engine, row.Address)
	last, known, _, _ := databaseRemoteLast(row.Engine, row.Port)
	if !known {
		return "waiting", nil
	}
	for _, r := range last.Active {
		if r.Database == row.Database && r.User == row.User && r.Address == canonical {
			return "active", nil
		}
	}
	for _, problem := range last.Problems {
		if problem.User == row.User && problem.Database == row.Database && problem.Address == canonical {
			copy := problem
			return "refused", &copy
		}
	}
	return "waiting", nil
}

// databaseRemoteDatabase is one database with its engine instance.
type databaseRemoteDatabase struct {
	ID             int
	Name           string
	SubscriptionID int
	Engine         string
	Port           int
	Host           string
}

var errDatabaseRemoteNotFound = errors.New("database not found")

func (p *Panel) databaseRemoteDatabaseFor(ctx context.Context, c *Caller, id int) (databaseRemoteDatabase, error) {
	var d databaseRemoteDatabase
	var serverType string
	err := p.db.GetDB().QueryRowContext(ctx, `
		SELECT d.id, d.name, d.subscription_id, dst.name, ds.port, COALESCE(ds.host, '')
		FROM databases_v2 d
		JOIN database_servers ds ON ds.id = d.server_id
		JOIN database_server_types dst ON dst.id = ds.type_id
		WHERE d.id = ?`, id).Scan(&d.ID, &d.Name, &d.SubscriptionID, &serverType, &d.Port, &d.Host)
	if errors.Is(err, sql.ErrNoRows) {
		return d, errDatabaseRemoteNotFound
	}
	if err != nil {
		return d, err
	}
	d.Engine = apiTypeNameFor(serverType)
	if err := p.canAccessSubscription(ctx, c, d.SubscriptionID); err != nil {
		if errors.Is(err, errNotFound) {
			return d, errDatabaseRemoteNotFound
		}
		return d, err
	}
	return d, nil
}

// databaseRemoteAccess says whether the caller may add rules to a database
// of this subscription, and if not, why and who decides.
type databaseRemoteAccess struct {
	MayEdit bool   `json:"may_edit"`
	Reason  string `json:"reason,omitempty"` // not_offered, not_in_plan, remote_server, subscription_suspended, ...
	Grantor string `json:"grantor,omitempty"`
}

func (p *Panel) databaseRemoteAccessFor(ctx context.Context, c *Caller, d databaseRemoteDatabase) (databaseRemoteAccess, error) {
	offered, err := p.databaseRemoteOffered(ctx)
	if err != nil {
		return databaseRemoteAccess{}, err
	}
	admin := c != nil && c.hasAccountRole(roleAdmin) && c.APIToken == nil
	switch {
	case !offered:
		return databaseRemoteAccess{Reason: "not_offered", Grantor: "the administrator"}, nil
	case !localDatabaseHost(d.Host):
		return databaseRemoteAccess{Reason: "remote_server", Grantor: "the administrator"}, nil
	}
	var status string
	if err := p.db.GetDB().QueryRowContext(ctx, `SELECT COALESCE(status, '') FROM subscriptions WHERE id = ?`, d.SubscriptionID).Scan(&status); err != nil {
		return databaseRemoteAccess{}, err
	}
	if status != "active" {
		return databaseRemoteAccess{Reason: "subscription_" + status, Grantor: "the administrator"}, nil
	}
	if admin {
		return databaseRemoteAccess{MayEdit: true}, nil
	}
	e, err := p.effectiveSubscriptionRights(ctx, d.SubscriptionID)
	if err != nil {
		return databaseRemoteAccess{}, err
	}
	if layer := e.permissionRefusal(rightDatabaseRemote); layer != "" {
		return databaseRemoteAccess{Reason: "not_in_plan", Grantor: e.grantor(layer)}, nil
	}
	return databaseRemoteAccess{MayEdit: true}, nil
}

// databaseRemoteClient is what to type into pgAdmin, DBeaver or HeidiSQL.
type databaseRemoteClient struct {
	Host    string `json:"host"`
	Port    int    `json:"port"`
	SSLMode string `json:"ssl_mode"`
}

func (p *Panel) databaseRemoteClientFor(ctx context.Context, d databaseRemoteDatabase) databaseRemoteClient {
	return databaseRemoteClient{Host: p.databaseRemoteServerHost(ctx), Port: d.Port, SSLMode: "require"}
}

// databaseRemoteServerHost is the name a customer's program connects to
// (alpha.131, owner): the server's own public host name, never the mail
// name. The server's host name when it is a full name, else the name of the
// panel's certificate, else the mail identity's (which falls back further).
func (p *Panel) databaseRemoteServerHost(ctx context.Context) string {
	if name, err := localServerName(); err == nil {
		if canonical, err := hostname.CanonicalFQDN(strings.TrimSuffix(strings.TrimSpace(name), ".")); err == nil && strings.Contains(canonical, ".") {
			return canonical
		}
	}
	if name := panelCertificateHostname(); name != "" {
		return name
	}
	return p.mailHostnameIdentity(ctx).Hostname
}

// databaseRemoteWaiting: the last answer has an instance whose rules wait
// for a prerequisite.
func databaseRemoteWaiting() bool {
	m := databaseRemoteMemory
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range m.answer.Targets {
		if t.State == transport.DatabaseRemoteStateWaitRestart || t.State == transport.DatabaseRemoteStateWaiting {
			return true
		}
	}
	return false
}

// probeDatabaseRemoteFilter asks the Agent (read only) whether the filter is
// still loaded while ports are open (review L2). A flushed filter closes the
// panel firewall's port at once and asks for an apply, which loads it again
// (the Agent's guard timer also does within 30 seconds); a filter that is
// back reopens the port.
//
// alpha.130 (D6): the same read also carries how often the Agent put the
// panel firewall's table back after a flush; when that number changes the
// panel syncs its live policy onto the restored table (the restore uses the
// saved policy and leaves the database ports out until their filter is
// proven). The read runs every minute, also with no database port open.
func (p *Panel) probeDatabaseRemoteFilter(ctx context.Context) {
	m := databaseRemoteMemory
	m.mu.Lock()
	expected := len(m.answer.FilterPorts) > 0 && !m.appliedAt.IsZero()
	was := m.answer.FilterLoaded
	m.mu.Unlock()
	probeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	status, err := databaseRemoteStatusFromAgent(p, probeCtx, transport.DatabaseRemoteStatusRequest{})
	restored := false
	if err == nil {
		m.mu.Lock()
		restored = m.firewallRestoresKnown && status.PanelFirewallRestores != m.firewallRestores
		m.firewallRestores, m.firewallRestoresKnown = status.PanelFirewallRestores, true
		m.firewallNote = status.PanelFirewallNote
		m.mu.Unlock()
	}
	loaded := err == nil && status.Error == "" && status.FilterLoaded
	if !expected || loaded == was {
		if restored {
			log.Printf("database remote access: the Agent put the panel firewall back after a flush; syncing its live policy")
			if err := p.syncFirewall(probeCtx); err != nil {
				log.Printf("database remote access: firewall sync after the restore: %v", err)
			}
		}
		return
	}
	m.mu.Lock()
	m.answer.FilterLoaded = loaded
	m.mu.Unlock()
	if err := p.syncFirewall(probeCtx); err != nil {
		log.Printf("database remote access: firewall sync after the filter probe: %v", err)
	}
	if !loaded {
		log.Printf("database remote access: the address filter is not loaded; the database ports are closed until it is")
		kickDatabaseRemoteSync()
	}
}
