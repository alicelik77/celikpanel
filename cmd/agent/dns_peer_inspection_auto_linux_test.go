//go:build linux

package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/dnspeerenrollowner"
	"github.com/alicelik/celikpanel/internal/dnspeertransport"
	"github.com/alicelik/celikpanel/internal/pdnspeerenrollment"
	"github.com/alicelik/celikpanel/internal/transport"
	"golang.org/x/crypto/ssh"
)

const (
	inspectionMainIP   = "192.0.2.10"
	inspectionBackupIP = "192.0.2.11"
)

// fakeInspectionNative is a whole native host: two enrollment files on the
// main, the channel files and marker on the backup.
type fakeInspectionNative struct {
	state         dnsEngineStateReceipt
	stateFound    bool
	enrollments   map[string]dnsPeerInspectionRecord
	unreadable    string
	prepared      map[string]string // credential -> public key
	prepareCalls  int
	activateCalls int
	probe         dnspeertransport.ProbeResult

	inspector    bool
	ports        []int
	other        string
	status       dnspeerenrollowner.Status
	marker       *dnsPeerInspectionAutoMarker
	installs     []bool // resume flag of each install call
	installErr   error
	stagedKey    string
	hostKeyValue string
}

func inspectionKey(t *testing.T) (string, string) {
	t.Helper()
	public, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ssh.NewPublicKey(public)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(key.Marshal())
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key))), hex.EncodeToString(sum[:])
}

func (f *fakeInspectionNative) host(t *testing.T) dnsPeerInspectionHost {
	return dnsPeerInspectionHost{
		engineState: func() (dnsEngineStateReceipt, bool, error) { return f.state, f.stateFound, nil },
		readEnrollment: func(engine string) (dnsPeerInspectionRecord, bool, error) {
			if f.unreadable == engine {
				return dnsPeerInspectionRecord{}, false, errors.New("unsafe")
			}
			record, ok := f.enrollments[engine]
			return record, ok, nil
		},
		prepare: func(engine string) (dnsPeerInspectionPrepared, error) {
			f.prepareCalls++
			public, _ := inspectionKey(t)
			id := strings.Repeat(string(rune('a'+f.prepareCalls)), 32)
			f.prepared[id] = public
			return dnsPeerInspectionPrepared{id, public}, nil
		},
		preparedPublic: func(engine, id string) (dnsPeerInspectionPrepared, error) {
			if public, ok := f.prepared[id]; ok {
				return dnsPeerInspectionPrepared{id, public}, nil
			}
			return dnsPeerInspectionPrepared{}, errors.New("absent")
		},
		activate: func(engine string, in pdnspeerenrollment.OwnerActivation) error {
			f.activateCalls++
			if _, ok := f.enrollments[engine]; ok {
				return errors.New("exists")
			}
			f.enrollments[engine] = dnsPeerInspectionRecord{Engine: engine, PrimaryIP: in.PrimaryIP, PeerIP: in.PeerIP,
				CatalogName: in.CatalogName, CredentialID: in.CredentialID, HostKeySHA256: in.HostKeySHA256}
			return nil
		},
		probe: func(context.Context, dnspeertransport.Enrollment) dnspeertransport.ProbeResult { return f.probe },

		inspectorPresent: func(string) bool { return f.inspector },
		sshPorts:         func() ([]int, error) { return f.ports, nil },
		otherEngine:      func(string) (string, error) { return f.other, nil },
		status:           func(string) (dnspeerenrollowner.Status, error) { return f.status, nil },
		install: func(engine string, o dnspeerenrollowner.SecondaryOptions, resume bool) error {
			f.installs = append(f.installs, resume)
			if o.PrimaryIP != inspectionMainIP || o.PeerIP != inspectionBackupIP || o.PrimaryPublicKeyPath == "" ||
				o.InspectorBinaryPath != dnsOwnerToolsDir+"/"+engine+"-peer-inspect" ||
				(engine == "pdns") != (o.CatalogAccount == pdnsPeerCatalogAccount) {
				t.Fatalf("install options: %+v", o)
			}
			if f.installErr != nil {
				// The library staged its policy before refusing.
				f.status = dnspeerenrollowner.Status{State: "incomplete", PrimaryIP: o.PrimaryIP, PeerIP: o.PeerIP, CatalogName: o.CatalogName, CatalogAccount: o.CatalogAccount}
				return f.installErr
			}
			_, digest, _ := canonicalDNSPeerInspectionKey(f.stagedKey)
			f.status = dnspeerenrollowner.Status{State: "configured", PrimaryIP: o.PrimaryIP, PeerIP: o.PeerIP, CatalogName: o.CatalogName, CatalogAccount: o.CatalogAccount, PrimaryKeySHA256: digest}
			return nil
		},
		hostKey: func() (string, error) { return f.hostKeyValue, nil },
		readMarker: func() (dnsPeerInspectionAutoMarker, bool, error) {
			if f.marker == nil {
				return dnsPeerInspectionAutoMarker{}, false, nil
			}
			return *f.marker, true, nil
		},
		writeMarker: func(m dnsPeerInspectionAutoMarker) error {
			m.Schema = dnsPeerInspectionAutoSchema
			f.marker = &m
			return nil
		},
		writePublicKey: func(raw []byte) (string, error) {
			f.stagedKey = strings.TrimSpace(string(raw))
			return dnsPeerInspectionPrimaryKeyPath, nil
		},
	}
}

