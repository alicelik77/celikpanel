package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.128 (G19): a Node.js site's environment variables. A secret value is
// sent to the Agent, never stored by the Panel or shown back; the save runs
// file → unit → restart.
func TestNodeEnvNeverStoresOrShowsASecret(t *testing.T) {
	p, domainID, owner, _ := newSiteFeatureFixture(t)
	recorder := featureRequest(p, domainID, owner, http.MethodGet, "app/env", "")
	if recorder.Code != http.StatusConflict || featureCode(t, recorder) != "NODE_ENV_REFUSED" {
		t.Fatalf("php site: %d %s", recorder.Code, recorder.Body.String())
	}
	if _, err := p.db.GetDB().Exec(`UPDATE sites SET project_type = 'node' WHERE domain_id = ?`, domainID); err != nil {
		t.Fatal(err)
	}
	var sent []transport.NodeSiteEnvRequest
	steps := []string{}
	savedEnv, savedUnit, savedRestart := nodeSiteEnvCall, nodeSiteEnvRenderUnit, nodeSiteEnvRestart
	t.Cleanup(func() { nodeSiteEnvCall, nodeSiteEnvRenderUnit, nodeSiteEnvRestart = savedEnv, savedUnit, savedRestart })
	nodeSiteEnvCall = func(_ *Panel, _ context.Context, req *transport.NodeSiteEnvRequest) (transport.RuntimeAppStepResponse, error) {
		sent = append(sent, *req)
		steps = append(steps, "file")
		return transport.RuntimeAppStepResponse{Done: true}, nil
	}
	nodeSiteEnvRenderUnit = func(*Panel, context.Context, int) error { steps = append(steps, "unit"); return nil }
	nodeSiteEnvRestart = func(*Panel, context.Context, int) (bool, error) { steps = append(steps, "restart"); return true, nil }

	recorder = featureRequest(p, domainID, owner, http.MethodPost, "app/env",
		`{"entries":[{"key":"API_URL","value":"https://x.example"},{"key":"API_TOKEN","value":"s3cret-value-long","secret":true}]}`)
	if recorder.Code != http.StatusOK || strings.Join(steps, ",") != "file,unit,restart" {
		t.Fatalf("save: %d %s %v", recorder.Code, recorder.Body.String(), steps)
	}
	if len(sent) != 1 || sent[0].Entries[1].Value != "s3cret-value-long" {
		t.Fatalf("agent request: %+v", sent)
	}
	var stored string
	_ = p.db.GetDB().QueryRow(`SELECT env_public || env_secret_keys FROM node_site_env`).Scan(&stored)
	if strings.Contains(stored, "s3cret") || !strings.Contains(stored, "API_TOKEN") {
		t.Fatalf("stored: %s", stored)
	}
	recorder = featureRequest(p, domainID, owner, http.MethodGet, "app/env", "")
	if strings.Contains(recorder.Body.String(), "s3cret") {
		t.Fatalf("the secret is shown back: %s", recorder.Body.String())
	}
	var view struct {
		Entries []nodeEnvEntryView `json:"entries"`
	}
	_ = json.Unmarshal(recorder.Body.Bytes(), &view)
	if len(view.Entries) != 2 || !view.Entries[0].Secret || view.Entries[0].Key != "API_TOKEN" {
		t.Fatalf("view: %+v", view)
	}
	// A new secret without a value is refused before anything is sent.
	recorder = featureRequest(p, domainID, owner, http.MethodPost, "app/env", `{"entries":[{"key":"NEW_SECRET","secret":true}]}`)
	if recorder.Code != http.StatusBadRequest || len(sent) != 1 {
		t.Fatalf("empty new secret: %d %s", recorder.Code, recorder.Body.String())
	}
}
