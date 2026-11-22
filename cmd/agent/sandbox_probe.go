package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alicelik/celikpanel/internal/hostingpath"
	"github.com/alicelik/celikpanel/internal/serviceapps"
	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/sitesandbox"
	"github.com/alicelik/celikpanel/internal/transport"
)

// The site sandbox on this server (alpha.119, docs/SITE-SANDBOX.md).
//
//   - SandboxCapsV1 reads what the host enforces and the state of the SMTP
//     egress block. It changes nothing (nft is only asked with --check and
//     list).
//   - ensureSandboxEgress writes the egress table file, its guard script and
//     the guard timer as CelikPanel-owned native files under the D-031 rule:
//     an absent file is written, CelikPanel's unchanged text is replaced, and
//     a file the owner changed is kept with CelikPanel's text beside it. It
//     runs only inside a mutation (activate, prepare), never from a read.
//   - sandboxProof compares a running unit with its profile.
//
// Sunucudaki site koruması: yetenek okuması hiçbir şeyi değiştirmez; SMTP
// engelinin dosyaları D-031 kuralıyla yazılır, sahibin değiştirdiği dosya korunur.

// sandboxRunner runs the probe's and the proof's read-only commands.
var sandboxRunner sitesandbox.Runner = sitesandbox.HostRunner{}

var (
	sandboxCapsMu     sync.Mutex
	sandboxCapsCached sitesandbox.Caps
	sandboxCapsAt     time.Time
)

// sandboxCaps probes the host, at most once a minute (status polls read it).
func sandboxCaps(ctx context.Context) sitesandbox.Caps {
	sandboxCapsMu.Lock()
	defer sandboxCapsMu.Unlock()
	if !sandboxCapsAt.IsZero() && time.Since(sandboxCapsAt) < time.Minute {
		return sandboxCapsCached
	}
	sandboxCapsCached = sitesandbox.Probe(ctx, sandboxRunner)
	sandboxCapsAt = time.Now()
	return sandboxCapsCached
}

func resetSandboxCapsCache() {
	sandboxCapsMu.Lock()
	sandboxCapsAt = time.Time{}
	sandboxCapsMu.Unlock()
}

func transportSandboxCaps(c sitesandbox.Caps) transport.SandboxCaps {
	return transport.SandboxCaps{SystemdVersion: c.SystemdVersion, BPFFramework: c.BPFFramework, CgroupV2: c.CgroupV2, NFTSocketCgroup: c.NFTSocketCgroup}
}

// sandboxNativeFile is one CelikPanel-owned native file of the sandbox.
type sandboxNativeFile struct {
	Kind string
	Path string
	Body string
}

// sandboxEgressDir and sandboxUnitDir are swapped by tests.
var (
	sandboxEgressDir = sitesandbox.EgressDir
	sandboxUnitDir   = unitDir
)

// sandboxNativeFiles are the sandbox's native files; submission writes the
// SMTP block with the exception for this server's own submission services
// (S4 R2b fix round: only while they are CelikPanel's own, site_tasks_mail.go).
func sandboxNativeFiles(submission bool) ([]sandboxNativeFile, error) {
	dir, err := sandboxUnitDir()
	if err != nil {
		return nil, err
	}
	return []sandboxNativeFile{
		{Kind: "sandbox_egress", Path: filepath.Join(sandboxEgressDir, filepath.Base(sitesandbox.EgressFile)), Body: sitesandbox.EgressRulesetWith(submission)},
		{Kind: "sandbox_egress_guard", Path: filepath.Join(sandboxEgressDir, filepath.Base(sitesandbox.EgressGuardScript)), Body: sitesandbox.EgressGuardScriptText()},
		{Kind: "sandbox_egress_unit", Path: filepath.Join(dir, sitesandbox.EgressGuardUnit), Body: sitesandbox.EgressGuardServiceText()},
		{Kind: "sandbox_egress_timer", Path: filepath.Join(dir, sitesandbox.EgressGuardTimer), Body: sitesandbox.EgressGuardTimerText()},
	}, nil
}

