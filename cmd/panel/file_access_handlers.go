package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/alicelik/celikpanel/internal/fileaccess"
	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// File access of a site (alpha.117): FTP over TLS, SFTP and, with the ssh
// right, a jailed command line. The account is the site's own system user;
// the Agent keeps the native configuration and is the record of who has
// access (cmd/agent/file_access.go). The Panel stores nothing: no password,
// no copy of the keys. It checks the subscription's rights and status, asks
// the Agent, and writes the audit trail.
//
// Sitenin dosya erişimi. Hesap sitenin kendi kullanıcısıdır; kaydı Agent
// tutar, Panel parola saklamaz. Panel hakları ve abonelik durumunu denetler.

const (
	fileAccessRight = "file_access"
	sshRight        = "ssh"
)

type fileAccessSite struct {
	DomainID           int
	Domain             string
	SubscriptionID     int
	SubscriptionStatus string
	SiteID             int
	ProjectType        string
}

func (p *Panel) loadFileAccessSite(ctx context.Context, domainID int) (fileAccessSite, error) {
	var site fileAccessSite
	var siteID sql.NullInt64
	var projectType, status sql.NullString
	err := p.db.GetDB().QueryRowContext(ctx, `
		SELECT d.id, d.name, d.subscription_id, s.status, si.id, si.project_type
		FROM domains d
		JOIN subscriptions s ON s.id = d.subscription_id
		LEFT JOIN sites si ON si.domain_id = d.id
		WHERE d.id = ?
		ORDER BY si.id LIMIT 1`, domainID).
		Scan(&site.DomainID, &site.Domain, &site.SubscriptionID, &status, &siteID, &projectType)
	if err != nil {
		return site, err
	}
	site.SubscriptionStatus = strings.TrimSpace(status.String)
	if site.SubscriptionStatus == "" {
		site.SubscriptionStatus = "active"
	}
	site.SiteID = int(siteID.Int64)
	site.ProjectType = projectType.String
	return site, nil
}

// hasAccount: a site with files has its own system account; a DNS-only
// domain has none.
func (s fileAccessSite) hasAccount() bool {
	return s.SiteID > 0 && s.ProjectType != "dnsonly"
}

func (s fileAccessSite) transportSite() transport.FileAccessSite {
	return transport.FileAccessSite{
		SubscriptionID: s.SubscriptionID,
		DomainID:       s.DomainID,
		Username:       services.SiteUsername(s.Domain),
	}
}

