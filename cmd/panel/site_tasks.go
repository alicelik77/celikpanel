package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// A site's scheduled tasks as timers in its own limits (S4 R2b, 22 Nov
// 2026; the Agent's cmd/agent/site_tasks*.go, docs/SITE-SANDBOX.md section
// 9):
//
//   - Every Scheduled tasks request names the site (CronTenant.SiteID) and
//     carries its slice's limits; the Agent decides from its own files
//     whether the site's tasks run as timers (a moved site) or in cron.
//   - The administrator's "Move into the limits" on the site's Overview also
//     moves the site's crontab (preview first, nothing half-moved), and
//     "Put the tasks back into cron" undoes it; both are audited.
//   - "Move all": the sites still outside their limits (programs or tasks),
//     moved one after another by the screen with the same action.
//   - Problems now (administrator): the server owner's cron.allow that lets a
//     moved site's account use cron, the owner's deletion of CelikPanel's file
//     (with "close it again"), cron.allow not written (directory service,
//     failure), a crontab that came back for a moved site, site accounts that
//     linger, and the lingering rule undone or not written.
//
// Görevler sitenin kendi sınırları içinde zamanlayıcı olarak çalışır;
// yönetici taşır, geri alır, hepsini taşır; açık kalan kapılar Şu anki
// sorunlar'da görünür.

// cronTenantFor is the tenant of a domain's Scheduled tasks with its site
// and the site's slice (a domain without a site keeps SiteID 0: cron).
func (p *Panel) cronTenantFor(ctx context.Context, subscriptionID, domainID int, domain string) transport.CronTenant {
	tenant := transport.CronTenant{SubscriptionID: subscriptionID, DomainID: domainID, Domain: domain}
	if p == nil || p.db == nil {
		return tenant
	}
	var siteID int
	if err := p.db.GetDB().QueryRowContext(ctx, `SELECT id FROM sites WHERE domain_id = ?`, domainID).Scan(&siteID); err == nil && siteID > 0 {
		tenant.SiteID = siteID
		tenant.Site = p.siteSliceForSubscription(ctx, siteID, subscriptionID, domain)
	}
	return tenant
}

// Agent calls (tests replace them).
var (
	moveSiteTasksCall = func(p *Panel, ctx context.Context, method string, req *transport.SiteTasksMoveRequest) (transport.SiteTasksMoveResponse, error) {
		var resp transport.SiteTasksMoveResponse
		err := p.callAgentContext(ctx, method, req, &resp)
		return resp, err
	}
	siteTasksMovedCall = func(p *Panel, ctx context.Context, req *transport.SiteTasksMovedRequest) (transport.SiteTasksMovedResponse, error) {
		var resp transport.SiteTasksMovedResponse
		err := p.callAgentContext(ctx, "Agent.SiteTasksMovedV1", req, &resp)
		return resp, err
	}
)

// siteTasksView is the administrator's line about a site's tasks on its
// Overview card.
type siteTasksView struct {
	// Moved: the tasks run as timers in the site's limits; otherwise in cron.
	Moved bool `json:"moved"`
	// Reappeared: lines of a crontab that came back for the moved site's
	// account (running outside the limits); BySite: the account could have
	// installed it itself, so the site's own code can have made it
	// (otherwise only root could: the server owner's).
	Reappeared int  `json:"reappeared,omitempty"`
	BySite     bool `json:"by_site,omitempty"`
	// MailSubmission: whether the SMTP block lets sites reach this server's
	// own submission services (transport.MailSubmission*; "" not known).
	MailSubmission string `json:"mail_submission,omitempty"`
	// Recovery: what the Agent did with an interrupted tasks move at its
	// start ("<time> <outcome>").
	Recovery string `json:"recovery,omitempty"`
	// Unknown: the Agent did not answer (an older Agent, or none).
	Unknown bool `json:"unknown,omitempty"`
}