func (f sandboxNativeFile) managed(recorded string) services.ManagedSiteFile {
	return services.ManagedSiteFile{
		Kind: f.Kind, Path: f.Path, HeaderPrefix: services.ManagedHeaderPrefixHash, Body: f.Body,
		Trigger: transport.SiteFileTriggerCreate, RecordedSHA256: recorded,
	}
}

// sandboxLedgerPath is the Agent's record of the sandbox's native files
// (path -> the body digest it last wrote), beside the service-app state.
var sandboxLedgerPath = filepath.Join(hostingpath.ServiceMutationStateRoot(), "sandbox", "native-files.json")

type sandboxLedgerEntry struct {
	Kind         string `json:"kind"`
	RenderSHA256 string `json:"render_sha256"`
	WrittenAt    string `json:"written_at"`
}

// sandboxLedgerMu is the ONE lock of the ledger file (S4 R2a review 12):
// every change is a read-modify-write of the file under it, so a writer never
// drops another writer's entry.
var sandboxLedgerMu sync.Mutex

func readSandboxLedger() map[string]sandboxLedgerEntry {
	ledger := map[string]sandboxLedgerEntry{}
	if raw, err := os.ReadFile(sandboxLedgerPath); err == nil {
		_ = json.Unmarshal(raw, &ledger)
	}
	return ledger
}

// updateSandboxLedger changes the ledger file under its lock: change gets
// the file's current entries and reports whether it changed them.
func updateSandboxLedger(change func(map[string]sandboxLedgerEntry) bool) error {
	sandboxLedgerMu.Lock()
	defer sandboxLedgerMu.Unlock()
	ledger := readSandboxLedger()
	if !change(ledger) {
		return nil
	}
	return writeSandboxLedger(ledger)
}

// setSandboxLedgerEntry records one file's digest.
func setSandboxLedgerEntry(path, kind, digest string) error {
	return updateSandboxLedger(func(l map[string]sandboxLedgerEntry) bool {
		if l[path].RenderSHA256 == digest && l[path].Kind == kind {
			return false
		}
		l[path] = sandboxLedgerEntry{Kind: kind, RenderSHA256: digest, WrittenAt: time.Now().UTC().Format(time.RFC3339)}
		return true
	})
}