// reconcileSites: the subscription's other accounts, which enabling access
// brings to the no-login shell and the site group.
func (p *Panel) fileAccessReconcileSites(ctx context.Context, site fileAccessSite) ([]transport.FileAccessSite, error) {
	rows, err := p.db.GetDB().QueryContext(ctx, `
		SELECT d.id, d.name
		FROM domains d
		JOIN sites si ON si.domain_id = d.id
		WHERE d.subscription_id = ? AND d.id <> ? AND COALESCE(si.project_type, 'php') <> 'dnsonly'
		ORDER BY d.id`, site.SubscriptionID, site.DomainID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []transport.FileAccessSite
	for rows.Next() {
		var id int
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		out = append(out, transport.FileAccessSite{SubscriptionID: site.SubscriptionID, DomainID: id, Username: services.SiteUsername(name)})
	}
	return out, rows.Err()
}

type fileAccessRights struct {
	FileAccess bool `json:"file_access"`
	SSH        bool `json:"ssh"`
}

func (p *Panel) fileAccessRights(ctx context.Context, subscriptionID int) (fileAccessRights, error) {
	files, err := p.subscriptionHasRight(ctx, int64(subscriptionID), fileAccessRight)
	if err != nil {
		return fileAccessRights{}, err
	}
	ssh, err := p.subscriptionHasRight(ctx, int64(subscriptionID), sshRight)
	if err != nil {
		return fileAccessRights{}, err
	}
	return fileAccessRights{FileAccess: files, SSH: files && ssh}, nil
}

type fileAccessConnection struct {
	FTPHost    string `json:"ftp_host"`
	SFTPHost   string `json:"sftp_host"`
	ServerIP   string `json:"server_ip,omitempty"`
	FTPPort    int    `json:"ftp_port"`
	SFTPPort   int    `json:"sftp_port"`
	Username   string `json:"username"`
	StartDir   string `json:"start_dir"`
	PassiveMin int    `json:"passive_min"`
	PassiveMax int    `json:"passive_max"`
}

type fileAccessView struct {
	Domain             string                             `json:"domain"`
	Available          bool                               `json:"available"`
	SubscriptionStatus string                             `json:"subscription_status"`
	Rights             fileAccessRights                   `json:"rights"`
	Connection         fileAccessConnection               `json:"connection"`
	State              *transport.FileAccessStateResponse `json:"state,omitempty"`
	// StateError says why State is missing: agent_unavailable or
	// agent_upgrade_required (unknown, not "no access").
	StateError string `json:"state_error,omitempty"`
}

func fileAccessConnectionFor(site fileAccessSite, state *transport.FileAccessStateResponse) fileAccessConnection {
	connection := fileAccessConnection{
		FTPHost: site.Domain, SFTPHost: site.Domain, ServerIP: serverPrimaryIP(),
		FTPPort: 21, SFTPPort: 22, Username: services.SiteUsername(site.Domain),
		StartDir: fileaccess.StartDir(site.DomainID), PassiveMin: fileaccess.PassiveMin, PassiveMax: fileaccess.PassiveMax,
	}
	if state != nil {
		if len(state.Server.SSHPorts) > 0 {
			connection.SFTPPort = state.Server.SSHPorts[0]
		}
		// A trusted certificate matches only its own name: FTP clients
		// connect to that name, or they warn about the certificate.
		if state.Server.FTPCertificate == transport.FileAccessCertMailHost && state.Server.FTPCertificateName != "" {
			connection.FTPHost = state.Server.FTPCertificateName
		}
	}
	return connection
}

// requireFileAccessAgent: an Agent older than this feature answers nothing
// useful; the page says the paired update is not finished.
func (p *Panel) requireFileAccessAgent(ctx context.Context) error {
	var agent transport.AgentVersionResponse
	if err := p.callAgentContext(ctx, "Agent.Version", &transport.Empty{}, &agent); err != nil {
		return err
	}
	return requireKnownAgentCapabilities(agent.Capabilities, transport.AgentCapabilityFileAccessV1)
}

var errFileAccessAgentUpgrade = errors.New("the Agent does not offer file access yet")

func (p *Panel) handleDomainFileAccess(w http.ResponseWriter, r *http.Request, domainID int, preview bool) {
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
	switch {
	case preview && r.Method == http.MethodPost:
		p.handlePutFileAccess(w, r, site, true)
	case !preview && r.Method == http.MethodGet:
		p.handleGetFileAccess(w, r, site)
	case !preview && r.Method == http.MethodPut:
		p.handlePutFileAccess(w, r, site, false)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (p *Panel) handleGetFileAccess(w http.ResponseWriter, r *http.Request, site fileAccessSite) {
	rights, err := p.fileAccessRights(r.Context(), site.SubscriptionID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	view := fileAccessView{
		Domain: site.Domain, Available: site.hasAccount(), SubscriptionStatus: site.SubscriptionStatus, Rights: rights,
	}
	if view.Available {
		if err := p.requireFileAccessAgent(r.Context()); err != nil {
			view.StateError = "agent_upgrade_required"
		} else {
			var state transport.FileAccessStateResponse
			if err := p.callAgentContext(r.Context(), "Agent.FileAccessStateV1",
				&transport.FileAccessStateRequest{Site: site.transportSite()}, &state); err != nil {
				view.StateError = "agent_unavailable"
			} else {
				view.State = &state
			}
		}
	}
	view.Connection = fileAccessConnectionFor(site, view.State)
	_ = json.NewEncoder(w).Encode(view)
}

type fileAccessPutRequest struct {
	Enabled    bool     `json:"enabled"`
	SSH        bool     `json:"ssh"`
	Password   string   `json:"password"`
	AddKeys    []string `json:"add_keys"`
	RemoveKeys []string `json:"remove_keys"`
}

func (p *Panel) handlePutFileAccess(w http.ResponseWriter, r *http.Request, site fileAccessSite, preview bool) {
	var request fileAccessPutRequest
	if err := decodeStrictJSON(w, r, &request); err != nil {
		writeCodedError(w, http.StatusBadRequest, errCodeFileAccessInvalid, "invalid request body", "")
		return
	}
	if !site.hasAccount() {
		writeCodedError(w, http.StatusConflict, errCodeFileAccessUnavailable,
			"this domain has no website files, so there is nothing to connect to; give it a website first", "")
		return
	}
	// Closing access is always allowed: it is what a suspension or a
	// narrowed plan needs. Opening or changing it needs the right and an
	// active subscription.
	opening := request.Enabled
	if opening {
		if site.SubscriptionStatus != "active" {
			writeCodedError(w, http.StatusConflict, errCodeFileAccessSubscriptionInactive,
				"the subscription is "+site.SubscriptionStatus+", so file access cannot be opened; the provider renews it first", "")
			return
		}
		rights, err := p.fileAccessRights(r.Context(), site.SubscriptionID)
		if err != nil {
			writeServerError(w, err)
			return
		}
		if !rights.FileAccess {
			writeCodedError(w, http.StatusForbidden, errCodeFileAccessNotPermitted,
				"this subscription's plan does not include file access (FTP and SFTP); the provider can add it", "")
			return
		}
		if request.SSH && !rights.SSH {
			writeCodedError(w, http.StatusForbidden, errCodeFileAccessSSHNotPermitted,
				"this subscription's plan does not include the command line (SSH); file access is not affected", "")
			return
		}
		// After a transfer the previous owner's password and keys are removed
		// first; opening access waits until that step has finished.
		if open, err := p.siteAccountRotationOpen(r.Context(), site.DomainID); err != nil {
			writeServerError(w, err)
			return
		} else if open && !preview {
			writeCodedError(w, http.StatusConflict, errCodeTransferRotationPending,
				"this site came with a subscription transfer, and the previous owner's password and SSH keys are not removed yet; nothing was changed. "+
					"Wait until the transfer's change of passwords and keys has finished (Account page), or retry it there if it stopped, then open file access with a new password.", "")
			return
		}
	} else if request.Password != "" || len(request.AddKeys) > 0 {
		writeCodedError(w, http.StatusBadRequest, errCodeFileAccessInvalid,
			"a password or a key is set only while file access is on", "")
		return
	}
	if request.Password != "" {
		if err := fileaccess.ValidatePassword(request.Password); err != nil {
			writeCodedError(w, http.StatusBadRequest, errCodeFileAccessInvalid, err.Error(), "")
			return
		}
	}
	if len(request.AddKeys) > fileaccess.MaxKeys || len(request.RemoveKeys) > fileaccess.MaxKeys {
		writeCodedError(w, http.StatusBadRequest, errCodeFileAccessInvalid, "too many keys in one change", "")
		return
	}
	for _, key := range request.AddKeys {
		if _, err := fileaccess.ParsePublicKey(key); err != nil {
			writeCodedError(w, http.StatusBadRequest, errCodeFileAccessInvalid, err.Error(), "")
			return
		}
	}
	if err := p.requireFileAccessAgent(r.Context()); err != nil {
		writeCodedError(w, http.StatusConflict, errCodeFileAccessAgentUpgrade,
			"the server's Agent does not offer file access yet; finish the panel update first", "")
		return
	}
	reconcile, err := p.fileAccessReconcileSites(r.Context(), site)
	if err != nil {
		writeServerError(w, err)
		return
	}
	change := transport.ApplyFileAccessRequest{
		ExpectedBuildCommit: strings.TrimSpace(buildCommit),
		Site:                site.transportSite(),
		Enabled:             request.Enabled,
		SSH:                 request.Enabled && request.SSH,
		Password:            request.Password,
		AddKeys:             request.AddKeys,
		RemoveKeys:          request.RemoveKeys,
		Reconcile:           reconcile,
	}
	var response transport.ApplyFileAccessResponse
	if preview {
		change.Password = ""
		if err := p.callAgentContext(r.Context(), "Agent.FileAccessPreviewV1",
			&transport.FileAccessPreviewRequest{Change: change}, &response); err != nil {
			writeServerErrorFor(w, r, err)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"preview": true, "refusal": response.Refusal, "detail": response.Detail,
			"changes": response.Changes, "warnings": response.Warnings, "state": response.State,
			"connection": fileAccessConnectionFor(site, &response.State)})
		return
	}
	// Read-only discovery first: what the account has now names the audit
	// action, and a failed read stops before any change.
	var before transport.FileAccessStateResponse
	if err := p.callAgentContext(r.Context(), "Agent.FileAccessStateV1",
		&transport.FileAccessStateRequest{Site: change.Site}, &before); err != nil {
		writeServerErrorFor(w, r, err)
		return
	}
	if err := p.callAgentContext(r.Context(), "Agent.ApplyFileAccessV1", &change, &response); err != nil {
		// The answer did not arrive: the change may or may not be in place.
		// The page reads the state again; saving again is safe (idempotent).
		writeServerErrorFor(w, r, err)
		return
	}
	details := map[string]any{
		"account": change.Site.Username, "enabled": change.Enabled, "ssh": change.SSH,
		"password_changed": request.Password != "", "keys_added": len(request.AddKeys), "keys_removed": len(request.RemoveKeys),
	}
	if response.Refusal != "" {
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(apiErrorBody{Error: response.Detail, Code: errCodeFileAccessRefused, Reason: response.Refusal})
		return
	}
	action := "file_access.update"
	switch {
	case !change.Enabled:
		action = "file_access.disable"
	case !before.Account.Enabled:
		action = "file_access.enable"
	}
	p.auditDetailed(r, action, "domain", site.DomainID, details)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": response.Applied, "changes": response.Changes, "warnings": response.Warnings,
		"state": response.State, "connection": fileAccessConnectionFor(site, &response.State),
	})
}
