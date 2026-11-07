package main

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// The action log of one account (alpha.128, G23): GET /api/v1/me/activity.
// The server-wide audit log stays the administrator's (/api/v1/audit-logs);
// this read gives every signed-in person the part of it that is theirs,
// filtered here by rights, never by the browser:
//   - everyone: the rows they did (user_id = me) and the rows done as them
//     through "Log in as" (acting_as = me), shown as done by the
//     administrator or the provider;
//   - a customer also its team members' rows;
//   - a reseller also its customers' rows and their team members';
//   - a team member only its own.
//
// A row says what was done as its action name and one plain target (a domain,
// an address, a component), nothing else: the details and anything after the
// name that could hold a reason or a path are never sent. The IP address is
// sent only on the viewer's own rows. Read only; newest first, paged by id
// (idx_audit_logs_user / idx_audit_logs_acting_as, migration 069).
//
// Hesabın işlem günlüğü: herkes kendi satırlarını ve "Farklı giriş yap" ile
// onun adına yapılanları görür; müşteri ekip üyelerininkini, bayi
// müşterilerininkini de görür; ekip üyesi yalnız kendisininkini. Ayrıntılar
// asla gönderilmez; IP yalnız kişinin kendi satırında gösterilir.

const (
	activityLogPath         = "/api/v1/me/activity"
	activityLogDefaultLimit = 50
	activityLogMaxLimit     = 100
)

