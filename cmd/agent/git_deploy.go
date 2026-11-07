package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/hostname"
	"github.com/alicelik/celikpanel/internal/nodesite"
	"github.com/alicelik/celikpanel/internal/siteruntime"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Git deploy of a site (alpha.128, PLESK-GAP G32; nodesite/gitdeploy.go).
// The deploy key is made here with the distribution's ssh-keygen, root's and
// 0600 in the runtime apps' root folder (0700); only its public half leaves
// the server. A deploy runs as the transient unit celikgit-<site id> in the
// site sandbox; its start is idempotent by request id, one runs per site, and
// the status read never starts anything (the Node.js install step's pattern).
//
// Git ile yayın: anahtar sunucuda yapılır, yalnız açık yarısı dışarı çıkar;
// yayın site korumasındaki geçici birimde çalışır.

// Seams (tests replace them).
var (
	gitDeployKeyDir = siteruntime.EnvDir
	gitBinaryPath   = nodesite.GitBinary
	gitKeygen       = func(ctx context.Context, args ...string) ([]byte, error) {
		return serviceAppRun(ctx, "ssh-keygen", args...)
	}
	gitDeployUnitShow = func(siteID int) (map[string]string, bool, error) {
		out, err := appUnitSystemctl(systemctlArgs("show", nodesite.GitUnitName(siteID), "--property=LoadState,ActiveState,Result,Job")...)
		if err != nil {
			return nil, false, fmt.Errorf("systemctl show: %v: %s", err, strings.TrimSpace(string(out)))
		}
		props := parseShow(string(out))
		return props, props["LoadState"] != "" && props["LoadState"] != "not-found", nil
	}
	gitDeployJournal = func(ctx context.Context, siteID int, since time.Time) []string {
		args := []string{"-u", nodesite.GitUnitName(siteID), "-n", "200", "--no-pager", "-o", "cat"}
		if !since.IsZero() {
			args = append(args, "--since=@"+strconv.FormatInt(since.Unix(), 10))
		}
		out, err := serviceAppRun(ctx, "journalctl", args...)
		if err != nil {
			return nil
		}
		return strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	}
)

func gitDeployKeyPath(siteID int) string {
	return filepath.Join(gitDeployKeyDir, "gitkey-"+strconv.Itoa(siteID))
}

func gitDeployRecordPath(siteID int) string {
	return filepath.Join(nodeInstallStateRoot, strconv.Itoa(siteID), "git.json")
}

func readGitDeployRecord(siteID int) nodeInstallRecord {
	var r nodeInstallRecord
	if raw, err := os.ReadFile(gitDeployRecordPath(siteID)); err == nil {
		_ = json.Unmarshal(raw, &r)
	}
	return r
}

func writeGitDeployRecord(siteID int, r nodeInstallRecord) error {
	p := gitDeployRecordPath(siteID)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	raw, err := json.Marshal(r)
	if err != nil {
		return err
	}
	if err := os.WriteFile(p+".tmp", raw, 0o600); err != nil {
		return err
	}
	return os.Rename(p+".tmp", p)
}

func gitAnswer(r nodeInstallRecord, resp *transport.GitDeployResponse) {
	resp.RequestID, resp.Phase, resp.StartedAt, resp.EndedAt, resp.Lines = r.RequestID, r.Phase, r.StartedAt, r.EndedAt, r.Lines
	resp.Class, resp.Reason, resp.Error = r.Class, r.Reason, r.Detail
}

func gitRefuse(resp *transport.GitDeployResponse, class, reason, detail string) {
	resp.Class, resp.Reason, resp.Error = class, reason, detail
}

