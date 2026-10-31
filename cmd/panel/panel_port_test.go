package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.114: new installations listen on 1923; an existing one keeps the
// port panel.env names. A change never locks the owner out: both ports
// answer until the owner confirms from the new one; otherwise the old port
// stays.

type fakePanelListeners struct {
	mu       sync.Mutex
	set      map[string]bool
	primary  string
	refuse   map[string]bool
	promoted []string
}

func newFakePanelListeners(primary string) *fakePanelListeners {
	return &fakePanelListeners{set: map[string]bool{primary: true}, primary: primary, refuse: map[string]bool{}}
}

func (f *fakePanelListeners) AddListener(listen string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.refuse[listen] {
		return errors.New("address already in use")
	}
	f.set[listen] = true
	return nil
}

func (f *fakePanelListeners) RemoveListener(listen string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if listen == f.primary {
		return errors.New("primary")
	}
	delete(f.set, listen)
	return nil
}

func (f *fakePanelListeners) PromoteListener(listen string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.set, f.primary)
	f.primary = listen
	f.promoted = append(f.promoted, listen)
	return nil
}

func (f *fakePanelListeners) HasListener(listen string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.set[listen]
}

type panelPortTestAgent struct {
	mu       sync.Mutex
	config   transport.PanelListenConfigResponse
	readErr  error
	setErr   error
	setReply *transport.PanelListenSetResponse
	sets     []transport.PanelListenSetRequest
	firewall [][]int
}

func setupPanelPortTest(t *testing.T, listen string) (*Panel, *fakePanelListeners, *panelPortTestAgent, int) {
	t.Helper()
	t.Setenv("CELIKPANEL_LISTEN", listen)
	panel, _, _, userID := newServiceStateAtomicPanel(t)
	listeners := newFakePanelListeners(listen)
	panel.listenerControl = listeners
	agent := &panelPortTestAgent{config: transport.PanelListenConfigResponse{Present: true, Listen: listen, SHA256: strings.Repeat("a", 64)}}
	restoreRead, restoreSet, restoreSync, restoreWindow, restoreNow := readPanelListenConfigFromAgent, setPanelListenThroughAgent, panelPortSyncFirewall, panelPortTrialWindow, panelPortNow
	t.Cleanup(func() {
		readPanelListenConfigFromAgent, setPanelListenThroughAgent, panelPortSyncFirewall, panelPortTrialWindow, panelPortNow = restoreRead, restoreSet, restoreSync, restoreWindow, restoreNow
		confirmedPanelListen.Store(nil)
		panelPortState.mu.Lock()
		if panelPortState.timer != nil {
			panelPortState.timer.Stop()
			panelPortState.timer = nil
		}
		panelPortState.trialPort.Store(0)
		panelPortState.mu.Unlock()
	})
	readPanelListenConfigFromAgent = func(*Panel, context.Context) (transport.PanelListenConfigResponse, error) {
		agent.mu.Lock()
		defer agent.mu.Unlock()
		return agent.config, agent.readErr
	}
	setPanelListenThroughAgent = func(_ *Panel, _ context.Context, req transport.PanelListenSetRequest) (transport.PanelListenSetResponse, error) {
		agent.mu.Lock()
		defer agent.mu.Unlock()
		agent.sets = append(agent.sets, req)
		if agent.setReply != nil {
			return *agent.setReply, agent.setErr
		}
		if agent.setErr != nil {
			return transport.PanelListenSetResponse{}, agent.setErr
		}
		agent.config.Listen = req.To
		return transport.PanelListenSetResponse{Changed: true, SHA256: strings.Repeat("b", 64)}, nil
	}
	panelPortSyncFirewall = func(*Panel, context.Context) error {
		agent.mu.Lock()
		defer agent.mu.Unlock()
		agent.firewall = append(agent.firewall, panelFirewallPorts())
		return nil
	}
	return panel, listeners, agent, userID
}

func panelPortRefusalCode(err error) string {
	var refusal *panelPortRefusal
	if errors.As(err, &refusal) {
		return refusal.code
	}
	if err != nil {
		return "error: " + err.Error()
	}
	return ""
}

