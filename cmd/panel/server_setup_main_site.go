package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/transport"
)

// The main domain's website at setup (alpha.114, owner decision 2026-10-31).
//
// Like Plesk's first site, the first server's setup ends with the main domain
// (apex and www) as a website with a Let's Encrypt certificate, unless the
// owner answered that the website is elsewhere. The step reuses the panel's
// own create path (handleCreateDomain: the site takes over the
// infrastructure zone setup prepared, publishTopLevelDomainZone /
// addInfrastructureZoneSiteRecords) and the SSL page's request
// (handleIssueLetsEncrypt), called in-process as the reviewing administrator,
// so the same checks, names, audit rows and native files result.
//
// D-024: the step never fails the setup. The site and the certificate are
// owner tasks once the server is prepared, so each outcome is recorded on the
// step (MainSite) and said on the ready screen with what to do: created and
// secured; created, certificate not issued yet (the reason, and the SSL page
// that requests it); not created (the reason, and "Host a website"). A step
// resumed after a restart reads what exists first and never creates twice.
//
// Ana alan adinin web sitesi kurulumda olusturulur; adim kurulumu hicbir zaman
// basarisiz kilmaz, sonuc adimda saklanir ve hazir ekraninda yapilacakla soylenir.

const (
	serverSetupMainSiteHere      = "here"
	serverSetupMainSiteElsewhere = "elsewhere"
)

// serverSetupMainSiteOutcome is additive on the execution step; an older page
// ignores it. Site: created | existed | failed. Certificate: issued | active |
// waiting_owner | pending | not_requested. Code/Reason/Message name a failure
// (bounded, no secrets).
type serverSetupMainSiteOutcome struct {
	Domain      string `json:"domain"`
	DomainID    int    `json:"domain_id,omitempty"`
	Site        string `json:"site"`
	Certificate string `json:"certificate"`
	Code        string `json:"code,omitempty"`
	Reason      string `json:"reason,omitempty"`
	Message     string `json:"message,omitempty"`
}

// serverSetupMainSiteStep: the first server hosts sites, the owner answered
// "here", and this server keeps the main domain's DNS (or another provider
// does). A joined server is never asked; its main domain lives on the first
// server.
func serverSetupMainSiteStep(d serverSetupDraft, joined bool) bool {
	return d.MainSite == serverSetupMainSiteHere && !joined && d.MainDomain != "" && serverSetupHostsSites(d) &&
		(d.DNSMode != setupDNSModeLocal || d.DNSRole == transport.DNSPairRolePrimary)
}

type serverSetupInternalResponse struct {
	header http.Header
	code   int
	body   bytes.Buffer
}

func (r *serverSetupInternalResponse) Header() http.Header {
	if r.header == nil {
		r.header = http.Header{}
	}
	return r.header
}
func (r *serverSetupInternalResponse) Write(b []byte) (int, error) {
	if r.code == 0 {
		r.code = http.StatusOK
	}
	return r.body.Write(b)
}
func (r *serverSetupInternalResponse) WriteHeader(code int) {
	if r.code == 0 {
		r.code = code
	}
}

// serverSetupAdminCaller is the reviewing administrator, read again: a
// removed, suspended or demoted account creates nothing.
func (p *Panel) serverSetupAdminCaller(ctx context.Context, actor serviceOperationActor) (*Caller, error) {
	var role, status, accountType string
	err := p.db.GetDB().QueryRowContext(ctx, `SELECT COALESCE(role,''), COALESCE(status,'active'), COALESCE(account_type,'') FROM users WHERE id=?`, actor.UserID).Scan(&role, &status, &accountType)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errors.New("the account that reviewed setup no longer exists")
	}
	if err != nil {
		return nil, err
	}
	if accountType == "" {
		accountType = string(core.AccountTypeAccount)
	}
	caller := &Caller{ID: actor.UserID, Role: role, AccountType: core.AccountType(accountType)}
	if status != "active" || !caller.hasAccountRole(roleAdmin) {
		return nil, errors.New("the account that reviewed setup is no longer an active administrator")
	}
	return caller, nil
}

func (p *Panel) serverSetupInternalPost(ctx context.Context, caller *Caller, handler func(http.ResponseWriter, *http.Request), path string, body any) (int, []byte, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return 0, nil, err
	}
	request, err := http.NewRequestWithContext(context.WithValue(ctx, callerKey, caller), http.MethodPost, path, bytes.NewReader(raw))
	if err != nil {
		return 0, nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.RemoteAddr = "127.0.0.1:0"
	response := &serverSetupInternalResponse{}
	handler(response, request)
	if response.code == 0 {
		response.code = http.StatusOK
	}
	return response.code, response.body.Bytes(), nil
}

func serverSetupBounded(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) > limit {
		value = value[:limit]
	}
	return value
}