// siteTasksViewOf reads one site's tasks state.
func (p *Panel) siteTasksViewOf(ctx context.Context, siteID int) *siteTasksView {
	resp, err := siteTasksMovedCall(p, ctx, &transport.SiteTasksMovedRequest{SiteIDs: []int{siteID}})
	if err != nil || resp.Error != "" {
		return &siteTasksView{Unknown: true}
	}
	view := &siteTasksView{}
	for _, id := range resp.Moved {
		if id == siteID {
			view.Moved = true
		}
	}
	key := strconv.Itoa(siteID)
	view.Reappeared = resp.Reappeared[key]
	view.BySite = resp.ReappearedBySite[key]
	view.Recovery = resp.Recovery[key]
	view.MailSubmission = resp.MailSubmission
	return view
}

// handleDomainSiteTaskSource: GET (what it would contain) or POST
// {request_id} (rebuild) /api/v1/domains/{id}/site-limits/tasks-source
// (administrator): a moved site's task file that is missing while its
// timers remain is rebuilt from the copy of what CelikPanel last made the
// timers from (S4 R2b fix round).
func (p *Panel) handleDomainSiteTaskSource(w http.ResponseWriter, r *http.Request, domainID int) {
	w.Header().Set("Cache-Control", "private, no-store")
	if c := currentCaller(r); c == nil || c.Role != roleAdmin {
		writeClientError(w, http.StatusForbidden, "administrator access required")
		return
	}
	preview := r.Method == http.MethodGet
	requestID := ""
	if !preview {
		var req struct {
			RequestID string `json:"request_id"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil || !refreshRequestIDPattern.MatchString(req.RequestID) {
			writeClientError(w, http.StatusBadRequest, "a request id is required")
			return
		}
		requestID = req.RequestID
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	sites, err := p.siteLimitSites(ctx, domainID)
	if err != nil || len(sites) != 1 {
		writeClientError(w, http.StatusNotFound, "site not found for domain")
		return
	}
	site := sites[0]
	tenant := p.cronTenantFor(ctx, site.SubscriptionID, site.DomainID, site.Domain)
	resp, err := rebuildSiteTaskSourceCall(p, ctx, &transport.SiteTasksMoveRequest{
		ExpectedBuildCommit: strings.TrimSpace(buildCommit), RequestID: requestID, CronTenant: tenant, Preview: preview})
	if err != nil {
		writeServiceAppRefusal(w, http.StatusServiceUnavailable, "agent_unreachable",
			"The server did not answer, so the result is unknown. Read this page again before you try again.", nil)
		return
	}
	if !preview {
		p.auditDetailed(r, "site.tasks_source_rebuild", "domain", domainID, map[string]any{
			"request_id": requestID, "site": site.Domain, "outcome": resp.Outcome, "error": resp.Error,
		})
	}
	writeRightsJSON(w, http.StatusOK, resp)
}

var rebuildSiteTaskSourceCall = func(p *Panel, ctx context.Context, req *transport.SiteTasksMoveRequest) (transport.SiteTaskSourceResponse, error) {
	var resp transport.SiteTaskSourceResponse
	err := p.callAgentContext(ctx, "Agent.RebuildSiteTaskSourceV1", req, &resp)
	return resp, err
}

// moveSiteTasks moves (or, preview, describes) one site's tasks.
func (p *Panel) moveSiteTasks(ctx context.Context, site siteLimitSite, requestID string, preview bool) (*transport.SiteTasksMoveResponse, error) {
	tenant := p.cronTenantFor(ctx, site.SubscriptionID, site.DomainID, site.Domain)
	req := &transport.SiteTasksMoveRequest{ExpectedBuildCommit: strings.TrimSpace(buildCommit), RequestID: requestID, CronTenant: tenant, Preview: preview}
	resp, err := moveSiteTasksCall(p, ctx, "Agent.MoveSiteTasksV1", req)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

// handleDomainSiteLimitsPreview: GET /api/v1/domains/{id}/site-limits/preview
// (administrator): what the move would make of the site's crontab; nothing
// changes.
func (p *Panel) handleDomainSiteLimitsPreview(w http.ResponseWriter, r *http.Request, domainID int) {
	w.Header().Set("Cache-Control", "private, no-store")
	if c := currentCaller(r); c == nil || c.Role != roleAdmin {
		writeClientError(w, http.StatusForbidden, "administrator access required")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	sites, err := p.siteLimitSites(ctx, domainID)
	if err != nil || len(sites) != 1 {
		writeClientError(w, http.StatusNotFound, "site not found for domain")
		return
	}
	resp, err := p.moveSiteTasks(ctx, sites[0], "", true)
	if err != nil {
		writeServiceAppRefusal(w, http.StatusServiceUnavailable, "agent_unreachable",
			"The server did not answer, so what the move would do is not known. Read this page again.", nil)
		return
	}
	writeRightsJSON(w, http.StatusOK, resp)
}

// handleDomainSiteTasksUndo: POST /api/v1/domains/{id}/site-limits/tasks-undo
// {request_id} (administrator): the site's tasks go back into its crontab.
func (p *Panel) handleDomainSiteTasksUndo(w http.ResponseWriter, r *http.Request, domainID int) {
	w.Header().Set("Content-Type", "application/json")
	if c := currentCaller(r); c == nil || c.Role != roleAdmin {
		writeClientError(w, http.StatusForbidden, "administrator access required")
		return
	}
	var req struct {
		RequestID string `json:"request_id"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil || !refreshRequestIDPattern.MatchString(req.RequestID) {
		writeClientError(w, http.StatusBadRequest, "a request id is required")
		return
	}
	siteMoveMu.Lock()
	if answer, ok := siteTasksUndoAnswers[req.RequestID]; ok {
		siteMoveMu.Unlock()
		_ = json.NewEncoder(w).Encode(answer)
		return
	}
	if siteMoveRunning {
		siteMoveMu.Unlock()
		writeServiceAppRefusal(w, http.StatusConflict, "site_limits_move_running",
			"Nothing new was started: a site is being moved into its limits now. This page updates by itself when it is done.", nil)
		return
	}
	siteMoveRunning = true
	siteMoveMu.Unlock()
	defer func() {
		siteMoveMu.Lock()
		siteMoveRunning = false
		siteMoveMu.Unlock()
	}()
	ctx, cancel := hostingDurableContext(r.Context())
	defer cancel()
	sites, err := p.siteLimitSites(ctx, domainID)
	if err != nil || len(sites) != 1 {
		writeClientError(w, http.StatusNotFound, "site not found for domain")
		return
	}
	site := sites[0]
	tenant := p.cronTenantFor(ctx, site.SubscriptionID, site.DomainID, site.Domain)
	resp, err := moveSiteTasksCall(p, ctx, "Agent.UndoSiteTasksMoveV1", &transport.SiteTasksMoveRequest{
		ExpectedBuildCommit: strings.TrimSpace(buildCommit), RequestID: req.RequestID, CronTenant: tenant})
	if err != nil {
		writeServiceAppRefusal(w, http.StatusServiceUnavailable, "agent_unreachable",
			"The server did not answer, so the result is unknown. Read this page again before you try again.", nil)
		return
	}
	p.auditDetailed(r, "site.tasks_undo", "domain", domainID, map[string]any{
		"request_id": req.RequestID, "site": site.Domain, "outcome": resp.Outcome, "refusal": resp.Refusal, "error": resp.Error,
	})
	siteMoveMu.Lock()
	siteTasksUndoAnswers[req.RequestID] = resp
	if len(siteTasksUndoAnswers) > 32 {
		for k := range siteTasksUndoAnswers {
			delete(siteTasksUndoAnswers, k)
			break
		}
	}
	siteMoveMu.Unlock()
	if resp.Outcome == transport.SiteTasksUndone {
		p.noteSiteTasksOutside(ctx, map[int]bool{site.SiteID: true}, nil, time.Now())
	}
	_ = json.NewEncoder(w).Encode(resp)
}

var siteTasksUndoAnswers = map[string]transport.SiteTasksMoveResponse{}

// --- Sites still outside: programs or tasks ------------------------------------

// noteSiteTasksOutside brings the tasks part of the "outside" record up to
// date: outside lists sites whose tasks run in cron, inside those whose
// tasks moved (a whole reading passes every site in one of the two).
func (p *Panel) noteSiteTasksOutside(ctx context.Context, outside, inside map[int]bool, now time.Time) {
	siteLimitOutsideMu.Lock()
	defer siteLimitOutsideMu.Unlock()
	raw := p.setting(ctx, settingSiteLimitOutside)
	state := siteLimitOutside{}
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &state)
	}
	before, _ := json.Marshal(state.Tasks)
	if state.Tasks == nil {
		state.Tasks = map[string]bool{}
	}
	for id := range outside {
		state.Tasks[strconv.Itoa(id)] = true
	}
	for id := range inside {
		delete(state.Tasks, strconv.Itoa(id))
	}
	if len(state.Tasks) == 0 {
		state.Tasks = nil
	}
	after, _ := json.Marshal(state.Tasks)
	if string(before) == string(after) {
		return
	}
	state.CheckedAt = metricsTime(now)
	if next, err := json.Marshal(state); err == nil {
		_ = p.setSetting(ctx, settingSiteLimitOutside, string(next))
	}
}

