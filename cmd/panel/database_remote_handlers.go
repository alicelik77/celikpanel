package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/dbremote"
	"github.com/alicelik/celikpanel/internal/transport"
)

// The routes of remote access to databases (alpha.129,
// docs/DATABASE-REMOTE-ACCESS.md):
//
//	GET    /api/v1/database-remote/databases/{id}        rules, access, engine state, client settings
//	POST   /api/v1/database-remote/databases/{id}/rules  {user_id, address}
//	DELETE /api/v1/database-remote/rules/{id}
//	GET    /api/v1/database-remote/server                administrator: switch, engines
//	PUT    /api/v1/database-remote/server                administrator: {offered} or {apply: true}
//
// Every route is ownership-checked (a database of another account answers
// "not found"); a team member never reaches them (middleware). Reading never
// changes anything; the changes report their steps (operation_steps.go) and
// are guarded by the request identity when the page sends one.
//
// Veritabanlarına dışarıdan erişimin uçları; sahiplik denetimli, okuma hiçbir
// şeyi değiştirmez.

const databaseRemotePath = "/api/v1/database-remote/"

const (
	errCodeDatabaseRemoteRefused  = "DATABASE_REMOTE_REFUSED"
	errCodeDatabaseRemoteAddress  = "DATABASE_REMOTE_ADDRESS"
	errCodeDatabaseRemoteLimit    = "DATABASE_REMOTE_LIMIT"
	errCodeDatabaseRemoteExists   = "DATABASE_REMOTE_EXISTS"
	errCodeDatabaseRemoteUser     = "DATABASE_REMOTE_USER"
	errCodeDatabaseRemoteNotFound = "DATABASE_REMOTE_NOT_FOUND"
	errCodeDatabaseRemoteAgent    = "DATABASE_REMOTE_AGENT"
)

type databaseRemoteRoute struct {
	kind    string
	id      int
	methods []string
}

func matchDatabaseRemoteRoute(r *http.Request) (databaseRemoteRoute, bool) {
	segments, ok := strictRouteSegments(r, databaseRemotePath)
	if !ok {
		return databaseRemoteRoute{}, false
	}
	switch {
	case len(segments) == 1 && segments[0] == "server":
		return databaseRemoteRoute{kind: "server", methods: []string{http.MethodGet, http.MethodPut}}, true
	case len(segments) == 2 && segments[0] == "databases":
		if id, ok := strictPositiveID(segments[1]); ok {
			return databaseRemoteRoute{kind: "database", id: id, methods: []string{http.MethodGet}}, true
		}
	case len(segments) == 3 && segments[0] == "databases" && segments[2] == "rules":
		if id, ok := strictPositiveID(segments[1]); ok {
			return databaseRemoteRoute{kind: "rules", id: id, methods: []string{http.MethodPost}}, true
		}
	case len(segments) == 2 && segments[0] == "rules":
		if id, ok := strictPositiveID(segments[1]); ok {
			return databaseRemoteRoute{kind: "rule", id: id, methods: []string{http.MethodDelete}}, true
		}
	}
	return databaseRemoteRoute{}, false
}

