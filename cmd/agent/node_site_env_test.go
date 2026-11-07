package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.128 (G19): a Node.js site's environment file is root's 0600, keeps a
// secret the request leaves empty, refuses a key the owner added by hand, and
// is removed with the last variable.
func TestNodeSiteEnvFileKeepsSecretsAndOwnerKeys(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "celikpanel-runtime")
	saved := nodeSiteEnvDir
	nodeSiteEnvDir = dir
	t.Cleanup(func() { nodeSiteEnvDir = saved })
	agent := &Agent{}
	call := func(req transport.NodeSiteEnvRequest) transport.RuntimeAppStepResponse {
		var resp transport.RuntimeAppStepResponse
		req.SiteID = 7
		_ = agent.NodeSiteEnvV1(&req, &resp)
		return resp
	}
	if nodeSiteEnvFileIfPresent(7) != "" {
		t.Fatal("a missing file is named")
	}
	resp := call(transport.NodeSiteEnvRequest{Entries: []transport.RuntimeAppEnvEntry{
		{Key: "API_URL", Value: "https://api.example"}, {Key: "API_TOKEN", Value: "s3cret-value-long", Secret: true},
	}})
	if !resp.Done {
		t.Fatalf("first save: %+v", resp)
	}
	path := nodeSiteEnvFileIfPresent(7)
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("file: %v %v", info, err)
	}
	// The secret without a value keeps the server's value.
	resp = call(transport.NodeSiteEnvRequest{PreviousKeys: []string{"API_URL", "API_TOKEN"}, Entries: []transport.RuntimeAppEnvEntry{
		{Key: "API_URL", Value: "https://api2.example"}, {Key: "API_TOKEN", Secret: true},
	}})
	raw, _ := os.ReadFile(path)
	if !resp.Done || !strings.Contains(string(raw), `API_TOKEN="s3cret-value-long"`) || !strings.Contains(string(raw), "api2.example") {
		t.Fatalf("second save: %+v\n%s", resp, raw)
	}
	// A key the owner added by hand is never dropped silently.
	if err := os.WriteFile(path, append(raw, []byte("OWNER_KEY=\"x\"\n")...), 0o600); err != nil {
		t.Fatal(err)
	}
	resp = call(transport.NodeSiteEnvRequest{PreviousKeys: []string{"API_URL", "API_TOKEN"}, Entries: []transport.RuntimeAppEnvEntry{{Key: "API_URL", Value: "x"}}})
	if resp.Done || resp.Reason != "env_edited" {
		t.Fatalf("owner key: %+v", resp)
	}
	// No variables: the file goes, and the unit stops naming it.
	resp = call(transport.NodeSiteEnvRequest{PreviousKeys: []string{"API_URL", "API_TOKEN", "OWNER_KEY"}})
	if !resp.Done || nodeSiteEnvFileIfPresent(7) != "" {
		t.Fatalf("cleared: %+v", resp)
	}
}