// replaceSiteTasksOutside replaces the tasks part from a reading of every
// site.
func (p *Panel) replaceSiteTasksOutside(ctx context.Context, outside map[int]bool, now time.Time) {
	siteLimitOutsideMu.Lock()
	raw := p.setting(ctx, settingSiteLimitOutside)
	state := siteLimitOutside{}
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &state)
	}
	siteLimitOutsideMu.Unlock()
	inside := map[int]bool{}
	for key := range state.Tasks {
		if id, err := strconv.Atoi(key); err == nil && !outside[id] {
			inside[id] = true
		}
	}
	p.noteSiteTasksOutside(ctx, outside, inside, now)
}

// readAllSiteTasks reads which sites run their tasks as timers and records
// the others as outside (none on a host that cannot hold a site's limits).
func (p *Panel) readAllSiteTasks(ctx context.Context, all []siteLimitSite, unsupported string, now time.Time) {
	if unsupported != "" {
		p.replaceSiteTasksOutside(ctx, map[int]bool{}, now)
		return
	}
	resp, err := siteTasksMovedCall(p, ctx, &transport.SiteTasksMovedRequest{})
	if err != nil || resp.Error != "" {
		return // an older Agent: nothing to say about tasks yet
	}
	moved := map[int]bool{}
	for _, id := range resp.Moved {
		moved[id] = true
	}
	outside := map[int]bool{}
	for _, s := range all {
		if !moved[s.SiteID] {
			outside[s.SiteID] = true
		}
	}
	p.replaceSiteTasksOutside(ctx, outside, now)
}

