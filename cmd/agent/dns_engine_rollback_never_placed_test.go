package main

import (
	"context"
	"errors"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Beta-1 re-run P6: BIND's install failed at APT simulation ("no
// installation candidate"), so nothing was placed. Its exact install receipt
// existed and the guard had restored the units (no masks), so the sealed
// proof failed and the evidence said "runtime unsealed": the setup step stayed
// paused until a panel restart. A host that shows none of the request's
// missing packages, no target unit running and nothing on port 53 is now
// proven rolled back.
func TestDNSEngineRollbackEvidenceProvesAFirstInstallThatNeverPlacedAPackage(t *testing.T) {
	withRollbackEvidenceReadersForTest(t)
	request, manifest := rollbackEvidenceRequestForTest(t, "", 0)
	receipt := installOwnershipForEvidenceTest(request, manifest)
	readRollbackEvidenceJournal = func() (dnsEngineSwitchJournal, bool, error) {
		return dnsEngineSwitchJournal{}, false, nil
	}
	readRollbackEvidenceState = func() (dnsEngineStateReceipt, bool, error) {
		return dnsEngineStateReceipt{}, false, nil
	}
	readRollbackEvidenceOwnership = func(transport.DNSEngine) (dnsEngineStateReceipt, bool, error) {
		return dnsEngineStateReceipt{}, false, nil
	}
	installExists := true
	readRollbackEvidenceInstallOwnership = func(transport.DNSEngine) (dnsEngineInstallOwnershipReceipt, bool, error) {
		return receipt, installExists, nil
	}
	// The guard restored what it found: no masks, so the sealed proof fails.
	verifyRollbackEvidenceTargetSeal = func(context.Context, transport.DNSEngine, dnsEngineRollbackTargetHost) error {
		return errors.New("BIND target is not exactly persistently masked and inactive")
	}
	neverPlaced, proofErr := true, error(nil)
	var proved dnsEngineInstallOwnershipReceipt
	readRollbackEvidenceNeverPlaced = func(_ context.Context, target transport.DNSEngine, install dnsEngineInstallOwnershipReceipt, _ dnsEngineRollbackTargetHost) (bool, error) {
		if target != transport.DNSEngineBIND {
			t.Fatalf("proved the wrong engine: %s", target)
		}
		proved = install
		return neverPlaced, proofErr
	}
	outcome, err := classifyDNSEngineRollbackHostEvidence(context.Background(), request, manifest)
	if err != nil || outcome != transport.DNSEngineRollbackSafe || proved.MutationRequestID != request.MutationRequestID {
		t.Fatalf("never-placed install: outcome=%q err=%v proved=%+v", outcome, err, proved)
	}
	// A package that is present again is the sealed proof's case, as before.
	neverPlaced = false
	if outcome, err := classifyDNSEngineRollbackHostEvidence(context.Background(), request, manifest); err != nil || outcome != transport.DNSEngineRollbackRuntimeUnsealed {
		t.Fatalf("placed package: outcome=%q err=%v", outcome, err)
	}
	// A proof that cannot be read proves nothing.
	neverPlaced, proofErr = true, errors.New("dpkg-query unavailable")
	if outcome, _ := classifyDNSEngineRollbackHostEvidence(context.Background(), request, manifest); outcome != transport.DNSEngineRollbackUnverified {
		t.Fatalf("unreadable proof: outcome=%q", outcome)
	}
	// Without this request's receipt there is no install to speak for.
	neverPlaced, proofErr, installExists = true, nil, false
	if outcome, _ := classifyDNSEngineRollbackHostEvidence(context.Background(), request, manifest); outcome != transport.DNSEngineRollbackRuntimeUnsealed {
		t.Fatalf("no receipt: outcome=%q", outcome)
	}
}

func TestDNSEngineInstallNeverPlacedProofRefusesAnythingTheInstallCouldHaveLeft(t *testing.T) {
	receipt := dnsEngineInstallOwnershipReceipt{Engine: transport.DNSEngineBIND, PackageManager: "apt", Packages: []string{"bind9", "bind9-utils"}, MissingBefore: []string{"bind9", "bind9-utils"}}
	absentUnit := bindInstallUnitState{name: "named.service", loadState: "not-found", activeState: "inactive"}
	for _, test := range []struct {
		name      string
		receipt   func(dnsEngineInstallOwnershipReceipt) dnsEngineInstallOwnershipReceipt
		installed map[string][]bool
		unit      bindInstallUnitState
		conflict  bool
		want      bool
	}{
		{name: "nothing placed", unit: absentUnit, want: true},
		{name: "a package placed", installed: map[string][]bool{"bind9-utils": {true}}, unit: absentUnit},
		{name: "a package placed during the proof", installed: map[string][]bool{"bind9": {false, true}}, unit: absentUnit},
		{name: "a unit running", unit: bindInstallUnitState{name: "named.service", loadState: "loaded", activeState: "active"}},
		{name: "a unit failed", unit: bindInstallUnitState{name: "named.service", loadState: "loaded", activeState: "failed"}},
		{name: "a unit still masked", unit: bindInstallUnitState{name: "named.service", loadState: "masked", activeState: "inactive", unitFileState: "masked"}},
		{name: "port 53 served", unit: absentUnit, conflict: true},
		{name: "packages adopted, not installed", receipt: func(r dnsEngineInstallOwnershipReceipt) dnsEngineInstallOwnershipReceipt {
			r.AdoptedPresent = true
			return r
		}, unit: absentUnit},
		{name: "nothing was missing", receipt: func(r dnsEngineInstallOwnershipReceipt) dnsEngineInstallOwnershipReceipt {
			r.MissingBefore = nil
			return r
		}, unit: absentUnit},
	} {
		t.Run(test.name, func(t *testing.T) {
			install := receipt
			if test.receipt != nil {
				install = test.receipt(install)
			}
			reads := map[string]int{}
			got, err := verifyDNSEngineInstallNeverPlacedWithOps(install, dnsEngineNeverPlacedOps{
				packageInstalled: func(name string) (bool, error) {
					answers := test.installed[name]
					index := reads[name]
					reads[name]++
					if index < len(answers) {
						return answers[index], nil
					}
					if len(answers) > 0 {
						return answers[len(answers)-1], nil
					}
					return false, nil
				},
				targetUnits:    func() ([]bindInstallUnitState, error) { return []bindInstallUnitState{test.unit}, nil },
				port53Conflict: func() (bool, error) { return test.conflict, nil },
			})
			if err != nil || got != test.want {
				t.Fatalf("got %v %v, want %v", got, err, test.want)
			}
		})
	}
}
