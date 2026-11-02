package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
)

// Transfer APIs (D-042 section 8, K3). Preview first, then a confirmed call
// with a request id; the same request id with the same content answers with
// the stored result and changes nothing.
//
//	GET  /api/v1/subscriptions/{id}/transfer/receivers   who the caller may choose
//	POST /api/v1/subscriptions/{id}/transfer/preview     {to_user_id}
//	POST /api/v1/subscriptions/{id}/transfer             {to_user_id, from_user_id, request_id, confirm}
//	POST /api/v1/subscriptions/{id}/transfer-request     customer: {request_id, to_username, note}
//	GET  /api/v1/transfer-requests                       requests, history and what the caller has not seen
//	POST /api/v1/transfer-requests/{id}/preview          approver: {to_user_id}
//	POST /api/v1/transfer-requests/{id}/approve          approver: {to_user_id, from_user_id, request_id, confirm}
//	POST /api/v1/transfer-requests/{id}/reject           approver: {request_id, reason}
//	POST /api/v1/transfer-requests/{id}/cancel           the requesting customer
//	POST /api/v1/transfer-requests/{id}/seen             the receiver or the requester
//
// Devir API'leri: önce önizleme, sonra istek kimliğiyle onaylı çağrı.

const transferRequestsPath = "/api/v1/transfer-requests"

type transferBody struct {
	ToUserID   int    `json:"to_user_id"`
	FromUserID int    `json:"from_user_id,omitempty"`
	RequestID  string `json:"request_id,omitempty"`
	Confirm    bool   `json:"confirm,omitempty"`
}

// parseSubscriptionTransferPath: /api/v1/subscriptions/{id}/transfer[/preview|/receivers]
// and /api/v1/subscriptions/{id}/transfer-request.
func parseSubscriptionTransferPath(path string) (int, string, bool) {
	rest, ok := strings.CutPrefix(path, "/api/v1/subscriptions/")
	if !ok {
		return 0, "", false
	}
	parts := strings.Split(rest, "/")
	if len(parts) < 2 {
		return 0, "", false
	}
	id, valid := strictPositiveID(parts[0])
	if !valid {
		return 0, "", false
	}
	switch {
	case len(parts) == 2 && parts[1] == "transfer":
		return id, "transfer", true
	case len(parts) == 2 && parts[1] == "transfer-request":
		return id, "request", true
	case len(parts) == 3 && parts[1] == "transfer" && (parts[2] == "preview" || parts[2] == "receivers"):
		return id, parts[2], true
	}
	return 0, "", false
}

func transferAccountCaller(c *Caller) bool {
	return c.hasAccountRole(roleAdmin) || c.hasAccountRole(roleReseller) || c.hasAccountRole(roleCustomer)
}

func writeTransferRefusal(w http.ResponseWriter, refusal *transferRefusal, preview *transferPreview) {
	status := refusal.Status
	if status == 0 {
		status = http.StatusConflict
	}
	body := map[string]any{"error": refusal.Message, "code": refusal.Code}
	if len(refusal.Details) > 0 {
		body["details"] = refusal.Details
	}
	if len(refusal.WaysOut) > 0 {
		body["ways_out"] = refusal.WaysOut
	}
	if preview != nil {
		body["preview"] = preview
	}
	writeRightsJSON(w, status, body)
}