// gitKeyFingerprint reads the key's fingerprint ("" without a key).
func gitKeyFingerprint(siteID int) string {
	pub := gitDeployKeyPath(siteID) + ".pub"
	if info, err := os.Lstat(pub); err != nil || !info.Mode().IsRegular() {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := gitKeygen(ctx, "-l", "-f", pub)
	if err != nil {
		return ""
	}
	fields := strings.Fields(string(out))
	if len(fields) < 2 {
		return ""
	}
	return fields[1]
}

// GitDeployKeyV1 makes the site's deploy key and returns its public half.
func (a *Agent) GitDeployKeyV1(req *transport.GitDeployKeyRequest, resp *transport.GitDeployKeyResponse) error {
	if req == nil {
		resp.Error = "a request is required"
		return nil
	}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "making a deploy key"); err != nil {
		resp.Error = err.Error()
		return nil
	}
	domain, err := hostname.CanonicalFQDN(req.Domain)
	if err != nil || req.SiteID <= 0 {
		resp.Error = "a site and its domain are required"
		return nil
	}
	unlock := lockAppUnitMutation(req.SiteID)
	defer unlock()
	key := gitDeployKeyPath(req.SiteID)
	if _, err := os.Lstat(key); err == nil && !req.Replace {
		resp.Reason, resp.Error = "key_exists", "this site already has a deploy key; make a new one only to replace it"
		return nil
	}
	if err := os.MkdirAll(gitDeployKeyDir, 0o700); err != nil {
		resp.Error = err.Error()
		return nil
	}
	if err := os.Chmod(gitDeployKeyDir, 0o700); err != nil {
		resp.Error = err.Error()
		return nil
	}
	tmp := key + ".new"
	_ = os.Remove(tmp)
	_ = os.Remove(tmp + ".pub")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if out, err := gitKeygen(ctx, "-q", "-t", "ed25519", "-N", "", "-C", "celikpanel-deploy@"+domain, "-f", tmp); err != nil {
		resp.Reason = "keygen"
		resp.Error = "ssh-keygen (openssh-client) could not make the key: " + boundedText(strings.TrimSpace(string(out)), 200)
		return nil
	}
	pub, err := os.ReadFile(tmp + ".pub")
	if err != nil {
		resp.Error = err.Error()
		return nil
	}
	_ = os.Chmod(tmp, 0o600)
	if err := os.Rename(tmp, key); err != nil {
		resp.Error = err.Error()
		return nil
	}
	if err := os.Rename(tmp+".pub", key+".pub"); err != nil {
		resp.Error = err.Error()
		return nil
	}
	resp.PublicKey = strings.TrimSpace(string(pub))
	resp.Fingerprint = gitKeyFingerprint(req.SiteID)
	return nil
}

