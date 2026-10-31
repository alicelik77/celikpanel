//go:build linux

package dnsenginerecovery

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alicelik/celikpanel/internal/dnsengineartifact"
)

// Arch PowerDNS follow-up drill (2026-10-30): a power cut right after the
// fresh primary's target-enable intent; after the reboot btrfs reported the
// candidate's filesystem as device 34 instead of the sealed 35, with the same
// inode and bytes, and the pre-start undo refused its own candidate. A sealed
// proof whose only difference is the device number is the same candidate;
// any other difference is still refused. Component test, not native evidence.
func rebootRenumbered(proof dnsengineartifact.PDNSTargetCandidateProofV4) dnsengineartifact.PDNSTargetCandidateProofV4 {
	proof.Device++
	return proof
}

func TestPDNSTargetCandidateSurvivesDeviceRenumbering(t *testing.T) {
	requireRootPDNSTargetFixture(t)
	root := t.TempDir()
	private := filepath.Join(root, "agent-private")
	liveDir := filepath.Join(root, "powerdns")
	if err := os.Mkdir(private, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(liveDir, 0o755); err != nil {
		t.Fatal(err)
	}
	candidate := filepath.Join(private, ".celikpanel-switch-test.sqlite3")
	live := filepath.Join(liveDir, "pdns.sqlite3")
	writePDNSTargetFixture(t, candidate)
	captured, err := CapturePDNSTargetCandidateV4(candidate)
	if err != nil {
		t.Fatal(err)
	}
	sealed := rebootRenumbered(captured)
	if !sealed.SameCandidate(captured) {
		t.Fatal("a renumbered device alone made the candidate foreign")
	}
	for name, edit := range map[string]func(*dnsengineartifact.PDNSTargetCandidateProofV4){
		"inode": func(p *dnsengineartifact.PDNSTargetCandidateProofV4) { p.Inode++ },
		"bytes": func(p *dnsengineartifact.PDNSTargetCandidateProofV4) {
			first := "0"
			if p.SHA256[0] == '0' {
				first = "1"
			}
			p.SHA256 = first + p.SHA256[1:]
		},
		"size":  func(p *dnsengineartifact.PDNSTargetCandidateProofV4) { p.Size++ },
		"owner": func(p *dnsengineartifact.PDNSTargetCandidateProofV4) { p.UID++ },
		"mode":  func(p *dnsengineartifact.PDNSTargetCandidateProofV4) { p.Mode = 0o600 },
		"path":  func(p *dnsengineartifact.PDNSTargetCandidateProofV4) { p.Path += "x" },
	} {
		other := sealed
		edit(&other)
		if other.SameCandidate(captured) {
			t.Fatalf("%s difference accepted", name)
		}
	}
	// The rename inverse and the live proof hold with the renumbered seal.
	if err := os.Rename(candidate, live); err != nil {
		t.Fatal(err)
	}
	if err := VerifyPDNSTargetLiveV4(sealed, live); err != nil {
		t.Fatalf("live proof refused a renumbered seal: %v", err)
	}
	if differs, err := ClassifyPDNSTargetLiveV4(sealed, live); err != nil || differs {
		t.Fatalf("status classified a renumbered seal as changed: %v %v", differs, err)
	}
	if err := restoreRenamedPDNSTargetFileV4(sealed, live, func() error { return nil }, nil); err != nil {
		t.Fatalf("rename inverse refused a renumbered seal: %v", err)
	}
	if err := VerifyPDNSTargetCandidateV4Exact(sealed); err != nil {
		t.Fatalf("restored candidate refused: %v", err)
	}
	if err := removeExactStagedPDNSTargetFileV4(sealed, live, func() error { return nil }, nil); err != nil {
		t.Fatalf("staged removal refused a renumbered seal: %v", err)
	}
	if _, err := os.Lstat(candidate); !os.IsNotExist(err) {
		t.Fatalf("candidate not removed: %v", err)
	}
}

// A changed byte is still refused at removal even with the device renumbered.
func TestPDNSTargetCandidateRenumberedStillRefusesOwnerChange(t *testing.T) {
	requireRootPDNSTargetFixture(t)
	root := t.TempDir()
	private := filepath.Join(root, "agent-private")
	liveDir := filepath.Join(root, "powerdns")
	if err := os.Mkdir(private, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(liveDir, 0o755); err != nil {
		t.Fatal(err)
	}
	candidate := filepath.Join(private, ".celikpanel-switch-test.sqlite3")
	writePDNSTargetFixture(t, candidate)
	captured, err := CapturePDNSTargetCandidateV4(candidate)
	if err != nil {
		t.Fatal(err)
	}
	sealed := rebootRenumbered(captured)
	file, err := os.OpenFile(candidate, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteAt([]byte{0x43}, 100); err != nil {
		t.Fatal(err)
	}
	file.Close()
	if err := removeExactStagedPDNSTargetFileV4(sealed, filepath.Join(liveDir, "pdns.sqlite3"), func() error { return nil }, nil); err == nil {
		t.Fatal("changed candidate removed")
	}
}