func newInspectionMain() *fakeInspectionNative {
	return &fakeInspectionNative{
		state: dnsEngineStateReceipt{Engine: transport.DNSEnginePowerDNS, PairRole: transport.DNSPairRolePrimary,
			PairLocalIP: inspectionMainIP, PairPeerIP: inspectionBackupIP},
		stateFound:  true,
		enrollments: map[string]dnsPeerInspectionRecord{},
		prepared:    map[string]string{},
		probe:       dnspeertransport.ProbeReachable,
	}
}

func newInspectionBackup(engine transport.DNSEngine) *fakeInspectionNative {
	return &fakeInspectionNative{
		state: dnsEngineStateReceipt{Engine: engine, PairRole: transport.DNSPairRoleSecondary,
			PairLocalIP: inspectionBackupIP, PairPeerIP: inspectionMainIP},
		stateFound:   true,
		inspector:    true,
		ports:        []int{22},
		status:       dnspeerenrollowner.Status{State: "disabled"},
		hostKeyValue: strings.Repeat("c", 64),
	}
}

func primaryStep(t *testing.T, f *fakeInspectionNative, req transport.DNSPeerInspectionPrimaryRequest) transport.DNSPeerInspectionPrimaryResponse {
	t.Helper()
	answer, err := f.host(t).primary(context.Background(), req)
	if err != nil {
		t.Fatalf("primary %+v: %v", req, err)
	}
	return answer
}

func secondaryStep(t *testing.T, f *fakeInspectionNative, req transport.DNSPeerInspectionSecondaryRequest) transport.DNSPeerInspectionSecondaryResponse {
	t.Helper()
	answer, err := f.host(t).secondary(req)
	if err != nil {
		t.Fatalf("secondary %+v: %v", req, err)
	}
	return answer
}