// serverSetupAPIError reads the panel's coded error body.
func serverSetupAPIError(body []byte) (code, reason, message string) {
	var answer struct {
		Error  string `json:"error"`
		Code   string `json:"code"`
		Reason string `json:"reason"`
	}
	if json.Unmarshal(body, &answer) != nil {
		return "", "", serverSetupBounded(string(body), 400)
	}
	return serverSetupBounded(answer.Code, 64), serverSetupBounded(answer.Reason, 64), serverSetupBounded(answer.Error, 400)
}

func (p *Panel) runServerSetupMainSite(ctx context.Context, plan serverSetupPlan, step *serverSetupExecutionStep) (bool, error) {
	if err := p.requireServerSetupAdmission(ctx); err != nil {
		return false, err
	}
	outcome := &serverSetupMainSiteOutcome{Domain: step.Target, Certificate: "not_requested"}
	step.MainSite = outcome
	caller, err := p.serverSetupAdminCaller(ctx, plan.Actor)
	if err != nil {
		if !strings.HasPrefix(err.Error(), "the account") {
			return false, err
		}
		outcome.Site, outcome.Code, outcome.Message = "failed", "server_setup_main_site_account", err.Error()
		return true, nil
	}
	domainID, siteExists, err := p.serverSetupMainSiteRecord(ctx, step.Target)
	if err != nil {
		return false, err
	}
	switch {
	case domainID != 0 && !siteExists:
		// A DNS-only domain the owner made: kept as it is, never replaced.
		outcome.DomainID, outcome.Site, outcome.Code = domainID, "failed", "server_setup_main_site_dns_only"
		outcome.Message = "This domain already exists here without a website. Setup did not change it."
		return true, nil
	case domainID != 0:
		outcome.DomainID, outcome.Site = domainID, "existed"
	default:
		code, body, callErr := p.serverSetupInternalPost(ctx, caller, p.handleCreateDomain, "/api/v1/domains/create",
			map[string]string{"domain": step.Target, "project_type": "static", "ssl_type": "letsencrypt"})
		if callErr != nil {
			return false, callErr
		}
		// The create answers a partial success (409, DNS publication pending)
		// once the site exists; whatever it said, the record decides.
		domainID, siteExists, err = p.serverSetupMainSiteRecord(ctx, step.Target)
		if err != nil {
			return false, err
		}
		if domainID == 0 || !siteExists {
			outcome.Site = "failed"
			outcome.Code, outcome.Reason, outcome.Message = serverSetupAPIError(body)
			if outcome.Code == "" {
				outcome.Code = fmt.Sprintf("http_%d", code)
			}
			return true, nil
		}
		outcome.DomainID, outcome.Site = domainID, "created"
		if code != http.StatusOK {
			outcome.Code, outcome.Reason, outcome.Message = serverSetupAPIError(body)
		}
	}
	var active int
	if err := p.db.GetDB().QueryRowContext(ctx, `SELECT COUNT(*) FROM ssl_certificates WHERE domain_id=? AND status='active'`, outcome.DomainID).Scan(&active); err != nil {
		return false, err
	}
	if active > 0 {
		outcome.Certificate = "active"
		return true, nil
	}
	code, body, err := p.serverSetupInternalPost(ctx, caller, p.handleIssueLetsEncrypt, fmt.Sprintf("/api/v1/domains/%d/ssl/letsencrypt", outcome.DomainID),
		map[string]any{"auto_renew": true, "include_mail": false, "provider": "letsencrypt", "reissue": false, "redirect_http": true})
	if err != nil {
		return false, err
	}
	if code == http.StatusOK {
		var answer struct {
			Status string `json:"status"`
		}
		_ = json.Unmarshal(body, &answer)
		outcome.Certificate = "issued"
		if answer.Status == "waiting_for_owner" {
			outcome.Certificate = "waiting_owner"
		}
		return true, nil
	}
	outcome.Certificate = "pending"
	outcome.Code, outcome.Reason, outcome.Message = serverSetupAPIError(body)
	if outcome.Code == "" {
		outcome.Code = fmt.Sprintf("http_%d", code)
	}
	return true, nil
}

// serverSetupMainSiteRecord: the domain row and whether it has a site.
func (p *Panel) serverSetupMainSiteRecord(ctx context.Context, name string) (int, bool, error) {
	var id int
	err := p.db.GetDB().QueryRowContext(ctx, `SELECT id FROM domains WHERE name=?`, name).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	var sites int
	if err := p.db.GetDB().QueryRowContext(ctx, `SELECT COUNT(*) FROM sites WHERE domain_id=? AND COALESCE(project_type,'php')!='dnsonly'`, id).Scan(&sites); err != nil {
		return 0, false, err
	}
	return id, sites > 0, nil
}
