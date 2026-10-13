package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/rpc"
	"os"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// The Dashboard dates a scan by subtracting scan_age_seconds from the moment
// it received the answer, so the three answers that carry a scan must carry
// its age on the server's own clock (set14 D1 follow-up, 13 Oct 2026).
// Dashboard bir taramayı, yanıtı aldığı andan scan_age_seconds'ı çıkararak
// tarihler; tarama taşıyan üç yanıt da onun yaşını sunucunun kendi saatiyle
// taşımalıdır.

func insertScanCacheAt(t *testing.T, panel *Panel, scannedAt time.Time) {
	t.Helper()
	const data = `{"observations":[{"id":"nftables","is_installed":true,"status":"installed"}]}`
	if _, err := panel.db.GetDB().Exec(
		`INSERT INTO service_scan_cache (id, data, scanned_at) VALUES (1, ?, ?)`,
		data, scannedAt.UTC().Format(time.RFC3339),
	); err != nil {
		t.Fatal(err)
	}
}

// scanAgeOnWire reads scan_age_seconds from the raw JSON, so a field that
// vanished from the payload cannot pass as a zero.
func scanAgeOnWire(t *testing.T, body []byte) json.RawMessage {
	t.Helper()
	var wire map[string]json.RawMessage
	if err := json.Unmarshal(body, &wire); err != nil {
		t.Fatal(err)
	}
	raw, present := wire["scan_age_seconds"]
	if !present {
		t.Fatalf("scan_age_seconds missing from the answer: %s", body)
	}
	return raw
}

func requireScanAgeBetween(t *testing.T, body []byte, low, high float64) {
	t.Helper()
	raw := scanAgeOnWire(t, body)
	var age float64
	if err := json.Unmarshal(raw, &age); err != nil {
		t.Fatalf("scan_age_seconds = %s, want a number", raw)
	}
	if age < low || age >= high {
		t.Fatalf("scan_age_seconds = %v, want in [%v, %v)", age, low, high)
	}
}

func TestManagedServicesGetCarriesScanAge(t *testing.T) {
	t.Run("no scan", func(t *testing.T) {
		panel := newManagedServiceCachePanel(t)
		recorder := httptest.NewRecorder()
		panel.handleManagedServices(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/managed-services", nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
		}
		if raw := scanAgeOnWire(t, recorder.Body.Bytes()); string(raw) != "null" {
			t.Fatalf("scan_age_seconds = %s, want null with no scan", raw)
		}
	})
	t.Run("cached scan", func(t *testing.T) {
		panel := newManagedServiceCachePanel(t)
		// RFC3339 keeps whole seconds, so the stored stamp is up to one
		// second older than this; the age is measured from the stored stamp.
		insertScanCacheAt(t, panel, time.Now().Add(-10*time.Second))
		recorder := httptest.NewRecorder()
		panel.handleManagedServices(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/managed-services", nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
		}
		requireScanAgeBetween(t, recorder.Body.Bytes(), 10, 13)
	})
}

func TestManagedServicesConditionalCachedScanCarriesScanAge(t *testing.T) {
	panel := newManagedServiceCachePanel(t)
	insertScanCacheAt(t, panel, time.Now().Add(-40*time.Second))
	recorder := httptest.NewRecorder()
	panel.handleManagedServicesScan(recorder, httptest.NewRequest(
		http.MethodPost, "/api/v1/managed-services/scan?max_age_seconds=300", nil,
	))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	requireScanAgeBetween(t, recorder.Body.Bytes(), 40, 43)
}

func TestManagedServicesFreshScanCarriesScanAge(t *testing.T) {
	panel, agent, _, userID := newServiceStateAtomicPanel(t)
	recorder := httptest.NewRecorder()
	panel.handleManagedServicesScan(recorder, serviceStateAtomicRequest(
		http.MethodPost, "/api/v1/managed-services/scan", "", userID,
	))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if agent.getServicesCalls.Load() != 1 {
		t.Fatalf("fresh scan asked the agent %d times, want 1", agent.getServicesCalls.Load())
	}
	// Only the milliseconds between the scan and the answer.
	requireScanAgeBetween(t, recorder.Body.Bytes(), 0, 2)
}

func refusedAgentDial(errno syscall.Errno) error {
	return fmt.Errorf("cannot reach agent socket: %w", &net.OpError{
		Op: "dial", Net: "unix", Err: os.NewSyscallError("connect", errno),
	})
}

