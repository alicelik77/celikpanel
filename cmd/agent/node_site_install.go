package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/hostingpath"
	"github.com/alicelik/celikpanel/internal/nodesite"
	"github.com/alicelik/celikpanel/internal/siteruntime"
	"github.com/alicelik/celikpanel/internal/sitesandbox"
	"github.com/alicelik/celikpanel/internal/transport"
)

// The install step of a Node.js site (alpha.127): npm ci (npm install
// without a lock file) and, when asked, npm run build, as the transient unit
// celikappinst-<site id> in the site sandbox (nodesite.InstallRunArgs). It
// runs only for a site whose application unit is already in the sandbox: a
// customer's package scripts never run outside it. The application is not
// touched; the customer restarts it when the step succeeded.
//
// The start is idempotent by request id and one step runs per site; the
// status read never starts anything. It records the end it observed so that
// the answer survives the transient unit (a reboot removes it).
//
// Node.js sitesinin kurulum adımı: npm ci/install ve istenirse npm run build,
// korumalı geçici birimde. Yalnız korumaya taşınmış sitede çalışır; durum
// okuması hiçbir şey başlatmaz.

var (
	nodeInstallStateRoot = filepath.Join(hostingpath.ServiceMutationStateRoot(), "node-sites")
	nodeInstallRequestID = regexp.MustCompile(`^[A-Za-z0-9-]{8,64}$`)
)

type nodeInstallRecord struct {
	RequestID string   `json:"request_id"`
	Phase     string   `json:"phase"`
	Clean     bool     `json:"clean"`
	Build     bool     `json:"build"`
	StartedAt string   `json:"started_at"`
	EndedAt   string   `json:"ended_at,omitempty"`
	Class     string   `json:"class,omitempty"`
	Reason    string   `json:"reason,omitempty"`
	Detail    string   `json:"detail,omitempty"`
	Lines     []string `json:"lines,omitempty"`
}

func nodeInstallRecordPath(siteID int) string {
	return filepath.Join(nodeInstallStateRoot, strconv.Itoa(siteID), "install.json")
}

func readNodeInstallRecord(siteID int) nodeInstallRecord {
	var r nodeInstallRecord
	if raw, err := os.ReadFile(nodeInstallRecordPath(siteID)); err == nil {
		_ = json.Unmarshal(raw, &r)
	}
	return r
}

