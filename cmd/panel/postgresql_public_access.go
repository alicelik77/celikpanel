package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/repositories"
	"github.com/alicelik/celikpanel/internal/transport"
)

// PostgreSQL databases made before alpha.141 closed to other accounts
// (security lane S1b, 20 November 2026; docs/DATABASE-USERS.md).
//
// alpha.141 makes every new PostgreSQL database closed to the server's other
// roles. A database made before keeps PostgreSQL's default rights: every
// role of the server may connect to it and make temporary tables in it, so
// one customer's database user can sign in to another customer's database
// and read its structure. This loop closes them, on this machine's clusters
// (reached through the Agent), for the databases CelikPanel records.
//
// The rule of the alpha.143 fix round (independent review, 21 November
// 2026):
//
//   - By itself only when nobody else can be seen using the database: its
//     own rights are still PostgreSQL's untouched default, nothing inside it
//     belongs to or is granted to a role outside CelikPanel's records for it
//     (pg_public_access.go lists every footprint read), every recorded user
//     has its role, and its cluster has no monitoring login role and no
//     replication role. Then it is watched: it is closed by itself at the
//     first hourly round that comes at least pgPublicWatchPeriod (24 hours)
//     after the watch began and after at least pgPublicWatchRounds (24)
//     rounds have read it, each finding all of the above. A round that
//     cannot read it does not count. A change of its rights, of the roles
//     inside it, of its owner or of its recorded users starts the count
//     again. A session of a role outside its records seen even once (in any
//     reading) ends the watch: the administrator decides, and the role is
//     named. The administrator can close it at any time.
//   - The close is one transaction inside the database (the Agent). Its
//     answers are three: closed, verified not closed (with the reason), and
//     unknown (it got past BEGIN and its answer was lost). An unknown answer
//     keeps the record ("sending") and the database's real state is read
//     before anything else: closed, unchanged (then it is a failure with its
//     reason, or, for the panel's own close whose answer was simply lost, the
//     same conditions as a first close apply again), or still unknown.
//   - A close that failed is shown with its reason and time; it is not tried
//     again by itself until something about the database changed, or the
//     administrator presses Close.
//   - Closes, undos and the panel's writes for one database are serialised
//     (pgPublicActMu): the hourly round and an administrator never both act;
//     the second reads the first's result. The record of a close (what an
//     undo needs) is never deleted while the database may be closed.
//   - A database an administrator opened again stays on the page as the
//     administrator's choice, out of Problems now and the alert mail.
//
// When: the database-rights loop's admission (database_grant_forms.go): not
// within the first minutes after a start, not before an update of the panel
// is verified, not while the server is busy, only with an Agent of the same
// build; then hourly. The page reads the last scan (with its time); "Read
// again" and the round start a scan, one at a time (pgPublicScans). Kept in
// panel_settings rows (no schema change): "postgresql_public_v1:<database
// id>" for each database CelikPanel watches, closed or is closing, and
// "postgresql_public_list_v1" for the list Problems now shows.
//
// alpha.141'den önce açılan PostgreSQL veritabanları başka hesaplara kapatılır:
// kendiliğinden yalnız başka hiç kimsenin kullandığı görülmeyen, bir gün (24
// saatlik denetim) izlenmiş veritabanlarında; diğerleri yöneticiye listelenir.
// Bilinmeyen sonuç "değişmedi" sayılmaz; başarısız kapatma nedeniyle gösterilir.

const (
	pgPublicMarkerPrefix = "postgresql_public_v1:"
	pgPublicListKey      = "postgresql_public_list_v1"
	pgPublicPath         = "/api/v1/admin/postgresql/public-access"

	// pgPublicWatch: nothing closed by CelikPanel; the record holds the
	// watch, the roles seen connected and the last failed close.
	pgPublicWatch   = "watch"
	pgPublicSending = "sending"
	pgPublicClosed  = "closed"
	pgPublicUndone  = "undone"

	pgPublicByPanel         = "panel"
	pgPublicByAdministrator = "administrator"

	// pgPublicWatchRounds: the rounds that must read a database, finding
	// nobody else, before CelikPanel closes it by itself.
	pgPublicWatchRounds = 24
)

var (
	// pgPublicAgent calls the Agent (a seam for tests).
	pgPublicAgent = func(p *Panel, ctx context.Context, method string, req, resp any) error {
		return p.callAgentContext(ctx, method, req, resp)
	}
	pgPublicNow = time.Now
	// pgPublicWatchPeriod: the time a database is watched before CelikPanel
	// closes it by itself.
	pgPublicWatchPeriod = 24 * time.Hour
	// pgPublicActMu serialises every action and write for the databases.
	pgPublicActMu sync.Mutex
)

