package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alicelik/celikpanel/internal/auth"
)

// Panel sessions, idle timeout and administrator access by address (G4,
// alpha.128).
//
//   - Every person sees their own signed-in sessions (device, address, last
//     use) and can end one or all others; the administrator also sees every
//     account's sessions and can end any. A password change already ends
//     every session of that account (user_repository.go).
//   - Idle timeout: a session not used for the administrator's choice
//     (15–480 minutes, default 60) ends at its next request. "Used" means a
//     change (any request that is not a read) or the screen's activity signal
//     (POST /api/v1/auth/activity, sent only while the person types, clicks
//     or touches the page). Background reads that a screen makes by itself do
//     not keep a session alive. last_seen_at is written at most once a minute.
//   - Administrator access by address: when the administrator lists
//     addresses or networks, administrator sign-ins and administrator
//     sessions (and "Log in as" sessions an administrator opened) are
//     accepted only from them. Customers and resellers are never affected:
//     the panel is their workplace too. The loopback address is always
//     allowed (an SSH tunnel is a recovery path), the address saving the list
//     must be in it (no lock-out by one's own hand), and the documented local
//     recovery is `sudo touch /etc/celikpanel/panel-access-open`, which
//     suspends the list while the file exists (docs/PANEL-ACCESS.md). The
//     list is the panel's own: SSH, mail, websites and the firewall are
//     untouched (D-022).
//
// Panel oturumları, boşta kalma süresi ve yönetici erişimi için adres listesi.
// Müşteri ve bayiler adres listesinden etkilenmez; loopback hep açıktır;
// listeyi kaydeden adres listede olmalıdır; yerel kurtarma dosyası listeyi
// askıya alır.

const (
	settingSessionIdleMinutes = "panel.sessions.idle_minutes"
	settingPanelAccessList    = "panel.access.allowlist"

	sessionIdleDefault     = 60
	sessionTouchInterval   = time.Minute
	panelAccessListMax     = 50
	panelAccessCacheTTL    = 10 * time.Second
	panelAccessRecoveryTTL = 5 * time.Second

	errCodeSessionIdle             = "SESSION_IDLE"
	errCodePanelAccessNotAllowed   = "PANEL_ACCESS_ADDRESS_NOT_ALLOWED"
	errCodePanelAccessInvalid      = "PANEL_ACCESS_SETTINGS_INVALID"
	errCodePanelAccessLockout      = "PANEL_ACCESS_WOULD_LOCK_OUT"
	errCodeSessionNotFound         = "SESSION_NOT_FOUND"
	errCodeSessionImpersonating    = "SESSION_CHANGE_WHILE_LOGGED_IN_AS"
	errCodeSessionCurrent          = "SESSION_IS_CURRENT"
	panelAccessRecoveryCommand     = "sudo touch /etc/celikpanel/panel-access-open"
	panelAccessRecoveryUndoCommand = "sudo rm /etc/celikpanel/panel-access-open"
)

// panelAccessRecoveryFile suspends the administrator address list while it
// exists. A variable so tests can point it elsewhere.
var panelAccessRecoveryFile = "/etc/celikpanel/panel-access-open"

var sessionIdleChoices = []int{15, 30, 60, 120, 240, 480}

func validSessionIdle(n int) bool {
	for _, choice := range sessionIdleChoices {
		if n == choice {
			return true
		}
	}
	return false
}

// panelAccessSnapshot is the cached form of the two settings.
type panelAccessSnapshot struct {
	idle     time.Duration
	idleMin  int
	list     []netip.Prefix
	raw      []string
	loadedAt time.Time
}

var panelAccessCache struct {
	mu        sync.Mutex
	snapshot  *panelAccessSnapshot
	recovery  bool
	checkedAt time.Time
}

func invalidatePanelAccessCache() {
	panelAccessCache.mu.Lock()
	panelAccessCache.snapshot = nil
	panelAccessCache.checkedAt = time.Time{}
	panelAccessCache.mu.Unlock()
}

