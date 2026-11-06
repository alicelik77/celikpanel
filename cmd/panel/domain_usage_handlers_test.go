package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/rpc"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

type siteUsageTestAgent struct {
	calls   atomic.Int32
	release chan struct{}
}

func (a *siteUsageTestAgent) SiteUsage(req *transport.SiteUsageRequest, resp *transport.SiteUsageResponse) error {
	a.calls.Add(1)
	if a.release != nil {
		<-a.release
	}
	if !req.DiskOnly {
		resp.Error = "a page read must ask for the disk only"
		return nil
	}
	resp.DiskBytes = 4096
	return nil
}

func newSiteUsageTestPanel(t *testing.T, agent *siteUsageTestAgent, usageUpdatedAt any) *Panel {
	t.Helper()
	database, err := openTemplatePanelDatabase(filepath.Join(t.TempDir(), "panel.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Close)
	server := rpc.NewServer()
	if err := server.RegisterName("Agent", agent); err != nil {
		t.Fatal(err)
	}
	connector := func(ctx context.Context) (*rpc.Client, error) {
		serverConn, clientConn := net.Pipe()
		go server.ServeConn(serverConn)
		return rpc.NewClient(clientConn), nil
	}
	client, _ := connector(context.Background())
	t.Cleanup(func() { _ = client.Close() })
	db := database.GetDB()
	if _, err := db.Exec(`INSERT INTO users (id, username, password_hash, email, role) VALUES (1, 'usage', 'x', 'usage@example.test', 'customer')`); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO subscriptions (id, owner_id, name) VALUES (1, 1, 'usage')`); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO domains (id, subscription_id, name, status) VALUES (7, 1, 'usage.test', 'active')`); err != nil {
		t.Fatalf("seed domain: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO sites (domain_id, document_root, disk_usage_bytes, usage_updated_at)
		VALUES (7, '/var/www/celikpanel/subscriptions/1/sites/7/public', 1024, ?)`, usageUpdatedAt); err != nil {
		t.Fatalf("seed site: %v", err)
	}
	return &Panel{agentClient: transport.NewReconnectingClientWithContextConnector(client, connector), db: database}
}

func readSiteUsage(t *testing.T, p *Panel) map[string]any {
	t.Helper()
	recorder := httptest.NewRecorder()
	p.handleDomainUsage(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/domains/7/usage", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body
}

// A recent measurement is shown as it is: the page does not walk the files.
func TestDomainUsageRecentMeasurementIsShownWithoutMeasuring(t *testing.T) {
	agent := &siteUsageTestAgent{}
	p := newSiteUsageTestPanel(t, agent, time.Now().UTC().Format(time.RFC3339))
	body := readSiteUsage(t, p)
	if body["disk_usage"] != float64(1024) || body["rechecking"] != false || agent.calls.Load() != 0 {
		t.Fatalf("body = %v, agent calls = %d; want the stored figure, no measurement", body, agent.calls.Load())
	}
}

// An old measurement is shown at once with its time while one measurement
// runs behind the page; asking again does not start a second one.
func TestDomainUsageOldMeasurementIsShownWhileMeasuredAgain(t *testing.T) {
	agent := &siteUsageTestAgent{release: make(chan struct{})}
	old := time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339)
	p := newSiteUsageTestPanel(t, agent, old)
	for i := 0; i < 3; i++ {
		body := readSiteUsage(t, p)
		if body["disk_usage"] != float64(1024) || body["rechecking"] != true || body["measured_at"] != old {
			t.Fatalf("read %d = %v; want the stored figure with its time, rechecking", i, body)
		}
	}
	close(agent.release)
	deadline := time.Now().Add(5 * time.Second)
	for {
		body := readSiteUsage(t, p)
		if body["disk_usage"] == float64(4096) && body["rechecking"] == false {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the measurement behind the page never arrived: %v", body)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if agent.calls.Load() != 1 {
		t.Fatalf("measurements = %d, want 1", agent.calls.Load())
	}
}

// A site never measured waits for its first measurement; 0 is never shown
// as a measurement.
func TestDomainUsageFirstMeasurementIsWaitedFor(t *testing.T) {
	agent := &siteUsageTestAgent{}
	p := newSiteUsageTestPanel(t, agent, nil)
	body := readSiteUsage(t, p)
	if body["disk_usage"] != float64(4096) || body["rechecking"] != false || agent.calls.Load() != 1 {
		t.Fatalf("body = %v, calls = %d; want the first measurement", body, agent.calls.Load())
	}
}