func TestPanelListenDefaultIs1923(t *testing.T) {
	t.Setenv("CELIKPANEL_LISTEN", "")
	confirmedPanelListen.Store(nil)
	if listenAddr() != ":1923" || panelPort() != 1923 {
		t.Fatalf("default listen %q port %d", listenAddr(), panelPort())
	}
	t.Setenv("CELIKPANEL_LISTEN", ":2083")
	if panelPort() != 2083 {
		t.Fatalf("an installation's own port is kept: %d", panelPort())
	}
}

func TestPanelPortChangeConfirmedFromTheNewPort(t *testing.T) {
	panel, listeners, agent, _ := setupPanelPortTest(t, ":2083")
	change, err := panel.startPanelPortChange(context.Background(), 1923)
	if err != nil {
		t.Fatal(err)
	}
	if !listeners.HasListener(":1923") || !listeners.HasListener(":2083") {
		t.Fatal("both ports answer during the trial")
	}
	if got := fmt.Sprint(agent.firewall); got != "[[2083 1923]]" {
		t.Fatalf("the firewall opens the new port first and keeps the old: %s", got)
	}
	if code := panelPortRefusalCode(panel.confirmPanelPortChange(context.Background(), change.ID, 2083)); code != "PANEL_PORT_CONFIRM_FROM_NEW" {
		t.Fatalf("confirming over the old port: %q", code)
	}
	if len(agent.sets) != 0 {
		t.Fatal("nothing is written before a confirmation over the new port")
	}
	if err := panel.confirmPanelPortChange(context.Background(), change.ID, 1923); err != nil {
		t.Fatal(err)
	}
	if len(agent.sets) != 1 || agent.sets[0].From != ":2083" || agent.sets[0].To != ":1923" || agent.sets[0].ExpectedSHA256 != strings.Repeat("a", 64) {
		t.Fatalf("agent request: %+v", agent.sets)
	}
	if listenAddr() != ":1923" || listeners.HasListener(":2083") || fmt.Sprint(listeners.promoted) != "[:1923]" {
		t.Fatalf("after confirmation: listen %q listeners %+v", listenAddr(), listeners.set)
	}
	if got := fmt.Sprint(agent.firewall[len(agent.firewall)-1]); got != "[1923]" {
		t.Fatalf("the old port closes: %s", got)
	}
	latest, _ := panel.loadPanelPortChange(context.Background(), false)
	if latest.Status != "confirmed" || latest.ToPort != 1923 || latest.FromPort != 2083 {
		t.Fatalf("record: %+v", latest)
	}
}

