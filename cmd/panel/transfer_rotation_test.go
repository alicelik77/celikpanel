package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/alicelik/celikpanel/internal/secrets"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Credential rotation after a transfer (owner decision, 2 November 2026).
// Subscription 9802 (customer 9703, reseller 9702) gets a site, a mailbox, a
// VPN device, a protected folder user and three database users: one plain,
// one a WordPress CelikPanel installed uses, one on another server. The
// server side is a fake that records every call and every password it saw.

type fakeRotationNative struct {
	mu        sync.Mutex
	calls     []string
	passwords []string
	// siteRefusal / mailErr / dbAppConflict make a step fail.
	siteRefusal   string
	mailErr       error
	mailErrOnce   bool
	dbAppConflict bool
	vpnSyncs      int
}

func (f *fakeRotationNative) record(call string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call)
}

func (f *fakeRotationNative) count(prefix string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if strings.HasPrefix(c, prefix) {
			n++
		}
	}
	return n
}

func (f *fakeRotationNative) fileAccessState(_ context.Context, site transport.FileAccessSite) (transport.FileAccessStateResponse, error) {
	f.record("state " + site.Username)
	return transport.FileAccessStateResponse{Account: transport.FileAccessAccount{Username: site.Username, Exists: true, Enabled: true,
		Password: "set", Keys: []transport.FileAccessKey{{Type: "ssh-ed25519", Fingerprint: "SHA256:x"}}}}, nil
}

func (f *fakeRotationNative) revokeFileAccess(_ context.Context, req transport.ApplyFileAccessRequest) (transport.ApplyFileAccessResponse, error) {
	f.record("revoke " + req.Site.Username)
	if !req.RevokeCredentials || req.Enabled || req.Password != "" {
		return transport.ApplyFileAccessResponse{}, errors.New("not a revocation")
	}
	if f.siteRefusal != "" {
		return transport.ApplyFileAccessResponse{Refusal: f.siteRefusal, Detail: "the sshd drop-in was changed by hand"}, nil
	}
	return transport.ApplyFileAccessResponse{Applied: true, State: transport.FileAccessStateResponse{
		Account: transport.FileAccessAccount{Username: req.Site.Username, Exists: true, Password: "none"}}}, nil
}

func (f *fakeRotationNative) setMailboxPassword(_ context.Context, email, password string) (bool, error) {
	f.record("mail " + email)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.mailErr != nil {
		err := f.mailErr
		if f.mailErrOnce {
			f.mailErr = nil
		}
		return false, err
	}
	f.passwords = append(f.passwords, password)
	return true, nil
}

func (f *fakeRotationNative) endMailSessions(_ context.Context, email string) (transport.EndMailSessionsResponse, error) {
	f.record("kick " + email)
	return transport.EndMailSessionsResponse{Applied: true}, nil
}

func (f *fakeRotationNative) rotateDatabaseUser(_ context.Context, req transport.RotateDatabaseUserPasswordRequest) (transport.RotateDatabaseUserPasswordResponse, error) {
	app := ""
	if req.App != nil {
		app = " app=" + req.App.Kind + ":" + req.App.DBName
	}
	f.record("db " + req.User.Username + app)
	f.mu.Lock()
	defer f.mu.Unlock()
	if req.App != nil && req.App.Required && f.dbAppConflict {
		return transport.RotateDatabaseUserPasswordResponse{AppConflict: true, AppDetail: "wp-config.php no longer uses database cust_wp with user wp_user"}, nil
	}
	f.passwords = append(f.passwords, req.NewPassword)
	return transport.RotateDatabaseUserPasswordResponse{Applied: true, PasswordChanged: true, SessionsEnded: true, AppUpdated: req.App != nil}, nil
}

func (f *fakeRotationNative) syncVPN(context.Context) error {
	f.record("vpn")
	return nil
}

type rotationFixture struct {
	transferFixture
	native *fakeRotationNative
	box    *secrets.Box
}

