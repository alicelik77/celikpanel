//go:build linux

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/mutationpayload"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Defect D5 (alpha.113 final lab, Arch standalone PowerDNS, 2026-10-31): a
// standalone server does not serve the parent of a deleted site zone, so
// PowerDNS answers an empty REFUSED and the strict parent-negative rule could
// never pass. The publication poisoned the ledger and every Agent restart
// re-derived the same hold. These tests use the real PowerDNS database
// tombstone and engine state; only the DNS answer and the host address are
// injected.

const standaloneDeleteProofAddress = "198.51.100.7"

type standalonePDNSDeletionFixture struct {
	state   dnsEngineStateReceipt
	zone    transport.DNSEngineSwitchZoneSnapshot
	binding transport.ServiceMutationBinding
	dbPath  string
}

func newStandalonePDNSDeletionFixture(t *testing.T, stateDir string) standalonePDNSDeletionFixture {
	t.Helper()
	ctx := context.Background()
	t.Setenv("CELIKPANEL_AGENT_STATE_DIR", stateDir)
	previousConf := dnsClusterConf
	previousAddress := standalonePDNSDeletionProofAddress
	previousProbe := probeDNSZoneSOA
	dnsClusterConf = filepath.Join(t.TempDir(), "absent-celikpanel-cluster.conf")
	standalonePDNSDeletionProofAddress = func(context.Context) (string, error) {
		return standaloneDeleteProofAddress, nil
	}
	t.Cleanup(func() {
		dnsClusterConf = previousConf
		standalonePDNSDeletionProofAddress = previousAddress
		probeDNSZoneSOA = previousProbe
	})

	state := legacyDurableDNSState(transport.DNSEnginePowerDNS)
	if err := writeDNSEngineState(state); err != nil {
		t.Fatalf("write standalone PowerDNS state: %v", err)
	}
	dbPath := filepath.Join(t.TempDir(), "pdns.sqlite3")
	db, err := initializePDNSEngineDB(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := initializePDNSEngineReceipts(ctx, db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO celikpanel_dns_engine_manifest_receipt
		 (singleton, engine, engine_epoch, request_id, owner_id, qualifier,
		  source_revision, zone_count, snapshot_bytes, schema)
		VALUES (1, 'pdns', ?, ?, ?, ?, ?, 0, 0, ?)`,
		state.EngineEpoch, state.MutationRequestID, state.MutationOwnerID,
		state.ManifestQualifier, state.SourceRevision, pdnsManifestSchema,
	); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CELIKPANEL_PDNS_DB", dbPath)

	const domain = "archr3shop113.test"
	binding := transport.ServiceMutationBinding{
		MutationRequestID: strings.Repeat("e", 32),
		MutationOwnerID:   strings.Repeat("f", 32),
	}
	create, err := mutationpayload.CanonicalDNSZoneSyncV3(
		transport.DNSEnginePowerDNS, 1, 1, domain, false, "NATIVE",
		testPDNSEngineRecords(domain),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := applyPDNSV3ZoneDatabaseForState(ctx, dbPath, create, binding, state); err != nil {
		t.Fatalf("publish standalone zone: %v", err)
	}
	deletion, err := mutationpayload.CanonicalDNSZoneSyncV3(
		transport.DNSEnginePowerDNS, 1, 2, domain, true, "NATIVE", nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := applyPDNSV3ZoneDatabaseForState(ctx, dbPath, deletion, binding, state); err != nil {
		t.Fatalf("delete standalone zone: %v", err)
	}
	return standalonePDNSDeletionFixture{
		state: state, binding: binding, dbPath: dbPath,
		zone: transport.DNSEngineSwitchZoneSnapshot{
			Domain: domain, DesiredGeneration: deletion.DesiredGeneration,
			Delete: true, ZoneType: deletion.ZoneType, Records: deletion.Records,
			ZoneQualifier: deletion.Qualifier,
		},
	}
}

func standaloneDeleteProbe(result dnsSOAProbeResult, err error) dnsZoneSOAProbe {
	return func(_ context.Context, _ string, address, _ string) (dnsSOAProbeResult, error) {
		if address != standaloneDeleteProofAddress {
			return dnsSOAProbeResult{}, errors.New("probe reached a foreign address")
		}
		return result, err
	}
}

func TestStandalonePDNSDeletionProofBindsEmptyRefusedToDatabaseTombstone(t *testing.T) {
	root := mutationTestRoot(t)
	stateDir := filepath.Join(root, "state")
	if err := os.Mkdir(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	fixture := newStandalonePDNSDeletionFixture(t, stateDir)
	ctx := context.Background()
	verify := func() error {
		return verifyPDNSV3PublishedZoneAuthority(ctx, fixture.state, fixture.zone, fixture.binding)
	}
	refused := dnsSOAProbeResult{LocalIP: standaloneDeleteProofAddress, RCode: dnsRCodeRefused}

	probeDNSZoneSOA = standaloneDeleteProbe(refused, nil)
	if err := verify(); err != nil {
		t.Fatalf("standalone deletion with empty REFUSED and exact tombstone failed: %v", err)
	}
	ancestor := dnsSOAProbeResult{
		LocalIP: standaloneDeleteProofAddress, ExactDeletedZone: true, Authoritative: true,
		RCode: dnsRCodeNameError, AuthoritySOAOwners: []string{"test"},
	}
	probeDNSZoneSOA = standaloneDeleteProbe(ancestor, nil)
	if err := verify(); err != nil {
		t.Fatalf("standalone deletion with an ancestor SOA failed: %v", err)
	}

	for _, tc := range []struct {
		name   string
		result dnsSOAProbeResult
		err    error
	}{
		{"servfail", dnsSOAProbeResult{LocalIP: standaloneDeleteProofAddress, RCode: 2}, nil},
		{"timeout", dnsSOAProbeResult{}, context.DeadlineExceeded},
		{"zone soa still served", dnsSOAProbeResult{
			LocalIP: standaloneDeleteProofAddress, Authoritative: true, RCode: dnsRCodeNoError,
			AnswerCount: 1, SOASerials: []uint32{2026081601}, AnswerSOAOwners: []string{fixture.zone.Domain},
		}, nil},
		{"refused from another socket", dnsSOAProbeResult{LocalIP: "198.51.100.8", RCode: dnsRCodeRefused}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			probeDNSZoneSOA = standaloneDeleteProbe(tc.result, tc.err)
			if err := verify(); err == nil {
				t.Fatal("unverifiable standalone deletion answer was accepted")
			}
		})
	}

	t.Run("zone row reappears during the DNS observation", func(t *testing.T) {
		probeDNSZoneSOA = func(ctx context.Context, network, address, domain string) (dnsSOAProbeResult, error) {
			if network == "tcp" {
				db, err := openPDNSEngineDB(fixture.dbPath, false)
				if err != nil {
					return dnsSOAProbeResult{}, err
				}
				defer db.Close()
				if _, err := db.ExecContext(ctx,
					`INSERT INTO domains (name, type) VALUES (?, 'NATIVE')`, domain,
				); err != nil {
					return dnsSOAProbeResult{}, err
				}
			}
			return refused, nil
		}
		if err := verify(); err == nil {
			t.Fatal("a zone row restored during the proof was accepted as deleted")
		}
		db, err := openPDNSEngineDB(fixture.dbPath, false)
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		if _, err := db.Exec(`DELETE FROM domains WHERE name = ?`, fixture.zone.Domain); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("legacy cluster pairing keeps the strict rule", func(t *testing.T) {
		previous := pdnsLegacyClusterPairingConfigured
		pdnsLegacyClusterPairingConfigured = func(context.Context) (bool, error) { return true, nil }
		t.Cleanup(func() { pdnsLegacyClusterPairingConfigured = previous })
		probeDNSZoneSOA = standaloneDeleteProbe(refused, nil)
		if err := verify(); err == nil {
			t.Fatal("legacy paired primary accepted an empty REFUSED")
		}
	})
}

// standaloneRecoveryBackend runs recoverPDNSV3Zone's database and authority
// proof (readPDNSV3ZoneSnapshot + verifyPDNSV3PublishedZoneAuthority) without
// the host-only systemd, cluster and cache-purge checks a unit test cannot run.
type standaloneRecoveryBackend struct {
	fakeDNSEngineBackend
	calls int
}

func (backend *standaloneRecoveryBackend) RecoverZone(
	ctx context.Context, domain, qualifier string, binding transport.ServiceMutationBinding,
) (bool, error) {
	backend.calls++
	state, exists, err := readDNSEngineState()
	if err != nil || !exists {
		return false, errors.Join(err, errors.New("DNS engine state is missing"))
	}
	snapshot, exact, err := readPDNSV3ZoneSnapshot(ctx, pdnsDBPath(), state, domain, qualifier, binding)
	if err != nil || !exact {
		return false, err
	}
	if err := verifyPDNSV3PublishedZoneAuthority(ctx, state, snapshot, binding); err != nil {
		return false, err
	}
	return true, nil
}

func persistAppliedDNSZoneV3StartupLedger(
	t *testing.T, manager *serviceMutationManager, fixture standalonePDNSDeletionFixture,
) {
	t.Helper()
	phase, err := formatDNSZoneSyncV3Phase(
		dnsZoneSyncV3Applied, fixture.binding.MutationRequestID,
		fixture.zone.Domain, fixture.zone.ZoneQualifier,
	)
	if err != nil {
		t.Fatal(err)
	}
	now := manager.now()
	job := &ServiceMutationJob{
		RequestID: fixture.binding.MutationRequestID, OwnerID: fixture.binding.MutationOwnerID,
		Kind: "dns_zone_sync", Target: fixture.zone.Domain, PackageName: fixture.zone.ZoneQualifier,
		Status: serviceMutationStatusRunning, Phase: phase, Attempt: 1,
		StartedAt: now.Add(-2 * time.Minute), UpdatedAt: now,
		LeaseExpiresAt: now.Add(time.Minute), DeadlineAt: now.Add(time.Hour),
	}
	ledger := serviceMutationLedger{
		Version: serviceMutationLedgerVersion, ActiveRequestID: job.RequestID,
		Jobs: map[string]*ServiceMutationJob{job.RequestID: job},
	}
	if err := validateServiceMutationLedger(&ledger); err != nil {
		t.Fatalf("invalid applied DNS zone V3 ledger fixture: %v", err)
	}
	manager.mu.Lock()
	before := cloneServiceMutationLedger(manager.ledger)
	manager.ledger = ledger
	err = manager.persistLedgerMutationLocked(before)
	manager.mu.Unlock()
	if err != nil {
		t.Fatalf("persist applied DNS zone V3 ledger: %v", err)
	}
}

// A server latched by the old proof keeps only the applied ledger job; the
// hold itself is recomputed at every Agent start. With a verifiable answer the
// owner's restart completes the same operation; with an unknown answer the
// hold stays.
func TestDNSZoneV3StartupRecoveryCompletesLatchedStandaloneDeletion(t *testing.T) {
	manager, root := newMutationTestManager(t)
	stateDir := filepath.Join(root, "state")
	lockPath := filepath.Join(root, "service-mutation.lock")
	fixture := newStandalonePDNSDeletionFixture(t, stateDir)
	persistAppliedDNSZoneV3StartupLedger(t, manager, fixture)
	backend := &standaloneRecoveryBackend{}
	useFakeDNSEngineBackend(t, backend)

	// First start: the answer is unknown (SERVFAIL). Fail closed, keep the job.
	probeDNSZoneSOA = standaloneDeleteProbe(
		dnsSOAProbeResult{LocalIP: standaloneDeleteProofAddress, RCode: 2}, nil,
	)
	held, err := newServiceMutationManager(stateDir, lockPath)
	if err == nil || held == nil || held.poisoned == nil {
		t.Fatalf("unknown deletion answer did not hold mutations: manager=%v err=%v", held != nil, err)
	}
	if job := held.status(fixture.binding.MutationRequestID); job == nil ||
		job.Status != serviceMutationStatusRunning {
		t.Fatalf("held recovery exposed a terminal receipt: %+v", job)
	}
	releasePoisonedFirewallApplyTestManager(held)

	// Owner restarts the Agent: the parentless server answers empty REFUSED.
	probeDNSZoneSOA = standaloneDeleteProbe(
		dnsSOAProbeResult{LocalIP: standaloneDeleteProofAddress, RCode: dnsRCodeRefused}, nil,
	)
	recovered, err := newServiceMutationManager(stateDir, lockPath)
	if err != nil || recovered == nil {
		t.Fatalf("restart did not recover the standalone deletion: %v", err)
	}
	recovered.mu.Lock()
	healthErr := recovered.healthErrorLocked()
	active := recovered.ledger.ActiveRequestID
	recovered.mu.Unlock()
	if healthErr != nil || active != "" {
		t.Fatalf("recovered manager still held: health=%v active=%q", healthErr, active)
	}
	wantPhase, err := formatDNSZoneSyncV3PublishedPhase(
		fixture.binding.MutationRequestID, fixture.zone.Domain, fixture.zone.ZoneQualifier,
	)
	if err != nil {
		t.Fatal(err)
	}
	job := recovered.status(fixture.binding.MutationRequestID)
	if job == nil || job.Status != serviceMutationStatusSucceeded || job.Phase != wantPhase {
		t.Fatalf("recovered deletion job=%+v", job)
	}
	if backend.calls != 2 {
		t.Fatalf("recovery calls=%d want 2", backend.calls)
	}
	// The next site can be published: a new mutation is admitted.
	if _, err := recovered.begin(&ServiceMutationBeginRequest{
		RequestID: strings.Repeat("1", 32), OwnerID: strings.Repeat("2", 32),
		Kind: "dns_zone_sync", Target: "next113.test",
		PackageName: "dns-zone-sync/v3:sha256:" + strings.Repeat("3", 64),
	}); err != nil {
		t.Fatalf("recovered Agent refused the next mutation: %v", err)
	}
	abandonFirewallApplyTestRuntime(t, recovered)
}