func TestPanelPortChangeCancelAndTimeoutKeepTheOldPort(t *testing.T) {
	panel, listeners, agent, userID := setupPanelPortTest(t, ":2083")
	change, err := panel.startPanelPortChange(context.Background(), 4443)
	if err != nil {
		t.Fatal(err)
	}
	if code := panelPortRefusalCode(func() error { _, err := panel.startPanelPortChange(context.Background(), 4444); return err }()); code != "PANEL_PORT_CHANGE_OPEN" {
		t.Fatalf("a second change while one is open: %q", code)
	}
	if err := panel.cancelPanelPortChange(context.Background(), change.ID); err != nil {
		t.Fatal(err)
	}
	if listeners.HasListener(":4443") || listenAddr() != ":2083" || len(agent.sets) != 0 {
		t.Fatal("a cancelled change leaves the old port only")
	}
	if got := fmt.Sprint(agent.firewall[len(agent.firewall)-1]); got != "[2083]" {
		t.Fatalf("the trial port closes again: %s", got)
	}
	latest, _ := panel.loadPanelPortChange(context.Background(), false)
	if latest.Status != "rolled_back" || latest.ReasonCode != "PANEL_PORT_CANCELLED" {
		t.Fatalf("record: %+v", latest)
	}

	// No confirmation in time: the view ends the trial (the timer does the
	// same in the background).
	now := time.Now().UTC()
	panelPortNow = func() time.Time { return now }
	change, err = panel.startPanelPortChange(context.Background(), 4445)
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(panelPortTrialWindow + time.Second)
	// A read past the deadline changes nothing by itself; the timer does.
	if open, _ := panel.loadPanelPortChange(context.Background(), true); open == nil || open.Status != "trial" {
		t.Fatal("the trial is still open until its timer runs")
	}
	panel.expirePanelPortChange(change.ID)
	recorder := httptest.NewRecorder()
	panel.handlePanelPort(recorder, serviceStateAtomicRequest(http.MethodGet, panelPortPath, "", userID))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"reason_code":"PANEL_PORT_TIMEOUT"`) || !strings.Contains(recorder.Body.String(), `"port":2083`) {
		t.Fatalf("timeout view: %d %s", recorder.Code, recorder.Body.String())
	}
	if listeners.HasListener(":4445") || panel.confirmPanelPortChange(context.Background(), change.ID, 4445) == nil {
		t.Fatal("a timed out change cannot be confirmed")
	}
}

func TestPanelPortChangeTimerRollsBack(t *testing.T) {
	panel, listeners, _, _ := setupPanelPortTest(t, ":2083")
	panelPortTrialWindow = 30 * time.Millisecond
	if _, err := panel.startPanelPortChange(context.Background(), 4446); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		latest, _ := panel.loadPanelPortChange(context.Background(), false)
		if latest != nil && latest.Status == "rolled_back" {
			if listeners.HasListener(":4446") || activePanelPortTrialPort() != 0 {
				t.Fatal("the trial listener stays after the automatic rollback")
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("no automatic rollback")
}

func TestPanelPortChangeRefusals(t *testing.T) {
	panel, listeners, agent, _ := setupPanelPortTest(t, ":2083")
	start := func(port int) string {
		_, err := panel.startPanelPortChange(context.Background(), port)
		return panelPortRefusalCode(err)
	}
	for port, want := range map[int]string{443: "PANEL_PORT_RANGE", 80: "PANEL_PORT_RANGE", 70000: "PANEL_PORT_RANGE", 3306: "PANEL_PORT_RESERVED", 2083: "PANEL_PORT_UNCHANGED"} {
		if got := start(port); got != want {
			t.Fatalf("port %d: %q, want %q", port, got, want)
		}
	}
	listeners.refuse[":5000"] = true
	if got := start(5000); got != "PANEL_PORT_IN_USE" {
		t.Fatalf("bound elsewhere: %q", got)
	}
	agent.config.Listen = ":8443"
	if got := start(5001); got != "PANEL_PORT_CONFIG_CHANGED" {
		t.Fatalf("owner edit: %q", got)
	}
	agent.config.Listen = ":2083"
	agent.readErr = errors.New("agent down")
	if got := start(5001); got != "PANEL_PORT_AGENT_UNAVAILABLE" {
		t.Fatalf("agent down: %q", got)
	}
	agent.readErr = nil
	agent.config.Present = false
	if got := start(5001); got != "PANEL_PORT_CONFIG_MISSING" {
		t.Fatalf("no panel.env: %q", got)
	}
	agent.config.Present = true
	if _, err := panel.db.GetDB().Exec(`INSERT INTO remote_dns_clients (id, credential_hash, label, created_at) VALUES ('c1', 'h', 'boston', '2026-10-31T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if got := start(5001); got != "PANEL_PORT_PAIRED" {
		t.Fatalf("a peer stores this server's port: %q", got)
	}
	if len(agent.firewall) != 0 || len(listeners.set) != 1 {
		t.Fatal("a refused change changed nothing")
	}
}

