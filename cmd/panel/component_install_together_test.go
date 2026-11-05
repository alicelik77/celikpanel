package main

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"
)

func installPlanStates(plan installPlan) []string {
	out := []string{}
	for _, requirement := range plan.Requires {
		line := requirement.Need + "=" + requirement.Component + ":" + requirement.State
		if requirement.Reason != "" {
			line += ":" + requirement.Reason
		}
		out = append(out, line)
	}
	return out
}

func hostWith(installed ...string) installPlanHost {
	set := map[string]bool{}
	for _, id := range installed {
		set[id] = true
	}
	return installPlanHost{Installed: set, Known: true, Blocked: func(string) string { return "" }}
}

// alpha.124 (D-045 follow-up): the install window's answer to "what does this
// component need?", at component level.
func TestInstallPlanSaysEachRequirementAtComponentLevel(t *testing.T) {
	plan := planComponentInstall("phpmyadmin", hostWith())
	if got := installPlanStates(plan); !slices.Equal(got, []string{
		"mariadb=mariadb:together", "web-server=nginx:together", "php-fpm=php-fpm:together",
	}) || plan.Blocked || !slices.Equal(plan.Together, []string{"mariadb", "nginx", "php-fpm"}) {
		t.Fatalf("empty server: %v together %v blocked %v", got, plan.Together, plan.Blocked)
	}
	if plan.TogetherQualifier() != "mariadb,nginx,php-fpm" {
		t.Fatalf("qualifier %q", plan.TogetherQualifier())
	}

	// Apache fills the web server role: installed, nothing more for it.
	plan = planComponentInstall("phpmyadmin", hostWith("apache", "mariadb", "php-fpm"))
	if got := installPlanStates(plan); !slices.Equal(got, []string{
		"mariadb=mariadb:installed", "web-server=apache:installed", "php-fpm=php-fpm:installed",
	}) || len(plan.Together) != 0 || plan.Blocked {
		t.Fatalf("apache: %v", got)
	}

	// Mail engines come with the mail setup: said, never installed here.
	plan = planComponentInstall("roundcube", hostWith("nginx", "php-fpm"))
	if got := installPlanStates(plan); !slices.Equal(got, []string{
		"smtp-server=postfix:unavailable:mail_setup", "imap-server=dovecot:unavailable:mail_setup",
		"web-server=nginx:installed", "php-fpm=php-fpm:installed",
	}) || !plan.Blocked || len(plan.Together) != 0 {
		t.Fatalf("roundcube: %v blocked %v", got, plan.Blocked)
	}

	// Fail2ban needs the firewall engine, installed together.
	plan = planComponentInstall("fail2ban", hostWith())
	if !slices.Equal(plan.Together, []string{"nftables"}) || plan.Blocked {
		t.Fatalf("fail2ban: %v", installPlanStates(plan))
	}

	// Not offered on this system: said with the reason.
	host := hostWith()
	host.Blocked = func(id string) string {
		if id == "nginx" || id == "apache" {
			return "not here"
		}
		return ""
	}
	plan = planComponentInstall("node", host)
	if got := installPlanStates(plan); !slices.Equal(got, []string{"web-server=nginx:unavailable:not_offered"}) || !plan.Blocked {
		t.Fatalf("not offered: %v", got)
	}

	// Never observed: unknown, nothing is planned.
	plan = planComponentInstall("node", installPlanHost{Installed: map[string]bool{}})
	if got := installPlanStates(plan); !slices.Equal(got, []string{"web-server=:unknown"}) || !plan.Blocked {
		t.Fatalf("unknown: %v", got)
	}

	// Nothing needed: an empty answer.
	plan = planComponentInstall("redis", hostWith())
	if len(plan.Requires) != 0 || plan.Blocked || len(plan.Together) != 0 {
		t.Fatalf("redis: %+v", plan)
	}
}