func (p *Panel) handleSubscriptionTransfer(w http.ResponseWriter, r *http.Request, subID int, action string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "private, no-store")
	c := currentCaller(r)
	if !transferAccountCaller(c) {
		writeCodedError(w, http.StatusForbidden, errCodeTransferNotAllowed, "account access required", "")
		return
	}
	if err := p.canAccessSubscription(r.Context(), c, subID); err != nil {
		if errors.Is(err, errNotFound) {
			writeClientError(w, http.StatusNotFound, "subscription not found")
		} else {
			writeServerError(w, err)
		}
		return
	}
	switch action {
	case "receivers":
		if r.Method != http.MethodGet {
			rejectRouteMethod(w, []string{http.MethodGet})
			return
		}
		p.writeTransferReceivers(w, r, c, subID)
	case "preview":
		if r.Method != http.MethodPost {
			rejectRouteMethod(w, []string{http.MethodPost})
			return
		}
		var body transferBody
		if err := decodeStrictJSON(w, r, &body); err != nil || body.ToUserID <= 0 {
			writeClientError(w, http.StatusBadRequest, "to_user_id is required")
			return
		}
		p.writeTransferPreview(w, r, c, subID, body.ToUserID)
	case "transfer":
		if r.Method != http.MethodPost {
			rejectRouteMethod(w, []string{http.MethodPost})
			return
		}
		var body transferBody
		if err := decodeStrictJSON(w, r, &body); err != nil || body.ToUserID <= 0 {
			writeClientError(w, http.StatusBadRequest, "to_user_id is required")
			return
		}
		p.performTransfer(w, r, c, subID, body, nil)
	case "request":
		if r.Method != http.MethodPost {
			rejectRouteMethod(w, []string{http.MethodPost})
			return
		}
		p.createTransferRequest(w, r, c, subID)
	}
}

// writeTransferReceivers lists the accounts the caller may transfer to.
func (p *Panel) writeTransferReceivers(w http.ResponseWriter, r *http.Request, c *Caller, subID int) {
	ctx := r.Context()
	db := p.db.GetDB()
	var ownerID int
	if err := db.QueryRowContext(ctx, `SELECT owner_id FROM subscriptions WHERE id = ?`, subID).Scan(&ownerID); err != nil {
		writeServerError(w, err)
		return
	}
	query := `
		SELECT u.id, u.username, u.role, COALESCE(p.username, '')
		FROM users u LEFT JOIN users p ON p.id = u.parent_id AND p.role = 'reseller'
		WHERE u.id <> ? AND COALESCE(NULLIF(u.account_type, ''), 'account') = 'account'
		  AND COALESCE(u.status, 'active') = 'active'`
	args := []any{ownerID}
	switch {
	case c.hasAccountRole(roleAdmin):
		query += ` AND u.role IN ('customer', 'reseller')`
	case c.hasAccountRole(roleReseller):
		query += ` AND u.role = 'customer' AND u.parent_id = ?`
		args = append(args, c.ID)
	default:
		refusal := transferAuthority(c, transferParty{}, transferParty{})
		writeTransferRefusal(w, refusal, nil)
		return
	}
	query += ` ORDER BY u.username LIMIT 500`
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		writeServerError(w, err)
		return
	}
	defer rows.Close()
	type receiver struct {
		ID       int    `json:"id"`
		Username string `json:"username"`
		Role     string `json:"role"`
		Reseller string `json:"reseller,omitempty"`
	}
	list := []receiver{}
	for rows.Next() {
		var item receiver
		if err := rows.Scan(&item.ID, &item.Username, &item.Role, &item.Reseller); err != nil {
			writeServerError(w, err)
			return
		}
		list = append(list, item)
	}
	if err := rows.Err(); err != nil {
		writeServerError(w, err)
		return
	}
	writeRightsJSON(w, http.StatusOK, map[string]any{"receivers": list})
}

// transferCandidates: the installed offering ids and the subscription's
// own items, read before any transaction (the Agent is not called inside
// one).
func (p *Panel) transferCandidates(ctx context.Context, subID int) ([]string, bool) {
	own, err := subscriptionRightsState(ctx, p.db.GetDB(), subID)
	if err != nil {
		return p.previewCandidates(ctx)
	}
	return p.previewCandidates(ctx, own.Set)
}

