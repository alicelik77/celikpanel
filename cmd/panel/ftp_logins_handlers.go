package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/alicelik/celikpanel/internal/fileaccess"
	"github.com/alicelik/celikpanel/internal/passwordpolicy"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Additional FTP/SFTP logins of a site (alpha.128, gap G16,
// docs/FTP-LOGINS.md). Each login is confined to one folder of the site and
// uploads as the site, so PHP edits what it uploads. The Agent keeps the
// native configuration and is the record of who has a login on the server
// (cmd/agent/ftp_logins.go); the Panel keeps a row per login for the plan's
// count (migration 072), never a password.
//
// Order (D-025): the row is written first, inside the count check, so two
// requests cannot both take the last place; the Agent then makes the login.
// A refusal removes the row again; a lost answer keeps it, and the screen
// reads the server to say what is there (removing is always allowed).
//
// Sitenin ek FTP/SFTP girişleri. Panel yalnız sayım için satır tutar, parola
// tutmaz; sunucudaki kayıt Agent'ındır.

const (
	errCodeFTPLoginInvalid        = "FTP_LOGIN_INVALID"
	errCodeFTPLoginNameTaken      = "FTP_LOGIN_NAME_TAKEN"
	errCodeFTPLoginNotFound       = "FTP_LOGIN_NOT_FOUND"
	errCodeFTPLoginLimit          = "FTP_LOGIN_LIMIT_REACHED"
	errCodeFTPLoginRefused        = "FTP_LOGIN_REFUSED"
	errCodeFTPLoginAgentUpgrade   = "FTP_LOGIN_AGENT_UPGRADE_REQUIRED"
	ftpLoginsDefaultServerCeiling = 10
)

// ftpLoginRequest is the body of an add or a change.
type ftpLoginRequest struct {
	Login    string `json:"login"`
	Folder   string `json:"folder"`
	Password string `json:"password"`
}

// requestIdentityFTPLoginDigest leaves the password out of the stored
// request digest (D-029).
func requestIdentityFTPLoginDigest(body []byte) []byte {
	var request map[string]any
	if json.Unmarshal(body, &request) != nil {
		return []byte("ftp-login")
	}
	delete(request, "password")
	clean, _ := json.Marshal(request)
	return clean
}

type ftpLoginRecord struct {
	Login     string `json:"login"`
	Folder    string `json:"folder"`
	CreatedAt string `json:"created_at"`
}

type ftpLoginLimit struct {
	Used int `json:"used"`
	// Ceiling: nil is no limit.
	Ceiling *int   `json:"ceiling"`
	SetBy   string `json:"set_by,omitempty"`
	SetName string `json:"set_by_name,omitempty"`
}

type ftpLoginsView struct {
	Domain             string                            `json:"domain"`
	Available          bool                              `json:"available"`
	SubscriptionStatus string                            `json:"subscription_status"`
	Rights             fileAccessRights                  `json:"rights"`
	Owner              string                            `json:"owner,omitempty"`
	Limit              ftpLoginLimit                     `json:"limit"`
	Records            []ftpLoginRecord                  `json:"records"`
	Connection         fileAccessConnection              `json:"connection"`
	State              *transport.FTPLoginsStateResponse `json:"state,omitempty"`
	StateError         string                            `json:"state_error,omitempty"`
}

// ftpLoginsCeiling is the subscription's ceiling of additional logins: the
// plan's count once the limits chain carries it, until then the server
// default. The administrator's own workspace has none.
var ftpLoginsCeiling = func(p *Panel, ctx context.Context, subID int) (ftpLoginLimit, error) {
	if p.administratorWorkspace(ctx, subID) {
		return ftpLoginLimit{}, nil
	}
	ceiling := ftpLoginsDefaultServerCeiling
	return ftpLoginLimit{Ceiling: &ceiling, SetBy: "server"}, nil
}

func (p *Panel) ftpLoginsUsed(ctx context.Context, q sqlQueryer, subID int) (int, error) {
	var used int
	err := q.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM ftp_logins f JOIN domains d ON d.id = f.domain_id WHERE d.subscription_id = ?`, subID).Scan(&used)
	return used, err
}

func (p *Panel) ftpLoginRecords(ctx context.Context, domainID int) ([]ftpLoginRecord, error) {
	rows, err := p.db.GetDB().QueryContext(ctx, `
		SELECT login, folder, created_at FROM ftp_logins WHERE domain_id = ? ORDER BY login`, domainID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ftpLoginRecord{}
	for rows.Next() {
		var record ftpLoginRecord
		if err := rows.Scan(&record.Login, &record.Folder, &record.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, record)
	}
	return out, rows.Err()
}

func (p *Panel) requireFTPLoginsAgent(ctx context.Context) error {
	var agent transport.AgentVersionResponse
	if err := p.callAgentContext(ctx, "Agent.Version", &transport.Empty{}, &agent); err != nil {
		return err
	}
	return requireKnownAgentCapabilities(agent.Capabilities, transport.AgentCapabilityFTPLoginsV1)
}

// handleDomainFTPLogins serves /api/v1/domains/{id}/access/logins (GET,
// POST) and /access/logins/{login} (PUT, DELETE).
func (p *Panel) handleDomainFTPLogins(w http.ResponseWriter, r *http.Request, domainID int) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	site, err := p.loadFileAccessSite(r.Context(), domainID)
	if errors.Is(err, sql.ErrNoRows) {
		writeClientError(w, http.StatusNotFound, "domain not found")
		return
	}
	if err != nil {
		writeServerError(w, err)
		return
	}
	login := ""
	if i := strings.Index(r.URL.Path, "/access/logins/"); i >= 0 {
		login = r.URL.Path[i+len("/access/logins/"):]
		if fileaccess.ValidateLoginName(login) != nil {
			writeClientError(w, http.StatusNotFound, "no such login")
			return
		}
	}
	switch {
	case login == "" && r.Method == http.MethodGet:
		p.handleGetFTPLogins(w, r, site)
	case login == "" && r.Method == http.MethodPost:
		p.handleAddFTPLogin(w, r, site)
	case login != "" && r.Method == http.MethodPut:
		p.handleChangeFTPLogin(w, r, site, login)
	case login != "" && r.Method == http.MethodDelete:
		p.handleRemoveFTPLogin(w, r, site, login)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (p *Panel) handleGetFTPLogins(w http.ResponseWriter, r *http.Request, site fileAccessSite) {
	ctx := r.Context()
	rights, err := p.fileAccessRights(ctx, site.SubscriptionID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	limit, err := ftpLoginsCeiling(p, ctx, site.SubscriptionID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if limit.Used, err = p.ftpLoginsUsed(ctx, p.db.GetDB(), site.SubscriptionID); err != nil {
		writeServerError(w, err)
		return
	}
	records, err := p.ftpLoginRecords(ctx, site.DomainID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	view := ftpLoginsView{
		Domain: site.Domain, Available: site.hasAccount(), SubscriptionStatus: site.SubscriptionStatus,
		Rights: rights, Owner: p.fileAccessManagedOwner(r, site.SubscriptionID), Limit: limit, Records: records,
	}
	if view.Available {
		if err := p.requireFTPLoginsAgent(ctx); err != nil {
			view.StateError = "agent_upgrade_required"
		} else {
			var state transport.FTPLoginsStateResponse
			if err := p.callAgentContext(ctx, "Agent.FTPLoginsStateV1", &transport.FTPLoginsStateRequest{Site: site.transportSite()}, &state); err != nil {
				view.StateError = "agent_unavailable"
			} else {
				view.State = &state
			}
		}
	}
	view.Connection = ftpLoginConnection(site, view.State)
	_ = json.NewEncoder(w).Encode(view)
}

func ftpLoginConnection(site fileAccessSite, state *transport.FTPLoginsStateResponse) fileAccessConnection {
	connection := fileAccessConnectionFor(site, nil)
	connection.Username = ""
	connection.StartDir = fileaccess.LoginStartDir
	if state != nil && len(state.SSHPorts) > 0 {
		connection.SFTPPort = state.SSHPorts[0]
	}
	return connection
}

// ftpLoginOpening checks what opening or changing a login needs: a site
// with files, an active subscription, the file access permission, and no
// transfer still removing the previous owner's credentials.
func (p *Panel) ftpLoginOpening(w http.ResponseWriter, r *http.Request, site fileAccessSite) bool {
	ctx := r.Context()
	if !site.hasAccount() {
		writeCodedError(w, http.StatusConflict, errCodeFileAccessUnavailable,
			"this domain has no website files, so there is nothing to connect to; give it a website first", "")
		return false
	}
	if site.SubscriptionStatus != "active" {
		writeCodedError(w, http.StatusConflict, errCodeFileAccessSubscriptionInactive,
			"the subscription is "+site.SubscriptionStatus+", so logins cannot be added or changed; the provider renews it first", "")
		return false
	}
	rights, err := p.fileAccessRights(ctx, site.SubscriptionID)
	if err != nil {
		writeServerError(w, err)
		return false
	}
	if !rights.FileAccess {
		writeCodedError(w, http.StatusForbidden, errCodeFileAccessNotPermitted,
			"this subscription's plan does not include file access (FTP and SFTP); the provider can add it", "")
		return false
	}
	if open, err := p.siteAccountRotationOpen(ctx, site.DomainID); err != nil {
		writeServerError(w, err)
		return false
	} else if open {
		writeCodedError(w, http.StatusConflict, errCodeTransferRotationPending,
			"this site came with a subscription transfer, and the previous owner's passwords are not removed yet; nothing was changed. "+
				"Wait until the transfer's change of passwords and keys has finished (Account page), then try again.", "")
		return false
	}
	if err := p.requireFTPLoginsAgent(ctx); err != nil {
		writeCodedError(w, http.StatusConflict, errCodeFTPLoginAgentUpgrade,
			"the server's Agent does not offer additional logins yet; finish the panel update first", "")
		return false
	}
	return true
}

func checkFTPLoginPassword(w http.ResponseWriter, password string) bool {
	// The owner's rule (and the administrator's level) first, as
	// PASSWORD_WEAK; ValidatePassword is the wire check the Agent repeats.
	if refusal := passwordpolicy.CheckField(password, "file_access"); refusal != nil {
		writePasswordRefusal(w, refusal)
		return false
	}
	if err := fileaccess.ValidatePassword(password); err != nil {
		writeCodedError(w, http.StatusBadRequest, errCodeFTPLoginInvalid, err.Error(), "")
		return false
	}
	return true
}

func (p *Panel) callFTPLoginAgent(ctx context.Context, change transport.ApplyFTPLoginRequest) (transport.ApplyFTPLoginResponse, error) {
	change.ExpectedBuildCommit = strings.TrimSpace(buildCommit)
	var response transport.ApplyFTPLoginResponse
	err := p.callAgentContext(ctx, "Agent.ApplyFTPLoginV1", &change, &response)
	return response, err
}

func writeFTPLoginRefusal(w http.ResponseWriter, response transport.ApplyFTPLoginResponse) {
	status := http.StatusConflict
	code := errCodeFTPLoginRefused
	switch response.Refusal {
	case transport.FTPLoginRefusedInvalid, transport.FTPLoginRefusedFolder:
		status = http.StatusBadRequest
	case transport.FTPLoginRefusedNameTaken:
		code = errCodeFTPLoginNameTaken
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": response.Detail, "code": code, "reason": response.Refusal,
		"steps": response.Steps, "warnings": response.Warnings,
	})
}

func (p *Panel) handleAddFTPLogin(w http.ResponseWriter, r *http.Request, site fileAccessSite) {
	ctx := r.Context()
	var request ftpLoginRequest
	if err := decodeStrictJSON(w, r, &request); err != nil {
		writeCodedError(w, http.StatusBadRequest, errCodeFTPLoginInvalid, "invalid request body", "")
		return
	}
	reportOperationPlan(ctx, "check", "server_change", "finish")
	reportOperationStep(ctx, "check")
	if err := fileaccess.ValidateLoginName(request.Login); err != nil {
		writeCodedError(w, http.StatusBadRequest, errCodeFTPLoginInvalid, err.Error(), "")
		return
	}
	folder, err := fileaccess.CleanLoginFolder(request.Folder)
	if err != nil {
		writeCodedError(w, http.StatusBadRequest, errCodeFTPLoginInvalid, err.Error(), "")
		return
	}
	if !checkFTPLoginPassword(w, request.Password) {
		return
	}
	if !p.ftpLoginOpening(w, r, site) {
		return
	}
	readOnly, err := subscriptionUploadsBlocked(ctx, p.db.GetDB(), site.SubscriptionID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	// The count and the row in one transaction: the last place is taken once.
	limit, err := ftpLoginsCeiling(p, ctx, site.SubscriptionID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	tx, err := p.db.GetDB().BeginTx(ctx, nil)
	if err != nil {
		writeServerError(w, err)
		return
	}
	defer func() { _ = tx.Rollback() }()
	used, err := p.ftpLoginsUsed(ctx, tx, site.SubscriptionID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if limit.Ceiling != nil && used >= *limit.Ceiling {
		message := "this subscription has all the additional logins its plan gives (" + strconv.Itoa(used) + " of " + strconv.Itoa(*limit.Ceiling) + "); remove one, or the provider raises the plan's number of logins"
		writeCodedErrorDetails(w, http.StatusConflict, errCodeFTPLoginLimit, message, myPlanPagePath, []string{strconv.Itoa(used), strconv.Itoa(*limit.Ceiling), limit.SetBy})
		return
	}
	var taken int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM ftp_logins WHERE login = ?`, request.Login).Scan(&taken); err != nil {
		writeServerError(w, err)
		return
	}
	if taken > 0 {
		writeCodedError(w, http.StatusConflict, errCodeFTPLoginNameTaken, "the login "+request.Login+" exists already; choose another name", "")
		return
	}
	caller := currentCaller(r)
	createdBy := sql.NullInt64{}
	if caller != nil {
		createdBy = sql.NullInt64{Int64: int64(caller.ID), Valid: true}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO ftp_logins (domain_id, login, folder, created_by) VALUES (?, ?, ?, ?)`,
		site.DomainID, request.Login, folder, createdBy); err != nil {
		writeServerError(w, err)
		return
	}
	if err := tx.Commit(); err != nil {
		writeServerError(w, err)
		return
	}
	reportOperationStep(ctx, "server_change")
	response, err := p.callFTPLoginAgent(ctx, transport.ApplyFTPLoginRequest{
		Site: site.transportSite(), Action: transport.FTPLoginAdd, Login: request.Login, Folder: folder,
		Password: request.Password, ReadOnly: readOnly,
	})
	if err != nil {
		// The answer did not arrive: the row stays, the screen reads the
		// server again and says what is there.
		writeServerErrorFor(w, r, err)
		return
	}
	if response.Refusal != "" {
		if _, err := p.db.GetDB().ExecContext(ctx, `DELETE FROM ftp_logins WHERE domain_id = ? AND login = ?`, site.DomainID, request.Login); err != nil {
			writeServerError(w, err)
			return
		}
		writeFTPLoginRefusal(w, response)
		return
	}
	reportOperationStep(ctx, "finish")
	p.auditDetailed(r, "ftp_login.add", "domain", site.DomainID, map[string]any{"login": request.Login, "folder": folder})
	_ = json.NewEncoder(w).Encode(map[string]any{"success": response.Applied, "steps": response.Steps, "warnings": response.Warnings, "state": response.State})
}

func (p *Panel) handleChangeFTPLogin(w http.ResponseWriter, r *http.Request, site fileAccessSite, login string) {
	ctx := r.Context()
	var request ftpLoginRequest
	if err := decodeStrictJSON(w, r, &request); err != nil {
		writeCodedError(w, http.StatusBadRequest, errCodeFTPLoginInvalid, "invalid request body", "")
		return
	}
	reportOperationPlan(ctx, "check", "server_change", "finish")
	reportOperationStep(ctx, "check")
	if request.Login != "" && request.Login != login {
		writeCodedError(w, http.StatusBadRequest, errCodeFTPLoginInvalid, "a login's name cannot change; add a new login instead", "")
		return
	}
	folder := ""
	if strings.TrimSpace(request.Folder) != "" {
		clean, err := fileaccess.CleanLoginFolder(request.Folder)
		if err != nil {
			writeCodedError(w, http.StatusBadRequest, errCodeFTPLoginInvalid, err.Error(), "")
			return
		}
		folder = clean
	}
	if folder == "" && request.Password == "" {
		writeCodedError(w, http.StatusBadRequest, errCodeFTPLoginInvalid, "give a new password or a new folder", "")
		return
	}
	if request.Password != "" && !checkFTPLoginPassword(w, request.Password) {
		return
	}
	var current string
	err := p.db.GetDB().QueryRowContext(ctx, `SELECT folder FROM ftp_logins WHERE domain_id = ? AND login = ?`, site.DomainID, login).Scan(&current)
	if errors.Is(err, sql.ErrNoRows) {
		writeCodedError(w, http.StatusNotFound, errCodeFTPLoginNotFound, "this site has no login "+login, "")
		return
	}
	if err != nil {
		writeServerError(w, err)
		return
	}
	if !p.ftpLoginOpening(w, r, site) {
		return
	}
	readOnly, err := subscriptionUploadsBlocked(ctx, p.db.GetDB(), site.SubscriptionID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	reportOperationStep(ctx, "server_change")
	response, err := p.callFTPLoginAgent(ctx, transport.ApplyFTPLoginRequest{
		Site: site.transportSite(), Action: transport.FTPLoginUpdate, Login: login, Folder: folder,
		Password: request.Password, ReadOnly: readOnly,
	})
	if err != nil {
		writeServerErrorFor(w, r, err)
		return
	}
	if response.Refusal != "" {
		writeFTPLoginRefusal(w, response)
		return
	}
	reportOperationStep(ctx, "finish")
	if folder != "" && folder != current {
		if _, err := p.db.GetDB().ExecContext(ctx, `UPDATE ftp_logins SET folder = ? WHERE domain_id = ? AND login = ?`, folder, site.DomainID, login); err != nil {
			writeServerError(w, err)
			return
		}
	}
	p.auditDetailed(r, "ftp_login.update", "domain", site.DomainID, map[string]any{
		"login": login, "folder_changed": folder != "" && folder != current, "folder": folder, "password_changed": request.Password != "",
	})
	_ = json.NewEncoder(w).Encode(map[string]any{"success": response.Applied, "steps": response.Steps, "warnings": response.Warnings, "state": response.State})
}

// handleRemoveFTPLogin closes and removes a login. Closing is always
// allowed (a suspended or narrowed subscription needs it too).
func (p *Panel) handleRemoveFTPLogin(w http.ResponseWriter, r *http.Request, site fileAccessSite, login string) {
	ctx := r.Context()
	reportOperationPlan(ctx, "check", "server_change", "finish")
	reportOperationStep(ctx, "check")
	if err := p.requireFTPLoginsAgent(ctx); err != nil {
		writeCodedError(w, http.StatusConflict, errCodeFTPLoginAgentUpgrade,
			"the server's Agent does not offer additional logins yet; finish the panel update first", "")
		return
	}
	reportOperationStep(ctx, "server_change")
	response, err := p.callFTPLoginAgent(ctx, transport.ApplyFTPLoginRequest{
		Site: site.transportSite(), Action: transport.FTPLoginRemove, Login: login,
	})
	if err != nil {
		writeServerErrorFor(w, r, err)
		return
	}
	if response.Refusal != "" {
		writeFTPLoginRefusal(w, response)
		return
	}
	reportOperationStep(ctx, "finish")
	if _, err := p.db.GetDB().ExecContext(ctx, `DELETE FROM ftp_logins WHERE domain_id = ? AND login = ?`, site.DomainID, login); err != nil {
		writeServerError(w, err)
		return
	}
	p.auditDetailed(r, "ftp_login.remove", "domain", site.DomainID, map[string]any{"login": login})
	_ = json.NewEncoder(w).Encode(map[string]any{"success": response.Applied, "steps": response.Steps, "warnings": response.Warnings, "state": response.State})
}