var (
	activityNamePattern     = regexp.MustCompile(`^[a-z0-9_]+(?:[.-][a-z0-9_]+)*$`)
	activityTargetPattern   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._@+-]{0,252}$`)
	activityResourcePattern = regexp.MustCompile(`^[a-z][a-z_]{0,31}$`)
)

type activityActor struct {
	// Kind: you, administrator, provider (the reseller), account (a
	// customer of the viewing reseller), team_member, system, someone.
	Kind string `json:"kind"`
	Name string `json:"name,omitempty"`
}

type activityEntry struct {
	ID           int           `json:"id"`
	At           string        `json:"at"`
	Action       string        `json:"action"`
	ResourceType string        `json:"resource_type,omitempty"`
	ResourceID   int           `json:"resource_id,omitempty"`
	Actor        activityActor `json:"actor"`
	// As: the account the actor acted as through "Log in as".
	As string `json:"as,omitempty"`
	IP string `json:"ip,omitempty"`
}

// activityAction keeps the action's name and its first target, and only when
// both are plain; everything after a space (details, reasons) is dropped.
func activityAction(raw string) string {
	event := raw
	if i := strings.Index(event, " — "); i >= 0 {
		event = event[:i]
	}
	event = strings.TrimSpace(event)
	if i := strings.IndexAny(event, " \t\n"); i >= 0 {
		event = event[:i]
	}
	name, rest, _ := strings.Cut(event, ":")
	if len(name) > 80 || !activityNamePattern.MatchString(name) {
		return ""
	}
	target, _, _ := strings.Cut(rest, ":")
	if target == "" || !activityTargetPattern.MatchString(target) {
		return name
	}
	return name + ":" + target
}

// activityPeople: the accounts whose rows the caller reads (SQL and args of
// one SELECT of ids).
func activityPeople(caller *Caller) (string, []any) {
	switch {
	case caller.hasAccountRole(roleReseller):
		return `SELECT ? UNION
			SELECT c.id FROM users c
			WHERE c.parent_id = ? AND c.role = 'customer' AND COALESCE(NULLIF(c.account_type, ''), 'account') = 'account'
			UNION
			SELECT m.id FROM users m JOIN users c ON c.id = m.parent_id
			WHERE c.parent_id = ? AND c.role = 'customer' AND COALESCE(NULLIF(c.account_type, ''), 'account') = 'account'
			  AND m.account_type = 'additional_user'`, []any{caller.ID, caller.ID, caller.ID}
	case caller.hasAccountRole(roleCustomer):
		return `SELECT ? UNION
			SELECT m.id FROM users m WHERE m.parent_id = ? AND m.account_type = 'additional_user'`, []any{caller.ID, caller.ID}
	default:
		// The administrator (whose full log is /api/v1/audit-logs) and a
		// team member: their own rows.
		return `SELECT ?`, []any{caller.ID}
	}
}

func (p *Panel) handleActivityLog(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		rejectRouteMethod(w, []string{http.MethodGet})
		return
	}
	caller := currentCaller(r)
	if caller == nil || caller.ID <= 0 || !caller.validAuthorizationIdentity() {
		writeCodedError(w, http.StatusUnauthorized, errCodeAuthRequired, "authentication required", "")
		return
	}
	query := r.URL.Query()
	limit := activityLogDefaultLimit
	if text := query.Get("limit"); text != "" {
		n, err := strconv.Atoi(text)
		if err != nil || n < 1 || n > activityLogMaxLimit {
			writeClientError(w, http.StatusBadRequest, "limit must be a number from 1 to 100")
			return
		}
		limit = n
	}
	before := int64(math.MaxInt64)
	if text := query.Get("before"); text != "" {
		n, err := strconv.ParseInt(text, 10, 64)
		if err != nil || n < 1 {
			writeClientError(w, http.StatusBadRequest, "before must be the id of a row from an earlier page")
			return
		}
		before = n
	}
	entries, more, err := p.readActivity(r.Context(), caller, before, limit)
	if err != nil {
		writeServerError(w, err)
		return
	}
	response := map[string]any{"entries": entries}
	if more && len(entries) > 0 {
		response["next_before"] = entries[len(entries)-1].ID
	}
	// Rows recorded but not saved yet are not in this list; it says so
	// (D-024), without the server-wide count.
	if p.auditPendingSummary(time.Now()) != nil {
		response["incomplete"] = true
	}
	_ = json.NewEncoder(w).Encode(response)
}

func (p *Panel) readActivity(ctx context.Context, caller *Caller, before int64, limit int) ([]activityEntry, bool, error) {
	db := p.db.GetDB()
	peopleSQL, peopleArgs := activityPeople(caller)
	people := map[int]bool{}
	rows, err := db.QueryContext(ctx, peopleSQL, peopleArgs...)
	if err != nil {
		return nil, false, err
	}
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, false, err
		}
		people[id] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, false, err
	}

	args := append([]any{}, peopleArgs...)
	args = append(args, before, limit+1, before, limit+1, limit+1)
	rows, err = db.QueryContext(ctx, `
		WITH people(id) AS (`+peopleSQL+`),
		picked(id) AS (
			SELECT id FROM (SELECT a.id FROM audit_logs a
				WHERE a.user_id IN (SELECT id FROM people) AND a.id < ? ORDER BY a.id DESC LIMIT ?)
			UNION
			SELECT id FROM (SELECT a.id FROM audit_logs a
				WHERE a.acting_as IN (SELECT id FROM people) AND a.id < ? ORDER BY a.id DESC LIMIT ?)
		)
		SELECT a.id, COALESCE(a.user_id, 0), COALESCE(a.acting_as, 0), a.action,
		       COALESCE(a.resource_type, ''), COALESCE(a.resource_id, 0), COALESCE(a.ip_address, ''),
		       COALESCE(a.created_at, ''),
		       COALESCE(u.username, ''), COALESCE(u.role, ''), COALESCE(u.account_type, ''),
		       COALESCE(acting.username, '')
		FROM audit_logs a
		JOIN picked ON picked.id = a.id
		LEFT JOIN users u ON u.id = a.user_id
		LEFT JOIN users acting ON acting.id = a.acting_as
		ORDER BY a.id DESC LIMIT ?`, args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	entries := []activityEntry{}
	for rows.Next() {
		var e activityEntry
		var userID, actingAs int
		var action, ip, username, role, accountType, actingName string
		if err := rows.Scan(&e.ID, &userID, &actingAs, &action, &e.ResourceType, &e.ResourceID, &ip, &e.At,
			&username, &role, &accountType, &actingName); err != nil {
			return nil, false, err
		}
		e.Action = activityAction(action)
		if !activityResourcePattern.MatchString(e.ResourceType) {
			e.ResourceType, e.ResourceID = "", 0
		}
		if e.ResourceID < 0 {
			e.ResourceID = 0
		}
		switch {
		case userID != 0 && userID == caller.ID:
			e.Actor = activityActor{Kind: "you"}
			e.IP = ip
			if actingAs != 0 {
				e.As = actingName
			}
		case userID != 0 && people[userID]:
			e.Actor = activityActor{Kind: "account", Name: username}
			if accountType == "additional_user" {
				e.Actor.Kind = "team_member"
			}
			if actingAs != 0 && actingAs != userID {
				e.As = actingName
			}
		default:
			// Someone outside the viewer's accounts acting as one of them
			// ("Log in as"): said by role, not by name.
			switch {
			case userID == 0:
				e.Actor = activityActor{Kind: "system"}
			case role == roleAdmin:
				e.Actor = activityActor{Kind: "administrator"}
			case role == roleReseller:
				e.Actor = activityActor{Kind: "provider"}
			default:
				e.Actor = activityActor{Kind: "someone"}
			}
			if actingAs != 0 && actingAs != caller.ID {
				e.As = actingName
			}
		}
		entries = append(entries, e)
	}
	// A read that broke off part-way is a failure, never a shorter list.
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	more := len(entries) > limit
	if more {
		entries = entries[:limit]
	}
	return entries, more, nil
}

