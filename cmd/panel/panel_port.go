package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/panellisten"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Panel port change (alpha.114, owner decision 2026-10-31).
//
//	GET  /api/v1/panel/port   the panel's port, an open change, what blocks one
//	POST /api/v1/panel/port   {"action":"start","port":N} | {"action":"confirm","id":…} | {"action":"cancel","id":…}
//
// A change never locks the owner out. The panel first proves it can bind the
// new port, opens it in the firewall, and listens on BOTH ports. The owner
// opens the new address and confirms from there (the confirmation is only
// accepted over the new port, which proves the path works end to end). Only
// then the Agent changes the single CELIKPANEL_LISTEN line in the root-only
// panel.env (over exactly the file seen at start: an owner's own edit is
// detected, never overwritten), the old listener is closed and the firewall
// closes the old port. Without a confirmation within panelPortTrialWindow,
// or when the owner cancels, or when the panel restarts meanwhile, the
// panel stays on the old port and the trial port is closed again. No restart
// is needed in either direction; panel.env is the next start's truth.
//
// Panel portu değişikliği sahibi asla dışarıda bırakmaz: panel iki portu da
// dinler; sahip yeni adresten onaylayınca panel.env'deki tek satır değişir ve
// eski port kapanır. Süre dolarsa, iptal edilirse ya da panel yeniden
// başlarsa panel eski portta kalır.

const panelPortPath = "/api/v1/panel/port"

var (
	panelPortTrialWindow = 10 * time.Minute
	panelPortNow         = func() time.Time { return time.Now().UTC() }

	readPanelListenConfigFromAgent = func(p *Panel, ctx context.Context) (transport.PanelListenConfigResponse, error) {
		var response transport.PanelListenConfigResponse
		err := p.callAgentContext(ctx, "Agent.PanelListenConfigV1", &transport.Empty{}, &response)
		return response, err
	}
	setPanelListenThroughAgent = func(p *Panel, ctx context.Context, req transport.PanelListenSetRequest) (transport.PanelListenSetResponse, error) {
		var response transport.PanelListenSetResponse
		port, _ := panellisten.Port(req.To)
		err := p.withStandaloneAgentMutation(ctx, "panel_port_change", "panel-listen", strconv.Itoa(port), func(callCtx context.Context, binding agentMutationBinding) error {
			req.ServiceMutationBinding = binding
			return p.callAgentContext(callCtx, "Agent.SetPanelListenPortV1", &req, &response)
		})
		return response, err
	}
	panelPortSyncFirewall = func(p *Panel, ctx context.Context) error { return p.syncFirewall(ctx) }
)

// panelListenerControl is the running HTTP server's listener set
// (runningPanelHTTPServer); tests use a fake.
type panelListenerControl interface {
	AddListener(listen string) error
	RemoveListener(listen string) error
	PromoteListener(listen string) error
	HasListener(listen string) bool
}

type panelPortRuntime struct {
	mu        sync.Mutex
	trialPort atomic.Int32
	timer     *time.Timer
}

var panelPortState panelPortRuntime

// activePanelPortTrialPort is the trial port while a change waits for the
// owner's confirmation, else 0 (firewall_handlers.go keeps it open).
func activePanelPortTrialPort() int { return int(panelPortState.trialPort.Load()) }

type panelPortChange struct {
	ID           string `json:"id"`
	FromListen   string `json:"-"`
	ToListen     string `json:"-"`
	EnvSHA256    string `json:"-"`
	FromPort     int    `json:"from_port"`
	ToPort       int    `json:"to_port"`
	Status       string `json:"status"`
	ReasonCode   string `json:"reason_code,omitempty"`
	Reason       string `json:"reason,omitempty"`
	FirewallNote string `json:"firewall_note,omitempty"`
	CreatedAt    string `json:"created_at"`
	DeadlineAt   string `json:"deadline_at"`
	FinishedAt   string `json:"finished_at,omitempty"`
	SecondsLeft  int    `json:"seconds_left,omitempty"`
}

type panelPortBlock struct {
	Code   string `json:"code"`
	Reason string `json:"reason"`
}

