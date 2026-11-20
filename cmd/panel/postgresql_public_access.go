package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
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
// (reached through the Agent), for the databases CelikPanel records:
//
//   - By itself only when all of this holds: the database's own rights are
//     still PostgreSQL's untouched default (nothing was ever granted or
//     revoked on it), nothing inside it belongs to or is granted to a role
//     outside CelikPanel's records for it, no such role is connected to it,
//     and no monitoring role (a login role in pg_monitor and the like) exists
//     on its cluster. Then, in one transaction inside the database, CelikPanel
//     gives its recorded users CONNECT (and TEMPORARY to a read-and-write
//     user), takes every right from PUBLIC and reads back the result; the
//     transaction changes nothing unless the database is still as it was
//     read. The administrator sees which databases were closed, when, and an
//     undo (the panel's own, and the plain statements without the panel).
//   - Everything else is left exactly as it is and listed (Problems now, and
//     the PostgreSQL page) with what was found and what closing would
//     change; the administrator closes it there after reading that, and can
//     undo it the same way. An undone database is never closed again by
//     itself.
//
// When: the database-rights loop's admission (database_grant_forms.go): not
// within the first minutes after a start, not before an update of the panel
// is verified, not while the server is busy, only with an Agent of the same
// build; then hourly. A cluster that does not answer is read again on the
// next round. Reading never changes anything; each database is decided on
// its own. Kept in panel_settings rows (no schema change):
// "postgresql_public_v1:<database id>" for each database CelikPanel closed
// (or is closing) with what the close changed, and
// "postgresql_public_list_v1" for the last round's list of databases left
// open.
//
// alpha.141'den önce açılan PostgreSQL veritabanları başka hesaplara kapatılır:
// kendiliğinden yalnız dokunulmamış varsayılan haklarda ve içinde kayıt dışı
// rol yokken; diğerleri yöneticiye listelenir. Her kapatma geri alınabilir.

const (
	pgPublicMarkerPrefix = "postgresql_public_v1:"
	pgPublicListKey      = "postgresql_public_list_v1"
	pgPublicPath         = "/api/v1/admin/postgresql/public-access"

	pgPublicSending = "sending"
	pgPublicClosed  = "closed"
	pgPublicUndone  = "undone"

	pgPublicByPanel         = "panel"
	pgPublicByAdministrator = "administrator"
)

var (
	// pgPublicAgent calls the Agent (a seam for tests).
	pgPublicAgent = func(p *Panel, ctx context.Context, method string, req, resp any) error {
		return p.callAgentContext(ctx, method, req, resp)
	}
	pgPublicNow = time.Now
)

// pgPublicMarker is what CelikPanel did to one database.
type pgPublicMarker struct {
	Version int    `json:"version"`
	State   string `json:"state"`
	By      string `json:"by"`
	Port    int    `json:"port"`
	Name    string `json:"name"`
	// The database's rights before and after the close, PUBLIC's rights
	// before it and the rights the close gave (an undo takes exactly these).
	BeforeACL       string   `json:"before_acl"`
	AfterACL        string   `json:"after_acl"`
	PublicConnect   bool     `json:"public_connect"`
	PublicTemporary bool     `json:"public_temporary"`
	Added           []string `json:"added"`
	// Grants: what the close was asked to give (kept while it is sent).
	Grants   []transport.PostgreSQLPublicAccessGrant `json:"grants,omitempty"`
	At       string                                  `json:"at"`
	UndoneAt string                                  `json:"undone_at,omitempty"`
}

// pgPublicEntry is one database on the administrator's view.
type pgPublicEntry struct {
	DatabaseID int    `json:"database_id"`
	Name       string `json:"name"`
	Port       int    `json:"port"`
	// State: closed (by the panel or the administrator), open (left as it
	// is: Found says why), unknown (its cluster did not answer).
	State string `json:"state"`
	By    string `json:"by,omitempty"`
	At    string `json:"at,omitempty"`
	Owner string `json:"owner,omitempty"`
	ACL   string `json:"acl,omitempty"`
	// Found: why it was left open (changed_rights, other_roles,
	// other_sessions, monitoring_roles, undone).
	Found      []string `json:"found,omitempty"`
	OtherRoles []string `json:"other_roles,omitempty"`
	Sessions   []string `json:"sessions,omitempty"`
	Monitoring []string `json:"monitoring,omitempty"`
	// Roles: every role inside it as read (sent back with a close, which
	// refuses when they changed). Grants: what a close would give.
	Roles  []string                                `json:"roles,omitempty"`
	Grants []transport.PostgreSQLPublicAccessGrant `json:"grants,omitempty"`
	// UndoStatements: the plain statements of an undo, for an owner without
	// the panel (run as the postgres account).
	UndoStatements []string `json:"undo_statements,omitempty"`
	Reason         string   `json:"reason,omitempty"`
}

