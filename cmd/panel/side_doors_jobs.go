package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Scheduled jobs a site's own code can make (S4 R1, 21 Nov 2026; the
// Agent's cmd/agent/side_doors_jobs.go and site_crontab_record.go), shown to
// the administrator in Problems now:
//
//   - site crontabs with lines not made in CelikPanel: ONE line naming the
//     sites and how many lines; the lines themselves are marked on each
//     site's Scheduled tasks list, where they are removed like tasks. The
//     side doors step counts them at each start, and every read of a site's
//     tasks brings that site's count up to date, so a removal clears it.
//   - `at`: pending jobs of site accounts (what, whose, when), an owner's
//     /etc/at.allow that lets site accounts use at, and at left open because
//     the accounts come from a directory service or the file could not be
//     written.
//
// Nothing is blocked or removed by itself; a crontab the server owner edited
// by hand is the same case and is worded "changed outside CelikPanel"
// (D-022).
//
// Sitenin kendi kodunun kurabileceği zamanlanmış işler yöneticiye Şu anki
// sorunlar'da gösterilir; hiçbir şey kendiliğinden silinmez.

const settingSiteCrontabsOutside = "site_crontabs_outside_v1"

// siteCrontabsOutside: domain id -> lines not made in CelikPanel.
type siteCrontabsOutside struct {
	CheckedAt string         `json:"checked_at"`
	Sites     map[string]int `json:"sites"`
	// Unreadable (S4 R1 fix round): account -> why its crontab could not be
	// read at the last step.
	Unreadable map[string]string `json:"unreadable,omitempty"`
}

var siteCrontabsOutsideMu sync.Mutex

func (p *Panel) readSiteCrontabsOutside(ctx context.Context) siteCrontabsOutside {
	state := siteCrontabsOutside{Sites: map[string]int{}}
	if raw := p.setting(ctx, settingSiteCrontabsOutside); raw != "" {
		_ = json.Unmarshal([]byte(raw), &state)
		if state.Sites == nil {
			state.Sites = map[string]int{}
		}
	}
	return state
}

func (p *Panel) writeSiteCrontabsOutside(ctx context.Context, state siteCrontabsOutside) {
	raw, err := json.Marshal(state)
	if err != nil {
		return
	}
	if err := p.setSetting(ctx, settingSiteCrontabsOutside, string(raw)); err != nil {
		log.Printf("site crontabs: the count of lines not made in CelikPanel was not recorded: %v", err)
	}
}

// recordSiteCrontabsOutside keeps the step's count for every site.
func (p *Panel) recordSiteCrontabsOutside(ctx context.Context, checkedAt string, rows []transport.SideDoorsSiteCrontab) {
	if p == nil || p.db == nil {
		return
	}
	siteCrontabsOutsideMu.Lock()
	defer siteCrontabsOutsideMu.Unlock()
	state := siteCrontabsOutside{CheckedAt: checkedAt, Sites: map[string]int{}}
	for _, row := range rows {
		if row.DomainID > 0 && row.Outside > 0 {
			state.Sites[strconv.Itoa(row.DomainID)] = row.Outside
		}
		if row.Unreadable != "" && row.Account != "" && len(state.Unreadable) < 50 {
			if state.Unreadable == nil {
				state.Unreadable = map[string]string{}
			}
			state.Unreadable[row.Account] = row.Unreadable
		}
	}
	p.writeSiteCrontabsOutside(ctx, state)
}

// noteSiteCrontabOutside brings one site's count up to date after a read of
// its tasks.
func (p *Panel) noteSiteCrontabOutside(ctx context.Context, domainID, outside int) {
	if p == nil || p.db == nil || domainID <= 0 {
		return
	}
	siteCrontabsOutsideMu.Lock()
	defer siteCrontabsOutsideMu.Unlock()
	state := p.readSiteCrontabsOutside(ctx)
	key := strconv.Itoa(domainID)
	if state.Sites[key] == outside || (outside == 0 && state.Sites[key] == 0) {
		return
	}
	if outside > 0 {
		state.Sites[key] = outside
	} else {
		delete(state.Sites, key)
	}
	p.writeSiteCrontabsOutside(ctx, state)
}

// siteCrontabOutsideProblem is the ONE line for every site with lines not made
// in CelikPanel. names maps domain ids to names; a site that is gone is left
// out.
func siteCrontabOutsideProblem(state siteCrontabsOutside, names map[int]string) []monitoringProblem {
	type site struct {
		name    string
		outside int
	}
	var sites []site
	total := 0
	for key, outside := range state.Sites {
		id, err := strconv.Atoi(key)
		name := names[id]
		if err != nil || name == "" || outside <= 0 {
			continue
		}
		sites = append(sites, site{name, outside})
		total += outside
	}
	if len(sites) == 0 {
		return nil
	}
	sort.Slice(sites, func(i, j int) bool { return sites[i].name < sites[j].name })
	listed := make([]string, 0, len(sites))
	for i, s := range sites {
		if i == 10 {
			listed = append(listed, "…")
			break
		}
		listed = append(listed, fmt.Sprintf("%s (%d)", s.name, s.outside))
	}
	subject := strings.Join(listed, ", ")
	return []monitoringProblem{{
		Key: "site_crontab_outside", Kind: "site_crontab_outside", Severity: "warning",
		Actor: "administrator", Name: subject, Subject: subject, Value: float64(total), ObservedAt: state.CheckedAt,
		Action: monitoringProblemAction{Kind: "open", To: "/domains/" + url.PathEscape(sites[0].name) + "?tab=cron"},
	}}
}