// pgPublicMarker is what CelikPanel did to, or saw of, one database.
type pgPublicMarker struct {
	Version int    `json:"version"`
	State   string `json:"state"`
	By      string `json:"by,omitempty"`
	Port    int    `json:"port,omitempty"`
	Name    string `json:"name,omitempty"`
	// The database's rights before and after the close, PUBLIC's rights
	// before it and the rights the close gave (an undo takes exactly these).
	BeforeACL       string   `json:"before_acl,omitempty"`
	AfterACL        string   `json:"after_acl,omitempty"`
	PublicConnect   bool     `json:"public_connect,omitempty"`
	PublicTemporary bool     `json:"public_temporary,omitempty"`
	Added           []string `json:"added"`
	// Grants: what the close was asked to give (kept while it is sent).
	Grants   []transport.PostgreSQLPublicAccessGrant `json:"grants,omitempty"`
	At       string                                  `json:"at,omitempty"`
	UndoneAt string                                  `json:"undone_at,omitempty"`
	// The watch before a close by itself: since when, how many rounds read
	// it, and the facts it watches (a change starts it again).
	WatchSince  string `json:"watch_since,omitempty"`
	Rounds      int    `json:"rounds,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
	// Roles outside the records seen connected to it, ever, and when first.
	SeenRoles []string `json:"seen_roles,omitempty"`
	SeenAt    string   `json:"seen_at,omitempty"`
	// The last close that failed (verified): when, whose, why, and the facts
	// it failed on (it is tried again by itself only when they change).
	FailedAt          string `json:"failed_at,omitempty"`
	FailedBy          string `json:"failed_by,omitempty"`
	Failure           string `json:"failure,omitempty"`
	FailedFingerprint string `json:"failed_fingerprint,omitempty"`
}

// pgPublicEntry is one database on the administrator's view.
type pgPublicEntry struct {
	DatabaseID int    `json:"database_id"`
	Name       string `json:"name"`
	Port       int    `json:"port"`
	// State: open (Found says why), closed (by the panel or an
	// administrator), reopened (an administrator opened it again), unknown
	// (it could not be read).
	State string `json:"state"`
	By    string `json:"by,omitempty"`
	At    string `json:"at,omitempty"`
	Owner string `json:"owner,omitempty"`
	ACL   string `json:"acl,omitempty"`
	// What every role of the server may do in it now.
	PublicConnect   bool `json:"public_connect,omitempty"`
	PublicTemporary bool `json:"public_temporary,omitempty"`
	// Found: why it is open (observing, close_failed, unknown_result,
	// changed_rights, other_roles, other_sessions, missing_users,
	// monitoring_roles, replication_roles).
	Found      []string `json:"found,omitempty"`
	OtherRoles []string `json:"other_roles,omitempty"`
	// Sessions: roles outside the records seen connected (ever);
	// SessionsNow: those connected at the reading.
	Sessions    []string `json:"sessions,omitempty"`
	SessionsNow []string `json:"sessions_now,omitempty"`
	Missing     []string `json:"missing,omitempty"`
	Monitoring  []string `json:"monitoring,omitempty"`
	Replication []string `json:"replication,omitempty"`
	// Roles: every role inside it as read (sent back with a close, which
	// refuses when they changed). Grants: what a close would give.
	Roles  []string                                `json:"roles,omitempty"`
	Grants []transport.PostgreSQLPublicAccessGrant `json:"grants,omitempty"`
	// The watch: since when, the time after which it closes by itself, and
	// the rounds counted.
	WatchSince string `json:"watch_since,omitempty"`
	ClosesAt   string `json:"closes_at,omitempty"`
	Rounds     int    `json:"rounds,omitempty"`
	// The last failed close.
	FailedAt string `json:"failed_at,omitempty"`
	Failure  string `json:"failure,omitempty"`
	// UndoStatements: the plain statements of an undo, for an owner without
	// the panel (run as the postgres account).
	UndoStatements []string `json:"undo_statements,omitempty"`
	Reason         string   `json:"reason,omitempty"`
}

type pgPublicView struct {
	CheckedAt string          `json:"checked_at,omitempty"`
	Open      []pgPublicEntry `json:"open"`
	Closed    []pgPublicEntry `json:"closed"`
	Reopened  []pgPublicEntry `json:"reopened"`
	Unknown   []pgPublicEntry `json:"unknown"`
}

// pgPublicRecord is one recorded database of this machine's PostgreSQL.
type pgPublicRecord struct {
	id     int
	name   string
	port   int
	users  map[string]bool // recorded user -> read-and-write
	userIn []string
}

func pgPublicMarkerKey(id int) string { return pgPublicMarkerPrefix + strconv.Itoa(id) }

func (p *Panel) readPGPublicMarker(ctx context.Context, id int) (*pgPublicMarker, error) {
	var raw string
	err := p.db.GetDB().QueryRowContext(ctx, `SELECT value FROM panel_settings WHERE key = ?`, pgPublicMarkerKey(id)).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var marker pgPublicMarker
	if err := json.Unmarshal([]byte(raw), &marker); err != nil || marker.Version != 1 {
		return nil, fmt.Errorf("the record of database %d's close cannot be read", id)
	}
	return &marker, nil
}

func (p *Panel) writePGPublicMarker(ctx context.Context, id int, marker *pgPublicMarker) error {
	marker.Version = 1
	if marker.Added == nil {
		marker.Added = []string{}
	}
	raw, err := json.Marshal(marker)
	if err != nil {
		return err
	}
	_, err = p.db.GetDB().ExecContext(context.WithoutCancel(ctx), `INSERT INTO panel_settings (key, value, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP`, pgPublicMarkerKey(id), string(raw))
	return err
}

// deletePGPublicWatch removes a watch record with nothing left to keep. It
// is only called for a record in the watch state: a close's record is never
// deleted.
func (p *Panel) deletePGPublicWatch(ctx context.Context, id int) {
	_, _ = p.db.GetDB().ExecContext(context.WithoutCancel(ctx), `DELETE FROM panel_settings WHERE key = ?`, pgPublicMarkerKey(id))
}

// pgPublicRecords are the PostgreSQL databases CelikPanel records on this
// machine's clusters, with their recorded users.
func (p *Panel) pgPublicRecords(ctx context.Context) ([]pgPublicRecord, error) {
	rows, err := p.db.GetDB().QueryContext(ctx, `
		SELECT ds.id FROM database_servers ds JOIN database_server_types t ON t.id = ds.type_id
		WHERE t.name = 'postgresql' AND COALESCE(ds.status, 'active') <> 'retired' ORDER BY ds.id`)
	if err != nil {
		return nil, err
	}
	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	repo := repositories.NewPostgresDatabaseServerRepository(p.db.GetDB())
	var records []pgPublicRecord
	for _, id := range ids {
		server, err := repo.GetByID(ctx, id)
		if err != nil {
			return nil, err
		}
		if !databaseRecordIsOurLocalEngine(server) {
			continue // a registered server: its own administrator's
		}
		_, port := databaseAgentEngine(server)
		list, err := p.pgPublicDatabasesOf(ctx, server, port)
		if err != nil {
			return nil, err
		}
		records = append(records, list...)
	}
	return records, nil
}

func (p *Panel) pgPublicDatabasesOf(ctx context.Context, server *core.DatabaseServer, port int) ([]pgPublicRecord, error) {
	rows, err := p.db.GetDB().QueryContext(ctx, `
		SELECT d.id, d.name, COALESCE(u.username, ''), COALESCE(g.privileges, '') FROM databases_v2 d
		LEFT JOIN database_user_grants g ON g.database_id = d.id
		LEFT JOIN database_users u ON u.id = g.user_id
		WHERE d.server_id = ? ORDER BY d.id, g.id`, server.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byID := map[int]*pgPublicRecord{}
	var order []int
	for rows.Next() {
		var id int
		var name, user, privileges string
		if err := rows.Scan(&id, &name, &user, &privileges); err != nil {
			return nil, err
		}
		record := byID[id]
		if record == nil {
			record = &pgPublicRecord{id: id, name: name, port: port, users: map[string]bool{}}
			byID[id] = record
			order = append(order, id)
		}
		if user != "" {
			if _, seen := record.users[user]; !seen {
				record.userIn = append(record.userIn, user)
			}
			record.users[user] = record.users[user] || !strings.EqualFold(strings.TrimSpace(privileges), "SELECT")
		}
	}
	out := make([]pgPublicRecord, 0, len(order))
	for _, id := range order {
		out = append(out, *byID[id])
	}
	return out, rows.Err()
}

// pgPublicCluster is what one cluster shows for all its databases.
type pgPublicCluster struct {
	monitoring  []string
	replication []string
}

// pgPublicScanResult is one scan of every recorded database.
type pgPublicScanResult struct {
	started  time.Time
	at       time.Time
	records  []pgPublicRecord
	facts    map[int]transport.PostgreSQLPublicAccessFacts
	clusters map[int]pgPublicCluster
	unread   map[int]string
}

// pgPublicScan reads every cluster's recorded databases (read only; one
// Agent call, and one database session, for each cluster).
func (p *Panel) pgPublicScan(ctx context.Context, records []pgPublicRecord) (map[int]transport.PostgreSQLPublicAccessFacts, map[int]pgPublicCluster, map[int]string, error) {
	byPort := map[int][]pgPublicRecord{}
	var ports []int
	for _, record := range records {
		if _, seen := byPort[record.port]; !seen {
			ports = append(ports, record.port)
		}
		byPort[record.port] = append(byPort[record.port], record)
	}
	facts := map[int]transport.PostgreSQLPublicAccessFacts{}
	clusters := map[int]pgPublicCluster{}
	unread := map[int]string{}
	for _, port := range ports {
		list := byPort[port]
		req := &transport.PostgreSQLPublicAccessScanRequest{ExpectedBuildCommit: strings.TrimSpace(buildCommit), Port: port}
		for _, record := range list {
			req.Databases = append(req.Databases, transport.PostgreSQLPublicAccessDatabase{Name: record.name, Recorded: nonNilStrings(record.userIn)})
		}
		var resp transport.PostgreSQLPublicAccessScanResponse
		err := pgPublicAgent(p, ctx, "Agent.PostgreSQLPublicAccessScanV1", req, &resp)
		switch {
		case err != nil:
			unread[port] = "the Agent did not answer within its time"
			continue
		case resp.BuildMismatch:
			return nil, nil, nil, errGrantFormsBuildMismatch
		case resp.Refusal != nil:
			unread[port] = "its PostgreSQL cluster does not run"
			continue
		case resp.Error != "":
			unread[port] = boundedAgentDiagnostic(resp.Error)
			continue
		}
		clusters[port] = pgPublicCluster{monitoring: resp.Monitoring, replication: resp.Replication}
		for i, record := range list {
			if i < len(resp.Databases) && resp.Databases[i].Name == record.name {
				facts[record.id] = resp.Databases[i]
			}
		}
	}
	return facts, clusters, unread, nil
}

// pgPublicScans holds the last scan of each panel and the one running:
// one scan at a time; others wait for its result.
var pgPublicScans = struct {
	sync.Mutex
	last    map[*Panel]*pgPublicScanResult
	running map[*Panel]*pgPublicFlight
}{last: map[*Panel]*pgPublicScanResult{}, running: map[*Panel]*pgPublicFlight{}}

type pgPublicFlight struct {
	started time.Time
	done    chan struct{}
	result  *pgPublicScanResult
	err     error
}

// pgPublicCachedScan is the last scan, nil before the first.
func (p *Panel) pgPublicCachedScan() *pgPublicScanResult {
	pgPublicScans.Lock()
	defer pgPublicScans.Unlock()
	return pgPublicScans.last[p]
}

// pgPublicFreshScan returns a scan that started at notBefore or later: the
// last one, the running one (waited for), or a new one once none runs.
func (p *Panel) pgPublicFreshScan(ctx context.Context, notBefore time.Time) (*pgPublicScanResult, error) {
	for {
		pgPublicScans.Lock()
		if last := pgPublicScans.last[p]; last != nil && !last.started.Before(notBefore) {
			pgPublicScans.Unlock()
			return last, nil
		}
		if flight := pgPublicScans.running[p]; flight != nil {
			pgPublicScans.Unlock()
			select {
			case <-flight.done:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			if flight.err == nil && !flight.started.Before(notBefore) {
				return flight.result, nil
			}
			if flight.err != nil && !flight.started.Before(notBefore) {
				return nil, flight.err
			}
			continue
		}
		flight := &pgPublicFlight{started: pgPublicNow(), done: make(chan struct{})}
		pgPublicScans.running[p] = flight
		pgPublicScans.Unlock()
		result, err := p.pgPublicScanAll(ctx)
		if result != nil {
			result.started = flight.started
		}
		pgPublicScans.Lock()
		flight.result, flight.err = result, err
		if err == nil {
			pgPublicScans.last[p] = result
		}
		delete(pgPublicScans.running, p)
		close(flight.done)
		pgPublicScans.Unlock()
		return result, err
	}
}

func (p *Panel) pgPublicScanAll(ctx context.Context) (*pgPublicScanResult, error) {
	records, err := p.pgPublicRecords(ctx)
	if err != nil {
		return nil, err
	}
	facts, clusters, unread, err := p.pgPublicScan(ctx, records)
	if err != nil {
		return nil, err
	}
	return &pgPublicScanResult{at: pgPublicNow().UTC(), records: records, facts: facts, clusters: clusters, unread: unread}, nil
}

// pgPublicFingerprint is the facts a watch and a failed close depend on: a
// change of any of them starts the watch again or lets a failed close be
// tried again.
func pgPublicFingerprint(record pgPublicRecord, f transport.PostgreSQLPublicAccessFacts) string {
	users := make([]string, 0, len(record.userIn))
	for _, user := range record.userIn {
		users = append(users, fmt.Sprintf("%s:%v", user, record.users[user]))
	}
	sort.Strings(users)
	roles := append([]string{}, f.Roles...)
	sort.Strings(roles)
	missing := append([]string{}, f.Missing...)
	sort.Strings(missing)
	sum := sha256.Sum256([]byte(strings.Join([]string{f.Owner, f.ACL, strings.Join(roles, ","), strings.Join(users, ","), strings.Join(missing, ",")}, "\x1f")))
	return hex.EncodeToString(sum[:12])
}

// pgPublicDecide is what one database's facts say.
func pgPublicDecide(record pgPublicRecord, f transport.PostgreSQLPublicAccessFacts, cluster pgPublicCluster, marker *pgPublicMarker) pgPublicEntry {
	entry := pgPublicEntry{DatabaseID: record.id, Name: record.name, Port: record.port, Owner: f.Owner, ACL: f.ACL,
		PublicConnect: f.PublicConnect, PublicTemporary: f.PublicTemporary, Roles: append([]string{}, f.Roles...)}
	for _, role := range f.Roles {
		if !containsString(record.userIn, role) {
			entry.OtherRoles = append(entry.OtherRoles, role)
		}
	}
	seen := map[string]bool{}
	if marker != nil {
		for _, role := range marker.SeenRoles {
			seen[role] = true
		}
	}
	for _, role := range f.Sessions {
		if !containsString(record.userIn, role) {
			entry.SessionsNow = append(entry.SessionsNow, role)
			seen[role] = true
		}
	}
	for role := range seen {
		entry.Sessions = append(entry.Sessions, role)
	}
	sort.Strings(entry.Sessions)
	entry.Missing = append(entry.Missing, f.Missing...)
	entry.Monitoring = append(entry.Monitoring, cluster.monitoring...)
	entry.Replication = append(entry.Replication, cluster.replication...)
	for _, user := range record.userIn {
		if user != f.Owner {
			entry.Grants = append(entry.Grants, transport.PostgreSQLPublicAccessGrant{Role: user, Temporary: record.users[user]})
		}
	}
	for reason, found := range map[string]bool{
		"changed_rights":    f.ACL != "",
		"other_roles":       len(entry.OtherRoles) > 0,
		"other_sessions":    len(entry.Sessions) > 0,
		"missing_users":     len(entry.Missing) > 0,
		"monitoring_roles":  len(entry.Monitoring) > 0,
		"replication_roles": len(entry.Replication) > 0,
	} {
		if found {
			entry.Found = append(entry.Found, reason)
		}
	}
	sort.Slice(entry.Found, func(i, j int) bool { return pgPublicFoundOrder[entry.Found[i]] < pgPublicFoundOrder[entry.Found[j]] })
	return entry
}

var pgPublicFoundOrder = map[string]int{"close_failed": 0, "unknown_result": 1, "changed_rights": 2, "other_roles": 3, "other_sessions": 4,
	"missing_users": 5, "monitoring_roles": 6, "replication_roles": 7, "observing": 8}

// startPostgreSQLPublicAccess runs the rounds until ctx ends.
func (p *Panel) startPostgreSQLPublicAccess(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		startedAt := p.processStartedAt()
		timer := time.NewTimer(databaseGrantFormsStartupQuiet)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
			}
			roundCtx, cancel := context.WithTimeout(ctx, 20*time.Minute)
			p.runPostgreSQLPublicRound(roundCtx, startedAt)
			cancel()
			timer.Reset(databaseGrantFormsInterval)
		}
	}()
	return done
}

// runPostgreSQLPublicRound reads every recorded database, counts the round
// for the watched ones and closes, when nothing holds the server, those it
// may close by itself.
func (p *Panel) runPostgreSQLPublicRound(ctx context.Context, startedAt time.Time) {
	closeNow := p.databaseGrantFormsHold(ctx, startedAt) == ""
	view, err := p.pgPublicRead(ctx, pgPublicNow(), pgPublicPass{write: true, round: true, closeNow: closeNow})
	if err != nil {
		if !errors.Is(err, errGrantFormsBuildMismatch) {
			log.Printf("postgresql databases: the rights could not be read: %v", err)
		}
		return
	}
	p.storePGPublicList(ctx, view)
}

// pgPublicPass says what one pass over a scan may do: write the panel's own
// records (reconcile a lost answer, keep the roles seen, the watch), count a
// round, close.
type pgPublicPass struct {
	write    bool
	round    bool
	closeNow bool
}

// pgPublicRead scans (one scan at a time, notBefore as pgPublicFreshScan)
// and makes the view.
func (p *Panel) pgPublicRead(ctx context.Context, notBefore time.Time, pass pgPublicPass) (pgPublicView, error) {
	scan, err := p.pgPublicFreshScan(ctx, notBefore)
	if err != nil {
		return pgPublicView{Open: []pgPublicEntry{}, Closed: []pgPublicEntry{}, Reopened: []pgPublicEntry{}, Unknown: []pgPublicEntry{}}, err
	}
	return p.pgPublicBuild(ctx, scan, pass)
}

// pgPublicBuild makes the view of one scan; without pass.write it changes
// nothing (the page's read of the last scan).
func (p *Panel) pgPublicBuild(ctx context.Context, scan *pgPublicScanResult, pass pgPublicPass) (pgPublicView, error) {
	view := pgPublicView{Open: []pgPublicEntry{}, Closed: []pgPublicEntry{}, Reopened: []pgPublicEntry{}, Unknown: []pgPublicEntry{}}
	view.CheckedAt = scan.at.Format(time.RFC3339)
	for _, record := range scan.records {
		list, entry, err := p.pgPublicOne(ctx, scan, record, pass)
		if err != nil {
			return view, err
		}
		switch list {
		case "open":
			view.Open = append(view.Open, entry)
		case pgPublicClosed:
			view.Closed = append(view.Closed, entry)
		case "reopened":
			view.Reopened = append(view.Reopened, entry)
		case "unknown":
			view.Unknown = append(view.Unknown, entry)
		}
	}
	sort.SliceStable(view.Open, func(i, j int) bool { return view.Open[i].Name < view.Open[j].Name })
	sort.SliceStable(view.Reopened, func(i, j int) bool { return view.Reopened[i].Name < view.Reopened[j].Name })
	sort.SliceStable(view.Closed, func(i, j int) bool { return view.Closed[i].At > view.Closed[j].At })
	return view, nil
}

// pgPublicOne decides one database: the list it belongs to and its entry.
func (p *Panel) pgPublicOne(ctx context.Context, scan *pgPublicScanResult, record pgPublicRecord, pass pgPublicPass) (string, pgPublicEntry, error) {
	if pass.write {
		pgPublicActMu.Lock()
		defer pgPublicActMu.Unlock()
	}
	marker, err := p.readPGPublicMarker(ctx, record.id)
	if err != nil {
		return "", pgPublicEntry{}, err
	}
	f, read := scan.facts[record.id]
	if !read || f.Error != "" {
		reason := scan.unread[record.port]
		if read {
			reason = boundedAgentDiagnostic(f.Error)
		}
		if marker != nil && marker.State == pgPublicClosed {
			return pgPublicClosed, pgPublicClosedEntry(record, marker), nil
		}
		return "unknown", pgPublicEntry{DatabaseID: record.id, Name: record.name, Port: record.port, State: "unknown", Reason: reason}, nil
	}
	if !f.Exists {
		return "", pgPublicEntry{}, nil
	}
	publicClosed := !f.PublicConnect && !f.PublicTemporary
	// A close whose answer was lost: what the engine shows decides, before
	// anything else.
	if marker != nil && marker.State == pgPublicSending && pass.write {
		if marker, _, err = p.pgPublicReconcile(ctx, record, marker, f); err != nil {
			return "", pgPublicEntry{}, err
		}
	}
	entry := pgPublicDecide(record, f, scan.clusters[record.port], marker)
	if marker != nil {
		switch marker.State {
		case pgPublicSending:
			if publicClosed {
				closed := *marker
				closed.AfterACL = f.ACL
				return pgPublicClosed, pgPublicClosedEntry(record, &closed), nil
			}
			entry.State, entry.By, entry.At = "open", marker.By, marker.At
			entry.Found = append([]string{"unknown_result"}, entry.Found...)
			return "open", entry, nil
		case pgPublicClosed:
			if publicClosed {
				return pgPublicClosed, pgPublicClosedEntry(record, marker), nil
			}
			// Opened again outside the panel: listed, never closed by itself.
			entry.State = "open"
			pgPublicFailureOf(&entry, marker, pgPublicFingerprint(record, f))
			if len(entry.Found) == 0 {
				entry.Found = []string{"changed_rights"}
			}
			return "open", entry, nil
		case pgPublicUndone:
			if publicClosed {
				return "", pgPublicEntry{}, nil
			}
			entry.State, entry.By, entry.At = "reopened", pgPublicByAdministrator, marker.UndoneAt
			pgPublicFailureOf(&entry, marker, pgPublicFingerprint(record, f))
			return "reopened", entry, nil
		}
	}
	if publicClosed {
		// Closed already (alpha.141 or later, or by its owner).
		if marker != nil && marker.State == pgPublicWatch && pass.write {
			p.deletePGPublicWatch(ctx, record.id)
		}
		return "", pgPublicEntry{}, nil
	}
	watch := &pgPublicMarker{State: pgPublicWatch}
	if marker != nil {
		copied := *marker
		watch = &copied
	}
	before, _ := json.Marshal(watch)
	now := pgPublicNow().UTC()
	if len(entry.Sessions) > len(watch.SeenRoles) {
		watch.SeenRoles = append([]string{}, entry.Sessions...)
		if watch.SeenAt == "" {
			watch.SeenAt = now.Format(time.RFC3339)
		}
	}
	fingerprint := pgPublicFingerprint(record, f)
	if watch.FailedAt != "" && watch.FailedFingerprint != fingerprint {
		// Something changed since the close failed: it may be tried again.
		watch.FailedAt, watch.FailedBy, watch.Failure, watch.FailedFingerprint = "", "", "", ""
	}
	failed := pgPublicFailureOf(&entry, watch, fingerprint)
	if len(entry.Found) == 0 {
		if watch.WatchSince == "" || watch.Fingerprint != fingerprint {
			watch.WatchSince, watch.Rounds, watch.Fingerprint = now.Format(time.RFC3339), 0, fingerprint
		}
		if pass.round {
			watch.Rounds++
		}
		since, _ := time.Parse(time.RFC3339, watch.WatchSince)
		if watch.Rounds >= pgPublicWatchRounds && !now.Before(since.Add(pgPublicWatchPeriod)) && pass.closeNow && pass.write {
			if string(before) != pgPublicJSON(watch) {
				if err := p.writePGPublicMarker(ctx, record.id, watch); err != nil {
					return "", pgPublicEntry{}, err
				}
			}
			outcome, closed, err := p.pgPublicCloseBy(ctx, record, entry, pgPublicByPanel, watch, fingerprint)
			if err != nil {
				return "", pgPublicEntry{}, err
			}
			if outcome == pgPublicClosed {
				return pgPublicClosed, pgPublicClosedEntry(record, closed), nil
			}
			// Not closed: what it says now is the record's.
			entry = pgPublicDecide(record, f, scan.clusters[record.port], closed)
			pgPublicFailureOf(&entry, closed, fingerprint)
			if closed != nil && closed.State == pgPublicSending {
				entry.Found = append([]string{"unknown_result"}, entry.Found...)
			}
			if len(entry.Found) == 0 {
				pgPublicObserving(&entry, closed)
			}
			entry.State = "open"
			return "open", entry, nil
		}
		pgPublicObserving(&entry, watch)
	} else if !failed && watch.WatchSince != "" {
		watch.WatchSince, watch.Rounds, watch.Fingerprint = "", 0, ""
	}
	if pass.write && string(before) != pgPublicJSON(watch) {
		if len(watch.SeenRoles) == 0 && watch.FailedAt == "" && watch.WatchSince == "" && watch.State == pgPublicWatch {
			p.deletePGPublicWatch(ctx, record.id)
		} else if err := p.writePGPublicMarker(ctx, record.id, watch); err != nil {
			return "", pgPublicEntry{}, err
		}
	}
	entry.State = "open"
	return "open", entry, nil
}

func pgPublicJSON(value any) string {
	raw, _ := json.Marshal(value)
	return string(raw)
}

// pgPublicFailureOf adds a failed close to an entry while the facts it
// failed on still hold.
func pgPublicFailureOf(entry *pgPublicEntry, marker *pgPublicMarker, fingerprint string) bool {
	if marker == nil || marker.FailedAt == "" || marker.FailedFingerprint != fingerprint {
		return false
	}
	entry.FailedAt, entry.Failure = marker.FailedAt, marker.Failure
	entry.Found = append([]string{"close_failed"}, entry.Found...)
	return true
}

// pgPublicObserving marks an entry as watched: since when, and the time
// after which the first round closes it (with the rounds counted).
func pgPublicObserving(entry *pgPublicEntry, watch *pgPublicMarker) {
	entry.Found = []string{"observing"}
	if watch == nil {
		return
	}
	entry.WatchSince, entry.Rounds = watch.WatchSince, watch.Rounds
	if since, err := time.Parse(time.RFC3339, watch.WatchSince); err == nil {
		closes := since.Add(pgPublicWatchPeriod)
		// Rounds still to come cannot all fall before it.
		if missing := pgPublicWatchRounds - watch.Rounds; missing > 0 {
			if earliest := pgPublicNow().UTC().Add(time.Duration(missing-1) * databaseGrantFormsInterval); earliest.After(closes) {
				closes = earliest
			}
		}
		entry.ClosesAt = closes.Format(time.RFC3339)
	}
}

// pgPublicReconcile settles a "sending" record from the engine's state:
// closed; unchanged (a failure with its reason; or, for the panel's own
// close whose answer was simply lost, a watch again under the same
// conditions); or still unknown. The caller holds pgPublicActMu.
func (p *Panel) pgPublicReconcile(ctx context.Context, record pgPublicRecord, marker *pgPublicMarker, f transport.PostgreSQLPublicAccessFacts) (*pgPublicMarker, string, error) {
	if f.Error != "" || !f.Exists {
		return marker, "unknown", nil
	}
	if !f.PublicConnect && !f.PublicTemporary {
		closed := *marker
		closed.State, closed.AfterACL = pgPublicClosed, f.ACL
		closed.Added = pgPublicAddedFrom(closed.BeforeACL, closed.Grants)
		closed.Grants, closed.Failure = nil, ""
		if err := p.writePGPublicMarker(ctx, record.id, &closed); err != nil {
			return marker, "unknown", err
		}
		if closed.By == pgPublicByPanel {
			p.recordAudit(auditRecord{Action: "database.postgresql.closed_to_other_accounts:" + record.name, ResourceType: "database", ResourceID: record.id})
		}
		return &closed, pgPublicClosed, nil
	}
	if f.ACL != marker.BeforeACL || f.PublicConnect != marker.PublicConnect || f.PublicTemporary != marker.PublicTemporary {
		return marker, "unknown", nil
	}
	watch := &pgPublicMarker{State: pgPublicWatch, WatchSince: marker.WatchSince, Rounds: marker.Rounds, Fingerprint: marker.Fingerprint,
		SeenRoles: marker.SeenRoles, SeenAt: marker.SeenAt}
	outcome := "unchanged"
	if marker.By == pgPublicByAdministrator || marker.Failure != "" {
		watch.FailedAt, watch.FailedBy, watch.FailedFingerprint = pgPublicNow().UTC().Format(time.RFC3339), marker.By, pgPublicFingerprint(record, f)
		watch.Failure = marker.Failure
		if watch.Failure == "" {
			watch.Failure = "the server's answer to the close was lost and the database is unchanged"
		}
		outcome = "failed"
	}
	if err := p.writePGPublicMarker(ctx, record.id, watch); err != nil {
		return marker, "unknown", err
	}
	return watch, outcome, nil
}

// pgPublicCloseBy closes one database; the record is written before the
// Agent is asked, so a lost answer is settled from the engine. prev is the
// record before (put back when the Agent verified that nothing changed);
// fingerprint is the facts the close was decided on. Outcomes: closed,
// changed (verified: nothing changed), failed (verified, with its reason),
// unchanged (the panel's answer was lost and nothing changed), unknown,
// mismatch. The caller holds pgPublicActMu.
func (p *Panel) pgPublicCloseBy(ctx context.Context, record pgPublicRecord, entry pgPublicEntry, by string, prev *pgPublicMarker, fingerprint string) (string, *pgPublicMarker, error) {
	connect, temporary := pgPublicRights(entry.ACL)
	marker := &pgPublicMarker{State: pgPublicSending, By: by, Port: record.port, Name: record.name, BeforeACL: entry.ACL,
		PublicConnect: connect, PublicTemporary: temporary, Grants: entry.Grants, At: pgPublicNow().UTC().Format(time.RFC3339)}
	if prev != nil && prev.State == pgPublicWatch {
		marker.WatchSince, marker.Rounds, marker.Fingerprint, marker.SeenRoles, marker.SeenAt = prev.WatchSince, prev.Rounds, prev.Fingerprint, prev.SeenRoles, prev.SeenAt
	}
	if err := p.writePGPublicMarker(ctx, record.id, marker); err != nil {
		return "", prev, fmt.Errorf("the close of %s could not be recorded first: %w", record.name, err)
	}
	putBack := func() error {
		if prev == nil {
			_, err := p.db.GetDB().ExecContext(context.WithoutCancel(ctx), `DELETE FROM panel_settings WHERE key = ?`, pgPublicMarkerKey(record.id))
			return err
		}
		return p.writePGPublicMarker(ctx, record.id, prev)
	}
	req := &transport.PostgreSQLPublicAccessCloseRequest{ExpectedBuildCommit: strings.TrimSpace(buildCommit), Port: record.port, Name: record.name,
		ExpectACL: entry.ACL, ExpectRoles: nonNilStrings(entry.Roles), Grants: entry.Grants, NoOtherSessions: by == pgPublicByPanel, Recorded: nonNilStrings(record.userIn)}
	var resp transport.PostgreSQLPublicAccessCloseResponse
	err := pgPublicAgent(p, ctx, "Agent.PostgreSQLPublicAccessCloseV1", req, &resp)
	switch {
	case err != nil:
		log.Printf("postgresql databases: %s: the close's answer was lost; the database is read again: %v", record.name, err)
		return p.pgPublicSettle(ctx, record, marker)
	case resp.BuildMismatch:
		return "mismatch", prev, putBack()
	case resp.Closed && resp.Change != nil:
		marker.State, marker.BeforeACL, marker.AfterACL, marker.Added = pgPublicClosed, resp.Change.BeforeACL, resp.Change.AfterACL, resp.Change.Added
		marker.PublicConnect, marker.PublicTemporary = pgPublicRights(resp.Change.BeforeACL)
		marker.Grants = nil
		if err := p.writePGPublicMarker(ctx, record.id, marker); err != nil {
			log.Printf("postgresql databases: %s was closed, but its record could not be written: %v", record.name, err)
		}
		if by == pgPublicByPanel {
			// An administrator's close is audited with the request.
			p.recordAudit(auditRecord{Action: "database.postgresql.closed_to_other_accounts:" + record.name, ResourceType: "database", ResourceID: record.id})
		}
		return pgPublicClosed, marker, nil
	case resp.Changed:
		return "changed", prev, putBack()
	case resp.Unknown:
		marker.Failure = boundedAgentDiagnostic(resp.Error)
		if err := p.writePGPublicMarker(ctx, record.id, marker); err != nil {
			return "unknown", marker, err
		}
		return p.pgPublicSettle(ctx, record, marker)
	}
	// Verified: nothing was changed; the failure is kept with the facts it
	// failed on.
	failed := &pgPublicMarker{State: pgPublicWatch}
	if prev != nil {
		copied := *prev
		failed = &copied
	}
	failed.FailedAt, failed.FailedBy, failed.FailedFingerprint = pgPublicNow().UTC().Format(time.RFC3339), by, fingerprint
	failed.Failure = boundedAgentDiagnostic(resp.Error)
	log.Printf("postgresql databases: %s was not closed: %s", record.name, failed.Failure)
	return "failed", failed, p.writePGPublicMarker(ctx, record.id, failed)
}

// pgPublicSettle reads the one database again at once and settles a
// "sending" record from it.
func (p *Panel) pgPublicSettle(ctx context.Context, record pgPublicRecord, marker *pgPublicMarker) (string, *pgPublicMarker, error) {
	facts, _, _, err := p.pgPublicScan(ctx, []pgPublicRecord{record})
	f, ok := facts[record.id]
	if err != nil || !ok {
		return "unknown", marker, nil
	}
	settled, outcome, err := p.pgPublicReconcile(ctx, record, marker, f)
	return outcome, settled, err
}

// pgPublicRights is what PUBLIC held in a database's rights: the untouched
// default ("") gives it CONNECT and TEMPORARY.
func pgPublicRights(acl string) (connect, temporary bool) {
	if strings.TrimSpace(acl) == "" {
		return true, true
	}
	public := pgACLPrivileges(acl)[""]
	return strings.Contains(public, "c"), strings.Contains(public, "T")
}

// pgPublicAddedFrom is what a close gave that the database's rights before
// did not hold (a close whose answer was lost).
func pgPublicAddedFrom(beforeACL string, grants []transport.PostgreSQLPublicAccessGrant) []string {
	had := pgACLPrivileges(beforeACL)
	added := []string{}
	for _, grant := range grants {
		if !strings.Contains(had[grant.Role], "c") {
			added = append(added, grant.Role+":CONNECT")
		}
		if grant.Temporary && !strings.Contains(had[grant.Role], "T") {
			added = append(added, grant.Role+":TEMPORARY")
		}
	}
	sort.Strings(added)
	return added
}

// pgACLPrivileges reads a database's rights as PostgreSQL prints them
// ({=Tc/owner,"a b"=c/owner}): the privilege letters of each grantee
// ("" is PUBLIC).
func pgACLPrivileges(acl string) map[string]string {
	out := map[string]string{}
	acl = strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(acl), "{"), "}")
	for _, item := range splitACLItems(acl) {
		item = strings.TrimSpace(item)
		if strings.HasPrefix(item, `"`) && strings.HasSuffix(item, `"`) && len(item) > 1 {
			// An item holding a quote or a comma is quoted as a whole.
			item = strings.ReplaceAll(item[1:len(item)-1], `\"`, `"`)
		}
		grantee, rest, ok := pgACLCut(item)
		if !ok {
			continue
		}
		privileges, _, _ := strings.Cut(rest, "/")
		out[grantee] += privileges
	}
	return out
}

// pgACLCut splits one item at the "=" after its grantee; a grantee with
// special characters is quoted, with "" for a quote inside.
func pgACLCut(item string) (grantee, rest string, ok bool) {
	if !strings.HasPrefix(item, `"`) {
		return strings.Cut(item, "=")
	}
	var name strings.Builder
	for i := 1; i < len(item); i++ {
		if item[i] != '"' {
			name.WriteByte(item[i])
			continue
		}
		if i+1 < len(item) && item[i+1] == '"' {
			name.WriteByte('"')
			i++
			continue
		}
		if i+1 < len(item) && item[i+1] == '=' {
			return name.String(), item[i+2:], true
		}
		return "", "", false
	}
	return "", "", false
}