// The four owner steps in order, as the setup link drives them: the main
// prepares, the backup installs for that key and returns its host key, the
// main activates pinned to it. Every value comes from each Agent's own pair.
func TestDNSPeerInspectionAutomationRunsTheOwnerStepsForThisPair(t *testing.T) {
	for _, engine := range []transport.DNSEngine{transport.DNSEnginePowerDNS, transport.DNSEngineBIND} {
		main, backup := newInspectionMain(), newInspectionBackup(engine)
		if got := secondaryStep(t, backup, transport.DNSPeerInspectionSecondaryRequest{Action: transport.DNSPeerInspectionActionCheck}); got.State != transport.DNSPeerInspectionReady || got.Engine != string(engine) {
			t.Fatalf("%s check: %+v", engine, got)
		}
		prepared := primaryStep(t, main, transport.DNSPeerInspectionPrimaryRequest{Action: transport.DNSPeerInspectionActionPrepare, Engine: string(engine)})
		if prepared.State != transport.DNSPeerInspectionPrepared || prepared.PublicKey == "" {
			t.Fatalf("%s prepare: %+v", engine, prepared)
		}
		installed := secondaryStep(t, backup, transport.DNSPeerInspectionSecondaryRequest{Action: transport.DNSPeerInspectionActionInstall, PublicKey: prepared.PublicKey})
		if installed.State != transport.DNSPeerInspectionInstalled || installed.HostKeySHA256 != backup.hostKeyValue || len(backup.installs) != 1 || backup.installs[0] {
			t.Fatalf("%s install: %+v %v", engine, installed, backup.installs)
		}
		if backup.stagedKey != prepared.PublicKey || backup.marker == nil || backup.marker.State != dnsPeerInspectionAutoInstalled {
			t.Fatalf("%s staged key or marker: %q %+v", engine, backup.stagedKey, backup.marker)
		}
		active := primaryStep(t, main, transport.DNSPeerInspectionPrimaryRequest{Action: transport.DNSPeerInspectionActionActivate, Engine: string(engine),
			CredentialID: prepared.CredentialID, HostKeySHA256: installed.HostKeySHA256})
		if active.State != transport.DNSPeerInspectionActive || active.Probe != string(dnspeertransport.ProbeReachable) {
			t.Fatalf("%s activate: %+v", engine, active)
		}
		record := main.enrollments[string(engine)]
		if record.PrimaryIP != inspectionMainIP || record.PeerIP != inspectionBackupIP || record.HostKeySHA256 != backup.hostKeyValue || !strings.HasPrefix(record.CatalogName, "catalog-") {
			t.Fatalf("%s enrollment: %+v", engine, record)
		}
	}
}

// Interrupted anywhere, the same steps again finish the same enrollment: the
// main reuses its prepared key, the backup resumes its own staged files, and
// an activation whose answer was lost is answered as active, never twice.
func TestDNSPeerInspectionAutomationRetriesAfterAnInterruption(t *testing.T) {
	main, backup := newInspectionMain(), newInspectionBackup(transport.DNSEnginePowerDNS)
	prepared := primaryStep(t, main, transport.DNSPeerInspectionPrimaryRequest{Action: transport.DNSPeerInspectionActionPrepare, Engine: "pdns"})
	again := primaryStep(t, main, transport.DNSPeerInspectionPrimaryRequest{Action: transport.DNSPeerInspectionActionPrepare, Engine: "pdns", CredentialID: prepared.CredentialID})
	if again.CredentialID != prepared.CredentialID || again.PublicKey != prepared.PublicKey || main.prepareCalls != 1 {
		t.Fatalf("prepare retry: %+v (%d prepares)", again, main.prepareCalls)
	}
	backup.installErr = errors.New("SSH reload failed; key remains disabled")
	failed := secondaryStep(t, backup, transport.DNSPeerInspectionSecondaryRequest{Action: transport.DNSPeerInspectionActionInstall, PublicKey: prepared.PublicKey})
	if failed.State != transport.DNSPeerInspectionFailed || failed.Reason != transport.DNSPeerInspectionReasonSSHInactive || backup.marker.State != dnsPeerInspectionAutoInstalling {
		t.Fatalf("interrupted install: %+v %+v", failed, backup.marker)
	}
	backup.installErr = nil
	if got := secondaryStep(t, backup, transport.DNSPeerInspectionSecondaryRequest{Action: transport.DNSPeerInspectionActionCheck}); got.State != transport.DNSPeerInspectionReady {
		t.Fatalf("check after the interruption: %+v", got)
	}
	installed := secondaryStep(t, backup, transport.DNSPeerInspectionSecondaryRequest{Action: transport.DNSPeerInspectionActionInstall, PublicKey: prepared.PublicKey})
	if installed.State != transport.DNSPeerInspectionInstalled || len(backup.installs) != 2 || !backup.installs[1] {
		t.Fatalf("resume: %+v %v", installed, backup.installs)
	}
	// The install answer was lost: the same install is answered from the
	// configured channel, with no native call.
	if got := secondaryStep(t, backup, transport.DNSPeerInspectionSecondaryRequest{Action: transport.DNSPeerInspectionActionInstall, PublicKey: prepared.PublicKey}); got.State != transport.DNSPeerInspectionInstalled || len(backup.installs) != 2 {
		t.Fatalf("install replay: %+v %v", got, backup.installs)
	}
	activate := transport.DNSPeerInspectionPrimaryRequest{Action: transport.DNSPeerInspectionActionActivate, Engine: "pdns", CredentialID: prepared.CredentialID, HostKeySHA256: installed.HostKeySHA256}
	primaryStep(t, main, activate)
	if got := primaryStep(t, main, activate); got.State != transport.DNSPeerInspectionActive || main.activateCalls != 1 {
		t.Fatalf("activate replay: %+v (%d activations)", got, main.activateCalls)
	}
	// The prepare of a retried run after activation is answered as active too.
	if got := primaryStep(t, main, transport.DNSPeerInspectionPrimaryRequest{Action: transport.DNSPeerInspectionActionPrepare, Engine: "pdns", CredentialID: prepared.CredentialID}); got.State != transport.DNSPeerInspectionActive || main.prepareCalls != 1 {
		t.Fatalf("prepare after activation: %+v", got)
	}
}