func (p *Panel) panelAccess(ctx context.Context) *panelAccessSnapshot {
	now := time.Now()
	panelAccessCache.mu.Lock()
	cached := panelAccessCache.snapshot
	panelAccessCache.mu.Unlock()
	if cached != nil && now.Sub(cached.loadedAt) < panelAccessCacheTTL {
		return cached
	}
	snapshot := &panelAccessSnapshot{idleMin: sessionIdleDefault, loadedAt: now}
	if p != nil && p.db != nil {
		if n, err := strconv.Atoi(p.setting(ctx, settingSessionIdleMinutes)); err == nil && validSessionIdle(n) {
			snapshot.idleMin = n
		}
		if raw := p.setting(ctx, settingPanelAccessList); raw != "" {
			var entries []string
			if json.Unmarshal([]byte(raw), &entries) == nil {
				if prefixes, err := parsePanelAccessList(entries); err == nil {
					snapshot.list, snapshot.raw = prefixes, entries
				}
			}
		}
	}
	snapshot.idle = time.Duration(snapshot.idleMin) * time.Minute
	panelAccessCache.mu.Lock()
	panelAccessCache.snapshot = snapshot
	panelAccessCache.mu.Unlock()
	return snapshot
}

// panelAccessRecoveryOpen: the owner's local recovery file exists.
func panelAccessRecoveryOpen() bool {
	now := time.Now()
	panelAccessCache.mu.Lock()
	defer panelAccessCache.mu.Unlock()
	if !panelAccessCache.checkedAt.IsZero() && now.Sub(panelAccessCache.checkedAt) < panelAccessRecoveryTTL {
		return panelAccessCache.recovery
	}
	_, err := os.Lstat(panelAccessRecoveryFile)
	panelAccessCache.recovery = err == nil
	panelAccessCache.checkedAt = now
	return panelAccessCache.recovery
}

// parsePanelAccessList accepts single addresses and networks (IPv4 and
// IPv6), normalised; duplicates are dropped.
func parsePanelAccessList(entries []string) ([]netip.Prefix, error) {
	if len(entries) > panelAccessListMax {
		return nil, errors.New("too many entries")
	}
	seen := map[netip.Prefix]bool{}
	var out []netip.Prefix
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		var prefix netip.Prefix
		if strings.Contains(entry, "/") {
			parsed, err := netip.ParsePrefix(entry)
			if err != nil {
				return nil, err
			}
			prefix = parsed.Masked()
		} else {
			addr, err := netip.ParseAddr(entry)
			if err != nil || addr.Zone() != "" {
				return nil, errors.New("invalid address")
			}
			prefix = netip.PrefixFrom(addr.Unmap(), addr.Unmap().BitLen())
		}
		if prefix.Addr().Is4In6() {
			prefix = netip.PrefixFrom(prefix.Addr().Unmap(), prefix.Bits()-96)
		}
		if prefix.Bits() == 0 {
			// "Everyone" is the empty list, said plainly; a /0 would hide it.
			return nil, errors.New("a network covering every address")
		}
		if !seen[prefix] {
			seen[prefix] = true
			out = append(out, prefix)
		}
	}
	return out, nil
}

func requestAddr(r *http.Request) (netip.Addr, bool) {
	addr, err := netip.ParseAddr(clientIP(r))
	if err != nil {
		return netip.Addr{}, false
	}
	return addr.Unmap(), true
}

