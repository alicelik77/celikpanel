package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// alpha.125 (P2): the page's scan only reads, so it does not hold the
// mutation lease while it reads. A change pressed during the scan is not
// refused because of it, and a reading that raced a change is not stored:
// the change's own scan writes the record.
func TestManualScanDoesNotHoldTheLeaseAndDiscardsARacedReading(t *testing.T) {
	panel, agent, database, userID := newServiceStateAtomicPanel(t)
	const oldData = `{"observations":[{"id":"redis","is_installed":false,"status":"inactive (dead)"}]}`
	const oldScannedAt = "2026-07-20T01:02:03Z"
	if _, err := database.GetDB().Exec(
		`INSERT INTO service_scan_cache (id, data, scanned_at) VALUES (1, ?, ?)`,
		oldData, oldScannedAt,
	); err != nil {
		t.Fatal(err)
	}
	leaseFree := false
	agent.getServicesHook = func() {
		// A start/stop pressed while the scan reads gets the lease.
		if panel.serviceMutationMu.TryLock() {
			leaseFree = true
			panel.serviceMutationMu.Unlock()
		}
	}
	recorder := httptest.NewRecorder()
	panel.handleManagedServicesScan(recorder, serviceStateAtomicRequest(
		http.MethodPost, "/api/v1/managed-services/scan", "", userID,
	))
	if !leaseFree {
		t.Fatal("the scan held the mutation lease while it read the host")
	}
	if recorder.Code != http.StatusConflict {
		t.Fatalf("a reading that raced a change was answered %d: %s", recorder.Code, recorder.Body.String())
	}
	var data, scannedAt string
	if err := database.GetDB().QueryRow(`SELECT data, scanned_at FROM service_scan_cache WHERE id = 1`).Scan(&data, &scannedAt); err != nil {
		t.Fatal(err)
	}
	if data != oldData || scannedAt != oldScannedAt {
		t.Fatalf("a raced reading replaced the record: %q %q", data, scannedAt)
	}

	// Without a change in between, the same scan is stored and the lease is
	// free again afterwards.
	agent.getServicesHook = nil
	recorder = httptest.NewRecorder()
	panel.handleManagedServicesScan(recorder, serviceStateAtomicRequest(
		http.MethodPost, "/api/v1/managed-services/scan", "", userID,
	))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if err := database.GetDB().QueryRow(`SELECT scanned_at FROM service_scan_cache WHERE id = 1`).Scan(&scannedAt); err != nil {
		t.Fatal(err)
	}
	if scannedAt == oldScannedAt {
		t.Fatal("an unraced reading was not stored")
	}
	if _, held := panel.serviceMutationMu.Generation(); held {
		t.Fatal("the scan left the lease held")
	}
}

func TestMutationLeaseGeneration(t *testing.T) {
	var lease mutationLease
	start, held := lease.Generation()
	if held {
		t.Fatal("a new lease is held")
	}
	lease.Lock()
	if _, held := lease.Generation(); !held {
		t.Fatal("a taken lease is not held")
	}
	if !lease.UnchangedSince(start) {
		t.Fatal("taken once since start, by the caller")
	}
	lease.Unlock()
	if !lease.TryLock() {
		t.Fatal("TryLock on a free lease failed")
	}
	if lease.UnchangedSince(start) {
		t.Fatal("a change in between was not seen")
	}
	lease.Unlock()
}
