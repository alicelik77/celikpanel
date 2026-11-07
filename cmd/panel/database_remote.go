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

const databaseRemoteSyncInterval = 5 * time.Minute

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
}

var databaseRemoteMemory = &databaseRemoteState{kick: make(chan struct{}, 1)}

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
}

const databaseRemoteRowsSQL = `
	SELECT r.id, d.id, d.name, u.id, u.username, r.address, dst.name, ds.port, COALESCE(ds.host, ''),
	       d.subscription_id, COALESCE(s.status, ''),
	       EXISTS (SELECT 1 FROM database_user_grants g WHERE g.database_id = d.id AND g.user_id = u.id)
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
			&row.Port, &row.Host, &row.SubscriptionID, &row.Status, &row.Granted); err != nil {
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
		norm, err := (dbremote.Target{Engine: target.Engine, Port: target.Port, Rules: fromTransportRemoteRules(target.Rules)}).Normalize()
		if err != nil {
			// One subscription's bad set must not close everybody else's:
			// that target's rules are left out and the screen says so.
			log.Printf("database remote access: %s left out: %v", target.Engine, err)
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

// databaseRemoteFirewallPorts are the ports the panel firewall must open:
// those of instances with eligible rules. The address filter of the Agent
// (table celikpanel_dbremote) narrows them to the listed addresses.
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
	ports := []int{}
	for _, t := range targets {
		if len(t.Rules) > 0 {
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
	req := transport.DatabaseRemoteApplyRequest{Targets: targets, HostName: p.mailHostnameIdentity(ctx).Hostname}
	answer, callErr := databaseRemoteApplyThroughAgent(p, ctx, req)
	m.mu.Lock()
	if callErr != nil {
		m.lastError = callErr.Error()
		m.digest = ""
	} else {
		m.answer, m.appliedAt = answer, time.Now()
		m.lastError = answer.Error
		m.digest = digest
		if answer.Error != "" {
			m.digest = ""
		}
	}
	m.mu.Unlock()
	if callErr != nil {
		return transport.DatabaseRemoteResponse{}, true, callErr
	}
	// The panel firewall opens or closes the engines' ports.
	reportOperationStep(ctx, "firewall")
	if !portSetsEqual(filterOpenPorts(previous), filterOpenPorts(answer)) || force {
		if err := p.syncFirewall(ctx); err != nil {
			log.Printf("database remote access: firewall sync: %v", err)
			answer.Error = strings.TrimSpace(answer.Error + " The panel firewall could not be updated (" + err.Error() + "); the database port may stay closed until it is.")
		}
	}
	return answer, true, nil
}

// filterOpenPorts: instances with rules in force.
func filterOpenPorts(answer transport.DatabaseRemoteResponse) []int {
	var ports []int
	for _, t := range answer.Targets {
		if len(t.Active) > 0 {
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
func (p *Panel) kickDatabaseRemoteSync() {
	select {
	case databaseRemoteMemory.kick <- struct{}{}:
	default:
	}
}

// startDatabaseRemoteSync runs the apply at start, on a kick and every five
// minutes; each run sends nothing when the eligible set did not change.
func (p *Panel) startDatabaseRemoteSync(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(databaseRemoteSyncInterval)
		defer ticker.Stop()
		run := func() {
			runCtx, cancel := context.WithTimeout(ctx, agentRPCMutationTimeout+time.Minute)
			defer cancel()
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
			if _, _, err := p.applyDatabaseRemote(runCtx, false); err != nil && !errors.Is(err, context.Canceled) {
				log.Printf("database remote access: apply: %v", err)
			}
		}
		run()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				run()
			case <-databaseRemoteMemory.kick:
				run()
			}
		}
	}()
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

// databaseRemoteCountAddresses: the distinct addresses one user already has
// on its engine.
func (p *Panel) databaseRemoteCountAddresses(ctx context.Context, userID int, except string) (int, error) {
	var n int
	err := p.db.GetDB().QueryRowContext(ctx, `SELECT COUNT(DISTINCT address) FROM database_remote_rules WHERE user_id = ? AND address <> ?`, userID, except).Scan(&n)
	return n, err
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
	return databaseRemoteClient{Host: p.mailHostnameIdentity(ctx).Hostname, Port: d.Port, SSLMode: "require"}
}