// An owner's own enrollment, channel or SSH configuration is reported and
// never replaced; a channel this automation set up and the owner revoked is
// never authorized again.
func TestDNSPeerInspectionAutomationNeverOverwritesTheOwner(t *testing.T) {
	ownerKey, ownerDigest := inspectionKey(t)
	_ = ownerKey
	ourKey, _ := inspectionKey(t)
	catalog := "catalog-c000020a.celikpanel.invalid"
	ownerStatus := dnspeerenrollowner.Status{State: "configured", PrimaryIP: inspectionMainIP, PeerIP: inspectionBackupIP, CatalogName: catalog, CatalogAccount: pdnsPeerCatalogAccount, PrimaryKeySHA256: ownerDigest}

	backup := newInspectionBackup(transport.DNSEnginePowerDNS)
	backup.status = ownerStatus
	if got := secondaryStep(t, backup, transport.DNSPeerInspectionSecondaryRequest{Action: transport.DNSPeerInspectionActionCheck}); got.State != transport.DNSPeerInspectionExisting || got.HostKeySHA256 == "" {
		t.Fatalf("owner channel check: %+v", got)
	}
	if got := secondaryStep(t, backup, transport.DNSPeerInspectionSecondaryRequest{Action: transport.DNSPeerInspectionActionInstall, PublicKey: ourKey}); got.State != transport.DNSPeerInspectionOwnerConflict || got.Reason != transport.DNSPeerInspectionReasonOtherKey {
		t.Fatalf("owner channel with another key: %+v", got)
	}
	// Owner-staged files without this automation's marker.
	staged := newInspectionBackup(transport.DNSEnginePowerDNS)
	staged.status = dnspeerenrollowner.Status{State: "incomplete", PrimaryIP: inspectionMainIP, PeerIP: inspectionBackupIP, CatalogName: catalog, CatalogAccount: pdnsPeerCatalogAccount}
	if got := secondaryStep(t, staged, transport.DNSPeerInspectionSecondaryRequest{Action: transport.DNSPeerInspectionActionInstall, PublicKey: ourKey}); got.State != transport.DNSPeerInspectionOwnerConflict || got.Reason != transport.DNSPeerInspectionReasonStaged {
		t.Fatalf("owner-staged files: %+v", got)
	}
	// An owner-edited sshd_config.
	edited := newInspectionBackup(transport.DNSEnginePowerDNS)
	edited.status = dnspeerenrollowner.Status{State: "unknown", Reason: "owner sshd_config changed since enrollment", PrimaryIP: inspectionMainIP}
	if got := secondaryStep(t, edited, transport.DNSPeerInspectionSecondaryRequest{Action: transport.DNSPeerInspectionActionInstall, PublicKey: ourKey}); got.State != transport.DNSPeerInspectionOwnerConflict || got.Reason != transport.DNSPeerInspectionReasonOwnerSSHConfig {
		t.Fatalf("owner-edited sshd_config: %+v", got)
	}
	// The other engine's channel or evidence.
	other := newInspectionBackup(transport.DNSEnginePowerDNS)
	other.other = "/etc/bind-peer-inspector/policy.json"
	if got := secondaryStep(t, other, transport.DNSPeerInspectionSecondaryRequest{Action: transport.DNSPeerInspectionActionCheck}); got.State != transport.DNSPeerInspectionOwnerConflict || got.Reason != transport.DNSPeerInspectionReasonOtherEngine {
		t.Fatalf("other engine: %+v", got)
	}
	// Revoked by the owner after this automation installed it.
	revoked := newInspectionBackup(transport.DNSEnginePowerDNS)
	revoked.marker = &dnsPeerInspectionAutoMarker{Schema: dnsPeerInspectionAutoSchema, Engine: "pdns", PrimaryIP: inspectionMainIP, PeerIP: inspectionBackupIP, CatalogName: catalog, PrimaryKeySHA256: ownerDigest, State: dnsPeerInspectionAutoInstalled}
	revoked.status = dnspeerenrollowner.Status{State: "disabled", PrimaryIP: inspectionMainIP, PeerIP: inspectionBackupIP, CatalogName: catalog, CatalogAccount: pdnsPeerCatalogAccount}
	if got := secondaryStep(t, revoked, transport.DNSPeerInspectionSecondaryRequest{Action: transport.DNSPeerInspectionActionInstall, PublicKey: ourKey}); got.State != transport.DNSPeerInspectionRevoked {
		t.Fatalf("owner revocation: %+v", got)
	}
	for _, f := range []*fakeInspectionNative{backup, staged, edited, other, revoked} {
		if len(f.installs) != 0 || f.stagedKey != "" {
			t.Fatalf("an owner state was changed: %v %q", f.installs, f.stagedKey)
		}
	}

	// The main: an owner enrollment for this pair is kept and named; one
	// for another pair or engine, or one that cannot be read, is a conflict.
	main := newInspectionMain()
	main.enrollments["pdns"] = dnsPeerInspectionRecord{Engine: "pdns", PrimaryIP: inspectionMainIP, PeerIP: inspectionBackupIP, CatalogName: catalog, CredentialID: strings.Repeat("e", 32), HostKeySHA256: strings.Repeat("c", 64)}
	if got := primaryStep(t, main, transport.DNSPeerInspectionPrimaryRequest{Action: transport.DNSPeerInspectionActionPrepare, Engine: "pdns"}); got.State != transport.DNSPeerInspectionExisting || main.prepareCalls != 0 {
		t.Fatalf("owner enrollment: %+v", got)
	}
	if got := primaryStep(t, main, transport.DNSPeerInspectionPrimaryRequest{Action: transport.DNSPeerInspectionActionActivate, Engine: "pdns", CredentialID: strings.Repeat("f", 32), HostKeySHA256: strings.Repeat("d", 64)}); got.State != transport.DNSPeerInspectionOwnerConflict || got.Reason != transport.DNSPeerInspectionReasonHostKey || main.activateCalls != 0 {
		t.Fatalf("owner enrollment for another host key: %+v", got)
	}
	if got := primaryStep(t, main, transport.DNSPeerInspectionPrimaryRequest{Action: transport.DNSPeerInspectionActionPrepare, Engine: "bind"}); got.State != transport.DNSPeerInspectionOwnerConflict || got.Reason != transport.DNSPeerInspectionReasonOtherEngine {
		t.Fatalf("owner enrollment of the other engine: %+v", got)
	}
	main.enrollments["pdns"] = dnsPeerInspectionRecord{Engine: "pdns", PrimaryIP: inspectionMainIP, PeerIP: "192.0.2.99", CatalogName: catalog, CredentialID: strings.Repeat("e", 32)}
	if got := primaryStep(t, main, transport.DNSPeerInspectionPrimaryRequest{Action: transport.DNSPeerInspectionActionPrepare, Engine: "pdns"}); got.State != transport.DNSPeerInspectionOwnerConflict || got.Reason != transport.DNSPeerInspectionReasonOtherPair {
		t.Fatalf("owner enrollment for another pair: %+v", got)
	}
	main.unreadable = "bind"
	if got := primaryStep(t, main, transport.DNSPeerInspectionPrimaryRequest{Action: transport.DNSPeerInspectionActionPrepare, Engine: "pdns"}); got.State != transport.DNSPeerInspectionOwnerConflict || got.Reason != transport.DNSPeerInspectionReasonUnreadable {
		t.Fatalf("unreadable enrollment: %+v", got)
	}
	if main.prepareCalls != 0 || main.activateCalls != 0 {
		t.Fatal("an owner enrollment was changed")
	}
}

