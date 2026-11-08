package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/dbremote"
	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.130 additions to remote access to databases
// (docs/DATABASE-REMOTE-ACCESS.md):
//
//   - timed access: an address is allowed for 1 day (the default), 7 days or
//     with no end; the sync loop removes it at its end exactly like a
//     manual removal and writes an audit entry; the customer can choose a
//     new duration without adding the address again;
//   - closing at the filter when the full apply cannot run (D5);
//   - the administrator's agreement that CelikPanel turns MariaDB's host
//     name lookup off while remote rules exist.
//
// Süreli erişim, tam uygulama yapılamazken süzgeçte kapatma ve MariaDB ad
// çözümlemesi onayı.

// Durations a customer chooses from. The default is the shortest: an
// address listed for one session closes by itself if it is forgotten (home
// and mobile addresses change hands), and extending is one click.
const (
	databaseRemoteDuration1Day      = "1d"
	databaseRemoteDuration7Days     = "7d"
	databaseRemoteDurationPermanent = "permanent"
	databaseRemoteDefaultDuration   = databaseRemoteDuration1Day
	databaseRemoteTimeLayout        = "2006-01-02 15:04:05"
)

// databaseRemoteNow is the clock (swapped by tests).
var databaseRemoteNow = func() time.Time { return time.Now().UTC() }

// databaseRemoteExpiresAt answers the stored end of a rule chosen now: ""
// for no end; ok false for a duration CelikPanel does not offer.
func databaseRemoteExpiresAt(duration string, now time.Time) (string, bool) {
	switch strings.TrimSpace(duration) {
	case "", databaseRemoteDuration1Day:
		return now.UTC().Add(24 * time.Hour).Format(databaseRemoteTimeLayout), true
	case databaseRemoteDuration7Days:
		return now.UTC().Add(7 * 24 * time.Hour).Format(databaseRemoteTimeLayout), true
	case databaseRemoteDurationPermanent:
		return "", true
	}
	return "", false
}

// databaseRemoteExpired: the stored end has passed. An end that cannot be
// read counts as passed (a rule is never kept open by a damaged value).
func databaseRemoteExpired(expiresAt string, now time.Time) bool {
	if strings.TrimSpace(expiresAt) == "" {
		return false
	}
	at, err := time.ParseInLocation(databaseRemoteTimeLayout, expiresAt, time.UTC)
	return err != nil || !now.Before(at)
}

// databaseRemoteExpiresISO is the end for the screens (RFC 3339, UTC); the
// browser shows it in the viewer's language and time zone.
func databaseRemoteExpiresISO(expiresAt string) string {
	at, err := time.ParseInLocation(databaseRemoteTimeLayout, expiresAt, time.UTC)
	if err != nil {
		return ""
	}
	return at.Format(time.RFC3339)
}

// sweepExpiredDatabaseRemoteRules removes every rule past its end, as a
// manual removal does (the row goes; the next apply closes the filter, the
// pg_hba line or the MariaDB account), with an audit entry each. It answers
// how many it removed. Rules past their end are already left out of every
// apply, so a sweep that fails keeps nothing open.
func (p *Panel) sweepExpiredDatabaseRemoteRules(ctx context.Context) int {
	if p == nil || p.db == nil {
		return 0
	}
	now := databaseRemoteNow()
	rows, err := p.db.GetDB().QueryContext(ctx, `
		SELECT r.id, d.name, u.username, r.address, r.expires_at
		FROM database_remote_rules r
		JOIN databases_v2 d ON d.id = r.database_id
		JOIN database_users u ON u.id = r.user_id
		WHERE r.expires_at IS NOT NULL AND r.expires_at <> ''`)
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			log.Printf("database remote access: expired rules could not be read: %v", err)
		}
		return 0
	}
	type expired struct {
		id                                int
		database, user, address, expireAt string
	}
	var due []expired
	for rows.Next() {
		var e expired
		if err := rows.Scan(&e.id, &e.database, &e.user, &e.address, &e.expireAt); err != nil {
			rows.Close()
			return 0
		}
		if databaseRemoteExpired(e.expireAt, now) {
			due = append(due, e)
		}
	}
	rows.Close()
	removed := 0
	for _, e := range due {
		// Only the row read as expired goes: a rule extended meanwhile keeps
		// its new end.
		result, err := p.db.GetDB().ExecContext(ctx, `DELETE FROM database_remote_rules WHERE id = ? AND expires_at = ?`, e.id, e.expireAt)
		if err != nil {
			log.Printf("database remote access: the expired rule %d could not be removed: %v", e.id, err)
			continue
		}
		if n, _ := result.RowsAffected(); n == 0 {
			continue
		}
		removed++
		p.recordAudit(auditRecord{Action: "database.remote.expire:" + e.database, ResourceType: "database_remote_rule", ResourceID: e.id,
			Details: auditDetailsText(map[string]any{"database": e.database, "user": e.user, "address": e.address, "expired_at": e.expireAt})})
	}
	return removed
}

