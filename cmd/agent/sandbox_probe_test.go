package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/alicelik/celikpanel/internal/sitesandbox"
	"github.com/alicelik/celikpanel/internal/transport"
)

// fakeSandboxRunner answers the probe's and the proof's commands.
type fakeSandboxRunner struct {
	mu    sync.Mutex
	nftOK bool
	calls []string
	set   string
	chain string
}

func (f *fakeSandboxRunner) Run(_ context.Context, stdin, name string, args ...string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, name+" "+strings.Join(args, " "))
	switch {
	case name == "systemctl":
		return []byte("systemd 257 (257.8-1~deb13u2)\n+PAM +BPF_FRAMEWORK\n"), nil
	case name == sitesandbox.NFT && len(args) > 0 && args[0] == "--check":
		if f.nftOK {
			return nil, nil
		}
		return nil, errors.New("exit 1")
	case name == sitesandbox.NFT && len(args) > 1 && args[0] == "list" && args[1] == "set":
		return []byte(f.set), nil
	case name == sitesandbox.NFT && len(args) > 1 && args[0] == "list" && args[1] == "chain":
		return []byte(f.chain), nil
	case name == sitesandbox.NFT && len(args) > 1 && args[0] == "list" && args[1] == "counter":
		return []byte("counter smtp_rejects {\n\tpackets 3 bytes 180\n}\n"), nil
	}
	return nil, errors.New("unexpected command " + name)
}

func (f *fakeSandboxRunner) Exists(path string) bool {
	return path == sitesandbox.CgroupControllers || path == sitesandbox.NFT
}

// useSandboxHost points the sandbox's native files at temporary folders and
// fakes the probe (systemd 257 with BPF, cgroup v2, nft as given).
func useSandboxHost(t *testing.T, nftOK bool) *fakeSandboxRunner {
	t.Helper()
	runner := &fakeSandboxRunner{nftOK: nftOK}
	oldRunner, oldDir, oldUnitDir, oldLedger := sandboxRunner, sandboxEgressDir, sandboxUnitDir, sandboxLedgerPath
	t.Cleanup(func() {
		sandboxRunner, sandboxEgressDir, sandboxUnitDir, sandboxLedgerPath = oldRunner, oldDir, oldUnitDir, oldLedger
		resetSandboxCapsCache()
	})
	units := t.TempDir()
	sandboxRunner = runner
	sandboxEgressDir = filepath.Join(t.TempDir(), "celikpanel-sandbox")
	sandboxUnitDir = func() (string, error) { return units, nil }
	sandboxLedgerPath = filepath.Join(t.TempDir(), "native-files.json")
	resetSandboxCapsCache()
	return runner
}