// addressAllowed: no list, loopback, or inside the list.
func addressAllowed(list []netip.Prefix, addr netip.Addr, known bool) bool {
	if len(list) == 0 {
		return true
	}
	if !known {
		return false
	}
	if addr.IsLoopback() {
		return true
	}
	for _, prefix := range list {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

// adminAddressAllowed decides for an administrator's request.
func (p *Panel) adminAddressAllowed(r *http.Request) bool {
	access := p.panelAccess(r.Context())
	if len(access.list) == 0 || panelAccessRecoveryOpen() {
		return true
	}
	addr, known := requestAddr(r)
	return addressAllowed(access.list, addr, known)
}

const panelAccessRefusedMessage = "Administrator sign-in is accepted only from the addresses the administrator listed, and this address is not one of them. " +
	"Nothing was changed. Connect from a listed address, or, on the server itself, run `" + panelAccessRecoveryCommand +
	"` to accept every address until you remove that file (see docs/PANEL-ACCESS.md)."

func writePanelAccessRefused(w http.ResponseWriter) {
	writeCodedError(w, http.StatusForbidden, errCodePanelAccessNotAllowed, panelAccessRefusedMessage, "")
}

// adminSessionAddressAllowed: false only for an administrator's session
// (or one an administrator opened with "Log in as") from an unlisted
// address. Customers and resellers are never refused here.
func (p *Panel) adminSessionAddressAllowed(r *http.Request, c *Caller) bool {
	if len(p.panelAccess(r.Context()).list) == 0 {
		return true
	}
	admin := c.hasAccountRole(roleAdmin)
	if !admin && c.ImpersonatorID > 0 && p.users != nil {
		operator, err := p.users.GetByID(r.Context(), c.ImpersonatorID)
		admin = err == nil && operator != nil && operator.Role == roleAdmin
	}
	return !admin || p.adminAddressAllowed(r)
}

// refuseAdministratorFromUnlistedAddress is called at sign-in before a
// session is created; true means the answer was written.
func (p *Panel) refuseAdministratorFromUnlistedAddress(w http.ResponseWriter, r *http.Request, userID int) bool {
	if p.users == nil {
		return false
	}
	user, err := p.users.GetByID(r.Context(), userID)
	if err != nil || user == nil || user.Role != roleAdmin {
		return false
	}
	if p.adminAddressAllowed(r) {
		return false
	}
	p.auditAs(r, userID, "auth.login.address_refused", "", 0)
	writePanelAccessRefused(w)
	return true
}

// sessionIsActivity: a change, or the screen's activity signal.
func sessionIsActivity(r *http.Request) bool {
	if r.URL.Path == sessionActivityPath {
		return true
	}
	return r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions
}

// sessionIdleCheck ends an idle session and otherwise records its use.
// It returns false when the answer (401) was written.
func (p *Panel) sessionIdleCheck(w http.ResponseWriter, r *http.Request, token string, session auth.SessionIdentity) bool {
	now := time.Now()
	access := p.panelAccess(r.Context())
	if !session.LastSeen.IsZero() && now.Sub(session.LastSeen) > access.idle {
		_ = p.sessions.Delete(r.Context(), token)
		writeCodedError(w, http.StatusUnauthorized, errCodeSessionIdle,
			"You were signed out because this session was not used for "+strconv.Itoa(access.idleMin)+
				" minutes. Nothing was lost on the server; sign in again to continue.", "")
		return false
	}
	if session.LastSeen.IsZero() || (sessionIsActivity(r) && now.Sub(session.LastSeen) >= sessionTouchInterval) {
		// Best effort: a failed write never refuses the request.
		_ = p.sessions.Touch(r.Context(), token, now, clientIP(r), r.UserAgent())
	}
	return true
}

// describeNewSession records address and browser right after sign-in.
func (p *Panel) describeNewSession(r *http.Request, token string) {
	if p.sessions != nil {
		_ = p.sessions.Touch(r.Context(), token, time.Now(), clientIP(r), r.UserAgent())
	}
}

// --- Handlers ----------------------------------------------------------------

const (
	sessionActivityPath    = "/api/v1/auth/activity"
	ownSessionsPath        = "/api/v1/auth/sessions"
	ownSessionEndPath      = "/api/v1/auth/sessions/end"
	ownSessionsEndOthers   = "/api/v1/auth/sessions/end-others"
	allSessionsPath        = "/api/v1/panel/sessions"
	allSessionEndPath      = "/api/v1/panel/sessions/end"
	panelAccessSettingPath = "/api/v1/panel/access"
)

type sessionView struct {
	Handle       string `json:"handle"`
	Current      bool   `json:"current"`
	UserID       int    `json:"user_id,omitempty"`
	Username     string `json:"username,omitempty"`
	Role         string `json:"role,omitempty"`
	CreatedAt    string `json:"created_at"`
	LastSeenAt   string `json:"last_seen_at,omitempty"`
	ExpiresAt    string `json:"expires_at"`
	ClientIP     string `json:"client_ip,omitempty"`
	UserAgent    string `json:"user_agent,omitempty"`
	OpenedBy     string `json:"opened_by,omitempty"`
	OpenedByRole string `json:"opened_by_role,omitempty"`
}

type sessionsReport struct {
	Sessions    []sessionView `json:"sessions"`
	IdleMinutes int           `json:"idle_minutes"`
}

func currentSessionHandle(r *http.Request) string {
	if cookie, err := r.Cookie(sessionCookieName); err == nil && cookie.Value != "" {
		return auth.SessionHandle(cookie.Value)
	}
	return ""
}

// readSessions lists live sessions; userID 0 = every account.
func (p *Panel) readSessions(ctx context.Context, userID int, current string) ([]sessionView, error) {
	query := `
		SELECT substr(s.token_hash, 1, 16), s.user_id, COALESCE(u.username, ''), COALESCE(u.role, ''),
		       COALESCE(s.created_at, ''), COALESCE(s.last_seen_at, ''), s.expires_at,
		       COALESCE(s.client_ip, ''), COALESCE(s.user_agent, ''),
		       COALESCE(o.username, ''), COALESCE(o.role, '')
		FROM sessions s
		LEFT JOIN users u ON u.id = s.user_id
		LEFT JOIN users o ON o.id = s.impersonator_id
		WHERE s.expires_at > ?`
	args := []any{time.Now().UTC().Format(time.RFC3339)}
	if userID > 0 {
		query += ` AND s.user_id = ?`
		args = append(args, userID)
	}
	query += ` ORDER BY COALESCE(s.last_seen_at, s.created_at) DESC LIMIT 500`
	rows, err := p.db.GetDB().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	idle := p.panelAccess(ctx).idle
	now := time.Now()
	out := []sessionView{}
	for rows.Next() {
		var view sessionView
		if err := rows.Scan(&view.Handle, &view.UserID, &view.Username, &view.Role, &view.CreatedAt, &view.LastSeenAt,
			&view.ExpiresAt, &view.ClientIP, &view.UserAgent, &view.OpenedBy, &view.OpenedByRole); err != nil {
			return nil, err
		}
		// An idle session is already over; it ends at its next request.
		if at, err := time.Parse(time.RFC3339, view.LastSeenAt); err == nil && now.Sub(at) > idle {
			continue
		}
		view.Current = view.Handle == current
		if userID > 0 {
			view.UserID, view.Username, view.Role = 0, "", ""
		}
		out = append(out, view)
	}
	return out, rows.Err()
}

func decodeSessionHandle(r *http.Request) (string, bool) {
	var req struct {
		Handle string `json:"handle"`
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil || len(req.Handle) != 16 {
		return "", false
	}
	for _, c := range req.Handle {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return "", false
		}
	}
	return req.Handle, true
}

func sessionsMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	w.Header().Set("Cache-Control", "no-store")
	return requireMethod(w, r, method)
}

func writeSessionsJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

// handleSessionActivity: POST, the screen's "the person is here" signal.
// The middleware has already recorded it; the answer is empty.
func (p *Panel) handleSessionActivity(w http.ResponseWriter, r *http.Request) {
	if !sessionsMethod(w, r, http.MethodPost) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleOwnSessions: GET the caller's own live sessions.
func (p *Panel) handleOwnSessions(w http.ResponseWriter, r *http.Request) {
	if !sessionsMethod(w, r, http.MethodGet) {
		return
	}
	c := currentCaller(r)
	if c == nil {
		writeCodedError(w, http.StatusUnauthorized, errCodeAuthRequired, "authentication required", "")
		return
	}
	sessions, err := p.readSessions(r.Context(), c.ID, currentSessionHandle(r))
	if err != nil {
		writeServerError(w, err)
		return
	}
	writeSessionsJSON(w, sessionsReport{Sessions: sessions, IdleMinutes: p.panelAccess(r.Context()).idleMin})
}

const sessionImpersonatingMessage = "Sessions cannot be ended while you are logged in as another account. " +
	"Return to your own account first; the account's owner can end them from My account."

// handleOwnSessionEnd: POST {"handle"} ends one of the caller's other
// sessions. The current one is ended with Sign out.
func (p *Panel) handleOwnSessionEnd(w http.ResponseWriter, r *http.Request) {
	if !sessionsMethod(w, r, http.MethodPost) {
		return
	}
	c := currentCaller(r)
	if c == nil {
		writeCodedError(w, http.StatusUnauthorized, errCodeAuthRequired, "authentication required", "")
		return
	}
	if c.ImpersonatorID > 0 {
		writeCodedError(w, http.StatusConflict, errCodeSessionImpersonating, sessionImpersonatingMessage, "")
		return
	}
	handle, ok := decodeSessionHandle(r)
	if !ok {
		writeCodedError(w, http.StatusBadRequest, errCodeSessionNotFound, "This session could not be read, so nothing was ended. Reload the list and try again.", "")
		return
	}
	if handle == currentSessionHandle(r) {
		writeCodedError(w, http.StatusConflict, errCodeSessionCurrent, "This is the session you are using now. Use Sign out to end it.", "")
		return
	}
	p.endSession(w, r, c.ID, handle, "auth.session.end")
}

// handleOwnSessionsEndOthers: POST ends every session of the caller but
// this one.
func (p *Panel) handleOwnSessionsEndOthers(w http.ResponseWriter, r *http.Request) {
	if !sessionsMethod(w, r, http.MethodPost) {
		return
	}
	c := currentCaller(r)
	if c == nil {
		writeCodedError(w, http.StatusUnauthorized, errCodeAuthRequired, "authentication required", "")
		return
	}
	if c.ImpersonatorID > 0 {
		writeCodedError(w, http.StatusConflict, errCodeSessionImpersonating, sessionImpersonatingMessage, "")
		return
	}
	current := currentSessionHandle(r)
	result, err := p.db.GetDB().ExecContext(r.Context(),
		`DELETE FROM sessions WHERE user_id = ? AND substr(token_hash, 1, 16) <> ?`, c.ID, current)
	if err != nil {
		writeServerError(w, err)
		return
	}
	ended, _ := result.RowsAffected()
	p.auditDetailed(r, "auth.session.end_others", "user", c.ID, map[string]any{"ended": ended})
	writeSessionsJSON(w, map[string]int64{"ended": ended})
}

// endSession deletes one session; userID 0 = any account (administrator).
func (p *Panel) endSession(w http.ResponseWriter, r *http.Request, userID int, handle, action string) {
	query := `DELETE FROM sessions WHERE substr(token_hash, 1, 16) = ?`
	args := []any{handle}
	if userID > 0 {
		query += ` AND user_id = ?`
		args = append(args, userID)
	}
	var owner int
	lookup := `SELECT user_id FROM sessions WHERE substr(token_hash, 1, 16) = ?`
	if userID > 0 {
		lookup += ` AND user_id = ?`
	}
	_ = p.db.GetDB().QueryRowContext(r.Context(), lookup, args...).Scan(&owner)
	result, err := p.db.GetDB().ExecContext(r.Context(), query, args...)
	if err != nil {
		writeServerError(w, err)
		return
	}
	ended, _ := result.RowsAffected()
	if ended == 0 {
		// Already gone (ended elsewhere, expired): the wanted state holds.
		writeSessionsJSON(w, map[string]int64{"ended": 0})
		return
	}
	p.auditDetailed(r, action, "user", owner, map[string]any{"session": handle})
	writeSessionsJSON(w, map[string]int64{"ended": ended})
}

// handleAllSessions: GET every account's live sessions (administrator).
func (p *Panel) handleAllSessions(w http.ResponseWriter, r *http.Request) {
	if !sessionsMethod(w, r, http.MethodGet) {
		return
	}
	sessions, err := p.readSessions(r.Context(), 0, currentSessionHandle(r))
	if err != nil {
		writeServerError(w, err)
		return
	}
	writeSessionsJSON(w, sessionsReport{Sessions: sessions, IdleMinutes: p.panelAccess(r.Context()).idleMin})
}

// handleAllSessionEnd: POST {"handle"} ends any session (administrator).
func (p *Panel) handleAllSessionEnd(w http.ResponseWriter, r *http.Request) {
	if !sessionsMethod(w, r, http.MethodPost) {
		return
	}
	c := currentCaller(r)
	if c == nil || !c.hasAccountRole(roleAdmin) {
		writeCodedError(w, http.StatusForbidden, errCodeAdminOnly, "administrator access required", "")
		return
	}
	if c.ImpersonatorID > 0 {
		writeCodedError(w, http.StatusConflict, errCodeSessionImpersonating, sessionImpersonatingMessage, "")
		return
	}
	handle, ok := decodeSessionHandle(r)
	if !ok {
		writeCodedError(w, http.StatusBadRequest, errCodeSessionNotFound, "This session could not be read, so nothing was ended. Reload the list and try again.", "")
		return
	}
	if handle == currentSessionHandle(r) {
		writeCodedError(w, http.StatusConflict, errCodeSessionCurrent, "This is the session you are using now. Use Sign out to end it.", "")
		return
	}
	p.endSession(w, r, 0, handle, "auth.session.end")
}

type panelAccessView struct {
	IdleMinutes     int      `json:"idle_minutes"`
	IdleChoices     []int    `json:"idle_choices"`
	Allowlist       []string `json:"allowlist"`
	YourAddress     string   `json:"your_address"`
	YourAddressIn   bool     `json:"your_address_allowed"`
	RecoveryOpen    bool     `json:"recovery_open"`
	RecoveryCommand string   `json:"recovery_command"`
	RecoveryUndo    string   `json:"recovery_undo"`
}

func (p *Panel) panelAccessView(r *http.Request) panelAccessView {
	access := p.panelAccess(r.Context())
	addr, known := requestAddr(r)
	list := access.raw
	if list == nil {
		list = []string{}
	}
	view := panelAccessView{
		IdleMinutes: access.idleMin, IdleChoices: sessionIdleChoices, Allowlist: list,
		YourAddressIn: addressAllowed(access.list, addr, known), RecoveryOpen: panelAccessRecoveryOpen(),
		RecoveryCommand: panelAccessRecoveryCommand, RecoveryUndo: panelAccessRecoveryUndoCommand,
	}
	if known {
		view.YourAddress = addr.String()
	}
	return view
}

// handlePanelAccess: GET the idle timeout and the administrator address
// list; PUT {"idle_minutes", "allowlist"} saves both. The address saving
// a non-empty list must be in it (or be the loopback address).
func (p *Panel) handlePanelAccess(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	switch r.Method {
	case http.MethodGet:
		writeSessionsJSON(w, p.panelAccessView(r))
		return
	case http.MethodPut:
	default:
		w.Header().Set("Allow", "GET, PUT")
		writeClientError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	c := currentCaller(r)
	if c == nil || !c.hasAccountRole(roleAdmin) {
		writeCodedError(w, http.StatusForbidden, errCodeAdminOnly, "administrator access required", "")
		return
	}
	if c.ImpersonatorID > 0 {
		writeCodedError(w, http.StatusConflict, errCodeSessionImpersonating,
			"Panel access cannot be changed while you are logged in as another account. Return to your own account first.", "")
		return
	}
	var req struct {
		IdleMinutes *int      `json:"idle_minutes"`
		Allowlist   *[]string `json:"allowlist"`
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, 8<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil || req.IdleMinutes == nil || req.Allowlist == nil || !validSessionIdle(*req.IdleMinutes) {
		writeCodedError(w, http.StatusBadRequest, errCodePanelAccessInvalid,
			"These settings could not be read, so nothing was changed. Reload the page and choose again.", "")
		return
	}
	prefixes, err := parsePanelAccessList(*req.Allowlist)
	if err != nil {
		writeCodedError(w, http.StatusBadRequest, errCodePanelAccessInvalid,
			"One of the addresses is not a single IP address or a network such as 203.0.113.0/24 (at most "+
				strconv.Itoa(panelAccessListMax)+", and not a network that covers every address). Nothing was changed; correct it and save again.", "")
		return
	}
	addr, known := requestAddr(r)
	if !addressAllowed(prefixes, addr, known) {
		writeCodedError(w, http.StatusConflict, errCodePanelAccessLockout,
			"Your own address ("+addr.String()+") is not in the list, so saving it would sign you out and keep you out. "+
				"Nothing was changed. Add your address (the screen offers it), then save.", "")
		return
	}
	normalised := make([]string, 0, len(prefixes))
	for _, prefix := range prefixes {
		if prefix.Bits() == prefix.Addr().BitLen() {
			normalised = append(normalised, prefix.Addr().String())
		} else {
			normalised = append(normalised, prefix.String())
		}
	}
	listJSON, _ := json.Marshal(normalised)
	if err := p.setSetting(r.Context(), settingSessionIdleMinutes, strconv.Itoa(*req.IdleMinutes)); err != nil {
		writeServerError(w, err)
		return
	}
	if err := p.setSetting(r.Context(), settingPanelAccessList, string(listJSON)); err != nil {
		writeServerError(w, err)
		return
	}
	invalidatePanelAccessCache()
	p.auditDetailed(r, "panel.access.settings", "settings", 0, map[string]any{
		"idle_minutes": *req.IdleMinutes, "allowlist_entries": len(normalised),
	})
	writeSessionsJSON(w, p.panelAccessView(r))
}
