package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/alicelik/celikpanel/internal/backupspec"
	"github.com/alicelik/celikpanel/internal/transport"
)

// WordPress updates (alpha.118): a backup is always made first; an update
// that changed files and failed is undone by restoring exactly that backup;
// an update whose answer was lost is never "undone" blind.

type wordpressToolsAgent struct {
	verifiedAPTAgentRPCFixture
	mu        sync.Mutex
	calls     []string
	createErr string
	update    transport.WordPressUpdateResponse
	updateErr error
	restore   backupspec.RestoreResponse
	restored  []string
	created   []backupspec.CreateRequest
}

func (a *wordpressToolsAgent) record(name string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.calls = append(a.calls, name)
}

func (a *wordpressToolsAgent) CreateBackup(req *backupspec.CreateRequest, resp *backupspec.CreateResponse) error {
	a.record("CreateBackup")
	a.mu.Lock()
	a.created = append(a.created, *req)
	a.mu.Unlock()
	if a.createErr != "" {
		resp.Error = a.createErr
		return nil
	}
	resp.Success = true
	resp.Backup = backupspec.Info{Name: "pre-update-1.tar", Type: req.Type, Origin: req.Origin, Restorable: true}
	return nil
}

func (a *wordpressToolsAgent) WordPressUpdateV1(req *transport.WordPressUpdateRequest, resp *transport.WordPressUpdateResponse) error {
	a.record("WordPressUpdateV1")
	*resp = a.update
	return a.updateErr
}

func (a *wordpressToolsAgent) InspectBackup(req *backupspec.InspectRequest, resp *backupspec.InspectResponse) error {
	a.record("InspectBackup")
	resp.Success = true
	resp.Backup = backupspec.Info{Name: req.BackupName, Type: backupspec.TypeFiles, Restorable: true}
	return nil
}

func (a *wordpressToolsAgent) RestoreBackup(req *backupspec.RestoreRequest, resp *backupspec.RestoreResponse) error {
	a.record("RestoreBackup")
	a.mu.Lock()
	a.restored = append(a.restored, req.BackupName)
	a.mu.Unlock()
	*resp = a.restore
	return nil
}

func (a *wordpressToolsAgent) snapshot() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.calls...)
}

func runPluginUpdateForTest(t *testing.T, agent *wordpressToolsAgent, kind string) (string, string, string) {
	t.Helper()
	fixture := newWordPressInstallFixture(t)
	attachWordPressInstallAgent(t, fixture.panel, agent)
	ctx := context.Background()
	target, err := fixture.panel.resolveWordPressSite(ctx, wordpressDomain)
	if err != nil {
		t.Fatal(err)
	}
	op := wordpressOperationView{OperationID: panelWordPressOperationID, Kind: "update", Scope: kind, Items: []string{"akismet/akismet.php"}, Origin: "manual"}
	if err := fixture.panel.insertWordPressOperation(ctx, op, wordpressDomain, wordpressSubscription, wordpressAdminID); err != nil {
		t.Fatal(err)
	}
	// A second operation for the same site is refused while this one is open.
	second := op
	second.OperationID = strings.Repeat("b", 32)
	if err := fixture.panel.insertWordPressOperation(ctx, second, wordpressDomain, wordpressSubscription, wordpressAdminID); !errors.Is(err, errWordPressBusy) {
		t.Fatalf("a second open operation was recorded: %v", err)
	}
	fixture.panel.runWordPressUpdate(ctx, op.OperationID, target, kind, op.Items, "6.7.1", false)
	var status, code, backup string
	if err := fixture.panel.db.GetDB().QueryRow(`SELECT status, COALESCE(code, ''), COALESCE(backup_name, '') FROM wordpress_operations WHERE operation_id = ?`,
		op.OperationID).Scan(&status, &code, &backup); err != nil {
		t.Fatal(err)
	}
	return status, code, backup
}

func TestWordPressUpdateMakesTheBackupFirst(t *testing.T) {
	agent := &wordpressToolsAgent{update: transport.WordPressUpdateResponse{Changed: true, Updated: []string{"akismet 5.4"}, Detail: "1 plugin(s) updated"}}
	status, _, backup := runPluginUpdateForTest(t, agent, "plugin")
	calls := agent.snapshot()
	if status != "applied" || backup != "pre-update-1.tar" {
		t.Fatalf("status=%s backup=%s", status, backup)
	}
	if len(calls) != 2 || calls[0] != "CreateBackup" || calls[1] != "WordPressUpdateV1" {
		t.Fatalf("calls = %v, want the backup before the update", calls)
	}
	if created := agent.created[0]; created.Origin != backupspec.OriginPreUpdate || created.Type != backupspec.TypeFiles || created.JobKey == "" {
		t.Fatalf("backup request = %+v", created)
	}
}

func TestWordPressUpdateDoesNothingWhenTheBackupFails(t *testing.T) {
	agent := &wordpressToolsAgent{createErr: "disk full"}
	status, code, _ := runPluginUpdateForTest(t, agent, "plugin")
	if status != "failed" || code != errCodeWordPressBackupFailed {
		t.Fatalf("status=%s code=%s", status, code)
	}
	for _, call := range agent.snapshot() {
		if call == "WordPressUpdateV1" {
			t.Fatal("the update ran without its backup")
		}
	}
}