func splitACLItems(acl string) []string {
	var items []string
	var current strings.Builder
	quoted, escaped := false, false
	for _, r := range acl {
		switch {
		case escaped:
			escaped = false
			current.WriteRune(r)
		case r == '\\' && quoted:
			escaped = true
			current.WriteRune(r)
		case r == '"':
			quoted = !quoted
			current.WriteRune(r)
		case r == ',' && !quoted:
			items = append(items, current.String())
			current.Reset()
		default:
			current.WriteRune(r)
		}
	}
	if current.Len() > 0 {
		items = append(items, current.String())
	}
	return items
}

func pgPublicClosedEntry(record pgPublicRecord, marker *pgPublicMarker) pgPublicEntry {
	return pgPublicEntry{DatabaseID: record.id, Name: record.name, Port: record.port, State: pgPublicClosed, By: marker.By, At: marker.At,
		ACL: marker.AfterACL, UndoStatements: pgPublicUndoStatements(marker)}
}

// pgPublicUndoStatements are the statements an owner without the panel runs
// (as the postgres account) to undo one close.
func pgPublicUndoStatements(marker *pgPublicMarker) []string {
	ident := `"` + strings.ReplaceAll(marker.Name, `"`, `""`) + `"`
	var public []string
	if marker.PublicConnect {
		public = append(public, "CONNECT")
	}
	if marker.PublicTemporary {
		public = append(public, "TEMPORARY")
	}
	var out []string
	if len(public) > 0 {
		out = append(out, "GRANT "+strings.Join(public, ", ")+" ON DATABASE "+ident+" TO PUBLIC;")
	}
	for _, entry := range marker.Added {
		role, privilege, ok := strings.Cut(entry, ":")
		if ok {
			out = append(out, "REVOKE "+privilege+" ON DATABASE "+ident+` FROM "`+strings.ReplaceAll(role, `"`, `""`)+`";`)
		}
	}
	return out
}

