package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/backupspec"
	"github.com/alicelik/celikpanel/internal/hostingpath"
	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.128 (G8): copying a site goes only to a site the caller manages; a
// target with content needs the overwrite confirmation, which makes a backup
// first; a failed copy keeps nothing.
func TestSiteCopyTargetsBackupFirstAndRefusals(t *testing.T) {
	p, sourceID, owner, _ := newSiteFeatureFixture(t)
	db := p.db.GetDB()
	var subID int
	if err := db.QueryRow(`SELECT subscription_id FROM domains WHERE id = ?`, sourceID).Scan(&subID); err != nil {
		t.Fatal(err)
	}
	res, err := db.Exec(`INSERT INTO domains (subscription_id, name) VALUES (?, 'copy-target.example')`, subID)
	if err != nil {
		t.Fatal(err)
	}
	targetID64, _ := res.LastInsertId()
	targetID := int(targetID64)
	docroot, err := hostingpath.DocumentRoot(subID, targetID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO sites (domain_id, document_root) VALUES (?, ?)`, targetID, docroot); err != nil {
		t.Fatal(err)
	}

	recorder := featureRequest(p, sourceID, owner, http.MethodGet, "copy", "")
	var listing struct {
		Targets []siteCopyTarget `json:"targets"`
	}
	_ = json.Unmarshal(recorder.Body.Bytes(), &listing)
	if recorder.Code != http.StatusOK || len(listing.Targets) != 1 || listing.Targets[0].DomainID != targetID {
		t.Fatalf("targets: %d %s", recorder.Code, recorder.Body.String())
	}

	calls := []string{}
	var copyReq transport.SiteCopyRequest
	notEmpty := true
	saved := siteCopyAgent
	t.Cleanup(func() { siteCopyAgent = saved })
	siteCopyAgent = func(_ *Panel, _ context.Context, method string, req, resp any) error {
		calls = append(calls, method)
		switch method {
		case "Agent.CreateBackup":
			resp.(*backupspec.CreateResponse).Success = true
			resp.(*backupspec.CreateResponse).Backup.Name = "copy-target.example-safety.tar.gz"
		case "Agent.SiteCopyV1":
			copyReq = *req.(*transport.SiteCopyRequest)
			answer := resp.(*transport.SiteCopyResponse)
			if notEmpty && !copyReq.Overwrite {
				answer.Code, answer.CompensationSafe = transport.SiteCopyTargetNotEmpty, true
				return nil
			}
			answer.Copied = true
		}
		return nil
	}
	body := `{"target_domain_id":` + featureNum(targetID) + `}`
	recorder = featureRequest(p, sourceID, owner, http.MethodPost, "copy", body)
	if recorder.Code != http.StatusConflict || featureCode(t, recorder) != errCodeSiteCopyNotEmpty || strings.Join(calls, ",") != "Agent.SiteCopyV1" {
		t.Fatalf("not empty: %d %s %v", recorder.Code, recorder.Body.String(), calls)
	}
	calls = nil
	recorder = featureRequest(p, sourceID, owner, http.MethodPost, "copy", `{"target_domain_id":`+featureNum(targetID)+`,"overwrite":true}`)
	if recorder.Code != http.StatusOK || strings.Join(calls, ",") != "Agent.CreateBackup,Agent.SiteCopyV1" ||
		!strings.Contains(recorder.Body.String(), "copy-target.example-safety.tar.gz") {
		t.Fatalf("overwrite: %d %s %v", recorder.Code, recorder.Body.String(), calls)
	}
	if copyReq.Source.DomainID != sourceID || copyReq.Target.DomainID != targetID || copyReq.SourceDatabase != "" {
		t.Fatalf("request: %+v", copyReq)
	}

	// A site the caller does not manage is never a target.
	stranger := &Caller{ID: owner.ID + 999, Role: roleCustomer, CustomerID: owner.ID + 999}
	recorder = featureRequest(p, sourceID, stranger, http.MethodPost, "copy", body)
	if recorder.Code == http.StatusOK {
		t.Fatalf("a stranger copied: %s", recorder.Body.String())
	}
	// A database that is not this site's is refused before anything runs.
	calls = nil
	recorder = featureRequest(p, sourceID, owner, http.MethodPost, "copy", `{"target_domain_id":`+featureNum(targetID)+`,"database_id":424242}`)
	if recorder.Code != http.StatusBadRequest || len(calls) != 0 {
		t.Fatalf("foreign database: %d %s %v", recorder.Code, recorder.Body.String(), calls)
	}
}
