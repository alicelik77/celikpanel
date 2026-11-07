package main

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.128 (G32): the repository address is checked, the key answer carries
// the public key only, and a deploy needs a saved repository.
func TestGitDeploySettingsKeyAndDeploy(t *testing.T) {
	p, domainID, owner, _ := newSiteFeatureFixture(t)
	calls := []string{}
	saved := gitDeployAgent
	t.Cleanup(func() { gitDeployAgent = saved })
	gitDeployAgent = func(_ *Panel, _ context.Context, method string, req, resp any) error {
		calls = append(calls, method)
		switch method {
		case "Agent.GitDeployKeyV1":
			resp.(*transport.GitDeployKeyResponse).PublicKey = "ssh-ed25519 AAAAC3Nza celikpanel-deploy@x"
			resp.(*transport.GitDeployKeyResponse).Fingerprint = "SHA256:abc"
		case "Agent.GitDeployV1":
			resp.(*transport.GitDeployResponse).RequestID = req.(*transport.GitDeployRequest).RequestID
			resp.(*transport.GitDeployResponse).Phase = "running"
		}
		return nil
	}
	deploy := `{"request_id":"git-request-0001"}`
	recorder := featureRequest(p, domainID, owner, http.MethodPost, "git/deploy", deploy)
	if recorder.Code != http.StatusConflict || len(calls) != 0 {
		t.Fatalf("deploy without repository: %d %s", recorder.Code, recorder.Body.String())
	}
	for _, bad := range []string{`{"repo_url":"https://u:p@github.com/a/b.git","branch":"main"}`, `{"repo_url":"file:///etc","branch":"main"}`,
		`{"repo_url":"https://github.com/a/b.git","branch":"-x"}`, `{"repo_url":"https://github.com/a/b.git","branch":"main","build":true}`} {
		if recorder = featureRequest(p, domainID, owner, http.MethodPost, "git", bad); recorder.Code != http.StatusBadRequest {
			t.Fatalf("%s: %d", bad, recorder.Code)
		}
	}
	recorder = featureRequest(p, domainID, owner, http.MethodPost, "git", `{"repo_url":"git@github.com:acme/site.git","branch":"main"}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("save: %d %s", recorder.Code, recorder.Body.String())
	}
	recorder = featureRequest(p, domainID, owner, http.MethodPost, "git/key", `{}`)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "ssh-ed25519") || strings.Contains(recorder.Body.String(), "PRIVATE") {
		t.Fatalf("key: %d %s", recorder.Code, recorder.Body.String())
	}
	recorder = featureRequest(p, domainID, owner, http.MethodPost, "git/deploy", deploy)
	if recorder.Code != http.StatusOK || calls[len(calls)-1] != "Agent.GitDeployV1" {
		t.Fatalf("deploy: %d %s %v", recorder.Code, recorder.Body.String(), calls)
	}
}