func newRotationFixture(t *testing.T) rotationFixture {
	t.Helper()
	base := newTransferFixture(t)
	box, err := secrets.LoadOrCreate(filepath.Join(t.TempDir(), "secrets.key"))
	if err != nil {
		t.Fatal(err)
	}
	base.panel.secrets = box
	native := &fakeRotationNative{}
	base.panel.rotationNative = native
	saved := siteFeatureApplyVhost
	siteFeatureApplyVhost = func(*Panel, context.Context, int) error { return nil }
	t.Cleanup(func() { siteFeatureApplyVhost = saved })

	var typeID int
	if err := base.panel.db.GetDB().QueryRow(`SELECT id FROM database_server_types WHERE name = 'mariadb'`).Scan(&typeID); err != nil {
		t.Fatal(err)
	}
	sealed, err := box.Encrypt("the-old-password")
	if err != nil {
		t.Fatal(err)
	}
	base.exec(t, `INSERT INTO sites (id, domain_id, document_root, project_type) VALUES (9930, 9902, '/srv/c', 'php')`)
	base.exec(t, `INSERT INTO email_accounts (domain_id, address, password_hash) VALUES (9902, 'box@customer.matrix.test', 'x')`)
	base.exec(t, `INSERT INTO database_servers (id, subscription_id, type_id, name, version, host, port, status) VALUES (9921, 9802, ?, 'local', '10.11', 'localhost', 3306, 'active')`, typeID)
	base.exec(t, `INSERT INTO database_servers (id, subscription_id, type_id, name, version, host, port, status) VALUES (9922, 9802, ?, 'remote', '10.11', 'db.example.net', 3306, 'active')`, typeID)
	base.exec(t, `INSERT INTO database_users (id, server_id, subscription_id, username, password) VALUES
		(9941, 9921, 9802, 'cust_user', ?), (9942, 9921, 9802, 'wp_user', ?), (9943, 9922, 9802, 'far_user', ?)`, sealed, sealed, sealed)
	base.exec(t, `INSERT INTO application_install_operations (operation_id, app_id, domain_id, subscription_id, site_id,
		database_server_id, database_name, database_user, cleanup_token_encrypted, status)
		VALUES ('0123456789abcdef0123456789abcdef', 'wordpress', 9902, 9802, 9930, 9921, 'cust_wp', 'wp_user', 'x', 'applied')`)
	base.exec(t, `INSERT INTO vpn_peers (subscription_id, name, public_key, preshared_key, ip, desired_state) VALUES (9802, 'phone', 'pk', 'psk', '10.66.0.2', 'active')`)
	base.exec(t, `INSERT INTO site_protected_dirs (id, site_id, path) VALUES (9951, 9930, '/private')`)
	base.exec(t, `INSERT INTO site_protected_users (dir_id, username, password_hash) VALUES (9951, 'boss', ?)`, strings.Repeat("h", 60))
	return rotationFixture{transferFixture: base, native: native, box: box}
}

const rotationTransferPath = "/api/v1/subscriptions/9802/transfer"

func (f rotationFixture) transfer(t *testing.T, requestID, extra string) {
	t.Helper()
	wantStatus(t, f.do(t, authzMatrixResellerID, http.MethodPost, rotationTransferPath,
		`{"to_user_id":9706,"from_user_id":9703,"request_id":"`+requestID+`","confirm":true`+extra+`}`), http.StatusOK, "")
}

func (f rotationFixture) run(t *testing.T) {
	t.Helper()
	if err := f.panel.runPendingTransferRotations(context.Background()); err != nil {
		t.Logf("worker: %v", err)
	}
}

func (f rotationFixture) steps(t *testing.T) map[string]string {
	t.Helper()
	rows, err := f.panel.db.GetDB().Query(`SELECT kind, target, state FROM transfer_rotation_effects ORDER BY seq`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var kind, target, state string
		if err := rows.Scan(&kind, &target, &state); err != nil {
			t.Fatal(err)
		}
		out[kind+" "+target] = state
	}
	return out
}

