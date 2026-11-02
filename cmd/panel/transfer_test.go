package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Subscription transfer (D-042 section 8, K3). The fixture is the rights
// one (admin 9701, reseller 9702 with customer 9703 / subscription 9802 /
// domain 9902, the administrator's customer 9704 / subscription 9803, the
// suspended customer 9705, the team member 9710 of 9703) plus a second
// customer of 9702 (9706), a second reseller 9707 with customer 9708
// (subscription 9804) and an administrator workspace (9805).

type transferFixture struct {
	rightsFixture
	handler http.Handler
}

func newTransferFixture(t *testing.T) transferFixture {
	t.Helper()
	base := newRightsFixture(t)
	base.exec(t, `INSERT INTO users (id, username, password_hash, email, role, parent_id, status) VALUES
		(9706, 'second-customer', 'x', 'second@example.test', 'customer', 9702, 'active'),
		(9707, 'other-reseller', 'x', 'other-reseller@example.test', 'reseller', NULL, 'active'),
		(9708, 'other-customer', 'x', 'other-customer@example.test', 'customer', 9707, 'active')`)
	base.exec(t, `INSERT INTO subscriptions (id, owner_id, name, status) VALUES
		(9804, 9708, 'other subscription', 'active'),
		(9805, 9701, 'Admin Subscription', 'active'),
		(9806, 9706, 'second subscription', 'active')`)
	base.exec(t, `INSERT INTO domains (id, subscription_id, name, status) VALUES (9906, 9804, 'other.matrix.test', 'active')`)
	for _, id := range []int{9706, 9707, 9708} {
		token, err := base.panel.sessions.Create(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		base.tokens[id] = token
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/users", base.panel.handleUsers)
	mux.HandleFunc("/api/v1/users/", base.panel.handleUserByID)
	mux.HandleFunc("/api/v1/plans", base.panel.handlePlans)
	mux.HandleFunc("/api/v1/subscriptions/", base.panel.handleSubscriptionEntitlements)
	mux.HandleFunc(transferRequestsPath, base.panel.handleTransferRequests)
	mux.HandleFunc(transferRequestsPath+"/", base.panel.handleTransferRequests)
	return transferFixture{rightsFixture: base, handler: base.panel.requireAuth(mux)}
}

func (f transferFixture) do(t *testing.T, userID int, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	return f.doToken(t, f.tokens[userID], method, path, body)
}

func (f transferFixture) doToken(t *testing.T, token, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var request *http.Request
	if body == "" {
		request = httptest.NewRequest(method, path, nil)
	} else {
		request = httptest.NewRequest(method, path, strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
	}
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	recorder := httptest.NewRecorder()
	f.handler.ServeHTTP(recorder, request)
	return recorder
}

func (f transferFixture) owner(t *testing.T, subID int) int {
	t.Helper()
	return f.count(t, `SELECT owner_id FROM subscriptions WHERE id = ?`, subID)
}

func transferPreviewOf(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	body := decodeBody(t, recorder)
	preview, ok := body["preview"].(map[string]any)
	if !ok {
		t.Fatalf("no preview in %s", recorder.Body.String())
	}
	return preview
}

func TestTransferWhoMayTransfer(t *testing.T) {
	f := newTransferFixture(t)
	preview := func(userID, subID, to int) *httptest.ResponseRecorder {
		return f.do(t, userID, http.MethodPost, "/api/v1/subscriptions/"+itoa(subID)+"/transfer/preview", `{"to_user_id":`+itoa(to)+`}`)
	}
	// A customer only asks.
	wantStatus(t, preview(authzMatrixCustomerID, 9802, 9706), http.StatusForbidden, errCodeTransferRequestOnly)
	wantStatus(t, f.do(t, authzMatrixCustomerID, http.MethodPost, "/api/v1/subscriptions/9802/transfer",
		`{"to_user_id":9706,"from_user_id":9703,"request_id":"cust-0001","confirm":true}`), http.StatusForbidden, errCodeTransferRequestOnly)
	// A reseller between its own customers only.
	ok := preview(authzMatrixResellerID, 9802, 9706)
	wantStatus(t, ok, http.StatusOK, "")
	if decodeBody(t, ok)["can_transfer"] != true {
		t.Fatalf("own customers: %s", ok.Body.String())
	}
	for _, to := range []int{9704, 9708, 9707, 99999} {
		wantStatus(t, preview(authzMatrixResellerID, 9802, to), http.StatusConflict, errCodeTransferNeedsAdmin)
	}
	// Its own subscription is not between customers.
	wantStatus(t, preview(authzMatrixResellerID, 9801, 9706), http.StatusConflict, errCodeTransferNeedsAdmin)
	// Not its customer's subscription: not found.
	wantStatus(t, preview(authzMatrixResellerID, 9803, 9706), http.StatusNotFound, "")
	wantStatus(t, preview(9707, 9802, 9708), http.StatusNotFound, "")
	// The administrator: anyone to anyone, across resellers.
	wantStatus(t, preview(authzMatrixAdminID, 9802, 9708), http.StatusOK, "")
	wantStatus(t, preview(authzMatrixAdminID, 9803, 9707), http.StatusOK, "")
	// Receivers that cannot own it.
	wantStatus(t, preview(authzMatrixAdminID, 9802, authzMatrixAdditionalUserID), http.StatusConflict, errCodeTransferReceiverInvalid)
	wantStatus(t, preview(authzMatrixAdminID, 9802, authzMatrixSuspendedID), http.StatusConflict, errCodeTransferReceiverInvalid)
	wantStatus(t, preview(authzMatrixAdminID, 9802, authzMatrixAdminID), http.StatusConflict, errCodeTransferReceiverInvalid)
	wantStatus(t, preview(authzMatrixAdminID, 9802, 9703), http.StatusConflict, errCodeTransferSameOwner)
	wantStatus(t, preview(authzMatrixAdminID, 9805, 9704), http.StatusConflict, errCodeTransferAdminWorkspace)
	// A team member never reaches the transfer routes.
	wantStatus(t, f.doToken(t, f.memberToken, http.MethodPost, "/api/v1/subscriptions/9802/transfer/preview", `{"to_user_id":9706}`), http.StatusForbidden, "")
	wantStatus(t, f.doToken(t, f.memberToken, http.MethodGet, transferRequestsPath, ""), http.StatusForbidden, "")
	// The receiver list follows the same rule.
	list := f.do(t, authzMatrixResellerID, http.MethodGet, "/api/v1/subscriptions/9802/transfer/receivers", "")
	wantStatus(t, list, http.StatusOK, "")
	if !strings.Contains(list.Body.String(), `"second-customer"`) || strings.Contains(list.Body.String(), `"other-customer"`) ||
		strings.Contains(list.Body.String(), `"matrix-customer"`) {
		t.Fatalf("reseller receivers = %s", list.Body.String())
	}
	all := f.do(t, authzMatrixAdminID, http.MethodGet, "/api/v1/subscriptions/9802/transfer/receivers", "")
	if !strings.Contains(all.Body.String(), `"other-customer"`) || strings.Contains(all.Body.String(), `"matrix-suspended"`) ||
		strings.Contains(all.Body.String(), `"matrix-member"`) || strings.Contains(all.Body.String(), `"matrix-admin"`) {
		t.Fatalf("administrator receivers = %s", all.Body.String())
	}
	wantStatus(t, f.do(t, authzMatrixCustomerID, http.MethodGet, "/api/v1/subscriptions/9802/transfer/receivers", ""), http.StatusForbidden, errCodeTransferRequestOnly)
	if f.owner(t, 9802) != 9703 {
		t.Fatal("a preview changed the owner")
	}
}

func TestTransferConfirmIdempotencyAndAudit(t *testing.T) {
	f := newTransferFixture(t)
	path := "/api/v1/subscriptions/9802/transfer"
	wantStatus(t, f.do(t, authzMatrixAdminID, http.MethodPost, path, `{"to_user_id":9708,"from_user_id":9703,"confirm":true}`), http.StatusBadRequest, errCodeTransferRequestID)
	unconfirmed := f.do(t, authzMatrixAdminID, http.MethodPost, path, `{"to_user_id":9708,"from_user_id":9703,"request_id":"move-0001"}`)
	wantStatus(t, unconfirmed, http.StatusConflict, errCodeTransferConfirm)
	if p := transferPreviewOf(t, unconfirmed); p["host_changes"] != "none" || p["from"].(map[string]any)["username"] != "matrix-customer" {
		t.Fatalf("preview = %v", p)
	}
	wantStatus(t, f.do(t, authzMatrixAdminID, http.MethodPost, path, `{"to_user_id":9708,"from_user_id":9704,"request_id":"move-0001","confirm":true}`), http.StatusConflict, errCodeTransferStale)
	if f.owner(t, 9802) != 9703 || f.count(t, `SELECT COUNT(*) FROM subscription_transfers`) != 0 {
		t.Fatal("a refused call changed something")
	}
	done := f.do(t, authzMatrixAdminID, http.MethodPost, path, `{"to_user_id":9708,"from_user_id":9703,"request_id":"move-0001","confirm":true}`)
	wantStatus(t, done, http.StatusOK, "")
	if f.owner(t, 9802) != 9708 {
		t.Fatalf("owner = %d", f.owner(t, 9802))
	}
	if n := f.count(t, `SELECT COUNT(*) FROM subscription_transfers WHERE request_id = 'move-0001' AND status = 'completed'
		AND from_owner_id = 9703 AND to_owner_id = 9708 AND requested_by = 9701 AND before_image LIKE '%"owner_id":9703%'`); n != 1 {
		t.Fatalf("transfer rows = %d", n)
	}
	if n := f.count(t, `SELECT COUNT(*) FROM audit_logs WHERE action = 'subscription.transfer:customer subscription'
		AND user_id = 9701 AND acting_as IS NULL AND details LIKE '%"to":"other-customer"%'`); n != 1 {
		t.Fatalf("audit rows = %d", n)
	}
	// The same call again answers with the stored result and changes nothing.
	again := f.do(t, authzMatrixAdminID, http.MethodPost, path, `{"to_user_id":9708,"from_user_id":9703,"request_id":"move-0001","confirm":true}`)
	wantStatus(t, again, http.StatusOK, "")
	if decodeBody(t, again)["replayed"] != true || f.count(t, `SELECT COUNT(*) FROM subscription_transfers`) != 1 ||
		f.count(t, `SELECT COUNT(*) FROM audit_logs WHERE action LIKE 'subscription.transfer:%'`) != 1 {
		t.Fatalf("replay = %s", again.Body.String())
	}
	// The same id for another change is refused.
	wantStatus(t, f.do(t, authzMatrixAdminID, http.MethodPost, path, `{"to_user_id":9704,"from_user_id":9708,"request_id":"move-0001","confirm":true}`), http.StatusConflict, errCodeTransferRequestIDReused)
	// The old owner lost it at once; the new owner has it.
	wantStatus(t, f.do(t, authzMatrixCustomerID, http.MethodGet, "/api/v1/subscriptions/9802/rights", ""), http.StatusNotFound, "")
	wantStatus(t, f.do(t, 9708, http.MethodGet, "/api/v1/subscriptions/9802/rights", ""), http.StatusOK, "")
	wantStatus(t, f.do(t, 9707, http.MethodGet, "/api/v1/subscriptions/9802/rights", ""), http.StatusOK, "")
	wantStatus(t, f.do(t, authzMatrixResellerID, http.MethodGet, "/api/v1/subscriptions/9802/rights", ""), http.StatusNotFound, "")
}

func TestTransferEndsLogInAsSessionsOfTheOldOwnerForThatSubscription(t *testing.T) {
	f := newTransferFixture(t)
	token, err := f.panel.sessions.CreateImpersonation(context.Background(), authzMatrixCustomerID, authzMatrixResellerID)
	if err != nil {
		t.Fatal(err)
	}
	wantStatus(t, f.doToken(t, token, http.MethodGet, "/api/v1/subscriptions/9802/rights", ""), http.StatusOK, "")
	wantStatus(t, f.do(t, authzMatrixResellerID, http.MethodPost, "/api/v1/subscriptions/9802/transfer",
		`{"to_user_id":9706,"from_user_id":9703,"request_id":"move-login-as","confirm":true}`), http.StatusOK, "")
	wantStatus(t, f.doToken(t, token, http.MethodGet, "/api/v1/subscriptions/9802/rights", ""), http.StatusNotFound, "")
}

func TestTransferRemovesTheOldOwnersAdditionalUserGrants(t *testing.T) {
	f := newTransferFixture(t)
	f.exec(t, `INSERT INTO additional_user_subscription_permissions (user_id, subscription_id, capability, mode) VALUES (9710, 9802, 'files', 'manage'), (9710, 9802, 'mail', 'view')`)
	f.exec(t, `INSERT INTO additional_user_domain_permissions (user_id, domain_id, capability, mode) VALUES (9710, 9902, 'dns', 'manage')`)
	preview := f.do(t, authzMatrixResellerID, http.MethodPost, "/api/v1/subscriptions/9802/transfer/preview", `{"to_user_id":9706}`)
	wantStatus(t, preview, http.StatusOK, "")
	grants := transferPreviewOf(t, preview)["grants_removed"].([]any)
	if len(grants) != 1 {
		t.Fatalf("grants = %v", grants)
	}
	line := grants[0].(map[string]any)
	if line["username"] != "matrix-member" || line["subscription_grants"] != float64(2) || line["domain_grants"] != float64(1) {
		t.Fatalf("grant line = %v", line)
	}
	if !strings.Contains(transferPreviewOf(t, preview)["message"].(string), "additional user") {
		t.Fatal("the message does not say who loses access")
	}
	wantStatus(t, f.do(t, authzMatrixResellerID, http.MethodPost, "/api/v1/subscriptions/9802/transfer",
		`{"to_user_id":9706,"from_user_id":9703,"request_id":"move-grants","confirm":true}`), http.StatusOK, "")
	if n := f.count(t, `SELECT (SELECT COUNT(*) FROM additional_user_subscription_permissions WHERE subscription_id = 9802)
		+ (SELECT COUNT(*) FROM additional_user_domain_permissions WHERE domain_id = 9902)`); n != 0 {
		t.Fatalf("grants left = %d", n)
	}
	var before string
	if err := f.panel.db.GetDB().QueryRow(`SELECT before_image FROM subscription_transfers WHERE request_id = 'move-grants'`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	var img transferBeforeImage
	if err := json.Unmarshal([]byte(before), &img); err != nil || len(img.Grants) != 3 || img.OwnerID != 9703 {
		t.Fatalf("before-image = %s (%v)", before, err)
	}
}

func TestTransferChecksTheReceivingResellersRightsAndPool(t *testing.T) {
	f := newTransferFixture(t)
	// The receiving reseller has no SSH and only Node.js 20.
	f.setReseller(t, 9707, newRightsSet(false, false, append(everyPermissionBut(rightSSH), "component:node:20", "component:php-fpm",
		"component:mariadb", "component:postgresql", "integration:acme:letsencrypt", "integration:acme:zerossl")...))
	// A Custom subscription that names SSH and Node.js 22: refused, with the
	// items and the ways out.
	f.setCustom(t, 9803, newRightsSet(false, false, "permission:mail", "permission:ssh", "component:node:22"))
	preview := f.do(t, authzMatrixAdminID, http.MethodPost, "/api/v1/subscriptions/9803/transfer/preview", `{"to_user_id":9708}`)
	wantStatus(t, preview, http.StatusOK, "")
	body := decodeBody(t, preview)
	refusal, _ := body["refusal"].(map[string]any)
	if body["can_transfer"] != false || refusal["code"] != errCodeTransferRightsOutside ||
		!strings.Contains(preview.Body.String(), `"permission:ssh"`) || !strings.Contains(preview.Body.String(), `"component:node:22"`) ||
		len(refusal["ways_out"].([]any)) != 3 {
		t.Fatalf("preview = %s", preview.Body.String())
	}
	confirm := f.do(t, authzMatrixAdminID, http.MethodPost, "/api/v1/subscriptions/9803/transfer",
		`{"to_user_id":9708,"from_user_id":9704,"request_id":"move-outside","confirm":true}`)
	wantStatus(t, confirm, http.StatusConflict, errCodeTransferRightsOutside)
	if f.owner(t, 9803) != 9704 {
		t.Fatal("a refused transfer changed the owner")
	}
	// Narrowed first (what it uses fits), it moves.
	f.setCustom(t, 9803, newRightsSet(false, false, "permission:mail", "component:node:20"))
	wantStatus(t, f.do(t, authzMatrixAdminID, http.MethodPost, "/api/v1/subscriptions/9803/transfer/preview", `{"to_user_id":9708}`), http.StatusOK, "")

	// An Unlimited subscription stays Unlimited and is capped by the
	// reseller's set: the preview shows what closes.
	f.exec(t, `UPDATE subscriptions SET max_domains = 0, max_databases = 0, max_email_accounts = 0, disk_quota_mb = 0 WHERE id = 9803`)
	f.exec(t, `DELETE FROM rights_sets WHERE owner_kind = 'subscription' AND owner_id = 9803`)
	capped := f.do(t, authzMatrixAdminID, http.MethodPost, "/api/v1/subscriptions/9803/transfer/preview", `{"to_user_id":9708}`)
	wantStatus(t, capped, http.StatusOK, "")
	cp := transferPreviewOf(t, capped)
	if decodeBody(t, capped)["can_transfer"] != true || cp["rights_from"] != "unlimited" || cp["rights_to"] != "unlimited" ||
		!strings.Contains(capped.Body.String(), `"capped":["component:node:22","permission:ssh"]`) {
		t.Fatalf("capped preview = %s", capped.Body.String())
	}

	// The pool: real use of the tree plus what the subscription brings.
	wantStatus(t, f.as(t, authzMatrixAdminID, http.MethodPut, "/api/v1/users/9707/pool",
		`{"pool":{"max_customers":0,"max_domains":1,"max_databases":0,"max_email_accounts":0,"disk_quota_mb":0}}`), http.StatusOK, "")
	full := f.do(t, authzMatrixAdminID, http.MethodPost, "/api/v1/subscriptions/9803/transfer",
		`{"to_user_id":9708,"from_user_id":9704,"request_id":"move-pool","confirm":true}`)
	wantStatus(t, full, http.StatusConflict, errCodeTransferPoolExceeded)
	if !strings.Contains(full.Body.String(), "domains: 1 in use + 1 brought = more than the pool of 1") {
		t.Fatalf("pool refusal = %s", full.Body.String())
	}
	wantStatus(t, f.as(t, authzMatrixAdminID, http.MethodPut, "/api/v1/users/9707/pool",
		`{"pool":{"max_customers":0,"max_domains":2,"max_databases":0,"max_email_accounts":0,"disk_quota_mb":0}}`), http.StatusOK, "")
	wantStatus(t, f.do(t, authzMatrixAdminID, http.MethodPost, "/api/v1/subscriptions/9803/transfer",
		`{"to_user_id":9708,"from_user_id":9704,"request_id":"move-pool-2","confirm":true}`), http.StatusOK, "")
	if f.owner(t, 9803) != 9708 {
		t.Fatal("a fitting transfer did not move")
	}
	// Within one reseller's tree the pool and rights do not change: no check.
	f.setReseller(t, 9702, newRightsSet(false, false, "permission:mail"))
	f.setCustom(t, 9802, newRightsSet(false, false, "permission:mail", "permission:ssh"))
	same := f.do(t, authzMatrixResellerID, http.MethodPost, "/api/v1/subscriptions/9802/transfer/preview", `{"to_user_id":9706}`)
	if decodeBody(t, same)["can_transfer"] != true || transferPreviewOf(t, same)["same_tree"] != true {
		t.Fatalf("same tree = %s", same.Body.String())
	}
}

func TestTransferConvertsAResellersOwnPlanOutsideItsTree(t *testing.T) {
	f := newTransferFixture(t)
	planID := createPlan(t, f.accountsFixture, authzMatrixResellerID,
		`{"name":"Reseller basic","max_domains":4,"max_databases":2,"max_email_accounts":3,"disk_quota_mb":100,"bandwidth_quota_mb":0,"rights":{"all_permissions":false,"all_software":false,"permissions":["mail","dns_edit"],"software":["component:php-fpm:8.3"]}}`)
	wantStatus(t, f.as(t, authzMatrixResellerID, http.MethodPut, "/api/v1/users/9703/plan", `{"plan_id":`+itoa(planID)+`}`), http.StatusOK, "")
	// Inside the reseller's tree the plan follows.
	inside := f.do(t, authzMatrixResellerID, http.MethodPost, "/api/v1/subscriptions/9802/transfer/preview", `{"to_user_id":9706}`)
	if p := transferPreviewOf(t, inside); p["plan_converted"] != false || p["rights_to"] != "plan:"+itoa(planID) {
		t.Fatalf("inside = %s", inside.Body.String())
	}
	// Outside it the subscription becomes Custom with the same items.
	outside := f.do(t, authzMatrixAdminID, http.MethodPost, "/api/v1/subscriptions/9802/transfer/preview", `{"to_user_id":9704}`)
	p := transferPreviewOf(t, outside)
	if p["plan_converted"] != true || p["rights_to"] != "custom" || !strings.Contains(p["message"].(string), "becomes Custom") {
		t.Fatalf("outside = %s", outside.Body.String())
	}
	wantStatus(t, f.do(t, authzMatrixAdminID, http.MethodPost, "/api/v1/subscriptions/9802/transfer",
		`{"to_user_id":9704,"from_user_id":9703,"request_id":"move-plan","confirm":true}`), http.StatusOK, "")
	if n := f.count(t, `SELECT COUNT(*) FROM subscriptions WHERE id = 9802 AND plan_id IS NULL AND max_domains = 4 AND disk_quota_mb = 100`); n != 1 {
		t.Fatal("the plan's numbers were not kept as Custom")
	}
	state, err := subscriptionRightsState(context.Background(), f.panel.db.GetDB(), 9802)
	if err != nil || state.State != rightsStateCustom || !state.Set.permits(rightDNSEdit) || state.Set.permits(rightSSH) ||
		!state.Set.offers("component:php-fpm:8.3") {
		t.Fatalf("state after = %+v %v", state, err)
	}
}

func TestTransferRequestApproveRejectCancel(t *testing.T) {
	f := newTransferFixture(t)
	ask := func(userID, subID int, requestID, extra string) *httptest.ResponseRecorder {
		return f.do(t, userID, http.MethodPost, "/api/v1/subscriptions/"+itoa(subID)+"/transfer-request",
			`{"request_id":"`+requestID+`"`+extra+`}`)
	}
	// Only the owning customer asks.
	wantStatus(t, ask(authzMatrixResellerID, 9802, "ask-0000", ""), http.StatusForbidden, errCodeTransferNotAllowed)
	wantStatus(t, ask(authzMatrixOutsiderID, 9802, "ask-0000", ""), http.StatusNotFound, "")
	created := ask(authzMatrixCustomerID, 9802, "ask-0001", `,"to_username":"second-customer","note":"selling the shop"`)
	wantStatus(t, created, http.StatusCreated, "")
	request := decodeBody(t, created)["request"].(map[string]any)
	id := int(request["id"].(float64))
	if request["approver"] != "reseller" || request["status"] != "requested" {
		t.Fatalf("request = %v", request)
	}
	again := ask(authzMatrixCustomerID, 9802, "ask-0001", `,"to_username":"second-customer","note":"selling the shop"`)
	if again.Code != http.StatusOK || decodeBody(t, again)["replayed"] != true {
		t.Fatalf("replay = %d %s", again.Code, again.Body.String())
	}
	wantStatus(t, ask(authzMatrixCustomerID, 9802, "ask-0002", ""), http.StatusConflict, errCodeTransferRequestOpen)
	if n := f.count(t, `SELECT COUNT(*) FROM audit_logs WHERE action = 'subscription.transfer_request:customer subscription' AND user_id = 9703`); n != 1 {
		t.Fatalf("request audit rows = %d", n)
	}
	// Who sees and decides it.
	reqPath := transferRequestsPath + "/" + itoa(id)
	list := f.do(t, authzMatrixResellerID, http.MethodGet, transferRequestsPath, "")
	if decodeBody(t, list)["waiting"] != float64(1) || !strings.Contains(list.Body.String(), `"selling the shop"`) {
		t.Fatalf("reseller list = %s", list.Body.String())
	}
	if strings.Contains(f.do(t, 9707, http.MethodGet, transferRequestsPath, "").Body.String(), `"selling the shop"`) {
		t.Fatal("another reseller sees the request")
	}
	wantStatus(t, f.do(t, 9707, http.MethodPost, reqPath+"/approve", `{"to_user_id":9708,"from_user_id":9703,"request_id":"dec-0000","confirm":true}`), http.StatusNotFound, "")
	wantStatus(t, f.do(t, authzMatrixCustomerID, http.MethodPost, reqPath+"/approve", `{"to_user_id":9706,"from_user_id":9703,"request_id":"dec-0000","confirm":true}`), http.StatusForbidden, errCodeTransferNotAllowed)
	// The reseller may not approve it to someone outside its customers.
	wantStatus(t, f.do(t, authzMatrixResellerID, http.MethodPost, reqPath+"/approve", `{"to_user_id":9708,"from_user_id":9703,"request_id":"dec-0000","confirm":true}`), http.StatusConflict, errCodeTransferNeedsAdmin)
	preview := f.do(t, authzMatrixResellerID, http.MethodPost, reqPath+"/preview", `{"to_user_id":9706}`)
	wantStatus(t, preview, http.StatusOK, "")
	wantStatus(t, f.do(t, authzMatrixResellerID, http.MethodPost, reqPath+"/approve", `{"to_user_id":9706,"from_user_id":9703,"request_id":"dec-0001"}`), http.StatusConflict, errCodeTransferConfirm)
	approved := f.do(t, authzMatrixResellerID, http.MethodPost, reqPath+"/approve", `{"to_user_id":9706,"from_user_id":9703,"request_id":"dec-0001","confirm":true}`)
	wantStatus(t, approved, http.StatusOK, "")
	if f.owner(t, 9802) != 9706 || f.count(t, `SELECT COUNT(*) FROM subscription_transfers WHERE id = ? AND status = 'completed' AND decision_request_id = 'dec-0001' AND decided_by = 9702`, id) != 1 {
		t.Fatal("the approval did not complete the request")
	}
	if n := f.count(t, `SELECT COUNT(*) FROM audit_logs WHERE action IN ('subscription.transfer_request_approve:customer subscription', 'subscription.transfer:customer subscription') AND user_id = 9702`); n != 2 {
		t.Fatalf("approval audit rows = %d", n)
	}
	replayed := f.do(t, authzMatrixResellerID, http.MethodPost, reqPath+"/approve", `{"to_user_id":9706,"from_user_id":9703,"request_id":"dec-0001","confirm":true}`)
	if replayed.Code != http.StatusOK || decodeBody(t, replayed)["replayed"] != true {
		t.Fatalf("approval replay = %d %s", replayed.Code, replayed.Body.String())
	}
	wantStatus(t, f.do(t, authzMatrixResellerID, http.MethodPost, reqPath+"/reject", `{"request_id":"dec-0002"}`), http.StatusConflict, errCodeTransferRequestClosed)
	// The receiver and the requester are told in the panel until they look.
	unseen := decodeBody(t, f.do(t, 9706, http.MethodGet, transferRequestsPath, ""))["unseen"].(map[string]any)
	if unseen["received"] != float64(1) {
		t.Fatalf("receiver unseen = %v", unseen)
	}
	if decodeBody(t, f.do(t, authzMatrixCustomerID, http.MethodGet, transferRequestsPath, ""))["unseen"].(map[string]any)["decided"] != float64(1) {
		t.Fatal("the requester is not told")
	}
	wantStatus(t, f.do(t, 9706, http.MethodPost, reqPath+"/seen", ""), http.StatusOK, "")
	if decodeBody(t, f.do(t, 9706, http.MethodGet, transferRequestsPath, ""))["unseen"].(map[string]any)["received"] != float64(0) {
		t.Fatal("seen did not clear")
	}

	// A customer without a reseller: the administrator decides and rejects.
	adminAsk := ask(authzMatrixOutsiderID, 9803, "ask-0100", `,"note":"please move"`)
	wantStatus(t, adminAsk, http.StatusCreated, "")
	adminID := int(decodeBody(t, adminAsk)["request"].(map[string]any)["id"].(float64))
	if decodeBody(t, adminAsk)["request"].(map[string]any)["approver"] != "administrator" {
		t.Fatal("the approver is not the administrator")
	}
	adminPath := transferRequestsPath + "/" + itoa(adminID)
	wantStatus(t, f.do(t, authzMatrixResellerID, http.MethodPost, adminPath+"/reject", `{"request_id":"dec-0100"}`), http.StatusNotFound, "")
	rejected := f.do(t, authzMatrixAdminID, http.MethodPost, adminPath+"/reject", `{"request_id":"dec-0100","reason":"not now"}`)
	wantStatus(t, rejected, http.StatusOK, "")
	if f.owner(t, 9803) != 9704 || f.count(t, `SELECT COUNT(*) FROM subscription_transfers WHERE id = ? AND status = 'rejected' AND decision_reason = 'not now'`, adminID) != 1 ||
		f.count(t, `SELECT COUNT(*) FROM audit_logs WHERE action = 'subscription.transfer_request_reject:outsider subscription' AND user_id = 9701`) != 1 {
		t.Fatal("the rejection was not recorded")
	}
	// The requester withdraws; a decision after that is refused.
	withdraw := ask(authzMatrixOutsiderID, 9803, "ask-0101", "")
	wantStatus(t, withdraw, http.StatusCreated, "")
	withdrawPath := transferRequestsPath + "/" + itoa(int(decodeBody(t, withdraw)["request"].(map[string]any)["id"].(float64)))
	wantStatus(t, f.do(t, authzMatrixAdminID, http.MethodPost, withdrawPath+"/cancel", ""), http.StatusForbidden, errCodeTransferNotAllowed)
	wantStatus(t, f.do(t, authzMatrixOutsiderID, http.MethodPost, withdrawPath+"/cancel", ""), http.StatusOK, "")
	wantStatus(t, f.do(t, authzMatrixAdminID, http.MethodPost, withdrawPath+"/approve", `{"to_user_id":9708,"from_user_id":9704,"request_id":"dec-0101","confirm":true}`), http.StatusConflict, errCodeTransferRequestClosed)
	if n := f.count(t, `SELECT COUNT(*) FROM audit_logs WHERE action = 'subscription.transfer_request_cancel:outsider subscription'`); n != 1 {
		t.Fatalf("cancel audit rows = %d", n)
	}
}

func TestTransferRequestDuringLogInAsNamesTheRealPerson(t *testing.T) {
	f := newTransferFixture(t)
	token, err := f.panel.sessions.CreateImpersonation(context.Background(), authzMatrixCustomerID, authzMatrixAdminID)
	if err != nil {
		t.Fatal(err)
	}
	wantStatus(t, f.doToken(t, token, http.MethodPost, "/api/v1/subscriptions/9802/transfer-request", `{"request_id":"ask-login-as"}`), http.StatusCreated, "")
	if n := f.count(t, `SELECT COUNT(*) FROM subscription_transfers WHERE request_id = 'ask-login-as' AND requested_by = 9701 AND requested_acting_as = 9703 AND from_owner_id = 9703`); n != 1 {
		t.Fatalf("request rows = %d", n)
	}
	if n := f.count(t, `SELECT COUNT(*) FROM audit_logs WHERE action LIKE 'subscription.transfer_request:%' AND user_id = 9701 AND acting_as = 9703`); n != 1 {
		t.Fatalf("audit rows = %d", n)
	}
}