func TestPanelPortConfirmSurvivesALostAnswerAndRefusesAnEditedFile(t *testing.T) {
	panel, _, agent, _ := setupPanelPortTest(t, ":2083")
	change, err := panel.startPanelPortChange(context.Background(), 1923)
	if err != nil {
		t.Fatal(err)
	}
	agent.setReply = &transport.PanelListenSetResponse{ErrorCode: "CONFIG_CHANGED", Error: "changed"}
	if code := panelPortRefusalCode(panel.confirmPanelPortChange(context.Background(), change.ID, 1923)); code != "PANEL_PORT_CONFIG_CHANGED" {
		t.Fatalf("edited file: %q", code)
	}
	if open, _ := panel.loadPanelPortChange(context.Background(), true); open == nil || open.Status != "trial" || listenAddr() != ":2083" {
		t.Fatalf("the trial continues after a refusal: %+v", open)
	}
	// The write happened but its answer was lost.
	agent.setReply, agent.setErr = &transport.PanelListenSetResponse{}, errors.New("connection reset")
	agent.config.Listen = ":1923"
	if err := panel.confirmPanelPortChange(context.Background(), change.ID, 1923); err != nil {
		t.Fatal(err)
	}
	if latest, _ := panel.loadPanelPortChange(context.Background(), false); latest.Status != "confirmed" || listenAddr() != ":1923" {
		t.Fatalf("the saved file decides: %+v %q", latest, listenAddr())
	}
}

func TestPanelPortReconcileAtStart(t *testing.T) {
	panel, _, agent, _ := setupPanelPortTest(t, ":2083")
	db := panel.db.GetDB()
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`INSERT INTO panel_port_changes (id, from_listen, to_listen, status, created_at, deadline_at) VALUES ('pp-old-trial', ':2083', ':1923', 'trial', ?, ?)`, now, now); err != nil {
		t.Fatal(err)
	}
	if err := panel.reconcilePanelPortAtStart(context.Background()); err != nil {
		t.Fatal(err)
	}
	latest, _ := panel.loadPanelPortChange(context.Background(), false)
	if latest.Status != "rolled_back" || latest.ReasonCode != "PANEL_PORT_RESTARTED" || len(agent.firewall) != 1 {
		t.Fatalf("a trial does not survive a restart: %+v", latest)
	}
	// A confirmation in flight whose file was saved: this process started
	// with the new address.
	t.Setenv("CELIKPANEL_LISTEN", ":1923")
	agent.config.Listen = ":1923"
	if _, err := db.Exec(`INSERT INTO panel_port_changes (id, from_listen, to_listen, status, created_at, deadline_at) VALUES ('pp-confirming', ':2083', ':1923', 'confirming', ?, ?)`, time.Now().UTC().Add(time.Second).Format(time.RFC3339), now); err != nil {
		t.Fatal(err)
	}
	if err := panel.reconcilePanelPortAtStart(context.Background()); err != nil {
		t.Fatal(err)
	}
	latest, _ = panel.loadPanelPortChange(context.Background(), false)
	if latest.ID != "pp-confirming" || latest.Status != "confirmed" {
		t.Fatalf("a saved confirmation is completed: %+v", latest)
	}
}

func TestRequestLocalPortReadsTheListenerPort(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request = request.WithContext(context.WithValue(request.Context(), http.LocalAddrContextKey, &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1923}))
	if requestLocalPort(request) != 1923 {
		t.Fatal("local port")
	}
}

// A real server answers on a trial listener with the same handler, and
// promoting it retires the old one without ending the process.
func TestRunningPanelHTTPServerAddsPromotesAndRetiresListeners(t *testing.T) {
	server := newPanelHTTPServer("127.0.0.1:0", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, requestLocalPort(r))
	}))
	running, err := startPanelHTTP(server, "", "")
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	trial := probe.Addr().String()
	probe.Close()
	if err := running.AddListener(trial); err != nil {
		t.Fatal(err)
	}
	get := func(address string) (string, error) {
		client := http.Client{Timeout: 2 * time.Second}
		response, err := client.Get("http://" + address + "/")
		if err != nil {
			return "", err
		}
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		return string(body), err
	}
	old := running.addr.String()
	if _, err := get(trial); err != nil {
		t.Fatalf("trial listener: %v", err)
	}
	if err := running.PromoteListener(trial); err != nil {
		t.Fatal(err)
	}
	if _, err := get(old); err == nil {
		t.Fatal("the retired listener still answers")
	}
	if _, err := get(trial); err != nil {
		t.Fatalf("the promoted listener: %v", err)
	}
	select {
	case err := <-running.serveResult:
		t.Fatalf("retiring the old listener ended the server: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
}