func (f rotationFixture) opState(t *testing.T) string {
	t.Helper()
	var state string
	if err := f.panel.db.GetDB().QueryRow(`SELECT state FROM transfer_rotations`).Scan(&state); err != nil {
		t.Fatal(err)
	}
	return state
}

func TestTransferRotationPreviewListsEveryCredential(t *testing.T) {
	f := newRotationFixture(t)
	preview := transferPreviewOf(t, f.do(t, authzMatrixResellerID, http.MethodPost, rotationTransferPath+"/preview", `{"to_user_id":9706}`))
	if preview["rotate_credentials"] != true || preview["host_changes"] != "credentials" {
		t.Fatalf("default: %v", preview)
	}
	raw, _ := json.Marshal(preview["rotation"])
	var rotation transferRotationPreview
	if err := json.Unmarshal(raw, &rotation); err != nil {
		t.Fatal(err)
	}
	if strings.Join(rotation.Mailboxes, ",") != "box@customer.matrix.test" || strings.Join(rotation.SiteAccounts, ",") != "customer_matrix_test" ||
		strings.Join(rotation.VPNDevices, ",") != "phone" || len(rotation.ProtectedUsers) != 1 || rotation.ProtectedUsers[0].Users != 1 ||
		len(rotation.DatabaseUsers) != 3 || rotation.Total != 7 {
		t.Fatalf("rotation preview: %+v", rotation)
	}
	byName := map[string]rotationDBUserLine{}
	for _, line := range rotation.DatabaseUsers {
		byName[line.Username] = line
	}
	if byName["wp_user"].AppDomain != "customer.matrix.test" || byName["far_user"].Host != "db.example.net" || byName["cust_user"].AppDomain != "" {
		t.Fatalf("database users: %+v", rotation.DatabaseUsers)
	}
	for _, want := range []string{"mail_clients", "database_apps", "file_access", "vpn_devices", "protected_folders"} {
		if !strings.Contains(strings.Join(rotation.After, ","), want) {
			t.Fatalf("after %v lacks %s", rotation.After, want)
		}
	}
	if !strings.Contains(preview["message"].(string), "every password and key the previous owner knew is changed") {
		t.Fatalf("message: %s", preview["message"])
	}
	if f.count(t, `SELECT COUNT(*) FROM transfer_rotations`) != 0 || len(f.native.calls) != 0 {
		t.Fatal("a preview started work")
	}
}