func writeNodeInstallRecord(siteID int, r nodeInstallRecord) error {
	p := nodeInstallRecordPath(siteID)
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

func (r nodeInstallRecord) answer(resp *transport.NodeSiteInstallResponse) {
	resp.RequestID, resp.Phase, resp.Clean, resp.Build = r.RequestID, r.Phase, r.Clean, r.Build
	resp.StartedAt, resp.EndedAt, resp.Lines = r.StartedAt, r.EndedAt, r.Lines
	resp.Class, resp.Reason, resp.Error = r.Class, r.Reason, r.Detail
}

func nodeInstallRefuse(resp *transport.NodeSiteInstallResponse, class, reason, detail string) {
	resp.Class, resp.Reason, resp.Error = class, reason, detail
}

// nodeInstallUnitShow reads the transient unit (a seam).
var nodeInstallUnitShow = func(siteID int) (map[string]string, bool, error) {
	out, err := appUnitSystemctl(systemctlArgs("show", nodesite.InstallUnitName(siteID), "--property=LoadState,ActiveState,Result,Job")...)
	if err != nil {
		return nil, false, fmt.Errorf("systemctl show: %v: %s", err, strings.TrimSpace(string(out)))
	}
	props := parseShow(string(out))
	return props, props["LoadState"] != "" && props["LoadState"] != "not-found", nil
}

// nodeInstallJournal is the step's own output since it started (a seam).
var nodeInstallJournal = func(ctx context.Context, siteID int, since time.Time) []string {
	args := []string{"-u", nodesite.InstallUnitName(siteID), "-n", "200", "--no-pager", "-o", "cat"}
	if !since.IsZero() {
		args = append(args, "--since=@"+strconv.FormatInt(since.Unix(), 10))
	}
	if systemdUserMode {
		args = append([]string{"--user"}, args...)
	}
	out, err := serviceAppRun(ctx, "journalctl", args...)
	if err != nil {
		return nil
	}
	return strings.Split(strings.TrimRight(string(out), "\n"), "\n")
}

// nodeInstallFileKind reports whether name in the document root is a
// regular file (never following a link): "file", "missing" or "other".
func nodeInstallFileKind(appDir, name string) string {
	info, err := os.Lstat(filepath.Join(appDir, name))
	switch {
	case errors.Is(err, os.ErrNotExist):
		return "missing"
	case err != nil || !info.Mode().IsRegular():
		return "other"
	}
	return "file"
}

// NodeSiteInstallV1 starts a Node.js site's install step (see the file
// comment). A repeated request id answers the first answer.
func (a *Agent) NodeSiteInstallV1(req *transport.NodeSiteInstallRequest, resp *transport.NodeSiteInstallResponse) error {
	if req == nil {
		nodeInstallRefuse(resp, transport.RuntimeAppVerifiedFailure, "invalid_request", "a request is required")
		return nil
	}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "installing a Node.js site's packages"); err != nil {
		nodeInstallRefuse(resp, transport.RuntimeAppPrerequisite, "build_mismatch", err.Error())
		return nil
	}
	if req.SiteID <= 0 || !nodeInstallRequestID.MatchString(req.RequestID) {
		nodeInstallRefuse(resp, transport.RuntimeAppVerifiedFailure, "invalid_request", "a site and a request identity are required")
		return nil
	}
	unlock := lockAppUnitMutation(req.SiteID)
	defer unlock()
	record := readNodeInstallRecord(req.SiteID)
	if record.RequestID == req.RequestID {
		nodeInstallObserve(req.SiteID, &record)
		record.answer(resp)
		return nil
	}
	if record.Phase == nodesite.InstallRunning {
		nodeInstallObserve(req.SiteID, &record)
		if record.Phase == nodesite.InstallRunning {
			nodeInstallRefuse(resp, transport.RuntimeAppPrerequisite, "install_running",
				"nothing new was started: an install step of this site is still running; it is shown until it ends")
			return nil
		}
	}
	if generation, read := nodeSiteUnitGeneration(req.SiteID); !read || generation < sitesandbox.Generation {
		nodeInstallRefuse(resp, transport.RuntimeAppPrerequisite, "earlier_release",
			"nothing was started: this site's application runs in an earlier release's protection, and its packages are installed only in the site sandbox. The administrator moves it under Components → Node.js, then install again")
		return nil
	}
	if err := nodesite.ValidAppDir(req.WorkDir); err != nil {
		nodeInstallRefuse(resp, transport.RuntimeAppPrerequisite, "app_folder", err.Error())
		return nil
	}
	if _, err := nodesite.RuntimeDir(req.NodeVersion); err != nil {
		nodeInstallRefuse(resp, transport.RuntimeAppVerifiedFailure, "invalid_request", err.Error())
		return nil
	}
	installed := filepath.Join(runtimesBaseDir, "node", req.NodeVersion)
	for _, p := range []string{nodesite.NodeBinary(installed), nodesite.NPMCLI(installed)} {
		if _, err := os.Stat(p); err != nil {
			nodeInstallRefuse(resp, transport.RuntimeAppPrerequisite, "node_missing",
				"Node.js "+req.NodeVersion+" (with its npm) is not installed on this server; the administrator installs it under Components → Node.js")
			return nil
		}
	}
	switch nodeInstallFileKind(req.WorkDir, "package.json") {
	case "missing":
		nodeInstallRefuse(resp, transport.RuntimeAppPrerequisite, "package_json_missing",
			"nothing was started: there is no package.json in the site's folder. Upload the project (with package.json) with the file manager or SFTP, then install again")
		return nil
	case "other":
		nodeInstallRefuse(resp, transport.RuntimeAppPrerequisite, "package_json_not_file",
			"nothing was started: package.json in the site's folder is not a regular file (a link or a folder). Put the real file there, then install again")
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	account, err := nodeSiteAccountLookup(req.RunAsUser, req.WorkDir)
	if err != nil {
		nodeInstallRefuse(resp, transport.RuntimeAppPrerequisite, "site_account", err.Error())
		return nil
	}
	caps := nodeSiteCaps(ctx)
	params := nodesite.InstallParams{
		SiteID: req.SiteID, Domain: strings.ToLower(strings.TrimSpace(req.Domain)), User: account.Name, Group: account.Group,
		AppDir: req.WorkDir, NodeVersion: req.NodeVersion, Clean: nodeInstallFileKind(req.WorkDir, "package-lock.json") == "file",
		Build: req.Build, Caps: caps,
	}
	args, err := nodesite.InstallRunArgs(params)
	if err != nil {
		nodeInstallRefuse(resp, transport.RuntimeAppVerifiedFailure, "unit_render", err.Error())
		return nil
	}
	plan := nodeSitePlan{sandboxed: true, account: account, caps: caps, params: nodesite.Params{AppDir: req.WorkDir}}
	if err := nodeSitePrepare(plan); err != nil {
		class := transport.RuntimeAppPrerequisite
		if code := nodeSiteRefusalCode(err); code == "sandbox_helper" || code == "sandbox_egress" {
			class = transport.RuntimeAppUnknown
		}
		nodeInstallRefuse(resp, class, nodeSiteRefusalCode(err), err.Error())
		return nil
	}
	// One step per site: the unit of an earlier step must be gone before
	// systemd-run makes the new one under the same name.
	if err := clearTransientUnit(nodesite.InstallUnitName(req.SiteID)); err != nil {
		nodeInstallRefuse(resp, transport.RuntimeAppUnknown, "install_unit", err.Error())
		return nil
	}
	record = nodeInstallRecord{RequestID: req.RequestID, Phase: nodesite.InstallRunning, Clean: params.Clean, Build: params.Build,
		StartedAt: time.Now().UTC().Format(time.RFC3339)}
	if err := writeNodeInstallRecord(req.SiteID, record); err != nil {
		nodeInstallRefuse(resp, transport.RuntimeAppUnknown, "install_record", err.Error())
		return nil
	}
	if out, err := serviceAppRun(ctx, "systemd-run", args...); err != nil {
		record.Phase, record.Class, record.Reason = nodesite.InstallFailed, transport.RuntimeAppVerifiedFailure, "install_not_started"
		record.Detail = "the install step could not be started: " + siteruntime.RedactBuildLine(strings.TrimSpace(string(out)))
		record.EndedAt = time.Now().UTC().Format(time.RFC3339)
		_ = writeNodeInstallRecord(req.SiteID, record)
	}
	record.answer(resp)
	return nil
}

// NodeSiteInstallStatusV1 reads a site's install step. It starts nothing; it
// records an end it observed.
func (a *Agent) NodeSiteInstallStatusV1(req *transport.NodeSiteInstallStatusRequest, resp *transport.NodeSiteInstallResponse) error {
	if req == nil || req.SiteID <= 0 {
		nodeInstallRefuse(resp, transport.RuntimeAppVerifiedFailure, "invalid_request", "a site is required")
		return nil
	}
	unlock := lockAppUnitMutation(req.SiteID)
	defer unlock()
	record := readNodeInstallRecord(req.SiteID)
	nodeInstallObserve(req.SiteID, &record)
	record.answer(resp)
	return nil
}

// nodeInstallObserve completes a running record from the unit's state.
func nodeInstallObserve(siteID int, record *nodeInstallRecord) {
	if record.Phase != nodesite.InstallRunning {
		return
	}
	props, found, err := nodeInstallUnitShow(siteID)
	if err != nil {
		return
	}
	phase := nodesite.InstallOutcome(found, props["ActiveState"], props["Result"], props["Job"])
	since, _ := time.Parse(time.RFC3339, record.StartedAt)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	lines := nodeInstallJournal(ctx, siteID, since)
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
		record.Class, record.Reason = transport.RuntimeAppVerifiedFailure, "install_failed"
		record.Detail = "npm ended with an error (" + strings.TrimSpace(props["ActiveState"]+" "+props["Result"]) + "); the last lines of its output are shown. Correct the project, then install again"
		if !found {
			record.Class, record.Reason = transport.RuntimeAppUnknown, "install_lost"
			record.Detail = "the install step is no longer on the server (the server restarted while it ran); install again"
		}
	}
	if err := writeNodeInstallRecord(siteID, *record); err != nil {
		log.Printf("node install site %d: record: %v", siteID, err)
	}
}
