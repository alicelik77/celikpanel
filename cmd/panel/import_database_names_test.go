package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.134 second review: a cPanel archive's database whose exact right
// MariaDB cannot keep (each `_` counts twice; 64 is the limit) is named in the
// preview with why, and the import leaves it out - nothing is created under
// it - while the other databases come in.
func TestImportLeavesOutADatabaseWhoseNameIsTooLongAndSaysSoFirst(t *testing.T) {
	long := "olduser_" + strings.Repeat("a_", 26)
	preview := set2Preview()
	preview.Databases = append(preview.Databases, transport.CpmoveDatabase{Name: long, DumpBytes: 10})

	panel, agent, userID, subscriptionID, domainID := newRequestIdentityHostingFixture(t)
	agent.preview = preview
	recorder := httptest.NewRecorder()
	panel.handleImportInspect(recorder, requestIdentityTestRequest("/api/v1/import/cpanel/inspect", `{"path":"`+set2ImportedPath+`"}`, "", &Caller{ID: userID, Role: roleAdmin}))
	if recorder.Code != http.StatusOK {
		t.Fatalf("preview %d: %s", recorder.Code, recorder.Body.String())
	}
	var seen struct {
		Databases []importPreviewDatabase `json:"databases"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &seen); err != nil {
		t.Fatal(err)
	}
	if len(seen.Databases) != 2 || seen.Databases[0].NameTooLong || !seen.Databases[1].NameTooLong {
		t.Fatalf("preview databases: %+v", seen.Databases)
	}

	// The apply's database steps (the fixture's Agent stops a whole apply
	// earlier, at the hosting check): the long name is a left-out step.
	var serverID int
	if _, err := panel.db.GetDB().Exec(`INSERT INTO database_servers (subscription_id, type_id, name, version, host, port, root_password_encrypted, status)
		VALUES (?, 2, 'import-test', 'test', '127.0.0.1', 3306, '', 'active')`, subscriptionID); err != nil {
		t.Fatal(err)
	}
	if err := panel.db.GetDB().QueryRow(`SELECT id FROM database_servers WHERE name = 'import-test'`).Scan(&serverID); err != nil {
		t.Fatal(err)
	}
	var steps []importStep
	panel.importArchiveDatabases(context.Background(), set2ImportedPath, subscriptionID, serverID, domainID, preview.Databases,
		func(step string, err error) { steps = append(steps, importStep{Step: step, Detail: err.Error()}) },
		func(step, detail string) { steps = append(steps, importStep{Step: step, OK: true, Detail: detail}) },
		func(step, state, detail string) {
			steps = append(steps, importStep{Step: step, OK: true, State: state, Detail: detail})
		})
	agent.mu.Lock()
	imported := append([]string(nil), agent.imported...)
	agent.mu.Unlock()
	if strings.Join(imported, ",") != "olduser_shop" {
		t.Fatalf("imported %v", imported)
	}
	if len(steps) != 2 || steps[0].Step != "database:olduser_shop" || !steps[0].OK || steps[0].State != "" ||
		steps[1].Step != "database:"+long || !steps[1].OK || steps[1].State != importStateNameTooLong || steps[1].Detail != importDatabaseNameTooLong {
		t.Fatalf("steps %+v", steps)
	}
	answer := importApplyAnswerFor("imported.example", 1, 1, steps)
	if len(answer.LeftOut) != 1 || answer.LeftOut[0] != "database:"+long {
		t.Fatalf("answer %+v", answer)
	}
	var recorded int
	if err := panel.db.GetDB().QueryRow(`SELECT COUNT(*) FROM databases_v2 WHERE name = ?`, long).Scan(&recorded); err != nil || recorded != 0 {
		t.Fatalf("recorded %d %v", recorded, err)
	}
	// The page's own words match the server's line.
	page := readPanelSource(t, "../../web/src/components/ImportPage.tsx")
	if !strings.Contains(page, "'"+importDatabaseNameTooLong+"'") {
		t.Fatal("the import page has no words for the left-out database line")
	}
}