func TestTransferRotationDefaultOnOwnerChangesFirst(t *testing.T) {
	f := newRotationFixture(t)
	f.transfer(t, "rotate-0001", "")
	// The owner change and the durable operation are committed together;
	// nothing ran on the server yet.
	if f.owner(t, 9802) != 9706 || f.opState(t) != "running" || len(f.native.calls) != 0 {
		t.Fatalf("owner %d, calls %v", f.owner(t, 9802), f.native.calls)
	}
	if f.count(t, `SELECT COUNT(*) FROM vpn_peers WHERE subscription_id = 9802 AND desired_state = 'active'`) != 0 {
		t.Fatal("the VPN device stayed active after the commit")
	}
	f.run(t)
	if f.opState(t) != "completed" {
		t.Fatalf("operation %s: %v", f.opState(t), f.steps(t))
	}
	// File access goes first: a shell could read the new passwords.
	if len(f.native.calls) < 2 || f.native.calls[0] != "state customer_matrix_test" || f.native.calls[1] != "revoke customer_matrix_test" {
		t.Fatalf("order: %v", f.native.calls)
	}
	steps := f.steps(t)
	for key, want := range map[string]string{
		"site_account customer.matrix.test":  rotationStepDone,
		"vpn wireguard":                      rotationStepDone,
		"mailbox box@customer.matrix.test":   rotationStepDone,
		"database_user 9921:cust_user":       rotationStepDone,
		"database_user 9921:wp_user":         rotationStepDone,
		"database_user 9922:far_user":        rotationStepManual,
		"protected_dir customer.matrix.test": rotationStepDone,
	} {
		if steps[key] != want {
			t.Fatalf("%s = %q; all %v", key, steps[key], steps)
		}
	}
	if f.native.count("kick box@customer.matrix.test") != 1 || f.native.count("db wp_user app=wordpress:cust_wp") != 1 || f.native.count("db far_user") != 0 {
		t.Fatalf("calls %v", f.native.calls)
	}
	// The engines' new passwords are kept sealed; the remote one is not touched.
	for id, changed := range map[int]bool{9941: true, 9942: true, 9943: false} {
		var stored string
		if err := f.panel.db.GetDB().QueryRow(`SELECT password FROM database_users WHERE id = ?`, id).Scan(&stored); err != nil {
			t.Fatal(err)
		}
		plain, err := f.box.Decrypt(stored)
		if err != nil {
			t.Fatal(err)
		}
		if (plain != "the-old-password") != changed {
			t.Fatalf("user %d changed=%t", id, plain != "the-old-password")
		}
	}
	if f.count(t, `SELECT COUNT(*) FROM site_protected_users WHERE password_hash = ?`, strings.Repeat("h", 60)) != 0 {
		t.Fatal("the protected folder user kept the old password")
	}
	// The audit names counts, never a password.
	var details string
	if err := f.panel.db.GetDB().QueryRow(`SELECT details FROM audit_logs WHERE action = 'subscription.transfer_rotate'`).Scan(&details); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(details, `"result":"completed"`) || !strings.Contains(details, `"mailbox_done":1`) {
		t.Fatalf("audit %s", details)
	}
	var all string
	if err := f.panel.db.GetDB().QueryRow(`SELECT group_concat(COALESCE(details, ''), ' ') FROM audit_logs`).Scan(&all); err != nil {
		t.Fatal(err)
	}
	var effects string
	_ = f.panel.db.GetDB().QueryRow(`SELECT group_concat(ref || before || COALESCE(detail, ''), ' ') FROM transfer_rotation_effects`).Scan(&effects)
	for _, secret := range f.native.passwords {
		if strings.Contains(all, secret) || strings.Contains(effects, secret) {
			t.Fatal("a password reached the audit trail or the checkpoints")
		}
	}
	if !strings.Contains(all, `"rotate_credentials":true`) {
		t.Fatalf("transfer audit: %s", all)
	}
}

func TestTransferRotationOptOutChangesNothingOnTheServer(t *testing.T) {
	f := newRotationFixture(t)
	f.transfer(t, "rotate-0002", `,"rotate_credentials":false`)
	if f.owner(t, 9802) != 9706 || f.count(t, `SELECT COUNT(*) FROM transfer_rotations`) != 0 {
		t.Fatal("an opted-out transfer wrote a rotation")
	}
	if f.count(t, `SELECT COUNT(*) FROM vpn_peers WHERE desired_state = 'active'`) != 1 {
		t.Fatal("an opted-out transfer revoked the VPN device")
	}
	var result string
	if err := f.panel.db.GetDB().QueryRow(`SELECT result FROM subscription_transfers WHERE request_id = 'rotate-0002'`).Scan(&result); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, `"rotate_credentials":false`) || !strings.Contains(result, "are NOT changed") {
		t.Fatalf("stored result: %s", result)
	}
	// The same id with the other choice is another change.
	wantStatus(t, f.do(t, authzMatrixResellerID, http.MethodPost, rotationTransferPath,
		`{"to_user_id":9706,"from_user_id":9703,"request_id":"rotate-0002","confirm":true}`), http.StatusConflict, errCodeTransferRequestIDReused)
	f.run(t)
	if len(f.native.calls) != 0 {
		t.Fatalf("server calls: %v", f.native.calls)
	}
}