type panelPortView struct {
	Port        int              `json:"port"`
	DefaultPort int              `json:"default_port"`
	LegacyPort  int              `json:"legacy_port"`
	RequestPort int              `json:"request_port"`
	Change      *panelPortChange `json:"change"`
	Blocked     *panelPortBlock  `json:"blocked"`
}

type panelPortRefusal struct {
	status int
	code   string
	reason string
}

func (r *panelPortRefusal) Error() string { return r.reason }

func refusePanelPort(status int, code, reason string) *panelPortRefusal {
	return &panelPortRefusal{status: status, code: code, reason: reason}
}

func (p *Panel) handlePanelPort(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if c := currentCaller(r); c == nil || c.Role != roleAdmin {
		writeClientError(w, http.StatusForbidden, "admin only")
		return
	}
	switch r.Method {
	case http.MethodGet:
		view, err := p.panelPortView(r.Context(), requestLocalPort(r))
		if err != nil {
			writeServerError(w, err)
			return
		}
		_ = json.NewEncoder(w).Encode(view)
	case http.MethodPost:
		var request struct {
			Action string `json:"action"`
			Port   int    `json:"port"`
			ID     string `json:"id"`
		}
		if decodeServiceOperationJSON(w, r, &request) != nil {
			writeClientError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		var err error
		switch request.Action {
		case "start":
			_, err = p.startPanelPortChange(r.Context(), request.Port)
		case "confirm":
			err = p.confirmPanelPortChange(r.Context(), request.ID, requestLocalPort(r))
		case "cancel":
			err = p.cancelPanelPortChange(r.Context(), request.ID)
		default:
			writeClientError(w, http.StatusBadRequest, "action must be start, confirm or cancel")
			return
		}
		var refusal *panelPortRefusal
		if errors.As(err, &refusal) {
			writeCodedError(w, refusal.status, refusal.code, refusal.reason, "")
			return
		}
		if err != nil {
			writeServerError(w, err)
			return
		}
		view, err := p.panelPortView(r.Context(), requestLocalPort(r))
		if err != nil {
			writeServerError(w, err)
			return
		}
		_ = json.NewEncoder(w).Encode(view)
	default:
		rejectRouteMethod(w, []string{http.MethodGet, http.MethodPost})
	}
}

// requestLocalPort is the server port the request arrived on.
func requestLocalPort(r *http.Request) int {
	if addr, ok := r.Context().Value(http.LocalAddrContextKey).(net.Addr); ok {
		if tcp, ok := addr.(*net.TCPAddr); ok {
			return tcp.Port
		}
		if _, portText, err := net.SplitHostPort(addr.String()); err == nil {
			port, _ := strconv.Atoi(portText)
			return port
		}
	}
	return 0
}

func (p *Panel) panelPortView(ctx context.Context, requestPort int) (panelPortView, error) {
	view := panelPortView{Port: panelPort(), DefaultPort: panellisten.DefaultPort, LegacyPort: panellisten.LegacyPort, RequestPort: requestPort}
	change, err := p.loadPanelPortChange(ctx, true)
	if err != nil {
		return view, err
	}
	if change != nil && change.Status == "trial" && !panelPortDeadlinePassed(change) && activePanelPortTrialPort() == 0 {
		// A trial this process does not run (it restarted): the startup
		// reconciler ends it; say so until then.
		change.Reason = "The panel restarted before the change was confirmed. It stays on the old port."
	}
	// A read never changes anything (D-024): a trial past its deadline is
	// ended by its own timer, which this view only waits for.
	if change == nil {
		change, err = p.loadPanelPortChange(ctx, false)
		if err != nil {
			return view, err
		}
	}
	if change != nil && change.Status == "trial" {
		if deadline, err := time.Parse(time.RFC3339, change.DeadlineAt); err == nil {
			change.SecondsLeft = max(0, int(deadline.Sub(panelPortNow()).Seconds()))
		}
	}
	view.Change = change
	if change == nil || (change.Status != "trial" && change.Status != "confirming") {
		if refusal := p.panelPortBlocked(ctx); refusal != nil {
			view.Blocked = &panelPortBlock{Code: refusal.code, Reason: refusal.reason}
		}
	}
	return view, nil
}

func panelPortTimeoutReason(change *panelPortChange) string {
	return fmt.Sprintf("Nobody confirmed the new port %d within %d minutes, so the panel stays on port %d and port %d was closed again. If the new address did not open, check the provider's firewall, then try again.", change.ToPort, int(panelPortTrialWindow/time.Minute), change.FromPort, change.ToPort)
}

func panelPortDeadlinePassed(change *panelPortChange) bool {
	deadline, err := time.Parse(time.RFC3339, change.DeadlineAt)
	return err != nil || !panelPortNow().Before(deadline)
}

// panelPortBlocked says why no change can start now, or nil.
func (p *Panel) panelPortBlocked(ctx context.Context) *panelPortRefusal {
	if refusal := p.panelPortPeerRefusal(ctx); refusal != nil {
		return refusal
	}
	_, refusal := p.panelPortConfig(ctx)
	return refusal
}

// panelPortConfig reads panel.env through the Agent and checks it names the
// address this panel runs on.
func (p *Panel) panelPortConfig(ctx context.Context) (transport.PanelListenConfigResponse, *panelPortRefusal) {
	config, err := readPanelListenConfigFromAgent(p, ctx)
	switch {
	case err != nil:
		return config, refusePanelPort(http.StatusServiceUnavailable, "PANEL_PORT_AGENT_UNAVAILABLE", "The panel could not read its settings file through the CelikPanel agent. Nothing was changed. Check that celikpanel-agent is running (sudo systemctl status celikpanel-agent), then try again.")
	case !config.Present:
		return config, refusePanelPort(http.StatusConflict, "PANEL_PORT_CONFIG_MISSING", "This panel has no /etc/celikpanel/panel.env, so its port cannot be changed here. Set CELIKPANEL_LISTEN where this panel is started instead.")
	case config.Error != "":
		return config, refusePanelPort(http.StatusConflict, "PANEL_PORT_CONFIG_UNREADABLE", "/etc/celikpanel/panel.env cannot be used: "+config.Error+". Nothing was changed. The server owner fixes the file (root only, mode 0600), then tries again.")
	case config.Listen != listenAddr():
		return config, refusePanelPort(http.StatusConflict, "PANEL_PORT_CONFIG_CHANGED", fmt.Sprintf("/etc/celikpanel/panel.env says CELIKPANEL_LISTEN=%s, but this panel runs on %s. Someone changed the file by hand; CelikPanel does not overwrite it. Either restart the panel to use that setting (sudo systemctl restart celikpanel-panel) or change the line back, then try again.", config.Listen, listenAddr()))
	}
	return config, nil
}

// panelPortPeerRefusal: another CelikPanel server stores this server's
// address with its port (a joined pair, or a panel publishing DNS records
// here). Changing the port would silently break that link, and there is no
// way yet to tell the other server, so the change is refused with the reason.
func (p *Panel) panelPortPeerRefusal(ctx context.Context) *panelPortRefusal {
	paired := false
	var join serverSetupJoinRecord
	if found, err := readServerSetupJSONSetting(ctx, p.db.GetDB(), serverSetupJoinKey, &join); err == nil && found && join.LinkCredentialHash != "" {
		paired = true
	}
	if !paired {
		if link, err := p.readServerSetupLink(ctx); err == nil && link != nil && link.LinkCredential != "" {
			paired = true
		}
	}
	if !paired {
		var clients int
		if err := p.db.GetDB().QueryRowContext(ctx, `SELECT COUNT(*) FROM remote_dns_clients WHERE revoked=0`).Scan(&clients); err == nil && clients > 0 {
			paired = true
		}
	}
	if !paired {
		return nil
	}
	return refusePanelPort(http.StatusConflict, "PANEL_PORT_PAIRED", fmt.Sprintf("Another CelikPanel server is linked to this one and reaches it at port %d (a joined server, or a panel that publishes DNS records here). Changing the port here would break that link, and this release cannot tell the other server the new address. Keep port %d on this server.", panelPort(), panelPort()))
}

func panelPortReservedByServices() map[int]string {
	reserved := map[int]string{}
	for i := range core.ManagedServices {
		service := &core.ManagedServices[i]
		for _, port := range service.FirewallPorts {
			if port.Port >= panellisten.MinChoice {
				reserved[port.Port] = service.Name
			}
		}
	}
	return reserved
}

func (p *Panel) startPanelPortChange(ctx context.Context, port int) (*panelPortChange, error) {
	panelPortState.mu.Lock()
	defer panelPortState.mu.Unlock()
	if p.listenerControl == nil {
		return nil, refusePanelPort(http.StatusServiceUnavailable, "PANEL_PORT_UNAVAILABLE", "The panel is still starting. Wait a moment, then try again.")
	}
	open, err := p.loadPanelPortChange(ctx, true)
	if err != nil {
		return nil, err
	}
	if open != nil {
		return nil, refusePanelPort(http.StatusConflict, "PANEL_PORT_CHANGE_OPEN", fmt.Sprintf("A change to port %d is already waiting for confirmation. Confirm it from the new address or cancel it first.", open.ToPort))
	}
	if code, reason := panellisten.ChoiceRefusal(port, panelPortReservedByServices()); code != "" {
		return nil, refusePanelPort(http.StatusBadRequest, code, reason)
	}
	from := listenAddr()
	if port == panelPort() {
		return nil, refusePanelPort(http.StatusBadRequest, "PANEL_PORT_UNCHANGED", fmt.Sprintf("The panel already uses port %d.", port))
	}
	if refusal := p.panelPortPeerRefusal(ctx); refusal != nil {
		return nil, refusal
	}
	config, refusal := p.panelPortConfig(ctx)
	if refusal != nil {
		return nil, refusal
	}
	to, err := panellisten.WithPort(from, port)
	if err != nil {
		return nil, err
	}
	// Binding first proves nothing else listens there, before the firewall
	// changes.
	if err := p.listenerControl.AddListener(to); err != nil {
		return nil, refusePanelPort(http.StatusConflict, "PANEL_PORT_IN_USE", fmt.Sprintf("Port %d cannot be used: something on this server already listens on it, or the system refused it. Nothing was changed. Choose another port.", port))
	}
	id, err := newServiceOperationID()
	if err != nil {
		_ = p.listenerControl.RemoveListener(to)
		return nil, err
	}
	now := panelPortNow()
	change := &panelPortChange{ID: "pp-" + id, FromListen: from, ToListen: to, EnvSHA256: config.SHA256, FromPort: panelPort(), ToPort: port, Status: "trial",
		CreatedAt: now.Format(time.RFC3339), DeadlineAt: now.Add(panelPortTrialWindow).Format(time.RFC3339)}
	if _, err := p.db.GetDB().ExecContext(ctx, `INSERT INTO panel_port_changes (id, from_listen, to_listen, env_sha256, status, created_at, deadline_at) VALUES (?, ?, ?, ?, 'trial', ?, ?)`,
		change.ID, change.FromListen, change.ToListen, change.EnvSHA256, change.CreatedAt, change.DeadlineAt); err != nil {
		_ = p.listenerControl.RemoveListener(to)
		return nil, err
	}
	panelPortState.trialPort.Store(int32(port))
	if err := panelPortSyncFirewall(p, ctx); err != nil {
		reason := fmt.Sprintf("The firewall could not open port %d (%s), so the panel stays on port %d. Open the Firewall page to see why, then try again.", port, boundedError(err), change.FromPort)
		panelPortState.trialPort.Store(0)
		_ = p.listenerControl.RemoveListener(to)
		_ = p.finishPanelPortChange(ctx, change.ID, "rolled_back", "PANEL_PORT_FIREWALL_FAILED", reason, "")
		// The trial port may have been opened before the failure; the next
		// sync closes it. A failure here is already reported above.
		_ = panelPortSyncFirewall(p, ctx)
		return nil, refusePanelPort(http.StatusBadGateway, "PANEL_PORT_FIREWALL_FAILED", reason)
	}
	p.armPanelPortTimer(change.ID, panelPortTrialWindow)
	log.Printf("[panel-port] listening on %s too; waiting for the owner's confirmation until %s", to, change.DeadlineAt)
	return change, nil
}

func boundedError(err error) string {
	text := err.Error()
	if len(text) > 200 {
		text = text[:200] + "…"
	}
	return text
}

func (p *Panel) armPanelPortTimer(id string, after time.Duration) {
	if panelPortState.timer != nil {
		panelPortState.timer.Stop()
	}
	panelPortState.timer = time.AfterFunc(after, func() { p.expirePanelPortChange(id) })
}

// expirePanelPortChange is the automatic rollback when nobody confirmed in
// time. A failed rollback is retried a minute later.
func (p *Panel) expirePanelPortChange(id string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	change, err := p.loadPanelPortChange(ctx, true)
	if err != nil || change == nil || change.ID != id || change.Status != "trial" {
		if err != nil {
			log.Printf("[panel-port] automatic rollback of %s: %v", id, err)
			time.AfterFunc(time.Minute, func() { p.expirePanelPortChange(id) })
		}
		return
	}
	if err := p.rollbackPanelPortChange(ctx, id, "PANEL_PORT_TIMEOUT", panelPortTimeoutReason(change)); err != nil {
		log.Printf("[panel-port] automatic rollback of %s: %v", id, err)
		time.AfterFunc(time.Minute, func() { p.expirePanelPortChange(id) })
	}
}

func (p *Panel) cancelPanelPortChange(ctx context.Context, id string) error {
	change, err := p.loadPanelPortChange(ctx, true)
	if err != nil {
		return err
	}
	if change == nil || change.ID != id {
		return refusePanelPort(http.StatusConflict, "PANEL_PORT_NO_CHANGE", "No port change is waiting. Reload this page.")
	}
	if change.Status != "trial" {
		return refusePanelPort(http.StatusConflict, "PANEL_PORT_CONFIRMING", "The new port is being saved now and can no longer be cancelled. Reload this page in a moment.")
	}
	return p.rollbackPanelPortChange(ctx, id, "PANEL_PORT_CANCELLED", fmt.Sprintf("The change was cancelled. The panel stays on port %d and port %d was closed again.", change.FromPort, change.ToPort))
}

// rollbackPanelPortChange ends a trial: the panel keeps its old port, the
// trial listener closes and the firewall closes the trial port.
func (p *Panel) rollbackPanelPortChange(ctx context.Context, id, code, reason string) error {
	panelPortState.mu.Lock()
	defer panelPortState.mu.Unlock()
	change, err := p.loadPanelPortChange(ctx, true)
	if err != nil {
		return err
	}
	if change == nil || change.ID != id || change.Status != "trial" {
		return nil
	}
	if panelPortState.timer != nil {
		panelPortState.timer.Stop()
		panelPortState.timer = nil
	}
	panelPortState.trialPort.Store(0)
	if p.listenerControl != nil {
		_ = p.listenerControl.RemoveListener(change.ToListen)
	}
	note := ""
	if err := panelPortSyncFirewall(p, ctx); err != nil {
		note = fmt.Sprintf("Port %d may still be open in the firewall (%s). Open the Firewall page and apply it again.", change.ToPort, boundedError(err))
	}
	log.Printf("[panel-port] change %s rolled back (%s); staying on %s", id, code, change.FromListen)
	return p.finishPanelPortChange(ctx, id, "rolled_back", code, reason, note)
}

func (p *Panel) confirmPanelPortChange(ctx context.Context, id string, requestPort int) error {
	panelPortState.mu.Lock()
	defer panelPortState.mu.Unlock()
	change, err := p.loadPanelPortChange(ctx, true)
	if err != nil {
		return err
	}
	if change == nil || change.ID != id {
		return refusePanelPort(http.StatusConflict, "PANEL_PORT_NO_CHANGE", "No port change is waiting. Reload this page.")
	}
	if change.Status != "trial" {
		return refusePanelPort(http.StatusConflict, "PANEL_PORT_CONFIRMING", "The new port is being saved now. Reload this page in a moment.")
	}
	if panelPortDeadlinePassed(change) {
		return refusePanelPort(http.StatusConflict, "PANEL_PORT_TIMEOUT", "The time to confirm has passed; the panel stays on the old port. Start the change again.")
	}
	if requestPort != change.ToPort {
		return refusePanelPort(http.StatusConflict, "PANEL_PORT_CONFIRM_FROM_NEW", fmt.Sprintf("Confirm from the new address (port %d), so the panel knows it really opens for you. Open it in a new tab, sign in if asked, then confirm there.", change.ToPort))
	}
	if _, err := p.db.GetDB().ExecContext(ctx, `UPDATE panel_port_changes SET status='confirming' WHERE id=? AND status='trial'`, id); err != nil {
		return err
	}
	response, callErr := setPanelListenThroughAgent(p, ctx, transport.PanelListenSetRequest{From: change.FromListen, To: change.ToListen, ExpectedSHA256: change.EnvSHA256})
	switch {
	case callErr == nil && response.Changed:
		return p.completePanelPortChangeLocked(ctx, change)
	case callErr == nil:
		// Refused before writing: the trial continues; the owner can cancel
		// or let it time out.
		_, _ = p.db.GetDB().ExecContext(ctx, `UPDATE panel_port_changes SET status='trial' WHERE id=? AND status='confirming'`, id)
		if response.ErrorCode == "CONFIG_CHANGED" {
			return refusePanelPort(http.StatusConflict, "PANEL_PORT_CONFIG_CHANGED", "/etc/celikpanel/panel.env changed after the port change started, so CelikPanel did not overwrite it. Cancel this change, check the file, then start again.")
		}
		return refusePanelPort(http.StatusBadGateway, "PANEL_PORT_SAVE_FAILED", "The new port could not be saved ("+response.Error+"). The panel still answers on both ports; cancel the change or try confirming again.")
	}
	// The answer was lost: what panel.env says now decides.
	config, readErr := readPanelListenConfigFromAgent(p, ctx)
	if readErr == nil && config.Present && config.Listen == change.ToListen {
		return p.completePanelPortChangeLocked(ctx, change)
	}
	if readErr == nil && config.Present && config.Listen == change.FromListen {
		_, _ = p.db.GetDB().ExecContext(ctx, `UPDATE panel_port_changes SET status='trial' WHERE id=? AND status='confirming'`, id)
		return refusePanelPort(http.StatusBadGateway, "PANEL_PORT_SAVE_FAILED", "The new port could not be saved ("+boundedError(callErr)+"). The panel still answers on both ports; try confirming again or cancel the change.")
	}
	return refusePanelPort(http.StatusServiceUnavailable, "PANEL_PORT_SAVE_UNKNOWN", "The panel could not learn whether the new port was saved. It checks again when this page reloads; nothing more is changed meanwhile.")
}

// completePanelPortChangeLocked runs after panel.env names the new port.
func (p *Panel) completePanelPortChangeLocked(ctx context.Context, change *panelPortChange) error {
	if panelPortState.timer != nil {
		panelPortState.timer.Stop()
		panelPortState.timer = nil
	}
	to := change.ToListen
	confirmedPanelListen.Store(&to)
	panelPortState.trialPort.Store(0)
	if p.listenerControl != nil && p.listenerControl.HasListener(to) {
		if err := p.listenerControl.PromoteListener(to); err != nil {
			log.Printf("[panel-port] old listener %s could not be closed: %v", change.FromListen, err)
		}
	}
	note := ""
	if err := panelPortSyncFirewall(p, ctx); err != nil {
		note = fmt.Sprintf("Port %d may still be open in the firewall (%s). Open the Firewall page and apply it again.", change.FromPort, boundedError(err))
	}
	log.Printf("[panel-port] change %s confirmed; the panel now listens on %s", change.ID, to)
	return p.finishPanelPortChange(ctx, change.ID, "confirmed", "", fmt.Sprintf("The panel now uses port %d. Port %d is closed. Update your bookmarks.", change.ToPort, change.FromPort), note)
}

func (p *Panel) finishPanelPortChange(ctx context.Context, id, status, code, reason, note string) error {
	_, err := p.db.GetDB().ExecContext(ctx, `UPDATE panel_port_changes SET status=?, reason_code=?, reason=?, firewall_note=?, finished_at=? WHERE id=?`,
		status, code, reason, note, panelPortNow().Format(time.RFC3339), id)
	return err
}

// loadPanelPortChange returns the open change (open=true) or the latest one.
func (p *Panel) loadPanelPortChange(ctx context.Context, open bool) (*panelPortChange, error) {
	query := `SELECT id, from_listen, to_listen, env_sha256, status, reason_code, reason, firewall_note, created_at, deadline_at, finished_at FROM panel_port_changes `
	if open {
		query += `WHERE status IN ('trial', 'confirming') `
	}
	query += `ORDER BY created_at DESC, rowid DESC LIMIT 1`
	var change panelPortChange
	err := p.db.GetDB().QueryRowContext(ctx, query).Scan(&change.ID, &change.FromListen, &change.ToListen, &change.EnvSHA256, &change.Status, &change.ReasonCode, &change.Reason, &change.FirewallNote, &change.CreatedAt, &change.DeadlineAt, &change.FinishedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	change.FromPort, _ = panellisten.Port(change.FromListen)
	change.ToPort, _ = panellisten.Port(change.ToListen)
	return &change, nil
}

// reconcilePanelPortAtStart settles a change a previous process left open.
// A trial listener never survives a restart, so an unconfirmed trial ends
// with the old port; a confirmation in flight is decided by what panel.env
// says (and therefore what this process listens on).
func (p *Panel) reconcilePanelPortAtStart(ctx context.Context) error {
	panelPortState.mu.Lock()
	defer panelPortState.mu.Unlock()
	change, err := p.loadPanelPortChange(ctx, true)
	if err != nil || change == nil {
		return err
	}
	running := listenAddr()
	if running == change.ToListen {
		note := ""
		if err := panelPortSyncFirewall(p, ctx); err != nil {
			note = fmt.Sprintf("Port %d may still be open in the firewall (%s). Open the Firewall page and apply it again.", change.FromPort, boundedError(err))
		}
		return p.finishPanelPortChange(ctx, change.ID, "confirmed", "", fmt.Sprintf("The panel now uses port %d. Port %d is closed. Update your bookmarks.", change.ToPort, change.FromPort), note)
	}
	code, reason := "PANEL_PORT_RESTARTED", fmt.Sprintf("The panel restarted before the new port %d was confirmed, so it stays on port %d. Start the change again when you are ready.", change.ToPort, change.FromPort)
	if change.Status == "confirming" {
		config, readErr := readPanelListenConfigFromAgent(p, ctx)
		if readErr != nil || !config.Present {
			return fmt.Errorf("panel port change %s: panel.env could not be read yet", change.ID)
		}
		if config.Listen == change.ToListen {
			// Saved, but this process started with the old address: the next
			// restart uses the new port. Say so instead of guessing.
			return p.finishPanelPortChange(ctx, change.ID, "confirmed", "PANEL_PORT_RESTART_NEEDED", fmt.Sprintf("Port %d is saved, but the panel still runs on port %d until it restarts. Restart it (sudo systemctl restart celikpanel-panel), then open the new address.", change.ToPort, change.FromPort), "")
		}
		code, reason = "PANEL_PORT_CONFIRM_NOT_SAVED", fmt.Sprintf("The panel stopped while saving port %d, and the new port was not saved. It stays on port %d. Start the change again.", change.ToPort, change.FromPort)
	}
	note := ""
	if err := panelPortSyncFirewall(p, ctx); err != nil {
		note = fmt.Sprintf("Port %d may still be open in the firewall (%s). Open the Firewall page and apply it again.", change.ToPort, boundedError(err))
	}
	return p.finishPanelPortChange(ctx, change.ID, "rolled_back", code, reason, note)
}

// startPanelPortReconciler settles a change left open by a previous process,
// retrying while the Agent cannot answer yet.
func (p *Panel) startPanelPortReconciler(ctx context.Context) {
	go func() {
		for attempt := 0; attempt < 30; attempt++ {
			callCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
			err := p.reconcilePanelPortAtStart(callCtx)
			cancel()
			if err == nil {
				return
			}
			log.Printf("[panel-port] open change not settled yet: %v", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(20 * time.Second):
			}
		}
	}()
}