// databaseRemoteFailing: the last full apply did not run (refused before
// it ran, or no answer; D5: it is tried again every minute). An apply that
// ran and reported a problem waits for the five-minute run as before.
func databaseRemoteFailing() bool {
	m := databaseRemoteMemory
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.callFailed
}

// narrowDatabaseRemote closes at the address filter what targets no longer
// name (D5). The last answer's rules in force shrink to the wanted ones, so
// the panel firewall never keeps a port open for an address that left.
func (p *Panel) narrowDatabaseRemote(ctx context.Context, targets []transport.DatabaseRemoteTarget) {
	// Also with nothing in memory (a Panel that started while the server was
	// still starting): the Agent's record says what the filter holds, and a
	// filter that already holds only wanted addresses is left untouched.
	m := databaseRemoteMemory
	digest := databaseRemoteDigest(targets)
	m.mu.Lock()
	already := digest == m.narrowDigest
	m.mu.Unlock()
	if already {
		return
	}
	answer, err := databaseRemoteNarrowThroughAgent(p, ctx, transport.DatabaseRemoteNarrowRequest{Targets: targets})
	if err != nil || answer.Error != "" {
		detail := answer.Error
		if err != nil {
			detail = err.Error()
		}
		log.Printf("database remote access: closing at the address filter did not finish: %s", detail)
		return
	}
	wanted := map[string]bool{}
	for _, t := range targets {
		for _, r := range t.Rules {
			canonical, _ := dbremote.CanonicalAddress(t.Engine, r.Address)
			wanted[dbremote.TargetKey(t.Engine, t.Port)+"|"+r.Database+"|"+r.User+"|"+canonical] = true
		}
	}
	m.mu.Lock()
	previous := filterOpenPorts(m.answer)
	for i, t := range m.answer.Targets {
		kept := t.Active[:0:0]
		for _, r := range t.Active {
			if wanted[dbremote.TargetKey(t.Engine, t.Port)+"|"+r.Database+"|"+r.User+"|"+r.Address] {
				kept = append(kept, r)
			}
		}
		m.answer.Targets[i].Active = kept
	}
	m.answer.FilterPorts, m.answer.FilterLoaded = answer.FilterPorts, answer.FilterLoaded
	m.narrowDigest = digest
	if len(answer.Targets) > 0 {
		// The Agent names the instances it closed at the filter.
		m.closePending, m.closedAt = true, time.Now()
	}
	if !portSetsEqual(previous, filterOpenPorts(m.answer)) {
		// The panel firewall's port closes at the next full apply, not here:
		// its sync is a mutation with a lease (alpha.130 lab), and the
		// address filter already drops every closed address.
		m.firewallStale = true
	}
	m.mu.Unlock()
}

// databaseRemoteClosePending answers whether the filter was narrowed while
// the full apply still waits, and since when.
func databaseRemoteClosePending() (bool, time.Time) {
	m := databaseRemoteMemory
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.closePending && m.callFailed, m.closedAt
}

// settingDatabaseRemoteSkipNameResolve: the administrator agreed that
// CelikPanel turns MariaDB's host name lookup off while remote rules exist.
const settingDatabaseRemoteSkipNameResolve = "database_remote_mariadb_skip_name_resolve"

func (p *Panel) databaseRemoteMariaDBSkipNameResolve(ctx context.Context) (bool, error) {
	var value string
	err := p.db.GetDB().QueryRowContext(ctx, `SELECT value FROM panel_settings WHERE key = ?`, settingDatabaseRemoteSkipNameResolve).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return value == "1", nil
}

func (p *Panel) saveDatabaseRemoteMariaDBSkipNameResolve(ctx context.Context, on bool) error {
	value := "0"
	if on {
		value = "1"
	}
	_, err := p.db.GetDB().ExecContext(ctx, `INSERT INTO panel_settings(key,value,updated_at) VALUES(?,?,CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=CURRENT_TIMESTAMP`, settingDatabaseRemoteSkipNameResolve, value)
	return err
}

// databaseRemoteLocalInstances lists this server's database engine
// instances (MariaDB; each PostgreSQL cluster by port) for the
// administrator's readiness view.
func (p *Panel) databaseRemoteLocalInstances(ctx context.Context) ([]transport.DatabaseRemoteTarget, error) {
	rows, err := p.db.GetDB().QueryContext(ctx, `
		SELECT DISTINCT dst.name, ds.port, COALESCE(ds.host, '')
		FROM database_servers ds JOIN database_server_types dst ON dst.id = ds.type_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	seen := map[string]bool{}
	var out []transport.DatabaseRemoteTarget
	for rows.Next() {
		var serverType, host string
		var port int
		if err := rows.Scan(&serverType, &port, &host); err != nil {
			return nil, err
		}
		engine := apiTypeNameFor(serverType)
		if !localDatabaseHost(host) || (engine != dbremote.EngineMySQL && engine != dbremote.EnginePostgreSQL) || port < 1 || port > 65535 {
			continue
		}
		key := dbremote.TargetKey(engine, port)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, transport.DatabaseRemoteTarget{Engine: engine, Port: port})
	}
	return out, rows.Err()
}