func TestTransferRotationResumesWithoutRepeatingFinishedSteps(t *testing.T) {
	f := newRotationFixture(t)
	f.native.mailErr, f.native.mailErrOnce = errors.New("agent connection reset"), true
	f.transfer(t, "rotate-0003", "")
	f.run(t)
	// An unknown result: the operation keeps running and says why.
	var lastError string
	if err := f.panel.db.GetDB().QueryRow(`SELECT state, COALESCE(last_error, '') FROM transfer_rotations`).Scan(new(string), &lastError); err != nil {
		t.Fatal(err)
	}
	if f.opState(t) != "running" || !strings.Contains(lastError, "mailbox box@customer.matrix.test") {
		t.Fatalf("after the first run: %s %q", f.opState(t), lastError)
	}
	if f.steps(t)["database_user 9921:cust_user"] != rotationStepDone {
		t.Fatal("a failed mailbox stopped the steps after it")
	}
	// A restart: the next run continues; finished steps are not repeated.
	f.run(t)
	if f.opState(t) != "completed" {
		t.Fatalf("after the second run: %v", f.steps(t))
	}
	if f.native.count("revoke ") != 1 || f.native.count("db cust_user") != 1 || f.native.count("mail ") != 2 {
		t.Fatalf("calls %v", f.native.calls)
	}
}