// writeTransferPreview answers 200 with the preview; a refusal that the
// preview explains (missing rights, a full pool) is 200 too, with
// "refusal", so the screen shows the list and the ways out.
func (p *Panel) writeTransferPreview(w http.ResponseWriter, r *http.Request, c *Caller, subID, toID int) {
	ctx := r.Context()
	candidates, unknown := p.transferCandidates(ctx, subID)
	preview, refusal, err := p.planTransfer(ctx, p.db.GetDB(), c, subID, toID, candidates, true)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if refusal != nil && preview == nil {
		writeTransferRefusal(w, refusal, nil)
		return
	}
	preview.InstalledUnknown = unknown
	body := map[string]any{"preview": preview, "can_transfer": refusal == nil}
	if refusal != nil {
		body["refusal"] = refusal
	}
	writeRightsJSON(w, http.StatusOK, body)
}

// transferReplay answers a request id that was used before: the stored
// result when the call is the same, else a refusal. handled is false when
// the id is new.
func (p *Panel) transferReplay(w http.ResponseWriter, r *http.Request, subID, toID int, requestID string, requestRow *transferRow) bool {
	row, err := loadTransferRow(r.Context(), p.db.GetDB(), `t.request_id = ? OR t.decision_request_id = ?`, requestID, requestID)
	if errors.Is(err, errNotFound) {
		return false
	}
	if err != nil {
		writeServerError(w, err)
		return true
	}
	same := row.Status == transferCompleted && row.SubscriptionID == subID && row.To != nil && row.To.ID == toID
	if requestRow == nil {
		same = same && row.RequestID == requestID && row.DecisionRequestID == ""
	} else {
		same = same && row.ID == requestRow.ID && row.DecisionRequestID == requestID
	}
	if !same {
		writeCodedError(w, http.StatusConflict, errCodeTransferRequestIDReused,
			"this request id was already used for another change; nothing was changed. Open the transfer again to get a new one.", "")
		return true
	}
	writeRightsJSON(w, http.StatusOK, map[string]any{"success": true, "replayed": true, "transfer": row})
	return true
}