func (p *Panel) handleDatabaseRemote(w http.ResponseWriter, r *http.Request) {
	route, ok := matchDatabaseRemoteRoute(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if !routeAllows(r.Method, route.methods) {
		rejectRouteMethod(w, route.methods)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	c := currentCaller(r)
	if c == nil || c.isAdditionalUser() {
		writeClientError(w, http.StatusNotFound, "not found")
		return
	}
	switch route.kind {
	case "server":
		p.handleDatabaseRemoteServer(w, r)
	case "database":
		p.handleDatabaseRemoteView(w, r, route.id)
	case "rules":
		p.handleDatabaseRemoteAdd(w, r, route.id)
	case "rule":
		p.handleDatabaseRemoteRemove(w, r, route.id)
	}
}

// databaseRemoteRuleView is one rule on the screen.
type databaseRemoteRuleView struct {
	ID        int                              `json:"id"`
	UserID    int                              `json:"user_id"`
	User      string                           `json:"user"`
	Address   string                           `json:"address"`
	Status    string                           `json:"status"`
	Problem   *transport.DatabaseRemoteProblem `json:"problem,omitempty"`
	CreatedAt string                           `json:"created_at"`
}

// databaseRemoteEngineView is the engine instance as the Agent reads it now.
type databaseRemoteEngineView struct {
	Known        bool                              `json:"known"`
	State        string                            `json:"state,omitempty"`
	Unit         string                            `json:"unit,omitempty"`
	TLS          string                            `json:"tls,omitempty"`
	SlowDown     string                            `json:"slow_down,omitempty"`
	ListenRemote bool                              `json:"listen_remote"`
	FilterReady  bool                              `json:"filter_ready"`
	NFTAvailable bool                              `json:"nft_available"`
	Problems     []transport.DatabaseRemoteProblem `json:"problems"`
	ReadError    string                            `json:"read_error,omitempty"`
	LastError    string                            `json:"last_error,omitempty"`
	AppliedAt    string                            `json:"applied_at,omitempty"`
}

type databaseRemoteUserView struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
}

// engineView reads the instance (read only) and adds the last apply's
// instance problems.
func (p *Panel) databaseRemoteEngineView(r *http.Request, engine string, port int) databaseRemoteEngineView {
	view := databaseRemoteEngineView{Problems: []transport.DatabaseRemoteProblem{}}
	status, err := databaseRemoteStatusFromAgent(p, r.Context(), transport.DatabaseRemoteStatusRequest{Targets: []transport.DatabaseRemoteTarget{{Engine: engine, Port: port}}})
	switch {
	case err != nil:
		view.ReadError = "the server could not be asked"
	case status.Error != "":
		view.ReadError = status.Error
	default:
		view.NFTAvailable = status.NFTAvailable
		for _, filtered := range status.FilterPorts {
			if filtered == port {
				view.FilterReady = true
			}
		}
		for _, t := range status.Targets {
			if t.Engine == engine && t.Port == port {
				view.Known = true
				view.State, view.Unit, view.TLS, view.SlowDown, view.ListenRemote = t.State, t.Unit, t.TLS, t.SlowDown, t.ListenRemote
				view.Problems = append(view.Problems, t.Problems...)
			}
		}
	}
	last, known, appliedAt, lastError := databaseRemoteLast(engine, port)
	if known {
		if view.TLS == "" || view.TLS == "off" {
			view.TLS = last.TLS
		}
		if view.SlowDown == "" || view.SlowDown == "none" {
			view.SlowDown = last.SlowDown
		}
		for _, problem := range last.Problems {
			if problem.User == "" && problem.Code != transport.DatabaseRemoteEngineDown && problem.Code != transport.DatabaseRemoteOwnerListen {
				view.Problems = append(view.Problems, problem)
			}
		}
	}
	if !appliedAt.IsZero() {
		view.AppliedAt = appliedAt.UTC().Format(time.RFC3339)
	}
	view.LastError = lastError
	return view
}

func (p *Panel) handleDatabaseRemoteView(w http.ResponseWriter, r *http.Request, databaseID int) {
	ctx := r.Context()
	c := currentCaller(r)
	d, err := p.databaseRemoteDatabaseFor(ctx, c, databaseID)
	if errors.Is(err, errDatabaseRemoteNotFound) {
		writeCodedError(w, http.StatusNotFound, errCodeDatabaseRemoteNotFound, "database not found", "")
		return
	}
	if err != nil {
		writeServerError(w, err)
		return
	}
	access, err := p.databaseRemoteAccessFor(ctx, c, d)
	if err != nil {
		writeServerError(w, err)
		return
	}
	rows, err := p.databaseRemoteRows(ctx, "WHERE r.database_id = ?", d.ID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	offered, err := p.databaseRemoteOffered(ctx)
	if err != nil {
		writeServerError(w, err)
		return
	}
	rights := map[int]*effectiveRights{}
	rules := []databaseRemoteRuleView{}
	var createdAt = map[int]string{}
	if err := func() error {
		list, err := p.db.GetDB().QueryContext(ctx, `SELECT id, created_at FROM database_remote_rules WHERE database_id = ?`, d.ID)
		if err != nil {
			return err
		}
		defer list.Close()
		for list.Next() {
			var id int
			var at string
			if err := list.Scan(&id, &at); err != nil {
				return err
			}
			createdAt[id] = at
		}
		return list.Err()
	}(); err != nil {
		writeServerError(w, err)
		return
	}
	for _, row := range rows {
		reason, err := p.databaseRemoteIneligible(ctx, row, offered, rights)
		if err != nil {
			writeServerError(w, err)
			return
		}
		status, problem := databaseRemoteRuleStatus(row, reason)
		address := row.Address
		if prefix, err := netip.ParsePrefix(row.Address); err == nil {
			address = dbremote.DisplayAddress(prefix)
		}
		rules = append(rules, databaseRemoteRuleView{ID: row.ID, UserID: row.UserID, User: row.User, Address: address,
			Status: status, Problem: problem, CreatedAt: createdAt[row.ID]})
	}
	users := []databaseRemoteUserView{}
	list, err := p.db.GetDB().QueryContext(ctx, `
		SELECT u.id, u.username FROM database_user_grants g JOIN database_users u ON u.id = g.user_id
		WHERE g.database_id = ? ORDER BY u.username`, d.ID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	for list.Next() {
		var u databaseRemoteUserView
		if err := list.Scan(&u.ID, &u.Username); err != nil {
			list.Close()
			writeServerError(w, err)
			return
		}
		users = append(users, u)
	}
	list.Close()
	answer := map[string]any{
		"database_id":        d.ID,
		"database":           d.Name,
		"engine":             d.Engine,
		"access":             access,
		"administrator":      c.hasAccountRole(roleAdmin),
		"rules":              rules,
		"users":              users,
		"client":             p.databaseRemoteClientFor(ctx, d),
		"max_addresses_user": dbremote.MaxAddressesPerUser,
	}
	// The address this page was opened from, offered as a one-click rule
	// when it is one that may be listed.
	if mine, err := dbremote.ParseAddress(d.Engine, clientIP(r)); err == nil {
		answer["your_address"] = dbremote.DisplayAddress(mine)
	}
	if offered && localDatabaseHost(d.Host) && (len(rules) > 0 || access.MayEdit) {
		answer["engine_state"] = p.databaseRemoteEngineView(r, d.Engine, d.Port)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(answer)
}

func (p *Panel) handleDatabaseRemoteAdd(w http.ResponseWriter, r *http.Request, databaseID int) {
	ctx := r.Context()
	reportOperationPlan(ctx, "check", "save", "apply", "firewall")
	reportOperationStep(ctx, "check")
	c := currentCaller(r)
	var request struct {
		UserID  int    `json:"user_id"`
		Address string `json:"address"`
	}
	if err := decodeStrictJSON(w, r, &request); err != nil {
		writeClientError(w, http.StatusBadRequest, "send {\"user_id\": <id>, \"address\": \"203.0.113.7\"}; nothing was changed")
		return
	}
	d, err := p.databaseRemoteDatabaseFor(ctx, c, databaseID)
	if errors.Is(err, errDatabaseRemoteNotFound) {
		writeCodedError(w, http.StatusNotFound, errCodeDatabaseRemoteNotFound, "database not found", "")
		return
	}
	if err != nil {
		writeServerError(w, err)
		return
	}
	access, err := p.databaseRemoteAccessFor(ctx, c, d)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if !access.MayEdit {
		writeCodedErrorDetails(w, http.StatusForbidden, errCodeDatabaseRemoteRefused, databaseRemoteRefusalSentence(access), "", []string{access.Reason, access.Grantor})
		return
	}
	// The user must be one of this database's own users (same subscription).
	var username string
	err = p.db.GetDB().QueryRowContext(ctx, `
		SELECT u.username FROM database_user_grants g JOIN database_users u ON u.id = g.user_id
		JOIN databases_v2 d ON d.id = g.database_id
		WHERE g.database_id = ? AND u.id = ? AND u.subscription_id = d.subscription_id`, d.ID, request.UserID).Scan(&username)
	if errors.Is(err, sql.ErrNoRows) {
		writeCodedError(w, http.StatusBadRequest, errCodeDatabaseRemoteUser, "choose a user of this database; nothing was changed", "")
		return
	}
	if err != nil {
		writeServerError(w, err)
		return
	}
	address, err := dbremote.CanonicalAddress(d.Engine, request.Address)
	var refusal *dbremote.AddressError
	if errors.As(err, &refusal) {
		writeCodedErrorDetails(w, http.StatusBadRequest, errCodeDatabaseRemoteAddress, refusal.Message+" Nothing was changed.", "", []string{refusal.Code})
		return
	}
	if err != nil {
		writeServerError(w, err)
		return
	}
	if _, err := (dbremote.Target{Engine: d.Engine, Port: d.Port, Rules: []dbremote.Rule{{Database: d.Name, User: username, Address: address}}}).Normalize(); err != nil {
		writeCodedError(w, http.StatusBadRequest, errCodeDatabaseRemoteUser, "this user cannot be reached from outside: "+err.Error()+"; nothing was changed", "")
		return
	}
	reportOperationStep(ctx, "save")
	// The limit is checked by the insert itself (review L3): one statement,
	// so two requests at the same moment cannot both pass a count taken
	// before either wrote.
	result, err := p.db.GetDB().ExecContext(ctx, `
		INSERT INTO database_remote_rules (database_id, user_id, address, created_by)
		SELECT ?, ?, ?, ?
		WHERE (SELECT COUNT(DISTINCT address) FROM database_remote_rules WHERE user_id = ? AND address <> ?) < ?`,
		d.ID, request.UserID, address, c.ID, request.UserID, address, dbremote.MaxAddressesPerUser)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			writeCodedError(w, http.StatusConflict, errCodeDatabaseRemoteExists, "this address is already listed for this user and database; nothing was changed", "")
			return
		}
		writeServerError(w, err)
		return
	}
	if inserted, err := result.RowsAffected(); err != nil || inserted == 0 {
		writeCodedError(w, http.StatusConflict, errCodeDatabaseRemoteLimit,
			"this user already has 10 addresses, the most one user may have. Remove one you no longer use first; nothing was changed.", "")
		return
	}
	id64, _ := result.LastInsertId()
	ruleID := int(id64)
	p.auditDetailed(r, "database.remote.add:"+d.Name, "database_remote_rule", ruleID,
		map[string]any{"database": d.Name, "user": username, "address": address})

	answer, _, err := p.applyDatabaseRemote(ctx, true)
	if err != nil {
		// The answer was lost: the row stays and the list reads the server.
		writeCodedError(w, http.StatusBadGateway, errCodeDatabaseRemoteAgent,
			"the address was saved but the server did not confirm the change; the list shows what is in force", "")
		return
	}
	row := databaseRemoteRow{Engine: d.Engine, Port: d.Port, Database: d.Name, User: username, Address: address}
	status, problem := databaseRemoteRuleStatus(row, "")
	if status == "refused" && problem != nil {
		// A verified refusal of this rule: the row goes, the reason stays.
		_, _ = p.db.GetDB().ExecContext(ctx, `DELETE FROM database_remote_rules WHERE id = ?`, ruleID)
		p.kickDatabaseRemoteSync()
		writeCodedErrorDetails(w, http.StatusConflict, errCodeDatabaseRemoteRefused, problem.Detail+"; the address was not added", "", []string{problem.Code})
		return
	}
	if answer.ErrorCode == transport.DatabaseRemoteNFTMissing {
		_, _ = p.db.GetDB().ExecContext(ctx, `DELETE FROM database_remote_rules WHERE id = ?`, ruleID)
		writeCodedErrorDetails(w, http.StatusConflict, errCodeDatabaseRemoteRefused, answer.Error, "/components", []string{answer.ErrorCode})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "id": ruleID, "status": status, "error": answer.Error})
}

func databaseRemoteRefusalSentence(access databaseRemoteAccess) string {
	switch access.Reason {
	case "not_offered":
		return "remote access to databases is off on this server; nothing was changed"
	case "not_in_plan":
		return "your plan does not include remote access to databases; ask " + access.Grantor + " to include it. Nothing was changed"
	case "remote_server":
		return "this database is on another server, which CelikPanel does not open to the outside; nothing was changed"
	}
	return "this subscription is not active, so no address can be added; nothing was changed"
}

func (p *Panel) handleDatabaseRemoteRemove(w http.ResponseWriter, r *http.Request, ruleID int) {
	ctx := r.Context()
	reportOperationPlan(ctx, "check", "save", "apply", "firewall")
	reportOperationStep(ctx, "check")
	c := currentCaller(r)
	var databaseID int
	var address, username string
	err := p.db.GetDB().QueryRowContext(ctx, `
		SELECT r.database_id, r.address, u.username FROM database_remote_rules r JOIN database_users u ON u.id = r.user_id
		WHERE r.id = ?`, ruleID).Scan(&databaseID, &address, &username)
	if errors.Is(err, sql.ErrNoRows) {
		writeCodedError(w, http.StatusNotFound, errCodeDatabaseRemoteNotFound, "rule not found", "")
		return
	}
	if err != nil {
		writeServerError(w, err)
		return
	}
	// Removing never needs the permission: an account whose plan lost it
	// still closes its own rules.
	d, err := p.databaseRemoteDatabaseFor(ctx, c, databaseID)
	if errors.Is(err, errDatabaseRemoteNotFound) {
		writeCodedError(w, http.StatusNotFound, errCodeDatabaseRemoteNotFound, "rule not found", "")
		return
	}
	if err != nil {
		writeServerError(w, err)
		return
	}
	reportOperationStep(ctx, "save")
	if _, err := p.db.GetDB().ExecContext(ctx, `DELETE FROM database_remote_rules WHERE id = ?`, ruleID); err != nil {
		writeServerError(w, err)
		return
	}
	p.auditDetailed(r, "database.remote.remove:"+d.Name, "database_remote_rule", ruleID,
		map[string]any{"database": d.Name, "user": username, "address": address})
	answer, _, err := p.applyDatabaseRemote(ctx, true)
	if err != nil {
		p.kickDatabaseRemoteSync()
		writeCodedError(w, http.StatusBadGateway, errCodeDatabaseRemoteAgent,
			"the address was removed from the list but the server did not confirm it yet; CelikPanel tries again within minutes", "")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "error": answer.Error})
}