// siteCrontabUnreadableProblem is ONE line naming the site accounts whose
// crontab could not be read, and why (S4 R1 fix round).
func siteCrontabUnreadableProblem(state siteCrontabsOutside) []monitoringProblem {
	if len(state.Unreadable) == 0 {
		return nil
	}
	accounts := make([]string, 0, len(state.Unreadable))
	for account := range state.Unreadable {
		accounts = append(accounts, account)
	}
	sort.Strings(accounts)
	why := state.Unreadable[accounts[0]]
	if len(accounts) > 5 {
		accounts = append(accounts[:5], "…")
	}
	subject := strings.Join(accounts, ", ")
	return []monitoringProblem{{
		Key: "site_crontab_unreadable", Kind: "site_crontab_unreadable", Severity: "warning", Actor: "server_owner",
		Name: why, Subject: subject, Value: float64(len(state.Unreadable)), ObservedAt: state.CheckedAt,
		Action: monitoringProblemAction{Kind: "open", To: "/monitoring"},
	}}
}

// siteAtProblems turns the step's answer about at into Problems now.
func siteAtProblems(state transport.SideDoorsState) []monitoringProblem {
	at := state.At
	if at == nil {
		return nil
	}
	var problems []monitoringProblem
	action := monitoringProblemAction{Kind: "open", To: "/monitoring"}
	if len(at.Jobs) > 0 {
		listed := make([]string, 0, len(at.Jobs))
		for i, job := range at.Jobs {
			if i == 5 {
				listed = append(listed, "…")
				break
			}
			// Number, account and time: the commands are the site's own
			// text and are never copied here (S4 R1 fix round).
			listed = append(listed, "#"+job.ID+" "+job.Account+", "+job.When)
		}
		subject := strings.Join(listed, "; ")
		problems = append(problems, monitoringProblem{
			Key: "site_at_jobs", Kind: "site_at_jobs", Severity: "warning", Actor: "administrator",
			Name: subject, Subject: subject, Value: float64(len(at.Jobs)), ObservedAt: state.CheckedAt, Action: action,
		})
	}
	switch at.State {
	case transport.SideDoorsAtOwnerFile:
		if len(at.SiteAccounts) > 0 {
			subject := strings.Join(at.SiteAccounts, ", ")
			problems = append(problems, monitoringProblem{
				Key: "at_allow_lists_sites", Kind: "at_allow_lists_sites", Severity: "warning", Actor: "server_owner",
				Name: subject, Subject: subject, Value: float64(len(at.SiteAccounts)), ObservedAt: state.CheckedAt, Action: action,
			})
		}
	case transport.SideDoorsAtDirectory:
		problems = append(problems, monitoringProblem{
			Key: "at_directory_accounts", Kind: "at_directory_accounts", Severity: "warning", Actor: "server_owner",
			Name: at.Path, Subject: at.Path, ObservedAt: state.CheckedAt, Action: action,
		})
	case transport.SideDoorsAtFailed:
		problems = append(problems, monitoringProblem{
			Key: "at_not_closed", Kind: "at_not_closed", Severity: "warning", Actor: "server_owner",
			Name: at.Path, Subject: at.Path, ObservedAt: state.CheckedAt, Action: action,
		})
	case transport.SideDoorsAtOwnerRemoved:
		// S4 R1 fix round: the owner deleted CelikPanel's file; it is not
		// written again by itself, and closing it again is explicit.
		problems = append(problems, monitoringProblem{
			Key: "at_owner_removed", Kind: "at_owner_removed", Severity: "warning", Actor: "administrator",
			Name: at.Path, Subject: at.Path, ObservedAt: state.CheckedAt,
			Action: monitoringProblemAction{Kind: "reclose", To: transport.SideDoorsReCloseAt},
		})
	}
	return problems
}

// siteJobsProblems: both, from what is recorded (a read only).
func (p *Panel) siteJobsProblems(ctx context.Context, db *sql.DB) []monitoringProblem {
	var problems []monitoringProblem
	if raw := p.setting(ctx, settingSideDoorsState); raw != "" {
		var state transport.SideDoorsState
		if json.Unmarshal([]byte(raw), &state) == nil {
			problems = append(problems, siteAtProblems(state)...)
			// S4 R2b: cron.allow, crontabs that came back, lingering.
			problems = append(problems, siteCronProblems(state)...)
			problems = append(problems, siteLingerProblems(state)...)
		}
	}
	outside := p.readSiteCrontabsOutside(ctx)
	problems = append(problems, siteCrontabUnreadableProblem(outside)...)
	if len(outside.Sites) == 0 || db == nil {
		return problems
	}
	names := map[int]string{}
	for key := range outside.Sites {
		id, err := strconv.Atoi(key)
		if err != nil || len(names) >= 200 {
			continue
		}
		var name string
		if db.QueryRowContext(ctx, `SELECT name FROM domains WHERE id = ?`, id).Scan(&name) == nil {
			names[id] = name
		}
	}
	return append(problems, siteCrontabOutsideProblem(outside, names)...)
}

// auditSiteAt records what the step did about at.
func (p *Panel) auditSiteAt(at *transport.SideDoorsAt) {
	if at == nil {
		return
	}
	details := auditDetailsText(map[string]any{"path": at.Path, "detail": at.Detail, "automatic": true})
	switch at.State {
	case transport.SideDoorsAtWritten:
		p.recordAudit(auditRecord{Action: "at.allow.written", ResourceType: "service", Details: details})
	case transport.SideDoorsAtFailed, transport.SideDoorsAtDirectory:
		p.recordAudit(auditRecord{Action: "at.allow.failed", ResourceType: "service", Details: details})
	}
}

// siteJobsCheckedAt is the step's time, or now for an older answer.
func siteJobsCheckedAt(state *transport.SideDoorsState) string {
	if state != nil && state.CheckedAt != "" {
		return state.CheckedAt
	}
	return time.Now().UTC().Format(time.RFC3339)
}
