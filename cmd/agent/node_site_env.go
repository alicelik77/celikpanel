package main

import (
	"os"
	"strings"

	"github.com/alicelik/celikpanel/internal/nodesite"
	"github.com/alicelik/celikpanel/internal/siteruntime"
	"github.com/alicelik/celikpanel/internal/transport"
)

// A Node.js site's environment variables (alpha.128, PLESK-GAP G19). The
// values live in one plain systemd EnvironmentFile, root's and 0600, in the
// runtime apps' root folder (0700): systemd reads it before the sandbox
// starts, the site's account cannot read it from disk or plant a link in its
// place, and the application keeps its environment without CelikPanel
// (D-022). The application unit names the file (EnvironmentFile=-) only
// while the file exists, so a site without variables keeps its unit exactly
// as it was. After the file changes the Panel renders the unit again (the
// line appears or goes) and restarts a running application.
//
// Node.js sitesinin ortam değişkenleri: root'a ait 0600 tek dosya; birim
// dosyayı yalnız dosya varken adlandırır.

// nodeSiteEnvDir is the folder of the environment files (a test seam).
var nodeSiteEnvDir = siteruntime.EnvDir

func nodeSiteEnvPath(siteID int) string { return nodesite.EnvPath(nodeSiteEnvDir, siteID) }

// nodeSiteEnvFileIfPresent is the file the unit names, "" while there is none.
func nodeSiteEnvFileIfPresent(siteID int) string {
	path := nodeSiteEnvPath(siteID)
	if info, err := os.Lstat(path); err == nil && info.Mode().IsRegular() {
		return path
	}
	return ""
}

// NodeSiteEnvV1 replaces the site's environment file. It starts and stops
// nothing; the Panel renders the unit and restarts the application after.
func (a *Agent) NodeSiteEnvV1(req *transport.NodeSiteEnvRequest, resp *transport.RuntimeAppStepResponse) error {
	if req == nil {
		runtimeStep(resp, transport.RuntimeAppVerifiedFailure, "invalid_request", "a request is required; nothing was changed")
		return nil
	}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "changing a Node.js site's environment"); err != nil {
		runtimeStep(resp, transport.RuntimeAppPrerequisite, "build_mismatch", err.Error())
		return nil
	}
	if req.SiteID <= 0 {
		runtimeStep(resp, transport.RuntimeAppVerifiedFailure, "invalid_request", "a site is required; nothing was changed")
		return nil
	}
	unlock := lockAppUnitMutation(req.SiteID)
	defer unlock()
	path := nodeSiteEnvPath(req.SiteID)
	entries := make([]siteruntime.EnvEntry, len(req.Entries))
	for i, e := range req.Entries {
		entries[i] = siteruntime.EnvEntry{Key: e.Key, Value: e.Value, Secret: e.Secret}
	}
	existing := map[string]string{}
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() {
			runtimeStep(resp, transport.RuntimeAppPrerequisite, "env_file",
				path+" is not a regular file (a link or a folder); nothing was changed. The administrator moves it aside, then save again")
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			runtimeStep(resp, transport.RuntimeAppUnknown, "env_file", err.Error())
			return nil
		}
		existing = siteruntime.ParseEnv(string(raw))
	} else if !os.IsNotExist(err) {
		runtimeStep(resp, transport.RuntimeAppUnknown, "env_file", err.Error())
		return nil
	}
	if added := siteruntime.EnvOwnerKeys(existing, req.PreviousKeys, entries); len(added) > 0 {
		runtimeStep(resp, transport.RuntimeAppPrerequisite, "env_edited",
			"the environment file "+path+" has keys CelikPanel did not write ("+strings.Join(added, ", ")+
				"); nothing was changed. Add them here to keep them, or remove them from the file, then save again")
		return nil
	}
	if len(entries) == 0 {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			runtimeStep(resp, transport.RuntimeAppUnknown, "env_file", err.Error())
			return nil
		}
		resp.Done = true
		return nil
	}
	merged, err := siteruntime.MergeEnv(entries, existing)
	if err != nil {
		runtimeStep(resp, transport.RuntimeAppVerifiedFailure, "env_values", err.Error())
		return nil
	}
	text, err := siteruntime.RenderEnv(merged)
	if err != nil {
		runtimeStep(resp, transport.RuntimeAppVerifiedFailure, "env_values", err.Error())
		return nil
	}
	if err := writeRootEnvFile(nodeSiteEnvDir, path, text); err != nil {
		runtimeStep(resp, transport.RuntimeAppUnknown, "env_file", err.Error())
		return nil
	}
	resp.Done = true
	return nil
}

// writeRootEnvFile writes an environment file root's and 0600 in a root
// folder (0700), through a new temporary file and one rename.
func writeRootEnvFile(dir, path, text string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return err
	}
	tmp := path + ".partial"
	_ = os.Remove(tmp)
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, werr := f.WriteString(text)
	if werr == nil {
		werr = f.Sync()
	}
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		_ = os.Remove(tmp)
		return werr
	}
	return os.Rename(tmp, path)
}