// writeSandboxLedger writes the whole file; only updateSandboxLedger calls
// it, under sandboxLedgerMu.
func writeSandboxLedger(ledger map[string]sandboxLedgerEntry) error {
	if err := os.MkdirAll(filepath.Dir(sandboxLedgerPath), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(ledger, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(sandboxLedgerPath+".tmp", raw, 0o600); err != nil {
		return err
	}
	return os.Rename(sandboxLedgerPath+".tmp", sandboxLedgerPath)
}

var sandboxEgressMu sync.Mutex

// ensureSandboxEgress puts the egress table, its guard script and the guard
// timer in place and starts the timer. A host that cannot hold the block
// gets nothing (its reduced level names the gap). A file the owner changed
// is kept and used as it is: if it no longer loads, every sandboxed unit
// refuses to start, which is the fail-closed answer.
func ensureSandboxEgress(caps sitesandbox.Caps) error {
	if !caps.EgressEnforced() {
		return nil
	}
	sandboxEgressMu.Lock()
	defer sandboxEgressMu.Unlock()
	submission, why := siteSubmissionStateNow()
	files, err := sandboxNativeFiles(submission == transport.MailSubmissionOurs)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(sandboxEgressDir, 0o755); err != nil {
		return fmt.Errorf("the sandbox folder %s: %v", sandboxEgressDir, err)
	}
	ledger := readSandboxLedger()
	changed, rules := false, false
	record := map[string]sandboxLedgerEntry{}
	for _, f := range files {
		result, err := services.ApplyManagedSiteFile(f.managed(ledger[f.Path].RenderSHA256), nil, nil)
		if err != nil {
			return fmt.Errorf("%s: %v", f.Path, err)
		}
		switch result.Outcome {
		case transport.SiteFileOutcomeWritten, transport.SiteFileOutcomeRecreated, transport.SiteFileOutcomeTaken:
			changed = true
			rules = rules || f.Kind == "sandbox_egress"
			record[f.Path] = sandboxLedgerEntry{Kind: f.Kind, RenderSHA256: result.RenderSHA256}
		case transport.SiteFileOutcomeUnchanged:
			if ledger[f.Path].RenderSHA256 != result.RenderSHA256 {
				record[f.Path] = sandboxLedgerEntry{Kind: f.Kind, RenderSHA256: result.RenderSHA256}
				changed = true
			}
		case transport.SiteFileOutcomeKept:
			log.Printf("site sandbox: %s was changed on this server and is kept (%s); CelikPanel's text is in %s", f.Path, result.State, result.PendingPath)
		default:
			return fmt.Errorf("%s could not be written (%s %s): %s", f.Path, result.Outcome, result.Reason, result.Detail)
		}
	}
	if changed {
		for path, entry := range record {
			if err := setSandboxLedgerEntry(path, entry.Kind, entry.RenderSHA256); err != nil {
				log.Printf("site sandbox: record the native files: %v", err)
			}
		}
		if out, err := appUnitSystemctl(systemctlArgs("daemon-reload")...); err != nil {
			return fmt.Errorf("systemctl daemon-reload: %v: %s", err, strings.TrimSpace(string(out)))
		}
	}
	// A changed rule set (S4 R2b: the submission exception added or taken
	// away) reaches the running table at once; loading the file again is
	// atomic and keeps the set's members (EgressRuleset).
	if rules {
		if err := sandboxReloadEgress(); err != nil {
			return err
		}
	}
	recordMailSubmission(submission, why)
	if out, err := appUnitSystemctl(systemctlArgs("enable", "--now", sitesandbox.EgressGuardTimer)...); err != nil {
		return fmt.Errorf("start %s: %v: %s", sitesandbox.EgressGuardTimer, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// siteSubmissionStateNow is siteSubmissionState (a test seam).
var siteSubmissionStateNow = siteSubmissionState

// sandboxReloadEgress loads the SMTP block's file again (a test seam). The
// launch names nft literally (fix round: no computed launch passes the
// launch guard by its file name alone).
var sandboxReloadEgress = func() error {
	path := filepath.Join(sandboxEgressDir, filepath.Base(sitesandbox.EgressFile))
	cmd := exec.Command("/usr/sbin/nft", "-f", path)
	cmd.Env = []string{}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("nft -f %s: %v: %s", path, err, strings.TrimSpace(string(out)))
	}
	return nil
}

var sandboxUnitFile = regexp.MustCompile(`^(celiksvc)-([1-9][0-9]{0,9})\.service$`)

// sandboxUnits lists the service-app unit files and their sandbox
// generation (read-only).
func sandboxUnits() []transport.SandboxUnit {
	out := []transport.SandboxUnit{}
	dir, err := unitDir()
	if err != nil {
		return out
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return out
	}
	for _, entry := range entries {
		m := sandboxUnitFile.FindStringSubmatch(entry.Name())
		if m == nil || !entry.Type().IsRegular() {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		id, _ := strconv.Atoi(m[2])
		out = append(out, transport.SandboxUnit{InstallID: id, Unit: entry.Name(),
			File: serviceapps.UnitTextState(string(raw)), Generation: serviceapps.UnitGeneration(string(raw))})
	}
	return out
}

// SandboxCapsV1 reads the host's sandbox capabilities, its level and the
// state of the SMTP egress block. It changes nothing.
func (a *Agent) SandboxCapsV1(_ *transport.Empty, resp *transport.SandboxCapsResponse) error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	resetSandboxCapsCache()
	caps := sandboxCaps(ctx)
	assessment := sitesandbox.Assess(caps)
	resp.Caps = transportSandboxCaps(caps)
	resp.Level = string(assessment.Level)
	resp.Gaps, resp.Extras = []string{}, []string{}
	for _, g := range assessment.Gaps {
		resp.Gaps = append(resp.Gaps, string(g))
	}
	for _, e := range assessment.Extras {
		resp.Extras = append(resp.Extras, string(e))
	}
	resp.Generation = sitesandbox.Generation
	resp.SMTPRejects = -1
	if files, err := sandboxNativeFiles(recordedMailSubmission() == transport.MailSubmissionOurs); err == nil {
		result, _, _ := services.InspectManagedSiteFile(files[0].managed(readSandboxLedger()[files[0].Path].RenderSHA256))
		resp.EgressFile = result.State
	}
	if out, err := sandboxRunner.Run(ctx, "", sitesandbox.NFT, "list", "chain", "inet", sitesandbox.EgressTable, "output"); err == nil {
		resp.EgressLoaded = strings.Contains(string(out), "@"+sitesandbox.EgressSet)
	}
	if out, err := sandboxRunner.Run(ctx, "", sitesandbox.NFT, "list", "counter", "inet", sitesandbox.EgressTable, sitesandbox.EgressCounter); err == nil {
		resp.SMTPRejects = sitesandbox.ParseCounter(string(out))
	}
	if out, err := appUnitSystemctl(systemctlArgs("show", sitesandbox.EgressGuardTimer, "--property=ActiveState", "--value")...); err == nil {
		resp.GuardTimer = strings.TrimSpace(string(out))
	}
	resp.Units = append(sandboxUnits(), nodeSiteUnits()...)
	return nil
}

// sandboxProof proves a running unit against its profile (fails closed).
func sandboxProof(ctx context.Context, unit string, profile sitesandbox.Profile, caps sitesandbox.Caps, listen *sitesandbox.ListenProof) sitesandbox.ProofResult {
	in := sitesandbox.ProofInput{Unit: unit, Profile: profile, Caps: caps, Listen: listen}
	args := append([]string{"show", unit}, "--property="+strings.Join(sitesandbox.ShowProperties(), ","))
	out, err := appUnitSystemctl(systemctlArgs(args...)...)
	if err != nil {
		return sitesandbox.ProofResult{Code: sitesandbox.ProofUnreadable, Detail: "systemctl show: " + strings.TrimSpace(string(out))}
	}
	in.Show = string(out)
	if caps.EgressEnforced() {
		if out, err := sandboxRunner.Run(ctx, "", sitesandbox.NFT, "list", "set", "inet", sitesandbox.EgressTable, sitesandbox.EgressSet); err == nil {
			in.SetElements = sitesandbox.ParseSetElements(string(out))
		}
		if out, err := sandboxRunner.Run(ctx, "", sitesandbox.NFT, "list", "chain", "inet", sitesandbox.EgressTable, "output"); err == nil {
			in.OutputChain = string(out)
		}
	}
	return sitesandbox.Prove(in)
}

// serviceAppSandboxProof proves a service app's long-running unit.
func serviceAppSandboxProof(ctx context.Context, sc serviceAppContext, req *transport.ServiceAppStatusRequest, listen sitesandbox.ListenProof) *transport.SandboxProof {
	caps := sandboxCaps(ctx)
	limits := serviceAppLimits(sc.app, req.Limits)
	aux := 0
	if sc.app.AuxPort {
		aux = req.AuxPort
	}
	unit := serviceapps.UnitName(req.Target.InstallID) + ".service"
	profile := serviceapps.SandboxProfile(unit, sc.layout, sc.account.Name, sc.account.Group, limits, limits.MemoryMB, req.Port, aux, caps)
	// S4 R2a: the unit may run in its site's slice.
	profile.SiteID = sc.siteID
	result := sandboxProof(ctx, unit, profile, caps, &listen)
	return &transport.SandboxProof{OK: result.OK, Code: result.Code, Detail: result.Detail}
}