func TestSandboxEgressFilesFollowTheOwnerRule(t *testing.T) {
	useSandboxHost(t, true)
	var ctl []string
	old := appUnitSystemctl
	t.Cleanup(func() { appUnitSystemctl = old })
	appUnitSystemctl = func(args ...string) ([]byte, error) {
		ctl = append(ctl, strings.Join(args, " "))
		return nil, nil
	}
	caps := sandboxCaps(context.Background())
	if !caps.EgressEnforced() {
		t.Fatalf("caps = %+v", caps)
	}
	if err := ensureSandboxEgress(caps); err != nil {
		t.Fatal(err)
	}
	egress := filepath.Join(sandboxEgressDir, "egress.nft")
	raw, err := os.ReadFile(egress)
	if err != nil || !strings.HasPrefix(string(raw), "# celikpanel-render v2 sha256=") || !strings.Contains(string(raw), "table inet celikpanel_egress") {
		t.Fatalf("egress file: %v\n%s", err, raw)
	}
	units, _ := sandboxUnitDir()
	for _, name := range []string{sitesandbox.EgressGuardUnit, sitesandbox.EgressGuardTimer} {
		if _, err := os.Stat(filepath.Join(units, name)); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if len(readSandboxLedger()) != 4 {
		t.Fatalf("ledger = %v", readSandboxLedger())
	}
	if !strings.Contains(strings.Join(ctl, "\n"), "enable --now celikpanel-egress-guard.timer") {
		t.Fatalf("the guard timer was not started: %v", ctl)
	}

	// The owner's change is kept, not overwritten; CelikPanel's text waits
	// beside it.
	edited := strings.Replace(string(raw), "reject with tcp reset", "accept", 1)
	if err := os.WriteFile(egress, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ensureSandboxEgress(caps); err != nil {
		t.Fatal(err)
	}
	now, _ := os.ReadFile(egress)
	if string(now) != edited {
		t.Fatal("the owner's egress file was overwritten")
	}
	if _, err := os.Stat(egress + ".celikpanel-pending"); err != nil {
		t.Fatalf("CelikPanel's text is not held beside the kept file: %v", err)
	}
	var resp transport.SandboxCapsResponse
	if err := (&Agent{}).SandboxCapsV1(&transport.Empty{}, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.EgressFile != transport.SiteFileOwnerEdited || resp.Level != "full" || resp.SMTPRejects != 3 || resp.Generation != sitesandbox.Generation {
		t.Fatalf("caps response = %+v", resp)
	}
}

func TestSandboxCapsIsAReadAndNamesTheGap(t *testing.T) {
	runner := useSandboxHost(t, false)
	old, oldDir := appUnitSystemctl, appUnitDirOverride
	t.Cleanup(func() { appUnitSystemctl, appUnitDirOverride = old, oldDir })
	var ctl []string
	appUnitSystemctl = func(args ...string) ([]byte, error) {
		ctl = append(ctl, strings.Join(args, " "))
		return []byte("inactive\n"), nil
	}
	units, _ := sandboxUnitDir()
	appUnitDirOverride = units
	if err := os.WriteFile(filepath.Join(units, "celiksvc-4.service"), []byte("# celikpanel-render-sha256: 00\n[Unit]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var resp transport.SandboxCapsResponse
	if err := (&Agent{}).SandboxCapsV1(&transport.Empty{}, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Level != "reduced" || len(resp.Gaps) != 1 || resp.Gaps[0] != "nft_cgroup_missing" {
		t.Fatalf("response = %+v", resp)
	}
	if len(resp.Units) != 1 || resp.Units[0].InstallID != 4 || resp.Units[0].Generation != 1 || resp.Units[0].File != "edited" {
		t.Fatalf("units = %+v", resp.Units)
	}
	for _, call := range runner.calls {
		if strings.HasPrefix(call, sitesandbox.NFT) && !strings.HasPrefix(call, sitesandbox.NFT+" --check") && !strings.HasPrefix(call, sitesandbox.NFT+" list") {
			t.Fatalf("the read changed nftables: %s", call)
		}
	}
	for _, call := range ctl {
		if !strings.HasPrefix(call, "show") {
			t.Fatalf("the read ran systemctl %s", call)
		}
	}
	if _, err := os.Stat(sandboxEgressDir); !os.IsNotExist(err) {
		t.Fatal("the read wrote the egress folder")
	}
	// A host that cannot hold the block gets no egress files.
	if err := ensureSandboxEgress(sandboxCaps(context.Background())); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(sandboxEgressDir); !os.IsNotExist(err) {
		t.Fatal("egress files written where nft cannot load them")
	}
}

func TestSandboxProofReadsSystemdAndTheSet(t *testing.T) {
	runner := useSandboxHost(t, true)
	caps := sandboxCaps(context.Background())
	profile := sitesandbox.Profile{Kind: sitesandbox.KindServiceApp, User: "flows_example_com", Group: "flows_example_com",
		AppDir: "/var/www/celikpanel/subscriptions/3/sites/7/apps/n8n", Port: 21010,
		Limits: sitesandbox.Limits{MemoryMB: 1536, CPUPercent: 100, Tasks: 512}, Level: sitesandbox.LevelFull}
	unit := "celiksvc-5.service"
	var show strings.Builder
	for name, e := range sitesandbox.Expected(profile, caps, unit) {
		value := e.Value
		if e.Contains {
			value = "{ path=/usr/sbin/nft ; argv[]=" + e.Value + " ; }"
		}
		show.WriteString(name + "=" + value + "\n")
	}
	old := appUnitSystemctl
	t.Cleanup(func() { appUnitSystemctl = old })
	appUnitSystemctl = func(args ...string) ([]byte, error) { return []byte(show.String()), nil }
	runner.set = "set smtp_blocked {\n\telements = { \"system.slice/celiksvc-5.service\" }\n}\n"
	runner.chain = "chain output {\n\ttcp dport { 25, 465, 587 } socket cgroupv2 level 2 @smtp_blocked counter name \"smtp_rejects\" reject with tcp reset\n}\n"
	listen := sitesandbox.ListenProof{Serving: true}
	if r := sandboxProof(context.Background(), unit, profile, caps, &listen); !r.OK {
		t.Fatalf("proof = %+v", r)
	}
	runner.set = "set smtp_blocked {\n}\n"
	if r := sandboxProof(context.Background(), unit, profile, caps, &listen); r.OK || r.Code != sitesandbox.ProofNotInSet {
		t.Fatalf("a unit outside the set = %+v", r)
	}
	appUnitSystemctl = func(args ...string) ([]byte, error) { return []byte("Failed to connect to bus"), errors.New("exit 1") }
	if r := sandboxProof(context.Background(), unit, profile, caps, &listen); r.OK || r.Code != sitesandbox.ProofUnreadable {
		t.Fatalf("an unreadable systemd = %+v", r)
	}
}