func TestAgentUnreachableNamesOnlyAFailedDial(t *testing.T) {
	for _, err := range []error{
		refusedAgentDial(syscall.ECONNREFUSED),
		refusedAgentDial(syscall.ENOENT),
		fmt.Errorf("verify privileged service mutation admission: %w", refusedAgentDial(syscall.ECONNREFUSED)),
	} {
		if !agentUnreachable(err) {
			t.Errorf("not named as unreachable: %v", err)
		}
	}
	for _, err := range []error{
		nil,
		errors.New("cannot reach agent socket: connection refused"),
		io.EOF,
		rpc.ErrShutdown,
		context.DeadlineExceeded,
		refusedAgentDial(syscall.EACCES),
		&net.OpError{Op: "read", Net: "unix", Err: os.NewSyscallError("read", syscall.ECONNRESET)},
		&net.OpError{Op: "dial", Net: "tcp", Err: os.NewSyscallError("connect", syscall.ECONNREFUSED)},
	} {
		if agentUnreachable(err) {
			t.Errorf("named as unreachable: %v", err)
		}
	}
}

func requireAgentUnreachableAnswer(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var body apiErrorBody
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != errCodeAgentUnreachable || body.Error != agentUnreachableMessage {
		t.Fatalf("body=%s", recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "socket") || strings.Contains(recorder.Body.String(), "refused") {
		t.Fatalf("internal dial text reached the client: %s", recorder.Body.String())
	}
}

// set14 O1: with the Agent stopped, the stale-cache conditional scan reached
// the mutation admission check and answered 500 INTERNAL.
func TestManagedServicesScanNamesUnreachableAgentAtAdmission(t *testing.T) {
	panel := newManagedServiceCachePanel(t)
	insertScanCacheAt(t, panel, time.Now().Add(-10*time.Minute))
	panel.agentClient = transport.NewReconnectingClientWithContextConnector(nil,
		func(context.Context) (*rpc.Client, error) { return nil, refusedAgentDial(syscall.ECONNREFUSED) })

	recorder := httptest.NewRecorder()
	panel.handleManagedServicesScan(recorder, httptest.NewRequest(
		http.MethodPost, "/api/v1/managed-services/scan?max_age_seconds=300", nil,
	))
	requireAgentUnreachableAnswer(t, recorder)
}

// unreachableAfterAdmissionAgent answers the admission check and then makes
// every later connection fail as a stopped Agent does.
type unreachableAfterAdmissionAgent struct {
	*serviceStateAtomicAgent
	down *atomic.Bool
}

func (a unreachableAfterAdmissionAgent) ServiceMutationStatus(
	req *ServiceOperationMutationStatusRequest,
	out *ServiceOperationMutationResponse,
) error {
	a.down.Store(true)
	return a.serviceStateAtomicAgent.ServiceMutationStatus(req, out)
}

func TestManagedServicesScanNamesUnreachableAgentDuringScan(t *testing.T) {
	panel, agent, _, userID := newServiceStateAtomicPanel(t)
	down := &atomic.Bool{}
	server := rpc.NewServer()
	if err := server.RegisterName("Agent", unreachableAfterAdmissionAgent{agent, down}); err != nil {
		t.Fatal(err)
	}
	panel.agentClient = transport.NewReconnectingClientWithContextConnector(nil,
		func(ctx context.Context) (*rpc.Client, error) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if down.Load() {
				return nil, refusedAgentDial(syscall.ENOENT)
			}
			serverConn, clientConn := net.Pipe()
			go server.ServeConn(serverConn)
			return rpc.NewClient(clientConn), nil
		})

	recorder := httptest.NewRecorder()
	panel.handleManagedServicesScan(recorder, serviceStateAtomicRequest(
		http.MethodPost, "/api/v1/managed-services/scan", "", userID,
	))
	if !down.Load() {
		t.Fatal("the admission check never reached the agent")
	}
	requireAgentUnreachableAnswer(t, recorder)
	if agent.getServicesCalls.Load() != 0 {
		t.Fatalf("GetServices ran %d times through a refused dial", agent.getServicesCalls.Load())
	}
}

// Any other scan failure keeps the generic answer.
func TestManagedServicesScanKeepsOtherFailuresInternal(t *testing.T) {
	panel, agent, _, userID := newServiceStateAtomicPanel(t)
	agent.getServicesError = errors.New("state probe unavailable")
	recorder := httptest.NewRecorder()
	panel.handleManagedServicesScan(recorder, serviceStateAtomicRequest(
		http.MethodPost, "/api/v1/managed-services/scan", "", userID,
	))
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var body apiErrorBody
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != errCodeInternal {
		t.Fatalf("body=%s", recorder.Body.String())
	}
}