// GitDeployV1 starts one deploy of the site's branch.
func (a *Agent) GitDeployV1(req *transport.GitDeployRequest, resp *transport.GitDeployResponse) error {
	if req == nil {
		gitRefuse(resp, transport.RuntimeAppVerifiedFailure, "invalid_request", "a request is required")
		return nil
	}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "deploying a site from Git"); err != nil {
		gitRefuse(resp, transport.RuntimeAppPrerequisite, "build_mismatch", err.Error())
		return nil
	}
	if req.SiteID <= 0 || !nodeInstallRequestID.MatchString(req.RequestID) {
		gitRefuse(resp, transport.RuntimeAppVerifiedFailure, "invalid_request", "a site and a request identity are required")
		return nil
	}
	unlock := lockAppUnitMutation(req.SiteID)
	defer unlock()
	record := readGitDeployRecord(req.SiteID)
	if record.RequestID == req.RequestID {
		gitObserve(req.SiteID, &record)
		gitAnswer(record, resp)
		return nil
	}
	if record.Phase == nodesite.InstallRunning {
		gitObserve(req.SiteID, &record)
		if record.Phase == nodesite.InstallRunning {
			gitRefuse(resp, transport.RuntimeAppPrerequisite, "deploy_running", "nothing new was started: a deploy of this site is still running; it is shown until it ends")
			return nil
		}
	}
	if _, err := os.Stat(gitBinaryPath); err != nil {
		gitRefuse(resp, transport.RuntimeAppPrerequisite, "git_missing", "git is not installed on this server; the administrator installs the distribution's git package, then deploy again")
		return nil
	}
	ssh, err := nodesite.ParseGitURL(req.RepoURL)
	if err != nil {
		gitRefuse(resp, transport.RuntimeAppVerifiedFailure, "repo_url", err.Error())
		return nil
	}
	keyFile := ""
	if ssh {
		keyFile = gitDeployKeyPath(req.SiteID)
		if info, err := os.Lstat(keyFile); err != nil || !info.Mode().IsRegular() {
			gitRefuse(resp, transport.RuntimeAppPrerequisite, "key_missing", "an SSH repository needs the site's deploy key; make one and add its public key to the repository, then deploy again")
			return nil
		}
	}
	if err := nodesite.ValidAppDir(req.WorkDir); err != nil {
		gitRefuse(resp, transport.RuntimeAppPrerequisite, "app_folder", err.Error())
		return nil
	}
	if req.Build {
		installed := filepath.Join(runtimesBaseDir, "node", req.NodeVersion)
		if _, err := os.Stat(nodesite.NodeBinary(installed)); err != nil {
			gitRefuse(resp, transport.RuntimeAppPrerequisite, "node_missing", "Node.js "+req.NodeVersion+" is not installed on this server; the administrator installs it under Components → Node.js")
			return nil
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	account, err := nodeSiteAccountLookup(req.RunAsUser, req.WorkDir)
	if err != nil {
		gitRefuse(resp, transport.RuntimeAppPrerequisite, "site_account", err.Error())
		return nil
	}
	caps := nodeSiteCaps(ctx)
	params := nodesite.GitDeployParams{
		SiteID: req.SiteID, Domain: strings.ToLower(strings.TrimSpace(req.Domain)), User: account.Name, Group: account.Group,
		AppDir: req.WorkDir, RepoURL: req.RepoURL, Branch: req.Branch, KeyFile: keyFile,
		NodeVersion: req.NodeVersion, Build: req.Build, Clean: nodeInstallFileKind(req.WorkDir, "package-lock.json") == "file", Caps: caps,
	}
	args, err := nodesite.GitDeployRunArgs(params)
	if err != nil {
		gitRefuse(resp, transport.RuntimeAppVerifiedFailure, "unit_render", err.Error())
		return nil
	}
	plan := nodeSitePlan{sandboxed: true, account: account, caps: caps, params: nodesite.Params{AppDir: req.WorkDir}}
	if err := nodeSitePrepare(plan); err != nil {
		gitRefuse(resp, transport.RuntimeAppPrerequisite, nodeSiteRefusalCode(err), err.Error())
		return nil
	}
	if err := clearTransientUnit(nodesite.GitUnitName(req.SiteID)); err != nil {
		gitRefuse(resp, transport.RuntimeAppUnknown, "deploy_unit", err.Error())
		return nil
	}
	record = nodeInstallRecord{RequestID: req.RequestID, Phase: nodesite.InstallRunning, Build: req.Build,
		StartedAt: time.Now().UTC().Format(time.RFC3339)}
	if err := writeGitDeployRecord(req.SiteID, record); err != nil {
		gitRefuse(resp, transport.RuntimeAppUnknown, "deploy_record", err.Error())
		return nil
	}
	if out, err := serviceAppRun(ctx, "systemd-run", args...); err != nil {
		record.Phase, record.Class, record.Reason = nodesite.InstallFailed, transport.RuntimeAppVerifiedFailure, "deploy_not_started"
		record.Detail = "the deploy could not be started: " + siteruntime.RedactBuildLine(strings.TrimSpace(string(out)))
		record.EndedAt = time.Now().UTC().Format(time.RFC3339)
		_ = writeGitDeployRecord(req.SiteID, record)
	}
	gitAnswer(record, resp)
	return nil
}

// GitDeployStatusV1 reads the last deploy and the key's fingerprint.
func (a *Agent) GitDeployStatusV1(req *transport.GitDeployStatusRequest, resp *transport.GitDeployResponse) error {
	if req == nil || req.SiteID <= 0 {
		gitRefuse(resp, transport.RuntimeAppVerifiedFailure, "invalid_request", "a site is required")
		return nil
	}
	unlock := lockAppUnitMutation(req.SiteID)
	defer unlock()
	record := readGitDeployRecord(req.SiteID)
	gitObserve(req.SiteID, &record)
	gitAnswer(record, resp)
	resp.KeyFingerprint = gitKeyFingerprint(req.SiteID)
	return nil
}

// gitObserve completes a running record from the unit's state.
func gitObserve(siteID int, record *nodeInstallRecord) {
	if record.Phase != nodesite.InstallRunning {
		return
	}
	props, found, err := gitDeployUnitShow(siteID)
	if err != nil {
		return
	}
	phase := nodesite.InstallOutcome(found, props["ActiveState"], props["Result"], props["Job"])
	since, _ := time.Parse(time.RFC3339, record.StartedAt)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	lines := gitDeployJournal(ctx, siteID, since)
	for i, line := range lines {
		lines[i] = siteruntime.RedactBuildLine(line)
	}
	record.Lines = siteruntime.BuildTail(lines, 12, 2400)
	if phase == nodesite.InstallRunning {
		return
	}
	record.Phase = phase
	record.EndedAt = time.Now().UTC().Format(time.RFC3339)
	if phase == nodesite.InstallFailed {
		record.Class, record.Reason = transport.RuntimeAppVerifiedFailure, "deploy_failed"
		record.Detail = "the deploy ended with an error; the last lines of its output are shown. The files of the last good deploy stay where git did not replace them yet; correct the problem, then deploy again"
		if !found {
			record.Class, record.Reason = transport.RuntimeAppUnknown, "deploy_lost"
			record.Detail = "the deploy is no longer on the server (the server restarted while it ran); deploy again"
		}
	}
	_ = writeGitDeployRecord(siteID, *record)
}