// handleDatabaseRemoteServer: the administrator's switch and the engines.
func (p *Panel) handleDatabaseRemoteServer(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c := currentCaller(r)
	if c == nil || c.APIToken != nil || !c.hasAccountRole(roleAdmin) {
		writeCodedError(w, http.StatusForbidden, errCodeAdminOnly, "administrator access required", "")
		return
	}
	if r.Method == http.MethodPut {
		reportOperationPlan(ctx, "save", "apply", "firewall")
		reportOperationStep(ctx, "save")
		var body struct {
			Offered *bool `json:"offered"`
			Apply   bool  `json:"apply"`
		}
		if err := decodeStrictJSON(w, r, &body); err != nil || (body.Offered == nil && !body.Apply) {
			writeClientError(w, http.StatusBadRequest, "send {\"offered\": true|false} or {\"apply\": true}; nothing was changed")
			return
		}
		if body.Offered != nil {
			offered, err := p.loadOfferedOptInPermissions(ctx)
			if err != nil {
				writeServerError(w, err)
				return
			}
			offered[rightDatabaseRemote] = *body.Offered
			if err := p.saveOfferedOptInPermissions(ctx, offered); err != nil {
				writeServerError(w, err)
				return
			}
			p.auditDetailed(r, "database.remote.offer", "panel_settings", 0, map[string]any{"offered": *body.Offered})
		}
		if _, _, err := p.applyDatabaseRemote(ctx, true); err != nil {
			p.kickDatabaseRemoteSync()
			writeCodedError(w, http.StatusBadGateway, errCodeDatabaseRemoteAgent,
				"the setting was saved but the server did not confirm the change; CelikPanel tries again within minutes", "")
			return
		}
	}
	offered, err := p.databaseRemoteOffered(ctx)
	if err != nil {
		writeServerError(w, err)
		return
	}
	var rules int
	if err := p.db.GetDB().QueryRowContext(ctx, `SELECT COUNT(*) FROM database_remote_rules`).Scan(&rules); err != nil {
		writeServerError(w, err)
		return
	}
	targets, err := p.databaseRemoteDesired(ctx)
	if err != nil {
		writeServerError(w, err)
		return
	}
	engines := []map[string]any{}
	for _, t := range targets {
		engines = append(engines, map[string]any{"engine": t.Engine, "port": t.Port, "rules": len(t.Rules),
			"state": p.databaseRemoteEngineView(r, t.Engine, t.Port)})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"offered": offered, "rules": rules, "engines": engines})
}