func TestTransferRotationFailureIsShownAndRetried(t *testing.T) {
	f := newRotationFixture(t)
	f.native.siteRefusal = transport.FileAccessRefusedSSHDOwner
	f.transfer(t, "rotate-0004", "")
	f.run(t)
	if f.opState(t) != "failed" {
		t.Fatalf("operation %s", f.opState(t))
	}
	// File access failed: the later steps wait (they would write passwords a
	// shell could read).
	steps := f.steps(t)
	if steps["site_account customer.matrix.test"] != rotationStepFailed || steps["mailbox box@customer.matrix.test"] != rotationStepPending {
		t.Fatalf("steps %v", steps)
	}
	calls := len(f.native.calls)
	// The new owner sees it with guidance; reading starts nothing.
	list := f.do(t, 9706, http.MethodGet, transferRequestsPath, "")
	wantStatus(t, list, http.StatusOK, "")
	var body struct {
		Requests []transferRow `json:"requests"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Requests) != 1 || body.Requests[0].Rotation == nil || body.Requests[0].Rotation.State != "failed" ||
		body.Requests[0].Rotation.Guidance.Result != "prerequisite" || body.Requests[0].Rotation.Counts.Failed != 1 {
		t.Fatalf("new owner's view: %s", list.Body.String())
	}
	if len(f.native.calls) != calls {
		t.Fatal("a GET ran a step")
	}
	// The approver sees it too; the previous owner (a customer) does not.
	// (The stored preview names what was rotated; the progress is "guidance".)
	if !strings.Contains(f.do(t, authzMatrixResellerID, http.MethodGet, transferRequestsPath, "").Body.String(), `"guidance"`) {
		t.Fatal("the approver does not see the rotation")
	}
	if strings.Contains(f.do(t, authzMatrixCustomerID, http.MethodGet, transferRequestsPath, "").Body.String(), `"guidance"`) {
		t.Fatal("the previous owner sees the rotation")
	}
	// Retry by the previous owner is refused; by the new owner it continues.
	retryPath := transferRequestsPath + "/" + itoa(body.Requests[0].ID) + "/rotation-retry"
	wantStatus(t, f.do(t, authzMatrixCustomerID, http.MethodPost, retryPath, ""), http.StatusForbidden, errCodeTransferNotAllowed)
	f.native.siteRefusal = ""
	wantStatus(t, f.do(t, 9706, http.MethodPost, retryPath, ""), http.StatusAccepted, "")
	// Pressing again while it runs starts nothing new.
	wantStatus(t, f.do(t, 9706, http.MethodPost, retryPath, ""), http.StatusAccepted, "")
	f.run(t)
	if f.opState(t) != "completed" {
		t.Fatalf("after retry: %v", f.steps(t))
	}
	wantStatus(t, f.do(t, 9706, http.MethodPost, retryPath, ""), http.StatusConflict, errCodeTransferRotationNothing)
	if f.count(t, `SELECT COUNT(*) FROM audit_logs WHERE action LIKE 'subscription.transfer_rotate_retry:%'`) != 1 {
		t.Fatal("the retry was not audited once")
	}
}

func TestTransferRotationWordPressConfigChangedIsStillToDo(t *testing.T) {
	f := newRotationFixture(t)
	f.native.dbAppConflict = true
	f.transfer(t, "rotate-0005", "")
	f.run(t)
	if f.opState(t) != "failed" || f.steps(t)["database_user 9921:wp_user"] != rotationStepFailed {
		t.Fatalf("operation %s: %v", f.opState(t), f.steps(t))
	}
	view, err := f.panel.transferRotationView(context.Background(), f.count(t, `SELECT transfer_id FROM transfer_rotations`))
	if err != nil {
		t.Fatal(err)
	}
	failed := ""
	for _, e := range view.Effects {
		if e.State == rotationStepFailed {
			failed = e.Detail
		}
	}
	if !strings.Contains(failed, "sets a new password for wp_user under Databases") {
		t.Fatalf("guidance: %q", failed)
	}
	// The new owner sets that user's password: the step is theirs now.
	claimed, err := f.panel.claimReplacedCredential(context.Background(), rotationEffectDatabaseUser, "9921:wp_user")
	if err != nil || !claimed {
		t.Fatalf("claim %t %v", claimed, err)
	}
	if f.opState(t) != "completed" || f.steps(t)["database_user 9921:wp_user"] != rotationStepReplaced {
		t.Fatalf("after the owner's change: %s %v", f.opState(t), f.steps(t))
	}
}

func TestTransferRotationNewOwnerSetsADatabasePassword(t *testing.T) {
	f := newRotationFixture(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/database-users/", func(w http.ResponseWriter, r *http.Request) {
		f.panel.handleDatabaseSubroute(w, r, "/api/v1/database-users/")
	})
	handler := f.panel.requireAuth(mux)
	call := func(userID int, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/database-users/9941/password", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: f.tokens[userID]})
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}
	f.transfer(t, "rotate-0006", "")
	// The previous owner no longer reaches the user.
	if got := call(authzMatrixCustomerID, `{}`); got.Code != http.StatusNotFound {
		t.Fatalf("previous owner: %d %s", got.Code, got.Body.String())
	}
	got := call(9706, `{}`)
	if got.Code != http.StatusOK {
		t.Fatalf("new owner: %d %s", got.Code, got.Body.String())
	}
	var answer map[string]any
	_ = json.Unmarshal(got.Body.Bytes(), &answer)
	shown, _ := answer["password"].(string)
	var stored string
	_ = f.panel.db.GetDB().QueryRow(`SELECT password FROM database_users WHERE id = 9941`).Scan(&stored)
	if plain, _ := f.box.Decrypt(stored); shown == "" || plain != shown {
		t.Fatal("the password shown once is not the one stored")
	}
	// Before the worker ran, the owner's change took the step over.
	if f.steps(t)["database_user 9921:cust_user"] != rotationStepReplaced {
		t.Fatalf("steps %v", f.steps(t))
	}
	f.run(t)
	if f.native.count("db cust_user") != 1 {
		t.Fatalf("the worker overwrote the owner's password: %v", f.native.calls)
	}
	if got := call(9706, `{"password":"short"}`); got.Code != http.StatusBadRequest {
		t.Fatalf("short password: %d", got.Code)
	}
}