type pgPublicView struct {
	CheckedAt string          `json:"checked_at,omitempty"`
	Open      []pgPublicEntry `json:"open"`
	Closed    []pgPublicEntry `json:"closed"`
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
	_, err = p.db.GetDB().ExecContext(ctx, `INSERT INTO panel_settings (key, value, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP`, pgPublicMarkerKey(id), string(raw))
	return err
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

// pgPublicScan reads every cluster's recorded databases (read only).
func (p *Panel) pgPublicScan(ctx context.Context, records []pgPublicRecord) (map[int]transport.PostgreSQLPublicAccessFacts, map[int][]string, map[int]string, error) {
	byPort := map[int][]pgPublicRecord{}
	for _, record := range records {
		byPort[record.port] = append(byPort[record.port], record)
	}
	facts := map[int]transport.PostgreSQLPublicAccessFacts{}
	monitoring := map[int][]string{}
	unread := map[int]string{}
	for port, list := range byPort {
		req := &transport.PostgreSQLPublicAccessScanRequest{ExpectedBuildCommit: strings.TrimSpace(buildCommit), Port: port}
		for _, record := range list {
			req.Databases = append(req.Databases, transport.PostgreSQLPublicAccessDatabase{Name: record.name, Recorded: record.userIn})
		}
		var resp transport.PostgreSQLPublicAccessScanResponse
		err := pgPublicAgent(p, ctx, "Agent.PostgreSQLPublicAccessScanV1", req, &resp)
		switch {
		case err != nil:
			unread[port] = "the Agent did not answer"
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
		monitoring[port] = resp.Monitoring
		for i, record := range list {
			if i < len(resp.Databases) && resp.Databases[i].Name == record.name {
				facts[record.id] = resp.Databases[i]
			}
		}
	}
	return facts, monitoring, unread, nil
}

// pgPublicDecide is what one database's facts say.
func pgPublicDecide(record pgPublicRecord, f transport.PostgreSQLPublicAccessFacts, monitoring []string, marker *pgPublicMarker) pgPublicEntry {
	entry := pgPublicEntry{DatabaseID: record.id, Name: record.name, Port: record.port, Owner: f.Owner, ACL: f.ACL,
		Roles: append([]string{}, f.Roles...)}
	for _, role := range f.Roles {
		if !containsString(record.userIn, role) {
			entry.OtherRoles = append(entry.OtherRoles, role)
		}
	}
	for _, role := range f.Sessions {
		if !containsString(record.userIn, role) {
			entry.Sessions = append(entry.Sessions, role)
		}
	}
	entry.Monitoring = append(entry.Monitoring, monitoring...)
	for _, user := range record.userIn {
		if user != f.Owner {
			entry.Grants = append(entry.Grants, transport.PostgreSQLPublicAccessGrant{Role: user, Temporary: record.users[user]})
		}
	}
	if marker != nil && marker.State == pgPublicUndone {
		entry.Found = append(entry.Found, "undone")
	}
	if f.ACL != "" {
		entry.Found = append(entry.Found, "changed_rights")
	}
	if len(entry.OtherRoles) > 0 {
		entry.Found = append(entry.Found, "other_roles")
	}
	if len(entry.Sessions) > 0 {
		entry.Found = append(entry.Found, "other_sessions")
	}
	if len(entry.Monitoring) > 0 {
		entry.Found = append(entry.Found, "monitoring_roles")
	}
	return entry
}

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

// runPostgreSQLPublicRound reads every recorded database and closes, when
// nothing holds the server, those it may close by itself.
func (p *Panel) runPostgreSQLPublicRound(ctx context.Context, startedAt time.Time) {
	view, err := p.pgPublicView(ctx, p.databaseGrantFormsHold(ctx, startedAt) == "")
	if err != nil {
		if !errors.Is(err, errGrantFormsBuildMismatch) {
			log.Printf("postgresql databases: the rights could not be read: %v", err)
		}
		return
	}
	p.storePGPublicList(ctx, view)
}

// pgPublicView reads every recorded database; with closeNow it closes the
// ones it may close by itself (and finishes a close whose answer was lost).
// Without closeNow it changes nothing.
func (p *Panel) pgPublicView(ctx context.Context, closeNow bool) (pgPublicView, error) {
	view := pgPublicView{Open: []pgPublicEntry{}, Closed: []pgPublicEntry{}, Unknown: []pgPublicEntry{}}
	records, err := p.pgPublicRecords(ctx)
	if err != nil {
		return view, err
	}
	facts, monitoring, unread, err := p.pgPublicScan(ctx, records)
	if err != nil {
		return view, err
	}
	view.CheckedAt = pgPublicNow().UTC().Format(time.RFC3339)
	for _, record := range records {
		marker, err := p.readPGPublicMarker(ctx, record.id)
		if err != nil {
			return view, err
		}
		f, read := facts[record.id]
		if !read || f.Error != "" {
			reason := unread[record.port]
			if read {
				reason = boundedAgentDiagnostic(f.Error)
			}
			if marker != nil && marker.State == pgPublicClosed {
				view.Closed = append(view.Closed, pgPublicClosedEntry(record, marker))
				continue
			}
			view.Unknown = append(view.Unknown, pgPublicEntry{DatabaseID: record.id, Name: record.name, Port: record.port, State: "unknown", Reason: reason})
			continue
		}
		if !f.Exists {
			continue
		}
		entry := pgPublicDecide(record, f, monitoring[record.port], marker)
		switch {
		case marker != nil && marker.State == pgPublicSending:
			// A close whose answer was lost: what the engine shows decides.
			if !f.PublicConnect && !f.PublicTemporary {
				marker.State, marker.AfterACL = pgPublicClosed, f.ACL
				marker.Added = pgPublicAddedFrom(marker.BeforeACL, marker.Grants)
				if err := p.writePGPublicMarker(ctx, record.id, marker); err != nil {
					return view, err
				}
				view.Closed = append(view.Closed, pgPublicClosedEntry(record, marker))
				continue
			}
			if f.ACL == marker.BeforeACL && closeNow {
				if closed := p.pgPublicCloseBy(ctx, record, entry, pgPublicByPanel); closed != nil {
					view.Closed = append(view.Closed, pgPublicClosedEntry(record, closed))
					continue
				}
			}
		case marker != nil && marker.State == pgPublicClosed:
			if !f.PublicConnect && !f.PublicTemporary {
				view.Closed = append(view.Closed, pgPublicClosedEntry(record, marker))
				continue
			}
		}
		if !f.PublicConnect && !f.PublicTemporary {
			continue // closed already (alpha.141 or later, or by its owner)
		}
		if len(entry.Found) == 0 && marker == nil && closeNow {
			if closed := p.pgPublicCloseBy(ctx, record, entry, pgPublicByPanel); closed != nil {
				view.Closed = append(view.Closed, pgPublicClosedEntry(record, closed))
				continue
			}
		}
		entry.State = "open"
		if len(entry.Found) == 0 {
			entry.Found = []string{"waiting"}
		}
		view.Open = append(view.Open, entry)
	}
	sort.SliceStable(view.Open, func(i, j int) bool { return view.Open[i].Name < view.Open[j].Name })
	sort.SliceStable(view.Closed, func(i, j int) bool { return view.Closed[i].At > view.Closed[j].At })
	return view, nil
}

// pgPublicCloseBy closes one database; the marker is written before the
// Agent is asked, so a lost answer is finished from the engine next time.
// nil when it was not closed (the reason is logged and the database stays
// listed).
func (p *Panel) pgPublicCloseBy(ctx context.Context, record pgPublicRecord, entry pgPublicEntry, by string) *pgPublicMarker {
	connect, temporary := pgPublicRights(entry.ACL)
	marker := &pgPublicMarker{State: pgPublicSending, By: by, Port: record.port, Name: record.name, BeforeACL: entry.ACL,
		PublicConnect: connect, PublicTemporary: temporary, Grants: entry.Grants, At: pgPublicNow().UTC().Format(time.RFC3339)}
	if err := p.writePGPublicMarker(ctx, record.id, marker); err != nil {
		log.Printf("postgresql databases: %s: the close could not be recorded first: %v", record.name, err)
		return nil
	}
	req := &transport.PostgreSQLPublicAccessCloseRequest{ExpectedBuildCommit: strings.TrimSpace(buildCommit), Port: record.port, Name: record.name,
		ExpectACL: entry.ACL, ExpectRoles: nonNilStrings(entry.Roles), Grants: entry.Grants, NoOtherSessions: by == pgPublicByPanel, Recorded: nonNilStrings(record.userIn)}
	var resp transport.PostgreSQLPublicAccessCloseResponse
	if err := pgPublicAgent(p, ctx, "Agent.PostgreSQLPublicAccessCloseV1", req, &resp); err != nil {
		log.Printf("postgresql databases: %s: the close's answer was lost; the next round reads the engine: %v", record.name, err)
		return nil
	}
	if !resp.Closed || resp.Change == nil {
		// Verified: nothing was changed. The marker goes again.
		_, _ = p.db.GetDB().ExecContext(context.WithoutCancel(ctx), `DELETE FROM panel_settings WHERE key = ?`, pgPublicMarkerKey(record.id))
		log.Printf("postgresql databases: %s was not closed: %s", record.name, boundedAgentDiagnostic(resp.Error))
		return nil
	}
	marker.State, marker.BeforeACL, marker.AfterACL, marker.Added = pgPublicClosed, resp.Change.BeforeACL, resp.Change.AfterACL, resp.Change.Added
	marker.PublicConnect, marker.PublicTemporary = pgPublicRights(resp.Change.BeforeACL)
	marker.Grants = nil
	if err := p.writePGPublicMarker(ctx, record.id, marker); err != nil {
		log.Printf("postgresql databases: %s was closed, but its record could not be written: %v", record.name, err)
	}
	if by == pgPublicByPanel {
		// An administrator's close is audited with the request.
		p.recordAudit(auditRecord{Action: "database.postgresql.closed_to_other_accounts:" + by, ResourceType: "database", ResourceID: record.id})
	}
	return marker
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

// storePGPublicList keeps the last round's open databases for Problems now.
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

// refreshPGPublicList reads again (changing nothing) after an
// administrator's action, so Problems now does not wait for the next round.
func (p *Panel) refreshPGPublicList(ctx context.Context) {
	if view, err := p.pgPublicView(ctx, false); err == nil {
		p.storePGPublicList(ctx, view)
	}
}

// pgPublicNames: the databases the administrator must decide on; one that
// only waits for the next round (it closes by itself) is not among them.
func pgPublicNames(entries []pgPublicEntry) []string {
	names := []string{}
	for _, entry := range entries {
		if len(entry.Found) == 1 && entry.Found[0] == "waiting" {
			continue
		}
		names = append(names, entry.Name)
	}
	return names
}

// pgPublicProblems is the Problems-now line: databases left open to other
// accounts, from the last round (read only).
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

// handlePostgreSQLPublicAccess: GET the view (read only; it changes
// nothing); POST {action: close|undo, database_id, acl, roles} for one
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
		view, err := p.pgPublicView(r.Context(), false)
		if errors.Is(err, errGrantFormsBuildMismatch) {
			writeCodedError(w, http.StatusServiceUnavailable, errCodePGPublicWaiting, "The panel and its Agent are different builds while an update finishes; read again in a minute. Nothing was changed.", "")
			return
		}
		if err != nil {
			writeServerErrorFor(w, r, err)
			return
		}
		// What was read now is also what Problems now says.
		p.storePGPublicList(r.Context(), view)
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
	marker, err := p.readPGPublicMarker(ctx, record.id)
	if err != nil {
		writeServerErrorFor(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if body.Action == "undo" {
		if marker == nil || marker.State != pgPublicClosed {
			writeCodedError(w, http.StatusConflict, errCodePGPublicChanged, "CelikPanel did not close this database, so there is nothing of its own to undo. Nothing was changed.", "")
			return
		}
		req := &transport.PostgreSQLPublicAccessUndoRequest{ExpectedBuildCommit: strings.TrimSpace(buildCommit), Port: record.port, Name: record.name,
			ExpectACL: marker.AfterACL, PublicConnect: marker.PublicConnect, PublicTemporary: marker.PublicTemporary, Added: marker.Added}
		var resp transport.PostgreSQLPublicAccessUndoResponse
		if err := pgPublicAgent(p, ctx, "Agent.PostgreSQLPublicAccessUndoV1", req, &resp); err != nil {
			writeCodedError(w, http.StatusBadGateway, errCodePGPublicUnknown, "The server did not answer, so whether the database was opened again is not known. Read the list again; it shows what the database holds now.", "")
			return
		}
		if !resp.Undone {
			p.writePGPublicRefusal(w, resp.Changed, resp.BuildMismatch, resp.Error)
			return
		}
		marker.State, marker.UndoneAt, marker.AfterACL = pgPublicUndone, pgPublicNow().UTC().Format(time.RFC3339), resp.AfterACL
		if err := p.writePGPublicMarker(ctx, record.id, marker); err != nil {
			writeServerErrorFor(w, r, err)
			return
		}
		p.audit(r, "database.postgresql.reopened_to_other_accounts", "database", record.id)
		p.refreshPGPublicList(ctx)
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "undone"})
		return
	}
	// The owner is read again (it holds its rights as owner and is given
	// nothing); what the administrator read is checked by the Agent.
	facts, _, _, err := p.pgPublicScan(ctx, []pgPublicRecord{*record})
	f, ok := facts[record.id]
	if errors.Is(err, errGrantFormsBuildMismatch) {
		writeCodedError(w, http.StatusServiceUnavailable, errCodePGPublicWaiting, "The panel and its Agent are different builds while an update finishes; try again in a minute. Nothing was changed.", "")
		return
	}
	if err != nil || !ok || f.Error != "" || !f.Exists {
		writeCodedError(w, http.StatusBadGateway, errCodePGPublicUnknown, "The database could not be read now, so nothing was changed. Read the list again, then decide.", "")
		return
	}
	if !f.PublicConnect && !f.PublicTemporary {
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "closed"})
		return
	}
	entry := pgPublicEntry{ACL: body.ACL, Roles: body.Roles}
	for _, user := range record.userIn {
		if user != f.Owner {
			entry.Grants = append(entry.Grants, transport.PostgreSQLPublicAccessGrant{Role: user, Temporary: record.users[user]})
		}
	}
	closed := p.pgPublicCloseByAdministrator(ctx, *record, entry)
	if closed == nil {
		writeCodedError(w, http.StatusConflict, errCodePGPublicChanged, "The database was not closed: it is no longer as the list showed it (its rights or the roles inside it changed), or the server refused. Nothing was changed. Read the list again, then decide.", "")
		return
	}
	p.audit(r, "database.postgresql.closed_to_other_accounts:administrator", "database", record.id)
	p.refreshPGPublicList(ctx)
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "closed"})
}

// pgPublicCloseByAdministrator: the owner of a database is given nothing
// (it holds its rights as owner); the Agent refuses when it is not as read.
func (p *Panel) pgPublicCloseByAdministrator(ctx context.Context, record pgPublicRecord, entry pgPublicEntry) *pgPublicMarker {
	return p.pgPublicCloseBy(ctx, record, entry, pgPublicByAdministrator)
}

func (p *Panel) writePGPublicRefusal(w http.ResponseWriter, changed, mismatch bool, agentError string) {
	switch {
	case mismatch:
		writeCodedError(w, http.StatusServiceUnavailable, errCodePGPublicWaiting, "The panel and its Agent are different builds while an update finishes; try again in a minute. Nothing was changed.", "")
	case changed:
		writeCodedError(w, http.StatusConflict, errCodePGPublicChanged, "The database's rights were changed since CelikPanel closed it, so the undo would not be exact. Nothing was changed. The statements under the database say what the close changed.", "")
	default:
		writeCodedErrorDetails(w, http.StatusBadGateway, errCodePGPublicFailed, "The database server did not do it. Nothing was changed. The reason is below.", "", []string{boundedAgentDiagnostic(agentError)})
	}
}