// storePGPublicList keeps the last scan's databases the administrator must
// decide on, for Problems now.
func (p *Panel) storePGPublicList(ctx context.Context, view pgPublicView) {
	raw, err := json.Marshal(struct {
		CheckedAt string   `json:"checked_at"`
		Open      []string `json:"open"`
	}{view.CheckedAt, pgPublicNames(view.Open)})
	if err != nil {
		return
	}
	if _, err := p.db.GetDB().ExecContext(ctx, `INSERT INTO panel_settings (key, value, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP`, pgPublicListKey, string(raw)); err != nil {
		log.Printf("postgresql databases: the list could not be kept: %v", err)
	}
}

// refreshPGPublicList scans again after an administrator's action (no
// close), so the page and Problems now say what holds after it.
func (p *Panel) refreshPGPublicList(ctx context.Context) {
	if view, err := p.pgPublicRead(ctx, pgPublicNow(), pgPublicPass{write: true}); err == nil {
		p.storePGPublicList(ctx, view)
	}
}

// pgPublicNames: the databases the administrator must decide on; one that
// is only watched (it closes by itself) is not among them, nor one an
// administrator opened again (the reopened list).
func pgPublicNames(entries []pgPublicEntry) []string {
	names := []string{}
	for _, entry := range entries {
		if len(entry.Found) == 1 && entry.Found[0] == "observing" {
			continue
		}
		names = append(names, entry.Name)
	}
	return names
}