// Prerequisites the owner resolves are named, and nothing is changed.
func TestDNSPeerInspectionAutomationNamesUnmetPrerequisites(t *testing.T) {
	key, _ := inspectionKey(t)
	port := newInspectionBackup(transport.DNSEnginePowerDNS)
	port.ports = []int{2222}
	if got := secondaryStep(t, port, transport.DNSPeerInspectionSecondaryRequest{Action: transport.DNSPeerInspectionActionInstall, PublicKey: key}); got.State != transport.DNSPeerInspectionSSHPort || len(got.SSHPorts) != 1 || got.SSHPorts[0] != 2222 {
		t.Fatalf("SSH port: %+v", got)
	}
	missing := newInspectionBackup(transport.DNSEngineBIND)
	missing.inspector = false
	if got := secondaryStep(t, missing, transport.DNSPeerInspectionSecondaryRequest{Action: transport.DNSPeerInspectionActionCheck}); got.State != transport.DNSPeerInspectionInspectorUnavailable {
		t.Fatalf("inspector: %+v", got)
	}
	unpaired := newInspectionBackup(transport.DNSEngineBIND)
	unpaired.stateFound = false
	if got := secondaryStep(t, unpaired, transport.DNSPeerInspectionSecondaryRequest{Action: transport.DNSPeerInspectionActionCheck}); got.State != transport.DNSPeerInspectionPairNotReady {
		t.Fatalf("no pair yet: %+v", got)
	}
	wrongSide := newInspectionMain()
	if got := secondaryStep(t, wrongSide, transport.DNSPeerInspectionSecondaryRequest{Action: transport.DNSPeerInspectionActionCheck}); got.State != transport.DNSPeerInspectionNotApplicable {
		t.Fatalf("a main asked to install: %+v", got)
	}
	for _, f := range []*fakeInspectionNative{port, missing, unpaired, wrongSide} {
		if len(f.installs) != 0 || f.marker != nil {
			t.Fatal("a prerequisite failure changed the host")
		}
	}
	// A request that is not one public Ed25519 key, or a bad digest, is refused.
	backup := newInspectionBackup(transport.DNSEnginePowerDNS)
	for _, bad := range []string{"", "ssh-rsa AAAA", key + "\n" + key, "command=\"x\" " + key} {
		if _, err := backup.host(t).secondary(transport.DNSPeerInspectionSecondaryRequest{Action: transport.DNSPeerInspectionActionInstall, PublicKey: bad}); err == nil {
			t.Fatalf("accepted key %q", bad)
		}
	}
	main := newInspectionMain()
	if _, err := main.host(t).primary(context.Background(), transport.DNSPeerInspectionPrimaryRequest{Action: transport.DNSPeerInspectionActionActivate, Engine: "pdns", CredentialID: strings.Repeat("a", 32), HostKeySHA256: "xyz"}); err == nil {
		t.Fatal("accepted a malformed host key digest")
	}
	// An unreachable channel is reported with the activation, which stays.
	prepared := primaryStep(t, main, transport.DNSPeerInspectionPrimaryRequest{Action: transport.DNSPeerInspectionActionPrepare, Engine: "pdns"})
	main.probe = dnspeertransport.ProbeUnreachable
	if got := primaryStep(t, main, transport.DNSPeerInspectionPrimaryRequest{Action: transport.DNSPeerInspectionActionActivate, Engine: "pdns", CredentialID: prepared.CredentialID, HostKeySHA256: strings.Repeat("c", 64)}); got.State != transport.DNSPeerInspectionActive || got.Probe != string(dnspeertransport.ProbeUnreachable) {
		t.Fatalf("unreachable: %+v", got)
	}
}
