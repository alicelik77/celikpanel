package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// A server setup run starts its own mail profile install. The operation says
// so (setup_child) so the browser does not warn about a certificate that the
// setup replaces a few steps later; an install the owner starts is unmarked.
func TestServiceOperationReadMarksServerSetupChildren(t *testing.T) {
	f := newServiceOperationTestFixture(t)
	db := f.database.GetDB()
	actor := serviceOperationActor{UserID: f.userID}

	childRequestID, err := newServiceOperationID()
	if err != nil {
		t.Fatal(err)
	}
	ownRequestID, err := newServiceOperationID()
	if err != nil {
		t.Fatal(err)
	}
	child, err := f.panel.createServiceOperationRequest(
		t.Context(), serviceOperationKindMailProfileInstall, "webmail", "", childRequestID, actor,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE service_operations SET status='succeeded', finished_at=? WHERE id=?`,
		child.StartedAt, child.ID); err != nil {
		t.Fatal(err)
	}
	own, err := f.panel.createServiceOperationRequest(
		t.Context(), serviceOperationKindMailProfileInstall, "webmail", "", ownRequestID, actor,
	)
	if err != nil {
		t.Fatal(err)
	}

	// The execution record names the first operation as one of its steps.
	execution := serverSetupExecution{ID: "e1", RequestID: "e1", PlanID: "p1", Status: "running"}
	execution.Steps = []serverSetupExecutionStep{{
		serverSetupPlanStep: serverSetupPlanStep{ID: "mail_profile", Kind: "mail_profile", Target: "webmail"},
		Status:              "running", RequestID: childRequestID, OwnerID: "o1",
	}}
	encoded, err := json.Marshal(execution)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO users (id,username,password_hash,email,role)
		VALUES (9001,'setup-owner','x','owner@example.test','admin')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO server_setup_plans (id,revision,plan_json,requested_by,created_at)
		VALUES ('p1',1,'{}',9001,'2026-11-02T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO server_setup_executions
		(id,request_id,plan_id,status,execution_json,created_at,updated_at)
		VALUES ('e1','e1','p1','succeeded',?,'2026-11-02T00:00:00Z','2026-11-02T00:00:00Z')`,
		string(encoded)); err != nil {
		t.Fatal(err)
	}

	got, body := getServiceOperation(t, f.panel, f.userID, child.ID)
	if got == nil || !got.SetupChild || !strings.Contains(body, `"setup_child":true`) {
		t.Fatalf("a setup child is not marked: %s", body)
	}
	got, body = getServiceOperation(t, f.panel, f.userID, own.ID)
	if got == nil || got.SetupChild || strings.Contains(body, "setup_child") {
		t.Fatalf("an owner-started install is marked as a setup child: %s", body)
	}
}