// performTransfer runs a confirmed transfer, directly or as the approval of
// requestRow. Every check runs again inside the write transaction.
func (p *Panel) performTransfer(w http.ResponseWriter, r *http.Request, c *Caller, subID int, body transferBody, requestRow *transferRow) {
	ctx := r.Context()
	if !validTransferRequestID(body.RequestID) {
		writeCodedError(w, http.StatusBadRequest, errCodeTransferRequestID,
			"request_id is required: 8 to 80 letters, digits or . _ : - (a new one for every transfer)", "")
		return
	}
	if p.transferReplay(w, r, subID, body.ToUserID, body.RequestID, requestRow) {
		return
	}
	if requestRow != nil && requestRow.Status != transferRequested {
		writeCodedError(w, http.StatusConflict, errCodeTransferRequestClosed,
			"this request was already decided or withdrawn; nothing was changed", "")
		return
	}
	db := p.db.GetDB()
	candidates, unknown := p.transferCandidates(ctx, subID)
	preview, refusal, err := p.planTransfer(ctx, db, c, subID, body.ToUserID, candidates, true)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if refusal != nil {
		writeTransferRefusal(w, refusal, preview)
		return
	}
	preview.InstalledUnknown = unknown
	if !body.Confirm {
		writeTransferRefusal(w, &transferRefusal{Status: http.StatusConflict, Code: errCodeTransferConfirm, Message: preview.Message}, preview)
		return
	}
	if body.FromUserID != preview.From.ID {
		writeTransferRefusal(w, &transferRefusal{Status: http.StatusConflict, Code: errCodeTransferStale,
			Message: "Nothing was changed: the subscription's owner is not the one the preview showed. Review the transfer again."}, preview)
		return
	}

	uid, actingAs := auditPerson(r)
	tx, err := beginTransferTx(ctx, db)
	if err != nil {
		writeServerError(w, err)
		return
	}
	defer tx.Rollback()
	if requestRow != nil {
		var status string
		if err := tx.QueryRowContext(ctx, `SELECT status FROM subscription_transfers WHERE id = ?`, requestRow.ID).Scan(&status); err != nil || status != transferRequested {
			writeCodedError(w, http.StatusConflict, errCodeTransferRequestClosed,
				"this request was already decided or withdrawn; nothing was changed", "")
			return
		}
	}
	final, refusal, err := p.planTransfer(ctx, tx, c, subID, body.ToUserID, candidates, false)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if refusal != nil {
		writeTransferRefusal(w, refusal, final)
		return
	}
	if final.From.ID != body.FromUserID {
		writeTransferRefusal(w, &transferRefusal{Status: http.StatusConflict, Code: errCodeTransferStale,
			Message: "Nothing was changed: the subscription changed owner meanwhile. Review the transfer again."}, final)
		return
	}
	final.CappedInUse = preview.CappedInUse
	final.InstalledUnknown = unknown
	img, err := applyTransfer(ctx, tx, final)
	if errors.Is(err, errTransferStale) {
		writeTransferRefusal(w, &transferRefusal{Status: http.StatusConflict, Code: errCodeTransferStale,
			Message: "Nothing was changed: the subscription changed owner meanwhile. Review the transfer again."}, final)
		return
	}
	if err != nil {
		writeServerError(w, err)
		return
	}
	beforeJSON, err := json.Marshal(img)
	if err != nil {
		writeServerError(w, err)
		return
	}
	resultJSON, err := json.Marshal(final)
	if err != nil {
		writeServerError(w, err)
		return
	}
	var transferID int
	if requestRow == nil {
		res, err := tx.ExecContext(ctx, `
			INSERT INTO subscription_transfers
			  (request_id, subscription_id, subscription_name, status, from_owner_id, to_owner_id,
			   requested_by, requested_acting_as, decided_by, decided_acting_as, before_image, result, decided_at)
			VALUES (?, ?, ?, 'completed', ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))`,
			body.RequestID, subID, final.SubscriptionName, final.From.ID, final.To.ID,
			uid, nullableID(actingAs), uid, nullableID(actingAs), string(beforeJSON), string(resultJSON))
		if err != nil {
			p.transferWriteFailed(w, r, subID, body, requestRow, err)
			return
		}
		id, _ := res.LastInsertId()
		transferID = int(id)
	} else {
		res, err := tx.ExecContext(ctx, `
			UPDATE subscription_transfers
			SET status = 'completed', to_owner_id = ?, decided_by = ?, decided_acting_as = ?, decision_request_id = ?,
			    before_image = ?, result = ?, decided_at = datetime('now')
			WHERE id = ? AND status = 'requested'`,
			final.To.ID, uid, nullableID(actingAs), body.RequestID, string(beforeJSON), string(resultJSON), requestRow.ID)
		if err != nil {
			p.transferWriteFailed(w, r, subID, body, requestRow, err)
			return
		}
		if n, err := res.RowsAffected(); err != nil || n != 1 {
			writeCodedError(w, http.StatusConflict, errCodeTransferRequestClosed,
				"this request was already decided or withdrawn; nothing was changed", "")
			return
		}
		transferID = requestRow.ID
	}
	if err := tx.Commit(); err != nil {
		writeServerError(w, err)
		return
	}

	requestRef := 0
	if requestRow != nil {
		requestRef = requestRow.ID
		p.auditDetailed(r, "subscription.transfer_request_approve:"+final.SubscriptionName, "subscription", subID,
			map[string]any{"transfer_request": requestRow.ID, "to": final.To.Username, "to_user_id": final.To.ID})
	}
	p.auditDetailed(r, "subscription.transfer:"+final.SubscriptionName, "subscription", subID,
		transferAuditDetails(final, transferID, requestRef))
	p.noteTransferPool(ctx, final)
	row, err := loadTransferRow(ctx, db, `t.id = ?`, transferID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	writeRightsJSON(w, http.StatusOK, map[string]any{"success": true, "transfer": row, "preview": final})
}

// transferWriteFailed: a unique request id taken by a parallel call is
// answered like a replay; anything else is a server error.
func (p *Panel) transferWriteFailed(w http.ResponseWriter, r *http.Request, subID int, body transferBody, requestRow *transferRow, err error) {
	if strings.Contains(err.Error(), "UNIQUE") {
		if p.transferReplay(w, r, subID, body.ToUserID, body.RequestID, requestRow) {
			return
		}
	}
	writeServerError(w, err)
}

// noteTransferPool stores the 80%/100% pool notices the new use reaches.
func (p *Panel) noteTransferPool(ctx context.Context, preview *transferPreview) {
	if preview.planned == nil || preview.planned.pool == nil {
		return
	}
	for _, line := range preview.Pool {
		p.notePoolLevel(ctx, preview.planned.toReseller, quotaKind(line.Kind), line.Used+line.Adds, line.Limit)
	}
}

func nullableID(id int) any {
	if id <= 0 {
		return nil
	}
	return id
}

// ---- requests (K3) ----

type transferRequestBody struct {
	RequestID  string `json:"request_id"`
	ToUsername string `json:"to_username,omitempty"`
	Note       string `json:"note,omitempty"`
}

// createTransferRequest: the owning customer asks; its reseller, or the
// administrator when it has none, decides.
func (p *Panel) createTransferRequest(w http.ResponseWriter, r *http.Request, c *Caller, subID int) {
	ctx := r.Context()
	if !c.hasAccountRole(roleCustomer) {
		writeCodedError(w, http.StatusForbidden, errCodeTransferNotAllowed,
			"a transfer request is sent by the customer who owns the subscription; the administrator and resellers transfer directly", "")
		return
	}
	var body transferRequestBody
	if err := decodeStrictJSON(w, r, &body); err != nil {
		writeClientError(w, http.StatusBadRequest, "invalid transfer request")
		return
	}
	body.ToUsername = strings.TrimSpace(body.ToUsername)
	body.Note = strings.TrimSpace(body.Note)
	if !validTransferRequestID(body.RequestID) {
		writeCodedError(w, http.StatusBadRequest, errCodeTransferRequestID,
			"request_id is required: 8 to 80 letters, digits or . _ : -", "")
		return
	}
	if len(body.ToUsername) > 100 || len(body.Note) > 500 {
		writeClientError(w, http.StatusBadRequest, "the receiver is at most 100 characters and the note at most 500")
		return
	}
	db := p.db.GetDB()
	if row, err := loadTransferRow(ctx, db, `t.request_id = ? OR t.decision_request_id = ?`, body.RequestID, body.RequestID); err == nil {
		if row.SubscriptionID == subID && row.From.ID == c.ID && row.RequestedTo == body.ToUsername && row.Note == body.Note && row.DecisionRequestID != body.RequestID {
			writeRightsJSON(w, http.StatusOK, map[string]any{"success": true, "replayed": true, "request": row})
			return
		}
		writeCodedError(w, http.StatusConflict, errCodeTransferRequestIDReused,
			"this request id was already used for another change; nothing was changed", "")
		return
	} else if !errors.Is(err, errNotFound) {
		writeServerError(w, err)
		return
	}
	var ownerID int
	var name string
	if err := db.QueryRowContext(ctx, `SELECT owner_id, name FROM subscriptions WHERE id = ?`, subID).Scan(&ownerID, &name); err != nil {
		writeServerError(w, err)
		return
	}
	if ownerID != c.ID {
		writeClientError(w, http.StatusNotFound, "subscription not found")
		return
	}
	approver, err := treeResellerID(ctx, db, c.ID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	uid, actingAs := auditPerson(r)
	res, err := db.ExecContext(ctx, `
		INSERT INTO subscription_transfers
		  (request_id, subscription_id, subscription_name, status, from_owner_id, requested_to, note,
		   approver_reseller_id, requested_by, requested_acting_as)
		VALUES (?, ?, ?, 'requested', ?, ?, ?, ?, ?, ?)`,
		body.RequestID, subID, name, c.ID, body.ToUsername, body.Note, nullableID(approver), uid, nullableID(actingAs))
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			if open, openErr := loadTransferRow(ctx, db, `t.subscription_id = ? AND t.status = 'requested'`, subID); openErr == nil {
				writeRightsJSON(w, http.StatusConflict, map[string]any{
					"error":   "Nothing was sent: this subscription already has an open transfer request. Wait for the decision, or withdraw it and send a new one.",
					"code":    errCodeTransferRequestOpen,
					"request": open,
				})
				return
			}
			writeCodedError(w, http.StatusConflict, errCodeTransferRequestIDReused,
				"this request id was already used for another change; nothing was changed", "")
			return
		}
		writeServerError(w, err)
		return
	}
	id, _ := res.LastInsertId()
	p.auditDetailed(r, "subscription.transfer_request:"+name, "subscription", subID, map[string]any{
		"transfer_request": id, "requested_to": body.ToUsername, "approver_reseller_id": approver,
	})
	row, err := loadTransferRow(ctx, db, `t.id = ?`, id)
	if err != nil {
		writeServerError(w, err)
		return
	}
	writeRightsJSON(w, http.StatusCreated, map[string]any{"success": true, "request": row})
}