func TestWordPressUpdateRestoresTheBackupWhenFilesChangedAndItFailed(t *testing.T) {
	agent := &wordpressToolsAgent{
		update:  transport.WordPressUpdateResponse{Changed: true, Code: transport.WordPressCodeHealthFailed, Error: "the site answers HTTP 500"},
		restore: backupspec.RestoreResponse{Success: true},
	}
	status, code, backup := runPluginUpdateForTest(t, agent, "plugin")
	if status != "rolled_back" || code != transport.WordPressCodeHealthFailed {
		t.Fatalf("status=%s code=%s", status, code)
	}
	if len(agent.restored) != 1 || agent.restored[0] != backup {
		t.Fatalf("restored %v, want exactly the backup %s", agent.restored, backup)
	}
}

func TestWordPressUpdateWhoseRestoreFailsNeedsReview(t *testing.T) {
	agent := &wordpressToolsAgent{
		update:  transport.WordPressUpdateResponse{Changed: true, Error: "copy failed"},
		restore: backupspec.RestoreResponse{Error: "restore failed"},
	}
	status, code, _ := runPluginUpdateForTest(t, agent, "plugin")
	if status != "needs_review" || code != errCodeWordPressRestoreFailed {
		t.Fatalf("status=%s code=%s", status, code)
	}
}

func TestWordPressUpdateThatChangedNothingIsNotRestored(t *testing.T) {
	for name, answer := range map[string]transport.WordPressUpdateResponse{
		"nothing changed": {Changed: false, Code: transport.WordPressCodeVerifyFailed, Error: "checksum mismatch"},
		"agent put back":  {Changed: true, RolledBack: true, Error: "health failed"},
	} {
		agent := &wordpressToolsAgent{update: answer}
		status, _, _ := runPluginUpdateForTest(t, agent, "plugin")
		if len(agent.restored) != 0 {
			t.Errorf("%s: a backup was restored", name)
		}
		if want := map[bool]string{false: "failed", true: "rolled_back"}[answer.Changed]; status != want {
			t.Errorf("%s: status=%s want %s", name, status, want)
		}
	}
}

func TestWordPressUpdateWithALostAnswerIsNeverUndoneBlind(t *testing.T) {
	agent := &wordpressToolsAgent{updateErr: errors.New("connection reset")}
	status, code, backup := runPluginUpdateForTest(t, agent, "plugin")
	if status != "needs_review" || code != errCodeWordPressResultUnknown || backup == "" {
		t.Fatalf("status=%s code=%s backup=%s", status, code, backup)
	}
	if len(agent.restored) != 0 {
		t.Fatal("a backup was restored although the update's result is unknown")
	}
}

func TestWordPressCoreUpdateNeedsTheDatabaseInTheBackup(t *testing.T) {
	agent := &wordpressToolsAgent{}
	status, code, _ := runPluginUpdateForTest(t, agent, "core")
	if status != "failed" || code != errCodeWordPressDBNotLinked || len(agent.snapshot()) != 0 {
		t.Fatalf("status=%s code=%s calls=%v", status, code, agent.snapshot())
	}
}

func TestInterruptedWordPressOperationsAreMarkedNotRepeated(t *testing.T) {
	fixture := newWordPressInstallFixture(t)
	ctx := context.Background()
	for i, status := range []string{"queued", "updating"} {
		id := strings.Repeat(string(rune('c'+i)), 32)
		if _, err := fixture.panel.db.GetDB().Exec(`INSERT INTO wordpress_operations (operation_id, domain_id, subscription_id, kind, status)
			VALUES (?, ?, ?, 'update', ?)`, id, wordpressDomain+i, wordpressSubscription, status); err != nil {
			t.Fatal(err)
		}
	}
	if n, err := fixture.panel.recoverInterruptedWordPressOperations(ctx); err != nil || n != 2 {
		t.Fatalf("recovered %d %v", n, err)
	}
	rows, _ := fixture.panel.db.GetDB().Query(`SELECT status FROM wordpress_operations ORDER BY operation_id`)
	defer rows.Close()
	var got []string
	for rows.Next() {
		var s string
		_ = rows.Scan(&s)
		got = append(got, s)
	}
	if strings.Join(got, ",") != "failed,needs_review" {
		t.Fatalf("statuses = %v", got)
	}
}

// Rights (D-042): the WordPress tools and the malware scan results are
// refused to a subscription without the permission; the administrator is
// never refused.
func TestSiteToolsRightsGates(t *testing.T) {
	f := newRightsFixture(t)
	customer := &Caller{ID: authzMatrixCustomerID, Role: roleCustomer}
	for _, row := range []struct {
		right string
		path  string
		serve func(http.ResponseWriter, *http.Request)
	}{
		{rightWordPressTools, "/api/v1/wordpress/9902", f.panel.handleWordPressTools},
		{rightMalwareScan, "/api/v1/malware/domains/9902", f.panel.handleMalware},
	} {
		f.setCustom(t, 9802, newRightsSet(false, true, everyPermissionBut(row.right)...))
		recorder := httptest.NewRecorder()
		row.serve(recorder, requestAs(http.MethodGet, row.path, customer, ""))
		if recorder.Code != http.StatusForbidden || !strings.Contains(recorder.Body.String(), errCodeRightNotGranted) {
			t.Errorf("%s without %s = %d %s", row.path, row.right, recorder.Code, recorder.Body.String())
		}
		f.setCustom(t, 9802, newRightsSet(false, true, permissionItemHead+row.right))
		recorder = httptest.NewRecorder()
		row.serve(recorder, requestAs(http.MethodGet, row.path, customer, ""))
		if recorder.Code == http.StatusForbidden {
			t.Errorf("%s with %s refused: %s", row.path, row.right, recorder.Body.String())
		}
	}
	// The new permissions are part of the closed set with names.
	for _, id := range []string{rightWordPressTools, rightMalwareScan} {
		if !knownPermission(id) || rightPermissionNames[id] == "" {
			t.Errorf("%s is not a known permission", id)
		}
	}
}