// siteOutsideRow is one site still outside its limits (Move all).
type siteOutsideRow struct {
	DomainID int    `json:"domain_id"`
	Domain   string `json:"domain"`
	Programs int    `json:"programs,omitempty"`
	Tasks    bool   `json:"tasks,omitempty"`
}

// handleSiteLimitsOutside: GET /api/v1/site-limits/outside (administrator):
// the sites whose programs or tasks still run outside their limits, as the
// last reading said, for "Move all".
func (p *Panel) handleSiteLimitsOutside(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	if r.Method != http.MethodGet {
		writeClientError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if c := currentCaller(r); c == nil || c.Role != roleAdmin {
		writeClientError(w, http.StatusForbidden, "administrator access required")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	state := siteLimitOutside{}
	if raw := p.setting(ctx, settingSiteLimitOutside); raw != "" {
		_ = json.Unmarshal([]byte(raw), &state)
	}
	rows := []siteOutsideRow{}
	if p.db != nil {
		dbRows, err := p.db.GetDB().QueryContext(ctx, `SELECT s.id, d.id, d.name FROM sites s JOIN domains d ON d.id = s.domain_id ORDER BY d.name`)
		if err == nil {
			for dbRows.Next() {
				var siteID, domainID int
				var name string
				if dbRows.Scan(&siteID, &domainID, &name) != nil {
					continue
				}
				key := strconv.Itoa(siteID)
				row := siteOutsideRow{DomainID: domainID, Domain: name, Programs: state.Sites[key], Tasks: state.Tasks[key]}
				if row.Programs > 0 || row.Tasks {
					rows = append(rows, row)
				}
			}
			dbRows.Close()
		}
	}
	writeRightsJSON(w, http.StatusOK, map[string]any{"checked_at": state.CheckedAt, "sites": rows})
}

// --- Problems now: cron.allow, crontabs that came back, lingering ----------------

// siteCronProblems turns the step's answer about cron.allow into Problems now.
func siteCronProblems(state transport.SideDoorsState) []monitoringProblem {
	c := state.Cron
	if c == nil {
		return nil
	}
	action := monitoringProblemAction{Kind: "open", To: "/monitoring"}
	var problems []monitoringProblem
	switch c.State {
	case transport.SideDoorsAtOwnerFile:
		if len(c.SiteAccounts) > 0 {
			subject := strings.Join(c.SiteAccounts, ", ")
			problems = append(problems, monitoringProblem{Key: "cron_allow_lists_sites", Kind: "cron_allow_lists_sites", Severity: "warning",
				Actor: "server_owner", Name: c.Path, Subject: subject, Value: float64(len(c.SiteAccounts)), ObservedAt: state.CheckedAt, Action: action})
		}
	case transport.SideDoorsAtDirectory, transport.SideDoorsAtFailed:
		problems = append(problems, monitoringProblem{Key: "cron_not_closed", Kind: "cron_not_closed", Severity: "warning",
			Actor: "server_owner", Name: c.Detail, Subject: c.Path, ObservedAt: state.CheckedAt, Action: action})
	case transport.SideDoorsAtOwnerRemoved:
		problems = append(problems, monitoringProblem{Key: "cron_owner_removed", Kind: "cron_owner_removed", Severity: "warning",
			Actor: "administrator", Name: c.Path, Subject: c.Path, ObservedAt: state.CheckedAt,
			Action: monitoringProblemAction{Kind: "reclose", To: transport.SideDoorsReCloseCron}})
	}
	// A crontab that came back for a moved site: one line for those only root
	// could have made (the server owner's), one for those the site's own code
	// can have made (cron's access files let the account in at some point).
	for _, bySite := range []bool{false, true} {
		var accounts []string
		total := 0
		for account, n := range c.Reappeared {
			if c.ReappearedBySite[account] == bySite {
				accounts = append(accounts, account)
				total += n
			}
		}
		if len(accounts) == 0 {
			continue
		}
		sort.Strings(accounts)
		if len(accounts) > 5 {
			accounts = append(accounts[:5], "…")
		}
		subject := strings.Join(accounts, ", ")
		kind := "site_crontab_returned"
		if bySite {
			kind = "site_crontab_by_site"
		}
		problems = append(problems, monitoringProblem{Key: kind, Kind: kind, Severity: "warning",
			Actor: "server_owner", Name: subject, Subject: subject, Value: float64(total), ObservedAt: state.CheckedAt, Action: action})
	}
	return problems
}

// siteTasksHeldProblems: moved sites whose timers the start held because
// their limits file is not in place and could not be written (S4 R2b fix
// round: no task runs without the site's limits).
func siteTasksHeldProblems(state transport.SideDoorsState) []monitoringProblem {
	if len(state.TasksHeld) == 0 {
		return nil
	}
	var names []string
	detail := ""
	for _, h := range state.TasksHeld {
		name := h.Domain
		if name == "" {
			name = "site " + strconv.Itoa(h.SiteID)
		}
		names = append(names, name)
		if detail == "" {
			detail = h.Detail
		}
	}
	sort.Strings(names)
	if len(names) > 5 {
		names = append(names[:5], "…")
	}
	subject := strings.Join(names, ", ")
	return []monitoringProblem{{Key: "site_tasks_held", Kind: "site_tasks_held", Severity: "critical", Actor: "administrator",
		Name: detail, Subject: subject, Value: float64(len(state.TasksHeld)), ObservedAt: state.CheckedAt,
		Action: monitoringProblemAction{Kind: "open", To: "/monitoring"}}}
}

// siteLingerProblems turns the step's answer about lingering into Problems
// now: the accounts that linger (with the command that ends it), and the
// rule undone or not written.
func siteLingerProblems(state transport.SideDoorsState) []monitoringProblem {
	l := state.Linger
	if l == nil {
		return nil
	}
	action := monitoringProblemAction{Kind: "open", To: "/monitoring"}
	var problems []monitoringProblem
	if len(l.Accounts) > 0 {
		accounts := l.Accounts
		command := "loginctl disable-linger " + strings.Join(accounts, " ")
		if len(accounts) > 5 {
			accounts = append(append([]string{}, accounts[:5]...), "…")
		}
		subject := strings.Join(accounts, ", ")
		problems = append(problems, monitoringProblem{Key: "site_lingering", Kind: "site_lingering", Severity: "warning",
			Actor: "server_owner", Name: command, Subject: subject, Value: float64(len(l.Accounts)), ObservedAt: state.CheckedAt, Action: action})
	}
	switch l.State {
	case transport.SideDoorsAtOwnerRemoved:
		problems = append(problems, monitoringProblem{Key: "linger_owner_removed", Kind: "linger_owner_removed", Severity: "warning",
			Actor: "administrator", Name: l.Path, Subject: l.Path, ObservedAt: state.CheckedAt,
			Action: monitoringProblemAction{Kind: "reclose", To: transport.SideDoorsReCloseLinger}})
	case transport.SideDoorsAtFailed, transport.SideDoorsAtOwnerFile:
		problems = append(problems, monitoringProblem{Key: "linger_not_closed", Kind: "linger_not_closed", Severity: "warning",
			Actor: "server_owner", Name: l.Detail, Subject: l.Path, ObservedAt: state.CheckedAt, Action: action})
	}
	return problems
}

// siteLimitsOutsideProblem is ONE line for the sites whose programs or tasks
// still run outside their limits; its action opens "Move all".
func siteLimitsOutsideProblem(raw string, names map[string]string) []monitoringProblem {
	if raw == "" {
		return nil
	}
	var state siteLimitOutside
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		return nil
	}
	set := map[string]bool{}
	for key, n := range state.Sites {
		if n > 0 && names[key] != "" {
			set[names[key]] = true
		}
	}
	for key, outside := range state.Tasks {
		if outside && names[key] != "" {
			set[names[key]] = true
		}
	}
	if len(set) == 0 {
		return nil
	}
	sites := make([]string, 0, len(set))
	for name := range set {
		sites = append(sites, name)
	}
	sort.Strings(sites)
	listed := sites
	if len(listed) > 5 {
		listed = append(append([]string{}, sites[:5]...), "…")
	}
	subject := strings.Join(listed, ", ")
	return []monitoringProblem{{
		Key: "site_limits_outside", Kind: "site_limits_outside", Severity: "warning", Actor: "administrator",
		Name: subject, Subject: subject, Value: float64(len(sites)), ObservedAt: state.CheckedAt,
		Action: monitoringProblemAction{Kind: "move_all", To: "/domains/" + url.PathEscape(sites[0])},
	}}
}