// handleTransferRequests serves /api/v1/transfer-requests[/{id}/{action}].
func (p *Panel) handleTransferRequests(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "private, no-store")
	c := currentCaller(r)
	if !transferAccountCaller(c) {
		writeCodedError(w, http.StatusForbidden, errCodeTransferNotAllowed, "account access required", "")
		return
	}
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, transferRequestsPath), "/")
	if rest == "" {
		if r.Method != http.MethodGet {
			rejectRouteMethod(w, []string{http.MethodGet})
			return
		}
		p.listTransferRequests(w, r, c)
		return
	}
	parts := strings.Split(rest, "/")
	id, ok := strictPositiveID(parts[0])
	if !ok || len(parts) != 2 {
		writeClientError(w, http.StatusNotFound, "transfer request not found")
		return
	}
	if r.Method != http.MethodPost {
		rejectRouteMethod(w, []string{http.MethodPost})
		return
	}
	ctx := r.Context()
	row, err := loadTransferRow(ctx, p.db.GetDB(), `t.id = ?`, id)
	if errors.Is(err, errNotFound) || (err == nil && !transferRowVisible(ctx, p, c, row)) {
		writeClientError(w, http.StatusNotFound, "transfer request not found")
		return
	}
	if err != nil {
		writeServerError(w, err)
		return
	}
	switch parts[1] {
	case "preview", "approve", "reject":
		if !transferApprover(c, row) {
			writeCodedError(w, http.StatusForbidden, errCodeTransferNotAllowed,
				"only the customer's reseller or the administrator decides a transfer request", "")
			return
		}
		// An approval checks the status after its replay check.
		if row.Status != transferRequested && parts[1] != "approve" {
			writeCodedError(w, http.StatusConflict, errCodeTransferRequestClosed,
				"this request was already decided or withdrawn; nothing was changed", "")
			return
		}
		switch parts[1] {
		case "preview":
			var body transferBody
			if err := decodeStrictJSON(w, r, &body); err != nil || body.ToUserID <= 0 {
				writeClientError(w, http.StatusBadRequest, "to_user_id is required")
				return
			}
			p.writeTransferPreview(w, r, c, row.SubscriptionID, body.ToUserID)
		case "approve":
			var body transferBody
			if err := decodeStrictJSON(w, r, &body); err != nil || body.ToUserID <= 0 {
				writeClientError(w, http.StatusBadRequest, "to_user_id is required")
				return
			}
			p.performTransfer(w, r, c, row.SubscriptionID, body, &row)
		case "reject":
			p.rejectTransferRequest(w, r, row)
		}
	case "cancel":
		p.cancelTransferRequest(w, r, c, row)
	case "seen":
		p.markTransferSeen(w, r, c, row)
	default:
		writeClientError(w, http.StatusNotFound, "transfer request not found")
	}
}