// pgPublicProblems is the Problems-now line: databases left open to other
// accounts, from the last scan (read only).
func pgPublicProblems(ctx context.Context, db *sql.DB) []monitoringProblem {
	var raw string
	if err := db.QueryRowContext(ctx, `SELECT value FROM panel_settings WHERE key = ?`, pgPublicListKey).Scan(&raw); err != nil {
		return nil
	}
	var list struct {
		CheckedAt string   `json:"checked_at"`
		Open      []string `json:"open"`
	}
	if json.Unmarshal([]byte(raw), &list) != nil || len(list.Open) == 0 {
		return nil
	}
	return []monitoringProblem{{Key: "postgresql_databases_open", Kind: "postgresql_databases_open", Severity: "warning", Actor: "administrator",
		Value: float64(len(list.Open)), Name: strings.Join(list.Open, ", "), ObservedAt: list.CheckedAt,
		Action: monitoringProblemAction{Kind: "open", To: "/components/postgresql#databases-open"}}}
}

// handlePostgreSQLPublicAccess: GET the view of the last scan (it changes
// nothing on the engine; ?fresh=1, or no scan yet, starts one, joining one
// that runs); POST {action: close|undo, database_id, acl, roles} for one
// database, administrator only.
func (p *Panel) handlePostgreSQLPublicAccess(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	c := currentCaller(r)
	if c == nil || c.APIToken != nil || !c.hasAccountRole(roleAdmin) {
		writeCodedError(w, http.StatusForbidden, errCodeAdminOnly, "administrator access required", "")
		return
	}
	switch r.Method {
	case http.MethodGet:
		var view pgPublicView
		var err error
		last := p.pgPublicCachedScan()
		if last == nil || r.URL.Query().Get("fresh") == "1" {
			notBefore := time.Time{}
			if last != nil {
				notBefore = last.started.Add(time.Nanosecond)
			}
			view, err = p.pgPublicRead(r.Context(), notBefore, pgPublicPass{write: true})
			if err == nil {
				p.storePGPublicList(r.Context(), view)
			}
		} else {
			view, err = p.pgPublicBuild(r.Context(), last, pgPublicPass{})
		}
		if errors.Is(err, errGrantFormsBuildMismatch) {
			writeCodedError(w, http.StatusServiceUnavailable, errCodePGPublicWaiting, "The panel and its Agent are different builds while an update finishes; read again in a minute. Nothing was changed.", "")
			return
		}
		if err != nil {
			writeServerErrorFor(w, r, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(view)
	case http.MethodPost:
		p.handlePostgreSQLPublicAccessAction(w, r)
	default:
		rejectRouteMethod(w, []string{http.MethodGet, http.MethodPost})
	}
}

const (
	errCodePGPublicWaiting = "POSTGRESQL_PUBLIC_WAITING"
	errCodePGPublicChanged = "POSTGRESQL_PUBLIC_CHANGED"
	errCodePGPublicFailed  = "POSTGRESQL_PUBLIC_FAILED"
	errCodePGPublicUnknown = "POSTGRESQL_PUBLIC_UNKNOWN"
)

func (p *Panel) handlePostgreSQLPublicAccessAction(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Action     string   `json:"action"`
		DatabaseID int      `json:"database_id"`
		ACL        string   `json:"acl"`
		Roles      []string `json:"roles"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&body); err != nil || body.DatabaseID < 1 || (body.Action != "close" && body.Action != "undo") {
		writeClientError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	ctx := r.Context()
	records, err := p.pgPublicRecords(ctx)
	if err != nil {
		writeServerErrorFor(w, r, err)
		return
	}
	var record *pgPublicRecord
	for i := range records {
		if records[i].id == body.DatabaseID {
			record = &records[i]
		}
	}
	if record == nil {
		writeClientError(w, http.StatusNotFound, "database not found")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	// One action at a time; the round waits, and reads this one's result.
	pgPublicActMu.Lock()
	status, answer := p.pgPublicAct(ctx, *record, body.Action, body.ACL, body.Roles)
	pgPublicActMu.Unlock()
	if status == http.StatusOK && answer.did {
		p.audit(r, map[string]string{"close": "database.postgresql.closed_to_other_accounts:", "undo": "database.postgresql.reopened_to_other_accounts:"}[body.Action]+record.name, "database", record.id)
	}
	// Whatever came of it, the list is read again (it says what holds now).
	p.refreshPGPublicList(ctx)
	answer.write(w)
}

// pgPublicAnswer is an action's answer, written after pgPublicActMu is
// released; did: this request made the change (it is audited).
type pgPublicAnswer struct {
	did   bool
	write func(http.ResponseWriter)
}

type pgPublicCoded = func(int, string, string, ...string) (int, pgPublicAnswer)

// pgPublicAct does one administrator action under pgPublicActMu.
func (p *Panel) pgPublicAct(ctx context.Context, record pgPublicRecord, action, acl string, roles []string) (int, pgPublicAnswer) {
	ok := func(status string, did bool) (int, pgPublicAnswer) {
		return http.StatusOK, pgPublicAnswer{did: did, write: func(w http.ResponseWriter) { _ = json.NewEncoder(w).Encode(map[string]any{"status": status}) }}
	}
	coded := func(status int, code, message string, details ...string) (int, pgPublicAnswer) {
		return status, pgPublicAnswer{write: func(w http.ResponseWriter) {
			if len(details) > 0 {
				writeCodedErrorDetails(w, status, code, message, "", details)
				return
			}
			writeCodedError(w, status, code, message, "")
		}}
	}
	serverError := func(err error) (int, pgPublicAnswer) {
		log.Printf("postgresql databases: %s: %v", record.name, err)
		return coded(http.StatusInternalServerError, "INTERNAL_ERROR", "The panel could not keep its record of this database. Read the list again.")
	}
	marker, err := p.readPGPublicMarker(ctx, record.id)
	if err != nil {
		return serverError(err)
	}
	facts, _, _, err := p.pgPublicScan(ctx, []pgPublicRecord{record})
	if errors.Is(err, errGrantFormsBuildMismatch) {
		return coded(http.StatusServiceUnavailable, errCodePGPublicWaiting, "The panel and its Agent are different builds while an update finishes; try again in a minute. Nothing was changed.")
	}
	f, read := facts[record.id]
	if err != nil || !read || f.Error != "" || !f.Exists {
		return coded(http.StatusBadGateway, errCodePGPublicUnknown, "The database could not be read now, so nothing was changed. Read the list again, then decide.")
	}
	// A close whose answer was lost is settled first.
	if marker != nil && marker.State == pgPublicSending {
		if marker, _, err = p.pgPublicReconcile(ctx, record, marker, f); err != nil {
			return serverError(err)
		}
	}
	publicClosed := !f.PublicConnect && !f.PublicTemporary
	if action == "undo" {
		if marker == nil || marker.State != pgPublicClosed {
			return coded(http.StatusConflict, errCodePGPublicChanged, "CelikPanel did not close this database, so there is nothing of its own to undo. Nothing was changed.")
		}
		return p.pgPublicUndo(ctx, record, marker, coded, serverError)
	}
	if publicClosed {
		// Closed already: by the round just now, or another administrator.
		return ok("closed", false)
	}
	entry := pgPublicEntry{ACL: acl, Roles: roles}
	for _, user := range record.userIn {
		if user != f.Owner {
			entry.Grants = append(entry.Grants, transport.PostgreSQLPublicAccessGrant{Role: user, Temporary: record.users[user]})
		}
	}
	outcome, closed, err := p.pgPublicCloseBy(ctx, record, entry, pgPublicByAdministrator, marker, pgPublicFingerprint(record, f))
	if err != nil {
		return serverError(err)
	}
	switch outcome {
	case pgPublicClosed:
		return ok("closed", true)
	case "mismatch":
		return coded(http.StatusServiceUnavailable, errCodePGPublicWaiting, "The panel and its Agent are different builds while an update finishes; try again in a minute. Nothing was changed.")
	case "changed":
		return coded(http.StatusConflict, errCodePGPublicChanged, "The database was not closed: it is no longer as the list showed it (its rights or the roles inside it changed). Nothing was changed. Read the list again, then decide.")
	case "failed":
		return coded(http.StatusBadGateway, errCodePGPublicFailed, "The database server did not close it. Nothing was changed. The reason is below; it stays on the list with it.", closed.Failure)
	}
	return coded(http.StatusBadGateway, errCodePGPublicUnknown, "The server's answer was lost, so whether the database was closed is not known. CelikPanel keeps its record and reads the database again; the list shows what it holds.")
}

// pgPublicUndo undoes one close; its record is never deleted (an unknown
// answer is settled from the engine).
func (p *Panel) pgPublicUndo(ctx context.Context, record pgPublicRecord, marker *pgPublicMarker, coded pgPublicCoded, serverError func(error) (int, pgPublicAnswer)) (int, pgPublicAnswer) {
	req := &transport.PostgreSQLPublicAccessUndoRequest{ExpectedBuildCommit: strings.TrimSpace(buildCommit), Port: record.port, Name: record.name,
		ExpectACL: marker.AfterACL, PublicConnect: marker.PublicConnect, PublicTemporary: marker.PublicTemporary, Added: marker.Added}
	var resp transport.PostgreSQLPublicAccessUndoResponse
	err := pgPublicAgent(p, ctx, "Agent.PostgreSQLPublicAccessUndoV1", req, &resp)
	undone, after := resp.Undone, resp.AfterACL
	switch {
	case err == nil && resp.BuildMismatch:
		return coded(http.StatusServiceUnavailable, errCodePGPublicWaiting, "The panel and its Agent are different builds while an update finishes; try again in a minute. Nothing was changed.")
	case err == nil && resp.Changed:
		return coded(http.StatusConflict, errCodePGPublicChanged, "The database's rights were changed since CelikPanel closed it, so the undo would not be exact. Nothing was changed. The statements under the database say what the close changed.")
	case err == nil && !resp.Undone && !resp.Unknown:
		return coded(http.StatusBadGateway, errCodePGPublicFailed, "The database server did not open it again. Nothing was changed. The reason is below.", boundedAgentDiagnostic(resp.Error))
	case err != nil || resp.Unknown:
		// Read what the database holds now.
		facts, _, _, scanErr := p.pgPublicScan(ctx, []pgPublicRecord{record})
		f, read := facts[record.id]
		switch {
		case scanErr != nil || !read || f.Error != "" || !f.Exists:
		case f.PublicConnect == marker.PublicConnect && f.PublicTemporary == marker.PublicTemporary && f.ACL != marker.AfterACL:
			undone, after = true, f.ACL
		case f.ACL == marker.AfterACL:
			return coded(http.StatusBadGateway, errCodePGPublicFailed, "The database was not opened again: the server's answer was lost and the database is unchanged. Nothing was changed; try again.")
		}
	}
	if !undone {
		return coded(http.StatusBadGateway, errCodePGPublicUnknown, "The server did not answer, so whether the database was opened again is not known. CelikPanel keeps its record of the close; the list shows what the database holds now.")
	}
	marker.State, marker.UndoneAt, marker.AfterACL = pgPublicUndone, pgPublicNow().UTC().Format(time.RFC3339), after
	if err := p.writePGPublicMarker(ctx, record.id, marker); err != nil {
		return serverError(err)
	}
	return http.StatusOK, pgPublicAnswer{did: true, write: func(w http.ResponseWriter) { _ = json.NewEncoder(w).Encode(map[string]any{"status": "undone"}) }}
}