// The install-together operation: refused unless the owner confirmed exactly
// the plan's list, then requirements first and the component last, as one
// durable operation.
func TestInstallTogetherInstallsRequirementsFirstOnTheServer(t *testing.T) {
	f := newServiceOperationTestFixture(t)
	// The fake Agent starts no unit by itself: Fail2ban's runs once installed.
	f.agent.mu.Lock()
	f.agent.active["fail2ban"] = true
	f.agent.mu.Unlock()

	// A list that is not the plan's: refused, nothing runs.
	recorder, _ := postServiceInstallRequest(t, f, serviceInstallRequest{
		ServiceID: "fail2ban", RequestID: mustServiceOperationRequestID(t), InstallTogether: []string{"nginx"},
	})
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("wrong list: %d %s", recorder.Code, recorder.Body.String())
	}
	if calls := f.agent.installCalls.Load(); calls != 0 {
		t.Fatalf("installs ran: %d", calls)
	}

	recorder, op := postServiceInstallRequest(t, f, serviceInstallRequest{
		ServiceID: "fail2ban", RequestID: mustServiceOperationRequestID(t), InstallTogether: []string{"nftables"},
	})
	if recorder.Code != http.StatusAccepted || op == nil || op.Kind != serviceOperationKindInstallTogether || op.PackageName != "nftables" {
		t.Fatalf("confirmed: %d %s", recorder.Code, recorder.Body.String())
	}
	done, body := waitForServiceOperation(t, f.panel, f.userID, op.ID, serviceOperationSucceeded)
	var result struct {
		Together []string `json:"installed_together"`
		Target   string   `json:"target"`
	}
	if err := json.Unmarshal(done.Result, &result); err != nil || !slices.Equal(result.Together, []string{"nftables"}) || result.Target != "fail2ban" {
		t.Fatalf("result = %s", body)
	}
	f.agent.mu.Lock()
	order := []string{}
	for _, request := range f.agent.installRequests {
		order = append(order, request.ID)
	}
	f.agent.mu.Unlock()
	if !slices.Equal(order, []string{"nftables", "fail2ban"}) {
		t.Fatalf("install order = %v", order)
	}

	// Asked again with nftables installed now: the plan has nothing to
	// install together, so the old list is refused with the new plan.
	f.agent.mu.Lock()
	f.agent.installed["fail2ban"] = false
	f.agent.mu.Unlock()
	recorder, _ = postServiceInstallRequest(t, f, serviceInstallRequest{
		ServiceID: "fail2ban", RequestID: mustServiceOperationRequestID(t), InstallTogether: []string{"nftables"},
	})
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), errCodeInstallPlanChanged) {
		t.Fatalf("changed plan: %d %s", recorder.Code, recorder.Body.String())
	}
}

// A failure keeps what was installed before it and says so.
func TestInstallTogetherFailureKeepsEarlierInstalls(t *testing.T) {
	f := newServiceOperationTestFixture(t)
	ctx := serviceOperationBoundContext()
	f.agent.mu.Lock()
	f.agent.installed["nftables"] = false
	f.agent.mu.Unlock()
	result, failure := f.panel.runInstallTogether(ctx, serviceInstallRequest{ServiceID: "fail2ban", InstallTogether: []string{"nftables"}},
		[]string{"nftables"}, func(string) error {
			f.agent.mu.Lock()
			if f.agent.installed["nftables"] {
				f.agent.installError = "fail2ban package failed"
			}
			f.agent.mu.Unlock()
			return nil
		})
	if failure == nil || !strings.Contains(failure.Message, "Installed before this step and kept: ") {
		t.Fatalf("failure = %+v result %v", failure, result)
	}
	if got := result["installed_together"].([]string); !slices.Equal(got, []string{"nftables"}) {
		t.Fatalf("kept = %v", got)
	}
}

// alpha.124 (P1d): one PHP version that a site uses is refused, and the
// refusal names the PHP tools on it too; tools alone never refuse.
func TestPHPVersionRefusalNamesBoundTools(t *testing.T) {
	db := newRemovalPlanTestDB(t)
	facts := removalHostFacts{PHPTools: map[string]string{"phpmyadmin": "8.3", "roundcube": "8.4"}}
	plan := planFor(t, db, "php-fpm", "8.3", nil, facts)
	if got := blockerKinds(plan); !slices.Equal(got, []string{"php_sites", "php_tools"}) {
		t.Fatalf("kinds = %v", got)
	}
	tools := plan.Blockers[1]
	if !slices.Equal(itemNames(tools), []string{"phpMyAdmin"}) || tools.Items[0].Link != "/components/phpmyadmin" {
		t.Fatalf("tools = %+v", tools)
	}
	// A version only a tool runs on: no refusal (the tool moves with it).
	plan = planFor(t, db, "php-fpm", "8.5", nil, removalHostFacts{PHPTools: map[string]string{"phpmyadmin": "8.5"}})
	if len(plan.Blockers) != 0 {
		t.Fatalf("tool alone refused: %v", blockerKinds(plan))
	}
}