// transferApprover: the administrator, or the reseller named on the request.
func transferApprover(c *Caller, row transferRow) bool {
	if c.hasAccountRole(roleAdmin) {
		return true
	}
	return c.hasAccountRole(roleReseller) && row.ApproverResellerID == c.ID
}

// transferRowVisible: the administrator sees every row; a reseller rows it
// decides or that touch it or its customers; a customer rows from or to it.
func transferRowVisible(ctx context.Context, p *Panel, c *Caller, row transferRow) bool {
	if c.hasAccountRole(roleAdmin) {
		return true
	}
	if row.From.ID == c.ID || (row.To != nil && row.To.ID == c.ID) {
		return true
	}
	if !c.hasAccountRole(roleReseller) {
		return false
	}
	if row.ApproverResellerID == c.ID {
		return true
	}
	ids, _, err := p.visibleOwnerIDs(ctx, c)
	if err != nil {
		return false
	}
	return ids[row.From.ID] || (row.To != nil && ids[row.To.ID])
}

func (p *Panel) listTransferRequests(w http.ResponseWriter, r *http.Request, c *Caller) {
	ctx := r.Context()
	query := `SELECT ` + transferRowColumns + transferRowFrom
	args := []any{}
	switch {
	case c.hasAccountRole(roleAdmin):
	case c.hasAccountRole(roleReseller):
		query += ` WHERE t.approver_reseller_id = ?1 OR t.from_owner_id = ?1 OR t.to_owner_id = ?1
			OR t.from_owner_id IN (SELECT id FROM users WHERE parent_id = ?1)
			OR t.to_owner_id IN (SELECT id FROM users WHERE parent_id = ?1)`
		args = append(args, c.ID)
	default:
		query += ` WHERE t.from_owner_id = ?1 OR t.to_owner_id = ?1`
		args = append(args, c.ID)
	}
	query += ` ORDER BY CASE t.status WHEN 'requested' THEN 0 ELSE 1 END, t.id DESC LIMIT 200`
	rows, err := p.db.GetDB().QueryContext(ctx, query, args...)
	if err != nil {
		writeServerError(w, err)
		return
	}
	defer rows.Close()
	list := []transferRow{}
	for rows.Next() {
		row, err := scanTransferRow(rows)
		if err != nil {
			writeServerError(w, err)
			return
		}
		list = append(list, row)
	}
	if err := rows.Err(); err != nil {
		writeServerError(w, err)
		return
	}
	// What the caller has not seen yet, and what waits for its decision.
	var received, decided, waiting int
	db := p.db.GetDB()
	if err := db.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM subscription_transfers WHERE status = 'completed' AND to_owner_id = ?1 AND receiver_seen_at IS NULL),
		(SELECT COUNT(*) FROM subscription_transfers WHERE status IN ('completed', 'rejected') AND from_owner_id = ?1
		   AND decision_request_id IS NOT NULL AND requester_seen_at IS NULL)`, c.ID).Scan(&received, &decided); err != nil {
		writeServerError(w, err)
		return
	}
	switch {
	case c.hasAccountRole(roleAdmin):
		err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM subscription_transfers WHERE status = 'requested'`).Scan(&waiting)
	case c.hasAccountRole(roleReseller):
		err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM subscription_transfers WHERE status = 'requested' AND approver_reseller_id = ?`, c.ID).Scan(&waiting)
	}
	if err != nil {
		writeServerError(w, err)
		return
	}
	writeRightsJSON(w, http.StatusOK, map[string]any{
		"requests": list,
		"unseen":   map[string]int{"received": received, "decided": decided},
		"waiting":  waiting,
	})
}

type transferDecisionBody struct {
	RequestID string `json:"request_id"`
	Reason    string `json:"reason,omitempty"`
}

func (p *Panel) rejectTransferRequest(w http.ResponseWriter, r *http.Request, row transferRow) {
	ctx := r.Context()
	var body transferDecisionBody
	if err := decodeStrictJSON(w, r, &body); err != nil || !validTransferRequestID(body.RequestID) {
		writeCodedError(w, http.StatusBadRequest, errCodeTransferRequestID, "request_id is required: 8 to 80 letters, digits or . _ : -", "")
		return
	}
	body.Reason = strings.TrimSpace(body.Reason)
	if len(body.Reason) > 500 {
		writeClientError(w, http.StatusBadRequest, "the reason is at most 500 characters")
		return
	}
	uid, actingAs := auditPerson(r)
	res, err := p.db.GetDB().ExecContext(ctx, `
		UPDATE subscription_transfers
		SET status = 'rejected', decided_by = ?, decided_acting_as = ?, decision_request_id = ?, decision_reason = ?, decided_at = datetime('now')
		WHERE id = ? AND status = 'requested'`, uid, nullableID(actingAs), body.RequestID, body.Reason, row.ID)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			writeCodedError(w, http.StatusConflict, errCodeTransferRequestIDReused, "this request id was already used for another change; nothing was changed", "")
			return
		}
		writeServerError(w, err)
		return
	}
	if n, err := res.RowsAffected(); err != nil || n != 1 {
		writeCodedError(w, http.StatusConflict, errCodeTransferRequestClosed, "this request was already decided or withdrawn; nothing was changed", "")
		return
	}
	p.auditDetailed(r, "subscription.transfer_request_reject:"+row.SubscriptionName, "subscription", row.SubscriptionID,
		map[string]any{"transfer_request": row.ID, "reason": body.Reason})
	p.writeTransferRowAnswer(w, r, row.ID)
}

func (p *Panel) cancelTransferRequest(w http.ResponseWriter, r *http.Request, c *Caller, row transferRow) {
	if row.From.ID != c.ID || !c.hasAccountRole(roleCustomer) {
		writeCodedError(w, http.StatusForbidden, errCodeTransferNotAllowed, "only the customer who asked withdraws a transfer request", "")
		return
	}
	res, err := p.db.GetDB().ExecContext(r.Context(), `
		UPDATE subscription_transfers SET status = 'cancelled', decision_reason = 'withdrawn by the requester', decided_at = datetime('now')
		WHERE id = ? AND status = 'requested'`, row.ID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if n, err := res.RowsAffected(); err != nil || n != 1 {
		writeCodedError(w, http.StatusConflict, errCodeTransferRequestClosed, "this request was already decided or withdrawn; nothing was changed", "")
		return
	}
	p.auditDetailed(r, "subscription.transfer_request_cancel:"+row.SubscriptionName, "subscription", row.SubscriptionID,
		map[string]any{"transfer_request": row.ID})
	p.writeTransferRowAnswer(w, r, row.ID)
}

// markTransferSeen: the receiver or the requester has seen the outcome.
func (p *Panel) markTransferSeen(w http.ResponseWriter, r *http.Request, c *Caller, row transferRow) {
	db := p.db.GetDB()
	ctx := r.Context()
	if row.To != nil && row.To.ID == c.ID {
		if _, err := db.ExecContext(ctx, `UPDATE subscription_transfers SET receiver_seen_at = datetime('now') WHERE id = ? AND receiver_seen_at IS NULL`, row.ID); err != nil {
			writeServerError(w, err)
			return
		}
	}
	if row.From.ID == c.ID {
		if _, err := db.ExecContext(ctx, `UPDATE subscription_transfers SET requester_seen_at = datetime('now') WHERE id = ? AND requester_seen_at IS NULL`, row.ID); err != nil {
			writeServerError(w, err)
			return
		}
	}
	writeRightsJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (p *Panel) writeTransferRowAnswer(w http.ResponseWriter, r *http.Request, id int) {
	row, err := loadTransferRow(r.Context(), p.db.GetDB(), `t.id = ?`, id)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if err := json.NewEncoder(w).Encode(map[string]any{"success": true, "request": row}); err != nil {
		log.Printf("encode transfer request: %v", err)
	}
}
