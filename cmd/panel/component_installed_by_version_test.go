package main

import (
	"encoding/json"
	"net/http"
	"testing"
)

// alpha.140 (alpha.138 lab): the generic package install for Node.js was
// queued and then failed at the package step, although Components → Node.js
// installs it by version. It is now refused before anything is queued, with
// the place where the version is chosen; no operation row, no Agent call.
func TestGenericInstallOfARuntimeInstalledByVersionIsRefusedBeforeQueueing(t *testing.T) {
	f := newServiceOperationTestFixture(t)

	recorder, started := postServiceInstall(t, f, "node")
	if recorder.Code != http.StatusConflict || started != nil {
		t.Fatalf("status=%d operation=%+v body=%s", recorder.Code, started, recorder.Body.String())
	}
	var body apiErrorBody
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != errCodeComponentInstalledByVersion || body.Action != "/components?component=node&install=" ||
		body.Vars["component"] != "node" || body.Vars["name"] != "Node.js" {
		t.Fatalf("refusal = %+v", body)
	}
	assertNoServiceOperationQueued(t, f)

	// The same through "install together" (a requirement confirmed in the
	// install window): refused, nothing queued.
	together, startedTogether := postServiceInstallRequest(t, f, serviceInstallRequest{
		ServiceID: "node", InstallTogether: []string{"nginx"}, RequestID: mustServiceOperationRequestID(t),
	})
	if together.Code == http.StatusAccepted || startedTogether != nil {
		t.Fatalf("install together status=%d operation=%+v body=%s", together.Code, startedTogether, together.Body.String())
	}
	assertNoServiceOperationQueued(t, f)
}

// A component with a distribution package keeps the generic install.
func TestGenericInstallOfAPackagedRuntimeIsNotRefusedAsInstalledByVersion(t *testing.T) {
	if refuseComponentInstalledByVersion(nil, "python", "nginx", "php-fpm", "unknown-id") {
		t.Fatal("a packaged component was refused as installed by version")
	}
}

func assertNoServiceOperationQueued(t *testing.T, f serviceOperationTestFixture) {
	t.Helper()
	var rows int
	if err := f.database.GetDB().QueryRow(`SELECT COUNT(*) FROM service_operations`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 || f.agent.installCalls.Load() != 0 {
		t.Fatalf("operations=%d install calls=%d", rows, f.agent.installCalls.Load())
	}
}
